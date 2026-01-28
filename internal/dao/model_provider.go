package dao

import (
	"sync"
	
	"ragflow/internal/config"
)

// ModelProviderDAO provides access to model provider configuration data
type ModelProviderDAO struct{}

var (
	modelProviderDAOInstance *ModelProviderDAO
	modelProviderDAOOnce     sync.Once
)

// NewModelProviderDAO creates a new ModelProviderDAO instance (singleton)
func NewModelProviderDAO() *ModelProviderDAO {
	modelProviderDAOOnce.Do(func() {
		modelProviderDAOInstance = &ModelProviderDAO{}
	})
	return modelProviderDAOInstance
}

// GetAllProviders returns all model providers
func (dao *ModelProviderDAO) GetAllProviders() []config.ModelProvider {
	return config.GetModelProviders()
}

// GetProviderByName returns the model provider with the given name
func (dao *ModelProviderDAO) GetProviderByName(name string) *config.ModelProvider {
	return config.GetModelProviderByName(name)
}

// GetLLMByProviderAndName returns the LLM with the given provider name and model name
func (dao *ModelProviderDAO) GetLLMByProviderAndName(providerName, modelName string) *config.LLM {
	return config.GetLLMByProviderAndName(providerName, modelName)
}

// GetLLMsByType returns all LLMs across all providers that match the given model type
func (dao *ModelProviderDAO) GetLLMsByType(modelType string) []config.LLM {
	var result []config.LLM
	for _, provider := range config.GetModelProviders() {
		for _, llm := range provider.LLMs {
			if llm.ModelType == modelType {
				result = append(result, llm)
			}
		}
	}
	return result
}

// GetProvidersByTag returns providers that have the given tag in their tags string
func (dao *ModelProviderDAO) GetProvidersByTag(tag string) []config.ModelProvider {
	var result []config.ModelProvider
	for _, provider := range config.GetModelProviders() {
		if containsTag(provider.Tags, tag) {
			result = append(result, provider)
		}
	}
	return result
}

// GetLLMsByProviderAndType returns LLMs for a specific provider that match the given model type
func (dao *ModelProviderDAO) GetLLMsByProviderAndType(providerName, modelType string) []config.LLM {
	provider := config.GetModelProviderByName(providerName)
	if provider == nil {
		return nil
	}
	var result []config.LLM
	for _, llm := range provider.LLMs {
		if llm.ModelType == modelType {
			result = append(result, llm)
		}
	}
	return result
}

// helper function to check if a comma-separated tag string contains a specific tag
func containsTag(tags, tag string) bool {
	// Simple implementation: check substring with boundaries
	// Assuming tags are uppercase and comma-separated without spaces
	// This may need refinement based on actual tag format
	for _, t := range splitTags(tags) {
		if t == tag {
			return true
		}
	}
	return false
}

func splitTags(tags string) []string {
	// Split by comma and trim spaces
	var result []string
	start := 0
	for i, ch := range tags {
		if ch == ',' {
			if start < i {
				result = append(result, tags[start:i])
			}
			start = i + 1
		}
	}
	if start < len(tags) {
		result = append(result, tags[start:])
	}
	return result
}