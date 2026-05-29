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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"ragflow/internal/common"
	"strings"

	"cloud.google.com/go/auth/credentials"
	"google.golang.org/genai"
)

type googleModelPage struct {
	items         []string
	nextPageToken string
}

func collectGoogleModelNames(ctx context.Context, listPage func(context.Context, string) (googleModelPage, error)) ([]string, error) {
	var modelNames []string
	pageToken := ""

	for {
		page, err := listPage(ctx, pageToken)
		if err != nil {
			return nil, err
		}

		modelNames = append(modelNames, page.items...)
		if page.nextPageToken == "" {
			return modelNames, nil
		}
		pageToken = page.nextPageToken
	}
}

var googleListModels = func(ctx context.Context, apiKey string) ([]string, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, err
	}

	return collectGoogleModelNames(ctx, func(ctx context.Context, pageToken string) (googleModelPage, error) {
		models, err := client.Models.List(ctx, &genai.ListModelsConfig{PageToken: pageToken})
		if err != nil {
			return googleModelPage{}, err
		}

		var modelNames []string
		for _, m := range models.Items {
			modelNames = append(modelNames, m.Name)
		}
		return googleModelPage{items: modelNames, nextPageToken: models.NextPageToken}, nil
	})
}

var googleVertexListModels = func(ctx context.Context, apiConfig *APIConfig, baseURL map[string]string) ([]string, error) {
	client, err := newGoogleVertexClient(ctx, apiConfig, baseURL)
	if err != nil {
		return nil, err
	}

	return collectGoogleModelNames(ctx, func(ctx context.Context, pageToken string) (googleModelPage, error) {
		models, err := client.Models.List(ctx, &genai.ListModelsConfig{PageToken: pageToken})
		if err != nil {
			return googleModelPage{}, err
		}

		var modelNames []string
		for _, m := range models.Items {
			modelNames = append(modelNames, m.Name)
		}
		return googleModelPage{items: modelNames, nextPageToken: models.NextPageToken}, nil
	})
}

// GoogleModel implements ModelDriver for Google AI
type GoogleModel struct {
	BaseURL   map[string]string
	URLSuffix URLSuffix
	Backend   genai.Backend
	name      string
}

// NewGoogleModel creates a new Google AI model instance
func NewGoogleModel(baseURL map[string]string, urlSuffix URLSuffix) *GoogleModel {
	return &GoogleModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
		Backend:   genai.BackendGeminiAPI,
		name:      "google",
	}
}

// NewGoogleVertexModel creates a new Google Vertex AI model instance.
func NewGoogleVertexModel(baseURL map[string]string, urlSuffix URLSuffix) *GoogleModel {
	return &GoogleModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
		Backend:   genai.BackendVertexAI,
		name:      "google vertex",
	}
}

func (g *GoogleModel) NewInstance(baseURL map[string]string) ModelDriver {
	return &GoogleModel{
		BaseURL:   baseURL,
		URLSuffix: g.URLSuffix,
		Backend:   g.Backend,
		name:      g.name,
	}
}

func (g *GoogleModel) Name() string {
	if g.name != "" {
		return g.name
	}
	return "google"
}

func (g *GoogleModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	ctx := context.Background()
	client, err := g.newClient(ctx, apiConfig)
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
	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	ctx := context.Background()
	client, err := g.newClient(ctx, apiConfig)
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
	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if len(texts) == 0 {
		return nil, fmt.Errorf("texts is empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	client, err := g.newClient(ctx, apiConfig)
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

func (g *GoogleModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	if g.Backend == genai.BackendVertexAI {
		return googleVertexListModels(context.Background(), apiConfig, g.BaseURL)
	}

	if apiConfig == nil || apiConfig.ApiKey == nil || strings.TrimSpace(*apiConfig.ApiKey) == "" {
		return nil, fmt.Errorf("api key is required")
	}

	return googleListModels(context.Background(), *apiConfig.ApiKey)
}

func (g *GoogleModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("no such method")
}

func (g *GoogleModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := g.ListModels(apiConfig)
	return err
}

func (g *GoogleModel) newClient(ctx context.Context, apiConfig *APIConfig) (*genai.Client, error) {
	if g.Backend == genai.BackendVertexAI {
		return newGoogleVertexClient(ctx, apiConfig, g.BaseURL)
	}

	if apiConfig == nil || apiConfig.ApiKey == nil || strings.TrimSpace(*apiConfig.ApiKey) == "" {
		return nil, fmt.Errorf("api key is required")
	}

	return genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  strings.TrimSpace(*apiConfig.ApiKey),
		Backend: genai.BackendGeminiAPI,
	})
}

type googleVertexAPIKey struct {
	APIKey                  string `json:"api_key"`
	GoogleProjectID         string `json:"google_project_id"`
	GoogleRegion            string `json:"google_region"`
	GoogleServiceAccountKey string `json:"google_service_account_key"`
}

func newGoogleVertexClient(ctx context.Context, apiConfig *APIConfig, baseURL map[string]string) (*genai.Client, error) {
	cfg, err := googleVertexClientConfig(apiConfig, baseURL)
	if err != nil {
		return nil, err
	}
	return genai.NewClient(ctx, cfg)
}

func googleVertexClientConfig(apiConfig *APIConfig, baseURL map[string]string) (*genai.ClientConfig, error) {
	cfg := &genai.ClientConfig{Backend: genai.BackendVertexAI}
	if apiConfig != nil && apiConfig.Region != nil {
		cfg.Location = strings.TrimSpace(*apiConfig.Region)
	}

	key := ""
	if apiConfig != nil && apiConfig.ApiKey != nil {
		key = strings.TrimSpace(*apiConfig.ApiKey)
	}

	if key != "" {
		var vertexKey googleVertexAPIKey
		if json.Unmarshal([]byte(key), &vertexKey) == nil && (vertexKey.GoogleProjectID != "" || vertexKey.GoogleRegion != "" || vertexKey.GoogleServiceAccountKey != "" || vertexKey.APIKey != "") {
			cfg.Project = strings.TrimSpace(vertexKey.GoogleProjectID)
			if strings.TrimSpace(vertexKey.GoogleRegion) != "" {
				cfg.Location = strings.TrimSpace(vertexKey.GoogleRegion)
			}
			if strings.TrimSpace(vertexKey.APIKey) != "" {
				cfg.APIKey = strings.TrimSpace(vertexKey.APIKey)
				cfg.Project = ""
				cfg.Location = ""
			} else if strings.TrimSpace(vertexKey.GoogleServiceAccountKey) != "" {
				credsJSON, err := decodeGoogleServiceAccountKey(vertexKey.GoogleServiceAccountKey)
				if err != nil {
					return nil, err
				}
				creds, err := credentials.DetectDefault(&credentials.DetectOptions{
					CredentialsJSON: credsJSON,
					Scopes:          []string{"https://www.googleapis.com/auth/cloud-platform"},
				})
				if err != nil {
					return nil, fmt.Errorf("failed to load Google Vertex service account credentials: %w", err)
				}
				cfg.Credentials = creds
			}
		} else {
			cfg.APIKey = key
			cfg.Project = ""
			cfg.Location = ""
		}
	}

	if cfg.APIKey == "" {
		region := cfg.Location
		if region == "" && apiConfig != nil && apiConfig.Region != nil {
			region = strings.TrimSpace(*apiConfig.Region)
		}
		if url := googleVertexBaseURLForRegion(baseURL, region); url != "" {
			cfg.HTTPOptions.BaseURL = url
		}
	}

	return cfg, nil
}

func decodeGoogleServiceAccountKey(encoded string) ([]byte, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, fmt.Errorf("google service account key is empty")
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err == nil {
		return decoded, nil
	}
	if strings.HasPrefix(encoded, "{") {
		return []byte(encoded), nil
	}
	return nil, fmt.Errorf("google service account key must be base64 encoded JSON")
}

func googleVertexBaseURLForRegion(baseURL map[string]string, region string) string {
	if len(baseURL) == 0 {
		return ""
	}
	if region != "" {
		if url := strings.TrimSpace(baseURL[region]); url != "" {
			return url
		}
	}
	return strings.TrimSpace(baseURL["default"])
}

// Rerank calculates similarity scores between query and documents
func (g *GoogleModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, Rerank not implemented", g.Name())
}

// TranscribeAudio transcribe audio
func (g *GoogleModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (z *GoogleModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", z.Name())
}

// AudioSpeech convert text to audio
func (g *GoogleModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (z *GoogleModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", z.Name())
}

// OCRFile OCR file
func (g *GoogleModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

// ParseFile parse file
func (z *GoogleModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", z.Name())
}

func (z *GoogleModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", z.Name())
}

func (z *GoogleModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", z.Name())
}
