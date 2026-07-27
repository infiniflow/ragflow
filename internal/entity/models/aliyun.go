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
	"strings"

	"ragflow/internal/common"
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
	return "Tongyi-Qianwen"
}

// AliyunChatResponse mirrors DashScope's OpenAI-compatible chat response.
type AliyunChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Index        int    `json:"index"`
		Logprobs     any    `json:"logprobs"`
		Message      struct {
			Content          string           `json:"content"`
			ReasoningContent string           `json:"reasoning_content"`
			Role             string           `json:"role"`
			ToolCalls        []map[string]any `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	SystemFingerprint string `json:"system_fingerprint"`
	Usage             struct {
		CompletionTokens        int `json:"completion_tokens"`
		PromptTokens            int `json:"prompt_tokens"`
		TotalTokens             int `json:"total_tokens"`
		CompletionTokensDetails struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
		PromptTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}

func (a *AliyunModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
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
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, a.baseModel.URLSuffix.Chat)

	// Build request body
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, false)

	if chatModelConfig != nil {

		if chatModelConfig.Thinking != nil {
			if *chatModelConfig.Thinking {
				reqBody["enable_thinking"] = true
			} else {
				reqBody["enable_thinking"] = false
			}
		}

		if chatModelConfig.Tools != nil {
			reqBody["tool_choice"] = aliyunToolChoice(modelName, messages, chatModelConfig.ToolChoice)
		}
	}

	// For qwen3 models on DashScope, enable_thinking defaults to true when
	// omitted. RAGFlow's default is to disable thinking unless explicitly
	// enabled by the user, matching Python's chat_model.py behavior.
	applyQwen3ThinkingDefault(modelName, reqBody)

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

	return parseChatCompletionResponse(body, chatModelConfig, modelUsage, func(body []byte, chatConfig *ChatConfig) (chatResponseParts, error) {
		var result AliyunChatResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return chatResponseParts{}, fmt.Errorf("failed to parse response: %w", err)
		}
		if len(result.Choices) == 0 {
			return chatResponseParts{}, fmt.Errorf("no choices in response")
		}

		choice := &result.Choices[0]
		content := choice.Message.Content
		if content == "" && len(choice.Message.ToolCalls) == 0 {
			return chatResponseParts{}, fmt.Errorf("response contains neither content nor tool calls")
		}
		reasonContent := ""
		if chatConfig != nil && chatConfig.Thinking != nil && *chatConfig.Thinking {
			reasonContent = choice.Message.ReasoningContent
			if reasonContent == "" {
				reasoning, answer := GetThinkingAndAnswer(chatConfig.ModelClass, &content)
				if reasoning != nil {
					reasonContent = *reasoning
					content = *answer
				}
			}
			if reasonContent != "" && reasonContent[0] == '\n' {
				reasonContent = reasonContent[1:]
			}
		}

		return chatResponseParts{
			RequestID:     result.ID,
			Content:       &content,
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
func (a *AliyunModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
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
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, a.baseModel.URLSuffix.Chat)

	// Build request body with streaming enabled
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, true)
	reqBody["stream_options"] = map[string]interface{}{"include_usage": true}

	if chatModelConfig != nil {
		if chatModelConfig.Stream != nil && !*chatModelConfig.Stream {
			return fmt.Errorf("stream must be true in ChatStreamlyWithSender")
		}
		chatModelConfig.ToolCallsResult = nil

		if chatModelConfig.Thinking != nil {
			reqBody["enable_thinking"] = *chatModelConfig.Thinking
		}

		if chatModelConfig.Tools != nil {
			reqBody["tool_choice"] = aliyunToolChoice(modelName, messages, chatModelConfig.ToolChoice)
		}
	}

	// For qwen3 models on DashScope, enable_thinking defaults to true when
	// omitted. RAGFlow's default is to disable thinking unless explicitly
	// enabled by the user, matching Python's chat_model.py behavior.
	applyQwen3ThinkingDefault(modelName, reqBody)

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

	resp, err := a.baseModel.httpClient.Do(req)
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
		if finishReason, ok := firstChoice["finish_reason"].(string); ok && finishReason != "" {
			sawTerminal = true
		}

		delta, ok := firstChoice["delta"].(map[string]interface{})
		if !ok {
			return nil
		}

		if accumulateToolCallDeltas(delta, accumulatedToolCalls) {
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
	})
	if err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}
	if !done && !sawTerminal {
		return fmt.Errorf("aliyun: stream ended before [DONE] or finish_reason")
	}

	setSortedToolCallsResult(chatModelConfig, accumulatedToolCalls)

	// Send [DONE] marker for OpenAI compatibility
	endOfStream := "[DONE]"
	return sender(&endOfStream, nil)
}

// applyQwen3ThinkingDefault ensures enable_thinking=false is sent for qwen3
// models when it hasn't been explicitly configured. DashScope defaults
// enable_thinking to true for qwen3 models, which produces reasoning output
// that RAGFlow doesn't expect in most pipelines. Mirrors Python's
// chat_model.py default of enable_thinking=False for qwen3.
func applyQwen3ThinkingDefault(modelName string, reqBody map[string]interface{}) {
	if !strings.Contains(strings.ToLower(modelName), "qwen3") {
		return
	}
	if _, alreadySet := reqBody["enable_thinking"]; alreadySet {
		return
	}
	reqBody["enable_thinking"] = false
}

// aliyunToolChoice prevents qwen-flash from repeatedly issuing another tool call
// after a tool result has already been supplied. With "auto", qwen-flash can
// keep emitting tool_calls even for a successful result until the ReAct graph
// exhausts its step limit. Other models, initial calls, and explicit choices
// retain their configured behavior.
func aliyunToolChoice(modelName string, messages []Message, configured *string) string {
	choice := "auto"
	if configured != nil && strings.TrimSpace(*configured) != "" {
		choice = *configured
	}
	if !strings.EqualFold(strings.TrimSpace(choice), "auto") {
		return choice
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(modelName)), "qwen-flash") {
		return choice
	}
	for _, message := range messages {
		if strings.EqualFold(message.Role, "tool") && message.ToolCallID != "" {
			return "none"
		}
	}
	return choice
}

type aliyunEmbeddingResponse struct {
	Data   []aliyunEmbeddingData `json:"data"`
	Model  string                `json:"model"`
	Object string                `json:"object"`
	Usage  aliyunUsage           `json:"usage"`
	ID     string                `json:"id"`
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
func (a *AliyunModel) Embed(ctx context.Context, modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
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

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
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
	recordResponseUsage(modelUsage, parsed.ID, &TokenUsage{
		PromptTokens: parsed.Usage.PromptTokens,
		TotalTokens:  parsed.Usage.TotalTokens,
	}, "embedding")

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
	ID      string `json:"id"`
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

func (a *AliyunModel) Rerank(ctx context.Context, modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
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

	topN := len(documents)
	if rerankConfig != nil && rerankConfig.TopN > 0 {
		topN = rerankConfig.TopN
	}
	if topN == 0 {
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

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
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

	var parsed aliyunRerankResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	rerankResponse := &RerankResponse{Data: make([]RerankResult, 0, len(parsed.Results))}
	for _, item := range parsed.Results {
		rerankResponse.Data = append(rerankResponse.Data, RerankResult{Index: item.Index, RelevanceScore: item.RelevanceScore})
	}
	recordResponseUsage(modelUsage, parsed.ID, &TokenUsage{
		TotalTokens: parsed.Usage.TotalTokens,
	}, "rerank")

	return rerankResponse, nil
}

// TranscribeAudio transcribe audio
func (a *AliyunModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AliyunModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", a.Name())
}

// AudioSpeech convert text to audio
func (a *AliyunModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AliyunModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", a.Name())
}

// OCRFile OCR file
func (a *AliyunModel) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

// ParseFile parse file
func (a *AliyunModel) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
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

func (a *AliyunModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
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

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
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

func (a *AliyunModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AliyunModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := a.ListModels(ctx, apiConfig)
	if err != nil {
		return err
	}
	return nil
}

func (a *AliyunModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AliyunModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}
