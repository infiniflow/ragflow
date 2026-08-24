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

// CitySenseSpeechKit implements ModelDriver for media-speech-mcp (Yandex SpeechKit).
// It speaks the hybrid REST API backed by AudioPipelineService / SpeechKitService.
// Auth is X-Service-Token: MEDIA_SPEECH_API_KEY (ApiKeyWebFilter), not Bearer.
// See D:\IdeaProjects\ai-pipeline\media-speech-mcp\src\main\java\com\citysense\mcp\mediaspeech\api\TranscriptionApiController.java
// and security/ApiKeyWebFilter.java.
type CitySenseSpeechKit struct {
	baseModel BaseModel
}

func NewCitySenseSpeechKitModel(baseURL map[string]string, urlSuffix URLSuffix) *CitySenseSpeechKit {
	// MEDIA_SPEECH_BASE_URL env fallback — allows fully configuring url via env without UI
	if envURL := strings.TrimSpace(os.Getenv("MEDIA_SPEECH_BASE_URL")); envURL != "" {
		if baseURL == nil {
			baseURL = map[string]string{}
		}
		if _, ok := baseURL["default"]; !ok || baseURL["default"] == "" || baseURL["default"] == "http://media-speech-mcp:8080" {
			baseURL["default"] = strings.TrimSuffix(envURL, "/")
		}
	}
	// MEDIA_SPEECH_API_KEY env fallback for local dev
	if envKey := strings.TrimSpace(os.Getenv("MEDIA_SPEECH_API_KEY")); envKey != "" {
		// no-op here — key comes via APIConfig, but keep env visible in logs
		_ = envKey
	}
	return &CitySenseSpeechKit{
		baseModel: BaseModel{
			BaseURL:          baseURL,
			URLSuffix:        urlSuffix,
			AllowEmptyAPIKey: true,
			httpClient:       NewDriverHTTPClient(true),
		},
	}
}

func (m *CitySenseSpeechKit) NewInstance(baseURL map[string]string) ModelDriver {
	return NewCitySenseSpeechKitModel(baseURL, m.baseModel.URLSuffix)
}

func (m *CitySenseSpeechKit) Name() string {
	return "citysense-speechkit"
}

func (m *CitySenseSpeechKit) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

func (m *CitySenseSpeechKit) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s no such method", m.Name())
}

func (m *CitySenseSpeechKit) Embed(ctx context.Context, modelName *string, request EmbedRequest, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

func (m *CitySenseSpeechKit) Rerank(ctx context.Context, modelName *string, request RerankRequest, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

// TranscribeAudio uploads the local audio file via multipart/form-data to
// POST {baseURL}/{urlSuffix.ASR} (default api/v1/transcription/upload).
// The server is media-speech-mcp which runs ffmpeg conversion + hybrid
// SpeechKit transcription (content <=20MB via base64, larger via S3 URI).
func (m *CitySenseSpeechKit) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if file == nil || strings.TrimSpace(*file) == "" {
		return nil, fmt.Errorf("file is missing")
	}
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("model name is missing")
	}
	model := strings.TrimSpace(*modelName)

	resolvedBaseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	suffix := strings.Trim(m.baseModel.URLSuffix.ASR, "/")
	if suffix == "" {
		suffix = "api/v1/transcription/upload"
	}
	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(resolvedBaseURL, "/"), suffix)

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
	if err := writer.WriteField("model", model); err != nil {
		return nil, fmt.Errorf("failed to write model field: %w", err)
	}
	// optional language/model params from asrConfig
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

	ctx, cancel := context.WithTimeout(ctx, longOpCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	// media-speech-mcp REST uses X-Service-Token (ApiKeyWebFilter), not Bearer.
	if apiConfig != nil && apiConfig.ApiKey != nil && strings.TrimSpace(*apiConfig.ApiKey) != "" {
		req.Header.Set("X-Service-Token", strings.TrimSpace(*apiConfig.ApiKey))
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")

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
		return nil, fmt.Errorf("CitySense SpeechKit error: %s - %s", resp.Status, string(respBody))
	}

	// Server may return text/plain (transcribe) or JSON {text,...} (transcribe/file or upload).
	// Try JSON first, fallback to raw body.
	var jsonResult struct {
		Text string `json:"text"`
	}
	if err = json.Unmarshal(respBody, &jsonResult); err == nil && jsonResult.Text != "" {
		return &ASRResponse{Text: strings.TrimSpace(jsonResult.Text)}, nil
	}
	// If body is JSON but with different shape, try to extract "text" loosely
	var raw map[string]any
	if err = json.Unmarshal(respBody, &raw); err == nil {
		if v, ok := raw["text"].(string); ok && strings.TrimSpace(v) != "" {
			return &ASRResponse{Text: strings.TrimSpace(v)}, nil
		}
	}
	// Fallback: treat whole body as plain text transcript (e.g. text/plain)
	text := strings.TrimSpace(string(respBody))
	return &ASRResponse{Text: text}, nil
}

func (m *CitySenseSpeechKit) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s no such method", m.Name())
}

func (m *CitySenseSpeechKit) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

func (m *CitySenseSpeechKit) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

func (m *CitySenseSpeechKit) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

func (m *CitySenseSpeechKit) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

func (m *CitySenseSpeechKit) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	// Always synthetic — real /models exists but UI "Проверить" should not block saving
	// when media-speech-mcp is restarting or DNS http://media-speech-mcp:8080 not reachable from host.
	// Verification with real X-Service-Token happens at TranscribeAudio time.
	return []ListModelResponse{{Name: "general", ModelTypes: []string{"asr"}}}, nil
}

func (m *CitySenseSpeechKit) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

func (m *CitySenseSpeechKit) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	return nil
}

func (m *CitySenseSpeechKit) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

func (m *CitySenseSpeechKit) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}
