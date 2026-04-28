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

func (z *GoogleModel) Name() string {
	return "google"
}

// Chat sends a message and returns response
func (z *GoogleModel) Chat(modelName, message *string, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  *apiConfig.ApiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, err
	}

	contents := []*genai.Content{
		genai.NewContentFromText(*message, genai.RoleUser),
	}

	generateContentConfig := &genai.GenerateContentConfig{}
	generateContentConfig.ThinkingConfig = &genai.ThinkingConfig{}
	if chatModelConfig.Thinking != nil && *chatModelConfig.Thinking {
		generateContentConfig.ThinkingConfig.IncludeThoughts = true
	} else {
		generateContentConfig.ThinkingConfig.IncludeThoughts = false
	}

	response, err := client.Models.GenerateContent(ctx, *modelName, contents, generateContentConfig)
	if err != nil {
		return nil, err
	}
	content := response.Text()

	var responseContent string
	if chatModelConfig.Thinking != nil && *chatModelConfig.Thinking {
		responseContent = response.Candidates[0].Content.Parts[0].Text
	}

	chatResponse := &ChatResponse{
		Answer:        &content,
		ReasonContent: &responseContent,
	}
	return chatResponse, nil
}

// ChatWithMessages sends multiple messages with roles and returns response
func (z *GoogleModel) ChatWithMessages(modelName string, apiKey *string, messages []Message, modelConfig *ChatConfig) (string, error) {
	return "", fmt.Errorf("not implemented")
}

// ChatStreamlyWithSender sends a message and streams response via sender function (best performance, no channel)
func (z *GoogleModel) ChatStreamlyWithSender(modelName, message *string, apiConfig *APIConfig, chatModelConfig *ChatConfig, sender func(*string, *string) error) error {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  *apiConfig.ApiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return err
	}
	contents := []*genai.Content{
		genai.NewContentFromText(*message, genai.RoleUser),
	}
	for response, err := range client.Models.GenerateContentStream(
		ctx,
		*modelName,
		contents,
		nil,
	) {
		if err != nil {
			return err
		}

		content := response.Text()

		var responseContent string
		if chatModelConfig.Thinking != nil && *chatModelConfig.Thinking {
			responseContent = response.Candidates[0].Content.Parts[0].Text
		}

		if responseContent != "" {
			logger.Info(fmt.Sprintf("Thinking: %s", responseContent))
			if err = sender(nil, &responseContent); err != nil {
				return err
			}
		}

		if content != "" {
			logger.Info(fmt.Sprintf("Answer: %s", responseContent))
			if err = sender(&content, nil); err != nil {
				return err
			}
		}
	}

	return err
}

// EncodeToEmbedding encodes a list of texts into embeddings
func (z *GoogleModel) EncodeToEmbedding(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([][]float64, error) {
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

// Encode encodes a list of texts into embeddings (convenience method)
func (z *GoogleModel) Encode(modelName *string, texts []string, apiConfig *APIConfig) ([][]float64, error) {
	return z.EncodeToEmbedding(modelName, texts, apiConfig, nil)
}

// EncodeQuery encodes a single query string into embedding (convenience method)
func (z *GoogleModel) EncodeQuery(modelName *string, query string, apiConfig *APIConfig) ([]float64, error) {
	embeddings, err := z.Encode(modelName, []string{query}, apiConfig)
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return embeddings[0], nil
}

// Rerank calculates similarity scores between query and texts
func (z *GoogleModel) Rerank(modelName *string, query string, texts []string, apiConfig *APIConfig) ([]float64, error) {
	return nil, fmt.Errorf("%s, Rerank not implemented", z.Name())
}
