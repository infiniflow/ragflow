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
	"ragflow/internal/dao"
)

// LLMService LLM service
type LLMService struct {
	tenantLLMDAO *dao.TenantLLMDAO
}

// NewLLMService create LLM service
func NewLLMService() *LLMService {
	return &LLMService{
		tenantLLMDAO: dao.NewTenantLLMDAO(),
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
