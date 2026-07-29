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
	"ragflow/internal/common"
	"strings"
	"time"
)

// GiteeModel implements ModelDriver for Gitee
type GiteeModel struct {
	baseModel BaseModel
}

// NewGiteeModel creates a new Gitee model instance
func NewGiteeModel(baseURL map[string]string, urlSuffix URLSuffix) *GiteeModel {
	return &GiteeModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(),
		},
	}
}

func (g *GiteeModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewGiteeModel(baseURL, g.baseModel.URLSuffix)
}

func (g *GiteeModel) Name() string {
	return "GiteeAI"
}

// GiteeChatResponse mirrors GiteeAI's OpenAI-compatible chat response.
type GiteeChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Index        int    `json:"index"`
		Logprobs     any    `json:"logprobs"`
		Message      struct {
			Content          string           `json:"content"`
			ReasoningContent string           `json:"reasoning_content"`
			Role             string           `json:"role"`
			ToolCalls        []map[string]any `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	SystemFingerprint string `json:"system_fingerprint"`
	Usage             struct {
		CompletionTokens    int `json:"completion_tokens"`
		PromptTokens        int `json:"prompt_tokens"`
		TotalTokens         int `json:"total_tokens"`
		PromptTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}

// ChatWithMessages sends multiple messages with roles and returns response
func (g *GiteeModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := g.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, g.baseModel.URLSuffix.Chat)
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, false)

	if chatModelConfig != nil {
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

	common.Info(fmt.Sprintf("GiteeAPI request body: %s", string(jsonData)))

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := g.baseModel.httpClient.Do(req)
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

	return parseChatCompletionResponse(body, chatModelConfig, modelUsage, func(body []byte, chatConfig *ChatConfig) (chatResponseParts, error) {
		var result GiteeChatResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return chatResponseParts{}, fmt.Errorf("failed to parse response: %w", err)
		}
		if len(result.Choices) == 0 {
			return chatResponseParts{}, fmt.Errorf("no choices in response")
		}

		choice := result.Choices[0]
		content := choice.Message.Content
		if content == "" && len(choice.Message.ToolCalls) == 0 {
			return chatResponseParts{}, fmt.Errorf("response contains neither content nor tool calls")
		}

		reasonContent := ""
		if chatConfig != nil && chatConfig.Thinking != nil && *chatConfig.Thinking {
			reasonContent = choice.Message.ReasoningContent
			if reasonContent == "" {
				reasoning, answer := GetThinkingAndAnswer(chatConfig.ModelClass, &content)
				if reasoning != nil {
					reasonContent = *reasoning
					content = *answer
				}
			}
			if strings.HasPrefix(reasonContent, "\n") {
				reasonContent = reasonContent[1:]
			}
		}

		return chatResponseParts{
			RequestID:     result.ID,
			Content:       &content,
			ReasonContent: &reasonContent,
			ToolCalls:     choice.Message.ToolCalls,
			Usage: &TokenUsage{
				PromptTokens:     result.Usage.PromptTokens,
				CompletionTokens: result.Usage.CompletionTokens,
				TotalTokens:      result.Usage.TotalTokens,
			},
		}, nil
	})
}

// ChatStreamlyWithSender sends messages and streams response via sender function (best performance, no channel)
func (g *GiteeModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}
	if chatModelConfig != nil {
		chatModelConfig.ToolCallsResult = nil
		chatModelConfig.UsageResult = nil
	}

	resolvedBaseURL, err := g.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(resolvedBaseURL, "/"), g.baseModel.URLSuffix.Chat)
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, true)
	reqBody["stream_options"] = map[string]any{"include_usage": true}

	if chatModelConfig != nil {
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

	ctx, cancel := context.WithTimeout(ctx, streamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := g.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	reserveText := ""
	thinkingPhase := false
	answerPhase := false
	sawTerminal := false

	accumulatedToolCalls := make(map[int]map[string]any)
	done, err := ParseSSEStream[map[string]any](resp.Body, func(event map[string]any) error {
		common.Info(fmt.Sprintf("%v", event))

		tokenUsage, found, usageErr := decodeOpenAICompatibleStreamUsage(event)
		if usageErr != nil {
			return usageErr
		}
		if found {
			applyStreamUsage(chatModelConfig, modelUsage, tokenUsage)
		}
		choices, ok := event["choices"].([]any)
		if !ok || len(choices) == 0 {
			return nil
		}
		choice, ok := choices[0].(map[string]any)
		if !ok {
			return nil
		}
		if finishReason, ok := choice["finish_reason"].(string); ok && finishReason != "" {
			sawTerminal = true
		}
		delta, ok := choice["delta"].(map[string]any)
		if !ok {
			return nil
		}

		accumulateToolCallDeltas(delta, accumulatedToolCalls)
		if reasoning, ok := delta["reasoning_content"].(string); ok && reasoning != "" {
			if err := sender(nil, &reasoning); err != nil {
				return err
			}
		}

		content, ok := delta["content"].(string)
		if ok && content != "" {
			common.Info(content)
			if content == "<think>" {
				thinkingPhase = true
				return nil

			} else if content == "</think>" {
				thinkingPhase = false
				answerPhase = true
				return nil
			}

			if thinkingPhase {
				if err = sender(nil, &content); err != nil {
					return err
				}
				reserveText = ""
			} else if answerPhase {
				if err = sender(&content, nil); err != nil {
					return err
				}
				reserveText = ""
			} else {
				content = strings.Trim(content, "\n")
				content = strings.Trim(content, " ")
				if content != "" {
					reserveText += content
				}
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}
	if !done && !sawTerminal {
		return fmt.Errorf("gitee: stream ended before [DONE] or finish_reason")
	}

	if reserveText != "" {
		if err = sender(&reserveText, nil); err != nil {
			return err
		}
	}

	setSortedToolCallsResult(chatModelConfig, accumulatedToolCalls)

	// Send [DONE] marker for OpenAI compatibility
	endOfStream := "[DONE]"
	if err = sender(&endOfStream, nil); err != nil {
		return err
	}

	return nil
}

// GiteeEmbeddingResponse mirrors GiteeAI's embeddings response.
type GiteeEmbeddingResponse struct {
	ID     string `json:"id"`
	Object string `json:"object"`
	Model  string `json:"model"`
	Data   []struct {
		Object    string    `json:"object"`
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// Embed embeds a list of texts into embeddings
func (g *GiteeModel) Embed(ctx context.Context, modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(texts) == 0 {
		return []EmbeddingData{}, nil
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	resolvedBaseURL, err := g.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL := resolvedBaseURL

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), g.baseModel.URLSuffix.Embedding)

	reqBody := map[string]interface{}{
		"model": *modelName,
		"input": texts,
	}
	if embeddingConfig != nil && embeddingConfig.Dimension > 0 {
		reqBody["dimensions"] = embeddingConfig.Dimension
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
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := g.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Gitee embeddings API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed GiteeEmbeddingResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	recordResponseUsage(modelUsage, parsed.ID, &TokenUsage{
		PromptTokens:     parsed.Usage.PromptTokens,
		CompletionTokens: parsed.Usage.CompletionTokens,
		TotalTokens:      parsed.Usage.TotalTokens,
	}, "embedding")

	var embeddings []EmbeddingData
	for _, dataElem := range parsed.Data {
		var embeddingData EmbeddingData
		embeddingData.Embedding = dataElem.Embedding
		embeddingData.Index = dataElem.Index
		embeddings = append(embeddings, embeddingData)
	}

	return embeddings, nil
}

type giteeRerankRequest struct {
	Model           string   `json:"model"`
	Query           string   `json:"query"`
	Documents       []string `json:"documents"`
	TopN            int      `json:"top_n"`
	ReturnDocuments bool     `json:"return_documents"`
}

// GiteeRerankResponse mirrors GiteeAI's rerank response.
type GiteeRerankResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Results []struct {
		Index    int `json:"index"`
		Document struct {
			Text string `json:"text"`
		} `json:"document"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// Rerank calculates similarity scores between query and documents
func (g *GiteeModel) Rerank(ctx context.Context, modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(documents) == 0 {
		return &RerankResponse{}, nil
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	resolvedBaseURL, err := g.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL := resolvedBaseURL

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), g.baseModel.URLSuffix.Rerank)

	topN := len(documents)
	if rerankConfig != nil && rerankConfig.TopN > 0 {
		topN = rerankConfig.TopN
	}

	reqBody := giteeRerankRequest{
		Model:           *modelName,
		Query:           query,
		Documents:       documents,
		TopN:            topN,
		ReturnDocuments: false,
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
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := g.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitee rerank API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed GiteeRerankResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	recordResponseUsage(modelUsage, parsed.ID, &TokenUsage{
		PromptTokens:     parsed.Usage.PromptTokens,
		CompletionTokens: parsed.Usage.CompletionTokens,
		TotalTokens:      parsed.Usage.TotalTokens,
	}, "rerank")

	rerankResponse := RerankResponse{Data: make([]RerankResult, 0, len(parsed.Results))}
	for _, result := range parsed.Results {
		rerankResponse.Data = append(rerankResponse.Data, RerankResult{
			Index:          result.Index,
			RelevanceScore: result.RelevanceScore,
		})
	}

	return &rerankResponse, nil
}

// TranscribeAudio transcribe audio
func (g *GiteeModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GiteeModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", g.Name())
}

// AudioSpeech convert text to audio
func (g *GiteeModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GiteeModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", g.Name())
}

type giteeOCRResponse struct {
	Text   string `json:"text_result"`
	Prompt string `json:"prompt"`
}

// OCRFile OCR file
func (g *GiteeModel) OCRFile(ctx context.Context, modelName *string, content []byte, imageURL *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if imageURL == nil && content == nil {
		return nil, fmt.Errorf("url or content is required")
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	resolvedBaseURL, err := g.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL := resolvedBaseURL

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), g.baseModel.URLSuffix.OCR)

	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)

	if err := writer.WriteField("model", *modelName); err != nil {
		return nil, fmt.Errorf("failed to write model field: %w", err)
	}

	if imageURL != nil {
		if err := writer.WriteField("image", *imageURL); err != nil {
			return nil, fmt.Errorf("failed to write image URL: %w", err)
		}
	} else if content != nil && len(content) > 0 {
		part, err := writer.CreateFormFile("image", "image")
		if err != nil {
			return nil, fmt.Errorf("failed to create image form file: %w", err)
		}
		if _, err = part.Write(content); err != nil {
			return nil, fmt.Errorf("failed to write image content: %w", err)
		}
	} else {
		return nil, fmt.Errorf("image or image URL is required")
	}

	writer.Close()

	ctx, cancel := context.WithTimeout(ctx, longOpCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := g.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitee OCR API error: %s, body: %s", resp.Status, string(body))
	}

	var giteeResponse giteeOCRResponse
	if err = json.Unmarshal(body, &giteeResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var ocrResponse = OCRFileResponse{
		Text: &giteeResponse.Text,
	}

	return &ocrResponse, nil
}

type giteeParseFileResponse struct {
	TaskID    string    `json:"task_id"`
	Status    string    `json:"status"`
	CreatedAt int64     `json:"created_at"`
	URLs      giteeURLs `json:"urls"`
}

type giteeURLs struct {
	Get    string `json:"get"`
	Cancel string `json:"cancel"`
}

// ParseFile parse file
func (g *GiteeModel) ParseFile(ctx context.Context, modelName *string, content []byte, documentURL *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if documentURL == nil && content == nil {
		return nil, fmt.Errorf("url or content is required")
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	resolvedBaseURL, err := g.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL := resolvedBaseURL

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), g.baseModel.URLSuffix.DocumentParse)

	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)

	if err := writer.WriteField("model", *modelName); err != nil {
		return nil, fmt.Errorf("failed to write model field: %w", err)
	}

	if documentURL != nil {
		if err := writer.WriteField("file", *documentURL); err != nil {
			return nil, fmt.Errorf("failed to write file URL: %w", err)
		}
	} else if content != nil && len(content) > 0 {
		part, err := writer.CreateFormFile("file", "file")
		if err != nil {
			return nil, fmt.Errorf("failed to create file form file: %w", err)
		}
		if _, err = part.Write(content); err != nil {
			return nil, fmt.Errorf("failed to write file content: %w", err)
		}
	} else {
		return nil, fmt.Errorf("file or file URL is required")
	}

	writer.Close()

	ctx, cancel := context.WithTimeout(ctx, longOpCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := g.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitee OCR API error: %s, body: %s", resp.Status, string(body))
	}

	var giteeParseFileResp giteeParseFileResponse
	if err = json.Unmarshal(body, &giteeParseFileResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	_, err = g.getParseFile(ctx, &baseURL, apiConfig.ApiKey, &giteeParseFileResp.TaskID, 5*time.Second, 10)
	if err != nil {
		return nil, err
	}

	var parseFileResponse = ParseFileResponse{}

	return &parseFileResponse, nil
}

type giteeGetParseFileResponse struct {
}

func (g *GiteeModel) getParseFile(ctx context.Context, baseURL *string, apiKey, taskID *string, timeOut time.Duration, count int) (*giteeGetParseFileResponse, error) {
	url := fmt.Sprintf("%s/task/%s/status", strings.TrimSuffix(*baseURL, "/"), *taskID)

	reqBody := map[string]interface{}{}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, longOpCallTimeout)
	defer cancel()

	for i := 0; i < count; i++ {
		req, err := http.NewRequestWithContext(ctx, "GET", url, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiKey))

		resp, err := g.baseModel.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to send request: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
		}

		// Parse response
		var result map[string]interface{}
		if err = json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		// Wait before the next poll, but honor context cancellation /
		// longOpCallTimeout instead of blocking uninterruptibly.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(timeOut):
		}
	}

	// if resp show the file is ok, download it. otherwise, provide timeout info
	return nil, nil
}

func (g *GiteeModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {

	resolvedBaseURL, err := g.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, g.baseModel.URLSuffix.Models)

	// Build request body
	reqBody := map[string]interface{}{}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := g.baseModel.httpClient.Do(req)
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
	var modelList ModelList
	if err = json.Unmarshal(body, &modelList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return ParseListModel(modelList), nil
}

func (g *GiteeModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	baseURL, err := g.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/%s", baseURL, g.baseModel.URLSuffix.Balance)

	// Build request body
	reqBody := map[string]interface{}{}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := g.baseModel.httpClient.Do(req)
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
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	balance := result["balance"].(float64)

	var response = map[string]interface{}{
		"balance":  balance,
		"currency": "CNY",
	}

	return response, nil
}

func (g *GiteeModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	resolvedBaseURL, err := g.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, g.baseModel.URLSuffix.Status)

	// Build request body
	reqBody := map[string]interface{}{}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := g.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

type giteeTaskListResponse struct {
	Total int             `json:"total"`
	Items []giteeTaskItem `json:"items"`
}

type giteeTaskItem struct {
	TaskID string `json:"task_id"`
	//Output      giteeTaskOutput `json:"output"`
	Status      string        `json:"status"`
	CreatedAt   int64         `json:"created_at"`
	StartedAt   int64         `json:"started_at,omitempty"`
	CompletedAt int64         `json:"completed_at,omitempty"`
	Price       float64       `json:"price"`
	Currency    string        `json:"currency"`
	URLs        giteeTaskURLs `json:"urls"`
}

type giteeTaskOutput struct {
	Segments []giteeSegment `json:"segments"`
}

type giteeSegment struct {
	Index   int    `json:"index"`
	Content string `json:"content"`
}
type giteeTaskURLs struct {
	Get    string `json:"get"`
	Cancel string `json:"cancel"`
}

func (g *GiteeModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	resolvedBaseURL, err := g.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, g.baseModel.URLSuffix.Tasks)

	// Build request body
	reqBody := map[string]interface{}{}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := g.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	var body []byte
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var giteeTaskList giteeTaskListResponse
	if err = json.Unmarshal(body, &giteeTaskList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	taskListResp := []ListTaskStatus{}
	for _, item := range giteeTaskList.Items {
		taskListResp = append(taskListResp, ListTaskStatus{
			TaskID: item.TaskID,
			Status: item.Status,
		})
	}
	return taskListResp, nil
}

func (g *GiteeModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	resolvedBaseURL, err := g.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s/%s/get", resolvedBaseURL, g.baseModel.URLSuffix.Task, taskID)

	// Build request body
	reqBody := map[string]interface{}{}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := g.baseModel.httpClient.Do(req)
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

	var taskOutput giteeTaskOutput
	if err = json.Unmarshal(body, &taskOutput); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	taskResp := &TaskResponse{}

	for _, segment := range taskOutput.Segments {
		taskResp.Segments = append(taskResp.Segments, TaskSegment{
			Index:   segment.Index,
			Content: segment.Content,
		})
	}

	return taskResp, nil
}
