//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package models

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// Default Bedrock URL suffixes used when conf/models/bedrock.json
// does not override them. Hard-coding sane defaults lets the driver
// still work when the configuration loader supplies a zero-value
// URLSuffix, while honouring an operator override (e.g. fronting
// Bedrock through a corporate VPC endpoint at a non-AWS path).
const (
	defaultBedrockChatSuffix          = "converse"
	defaultBedrockStreamSuffix        = "converse-stream"
	defaultBedrockListModelsSuffix    = "foundation-models"
	bedrockStreamSuffixSuffix         = "-stream"
)

// Bedrock signing services and endpoint hostnames.
//
// The control plane (foundation-model catalog) and the runtime plane
// (Converse / Converse-Stream) sit on different subdomains and sign
// against different SigV4 service names.
const (
	bedrockRuntimeService  = "bedrock-runtime"
	bedrockControlService  = "bedrock"
	bedrockRuntimeHostTmpl = "bedrock-runtime.%s.amazonaws.com"
	bedrockControlHostTmpl = "bedrock.%s.amazonaws.com"
)

// Bedrock authentication modes mirroring the Python LiteLLMBase
// dispatch at rag/llm/chat_model.py:1872. The API key is stored as a
// JSON blob containing one of these modes plus its required fields.
const (
	bedrockAuthAccessKey  = "access_key_secret"
	bedrockAuthIAMRole    = "iam_role"
	bedrockAuthAssumeRole = "assume_role"
)

// bedrockAssumeRoleSession identifies temporary sessions in CloudTrail
// when iam_role mode triggers STS AssumeRole. Matches the Python
// implementation's RoleSessionName so audit logs stay consistent.
const bedrockAssumeRoleSession = "BedrockSession"

// BedrockModel implements ModelDriver for AWS Bedrock.
//
// Bedrock is AWS-signed (SigV4) rather than OpenAI-compatible, so this
// driver differs from the SaaS cluster in three ways:
//   - Authentication uses AWS SigV4 over an access key + secret (and
//     optionally a session token), not a static Bearer token.
//   - The "api key" is a JSON blob carrying auth_mode, region, and the
//     mode-specific credential material (access_key_secret /
//     iam_role / assume_role). This mirrors the Python implementation
//     at rag/llm/chat_model.py:1872.
//   - The streaming response uses the AWS event-stream binary framing
//     (vnd.amazon.eventstream), not Server-Sent Events. Each frame is
//     decoded with the aws-sdk-go-v2 eventstream package.
//
// The base URL is computed from the configured region rather than
// supplied from conf/models/bedrock.json, because every Bedrock region
// has its own endpoint and the URL is fully determined by the region
// in the API key.
type BedrockModel struct {
	BaseURL    map[string]string
	URLSuffix  URLSuffix
	httpClient *http.Client
}

// NewBedrockModel creates a new Bedrock model instance.
//
// We clone http.DefaultTransport to keep Go's defaults for
// ProxyFromEnvironment, DialContext (with KeepAlive), HTTP/2,
// TLSHandshakeTimeout, and ExpectContinueTimeout, and only override
// the connection-pool fields we care about.
//
// The Client itself has no overall Timeout because Bedrock
// Converse-Stream is long-lived. http.Client.Timeout would also cap
// time spent reading the response body, cutting off mid-stream.
// Non-streaming callers wrap each request in context.WithTimeout
// instead, and ResponseHeaderTimeout still caps connection setup.
func NewBedrockModel(baseURL map[string]string, urlSuffix URLSuffix) *BedrockModel {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = 90 * time.Second
	transport.DisableCompression = false
	transport.ResponseHeaderTimeout = 60 * time.Second

	return &BedrockModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
		httpClient: &http.Client{
			Transport: transport,
		},
	}
}

// NewInstance returns a fresh BedrockModel bound to the supplied
// BaseURL map. Used by the factory layer when adding a tenant
// instance with a custom endpoint override (e.g. a VPC endpoint
// fronting Bedrock for compliance reasons).
func (b *BedrockModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewBedrockModel(baseURL, b.URLSuffix)
}

// Name returns the canonical lower-case provider name used by the
// factory dispatch and by conf/models/bedrock.json.
func (b *BedrockModel) Name() string {
	return "bedrock"
}

// bedrockKey carries the parsed contents of the API key JSON blob.
// Every auth_mode has a different shape, so non-applicable fields
// are simply left zero-valued; resolution time validates them.
type bedrockKey struct {
	AuthMode    string `json:"auth_mode"`
	Region      string `json:"bedrock_region"`
	AccessKey   string `json:"bedrock_ak"`
	SecretKey   string `json:"bedrock_sk"`
	AWSRoleARN  string `json:"aws_role_arn"`
	ExternalID  string `json:"aws_external_id"`
	SessionName string `json:"role_session_name"`
}

// parseBedrockKey decodes the JSON blob stored in APIConfig.ApiKey
// and validates that all fields required by the chosen auth_mode are
// present. Returning the typed struct lets the rest of the driver
// avoid touching the raw blob; returning a clear error on missing
// fields lets the operator fix configuration without reading code.
func parseBedrockKey(raw string) (*bedrockKey, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("bedrock: api key is required")
	}
	var key bedrockKey
	if err := json.Unmarshal([]byte(raw), &key); err != nil {
		return nil, fmt.Errorf("bedrock: api key must be a JSON object: %w", err)
	}
	if key.AuthMode == "" {
		return nil, fmt.Errorf("bedrock: auth_mode is required")
	}
	switch key.AuthMode {
	case bedrockAuthAccessKey:
		if key.AccessKey == "" || key.SecretKey == "" {
			return nil, fmt.Errorf("bedrock: access_key_secret mode requires bedrock_ak and bedrock_sk")
		}
	case bedrockAuthIAMRole:
		if key.AWSRoleARN == "" {
			return nil, fmt.Errorf("bedrock: iam_role mode requires aws_role_arn")
		}
	case bedrockAuthAssumeRole:
		// Default credential chain handles its own validation when
		// we ask config.LoadDefaultConfig to materialize credentials.
	default:
		return nil, fmt.Errorf("bedrock: unsupported auth_mode %q", key.AuthMode)
	}
	return &key, nil
}

// resolveRegion picks the region to use for both endpoint selection
// and SigV4 signing. The APIConfig override wins (callers can route
// the same instance to a different region), falling back to the
// region declared inside the API key JSON.
func resolveBedrockRegion(apiConfig *APIConfig, key *bedrockKey) (string, error) {
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		return *apiConfig.Region, nil
	}
	if key.Region == "" {
		return "", fmt.Errorf("bedrock: region is required (set apiConfig.Region or bedrock_region in the API key)")
	}
	return key.Region, nil
}

// resolveBedrockCredentials returns AWS credentials for the chosen
// auth mode. For static keys this is a one-liner; for iam_role we
// load the default credential chain and ask STS to assume the role,
// matching boto3 sts_client.assume_role at rag/llm/chat_model.py:1893.
// For assume_role we simply expose the default chain (IRSA, instance
// profile, etc.) — same semantics as the Python "default credential
// chain" comment.
func resolveBedrockCredentials(ctx context.Context, key *bedrockKey, region string) (awssdk.Credentials, error) {
	switch key.AuthMode {
	case bedrockAuthAccessKey:
		return awssdk.Credentials{
			AccessKeyID:     key.AccessKey,
			SecretAccessKey: key.SecretKey,
			Source:          "ragflow/bedrock/access_key_secret",
		}, nil
	case bedrockAuthIAMRole:
		return assumeBedrockRole(ctx, key, region)
	case bedrockAuthAssumeRole:
		cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
		if err != nil {
			return awssdk.Credentials{}, fmt.Errorf("bedrock: load default AWS config: %w", err)
		}
		creds, err := cfg.Credentials.Retrieve(ctx)
		if err != nil {
			return awssdk.Credentials{}, fmt.Errorf("bedrock: retrieve default credentials: %w", err)
		}
		return creds, nil
	default:
		return awssdk.Credentials{}, fmt.Errorf("bedrock: unsupported auth_mode %q", key.AuthMode)
	}
}

// assumeBedrockRole performs an STS AssumeRole against the configured
// role ARN, using the default credential chain to authenticate the
// AssumeRole call itself. The returned credentials are temporary and
// scoped to the role; callers re-resolve per request rather than
// caching, since this driver is invoked once per RAG request.
func assumeBedrockRole(ctx context.Context, key *bedrockKey, region string) (awssdk.Credentials, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return awssdk.Credentials{}, fmt.Errorf("bedrock: load AWS config for AssumeRole: %w", err)
	}
	client := sts.NewFromConfig(cfg)
	sessionName := key.SessionName
	if sessionName == "" {
		sessionName = bedrockAssumeRoleSession
	}
	input := &sts.AssumeRoleInput{
		RoleArn:         awssdk.String(key.AWSRoleARN),
		RoleSessionName: awssdk.String(sessionName),
	}
	if key.ExternalID != "" {
		input.ExternalId = awssdk.String(key.ExternalID)
	}
	out, err := client.AssumeRole(ctx, input)
	if err != nil {
		return awssdk.Credentials{}, fmt.Errorf("bedrock: AssumeRole(%s): %w", key.AWSRoleARN, err)
	}
	if out == nil || out.Credentials == nil {
		return awssdk.Credentials{}, fmt.Errorf("bedrock: AssumeRole returned no credentials")
	}
	creds := awssdk.Credentials{
		AccessKeyID:     awssdk.ToString(out.Credentials.AccessKeyId),
		SecretAccessKey: awssdk.ToString(out.Credentials.SecretAccessKey),
		SessionToken:    awssdk.ToString(out.Credentials.SessionToken),
		Source:          "ragflow/bedrock/iam_role",
	}
	if out.Credentials.Expiration != nil {
		creds.CanExpire = true
		creds.Expires = *out.Credentials.Expiration
	}
	return creds, nil
}

// chatSuffix returns the configured chat URL suffix, falling back to
// the AWS-defined "converse" path when conf/models/bedrock.json does
// not override it.
func (b *BedrockModel) chatSuffix() string {
	if b.URLSuffix.Chat != "" {
		return b.URLSuffix.Chat
	}
	return defaultBedrockChatSuffix
}

// streamSuffix returns the streaming-chat URL suffix. Bedrock pairs
// each Converse operation with a "-stream" variant, so we derive the
// stream path from the chat suffix rather than carrying a separate
// configuration field that would have to stay in sync.
func (b *BedrockModel) streamSuffix() string {
	if b.URLSuffix.AsyncChat != "" {
		return b.URLSuffix.AsyncChat
	}
	if b.URLSuffix.Chat != "" {
		return b.URLSuffix.Chat + bedrockStreamSuffixSuffix
	}
	return defaultBedrockStreamSuffix
}

// modelsSuffix returns the list-models URL suffix on the control
// plane, falling back to the AWS-defined "foundation-models" path.
func (b *BedrockModel) modelsSuffix() string {
	if b.URLSuffix.Models != "" {
		return b.URLSuffix.Models
	}
	return defaultBedrockListModelsSuffix
}

// bedrockRuntimeURL builds the per-region runtime endpoint URL for a
// given Bedrock operation. Bedrock paths are deployment-style:
// {host}/model/{modelId}/{op}. Any user-supplied override in BaseURL
// wins so on-premises proxies (e.g. CloudFront-fronted VPC endpoints)
// keep working.
func (b *BedrockModel) bedrockRuntimeURL(region, modelID, op string) string {
	if override, ok := b.BaseURL[region]; ok && override != "" {
		return joinBedrockPath(override, "model", modelID, op)
	}
	host := fmt.Sprintf(bedrockRuntimeHostTmpl, region)
	return fmt.Sprintf("https://%s/model/%s/%s", host, modelID, op)
}

// bedrockControlURL builds the per-region control-plane endpoint URL
// for a given operation (typically "foundation-models" for the model
// catalog).
func (b *BedrockModel) bedrockControlURL(region, op string) string {
	if override, ok := b.BaseURL["control:"+region]; ok && override != "" {
		return joinBedrockPath(override, op)
	}
	host := fmt.Sprintf(bedrockControlHostTmpl, region)
	return fmt.Sprintf("https://%s/%s", host, op)
}

// joinBedrockPath joins a base URL with one or more path segments
// without producing duplicate slashes. We do not use path.Join because
// it would canonicalise the scheme separator (https:// -> https:/).
func joinBedrockPath(base string, parts ...string) string {
	out := strings.TrimRight(base, "/")
	for _, p := range parts {
		out += "/" + strings.Trim(p, "/")
	}
	return out
}

// bedrockMessage is one entry in the Converse "messages" array.
// Bedrock uses a content-block list rather than a flat string so
// vision/tool-use modalities can land additively in follow-on PRs
// without re-shaping the wire format.
type bedrockMessage struct {
	Role    string                `json:"role"`
	Content []bedrockContentBlock `json:"content"`
}

// bedrockContentBlock represents a single content block. For the
// text-only chat path we only emit {"text": "..."}. Other block types
// (image, document, tool_use) are reserved for follow-on PRs.
type bedrockContentBlock struct {
	Text string `json:"text,omitempty"`
}

// bedrockSystemBlock is the same shape as a content block but lives
// at the request top level under "system".
type bedrockSystemBlock struct {
	Text string `json:"text"`
}

// bedrockInferenceConfig mirrors the Converse inferenceConfig sub-
// object. We only emit fields the caller has explicitly set; Bedrock
// applies model defaults for anything omitted.
type bedrockInferenceConfig struct {
	MaxTokens     *int     `json:"maxTokens,omitempty"`
	Temperature   *float64 `json:"temperature,omitempty"`
	TopP          *float64 `json:"topP,omitempty"`
	StopSequences []string `json:"stopSequences,omitempty"`
}

// bedrockConverseRequest is the full Converse request body. Fields
// are pointers/slices so omitempty correctly elides unset values.
type bedrockConverseRequest struct {
	Messages        []bedrockMessage        `json:"messages"`
	System          []bedrockSystemBlock    `json:"system,omitempty"`
	InferenceConfig *bedrockInferenceConfig `json:"inferenceConfig,omitempty"`
}

// bedrockConverseResponse is the relevant subset of a Converse response.
// Bedrock returns much more (usage, metrics) which we currently ignore.
type bedrockConverseResponse struct {
	Output struct {
		Message struct {
			Role    string                `json:"role"`
			Content []bedrockContentBlock `json:"content"`
		} `json:"message"`
	} `json:"output"`
	StopReason string `json:"stopReason"`
}

// buildConverseRequest translates the driver's neutral Messages slice
// (plus optional system prompt extraction) into the Bedrock-shaped
// body. The first contiguous run of "system" role messages becomes the
// system block; subsequent system messages are surfaced as an error
// because Bedrock does not accept interleaved system turns.
func buildConverseRequest(messages []Message, cfg *ChatConfig) (*bedrockConverseRequest, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}
	req := &bedrockConverseRequest{}
	for i, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		text, err := stringContent(msg.Content)
		if err != nil {
			return nil, fmt.Errorf("message[%d]: %w", i, err)
		}
		if role == "system" {
			if len(req.Messages) > 0 {
				return nil, fmt.Errorf("bedrock: system messages must come before user/assistant turns")
			}
			req.System = append(req.System, bedrockSystemBlock{Text: text})
			continue
		}
		if role != "user" && role != "assistant" {
			return nil, fmt.Errorf("bedrock: unsupported role %q", msg.Role)
		}
		req.Messages = append(req.Messages, bedrockMessage{
			Role:    role,
			Content: []bedrockContentBlock{{Text: text}},
		})
	}
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("bedrock: no user/assistant messages after extracting system prompt")
	}
	req.InferenceConfig = mapChatConfigToInference(cfg)
	return req, nil
}

// mapChatConfigToInference projects the neutral ChatConfig onto
// Bedrock's inferenceConfig. Returns nil when no fields are set so
// the request body simply omits the object.
func mapChatConfigToInference(cfg *ChatConfig) *bedrockInferenceConfig {
	if cfg == nil {
		return nil
	}
	inf := &bedrockInferenceConfig{}
	hasField := false
	if cfg.MaxTokens != nil {
		inf.MaxTokens = cfg.MaxTokens
		hasField = true
	}
	if cfg.Temperature != nil {
		inf.Temperature = cfg.Temperature
		hasField = true
	}
	if cfg.TopP != nil {
		inf.TopP = cfg.TopP
		hasField = true
	}
	if cfg.Stop != nil && len(*cfg.Stop) > 0 {
		inf.StopSequences = *cfg.Stop
		hasField = true
	}
	if !hasField {
		return nil
	}
	return inf
}

// stringContent normalises Message.Content (interface{}) into a single
// string. Multimodal content arrays are rejected here because this PR
// only ships the text-only chat path; image/document blocks will be
// added in a follow-on along with vision Bedrock model routing.
func stringContent(content interface{}) (string, error) {
	switch v := content.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	default:
		return "", fmt.Errorf("bedrock: only string Message.Content is supported in this build, got %T", content)
	}
}

// extractAnswer collects all text blocks from the assistant message
// into a single string. Bedrock typically returns one block, but the
// schema is a slice so a defensive concatenation matches the wire
// contract.
func extractAnswer(resp *bedrockConverseResponse) string {
	var sb strings.Builder
	for _, blk := range resp.Output.Message.Content {
		sb.WriteString(blk.Text)
	}
	return sb.String()
}

// signBedrockRequest signs an in-flight http.Request with SigV4 using
// the supplied credentials and service identifier. The payload hash
// is computed over the request body and re-attached as the body for
// transmission (http.NewRequest consumed it once for hashing).
func signBedrockRequest(ctx context.Context, req *http.Request, body []byte, creds awssdk.Credentials, service, region string) error {
	hash := sha256.Sum256(body)
	payloadHash := hex.EncodeToString(hash[:])
	if len(body) > 0 {
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
	}
	signer := v4.NewSigner()
	if err := signer.SignHTTP(ctx, creds, req, payloadHash, service, region, time.Now().UTC()); err != nil {
		return fmt.Errorf("bedrock: SigV4 sign: %w", err)
	}
	return nil
}

// ChatWithMessages sends a non-streaming Converse request and returns
// the joined assistant answer. ReasonContent is always non-nil per the
// driver contract; Bedrock surfaces no reasoning channel today, so it
// is left empty rather than nil.
func (b *BedrockModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if apiConfig == nil || apiConfig.ApiKey == nil {
		return nil, fmt.Errorf("api key is required")
	}
	if modelName == "" {
		return nil, fmt.Errorf("bedrock: model id is required")
	}
	key, err := parseBedrockKey(*apiConfig.ApiKey)
	if err != nil {
		return nil, err
	}
	region, err := resolveBedrockRegion(apiConfig, key)
	if err != nil {
		return nil, err
	}

	body, err := buildConverseRequest(messages, chatModelConfig)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("bedrock: marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	creds, err := resolveBedrockCredentials(ctx, key, region)
	if err != nil {
		return nil, err
	}

	url := b.bedrockRuntimeURL(region, modelName, b.chatSuffix())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("bedrock: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if err := signBedrockRequest(ctx, req, raw, creds, bedrockRuntimeService, region); err != nil {
		return nil, err
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bedrock: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("bedrock: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bedrock: API request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed bedrockConverseResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("bedrock: parse response: %w", err)
	}
	answer := extractAnswer(&parsed)
	reason := ""
	return &ChatResponse{
		Answer:        &answer,
		ReasonContent: &reason,
	}, nil
}

// ChatStreamlyWithSender sends a Converse-Stream request and forwards
// each text delta through sender. The wire format is the AWS event-
// stream binary protocol, not SSE; frames are decoded with the AWS
// SDK's eventstream package so we do not re-implement the framing.
//
// Each frame carries headers identifying the event type and a JSON
// payload. For chat we only need messageStart, contentBlockDelta,
// messageStop, and (for error propagation) exception frames; other
// events are ignored.
func (b *BedrockModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, sender func(*string, *string) error) error {
	if apiConfig == nil || apiConfig.ApiKey == nil {
		return fmt.Errorf("api key is required")
	}
	if modelName == "" {
		return fmt.Errorf("bedrock: model id is required")
	}
	if sender == nil {
		return fmt.Errorf("sender is required")
	}
	if chatModelConfig != nil && chatModelConfig.Stream != nil && !*chatModelConfig.Stream {
		return fmt.Errorf("stream must be true in ChatStreamlyWithSender")
	}

	key, err := parseBedrockKey(*apiConfig.ApiKey)
	if err != nil {
		return err
	}
	region, err := resolveBedrockRegion(apiConfig, key)
	if err != nil {
		return err
	}

	body, err := buildConverseRequest(messages, chatModelConfig)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("bedrock: marshal request: %w", err)
	}

	// Background context: event streams are long-lived so we attach
	// no overall deadline. ResponseHeaderTimeout (set in the
	// constructor) still caps connection setup.
	ctx := context.Background()
	creds, err := resolveBedrockCredentials(ctx, key, region)
	if err != nil {
		return err
	}

	url := b.bedrockRuntimeURL(region, modelName, b.streamSuffix())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("bedrock: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.amazon.eventstream")
	if err := signBedrockRequest(ctx, req, raw, creds, bedrockRuntimeService, region); err != nil {
		return err
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("bedrock: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("bedrock: API request failed with status %d: %s", resp.StatusCode, string(errBody))
	}

	if err := decodeBedrockEventStream(resp.Body, sender); err != nil {
		return err
	}
	done := "[DONE]"
	return sender(&done, nil)
}

// decodeBedrockEventStream reads vnd.amazon.eventstream frames off the
// supplied reader and dispatches each to the supplied sender. The
// loop exits cleanly on a messageStop event or on EOF; an exception
// frame is surfaced as a Go error so partial streams cannot be
// mistaken for successful ones.
func decodeBedrockEventStream(r io.Reader, sender func(*string, *string) error) error {
	dec := eventstream.NewDecoder()
	payload := make([]byte, 0, 8*1024)
	sawTerminal := false
	for {
		msg, err := dec.Decode(r, payload)
		if err != nil {
			if errors.Is(err, io.EOF) {
				if !sawTerminal {
					return fmt.Errorf("bedrock: stream ended before messageStop")
				}
				return nil
			}
			return fmt.Errorf("bedrock: decode event-stream frame: %w", err)
		}
		eventType := lookupBedrockEventHeader(msg.Headers, ":event-type")
		messageType := lookupBedrockEventHeader(msg.Headers, ":message-type")
		if messageType == "exception" || messageType == "error" {
			return fmt.Errorf("bedrock: upstream %s %q: %s", messageType, eventType, string(msg.Payload))
		}
		switch eventType {
		case "contentBlockDelta":
			text, err := extractBedrockDeltaText(msg.Payload)
			if err != nil {
				return err
			}
			if text == "" {
				continue
			}
			if err := sender(&text, nil); err != nil {
				return err
			}
		case "messageStop":
			sawTerminal = true
			return nil
		case "messageStart", "contentBlockStart", "contentBlockStop", "metadata":
			// Lifecycle events with no caller-visible payload.
		default:
			// Ignore unknown events rather than hard-failing so new
			// Bedrock event types do not break this driver.
		}
	}
}

// lookupBedrockEventHeader returns the string value of an event-stream
// header by name, or "" if it is absent or non-string. Bedrock always
// supplies :event-type and :message-type as strings, so the
// permissive fall-through to "" only fires on truly malformed frames.
func lookupBedrockEventHeader(headers eventstream.Headers, name string) string {
	for _, h := range headers {
		if h.Name == name {
			if s, ok := h.Value.Get().(string); ok {
				return s
			}
			return ""
		}
	}
	return ""
}

// bedrockDeltaEnvelope is the contentBlockDelta payload shape:
//
//	{"delta": {"text": "..."}, "contentBlockIndex": 0}
//
// Other delta variants (toolUse, etc.) are ignored because text is
// the only content type this PR ships.
type bedrockDeltaEnvelope struct {
	Delta struct {
		Text string `json:"text"`
	} `json:"delta"`
}

// extractBedrockDeltaText parses one contentBlockDelta payload and
// returns just the text increment. A frame with no text (e.g. a
// future tool_use delta) returns "" with no error so the caller
// simply skips it.
func extractBedrockDeltaText(payload []byte) (string, error) {
	var env bedrockDeltaEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return "", fmt.Errorf("bedrock: invalid contentBlockDelta payload: %w", err)
	}
	return env.Delta.Text, nil
}

// bedrockListModelsResponse mirrors the relevant subset of the
// control-plane response shape. The full record carries provider,
// modality, lifecycle status, etc., which we deliberately drop to
// match the ModelDriver.ListModels return type ([]string).
type bedrockListModelsResponse struct {
	ModelSummaries []struct {
		ModelID string `json:"modelId"`
	} `json:"modelSummaries"`
}

// ListModels returns Bedrock foundation model IDs visible to the
// configured credentials. The control plane lives at
// bedrock.{region}.amazonaws.com (not bedrock-runtime), signs against
// the "bedrock" service, and is GET-only.
func (b *BedrockModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	if apiConfig == nil || apiConfig.ApiKey == nil {
		return nil, fmt.Errorf("api key is required")
	}
	key, err := parseBedrockKey(*apiConfig.ApiKey)
	if err != nil {
		return nil, err
	}
	region, err := resolveBedrockRegion(apiConfig, key)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	creds, err := resolveBedrockCredentials(ctx, key, region)
	if err != nil {
		return nil, err
	}

	url := b.bedrockControlURL(region, b.modelsSuffix())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("bedrock: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if err := signBedrockRequest(ctx, req, nil, creds, bedrockControlService, region); err != nil {
		return nil, err
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bedrock: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("bedrock: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bedrock: ListModels failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed bedrockListModelsResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("bedrock: parse ListModels response: %w", err)
	}
	models := make([]string, 0, len(parsed.ModelSummaries))
	for _, m := range parsed.ModelSummaries {
		if m.ModelID == "" {
			continue
		}
		models = append(models, m.ModelID)
	}
	return models, nil
}

// CheckConnection delegates to ListModels: a successful catalog query
// proves credentials, region, and network reachability in one round
// trip without burning a chat completion.
func (b *BedrockModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := b.ListModels(apiConfig)
	return err
}

// Embed is not exposed by Bedrock through the Converse API; the
// embeddings surface is per-model (Titan, Cohere) and ships in a
// follow-on PR alongside conf/models/bedrock.json embedding entries.
func (b *BedrockModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s, no such method", b.Name())
}

// Rerank is not exposed by Bedrock.
func (b *BedrockModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, no such method", b.Name())
}

// Balance is not exposed by Bedrock.
func (b *BedrockModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", b.Name())
}

// TranscribeAudio is not exposed by Bedrock. Speech-to-text on AWS
// lives in Amazon Transcribe, a separate service.
func (b *BedrockModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", b.Name())
}

func (b *BedrockModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", b.Name())
}

// AudioSpeech is not exposed by Bedrock. Text-to-speech on AWS lives
// in Amazon Polly, a separate service.
func (b *BedrockModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", b.Name())
}

func (b *BedrockModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", b.Name())
}

// OCRFile is not exposed by Bedrock. OCR on AWS lives in Amazon
// Textract, a separate service.
func (b *BedrockModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", b.Name())
}

// ParseFile is not exposed by Bedrock.
func (b *BedrockModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", b.Name())
}

// ListTasks is not exposed by Bedrock through the Converse API.
func (b *BedrockModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", b.Name())
}

// ShowTask is not exposed by Bedrock through the Converse API.
func (b *BedrockModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", b.Name())
}
