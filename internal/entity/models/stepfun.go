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
	"net/http"
	"ragflow/internal/common"
	"strings"
)

// StepFunModel implements ModelDriver for StepFun (阶跃星辰).
type StepFunModel struct {
	baseModel BaseModel
}

// NewStepFunModel creates a new StepFun model instance.
func NewStepFunModel(baseURL map[string]string, urlSuffix URLSuffix) *StepFunModel {
	return &StepFunModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(),
		},
	}
}

func (s *StepFunModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewStepFunModel(baseURL, s.baseModel.URLSuffix)
}

func (s *StepFunModel) Name() string {
	return "stepfun"
}

// StepFunChatResponse is the StepFun chat completion response. StepFun speaks
// an OpenAI-compatible protocol, so the usage shape matches OpenAI's.
type StepFunChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int    `json:"index"`
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Role             string `json:"role"`
			Content          string `json:"content"`
			Reasoning        string `json:"reasoning"`
			ReasoningContent string `json:"reasoning_content"`
			Audio            *struct {
				Data       string `json:"data"`
				Transcript string `json:"transcript"`
			} `json:"audio,omitempty"`
			ToolCalls []map[string]any `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		TotalTokens         int `json:"total_tokens"`
		PromptTokensDetails *struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details,omitempty"`
		CompletionTokensDetails *struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details,omitempty"`
	} `json:"usage"`
}

// ChatWithMessages sends multiple messages with roles and returns the response.
func (s *StepFunModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if err := s.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	baseURL, err := s.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/%s", baseURL, s.baseModel.URLSuffix.Chat)
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, false)

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

	resp, err := s.baseModel.httpClient.Do(req)
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

	return parseChatCompletionResponse(body, chatModelConfig, modelUsage, func(body []byte, chatConfig *ChatConfig) (chatResponseParts, error) {
		var result StepFunChatResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return chatResponseParts{}, fmt.Errorf("failed to parse response: %w", err)
		}

		if len(result.Choices) == 0 {
			return chatResponseParts{}, fmt.Errorf("no choices in response")
		}

		choice := &result.Choices[0]
		content := choice.Message.Content
		toolCalls := choice.Message.ToolCalls

		reasonContent := choice.Message.Reasoning
		if reasonContent == "" {
			reasonContent = choice.Message.ReasoningContent
		}

		// StepFun reasoning models may emit all output as reasoning/content
		// with the other left empty. Only reject when there is neither
		// content, reasoning, nor tool calls.
		if content == "" && reasonContent == "" && len(toolCalls) == 0 {
			return chatResponseParts{}, fmt.Errorf("no message in response")
		}

		totalTokens := result.Usage.TotalTokens
		if totalTokens == 0 {
			totalTokens = result.Usage.PromptTokens + result.Usage.CompletionTokens
		}

		var usage *TokenUsage
		if totalTokens > 0 {
			usage = &TokenUsage{
				PromptTokens:     result.Usage.PromptTokens,
				CompletionTokens: result.Usage.CompletionTokens,
				TotalTokens:      totalTokens,
			}
		}

		return chatResponseParts{
			RequestID:     result.ID,
			Content:       &content,
			ReasonContent: &reasonContent,
			ToolCalls:     toolCalls,
			Usage:         usage,
		}, nil
	})
}

// ChatStreamlyWithSender sends messages and streams the response
func (s *StepFunModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if err := s.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if sender == nil {
		return fmt.Errorf("sender is required")
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	if chatModelConfig != nil {
		chatModelConfig.ToolCallsResult = nil
		chatModelConfig.UsageResult = nil
	}

	baseURL, err := s.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/%s", baseURL, s.baseModel.URLSuffix.Chat)
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, true)

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := s.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	sawTerminal := false
	accumulatedToolCalls := make(map[int]map[string]any)
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

		reasoningContent, ok := delta["reasoning"].(string)
		if !ok || reasoningContent == "" {
			reasoningContent, _ = delta["reasoning_content"].(string)
		}
		if reasoningContent != "" {
			if err := sender(nil, &reasoningContent); err != nil {
				return err
			}
		}

		content, ok := delta["content"].(string)
		if ok && content != "" {
			if err := sender(&content, nil); err != nil {
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
	setSortedToolCallsResult(chatModelConfig, accumulatedToolCalls)
	if !done && !sawTerminal {
		return fmt.Errorf("stepfun: stream ended before [DONE] or finish_reason")
	}

	endOfStream := "[DONE]"
	if err := sender(&endOfStream, nil); err != nil {
		return err
	}

	return nil
}

// Embed is left as a stub.
func (s *StepFunModel) Embed(ctx context.Context, modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("not implemented")
}

// ListModels returns the list of model ids visible to the API key.
func (s *StepFunModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := s.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	baseURL, err := s.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/%s", baseURL, s.baseModel.URLSuffix.Models)

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := s.baseModel.httpClient.Do(req)
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

// Balance is not exposed by the StepFun API, so this returns "no such method".
func (s *StepFunModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("no such method")
}

// CheckConnection runs a lightweight ListModels call to verify the API key.
func (s *StepFunModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := s.ListModels(ctx, apiConfig)
	if err != nil {
		return err
	}
	return nil
}

// Rerank calculates similarity scores between query and documents. StepFun
// does not expose a public rerank API, so this returns "no such method".
func (s *StepFunModel) Rerank(ctx context.Context, modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	return nil, fmt.Errorf("no such method")
}

// TranscribeAudio transcribe audio
func (s *StepFunModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", s.Name())
}

func (s *StepFunModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", s.Name())
}

// AudioSpeech convert text to audio
func (s *StepFunModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	if err := s.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	// TODO Test it
	if audioContent == nil || *audioContent == "" {
		return nil, fmt.Errorf("audio content is empty")
	}

	resolvedBaseURL, err := s.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, s.baseModel.URLSuffix.TTS)

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

	resp, err := s.baseModel.httpClient.Do(req)
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

// AudioSpeechWithSender for Streaming TTS
func (s *StepFunModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if err := s.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if audioContent == nil || *audioContent == "" {
		return fmt.Errorf("audio content is empty")
	}

	resolvedBaseURL, err := s.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, s.baseModel.URLSuffix.TTS)

	reqBody := map[string]interface{}{
		"model":         *modelName,
		"input":         *audioContent,
		"stream_format": "sse",
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
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := s.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("StepFun stream TTS API error: %d - %s", resp.StatusCode, string(body))
	}

	type ttsEvent struct {
		Type  string `json:"type"`
		Audio string `json:"audio"`
	}
	if _, err := ParseSSEStream[ttsEvent](resp.Body, func(event ttsEvent) error {
		if event.Type == "speech.audio.error" {
			return fmt.Errorf("StepFun stream encountered an error during generation")
		}

		// Extract the Base64 string containing the audio and decode it
		if event.Type == "speech.audio.delta" && event.Audio != "" {
			audioBytes, err := base64.StdEncoding.DecodeString(event.Audio)
			if err == nil && len(audioBytes) > 0 {
				chunk := string(audioBytes)
				if errSend := sender(&chunk, nil); errSend != nil {
					return errSend
				}
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}

	return nil
}

// OCRFile OCR file
func (s *StepFunModel) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", s.Name())
}

// ParseFile parse file
func (s *StepFunModel) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", s.Name())
}

func (s *StepFunModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", s.Name())
}

func (s *StepFunModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", s.Name())
}
