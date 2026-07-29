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
		filtered = append(filtered, model)
	}

	return filtered, nil
}

// Hint keywords for model type inference, matching Python's
// OpenAIAPICompatible class-level hint constants.
var (
	embeddingHints = []string{"embed", "embedding", "bge"}
	rerankHints    = []string{"rerank", "reranker"}
	asrHints       = []string{"asr", "stt", "transcribe", "transcriber", "whisper"}
	ttsHints       = []string{"tts", "text-to-speech"}
	visionHints    = []string{
		"vl", "vision", "llava", "internvl", "minicpm-v",
		"gpt-4o", "glm-4v", "qvq", "qwen-vl", "pixtral",
	}
	ocrHints = []string{"ocr"}
)

// containsHint checks whether modelName (lowercased) contains any of the
// given hint substrings.
func containsHint(modelName string, hints []string) bool {
	for _, hint := range hints {
		if strings.Contains(modelName, hint) {
			return true
		}
	}
	return false
}

// InferModelTypes derives RAGFlow LLM model types from the model name using
// keyword heuristics, covering all seven supported types: chat, embedding,
// rerank, asr, tts, ocr, and vision (always combined with chat).
func InferModelTypes(modelName string) []string {
	lower := strings.ToLower(modelName)

	if containsHint(lower, rerankHints) {
		return []string{"rerank"}
	}
	if containsHint(lower, embeddingHints) {
		return []string{"embedding"}
	}
	if containsHint(lower, asrHints) {
		return []string{"asr"}
	}
	if containsHint(lower, ttsHints) {
		return []string{"tts"}
	}
	if containsHint(lower, ocrHints) {
		return []string{"ocr"}
	}

	types := []string{"chat"}
	if containsHint(lower, visionHints) {
		types = append(types, "vision")
	}
	return types
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

	resolvedBaseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(resolvedBaseURL, "/"), strings.TrimPrefix(m.baseModel.URLSuffix.TTS, "/"))

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

	resolvedBaseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(resolvedBaseURL, "/"), strings.TrimPrefix(m.baseModel.URLSuffix.ASR, "/"))

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
	if err := json.Unmarshal(respBody, &result); err != nil {
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
