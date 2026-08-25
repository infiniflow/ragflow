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

	"go.uber.org/zap"
)

// CitySenseSpeechKit реализует ModelDriver для media-speech-mcp (Yandex SpeechKit).
// Использует гибридный REST API на базе AudioPipelineService / SpeechKitService.
// Авторизация — X-Service-Token: MEDIA_SPEECH_API_KEY (ApiKeyWebFilter), а не Bearer.
type CitySenseSpeechKit struct {
	baseModel BaseModel
}

func NewCitySenseSpeechKitModel(baseURL map[string]string, urlSuffix URLSuffix) *CitySenseSpeechKit {
	// URL задается только через UI (base_url инстанса) или env MEDIA_SPEECH_BASE_URL — хардкода нет
	if envURL := strings.TrimSpace(os.Getenv("MEDIA_SPEECH_BASE_URL")); envURL != "" {
		if baseURL == nil {
			baseURL = map[string]string{}
		}
		if strings.TrimSpace(baseURL["default"]) == "" {
			baseURL["default"] = strings.TrimSuffix(envURL, "/")
		}
	}
	return &CitySenseSpeechKit{
		baseModel: BaseModel{
			BaseURL:          baseURL,
			URLSuffix:        urlSuffix,
			AllowEmptyAPIKey: false,
			httpClient:       NewDriverHTTPClient(true),
			authHeader: func(apiConfig *APIConfig) (string, string) {
				if apiConfig == nil || apiConfig.ApiKey == nil {
					return "X-Service-Token", ""
				}
				return "X-Service-Token", strings.TrimSpace(*apiConfig.ApiKey)
			},
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
	return nil, fmt.Errorf("метод ChatWithMessages не поддерживается провайдером %s", m.Name())
}

func (m *CitySenseSpeechKit) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("метод ChatStreamlyWithSender не поддерживается провайдером %s", m.Name())
}

func (m *CitySenseSpeechKit) Embed(ctx context.Context, modelName *string, request EmbedRequest, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("метод Embed не поддерживается провайдером %s", m.Name())
}

func (m *CitySenseSpeechKit) Rerank(ctx context.Context, modelName *string, request RerankRequest, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	return nil, fmt.Errorf("метод Rerank не поддерживается провайдером %s", m.Name())
}

// TranscribeAudio загружает локальный аудиофайл через multipart/form-data на
// POST {baseURL}/{urlSuffix.ASR} (по умолчанию api/v1/transcription/upload).
// Сервер — media-speech-mcp, который выполняет конвертацию ffmpeg и гибридную
// транскрибацию SpeechKit (контент <=20MB через base64, больше — через S3 URI).
func (m *CitySenseSpeechKit) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if file == nil || strings.TrimSpace(*file) == "" {
		return nil, fmt.Errorf("файл не указан")
	}
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("имя модели не указано")
	}
	model := strings.TrimSpace(*modelName)

	resolvedBaseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}

	// URL-режим: если file — http(s) ссылка, используем JSON эндпоинт /transcription/transcribe (как в Python-драйвере)
	trimmedFile := strings.TrimSpace(*file)
	if strings.HasPrefix(trimmedFile, "http://") || strings.HasPrefix(trimmedFile, "https://") {
		transcribeSuffix := "api/v1/transcription/transcribe"
		url := fmt.Sprintf("%s/%s", strings.TrimSuffix(resolvedBaseURL, "/"), transcribeSuffix)
		ctx2, cancel := context.WithTimeout(ctx, longOpCallTimeout)
		defer cancel()
		payload := map[string]any{
			"mediaFileUrl": trimmedFile,
			"fileName":     filepath.Base(trimmedFile),
		}
		if asrConfig != nil && asrConfig.Params != nil {
			if lang, ok := asrConfig.Params["language"]; ok {
				if s, ok := lang.(string); ok && strings.TrimSpace(s) != "" {
					payload["language"] = strings.TrimSpace(s)
				}
			}
		}
		jsonBody, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx2, http.MethodPost, url, bytes.NewReader(jsonBody))
		if err != nil {
			return nil, fmt.Errorf("не удалось создать запрос: %w", err)
		}
		m.baseModel.applyAuth(req, apiConfig)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err := m.baseModel.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("не удалось отправить запрос к CitySense %s: %w", url, err)
		}
		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("не удалось прочитать тело ответа: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("ошибка CitySense SpeechKit: %s - %s", resp.Status, string(respBody))
		}
		var raw map[string]any
		if err = json.Unmarshal(respBody, &raw); err == nil {
			if v, ok := raw["text"].(string); ok && strings.TrimSpace(v) != "" {
				return &ASRResponse{Text: strings.TrimSpace(v)}, nil
			}
		}
		return &ASRResponse{Text: strings.TrimSpace(string(respBody))}, nil
	}

	suffix := strings.Trim(m.baseModel.URLSuffix.ASR, "/")
	if suffix == "" {
		suffix = "api/v1/transcription/upload"
	}
	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(resolvedBaseURL, "/"), suffix)

	// Стриминг через io.Pipe — не держим весь файл в RAM (важно для 1ГБ/4ч)
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	contentType := writer.FormDataContentType()
	// MIME по расширению, фолбэк audio/mpeg
	ext := strings.ToLower(filepath.Ext(trimmedFile))
	mimeType := "audio/mpeg"
	switch ext {
	case ".wav":
		mimeType = "audio/wav"
	case ".flac":
		mimeType = "audio/flac"
	case ".ogg", ".oga":
		mimeType = "audio/ogg"
	case ".m4a", ".mp4":
		mimeType = "audio/mp4"
	case ".webm":
		mimeType = "audio/webm"
	}
	go func() {
		defer pw.Close()
		audioFile, err := os.Open(trimmedFile)
		if err != nil {
			pw.CloseWithError(fmt.Errorf("не удалось открыть аудиофайл: %w", err))
			return
		}
		defer audioFile.Close()
		partHeaders := make(map[string][]string)
		partHeaders["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="%s"; filename="%s"`, "file", filepath.Base(trimmedFile))}
		partHeaders["Content-Type"] = []string{mimeType}
		part, err := writer.CreatePart(partHeaders)
		if err != nil {
			pw.CloseWithError(fmt.Errorf("не удалось создать multipart-файл: %w", err))
			return
		}
		if _, err = io.Copy(part, audioFile); err != nil {
			pw.CloseWithError(fmt.Errorf("не удалось скопировать аудиоданные: %w", err))
			return
		}
		if err := writer.WriteField("model", model); err != nil {
			pw.CloseWithError(fmt.Errorf("не удалось записать поле model: %w", err))
			return
		}
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
					pw.CloseWithError(fmt.Errorf("не удалось записать поле %s: %w", key, err))
					return
				}
			}
		}
		writer.Close()
	}()

	ctx, cancel := context.WithTimeout(ctx, longOpCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, pr)
	if err != nil {
		return nil, fmt.Errorf("не удалось создать запрос: %w", err)
	}
	m.baseModel.applyAuth(req, apiConfig)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	resp, err := m.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("не удалось отправить запрос к CitySense %s: %w", url, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать тело ответа: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ошибка CitySense SpeechKit: %s - %s", resp.Status, string(respBody))
	}

	var raw map[string]any
	if err = json.Unmarshal(respBody, &raw); err == nil {
		if v, ok := raw["text"].(string); ok && strings.TrimSpace(v) != "" {
			return &ASRResponse{Text: strings.TrimSpace(v)}, nil
		}
	}
	return &ASRResponse{Text: strings.TrimSpace(string(respBody))}, nil
}

func (m *CitySenseSpeechKit) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("метод TranscribeAudioWithSender не поддерживается провайдером %s", m.Name())
}

func (m *CitySenseSpeechKit) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return nil, fmt.Errorf("метод AudioSpeech не поддерживается провайдером %s", m.Name())
}

func (m *CitySenseSpeechKit) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return nil, fmt.Errorf("метод AudioSpeechWithSender не поддерживается провайдером %s", m.Name())
}

func (m *CitySenseSpeechKit) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("метод OCRFile не поддерживается провайдером %s", m.Name())
}

func (m *CitySenseSpeechKit) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("метод ParseFile не поддерживается провайдером %s", m.Name())
}

func (m *CitySenseSpeechKit) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	// Гибрид: пробуем живой GET, при любой ошибке — fallback на synthetic citysense-speech-kit-v1 с warning.
	// Это не блокирует сохранение инстанса в UI когда media-speech-mcp перезапускается
	// или недоступен по DNS из хоста. Реальная верификация с X-Service-Token происходит в TranscribeAudio.
	models, err := m.listModelsLive(ctx, apiConfig)
	if err == nil {
		return models, nil
	}
	common.Warn("CitySense SpeechKit: ListModels live check failed, fallback to synthetic citysense-speech-kit-v1", zap.Error(err))
	return []ListModelResponse{{Name: "citysense-speech-kit-v1", ModelTypes: []string{"asr"}}}, nil
}

func (m *CitySenseSpeechKit) listModelsLive(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	resolvedBaseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(resolvedBaseURL) == "" {
		return nil, fmt.Errorf("base URL не задан — укажите через UI Base URL или env MEDIA_SPEECH_BASE_URL")
	}
	suffix := strings.Trim(m.baseModel.URLSuffix.Models, "/")
	if suffix == "" {
		suffix = "api/v1/models"
	}
	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(resolvedBaseURL, "/"), suffix)
	ctx2, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx2, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("не удалось создать запрос: %w", err)
	}
	m.baseModel.applyAuth(req, apiConfig)
	req.Header.Set("Accept", "application/json")
	resp, err := m.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("не удалось подключиться к CitySense по адресу %s: %w", resolvedBaseURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать тело ответа: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("ключ CitySense недействителен (несовпадение X-Service-Token) для %s: %s", resolvedBaseURL, string(body))
		}
		return nil, fmt.Errorf("запрос API завершился с ошибкой %d: %s", resp.StatusCode, string(body))
	}
	var modelList ModelList
	if err = json.Unmarshal(body, &modelList); err != nil {
		return nil, fmt.Errorf("не удалось разобрать ответ: %w", err)
	}
	if modelList.Models == nil {
		return nil, fmt.Errorf("неверный формат списка моделей")
	}
	models := ParseListModel(modelList)
	if len(models) == 0 {
		return nil, fmt.Errorf("неверный формат списка моделей")
	}
	return models, nil
}

func (m *CitySenseSpeechKit) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("метод Balance не поддерживается провайдером %s", m.Name())
}

func (m *CitySenseSpeechKit) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := m.listModelsLive(ctx, apiConfig)
	return err
}

func (m *CitySenseSpeechKit) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("метод ListTasks не поддерживается провайдером %s", m.Name())
}

func (m *CitySenseSpeechKit) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("метод ShowTask не поддерживается провайдером %s", m.Name())
}
