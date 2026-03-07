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
	"strings"

	"ragflow/internal/dao"
)

// LLMService LLM service
type LLMService struct {
	tenantLLMDAO *dao.TenantLLMDAO
	llmDAO       *dao.LLMDAO
}

// NewLLMService create LLM service
func NewLLMService() *LLMService {
	return &LLMService{
		tenantLLMDAO: dao.NewTenantLLMDAO(),
		llmDAO:       dao.NewLLMDAO(),
	}
}

// MyLLMItem represents a single LLM item in the response
type MyLLMItem struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	UsedToken int64  `json:"used_token"`
	Status    string `json:"status"`
	APIBase   string `json:"api_base,omitempty"`
	MaxTokens int64  `json:"max_tokens,omitempty"`
}

// MyLLMResponse represents the response structure for my LLMs
type MyLLMResponse struct {
	Tags string      `json:"tags"`
	LLM  []MyLLMItem `json:"llm"`
}

// GetMyLLMs get my LLMs for a tenant
func (s *LLMService) GetMyLLMs(tenantID string, includeDetails bool) (map[string]MyLLMResponse, error) {
	// Get LLM list from database
	myLLMs, err := s.tenantLLMDAO.GetMyLLMs(tenantID, includeDetails)
	if err != nil {
		return nil, err
	}

	// Group by factory
	result := make(map[string]MyLLMResponse)
	providerDAO := dao.NewModelProviderDAO()
	for _, llm := range myLLMs {
		// Get or create factory entry
		resp, exists := result[llm.LLMFactory]
		if !exists {
			resp = MyLLMResponse{
				Tags: llm.Tags,
				LLM:  []MyLLMItem{},
			}
		}

		// Create LLM item
		item := MyLLMItem{
			Type:      llm.ModelType,
			Name:      llm.LLMName,
			UsedToken: llm.UsedTokens,
			Status:    llm.Status,
		}

		// Add detailed fields if requested
		if includeDetails {
			item.APIBase = llm.APIBase
			item.MaxTokens = llm.MaxTokens

			// If APIBase is empty, try to get from model provider configuration
			if item.APIBase == "" {
				provider := providerDAO.GetProviderByName(llm.LLMFactory)
				if provider != nil {
					// Determine appropriate API base URL based on model type
					switch llm.ModelType {
					case "embedding":
						if provider.DefaultEmbeddingURL != "" {
							item.APIBase = provider.DefaultEmbeddingURL
						}
						// Add other model types here if needed
						// case "chat":
						// case "rerank":
						// etc.
					}
				}
			}
		}

		resp.LLM = append(resp.LLM, item)
		result[llm.LLMFactory] = resp
	}

	return result, nil
}

// LLMListItem represents a single LLM item in the list response
type LLMListItem struct {
	LLMName    string  `json:"llm_name"`
	ModelType  string  `json:"model_type"`
	FID        string  `json:"fid"`
	Available  bool    `json:"available"`
	Status     string  `json:"status"`
	MaxTokens  int64   `json:"max_tokens,omitempty"`
	CreateDate *string `json:"create_date,omitempty"`
	CreateTime *int64  `json:"create_time,omitempty"`
	UpdateDate *string `json:"update_date,omitempty"`
	UpdateTime *int64  `json:"update_time,omitempty"`
	IsTools    bool    `json:"is_tools"`
	Tags       string  `json:"tags,omitempty"`
}

// ListLLMsResponse represents the response for list LLMs
type ListLLMsResponse map[string][]LLMListItem

// ListLLMs lists LLMs for a tenant with availability info
func (s *LLMService) ListLLMs(tenantID string, modelType string) (ListLLMsResponse, error) {
	selfDeployed := map[string]bool{
		"FastEmbed":  true,
		"Ollama":     true,
		"Xinference": true,
		"LocalAI":    true,
		"LM-Studio":  true,
		"GPUStack":   true,
	}

	// Get tenant LLMs
	tenantLLMs, err := s.tenantLLMDAO.ListAllByTenant(tenantID)
	if err != nil {
		return nil, err
	}

	// Build set of factories with valid API keys
	facts := make(map[string]bool)
	// Build set of valid LLM names@factories
	status := make(map[string]bool)
	for _, tl := range tenantLLMs {
		if tl.APIKey != nil && *tl.APIKey != "" && tl.Status == "1" {
			facts[tl.LLMFactory] = true
		}
		llmName := ""
		if tl.LLMName != nil {
			llmName = *tl.LLMName
		}
		key := llmName + "@" + tl.LLMFactory
		if tl.Status == "1" {
			status[key] = true
		}
	}

	// Get all valid LLMs
	allLLMs, err := s.llmDAO.GetAllValid()
	if err != nil {
		return nil, err
	}

	// Filter and build result
	llmSet := make(map[string]bool)
	result := make(ListLLMsResponse)

	for _, llm := range allLLMs {
		if llm.Status == nil || *llm.Status != "1" {
			continue
		}

		key := llm.LLMName + "@" + llm.FID

		// Check if valid (Builtin factory or in status set)
		if llm.FID != "Builtin" && !status[key] {
			continue
		}

		// Filter by model type if specified
		if modelType != "" && !strings.Contains(llm.ModelType, modelType) {
			continue
		}

		// Determine availability
		available := facts[llm.FID] || selfDeployed[llm.FID] || llm.LLMName == "flag-embedding"

		item := LLMListItem{
			LLMName:   llm.LLMName,
			ModelType: llm.ModelType,
			FID:       llm.FID,
			Available: available,
			Status:    "1",
			MaxTokens: llm.MaxTokens,
			IsTools:   llm.IsTools,
			Tags:      llm.Tags,
		}

		// Add BaseModel fields
		if llm.CreateDate != nil {
			createDateStr := llm.CreateDate.Format("2006-01-02T15:04:05")
			item.CreateDate = &createDateStr
		}
		item.CreateTime = llm.CreateTime
		if llm.UpdateDate != nil {
			updateDateStr := llm.UpdateDate.Format("2006-01-02T15:04:05")
			item.UpdateDate = &updateDateStr
		}
		if llm.UpdateTime != nil {
			item.UpdateTime = llm.UpdateTime
		}

		result[llm.FID] = append(result[llm.FID], item)
		llmSet[key] = true
	}

	// Add tenant LLMs that are not in the global list
	for _, tl := range tenantLLMs {
		llmName := ""
		if tl.LLMName != nil {
			llmName = *tl.LLMName
		}
		key := llmName + "@" + tl.LLMFactory
		if llmSet[key] {
			continue
		}

		// Filter by model type if specified
		modelTypeValue := ""
		if tl.ModelType != nil {
			modelTypeValue = *tl.ModelType
		}
		if modelType != "" && !strings.Contains(modelTypeValue, modelType) {
			continue
		}

		item := LLMListItem{
			LLMName:   llmName,
			ModelType: modelTypeValue,
			FID:       tl.LLMFactory,
			Available: true,
			Status:    tl.Status,
		}

		result[tl.LLMFactory] = append(result[tl.LLMFactory], item)
	}

	return result, nil
}
