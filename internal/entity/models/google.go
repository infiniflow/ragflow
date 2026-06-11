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
	"ragflow/internal/common"
	"strings"

	"google.golang.org/genai"
)

type googleModelPage struct {
	items         []DSModel
	nextPageToken string
}

func collectGoogleModelNames(ctx context.Context, listPage func(context.Context, string) (googleModelPage, error)) ([]ListModelResponse, error) {
	var models []DSModel
	pageToken := ""

	for {
		page, err := listPage(ctx, pageToken)
		if err != nil {
			return nil, err
		}

		models = append(models, page.items...)
		if page.nextPageToken == "" {
			return ParseListModel(ModelList{Models: models}), nil
		}
		pageToken = page.nextPageToken
	}
}

var googleListModels = func(ctx context.Context, config *genai.ClientConfig) ([]ListModelResponse, error) {
	client, err := genai.NewClient(ctx, config)
	if err != nil {
		return nil, err
	}

	return collectGoogleModelNames(ctx, func(ctx context.Context, pageToken string) (googleModelPage, error) {
		models, err := client.Models.List(ctx, &genai.ListModelsConfig{PageToken: pageToken})
		if err != nil {
			return googleModelPage{}, err
		}

		var modelNames []DSModel
		for _, m := range models.Items {
			modelName := strings.TrimSpace(m.DisplayName)
			if modelName == "" {
				modelName = strings.TrimSpace(m.Name)
			}
			if modelName != "" {
				modelNames = append(modelNames, DSModel{
					ID:      modelName,
					OwnedBy: "Google",
				})
			}
		}
		return googleModelPage{items: modelNames, nextPageToken: models.NextPageToken}, nil
	})
}

// GoogleModel implements ModelDriver for Google AI
type GoogleModel struct {
	baseModel BaseModel
}

// NewGoogleModel creates a new Google AI model instance
func NewGoogleModel(baseURL map[string]string, urlSuffix URLSuffix) *GoogleModel {
	return &GoogleModel{
		baseModel: BaseModel{
			BaseURL:   baseURL,
			URLSuffix: urlSuffix,
		},
	}
}

func (g *GoogleModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewGoogleModel(baseURL, g.baseModel.URLSuffix)
}

func (g *GoogleModel) Name() string {
	return "google"
}

func (g *GoogleModel) clientConfig(apiKey string, apiConfig *APIConfig) *genai.ClientConfig {
	return &genai.ClientConfig{APIKey: apiKey, Backend: genai.BackendGeminiAPI, HTTPOptions: genai.HTTPOptions{BaseURL: g.baseURL(apiConfig)}}
}

func (g *GoogleModel) baseURL(apiConfig *APIConfig) string {
	baseURL, err := g.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		defaultConfig := &APIConfig{}
		if apiConfig != nil {
			defaultConfig.BaseURL = apiConfig.BaseURL
		}
		baseURL, err = g.baseModel.GetBaseURL(defaultConfig)
		if err != nil {
			return ""
		}
	}
	return strings.TrimSpace(baseURL)
}

func (g *GoogleModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if strings.TrimSpace(modelName) == "" {
		return nil, fmt.Errorf("model name is empty")
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, g.clientConfig(strings.TrimSpace(*apiConfig.ApiKey), apiConfig))
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
func (g *GoogleModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, sender func(*string, *string) error) error {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}
	if strings.TrimSpace(modelName) == "" {
		return fmt.Errorf("model name is empty")
	}
	if sender == nil {
		return fmt.Errorf("sender is nil")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, g.clientConfig(strings.TrimSpace(*apiConfig.ApiKey), apiConfig))
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
			if len(response.Candidates) > 0 &&
				response.Candidates[0].Content != nil &&
				len(response.Candidates[0].Content.Parts) > 0 {
				responseContent = response.Candidates[0].Content.Parts[0].Text
			}
		}

		if responseContent != "" {
			common.Info(fmt.Sprintf("Thinking: %s", responseContent))
			if err = sender(nil, &responseContent); err != nil {
				return err
			}
		}

		if content != "" {
			common.Info(fmt.Sprintf("Answer: %s", content))
			if err = sender(&content, nil); err != nil {
				return err
			}
		}
	}

	return err
}

// Embed generates embeddings for a batch of texts using the Gemini embeddings API.
// The SDK routes to batchEmbedContents internally, so all texts are sent in one request.
func (g *GoogleModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if len(texts) == 0 {
		return nil, fmt.Errorf("texts is empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	client, err := genai.NewClient(ctx, g.clientConfig(strings.TrimSpace(*apiConfig.ApiKey), apiConfig))
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	contents := make([]*genai.Content, len(texts))
	for i, text := range texts {
		contents[i] = genai.NewContentFromText(text, genai.RoleUser)
	}

	var cfg *genai.EmbedContentConfig
	if embeddingConfig != nil && embeddingConfig.Dimension > 0 {
		dim := int32(embeddingConfig.Dimension)
		cfg = &genai.EmbedContentConfig{OutputDimensionality: &dim}
	}

	resp, err := client.Models.EmbedContent(ctx, *modelName, contents, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to embed content: %w", err)
	}

	if len(resp.Embeddings) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(resp.Embeddings))
	}

	result := make([]EmbeddingData, len(resp.Embeddings))
	for i, emb := range resp.Embeddings {
		vec := make([]float64, len(emb.Values))
		for j, v := range emb.Values {
			vec[j] = float64(v)
		}
		result[i] = EmbeddingData{
			Embedding: vec,
			Index:     i,
		}
	}

	return result, nil
}

func (g *GoogleModel) ListModels(apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	return googleListModels(context.Background(), g.clientConfig(strings.TrimSpace(*apiConfig.ApiKey), apiConfig))
}

func (g *GoogleModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("no such method")
}

func (g *GoogleModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := g.ListModels(apiConfig)
	return err
}

// Rerank calculates similarity scores between query and documents
func (g *GoogleModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, Rerank not implemented", g.Name())
}

// TranscribeAudio transcribe audio
func (g *GoogleModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GoogleModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", g.Name())
}

// AudioSpeech convert text to audio
func (g *GoogleModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GoogleModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", g.Name())
}

// OCRFile OCR file
func (g *GoogleModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

// ParseFile parse file
func (g *GoogleModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GoogleModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GoogleModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}
