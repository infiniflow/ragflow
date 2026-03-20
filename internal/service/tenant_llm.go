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
	"ragflow/internal/model"
)

// TenantLLMService tenant LLM service
// This service handles operations related to tenant-specific LLM configurations
type TenantLLMService struct {
	tenantLLMDAO *dao.TenantLLMDAO
}

// NewTenantLLMService creates a new TenantLLMService instance
//
// Returns:
//   - *TenantLLMService: a new TenantLLMService instance
//
// Example:
//
//	service := NewTenantLLMService()
//	tenantLLM, err := service.GetAPIKey("tenant-123", "gpt-4@OpenAI")
func NewTenantLLMService() *TenantLLMService {
	return &TenantLLMService{
		tenantLLMDAO: dao.NewTenantLLMDAO(),
	}
}

// GetAPIKey retrieves the tenant LLM record by tenant ID and model name
//
// This method splits the model name into name and factory parts using the "@" separator,
// then queries the database for the matching tenant LLM configuration.
//
// Parameters:
//   - tenantID: the unique identifier of the tenant
//   - modelName: the model name, optionally including factory suffix (e.g., "gpt-4@OpenAI")
//
// Returns:
//   - *model.TenantLLM: the tenant LLM record if found, nil otherwise
//   - error: an error if the query fails, nil otherwise
//
// Example:
//
//	service := NewTenantLLMService()
//
//	// Get API key for model with factory
//	tenantLLM, err := service.GetAPIKey("tenant-123", "gpt-4@OpenAI")
//	if err != nil {
//	    log.Printf("Error: %v", err)
//	}
//
//	// Get API key for model without factory
//	tenantLLM, err := service.GetAPIKey("tenant-123", "gpt-4")
func (s *TenantLLMService) GetAPIKey(tenantID, modelName string) (*model.TenantLLM, error) {
	modelName, factory := s.SplitModelNameAndFactory(modelName)

	var tenantLLM *model.TenantLLM
	var err error

	if factory == "" {
		// Query without factory
		tenantLLM, err = s.tenantLLMDAO.GetByTenantIDAndLLMName(tenantID, modelName)
	} else {
		// Query with factory
		tenantLLM, err = s.tenantLLMDAO.GetByTenantIDLLMNameAndFactory(tenantID, modelName, factory)
	}

	if err != nil {
		return nil, err
	}

	return tenantLLM, nil
}

// SplitModelNameAndFactory splits a model name into name and factory parts
//
// The model name can include a factory suffix separated by "@" (e.g., "gpt-4@OpenAI").
// This method extracts both parts and returns them separately.
//
// Parameters:
//   - modelName: the model name, optionally including factory suffix
//
// Returns:
//   - string: the model name without factory suffix
//   - string: the factory name, or empty string if not present
//
// Example:
//
//	service := NewTenantLLMService()
//
//	// Model with factory
//	name, factory := service.SplitModelNameAndFactory("gpt-4@OpenAI")
//	// name = "gpt-4", factory = "OpenAI"
//
//	// Model without factory
//	name, factory := service.SplitModelNameAndFactory("gpt-4")
//	// name = "gpt-4", factory = ""
func (s *TenantLLMService) SplitModelNameAndFactory(modelName string) (string, string) {
	arr := strings.Split(modelName, "@")
	if len(arr) < 2 {
		return modelName, ""
	}
	if len(arr) > 2 {
		return strings.Join(arr[0:len(arr)-1], "@"), arr[len(arr)-1]
	}
	return arr[0], arr[1]
}

// EnsureTenantModelIDForParams ensures tenant model IDs are populated for LLM-related parameters
//
// This method iterates through a predefined list of LLM-related parameter keys (llm_id, embd_id,
// asr_id, img2txt_id, rerank_id, tts_id) and automatically populates the corresponding tenant_*
// fields (tenant_llm_id, tenant_embd_id, etc.) with the tenant LLM record IDs.
//
// If a parameter key exists and its corresponding tenant_* key doesn't exist, this method will:
//  1. Query the tenant LLM record using GetAPIKey
//  2. If found, set the tenant_* key to the record's ID
//  3. If not found, set the tenant_* key to 0
//
// Parameters:
//   - tenantID: the unique identifier of the tenant
//   - params: a map of parameters to be updated (will be modified in place)
//
// Returns:
//   - map[string]interface{}: the updated parameters map (same as input, modified in place)
//
// Example:
//
//	service := NewTenantLLMService()
//	params := map[string]interface{}{
//	    "llm_id": "gpt-4@OpenAI",
//	    "embd_id": "text-embedding-3-small@OpenAI",
//	}
//	result := service.EnsureTenantModelIDForParams("tenant-123", params)
//	// result will contain:
//	// {
//	//     "llm_id": "gpt-4@OpenAI",
//	//     "embd_id": "text-embedding-3-small@OpenAI",
//	//     "tenant_llm_id": 123,    // ID from tenant_llm table
//	//     "tenant_embd_id": 456,   // ID from tenant_llm table
//	// }
func (s *TenantLLMService) EnsureTenantModelIDForParams(tenantID string, params map[string]interface{}) map[string]interface{} {
	// Define the list of LLM-related parameter keys
	paramKeys := []string{"llm_id", "embd_id", "asr_id", "img2txt_id", "rerank_id", "tts_id"}

	for _, key := range paramKeys {
		tenantKey := "tenant_" + key

		// Check if the parameter exists and tenant_* key doesn't exist
		if value, exists := params[key]; exists && value != nil && value != "" {
			if _, tenantExists := params[tenantKey]; !tenantExists {
				// Get model name as string
				modelName, ok := value.(string)
				if !ok || modelName == "" {
					continue
				}

				// Query tenant LLM record
				tenantLLM, err := s.GetAPIKey(tenantID, modelName)
				if err == nil && tenantLLM != nil {
					params[tenantKey] = tenantLLM.ID
				} else {
					params[tenantKey] = int64(0)
				}
			}
		}
	}

	return params
}
