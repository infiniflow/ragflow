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
)

type MinerUModel struct {
	baseModel BaseModel
}

func NewMinerUModel(baseURL map[string]string, urlSuffix URLSuffix) *MinerUModel {
	return &MinerUModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(),
		},
	}
}

func (m *MinerUModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewMinerUModel(baseURL, m.baseModel.URLSuffix)
}

func (m *MinerUModel) Name() string {
	return "mineru.net"
}

func (m *MinerUModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

func (m *MinerUModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s no such method", m.Name())
}

func (m *MinerUModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

func (m *MinerUModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

func (m *MinerUModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

func (m *MinerUModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s no such method", m.Name())
}

func (m *MinerUModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

func (m *MinerUModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s no such method", m.Name())
}

func (m *MinerUModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

func (m *MinerUModel) ListModels(apiConfig *APIConfig) ([]ListModelResponse, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

func (m *MinerUModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

func (m *MinerUModel) CheckConnection(apiConfig *APIConfig) error {
	return fmt.Errorf("%s no such method", m.Name())
}

type mineruTaskSubmitResponse struct {
	Code int `json:"code"`
	Data struct {
		TaskID string `json:"task_id"`
	} `json:"data"`
	Msg     string `json:"msg"`
	TraceID string `json:"trace_id"`
}

func (m *MinerUModel) ParseFile(modelName *string, content []byte, documentURL *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if documentURL == nil || *documentURL == "" {
		return nil, fmt.Errorf("MinerU API requires a valid public document URL; direct file upload is not supported")
	}

	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	resolvedBaseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	apiURL := fmt.Sprintf("%s/api/%s", resolvedBaseURL, m.baseModel.URLSuffix.DocumentParse)

	reqBody := map[string]interface{}{
		"url": *documentURL,
	}

	if modelName != nil && *modelName != "" {
		reqBody["model_version"] = *modelName
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), longOpCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := m.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MinerU API failed with status %d: %s", resp.StatusCode, string(body))
	}

	var taskResp mineruTaskSubmitResponse
	if err := json.Unmarshal(body, &taskResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if taskResp.Code != 0 {
		return nil, fmt.Errorf("MinerU task creation failed (code %d): %s", taskResp.Code, taskResp.Msg)
	}

	return &ParseFileResponse{
		TaskID: taskResp.Data.TaskID,
	}, nil
}

func (m *MinerUModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

type mineruTaskQueryResponse struct {
	Code int `json:"code"`
	Data struct {
		TaskID          string `json:"task_id"`
		State           string `json:"state"` // including: pending, running, done, failed, converting
		FullZipURL      string `json:"full_zip_url"`
		ErrMsg          string `json:"err_msg"`
		ExtractProgress struct {
			ExtractedPages int `json:"extracted_pages"`
			TotalPages     int `json:"total_pages"`
		} `json:"extract_progress"`
	} `json:"data"`
	Msg string `json:"msg"`
}

func (m *MinerUModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	// URL: https://mineru.net/api/v4/extract/task/{task_id}
	resolvedBaseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	apiURL := fmt.Sprintf("%s/api/%s/%s", resolvedBaseURL, m.baseModel.URLSuffix.DocumentParse, taskID)

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := m.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MinerU query API failed with status %d: %s", resp.StatusCode, string(body))
	}

	var queryResp mineruTaskQueryResponse
	if err := json.Unmarshal(body, &queryResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if queryResp.Code != 0 {
		return nil, fmt.Errorf("MinerU task query failed: %s", queryResp.Msg)
	}

	// failed state
	if queryResp.Data.State == "failed" {
		return nil, fmt.Errorf("MinerU task failed: %s", queryResp.Data.ErrMsg)
	}

	content := ""
	if queryResp.Data.State == "done" {
		content = queryResp.Data.FullZipURL
	} else if queryResp.Data.State == "running" {
		content = fmt.Sprintf("Task is running... Progress: %d / %d pages",
			queryResp.Data.ExtractProgress.ExtractedPages,
			queryResp.Data.ExtractProgress.TotalPages)
	} else {
		// queue or formating
		content = fmt.Sprintf("Task state: %s", queryResp.Data.State)
	}

	return &TaskResponse{
		Segments: []TaskSegment{
			{
				Index:   0,
				Content: content,
			},
		},
	}, nil
}
