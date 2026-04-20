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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"ragflow/internal/entity"

	"strings"
)

// openAIEmbeddingModel implements EmbeddingModel for OpenAI API
type openAIEmbeddingModel struct {
	apiKey     string
	apiBase    string
	model      string
	httpClient *http.Client
}

// OpenAIEmbeddingRequest represents OpenAI embedding request
type OpenAIEmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// OpenAIEmbeddingResponse represents OpenAI embedding response
type OpenAIEmbeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

// Encode encodes a list of texts into embeddings using OpenAI API
func (m *openAIEmbeddingModel) Encode(texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return [][]float64{}, nil
	}

	reqBody := OpenAIEmbeddingRequest{
		Model: m.model,
		Input: texts,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", m.apiBase+"/embeddings", strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.apiKey)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI API error: %s, body: %s", resp.Status, string(body))
	}

	var embeddingResp OpenAIEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embeddingResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Sort embeddings by index to ensure correct order
	embeddings := make([][]float64, len(texts))
	for _, data := range embeddingResp.Data {
		if data.Index < len(embeddings) {
			embeddings[data.Index] = data.Embedding
		}
	}

	return embeddings, nil
}

// EncodeQuery encodes a single query string into embedding
func (m *openAIEmbeddingModel) EncodeQuery(query string) ([]float64, error) {
	embeddings, err := m.Encode([]string{query})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return embeddings[0], nil
}

// init registers the OpenAI embedding model factory
func init() {
	RegisterEmbeddingModelFactory("OpenAI", func(apiKey, apiBase, modelName string, httpClient *http.Client) entity.EmbeddingModel {
		return &openAIEmbeddingModel{
			apiKey:     apiKey,
			apiBase:    apiBase,
			model:      modelName,
			httpClient: httpClient,
		}
	})

	// Register chat model factory
	RegisterChatModelFactory("OpenAI", func(apiKey, apiBase, modelName string, httpClient *http.Client) entity.ChatModel {
		return &openAIChatModel{
			apiKey:     apiKey,
			apiBase:    apiBase,
			model:      modelName,
			httpClient: httpClient,
		}
	})
}

// openAIChatModel implements ChatModel for OpenAI API
type openAIChatModel struct {
	apiKey     string
	apiBase    string
	model      string
	httpClient *http.Client
}

// ChatMessage represents a chat message
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OpenAIChatRequest represents OpenAI chat request
type OpenAIChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
}

// OpenAIChatResponse represents OpenAI chat response
type OpenAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// Chat sends a chat message and returns response
func (m *openAIChatModel) Chat(system string, history []map[string]string, genConf map[string]interface{}) (string, error) {
	// Build messages array
	var messages []ChatMessage

	// Add system message if provided
	if system != "" {
		messages = append(messages, ChatMessage{Role: "system", Content: system})
	}

	// Add history messages
	for _, msg := range history {
		role := msg["role"]
		content := msg["content"]
		if role != "" && content != "" {
			messages = append(messages, ChatMessage{Role: role, Content: content})
		}
	}

	// Extract generation config
	temperature := 0.7
	if temp, ok := genConf["temperature"].(float64); ok {
		temperature = temp
	}
	maxTokens := 1024
	if mt, ok := genConf["max_tokens"].(int); ok {
		maxTokens = mt
	}

	// Build request
	reqBody := OpenAIChatRequest{
		Model:       m.model,
		Messages:    messages,
		Temperature: temperature,
		MaxTokens:   maxTokens,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Build URL - append /chat/completions if not already present
	url := m.apiBase
	if !strings.HasSuffix(url, "/chat/completions") {
		if !strings.HasSuffix(url, "/") {
			url += "/"
		}
		url += "chat/completions"
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.apiKey)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenAI API error: %s, body: %s", resp.Status, string(body))
	}

	var chatResp OpenAIChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if chatResp.Error.Message != "" {
		return "", fmt.Errorf("OpenAI API error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no response choices returned")
	}

	return chatResp.Choices[0].Message.Content, nil
}

// ChatStreamly sends a chat message and streams response
func (m *openAIChatModel) ChatStreamly(system string, history []map[string]string, genConf map[string]interface{}) (<-chan string, error) {
	// For now, return a simple non-streaming implementation
	// Streaming can be implemented later with SSE support
	responseChan := make(chan string)

	go func() {
		defer close(responseChan)
		response, err := m.Chat(system, history, genConf)
		if err != nil {
			responseChan <- "**ERROR**: " + err.Error()
			return
		}
		responseChan <- response
	}()

	return responseChan, nil
}
