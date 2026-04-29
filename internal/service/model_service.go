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
	"encoding/json"
	"errors"
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"strings"
	"time"
)

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

func NewModelProviderService() *ModelProviderService {
	return &ModelProviderService{
		modelProviderDAO:     dao.NewTenantModelProviderDAO(),
		modelInstanceDAO:     dao.NewTenantModelInstanceDAO(),
		modelDAO:             dao.NewTenantModelDAO(),
		modelGroupDAO:        dao.NewTenantModelGroupDAO(),
		modelGroupMappingDAO: dao.NewTenantModelGroupMappingDAO(),
		userTenantDAO:        dao.NewUserTenantDAO(),
	}
}

type ModelProviderService struct {
	modelProviderDAO     *dao.TenantModelProviderDAO
	modelInstanceDAO     *dao.TenantModelInstanceDAO
	modelDAO             *dao.TenantModelDAO
	modelGroupDAO        *dao.TenantModelGroupDAO
	modelGroupMappingDAO *dao.TenantModelGroupMappingDAO
	userTenantDAO        *dao.UserTenantDAO
}

func (m *ModelProviderService) AddModelProvider(providerName, userID string) (common.ErrorCode, error) {

	_, err := dao.GetModelProviderManager().GetProviderByName(providerName)
	if err != nil {
		return common.CodeNotFound, err
	}

	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	providerID, err := generateUUID1Hex()
	if err != nil {
		return common.CodeServerError, errors.New("fail to get UUID")
	}

	now := time.Now().Unix()
	nowDate := time.Now().Truncate(time.Second)
	tenantModelProvider := &entity.TenantModelProvider{
		ID:           providerID,
		ProviderName: providerName,
		TenantID:     tenantID,
	}
	tenantModelProvider.CreateTime = &now
	tenantModelProvider.UpdateTime = &now
	tenantModelProvider.CreateDate = &nowDate
	tenantModelProvider.UpdateDate = &nowDate
	err = m.modelProviderDAO.Create(tenantModelProvider)
	if err != nil {
		return common.CodeServerError, errors.New("fail to create model provider")
	}
	return common.CodeSuccess, nil
}

func (m *ModelProviderService) ListProvidersOfTenant(userID string) ([]map[string]interface{}, common.ErrorCode, error) {

	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return nil, common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return nil, common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	providerNames, err := m.modelProviderDAO.ListByID(tenantID)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	var result []map[string]interface{}
	for _, providerName := range providerNames {
		provider, err := dao.GetModelProviderManager().GetProviderByName(providerName)
		if err != nil {
			return nil, common.CodeServerError, err
		}
		result = append(result, provider)
	}

	return result, common.CodeSuccess, nil
}

func (m *ModelProviderService) DeleteModelProvider(providerName, userID string) (common.ErrorCode, error) {
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return common.CodeServerError, err
	}
	if len(tenants) == 0 {
		return common.CodeNotFound, errors.New("user has no tenants")
	}
	tenantID := tenants[0].TenantID

	_, err = m.modelProviderDAO.DeleteByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return common.CodeServerError, err
	}

	return common.CodeSuccess, nil
}

func (m *ModelProviderService) ListSupportedModels(providerName, instanceName, userID string) ([]string, error) {

	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return nil, errors.New("fail to get tenant")
	}

	if len(tenants) == 0 {
		return nil, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return nil, err
	}

	instance, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return nil, err
	}

	providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
	if providerInfo == nil {
		return nil, fmt.Errorf("provider %s not found", providerName)
	}

	var extra map[string]string
	err = json.Unmarshal([]byte(instance.Extra), &extra)
	if err != nil {
		return nil, err
	}

	apiConfig := &modelModule.APIConfig{
		ApiKey: nil,
		Region: nil,
	}

	region := extra["region"]
	apiConfig.Region = &region
	apiConfig.ApiKey = &instance.APIKey

	return providerInfo.ModelDriver.ListModels(apiConfig)
}

func (m *ModelProviderService) CreateProviderInstance(providerName, instanceName, apiKey, userID, region string) (common.ErrorCode, error) {
	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return common.CodeServerError, err
	}

	instanceID, err := generateUUID1Hex()
	if err != nil {
		return common.CodeServerError, errors.New("fail to get UUID")
	}

	extra := make(map[string]string)
	extra["region"] = region
	// convert extra to string
	extraByte, err := json.Marshal(extra)
	if err != nil {
		return common.CodeServerError, errors.New("fail to marshal extra")
	}
	extraStr := string(extraByte)

	now := time.Now().Unix()
	nowDate := time.Now().Truncate(time.Second)
	tenantModelProvider := &entity.TenantModelInstance{
		ID:           instanceID,
		InstanceName: instanceName,
		ProviderID:   provider.ID,
		APIKey:       apiKey,
		Status:       "enable",
		Extra:        extraStr,
	}
	tenantModelProvider.CreateTime = &now
	tenantModelProvider.UpdateTime = &now
	tenantModelProvider.CreateDate = &nowDate
	tenantModelProvider.UpdateDate = &nowDate
	err = m.modelInstanceDAO.Create(tenantModelProvider)

	if err != nil {
		return common.CodeServerError, errors.New("fail to create model provider")
	}
	return common.CodeSuccess, nil
}

func (m *ModelProviderService) ListProviderInstances(providerName, userID string) ([]map[string]interface{}, common.ErrorCode, error) {

	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return nil, common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return nil, common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	// Check if provider exists
	instances, err := m.modelInstanceDAO.GetAllInstancesByProviderID(provider.ID)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	var result []map[string]interface{}
	for _, instance := range instances {
		// convert instance.Extra (json string) to map
		var extra map[string]string
		err = json.Unmarshal([]byte(instance.Extra), &extra)
		if err != nil {
			return nil, common.CodeServerError, err
		}

		result = append(result, map[string]interface{}{
			"id":           instance.ID,
			"instanceName": instance.InstanceName,
			"providerID":   instance.ProviderID,
			"apiKey":       instance.APIKey,
			"status":       instance.Status,
			"region":       extra["region"],
		})
	}

	return result, common.CodeSuccess, nil
}

func (m *ModelProviderService) ShowProviderInstance(providerName, instanceName, userID string) (map[string]interface{}, common.ErrorCode, error) {

	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return nil, common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return nil, common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	instance, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	// convert instance.Extra (json string) to map
	var extra map[string]string
	err = json.Unmarshal([]byte(instance.Extra), &extra)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	result := map[string]interface{}{
		"id":           instance.ID,
		"instanceName": instance.InstanceName,
		"providerID":   instance.ProviderID,
		"status":       instance.Status,
		"region":       extra["region"],
	}

	return result, common.CodeSuccess, nil
}

func (m *ModelProviderService) ShowInstanceBalance(providerName, instanceName, userID string) (map[string]interface{}, common.ErrorCode, error) {

	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return nil, common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return nil, common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	instance, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
	if providerInfo == nil {
		return nil, common.CodeServerError, fmt.Errorf("provider %s not found", providerName)
	}

	var extra map[string]string
	err = json.Unmarshal([]byte(instance.Extra), &extra)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	apiConfig := &modelModule.APIConfig{
		ApiKey: nil,
		Region: nil,
	}

	region := extra["region"]
	apiConfig.Region = &region
	apiConfig.ApiKey = &instance.APIKey

	var result map[string]interface{}
	result, err = providerInfo.ModelDriver.Balance(apiConfig)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	return result, common.CodeSuccess, nil
}

func (m *ModelProviderService) CheckProviderConnection(providerName, instanceName, userID string) (common.ErrorCode, error) {

	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return common.CodeServerError, err
	}

	instance, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return common.CodeServerError, err
	}

	providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
	if providerInfo == nil {
		return common.CodeServerError, fmt.Errorf("provider %s not found", providerName)
	}

	var extra map[string]string
	err = json.Unmarshal([]byte(instance.Extra), &extra)
	if err != nil {
		return common.CodeServerError, err
	}

	apiConfig := &modelModule.APIConfig{
		ApiKey: nil,
		Region: nil,
	}

	region := extra["region"]
	apiConfig.Region = &region
	apiConfig.ApiKey = &instance.APIKey

	err = providerInfo.ModelDriver.CheckConnection(apiConfig)
	if err != nil {
		return common.CodeServerError, err
	}
	return common.CodeSuccess, nil
}

func (m *ModelProviderService) AlterProviderInstance(providerName, instanceName, newInstanceName, apiKey, userID string) (common.ErrorCode, error) {
	return common.CodeSuccess, nil
}
func (m *ModelProviderService) DropProviderInstances(providerName, userID string, instances []string) (common.ErrorCode, error) {

	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return common.CodeServerError, err
	}

	for _, instanceName := range instances {
		count, err := m.modelInstanceDAO.DeleteByProviderIDAndInstanceName(provider.ID, instanceName)
		if err != nil {
			return common.CodeServerError, err
		}

		if count == 0 {
			return common.CodeNotFound, errors.New("provider instance not found")
		}
	}

	return common.CodeSuccess, nil
}

func (m *ModelProviderService) ListInstanceModels(providerName, instanceName, userID string) ([]map[string]interface{}, error) {
	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return nil, err
	}

	if len(tenants) == 0 {
		return nil, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return nil, err
	}

	// Get instance
	instance, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return nil, err
	}

	// Get all models for this instance
	disabledModels, err := m.modelDAO.GetModelsByInstanceID(instance.ID)
	if err != nil {
		return nil, err
	}

	// insert models name into a set
	modelNames := make(map[string]bool)
	for _, model := range disabledModels {
		modelNames[model.ModelName] = true
	}

	allModels, err := dao.GetModelProviderManager().ListModels(providerName)

	for _, model := range allModels {
		// convert model["name"] to string
		modelName := model["name"].(string)
		if modelNames[modelName] {
			model["status"] = "disabled"
		} else {
			model["status"] = "enabled"
		}

	}

	return allModels, nil
}

func (m *ModelProviderService) UpdateModelStatus(providerName, instanceName, modelName, userID, status string) (common.ErrorCode, error) {

	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return common.CodeServerError, err
	}

	instance, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return common.CodeServerError, err
	}

	model, err := m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(provider.ID, instance.ID, modelName)
	if err != nil {
		var modelID string
		modelID, err = generateUUID1Hex()
		if err != nil {
			return common.CodeServerError, errors.New("fail to get UUID")
		}

		var modelSchema *entity.Model
		modelSchema, err = dao.GetModelProviderManager().GetModelByName(providerName, modelName)
		if err != nil {
			return common.CodeNotFound, errors.New(fmt.Sprintf("provider %s model %s not found", providerName, modelName))
		}

		// Get model info from provider
		model = &entity.TenantModel{
			ID:         modelID,
			ModelName:  modelName,
			ModelType:  modelSchema.ModelTypes[0],
			ProviderID: provider.ID,
			InstanceID: instance.ID,
			Status:     status,
		}
		err = m.modelDAO.Create(model)
		if err != nil {
			return common.CodeServerError, errors.New("fail to create model")
		}
		return common.CodeSuccess, nil
	}

	count, err := m.modelDAO.DeleteByModelID(model.ID)
	if err != nil {
		return common.CodeServerError, err
	}
	if count == 0 {
		return common.CodeNotFound, errors.New("model not found")
	}

	return common.CodeSuccess, nil
}

func (m *ModelProviderService) ChatToModel(providerName, instanceName, modelName, userID, message string, apiConfig *modelModule.APIConfig, modelConfig *modelModule.ChatConfig) (*modelModule.ChatResponse, common.ErrorCode, error) {

	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return nil, common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return nil, common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	instance, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	_, err = m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(provider.ID, instance.ID, modelName)
	if err != nil {
		providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
		if providerInfo == nil {
			return nil, common.CodeNotFound, errors.New("provider not found")
		}

		var model *entity.Model = nil
		model, err = dao.GetModelProviderManager().GetModelByName(providerName, modelName)
		if err != nil {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("provider %s model %s not found", providerName, modelName))
		}

		modelConfig.ModelClass = model.Class

		var extra map[string]string
		err = json.Unmarshal([]byte(instance.Extra), &extra)
		if err != nil {
			return nil, common.CodeServerError, err
		}

		region := extra["region"]
		apiConfig.Region = &region
		apiConfig.ApiKey = &instance.APIKey

		var response *modelModule.ChatResponse
		response, err = providerInfo.ModelDriver.Chat(&modelName, &message, apiConfig, modelConfig)
		if err != nil {
			return nil, common.CodeServerError, err
		}

		return response, common.CodeSuccess, nil
	}

	return nil, common.CodeServerError, errors.New("model is disabled")
}

// ChatWithMessagesToModelByApiKey sends multiple messages with roles and returns response
func (m *ModelProviderService) ChatWithMessagesToModelByApiKey(providerName, modelName, apiKey string, messages []modelModule.Message) (*string, common.ErrorCode, error) {
	providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
	if providerInfo == nil {
		return nil, common.CodeNotFound, errors.New("provider not found")
	}

	_, err := dao.GetModelProviderManager().GetModelByName(providerName, modelName)
	if err != nil {
		return nil, common.CodeNotFound, errors.New(fmt.Sprintf("provider %s model %s not found", providerName, modelName))
	}

	var response string
	response, err = providerInfo.ModelDriver.ChatWithMessages(modelName, &apiKey, messages, nil)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	return &response, common.CodeSuccess, nil
}

// ChatToModelStreamWithSender streams chat response directly via sender function (best performance, no channel)
func (m *ModelProviderService) ChatToModelStreamWithSender(providerName, instanceName, modelName, userID, message string, apiConfig *modelModule.APIConfig, modelConfig *modelModule.ChatConfig, sender func(*string, *string) error) (common.ErrorCode, error) {
	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return common.CodeServerError, err
	}

	instance, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return common.CodeServerError, err
	}

	_, err = m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(provider.ID, instance.ID, modelName)
	if err != nil {
		providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
		if providerInfo == nil {
			return common.CodeNotFound, err
		}

		_, err = dao.GetModelProviderManager().GetModelByName(providerName, modelName)
		if err != nil {
			return common.CodeNotFound, err
		}

		var extra map[string]string
		err = json.Unmarshal([]byte(instance.Extra), &extra)
		if err != nil {
			return common.CodeServerError, err
		}

		region := extra["region"]
		apiConfig.Region = &region
		apiConfig.ApiKey = &instance.APIKey

		// Direct call with sender function
		err = providerInfo.ModelDriver.ChatStreamlyWithSender(&modelName, &message, apiConfig, modelConfig, sender)
		if err != nil {
			return common.CodeServerError, err
		}

		return common.CodeSuccess, nil
	}

	return common.CodeServerError, errors.New("model is disabled")
}

func (m *ModelProviderService) GetDefaultModel(modelType entity.ModelType, tenantID string) (*entity.ModelCredentials, error) {
	// Get tenant record to find default model name
	tenant, err := dao.NewTenantDAO().GetByID(tenantID)
	if err != nil {
		return nil, fmt.Errorf("tenant not found: %w", err)
	}

	// Determine model name based on model type
	var defaultModelName string
	switch modelType {
	case entity.ModelTypeChat:
		defaultModelName = tenant.LLMID
	case entity.ModelTypeEmbedding:
		defaultModelName = tenant.EmbdID
	case entity.ModelTypeSpeech2Text:
		defaultModelName = tenant.ASRID
	case entity.ModelTypeImage2Text:
		defaultModelName = tenant.Img2TxtID
	case entity.ModelTypeRerank:
		defaultModelName = tenant.RerankID
	case entity.ModelTypeTTS:
		if tenant.TTSID != nil {
			defaultModelName = *tenant.TTSID
		}
	case entity.ModelTypeOCR:
		return nil, errors.New("OCR model name is required")
	default:
		return nil, fmt.Errorf("unknown model type: %s", modelType)
	}

	if defaultModelName == "" {
		return nil, fmt.Errorf("no default %s model is set", modelType)
	}

	// Look up the TenantLLM record to get provider name and API key
	// Use GetByTenantIDAndLLMName which handles splitting model name and factory
	tenantLLM, err := dao.NewTenantLLMDAO().GetByTenantIDAndLLMName(tenantID, defaultModelName)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant default model: %w", err)
	}

	if tenantLLM == nil {
		return nil, fmt.Errorf("no default %s model found for tenant", modelType)
	}

	if tenantLLM.LLMName == nil || tenantLLM.APIKey == nil {
		return nil, fmt.Errorf("tenant model %q has missing name or api key", defaultModelName)
	}
	return &entity.ModelCredentials{
		ProviderName: tenantLLM.LLMFactory,
		ModelName:    *tenantLLM.LLMName,
		APIKey:       *tenantLLM.APIKey,
	}, nil
}

// GetModelByName gets model credentials by model name (chat_id from search_config)
func (m *ModelProviderService) GetModelByName(modelName string, tenantID string) (*entity.ModelCredentials, error) {
	tenantLLM, err := dao.NewTenantLLMDAO().GetByTenantIDAndLLMName(tenantID, modelName)
	if err != nil {
		return nil, fmt.Errorf("failed to get model by name: %w", err)
	}
	if tenantLLM == nil {
		return nil, fmt.Errorf("model not found: %s", modelName)
	}

	return &entity.ModelCredentials{
		ProviderName: tenantLLM.LLMFactory,
		ModelName:    *tenantLLM.LLMName,
		APIKey:       *tenantLLM.APIKey,
	}, nil
}

// GetEmbeddingModel returns an EmbeddingModel wrapper for the given tenant
func (m *ModelProviderService) GetEmbeddingModel(tenantID, compositeModelName string) (*modelModule.EmbeddingModel, error) {
	driver, modelName, apiConfig, err := m.getModelConfig(tenantID, compositeModelName)
	if err != nil {
		return nil, err
	}
	return modelModule.NewEmbeddingModel(driver, &modelName, apiConfig), nil
}

// GetRerankModel returns a RerankModel wrapper for the given tenant
func (m *ModelProviderService) GetRerankModel(tenantID, compositeModelName string) (*modelModule.RerankModel, error) {
	driver, modelName, apiConfig, err := m.getModelConfig(tenantID, compositeModelName)
	if err != nil {
		return nil, err
	}
	return modelModule.NewRerankModel(driver, &modelName, apiConfig), nil
}

// GetChatModel returns a ChatModel wrapper for the given tenant
func (m *ModelProviderService) GetChatModel(tenantID, compositeModelName string) (*modelModule.ChatModel, error) {
	driver, modelName, apiConfig, err := m.getModelConfig(tenantID, compositeModelName)
	if err != nil {
		return nil, err
	}
	return modelModule.NewChatModel(driver, &modelName, apiConfig), nil
}

// getModelConfig returns the model driver, model name, and API config for a model
func (m *ModelProviderService) getModelConfig(tenantID, compositeModelName string) (modelModule.ModelDriver, string, *modelModule.APIConfig, error) {
	modelName, providerName, err := parseModelName(compositeModelName)
	if err != nil {
		return nil, "", nil, err
	}

	// Check if provider exists
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return nil, "", nil, err
	}
	if provider == nil {
		return nil, "", nil, fmt.Errorf("provider %s not found", providerName)
	}

	instanceName := "default_instance"
	instance, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return nil, "", nil, err
	}
	if instance == nil {
		return nil, "", nil, fmt.Errorf("instance %s not found for provider %s", instanceName, providerName)
	}

	_, err = m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(provider.ID, instance.ID, modelName)
	if err != nil {
		providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
		if providerInfo == nil {
			return nil, "", nil, fmt.Errorf("provider %s not found", providerName)
		}

		_, err = dao.GetModelProviderManager().GetModelByName(providerName, modelName)
		if err != nil {
			return nil, "", nil, fmt.Errorf("provider %s model %s not found", providerName, modelName)
		}

		var extra map[string]string
		err = json.Unmarshal([]byte(instance.Extra), &extra)
		if err != nil {
			return nil, "", nil, err
		}
		region := extra["region"]

		apiConfig := &modelModule.APIConfig{ApiKey: &instance.APIKey, Region: &region}
		return providerInfo.ModelDriver, modelName, apiConfig, nil
	}

	var extra map[string]string
	err = json.Unmarshal([]byte(instance.Extra), &extra)
	if err != nil {
		return nil, "", nil, err
	}
	region := extra["region"]

	providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
	if providerInfo == nil {
		return nil, "", nil, fmt.Errorf("provider %s not found", providerName)
	}

	apiConfig := &modelModule.APIConfig{ApiKey: &instance.APIKey, Region: &region}
	return providerInfo.ModelDriver, modelName, apiConfig, nil
}
