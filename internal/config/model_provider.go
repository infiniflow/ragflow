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

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// ModelProvider represents a model provider configuration
type ModelProvider struct {
	Name                string `json:"name"`
	Logo                string `json:"logo"`
	Tags                string `json:"tags"`
	Status              string `json:"status"`
	Rank                string `json:"rank"`
	LLMs                []LLM  `json:"llm"`
	DefaultEmbeddingURL string `json:"default_embedding_url,omitempty"`
}

// LLM represents a language model within a provider
type LLM struct {
	LLMName   string `json:"llm_name"`
	Tags      string `json:"tags"`
	MaxTokens int    `json:"max_tokens"`
	ModelType string `json:"model_type"`
	IsTools   bool   `json:"is_tools"`
}

var (
	modelProviders     []ModelProvider
	modelProviderMap   map[string]int // name -> index in modelProviders slice
	modelProvidersOnce sync.Once
	modelProvidersErr  error
)

// LoadModelProviders loads model providers from JSON file.
// If path is empty, it defaults to "conf/model_providers.json" relative to current working directory.
func LoadModelProviders(path string) error {
	modelProvidersOnce.Do(func() {
		if path == "" {
			path = "conf/model_providers.json"
		}

		data, err := os.ReadFile(path)
		if err != nil {
			modelProvidersErr = fmt.Errorf("failed to read model providers file %s: %w", path, err)
			return
		}

		var root struct {
			Providers []ModelProvider `json:"model_providers"`
		}
		if err := json.Unmarshal(data, &root); err != nil {
			modelProvidersErr = fmt.Errorf("failed to unmarshal model providers JSON: %w", err)
			return
		}

		modelProviders = root.Providers
		// Build name to index map for fast lookup
		modelProviderMap = make(map[string]int, len(modelProviders))
		for i, provider := range modelProviders {
			modelProviderMap[provider.Name] = i
		}
	})

	return modelProvidersErr
}

// GetModelProviders returns the loaded model providers.
// Call LoadModelProviders first, otherwise returns empty slice.
func GetModelProviders() []ModelProvider {
	return modelProviders
}

// GetModelProviderByName returns the model provider with the given name.
func GetModelProviderByName(name string) *ModelProvider {
	if modelProviderMap == nil {
		return nil
	}
	if idx, ok := modelProviderMap[name]; ok {
		return &modelProviders[idx]
	}
	return nil
}

// GetLLMByProviderAndName returns the LLM with the given provider name and model name.
func GetLLMByProviderAndName(providerName, modelName string) *LLM {
	provider := GetModelProviderByName(providerName)
	if provider == nil {
		return nil
	}
	for i := range provider.LLMs {
		if provider.LLMs[i].LLMName == modelName {
			return &provider.LLMs[i]
		}
	}
	return nil
}
