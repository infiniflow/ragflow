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
	"ragflow/internal/utility"
	"strings"
)

// TenantService tenant service
type TenantService struct {
	tenantDAO            *dao.TenantDAO
	userTenantDAO        *dao.UserTenantDAO
	userDAO              *dao.UserDAO
	modelProviderDAO     *dao.TenantModelProviderDAO
	modelInstanceDAO     *dao.TenantModelInstanceDAO
	modelDAO             *dao.TenantModelDAO
	modelGroupDAO        *dao.TenantModelGroupDAO
	modelGroupMappingDAO *dao.TenantModelGroupMappingDAO
	kbDAO                *dao.KnowledgebaseDAO
	docEngine            engine.DocEngine
}

// NewTenantService create tenant service
func NewTenantService() *TenantService {
	return &TenantService{
		tenantDAO:            dao.NewTenantDAO(),
		userTenantDAO:        dao.NewUserTenantDAO(),
		userDAO:              dao.NewUserDAO(),
		modelProviderDAO:     dao.NewTenantModelProviderDAO(),
		modelInstanceDAO:     dao.NewTenantModelInstanceDAO(),
		modelDAO:             dao.NewTenantModelDAO(),
		modelGroupDAO:        dao.NewTenantModelGroupDAO(),
		modelGroupMappingDAO: dao.NewTenantModelGroupMappingDAO(),
		kbDAO:                dao.NewKnowledgebaseDAO(),
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
	TTSID     string  `json:"tts_id"`
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
	tenantLLMDAO     *dao.TenantLLMDAO
	modelProviderDAO *dao.TenantModelProviderDAO
	modelInstanceDAO *dao.TenantModelInstanceDAO
	modelDAO         *dao.TenantModelDAO
}

// NewTenantLLMService creates a new TenantLLMService instance
func NewTenantLLMService() *TenantLLMService {
	return &TenantLLMService{
		tenantLLMDAO:     dao.NewTenantLLMDAO(),
		modelProviderDAO: dao.NewTenantModelProviderDAO(),
		modelInstanceDAO: dao.NewTenantModelInstanceDAO(),
		modelDAO:         dao.NewTenantModelDAO(),
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

// GetAPIKeyFromInstance returns the API key for the given composite model name
// by looking it up in the tenant_model_instance table. compositeModelName is in
// "model@instance@provider" or "model@provider" format.
func (s *TenantLLMService) GetAPIKeyFromInstance(tenantID, compositeModelName string) (string, error) {
	parts := strings.Split(compositeModelName, "@")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid model name format: %s", compositeModelName)
	}

	var providerName, instanceName string
	switch len(parts) {
	case 2:
		instanceName = "default"
		providerName = parts[1]
	case 3:
		instanceName = parts[1]
		providerName = parts[2]
	default:
		return "", fmt.Errorf("invalid model name format: %s", compositeModelName)
	}

	provider, err := s.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return "", fmt.Errorf("provider %q not found: %w", providerName, err)
	}
	if provider == nil {
		return "", fmt.Errorf("provider %q not found", providerName)
	}

	instance, err := s.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return "", fmt.Errorf("instance %q not found: %w", instanceName, err)
	}
	if instance == nil {
		return "", fmt.Errorf("instance %q not found", instanceName)
	}

	if instance.APIKey == "" {
		return "", fmt.Errorf("no API key configured for model %s", compositeModelName)
	}
	return instance.APIKey, nil
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

// CreateMetadataStore creates the metadata store for a tenant
func (s *TenantService) CreateMetadataStore(tenantID string) (common.ErrorCode, error) {
	// Call document engine to create doc meta table
	err := s.docEngine.CreateMetadataStore(context.Background(), tenantID)
	if err != nil {
		return common.CodeServerError, fmt.Errorf("failed to create metadata table: %w", err)
	}

	return common.CodeSuccess, nil
}

// DeleteMetadataStore deletes the metadata store for a tenant
func (s *TenantService) DeleteMetadataStore(tenantID string) (common.ErrorCode, error) {
	// Call document engine to delete doc meta table
	err := s.docEngine.DropMetadataStore(context.Background(), tenantID)
	if err != nil {
		return common.CodeServerError, fmt.Errorf("failed to delete doc meta table: %w", err)
	}

	return common.CodeSuccess, nil
}

// CreateDatasetTableRequest represents the request for creating a dataset table
type CreateDatasetTableRequest struct {
	KBID       string `json:"kb_id" binding:"required"`
	VectorSize int    `json:"vector_size" binding:"required"`
	ParserID   string `json:"parser_id,omitempty"`
}

// CreateChunkStoreResponse represents the response for creating a chunk store
type CreateChunkStoreResponse struct {
	KBID       string `json:"kb_id"`
	TableName  string `json:"table_name"`
	VectorSize int    `json:"vector_size"`
}

// CreateChunkStore creates a chunk store in the document engine for a knowledge base
func (s *TenantService) CreateChunkStore(req *CreateDatasetTableRequest) (*CreateChunkStoreResponse, common.ErrorCode, error) {
	if req == nil {
		return nil, common.CodeDataError, fmt.Errorf("request is required")
	}
	// Get KB to find tenant_id for building table name
	kb, err := s.kbDAO.GetByID(req.KBID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, fmt.Errorf("knowledge base not found: %s", req.KBID)
		}
		return nil, common.CodeServerError, fmt.Errorf("failed to query knowledge base %s: %w", req.KBID, err)
	}

	// vector_size is required
	vecSize := req.VectorSize
	if vecSize <= 0 {
		return nil, common.CodeDataError, fmt.Errorf("vector_size must be positive")
	}

	// Build table name prefix: ragflow_<tenant_id>
	tableName := fmt.Sprintf("ragflow_%s", kb.TenantID)

	// Call document engine to create table
	// Full table name will be built as "{tableName}_{kb_id}"
	err = s.docEngine.CreateChunkStore(context.Background(), tableName, req.KBID, vecSize, req.ParserID)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to create dataset: %w", err)
	}

	return &CreateChunkStoreResponse{
		KBID:       req.KBID,
		TableName:  tableName,
		VectorSize: vecSize,
	}, common.CodeSuccess, nil
}

// DeleteChunkStore deletes the chunk store in the document engine for a knowledge base
func (s *TenantService) DeleteChunkStore(kbID string) (common.ErrorCode, error) {
	// Get KB to find tenant_id for building table name
	kb, err := s.kbDAO.GetByID(kbID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return common.CodeDataError, fmt.Errorf("knowledge base not found: %s", kbID)
		}
		return common.CodeServerError, fmt.Errorf("failed to query knowledge base %s: %w", kbID, err)
	}

	// Call document engine to delete table
	err = s.docEngine.DropChunkStore(context.Background(), fmt.Sprintf("ragflow_%s", kb.TenantID), kbID)
	if err != nil {
		return common.CodeServerError, fmt.Errorf("failed to delete table: %w", err)
	}

	return common.CodeSuccess, nil
}

type ModelItem struct {
	ModelProvider *string `json:"model_provider"`
	ModelInstance *string `json:"model_instance"`
	ModelName     *string `json:"model_name"`
	ModelID       string  `json:"model_id"`
	ModelType     string  `json:"model_type"`
	Enable        bool    `json:"enable"`
}

func tenantDefaultModelFields(modelType string) (string, string, entity.ModelType, error) {
	switch modelType {
	case "chat":
		return "llm_id", "tenant_llm_id", entity.ModelTypeChat, nil
	case "embedding":
		return "embd_id", "tenant_embd_id", entity.ModelTypeEmbedding, nil
	case "rerank":
		return "rerank_id", "tenant_rerank_id", entity.ModelTypeRerank, nil
	case "asr", "speech2text":
		return "asr_id", "tenant_asr_id", entity.ModelTypeSpeech2Text, nil
	case "vision", "image2text":
		return "img2txt_id", "tenant_img2txt_id", entity.ModelTypeImage2Text, nil
	case "tts":
		return "tts_id", "tenant_tts_id", entity.ModelTypeTTS, nil
	case "ocr":
		return "ocr_id", "tenant_ocr_id", entity.ModelTypeOCR, nil
	default:
		return "", "", 0, fmt.Errorf("model type %s is invalid", modelType)
	}
}

func ptrStringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func factoryModelTypeName(modelType string) string {
	switch modelType {
	case "image2text":
		return "vision"
	case "speech2text":
		return "asr"
	default:
		return modelType
	}
}

// GetDefaultModelName returns the full default model ID for a tenant and model type
// Format: modelName@instanceName@providerName or modelName@providerName
// Returns empty string if no default model is set
func (s *TenantService) GetDefaultModelName(tenantID string, modelType entity.ModelType) (string, error) {
	tenant, err := s.tenantDAO.GetByID(tenantID)
	if err != nil {
		return "", err
	}

	var modelID string
	switch modelType {
	case entity.ModelTypeChat:
		modelID = tenant.LLMID
	case entity.ModelTypeEmbedding:
		modelID = tenant.EmbdID
	case entity.ModelTypeRerank:
		modelID = tenant.RerankID
	case entity.ModelTypeSpeech2Text:
		modelID = tenant.ASRID
	case entity.ModelTypeImage2Text:
		modelID = tenant.Img2TxtID
	case entity.ModelTypeTTS:
		modelID = *tenant.TTSID
	case entity.ModelTypeOCR:
		modelID = *tenant.OCRID
	default:
		return "", fmt.Errorf("invalid model type: %s", modelType)
	}

	return modelID, nil
}

func (s *TenantService) GetModelInfo(tenantID string, defaultModel string, modelType string) (*string, *string, *string, bool, error) {
	// Mirror Python's _get_model_info: right-anchored rsplit so that model
	// names containing '@' (e.g. LM Studio IDs like
	// "text-embedding-nomic-embed-text-v1.5@q8_0") remain intact.
	// The composite key is: modelName@instanceName@providerName or
	// modelName@providerName.
	parts := rsplitN(defaultModel, "@", 2)
	var modelName, instanceName, providerName string
	switch len(parts) {
	case 3:
		modelName, instanceName, providerName = parts[0], parts[1], parts[2]
	case 2:
		modelName, providerName = parts[0], parts[1]
		instanceName = "default"
	default:
		modelName = parts[0]
		providerName = ""
		instanceName = "default"
	}

	// Special case: OCR with infiniflow@default@deepdoc is always enabled.
	if modelType == "ocr" && providerName == "infiniflow" && instanceName == "default" && modelName == "deepdoc" {
		return &providerName, &instanceName, &modelName, true, nil
	}

	// Special case: TEI Builtin embedding model.
	composeProfiles := common.GetEnv(common.EnvComposeProfiles)
	teiModel := common.GetEnv(common.EnvTEIModel)
	if modelType == "embedding" && strings.Contains(composeProfiles, "tei-") && teiModel != "" &&
		modelName == teiModel && (providerName == "" || providerName == "Builtin") {
		return &providerName, &instanceName, &modelName, true, nil
	}

	// Check if the provider exists for the tenant.
	modelProvider, err := s.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return nil, nil, nil, false, err
	}

	// Check if the instance exists.
	modelInstance, err := s.modelInstanceDAO.GetByProviderIDAndInstanceName(modelProvider.ID, instanceName)
	if err != nil {
		return nil, nil, nil, false, err
	}

	// Validate that the factory model supports this model type.
	modelSchema, err := dao.GetModelProviderManager().GetModelByName(providerName, modelName)
	if err != nil {
		return nil, nil, nil, false, err
	}
	factoryModelType := factoryModelTypeName(modelType)
	if !modelSchema.ModelTypeMap[factoryModelType] {
		return nil, nil, nil, false, fmt.Errorf("model %s isn't a %s model", modelName, modelType)
	}

	// Check if the model exists and is active.
	modelEntity, err := s.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(modelProvider.ID, modelInstance.ID, modelName)
	if err != nil {
		if !dao.IsNotFoundErr(err) {
			return nil, nil, nil, false, err
		}
	}
	if modelEntity == nil {
		return nil, nil, nil, false, fmt.Errorf("model %s isn't available", modelName)
	}
	if modelEntity.Status != "active" {
		return nil, nil, nil, false, fmt.Errorf("model %s isn't available", modelName)
	}

	return &providerName, &instanceName, &modelName, true, nil
}

// rsplitN splits s by sep from the right, limiting to n+1 parts (mirrors
// Python's str.rsplit(sep, maxsplit)).
func rsplitN(s, sep string, n int) []string {
	if n <= 0 {
		return []string{s}
	}
	result := make([]string, 0, n+1)
	remaining := s
	for i := 0; i < n; i++ {
		idx := strings.LastIndex(remaining, sep)
		if idx < 0 {
			result = append(result, remaining)
			// Reverse the collected parts.
			for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
				result[left], result[right] = result[right], result[left]
			}
			return result
		}
		result = append(result, remaining[idx+len(sep):])
		remaining = remaining[:idx]
	}
	result = append(result, remaining)
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
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
			ModelID:       ptrStringValue(ownedTenant.TenantLLMID),
			ModelType:     "chat",
			Enable:        defaultChatModelEnable,
		})
	}

	defaultEmbeddingModelProvider, defaultEmbeddingModelInstance, defaultEmbeddingModelName, defaultEmbeddingModelEnable, err := s.GetModelInfo(ownedTenant.TenantID, ownedTenant.EmbDID, "embedding")
	if err == nil {
		result = append(result, ModelItem{
			ModelProvider: defaultEmbeddingModelProvider,
			ModelInstance: defaultEmbeddingModelInstance,
			ModelName:     defaultEmbeddingModelName,
			ModelID:       ptrStringValue(ownedTenant.TenantEmbdID),
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
			ModelID:       ptrStringValue(ownedTenant.TenantRerankID),
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
			ModelID:       ptrStringValue(ownedTenant.TenantASRID),
			ModelType:     "asr",
			Enable:        defaultASREnable,
		})
	}

	defaultImage2TextModelProvider, defaultImage2TextModelInstance, defaultImage2TextModelName, defaultImage2TextModelEnable, err := s.GetModelInfo(ownedTenant.TenantID, ownedTenant.Img2TxtID, "vision")
	if err == nil {
		result = append(result, ModelItem{
			ModelProvider: defaultImage2TextModelProvider,
			ModelInstance: defaultImage2TextModelInstance,
			ModelName:     defaultImage2TextModelName,
			ModelID:       ptrStringValue(ownedTenant.TenantImg2TxtID),
			ModelType:     "vision",
			Enable:        defaultImage2TextModelEnable,
		})
	}

	if ownedTenant.OCRID != "" {
		defaultOCRModelProvider, defaultOCRModelInstance, defaultOCRModelName, defaultOCRModelEnable, err := s.GetModelInfo(ownedTenant.TenantID, ownedTenant.OCRID, "ocr")
		if err == nil {
			result = append(result, ModelItem{
				ModelProvider: defaultOCRModelProvider,
				ModelInstance: defaultOCRModelInstance,
				ModelName:     defaultOCRModelName,
				ModelID:       ptrStringValue(ownedTenant.TenantOCRID),
				ModelType:     "ocr",
				Enable:        defaultOCRModelEnable,
			})
		}
	}

	if ownedTenant.TTSID != "" {
		defaultTTSModelProvider, defaultTTSModelInstance, defaultTTSModelName, defaultTTSModelEnable, err := s.GetModelInfo(ownedTenant.TenantID, ownedTenant.TTSID, "tts")
		if err == nil {
			result = append(result, ModelItem{
				ModelProvider: defaultTTSModelProvider,
				ModelInstance: defaultTTSModelInstance,
				ModelName:     defaultTTSModelName,
				ModelID:       ptrStringValue(ownedTenant.TenantTTSID),
				ModelType:     "tts",
				Enable:        defaultTTSModelEnable,
			})
		}
	}

	return result, nil
}

func (s *TenantService) checkModelAvailable(tenantID, providerName, instanceName, modelName, modelType string) error {
	_, _, modelTypeBit, err := tenantDefaultModelFields(modelType)
	if err != nil {
		return err
	}

	// Static bypass: deepdoc is a built-in model that doesn't need DB checks (mirrors Python _check_model_available).
	if providerName == "infiniflow" && instanceName == "default" && modelName == "deepdoc" {
		return nil
	}

	// Static bypass: OCR with infiniflow@default@deepdoc is always enabled (mirrors Python _check_model_available).
	if modelType == "ocr" && providerName == "infiniflow" && instanceName == "default" && modelName == "deepdoc" {
		return nil
	}

	// Static bypass: TEI Builtin embedding model when COMPOSE_PROFILES includes tei- (mirrors Python _check_model_available).
	composeProfiles := common.GetEnv(common.EnvComposeProfiles)
	teiModel := common.GetEnv(common.EnvTEIModel)
	if modelType == "embedding" && strings.Contains(composeProfiles, "tei-") && teiModel != "" &&
		modelName == teiModel && (providerName == "" || providerName == "Builtin") {
		return nil
	}

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

	factoryModelType := factoryModelTypeName(modelType)
	if !modelSchema.ModelTypeMap[factoryModelType] {
		return fmt.Errorf("model %s isn't a %s model", modelName, modelType)
	}

	modelEntity, err := s.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(modelProvider.ID, modelInstance.ID, modelName)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return fmt.Errorf("model %s isn't available", modelName)
		}
		return err
	}
	if modelEntity.Status != "active" {
		return fmt.Errorf("model %s isn't available", modelName)
	}
	if !entity.ModelType(modelEntity.ModelType).Has(modelTypeBit) {
		return fmt.Errorf("model %s isn't a %s model", modelName, modelType)
	}

	return nil
}

func (s *TenantService) SetTenantDefaultModels(userID, modelProvider, modelInstance, modelName, modelType, modelID string) error {

	tenantInfos, err := s.tenantDAO.GetInfoByUserID(userID)
	if err != nil {
		return err
	}
	if len(tenantInfos) == 0 {
		return nil // No tenant found (should not happen for valid user)
	}

	ownedTenant := tenantInfos[0]
	var defaultModel string
	modelTypeID, tenantModelTypeID, modelTypeBit, err := tenantDefaultModelFields(modelType)
	if err != nil {
		return err
	}

	var tenantModelID interface{}
	if modelID != "" {
		modelEntity, err := s.modelDAO.GetByID(modelID)
		if err != nil {
			return fmt.Errorf("model ID %s is invalid", modelID)
		}
		instanceEntity, err := s.modelInstanceDAO.GetByID(modelEntity.InstanceID)
		if err != nil {
			return fmt.Errorf("instance for model %s not found: %w", modelID, err)
		}
		providerEntity, err := s.modelProviderDAO.GetByID(instanceEntity.ProviderID)
		if err != nil {
			return fmt.Errorf("provider for model %s not found: %w", modelID, err)
		}

		if providerEntity.TenantID != ownedTenant.TenantID {
			return fmt.Errorf("model %s does not belong to your tenant", modelID)
		}
		if modelEntity.Status != "active" {
			return fmt.Errorf("model %s isn't available", modelEntity.ModelName)
		}
		if !entity.ModelType(modelEntity.ModelType).Has(modelTypeBit) {
			return fmt.Errorf("model %s isn't a %s model", modelEntity.ModelName, modelType)
		}

		modelProvider = providerEntity.ProviderName
		modelInstance = instanceEntity.InstanceName
		modelName = modelEntity.ModelName
		tenantModelID = modelID
	}

	if modelProvider == "" && modelInstance == "" && modelName == "" {
		defaultModel = ""
		tenantModelID = nil
	} else if modelProvider != "" && modelInstance != "" && modelName != "" {
		err = s.checkModelAvailable(ownedTenant.TenantID, modelProvider, modelInstance, modelName, modelType)
		if err != nil {
			return err
		}
		if modelID == "" {
			// Builtin provider doesn't use tenant_model rows; leave tenantModelID nil
			// (mirrors Python resolve_model_id returning None for Builtin).
			if modelProvider == "Builtin" {
				tenantModelID = nil
			} else {
				modelProviderEntity, err := s.modelProviderDAO.GetByTenantIDAndProviderName(ownedTenant.TenantID, modelProvider)
				if err != nil {
					return err
				}
				modelInstanceEntity, err := s.modelInstanceDAO.GetByProviderIDAndInstanceName(modelProviderEntity.ID, modelInstance)
				if err != nil {
					return err
				}
				modelEntity, err := s.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(modelProviderEntity.ID, modelInstanceEntity.ID, modelName)
				if err != nil {
					return err
				}
				tenantModelID = modelEntity.ID
			}
		}
		defaultModel = fmt.Sprintf("%s@%s@%s", modelName, modelInstance, modelProvider)
	} else {
		return fmt.Errorf("model provider, instance and name must be specified together")
	}

	err = s.tenantDAO.Update(ownedTenant.TenantID, map[string]interface{}{
		modelTypeID:       defaultModel,
		tenantModelTypeID: tenantModelID,
	})

	return nil
}

// Tenant member role constants.
const (
	TenantRoleOwner  = "owner"
	TenantRoleNormal = "normal"
	TenantRoleInvite = "invite"
	TenantRoleAdmin  = "admin"
)

// TenantMemberResponse is one entry in the member list response.
type TenantMemberResponse struct {
	ID              string  `json:"id"`
	UserID          string  `json:"user_id"`
	Role            string  `json:"role"`
	Status          string  `json:"status"`
	Nickname        string  `json:"nickname"`
	Email           string  `json:"email"`
	Avatar          string  `json:"avatar"`
	IsAuthenticated bool    `json:"is_authenticated"`
	IsActive        string  `json:"is_active"`
	IsAnonymous     bool    `json:"is_anonymous"`
	IsSuperuser     bool    `json:"is_superuser"`
	UpdateDate      string  `json:"update_date"`
	DeltaSeconds    float64 `json:"delta_seconds"`
}

// ListMembers returns all non-owner members of tenantID.
// Only the tenant owner (userID == tenantID) may call this.
func (s *TenantService) ListMembers(userID, tenantID string) ([]*TenantMemberResponse, common.ErrorCode, error) {
	if userID != tenantID {
		return nil, common.CodeAuthenticationError, fmt.Errorf("no authorization")
	}
	rows, err := s.userTenantDAO.GetMembersByTenantID(tenantID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	result := make([]*TenantMemberResponse, 0, len(rows))
	for _, r := range rows {
		delta, _ := common.DeltaSeconds(r.UpdateDate)
		result = append(result, &TenantMemberResponse{
			ID:              r.ID,
			UserID:          r.UserID,
			Role:            r.Role,
			Status:          r.Status,
			Nickname:        r.Nickname,
			Email:           r.Email,
			Avatar:          r.Avatar,
			IsAuthenticated: r.IsAuthenticated,
			IsActive:        r.IsActive,
			IsAnonymous:     r.IsAnonymous,
			IsSuperuser:     r.IsSuperuser,
			UpdateDate:      r.UpdateDate,
			DeltaSeconds:    delta,
		})
	}
	return result, common.CodeSuccess, nil
}

// AddMemberRequest holds the invite payload.
type AddMemberRequest struct {
	Email string `json:"email"`
}

// AddMemberResponse holds the new member's public data.
type AddMemberResponse struct {
	ID       string `json:"id"`
	Avatar   string `json:"avatar"`
	Email    string `json:"email"`
	Nickname string `json:"nickname"`
}

// AddMember invites a user (by email) to the tenant.
// Only the tenant owner (userID == tenantID) may call this.
func (s *TenantService) AddMember(userID, tenantID string, req *AddMemberRequest) (*AddMemberResponse, common.ErrorCode, error) {
	if userID != tenantID {
		return nil, common.CodeAuthenticationError, fmt.Errorf("no authorization")
	}
	if req.Email == "" {
		return nil, common.CodeArgumentError, fmt.Errorf("email is required")
	}

	invitee, err := s.userDAO.GetByEmail(req.Email)
	if err != nil {
		return nil, common.CodeDataError, fmt.Errorf("user not found")
	}

	// Reject if already a member or has a pending invitation.
	existing, _ := s.userTenantDAO.FilterByUserIDAndTenantID(invitee.ID, tenantID)
	if existing != nil {
		switch existing.Role {
		case TenantRoleOwner:
			return nil, common.CodeDataError, fmt.Errorf("user is already the tenant owner")
		case TenantRoleNormal, TenantRoleAdmin:
			return nil, common.CodeDataError, fmt.Errorf("user is already a member")
		case TenantRoleInvite:
			return nil, common.CodeDataError, fmt.Errorf("user already has a pending invitation")
		}
	}

	status := "1"
	ut := &entity.UserTenant{
		ID:        utility.GenerateUUID(),
		UserID:    invitee.ID,
		TenantID:  tenantID,
		Role:      TenantRoleInvite,
		InvitedBy: userID,
		Status:    &status,
	}
	if err = s.userTenantDAO.Create(ut); err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to create invitation: %w", err)
	}

	avatar := ""
	if invitee.Avatar != nil {
		avatar = *invitee.Avatar
	}
	return &AddMemberResponse{
		ID:       invitee.ID,
		Avatar:   avatar,
		Email:    invitee.Email,
		Nickname: invitee.Nickname,
	}, common.CodeSuccess, nil
}

// RemoveMember removes a user from the tenant.
// Either the owner (userID == tenantID) or the member themselves (userID == targetUserID) may call this.
// The tenant owner (targetUserID == tenantID) cannot be removed.
func (s *TenantService) RemoveMember(userID, tenantID, targetUserID string) (common.ErrorCode, error) {
	if userID != tenantID && userID != targetUserID {
		return common.CodeAuthenticationError, fmt.Errorf("no authorization")
	}
	if targetUserID == tenantID {
		return common.CodeArgumentError, fmt.Errorf("cannot remove the tenant owner")
	}
	if s.userTenantDAO == nil {
		return common.CodeServerError, fmt.Errorf("userTenantDAO not initialized")
	}
	if err := s.userTenantDAO.DeleteByUserAndTenant(targetUserID, tenantID); err != nil {
		return common.CodeServerError, fmt.Errorf("failed to remove member: %w", err)
	}
	return common.CodeSuccess, nil
}

// AcceptInvite transitions the calling user's role from "invite" → "normal" for the given tenant.
func (s *TenantService) AcceptInvite(userID, tenantID string) (common.ErrorCode, error) {
	if s.userTenantDAO == nil {
		return common.CodeServerError, fmt.Errorf("userTenantDAO not initialized")
	}
	existing, err := s.userTenantDAO.FilterByUserIDAndTenantID(userID, tenantID)
	if err != nil || existing == nil {
		return common.CodeDataError, fmt.Errorf("no pending invitation found")
	}
	if existing.Role != TenantRoleInvite {
		return common.CodeArgumentError, fmt.Errorf("no pending invitation to accept")
	}
	if err := s.userTenantDAO.UpdateRoleByUserAndTenant(userID, tenantID, TenantRoleNormal); err != nil {
		return common.CodeServerError, fmt.Errorf("failed to accept invitation: %w", err)
	}
	return common.CodeSuccess, nil
}
