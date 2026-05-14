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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"ragflow/internal/common"
	"strconv"
	"strings"
	"time"
)

// SiliconflowModel implements ModelDriver for Siliconflow
type SiliconflowModel struct {
	BaseURL    map[string]string
	URLSuffix  URLSuffix
	httpClient *http.Client // Reusable HTTP client with connection pool
}

func (s *SiliconflowModel) ParseFile() {
	//TODO implement me
	panic("implement me")
}

// NewSiliconflowModel creates a new Siliconflow model instance
func NewSiliconflowModel(baseURL map[string]string, urlSuffix URLSuffix) *SiliconflowModel {
	return &SiliconflowModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				DisableCompression:  false,
			},
		},
	}
}

func (z *SiliconflowModel) NewInstance(baseURL map[string]string) ModelDriver {
	return nil
}

func (z *SiliconflowModel) Name() string {
	return "siliconflow"
}

// SiliconflowRerankRequest represents SILICONFLOW rerank request
type SiliconflowRerankRequest struct {
	Model           string   `json:"model"`
	Query           string   `json:"query"`
	Documents       []string `json:"documents"`
	TopN            int      `json:"top_n"`
	ReturnDocuments bool     `json:"return_documents"`
	MaxChunksPerDoc int      `json:"max_chunks_per_doc"`
	OverlapTokens   int      `json:"overlap_tokens"`
}

// ChatWithMessages sends multiple messages with roles and returns response
func (z *SiliconflowModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is nil or empty")
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	region := "default"
	if apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}
	url := fmt.Sprintf("%s/%s", z.BaseURL[region], z.URLSuffix.Chat)

	// Convert messages to the format expected by API
	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	// Build request body
	reqBody := map[string]interface{}{
		"model":       modelName,
		"messages":    apiMessages,
		"stream":      false,
		"temperature": 1,
	}

	if chatModelConfig != nil {
		if chatModelConfig.Stream != nil {
			reqBody["stream"] = *chatModelConfig.Stream
		}

		if chatModelConfig.MaxTokens != nil {
			reqBody["max_tokens"] = *chatModelConfig.MaxTokens
		}

		if chatModelConfig.Temperature != nil {
			reqBody["temperature"] = *chatModelConfig.Temperature
		}

		if chatModelConfig.TopP != nil {
			reqBody["top_p"] = *chatModelConfig.TopP
		}

		if chatModelConfig.Stop != nil {
			reqBody["stop"] = *chatModelConfig.Stop
		}
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := z.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	firstChoice, ok := choices[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid choice format")
	}

	messageMap, ok := firstChoice["message"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid message format")
	}

	content, ok := messageMap["content"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid content format")
	}

	var reasonContent string
	if chatModelConfig != nil && chatModelConfig.Thinking != nil && *chatModelConfig.Thinking {
		reasonContent, ok = messageMap["reasoning_content"].(string)
		if !ok {
			// If reasoning_content not in response, try parsing from content tags
			reasoning, answer := GetThinkingAndAnswer(chatModelConfig.ModelClass, &content)
			if reasoning != nil {
				reasonContent = *reasoning
				content = *answer
			}
		} else {
			// if first char of reasonContent is \n remove the '\n'
			if reasonContent != "" && reasonContent[0] == '\n' {
				reasonContent = reasonContent[1:]
			}
		}
	}

	chatResponse := &ChatResponse{
		Answer:        &content,
		ReasonContent: &reasonContent,
	}

	return chatResponse, nil
}

// ChatStreamlyWithSender sends messages and streams response via sender function (best performance, no channel)
func (z *SiliconflowModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, sender func(*string, *string) error) error {
	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	var region = "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/chat/completions", z.BaseURL[region])

	// Convert messages to API format
	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	// Build request body with streaming enabled
	reqBody := map[string]interface{}{
		"model":       modelName,
		"messages":    apiMessages,
		"stream":      true,
		"temperature": 1,
	}

	if chatModelConfig != nil {
		if chatModelConfig.Stream != nil {
			reqBody["stream"] = *chatModelConfig.Stream
		}

		if chatModelConfig.MaxTokens != nil {
			reqBody["max_tokens"] = *chatModelConfig.MaxTokens
		}

		if chatModelConfig.Temperature != nil {
			reqBody["temperature"] = *chatModelConfig.Temperature
		}

		if chatModelConfig.DoSample != nil {
			reqBody["do_sample"] = *chatModelConfig.DoSample
		}

		if chatModelConfig.TopP != nil {
			reqBody["top_p"] = *chatModelConfig.TopP
		}

		if chatModelConfig.Stop != nil {
			reqBody["stop"] = *chatModelConfig.Stop
		}

		if chatModelConfig.Thinking != nil {
			if *chatModelConfig.Thinking {
				reqBody["thinking"] = map[string]interface{}{
					"type": "enabled",
				}
			} else {
				reqBody["thinking"] = map[string]interface{}{
					"type": "disabled",
				}
			}
		}
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := z.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// SSE parsing: read line by line
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		common.Info(line)

		// SSE data line starts with "data:"
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		// Extract JSON after "data:"
		data := strings.TrimSpace(line[5:])

		// [DONE] marks the end of stream
		if data == "[DONE]" {
			break
		}

		// Parse the JSON event
		var event map[string]interface{}
		if err = json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		choices, ok := event["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			continue
		}

		firstChoice, ok := choices[0].(map[string]interface{})
		if !ok {
			continue
		}

		delta, ok := firstChoice["delta"].(map[string]interface{})
		if !ok {
			continue
		}

		reasoningContent, ok := delta["reasoning_content"].(string)
		if ok && reasoningContent != "" {
			if err := sender(nil, &reasoningContent); err != nil {
				return err
			}
		}

		content, ok := delta["content"].(string)
		if ok && content != "" {
			if err := sender(&content, nil); err != nil {
				return err
			}
		}

		finishReason, ok := firstChoice["finish_reason"].(string)
		if ok && finishReason != "" {
			break
		}
	}

	// Send [DONE] marker for OpenAI compatibility
	endOfStream := "[DONE]"
	if err = sender(&endOfStream, nil); err != nil {
		return err
	}

	return scanner.Err()
}

type siliconflowEmbeddingResponse struct {
	Object []string                   `json:"object"`
	Model  string                     `json:"model"`
	Data   []siliconflowEmbeddingData `json:"data"`
	Usage  siliconflowUsage           `json:"usage"`
}

type siliconflowEmbeddingData struct {
	Object    string    `json:"object"`
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

type siliconflowUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// siliconflowMaxBatchSize is the per-request input limit documented at
// https://docs.siliconflow.cn/en/api-reference/embeddings/create-embeddings.
const siliconflowMaxBatchSize = 32

// Embed embeds a list of texts into embeddings
func (s *SiliconflowModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	if len(texts) == 0 {
		return []EmbeddingData{}, nil
	}
	if len(texts) > siliconflowMaxBatchSize {
		return nil, fmt.Errorf("siliconflow supports a maximum of %d inputs per request", siliconflowMaxBatchSize)
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	var region = "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(s.BaseURL[region], "/"), s.URLSuffix.Embedding)

	apiKey := ""
	if apiConfig != nil && apiConfig.ApiKey != nil {
		apiKey = *apiConfig.ApiKey
	}

	reqBody := map[string]interface{}{
		"model": modelName,
		"input": texts,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()

	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SILICONFLOW API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed siliconflowEmbeddingResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var embeddings []EmbeddingData
	for _, dataElem := range parsed.Data {
		var embeddingData EmbeddingData
		embeddingData.Embedding = dataElem.Embedding
		embeddingData.Index = dataElem.Index
		embeddings = append(embeddings, embeddingData)
	}

	return embeddings, nil
}

func (z *SiliconflowModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	var region = "default"
	if apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/%s", z.BaseURL[region], z.URLSuffix.Models)

	// Build request body
	reqBody := map[string]interface{}{}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("GET", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := z.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var modelList DSModelList
	if err = json.Unmarshal(body, &modelList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var models []string
	for _, model := range modelList.Models {
		modelName := model.ID
		if model.OwnedBy != "" {
			modelName = model.ID + "@" + model.OwnedBy
		}
		models = append(models, modelName)
	}

	return models, nil
}

type siliconflowBalanceResponse struct {
	Code    int    `json:"code"`
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    struct {
		Balance      string `json:"balance"`
		TotalBalance string `json:"totalBalance"`
	} `json:"data"`
}

func (z *SiliconflowModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}

	region := "default"
	if apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	baseURL := z.BaseURL["default"]
	if region != "default" {
		if regional, ok := z.BaseURL[region]; ok && regional != "" {
			baseURL = regional
		}
	}
	if baseURL == "" {
		return nil, fmt.Errorf("siliconflow: no base URL configured for default region")
	}

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), z.URLSuffix.Balance)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := z.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SiliconFlow balance API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed siliconflowBalanceResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !parsed.Status {
		msg := parsed.Message
		if msg == "" {
			msg = "unknown API error"
		}
		return nil, fmt.Errorf("SiliconFlow API error (code %d): %s", parsed.Code, msg)
	}

	raw := parsed.Data.TotalBalance
	if raw == "" {
		raw = parsed.Data.Balance
	}
	if raw == "" {
		return nil, fmt.Errorf("no balance info in response")
	}

	total, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid balance %q: %w", raw, err)
	}

	return map[string]interface{}{
		"balance":  total,
		"currency": "CNY",
	}, nil
}

func (z *SiliconflowModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := z.ListModels(apiConfig)
	if err != nil {
		return err
	}
	return nil
}

// SiliconflowRerankResponse represents SILICONFLOW rerank response
type SiliconflowRerankResponse struct {
	ID      string `json:"id"`
	Results []struct {
		Index    int `json:"index"`
		Document struct {
			Text string `json:"text"`
		} `json:"document"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
	Meta struct {
		Tokens struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			ImageTokens  int `json:"image_tokens"`
		} `json:"tokens"`
		BilledUnits struct {
			InputTokens     int `json:"input_tokens"`
			OutputTokens    int `json:"output_tokens"`
			ImageTokens     int `json:"image_tokens"`
			SearchUnits     int `json:"search_units"`
			Classifications int `json:"classifications"`
		} `json:"billed_units"`
	} `json:"meta"`
}

// Rerank calculates similarity scores between query and documents
func (s *SiliconflowModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	if len(documents) == 0 {
		return &RerankResponse{}, nil
	}

	var region = "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	apiKey := ""
	if apiConfig != nil && apiConfig.ApiKey != nil {
		apiKey = *apiConfig.ApiKey
	}

	var topN = rerankConfig.TopN
	if rerankConfig.TopN == 0 {
		topN = len(documents)
	}

	reqBody := SiliconflowRerankRequest{
		Model:           *modelName,
		Query:           query,
		Documents:       documents,
		TopN:            topN,
		ReturnDocuments: false,
		MaxChunksPerDoc: 1024,
		OverlapTokens:   80,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(s.BaseURL[region], "/"), s.URLSuffix.Rerank)

	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("SiliconFlow Rerank API error: %s, body: %s", resp.Status, string(body))
	}

	body, _ := io.ReadAll(resp.Body)

	var siliconflowRerankResp SiliconflowRerankResponse
	if err = json.Unmarshal(body, &siliconflowRerankResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var rerankResponse RerankResponse
	for _, result := range siliconflowRerankResp.Results {
		rerankResponse.Data = append(rerankResponse.Data, RerankResult{
			Index:          result.Index,
			RelevanceScore: result.RelevanceScore,
		})
	}
	return &rerankResponse, nil
}

// TranscribeAudio transcribe audio
func (o *SiliconflowModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", o.Name())
}

func (z *SiliconflowModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", z.Name())
}

// AudioSpeech convert audio to text
func (o *SiliconflowModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, asrConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", o.Name())
}

func (z *SiliconflowModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", z.Name())
}

// OCRFile OCR file
func (m *SiliconflowModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}
