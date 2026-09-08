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

// NovitaModel implements ModelDriver for Novita.ai
type NovitaModel struct {
	baseModel BaseModel
}

// NewNovitaModel creates a new Novita model instance.
func NewNovitaModel(baseURL map[string]string, urlSuffix URLSuffix) *NovitaModel {
	return &NovitaModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(false),
		},
	}
}

func (n *NovitaModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewNovitaModel(baseURL, n.baseModel.URLSuffix)
}

func (n *NovitaModel) Name() string {
	return "NovitaAI"
}

// ChatWithMessages sends multiple messages with roles and returns the response.
func (n *NovitaModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if err := n.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	baseURL, err := n.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/%s", baseURL, n.baseModel.URLSuffix.Chat)
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, false)

	if chatModelConfig != nil {
		if chatModelConfig.Thinking != nil {
			reqBody["enable_thinking"] = *chatModelConfig.Thinking
		}
	}

	body, err := n.baseModel.doRequest(ctx, url, apiConfig, reqBody, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	return HandleNonStreamingResponse(body, modelUsage, chatModelConfig, OpenAIParserConfig)
}

// ChatStreamlyWithSender sends messages and streams the response via
// the sender. Handles both reasoning shapes Novita can emit:
//   - delta.reasoning_content (deepseek-v3.1 / glm-4.5 / any model
//     with separate reasoning): forwarded as-is to the second arg.
//   - delta.content (qwen3-* and other inline-style models): forwarded
//     to the first arg.
func (n *NovitaModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if err := n.baseModel.APIConfigCheck(apiConfig); err != nil {
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

	baseURL, err := n.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/%s", baseURL, n.baseModel.URLSuffix.Chat)
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, true)

	if chatModelConfig != nil {
		if chatModelConfig.Thinking != nil {
			reqBody["enable_thinking"] = *chatModelConfig.Thinking
		}
	}

	reqBody["stream_options"] = map[string]interface{}{"include_usage": true}

	return n.baseModel.doStreamRequest(ctx, url, apiConfig, reqBody, streamCallTimeout, func(body io.ReadCloser) error {
		// Novita qwen3 embeds <think>...</think> inline in
		// delta.content (tags can span multiple SSE deltas). Split
		// those blocks so reasoning routes to the sender's second arg.
		return novitaHandleStream(body, modelUsage, chatModelConfig, sender)
	})
}

// novitaThinkSegment is one routing decision: emit `content` via the
// sender's first arg, or emit `reasoning` via the second. Exactly one of
// the two fields is non-empty.
type novitaThinkSegment struct {
	content   string
	reasoning string
}

// novitaThinkSplitter holds state across streaming content chunks so a
// <think>...</think> block that spans multiple SSE deltas is still split
// correctly. Trailing bytes that could be the start of a tag are held
// back until the next chunk.
type novitaThinkSplitter struct {
	buf    strings.Builder
	inside bool
}

const (
	novitaThinkOpen  = "<think>"
	novitaThinkClose = "</think>"
)

func (s *novitaThinkSplitter) feed(chunk string) []novitaThinkSegment {
	s.buf.WriteString(chunk)
	str := s.buf.String()
	var out []novitaThinkSegment
	for {
		var marker string
		if s.inside {
			marker = novitaThinkClose
		} else {
			marker = novitaThinkOpen
		}
		idx := strings.Index(str, marker)
		if idx < 0 {
			// No marker yet. Emit everything except a possible
			// partial-tag suffix at the very end.
			reserve := max(len(novitaThinkOpen)-1, len(novitaThinkClose)-1)
			safe := max(len(str)-reserve, 0)
			if safe < len(str) && !strings.Contains(str[safe:], "<") {
				safe = len(str)
			}
			if safe > 0 {
				if s.inside {
					out = append(out, novitaThinkSegment{reasoning: str[:safe]})
				} else {
					out = append(out, novitaThinkSegment{content: str[:safe]})
				}
				str = str[safe:]
			}
			s.buf.Reset()
			s.buf.WriteString(str)
			return out
		}
		if s.inside {
			out = append(out, novitaThinkSegment{reasoning: str[:idx]})
		} else {
			out = append(out, novitaThinkSegment{content: str[:idx]})
		}
		str = str[idx+len(marker):]
		s.inside = !s.inside
	}
}

func (s *novitaThinkSplitter) flush() *novitaThinkSegment {
	if s.buf.Len() == 0 {
		return nil
	}
	remaining := s.buf.String()
	s.buf.Reset()
	if s.inside {
		return &novitaThinkSegment{reasoning: remaining}
	}
	return &novitaThinkSegment{content: remaining}
}

// novitaHandleStream processes a Novita streaming chat response, splitting
// inline <think>...</think> blocks in delta.content across SSE deltas.
func novitaHandleStream(
	body io.Reader,
	modelUsage *common.ModelUsage,
	chatConfig *ChatConfig,
	sender func(*string, *string) error,
) error {
	if sender == nil {
		return fmt.Errorf("sender is required")
	}

	var sawTerminal bool
	thinkSplitter := &novitaThinkSplitter{}

	done, err := ParseSSEStream[map[string]any](body, func(event map[string]any) error {
		tokenUsage, found := extractOpenAIStreamUsage(event)
		if found {
			applyStreamUsage(chatConfig, modelUsage, tokenUsage)
		}

		if apiErr, ok := event["error"]; ok && apiErr != nil {
			return fmt.Errorf("upstream stream error: %v", apiErr)
		}

		choices, ok := event["choices"].([]any)
		if !ok || len(choices) == 0 {
			return nil
		}

		firstChoice, ok := choices[0].(map[string]any)
		if !ok {
			return nil
		}

		delta, ok := firstChoice["delta"].(map[string]any)
		if !ok {
			return nil
		}

		if reasoningContent, ok := delta["reasoning_content"].(string); ok && reasoningContent != "" {
			if err := sender(nil, &reasoningContent); err != nil {
				return err
			}
		}

		if content, ok := delta["content"].(string); ok && content != "" {
			for _, seg := range thinkSplitter.feed(content) {
				if seg.content != "" {
					c := seg.content
					if err := sender(&c, nil); err != nil {
						return err
					}
				}
				if seg.reasoning != "" {
					r := seg.reasoning
					if err := sender(nil, &r); err != nil {
						return err
					}
				}
			}
		}

		if finishReason, ok := firstChoice["finish_reason"].(string); ok && finishReason != "" {
			sawTerminal = true
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}

	if !done && !sawTerminal {
		return fmt.Errorf("stream ended before [DONE] or finish_reason")
	}

	if seg := thinkSplitter.flush(); seg != nil {
		if seg.content != "" {
			c := seg.content
			if err := sender(&c, nil); err != nil {
				return err
			}
		}
		if seg.reasoning != "" {
			r := seg.reasoning
			if err := sender(nil, &r); err != nil {
				return err
			}
		}
	}

	endOfStream := "[DONE]"
	return sender(&endOfStream, nil)
}

// ListModels returns the list of model ids visible to the API key.
func (n *NovitaModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := n.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	baseURL, err := n.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/%s", baseURL, n.baseModel.URLSuffix.Models)

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := n.baseModel.httpClient.Do(req)
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

// CheckConnection runs a lightweight ListModels call to verify the API key.
func (n *NovitaModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := n.ListModels(ctx, apiConfig)
	return err
}

type novitaEmbeddingData struct {
	Embedding []float64 `json:"embedding"`
	Object    string    `json:"object"`
	Index     int       `json:"index"`
}

type novitaEmbeddingResponse struct {
	ID     string                `json:"id"`
	Data   []novitaEmbeddingData `json:"data"`
	Model  string                `json:"model"`
	Object string                `json:"object"`
	Usage  struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// Embed turns a list of texts into embedding vectors using the Novita
// /v3/embeddings endpoint. The output has one vector per input, in the
// same order the inputs were given.
func (n *NovitaModel) Embed(ctx context.Context, modelName *string, request EmbedRequest, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	if err := n.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(request.Texts) == 0 {
		return []EmbeddingData{}, nil
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	baseURL, err := n.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/%s", baseURL, n.baseModel.URLSuffix.Embedding)

	reqBody := map[string]interface{}{
		"model": *modelName,
		"input": request.Texts,
	}
	if embeddingConfig != nil && embeddingConfig.Dimension > 0 {
		reqBody["dimensions"] = embeddingConfig.Dimension
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

	resp, err := n.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Novita embeddings API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed novitaEmbeddingResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	embeddings := make([]EmbeddingData, len(request.Texts))
	filled := make([]bool, len(request.Texts))
	for _, item := range parsed.Data {
		if item.Index < 0 || item.Index >= len(request.Texts) {
			return nil, fmt.Errorf("novita: response index %d out of range for %d inputs", item.Index, len(request.Texts))
		}
		if filled[item.Index] {
			return nil, fmt.Errorf("novita: duplicate embedding index %d in response", item.Index)
		}
		embeddings[item.Index] = EmbeddingData{
			Embedding: item.Embedding,
			Index:     item.Index,
		}
		filled[item.Index] = true
	}
	for i, ok := range filled {
		if !ok {
			return nil, fmt.Errorf("novita: missing embedding for input index %d", i)
		}
	}
	recordResponseUsage(modelUsage, parsed.ID, &TokenUsage{
		PromptTokens: parsed.Usage.PromptTokens,
		TotalTokens:  parsed.Usage.TotalTokens,
	}, "embedding")

	return embeddings, nil
}

type novitaRerankResult struct {
	Document struct {
		Text string `json:"text"`
	} `json:"document"`
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

type novitaRerankResponse struct {
	ID      string               `json:"id"`
	Results []novitaRerankResult `json:"results"`
	Usage   struct {
		CompletionTokens int `json:"completion_tokens"`
		PromptTokens     int `json:"prompt_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// Rerank scores documents against the query using the Novita
// /openai/v1/rerank endpoint and returns one RerankResult per scored
// document in the API's ranking order. Caller may sort by Index to
// recover original input order.
func (n *NovitaModel) Rerank(ctx context.Context, modelName *string, request RerankRequest, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	if err := n.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	documents := request.Documents
	query := request.Query
	if len(documents) == 0 {
		return &RerankResponse{}, nil
	}
	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	baseURL, err := n.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	if n.baseModel.URLSuffix.Rerank == "" {
		return nil, fmt.Errorf("novita: no rerank URL suffix configured")
	}
	url := fmt.Sprintf("%s/%s", baseURL, n.baseModel.URLSuffix.Rerank)

	topN := len(documents)
	if rerankConfig != nil && rerankConfig.TopN > 0 && rerankConfig.TopN < topN {
		topN = rerankConfig.TopN
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

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := n.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Novita rerank API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed novitaRerankResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	rerankResponse := RerankResponse{Data: make([]RerankResult, 0, len(parsed.Results))}
	seen := make([]bool, len(documents))
	for _, item := range parsed.Results {
		if item.Index < 0 || item.Index >= len(documents) {
			return nil, fmt.Errorf("novita: rerank index %d out of range for %d inputs", item.Index, len(documents))
		}
		if seen[item.Index] {
			return nil, fmt.Errorf("novita: duplicate rerank index %d in response", item.Index)
		}
		rerankResponse.Data = append(rerankResponse.Data, RerankResult{
			Index:          item.Index,
			RelevanceScore: item.RelevanceScore,
		})
		seen[item.Index] = true
	}
	recordResponseUsage(modelUsage, parsed.ID, &TokenUsage{
		PromptTokens:     parsed.Usage.PromptTokens,
		CompletionTokens: parsed.Usage.CompletionTokens,
		TotalTokens:      parsed.Usage.TotalTokens,
	}, "rerank")

	return &rerankResponse, nil
}

// Balance Get remaining credit
func (n *NovitaModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	if err := n.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	baseURL, err := n.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", baseURL, n.baseModel.URLSuffix.Balance)

	// Build request body
	reqBody := map[string]interface{}{}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("GET", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := n.baseModel.httpClient.Do(req)
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

	balanceInterface, exists := result["availableBalance"]
	if !exists || balanceInterface == nil {
		return nil, fmt.Errorf("missing 'availableBalance' in response. Raw body: %s", string(body))
	}

	balanceStr, ok := balanceInterface.(string)
	if !ok {
		return nil, fmt.Errorf("'availableBalance' is not a string. Raw body: %s", string(body))
	}
	balance, err := strconv.ParseFloat(balanceStr, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse 'availableBalance' as float: %w. Raw body: %s", err, string(body))
	}

	var response = map[string]interface{}{
		"balance":  balance,
		"currency": "USD",
	}

	return response, nil
}

func (n *NovitaModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", n.Name())
}

func (n *NovitaModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", n.Name())
}

func (n *NovitaModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", n.Name())
}

func (n *NovitaModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", n.Name())
}

// OCRFile OCR file
func (n *NovitaModel) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", n.Name())
}

// ParseFile parse file
func (n *NovitaModel) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", n.Name())
}

func (n *NovitaModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", n.Name())
}

func (n *NovitaModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", n.Name())
}
