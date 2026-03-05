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
	"ragflow/internal/dao"
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
	// Always get API base from model provider configuration
	providerDAO := dao.NewModelProviderDAO()
	providerConfig := providerDAO.GetProviderByName(provider)
	if providerConfig == nil || providerConfig.DefaultEmbeddingURL == "" {
		return nil, fmt.Errorf("no API base found for provider %s", provider)
	}
	apiBase := providerConfig.DefaultEmbeddingURL

	return models.CreateEmbeddingModel(provider, apiKey, apiBase, modelName, p.httpClient)
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
