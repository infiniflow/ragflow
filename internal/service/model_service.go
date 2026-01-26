package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"ragflow/internal/model"
)

// ModelProvider provides model instances based on tenant and model type
type ModelProvider interface {
	// GetEmbeddingModel returns an embedding model for the given tenant
	GetEmbeddingModel(ctx context.Context, tenantID string, modelName ...string) (model.EmbeddingModel, error)
	// GetChatModel returns a chat model for the given tenant
	GetChatModel(ctx context.Context, tenantID string, modelName ...string) (model.ChatModel, error)
	// GetRerankModel returns a rerank model for the given tenant
	GetRerankModel(ctx context.Context, tenantID string, modelName ...string) (model.RerankModel, error)
}

// ModelProviderImpl implements ModelProvider
type ModelProviderImpl struct {
	httpClient *http.Client
}

// NewModelProvider creates a new ModelProvider
func NewModelProvider() *ModelProviderImpl {
	return &ModelProviderImpl{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetEmbeddingModel returns an embedding model for the given tenant
func (p *ModelProviderImpl) GetEmbeddingModel(ctx context.Context, tenantID string, modelName ...string) (model.EmbeddingModel, error) {
	// For now, return a default OpenAI embedding model
	// Configuration can come from environment variables or config file
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("EMBEDDING_API_KEY")
	}
	apiBase := os.Getenv("OPENAI_API_BASE")
	if apiBase == "" {
		apiBase = "https://api.openai.com/v1"
	}
	modelNameStr := os.Getenv("EMBEDDING_MODEL")
	if modelNameStr == "" {
		modelNameStr = "text-embedding-ada-002"
	}

	return &openAIEmbeddingModel{
		apiKey:  apiKey,
		apiBase: apiBase,
		model:   modelNameStr,
		httpClient: p.httpClient,
	}, nil
}

// GetChatModel returns a chat model for the given tenant
func (p *ModelProviderImpl) GetChatModel(ctx context.Context, tenantID string, modelName ...string) (model.ChatModel, error) {
	return nil, fmt.Errorf("not implemented yet")
}

// GetRerankModel returns a rerank model for the given tenant
func (p *ModelProviderImpl) GetRerankModel(ctx context.Context, tenantID string, modelName ...string) (model.RerankModel, error) {
	return nil, fmt.Errorf("not implemented yet")
}

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