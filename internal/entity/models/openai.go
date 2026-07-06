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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// OpenAIModel implements ModelDriver for OpenAI (GPT models).
type OpenAIModel struct {
	baseModel BaseModel
}

// NewOpenAIModel creates a new OpenAI model instance.
func NewOpenAIModel(baseURL map[string]string, urlSuffix URLSuffix) *OpenAIModel {
	return &OpenAIModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(),
		},
	}
}

func (o *OpenAIModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewOpenAIModel(baseURL, o.baseModel.URLSuffix)
}

func (o *OpenAIModel) Name() string {
	return "openai"
}

// ChatWithMessages sends multiple messages with roles and returns the response
func (o *OpenAIModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if err := o.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	baseURL, err := o.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/%s", baseURL, o.baseModel.URLSuffix.Chat)

	// Convert messages to API format (supports multimodal content)
	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMsg := map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
		if msg.ToolCallID != "" {
			apiMsg["tool_call_id"] = msg.ToolCallID
		}
		if len(msg.ToolCalls) > 0 {
			apiMsg["tool_calls"] = msg.ToolCalls
		}
		apiMessages[i] = apiMsg
	}

	// Build request body
	reqBody := map[string]interface{}{
		"model":    modelName,
		"messages": apiMessages,
		"stream":   false,
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

		if chatModelConfig.Tools != nil {
			reqBody["tools"] = chatModelConfig.Tools
			tc := "auto"
			if chatModelConfig.ToolChoice != nil {
				tc = *chatModelConfig.ToolChoice
			}
			reqBody["tool_choice"] = tc
		}
	}

	// Qwen3 family: disable thinking by default (matches Python's
	// _apply_model_family_policies in rag/llm/chat_model.py:119-121).
	if strings.Contains(strings.ToLower(modelName), "qwen3") && (chatModelConfig == nil || chatModelConfig.Thinking == nil) {
		reqBody["enable_thinking"] = false
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

	resp, err := o.baseModel.httpClient.Do(req)
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

	var content string
	if c, ok := messageMap["content"].(string); ok {
		content = c
	}

	// OpenAI reasoning models (o-series and similar) return reasoning text in
	// the reasoning_content field. Pass it through when present.
	var reasonContent string
	if rc, ok := messageMap["reasoning_content"].(string); ok {
		reasonContent = rc
		if reasonContent != "" && reasonContent[0] == '\n' {
			reasonContent = reasonContent[1:]
		}
	}

	var toolCalls []map[string]interface{}
	if tcs, ok := messageMap["tool_calls"].([]interface{}); ok {
		for _, tc := range tcs {
			if tcMap, ok := tc.(map[string]interface{}); ok {
				toolCalls = append(toolCalls, tcMap)
			}
		}
	}

	chatResponse := &ChatResponse{
		Answer:        &content,
		ReasonContent: &reasonContent,
		ToolCalls:     toolCalls,
	}

	// Extract usage split (prompt/completion/total) from the raw API
	// response for accurate per-call token accounting. Non-OpenAI
	// providers that implement the OpenAI-compat API surface (DeepSeek,
	// Moonshot, etc.) also return a "usage" key with the same shape.
	if pt, ct, tt := extractUsageFromMap(result); tt > 0 {
		chatResponse.Usage = &ChatUsage{
			PromptTokens: pt, CompletionTokens: ct, TotalTokens: tt,
		}
	}

	return chatResponse, nil
}

// ChatStreamlyWithSender sends messages and streams the response
func (o *OpenAIModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, sender func(*string, *string) error) error {
	if err := o.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	baseURL, err := o.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/%s", baseURL, o.baseModel.URLSuffix.Chat)

	// Convert messages to API format (supports multimodal content and tool messages)
	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMsg := map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
		if msg.ToolCallID != "" {
			apiMsg["tool_call_id"] = msg.ToolCallID
		}
		if len(msg.ToolCalls) > 0 {
			apiMsg["tool_calls"] = msg.ToolCalls
		}
		apiMessages[i] = apiMsg
	}

	// Build request body with streaming on by default.
	// stream_options.include_usage asks the provider to attach a
	// usage block to the final streaming chunk (mirrors Python's
	// chat_model.py _stream_options / stream_options.include_usage).
	reqBody := map[string]interface{}{
		"model":    modelName,
		"messages": apiMessages,
		"stream":   true,
		"stream_options": map[string]interface{}{
			"include_usage": true,
		},
	}

	if chatModelConfig != nil {
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

		if chatModelConfig.Tools != nil {
			reqBody["tools"] = chatModelConfig.Tools
			tc := "auto"
			if chatModelConfig.ToolChoice != nil {
				tc = *chatModelConfig.ToolChoice
			}
			reqBody["tool_choice"] = tc
		}
	}

	// Qwen3 family: disable thinking by default.
	if strings.Contains(strings.ToLower(modelName), "qwen3") && (chatModelConfig == nil || chatModelConfig.Thinking == nil) {
		reqBody["enable_thinking"] = false
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := o.baseModel.httpClient.Do(req)
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
	// Capture the authoritative usage block from the final streaming
	// chunk (when provider honours stream_options.include_usage=true).
	// The last chunk in the stream carries the "usage" key alongside
	// empty choices; we overwrite on every chunk so the final frame
	// wins, matching Python's chat_model.py usage_from_response loop.
	var streamUsage *ChatUsage
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// SSE data line starts with "data:"
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		// Extract JSON after "data:"
		data := strings.TrimSpace(line[5:])

		// [DONE] marks the end of the stream
		if data == "[DONE]" {
			sawTerminal = true
			break
		}

		// Parse the JSON event
		var event map[string]interface{}
		if err = json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		// Extract usage from this chunk. When stream_options.include_usage
		// is true, the final chunk carries the full usage breakdown at the
		// top level of the event alongside (possibly empty) choices.
		if pt, ct, tt := extractUsageFromMap(event); tt > 0 {
			streamUsage = &ChatUsage{PromptTokens: pt, CompletionTokens: ct, TotalTokens: tt}
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

		// Accumulate streaming tool_call deltas (mirrors Python's
		// async_chat_streamly_with_tools in rag/llm/chat_model.py:500-509).
		if tcs, ok := delta["tool_calls"].([]interface{}); ok {
			for _, tc := range tcs {
				if tcMap, ok := tc.(map[string]interface{}); ok {
					idxF, ok := tcMap["index"].(float64)
					if !ok {
						continue
					}
					idx := int(idxF)
					existing, hasExisting := accumulatedToolCalls[idx]
					if hasExisting {
						if fn, ok := tcMap["function"].(map[string]interface{}); ok {
							if args, ok := fn["arguments"].(string); ok {
								if ef, ok := existing["function"].(map[string]interface{}); ok {
									if ea, ok := ef["arguments"].(string); ok {
										ef["arguments"] = ea + args
									} else {
										ef["arguments"] = args
									}
								}
							}
						}
					} else {
						accumulatedToolCalls[idx] = cloneMap(tcMap)
					}
				}
			}
			continue // tool_call deltas don't carry content
		}

		reasoningContent, ok := delta["reasoning_content"].(string)
		if ok && reasoningContent != "" {
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
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}
	if !sawTerminal {
		return fmt.Errorf("openai: stream ended before [DONE] or finish_reason")
	}

	// Populate ToolCallsResult with accumulated streaming tool_calls.
	if len(accumulatedToolCalls) > 0 && chatModelConfig != nil {
		tcs := make([]map[string]interface{}, 0, len(accumulatedToolCalls))
		for _, tc := range accumulatedToolCalls {
			tcs = append(tcs, tc)
		}
		chatModelConfig.ToolCallsResult = &tcs
	}

	// Populate UsageResult with the authoritative usage from the stream.
	if streamUsage != nil && chatModelConfig != nil {
		chatModelConfig.UsageResult = streamUsage
	}

	// Send the [DONE] marker for OpenAI compatibility
	endOfStream := "[DONE]"
	if err := sender(&endOfStream, nil); err != nil {
		return err
	}

	return nil
}

type openaiEmbeddingResponse struct {
	Data   []openrouterEmbeddingData `json:"data"`
	Model  string                    `json:"model"`
	Object string                    `json:"object"`
	Usage  openrouterUsage           `json:"usage"`
}

type openaiEmbeddingData struct {
	Embedding []float64 `json:"embedding"`
	Object    string    `json:"object"`
	Index     int       `json:"index"`
}

type openaiUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

func (o *OpenAIModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	if err := o.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(texts) == 0 {
		return []EmbeddingData{}, nil
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	baseURL, err := o.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/%s", baseURL, o.baseModel.URLSuffix.Embedding)

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

	resp, err := o.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI embeddings API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed openaiEmbeddingResponse
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

// ListModels returns the list of model ids visible to the API key.
func (o *OpenAIModel) ListModels(apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := o.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	baseURL, err := o.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/%s", baseURL, o.baseModel.URLSuffix.Models)

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// GET has no body, so Content-Type is not needed.
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := o.baseModel.httpClient.Do(req)
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

// Balance is not exposed by the OpenAI API, so this returns "no such method".
func (o *OpenAIModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("no such method")
}

// CheckConnection runs a lightweight ListModels call to verify the API key.
func (o *OpenAIModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := o.ListModels(apiConfig)
	if err != nil {
		return err
	}
	return nil
}

// Rerank calculates similarity scores between query and documents. OpenAI does
// not expose a rerank API, so this is left unimplemented.
func (o *OpenAIModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, Rerank not implemented", o.Name())
}

// TranscribeAudio transcribe audio
func (o *OpenAIModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, responseFormat, err := o.newOpenAIASRRequest(ctx, modelName, file, apiConfig, asrConfig, false)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")

	resp, err := o.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI ASR API error: %s, body: %s", resp.Status, string(respBody))
	}

	return decodeOpenAIASRResponse(respBody, responseFormat)
}

func (o *OpenAIModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	if sender == nil {
		return fmt.Errorf("sender is required")
	}

	req, responseFormat, err := o.newOpenAIASRRequest(context.Background(), modelName, file, apiConfig, asrConfig, true)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := o.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OpenAI ASR stream API error: %s, body: %s", resp.Status, string(respBody))
	}

	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}
		response, err := decodeOpenAIASRResponse(respBody, responseFormat)
		if err != nil {
			return err
		}
		if response != nil && response.Text != "" {
			if err = sender(&response.Text, nil); err != nil {
				return err
			}
		}
		done := "[DONE]"
		return sender(&done, nil)
	}

	sentDelta := false
	if _, err = ParseSSEStream[struct {
		Type  string `json:"type"`
		Delta string `json:"delta"`
		Text  string `json:"text"`
	}](resp.Body, func(event struct {
		Type  string `json:"type"`
		Delta string `json:"delta"`
		Text  string `json:"text"`
	}) error {
		switch {
		case event.Delta != "":
			if err = sender(&event.Delta, nil); err != nil {
				return err
			}
			sentDelta = true
		case event.Type == "transcript.text.segment" && event.Text != "":
			if err = sender(&event.Text, nil); err != nil {
				return err
			}
			sentDelta = true
		case event.Type == "transcript.text.done" && !sentDelta && event.Text != "":
			if err = sender(&event.Text, nil); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("error reading OpenAI ASR stream: %w", err)
	}

	done := "[DONE]"
	return sender(&done, nil)
}

func decodeOpenAIASRResponse(respBody []byte, responseFormat string) (*ASRResponse, error) {
	switch responseFormat {
	case "text", "srt", "vtt":
		return &ASRResponse{Text: string(respBody)}, nil
	}

	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w, body=%s", err, string(respBody))
	}

	return &ASRResponse{Text: result.Text}, nil
}

// AudioSpeech convert text to audio
func (o *OpenAIModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, _, err := o.newOpenAITTSRequest(ctx, modelName, audioContent, apiConfig, ttsConfig, false)
	if err != nil {
		return nil, err
	}

	resp, err := o.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI TTS API error: %s, body: %s", resp.Status, string(body))
	}

	return &TTSResponse{Audio: body}, nil
}

func (o *OpenAIModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	if sender == nil {
		return fmt.Errorf("sender is required")
	}

	req, streamFormat, err := o.newOpenAITTSRequest(context.Background(), modelName, audioContent, apiConfig, ttsConfig, true)
	if err != nil {
		return err
	}
	if streamFormat == "sse" {
		req.Header.Set("Accept", "text/event-stream")
	}

	resp, err := o.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OpenAI TTS stream API error: %s, body: %s", resp.Status, string(body))
	}

	if streamFormat == "sse" || strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		return readOpenAITTSSSEStream(resp.Body, sender)
	}
	return readOpenAITTSRawStream(resp.Body, sender)
}

func (o *OpenAIModel) newOpenAIASRRequest(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, stream bool) (*http.Request, string, error) {
	if err := o.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, "", err
	}
	if modelName == nil || *modelName == "" {
		return nil, "", fmt.Errorf("model name is required")
	}
	if file == nil || *file == "" {
		return nil, "", fmt.Errorf("file is missing")
	}
	if strings.TrimSpace(o.baseModel.URLSuffix.ASR) == "" {
		return nil, "", fmt.Errorf("openai ASR URL suffix is required")
	}

	baseURL, err := o.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, "", err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), strings.TrimPrefix(o.baseModel.URLSuffix.ASR, "/"))

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// codeql[go/path-injection] False positive: *file is the audio file path the caller passes in to upload. The user (or operator-supplied pipeline) explicitly chose this path, and the OS access check enforces permissions anyway.
	audioFile, err := os.Open(*file)
	if err != nil {
		return nil, "", fmt.Errorf("failed to open audio file: %w", err)
	}
	defer audioFile.Close()

	part, err := writer.CreateFormFile("file", filepath.Base(*file))
	if err != nil {
		return nil, "", fmt.Errorf("failed to create multipart file: %w", err)
	}
	if _, err = io.Copy(part, audioFile); err != nil {
		return nil, "", fmt.Errorf("failed to copy audio data: %w", err)
	}
	if err = writer.WriteField("model", *modelName); err != nil {
		return nil, "", fmt.Errorf("failed to write model field: %w", err)
	}

	responseFormat := ""
	if asrConfig != nil && asrConfig.Params != nil {
		if value, ok := asrConfig.Params["response_format"].(string); ok {
			responseFormat = value
		}
		for key, value := range asrConfig.Params {
			if stream && key == "stream" {
				continue
			}
			if err = writeOpenAIMultipartField(writer, key, value); err != nil {
				return nil, "", err
			}
		}
	}
	if stream {
		if err = writer.WriteField("stream", "true"); err != nil {
			return nil, "", fmt.Errorf("failed to write stream field: %w", err)
		}
	}

	if err = writer.Close(); err != nil {
		return nil, "", fmt.Errorf("failed to close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, &body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req, responseFormat, nil
}

func (o *OpenAIModel) newOpenAITTSRequest(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, stream bool) (*http.Request, string, error) {
	if err := o.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, "", err
	}
	if modelName == nil || *modelName == "" {
		return nil, "", fmt.Errorf("model name is required")
	}
	if audioContent == nil || *audioContent == "" {
		return nil, "", fmt.Errorf("audio content is empty")
	}
	if strings.TrimSpace(o.baseModel.URLSuffix.TTS) == "" {
		return nil, "", fmt.Errorf("openai TTS URL suffix is required")
	}

	baseURL, err := o.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, "", err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), strings.TrimPrefix(o.baseModel.URLSuffix.TTS, "/"))

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
	if stream {
		if _, ok := reqBody["stream_format"]; !ok {
			reqBody["stream_format"] = "audio"
		}
	}

	voice, ok := reqBody["voice"]
	if !ok || voice == nil {
		return nil, "", fmt.Errorf("voice is required")
	}
	voiceString, ok := voice.(string)
	if !ok || strings.TrimSpace(voiceString) == "" {
		return nil, "", fmt.Errorf("voice is required")
	}

	streamFormat := ""
	if value, ok := reqBody["stream_format"].(string); ok {
		streamFormat = value
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))
	return req, streamFormat, nil
}

func readOpenAITTSSSEStream(body io.Reader, sender func(*string, *string) error) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		dataStr := strings.TrimSpace(line[6:])
		if dataStr == "" || dataStr == "[DONE]" {
			continue
		}

		var event struct {
			Type  string `json:"type"`
			Audio string `json:"audio"`
		}
		if err := json.Unmarshal([]byte(dataStr), &event); err != nil {
			continue
		}

		if event.Type == "speech.audio.delta" && event.Audio != "" {
			audioBytes, err := base64.StdEncoding.DecodeString(event.Audio)
			if err == nil && len(audioBytes) > 0 {
				chunk := string(audioBytes)
				if errSend := sender(&chunk, nil); errSend != nil {
					return errSend
				}
			}
		}
		if event.Type == "speech.audio.done" {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading OpenAI TTS stream: %w", err)
	}
	return nil
}

func readOpenAITTSRawStream(body io.Reader, sender func(*string, *string) error) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := body.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])
			if errSend := sender(&chunk, nil); errSend != nil {
				return errSend
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("error reading OpenAI TTS stream: %w", err)
		}
	}
}

func writeOpenAIMultipartField(writer *multipart.Writer, key string, value interface{}) error {
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

	if err := writer.WriteField(key, val); err != nil {
		return fmt.Errorf("failed to write field %s: %w", key, err)
	}
	return nil
}

// OCRFile OCR file
func (o *OpenAIModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", o.Name())
}

// ParseFile parse file
func (o *OpenAIModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", o.Name())
}

func (o *OpenAIModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", o.Name())
}

func (o *OpenAIModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", o.Name())
}

// extractUsageFromMap reads the "usage" key from an OpenAI-style API
// response and returns (prompt_tokens, completion_tokens, total_tokens).
// All return values are zero when the response carries no usage block.
func extractUsageFromMap(raw map[string]interface{}) (int, int, int) {
	if raw == nil {
		return 0, 0, 0
	}
	ru, ok := raw["usage"]
	if !ok {
		return 0, 0, 0
	}
	usage, ok := ru.(map[string]interface{})
	if !ok {
		return 0, 0, 0
	}
	get := func(keys ...string) int {
		for _, k := range keys {
			v, ok := usage[k]
			if !ok {
				continue
			}
			switch val := v.(type) {
			case float64:
				return int(val)
			case int:
				return val
			case json.Number:
				n, err := val.Int64()
				if err == nil {
					return int(n)
				}
			}
		}
		return 0
	}
	pt := get("prompt_tokens", "input_tokens")
	ct := get("completion_tokens", "output_tokens")
	tt := get("total_tokens")
	if tt == 0 {
		tt = pt + ct
	}
	return pt, ct, tt
}

func cloneMap(m map[string]interface{}) map[string]interface{} {
	cp := make(map[string]interface{}, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}
