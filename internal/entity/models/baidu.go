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
	"sort"
	"strings"
)

type BaiduModel struct {
	baseModel BaseModel
}

func (b *BaiduModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewBaiduModel(baseURL, b.baseModel.URLSuffix)
}

func NewBaiduModel(baseURL map[string]string, urlSuffix URLSuffix) *BaiduModel {
	return &BaiduModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(),
		},
	}
}

func (b *BaiduModel) Name() string {
	return "BaiduYiyan"
}

func (b *BaiduModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if err := b.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := b.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, b.baseModel.URLSuffix.Chat)

	// Convert messages to API format
	apiMessages := make([]map[string]any, len(messages))
	for i, msg := range messages {
		apiMessages[i] = map[string]any{
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
			lowerModelName := strings.ToLower(modelName)

			// `enable_think` for qwen and erine
			if strings.HasPrefix(lowerModelName, "qwen") || strings.HasPrefix(lowerModelName, "ernie") {
				reqBody["enable_thinking"] = *chatModelConfig.Thinking
			} else {
				if *chatModelConfig.Thinking {
					thinkingFlag := "enabled"

					if strings.Contains(lowerModelName, "deepseek-v4") {
						effort := "high"
						if chatModelConfig.Effort != nil {
							effort = *chatModelConfig.Effort
						}
						switch effort {
						case "none", "low", "medium":
							thinkingFlag = "disabled"
						case "high", "default":
							thinkingFlag = "enabled"
							reqBody["reasoning_effort"] = "high"
						case "max":
							thinkingFlag = "enabled"
							reqBody["reasoning_effort"] = "max"
						default:
							thinkingFlag = "enabled"
							reqBody["reasoning_effort"] = effort
						}
					}

					reqBody["thinking"] = map[string]interface{}{
						"type": thinkingFlag,
					}
				} else {
					reqBody["thinking"] = map[string]interface{}{
						"type": "disabled",
					}
				}
			}
		}
		if chatModelConfig.Tools != nil {
			reqBody["tools"] = chatModelConfig.Tools
		}
		if chatModelConfig.ToolChoice != nil {
			reqBody["tool_choice"] = *chatModelConfig.ToolChoice
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

	resp, err := b.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
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

	var toolCalls []map[string]any
	if tcs, ok := messageMap["tool_calls"].([]any); ok {
		for _, tc := range tcs {
			if toolCall, ok := tc.(map[string]any); ok {
				toolCalls = append(toolCalls, toolCall)
			}
		}
	}

	var reasonContent string
	if chatModelConfig != nil && chatModelConfig.Thinking != nil && *chatModelConfig.Thinking {
		reasonContent, ok = messageMap["reasoning_content"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid reasoning content format")
		}
		// if first char of reasonContent is \n remove the '\n'
		if reasonContent != "" && reasonContent[0] == '\n' {
			reasonContent = reasonContent[1:]
		}
	}

	chatResponse := &ChatResponse{
		Answer:        &content,
		ReasonContent: &reasonContent,
		ToolCalls:     toolCalls,
	}
	if pt, ct, tt := extractUsageFromMap(result); tt > 0 {
		chatResponse.Usage = &TokenUsage{
			PromptTokens: pt, CompletionTokens: ct, TotalTokens: tt,
		}
	}

	return chatResponse, nil
}

func (b *BaiduModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if err := b.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := b.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(resolvedBaseURL, "/"), b.baseModel.URLSuffix.Chat)

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

	if modelConfig != nil {
		if modelConfig.Stream != nil {
			reqBody["stream"] = *modelConfig.Stream
		}

		if modelConfig.MaxTokens != nil {
			reqBody["max_tokens"] = *modelConfig.MaxTokens
		}

		if modelConfig.Temperature != nil {
			reqBody["temperature"] = *modelConfig.Temperature
		}

		if modelConfig.DoSample != nil {
			reqBody["do_sample"] = *modelConfig.DoSample
		}

		if modelConfig.TopP != nil {
			reqBody["top_p"] = *modelConfig.TopP
		}

		if modelConfig.Stop != nil {
			reqBody["stop"] = *modelConfig.Stop
		}

		if modelConfig.Thinking != nil {
			lowerModelName := strings.ToLower(modelName)

			// `enable_think` for qwen and erine
			if strings.HasPrefix(lowerModelName, "qwen") || strings.HasPrefix(lowerModelName, "ernie") {
				reqBody["enable_thinking"] = *modelConfig.Thinking
			} else {
				if *modelConfig.Thinking {
					thinkingFlag := "enabled"

					if strings.Contains(lowerModelName, "deepseek-v4") {
						effort := "high"
						if modelConfig.Effort != nil {
							effort = *modelConfig.Effort
						}
						switch effort {
						case "none", "low", "medium":
							thinkingFlag = "disabled"
						case "high", "default":
							thinkingFlag = "enabled"
							reqBody["reasoning_effort"] = "high"
						case "max":
							thinkingFlag = "enabled"
							reqBody["reasoning_effort"] = "max"
						default:
							thinkingFlag = "enabled"
							reqBody["reasoning_effort"] = effort
						}
					}

					reqBody["thinking"] = map[string]interface{}{
						"type": thinkingFlag,
					}
				} else {
					reqBody["thinking"] = map[string]interface{}{
						"type": "disabled",
					}
				}
			}
		}

		if modelConfig.Tools != nil {
			reqBody["tools"] = modelConfig.Tools
		}
		if modelConfig.ToolChoice != nil {
			reqBody["tool_choice"] = *modelConfig.ToolChoice
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

	resp, err := b.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// SSE parsing: read line by line
	sawTerminal := false
	accumulatedToolCalls := make(map[int]map[string]interface{})
	done, err := ParseSSEStream[map[string]interface{}](resp.Body, func(event map[string]interface{}) error {
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

		reasoningContent, ok := delta["reasoning_content"].(string)
		if ok && reasoningContent != "" {
			if err = sender(nil, &reasoningContent); err != nil {
				return err
			}
		}

		content, ok := delta["content"].(string)
		if ok && content != "" {
			if err = sender(&content, nil); err != nil {
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
		return fmt.Errorf("baidu: stream ended before [DONE] or finish_reason")
	}

	if len(accumulatedToolCalls) > 0 && modelConfig != nil {
		indices := make([]int, 0, len(accumulatedToolCalls))
		for idx := range accumulatedToolCalls {
			indices = append(indices, idx)
		}
		sort.Ints(indices)
		tcs := make([]map[string]interface{}, 0, len(accumulatedToolCalls))
		for _, idx := range indices {
			tcs = append(tcs, accumulatedToolCalls[idx])
		}
		modelConfig.ToolCallsResult = &tcs
	}

	// Send [DONE] marker for OpenAI compatibility
	endOfStream := "[DONE]"
	if err = sender(&endOfStream, nil); err != nil {
		return err
	}

	return nil
}

type baiduEmbeddingResponse struct {
	ID      string               `json:"id"`
	Object  string               `json:"object"`
	Created int64                `json:"created"`
	Data    []baiduEmbeddingData `json:"data"`
	Model   string               `json:"model"`
	Usage   baiduUsage           `json:"usage"`
}

type baiduEmbeddingData struct {
	Object    string    `json:"object"`
	Embedding []float64 `json:"embedding"`
	Index     *int      `json:"index"`
}

type baiduUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

func (b *BaiduModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	if err := b.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if len(texts) == 0 {
		return []EmbeddingData{}, nil
	}

	resolvedBaseURL, err := b.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, b.baseModel.URLSuffix.Embedding)

	reqBody := map[string]interface{}{
		"model": *modelName,
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
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := b.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Baidu embedding API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var parsed baiduEmbeddingResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(parsed.Data) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(parsed.Data))
	}

	embeddings := make([]EmbeddingData, len(texts))
	seen := make([]bool, len(texts))
	for _, item := range parsed.Data {
		if item.Index == nil {
			return nil, fmt.Errorf("missing index field in embedding item")
		}
		idx := *item.Index
		if idx < 0 || idx >= len(texts) {
			return nil, fmt.Errorf("embedding index %d out of range", idx)
		}
		if seen[idx] {
			return nil, fmt.Errorf("duplicate embedding index %d", idx)
		}
		if len(item.Embedding) == 0 {
			return nil, fmt.Errorf("empty embedding at index %d", idx)
		}
		seen[idx] = true
		embeddings[idx] = EmbeddingData{
			Embedding: item.Embedding,
			Index:     idx,
		}
	}

	for i, ok := range seen {
		if !ok {
			return nil, fmt.Errorf("missing embedding index %d", i)
		}
	}

	return embeddings, nil
}

func (b *BaiduModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	if err := b.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(documents) == 0 {
		return &RerankResponse{}, nil
	}

	resolvedBaseURL, err := b.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(resolvedBaseURL, "/"), b.baseModel.URLSuffix.Rerank)

	var topN = rerankConfig.TopN
	if rerankConfig.TopN == 0 {
		topN = len(documents)
	}

	reqBody := map[string]interface{}{
		"model":     *modelName,
		"query":     query,
		"documents": documents,
		"top_n":     topN,
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

	resp, err := b.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Baidu rerank API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var rerankResp struct {
		Results []struct {
			Index          int     `json:"index"`
			RelevanceScore float64 `json:"relevance_score"`
		} `json:"results"`
	}

	if err = json.Unmarshal(body, &rerankResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var rerankResponse RerankResponse
	for _, result := range rerankResp.Results {
		rerankResult := RerankResult{
			Index:          result.Index,
			RelevanceScore: result.RelevanceScore,
		}
		rerankResponse.Data = append(rerankResponse.Data, rerankResult)
	}

	return &rerankResponse, nil
}

// TranscribeAudio transcribe audio
func (b *BaiduModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", b.Name())
}

func (b *BaiduModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", b.Name())
}

// AudioSpeech convert text to audio
func (b *BaiduModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", b.Name())
}

func (b *BaiduModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", b.Name())
}

// OCRFile OCR file
type qianfanOCRResponse struct {
	Id     string `json:"id"`
	Result struct {
		LayoutParsingResults []struct {
			Markdown struct {
				Text string `json:"text"`
			} `json:"markdown"`
		} `json:"layoutParsingResults"`
	} `json:"result"`
}

func (b *BaiduModel) OCRFile(modelName *string, content []byte, fileURL *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	if err := b.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if (fileURL == nil || *fileURL == "") && (content == nil || len(content) == 0) {
		return nil, fmt.Errorf("image url or content is required")
	}
	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	resolvedBaseURL, err := b.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, b.baseModel.URLSuffix.OCR)

	reqData := map[string]interface{}{
		"model": *modelName,
	}

	if fileURL != nil && *fileURL != "" {
		reqData["file"] = *fileURL
		if strings.HasSuffix(strings.ToLower(*fileURL), ".pdf") {
			reqData["fileType"] = 0 // PDF
		} else {
			reqData["fileType"] = 1 // img
		}
	} else if content != nil && len(content) > 0 {
		reqData["file"] = base64.StdEncoding.EncodeToString(content)

		mimeType := http.DetectContentType(content)
		if strings.Contains(mimeType, "pdf") {
			reqData["fileType"] = 0 // PDF
		} else {
			reqData["fileType"] = 1 // img
		}
	}

	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal json payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), longOpCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := b.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %s, body: %s", resp.Status, string(body))
	}

	var apiResponse qianfanOCRResponse
	if err = json.Unmarshal(body, &apiResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response json: %w", err)
	}

	var extractedText string
	if len(apiResponse.Result.LayoutParsingResults) > 0 {
		extractedText = apiResponse.Result.LayoutParsingResults[0].Markdown.Text
	} else {
		return nil, fmt.Errorf("no parsing results returned from API")
	}

	var ocrResponse = OCRFileResponse{
		Text: &extractedText,
	}

	return &ocrResponse, nil
}

func (b *BaiduModel) ListModels(apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := b.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	resolvedBaseURL, err := b.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, b.baseModel.URLSuffix.Models)

	reqBody := map[string]string{}

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

	resp, err := b.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
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

func (b *BaiduModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", b.Name())
}

func (b *BaiduModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := b.ListModels(apiConfig)
	return err
}

func (b *BaiduModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", b.Name())
}

func (b *BaiduModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", b.Name())
}

func (b *BaiduModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", b.Name())
}
