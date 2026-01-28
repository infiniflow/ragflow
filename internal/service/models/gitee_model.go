package models

import (
	"fmt"
	"net/http"
	"ragflow/internal/model"
)

// giteeEmbeddingModel implements EmbeddingModel for GiteeAI API (assumed OpenAI-compatible)
type giteeEmbeddingModel struct {
	apiKey     string
	apiBase    string
	model      string
	httpClient *http.Client
}

// Encode encodes a list of texts into embeddings using GiteeAI API
func (m *giteeEmbeddingModel) Encode(texts []string) ([][]float64, error) {
	// For now, reuse the same implementation as OpenAI
	// This can be customized later if GiteeAI API differs
	openAIModel := &openAIEmbeddingModel{
		apiKey:     m.apiKey,
		apiBase:    m.apiBase,
		model:      m.model,
		httpClient: m.httpClient,
	}
	return openAIModel.Encode(texts)
}

// EncodeQuery encodes a single query string into embedding
func (m *giteeEmbeddingModel) EncodeQuery(query string) ([]float64, error) {
	embeddings, err := m.Encode([]string{query})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return embeddings[0], nil
}

// init registers the GiteeAI embedding model factory
func init() {
	RegisterEmbeddingModelFactory("GiteeAI", func(apiKey, apiBase, modelName string, httpClient *http.Client) model.EmbeddingModel {
		return &giteeEmbeddingModel{
			apiKey:     apiKey,
			apiBase:    apiBase,
			model:      modelName,
			httpClient: httpClient,
		}
	})
}