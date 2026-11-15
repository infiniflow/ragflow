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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"ragflow/internal/common"

	"go.uber.org/zap"
)

type PaddleOCRLocalModel struct {
	baseModel BaseModel
}

func NewPaddleOCRLocalModel(baseURL map[string]string, urlSuffix URLSuffix) *PaddleOCRLocalModel {
	return &PaddleOCRLocalModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: common.GetSchemeSafeHTTPClient(),
		},
	}
}

func (p *PaddleOCRLocalModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewPaddleOCRLocalModel(baseURL, p.baseModel.URLSuffix)
}

func (p *PaddleOCRLocalModel) Name() string {
	return "paddleocr.local"
}

func (p *PaddleOCRLocalModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	return nil, fmt.Errorf("%s: no such method: ChatWithMessages", p.Name())
}

func (p *PaddleOCRLocalModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s: no such method: ChatStreamlyWithSender", p.Name())
}

func (p *PaddleOCRLocalModel) Embed(ctx context.Context, modelName *string, request EmbedRequest, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s: no such method: Embed", p.Name())
}

func (p *PaddleOCRLocalModel) Rerank(ctx context.Context, modelName *string, request RerankRequest, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s: no such method: Rerank", p.Name())
}

func (p *PaddleOCRLocalModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s: no such method: TranscribeAudio", p.Name())
}

func (p *PaddleOCRLocalModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s: no such method: TranscribeAudioWithSender", p.Name())
}

func (p *PaddleOCRLocalModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s: no such method: AudioSpeech", p.Name())
}

func (p *PaddleOCRLocalModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s: no such method: AudioSpeechWithSender", p.Name())
}

func (p *PaddleOCRLocalModel) OCRFile(ctx context.Context, modelName *string, content []byte, fileURL *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	if len(content) == 0 {
		return nil, fmt.Errorf("local PaddleOCR requires file content, but content is empty")
	}

	apiConfig = paddleOCRResolvedAPIConfig(apiConfig)

	url, err := p.baseModel.EndpointURL(apiConfig, p.baseModel.URLSuffix.OCR)
	if err != nil {
		return nil, err
	}

	base64Str := base64.StdEncoding.EncodeToString(content)
	fileType := paddleOCRDetectFileType(content, fileURL)

	reqData := map[string]interface{}{
		"file":                      base64Str,
		"fileType":                  fileType,
		"useDocOrientationClassify": false,
		"useDocUnwarping":           false,
	}

	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal local PaddleOCR request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, longOpCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if auth := BearerAuth(apiConfig); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	model := ""
	if modelName != nil {
		model = *modelName
	}
	common.Info("paddleocr local submit: sending",
		zap.String("driver", p.Name()),
		zap.String("url", url),
		zap.String("model", model),
		zap.Int("content_bytes", len(content)))

	resp, err := p.baseModel.httpClient.Do(req)
	if err != nil {
		common.Error("paddleocr local submit: request failed",
			err,
			zap.String("driver", p.Name()),
			zap.String("url", url))
		return nil, fmt.Errorf("failed to send request to local PaddleOCR: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody := readErrorBody(resp.Body)
		common.Error("paddleocr local submit: non-200",
			fmt.Errorf("status %d", resp.StatusCode),
			zap.String("driver", p.Name()),
			zap.String("url", url),
			zap.Int("status", resp.StatusCode),
			zap.String("body", errBody))
		return nil, fmt.Errorf("local PaddleOCR failed with status %d: %s", resp.StatusCode, errBody)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	response, err := parsePaddleOCRResultBody(respBody)
	if err != nil {
		common.Error("paddleocr local result: parse failed", err,
			zap.String("driver", p.Name()), zap.String("url", url),
			zap.Int("body_bytes", len(respBody)))
		return nil, fmt.Errorf("local PaddleOCR result: %w", err)
	}
	if len(response.Warnings) > 0 {
		common.Warn("paddleocr local result: partial failure", zap.String("driver", p.Name()), zap.Strings("warnings", response.Warnings))
	}
	common.Info("paddleocr local submit: ok", zap.String("driver", p.Name()), zap.String("url", url), zap.Int("status", resp.StatusCode))
	return response, nil
}

func (p *PaddleOCRLocalModel) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s: no such method: ParseFile", p.Name())
}

func (p *PaddleOCRLocalModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	provider := GetProviderManager().FindProvider(p.Name())
	if provider == nil {
		return nil, fmt.Errorf("provider '%s' not found", p.Name())
	}

	var modelList []ListModelResponse
	for _, model := range provider.Models {
		modelList = append(modelList, ListModelResponse{
			Name:          model.Name,
			ContextLength: model.ContextLength,
			MaxOutput:     model.MaxOutput,
			ModelTypes:    model.ModelTypes,
			Thinking:      model.Thinking,
			MaxDimension:  model.MaxDimension,
			MaxBatchSize:  model.MaxBatchSize,
			Dimensions:    model.Dimensions,
		})
	}

	if len(modelList) == 0 {
		return nil, fmt.Errorf("no models found for provider '%s'", p.Name())
	}

	return modelList, nil
}

func (p *PaddleOCRLocalModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s: no such method: Balance", p.Name())
}

func (p *PaddleOCRLocalModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	return fmt.Errorf("%s: no such method: CheckConnection", p.Name())
}

func (p *PaddleOCRLocalModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s: no such method: ListTasks", p.Name())
}

func (p *PaddleOCRLocalModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s: no such method: ShowTask", p.Name())
}

func (p *PaddleOCRLocalModel) Decide(ctx context.Context, modelName *string, request *DecideRequest, apiConfig *APIConfig, modelUsage *common.ModelUsage) (*DecideResponse, error) {
	return nil, fmt.Errorf("%s: no such method: Decide", p.Name())
}
