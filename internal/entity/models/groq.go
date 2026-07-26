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
)

// GroqModel implements ModelDriver for Groq.
type GroqModel struct {
	baseModel BaseModel
}

func NewGroqModel(baseURL map[string]string, urlSuffix URLSuffix) *GroqModel {
	return &GroqModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(),
		},
	}
}

func (g *GroqModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewGroqModel(baseURL, g.baseModel.URLSuffix)
}

func (g *GroqModel) Name() string {
	return "groq"
}

func (g *GroqModel) endpoint(apiConfig *APIConfig, suffix string) (string, error) {
	baseURL, err := g.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return "", err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	return fmt.Sprintf("%s/%s", baseURL, strings.TrimPrefix(suffix, "/")), nil
}

func applyGroqReasoningRequestParams(reqBody map[string]any, modelName string, chatModelConfig *ChatConfig) {
	modelLower := strings.ToLower(modelName)
	if strings.Contains(modelLower, "gpt-oss") {
		reqBody["include_reasoning"] = true
		if chatModelConfig != nil && chatModelConfig.Effort != nil {
			reqBody["reasoning_effort"] = *chatModelConfig.Effort
		}
	} else if strings.Contains(modelLower, "qwen") || strings.Contains(modelLower, "deepseek") {
		reqBody["reasoning_format"] = "parsed"
	}
}

type groqChatMessage struct {
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content"`
	Reasoning        string `json:"reasoning"`
}

type groqChatChoice struct {
	Message      groqChatMessage `json:"message"`
	Delta        groqChatMessage `json:"delta"`
	FinishReason string          `json:"finish_reason"`
}

type groqChatResponse struct {
	Choices      []groqChatChoice `json:"choices"`
	Error        interface{}      `json:"error"`
	FinishReason string           `json:"finish_reason"`
}

func (g *GroqModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if strings.TrimSpace(modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	url, err := g.endpoint(apiConfig, g.baseModel.URLSuffix.Chat)
	if err != nil {
		return nil, err
	}

	reqBody := buildRequestBody(chatModelConfig, modelName, messages, false)
	applyGroqReasoningRequestParams(reqBody, modelName, chatModelConfig)
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := g.baseModel.httpClient.Do(req)
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

	var result groqChatResponse
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("groq: upstream error: %v", result.Error)
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	content := result.Choices[0].Message.Content
	reasonContent := result.Choices[0].Message.ReasoningContent
	if reasonContent == "" {
		reasonContent = result.Choices[0].Message.Reasoning
	}

	return &ChatResponse{
		Answer:        &content,
		ReasonContent: &reasonContent,
	}, nil
}

func (g *GroqModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if sender == nil {
		return fmt.Errorf("sender is required")
	}
	if strings.TrimSpace(modelName) == "" {
		return fmt.Errorf("model name is required")
	}
	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}
	if chatModelConfig != nil && chatModelConfig.Stream != nil && !*chatModelConfig.Stream {
		return fmt.Errorf("stream must be true in ChatStreamlyWithSender")
	}

	url, err := g.endpoint(apiConfig, g.baseModel.URLSuffix.Chat)
	if err != nil {
		return err
	}

	reqBody := buildRequestBody(chatModelConfig, modelName, messages, true)
	applyGroqReasoningRequestParams(reqBody, modelName, chatModelConfig)
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, streamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))
	req.Header.Set("Accept", "text/event-stream")

	resp, err := g.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	sawTerminal := false
	accumulatedToolCalls := make(map[int]map[string]interface{})
	done, err := ParseSSEStream[map[string]interface{}](resp.Body, func(event map[string]interface{}) error {
		common.Info(fmt.Sprintf("%v", event))

		tokenUsage, found, usageErr := decodeOpenAICompatibleStreamUsage(event)
		if usageErr != nil {
			return usageErr
		}
		if found {
			applyStreamUsage(chatModelConfig, modelUsage, tokenUsage)
		}

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

		accumulateToolCallDeltas(delta, accumulatedToolCalls)

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

		finishReason, ok := firstChoice["finish_reason"].(string)
		if ok && finishReason != "" {
			sawTerminal = true
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}
	if !done && !sawTerminal {
		return fmt.Errorf("deepseek: stream ended before [DONE] or finish_reason")
	}

	setSortedToolCallsResult(chatModelConfig, accumulatedToolCalls)
	endOfStream := "[DONE]"
	return sender(&endOfStream, nil)
}

type groqModelInfo struct {
	ID string `json:"id"`
}

type groqListModelsResponse struct {
	Data  []ModelListItem `json:"data"`
	Error interface{}     `json:"error"`
}

func (g *GroqModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	url, err := g.endpoint(apiConfig, g.baseModel.URLSuffix.Models)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := g.baseModel.httpClient.Do(req)
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

	var result groqListModelsResponse
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("groq: upstream error: %v", result.Error)
	}

	return ParseListModel(ModelList{Models: result.Data}), nil
}

func (g *GroqModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := g.ListModels(ctx, apiConfig)
	return err
}

func (g *GroqModel) Embed(ctx context.Context, modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GroqModel) Rerank(ctx context.Context, modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GroqModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GroqModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if file == nil || *file == "" {
		return nil, fmt.Errorf("file is missing")
	}

	resolvedBaseURL, err := g.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, g.baseModel.URLSuffix.ASR)

	// multipart body
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// open audio file

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

	// send request
	resp, err := g.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Groq ASR error: %s - %s", resp.Status, string(respBody))
	}

	// response
	var result struct {
		Text string `json:"text"`
	}

	if err = json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w, body=%s", err, string(respBody))
	}

	return &ASRResponse{Text: result.Text}, nil
}

func (g *GroqModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", g.Name())
}

func (g *GroqModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if audioContent == nil || *audioContent == "" {
		return nil, fmt.Errorf("audio content is empty")
	}

	resolvedBaseURL, err := g.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, g.baseModel.URLSuffix.TTS)

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

	resp, err := g.baseModel.httpClient.Do(req)
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

func (g *GroqModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", g.Name())
}

func (g *GroqModel) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GroqModel) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GroqModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GroqModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}
