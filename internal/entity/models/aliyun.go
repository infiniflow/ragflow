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
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"ragflow/internal/common"
	"strconv"
	"strings"
)

// AliyunModel implements ModelDriver for Aliyun
type AliyunModel struct {
	baseModel BaseModel
}

// NewAliyunModel creates a new Aliyun model instance
func NewAliyunModel(baseURL map[string]string, urlSuffix URLSuffix) *AliyunModel {
	return &AliyunModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(),
		},
	}
}

func (a *AliyunModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewAliyunModel(baseURL, a.baseModel.URLSuffix)
}

func (a *AliyunModel) Name() string {
	return "aliyun"
}

func (a *AliyunModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if err := a.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := a.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL := resolvedBaseURL

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), a.baseModel.URLSuffix.Chat)

	// Convert messages to the format expected by API
	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	// Build request body
	reqBody := map[string]interface{}{
		"model":       modelName,
		"messages":    apiMessages,
		"stream":      false,
		"temperature": 1,
	}

	if chatModelConfig != nil {
		if chatModelConfig.Stream != nil {
			reqBody["stream"] = *chatModelConfig.Stream
		}

		if chatModelConfig.MaxTokens != nil {
			reqBody["max_tokens"] = *chatModelConfig.MaxTokens
		}

		if chatModelConfig.Temperature != nil {
			reqBody["temperature"] = *chatModelConfig.Temperature
		}

		if chatModelConfig.TopP != nil {
			reqBody["top_p"] = *chatModelConfig.TopP
		}

		if chatModelConfig.Stop != nil {
			reqBody["stop"] = *chatModelConfig.Stop
		}

		if chatModelConfig.Thinking != nil {
			if *chatModelConfig.Thinking {
				reqBody["enable_thinking"] = true
			} else {
				reqBody["enable_thinking"] = false
			}
		}
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := a.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result map[string]interface{}
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	firstChoice, ok := choices[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid choice format")
	}

	messageMap, ok := firstChoice["message"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid message format")
	}

	answer, ok := messageMap["content"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid content format")
	}

	var reasonContent string
	if chatModelConfig != nil && chatModelConfig.Thinking != nil && *chatModelConfig.Thinking {
		reasonContent, ok = messageMap["reasoning_content"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid content format")
		}
		// if first char of reasonContent is \n remove the '\n'
		if reasonContent != "" && reasonContent[0] == '\n' {
			reasonContent = reasonContent[1:]
		}
	}

	chatResponse := &ChatResponse{
		Answer:        &answer,
		ReasonContent: &reasonContent,
	}

	return chatResponse, nil
}

// ChatStreamlyWithSender sends messages and streams response via sender function (best performance, no channel)
func (a *AliyunModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, sender func(*string, *string) error) error {
	if err := a.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := a.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	baseURL := resolvedBaseURL

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), a.baseModel.URLSuffix.Chat)

	// Convert messages to API format
	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	// Build request body with streaming enabled
	reqBody := map[string]interface{}{
		"model":       modelName,
		"messages":    apiMessages,
		"stream":      true,
		"temperature": 1,
	}

	if chatModelConfig.Stream != nil {
		reqBody["stream"] = *chatModelConfig.Stream
	}

	if chatModelConfig.MaxTokens != nil {
		reqBody["max_tokens"] = *chatModelConfig.MaxTokens
	}

	if chatModelConfig.Temperature != nil {
		reqBody["temperature"] = *chatModelConfig.Temperature
	}

	if chatModelConfig.DoSample != nil {
		reqBody["do_sample"] = *chatModelConfig.DoSample
	}

	if chatModelConfig.TopP != nil {
		reqBody["top_p"] = *chatModelConfig.TopP
	}

	if chatModelConfig.Stop != nil {
		reqBody["stop"] = *chatModelConfig.Stop
	}

	if chatModelConfig.Thinking != nil {
		if *chatModelConfig.Thinking {
			reqBody["enable_thinking"] = true
		} else {
			reqBody["enable_thinking"] = false
		}
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), streamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := a.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// SSE parsing: read line by line
	if _, err := ParseSSEStream[map[string]interface{}](resp.Body, func(event map[string]interface{}) error {
		common.Info(fmt.Sprintf("%v", event))

		choices, ok := event["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			return nil
		}

		firstChoice, ok := choices[0].(map[string]interface{})
		if !ok {
			return nil
		}

		delta, ok := firstChoice["delta"].(map[string]interface{})
		if !ok {
			return nil
		}

		content, ok := delta["content"].(string)
		if ok && content != "" {
			if err := sender(&content, nil); err != nil {
				return err
			}
		}

		reasoningContent, ok := delta["reasoning_content"].(string)
		if ok && reasoningContent != "" {
			if err := sender(nil, &reasoningContent); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}

	// Send [DONE] marker for OpenAI compatibility
	endOfStream := "[DONE]"
	return sender(&endOfStream, nil)
}

type aliyunEmbeddingResponse struct {
	Data   []EmbeddingData `json:"data"`
	Model  string          `json:"model"`
	Object string          `json:"object"`
	Usage  aliyunUsage     `json:"usage"`
	ID     string          `json:"id"`
}

type aliyunEmbeddingData struct {
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
	Object    string    `json:"object"`
}

type aliyunUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// Embed embeds a list of texts into embeddings
func (a *AliyunModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	if err := a.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(texts) == 0 {
		return []EmbeddingData{}, nil
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	resolvedBaseURL, err := a.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL := resolvedBaseURL

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), a.baseModel.URLSuffix.Embedding)

	reqBody := map[string]interface{}{
		"model": *modelName,
		"input": texts,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := a.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Aliyun embeddings API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed aliyunEmbeddingResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var embeddings []EmbeddingData
	for _, dataElem := range parsed.Data {
		var embeddingData EmbeddingData
		embeddingData.Embedding = dataElem.Embedding
		embeddingData.Index = dataElem.Index
		embeddings = append(embeddings, embeddingData)
	}

	return embeddings, nil
}

type aliyunRerankRequest struct {
	Model           string   `json:"model"`
	Query           string   `json:"query"`
	Documents       []string `json:"documents"`
	TopN            int      `json:"top_n"`
	ReturnDocuments bool     `json:"return_documents"`
}

type aliyunRerankResponse struct {
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
}

func (a *AliyunModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	if err := a.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(documents) == 0 {
		return &RerankResponse{}, nil
	}
	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	resolvedBaseURL, err := a.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL := resolvedBaseURL

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), a.baseModel.URLSuffix.Rerank)

	var topN = rerankConfig.TopN
	if rerankConfig.TopN == 0 {
		topN = len(documents)
	}

	reqBody := aliyunRerankRequest{
		Model:           *modelName,
		Query:           query,
		Documents:       documents,
		TopN:            topN,
		ReturnDocuments: false,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := a.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Aliyun rerank API error: %s, body: %s", resp.Status, string(body))
	}

	var rerankResponse RerankResponse
	if err = json.Unmarshal(body, &rerankResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &rerankResponse, nil
}

// TranscribeAudio transcribe audio
func (a *AliyunModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AliyunModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", a.Name())
}

// AudioSpeech convert text to audio
func (a *AliyunModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AliyunModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", a.Name())
}

// OCRFile OCR file
func (a *AliyunModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

// ParseFile parse file
func (a *AliyunModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

type AliyunModelItem struct {
	ModelName    string `json:"model_name"`
	BaseCapacity int    `json:"base_capacity"`
}

type AliyunModelOutput struct {
	Models   []AliyunModelItem `json:"models"`
	PageNo   int               `json:"page_no"`
	PageSize int               `json:"page_size"`
	Total    int               `json:"total"`
}

type AliyunModelList struct {
	RequestID string            `json:"request_id"`
	Output    AliyunModelOutput `json:"output"`
}

func (a *AliyunModel) ListModels(apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := a.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	resolvedBaseURL, err := a.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL := resolvedBaseURL

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), a.baseModel.URLSuffix.Models)

	// Build request body
	reqBody := map[string]interface{}{}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := a.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var modelList ModelList
	if err = json.Unmarshal(body, &modelList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return ParseListModel(modelList), nil
}

// bssEndpointForRegion picks the Aliyun BSS Open API host for a given
// DashScope-side region label. BSS endpoints are NOT per-Aliyun-region — there
// are only two public hosts (mainland and international), and an account is
// served by exactly one of them based on where it was created. Callers using
// the "singapore" region for the DashScope chat path are international
// accounts and must hit the international host.
func bssEndpointForRegion(region string) string {
	switch region {
	case "singapore", "intl", "international", "ap-southeast-1", "ap-southeast-5":
		return "https://business.ap-southeast-1.aliyuncs.com"
	default:
		return "https://business.aliyuncs.com"
	}
}

// bssRegionIDForRegion is the query-param "RegionId" value passed in the BSS
// QueryAccountBalance call. Aliyun accepts any region this account is allowed
// to touch and uses it only for routing/locale; the balance itself is account-
// wide. Defaulting to "cn-hangzhou" on the mainland host and "ap-southeast-1"
// on the international host matches what aliyun-cli does.
func bssRegionIDForRegion(region string) string {
	switch region {
	case "singapore", "intl", "international", "ap-southeast-1":
		return "ap-southeast-1"
	case "ap-southeast-5":
		return "ap-southeast-5"
	default:
		return "cn-hangzhou"
	}
}

// aliyunBSSBalanceData is the inner block returned by Aliyun's
// BSS QueryAccountBalance (version 2017-12-14). Reference:
// https://www.alibabacloud.com/help/en/bss-openapi/latest/api-bssopenapi-2017-12-14-queryaccountbalance
//
// Every numeric field is emitted as a JSON STRING by BSS (not a JSON number),
// which is why these are typed as string + parsed via strconv at extraction
// time. Currency is "CNY" for mainland accounts and "USD" for international
// accounts.
type aliyunBSSBalanceData struct {
	AvailableAmount     string `json:"AvailableAmount"`
	AvailableCashAmount string `json:"AvailableCashAmount"`
	Credit              string `json:"CreditAmount"`
	MybankCreditAmount  string `json:"MybankCreditAmount"`
	Currency            string `json:"Currency"`
}

// aliyunBSSBalanceResponse is the BSS envelope around the balance payload.
// On error, Data is empty/zero and Code carries an Aliyun error code such as
// "InvalidAccessKeyId.NotFound" / "NoPermission" / "BusinessAvailable.Forbidden"
// with a human-readable Message.
type aliyunBSSBalanceResponse struct {
	Code      string               `json:"Code"`
	Message   string               `json:"Message"`
	RequestID string               `json:"RequestId"`
	Success   bool                 `json:"Success"`
	Data      aliyunBSSBalanceData `json:"Data"`
}

// aliyunRandomNonce generates an 8-byte (16-char hex) value for the
// X-Acs-Signature-Nonce header. Aliyun only requires uniqueness over a 15-min
// window per AccessKey; 64 bits of entropy is comfortably enough.
func aliyunRandomNonce() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("aliyun: nonce generation failed: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}

// Balance queries the Aliyun account balance via the BSS Open API:
//
//	GET business{,-ap-southeast-1}.aliyuncs.com/?Action=QueryAccountBalance&Version=2017-12-14&RegionId=...
//
// Authentication is Aliyun SigV3 (ACS3-HMAC-SHA256) signed with the tenant's
// AccessKey ID + AccessKey Secret, NOT the DashScope/Bailian "sk-..." key used
// for the chat / embedding paths. The two credential systems coexist on every
// Aliyun account but are not interchangeable; DashScope itself does not expose
// any per-key balance endpoint. Callers must therefore supply
// APIConfig.AccessKeyID + APIConfig.AccessKeySecret separately from
// APIConfig.ApiKey.
//
// On success the method returns {balance: float64, currency: string}, where
// balance is parsed from BSS's AvailableAmount (string-encoded decimal) and
// currency is what BSS reports ("CNY" or "USD"). The shape matches what the
// sibling SiliconFlow / Moonshot / DeepSeek Balance methods already emit so
// the UI's balance panel renders identically.
func (a *AliyunModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	if apiConfig == nil {
		return nil, fmt.Errorf("api key is required")
	}
	if apiConfig.AccessKeyID == nil || *apiConfig.AccessKeyID == "" ||
		apiConfig.AccessKeySecret == nil || *apiConfig.AccessKeySecret == "" {
		return nil, fmt.Errorf("aliyun balance requires AccessKeyID and AccessKeySecret " +
			"(the BSS QueryAccountBalance endpoint cannot be authenticated with a DashScope sk- key)")
	}

	region := "default"
	if apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}
	endpoint := bssEndpointForRegion(region)

	q := url.Values{}
	q.Set("Action", "QueryAccountBalance")
	q.Set("Version", "2017-12-14")
	q.Set("RegionId", bssRegionIDForRegion(region))

	rawURL := fmt.Sprintf("%s/?%s", endpoint, q.Encode())

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("aliyun: failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	nonce, err := aliyunRandomNonce()
	if err != nil {
		return nil, err
	}
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	if err := signAliyunV3(req,
		*apiConfig.AccessKeyID,
		*apiConfig.AccessKeySecret,
		"QueryAccountBalance",
		"2017-12-14",
		nonce, timestamp,
		nil, // GET request, empty body
	); err != nil {
		return nil, err
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("aliyun: failed to send BSS request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("aliyun: failed to read BSS response: %w", err)
	}

	return parseAliyunBSSBalanceResponse(resp.StatusCode, body)
}

// parseAliyunBSSBalanceResponse decodes the BSS QueryAccountBalance reply and
// extracts {balance, currency} from it, surfacing every BSS-side error
// (HTTP, JSON, API code) as a Go error with the original RequestId attached
// so the maintainer can grep for it in BSS audit logs.
func parseAliyunBSSBalanceResponse(statusCode int, body []byte) (map[string]interface{}, error) {
	// BSS returns its error envelope with HTTP 4xx in some failure modes and
	// HTTP 200 with Success=false in others; treat anything non-2xx as an
	// error but still try to parse the envelope so we can surface Code/Msg.
	var parsed aliyunBSSBalanceResponse
	if jsonErr := json.Unmarshal(body, &parsed); jsonErr != nil {
		if statusCode != http.StatusOK {
			return nil, fmt.Errorf("Aliyun BSS API error: status %d, body: %s", statusCode, string(body))
		}
		return nil, fmt.Errorf("aliyun: failed to parse BSS response: %w (body: %s)", jsonErr, string(body))
	}

	if statusCode != http.StatusOK || (!parsed.Success && parsed.Code != "" && parsed.Code != "Success") {
		msg := parsed.Message
		if msg == "" {
			msg = "unknown BSS API error"
		}
		code := parsed.Code
		if code == "" {
			code = fmt.Sprintf("HTTP_%d", statusCode)
		}
		if parsed.RequestID != "" {
			return nil, fmt.Errorf("Aliyun BSS API error (code %s, requestId %s): %s", code, parsed.RequestID, msg)
		}
		return nil, fmt.Errorf("Aliyun BSS API error (code %s): %s", code, msg)
	}

	raw := parsed.Data.AvailableAmount
	if raw == "" {
		raw = parsed.Data.AvailableCashAmount
	}
	if raw == "" {
		return nil, fmt.Errorf("aliyun: no balance amount in BSS response (requestId=%s)", parsed.RequestID)
	}
	amount, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, fmt.Errorf("aliyun: invalid BSS balance amount %q: %w", raw, err)
	}

	currency := parsed.Data.Currency
	if currency == "" {
		currency = "CNY"
	}
	return map[string]interface{}{
		"balance":  amount,
		"currency": currency,
	}, nil
}

func (a *AliyunModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := a.ListModels(apiConfig)
	if err != nil {
		return err
	}
	return nil
}

func (a *AliyunModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AliyunModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}
