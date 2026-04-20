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
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	"strings"
)

// TenantService tenant service
type TenantService struct {
	tenantDAO            *dao.TenantDAO
	userTenantDAO        *dao.UserTenantDAO
	modelProviderDAO     *dao.TenantModelProviderDAO
	modelInstanceDAO     *dao.TenantModelInstanceDAO
	modelDAO             *dao.TenantModelDAO
	modelGroupDAO        *dao.TenantModelGroupDAO
	modelGroupMappingDAO *dao.TenantModelGroupMappingDAO
	docEngine            engine.DocEngine
}

// NewTenantService create tenant service
func NewTenantService() *TenantService {
	return &TenantService{
		tenantDAO:            dao.NewTenantDAO(),
		userTenantDAO:        dao.NewUserTenantDAO(),
		modelProviderDAO:     dao.NewTenantModelProviderDAO(),
		modelInstanceDAO:     dao.NewTenantModelInstanceDAO(),
		modelDAO:             dao.NewTenantModelDAO(),
		modelGroupDAO:        dao.NewTenantModelGroupDAO(),
		modelGroupMappingDAO: dao.NewTenantModelGroupMappingDAO(),
		docEngine:            engine.Get(),
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
func (s *TenantLLMService) GetAPIKey(tenantID, modelName string) (*entity.TenantLLM, error) {
	modelName, factory := s.SplitModelNameAndFactory(modelName)

	var tenantLLM *entity.TenantLLM
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

	for i, t := range tenants {
		// Parse update_date and calculate delta_seconds
		var deltaSeconds float64
		if t.UpdateDate != "" {
			deltaSeconds, err = common.DeltaSeconds(t.UpdateDate)
			if err != nil {
				return nil, err
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

// CreateMetadataInDocEngine creates the document metadata table for a tenant
func (s *TenantService) CreateMetadataInDocEngine(tenantID string) (common.ErrorCode, error) {
	// Build table name: ragflow_doc_meta_<tenant_id>
	tableName := fmt.Sprintf("ragflow_doc_meta_%s", tenantID)

	// Call document engine to create doc meta table
	err := s.docEngine.CreateMetadata(context.Background(), tableName)
	if err != nil {
		return common.CodeServerError, fmt.Errorf("failed to create metadata table: %w", err)
	}

	return common.CodeSuccess, nil
}

// DeleteMetadataInDocEngine deletes the document metadata table for a tenant
func (s *TenantService) DeleteMetadataInDocEngine(tenantID string) (common.ErrorCode, error) {
	// Build table name: ragflow_doc_meta_<tenant_id>
	tableName := fmt.Sprintf("ragflow_doc_meta_%s", tenantID)

	// Call document engine to delete doc meta table
	err := s.docEngine.DropTable(context.Background(), tableName)
	if err != nil {
		return common.CodeServerError, fmt.Errorf("failed to delete doc meta table: %w", err)
	}

	return common.CodeSuccess, nil
}

type ModelItem struct {
	ModelProvider *string `json:"model_provider"`
	ModelInstance *string `json:"model_instance"`
	ModelName     *string `json:"model_name"`
	ModelType     string  `json:"model_type"`
	Enable        bool    `json:"enable"`
}

type DefaultModelResponse struct {
	Models []ModelItem `json:"models,omitempty"`
}

func (s *TenantService) GetModelInfo(tenantID string, defaultModel string, modelType string) (*string, *string, *string, bool, error) {
	// normally the model string is: modelName@instanceName@providerName, sometimes it's just modelName@providerName
	// for the 1st case, parse defaultChatModel into three parts
	defaultChatModelParts := strings.Split(defaultModel, "@")
	var providerName *string
	var instanceName *string
	var modelName *string
	if len(defaultChatModelParts) == 3 {
		providerName = &defaultChatModelParts[2]
		instanceName = &defaultChatModelParts[1]
		modelName = &defaultChatModelParts[0]

	} else if len(defaultChatModelParts) == 2 {
		providerName = &defaultChatModelParts[1]
		instanceName = new(string)
		*instanceName = "default"
		modelName = &defaultChatModelParts[0]
	} else {
		return nil, nil, nil, false, fmt.Errorf("invalid model string: %s", defaultModel)
	}

	if modelType == "ocr" {
		if *providerName == "infiniflow" && *instanceName == "default" && *modelName == "deepdoc" {
			return providerName, instanceName, modelName, true, nil
		}
	}

	// Check if the provider and instance exists
	modelProvider, err := s.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, *providerName)
	if err != nil {
		return nil, nil, nil, false, err
	}

	modelInstance, err := s.modelInstanceDAO.GetByProviderIDAndInstanceName(modelProvider.ID, *instanceName)
	if err != nil {
		return nil, nil, nil, false, err
	}

	modelSchema, err := dao.GetModelProviderManager().GetModelByName(*providerName, *modelName)
	if err != nil {
		return nil, nil, nil, false, err
	}

	if !modelSchema.ModelTypeMap[modelType] {
		return nil, nil, nil, false, fmt.Errorf("model %s isn't a chat model", *modelName)
	}

	var modelEntity *entity.TenantModel
	modelEntity, err = s.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(modelProvider.ID, modelInstance.ID, *modelName)
	if err != nil {
		errString := err.Error()
		if !strings.Contains(errString, "record not found") {
			return nil, nil, nil, false, err
		}
	}

	enable := modelEntity == nil

	return providerName, instanceName, modelName, enable, nil

}

func (s *TenantService) ListTenantDefaultModels(userID string) ([]ModelItem, error) {

	tenantInfos, err := s.tenantDAO.GetInfoByUserID(userID)
	if err != nil {
		return nil, err
	}
	if len(tenantInfos) == 0 {
		return nil, nil // No tenant found (should not happen for valid user)
	}

	ownedTenant := tenantInfos[0]

	var result []ModelItem

	defaultChatModelProvider, defaultChatModelInstance, defaultChatModelName, defaultChatModelEnable, err := s.GetModelInfo(ownedTenant.TenantID, ownedTenant.LLMID, "chat")
	if err == nil {
		result = append(result, ModelItem{
			ModelProvider: defaultChatModelProvider,
			ModelInstance: defaultChatModelInstance,
			ModelName:     defaultChatModelName,
			ModelType:     "llm",
			Enable:        defaultChatModelEnable,
		})
	}

	defaultEmbeddingModelProvider, defaultEmbeddingModelInstance, defaultEmbeddingModelName, defaultEmbeddingModelEnable, err := s.GetModelInfo(ownedTenant.TenantID, ownedTenant.EmbDID, "embedding")
	if err == nil {
		result = append(result, ModelItem{
			ModelProvider: defaultEmbeddingModelProvider,
			ModelInstance: defaultEmbeddingModelInstance,
			ModelName:     defaultEmbeddingModelName,
			ModelType:     "embedding",
			Enable:        defaultEmbeddingModelEnable,
		})
	}

	defaultRerankModelProvider, defaultRerankModelInstance, defaultRerankModelName, defaultRerankModelEnable, err := s.GetModelInfo(ownedTenant.TenantID, ownedTenant.RerankID, "rerank")
	if err == nil {
		result = append(result, ModelItem{
			ModelProvider: defaultRerankModelProvider,
			ModelInstance: defaultRerankModelInstance,
			ModelName:     defaultRerankModelName,
			ModelType:     "rerank",
			Enable:        defaultRerankModelEnable,
		})
	}

	defaultASRModelProvider, defaultASRModelInstance, defaultASRModelName, defaultASREnable, err := s.GetModelInfo(ownedTenant.TenantID, ownedTenant.ASRID, "asr")
	if err == nil {
		result = append(result, ModelItem{
			ModelProvider: defaultASRModelProvider,
			ModelInstance: defaultASRModelInstance,
			ModelName:     defaultASRModelName,
			ModelType:     "asr",
			Enable:        defaultASREnable,
		})
	}

	defaultImage2TextModelProvider, defaultImage2TextModelInstance, defaultImage2TextModelName, defaultImage2TextModelEnable, err := s.GetModelInfo(ownedTenant.TenantID, ownedTenant.Img2TxtID, "image2text")
	if err == nil {
		result = append(result, ModelItem{
			ModelProvider: defaultImage2TextModelProvider,
			ModelInstance: defaultImage2TextModelInstance,
			ModelName:     defaultImage2TextModelName,
			ModelType:     "image2text",
			Enable:        defaultImage2TextModelEnable,
		})
	}

	defaultOCRModelProvider, defaultOCRModelInstance, defaultOCRModelName, defaultOCRModelEnable, err := s.GetModelInfo(ownedTenant.TenantID, ownedTenant.OCRID, "ocr")
	if err == nil {
		result = append(result, ModelItem{
			ModelProvider: defaultOCRModelProvider,
			ModelInstance: defaultOCRModelInstance,
			ModelName:     defaultOCRModelName,
			ModelType:     "ocr",
			Enable:        defaultOCRModelEnable,
		})
	}

	if ownedTenant.TTSID == nil {
		return result, nil
	}

	defaultTTSModelProvider, defaultTTSModelInstance, defaultTTSModelName, defaultTTSModelEnable, err := s.GetModelInfo(ownedTenant.TenantID, *ownedTenant.TTSID, "tts")
	if err == nil {
		result = append(result, ModelItem{
			ModelProvider: defaultTTSModelProvider,
			ModelInstance: defaultTTSModelInstance,
			ModelName:     defaultTTSModelName,
			ModelType:     "tts",
			Enable:        defaultTTSModelEnable,
		})
	}

	return result, nil
}

func (s *TenantService) checkModelAvailable(tenantID, providerName, instanceName, modelName, modelType string) error {
	// Check if the provider and instance exists
	modelProvider, err := s.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return err
	}

	modelInstance, err := s.modelInstanceDAO.GetByProviderIDAndInstanceName(modelProvider.ID, instanceName)
	if err != nil {
		return err
	}

	modelSchema, err := dao.GetModelProviderManager().GetModelByName(providerName, modelName)
	if err != nil {
		return err
	}

	if !modelSchema.ModelTypeMap[modelType] {
		return fmt.Errorf("model %s isn't a chat model", modelName)
	}

	var modelEntity *entity.TenantModel
	modelEntity, err = s.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(modelProvider.ID, modelInstance.ID, modelName)
	if err != nil || modelEntity != nil {
		var errString = err.Error()
		if errString == "record not found" {
			return nil
		}
		return fmt.Errorf("model %s isn't available", modelName)
	}

	return nil
}

func (s *TenantService) SetTenantDefaultModels(userID, modelProvider, modelInstance, modelName, modelType string) error {

	tenantInfos, err := s.tenantDAO.GetInfoByUserID(userID)
	if err != nil {
		return err
	}
	if len(tenantInfos) == 0 {
		return nil // No tenant found (should not happen for valid user)
	}

	ownedTenant := tenantInfos[0]
	err = s.checkModelAvailable(ownedTenant.TenantID, modelProvider, modelInstance, modelName, modelType)
	if err != nil {
		return err
	}

	var modelTypeID string
	if modelType == "chat" {
		modelTypeID = "llm_id"
	}
	if modelType == "embedding" {
		modelTypeID = "embd_id"
	}
	if modelType == "rerank" {
		modelTypeID = "rerank_id"
	}
	if modelType == "asr" {
		modelTypeID = "asr_id"
	}
	if modelType == "image2text" {
		modelTypeID = "img2txt_id"
	}
	if modelType == "tts" {
		modelTypeID = "tts_id"
	}
	if modelTypeID == "" {
		return fmt.Errorf("model type %s is invalid", modelType)
	}

	defaultModel := fmt.Sprintf("%s@%s@%s", modelName, modelInstance, modelProvider)
	err = s.tenantDAO.Update(ownedTenant.TenantID, map[string]interface{}{
		modelTypeID: defaultModel,
	})

	return nil
}
