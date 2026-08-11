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
	"net/http"
	"ragflow/internal/common"
	"strconv"
	"strings"
)

// DeepSeekModel implements ModelDriver for DeepSeek
type DeepSeekModel struct {
	baseModel BaseModel
}

// NewDeepSeekModel creates a new DeepSeek model instance
func NewDeepSeekModel(baseURL map[string]string, urlSuffix URLSuffix) *DeepSeekModel {
	return &DeepSeekModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(),
		},
	}
}

func (d *DeepSeekModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewDeepSeekModel(baseURL, d.baseModel.URLSuffix)
}

func (d *DeepSeekModel) Name() string {
	return "deepseek"
}

// DeepSeekChatResponse mirrors the response returned by DeepSeek's
// POST /chat/completions endpoint. The shape follows the official OpenAI-
// compatible response schema, including reasoning content, tool calls, and
// the cache/reasoning token usage details.
type DeepSeekChatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Index        int    `json:"index"`
		Message      struct {
			Content          string           `json:"content"`
			ReasoningContent string           `json:"reasoning_content"`
			Role             string           `json:"role"`
			ToolCalls        []map[string]any `json:"tool_calls"`
		} `json:"message"`
		Logprobs interface{} `json:"logprobs"`
	} `json:"choices"`
	Created           int    `json:"created"`
	Model             string `json:"model"`
	SystemFingerprint string `json:"system_fingerprint"`
	Object            string `json:"object"`
	Usage             struct {
		CompletionTokens        int `json:"completion_tokens"`
		PromptTokens            int `json:"prompt_tokens"`
		TotalTokens             int `json:"total_tokens"`
		PromptCacheHitTokens    int `json:"prompt_cache_hit_tokens"`
		PromptCacheMissTokens   int `json:"prompt_cache_miss_tokens"`
		CompletionTokensDetails struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
	} `json:"usage"`
}

func (d *DeepSeekModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if err := d.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := d.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, d.baseModel.URLSuffix.Chat)

	// Build request body
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, false)
	if chatModelConfig != nil && chatModelConfig.Thinking != nil && *chatModelConfig.Thinking {
		var thinkingFlag string
		effort := "high"
		if chatModelConfig.Effort != nil {
			effort = *chatModelConfig.Effort
		}
		switch effort {
		case "none":
			thinkingFlag = "disabled"
			break
		case "low":
			thinkingFlag = "disabled"
			break
		case "medium":
			thinkingFlag = "disabled"
			break
		case "high":
			thinkingFlag = "enabled"
			reqBody["reasoning_effort"] = "high"
			break
		case "default":
			thinkingFlag = "enabled"
			reqBody["reasoning_effort"] = "high"
			break
		case "max":
			thinkingFlag = "enabled"
			reqBody["reasoning_effort"] = "max"
			break
		}
		reqBody["thinking"] = map[string]interface{}{
			"type": thinkingFlag,
		}
	} else {
		reqBody["thinking"] = map[string]interface{}{
			"type": "disabled",
		}
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

	resp, err := d.baseModel.httpClient.Do(req)
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

	return parseChatCompletionResponse(body, chatModelConfig, modelUsage, func(body []byte, _ *ChatConfig) (chatResponseParts, error) {
		var result DeepSeekChatResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return chatResponseParts{}, fmt.Errorf("failed to parse response: %w", err)
		}

		if len(result.Choices) == 0 {
			return chatResponseParts{}, fmt.Errorf("no choices in response")
		}

		choice := &result.Choices[0]
		reasonContent := choice.Message.ReasoningContent
		// DeepSeek may prefix reasoning content with a newline in non-streaming
		// responses; keep the existing provider behavior of removing that prefix.
		if reasonContent != "" && reasonContent[0] == '\n' {
			reasonContent = reasonContent[1:]
		}

		return chatResponseParts{
			RequestID:     result.ID,
			Content:       &choice.Message.Content,
			ReasonContent: &reasonContent,
			ToolCalls:     choice.Message.ToolCalls,
			Usage: &TokenUsage{
				PromptTokens:     result.Usage.PromptTokens,
				CompletionTokens: result.Usage.CompletionTokens,
				TotalTokens:      result.Usage.TotalTokens,
			},
		}, nil
	})
}

// ChatStreamlyWithSender sends messages and streams response via sender function (best performance, no channel)
func (d *DeepSeekModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if err := d.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := d.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/chat/completions", resolvedBaseURL)

	// Build request body with streaming enabled
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, true)
	if chatModelConfig != nil && chatModelConfig.Thinking != nil && *chatModelConfig.Thinking {
		var thinkingFlag string
		effort := "high"
		if chatModelConfig.Effort != nil {
			effort = *chatModelConfig.Effort
		}
		switch effort {
		case "none":
			thinkingFlag = "disabled"
		case "low":
			thinkingFlag = "disabled"
		case "medium":
			thinkingFlag = "disabled"
		case "high":
			thinkingFlag = "enabled"
			reqBody["reasoning_effort"] = "high"
		case "default":
			thinkingFlag = "enabled"
			reqBody["reasoning_effort"] = "high"
		case "max":
			thinkingFlag = "enabled"
			reqBody["reasoning_effort"] = "max"
		default:
			return fmt.Errorf("invalid effort level")
		}
		reqBody["thinking"] = map[string]interface{}{
			"type": thinkingFlag,
		}
	} else {
		reqBody["thinking"] = map[string]interface{}{
			"type": "disabled",
		}
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, streamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := d.baseModel.httpClient.Do(req)
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

	// Send [DONE] marker for OpenAI compatibility
	endOfStream := "[DONE]"
	return sender(&endOfStream, nil)
}

// Embed embeds a list of texts into embeddings
func (d *DeepSeekModel) Embed(ctx context.Context, modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s, no such method", d.Name())
}

func (d *DeepSeekModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := d.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	resolvedBaseURL, err := d.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, d.baseModel.URLSuffix.Models)

	// Build request body
	reqBody := map[string]interface{}{}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := d.baseModel.httpClient.Do(req)
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

	return ParseListModel(modelList), nil
}

// deepseekBalanceResponse is the shape returned by
// GET /user/balance. The balance fields are strings in the
// upstream API, so we parse them on our side.
type deepseekBalanceResponse struct {
	IsAvailable  bool `json:"is_available"`
	BalanceInfos []struct {
		Currency        string `json:"currency"`
		TotalBalance    string `json:"total_balance"`
		GrantedBalance  string `json:"granted_balance"`
		ToppedUpBalance string `json:"topped_up_balance"`
	} `json:"balance_infos"`
}

// Balance returns the user's available balance on DeepSeek by
// calling GET /user/balance with the configured Bearer token.
// The result map matches the shape used by the Moonshot driver,
// so the UI can render it without provider-specific code.
func (d *DeepSeekModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	if err := d.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	baseURL, err := d.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), d.baseModel.URLSuffix.Balance)

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := d.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DeepSeek balance API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed deepseekBalanceResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(parsed.BalanceInfos) == 0 {
		return nil, fmt.Errorf("no balance info in response")
	}

	// Pick the first balance entry, the same way the Moonshot
	// driver returns a single {balance, currency} pair to the UI.
	first := parsed.BalanceInfos[0]
	total, err := strconv.ParseFloat(first.TotalBalance, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid total_balance %q: %w", first.TotalBalance, err)
	}

	return map[string]interface{}{
		"balance":  total,
		"currency": first.Currency,
	}, nil
}

func (d *DeepSeekModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := d.ListModels(ctx, apiConfig)
	if err != nil {
		return err
	}
	return nil
}

// Rerank calculates similarity scores between query and documents
func (d *DeepSeekModel) Rerank(ctx context.Context, modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, Rerank not implemented", d.Name())
}

// TranscribeAudio transcribe audio
func (d *DeepSeekModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", d.Name())
}

func (d *DeepSeekModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", d.Name())
}

// AudioSpeech convert text to audio
func (d *DeepSeekModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", d.Name())
}

func (d *DeepSeekModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", d.Name())
}

// OCRFile OCR file
func (d *DeepSeekModel) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", d.Name())
}

// ParseFile parse file
func (d *DeepSeekModel) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", d.Name())
}

func (d *DeepSeekModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", d.Name())
}

func (d *DeepSeekModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", d.Name())
}
