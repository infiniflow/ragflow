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
	"ragflow/internal/engine/clickhouse"
	"sort"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
)

// ZhipuAIModel implements ModelDriver for Zhipu AI
type ZhipuAIModel struct {
	baseModel BaseModel
}

// NewZhipuAIModel creates a new Zhipu AI model instance
func NewZhipuAIModel(baseURL map[string]string, urlSuffix URLSuffix) *ZhipuAIModel {
	return &ZhipuAIModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(),
		},
	}
}

func (z *ZhipuAIModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewZhipuAIModel(baseURL, z.baseModel.URLSuffix)
}

func (z *ZhipuAIModel) Name() string {
	return "zhipu"
}

type ZhipuChatResponse struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Index        int    `json:"index"`
		Message      struct {
			Content          string           `json:"content"`
			ReasoningContent string           `json:"reasoning_content"`
			Role             string           `json:"role"`
			ToolCalls        []map[string]any `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Created   int    `json:"created"`
	Id        string `json:"id"`
	Model     string `json:"model"`
	Object    string `json:"object"`
	RequestId string `json:"request_id"`
	Usage     struct {
		CompletionTokens    int `json:"completion_tokens"`
		PromptTokens        int `json:"prompt_tokens"`
		PromptTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

// ChatWithMessages sends multiple messages with roles and returns response
func (z *ZhipuAIModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if err := z.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	baseURL, err := z.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/%s", baseURL, z.baseModel.URLSuffix.Chat)

	// Convert messages to the format expected by API
	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
		if msg.ToolCallID != "" {
			apiMessages[i]["tool_call_id"] = msg.ToolCallID
		}
		if len(msg.ToolCalls) > 0 {
			apiMessages[i]["tool_calls"] = msg.ToolCalls
		}
	}

	// Build request body
	reqBody := map[string]interface{}{
		"model":    modelName,
		"messages": apiMessages,
		"stream":   false,
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
				reqBody["thinking"] = map[string]interface{}{
					"type": "enabled",
				}
			} else {
				reqBody["thinking"] = map[string]interface{}{
					"type": "disabled",
				}
			}
		}

		if chatModelConfig.Tools != nil {
			reqBody["tools"] = chatModelConfig.Tools
		}
		if chatModelConfig.ToolChoice != nil {
			reqBody["tool_choice"] = chatModelConfig.ToolChoice
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

	resp, err := z.baseModel.httpClient.Do(req)
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
	var result ZhipuChatResponse
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Choices == nil || len(result.Choices) == 0 {
		return nil, fmt.Errorf("empty response")
	}

	content := &result.Choices[0].Message.Content

	var reasonContent *string
	if chatModelConfig != nil && chatModelConfig.Thinking != nil && *chatModelConfig.Thinking {
		reasonContent = &result.Choices[0].Message.ReasoningContent
	}

	var toolCalls = result.Choices[0].Message.ToolCalls

	usage := &TokenUsage{
		PromptTokens:     result.Usage.PromptTokens,
		CompletionTokens: result.Usage.CompletionTokens,
		TotalTokens:      result.Usage.TotalTokens,
	}

	if modelUsage != nil {
		modelUsage.RequestID = result.RequestId
		modelUsage.InputTokens = result.Usage.PromptTokens
		modelUsage.OutputTokens = result.Usage.CompletionTokens
		modelUsage.TotalTokens = result.Usage.TotalTokens
		modelUsage.ResponseTimeMS = time.Since(modelUsage.StartAt).Milliseconds()

		clickhouseDriver := clickhouse.GetDriver()
		err = clickhouseDriver.CollectModelUsage(modelUsage)
		if err != nil {
			return nil, fmt.Errorf("failed to collect model usage: %w", err)
		}
	}

	chatResponse := &ChatResponse{
		Answer:        content,
		ReasonContent: reasonContent,
		Usage:         usage,
		ToolCalls:     toolCalls,
	}

	return chatResponse, nil
}

// ChatStreamlyWithSender sends messages and streams response via sender function (best performance, no channel)
func (z *ZhipuAIModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if err := z.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	baseURL, err := z.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/%s", baseURL, z.baseModel.URLSuffix.Chat)

	// Convert messages to API format
	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
		if msg.ToolCallID != "" {
			apiMessages[i]["tool_call_id"] = msg.ToolCallID
		}

		if len(msg.ToolCalls) > 0 {
			apiMessages[i]["tool_calls"] = msg.ToolCalls
		}
	}

	// Build request body with streaming enabled
	reqBody := map[string]interface{}{
		"model":    modelName,
		"messages": apiMessages,
		"stream":   true,
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
				reqBody["thinking"] = map[string]interface{}{
					"type": "enabled",
				}
			} else {
				reqBody["thinking"] = map[string]interface{}{
					"type": "disabled",
				}
			}
		}

		if chatModelConfig.Tools != nil {
			reqBody["tools"] = chatModelConfig.Tools
		}

		if chatModelConfig.ToolChoice != nil {
			reqBody["tool_choice"] = chatModelConfig.ToolChoice
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

	resp, err := z.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// SSE parsing: read line by line
	accumulatedToolCalls := make(map[int]map[string]any)
	if _, err = ParseSSEStream[map[string]interface{}](resp.Body, func(event map[string]interface{}) error {
		common.Info(fmt.Sprintf("%v", event))

		tokenUsageMap, ok := event["usage"].(map[string]interface{})
		if ok {
			tokenUsage := TokenUsage{}
			if err = mapstructure.Decode(tokenUsageMap, &tokenUsage); err != nil {
				return err
			}
			if chatModelConfig != nil {
				chatModelConfig.UsageResult = &tokenUsage
			}
			if modelUsage != nil {
				modelUsage.InputTokens = tokenUsage.PromptTokens
				modelUsage.OutputTokens = tokenUsage.CompletionTokens
				modelUsage.TotalTokens = tokenUsage.TotalTokens
				modelUsage.ResponseTimeMS = time.Since(modelUsage.StartAt).Milliseconds()
				clickhouseDriver := clickhouse.GetDriver()
				if err = clickhouseDriver.CollectModelUsage(modelUsage); err != nil {
					return err
				}
			}
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

		reasoningContent, ok := delta["reasoning_content"].(string)
		if ok && reasoningContent != "" {
			if err = sender(nil, &reasoningContent); err != nil {
				return err
			}
		}

		if tcs, ok := delta["tool_calls"].([]interface{}); ok {
			for _, tc := range tcs {
				tcMap, ok := tc.(map[string]interface{})
				if !ok {
					continue
				}
				idxF, ok := tcMap["index"].(float64)
				if !ok {
					continue
				}
				idx := int(idxF)
				existing, hasExisting := accumulatedToolCalls[idx]
				if !hasExisting {
					accumulatedToolCalls[idx] = cloneMap(tcMap)
					continue
				}
				if id, ok := tcMap["id"].(string); ok && id != "" {
					if eid, ok := existing["id"].(string); ok {
						existing["id"] = eid + id
					} else {
						existing["id"] = id
					}
				}
				if typ, ok := tcMap["type"].(string); ok && typ != "" {
					existing["type"] = typ
				}
				if fn, ok := tcMap["function"].(map[string]interface{}); ok {
					ef, ok := existing["function"].(map[string]interface{})
					if !ok {
						ef = make(map[string]interface{})
						existing["function"] = ef
					}
					if name, ok := fn["name"].(string); ok && name != "" {
						if en, ok := ef["name"].(string); ok {
							ef["name"] = en + name
						} else {
							ef["name"] = name
						}
					}
					if args, ok := fn["arguments"].(string); ok && args != "" {
						if ea, ok := ef["arguments"].(string); ok {
							ef["arguments"] = ea + args
						} else {
							ef["arguments"] = args
						}
					}
				}
			}
		}

		content, ok := delta["content"].(string)
		if ok && content != "" {
			if err = sender(&content, nil); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}

	if len(accumulatedToolCalls) > 0 && chatModelConfig != nil {
		indices := make([]int, 0, len(accumulatedToolCalls))
		for idx := range accumulatedToolCalls {
			indices = append(indices, idx)
		}
		sort.Ints(indices)
		toolCalls := make([]map[string]interface{}, 0, len(accumulatedToolCalls))
		for _, idx := range indices {
			toolCalls = append(toolCalls, accumulatedToolCalls[idx])
		}
		chatModelConfig.ToolCallsResult = &toolCalls
	}

	// Send [DONE] marker for OpenAI compatibility
	endOfStream := "[DONE]"
	return sender(&endOfStream, nil)
}

type zhipuEmbeddingResponse struct {
	Data   []zhipuEmbeddingData `json:"data"`
	Model  string               `json:"model"`
	Object string               `json:"object"`
	Usage  zhipuUsage           `json:"usage"`
}

type zhipuEmbeddingData struct {
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
	Object    string    `json:"object"`
}

type zhipuUsage struct {
	CompletionTokens int `json:"completion_tokens"`
	PromptTokens     int `json:"prompt_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Embed embeddings a list of texts into embeddings
func (z *ZhipuAIModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	if err := z.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(texts) == 0 {
		return []EmbeddingData{}, nil
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	baseURL, err := z.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/%s", baseURL, z.baseModel.URLSuffix.Embedding)

	reqBody := map[string]interface{}{}
	reqBody["model"] = modelName
	reqBody["input"] = texts
	if embeddingConfig.Dimension > 0 {
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
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := z.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()

	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var zhipuResp zhipuEmbeddingResponse
	if err = json.Unmarshal(body, &zhipuResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var embeddings []EmbeddingData
	for _, dataElem := range zhipuResp.Data {
		var embeddingData EmbeddingData
		embeddingData.Embedding = dataElem.Embedding
		embeddingData.Index = dataElem.Index
		embeddings = append(embeddings, embeddingData)
	}

	return embeddings, nil
}

func (z *ZhipuAIModel) ListModels(apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := z.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	baseURL, err := z.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/%s", baseURL, z.baseModel.URLSuffix.Models)

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := z.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ZhipuAI models API error: %s, body: %s", resp.Status, string(body))
	}

	// Parse response
	var modelList ModelList
	if err = json.Unmarshal(body, &modelList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return ParseListModel(modelList), nil
}

func (z *ZhipuAIModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", z.Name())
}

func (z *ZhipuAIModel) CheckConnection(apiConfig *APIConfig) error {
	if err := z.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	baseURL, err := z.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/%s", baseURL, z.baseModel.URLSuffix.Files)

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := z.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// zhipuRerankRequest is the request body for the ZhipuAI rerank
// endpoint. The shape matches the standard OpenAI-compatible rerank
// API also used by SiliconFlow.
type zhipuRerankRequest struct {
	Model           string   `json:"model"`
	Query           string   `json:"query"`
	Documents       []string `json:"documents"`
	TopN            int      `json:"top_n"`
	ReturnDocuments bool     `json:"return_documents"`
}

// zhipuRerankResponse is the response shape for the ZhipuAI rerank
// endpoint.
type zhipuRerankResponse struct {
	Created   int64  `json:"created"`
	ID        string `json:"id"`
	RequestID string `json:"request_id"`
	Usage     struct {
		CompletionTokens int `json:"completion_tokens"`
		PromptTokens     int `json:"prompt_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
}

type zhipuOCRResponse struct {
	MarkdownResults *string `json:"md_results"`
}

// Rerank calculates similarity scores between query and documents using
// the ZhipuAI /rerank endpoint (e.g. glm-rerank). The result is one
// score per input text, in the same order the documents were given.
func (z *ZhipuAIModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	if err := z.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(documents) == 0 {
		return &RerankResponse{}, nil
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	baseURL, err := z.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/%s", baseURL, z.baseModel.URLSuffix.Rerank)

	var topN = rerankConfig.TopN
	if rerankConfig.TopN == 0 {
		topN = len(documents)
	}

	reqBody := zhipuRerankRequest{
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

	resp, err := z.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ZhipuAI rerank API error: %s, body: %s", resp.Status, string(body))
	}

	var zhipuRerankResp zhipuRerankResponse
	if err = json.Unmarshal(body, &zhipuRerankResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var rerankResponse RerankResponse
	for _, result := range zhipuRerankResp.Results {
		rerankResult := RerankResult{
			Index:          result.Index,
			RelevanceScore: result.RelevanceScore,
		}
		rerankResponse.Data = append(rerankResponse.Data, rerankResult)
	}

	return &rerankResponse, nil
}

// TranscribeAudio transcribe audio
func (z *ZhipuAIModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	if err := z.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if file == nil || *file == "" {
		return nil, fmt.Errorf("file is required")
	}
	if z.baseModel.URLSuffix.ASR == "" {
		return nil, fmt.Errorf("zhipu-ai: ASR URL suffix is not configured")
	}

	baseURL, err := z.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/%s", baseURL, strings.TrimLeft(z.baseModel.URLSuffix.ASR, "/"))

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err = writer.WriteField("model", *modelName); err != nil {
		return nil, fmt.Errorf("failed to write model field: %w", err)
	}
	if err = writer.WriteField("stream", "false"); err != nil {
		return nil, fmt.Errorf("failed to write stream field: %w", err)
	}
	if err = writeZhipuASRParams(writer, asrConfig); err != nil {
		return nil, err
	}

	// codeql[go/path-injection] False positive: *file is the audio file path the caller passes in to upload. The user (or operator-supplied pipeline) explicitly chose this path, and the OS access check enforces permissions anyway.
	audioFile, err := os.Open(*file)
	if err != nil {
		return nil, fmt.Errorf("failed to open audio file: %w", err)
	}
	defer audioFile.Close()

	part, err := writer.CreateFormFile("file", filepath.Base(*file))
	if err != nil {
		return nil, fmt.Errorf("failed to create multipart file: %w", err)
	}
	if _, err = io.Copy(part, audioFile); err != nil {
		return nil, fmt.Errorf("failed to copy audio data: %w", err)
	}
	if err = writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), longOpCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := z.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ZhipuAI ASR API error: %s, body: %s", resp.Status, string(respBody))
	}

	var result struct {
		Text string `json:"text"`
	}
	if err = json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &ASRResponse{Text: result.Text}, nil
}

func writeZhipuASRParams(writer *multipart.Writer, asrConfig *ASRConfig) error {
	if asrConfig == nil || asrConfig.Params == nil {
		return nil
	}
	for key, value := range asrConfig.Params {
		switch key {
		case "model", "stream", "file", "file_base64":
			continue
		}
		if err := writeZhipuASRField(writer, key, value); err != nil {
			return err
		}
	}
	return nil
}

func writeZhipuASRField(writer *multipart.Writer, key string, value interface{}) error {
	switch v := value.(type) {
	case nil:
		return nil
	case []string:
		for _, item := range v {
			if err := writer.WriteField(key, item); err != nil {
				return fmt.Errorf("failed to write field %s: %w", key, err)
			}
		}
		return nil
	case []interface{}:
		for _, item := range v {
			if err := writer.WriteField(key, fmt.Sprint(item)); err != nil {
				return fmt.Errorf("failed to write field %s: %w", key, err)
			}
		}
		return nil
	default:
		if err := writer.WriteField(key, fmt.Sprint(v)); err != nil {
			return fmt.Errorf("failed to write field %s: %w", key, err)
		}
		return nil
	}
}

func (z *ZhipuAIModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", z.Name())
}

// AudioSpeech convert text to audio
func (z *ZhipuAIModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	if err := z.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	reqBody, url, err := z.buildTTSRequest(modelName, audioContent, apiConfig, ttsConfig, false)
	if err != nil {
		return nil, err
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), longOpCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := z.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ZhipuAI TTS API error: %s, body: %s", resp.Status, string(body))
	}

	return &TTSResponse{Audio: body}, nil
}

func (z *ZhipuAIModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if err := z.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if sender == nil {
		return fmt.Errorf("sender is required")
	}
	reqBody, url, err := z.buildTTSRequest(modelName, audioContent, apiConfig, ttsConfig, true)
	if err != nil {
		return err
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), streamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := z.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ZhipuAI stream TTS API error: %s, body: %s", resp.Status, string(body))
	}

	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])
			if errSend := sender(&chunk, nil); errSend != nil {
				return errSend
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error reading ZhipuAI binary audio stream: %w", err)
		}
	}
	return nil
}

func (z *ZhipuAIModel) buildTTSRequest(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, stream bool) (map[string]interface{}, string, error) {

	if modelName == nil || *modelName == "" {
		return nil, "", fmt.Errorf("model name is required")
	}
	if audioContent == nil || *audioContent == "" {
		return nil, "", fmt.Errorf("audio content is empty")
	}
	if z.baseModel.URLSuffix.TTS == "" {
		return nil, "", fmt.Errorf("zhipu-ai: TTS URL suffix is not configured")
	}

	baseURL, err := z.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, "", err
	}

	reqBody := map[string]interface{}{
		"model":  *modelName,
		"input":  *audioContent,
		"stream": stream,
	}
	if ttsConfig != nil {
		for key, value := range ttsConfig.Params {
			switch key {
			case "model", "input", "stream", "response_format":
				continue
			}
			reqBody[key] = value
		}
		if ttsConfig.Format != "" {
			reqBody["response_format"] = ttsConfig.Format
		}
	}

	url := fmt.Sprintf("%s/%s", baseURL, strings.TrimLeft(z.baseModel.URLSuffix.TTS, "/"))
	return reqBody, url, nil
}

// OCRFile OCR file
func (z *ZhipuAIModel) OCRFile(modelName *string, content []byte, fileURL *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	if err := z.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	if (fileURL == nil || *fileURL == "") && len(content) == 0 {
		return nil, fmt.Errorf("file url or content is required")
	}

	baseURL, err := z.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}

	if z.baseModel.URLSuffix.OCR == "" {
		return nil, fmt.Errorf("zhipu-ai: no OCR URL suffix configured")
	}

	file := ""
	if fileURL != nil && *fileURL != "" {
		file = *fileURL
	} else {
		mimeType := http.DetectContentType(content)
		if len(content) > 4 && string(content[:4]) == "%PDF" {
			mimeType = "application/pdf"
		}
		file = fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(content))
	}

	reqBody := map[string]interface{}{
		"model": *modelName,
		"file":  file,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/%s", baseURL, strings.TrimPrefix(z.baseModel.URLSuffix.OCR, "/"))
	ctx, cancel := context.WithTimeout(context.Background(), longOpCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := z.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ZhipuAI OCR API error: %s, body: %s", resp.Status, string(body))
	}

	var zhipuResp zhipuOCRResponse
	if err = json.Unmarshal(body, &zhipuResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if zhipuResp.MarkdownResults == nil {
		return nil, fmt.Errorf("ZhipuAI OCR API response missing md_results")
	}

	return &OCRFileResponse{Text: zhipuResp.MarkdownResults}, nil
}

// ParseFile parse file
func (z *ZhipuAIModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", z.Name())
}

func (z *ZhipuAIModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", z.Name())
}

func (z *ZhipuAIModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", z.Name())
}
