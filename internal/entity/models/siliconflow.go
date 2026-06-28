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

// SiliconflowModel implements ModelDriver for Siliconflow
type SiliconflowModel struct {
	baseModel BaseModel
}

// NewSiliconflowModel creates a new Siliconflow model instance
func NewSiliconflowModel(baseURL map[string]string, urlSuffix URLSuffix) *SiliconflowModel {
	return &SiliconflowModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(),
		},
	}
}

func (s *SiliconflowModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewSiliconflowModel(baseURL, s.baseModel.URLSuffix)
}

func (s *SiliconflowModel) Name() string {
	return "siliconflow"
}

// SiliconflowRerankRequest represents SILICONFLOW rerank request
type SiliconflowRerankRequest struct {
	Model           string   `json:"model"`
	Query           string   `json:"query"`
	Documents       []string `json:"documents"`
	TopN            int      `json:"top_n"`
	ReturnDocuments bool     `json:"return_documents"`
	MaxChunksPerDoc int      `json:"max_chunks_per_doc"`
	OverlapTokens   int      `json:"overlap_tokens"`
}

// ChatWithMessages sends multiple messages with roles and returns response
func (s *SiliconflowModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if err := s.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := s.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, s.baseModel.URLSuffix.Chat)

	// Convert messages to the format expected by API
	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
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
			// SiliconFlow's chat completions API expects a boolean
			// `enable_thinking` field, not a `thinking: {type: ...}` map
			// (the latter is the DeepSeek format and is silently ignored
			// by SiliconFlow, breaking the thinking feature).
			reqBody["enable_thinking"] = *chatModelConfig.Thinking
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

	var reasonContent string
	if chatModelConfig != nil && chatModelConfig.Thinking != nil && *chatModelConfig.Thinking {
		reasonContent, ok = messageMap["reasoning_content"].(string)
		if !ok {
			// If reasoning_content not in response, try parsing from content tags
			reasoning, answer := GetThinkingAndAnswer(chatModelConfig.ModelClass, &content)
			if reasoning != nil {
				reasonContent = *reasoning
				content = *answer
			}
		} else {
			// if first char of reasonContent is \n remove the '\n'
			if reasonContent != "" && reasonContent[0] == '\n' {
				reasonContent = reasonContent[1:]
			}
		}
	}

	chatResponse := &ChatResponse{
		Answer:        &content,
		ReasonContent: &reasonContent,
	}

	return chatResponse, nil
}

// ChatStreamlyWithSender sends messages and streams response via sender function (best performance, no channel)
func (s *SiliconflowModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, sender func(*string, *string) error) error {
	if err := s.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := s.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, s.baseModel.URLSuffix.Chat)

	// Convert messages to API format
	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
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
	}

	// Qwen3 family: disable thinking by default (matches Python's
	// _apply_model_family_policies in rag/llm/chat_model.py:119-121).
	if strings.Contains(strings.ToLower(modelName), "qwen3") && (chatModelConfig == nil || chatModelConfig.Thinking == nil) {
		reqBody["enable_thinking"] = false
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

	resp, err := s.baseModel.httpClient.Do(req)
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

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}
	if !done && !sawTerminal {
		return fmt.Errorf("siliconflow: stream ended before [DONE] or finish_reason")
	}

	// Send [DONE] marker for OpenAI compatibility
	endOfStream := "[DONE]"
	if err = sender(&endOfStream, nil); err != nil {
		return err
	}

	return nil
}

type siliconflowEmbeddingResponse struct {
	Object string                     `json:"object"`
	Model  string                     `json:"model"`
	Data   []siliconflowEmbeddingData `json:"data"`
	Usage  siliconflowUsage           `json:"usage"`
}

type siliconflowEmbeddingData struct {
	Object    string    `json:"object"`
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

type siliconflowUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// siliconflowMaxBatchSize is the per-request input limit documented at
const siliconflowMaxBatchSize = 32

// Embed embeds a list of texts into embeddings
func (s *SiliconflowModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	if err := s.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(texts) == 0 {
		return []EmbeddingData{}, nil
	}
	if len(texts) > siliconflowMaxBatchSize {
		return nil, fmt.Errorf("siliconflow supports a maximum of %d inputs per request", siliconflowMaxBatchSize)
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	resolvedBaseURL, err := s.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(resolvedBaseURL, "/"), s.baseModel.URLSuffix.Embedding)

	apiKey := *apiConfig.ApiKey

	reqBody := map[string]interface{}{
		"model": modelName,
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
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := s.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()

	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SILICONFLOW API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed siliconflowEmbeddingResponse
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

func (s *SiliconflowModel) ListModels(apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := s.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	resolvedBaseURL, err := s.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, s.baseModel.URLSuffix.Models)

	// Build request body
	reqBody := map[string]interface{}{}

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

	return ParseListModel(modelList), nil
}

type siliconflowBalanceResponse struct {
	Code    int    `json:"code"`
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    struct {
		Balance      string `json:"balance"`
		TotalBalance string `json:"totalBalance"`
	} `json:"data"`
}

func (s *SiliconflowModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	if err := s.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	baseURL, err := s.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), s.baseModel.URLSuffix.Balance)

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
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
		return nil, fmt.Errorf("SiliconFlow balance API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed siliconflowBalanceResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !parsed.Status {
		msg := parsed.Message
		if msg == "" {
			msg = "unknown API error"
		}
		return nil, fmt.Errorf("SiliconFlow API error (code %d): %s", parsed.Code, msg)
	}

	raw := parsed.Data.TotalBalance
	if raw == "" {
		raw = parsed.Data.Balance
	}
	if raw == "" {
		return nil, fmt.Errorf("no balance info in response")
	}

	total, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid balance %q: %w", raw, err)
	}

	return map[string]interface{}{
		"balance":  total,
		"currency": "CNY",
	}, nil
}

func (s *SiliconflowModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := s.ListModels(apiConfig)
	if err != nil {
		return err
	}
	return nil
}

// SiliconflowRerankResponse represents SILICONFLOW rerank response
type SiliconflowRerankResponse struct {
	ID      string `json:"id"`
	Results []struct {
		Index    int `json:"index"`
		Document struct {
			Text string `json:"text"`
		} `json:"document"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
	Meta struct {
		Tokens struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			ImageTokens  int `json:"image_tokens"`
		} `json:"tokens"`
		BilledUnits struct {
			InputTokens     int `json:"input_tokens"`
			OutputTokens    int `json:"output_tokens"`
			ImageTokens     int `json:"image_tokens"`
			SearchUnits     int `json:"search_units"`
			Classifications int `json:"classifications"`
		} `json:"billed_units"`
	} `json:"meta"`
}

// Rerank calculates similarity scores between query and documents
func (s *SiliconflowModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	if err := s.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(documents) == 0 {
		return &RerankResponse{}, nil
	}

	apiKey := *apiConfig.ApiKey

	var topN = rerankConfig.TopN
	if rerankConfig.TopN == 0 {
		topN = len(documents)
	}

	reqBody := SiliconflowRerankRequest{
		Model:           *modelName,
		Query:           query,
		Documents:       documents,
		TopN:            topN,
		ReturnDocuments: false,
		MaxChunksPerDoc: 1024,
		OverlapTokens:   80,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resolvedBaseURL, err := s.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(resolvedBaseURL, "/"), s.baseModel.URLSuffix.Rerank)

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := s.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("SiliconFlow Rerank API error: %s, body: %s", resp.Status, string(body))
	}

	body, _ := io.ReadAll(resp.Body)

	var siliconflowRerankResp SiliconflowRerankResponse
	if err = json.Unmarshal(body, &siliconflowRerankResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var rerankResponse RerankResponse
	for _, result := range siliconflowRerankResp.Results {
		rerankResponse.Data = append(rerankResponse.Data, RerankResult{
			Index:          result.Index,
			RelevanceScore: result.RelevanceScore,
		})
	}
	return &rerankResponse, nil
}

// TranscribeAudio transcribe audio
func (s *SiliconflowModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	if err := s.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if file == nil || *file == "" {
		return nil, fmt.Errorf("file is missing")
	}

	resolvedBaseURL, err := s.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, s.baseModel.URLSuffix.ASR)

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
	ctx, cancel := context.WithTimeout(context.Background(), longOpCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, &body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	// send request
	resp, err := s.baseModel.httpClient.Do(req)
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

func (s *SiliconflowModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", s.Name())
}

// AudioSpeech convert text to audio
func (s *SiliconflowModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	if err := s.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if audioContent == nil || *audioContent == "" {
		return nil, fmt.Errorf("audio content is empty")
	}

	resolvedBaseURL, err := s.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, s.baseModel.URLSuffix.TTS)

	reqBody := map[string]interface{}{
		"model":  *modelName,
		"input":  *audioContent,
		"stream": false,
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

	ctx, cancel := context.WithTimeout(context.Background(), longOpCallTimeout)
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
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s - %s", resp.Status, string(body))
	}

	return &TTSResponse{Audio: body}, nil
}

func (s *SiliconflowModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
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
		"model":  *modelName,
		"input":  *audioContent,
		"stream": true,
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

	ctx, cancel := context.WithTimeout(context.Background(), streamCallTimeout)
	defer cancel()

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
		return fmt.Errorf("SiliconFlow stream TTS API error: %d, body: %s", resp.StatusCode, string(body))
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
			return fmt.Errorf("error reading SiliconFlow binary audio stream: %w", err)
		}
	}

	return nil
}

// OCRFile OCR file
func (s *SiliconflowModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", s.Name())
}

// ParseFile parse file
func (s *SiliconflowModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", s.Name())
}

func (s *SiliconflowModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", s.Name())
}

func (s *SiliconflowModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", s.Name())
}
