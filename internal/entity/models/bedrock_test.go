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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"ragflow/internal/common"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream"
)

// validBedrockKey returns a JSON API-key blob using the access_key_secret
// auth mode. We use static credentials in tests so SigV4 has well-known
// material to sign with, and our httptest server simply accepts any
// signature rather than re-verifying it.
func validBedrockKey() string {
	return `{"auth_mode":"access_key_secret","bedrock_region":"us-east-1","bedrock_ak":"AKIATEST","bedrock_sk":"secret-test"}`
}

// newBedrockForTest constructs a BedrockModel whose runtime and
// control endpoints are both overridden to point at the supplied
// httptest base URL. The override map keys ("us-east-1" and
// "control:us-east-1") match the lookups in bedrockRuntimeURL and
// bedrockControlURL respectively.
func newBedrockForTest(baseURL string) *BedrockModel {
	return NewBedrockModel(
		map[string]string{
			"us-east-1":         baseURL,
			"control:us-east-1": baseURL,
		},
		URLSuffix{Chat: "converse", Models: "foundation-models"},
	)
}

func TestBedrockName(t *testing.T) {
	if got := newBedrockForTest("http://unused").Name(); got != "bedrock" {
		t.Errorf("Name()=%q, want %q", got, "bedrock")
	}
}

func TestParseBedrockKeyRejectsEmpty(t *testing.T) {
	if _, err := parseBedrockKey(""); err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("empty key: want api-key error, got %v", err)
	}
	if _, err := parseBedrockKey("   "); err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("whitespace key: want api-key error, got %v", err)
	}
}

func TestParseBedrockKeyRejectsNonJSON(t *testing.T) {
	if _, err := parseBedrockKey("not-json"); err == nil || !strings.Contains(err.Error(), "JSON object") {
		t.Errorf("non-JSON key: want JSON-object error, got %v", err)
	}
}

func TestParseBedrockKeyRejectsMissingAuthMode(t *testing.T) {
	if _, err := parseBedrockKey(`{"bedrock_region":"us-east-1"}`); err == nil || !strings.Contains(err.Error(), "auth_mode") {
		t.Errorf("missing auth_mode: want explicit error, got %v", err)
	}
}

func TestParseBedrockKeyRejectsUnknownAuthMode(t *testing.T) {
	if _, err := parseBedrockKey(`{"auth_mode":"oauth"}`); err == nil || !strings.Contains(err.Error(), "unsupported auth_mode") {
		t.Errorf("unknown auth_mode: want unsupported error, got %v", err)
	}
}

func TestParseBedrockKeyAccessKeySecretValidates(t *testing.T) {
	// Both AK and SK must be present; one without the other is rejected
	// so a misconfigured tenant fails fast instead of producing an
	// unsigned request the server then rejects opaquely.
	missingSK := `{"auth_mode":"access_key_secret","bedrock_region":"us-east-1","bedrock_ak":"AKIATEST"}`
	if _, err := parseBedrockKey(missingSK); err == nil || !strings.Contains(err.Error(), "bedrock_ak and bedrock_sk") {
		t.Errorf("missing SK: want ak/sk error, got %v", err)
	}
	missingAK := `{"auth_mode":"access_key_secret","bedrock_region":"us-east-1","bedrock_sk":"secret"}`
	if _, err := parseBedrockKey(missingAK); err == nil || !strings.Contains(err.Error(), "bedrock_ak and bedrock_sk") {
		t.Errorf("missing AK: want ak/sk error, got %v", err)
	}
}

func TestParseBedrockKeyIAMRoleRequiresARN(t *testing.T) {
	if _, err := parseBedrockKey(`{"auth_mode":"iam_role","bedrock_region":"us-east-1"}`); err == nil || !strings.Contains(err.Error(), "aws_role_arn") {
		t.Errorf("iam_role no ARN: want aws_role_arn error, got %v", err)
	}
}

func TestParseBedrockKeyAssumeRoleAcceptsBareConfig(t *testing.T) {
	// assume_role intentionally delegates to the default credential
	// chain, so parseBedrockKey must accept a blob with no AK/SK/ARN.
	if _, err := parseBedrockKey(`{"auth_mode":"assume_role","bedrock_region":"us-east-1"}`); err != nil {
		t.Errorf("assume_role: want no error, got %v", err)
	}
}

func TestParseBedrockAPIKeyRequiresToken(t *testing.T) {
	_, err := parseBedrockKey(`{"auth_mode":"bedrock_api_key","bedrock_region":"ap-northeast-1"}`)
	if err == nil || !strings.Contains(err.Error(), "requires bedrock_api_key") {
		t.Fatalf("missing Bedrock API key: got %v", err)
	}
}

func TestParseBedrockAPIKeyTrimsToken(t *testing.T) {
	key, err := parseBedrockKey(`{"auth_mode":"bedrock_api_key","bedrock_region":"ap-northeast-1","bedrock_api_key":" token "}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if key.BedrockAPIKey != "token" {
		t.Fatalf("BedrockAPIKey=%q", key.BedrockAPIKey)
	}
}

func TestParseBedrockAPIKeyNormalizesMantleEndpoint(t *testing.T) {
	key, err := parseBedrockKey(`{"auth_mode":"bedrock_api_key","bedrock_region":"ap-northeast-1","bedrock_api_key":"token","bedrock_endpoint_type":"mantle_anthropic","bedrock_endpoint_url":"https://bedrock-mantle.ap-northeast-1.api.aws/v1/models"}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if key.EndpointURL != "https://bedrock-mantle.ap-northeast-1.api.aws/anthropic" {
		t.Fatalf("EndpointURL=%q", key.EndpointURL)
	}
}

func TestNormalizeBedrockEndpointStopsAfterFirstSuffix(t *testing.T) {
	got := normalizeBedrockEndpoint(bedrockEndpointMantleOpenAI, "https://bedrock-mantle.ap-northeast-1.api.aws/anthropic/v1/models")
	want := "https://bedrock-mantle.ap-northeast-1.api.aws/anthropic/v1"
	if got != want {
		t.Fatalf("normalizeBedrockEndpoint() = %q, want %q", got, want)
	}
}

func TestAuthorizeBedrockRequestUsesBearerWithoutSigV4(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://bedrock.ap-northeast-1.amazonaws.com/foundation-models", nil)
	if err != nil {
		t.Fatal(err)
	}
	key := &bedrockKey{AuthMode: bedrockAuthAPIKey, BedrockAPIKey: "test-token"}
	if err = authorizeBedrockRequest(t.Context(), req, nil, key, bedrockControlService, "ap-northeast-1"); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer test-token" {
		t.Fatalf("Authorization=%q", got)
	}
	if got := req.Header.Get("X-Amz-Date"); got != "" {
		t.Fatalf("request was unexpectedly SigV4-signed: X-Amz-Date=%q", got)
	}
}

func TestAuthorizeBedrockRequestRejectsUntrustedEndpointBeforeAddingBearer(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://attacker.example.com/model/test/converse", nil)
	if err != nil {
		t.Fatal(err)
	}
	key := &bedrockKey{AuthMode: bedrockAuthAPIKey, BedrockAPIKey: "test-token"}
	err = authorizeBedrockRequest(t.Context(), req, nil, key, bedrockRuntimeService, "ap-northeast-1")
	if err == nil || !strings.Contains(err.Error(), "untrusted endpoint") {
		t.Fatalf("authorize untrusted endpoint: got %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization leaked to untrusted endpoint: %q", got)
	}
}

func TestAuthorizeBedrockRequestAllowsOnDemandCatalogFilter(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://bedrock.ap-northeast-1.amazonaws.com/foundation-models?byInferenceType=ON_DEMAND", nil)
	if err != nil {
		t.Fatal(err)
	}
	key := &bedrockKey{AuthMode: bedrockAuthAPIKey, BedrockAPIKey: "test-token"}
	if err = authorizeBedrockRequest(t.Context(), req, nil, key, bedrockControlService, "ap-northeast-1"); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer test-token" {
		t.Fatalf("Authorization=%q", got)
	}
}

func TestAuthorizeBedrockRequestRejectsUnexpectedCatalogQuery(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://bedrock.ap-northeast-1.amazonaws.com/foundation-models?target=example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	key := &bedrockKey{AuthMode: bedrockAuthAPIKey, BedrockAPIKey: "test-token"}
	if err = authorizeBedrockRequest(t.Context(), req, nil, key, bedrockControlService, "ap-northeast-1"); err == nil {
		t.Fatal("unexpected catalog query was accepted")
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("Bearer token was added before target validation: %q", got)
	}
}

func TestResolveBedrockEndpointRejectsUntrustedHost(t *testing.T) {
	_, _, err := resolveBedrockEndpoint(bedrockAuthAPIKey, bedrockEndpointMantleOpenAI, "https://example.com/v1")
	if err == nil || !strings.Contains(err.Error(), "hostname is not allowed") {
		t.Fatalf("untrusted endpoint: got %v", err)
	}
}

func TestValidateBedrockEndpointRejectsNonDefaultPort(t *testing.T) {
	err := validateBedrockEndpointTarget("https://bedrock-runtime.us-east-1.amazonaws.com:8443")
	if err == nil || !strings.Contains(err.Error(), "non-default port") {
		t.Fatalf("non-default port: got %v", err)
	}
}

func TestValidateBedrockEndpointAllowsHTTPSPort(t *testing.T) {
	if err := validateBedrockEndpointTarget("https://bedrock-runtime.us-east-1.amazonaws.com:443"); err != nil {
		t.Fatalf("HTTPS port: %v", err)
	}
}

func TestValidateBedrockEndpointAllowsExplicitProxyPort(t *testing.T) {
	t.Setenv("BEDROCK_ENDPOINT_HOST_ALLOWLIST", "bedrock-proxy.example.com:8443")
	if err := validateBedrockEndpointTarget("https://bedrock-proxy.example.com:8443"); err != nil {
		t.Fatalf("allowlisted proxy port: %v", err)
	}
}

func TestValidateBedrockEndpointRequiresExactProxyPort(t *testing.T) {
	t.Setenv("BEDROCK_ENDPOINT_HOST_ALLOWLIST", "bedrock-proxy.example.com")
	err := validateBedrockEndpointTarget("https://bedrock-proxy.example.com:8443")
	if err == nil || !strings.Contains(err.Error(), "exact host:port") {
		t.Fatalf("hostname-only allowlist entry: got %v", err)
	}
}

func TestBedrockMantleDriversReuseHTTPClient(t *testing.T) {
	bedrock := newBedrockForTest("http://unused")
	key := &bedrockKey{EndpointURL: "https://bedrock-mantle.us-east-1.api.aws"}

	if got := bedrock.mantleOpenAI(key).baseModel.httpClient; got != bedrock.baseModel.httpClient {
		t.Fatal("Mantle OpenAI driver did not reuse the Bedrock HTTP client")
	}
	if got := bedrock.mantleAnthropic(key).baseModel.httpClient; got != bedrock.baseModel.httpClient {
		t.Fatal("Mantle Anthropic driver did not reuse the Bedrock HTTP client")
	}
}

func TestResolveBedrockRegionPrefersAPIConfig(t *testing.T) {
	key := &bedrockKey{Region: "us-east-1"}
	override := "eu-west-1"
	got, err := resolveBedrockRegion(&APIConfig{Region: &override}, key)
	if err != nil || got != "eu-west-1" {
		t.Errorf("got region=%q err=%v, want eu-west-1", got, err)
	}
}

func TestResolveBedrockRegionFallsBackToKey(t *testing.T) {
	key := &bedrockKey{Region: "us-east-1"}
	got, err := resolveBedrockRegion(&APIConfig{}, key)
	if err != nil || got != "us-east-1" {
		t.Errorf("got region=%q err=%v, want us-east-1", got, err)
	}
}

func TestResolveBedrockRegionRequiresOne(t *testing.T) {
	key := &bedrockKey{}
	if _, err := resolveBedrockRegion(&APIConfig{}, key); err == nil || !strings.Contains(err.Error(), "region is required") {
		t.Errorf("no region: want region-required error, got %v", err)
	}
}

func TestBuildConverseRequestExtractsSystem(t *testing.T) {
	req, err := buildConverseRequest([]Message{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Hi"},
	}, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(req.System) != 1 || req.System[0].Text != "You are helpful." {
		t.Errorf("system block wrong: %+v", req.System)
	}
	if len(req.Messages) != 1 || req.Messages[0].Role != "user" {
		t.Errorf("messages wrong: %+v", req.Messages)
	}
}

func TestBuildConverseRequestRejectsInterleavedSystem(t *testing.T) {
	_, err := buildConverseRequest([]Message{
		{Role: "user", Content: "Hi"},
		{Role: "system", Content: "Mid-conversation system prompt"},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "system messages must come before") {
		t.Errorf("interleaved system: want explicit error, got %v", err)
	}
}

func TestBuildConverseRequestRejectsUnsupportedRole(t *testing.T) {
	_, err := buildConverseRequest([]Message{{Role: "function", Content: "x"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "unsupported role") {
		t.Errorf("unsupported role: want explicit error, got %v", err)
	}
}

func TestBuildConverseRequestRejectsEmpty(t *testing.T) {
	_, err := buildConverseRequest(nil, nil)
	if err == nil || !strings.Contains(err.Error(), "messages is empty") {
		t.Errorf("empty: want messages-empty error, got %v", err)
	}
}

func TestBuildConverseRequestRejectsOnlySystem(t *testing.T) {
	_, err := buildConverseRequest([]Message{{Role: "system", Content: "x"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "no user/assistant") {
		t.Errorf("only system: want no-turns error, got %v", err)
	}
}

func TestBuildConverseRequestRejectsMultimodalForNow(t *testing.T) {
	// The text-only path is the only one this PR ships. A multimodal
	// content array must fail loudly so the operator gets a clear
	// migration path rather than a silently-truncated request.
	_, err := buildConverseRequest([]Message{{Role: "user", Content: []interface{}{
		map[string]interface{}{"type": "text", "text": "Hi"},
	}}}, nil)
	if err == nil || !strings.Contains(err.Error(), "only string Message.Content") {
		t.Errorf("multimodal: want text-only error, got %v", err)
	}
}

func TestMapChatConfigToInferenceForwardsAllFields(t *testing.T) {
	mt := 4096
	temp := 0.5
	topP := 0.9
	stop := []string{"END"}
	inf := mapChatConfigToInference(&ChatConfig{
		MaxTokens: &mt, Temperature: &temp, TopP: &topP, Stop: &stop,
	})
	if inf == nil {
		t.Fatal("expected non-nil inferenceConfig")
	}
	if inf.MaxTokens != nil {
		t.Errorf("maxTokens should be omitted, got %v", inf.MaxTokens)
	}
	if inf.Temperature == nil || *inf.Temperature != 0.5 {
		t.Errorf("temperature=%v", inf.Temperature)
	}
	if inf.TopP == nil || *inf.TopP != 0.9 {
		t.Errorf("topP=%v", inf.TopP)
	}
	if len(inf.StopSequences) != 1 || inf.StopSequences[0] != "END" {
		t.Errorf("stopSequences=%v", inf.StopSequences)
	}
}

func TestMapChatConfigToInferenceReturnsNilWhenUnset(t *testing.T) {
	if got := mapChatConfigToInference(nil); got != nil {
		t.Errorf("nil cfg: want nil, got %+v", got)
	}
	if got := mapChatConfigToInference(&ChatConfig{}); got != nil {
		t.Errorf("empty cfg: want nil, got %+v", got)
	}
}

func TestExtractAnswerConcatenatesContentBlocks(t *testing.T) {
	resp := &bedrockConverseResponse{}
	resp.Output.Message.Content = []bedrockContentBlock{
		{Text: "Hello "},
		{Text: "world"},
	}
	if got := extractAnswer(resp); got != "Hello world" {
		t.Errorf("extractAnswer=%q", got)
	}
}

func TestBedrockRuntimeURLUsesOverride(t *testing.T) {
	b := NewBedrockModel(map[string]string{"us-east-1": "https://proxy.example.com/bedrock"}, URLSuffix{})
	got := b.bedrockRuntimeURL("us-east-1", "anthropic.claude-3-haiku-20240307-v1:0", "converse")
	want := "https://proxy.example.com/bedrock/model/anthropic.claude-3-haiku-20240307-v1:0/converse"
	if got != want {
		t.Errorf("override URL=%q, want %q", got, want)
	}
}

func TestBedrockRuntimeURLFallsBackToAWS(t *testing.T) {
	b := NewBedrockModel(nil, URLSuffix{})
	got := b.bedrockRuntimeURL("eu-west-1", "amazon.nova-lite-v1:0", "converse")
	want := "https://bedrock-runtime.eu-west-1.amazonaws.com/model/amazon.nova-lite-v1:0/converse"
	if got != want {
		t.Errorf("default URL=%q, want %q", got, want)
	}
}

func TestBedrockControlURLFallsBackToAWS(t *testing.T) {
	b := NewBedrockModel(nil, URLSuffix{})
	got := b.bedrockControlURL("us-west-2", "foundation-models")
	want := "https://bedrock.us-west-2.amazonaws.com/foundation-models"
	if got != want {
		t.Errorf("control URL=%q, want %q", got, want)
	}
}

func TestJoinBedrockPathHandlesTrailingSlashes(t *testing.T) {
	got := joinBedrockPath("https://x.example.com/", "model", "/m/", "converse")
	want := "https://x.example.com/model/m/converse"
	if got != want {
		t.Errorf("joinBedrockPath=%q, want %q", got, want)
	}
}

func TestExtractBedrockDeltaTextHappyPath(t *testing.T) {
	got, err := extractBedrockDeltaText([]byte(`{"delta":{"text":"hi"},"contentBlockIndex":0}`))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if got != "hi" {
		t.Errorf("text=%q", got)
	}
}

func TestExtractBedrockDeltaTextSkipsEmpty(t *testing.T) {
	// A toolUse-only delta has no text field; the helper must return
	// "" with no error so the streaming loop simply skips the frame.
	got, err := extractBedrockDeltaText([]byte(`{"delta":{"toolUse":{}},"contentBlockIndex":0}`))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty text, got %q", got)
	}
}

func TestExtractBedrockDeltaTextRejectsMalformed(t *testing.T) {
	_, err := extractBedrockDeltaText([]byte(`{not-json}`))
	if err == nil || !strings.Contains(err.Error(), "invalid contentBlockDelta") {
		t.Errorf("malformed: want explicit error, got %v", err)
	}
}

// newBedrockServer returns an httptest.Server that asserts the
// request method, path, and Authorization header before delegating
// to the supplied handler. We accept any "AWS4-HMAC-SHA256 ..."
// header rather than re-verify the signature: SigV4 correctness is
// the SDK's responsibility and is covered by its own tests.
func newBedrockServer(t *testing.T, wantMethod, wantPath string, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != wantMethod {
			t.Errorf("method=%s, want %s", r.Method, wantMethod)
		}
		if r.URL.Path != wantPath {
			t.Errorf("path=%s, want %s", r.URL.Path, wantPath)
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 ") {
			t.Errorf("Authorization=%q, want SigV4 prefix", auth)
		}
		handler(w, r)
	}))
}

func TestBedrockChatHappyPath(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newBedrockServer(t, http.MethodPost,
		"/model/anthropic.claude-3-haiku-20240307-v1:0/converse",
		func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			var body bedrockConverseRequest
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("unmarshal body: %v", err)
				return
			}
			if len(body.Messages) != 1 || body.Messages[0].Role != "user" {
				t.Errorf("messages wrong: %+v", body.Messages)
			}
			resp := bedrockConverseResponse{}
			resp.Output.Message.Role = "assistant"
			resp.Output.Message.Content = []bedrockContentBlock{{Text: "pong"}}
			resp.StopReason = "end_turn"
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		})
	defer srv.Close()

	m := newBedrockForTest(srv.URL)
	key := validBedrockKey()
	resp, err := m.ChatWithMessages(ctx, "anthropic.claude-3-haiku-20240307-v1:0",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &key}, nil, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Answer == nil || resp.ReasonContent == nil {
		t.Fatalf("answer/reason must be non-nil pointers, got %v / %v", resp.Answer, resp.ReasonContent)
	}
	if *resp.Answer != "pong" {
		t.Errorf("answer=%q want pong", *resp.Answer)
	}
	if *resp.ReasonContent != "" {
		t.Errorf("reason=%q want empty", *resp.ReasonContent)
	}
}

func TestBedrockChatRequiresAPIKey(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	m := newBedrockForTest("http://unused")
	_, err := m.ChatWithMessages(ctx, "m", []Message{{Role: "user", Content: "x"}}, &APIConfig{}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("want api-key error, got %v", err)
	}
}

func TestBedrockMantleAnthropicUsesAPIKeyHeader(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/anthropic/v1/messages" {
			t.Errorf("path=%q want /anthropic/v1/messages", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "token" {
			t.Errorf("x-api-key=%q want token", got)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization=%q want empty", got)
		}
		if got := r.Header.Get("anthropic-version"); got != anthropicVersion {
			t.Errorf("anthropic-version=%q want %q", got, anthropicVersion)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"pong"}]}`))
	}))
	defer srv.Close()

	testURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	key := fmt.Sprintf(
		`{"auth_mode":"bedrock_api_key","bedrock_region":"us-east-1","bedrock_api_key":"token","bedrock_endpoint_type":"mantle_anthropic","bedrock_endpoint_url":%q}`,
		"https://bedrock-mantle.us-east-1.api.aws",
	)
	m := NewBedrockModel(nil, URLSuffix{})
	m.baseModel.httpClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req = req.Clone(req.Context())
		req.URL.Scheme = testURL.Scheme
		req.URL.Host = testURL.Host
		return srv.Client().Transport.RoundTrip(req)
	})}
	resp, err := m.ChatWithMessages(
		t.Context(),
		"anthropic.claude-sonnet-current",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &key},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.Answer == nil || *resp.Answer != "pong" {
		t.Fatalf("answer=%v want pong", resp.Answer)
	}
}

func TestBedrockChatRequiresModelID(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	m := newBedrockForTest("http://unused")
	key := validBedrockKey()
	_, err := m.ChatWithMessages(ctx, "", []Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &key}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "model id is required") {
		t.Errorf("want model-required error, got %v", err)
	}
}

func TestBedrockChatPropagatesHTTPError(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newBedrockServer(t, http.MethodPost,
		"/model/m/converse",
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"InvalidSignatureException"}`))
		})
	defer srv.Close()

	m := newBedrockForTest(srv.URL)
	key := validBedrockKey()
	_, err := m.ChatWithMessages(ctx, "m",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &key}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("want 401 propagated, got %v", err)
	}
	if err != nil && !strings.Contains(err.Error(), "InvalidSignatureException") {
		t.Errorf("want body included in error, got %v", err)
	}
}

func TestBedrockListModelsParsesCatalog(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newBedrockServer(t, http.MethodGet,
		"/foundation-models",
		func(w http.ResponseWriter, r *http.Request) {
			if got := r.URL.Query().Get("byInferenceType"); got != "ON_DEMAND" {
				t.Errorf("byInferenceType=%q, want ON_DEMAND", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"modelSummaries": [
					{"modelId":"anthropic.claude-3-haiku-20240307-v1:0","inputModalities":["TEXT"],"outputModalities":["TEXT"],"inferenceTypesSupported":["ON_DEMAND"],"modelLifecycle":{"status":"ACTIVE"}},
					{"modelId":"amazon.nova-lite-v1:0","inputModalities":["TEXT","IMAGE"],"outputModalities":["TEXT"],"inferenceTypesSupported":["ON_DEMAND","PROVISIONED"],"modelLifecycle":{"status":"ACTIVE"}},
					{"modelId":"anthropic.provisioned-only","inputModalities":["TEXT"],"outputModalities":["TEXT"],"inferenceTypesSupported":["PROVISIONED"],"modelLifecycle":{"status":"ACTIVE"}},
					{"modelId":"anthropic.legacy-model","inputModalities":["TEXT"],"outputModalities":["TEXT"],"inferenceTypesSupported":["ON_DEMAND"],"modelLifecycle":{"status":"LEGACY"}},
					{"modelId":"missing-modalities"},
					{"modelId":""}
				]
			}`))
		})
	defer srv.Close()

	m := newBedrockForTest(srv.URL)
	key := validBedrockKey()
	got, err := m.ListModels(ctx, &APIConfig{ApiKey: &key})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	// The empty-modelId summary is filtered so a malformed AWS
	// response never leaks an empty string up to the UI dropdown.
	want := []string{
		"anthropic.claude-3-haiku-20240307-v1:0",
		"amazon.nova-lite-v1:0",
	}
	if len(got) != len(want) {
		t.Fatalf("len(got)=%d want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Name != want[i] {
			t.Errorf("got[%d]=%s want %q", i, got[i].Name, want[i])
		}
	}
	if diff := strings.Join(got[0].ModelTypes, ","); diff != "chat" {
		t.Errorf("Claude model types=%q, want chat", diff)
	}
	if diff := strings.Join(got[1].ModelTypes, ","); diff != "chat" {
		t.Errorf("Nova model types=%q, want chat", diff)
	}
}

func TestBedrockModelTypesDoesNotAdvertiseUnsupportedVision(t *testing.T) {
	got := bedrockModelTypes("amazon.nova-lite-v1:0", []string{"TEXT", "IMAGE"}, []string{"TEXT"})
	if diff := strings.Join(got, ","); diff != "chat" {
		t.Fatalf("model types=%q, want chat", diff)
	}
}

func TestBedrockCheckConnectionDelegates(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newBedrockServer(t, http.MethodGet,
		"/foundation-models",
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		})
	defer srv.Close()
	m := newBedrockForTest(srv.URL)
	key := validBedrockKey()
	if err := m.CheckConnection(ctx, &APIConfig{ApiKey: &key}); err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("want 403 surfaced via ListModels, got %v", err)
	}
}

func TestBedrockCheckConnectionRejectsEmptyCompatibleCatalog(t *testing.T) {
	withSSRFBypass(t)
	srv := newBedrockServer(t, http.MethodGet,
		"/foundation-models",
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"modelSummaries":[]}`))
		})
	defer srv.Close()

	m := newBedrockForTest(srv.URL)
	key := validBedrockKey()
	err := m.CheckConnection(t.Context(), &APIConfig{ApiKey: &key})
	if err == nil || !strings.Contains(err.Error(), "no compatible Bedrock") {
		t.Fatalf("want empty-catalog error, got %v", err)
	}
}

// encodeBedrockEventFrames builds an in-memory event-stream that
// looks like a real Bedrock Converse-Stream response: a messageStart
// lifecycle event, two contentBlockDelta payloads, and a messageStop
// terminator. We use the same encoder the SDK uses on the wire so
// the decode path is exercised end-to-end.
func encodeBedrockEventFrames(t *testing.T, events []struct {
	eventType   string
	messageType string
	payload     []byte
}) []byte {
	t.Helper()
	var buf bytes.Buffer
	enc := eventstream.NewEncoder()
	for _, e := range events {
		msg := eventstream.Message{
			Headers: eventstream.Headers{
				{Name: ":event-type", Value: eventstream.StringValue(e.eventType)},
			},
			Payload: e.payload,
		}
		if e.messageType != "" {
			msg.Headers = append(msg.Headers, eventstream.Header{
				Name:  ":message-type",
				Value: eventstream.StringValue(e.messageType),
			})
		} else {
			msg.Headers = append(msg.Headers, eventstream.Header{
				Name:  ":message-type",
				Value: eventstream.StringValue("event"),
			})
		}
		if err := enc.Encode(&buf, msg); err != nil {
			t.Fatalf("encode event-stream frame: %v", err)
		}
	}
	return buf.Bytes()
}

func TestBedrockStreamDecodesContentDeltas(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	frames := encodeBedrockEventFrames(t, []struct {
		eventType   string
		messageType string
		payload     []byte
	}{
		{eventType: "messageStart", payload: []byte(`{"role":"assistant"}`)},
		{eventType: "contentBlockDelta", payload: []byte(`{"delta":{"text":"Hello"},"contentBlockIndex":0}`)},
		{eventType: "contentBlockDelta", payload: []byte(`{"delta":{"text":" world"},"contentBlockIndex":0}`)},
		{eventType: "messageStop", payload: []byte(`{"stopReason":"end_turn"}`)},
	})

	srv := newBedrockServer(t, http.MethodPost,
		"/model/m/converse-stream",
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
			_, _ = w.Write(frames)
		})
	defer srv.Close()

	m := newBedrockForTest(srv.URL)
	key := validBedrockKey()
	var chunks []string
	sawDone := false
	err := m.ChatStreamlyWithSender(ctx, "m",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &key}, nil, nil,
		func(c *string, _ *string) error {
			if c == nil {
				return nil
			}
			if *c == "[DONE]" {
				sawDone = true
				return nil
			}
			chunks = append(chunks, *c)
			return nil
		})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if got := strings.Join(chunks, ""); got != "Hello world" {
		t.Errorf("chunks=%q want %q", got, "Hello world")
	}
	if !sawDone {
		t.Error("expected [DONE] sentinel")
	}
}

func TestBedrockStreamSurfacesException(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	frames := encodeBedrockEventFrames(t, []struct {
		eventType   string
		messageType string
		payload     []byte
	}{
		{eventType: "contentBlockDelta", payload: []byte(`{"delta":{"text":"partial"},"contentBlockIndex":0}`)},
		{
			eventType:   "throttlingException",
			messageType: "exception",
			payload:     []byte(`{"message":"Too many requests"}`),
		},
	})
	srv := newBedrockServer(t, http.MethodPost,
		"/model/m/converse-stream",
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
			_, _ = w.Write(frames)
		})
	defer srv.Close()

	m := newBedrockForTest(srv.URL)
	key := validBedrockKey()
	err := m.ChatStreamlyWithSender(ctx, "m",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &key}, nil, nil,
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "exception") {
		t.Errorf("want exception surfaced, got %v", err)
	}
}

func TestBedrockStreamFailsWithoutTerminal(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	// Connection closed cleanly after a delta but before messageStop.
	// This used to be silently treated as success, masking truncated
	// answers; the driver must now surface a "stream ended before
	// messageStop" error so the caller can retry or alert.
	frames := encodeBedrockEventFrames(t, []struct {
		eventType   string
		messageType string
		payload     []byte
	}{
		{eventType: "contentBlockDelta", payload: []byte(`{"delta":{"text":"half"},"contentBlockIndex":0}`)},
	})
	srv := newBedrockServer(t, http.MethodPost,
		"/model/m/converse-stream",
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
			_, _ = w.Write(frames)
		})
	defer srv.Close()

	m := newBedrockForTest(srv.URL)
	key := validBedrockKey()
	err := m.ChatStreamlyWithSender(ctx, "m",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &key}, nil, nil,
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "stream ended before") {
		t.Errorf("want truncation error, got %v", err)
	}
}

func TestBedrockStreamRecordsUsageFromPostMessageStopMetadata(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	// The AWS Bedrock Converse-Stream protocol sends a final
	// "metadata" frame AFTER messageStop carrying token usage. The
	// driver must not exit the decoder on messageStop, otherwise this
	// metadata (and therefore the captured usage) is silently dropped.
	frames := encodeBedrockEventFrames(t, []struct {
		eventType   string
		messageType string
		payload     []byte
	}{
		{eventType: "messageStart", payload: []byte(`{"role":"assistant"}`)},
		{eventType: "contentBlockDelta", payload: []byte(`{"delta":{"text":"hi"},"contentBlockIndex":0}`)},
		{eventType: "messageStop", payload: []byte(`{"stopReason":"end_turn"}`)},
		{
			eventType: "metadata",
			payload:   []byte(`{"usage":{"inputTokens":7,"outputTokens":3,"totalTokens":10},"metrics":{"latencyMs":42}}`),
		},
	})
	srv := newBedrockServer(t, http.MethodPost,
		"/model/m/converse-stream",
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
			_, _ = w.Write(frames)
		})
	defer srv.Close()

	m := newBedrockForTest(srv.URL)
	key := validBedrockKey()
	usage := &common.ModelUsage{}
	cfg := &ChatConfig{}
	err := m.ChatStreamlyWithSender(ctx, "m",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &key}, cfg, usage,
		func(*string, *string) error { return nil })
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if usage.InputTokens != 7 || usage.OutputTokens != 3 || usage.TotalTokens != 10 {
		t.Errorf("modelUsage=(%d,%d,%d), want (7,3,10)", usage.InputTokens, usage.OutputTokens, usage.TotalTokens)
	}
	if cfg.UsageResult == nil {
		t.Fatal("UsageResult is nil, want populated from post-messageStop metadata")
	}
	if cfg.UsageResult.PromptTokens != 7 || cfg.UsageResult.CompletionTokens != 3 || cfg.UsageResult.TotalTokens != 10 {
		t.Errorf("UsageResult=(%d,%d,%d), want (7,3,10)", cfg.UsageResult.PromptTokens, cfg.UsageResult.CompletionTokens, cfg.UsageResult.TotalTokens)
	}
}

func TestBedrockStreamRejectsExplicitFalse(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	m := newBedrockForTest("http://unused")
	key := validBedrockKey()
	stream := false
	err := m.ChatStreamlyWithSender(ctx, "m",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &key},
		&ChatConfig{Stream: &stream},
		nil,
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "stream must be true") {
		t.Errorf("want stream-true guard, got %v", err)
	}
}

func TestBedrockStreamRequiresSender(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	m := newBedrockForTest("http://unused")
	key := validBedrockKey()
	err := m.ChatStreamlyWithSender(ctx, "m",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &key}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "sender is required") {
		t.Errorf("want sender-required error, got %v", err)
	}
}

func TestLookupBedrockEventHeader(t *testing.T) {
	headers := eventstream.Headers{
		{Name: ":event-type", Value: eventstream.StringValue("contentBlockDelta")},
		{Name: ":message-type", Value: eventstream.StringValue("event")},
	}
	if got := lookupBedrockEventHeader(headers, ":event-type"); got != "contentBlockDelta" {
		t.Errorf("event-type=%q", got)
	}
	if got := lookupBedrockEventHeader(headers, ":nonexistent"); got != "" {
		t.Errorf("nonexistent header=%q want empty", got)
	}
}

func TestBedrockTitanEmbedHappyPath(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	var seenInputs []string
	srv := newBedrockServer(t, http.MethodPost,
		"/model/amazon.titan-embed-text-v2:0/invoke",
		func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			var body bedrockTitanEmbeddingRequest
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("unmarshal body: %v", err)
				return
			}
			seenInputs = append(seenInputs, body.InputText)
			if body.Dimensions == nil || *body.Dimensions != 256 {
				t.Errorf("dimensions=%v, want 256", body.Dimensions)
			}
			w.Header().Set("Content-Type", "application/json")
			if body.InputText == "alpha" {
				_, _ = w.Write([]byte(`{"embedding":[0.1,0.2]}`))
			} else {
				_, _ = w.Write([]byte(`{"embedding":[0.3,0.4]}`))
			}
		})
	defer srv.Close()

	m := newBedrockForTest(srv.URL)
	key := validBedrockKey()
	model := "amazon.titan-embed-text-v2:0"
	got, err := m.Embed(ctx, &model, []string{"alpha", "beta"}, &APIConfig{ApiKey: &key}, &EmbeddingConfig{Dimension: 256}, nil)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(seenInputs) != 2 || seenInputs[0] != "alpha" || seenInputs[1] != "beta" {
		t.Fatalf("seen inputs=%v", seenInputs)
	}
	if len(got) != 2 {
		t.Fatalf("len(got)=%d want 2", len(got))
	}
	if got[0].Index != 0 || got[0].Embedding[0] != 0.1 || got[1].Index != 1 || got[1].Embedding[0] != 0.3 {
		t.Errorf("embeddings=%+v", got)
	}
}

func TestBedrockTitanV1OmitsDimension(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newBedrockServer(t, http.MethodPost,
		"/model/amazon.titan-embed-text-v1/invoke",
		func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			if strings.Contains(string(raw), "dimensions") {
				t.Errorf("Titan v1 body must not include dimensions: %s", string(raw))
			}
			_, _ = w.Write([]byte(`{"embedding":[0.1,0.2]}`))
		})
	defer srv.Close()

	m := newBedrockForTest(srv.URL)
	key := validBedrockKey()
	model := "amazon.titan-embed-text-v1"
	if _, err := m.Embed(ctx, &model, []string{"alpha"}, &APIConfig{ApiKey: &key}, &EmbeddingConfig{Dimension: 256}, nil); err != nil {
		t.Fatalf("Embed: %v", err)
	}
}

func TestBedrockCohereEmbedHappyPath(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newBedrockServer(t, http.MethodPost,
		"/model/cohere.embed-english-v3/invoke",
		func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			var body bedrockCohereEmbeddingRequest
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("unmarshal body: %v", err)
				return
			}
			if len(body.Texts) != 2 || body.Texts[0] != "first" || body.Texts[1] != "second" {
				t.Errorf("texts=%v", body.Texts)
			}
			if body.InputType != "search_document" {
				t.Errorf("input_type=%q want search_document", body.InputType)
			}
			if body.OutputDimension != nil {
				t.Errorf("v3 output_dimension=%v, want omitted", *body.OutputDimension)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"embeddings":[[1,2],[3,4]]}`))
		})
	defer srv.Close()

	m := newBedrockForTest(srv.URL)
	key := validBedrockKey()
	model := "cohere.embed-english-v3"
	got, err := m.Embed(ctx, &model, []string{"first", "second"}, &APIConfig{ApiKey: &key}, &EmbeddingConfig{Dimension: 128}, nil)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(got) != 2 || got[0].Index != 0 || got[0].Embedding[0] != 1 || got[1].Index != 1 || got[1].Embedding[0] != 3 {
		t.Errorf("embeddings=%+v", got)
	}
}

func TestBedrockCohereV4ForwardsDimensionAndParsesTypedResponse(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newBedrockServer(t, http.MethodPost,
		"/model/cohere.embed-v4:0/invoke",
		func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			var body bedrockCohereEmbeddingRequest
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("unmarshal body: %v", err)
				return
			}
			if body.OutputDimension == nil || *body.OutputDimension != 512 {
				t.Errorf("output_dimension=%v, want 512", body.OutputDimension)
			}
			_, _ = w.Write([]byte(`{"embeddings":{"float":[[0.5,0.6]]}}`))
		})
	defer srv.Close()

	m := newBedrockForTest(srv.URL)
	key := validBedrockKey()
	model := "cohere.embed-v4:0"
	got, err := m.Embed(ctx, &model, []string{"first"}, &APIConfig{ApiKey: &key}, &EmbeddingConfig{Dimension: 512}, nil)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(got) != 1 || got[0].Index != 0 || got[0].Embedding[0] != 0.5 {
		t.Errorf("embeddings=%+v", got)
	}
}

func TestBedrockEmbedShortCircuitsEmptyInput(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	m := newBedrockForTest("http://unused")
	got, err := m.Embed(ctx, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("Embed empty: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got)=%d want 0", len(got))
	}
}

func TestBedrockEmbedRequiresAPIKeyAndModel(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	m := newBedrockForTest("http://unused")
	model := "x"
	if _, err := m.Embed(ctx, &model, []string{"a"}, &APIConfig{}, nil, nil); err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("Embed: want api-key error, got %v", err)
	}
	key := validBedrockKey()
	blank := " "
	if _, err := m.Embed(ctx, &blank, []string{"a"}, &APIConfig{ApiKey: &key}, nil, nil); err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Errorf("Embed: want model-required error, got %v", err)
	}
}

func TestBedrockEmbedRejectsUnsupportedModel(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	m := newBedrockForTest("http://unused")
	key := validBedrockKey()
	model := "anthropic.claude-3-haiku-20240307-v1:0"
	if _, err := m.Embed(ctx, &model, []string{"a"}, &APIConfig{ApiKey: &key}, nil, nil); err == nil || !strings.Contains(err.Error(), "unsupported embedding model") {
		t.Errorf("Embed: want unsupported-model error, got %v", err)
	}
}

func TestBedrockEmbedPropagatesHTTPError(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newBedrockServer(t, http.MethodPost,
		"/model/amazon.titan-embed-text-v2:0/invoke",
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"bad input"}`))
		})
	defer srv.Close()

	m := newBedrockForTest(srv.URL)
	key := validBedrockKey()
	model := "amazon.titan-embed-text-v2:0"
	if _, err := m.Embed(ctx, &model, []string{"a"}, &APIConfig{ApiKey: &key}, nil, nil); err == nil || !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "bad input") {
		t.Errorf("Embed: want HTTP error with body, got %v", err)
	}
}

func TestBedrockRerankReturnsNoSuchMethod(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	m := newBedrockForTest("http://unused")
	model := "x"
	if _, err := m.Rerank(ctx, &model, "q", []string{"a"}, &APIConfig{}, &RerankConfig{TopN: 1}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Rerank: want no-such-method, got %v", err)
	}
}

func TestBedrockBalanceReturnsNoSuchMethod(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	m := newBedrockForTest("http://unused")
	if _, err := m.Balance(ctx, &APIConfig{}); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Balance: want no-such-method, got %v", err)
	}
}

func TestBedrockAudioOCRReturnNoSuchMethod(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	m := newBedrockForTest("http://unused")
	model := "x"
	if _, err := m.TranscribeAudio(ctx, &model, &model, &APIConfig{}, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("TranscribeAudio: want no-such-method, got %v", err)
	}
	if _, err := m.AudioSpeech(ctx, &model, &model, &APIConfig{}, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("AudioSpeech: want no-such-method, got %v", err)
	}
	if _, err := m.OCRFile(ctx, &model, nil, &model, &APIConfig{}, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("OCRFile: want no-such-method, got %v", err)
	}
	if _, err := m.ParseFile(ctx, &model, nil, &model, &APIConfig{}, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("ParseFile: want no-such-method, got %v", err)
	}
	if _, err := m.ListTasks(ctx, &APIConfig{}); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("ListTasks: want no-such-method, got %v", err)
	}
	if _, err := m.ShowTask(ctx, "t", &APIConfig{}); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("ShowTask: want no-such-method, got %v", err)
	}
}

// Compile-time check that BedrockModel satisfies the ModelDriver
// contract. Any missing or mis-typed method shows up here as a build
// error instead of a confusing runtime nil-method-set panic.
var _ ModelDriver = (*BedrockModel)(nil)
