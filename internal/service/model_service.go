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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"strings"
	"time"

	"ragflow/internal/service/models"
)

// ModelProvider provides model instances based on tenant and model type
type ModelProvider interface {
	// GetEmbeddingModel returns an embedding model for the given tenant
	GetEmbeddingModel(ctx context.Context, tenantID string, modelName string) (entity.EmbeddingModel, error)
	// GetChatModel returns a chat model for the given tenant
	GetChatModel(ctx context.Context, tenantID string, modelName string) (entity.ChatModel, error)
	// GetRerankModel returns a rerank model for the given tenant
	GetRerankModel(ctx context.Context, tenantID string, modelName string) (entity.RerankModel, error)
}

// ModelProviderImpl implements ModelProvider
type ModelProviderImpl struct {
	httpClient *http.Client
}

// NewModelProvider creates a new ModelProvider
func NewModelProvider() *ModelProviderImpl {
	return &ModelProviderImpl{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

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

// GetEmbeddingModel returns an embedding model for the given tenant
func (p *ModelProviderImpl) GetEmbeddingModel(ctx context.Context, tenantID string, compositeModelName string) (entity.EmbeddingModel, error) {
	// Parse composite model name to extract model name and provider
	modelName, provider, err := parseModelName(compositeModelName)
	if err != nil {
		return nil, err
	}

	// Get API key and configuration
	embeddingModel, err := dao.NewTenantLLMDAO().GetByTenantFactoryAndModelName(tenantID, provider, modelName)
	if err != nil {
		return nil, err
	}

	apiKey := embeddingModel.APIKey
	if apiKey == nil || *apiKey == "" {
		return nil, fmt.Errorf("no API key found for tenant %s and model %s", tenantID, compositeModelName)
	}

	// Get API base from TenantLLM if set, otherwise from model provider configuration
	apiBase := ""
	if embeddingModel.APIBase != nil && *embeddingModel.APIBase != "" {
		apiBase = *embeddingModel.APIBase
	} else {
		providerDAO := dao.NewModelProviderDAO()
		providerConfig := providerDAO.GetProviderByName(provider)
		if providerConfig == nil || providerConfig.DefaultURL == "" {
			return nil, fmt.Errorf("no API base found for provider %s", provider)
		}
		apiBase = providerConfig.DefaultURL
	}

	return models.CreateEmbeddingModel(provider, *apiKey, apiBase, modelName, p.httpClient)
}

// GetChatModel returns a chat model for the given tenant
func (p *ModelProviderImpl) GetChatModel(ctx context.Context, tenantID string, compositeModelName string) (entity.ChatModel, error) {
	// Parse composite model name to extract model name and provider
	modelName, provider, err := parseModelName(compositeModelName)
	if err != nil {
		return nil, err
	}

	// Get chat model from database
	chatModel, err := dao.NewTenantLLMDAO().GetByTenantFactoryAndModelName(tenantID, provider, modelName)
	if err != nil {
		return nil, fmt.Errorf("no chat model found for tenant %s and model %s: %w", tenantID, compositeModelName, err)
	}

	apiKey := chatModel.APIKey
	if apiKey == nil || *apiKey == "" {
		return nil, fmt.Errorf("no API key found for tenant %s and model %s", tenantID, compositeModelName)
	}

	// Get API base from TenantLLM if set, otherwise from model provider configuration
	apiBase := ""
	if chatModel.APIBase != nil && *chatModel.APIBase != "" {
		apiBase = *chatModel.APIBase
	} else {
		providerDAO := dao.NewModelProviderDAO()
		providerConfig := providerDAO.GetProviderByName(provider)
		if providerConfig == nil || providerConfig.DefaultURL == "" {
			return nil, fmt.Errorf("no API base found for provider %s", provider)
		}
		apiBase = providerConfig.DefaultURL
	}

	return models.CreateChatModel(provider, *apiKey, apiBase, modelName, p.httpClient)
}

// GetRerankModel returns a rerank model for the given tenant
func (p *ModelProviderImpl) GetRerankModel(ctx context.Context, tenantID string, compositeModelName string) (entity.RerankModel, error) {
	// Parse composite model name to extract model name and provider
	modelName, provider, err := parseModelName(compositeModelName)
	if err != nil {
		return nil, err
	}

	// Get rerank model from database
	rerankModel, err := dao.NewTenantLLMDAO().GetByTenantFactoryAndModelName(tenantID, provider, modelName)
	if err != nil {
		return nil, fmt.Errorf("no rerank model found for tenant %s and model %s: %w", tenantID, compositeModelName, err)
	}

	apiKey := rerankModel.APIKey
	if apiKey == nil || *apiKey == "" {
		return nil, fmt.Errorf("no API key found for tenant %s and model %s", tenantID, compositeModelName)
	}

	// Get API base from TenantLLM if set, otherwise from model provider configuration
	apiBase := ""
	if rerankModel.APIBase != nil && *rerankModel.APIBase != "" {
		apiBase = *rerankModel.APIBase
	} else {
		providerDAO := dao.NewModelProviderDAO()
		providerConfig := providerDAO.GetProviderByName(provider)
		if providerConfig == nil || providerConfig.DefaultURL == "" {
			return nil, fmt.Errorf("no API base found for provider %s", provider)
		}
		apiBase = providerConfig.DefaultURL
	}

	return models.CreateRerankModel(provider, *apiKey, apiBase, modelName, p.httpClient)
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

		_, err = dao.GetModelProviderManager().GetModelByName(providerName, modelName)
		if err != nil {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("provider %s model %s not found", providerName, modelName))
		}

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

func (m *ModelProviderService) ChatToModelByApiKey(providerName, modelName, apiKey, message string) (*string, common.ErrorCode, error) {
	providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
	if providerInfo == nil {
		return nil, common.CodeNotFound, errors.New("provider not found")
	}

	_, err := dao.GetModelProviderManager().GetModelByName(providerName, modelName)
	if err != nil {
		return nil, common.CodeNotFound, errors.New(fmt.Sprintf("provider %s model %s not found", providerName, modelName))
	}

	var response string
	response, err = providerInfo.ModelDriver.Chat(&modelName, &apiKey, &message, nil)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	return &response, common.CodeSuccess, nil
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
func (m *ModelProviderService) ChatToModelStreamWithSender(providerName, instanceName, modelName, userID, message string, apiConfig *modelModule.APIConfig, modelConfig *modelModule.ChatConfig, sender func(*string, *string) error) common.ErrorCode {
	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return common.CodeServerError
	}

	if len(tenants) == 0 {
		return common.CodeNotFound
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return common.CodeServerError
	}

	instance, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return common.CodeServerError
	}

	_, err = m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(provider.ID, instance.ID, modelName)
	if err != nil {
		providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
		if providerInfo == nil {
			return common.CodeNotFound
		}

		_, err = dao.GetModelProviderManager().GetModelByName(providerName, modelName)
		if err != nil {
			return common.CodeNotFound
		}

		var extra map[string]string
		err = json.Unmarshal([]byte(instance.Extra), &extra)
		if err != nil {
			return common.CodeServerError
		}

		region := extra["region"]
		apiConfig.Region = &region
		apiConfig.ApiKey = &instance.APIKey

		// Direct call with sender function
		err = providerInfo.ModelDriver.ChatStreamlyWithSender(&modelName, &message, apiConfig, modelConfig, sender)
		if err != nil {
			return common.CodeServerError
		}

		return common.CodeSuccess
	}

	return common.CodeServerError
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
