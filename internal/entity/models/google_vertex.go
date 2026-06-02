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

// GoogleVertexModel implements ModelDriver for Google Cloud Vertex AI.
type GoogleVertexModel struct {
	BaseURL   map[string]string
	URLSuffix URLSuffix
}

// NewGoogleVertexModel creates a new Google Vertex AI model instance.
func NewGoogleVertexModel(baseURL map[string]string, urlSuffix URLSuffix) *GoogleVertexModel {
	return &GoogleVertexModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
	}
}

func (g *GoogleVertexModel) NewInstance(baseURL map[string]string) ModelDriver {
	return &GoogleVertexModel{
		BaseURL:   baseURL,
		URLSuffix: g.URLSuffix,
	}
}

func (g *GoogleVertexModel) Name() string {
	return "google vertex"
}

func buildVertexContents(messages []Message) []*genai.Content {
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

		switch c := msg.Content.(type) {
		case string:
			contents = append(contents, genai.NewContentFromText(c, role))
		case []interface{}:
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
	return contents
}

func vertexThinkingText(response *genai.GenerateContentResponse) string {
	if response == nil || len(response.Candidates) == 0 {
		return ""
	}
	content := response.Candidates[0].Content
	if content == nil || len(content.Parts) == 0 || content.Parts[0] == nil {
		return ""
	}
	return content.Parts[0].Text
}

func (g *GoogleVertexModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	ctx := context.Background()
	client, err := g.newClient(ctx, apiConfig)
	if err != nil {
		return nil, err
	}

	contents := buildVertexContents(messages)

	response, err := client.Models.GenerateContent(ctx, modelName, contents, nil)
	if err != nil {
		return nil, err
	}

	answer := response.Text()
	return &ChatResponse{Answer: &answer}, nil
}

func (g *GoogleVertexModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, sender func(*string, *string) error) error {
	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	ctx := context.Background()
	client, err := g.newClient(ctx, apiConfig)
	if err != nil {
		return err
	}

	contents := buildVertexContents(messages)

	for response, err := range client.Models.GenerateContentStream(ctx, modelName, contents, nil) {
		if err != nil {
			return err
		}

		content := response.Text()
		var responseContent string
		if chatModelConfig != nil && chatModelConfig.Thinking != nil && *chatModelConfig.Thinking {
			responseContent = vertexThinkingText(response)
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

	return nil
}

func (g *GoogleVertexModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
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

func (g *GoogleVertexModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	return googleVertexListModels(context.Background(), apiConfig, g.BaseURL)
}

func (g *GoogleVertexModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("no such method")
}

func (g *GoogleVertexModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := g.ListModels(apiConfig)
	return err
}

func (g *GoogleVertexModel) newClient(ctx context.Context, apiConfig *APIConfig) (*genai.Client, error) {
	return newGoogleVertexClient(ctx, apiConfig, g.BaseURL)
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

func (g *GoogleVertexModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, Rerank not implemented", g.Name())
}

func (g *GoogleVertexModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GoogleVertexModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", g.Name())
}

func (g *GoogleVertexModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GoogleVertexModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", g.Name())
}

func (g *GoogleVertexModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GoogleVertexModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GoogleVertexModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GoogleVertexModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}
