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
	"strings"
)

type PaddleOCRLocalModel struct {
	baseModel BaseModel
}

func NewPaddleOCRLocalModel(baseURL map[string]string, urlSuffix URLSuffix) *PaddleOCRLocalModel {
	return &PaddleOCRLocalModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(),
		},
	}
}

func (p *PaddleOCRLocalModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewPaddleOCRLocalModel(baseURL, p.baseModel.URLSuffix)
}

func (p *PaddleOCRLocalModel) Name() string {
	return "paddleocr"
}

func (p *PaddleOCRLocalModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	return nil, fmt.Errorf("%s no such method", p.Name())
}

func (p *PaddleOCRLocalModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s no such method", p.Name())
}

func (p *PaddleOCRLocalModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s no such method", p.Name())
}

func (p *PaddleOCRLocalModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s no such method", p.Name())
}

func (p *PaddleOCRLocalModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s no such method", p.Name())
}

func (p *PaddleOCRLocalModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s no such method", p.Name())
}

func (p *PaddleOCRLocalModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s no such method", p.Name())
}

func (p *PaddleOCRLocalModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s no such method", p.Name())
}

// For different model, paddleOCR have different url_suffix:
// e.g.PaddleOCR-VL: /layout-parsing   |   PP-OCRv5: /ocr
// We select `PaddleOCR-VL` here
type paddleLocalOCRResponse struct {
	LogId     string `json:"logId"`
	ErrorCode int    `json:"errorCode"`
	ErrorMsg  string `json:"errorMsg"`
	Result    struct {
		LayoutParsingResults []struct {
			Markdown struct {
				Text string `json:"text"`
			} `json:"markdown"`
		} `json:"layoutParsingResults"`
	} `json:"result"`
}

func (p *PaddleOCRLocalModel) OCRFile(modelName *string, content []byte, fileURL *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	if len(content) == 0 {
		return nil, fmt.Errorf("local PaddleOCR requires file content, but content is empty")
	}

	resolvedBaseURL, err := p.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, p.baseModel.URLSuffix.OCR)

	base64Str := base64.StdEncoding.EncodeToString(content)

	fileType := 1
	if fileURL != nil && *fileURL != "" {
		if strings.HasSuffix(strings.ToLower(*fileURL), ".pdf") {
			fileType = 0
		}
	} else if len(content) > 4 && string(content[:4]) == "%PDF" {
		fileType = 0
	}

	reqData := map[string]interface{}{
		"file":     base64Str,
		"fileType": fileType,
	}
	if ocrConfig != nil && strings.TrimSpace(ocrConfig.Algorithm) != "" {
		reqData["algorithm"] = ocrConfig.Algorithm
	}

	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal local PaddleOCR request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), longOpCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if auth := BearerAuth(apiConfig); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := p.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to local PaddleOCR: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("local PaddleOCR failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var ocrResp paddleLocalOCRResponse
	if err := json.Unmarshal(respBody, &ocrResp); err != nil {
		return nil, fmt.Errorf("failed to parse local PaddleOCR response: %w, raw: %s", err, string(respBody))
	}

	if ocrResp.ErrorCode != 0 {
		return nil, fmt.Errorf("local PaddleOCR task failed: %s (errorCode: %d)", ocrResp.ErrorMsg, ocrResp.ErrorCode)
	}

	var fullMarkdown strings.Builder
	for _, layoutRes := range ocrResp.Result.LayoutParsingResults {
		if layoutRes.Markdown.Text != "" {
			fullMarkdown.WriteString(layoutRes.Markdown.Text)
			fullMarkdown.WriteString("\n\n")
		}
	}

	extractedText := strings.TrimSpace(fullMarkdown.String())

	return &OCRFileResponse{
		Text: &extractedText,
	}, nil
}

func (p *PaddleOCRLocalModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s no such method", p.Name())
}

func (p *PaddleOCRLocalModel) ListModels(apiConfig *APIConfig) ([]ListModelResponse, error) {
	return nil, fmt.Errorf("%s no such method", p.Name())
}

func (p *PaddleOCRLocalModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s no such method", p.Name())
}

func (p *PaddleOCRLocalModel) CheckConnection(apiConfig *APIConfig) error {
	return fmt.Errorf("%s no such method", p.Name())
}

func (p *PaddleOCRLocalModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s no such method", p.Name())
}

func (p *PaddleOCRLocalModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s no such method", p.Name())
}
