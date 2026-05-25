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
	"net/http"
	"strings"
	"time"
)

// azureAPIVersion is the Azure OpenAI REST API version sent as the
// api-version query parameter on every request. Azure requires this on
// all data-plane calls; 2024-10-21 is the latest GA (non-preview) version.
const azureAPIVersion = "2024-10-21"

// AzureOpenAIModel implements ModelDriver for Azure OpenAI.
//
// Azure OpenAI is not a base-URL swap of the OpenAI driver. It differs in
// three ways that this driver handles:
//   - Endpoints are deployment-scoped:
//     {baseURL}/deployments/{deployment}/{op}?api-version={azureAPIVersion}
//     The model name passed in is the Azure deployment name, which goes in
//     the URL path rather than the request body.
//   - Authentication uses the "api-key" header, not "Authorization: Bearer".
//   - Listing models means listing deployments via {baseURL}/deployments.
//
// The base URL is user-supplied (e.g. https://<resource>.openai.azure.com/openai)
// because each Azure resource has its own endpoint; there is no shared default.
type AzureOpenAIModel struct {
	BaseURL    map[string]string
	URLSuffix  URLSuffix
	httpClient *http.Client // Reusable HTTP client with connection pool
}

// NewAzureOpenAIModel creates a new Azure OpenAI model instance.
//
// The transport mirrors the OpenAI driver: clone http.DefaultTransport to
// keep Go's defaults (proxy, dial keep-alive, HTTP/2, TLS handshake) and
// only tune the connection pool. The Client has no overall Timeout so SSE
// streams in ChatStreamlyWithSender are not cut off; non-streaming callers
// wrap each request in context.WithTimeout, and ResponseHeaderTimeout caps
// how long we wait for the first response header.
func NewAzureOpenAIModel(baseURL map[string]string, urlSuffix URLSuffix) *AzureOpenAIModel {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = 90 * time.Second
	transport.DisableCompression = false
	transport.ResponseHeaderTimeout = 60 * time.Second

	return &AzureOpenAIModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
		httpClient: &http.Client{
			Transport: transport,
		},
	}
}

func (z *AzureOpenAIModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewAzureOpenAIModel(baseURL, z.URLSuffix)
}

func (z *AzureOpenAIModel) Name() string {
	return "azure-openai"
}

// baseURLForRegion returns the base URL for the given region, or an error if
// no entry exists. A misconfigured region fails fast with a clear message
// instead of silently producing a relative URL the transport then rejects.
func (z *AzureOpenAIModel) baseURLForRegion(region string) (string, error) {
	base, ok := z.BaseURL[region]
	if !ok || base == "" {
		return "", fmt.Errorf("azure-openai: no base URL configured for region %q", region)
	}
	return base, nil
}

// deploymentURL builds a deployment-scoped data-plane URL of the form
// {baseURL}/deployments/{deployment}/{op}?api-version={azureAPIVersion}.
func (z *AzureOpenAIModel) deploymentURL(baseURL, deployment, op string) string {
	return fmt.Sprintf("%s/deployments/%s/%s?api-version=%s",
		strings.TrimRight(baseURL, "/"), deployment, op, azureAPIVersion)
}

// ChatWithMessages sends multiple messages with roles and returns the response.
func (z *AzureOpenAIModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	if modelName == "" {
		return nil, fmt.Errorf("deployment name is required")
	}

	region := "default"
	if apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	baseURL, err := z.baseURLForRegion(region)
	if err != nil {
		return nil, err
	}
	url := z.deploymentURL(baseURL, modelName, z.URLSuffix.Chat)

	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	// The deployment (and therefore the model) is identified by the URL path,
	// so Azure does not take a "model" field in the body.
	reqBody := map[string]interface{}{
		"messages":    apiMessages,
		"stream":      false,
		"temperature": 1,
	}

	if chatModelConfig != nil {
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
	req.Header.Set("api-key", *apiConfig.ApiKey)

	resp, err := z.httpClient.Do(req)
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

	content, ok := messageMap["content"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid content format")
	}

	var reasonContent string
	if rc, ok := messageMap["reasoning_content"].(string); ok {
		reasonContent = rc
		if reasonContent != "" && reasonContent[0] == '\n' {
			reasonContent = reasonContent[1:]
		}
	}

	return &ChatResponse{
		Answer:        &content,
		ReasonContent: &reasonContent,
	}, nil
}

// ChatStreamlyWithSender sends messages and streams the response via the
// sender function. Used for streaming chat responses with no extra channel.
func (z *AzureOpenAIModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, sender func(*string, *string) error) error {
	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return fmt.Errorf("api key is required")
	}

	if modelName == "" {
		return fmt.Errorf("deployment name is required")
	}

	if sender == nil {
		return fmt.Errorf("sender is required")
	}

	region := "default"
	if apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	baseURL, err := z.baseURLForRegion(region)
	if err != nil {
		return err
	}
	url := z.deploymentURL(baseURL, modelName, z.URLSuffix.Chat)

	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	reqBody := map[string]interface{}{
		"messages": apiMessages,
		"stream":   true,
	}

	if chatModelConfig != nil {
		// This code path only knows how to read SSE, so refuse an explicit
		// stream=false rather than mis-parsing a single JSON response as a
		// stream and emitting no chunks.
		if chatModelConfig.Stream != nil && !*chatModelConfig.Stream {
			return fmt.Errorf("stream must be true in ChatStreamlyWithSender")
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
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Background context: SSE streams are long-lived so we attach no hard
	// deadline. The transport's ResponseHeaderTimeout caps connection setup.
	req, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", *apiConfig.ApiKey)

	resp, err := z.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// SSE parsing: bump the scanner buffer to 1MB so a long data: line is
	// never silently truncated by the default 64KB cap.
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	// sawTerminal flips true when upstream signals the stream is done (a
	// "[DONE]" marker or a non-empty finish_reason). If the body closes
	// before either, we must not emit a synthetic "[DONE]" that would hide
	// a truncated response from the caller.
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

		var event map[string]interface{}
		if err = json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		choices, ok := event["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			continue
		}

		firstChoice, ok := choices[0].(map[string]interface{})
		if !ok {
			continue
		}

		delta, ok := firstChoice["delta"].(map[string]interface{})
		if !ok {
			continue
		}

		if reasoningContent, ok := delta["reasoning_content"].(string); ok && reasoningContent != "" {
			if err := sender(nil, &reasoningContent); err != nil {
				return err
			}
		}

		if content, ok := delta["content"].(string); ok && content != "" {
			if err := sender(&content, nil); err != nil {
				return err
			}
		}

		if finishReason, ok := firstChoice["finish_reason"].(string); ok && finishReason != "" {
			sawTerminal = true
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}
	if !sawTerminal {
		return fmt.Errorf("azure-openai: stream ended before [DONE] or finish_reason")
	}

	endOfStream := "[DONE]"
	return sender(&endOfStream, nil)
}

type azureEmbeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

// Embed turns a list of texts into embedding vectors using the Azure OpenAI
// embeddings deployment. The output has one vector per input, in the same
// order the inputs were given.
func (z *AzureOpenAIModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	if len(texts) == 0 {
		return []EmbeddingData{}, nil
	}

	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("deployment name is required")
	}

	region := "default"
	if apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	baseURL, err := z.baseURLForRegion(region)
	if err != nil {
		return nil, err
	}
	url := z.deploymentURL(baseURL, *modelName, z.URLSuffix.Embedding)

	// As with chat, the deployment is in the URL path, so no "model" field.
	reqBody := map[string]interface{}{
		"input": texts,
	}
	if embeddingConfig != nil && embeddingConfig.Dimension > 0 {
		reqBody["dimensions"] = embeddingConfig.Dimension
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
	req.Header.Set("api-key", *apiConfig.ApiKey)

	resp, err := z.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Azure OpenAI embeddings API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed azureEmbeddingResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Azure returns one item per input, but does not guarantee response
	// order. Each item's Index refers to its position in the input list, so
	// place vectors by Index to honor the documented input-order guarantee.
	// Reject an out-of-range index instead of panicking.
	embeddings := make([]EmbeddingData, len(texts))
	for _, d := range parsed.Data {
		if d.Index < 0 || d.Index >= len(embeddings) {
			return nil, fmt.Errorf("embedding index %d out of range [0,%d)", d.Index, len(embeddings))
		}
		embeddings[d.Index] = EmbeddingData{
			Embedding: d.Embedding,
			Index:     d.Index,
		}
	}

	return embeddings, nil
}

// ListModels returns the deployment names visible to the configured API key.
// Azure exposes deployments (not a shared model catalog) at
// {baseURL}/deployments?api-version={azureAPIVersion}.
func (z *AzureOpenAIModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}

	region := "default"
	if apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	baseURL, err := z.baseURLForRegion(region)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s?api-version=%s",
		strings.TrimRight(baseURL, "/"), z.URLSuffix.Models, azureAPIVersion)

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("api-key", *apiConfig.ApiKey)

	resp, err := z.httpClient.Do(req)
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

	var result map[string]interface{}
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	data, ok := result["data"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid deployments list format")
	}

	models := make([]string, 0, len(data))
	for _, item := range data {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		// Each deployment object exposes its name under "id".
		id, ok := m["id"].(string)
		if !ok {
			continue
		}
		models = append(models, id)
	}

	return models, nil
}

// CheckConnection runs a lightweight ListModels call to verify the endpoint
// and API key.
func (z *AzureOpenAIModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := z.ListModels(apiConfig)
	return err
}

// Balance is not exposed by the Azure OpenAI API.
func (z *AzureOpenAIModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("no such method")
}

// Rerank is not exposed by the Azure OpenAI API.
func (z *AzureOpenAIModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, no such method", z.Name())
}

func (z *AzureOpenAIModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", z.Name())
}

func (z *AzureOpenAIModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", z.Name())
}

func (z *AzureOpenAIModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", z.Name())
}

func (z *AzureOpenAIModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", z.Name())
}

func (z *AzureOpenAIModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", z.Name())
}

func (z *AzureOpenAIModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", z.Name())
}

func (z *AzureOpenAIModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", z.Name())
}

func (z *AzureOpenAIModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", z.Name())
}
