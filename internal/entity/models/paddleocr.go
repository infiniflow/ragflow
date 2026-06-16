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
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

type PaddleOCRModel struct {
	baseModel BaseModel
}

func NewPaddleOCRModel(baseURL map[string]string, urlSuffix URLSuffix) *PaddleOCRModel {
	return &PaddleOCRModel{
		baseModel: BaseModel{
			BaseURL:          baseURL,
			URLSuffix:        urlSuffix,
			AllowEmptyAPIKey: true,
			httpClient:       NewDriverHTTPClient(),
		},
	}
}

func (p PaddleOCRModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewPaddleOCRModel(baseURL, p.baseModel.URLSuffix)
}

func (p *PaddleOCRModel) Name() string {
	return "paddle_ocr.net"
}

func (p *PaddleOCRModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}

func (p *PaddleOCRModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", p.Name())
}

func (p *PaddleOCRModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}

func (p *PaddleOCRModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}

func (p *PaddleOCRModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}

func (p *PaddleOCRModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", p.Name())
}

func (p *PaddleOCRModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}

func (p *PaddleOCRModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", p.Name())
}

type paddleSubmitResponse struct {
	Data struct {
		JobId string `json:"jobId"`
	} `json:"data"`
}

type paddlePollResponse struct {
	Data struct {
		State     string `json:"state"`
		ErrorMsg  string `json:"errorMsg"`
		ResultUrl struct {
			JsonUrl string `json:"jsonUrl"`
		} `json:"resultUrl"`
	} `json:"data"`
}

type paddleJsonlLine struct {
	Result struct {
		LayoutParsingResults []struct {
			Markdown struct {
				Text string `json:"text"`
			} `json:"markdown"`
		} `json:"layoutParsingResults"`
	} `json:"result"`
}

func (p *PaddleOCRModel) OCRFile(modelName *string, content []byte, fileURL *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	if err := p.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if (content == nil || len(content) == 0) && (fileURL == nil || *fileURL == "") {
		return nil, fmt.Errorf("content and fileURL cannot be both empty")
	}

	resolvedBaseURL, err := p.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, p.baseModel.URLSuffix.OCR)

	optionalPayload := map[string]bool{
		"useDocOrientationClassify": false,
		"useDocUnwarping":           false,
		"useChartRecognition":       false,
	}
	optBytes, _ := json.Marshal(optionalPayload)

	// One generous deadline bounds the whole OCR operation (submit + poll +
	// result download), so the poll loop below can no longer spin forever.
	ctx, cancel := context.WithTimeout(context.Background(), longOpCallTimeout)
	defer cancel()

	var req *http.Request

	if fileURL != nil && strings.HasPrefix(*fileURL, "http") {
		reqData := map[string]interface{}{
			"fileUrl":         *fileURL,
			"model":           *modelName,
			"optionalPayload": optionalPayload,
		}
		jsonData, err := json.Marshal(reqData)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal json: %w", err)
		}
		req, err = http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
	} else {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		_ = writer.WriteField("model", *modelName)
		_ = writer.WriteField("optionalPayload", string(optBytes))

		part, err := writer.CreateFormFile("file", "document.pdf")
		if err != nil {
			return nil, fmt.Errorf("failed to create form file: %w", err)
		}
		part.Write(content)
		writer.Close()

		req, err = http.NewRequestWithContext(ctx, "POST", url, body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
	}

	if auth := BearerAuth(apiConfig); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := p.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to submit job: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("submit job failed: %s", string(respBody))
	}

	var submitResp paddleSubmitResponse
	if err := json.Unmarshal(respBody, &submitResp); err != nil {
		return nil, fmt.Errorf("failed to parse submit response: %w", err)
	}

	jobId := submitResp.Data.JobId
	if jobId == "" {
		return nil, fmt.Errorf("failed to get jobId from response")
	}

	pollUrl := fmt.Sprintf("%s/%s", url, jobId)
	var jsonlUrl string

	for {
		select {
		case <-time.After(3 * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}

		pollReq, _ := http.NewRequestWithContext(ctx, "GET", pollUrl, nil)
		if auth := BearerAuth(apiConfig); auth != "" {
			pollReq.Header.Set("Authorization", auth)
		}

		pollResp, err := p.baseModel.httpClient.Do(pollReq)
		if err != nil {
			return nil, fmt.Errorf("failed to poll job status: %w", err)
		}

		pollBody, _ := io.ReadAll(pollResp.Body)
		pollResp.Body.Close()

		if pollResp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("poll job failed: %s", string(pollBody))
		}

		var pollData paddlePollResponse
		if err = json.Unmarshal(pollBody, &pollData); err != nil {
			return nil, fmt.Errorf("failed to parse poll response: %w", err)
		}

		// end if 'done' or 'failed'
		state := pollData.Data.State
		if state == "done" {
			jsonlUrl = pollData.Data.ResultUrl.JsonUrl
			break
		} else if state == "failed" {
			return nil, fmt.Errorf("ocr job failed on server: %s", pollData.Data.ErrorMsg)
		}
	}

	if jsonlUrl == "" {
		return nil, fmt.Errorf("job done but jsonl url is empty")
	}

	resReq, err := http.NewRequestWithContext(ctx, "GET", jsonlUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for jsonl: %w", err)
	}

	resResp, err := p.baseModel.httpClient.Do(resReq)
	if err != nil {
		return nil, fmt.Errorf("failed to download jsonl result: %w", err)
	}
	defer resResp.Body.Close()

	if resResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download jsonl, status: %d", resResp.StatusCode)
	}

	var fullMarkdown strings.Builder
	scanner := bufio.NewScanner(resResp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var lineData paddleJsonlLine
		if err := json.Unmarshal([]byte(line), &lineData); err != nil {
			continue
		}

		for _, layoutRes := range lineData.Result.LayoutParsingResults {
			fullMarkdown.WriteString(layoutRes.Markdown.Text)
			fullMarkdown.WriteString("\n\n")
		}
	}

	if err = scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading jsonl: %w", err)
	}

	extractedText := strings.TrimSpace(fullMarkdown.String())

	return &OCRFileResponse{Text: &extractedText}, nil
}

func (p *PaddleOCRModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}

func (p *PaddleOCRModel) ListModels(apiConfig *APIConfig) ([]ListModelResponse, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}

func (p *PaddleOCRModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}

func (p *PaddleOCRModel) CheckConnection(apiConfig *APIConfig) error {
	return fmt.Errorf("%s, no such method", p.Name())
}

func (p *PaddleOCRModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}

func (p *PaddleOCRModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}
