package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type MinerUModel struct {
	BaseURL    map[string]string
	URLSuffix  URLSuffix
	httpClient *http.Client
}

func NewMinerUModel(baseURL map[string]string, urlSuffix URLSuffix) *MinerUModel {
	return &MinerUModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
		httpClient: &http.Client{
			Timeout: time.Second * 120,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     time.Second * 90,
				DisableCompression:  false,
			},
		},
	}
}

func (m *MinerUModel) NewInstance(baseURL map[string]string) ModelDriver {
	return &MinerUModel{
		BaseURL:   baseURL,
		URLSuffix: m.URLSuffix,
		httpClient: &http.Client{
			Timeout: time.Second * 120,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     time.Second * 90,
				DisableCompression:  false,
			},
		},
	}
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

func (m *MinerUModel) ListModels(apiConfig *APIConfig) ([]string, error) {
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
	if documentURL == nil || *documentURL == "" {
		return nil, fmt.Errorf("MinerU API requires a valid public document URL; direct file upload is not supported")
	}

	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}

	var region = "default"
	if apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	apiURL := fmt.Sprintf("%s/api/%s", m.BaseURL[region], m.URLSuffix.DocumentParse)

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

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := m.httpClient.Do(req)
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
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}

	var region = "default"
	if apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	// URL: https://mineru.net/api/v4/extract/task/{task_id}
	apiURL := fmt.Sprintf("%s/api/%s/%s", m.BaseURL[region], m.URLSuffix.DocumentParse, taskID)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := m.httpClient.Do(req)
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
