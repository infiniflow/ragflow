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
	"math"
	"net/http"
	"ragflow/internal/common"
	"strings"
)

// ollamaDefaultDecisionSuffix is the default System One endpoint path in Ollama.
const ollamaDefaultDecisionSuffix = "v1/systemone"

// OllamaModel implements ModelDriver for Ollama AI
type OllamaModel struct {
	baseModel BaseModel
}

// NewOllamaModel creates a new Ollama AI model instance
func NewOllamaModel(baseURL map[string]string, urlSuffix URLSuffix) *OllamaModel {
	if urlSuffix.Decision == "" {
		urlSuffix.Decision = ollamaDefaultDecisionSuffix
	}
	return &OllamaModel{
		baseModel: BaseModel{
			BaseURL:          baseURL,
			URLSuffix:        urlSuffix,
			AllowEmptyAPIKey: true,
			httpClient:       common.GetSchemeSafeHTTPClient(),
		},
	}
}

func (o *OllamaModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewOllamaModel(baseURL, o.baseModel.URLSuffix)
}

func (o *OllamaModel) Name() string {
	return "Ollama"
}

func buildOllamaRequestBody(cfg *ChatConfig, modelName string, messages []Message, stream bool) map[string]any {
	reqBody := buildRequestBody(cfg, modelName, messages, stream)
	reqBody["messages"] = buildOllamaMessages(messages)
	if cfg != nil && cfg.Thinking != nil {
		reqBody["think"] = *cfg.Thinking
	}
	return reqBody
}

func buildOllamaMessages(messages []Message) []map[string]any {
	apiMessages := buildChatMessages(messages)
	for i, message := range messages {
		content, images, ok := ollamaMultimodalContent(message.Content)
		if !ok {
			continue
		}
		apiMessages[i]["content"] = content
		if len(images) > 0 {
			apiMessages[i]["images"] = images
		}
	}
	return apiMessages
}

func ollamaMultimodalContent(content interface{}) (string, []string, bool) {
	var parts []interface{}
	switch value := content.(type) {
	case []interface{}:
		parts = value
	case []map[string]interface{}:
		parts = make([]interface{}, len(value))
		for i := range value {
			parts[i] = value[i]
		}
	default:
		return "", nil, false
	}

	var textParts []string
	var images []string
	for _, part := range parts {
		partMap, ok := part.(map[string]interface{})
		if !ok {
			if text, ok := part.(string); ok {
				textParts = append(textParts, text)
			}
			continue
		}

		partType, _ := partMap["type"].(string)
		switch partType {
		case "text", "input_text":
			if text, ok := partMap["text"].(string); ok {
				textParts = append(textParts, text)
			}
		case "image_url":
			if imageURL := ollamaImageURL(partMap["image_url"]); imageURL != "" {
				images = append(images, cleanOllamaImageData(imageURL))
			}
		}
	}

	return strings.Join(textParts, "\n"), images, true
}

func ollamaImageURL(value interface{}) string {
	switch image := value.(type) {
	case string:
		return image
	case map[string]interface{}:
		url, _ := image["url"].(string)
		return url
	case map[string]string:
		return image["url"]
	default:
		return ""
	}
}

func cleanOllamaImageData(image string) string {
	const base64Marker = ";base64,"
	if strings.HasPrefix(image, "data:") {
		if marker := strings.Index(image, base64Marker); marker >= 0 {
			return image[marker+len(base64Marker):]
		}
	}
	return image
}

func (o *OllamaModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("message is nil")
	}

	resolvedBaseURL, err := o.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := joinURL(resolvedBaseURL, o.baseModel.URLSuffix.Chat)

	// For qwen/glm models, use async chat endpoint
	modelType := strings.Split(modelName, "_")[0]
	if modelType == "qwen" || modelType == "glm" {
		url = joinURL(resolvedBaseURL, o.baseModel.URLSuffix.AsyncChat)
	}

	// Build request body
	reqBody := buildOllamaRequestBody(chatModelConfig, modelName, messages, false)

	if chatModelConfig != nil {
		if chatModelConfig.Effort != nil && *chatModelConfig.Effort != "" {
			if strings.HasPrefix(strings.ToLower(modelName), "gpt-oss") {
				reqBody["think"] = *chatModelConfig.Effort
			}
		}
	}

	body, err := o.baseModel.doRequest(ctx, url, apiConfig, reqBody, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	return handleOllamaNonStreamingResponse(body, modelUsage, chatModelConfig)
}

func (o *OllamaModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := o.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	url := joinURL(resolvedBaseURL, o.baseModel.URLSuffix.Chat)
	modelType := strings.Split(modelName, "-")[0]
	if modelType == "qwen" || modelType == "glm" {
		url = joinURL(resolvedBaseURL, o.baseModel.URLSuffix.AsyncChat)
	}

	// Build request body with streaming enabled
	reqBody := buildOllamaRequestBody(modelConfig, modelName, messages, true)

	if modelConfig != nil {
		if modelConfig.Effort != nil && *modelConfig.Effort != "" {
			if strings.HasPrefix(strings.ToLower(modelName), "gpt-oss") {
				reqBody["think"] = *modelConfig.Effort
			}
		}
	}

	reqBody["stream_options"] = map[string]interface{}{"include_usage": true}

	return o.baseModel.doStreamRequest(ctx, url, apiConfig, reqBody, streamCallTimeout, func(body io.ReadCloser) error {
		return handleOllamaStreamingResponse(body, modelUsage, modelConfig, sender)
	})
}

type ollamaChatMessage struct {
	Content  string `json:"content"`
	Thinking string `json:"thinking"`
}

type ollamaChatResponse struct {
	Model           string            `json:"model"`
	Message         ollamaChatMessage `json:"message"`
	Done            bool              `json:"done"`
	PromptEvalCount int               `json:"prompt_eval_count"`
	EvalCount       int               `json:"eval_count"`
	TotalDuration   int64             `json:"total_duration"`
	Error           any               `json:"error"`
}

func handleOllamaNonStreamingResponse(body []byte, modelUsage *common.ModelUsage, chatConfig *ChatConfig) (*ChatResponse, error) {
	var result ollamaChatResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("upstream error: %v", result.Error)
	}
	content := result.Message.Content
	reasonContent := result.Message.Thinking
	usage := ollamaTokenUsage(result)
	if usage != nil {
		recordResponseUsage(modelUsage, "", usage, "chat")
		if chatConfig != nil {
			chatConfig.UsageResult = usage
		}
	}
	if content == "" && reasonContent == "" {
		return nil, fmt.Errorf("no message in response")
	}
	return &ChatResponse{
		Answer:        &content,
		ReasonContent: &reasonContent,
		Usage:         usage,
	}, nil
}

func handleOllamaStreamingResponse(body io.Reader, modelUsage *common.ModelUsage, chatConfig *ChatConfig, sender func(*string, *string) error) error {
	if sender == nil {
		return fmt.Errorf("sender is required")
	}
	var usage *TokenUsage
	sawDone := false
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event ollamaChatResponse
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return fmt.Errorf("invalid Ollama stream event: %w", err)
		}
		if event.Error != nil {
			return fmt.Errorf("upstream stream error: %v", event.Error)
		}
		if event.Message.Thinking != "" {
			if err := sender(nil, &event.Message.Thinking); err != nil {
				return err
			}
		}
		if event.Message.Content != "" {
			if err := sender(&event.Message.Content, nil); err != nil {
				return err
			}
		}
		if event.Done {
			sawDone = true
			usage = ollamaTokenUsage(event)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}
	if !sawDone {
		return fmt.Errorf("stream ended before done")
	}
	if usage != nil {
		recordResponseUsage(modelUsage, "", usage, "chat")
		if chatConfig != nil {
			chatConfig.UsageResult = usage
		}
	}
	endOfStream := "[DONE]"
	return sender(&endOfStream, nil)
}

func ollamaTokenUsage(result ollamaChatResponse) *TokenUsage {
	if result.PromptEvalCount == 0 && result.EvalCount == 0 {
		return nil
	}
	return &TokenUsage{
		PromptTokens:     result.PromptEvalCount,
		CompletionTokens: result.EvalCount,
		TotalTokens:      result.PromptEvalCount + result.EvalCount,
	}
}

func (o *OllamaModel) Embed(ctx context.Context, modelName *string, request EmbedRequest, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	if err := o.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(request.Texts) == 0 {
		return []EmbeddingData{}, nil
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	resolvedBaseURL, err := o.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL := resolvedBaseURL
	if baseURL == "" {
		baseURL = resolvedBaseURL
	}
	if baseURL == "" {
		return nil, fmt.Errorf("missing base URL: please configure the local access address for Ollama (e.g., http://127.0.0.1:11434/v1)")
	}

	url := joinURL(baseURL, o.baseModel.URLSuffix.Embedding)

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
	if auth := BearerAuth(apiConfig); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := o.baseModel.httpClient.Do(req)
	if err != nil {
		common.Error(fmt.Sprintf("failed to send request, response: %v", resp), err)
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama embeddings API error: %s, body: %s", resp.Status, string(body))
	}

	var embedResp struct {
		Model      string      `json:"model"`
		Embeddings [][]float64 `json:"embeddings"`
	}

	if err = json.Unmarshal(body, &embedResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(embedResp.Embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}

	embeddings := make([]EmbeddingData, 0, len(embedResp.Embeddings))

	for i, emb := range embedResp.Embeddings {
		if len(emb) == 0 {
			return nil, fmt.Errorf("empty embedding at index %d", i)
		}

		embeddings = append(embeddings, EmbeddingData{
			Embedding: emb,
			Index:     i,
		})
	}

	return embeddings, nil
}

func (o *OllamaModel) Rerank(ctx context.Context, modelName *string, request RerankRequest, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	return nil, fmt.Errorf("no such method")
}

// TranscribeAudio transcribe audio
func (o *OllamaModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", o.Name())
}

func (o *OllamaModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", o.Name())
}

// AudioSpeech convert text to audio
func (o *OllamaModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", o.Name())
}

func (o *OllamaModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", o.Name())
}

// OCRFile OCR file
func (o *OllamaModel) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", o.Name())
}

// ParseFile parse file
func (o *OllamaModel) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", o.Name())
}

func (o *OllamaModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {

	baseURL, err := o.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	if baseURL == "" {
		return nil, fmt.Errorf("missing base URL: please configure the local access address for Ollama (e.g., http://127.0.0.1:11434)")
	}

	url := joinURL(baseURL, o.baseModel.URLSuffix.Models)

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

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

	// Ollama's /api/tags returns {"models":[{"name":...,"model":...}]}, a shape
	// that differs from the OpenAI list. Decode it into a local struct, map the
	// names into ModelList, then enrich through the shared ParseListModel helper
	// (issue #15853). Using a typed struct also avoids the previous unchecked
	// type assertions, which panicked when "models" was absent or malformed.
	var result struct {
		Models []struct {
			Name  string `json:"name"`
			Model string `json:"model"`
		} `json:"models"`
	}
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	modelList := ModelList{Object: "list"}
	for _, m := range result.Models {
		name := strings.TrimSpace(m.Name)
		if name == "" {
			name = strings.TrimSpace(m.Model)
		}
		if name == "" {
			continue
		}
		modelList.Models = append(modelList.Models, ModelListItem{ID: name})
	}

	return ParseListModel(modelList), nil
}

func (o *OllamaModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("no such method")
}

// CheckConnection verifies that the configured Ollama base URL is reachable
func (o *OllamaModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := o.ListModels(ctx, apiConfig)
	return err
}

func (o *OllamaModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", o.Name())
}

func (o *OllamaModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", o.Name())
}

const maxOllamaDecisionQuestions = 64

type ollamaDecisionAnswer struct {
	Type          string             `json:"type"`
	Noul          *float64           `json:"noul,omitempty"`
	Choice        *string            `json:"choice,omitempty"`
	Score         *float64           `json:"score,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
	Legend        map[string]string  `json:"legend,omitempty"`
}

type ollamaDecisionResponse struct {
	ID      string                          `json:"id"`
	Model   string                          `json:"model"`
	Answers map[string]ollamaDecisionAnswer `json:"answers"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func serializeOllamaQuestions(questions map[string]Question) (map[string]any, error) {
	out := make(map[string]any, len(questions))
	for id, q := range questions {
		if strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("ollama: question id cannot be empty")
		}
		if q == nil {
			return nil, fmt.Errorf("ollama: question %q cannot be nil", id)
		}
		switch v := q.(type) {
		case Noul:
			out[id] = v
		case *Noul:
			if v == nil {
				return nil, fmt.Errorf("ollama: question %q cannot be nil", id)
			}
			out[id] = *v
		case Choice:
			out[id] = map[string]any{
				"type":         "choice",
				"instructions": v.Instructions,
				"criteria":     v.Criteria,
			}
		case *Choice:
			if v == nil {
				return nil, fmt.Errorf("ollama: question %q cannot be nil", id)
			}
			out[id] = map[string]any{
				"type":         "choice",
				"instructions": v.Instructions,
				"criteria":     v.Criteria,
			}
		case Score:
			out[id] = map[string]any{
				"type":         "score",
				"instructions": v.Instructions,
				"criteria":     v.Criteria,
			}
		case *Score:
			if v == nil {
				return nil, fmt.Errorf("ollama: question %q cannot be nil", id)
			}
			out[id] = map[string]any{
				"type":         "score",
				"instructions": v.Instructions,
				"criteria":     v.Criteria,
			}
		default:
			return nil, fmt.Errorf("ollama: unsupported question type %T for question %q", q, id)
		}
	}
	return out, nil
}

// Decide asks probability, choice, or scoring questions about one state in a single
// request using Ollama's System One decision endpoint (/v1/systemone).
func (o *OllamaModel) Decide(ctx context.Context, modelName *string, request *DecideRequest, apiConfig *APIConfig, modelUsage *common.ModelUsage) (*DecideResponse, error) {
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("ollama: model name is required")
	}
	if request == nil || len(request.Questions) == 0 || len(request.Questions) > maxOllamaDecisionQuestions {
		return nil, fmt.Errorf("ollama: between 1 and %d questions are required", maxOllamaDecisionQuestions)
	}

	serializedQuestions, err := serializeOllamaQuestions(request.Questions)
	if err != nil {
		return nil, err
	}

	baseURL, err := o.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("missing base URL: please configure the access address for Ollama (e.g., http://127.0.0.1:11434)")
	}

	suffix := strings.Trim(o.baseModel.URLSuffix.Decision, "/")
	if suffix == "" {
		suffix = ollamaDefaultDecisionSuffix
	} else if suffix == "systemone" && !strings.HasSuffix(strings.ToLower(strings.TrimRight(baseURL, "/")), "/v1") {
		suffix = ollamaDefaultDecisionSuffix
	}

	url := strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(strings.ToLower(url), "/"+strings.ToLower(suffix)) &&
		!strings.HasSuffix(strings.ToLower(url), "/systemone") {
		url = joinURL(url, suffix)
	}

	state := request.State
	var images []string
	if stateMap, ok := request.State.(map[string]any); ok {
		if rawImages, exists := stateMap["images"]; exists {
			switch imgs := rawImages.(type) {
			case []string:
				images = imgs
			case []any:
				for _, img := range imgs {
					if str, ok := img.(string); ok {
						images = append(images, str)
					}
				}
			}
		}
		if rawState, exists := stateMap["state"]; exists {
			state = rawState
		}
	}
	if state == nil {
		state = ""
	}

	reqBody := map[string]any{
		"model":     strings.TrimSpace(*modelName),
		"state":     state,
		"questions": serializedQuestions,
	}
	if len(images) > 0 {
		reqBody["images"] = images
	}

	body, err := o.baseModel.doRequest(ctx, url, apiConfig, reqBody, nonStreamCallTimeout)
	if err != nil {
		return nil, fmt.Errorf("ollama: %w", err)
	}

	var errResp struct {
		Error any `json:"error"`
	}
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != nil && errResp.Error != "" {
		return nil, fmt.Errorf("ollama: %v", errResp.Error)
	}

	var rawResp ollamaDecisionResponse
	if err := json.Unmarshal(body, &rawResp); err != nil {
		return nil, fmt.Errorf("ollama: decode response: %w", err)
	}
	if len(rawResp.Answers) == 0 {
		return nil, fmt.Errorf("ollama: no answers in response")
	}

	response := DecideResponse{
		ID:      rawResp.ID,
		Answers: make(map[string]DecisionAnswer, len(rawResp.Answers)),
		Usage: struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		}{
			InputTokens:  rawResp.Usage.InputTokens,
			OutputTokens: rawResp.Usage.OutputTokens,
		},
	}

	for id, answer := range rawResp.Answers {
		if answer.Noul != nil && (math.IsNaN(*answer.Noul) || math.IsInf(*answer.Noul, 0) || *answer.Noul < 0 || *answer.Noul > 1) {
			return nil, fmt.Errorf("ollama: invalid probability for question %q", id)
		}
		if answer.Confidence != nil && (math.IsNaN(*answer.Confidence) || math.IsInf(*answer.Confidence, 0) || *answer.Confidence < 0 || *answer.Confidence > 1) {
			return nil, fmt.Errorf("ollama: invalid confidence for question %q", id)
		}
		if answer.Score != nil && (math.IsNaN(*answer.Score) || math.IsInf(*answer.Score, 0) || *answer.Score < 0) {
			return nil, fmt.Errorf("ollama: invalid score for question %q", id)
		}
		for opt, prob := range answer.Probabilities {
			if math.IsNaN(prob) || math.IsInf(prob, 0) || prob < 0 || prob > 1 {
				return nil, fmt.Errorf("ollama: invalid probability for option %q in question %q", opt, id)
			}
		}

		response.Answers[id] = DecisionAnswer{
			Type: answer.Type,
			Noul: answer.Noul,
		}
	}

	recordResponseUsage(modelUsage, response.ID, &TokenUsage{
		PromptTokens:     response.Usage.InputTokens,
		CompletionTokens: response.Usage.OutputTokens,
		TotalTokens:      response.Usage.InputTokens + response.Usage.OutputTokens,
	}, "decision")

	return &response, nil
}
