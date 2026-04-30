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
	"context"
	"fmt"
	"ragflow/internal/logger"

	"google.golang.org/genai"
)

// GoogleModel implements ModelDriver for Dummy AI
type GoogleModel struct {
	BaseURL   map[string]string
	URLSuffix URLSuffix
}

// NewGoogleModel creates a new Google AI model instance
func NewGoogleModel(baseURL map[string]string, urlSuffix URLSuffix) *GoogleModel {
	return &GoogleModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
	}
}

func (z *GoogleModel) NewInstance(baseURL map[string]string) ModelDriver {
	return nil
}

func (z *GoogleModel) Name() string {
	return "google"
}

func (z *GoogleModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is nil or empty")
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  *apiConfig.ApiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, err
	}

	// Convert messages to Google SDK format
	var contents []*genai.Content
	for _, msg := range messages {
		var role genai.Role
		switch msg.Role {
		case "user":
			role = genai.RoleUser
		case "model", "assistant":
			role = genai.RoleModel
		default:
			role = genai.RoleUser
		}

		// Handle content based on type
		switch c := msg.Content.(type) {
		case string:
			contents = append(contents, genai.NewContentFromText(c, role))
		case []interface{}:
			// Multimodal content - group parts within a single content
			var parts []*genai.Part
			for _, item := range c {
				if itemMap, ok := item.(map[string]interface{}); ok {
					contentType, _ := itemMap["type"].(string)
					switch contentType {
					case "text":
						if text, ok := itemMap["text"].(string); ok {
							parts = append(parts, genai.NewPartFromText(text))
						}
					case "image_url":
						if imgMap, ok := itemMap["image_url"].(map[string]interface{}); ok {
							if url, ok := imgMap["url"].(string); ok {
								parts = append(parts, genai.NewPartFromURI(url, "image/jpeg"))
							}
						}
					}
				}
			}
			if len(parts) > 0 {
				contents = append(contents, genai.NewContentFromParts(parts, role))
			}
		}
	}

	// Generate content (non-streaming)
	response, err := client.Models.GenerateContent(ctx, modelName, contents, nil)
	if err != nil {
		return nil, err
	}

	// Extract text from response
	answer := response.Text()

	return &ChatResponse{Answer: &answer}, nil
}

// ChatStreamlyWithSender sends messages and streams response via sender function (best performance, no channel)
func (z *GoogleModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, sender func(*string, *string) error) error {
	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  *apiConfig.ApiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return err
	}

	// Convert messages to Google SDK format
	var contents []*genai.Content
	for _, msg := range messages {
		var role genai.Role
		switch msg.Role {
		case "user":
			role = genai.RoleUser
		case "model", "assistant":
			role = genai.RoleModel
		default:
			role = genai.RoleUser
		}

		// Handle content based on type
		switch c := msg.Content.(type) {
		case string:
			contents = append(contents, genai.NewContentFromText(c, role))
		case []interface{}:
			// Multimodal content - group parts within a single content
			var parts []*genai.Part
			for _, item := range c {
				if itemMap, ok := item.(map[string]interface{}); ok {
					contentType, _ := itemMap["type"].(string)
					switch contentType {
					case "text":
						if text, ok := itemMap["text"].(string); ok {
							parts = append(parts, genai.NewPartFromText(text))
						}
					case "image_url":
						if imgMap, ok := itemMap["image_url"].(map[string]interface{}); ok {
							if url, ok := imgMap["url"].(string); ok {
								parts = append(parts, genai.NewPartFromURI(url, "image/jpeg"))
							}
						}
					}
				}
			}
			if len(parts) > 0 {
				contents = append(contents, genai.NewContentFromParts(parts, role))
			}
		}
	}

	for response, err := range client.Models.GenerateContentStream(
		ctx,
		modelName,
		contents,
		nil,
	) {
		if err != nil {
			return err
		}

		content := response.Text()

		var responseContent string
		if chatModelConfig != nil && chatModelConfig.Thinking != nil && *chatModelConfig.Thinking {
			responseContent = response.Candidates[0].Content.Parts[0].Text
		}

		if responseContent != "" {
			logger.Info(fmt.Sprintf("Thinking: %s", responseContent))
			if err = sender(nil, &responseContent); err != nil {
				return err
			}
		}

		if content != "" {
			logger.Info(fmt.Sprintf("Answer: %s", content))
			if err = sender(&content, nil); err != nil {
				return err
			}
		}
	}

	return err
}

// Encode encodes a list of texts into embeddings
func (z *GoogleModel) Encode(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([][]float64, error) {
	return nil, fmt.Errorf("not implemented")
}

func (z *GoogleModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  *apiConfig.ApiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, err
	}

	// Retrieve the list of models.
	models, err := client.Models.List(ctx, &genai.ListModelsConfig{})
	if err != nil {
		return nil, err
	}

	var modelNames []string
	for _, m := range models.Items {
		modelNames = append(modelNames, m.Name)
	}
	return modelNames, nil
}

func (z *GoogleModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("no such method")
}

func (z *GoogleModel) CheckConnection(apiConfig *APIConfig) error {
	return fmt.Errorf("no such method")
}

// Rerank calculates similarity scores between query and texts
func (z *GoogleModel) Rerank(modelName *string, query string, texts []string, apiConfig *APIConfig) ([]float64, error) {
	return nil, fmt.Errorf("%s, Rerank not implemented", z.Name())
}
