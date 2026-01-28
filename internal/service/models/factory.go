package models

import (
	"fmt"
	"net/http"
	"ragflow/internal/model"
	"sync"
)

// EmbeddingModelFactory creates an EmbeddingModel instance
type EmbeddingModelFactory func(apiKey, apiBase, modelName string, httpClient *http.Client) model.EmbeddingModel

var (
	embeddingModelFactories = make(map[string]EmbeddingModelFactory)
	factoryMu               sync.RWMutex
)

// RegisterEmbeddingModelFactory registers a factory for a provider name.
// Should be called from init() functions of provider implementations.
func RegisterEmbeddingModelFactory(providerName string, factory EmbeddingModelFactory) {
	factoryMu.Lock()
	defer factoryMu.Unlock()
	embeddingModelFactories[providerName] = factory
}

// GetEmbeddingModelFactory returns the factory for the given provider name.
// Returns nil if not found.
func GetEmbeddingModelFactory(providerName string) EmbeddingModelFactory {
	factoryMu.RLock()
	defer factoryMu.RUnlock()
	return embeddingModelFactories[providerName]
}

// CreateEmbeddingModel creates an EmbeddingModel instance for the given provider.
// Returns error if provider not registered.
func CreateEmbeddingModel(providerName, apiKey, apiBase, modelName string, httpClient *http.Client) (model.EmbeddingModel, error) {
	factory := GetEmbeddingModelFactory(providerName)
	if factory == nil {
		return nil, fmt.Errorf("no embedding model factory registered for provider %s", providerName)
	}
	return factory(apiKey, apiBase, modelName, httpClient), nil
}