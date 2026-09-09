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
	"net/url"
	"ragflow/internal/common"
	"strings"
	"time"
	"unicode/utf8"

	"go.uber.org/zap"
)

type JinaModel struct {
	baseModel BaseModel
}

func NewJinaModel(baseURL map[string]string, urlSuffix URLSuffix) *JinaModel {
	// Embed/Rerank/ListModels issue requests without a per-call context
	// deadline, so keep an explicit 90s client-level timeout to bound them.
	// Built on the shared transport via NewDriverHTTPClient.
	client := NewDriverHTTPClient(false)
	client.Timeout = 90 * time.Second
	return &JinaModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: client,
		},
	}
}

func (j *JinaModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewJinaModel(baseURL, j.baseModel.URLSuffix)
}

func (j *JinaModel) Name() string {
	return "jina"
}

// JinaEmbeddingResponse mirrors Jina's embeddings response. Embeddings is
// populated by multivector models such as jina-embeddings-v4.
type JinaEmbeddingResponse struct {
	ID     string `json:"id"`
	Object string `json:"object"`
	Model  string `json:"model"`
	Data   []struct {
		Object     string      `json:"object"`
		Embedding  []float64   `json:"embedding"`
		Embeddings [][]float64 `json:"embeddings"`
		Index      int         `json:"index"`
	} `json:"data"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// JinaRerankResponse mirrors Jina's rerank response.
type JinaRerankResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Results []struct {
		Index    int `json:"index"`
		Document struct {
			Text string `json:"text"`
		} `json:"document"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

func (j *JinaModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if err := j.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	baseURL, err := j.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	messages = prepareJinaMessages(baseURL, messages)
	url := fmt.Sprintf("%s/%s", baseURL, j.baseModel.URLSuffix.Chat)

	reqBody := buildRequestBody(chatModelConfig, modelName, messages, false)

	if chatModelConfig != nil {
		if chatModelConfig.Thinking != nil {
			reqBody["enable_thinking"] = *chatModelConfig.Thinking
		}
	}

	body, err := j.baseModel.doRequest(ctx, url, apiConfig, reqBody, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	return HandleNonStreamingResponse(body, modelUsage, chatModelConfig, OpenAIParserConfig)
}

func (j *JinaModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if sender == nil {
		return fmt.Errorf("sender is required")
	}
	if err := j.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}
	if modelName == "" {
		return fmt.Errorf("model name is required")
	}
	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}
	if err := validateStreamConfig(chatModelConfig); err != nil {
		return err
	}

	baseURL, err := j.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	messages = prepareJinaMessages(baseURL, messages)
	url := fmt.Sprintf("%s/%s", baseURL, j.baseModel.URLSuffix.Chat)

	reqBody := buildRequestBody(chatModelConfig, modelName, messages, true)

	if chatModelConfig != nil {
		chatModelConfig.ToolCallsResult = nil
		chatModelConfig.UsageResult = nil
		if chatModelConfig.Thinking != nil {
			reqBody["enable_thinking"] = *chatModelConfig.Thinking
		}
	}

	err = j.baseModel.doStreamRequest(ctx, url, apiConfig, reqBody, streamCallTimeout, func(body io.ReadCloser) error {
		return handleJinaStreamingResponse(body, modelUsage, chatModelConfig, sender)
	})
	if err != nil {
		upstreamResponse := err.Error()
		errorChunk := "**ERROR**: " + upstreamResponse
		if sendErr := sender(&errorChunk, nil); sendErr != nil {
			return fmt.Errorf("forward Jina stream error: %w", sendErr)
		}
		endOfStream := "[DONE]"
		if sendErr := sender(&endOfStream, nil); sendErr != nil {
			return fmt.Errorf("finish Jina error stream: %w", sendErr)
		}
		return nil
	}
	return nil
}

func prepareJinaMessages(baseURL string, messages []Message) []Message {
	parsedURL, err := url.Parse(baseURL)
	if err != nil || !strings.EqualFold(parsedURL.Hostname(), "api.jina.ai") {
		return messages
	}
	return mergeLeadingJinaSystemMessages(messages)
}

func mergeLeadingJinaSystemMessages(messages []Message) []Message {
	if len(messages) == 0 {
		return nil
	}
	firstNonSystem := 0
	systemParts := make([]string, 0, 1)
	for firstNonSystem < len(messages) && strings.EqualFold(strings.TrimSpace(messages[firstNonSystem].Role), "system") {
		if content, ok := messages[firstNonSystem].Content.(string); ok && strings.TrimSpace(content) != "" {
			systemParts = append(systemParts, content)
		}
		firstNonSystem++
	}
	if firstNonSystem == 0 {
		return messages
	}

	out := append([]Message(nil), messages[firstNonSystem:]...)
	systemText := strings.Join(systemParts, "\n\n")
	if len(out) == 0 || !strings.EqualFold(strings.TrimSpace(out[0].Role), "user") {
		return append([]Message{{Role: "user", Content: systemText}}, out...)
	}
	out[0].Content = prependJinaSystemText(out[0].Content, systemText)
	return out
}

func prependJinaSystemText(content any, systemText string) any {
	if strings.TrimSpace(systemText) == "" {
		return content
	}
	switch value := content.(type) {
	case string:
		if strings.TrimSpace(value) == "" {
			return systemText
		}
		return systemText + "\n\n" + value
	case []interface{}:
		parts := make([]interface{}, 0, len(value)+1)
		parts = append(parts, map[string]interface{}{"type": "text", "text": systemText})
		return append(parts, value...)
	default:
		return systemText
	}
}

// handleJinaStreamingResponse adapts Jina-VLM's thinking stream to the
// internal content/reasoning split. Jina emits both kinds of text in
// delta.content and marks thinking chunks with delta.type="think"; the
// generic OpenAI handler treats all of them as answer content.
func handleJinaStreamingResponse(body io.Reader, modelUsage *common.ModelUsage, chatConfig *ChatConfig, sender func(*string, *string) error) error {
	if sender == nil {
		return fmt.Errorf("sender is required")
	}

	var streamUsage *TokenUsage
	accumulatedToolCalls := make(map[int]map[string]any)
	thinking := false
	thinkingClosed := false
	var pending strings.Builder
	streamModel := ""
	streamResponseID := ""

	emit := func(text string, reasoning bool) error {
		if text == "" {
			return nil
		}
		if reasoning {
			return sender(nil, &text)
		}
		return sender(&text, nil)
	}

	flushThinking := func(final bool) error {
		value := pending.String()
		if !final && len(value) > len("</think>")-1 {
			safeEnd := len(value) - (len("</think>") - 1)
			for safeEnd > 0 && !utf8.RuneStart(value[safeEnd]) {
				safeEnd--
			}
			safe := value[:safeEnd]
			pending.Reset()
			pending.WriteString(value[len(safe):])
			return emit(safe, true)
		}
		pending.Reset()
		return emit(value, true)
	}

	consumeContent := func(content string, typ string) error {
		if typ == "think" && !thinkingClosed {
			thinking = true
		}
		if !thinking {
			return emit(content, false)
		}

		pending.WriteString(content)
		for thinking {
			value := pending.String()
			idx := strings.Index(value, "</think>")
			if idx < 0 {
				return flushThinking(false)
			}
			if err := emit(value[:idx], true); err != nil {
				return err
			}
			pending.Reset()
			pending.WriteString(value[idx+len("</think>"):])
			thinking = false
			thinkingClosed = true
		}
		value := pending.String()
		pending.Reset()
		return emit(value, false)
	}

	_, eventCount, err := parseJinaStream(body, func(event map[string]any) error {
		if id, ok := event["id"].(string); ok && id != "" {
			streamResponseID = id
		}
		if u, ok := OpenAIParserConfig.StreamParser(event); ok {
			streamUsage = u
		}
		if m, ok := event["model"].(string); ok {
			streamModel = m
		}
		if apiErr, ok := event["error"]; ok && apiErr != nil {
			return fmt.Errorf("upstream stream error: %v", apiErr)
		}
		choices, ok := event["choices"].([]any)
		if !ok || len(choices) == 0 {
			return nil
		}
		choice, ok := choices[0].(map[string]any)
		if !ok {
			return nil
		}
		delta, ok := choice["delta"].(map[string]any)
		if !ok {
			return nil
		}
		accumulateToolCallDeltas(delta, accumulatedToolCalls)
		content, _ := delta["content"].(string)
		typ, _ := delta["type"].(string)
		return consumeContent(content, typ)
	})
	if err != nil {
		return fmt.Errorf("failed to scan Jina response body: %w", err)
	}
	if thinking {
		if err := flushThinking(true); err != nil {
			return err
		}
	}
	if eventCount == 0 {
		return fmt.Errorf("Jina stream contained no JSON events")
	}
	if chatConfig != nil {
		setSortedToolCallsResult(chatConfig, accumulatedToolCalls)
	}
	if streamUsage != nil {
		recordResponseUsage(modelUsage, streamResponseID, streamUsage, "chat")
		if chatConfig != nil {
			chatConfig.UsageResult = streamUsage
			common.Info("StreamUsage", zap.String("model", streamModel), zap.Int("prompt", streamUsage.PromptTokens), zap.Int("completion", streamUsage.CompletionTokens), zap.Int("total", streamUsage.TotalTokens))
		}
	}
	endOfStream := "[DONE]"
	return sender(&endOfStream, nil)
}

// parseJinaStream accepts both OpenAI-style SSE frames and Jina-VLM's
// newline-delimited JSON stream. Jina may terminate a successful stream by
// closing the connection instead of emitting a final [DONE] frame.
func parseJinaStream(body io.Reader, onEvent func(map[string]any) error) (done bool, eventCount int, err error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var sseData []string
	flushSSE := func() error {
		if len(sseData) == 0 {
			return nil
		}
		line := strings.Join(sseData, "\n")
		sseData = nil
		if line == "[DONE]" {
			done = true
			return nil
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return fmt.Errorf("invalid Jina stream event: %w", err)
		}
		eventCount++
		return onEvent(event)
	}
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" {
			if err := flushSSE(); err != nil {
				return false, eventCount, err
			}
			if done {
				return true, eventCount, nil
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			sseData = append(sseData, strings.TrimSpace(line[len("data:"):]))
			continue
		} else if strings.Contains(line, ":") && !strings.HasPrefix(line, "{") && !strings.HasPrefix(line, "[") {
			// Ignore non-data SSE fields such as event:, id:, and retry:.
			continue
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "{") || strings.HasPrefix(line, "[") {
			var event map[string]any
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				return false, eventCount, fmt.Errorf("invalid Jina stream event: %w", err)
			}
			eventCount++
			if err := onEvent(event); err != nil {
				return false, eventCount, err
			}
			continue
		}
	}
	if err := scanner.Err(); err != nil {
		return false, eventCount, err
	}
	if err := flushSSE(); err != nil {
		return false, eventCount, err
	}
	if done {
		return true, eventCount, nil
	}
	return false, eventCount, nil
}

func (j *JinaModel) Embed(ctx context.Context, modelName *string, request EmbedRequest, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	if err := j.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(request.Texts) == 0 {
		return []EmbeddingData{}, nil
	}
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}

	resolvedBaseURL, err := j.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, j.baseModel.URLSuffix.Embedding)

	reqBody := map[string]interface{}{
		"model": *modelName,
		"input": request.Texts,
	}

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

	resp, err := j.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Jina embedding API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var parsedResponse JinaEmbeddingResponse

	if err = json.Unmarshal(body, &parsedResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(parsedResponse.Data) == 0 {
		return nil, fmt.Errorf("Jina embedding response contains no data: %s", string(body))
	}
	recordResponseUsage(modelUsage, parsedResponse.ID, &TokenUsage{
		PromptTokens: parsedResponse.Usage.PromptTokens,
		TotalTokens:  parsedResponse.Usage.TotalTokens,
	}, "embedding")

	var embeddings []EmbeddingData
	for _, dataElem := range parsedResponse.Data {
		embedding := dataElem.Embedding
		if len(embedding) == 0 && len(dataElem.Embeddings) > 0 {
			dimensions := len(dataElem.Embeddings[0])
			if dimensions == 0 {
				return nil, fmt.Errorf("Jina embedding response contains an empty multivector at index %d", dataElem.Index)
			}
			embedding = make([]float64, dimensions)
			for _, vector := range dataElem.Embeddings {
				if len(vector) != dimensions {
					return nil, fmt.Errorf("Jina embedding response contains inconsistent multivector dimensions at index %d", dataElem.Index)
				}
				for i, value := range vector {
					embedding[i] += value
				}
			}
			for i := range embedding {
				embedding[i] /= float64(len(dataElem.Embeddings))
			}
		}
		if len(embedding) == 0 {
			return nil, fmt.Errorf("Jina embedding response contains an empty vector at index %d", dataElem.Index)
		}
		embeddings = append(embeddings, EmbeddingData{
			Embedding: embedding,
			Index:     dataElem.Index,
		})
	}

	return embeddings, nil
}

func (j *JinaModel) Rerank(ctx context.Context, modelName *string, request RerankRequest, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	if err := j.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	documents := request.Documents
	query := request.Query
	if len(documents) == 0 {
		return &RerankResponse{}, nil
	}
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}

	resolvedBaseURL, err := j.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, j.baseModel.URLSuffix.Rerank)

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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := j.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Jina Rerank API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var rerankResp JinaRerankResponse

	if err = json.Unmarshal(body, &rerankResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	recordResponseUsage(modelUsage, rerankResp.ID, &TokenUsage{
		PromptTokens: rerankResp.Usage.PromptTokens,
		TotalTokens:  rerankResp.Usage.TotalTokens,
	}, "rerank")

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

func (j *JinaModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {

	resolvedBaseURL, err := j.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, j.baseModel.URLSuffix.Models)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := j.baseModel.httpClient.Do(req)
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
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// convert result["data"] to []map[string]interface{}
	models := make([]ModelListItem, 0, len(result["data"].([]interface{})))
	for _, model := range result["data"].([]interface{}) {
		modelName := model.(map[string]interface{})["id"].(string)
		models = append(models, ModelListItem{
			ID:      modelName,
			OwnedBy: "",
		})
	}
	// Jina list models: `Jina AI: Jina Embeddings v5 Text Nano`
	return ParseListModel(ModelList{Models: models}), nil
}

func (j *JinaModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("no such method")
}

func (j *JinaModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := j.ListModels(ctx, apiConfig)
	return err
}

// TranscribeAudio transcribe audio
func (j *JinaModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", j.Name())
}

func (j *JinaModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", j.Name())
}

// AudioSpeech convert text to audio
func (j *JinaModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", j.Name())
}

func (j *JinaModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", j.Name())
}

// OCRFile OCR file
func (j *JinaModel) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", j.Name())
}

// ParseFile parse file
func (j *JinaModel) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", j.Name())
}

func (j *JinaModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", j.Name())
}

func (j *JinaModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", j.Name())
}
