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

package service

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"ragflow/internal/dao"
	"ragflow/internal/server"
	"strings"
	"time"

	"ragflow/internal/model"
	"ragflow/internal/service/models"
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

// isTEIFallback checks if the fallback to global config should happen
func isTEIFallback(provider, modelName string) bool {
	if provider != "Builtin" {
		return false
	}

	composeProfiles := os.Getenv("COMPOSE_PROFILES")
	if !strings.Contains(composeProfiles, "tei-") {
		return false
	}

	teiModel := os.Getenv("TEI_MODEL")

	return modelName == teiModel
}

// GetEmbeddingModel returns an embedding model for the given tenant
func (p *ModelProviderImpl) GetEmbeddingModel(ctx context.Context, tenantID string, compositeModelName string) (model.EmbeddingModel, error) {
	// Parse composite model name to extract model name and provider
	modelName, provider, err := parseModelName(compositeModelName)
	if err != nil {
		return nil, err
	}

	// Try to get from tenant-specific configuration first
	embeddingModel, err := dao.NewTenantLLMDAO().GetByTenantFactoryAndModelName(tenantID, provider, modelName)
	if err == nil && embeddingModel != nil {
		// Found tenant-specific model
		apiKey := embeddingModel.APIKey
		if apiKey != nil && *apiKey != "" {
			// Get API base from model provider configuration
			providerDAO := dao.NewModelProviderDAO()
			providerConfig := providerDAO.GetProviderByName(provider)
			if providerConfig == nil || providerConfig.DefaultEmbeddingURL == "" {
				return nil, fmt.Errorf("no API base found for provider %s", provider)
			}
			apiBase := providerConfig.DefaultEmbeddingURL
			return models.CreateEmbeddingModel(provider, *apiKey, apiBase, modelName, p.httpClient)
		}
	}

	// Fallback to global config
	if !isTEIFallback(provider, modelName) {
		return nil, fmt.Errorf("model %s not found for tenant %s (and not eligible for TEI fallback)", compositeModelName, tenantID)
	}

	config := server.GetConfig()
	if config == nil || config.UserDefaultLLM.DefaultModels.EmbeddingModel.Name == "" {
		return nil, fmt.Errorf("no embedding model found for tenant %s (and no global config)", tenantID)
	}

	globalModel := config.UserDefaultLLM.DefaultModels.EmbeddingModel
	globalModelName, globalProvider, _ := parseModelName(globalModel.Name)
	if globalModelName == "" {
		globalModelName = modelName
	}
	if globalProvider == "" {
		globalProvider = provider
	}

	// Use global config values
	apiKey := ""
	if globalModel.APIKey != "" {
		apiKey = globalModel.APIKey
	}

	apiBase := globalModel.BaseURL
	if apiBase == "" {
    	// Fallback to provider default
		providerDAO := dao.NewModelProviderDAO()
		providerConfig := providerDAO.GetProviderByName(globalProvider)
		if providerConfig != nil {
			apiBase = providerConfig.DefaultEmbeddingURL
		}
	}

	if apiBase == "" {
		return nil, fmt.Errorf("no API base found for provider %s", globalProvider)
	}

	return models.CreateEmbeddingModel(globalProvider, apiKey, apiBase, globalModelName, p.httpClient)
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
