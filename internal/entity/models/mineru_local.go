package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

type MinerULocalModel struct {
	BaseURL    map[string]string
	URLSuffix  URLSuffix
	httpClient *http.Client
}

func NewMinerLocalUModel(baseURL map[string]string, urlSuffix URLSuffix) *MinerULocalModel {
	return &MinerULocalModel{
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

func (m *MinerULocalModel) NewInstance(baseURL map[string]string) ModelDriver {
	return &MinerULocalModel{
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

func (m *MinerULocalModel) Name() string {
	return "mineru"
}

func (m *MinerULocalModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

func (m *MinerULocalModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s no such method", m.Name())
}

func (m *MinerULocalModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

func (m *MinerULocalModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

func (m *MinerULocalModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

func (m *MinerULocalModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s no such method", m.Name())
}

func (m *MinerULocalModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

func (m *MinerULocalModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s no such method", m.Name())
}

func (m *MinerULocalModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

func (m *MinerULocalModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

func (m *MinerULocalModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

func (m *MinerULocalModel) CheckConnection(apiConfig *APIConfig) error {
	return fmt.Errorf("%s no such method", m.Name())
}

func (m *MinerULocalModel) ParseFile(modelName *string, content []byte, documentURL *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	if len(content) == 0 {
		return nil, fmt.Errorf("local MinerU API requires file content byte array, but content is empty")
	}

	var region = "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	apiURL := fmt.Sprintf("%s/%s", m.BaseURL[region], m.URLSuffix.DocumentParse)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Get file
	part, err := writer.CreateFormFile("files", "upload_document.pdf")
	if err != nil {
		return nil, fmt.Errorf("failed to create multipart file field: %w", err)
	}
	if _, err = part.Write(content); err != nil {
		return nil, fmt.Errorf("failed to write file content: %w", err)
	}

	if modelName != nil && *modelName != "" {
		_ = writer.WriteField("backend", *modelName)
	} else {
		_ = writer.WriteField("backend", "pipeline")
	}

	if err = writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	req, err := http.NewRequest("POST", apiURL, &body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	if apiConfig != nil && apiConfig.ApiKey != nil && *apiConfig.ApiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != 202 {
		return nil, fmt.Errorf("local MinerU API failed with status %d: %s (URL: %s)", resp.StatusCode, string(respBody), apiURL)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response JSON: %w, body: %s", err, string(respBody))
	}
	// Get task ID
	var taskID string
	if dataMap, ok := result["data"].(map[string]interface{}); ok {
		if tid, ok := dataMap["task_id"].(string); ok {
			taskID = tid
		}
	} else if tid, ok := result["task_id"].(string); ok {
		taskID = tid
	}

	if taskID == "" {
		return nil, fmt.Errorf("failed to extract task_id from local MinerU response: %s", string(respBody))
	}

	return &ParseFileResponse{
		TaskID: taskID,
	}, nil
}

func (m *MinerULocalModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s no such method", m.Name())
}

func (m *MinerULocalModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	if taskID == "" {
		return nil, fmt.Errorf("taskID is empty")
	}

	var region = "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/%s/%s/result", m.BaseURL[region], m.URLSuffix.Task, taskID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create status request: %w", err)
	}

	if apiConfig != nil && apiConfig.ApiKey != nil && *apiConfig.ApiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send status request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read status response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != 202 {
		return nil, fmt.Errorf("MinerU local status API failed with status %d: %s", resp.StatusCode, string(body))
	}

	// parse JSON
	var result map[string]interface{}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	content := ""

	// results
	results, ok := result["results"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing results field")
	}

	// Get markdown
	for _, fileObj := range results {

		fileMap, ok := fileObj.(map[string]interface{})
		if !ok {
			continue
		}

		md, ok := fileMap["md_content"].(string)
		if ok {
			content = md
			break
		}
	}

	if content == "" {
		return nil, fmt.Errorf("md_content not found")
	}

	return &TaskResponse{
		Segments: []TaskSegment{
			{
				Index:   1,
				Content: content,
			},
		},
	}, nil
}
