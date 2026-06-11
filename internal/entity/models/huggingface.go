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
	"ragflow/internal/common"
	"sort"
	"strings"
)

// HuggingFaceModel implements ModelDriver for HuggingFace
type HuggingFaceModel struct {
	baseModel BaseModel
}

// NewHuggingFaceModel creates a new huggingFace model instance
func NewHuggingFaceModel(baseURL map[string]string, urlSuffix URLSuffix) *HuggingFaceModel {
	return &HuggingFaceModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: &http.Client{},
		},
	}
}
func (h *HuggingFaceModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewHuggingFaceModel(baseURL, h.baseModel.URLSuffix)
}

func (h *HuggingFaceModel) Name() string {
	return "huggingface"
}

func (h *HuggingFaceModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if err := h.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := h.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, h.baseModel.URLSuffix.Chat)

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
		"model":       modelName,
		"messages":    apiMessages,
		"stream":      false,
		"temperature": 0.6,
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

	resp, err := h.baseModel.httpClient.Do(req)
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

	content, ok := messageMap["content"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid content format")
	}

	var reasonContent string
	if chatModelConfig != nil && chatModelConfig.Thinking != nil && *chatModelConfig.Thinking {
		reasonContent, ok = messageMap["reasoning_content"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid content format")
		}
		// if first char of reasonContent is \n remove the \n
		if reasonContent != "" && reasonContent[0] == '\n' {
			reasonContent = reasonContent[1:]
		}
	}

	chatResponse := &ChatResponse{
		Answer:        &content,
		ReasonContent: &reasonContent,
	}

	return chatResponse, nil
}

func (h *HuggingFaceModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error {
	if err := h.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := h.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, h.baseModel.URLSuffix.Chat)

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
		if *modelConfig.Thinking {
			reqBody["thinking"] = map[string]interface{}{
				"type": "enabled",
			}
		} else {
			reqBody["thinking"] = map[string]interface{}{
				"type": "disabled",
			}
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

	resp, err := h.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// SSE parsing: read line by line
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		common.Info(line)

		// SSE data line starts with "data:"
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		// Extract JSON after "data:"
		data := strings.TrimSpace(line[5:])

		// [DONE] marks the end of stream
		if data == "[DONE]" {
			break
		}

		// Parse the JSON event
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
			break
		}
	}

	// Send [DONE] marker for OpenAI compatibility
	endOfStream := "[DONE]"
	if err = sender(&endOfStream, nil); err != nil {
		return err
	}

	return scanner.Err()
}

func (h *HuggingFaceModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	if err := h.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(texts) == 0 {
		return []EmbeddingData{}, nil
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	reqBody := map[string]interface{}{
		"inputs": texts,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	resolvedBaseURL, err := h.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s/%s", resolvedBaseURL, h.baseModel.URLSuffix.Embedding, *modelName)

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := h.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HF embeddings API error: %s", string(body))
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

// huggingfaceRerankURL builds the HF Inference Providers per-model
// URL. The shipped chat surface uses the OpenAI-style
// /v1/chat/completions router path, but rerank models on HF live at
// the hf-inference per-model path (the same place sentence-classifier
// pipelines run). The router does not expose a /v1/rerank route —
// POST /v1/rerank returns 404 with a valid token (probe-verified) —
// so the canonical rerank URL on the router is
//
//	https://router.huggingface.co/hf-inference/models/{owner}/{name}
//
// Note this is NOT under /v1/. The driver strips a trailing "/v1"
// from BaseURL so an existing HF provider configured with
// "https://router.huggingface.co/v1" still produces the right URL,
// while a BaseURL already pointing at "https://router.huggingface.co"
// works unchanged.
func huggingfaceRerankURL(baseURL, modelName string) string {
	host := strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/v1")
	return fmt.Sprintf("%s/hf-inference/models/%s", host, modelName)
}

// huggingfaceRerankRequest is the wire shape HF's hf-inference router
// accepts for cross-encoder / sentence-classifier reranker models
// (BAAI/bge-reranker-v2-m3, BAAI/bge-reranker-base, etc.).
//
// The pipeline expects a JSON array of {text, text_pair} pairs;
// sending the more-OpenAI-like [[query, doc], ...] shape gets a
// "send a dictionary {text, text_pair}" 400 error from the
// TextClassificationPipeline. Live-verified against
// BAAI/bge-reranker-v2-m3.
type huggingfaceRerankPair struct {
	Text     string `json:"text"`
	TextPair string `json:"text_pair"`
}

type huggingfaceRerankRequest struct {
	Inputs []huggingfaceRerankPair `json:"inputs"`
}

// huggingfaceRerankPrediction is one cross-encoder classification
// output. HF's TextClassificationPipeline returns score values that
// are NOT normalized to [0, 1] for raw-logit reranker models like
// BAAI/bge-reranker-v2-m3 — they're raw logits where higher means
// more relevant. The label is the model's class id (typically
// "LABEL_0"); the driver does not interpret it.
type huggingfaceRerankPrediction struct {
	Label string  `json:"label"`
	Score float64 `json:"score"`
}

// Rerank scores a query against a list of documents using a HF
// Inference Providers cross-encoder rerank model. The driver targets
// the BAAI bge-reranker family (BAAI/bge-reranker-v2-m3,
// BAAI/bge-reranker-base, …), which is what the hf-inference provider
// supports out of the box for cross-encoder rerank scoring; jina or
// generic cross-encoder models that aren't on the hf-inference
// provider return "Model not supported by provider hf-inference".
func (h *HuggingFaceModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	if len(documents) == 0 {
		return &RerankResponse{}, nil
	}
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}

	region := "default"
	if apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}
	baseURL := h.BaseURL[region]
	if baseURL == "" {
		return nil, fmt.Errorf("huggingface: no base URL configured for region %q", region)
	}
	url := huggingfaceRerankURL(baseURL, *modelName)

	pairs := make([]huggingfaceRerankPair, len(documents))
	for i, doc := range documents {
		pairs[i] = huggingfaceRerankPair{Text: query, TextPair: doc}
	}
	reqBody := huggingfaceRerankRequest{Inputs: pairs}

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

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HF rerank API error: %s, body: %s", resp.Status, string(body))
	}

	// Cross-encoder pipelines wrap their batch output in a single
	// outer array: [[{label,score}, {label,score}, ...]]. Unwrap one
	// level before pairing scores back to documents. Verify count
	// matches input cardinality so a malformed response fails loudly
	// rather than silently truncating the rerank batch.
	var wrapped [][]huggingfaceRerankPrediction
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if len(wrapped) != 1 {
		return nil, fmt.Errorf("huggingface: expected single batch in rerank response, got %d", len(wrapped))
	}
	scores := wrapped[0]
	if len(scores) != len(documents) {
		return nil, fmt.Errorf("huggingface: expected %d rerank scores, got %d", len(documents), len(scores))
	}

	// Build the response in original document order; if the caller
	// asked for top_n, sort by score descending (stable on Index for
	// deterministic tie-breaking) and truncate. Matches the
	// surrounding rerank drivers (Voyage, Replicate) in this package.
	results := make([]RerankResult, len(documents))
	for i, s := range scores {
		results[i] = RerankResult{Index: i, RelevanceScore: s.Score}
	}
	topN := len(documents)
	if rerankConfig != nil && rerankConfig.TopN > 0 && rerankConfig.TopN < topN {
		topN = rerankConfig.TopN
		sort.SliceStable(results, func(a, b int) bool {
			if results[a].RelevanceScore == results[b].RelevanceScore {
				return results[a].Index < results[b].Index
			}
			return results[a].RelevanceScore > results[b].RelevanceScore
		})
		results = results[:topN]
	}
	return &RerankResponse{Data: results}, nil
}

// TranscribeAudio transcribe audio
func (h *HuggingFaceModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", h.Name())
}

func (h *HuggingFaceModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", h.Name())
}

// AudioSpeech convert text to audio
func (h *HuggingFaceModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", h.Name())
}

func (h *HuggingFaceModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", h.Name())
}

// OCRFile OCR file
func (h *HuggingFaceModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", h.Name())
}

// ParseFile parse file
func (h *HuggingFaceModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", h.Name())
}

func (h *HuggingFaceModel) ListModels(apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := h.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	resolvedBaseURL, err := h.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, h.baseModel.URLSuffix.Models)

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

	resp, err := h.baseModel.httpClient.Do(req)
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

func (h *HuggingFaceModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("no such method")
}

func (h *HuggingFaceModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := h.ListModels(apiConfig)
	return err
}

func (h *HuggingFaceModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", h.Name())
}

func (h *HuggingFaceModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", h.Name())
}
