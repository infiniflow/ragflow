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
	"fmt"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
)

// ModelBundle provides a unified interface for various model operations
// Similar to Python's LLMBundle but with a more generic name
type ModelBundle struct {
	tenantID        string
	modelType       entity.ModelType
	modelName       string
	model           interface{} // underlying model instance
	apiConfig       *modelModule.APIConfig
	embeddingConfig *modelModule.EmbeddingConfig
}

// NewModelBundle creates a new ModelBundle for the given tenant and model type
// If modelName is empty, uses the default model for the tenant and type
func NewModelBundle(tenantID string, modelType entity.ModelType, modelName ...string) (*ModelBundle, error) {
	bundle := &ModelBundle{
		tenantID:        tenantID,
		modelType:       modelType,
		embeddingConfig: &modelModule.EmbeddingConfig{},
	}

	// Use provided model name if available
	if len(modelName) > 0 && modelName[0] != "" {
		bundle.modelName = modelName[0]
	}

	// Get model instance based on type
	modelProviderSvc := NewModelProviderService()
	switch modelType {
	case entity.ModelTypeEmbedding:
		embd, err := modelProviderSvc.GetEmbeddingModel(tenantID, bundle.modelName)
		if err != nil {
			return nil, fmt.Errorf("failed to get embedding model: %w", err)
		}
		bundle.model = embd.ModelDriver
		bundle.apiConfig = embd.APIConfig
	case entity.ModelTypeChat:
		chatMdl, err := modelProviderSvc.GetChatModel(tenantID, bundle.modelName)
		if err != nil {
			return nil, fmt.Errorf("failed to get chat model: %w", err)
		}
		bundle.model = chatMdl.ModelDriver
		bundle.apiConfig = chatMdl.APIConfig
	case entity.ModelTypeRerank:
		rerankMdl, err := modelProviderSvc.GetRerankModel(tenantID, bundle.modelName)
		if err != nil {
			return nil, fmt.Errorf("failed to get rerank model: %w", err)
		}
		bundle.model = rerankMdl.ModelDriver
		bundle.apiConfig = rerankMdl.APIConfig
	default:
		return nil, fmt.Errorf("unsupported model type: %s", modelType)
	}

	return bundle, nil
}

// Encode encodes a list of texts into embeddings
// Returns embeddings and token count (for compatibility with Python interface)
func (b *ModelBundle) Encode(texts []string) ([][]float64, int64, error) {
	if b.modelType != entity.ModelTypeEmbedding {
		return nil, 0, fmt.Errorf("model type %s does not support encode", b.modelType)
	}

	embeddingModel, ok := b.model.(entity.EmbeddingModel)
	if !ok {
		return nil, 0, fmt.Errorf("model is not an embedding model")
	}

	embeddings, err := embeddingModel.Encode(&b.modelName, texts, b.apiConfig, b.embeddingConfig)
	if err != nil {
		return nil, 0, err
	}

	// TODO: Calculate actual token count
	// For now, return a dummy token count
	tokenCount := int64(0)
	for _, text := range texts {
		tokenCount += int64(len(text) / 4) // rough approximation
	}

	return embeddings, tokenCount, nil
}

// EncodeQuery encodes a single query string into embedding
// Returns embedding and token count
func (b *ModelBundle) EncodeQuery(query string) ([]float64, int64, error) {
	if b.modelType != entity.ModelTypeEmbedding {
		return nil, 0, fmt.Errorf("model type %s does not support encode query", b.modelType)
	}

	embeddingModel, ok := b.model.(entity.EmbeddingModel)
	if !ok {
		return nil, 0, fmt.Errorf("model is not an embedding model")
	}

	embeddings, err := embeddingModel.Encode(&b.modelName, []string{query}, b.apiConfig, b.embeddingConfig)
	if err != nil {
		return nil, 0, err
	}
	if len(embeddings) == 0 {
		return nil, 0, fmt.Errorf("no embedding returned")
	}

	// TODO: Calculate actual token count
	tokenCount := int64(len(query) / 4)

	return embeddings[0], tokenCount, nil
}

// Chat sends a chat message and returns response
func (b *ModelBundle) Chat(system string, history []map[string]string, genConf map[string]interface{}) (string, int64, error) {
	if b.modelType != entity.ModelTypeChat {
		return "", 0, fmt.Errorf("model type %s does not support chat", b.modelType)
	}

	chatModel, ok := b.model.(entity.ChatModel)
	if !ok {
		return "", 0, fmt.Errorf("model is not a chat model")
	}

	response, err := chatModel.Chat(system, history, genConf)
	if err != nil {
		return "", 0, err
	}

	// TODO: Calculate actual token count
	tokenCount := int64(len(response) / 4)

	return response, tokenCount, nil
}

// Rerank calculates similarity between query and texts
func (b *ModelBundle) Rerank(query string, texts []string) ([]float64, int64, error) {
	if b.modelType != entity.ModelTypeRerank {
		return nil, 0, fmt.Errorf("model type %s does not support rerank", b.modelType)
	}

	rerankModel, ok := b.model.(entity.RerankModel)
	if !ok {
		return nil, 0, fmt.Errorf("model is not a rerank model")
	}

	similarities, err := rerankModel.Rerank(query, texts, b.apiConfig)
	if err != nil {
		return nil, 0, err
	}

	// TODO: Calculate actual token count
	tokenCount := int64(len(query)/4) + int64(len(texts)*10)

	return similarities, tokenCount, nil
}

// GetModel returns the underlying model instance
func (b *ModelBundle) GetModel() interface{} {
	return b.model
}
