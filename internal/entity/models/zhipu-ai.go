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
	"strings"
)

// ZhipuAIModel implements ModelDriver for Zhipu AI (智谱 AI)
type ZhipuAIModel struct {
	BaseURL   string
	URLSuffix URLSuffix
}

// NewZhipuAIModel creates a new Zhipu AI model instance
func NewZhipuAIModel(baseURL string, urlSuffix URLSuffix) *ZhipuAIModel {
	return &ZhipuAIModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
	}
}

// Chat sends a message and returns response
func (z *ZhipuAIModel) Chat(modelName, apiKey, message *string, genConf map[string]interface{}) (string, error) {
	if message == nil {
		return "", fmt.Errorf("message is nil")
	}

	url := fmt.Sprintf("%s/%s", z.BaseURL, z.URLSuffix.Chat)

	// Build request body
	reqBody := map[string]interface{}{
		"model": modelName,
		"messages": []map[string]string{
			{"role": "user", "content": *message},
		},
		"stream":      false,
		"temperature": 1,
	}

	// Add generation config if provided
	if genConf != nil {
		if maxTokens, ok := genConf["max_tokens"]; ok {
			reqBody["max_tokens"] = maxTokens
		}
		if temperature, ok := genConf["temperature"]; ok {
			reqBody["temperature"] = temperature
		}
		if topP, ok := genConf["top_p"]; ok {
			reqBody["top_p"] = topP
		}
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiKey))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	firstChoice, ok := choices[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid choice format")
	}

	messageMap, ok := firstChoice["message"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid message format")
	}

	content, ok := messageMap["content"].(string)
	if !ok {
		return "", fmt.Errorf("invalid content format")
	}

	return content, nil
}

// ChatStreamly sends a message and streams response
func (z *ZhipuAIModel) ChatStreamly(modelName, apiKey, message *string, genConf map[string]interface{}) (<-chan string, error) {
	url := fmt.Sprintf("%s/chat/completions", z.BaseURL)

	// Build request body with streaming enabled
	reqBody := map[string]interface{}{
		"model": modelName,
		"messages": []map[string]string{
			{"role": "user", "content": *message},
		},
		"stream":      true,
		"temperature": 1,
	}

	// Add generation config if provided
	if genConf != nil {
		if maxTokens, ok := genConf["max_tokens"]; ok {
			reqBody["max_tokens"] = maxTokens
		}
		if temperature, ok := genConf["temperature"]; ok {
			reqBody["temperature"] = temperature
		}
		if topP, ok := genConf["top_p"]; ok {
			reqBody["top_p"] = topP
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
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiKey))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Create channel for streaming
	resultChan := make(chan string)

	go func() {
		defer close(resultChan)
		defer resp.Body.Close()

		// SSE parsing: read line by line
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			
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
				resultChan <- content
			}

			finishReason, ok := firstChoice["finish_reason"].(string)
			if ok && finishReason != "" {
				break
			}
		}
	}()

	return resultChan, nil
}

// ChatStreamlyWithChannel sends a message and streams response to channel (better performance)
func (z *ZhipuAIModel) ChatStreamlyWithChannel(modelName, apiKey, message *string, genConf map[string]interface{}, resultChan chan<- string) error {
	url := fmt.Sprintf("%s/chat/completions", z.BaseURL)

	// Build request body with streaming enabled
	reqBody := map[string]interface{}{
		"model": modelName,
		"messages": []map[string]string{
			{"role": "user", "content": *message},
		},
		"stream":      true,
		"temperature": 1,
	}

	// Add generation config if provided
	if genConf != nil {
		if maxTokens, ok := genConf["max_tokens"]; ok {
			reqBody["max_tokens"] = maxTokens
		}
		if temperature, ok := genConf["temperature"]; ok {
			reqBody["temperature"] = temperature
		}
		if topP, ok := genConf["top_p"]; ok {
			reqBody["top_p"] = topP
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
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiKey))

	client := &http.Client{}
	resp, err := client.Do(req)
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
			resultChan <- content
		}

		finishReason, ok := firstChoice["finish_reason"].(string)
		if ok && finishReason != "" {
			break
		}
	}

	// Send [DONE] marker for OpenAI compatibility
	resultChan <- "[DONE]"

	return scanner.Err()
}

// EncodeToEmbedding encodes a list of texts into embeddings
func (z *ZhipuAIModel) EncodeToEmbedding(modelName, apiKey *string, texts []string) ([][]float64, error) {
	url := fmt.Sprintf("%s/embedding", z.BaseURL)

	embeddings := make([][]float64, len(texts))

	for i, text := range texts {
		reqBody := map[string]interface{}{
			"model": modelName,
			"input": text,
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
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiKey))

		client := &http.Client{}
		resp, err := client.Do(req)
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
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		data, ok := result["data"].([]interface{})
		if !ok || len(data) == 0 {
			return nil, fmt.Errorf("no data in response")
		}

		firstData, ok := data[0].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid data format")
		}

		embeddingSlice, ok := firstData["embedding"].([]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid embedding format")
		}

		embedding := make([]float64, len(embeddingSlice))
		for j, v := range embeddingSlice {
			switch val := v.(type) {
			case float64:
				embedding[j] = val
			case float32:
				embedding[j] = float64(val)
			default:
				return nil, fmt.Errorf("unexpected embedding value type")
			}
		}

		embeddings[i] = embedding
	}

	return embeddings, nil
}
