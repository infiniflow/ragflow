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
	"fmt"
	"net/http"
	"ragflow/internal/entity"

	"sync"
)

// EmbeddingModelFactory creates an EmbeddingModel instance
type EmbeddingModelFactory func(apiKey, apiBase, modelName string, httpClient *http.Client) entity.EmbeddingModel

// ChatModelFactory creates a ChatModel instance
type ChatModelFactory func(apiKey, apiBase, modelName string, httpClient *http.Client) entity.ChatModel

// RerankModelFactory creates a RerankModel instance
type RerankModelFactory func(apiKey, apiBase, modelName string, httpClient *http.Client) entity.RerankModel

var (
	embeddingModelFactories = make(map[string]EmbeddingModelFactory)
	chatModelFactories      = make(map[string]ChatModelFactory)
	rerankModelFactories    = make(map[string]RerankModelFactory)
	factoryMu               sync.RWMutex
)

// RegisterEmbeddingModelFactory registers a factory for a provider name.
// Should be called from init() functions of provider implementations.
func RegisterEmbeddingModelFactory(providerName string, factory EmbeddingModelFactory) {
	factoryMu.Lock()
	defer factoryMu.Unlock()
	embeddingModelFactories[providerName] = factory
}

// RegisterChatModelFactory registers a factory for a chat provider name.
// Should be called from init() functions of provider implementations.
func RegisterChatModelFactory(providerName string, factory ChatModelFactory) {
	factoryMu.Lock()
	defer factoryMu.Unlock()
	chatModelFactories[providerName] = factory
}

// GetEmbeddingModelFactory returns the factory for the given provider name.
// Returns nil if not found.
func GetEmbeddingModelFactory(providerName string) EmbeddingModelFactory {
	factoryMu.RLock()
	defer factoryMu.RUnlock()
	return embeddingModelFactories[providerName]
}

// GetChatModelFactory returns the factory for the given chat provider name.
// Returns nil if not found.
func GetChatModelFactory(providerName string) ChatModelFactory {
	factoryMu.RLock()
	defer factoryMu.RUnlock()
	return chatModelFactories[providerName]
}

// RegisterRerankModelFactory registers a factory for a rerank provider name.
// Should be called from init() functions of provider implementations.
func RegisterRerankModelFactory(providerName string, factory RerankModelFactory) {
	factoryMu.Lock()
	defer factoryMu.Unlock()
	rerankModelFactories[providerName] = factory
}

// GetRerankModelFactory returns the factory for the given rerank provider name.
// Returns nil if not found.
func GetRerankModelFactory(providerName string) RerankModelFactory {
	factoryMu.RLock()
	defer factoryMu.RUnlock()
	return rerankModelFactories[providerName]
}

// CreateEmbeddingModel creates an EmbeddingModel instance for the given provider.
// Returns error if provider not registered.
func CreateEmbeddingModel(providerName, apiKey, apiBase, modelName string, httpClient *http.Client) (entity.EmbeddingModel, error) {
	factory := GetEmbeddingModelFactory(providerName)
	if factory == nil {
		return nil, fmt.Errorf("no embedding model factory registered for provider %s", providerName)
	}
	return factory(apiKey, apiBase, modelName, httpClient), nil
}

// CreateChatModel creates a ChatModel instance for the given provider.
// Returns error if provider not registered.
func CreateChatModel(providerName, apiKey, apiBase, modelName string, httpClient *http.Client) (entity.ChatModel, error) {
	factory := GetChatModelFactory(providerName)
	if factory == nil {
		return nil, fmt.Errorf("no chat model factory registered for provider %s", providerName)
	}
	return factory(apiKey, apiBase, modelName, httpClient), nil
}

// CreateRerankModel creates a RerankModel instance for the given provider.
// Returns error if provider not registered.
func CreateRerankModel(providerName, apiKey, apiBase, modelName string, httpClient *http.Client) (entity.RerankModel, error) {
	factory := GetRerankModelFactory(providerName)
	if factory == nil {
		return nil, fmt.Errorf("no rerank model factory registered for provider %s", providerName)
	}
	return factory(apiKey, apiBase, modelName, httpClient), nil
}
