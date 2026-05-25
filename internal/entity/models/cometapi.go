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
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// CometAPIModel implements ModelDriver for CometAPI AI.
//
// CometAPI exposes OpenAI-compatible chat and embeddings under
// https://api.cometapi.com/v1, a public model catalog under
// https://api.cometapi.com/api/models, and account quota data through the
// separate query service at https://query.cometapi.com/user/quota.
type CometAPIModel struct {
	BaseURL    map[string]string
	URLSuffix  URLSuffix
	httpClient *http.Client
}

// NewCometAPIModel creates a new CometAPI model instance.
//
// We clone http.DefaultTransport so we keep Go's defaults for
// ProxyFromEnvironment, DialContext (with KeepAlive), HTTP/2,
// TLSHandshakeTimeout, and ExpectContinueTimeout, and only override
// the connection-pool fields we care about.
//
// The Client itself has no Timeout. http.Client.Timeout would also
// cap the time spent reading the response body, which would cut off
// long-lived SSE streams in ChatStreamlyWithSender. Non-streaming
// callers wrap each request with context.WithTimeout instead.
func NewCometAPIModel(baseURL map[string]string, urlSuffix URLSuffix) *CometAPIModel {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = 90 * time.Second
	transport.DisableCompression = false
	transport.ResponseHeaderTimeout = 60 * time.Second

	return &CometAPIModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
		httpClient: &http.Client{
			Transport: transport,
		},
	}
}

func (m *CometAPIModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewCometAPIModel(baseURL, m.URLSuffix)
}

func (m *CometAPIModel) Name() string {
	return "cometapi"
}

func validateCometAPIAPIKey(apiConfig *APIConfig) (string, error) {
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return "", fmt.Errorf("api key is required")
	}
	return *apiConfig.ApiKey, nil
}

func validateCometAPIModelName(modelName string) error {
	if strings.TrimSpace(modelName) == "" {
		return fmt.Errorf("model name is required")
	}
	return nil
}

func cometapiRegion(apiConfig *APIConfig) string {
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		return *apiConfig.Region
	}
	return "default"
}

// baseURLForRegion returns the base URL for the given region, or an
// error if no entry exists. This makes a misconfigured region fail
// fast with a clear message, instead of silently producing a relative
// URL that the HTTP transport then rejects.
func (m *CometAPIModel) baseURLForRegion(region string) (string, error) {
	base, ok := m.BaseURL[region]
	if !ok || base == "" {
		return "", fmt.Errorf("cometapi: no base URL configured for region %q", region)
	}
	return strings.TrimRight(base, "/"), nil
}

func (m *CometAPIModel) endpointURL(region, suffix string) (string, error) {
	baseURL, err := m.baseURLForRegion(region)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s", baseURL, strings.TrimLeft(suffix, "/")), nil
}

func (m *CometAPIModel) balanceURL(apiKey string) string {
	rawURL := strings.TrimSpace(m.URLSuffix.Balance)
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = fmt.Sprintf("https://query.cometapi.com/%s", strings.TrimLeft(rawURL, "/"))
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	query := parsed.Query()
	query.Set("key", apiKey)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

type cometapiChatRequest struct {
	Model       string               `json:"model"`
	Messages    []cometapiAPIMessage `json:"messages"`
	Stream      bool                 `json:"stream"`
	MaxTokens   *int                 `json:"max_tokens,omitempty"`
	Temperature *float64             `json:"temperature,omitempty"`
	TopP        *float64             `json:"top_p,omitempty"`
	Stop        *[]string            `json:"stop,omitempty"`
}

type cometapiAPIMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

func buildCometAPIChatRequest(modelName string, messages []Message, stream bool, chatModelConfig *ChatConfig) cometapiChatRequest {
	apiMessages := make([]cometapiAPIMessage, len(messages))
	for i, msg := range messages {
		apiMessages[i] = cometapiAPIMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	reqBody := cometapiChatRequest{
		Model:    modelName,
		Messages: apiMessages,
		Stream:   stream,
	}
	if chatModelConfig != nil {
		reqBody.MaxTokens = chatModelConfig.MaxTokens
		reqBody.Temperature = chatModelConfig.Temperature
		reqBody.TopP = chatModelConfig.TopP
		reqBody.Stop = chatModelConfig.Stop
	}
	return reqBody
}

func newCometAPIJSONRequest(ctx context.Context, method string, endpoint string, payload interface{}, apiKey string) (*http.Request, error) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	}
	return req, nil
}

type cometapiHTTPResponse struct {
	StatusCode int
	Status     string
	Body       []byte
}

func (m *CometAPIModel) doCometAPIRequest(req *http.Request) (*cometapiHTTPResponse, error) {
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return &cometapiHTTPResponse{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Body:       body,
	}, nil
}

type cometapiChatResponsePayload struct {
	Choices []cometapiChatChoice `json:"choices"`
}

type cometapiChatChoice struct {
	Message      cometapiChatMessage `json:"message"`
	Delta        cometapiChatDelta   `json:"delta"`
	FinishReason string              `json:"finish_reason"`
}

type cometapiChatMessage struct {
	Content          *string `json:"content"`
	ReasoningContent string  `json:"reasoning_content"`
}

type cometapiChatDelta struct {
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content"`
}

func parseCometAPIChatResponse(body []byte) (*ChatResponse, error) {
	var parsed cometapiChatResponsePayload
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}
	if parsed.Choices[0].Message.Content == nil {
		return nil, fmt.Errorf("invalid content format")
	}

	content := *parsed.Choices[0].Message.Content
	reasonContent := strings.TrimLeft(parsed.Choices[0].Message.ReasoningContent, "\n")
	return &ChatResponse{
		Answer:        &content,
		ReasonContent: &reasonContent,
	}, nil
}

func parseCometAPIStreamEvent(data string) (content string, reasonContent string, terminal bool, ok bool) {
	var event cometapiChatResponsePayload
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return "", "", false, false
	}
	if len(event.Choices) == 0 {
		return "", "", false, false
	}
	choice := event.Choices[0]
	return choice.Delta.Content, choice.Delta.ReasoningContent, choice.FinishReason != "", true
}

type cometapiModelCatalogResponse struct {
	Data []cometapiModelCatalogItem `json:"data"`
}

type cometapiModelCatalogItem struct {
	ID string `json:"id"`
}

func parseCometAPIModelCatalog(body []byte) ([]string, error) {
	var parsed cometapiModelCatalogResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	models := make([]string, 0, len(parsed.Data))
	for _, model := range parsed.Data {
		if model.ID != "" {
			models = append(models, model.ID)
		}
	}
	return models, nil
}

// ChatWithMessages sends multiple messages with roles and returns the response.
func (m *CometAPIModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	apiKey, err := validateCometAPIAPIKey(apiConfig)
	if err != nil {
		return nil, err
	}
	if err := validateCometAPIModelName(modelName); err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	url, err := m.endpointURL(cometapiRegion(apiConfig), m.URLSuffix.Chat)
	if err != nil {
		return nil, err
	}

	// Note: do NOT propagate chatModelConfig.Stream into the request body
	// here. ChatWithMessages parses a single JSON response, so stream must
	// always be off for this code path.
	reqBody := buildCometAPIChatRequest(modelName, messages, false, chatModelConfig)

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := newCometAPIJSONRequest(ctx, "POST", url, reqBody, apiKey)
	if err != nil {
		return nil, err
	}
	resp, err := m.doCometAPIRequest(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(resp.Body))
	}
	return parseCometAPIChatResponse(resp.Body)
}

// ChatStreamlyWithSender sends messages and streams the response via the
// sender function. The CometAPI SSE stream uses the same shape as OpenAI:
// "data:" lines carrying JSON events, with a final "[DONE]" line.
func (m *CometAPIModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, sender func(*string, *string) error) error {
	if sender == nil {
		return fmt.Errorf("sender is required")
	}

	if err := validateCometAPIModelName(modelName); err != nil {
		return err
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	apiKey, err := validateCometAPIAPIKey(apiConfig)
	if err != nil {
		return err
	}

	url, err := m.endpointURL(cometapiRegion(apiConfig), m.URLSuffix.Chat)
	if err != nil {
		return err
	}

	if chatModelConfig != nil {
		// Refuse to run if the caller explicitly asked for stream=false.
		// The body of this method only knows how to read SSE, so a
		// non-SSE JSON response would be parsed as if it were a stream
		// and produce no chunks. Better to fail clearly.
		if chatModelConfig.Stream != nil && !*chatModelConfig.Stream {
			return fmt.Errorf("stream must be true in ChatStreamlyWithSender")
		}
	}
	reqBody := buildCometAPIChatRequest(modelName, messages, true, chatModelConfig)

	// Use an explicit background context. SSE streams are long-lived
	// so we do not attach a hard deadline here; the transport's
	// ResponseHeaderTimeout caps the connection-establishment phase.
	req, err := newCometAPIJSONRequest(context.Background(), "POST", url, reqBody, apiKey)
	if err != nil {
		return err
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// SSE parsing: bump the scanner buffer from the 64KB default to 1MB
	// so we never silently truncate a long data: line.
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	sawTerminal := false
	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(line[5:])

		if data == "[DONE]" {
			sawTerminal = true
			break
		}

		content, reasoningContent, terminal, ok := parseCometAPIStreamEvent(data)
		if !ok {
			continue
		}

		if reasoningContent != "" {
			if err := sender(nil, &reasoningContent); err != nil {
				return err
			}
		}

		if content != "" {
			if err := sender(&content, nil); err != nil {
				return err
			}
		}

		if terminal {
			sawTerminal = true
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}
	if !sawTerminal {
		return fmt.Errorf("cometapi: stream ended before [DONE] or finish_reason")
	}

	endOfStream := "[DONE]"
	if err := sender(&endOfStream, nil); err != nil {
		return err
	}

	return nil
}

type cometapiEmbeddingData struct {
	Embedding []float64 `json:"embedding"`
	Object    string    `json:"object"`
	Index     int       `json:"index"`
}

type cometapiEmbeddingResponse struct {
	Data   []cometapiEmbeddingData `json:"data"`
	Model  string                  `json:"model"`
	Object string                  `json:"object"`
}

type cometapiEmbeddingRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions int      `json:"dimensions,omitempty"`
}

// Embed turns a list of texts into embedding vectors using the
// CometAPI /v1/embeddings endpoint. The output has one vector per input,
// in the same order the inputs were given.
func (m *CometAPIModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	if len(texts) == 0 {
		return []EmbeddingData{}, nil
	}

	apiKey, err := validateCometAPIAPIKey(apiConfig)
	if err != nil {
		return nil, err
	}

	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}

	url, err := m.endpointURL(cometapiRegion(apiConfig), m.URLSuffix.Embedding)
	if err != nil {
		return nil, err
	}

	reqBody := cometapiEmbeddingRequest{
		Model: *modelName,
		Input: texts,
	}
	if embeddingConfig != nil && embeddingConfig.Dimension > 0 {
		reqBody.Dimensions = embeddingConfig.Dimension
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := newCometAPIJSONRequest(ctx, "POST", url, reqBody, apiKey)
	if err != nil {
		return nil, err
	}

	resp, err := m.doCometAPIRequest(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CometAPI embeddings API error: %s, body: %s", resp.Status, string(resp.Body))
	}

	var parsed cometapiEmbeddingResponse
	if err = json.Unmarshal(resp.Body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Reorder the returned vectors by their reported index so the output
	// always lines up with the input texts, even if the upstream API ever
	// returns items out of order. A nil slot at the end indicates the
	// upstream did not return an embedding for that input.
	embeddings := make([]EmbeddingData, len(texts))
	filled := make([]bool, len(texts))
	for _, item := range parsed.Data {
		if item.Index < 0 || item.Index >= len(texts) {
			return nil, fmt.Errorf("cometapi: response index %d out of range for %d inputs", item.Index, len(texts))
		}
		if filled[item.Index] {
			// A malformed response that repeats the same index would
			// silently overwrite the earlier vector. Fail loudly so
			// the caller never uses ambiguous output.
			return nil, fmt.Errorf("cometapi: duplicate embedding index %d in response", item.Index)
		}
		embeddings[item.Index] = EmbeddingData{
			Embedding: item.Embedding,
			Index:     item.Index,
		}
		filled[item.Index] = true
	}
	for i, ok := range filled {
		if !ok {
			return nil, fmt.Errorf("cometapi: missing embedding for input index %d", i)
		}
	}

	return embeddings, nil
}

// ListModels returns the public CometAPI model catalog.
func (m *CometAPIModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	url, err := m.endpointURL(cometapiRegion(apiConfig), m.URLSuffix.Models)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := m.doCometAPIRequest(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(resp.Body))
	}
	return parseCometAPIModelCatalog(resp.Body)
}

// Balance queries CometAPI's quota service. Unlike model requests, this
// endpoint authenticates with the key query parameter on query.cometapi.com.
func (m *CometAPIModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}
	if strings.TrimSpace(m.URLSuffix.Balance) == "" {
		return nil, fmt.Errorf("balance URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", m.balanceURL(*apiConfig.ApiKey), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := m.doCometAPIRequest(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CometAPI quota API error: %s, body: %s", resp.Status, string(resp.Body))
	}

	var result map[string]interface{}
	if err = json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

// CheckConnection runs a quota query to verify the API key.
func (m *CometAPIModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := m.Balance(apiConfig)
	if err != nil {
		return err
	}
	return nil
}

// Rerank calculates similarity scores between query and documents. CometAPI
// does not expose a public rerank API, so this returns "no such method".
func (m *CometAPIModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("no such method")
}

// TranscribeAudio transcribe audio
func (m *CometAPIModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	if file == nil || *file == "" {
		return nil, fmt.Errorf("file is missing")
	}

	region := "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/%s", m.BaseURL[region], m.URLSuffix.ASR)

	// multipart body
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// open audio file
	audioFile, err := os.Open(*file)
	if err != nil {
		return nil, fmt.Errorf("failed to open audio file: %w", err)
	}
	defer audioFile.Close()

	// create multipart file field
	part, err := writer.CreateFormFile("file", filepath.Base(*file))
	if err != nil {
		return nil, fmt.Errorf("failed to create multipart file: %w", err)
	}

	// copy file content
	if _, err = io.Copy(part, audioFile); err != nil {
		return nil, fmt.Errorf("failed to copy audio data: %w", err)
	}

	// model field
	if err := writer.WriteField("model", *modelName); err != nil {
		return nil, fmt.Errorf("failed to write model field: %w", err)
	}

	// extra params
	if asrConfig != nil && asrConfig.Params != nil {
		for key, value := range asrConfig.Params {

			var val string

			switch v := value.(type) {
			case string:
				val = v
			case bool:
				val = strconv.FormatBool(v)
			case int:
				val = strconv.Itoa(v)
			case int64:
				val = strconv.FormatInt(v, 10)
			case float32:
				val = strconv.FormatFloat(float64(v), 'f', -1, 32)
			case float64:
				val = strconv.FormatFloat(v, 'f', -1, 64)
			default:
				val = fmt.Sprintf("%v", v)
			}

			if err = writer.WriteField(key, val); err != nil {
				return nil, fmt.Errorf("failed to write field %s: %w", key, err)
			}
		}
	}

	if err = writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// build request
	req, err := http.NewRequest("POST", url, &body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	// send request
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SiliconFlow ASR error: %s - %s", resp.Status, string(respBody))
	}

	// SiliconFlow response
	var result struct {
		Text string `json:"text"`
	}

	if err = json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w, body=%s", err, string(respBody))
	}

	return &ASRResponse{Text: result.Text}, nil
}

func (m *CometAPIModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", m.Name())
}

// AudioSpeech synthesizes speech audio from text.
func (m *CometAPIModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	if audioContent == nil || *audioContent == "" {
		return nil, fmt.Errorf("audio content is empty")
	}

	var region = "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/%s", m.BaseURL[region], m.URLSuffix.TTS)

	reqBody := map[string]interface{}{
		"model": *modelName,
		"input": *audioContent,
	}

	if ttsConfig != nil && ttsConfig.Params != nil {
		for key, value := range ttsConfig.Params {
			reqBody[key] = value
		}
	}
	if ttsConfig != nil && ttsConfig.Format != "" {
		reqBody["response_format"] = ttsConfig.Format
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s - %s", resp.Status, string(body))
	}

	return &TTSResponse{Audio: body}, nil
}

func (m *CometAPIModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", m.Name())
}

// OCRFile OCR file
func (m *CometAPIModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *CometAPIModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *CometAPIModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *CometAPIModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}
