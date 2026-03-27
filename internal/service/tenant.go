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
	"context"
	"fmt"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/model"
	"ragflow/internal/engine"
)

// TenantService tenant service
type TenantService struct {
	tenantDAO     *dao.TenantDAO
	userTenantDAO *dao.UserTenantDAO
	docEngine     engine.DocEngine
}

// NewTenantService create tenant service
func NewTenantService() *TenantService {
	return &TenantService{
		tenantDAO:     dao.NewTenantDAO(),
		userTenantDAO: dao.NewUserTenantDAO(),
		docEngine:     engine.Get(),
	}
}

// TenantInfoResponse tenant information response
type TenantInfoResponse struct {
	TenantID  string  `json:"tenant_id"`
	Name      *string `json:"name,omitempty"`
	LLMID     string  `json:"llm_id"`
	EmbDID    string  `json:"embd_id"`
	RerankID  string  `json:"rerank_id"`
	ASRID     string  `json:"asr_id"`
	Img2TxtID string  `json:"img2txt_id"`
	TTSID     *string `json:"tts_id,omitempty"`
	ParserIDs string  `json:"parser_ids"`
	Role      string  `json:"role"`
}

// GetTenantInfo get tenant information for the current user (owner tenant)
func (s *TenantService) GetTenantInfo(userID string) (*TenantInfoResponse, error) {
	tenantInfos, err := s.tenantDAO.GetInfoByUserID(userID)
	if err != nil {
		return nil, err
	}
	if len(tenantInfos) == 0 {
		return nil, nil // No tenant found (should not happen for valid user)
	}
	// Return the first tenant (should be only one owner tenant per user)
	ti := tenantInfos[0]
	return &TenantInfoResponse{
		TenantID:  ti.TenantID,
		Name:      ti.Name,
		LLMID:     ti.LLMID,
		EmbDID:    ti.EmbDID,
		RerankID:  ti.RerankID,
		ASRID:     ti.ASRID,
		Img2TxtID: ti.Img2TxtID,
		TTSID:     ti.TTSID,
		ParserIDs: ti.ParserIDs,
		Role:      ti.Role,
	}, nil
}

// TenantListItem tenant list item response
type TenantListItem struct {
	TenantID     string  `json:"tenant_id"`
	Role         string  `json:"role"`
	Nickname     string  `json:"nickname"`
	Email        string  `json:"email"`
	Avatar       string  `json:"avatar"`
	UpdateDate   string  `json:"update_date"`
	DeltaSeconds float64 `json:"delta_seconds"`
}

// TenantLLMService tenant LLM service
// This service handles operations related to tenant-specific LLM configurations
type TenantLLMService struct {
	tenantLLMDAO *dao.TenantLLMDAO
}

// NewTenantLLMService creates a new TenantLLMService instance
func NewTenantLLMService() *TenantLLMService {
	return &TenantLLMService{
		tenantLLMDAO: dao.NewTenantLLMDAO(),
	}
}

// GetAPIKey retrieves the tenant LLM record by tenant ID and model name
/**
 * This method splits the model name into name and factory parts using the "@" separator,
 * then queries the database for the matching tenant LLM configuration.
 *
 * Parameters:
 *   - tenantID: the unique identifier of the tenant
 *   - modelName: the model name, optionally including factory suffix (e.g., "gpt-4@OpenAI")
 *
 * Returns:
 *   - *model.TenantLLM: the tenant LLM record if found, nil otherwise
 *   - error: an error if the query fails, nil otherwise
 *
 * Example:
 *
 *	service := NewTenantLLMService()
 *
 *	// Get API key for model with factory
 *	tenantLLM, err := service.GetAPIKey("tenant-123", "gpt-4@OpenAI")
 *	if err != nil {
 *	    log.Printf("Error: %v", err)
 *	}
 *
 *	// Get API key for model without factory
 *	tenantLLM, err := service.GetAPIKey("tenant-123", "gpt-4")
 */
func (s *TenantLLMService) GetAPIKey(tenantID, modelName string) (*model.TenantLLM, error) {
	modelName, factory := s.SplitModelNameAndFactory(modelName)

	var tenantLLM *model.TenantLLM
	var err error

	if factory == "" {
		tenantLLM, err = s.tenantLLMDAO.GetByTenantIDAndLLMName(tenantID, modelName)
	} else {
		tenantLLM, err = s.tenantLLMDAO.GetByTenantIDLLMNameAndFactory(tenantID, modelName, factory)
	}

	if err != nil {
		return nil, err
	}

	return tenantLLM, nil
}

// SplitModelNameAndFactory splits a model name into name and factory parts
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
/**
 * This method iterates through a predefined list of LLM-related parameter keys (llm_id, embd_id,
 * asr_id, img2txt_id, rerank_id, tts_id) and automatically populates the corresponding tenant_*
 * fields (tenant_llm_id, tenant_embd_id, etc.) with the tenant LLM record IDs.
 *
 * If a parameter key exists and its corresponding tenant_* key doesn't exist, this method will:
 *  1. Query the tenant LLM record using GetAPIKey
 *  2. If found, set the tenant_* key to the record's ID
 *  3. If not found, set the tenant_* key to 0
 *
 * Parameters:
 *   - tenantID: the unique identifier of the tenant
 *   - params: a map of parameters to be updated (will be modified in place)
 *
 * Returns:
 *   - map[string]interface{}: the updated parameters map (same as input, modified in place)
 *
 * Example:
 *
 *	service := NewTenantLLMService()
 *	params := map[string]interface{}{
 *	    "llm_id": "gpt-4@OpenAI",
 *	    "embd_id": "text-embedding-3-small@OpenAI",
 *	}
 *	result := service.EnsureTenantModelIDForParams("tenant-123", params)
 *	// result will contain:
 *	// {
 *	//     "llm_id": "gpt-4@OpenAI",
 *	//     "embd_id": "text-embedding-3-small@OpenAI",
 *	//     "tenant_llm_id": 123,    // ID from tenant_llm table
 *	//     "tenant_embd_id": 456,   // ID from tenant_llm table
 *	// }
 */
func (s *TenantLLMService) EnsureTenantModelIDForParams(tenantID string, params map[string]interface{}) map[string]interface{} {
	paramKeys := []string{"llm_id", "embd_id", "asr_id", "img2txt_id", "rerank_id", "tts_id"}

	for _, key := range paramKeys {
		tenantKey := "tenant_" + key

		if value, exists := params[key]; exists && value != nil && value != "" {
			if _, tenantExists := params[tenantKey]; !tenantExists {
				modelName, ok := value.(string)
				if !ok || modelName == "" {
					continue
				}

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

// GetTenantList get tenant list for a user
func (s *TenantService) GetTenantList(userID string) ([]*TenantListItem, error) {
	tenants, err := s.userTenantDAO.GetTenantsByUserID(userID)
	if err != nil {
		return nil, err
	}

	result := make([]*TenantListItem, len(tenants))
	now := time.Now()

	for i, t := range tenants {
		// Parse update_date and calculate delta_seconds
		var deltaSeconds float64
		if t.UpdateDate != "" {
			if updateTime, err := time.Parse("2006-01-02 15:04:05", t.UpdateDate); err == nil {
				deltaSeconds = now.Sub(updateTime).Seconds()
			}
		}

		result[i] = &TenantListItem{
			TenantID:     t.TenantID,
			Role:         t.Role,
			Nickname:     t.Nickname,
			Email:        t.Email,
			Avatar:       t.Avatar,
			UpdateDate:   t.UpdateDate,
			DeltaSeconds: deltaSeconds,
		}
	}

	return result, nil
}

// CreateDocMetaIndex creates the document metadata index for a tenant
func (s *TenantService) CreateDocMetaIndex(tenantID string) (common.ErrorCode, error) {
	// Build index name: ragflow_doc_meta_<tenant_id>
	indexName := fmt.Sprintf("ragflow_doc_meta_%s", tenantID)

	// Call document engine to create doc meta index
	err := s.docEngine.CreateDocMetaIndex(context.Background(), indexName)
	if err != nil {
		return common.CodeServerError, fmt.Errorf("failed to create doc meta index: %w", err)
	}

	return common.CodeSuccess, nil
}

// DeleteDocMetaIndex deletes the document metadata index for a tenant
func (s *TenantService) DeleteDocMetaIndex(tenantID string) (common.ErrorCode, error) {
	// Build index name: ragflow_doc_meta_<tenant_id>
	indexName := fmt.Sprintf("ragflow_doc_meta_%s", tenantID)

	// Call document engine to delete doc meta index
	err := s.docEngine.DeleteIndex(context.Background(), indexName)
	if err != nil {
		return common.CodeServerError, fmt.Errorf("failed to delete doc meta index: %w", err)
	}

	return common.CodeSuccess, nil
}
