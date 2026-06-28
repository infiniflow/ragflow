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
	"strconv"
	"strings"
)

type CoHereModel struct {
	baseModel BaseModel
}

func (c *CoHereModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewCoHereModel(baseURL, c.baseModel.URLSuffix)
}

func NewCoHereModel(baseURL map[string]string, urlSuffix URLSuffix) *CoHereModel {
	return &CoHereModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(),
		},
	}
}

func (c *CoHereModel) Name() string {
	return "cohere"
}

func (c *CoHereModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if err := c.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := c.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, c.baseModel.URLSuffix.Chat)

	// Convert messages to API format
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
		"temperature": 0.3,
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
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("content-Type", "application/json")
	req.Header.Set("accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("bearer %s", strings.TrimSpace(*apiConfig.ApiKey)))

	resp, err := c.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Cohere chat API error: %d %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	messageMap, ok := result["message"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no message found in Cohere response: %s", string(body))
	}

	contentArray, ok := messageMap["content"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("content is not an array in Cohere response")
	}

	var fullContent string
	var reasonContent string
	for _, cBlock := range contentArray {
		cmap, ok := cBlock.(map[string]interface{})
		if !ok {
			continue
		}
		if blockType, ok := cmap["type"].(string); ok && blockType == "thinking" {
			if thinkingText, ok := cmap["thinking"].(string); ok {
				reasonContent += thinkingText
			}
		} else if text, ok := cmap["text"].(string); ok {
			fullContent += text
		}
	}

	chatResponse := &ChatResponse{
		Answer:        &fullContent,
		ReasonContent: &reasonContent,
	}

	return chatResponse, nil
}

func (c *CoHereModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error {
	if err := c.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := c.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, c.baseModel.URLSuffix.Chat)

	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	reqBody := map[string]interface{}{
		"model":       modelName,
		"messages":    apiMessages,
		"stream":      true,
		"temperature": 1,
	}

	if modelConfig != nil {
		if modelConfig.MaxTokens != nil {
			reqBody["max_tokens"] = *modelConfig.MaxTokens
		}
		if modelConfig.Temperature != nil {
			reqBody["temperature"] = *modelConfig.Temperature
		}
		if modelConfig.TopP != nil {
			reqBody["p"] = *modelConfig.TopP
		}
	}

	if modelConfig != nil {
		if modelConfig.Stream != nil {
			reqBody["stream"] = *modelConfig.Stream
		}

		if modelConfig.MaxTokens != nil {
			reqBody["max_tokens"] = *modelConfig.MaxTokens
		}

		if modelConfig.Temperature != nil {
			reqBody["temperature"] = *modelConfig.Temperature
		}

		if modelConfig.TopP != nil {
			reqBody["top_p"] = *modelConfig.TopP
		}

		if modelConfig.Thinking != nil {
			if *modelConfig.Thinking {
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

	ctx, cancel := context.WithTimeout(context.Background(), streamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", strings.TrimSpace(*apiConfig.ApiKey)))

	resp, err := c.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Cohere stream API error %d: %s", resp.StatusCode, string(body))
	}

	sawTerminal := false
	done, err := ParseSSEStream[map[string]interface{}](resp.Body, func(event map[string]interface{}) error {
		eventType, ok := event["type"].(string)
		if !ok {
			return nil
		}

		if eventType == "message-end" {
			sawTerminal = true
			return nil
		}

		if eventType == "content-delta" {
			delta, ok := event["delta"].(map[string]interface{})
			if !ok {
				return nil
			}
			msg, ok := delta["message"].(map[string]interface{})
			if !ok {
				return nil
			}
			content, ok := msg["content"].(map[string]interface{})
			if !ok {
				return nil
			}

			if thinking, ok := content["thinking"].(string); ok && thinking != "" {
				if err := sender(nil, &thinking); err != nil {
					return err
				}
			}

			if text, ok := content["text"].(string); ok && text != "" {
				if err := sender(&text, nil); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}
	if !done && !sawTerminal {
		return fmt.Errorf("Cohere: stream ended before [DONE] or finish_reason")
	}

	endOfStream := "[DONE]"
	return sender(&endOfStream, nil)
}

func (c *CoHereModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	if err := c.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(texts) == 0 {
		return []EmbeddingData{}, nil
	}

	resolvedBaseURL, err := c.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL := strings.TrimSuffix(resolvedBaseURL, "/")
	suffix := strings.TrimPrefix(c.baseModel.URLSuffix.Embedding, "/")
	url := fmt.Sprintf("%s/%s", baseURL, suffix)

	reqBody := map[string]interface{}{
		"model":           *modelName,
		"texts":           texts,
		"input_type":      "search_document",
		"embedding_types": []string{"float"},
	}
	// This is only available for embed-v4 and newer models. Possible values are 256, 512, 1024, and 1536. The default is 1536.
	if embeddingConfig != nil && embeddingConfig.Dimension > 0 {
		reqBody["output_dimension"] = embeddingConfig.Dimension
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", strings.TrimSpace(*apiConfig.ApiKey)))

	resp, err := c.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Cohere embedding API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Embeddings struct {
			Float [][]float64 `json:"float"`
		} `json:"embeddings"`
	}
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Embeddings.Float) == 0 {
		return nil, fmt.Errorf("Cohere embedding response contains no float data: %s", string(body))
	}

	var embeddings []EmbeddingData
	for i, floatArr := range result.Embeddings.Float {
		embeddings = append(embeddings, EmbeddingData{
			Embedding: floatArr,
			Index:     i,
		})
	}

	return embeddings, nil
}

func (c *CoHereModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	if err := c.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(documents) == 0 {
		return &RerankResponse{}, nil
	}

	resolvedBaseURL, err := c.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL := strings.TrimSuffix(resolvedBaseURL, "/")
	suffix := strings.TrimPrefix(c.baseModel.URLSuffix.Rerank, "/")
	url := fmt.Sprintf("%s/%s", baseURL, suffix)

	var topN = rerankConfig.TopN
	if rerankConfig.TopN == 0 {
		topN = len(documents)
	}

	reqBody := map[string]interface{}{
		"model":     *modelName,
		"query":     query,
		"documents": documents,
		"top_n":     topN,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", strings.TrimSpace(*apiConfig.ApiKey)))

	resp, err := c.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Cohere rerank API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var rerankResp struct {
		Results []struct {
			Index          int     `json:"index"`
			RelevanceScore float64 `json:"relevance_score"`
		} `json:"results"`
	}

	if err := json.Unmarshal(body, &rerankResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var rerankResponse RerankResponse
	for _, result := range rerankResp.Results {
		rerankResult := RerankResult{
			Index:          result.Index,
			RelevanceScore: result.RelevanceScore,
		}
		rerankResponse.Data = append(rerankResponse.Data, rerankResult)
	}

	return &rerankResponse, nil
}

// TranscribeAudio transcribe audio
func (c *CoHereModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	if err := c.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if file == nil || *file == "" {
		return nil, fmt.Errorf("file is missing")
	}

	resolvedBaseURL, err := c.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, c.baseModel.URLSuffix.ASR)

	// multipart body

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// open audio file

	// codeql[go/path-injection] False positive: *file is the audio file path the caller passes in to upload. The user (or operator-supplied pipeline) explicitly chose this path, and the OS access check enforces permissions anyway.
	audioFile, err := os.Open(*file)
	if err != nil {
		return nil, fmt.Errorf("failed to open audio file: %w", err)
	}
	defer audioFile.Close()

	// create multipart file field

	if err = writer.WriteField("model", *modelName); err != nil {
		return nil, fmt.Errorf("failed to write model name: %w", err)
	}
	// extra params
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

	// all form fields (model, language) must appear before the file part in the multipart body
	part, err := writer.CreateFormFile("file", filepath.Base(*file))
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := io.Copy(part, audioFile); err != nil {
		return nil, fmt.Errorf("failed to copy audio file: %w", err)
	}

	if err = writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close writer: %w", err)
	}

	// build request
	ctx, cancel := context.WithTimeout(context.Background(), longOpCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, &body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Cohere ASR API error: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Text string `json:"text"`
	}

	if err = json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &ASRResponse{Text: result.Text}, nil
}

func (c *CoHereModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", c.Name())
}

// AudioSpeech convert text to audio
func (c *CoHereModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", c.Name())
}

func (c *CoHereModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", c.Name())
}

// OCRFile OCR file
func (c *CoHereModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", c.Name())
}

// ParseFile parse file
func (c *CoHereModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", c.Name())
}

func (c *CoHereModel) ListModels(apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := c.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	resolvedBaseURL, err := c.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, c.baseModel.URLSuffix.Models)

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("bearer %s", strings.TrimSpace(*apiConfig.ApiKey)))

	resp, err := c.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Cohere API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result struct {
		Models []struct {
			ModelName string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	models := make([]DSModel, 0, len(result.Models))
	for _, model := range result.Models {
		if model.ModelName != "" {
			models = append(models, DSModel{
				ID:      model.ModelName,
				OwnedBy: c.Name(),
			})
		}
	}

	return ParseListModel(ModelList{Models: models}), nil
}

func (c *CoHereModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", c.Name())
}

func (c *CoHereModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := c.ListModels(apiConfig)
	return err
}

func (c *CoHereModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", c.Name())
}

func (c *CoHereModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", c.Name())
}
