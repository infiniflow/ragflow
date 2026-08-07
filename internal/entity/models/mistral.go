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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"ragflow/internal/common"
	"strconv"
	"strings"
	"time"
)

// normalizeMistralStructuredContent rewrites a Mistral magistral response
// whose message.content is a structured array ([{type:text},{type:thinking,
// thinking:[{type:text}]}]) into the flat string shape the shared handler
// expects: content becomes the concatenated text parts and a top-level
// reasoning_content carries the thinking parts. The rewrite is in-place and
// only applied when content is actually an array.
func normalizeMistralStructuredContent(body []byte) []byte {
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return body
	}
	choices, ok := result["choices"].([]any)
	if !ok || len(choices) == 0 {
		return body
	}
	firstChoice, ok := choices[0].(map[string]any)
	if !ok {
		return body
	}
	messageMap, ok := firstChoice["message"].(map[string]any)
	if !ok {
		return body
	}
	parts, ok := messageMap["content"].([]any)
	if !ok {
		return body
	}

	var answer, reasoning strings.Builder
	for _, p := range parts {
		part, ok := p.(map[string]any)
		if !ok {
			continue
		}
		switch part["type"] {
		case "text":
			if t, ok := part["text"].(string); ok {
				answer.WriteString(t)
			}
		case "thinking":
			if thinking, ok := part["thinking"].([]any); ok {
				for _, tp := range thinking {
					if tpm, ok := tp.(map[string]any); ok {
						if t, ok := tpm["text"].(string); ok {
							reasoning.WriteString(t)
						}
					}
				}
			}
		}
	}
	// Only rewrite if we actually extracted something; otherwise leave
	// the body untouched so the shared handler surfaces its normal error.
	if answer.Len() == 0 && reasoning.Len() == 0 {
		return body
	}

	messageMap["content"] = answer.String()
	if existing, ok := messageMap["reasoning_content"].(string); ok && existing != "" {
		reasoning.WriteString(existing)
	}
	if reasoning.Len() > 0 {
		messageMap["reasoning_content"] = reasoning.String()
	}
	out, err := json.Marshal(result)
	if err != nil {
		return body
	}
	return out
}

type MistralModel struct {
	baseModel BaseModel
}

// NewMistralModel creates a new Mistral model instance.
func NewMistralModel(baseURL map[string]string, urlSuffix URLSuffix) *MistralModel {
	return &MistralModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(false),
		},
	}
}

func (m *MistralModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewMistralModel(baseURL, m.baseModel.URLSuffix)
}

func (m *MistralModel) Name() string {
	return "mistral"
}

// ChatWithMessages sends multiple messages with roles and returns the response.
func (m *MistralModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	baseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", baseURL, m.baseModel.URLSuffix.Chat)
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, false)

	body, err := m.baseModel.doRequest(ctx, url, apiConfig, reqBody, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	// Mistral magistral returns content as a structured array. Normalize it
	// to the flat string shape the shared handler understands.
	body = normalizeMistralStructuredContent(body)

	return HandleNonStreamingResponse(body, modelUsage, chatModelConfig, OpenAIParserConfig)
}

// ChatStreamlyWithSender sends messages and streams the response
func (m *MistralModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if sender == nil {
		return fmt.Errorf("sender is required")
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}
	if err := validateStreamConfig(chatModelConfig); err != nil {
		return err
	}

	baseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/%s", baseURL, m.baseModel.URLSuffix.Chat)
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, true)
	reqBody["stream_options"] = map[string]interface{}{
		"include_usage": true,
	}

	return m.baseModel.doStreamRequest(ctx, url, apiConfig, reqBody, streamCallTimeout, func(body io.ReadCloser) error {
		return HandleStreamingResponse(body, modelUsage, chatModelConfig, OpenAIParserConfig, sender)
	})
}

type mistralEmbeddingData struct {
	Embedding []float64 `json:"embedding"`
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Usage     struct {
		CompletionTokens   int `json:"completion_tokens"`
		NumCachedTokens    int `json:"num_cached_tokens"`
		PromptAudioSeconds int `json:"prompt_audio_seconds"`
		PromptTokenDetails struct {
			CachedTokens int `json:"cached_tokens"`
		}
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	}
}

type mistralEmbeddingResponse struct {
	Data   []mistralEmbeddingData `json:"data"`
	Model  string                 `json:"model"`
	Object string                 `json:"object"`
}

// Embed turns a list of texts into embedding vectors
func (m *MistralModel) Embed(ctx context.Context, modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(texts) == 0 {
		return []EmbeddingData{}, nil
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	baseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/%s", baseURL, m.baseModel.URLSuffix.Embedding)

	reqBody := map[string]interface{}{
		"model": *modelName,
		"input": texts,
	}
	if embeddingConfig != nil && embeddingConfig.Dimension > 0 {
		reqBody["output_dimension"] = embeddingConfig.Dimension
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := m.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Mistral embeddings API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed mistralEmbeddingResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	embeddings := make([]EmbeddingData, len(texts))
	filled := make([]bool, len(texts))
	for _, item := range parsed.Data {
		if item.Index < 0 || item.Index >= len(texts) {
			return nil, fmt.Errorf("mistral: response index %d out of range for %d inputs", item.Index, len(texts))
		}
		if filled[item.Index] {
			return nil, fmt.Errorf("mistral: duplicate embedding index %d in response", item.Index)
		}
		embeddings[item.Index] = EmbeddingData{
			Embedding: item.Embedding,
			Index:     item.Index,
		}
		filled[item.Index] = true
	}
	for i, ok := range filled {
		if !ok {
			return nil, fmt.Errorf("mistral: missing embedding for input index %d", i)
		}
	}

	return embeddings, nil
}

// ListModels returns the list of model ids visible to the API key.
func (m *MistralModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	baseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/%s", baseURL, m.baseModel.URLSuffix.Models)

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := m.baseModel.httpClient.Do(req)
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
	var modelList ModelList
	if err = json.Unmarshal(body, &modelList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if modelList.Models == nil {
		return nil, fmt.Errorf("invalid models list format")
	}

	return ParseListModel(modelList), nil
}

// Balance is not exposed by the Mistral API, so this returns "no such method".
func (m *MistralModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

// CheckConnection runs a lightweight ListModels call to verify the API key.
func (m *MistralModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := m.ListModels(ctx, apiConfig)
	return err
}

// Rerank calculates similarity scores between query and documents
func (m *MistralModel) Rerank(ctx context.Context, modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

// TranscribeAudio transcribe audio
func (m *MistralModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}

	if file == nil || *file == "" {
		return nil, fmt.Errorf("file is missing")
	}

	resolvedBaseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, m.baseModel.URLSuffix.ASR)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// codeql[go/path-injection] False positive: *file is the audio file path the caller passes in to upload. The user (or operator-supplied pipeline) explicitly chose this path, and the OS access check enforces permissions anyway.
	audioFile, err := os.Open(*file)
	if err != nil {
		return nil, fmt.Errorf("failed to open audio file: %w", err)
	}
	defer audioFile.Close()

	// create multipart file field
	part, err := writer.CreateFormFile(
		"file",
		filepath.Base(*file),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create multipart file: %w", err)
	}

	// copy file content
	if _, err = io.Copy(part, audioFile); err != nil {
		return nil, fmt.Errorf("failed to copy audio data: %w", err)
	}

	if err := writer.WriteField("model", strings.TrimSpace(*modelName)); err != nil {
		return nil, fmt.Errorf("failed to write model field: %w", err)
	}

	if asrConfig != nil && asrConfig.Params != nil {
		for key, value := range asrConfig.Params {
			if err = writeMistralFormField(writer, key, value); err != nil {
				return nil, fmt.Errorf("failed to write field %s: %w", key, err)
			}
		}
	}

	if err = writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, longOpCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, &body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", strings.TrimSpace(*apiConfig.ApiKey)))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	resp, err := m.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Mistral ASR API error: %s, body: %s", resp.Status, string(respBody))
	}

	var result struct {
		Text string `json:"text"`
	}

	if err = json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse Mistral ASR response: %w, body=%s", err, string(respBody))
	}
	if result.Text == "" {
		return nil, fmt.Errorf("Mistral ASR response missing text")
	}

	return &ASRResponse{Text: result.Text}, nil
}

func writeMistralFormField(writer *multipart.Writer, key string, value interface{}) error {
	var fieldValue string
	switch v := value.(type) {
	case string:
		fieldValue = v
	case bool:
		fieldValue = strconv.FormatBool(v)
	case int:
		fieldValue = strconv.Itoa(v)
	case int64:
		fieldValue = strconv.FormatInt(v, 10)
	case float32:
		fieldValue = strconv.FormatFloat(float64(v), 'f', -1, 32)
	case float64:
		fieldValue = strconv.FormatFloat(v, 'f', -1, 64)
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		fieldValue = string(data)
	}
	return writer.WriteField(key, fieldValue)
}

func (m *MistralModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", m.Name())
}

// AudioSpeech convert text to audio
func (m *MistralModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if audioContent == nil || *audioContent == "" {
		return nil, fmt.Errorf("text content is empty")
	}

	resolvedBaseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, m.baseModel.URLSuffix.TTS)

	reqBody := map[string]interface{}{
		"model": strings.TrimSpace(*modelName),
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
	reqBody["stream"] = false

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, longOpCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", strings.TrimSpace(*apiConfig.ApiKey)))

	resp, err := m.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Mistral TTS API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result struct {
		AudioData string `json:"audio_data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// format HEX
	audioBytes, err := base64.StdEncoding.DecodeString(result.AudioData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode Mistral base64 audio: %w", err)
	}

	return &TTSResponse{
		Audio: audioBytes,
	}, nil
}

func (m *MistralModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", m.Name())
}

// OCRFile OCR file
func (m *MistralModel) OCRFile(ctx context.Context, modelName *string, content []byte, urls *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if (urls == nil || *urls == "") && (content == nil || len(content) == 0) {
		return nil, fmt.Errorf("file url or content is required")
	}

	resolvedBaseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, m.baseModel.URLSuffix.OCR)

	var docURL string
	if urls != nil && *urls != "" {
		docURL = *urls
	} else {
		mimeType := http.DetectContentType(content)
		base64Str := base64.StdEncoding.EncodeToString(content)
		docURL = fmt.Sprintf("data:%s;base64,%s", mimeType, base64Str)
	}

	reqData := map[string]interface{}{
		"model": *modelName,
		"document": map[string]interface{}{
			"type":         "document_url",
			"document_url": docURL,
		},
	}

	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal json payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := m.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Mistral OCR API error: %s, body: %s", resp.Status, string(body))
	}

	var mistralResp struct {
		Pages []struct {
			Index    int    `json:"index"`
			Markdown string `json:"markdown"`
		} `json:"pages"`
	}

	if err = json.Unmarshal(body, &mistralResp); err != nil {
		return nil, fmt.Errorf("failed to parse response json: %w", err)
	}

	var fullMarkdown strings.Builder
	for _, page := range mistralResp.Pages {
		fullMarkdown.WriteString(page.Markdown)
		fullMarkdown.WriteString("\n\n")
	}

	resultText := strings.TrimSpace(fullMarkdown.String())

	return &OCRFileResponse{
		Text: &resultText,
	}, nil
}

func (m *MistralModel) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *MistralModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *MistralModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}
