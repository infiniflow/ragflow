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
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"ragflow/internal/common"
)

// OpenAIAPICompatibleModel implements ModelDriver for any OpenAI-API-compatible
// provider. It reuses VllmModel's implementation via struct embedding, matching
// the Python backend where OpenAI-API-Compatible and VLLM share the same
// OpenAIAPIChat class.
type OpenAIAPICompatibleModel struct {
	*VllmModel
}

// NewOpenAIAPICompatibleModel creates a new OpenAI-API-Compatible model instance
func NewOpenAIAPICompatibleModel(baseURL map[string]string, urlSuffix URLSuffix) *OpenAIAPICompatibleModel {
	return &OpenAIAPICompatibleModel{
		VllmModel: NewVllmModel(baseURL, urlSuffix),
	}
}

// Name returns the provider identifier
func (m *OpenAIAPICompatibleModel) Name() string {
	return "OpenAI-API-Compatible"
}

// NewInstance creates a new instance with the given baseURL, returning the
// same OpenAIAPICompatibleModel type.
func (m *OpenAIAPICompatibleModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewOpenAIAPICompatibleModel(baseURL, m.baseModel.URLSuffix)
}

// ListModels overrides VllmModel.ListModels to apply hint-based model type
// inference and filter out models whose types cannot be mapped to any known
// RAGFlow LLM type (e.g. image-generation-only models).
func (m *OpenAIAPICompatibleModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	models, err := m.VllmModel.ListModels(ctx, apiConfig)
	if err != nil {
		return nil, err
	}

	filtered := make([]ListModelResponse, 0, len(models))
	for _, model := range models {
		inferred := InferModelTypes(model.Name)
		if len(inferred) == 0 {
			continue
		}
		model.ModelTypes = inferred
		model.ModelTypesInferred = true
		filtered = append(filtered, model)
	}

	return filtered, nil
}

// ChatWithMessages sends multiple messages with roles and returns response
func (m *OpenAIAPICompatibleModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	url, err := m.baseModel.EndpointURL(apiConfig, m.baseModel.URLSuffix.Chat)
	if err != nil {
		return nil, err
	}

	// Build request body
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, false)
	applyVllmCompatibleThinking(reqBody, modelName, chatModelConfig)

	body, err := m.baseModel.doRequest(ctx, url, apiConfig, reqBody, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	return HandleNonStreamingResponse(ctx, body, modelUsage, chatModelConfig, OpenAIParserConfig)
}

// ChatStreamlyWithSender sends messages and streams response via sender function
func (m *OpenAIAPICompatibleModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}
	if sender == nil {
		return fmt.Errorf("sender is required")
	}

	url, err := m.baseModel.EndpointURL(apiConfig, m.baseModel.URLSuffix.Chat)
	if err != nil {
		return err
	}

	// Build request body with streaming enabled
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, true)
	reqBody["stream_options"] = map[string]interface{}{
		"include_usage": true,
	}

	applyVllmCompatibleThinking(reqBody, modelName, chatModelConfig)

	return m.baseModel.doStreamRequest(ctx, url, apiConfig, reqBody, streamCallTimeout, func(body io.ReadCloser) error {
		return HandleStreamingResponse(body, modelUsage, chatModelConfig, OpenAIParserConfig, sender)
	})
}

// ttsVoiceForModel maps model names to appropriate TTS voices for
// OpenAI-compatible providers (e.g. SiliconFlow). Returns "alloy" as
// the generic fallback.
func ttsVoiceForModel(modelName string) string {
	lower := strings.ToLower(modelName)
	switch {
	case strings.Contains(lower, "cosyvoice"):
		return modelName + ":anna"
	case strings.Contains(lower, "fishaudio") || strings.Contains(lower, "fish-speech"):
		return "alex"
	case strings.Contains(lower, "chattts"):
		return "alex"
	case strings.Contains(lower, "gpt-sovits"):
		return "alex"
	case strings.Contains(lower, "bert-vits2"):
		return "alex"
	default:
		return "alloy"
	}
}

// AudioSpeech converts text to speech via the OpenAI-compatible
// POST /v1/audio/speech endpoint. It does not require a voice parameter,
// matching the behaviour of many OpenAI-compatible gateway providers (e.g.
// SiliconFlow) where voice is optional or provider-specific.
func (m *OpenAIAPICompatibleModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if audioContent == nil || *audioContent == "" {
		return nil, fmt.Errorf("audio content is empty")
	}
	if strings.TrimSpace(m.baseModel.URLSuffix.TTS) == "" {
		return nil, fmt.Errorf("%s TTS URL suffix is not configured", m.Name())
	}

	reqCtx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	url, err := m.baseModel.EndpointURL(apiConfig, m.baseModel.URLSuffix.TTS)
	if err != nil {
		return nil, err
	}

	reqBody := map[string]interface{}{
		"model": *modelName,
		"input": *audioContent,
		"voice": ttsVoiceForModel(*modelName),
	}

	if ttsConfig != nil {
		if ttsConfig.Format != "" {
			reqBody["response_format"] = ttsConfig.Format
		}
		if ttsConfig.Params != nil {
			for key, value := range ttsConfig.Params {
				reqBody[key] = value
			}
		}
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(reqCtx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if auth := BearerAuth(apiConfig); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := m.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s TTS API error: %s, body: %s", m.Name(), resp.Status, string(body))
	}

	return &TTSResponse{Audio: body}, nil
}

// AudioSpeechWithSender streams text-to-speech audio chunks.  This stub is
// intentionally not implemented; the non-streaming AudioSpeech suffices for
// connection verification and the streaming path is not currently required
// for OpenAI-API-Compatible providers.
func (m *OpenAIAPICompatibleModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s audio speech streaming not implemented", m.Name())
}

// TranscribeAudio sends an audio file for speech-to-text transcription via
// the OpenAI-compatible POST /v1/audio/transcriptions endpoint (multipart).
func (m *OpenAIAPICompatibleModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if file == nil || *file == "" {
		return nil, fmt.Errorf("file is missing")
	}
	if strings.TrimSpace(m.baseModel.URLSuffix.ASR) == "" {
		return nil, fmt.Errorf("%s ASR URL suffix is not configured", m.Name())
	}

	reqCtx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	url, err := m.baseModel.EndpointURL(apiConfig, m.baseModel.URLSuffix.ASR)
	if err != nil {
		return nil, err
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

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
	if err = writer.WriteField("model", *modelName); err != nil {
		return nil, fmt.Errorf("failed to write model field: %w", err)
	}

	if asrConfig != nil && asrConfig.Params != nil {
		for key, value := range asrConfig.Params {
			strVal := fmt.Sprintf("%v", value)
			if err = writer.WriteField(key, strVal); err != nil {
				return nil, fmt.Errorf("failed to write field %s: %w", key, err)
			}
		}
	}

	if err = writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(reqCtx, "POST", url, &body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	if auth := BearerAuth(apiConfig); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := m.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s ASR API error: %s, body: %s", m.Name(), resp.Status, string(respBody))
	}

	var result struct {
		Text string `json:"text"`
	}
	if err = json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w, body=%s", err, string(respBody))
	}

	return &ASRResponse{Text: result.Text}, nil
}

// TranscribeAudioWithSender streams ASR transcription. This stub is
// intentionally not implemented; the non-streaming TranscribeAudio suffices
// for connection verification.
func (m *OpenAIAPICompatibleModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s ASR streaming not implemented", m.Name())
}

func serializeOpenAiQuestions(questions map[string]Question) (map[string]any, error) {
	out := make(map[string]any, len(questions))
	for id, q := range questions {
		if strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("openai: question id cannot be empty")
		}
		if q == nil {
			return nil, fmt.Errorf("openai: question %q cannot be nil", id)
		}
		switch v := q.(type) {
		case Noul:
			out[id] = v
		case *Noul:
			if v == nil {
				return nil, fmt.Errorf("openai: question %q cannot be nil", id)
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
				return nil, fmt.Errorf("openai: question %q cannot be nil", id)
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
				return nil, fmt.Errorf("openai: question %q cannot be nil", id)
			}
			out[id] = map[string]any{
				"type":         "score",
				"instructions": v.Instructions,
				"criteria":     v.Criteria,
			}
		default:
			return nil, fmt.Errorf("openai: unsupported question type %T for question %q", q, id)
		}
	}
	return out, nil
}

type openaiDecisionAnswer struct {
	Type          string             `json:"type"`
	Noul          *float64           `json:"noul,omitempty"`
	Choice        *string            `json:"choice,omitempty"`
	Score         *float64           `json:"score,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
	Legend        map[string]string  `json:"legend,omitempty"`
}

type openaiDecisionResponse struct {
	ID      string                          `json:"id"`
	Model   string                          `json:"model"`
	Answers map[string]openaiDecisionAnswer `json:"answers"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (m *OpenAIAPICompatibleModel) Decide(ctx context.Context, modelName *string, request *DecideRequest, apiConfig *APIConfig, modelUsage *common.ModelUsage) (*DecideResponse, error) {
	const maxOpenAiDecisionQuestions = 64

	const openAIDefaultDecisionSuffix = "systemone"
	const openAIFullDecisionSuffix = "v1/systemone"

	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("OpenAi: model name is required")
	}
	if request == nil || len(request.Questions) == 0 || len(request.Questions) > maxOpenAiDecisionQuestions {
		return nil, fmt.Errorf("OpenAi: between 1 and %d questions are required", maxOpenAiDecisionQuestions)
	}

	serializedQuestions, err := serializeOpenAiQuestions(request.Questions)
	if err != nil {
		return nil, err
	}

	baseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("missing base URL: please configure the access address for Ollama (e.g., http://127.0.0.1:11434)")
	}

	suffix := strings.Trim(m.baseModel.URLSuffix.Decision, "/")
	if suffix == "" || suffix == "systemone" {
		if strings.HasSuffix(strings.ToLower(strings.TrimRight(baseURL, "/")), "/v1") {
			suffix = "systemone"
		} else {
			suffix = "v1/systemone"
		}
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

	body, err := m.baseModel.doRequest(ctx, url, apiConfig, reqBody, nonStreamCallTimeout)
	if err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}

	var errResp struct {
		Error any `json:"error"`
	}
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != nil && errResp.Error != "" {
		return nil, fmt.Errorf("openai: %v", errResp.Error)
	}

	var rawResp openaiDecisionResponse
	if err := json.Unmarshal(body, &rawResp); err != nil {
		return nil, fmt.Errorf("openai: decode response: %w", err)
	}
	if len(rawResp.Answers) == 0 {
		return nil, fmt.Errorf("openai: no answers in response")
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
			return nil, fmt.Errorf("openai: invalid probability for question %q", id)
		}
		if answer.Confidence != nil && (math.IsNaN(*answer.Confidence) || math.IsInf(*answer.Confidence, 0) || *answer.Confidence < 0 || *answer.Confidence > 1) {
			return nil, fmt.Errorf("openai: invalid confidence for question %q", id)
		}
		if answer.Score != nil && (math.IsNaN(*answer.Score) || math.IsInf(*answer.Score, 0) || *answer.Score < 0) {
			return nil, fmt.Errorf("openai: invalid score for question %q", id)
		}
		for opt, prob := range answer.Probabilities {
			if math.IsNaN(prob) || math.IsInf(prob, 0) || prob < 0 || prob > 1 {
				return nil, fmt.Errorf("openai: invalid probability for option %q in question %q", opt, id)
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
