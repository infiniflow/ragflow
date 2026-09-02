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
			httpClient: NewDriverHTTPClient(false),
		},
	}
}

func (c *CoHereModel) Name() string {
	return "Cohere"
}

type CohereChatResponse struct {
	ID           string `json:"id"`
	FinishReason string `json:"finish_reason"`
	Message      struct {
		Role      string           `json:"role"`
		ToolCalls []map[string]any `json:"tool_calls"`

		Content []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Thinking string `json:"thinking"`
		} `json:"content"`
	} `json:"message"`
	Usage struct {
		Tokens struct {
			InputTokens  float64 `json:"input_tokens"`
			OutputTokens float64 `json:"output_tokens"`
		} `json:"tokens"`
	} `json:"usage"`
}

func buildCohereTokenUsage(inputTokens, outputTokens float64) *TokenUsage {
	input := int(inputTokens)
	output := int(outputTokens)
	return &TokenUsage{
		PromptTokens:     input,
		CompletionTokens: output,
		TotalTokens:      input + output,
	}
}

func decodeCohereStreamUsage(event map[string]interface{}) (*TokenUsage, bool, error) {
	eventType, _ := event["type"].(string)
	if eventType != "message-end" {
		return nil, false, nil
	}
	var parsed struct {
		Delta struct {
			Usage struct {
				Tokens struct {
					InputTokens  float64 `json:"input_tokens"`
					OutputTokens float64 `json:"output_tokens"`
				} `json:"tokens"`
			} `json:"usage"`
		} `json:"delta"`
		Usage struct {
			Tokens struct {
				InputTokens  float64 `json:"input_tokens"`
				OutputTokens float64 `json:"output_tokens"`
			} `json:"tokens"`
		} `json:"usage"`
		Response struct {
			Usage struct {
				Tokens struct {
					InputTokens  float64 `json:"input_tokens"`
					OutputTokens float64 `json:"output_tokens"`
				} `json:"tokens"`
			} `json:"usage"`
		} `json:"response"`
	}
	body, err := json.Marshal(event)
	if err != nil {
		return nil, false, err
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, false, err
	}
	tokens := parsed.Response.Usage.Tokens
	if tokens.InputTokens == 0 && tokens.OutputTokens == 0 {
		tokens = parsed.Delta.Usage.Tokens
	}
	if tokens.InputTokens == 0 && tokens.OutputTokens == 0 {
		tokens = parsed.Usage.Tokens
	}
	if tokens.InputTokens == 0 && tokens.OutputTokens == 0 {
		return nil, false, nil
	}
	return buildCohereTokenUsage(tokens.InputTokens, tokens.OutputTokens), true, nil
}

func (c *CoHereModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
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

	// Build request body
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, false)

	if chatModelConfig != nil {
		if chatModelConfig.TopP != nil {
			reqBody["p"] = *chatModelConfig.TopP
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

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
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
		return nil, fmt.Errorf("cohere chat API error: %d %s", resp.StatusCode, string(body))
	}

	return parseChatCompletionResponse(body, chatModelConfig, modelUsage, func(body []byte, chatConfig *ChatConfig) (chatResponseParts, error) {
		var result CohereChatResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return chatResponseParts{}, fmt.Errorf("failed to unmarshal response: %w", err)
		}
		if len(result.Message.Content) == 0 && len(result.Message.ToolCalls) == 0 {
			return chatResponseParts{}, fmt.Errorf("content is not an array in Cohere response")
		}

		var fullContent string
		var reasonContent string
		for _, block := range result.Message.Content {
			if block.Type == "thinking" {
				reasonContent += block.Thinking
			} else {
				fullContent += block.Text
			}
		}

		return chatResponseParts{
			RequestID:     result.ID,
			Content:       &fullContent,
			ReasonContent: &reasonContent,
			ToolCalls:     result.Message.ToolCalls,
			Usage:         buildCohereTokenUsage(result.Usage.Tokens.InputTokens, result.Usage.Tokens.OutputTokens),
		}, nil
	})
}

func (c *CoHereModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
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

	reqBody := buildRequestBody(modelConfig, modelName, messages, true)

	if modelConfig != nil {
		if modelConfig.TopP != nil {
			reqBody["p"] = *modelConfig.TopP
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

	ctx, cancel := context.WithTimeout(ctx, streamCallTimeout)
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
		return fmt.Errorf("cohere stream API error %d: %s", resp.StatusCode, string(body))
	}

	sawTerminal := false
	accumulatedToolCalls := make(map[int]map[string]any)
	done, err := ParseSSEStream[map[string]interface{}](resp.Body, func(event map[string]interface{}) error {
		eventType, ok := event["type"].(string)
		if !ok {
			return nil
		}

		tokenUsage, found, usageErr := decodeCohereStreamUsage(event)
		if usageErr != nil {
			return usageErr
		}
		if found {
			applyStreamUsage(modelConfig, modelUsage, tokenUsage)
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

			accumulateToolCallDeltas(delta, accumulatedToolCalls)

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
		return fmt.Errorf("cohere: stream ended before [DONE] or finish_reason")
	}

	setSortedToolCallsResult(modelConfig, accumulatedToolCalls)

	endOfStream := "[DONE]"
	return sender(&endOfStream, nil)
}

func (c *CoHereModel) Embed(ctx context.Context, modelName *string, request EmbedRequest, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	if err := c.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(request.Texts) == 0 {
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
		"texts":           request.Texts,
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

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
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
		return nil, fmt.Errorf("cohere embedding API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result struct {
		ID         string `json:"id"`
		Embeddings struct {
			Float [][]float64 `json:"float"`
		} `json:"embeddings"`
		Meta struct {
			Tokens struct {
				InputTokens  float64 `json:"input_tokens"`
				OutputTokens float64 `json:"output_tokens"`
			}
		} `json:"meta"`
	}
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Embeddings.Float) == 0 {
		return nil, fmt.Errorf("cohere embedding response contains no float data: %s", string(body))
	}

	var embeddings []EmbeddingData
	for i, floatArr := range result.Embeddings.Float {
		embeddings = append(embeddings, EmbeddingData{
			Embedding: floatArr,
			Index:     i,
		})
	}
	recordResponseUsage(modelUsage, result.ID, buildCohereTokenUsage(result.Meta.Tokens.InputTokens, result.Meta.Tokens.OutputTokens), "embedding")

	return embeddings, nil
}

func (c *CoHereModel) Rerank(ctx context.Context, modelName *string, request RerankRequest, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	if err := c.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	documents := request.Documents
	query := request.Query
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

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
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
		return nil, fmt.Errorf("cohere rerank API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var rerankResp struct {
		ID      string `json:"id"`
		Results []struct {
			Index          int     `json:"index"`
			RelevanceScore float64 `json:"relevance_score"`
		} `json:"results"`
		Meta struct {
			Tokens struct {
				InputTokens  float64 `json:"input_tokens"`
				OutputTokens float64 `json:"output_tokens"`
			}
		} `json:"meta"`
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
	recordResponseUsage(modelUsage, rerankResp.ID, buildCohereTokenUsage(rerankResp.Meta.Tokens.InputTokens, rerankResp.Meta.Tokens.OutputTokens), "rerank")

	return &rerankResponse, nil
}

// TranscribeAudio transcribe audio
func (c *CoHereModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
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
	ctx, cancel := context.WithTimeout(ctx, longOpCallTimeout)
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
		return nil, fmt.Errorf("cohere ASR API error: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Text string `json:"text"`
	}

	if err = json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &ASRResponse{Text: result.Text}, nil
}

func (c *CoHereModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", c.Name())
}

// AudioSpeech convert text to audio
func (c *CoHereModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", c.Name())
}

func (c *CoHereModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", c.Name())
}

// OCRFile OCR file
func (c *CoHereModel) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", c.Name())
}

// ParseFile parse file
func (c *CoHereModel) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", c.Name())
}

func (c *CoHereModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := c.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	resolvedBaseURL, err := c.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, c.baseModel.URLSuffix.Models)

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
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
		return nil, fmt.Errorf("cohere API request failed with status %d: %s", resp.StatusCode, string(body))
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

	models := make([]ModelListItem, 0, len(result.Models))
	for _, model := range result.Models {
		if model.ModelName != "" {
			models = append(models, ModelListItem{
				ID:      model.ModelName,
				OwnedBy: c.Name(),
			})
		}
	}

	return ParseListModel(ModelList{Models: models}), nil
}

func (c *CoHereModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", c.Name())
}

func (c *CoHereModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := c.ListModels(ctx, apiConfig)
	return err
}

func (c *CoHereModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", c.Name())
}

func (c *CoHereModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", c.Name())
}
