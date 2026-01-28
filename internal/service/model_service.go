package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"ragflow/internal/dao"
	"strings"
	"time"

	"ragflow/internal/model"
)

// ModelProvider provides model instances based on tenant and model type
type ModelProvider interface {
	// GetEmbeddingModel returns an embedding model for the given tenant
	GetEmbeddingModel(ctx context.Context, tenantID string, modelName string) (model.EmbeddingModel, error)
	// GetChatModel returns a chat model for the given tenant
	GetChatModel(ctx context.Context, tenantID string, modelName string) (model.ChatModel, error)
	// GetRerankModel returns a rerank model for the given tenant
	GetRerankModel(ctx context.Context, tenantID string, modelName string) (model.RerankModel, error)
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

// parseModelName parses a composite model name in format "model_name@provider"
// Returns modelName and provider separately
func parseModelName(compositeName string) (modelName, provider string, err error) {
	parts := strings.Split(compositeName, "@")
	if len(parts) == 2 {
		return parts[0], parts[1], nil
	} else if len(parts) == 1 {
		return parts[0], "", fmt.Errorf("provider name missing in model name: %s", compositeName)
	} else {
		return "", "", fmt.Errorf("invalid model name format: %s", compositeName)
	}
}

// GetEmbeddingModel returns an embedding model for the given tenant
func (p *ModelProviderImpl) GetEmbeddingModel(ctx context.Context, tenantID string, compositeModelName string) (model.EmbeddingModel, error) {
	// Parse composite model name to extract model name and provider
	modelName, provider, err := parseModelName(compositeModelName)
	if err != nil {
		return nil, err
	}

	// Get API key and configuration
	embeddingModel, err := dao.NewTenantLLMDAO().GetByTenantFactoryAndModelName(tenantID, provider, modelName)
	if err != nil {
		return nil, err
	}

	apiKey := embeddingModel.APIKey
	if apiKey == "" {
		return nil, fmt.Errorf("no API key found for tenant %s and model %s", tenantID, compositeModelName)
	}
	apiBase := embeddingModel.APIBase
	if apiBase == "" {
		return nil, fmt.Errorf("no API base found for tenant %s and model %s", tenantID, compositeModelName)
	}

	return &openAIEmbeddingModel{
		apiKey:     apiKey,
		apiBase:    apiBase,
		model:      modelName,
		httpClient: p.httpClient,
	}, nil
}

// GetChatModel returns a chat model for the given tenant
func (p *ModelProviderImpl) GetChatModel(ctx context.Context, tenantID string, compositeModelName string) (model.ChatModel, error) {
	// Parse composite model name to extract model name and provider
	_, _, err := parseModelName(compositeModelName)
	if err != nil {
		return nil, err
	}
	// TODO: implement chat model creation
	return nil, fmt.Errorf("chat model not implemented yet for model: %s", compositeModelName)
}

// GetRerankModel returns a rerank model for the given tenant
func (p *ModelProviderImpl) GetRerankModel(ctx context.Context, tenantID string, compositeModelName string) (model.RerankModel, error) {
	// Parse composite model name to extract model name and provider
	_, _, err := parseModelName(compositeModelName)
	if err != nil {
		return nil, err
	}
	// TODO: implement rerank model creation
	return nil, fmt.Errorf("rerank model not implemented yet for model: %s", compositeModelName)
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
