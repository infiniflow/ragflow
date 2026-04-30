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
	"ragflow/internal/logger"
	"strings"
	"time"
)

// VolcEngine implements ModelDriver for VolcEngine
type VolcEngine struct {
	BaseURL    map[string]string
	URLSuffix  URLSuffix
	httpClient *http.Client // Reusable HTTP client with connection pool
}

// NewVolcEngine creates a new VolcEngine model instance
func NewVolcEngine(baseURL map[string]string, urlSuffix URLSuffix) *VolcEngine {
	return &VolcEngine{
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

func (z *VolcEngine) NewInstance(baseURL map[string]string) ModelDriver {
	return nil
}

func (z *VolcEngine) Name() string {
	return "volcengine"
}

// Chat sends a message and returns response
func (z *VolcEngine) Chat(modelName, message *string, apiConfig *APIConfig, modelConfig *ChatConfig) (*ChatResponse, error) {
	if message == nil {
		return nil, fmt.Errorf("message is nil")
	}

	var region = "default"
	if apiConfig.Region != nil {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/%s", z.BaseURL[region], z.URLSuffix.Chat)

	//Build request body
	reqBody := map[string]interface{}{
		"model": modelName,
		"messages": []map[string]string{
			{"role": "user", "content": *message},
		},
		"stream":      false,
		"temperature": 1,
	}

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
	// TODO VolcEngine has `auto` mode
	if modelConfig.Thinking != nil {
		if *modelConfig.Thinking {
			var thinkingFlag string
			switch *modelConfig.Effort {
			case "none", "minimal":
				thinkingFlag = "disabled"
				reqBody["reasoning_effort"] = "minimal"
				break
			case "low":
				thinkingFlag = "enabled"
				reqBody["reasoning_effort"] = "low"
				break
			case "medium":
				thinkingFlag = "enabled"
				reqBody["reasoning_effort"] = "medium"
				break
			case "auto", "default":
				thinkingFlag = "enabled"
				reqBody["reasoning_effort"] = "medium"
				break
			case "high":
				thinkingFlag = "enabled"
				reqBody["reasoning_effort"] = "high"
				break
			default:
				return nil, fmt.Errorf("invalid effort level")
			}
			reqBody["thinking"] = map[string]interface{}{
				"type": thinkingFlag,
			}
		} else {
			reqBody["thinking"] = map[string]interface{}{
				"type": "disabled",
			}
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

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result map[string]interface{}
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return nil, fmt.Errorf("no choices in responses")
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
	if modelConfig.Thinking != nil && *modelConfig.Thinking {
		reasonContent, ok = messageMap["reasoning_content"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid reasonContent format")
		}
		// if first char of reasonContent is \n remove the \n
		if reasonContent != "" && reasonContent[0] == '\n' {
			reasonContent = reasonContent[1:]
		}
	}

	chatResponse := &ChatResponse{
		Answer:        &content,
		ReasonContent: &reasonContent,
	}

	return chatResponse, nil
}

// ChatWithMessages sends multiple messages with roles and returns response
func (z *VolcEngine) ChatWithMessages(modelName string, apiConfig *APIConfig, messages []Message, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	var region = "default"
	if apiConfig.Region != nil {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/%s", z.BaseURL[region], z.URLSuffix.Chat)

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
		"messages":     apiMessages,
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

		if chatModelConfig.Thinking != nil {
			if *chatModelConfig.Thinking {
				var thinkingFlag string
				switch *chatModelConfig.Effort {
				case "none", "minimal":
					thinkingFlag = "disabled"
					reqBody["reasoning_effort"] = "minimal"
				case "low":
					thinkingFlag = "enabled"
					reqBody["reasoning_effort"] = "low"
				case "medium":
					thinkingFlag = "enabled"
					reqBody["reasoning_effort"] = "medium"
				case "auto", "default":
					thinkingFlag = "enabled"
					reqBody["reasoning_effort"] = "medium"
				case "high":
					thinkingFlag = "enabled"
					reqBody["reasoning_effort"] = "high"
				default:
					return nil, fmt.Errorf("invalid effort level")
				}
				reqBody["thinking"] = map[string]interface{}{
					"type": thinkingFlag,
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
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
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
			return nil, fmt.Errorf("invalid reasonContent format")
		}
		if reasonContent != "" && reasonContent[0] == '\n' {
			reasonContent = reasonContent[1:]
		}
	}

	chatResponse := &ChatResponse{
		Answer:        &content,
		ReasonContent: &reasonContent,
	}

	return chatResponse, nil
}

// ChatStreamlyWithSender sends a message and streams response via sender function (best performance, no channel)
func (z *VolcEngine) ChatStreamlyWithSender(modelName, message *string, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error {
	var region = "default"

	if apiConfig.Region != nil {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/chat/completions", z.BaseURL[region])

	// Build request body with streaming enabled
	reqBody := map[string]interface{}{
		"model": modelName,
		"messages": []map[string]interface{}{
			{"role": "user", "content": *message},
		},
		"stream":      true,
		"temperature": 1,
	}

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

	if modelConfig.DoSample != nil {
		reqBody["do_sample"] = *modelConfig.DoSample
	}

	if modelConfig.Stop != nil {
		reqBody["stop"] = *modelConfig.Stop
	}

	// TODO VolcEngine has `auto` mode
	if modelConfig.Thinking != nil {
		if *modelConfig.Thinking {
			var thinkingFlag string
			switch *modelConfig.Effort {
			case "none", "minimal":
				thinkingFlag = "disabled"
				reqBody["reasoning_effort"] = "minimal"
				break
			case "low":
				thinkingFlag = "enabled"
				reqBody["reasoning_effort"] = "low"
				break
			case "medium":
				thinkingFlag = "enabled"
				reqBody["reasoning_effort"] = "medium"
				break
			case "auto", "default":
				thinkingFlag = "enabled"
				reqBody["reasoning_effort"] = "medium"
				break
			case "high":
				thinkingFlag = "enabled"
				reqBody["reasoning_effort"] = "high"
				break
			default:
				return fmt.Errorf("invalid effort level")
			}
			reqBody["thinking"] = map[string]interface{}{
				"type": thinkingFlag,
			}
		} else {
			reqBody["thinking"] = map[string]interface{}{
				"type": "disabled",
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
		logger.Info(line)

		// SSE data line start with data:
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		// Extract JSON after data:
		data := strings.TrimSpace(line[5:])

		// [DONE] marks the end of stream
		if data == "[DONE]" {
			break
		}

		// Parse the JSON event
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
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

		content, ok := delta["content"].(string)
		if ok && content != "" {
			if err := sender(&content, nil); err != nil {
				return err
			}
		}

		reasoningContent, ok := delta["reasoning_content"].(string)
		if ok && reasoningContent != "" {
			if err := sender(nil, &reasoningContent); err != nil {
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

// Encode encodes a list of texts into embeddings
func (z *VolcEngine) Encode(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([][]float64, error) {
	return nil, fmt.Errorf("not implemented")
}

// Rerank calculates similarity scores between query and texts
func (z *VolcEngine) Rerank(modelName *string, query string, texts []string, apiConfig *APIConfig) ([]float64, error) {
	return nil, fmt.Errorf("%s, Rerank not implemented", z.Name())
}

func (z *VolcEngine) ListModels(apiConfig *APIConfig) ([]string, error) {
	return nil, fmt.Errorf("%s, no such method", z.Name())
}

func (z *VolcEngine) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", z.Name())
}

func (z *VolcEngine) CheckConnection(apiConfig *APIConfig) error {
	var region = "default"
	if apiConfig.Region != nil {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/%s", z.BaseURL[region], z.URLSuffix.Files)

	req, err := http.NewRequest("GET", url, nil)
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
