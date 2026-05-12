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

// parseModelName parses a composite model name in format "model@instance@provider" or "model@provider"
// Returns modelName, instanceName, providerName separately
func parseModelName(compositeName string) (modelName, instanceName, providerName string, err error) {
	parts := strings.Split(compositeName, "@")
	if len(parts) == 3 {
		// Format: model@instance@provider
		return parts[0], parts[1], parts[2], nil
	} else if len(parts) == 2 {
		// Format: model@provider -> instance defaults to "default"
		return parts[0], "default", parts[1], nil
	} else if len(parts) == 1 {
		return parts[0], "", "", fmt.Errorf("provider name missing in model name: %s", compositeName)
	} else {
		return "", "", "", fmt.Errorf("invalid model name format: %s", compositeName)
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
		return common.CodeServerError, fmt.Errorf("fail to create model provider: %s", err.Error())
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

	driver := providerInfo.ModelDriver

	// For local deployed models
	if baseURL, ok := extra["base_url"]; ok && baseURL != "" {
		newURL := map[string]string{
			region: baseURL,
		}

		driver = driver.NewInstance(newURL)
	}

	return driver.ListModels(apiConfig)
}

func (m *ModelProviderService) CreateProviderInstance(providerName, instanceName, apiKey, baseURL, region, userID string) (common.ErrorCode, error) {
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
	extra["base_url"] = baseURL
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
		return common.CodeServerError, fmt.Errorf("fail to create model instance: %s", err.Error())
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
			"extra":        instance.Extra,
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

	driver := providerInfo.ModelDriver
	if baseURL, ok := extra["base_url"]; ok && baseURL != "" {
		newURL := map[string]string{
			region: baseURL,
		}
		driver = driver.NewInstance(newURL)
	}

	err = driver.CheckConnection(apiConfig)
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
		// Get model instance
		var tenantModelInstance *entity.TenantModelInstance
		tenantModelInstance, err = m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
		if err != nil {
			return common.CodeServerError, err
		}

		// Delete all models of this instance
		var count int64 = 0
		count, err = m.modelDAO.DeleteByProviderIDAndInstanceID(provider.ID, tenantModelInstance.ID)
		if err != nil {
			return common.CodeServerError, err
		}

		// Delete model instance
		count, err = m.modelInstanceDAO.DeleteByProviderIDAndInstanceName(provider.ID, instanceName)
		if err != nil {
			return common.CodeServerError, err
		}

		if count == 0 {
			return common.CodeNotFound, errors.New("provider instance not found")
		}
	}

	return common.CodeSuccess, nil
}

func (m *ModelProviderService) DropInstanceModels(providerName, instanceName, userID string, models []string) (common.ErrorCode, error) {

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

	var modelInstance *entity.TenantModelInstance
	modelInstance, err = m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return common.CodeServerError, err
	}

	for _, modelName := range models {
		// Delete all models of this instance
		var count int64 = 0
		count, err = m.modelDAO.DeleteByProviderIDAndInstanceIDAndModelName(provider.ID, modelInstance.ID, modelName)
		if err != nil {
			return common.CodeServerError, err
		}

		if count == 0 {
			return common.CodeNotFound, fmt.Errorf("model: %s not found", modelName)
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

	allModels, err := dao.GetModelProviderManager().ListModels(providerName)

	// insert models name into a set
	modelNames := make(map[string]bool)
	for _, model := range disabledModels {
		if model.Status == "active" {
			modelData := map[string]interface{}{
				"name": model.ModelName,
			}
			allModels = append(allModels, modelData)
		} else {
			modelNames[model.ModelName] = true
		}

	}

	for _, model := range allModels {
		// convert model["name"] to string
		modelName := model["name"].(string)
		if modelNames[modelName] {
			model["status"] = "inactive"
		} else {
			model["status"] = "active"
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

// ChatToModelWithMessages sends messages to the model with messages array
func (m *ModelProviderService) ChatToModelWithMessages(providerName, instanceName, modelName, userID string, messages []modelModule.Message, apiConfig *modelModule.APIConfig, modelConfig *modelModule.ChatConfig) (*modelModule.ChatResponse, common.ErrorCode, error) {
	if apiConfig == nil {
		apiConfig = &modelModule.APIConfig{}
	}
	if modelConfig == nil {
		modelConfig = &modelModule.ChatConfig{}
	}

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

	modelInfo, err := m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(provider.ID, instance.ID, modelName)
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
		response, err = providerInfo.ModelDriver.ChatWithMessages(modelName, messages, apiConfig, modelConfig)
		if err != nil {
			return nil, common.CodeServerError, err
		}
		if response == nil {
			return nil, common.CodeServerError, errors.New("empty chat response")
		}

		return response, common.CodeSuccess, nil
	}

	if modelInfo.Status == "active" {
		// For local deployed models
		providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
		if providerInfo == nil {
			return nil, common.CodeNotFound, errors.New("provider not found")
		}

		var extra map[string]string
		err = json.Unmarshal([]byte(instance.Extra), &extra)
		if err != nil {
			return nil, common.CodeServerError, err
		}

		region := extra["region"]
		apiConfig.Region = &region
		apiConfig.ApiKey = &instance.APIKey

		modelConfig.ModelClass = &providerInfo.Class

		newURL := map[string]string{
			region: extra["base_url"],
		}
		newProviderInfo := providerInfo.ModelDriver.NewInstance(newURL)

		var response *modelModule.ChatResponse
		response, err = newProviderInfo.ChatWithMessages(modelName, messages, apiConfig, modelConfig)
		if err != nil {
			return nil, common.CodeServerError, err
		}
		if response == nil {
			return nil, common.CodeServerError, errors.New("empty chat response")
		}

		return response, common.CodeSuccess, nil
	}

	return nil, common.CodeServerError, errors.New("model is disabled")
}

// ChatToModelStreamWithSender streams chat response directly via sender function (best performance, no channel)
func (m *ModelProviderService) ChatToModelStreamWithSender(providerName, instanceName, modelName, userID string, messages []modelModule.Message, apiConfig *modelModule.APIConfig, modelConfig *modelModule.ChatConfig, sender func(*string, *string) error) (common.ErrorCode, error) {
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

	modelInfo, err := m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(provider.ID, instance.ID, modelName)
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

		err = providerInfo.ModelDriver.ChatStreamlyWithSender(modelName, messages, apiConfig, modelConfig, sender)
		if err != nil {
			return common.CodeServerError, err
		}

		return common.CodeSuccess, nil
	}

	if modelInfo.Status == "active" {
		// For local deployed models
		providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
		if providerInfo == nil {
			return common.CodeNotFound, errors.New("provider not found")
		}

		var extra map[string]string
		err = json.Unmarshal([]byte(instance.Extra), &extra)
		if err != nil {
			return common.CodeServerError, err
		}

		region := extra["region"]
		apiConfig.Region = &region
		apiConfig.ApiKey = &instance.APIKey

		modelConfig.ModelClass = &providerInfo.Class

		newURL := map[string]string{
			region: extra["base_url"],
		}
		newProviderInfo := providerInfo.ModelDriver.NewInstance(newURL)

		err = newProviderInfo.ChatStreamlyWithSender(modelName, messages, apiConfig, modelConfig, sender)
		if err != nil {
			return common.CodeServerError, err
		}
		return common.CodeSuccess, nil
	}

	return common.CodeServerError, errors.New("model is disabled")
}

// EmbedText sends texts to the embedding model
func (m *ModelProviderService) EmbedText(providerName, instanceName, modelName, userID string, texts []string, apiConfig *modelModule.APIConfig, modelConfig *modelModule.EmbeddingConfig) ([]modelModule.EmbeddingData, common.ErrorCode, error) {
	if apiConfig == nil {
		apiConfig = &modelModule.APIConfig{}
	}
	if modelConfig == nil {
		modelConfig = &modelModule.EmbeddingConfig{}
	}

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

	modelInfo, err := m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(provider.ID, instance.ID, modelName)
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

		if !model.ModelTypeMap["embedding"] {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("provider %s model %s is not an embedding model", providerName, modelName))
		}

		var extra map[string]string
		err = json.Unmarshal([]byte(instance.Extra), &extra)
		if err != nil {
			return nil, common.CodeServerError, err
		}

		region := extra["region"]
		apiConfig.Region = &region
		apiConfig.ApiKey = &instance.APIKey

		var response []modelModule.EmbeddingData
		response, err = providerInfo.ModelDriver.Embed(&modelName, texts, apiConfig, modelConfig)
		if err != nil {
			return nil, common.CodeServerError, err
		}
		if response == nil || len(response) == 0 {
			return nil, common.CodeServerError, errors.New("empty embed response")
		}

		return response, common.CodeSuccess, nil
	}

	if modelInfo.Status == "active" {
		// For local deployed models
		providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
		if providerInfo == nil {
			return nil, common.CodeNotFound, errors.New("provider not found")
		}

		var extra map[string]string
		err = json.Unmarshal([]byte(instance.Extra), &extra)
		if err != nil {
			return nil, common.CodeServerError, err
		}

		region := extra["region"]
		apiConfig.Region = &region
		apiConfig.ApiKey = &instance.APIKey

		newURL := map[string]string{
			region: extra["base_url"],
		}
		newProviderInfo := providerInfo.ModelDriver.NewInstance(newURL)

		var response []modelModule.EmbeddingData
		response, err = newProviderInfo.Embed(&modelName, texts, apiConfig, modelConfig)
		if err != nil {
			return nil, common.CodeServerError, err
		}
		if response == nil || len(response) == 0 {
			return nil, common.CodeServerError, errors.New("empty embed response")
		}

		return response, common.CodeSuccess, nil
	}

	return nil, common.CodeServerError, errors.New("model is disabled")
}

// RerankDocument sends texts to the embedding model
func (m *ModelProviderService) RerankDocument(providerName, instanceName, modelName, userID, query string, documents []string, apiConfig *modelModule.APIConfig, modelConfig *modelModule.RerankConfig) (*modelModule.RerankResponse, common.ErrorCode, error) {
	if apiConfig == nil {
		apiConfig = &modelModule.APIConfig{}
	}
	if modelConfig == nil {
		modelConfig = &modelModule.RerankConfig{}
	}

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

	modelInfo, err := m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(provider.ID, instance.ID, modelName)
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

		if !model.ModelTypeMap["rerank"] {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("provider %s model %s is not an embedding model", providerName, modelName))
		}

		var extra map[string]string
		err = json.Unmarshal([]byte(instance.Extra), &extra)
		if err != nil {
			return nil, common.CodeServerError, err
		}

		region := extra["region"]
		apiConfig.Region = &region
		apiConfig.ApiKey = &instance.APIKey

		var response *modelModule.RerankResponse
		response, err = providerInfo.ModelDriver.Rerank(&modelName, query, documents, apiConfig, modelConfig)
		if err != nil {
			return nil, common.CodeServerError, err
		}

		return response, common.CodeSuccess, nil
	}

	if modelInfo.Status == "active" {
		// For local deployed models
		providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
		if providerInfo == nil {
			return nil, common.CodeNotFound, errors.New("provider not found")
		}

		var extra map[string]string
		err = json.Unmarshal([]byte(instance.Extra), &extra)
		if err != nil {
			return nil, common.CodeServerError, err
		}

		region := extra["region"]
		apiConfig.Region = &region
		apiConfig.ApiKey = &instance.APIKey

		newURL := map[string]string{
			region: extra["base_url"],
		}
		newProviderInfo := providerInfo.ModelDriver.NewInstance(newURL)

		var response *modelModule.RerankResponse
		response, err = newProviderInfo.Rerank(&modelName, query, documents, apiConfig, modelConfig)
		if err != nil {
			return nil, common.CodeServerError, err
		}

		return response, common.CodeSuccess, nil
	}

	return nil, common.CodeServerError, errors.New("model is disabled")
}

// TranscribeAudio transcribe audio file to text
func (m *ModelProviderService) TranscribeAudio(providerName, instanceName, modelName, userID string, audioFile *string, apiConfig *modelModule.APIConfig, asrConfig *modelModule.ASRConfig) (*modelModule.ASRResponse, common.ErrorCode, error) {
	if apiConfig == nil {
		apiConfig = &modelModule.APIConfig{}
	}
	if asrConfig == nil {
		asrConfig = &modelModule.ASRConfig{}
	}

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

	modelInfo, err := m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(provider.ID, instance.ID, modelName)
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

		var response *modelModule.ASRResponse
		response, err = providerInfo.ModelDriver.TranscribeAudio(&modelName, audioFile, apiConfig, asrConfig)
		if err != nil {
			return nil, common.CodeServerError, err
		}
		if response == nil {
			return nil, common.CodeServerError, errors.New("empty chat response")
		}

		return response, common.CodeSuccess, nil
	}

	if modelInfo.Status == "active" {
		// For local deployed models
		providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
		if providerInfo == nil {
			return nil, common.CodeNotFound, errors.New("provider not found")
		}

		var extra map[string]string
		err = json.Unmarshal([]byte(instance.Extra), &extra)
		if err != nil {
			return nil, common.CodeServerError, err
		}

		region := extra["region"]
		apiConfig.Region = &region
		apiConfig.ApiKey = &instance.APIKey

		newURL := map[string]string{
			region: extra["base_url"],
		}
		newProviderInfo := providerInfo.ModelDriver.NewInstance(newURL)

		var response *modelModule.ASRResponse
		response, err = newProviderInfo.TranscribeAudio(&modelName, audioFile, apiConfig, asrConfig)
		if err != nil {
			return nil, common.CodeServerError, err
		}
		if response == nil {
			return nil, common.CodeServerError, errors.New("empty chat response")
		}

		return response, common.CodeSuccess, nil
	}

	return nil, common.CodeServerError, errors.New("model is disabled")
}

// ChatToModelStreamWithSender streams chat response directly via sender function (best performance, no channel)
func (m *ModelProviderService) TranscribeAudioStream(providerName, instanceName, modelName, userID string, audioFile *string, apiConfig *modelModule.APIConfig, asrConfig *modelModule.ASRConfig, sender func(*string, *string) error) (common.ErrorCode, error) {
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

	modelInfo, err := m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(provider.ID, instance.ID, modelName)
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

		err = providerInfo.ModelDriver.TranscribeAudioWithSender(&modelName, audioFile, apiConfig, asrConfig, sender)
		if err != nil {
			return common.CodeServerError, err
		}

		return common.CodeSuccess, nil
	}

	if modelInfo.Status == "active" {
		// For local deployed models
		providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
		if providerInfo == nil {
			return common.CodeNotFound, errors.New("provider not found")
		}

		var extra map[string]string
		err = json.Unmarshal([]byte(instance.Extra), &extra)
		if err != nil {
			return common.CodeServerError, err
		}

		region := extra["region"]
		apiConfig.Region = &region
		apiConfig.ApiKey = &instance.APIKey

		newURL := map[string]string{
			region: extra["base_url"],
		}
		newProviderInfo := providerInfo.ModelDriver.NewInstance(newURL)

		err = newProviderInfo.TranscribeAudioWithSender(&modelName, audioFile, apiConfig, asrConfig, sender)
		if err != nil {
			return common.CodeServerError, err
		}
		return common.CodeSuccess, nil
	}

	return common.CodeServerError, errors.New("model is disabled")
}

// TranscribeAudio transcribe audio file to text
func (m *ModelProviderService) AudioSpeech(providerName, instanceName, modelName, userID string, audioContent *string, apiConfig *modelModule.APIConfig, ttsConfig *modelModule.TTSConfig) (*modelModule.TTSResponse, common.ErrorCode, error) {
	if apiConfig == nil {
		apiConfig = &modelModule.APIConfig{}
	}
	if ttsConfig == nil {
		ttsConfig = &modelModule.TTSConfig{}
	}

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

	modelInfo, err := m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(provider.ID, instance.ID, modelName)
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

		var response *modelModule.TTSResponse
		response, err = providerInfo.ModelDriver.AudioSpeech(&modelName, audioContent, apiConfig, ttsConfig)
		if err != nil {
			return nil, common.CodeServerError, err
		}
		if response == nil {
			return nil, common.CodeServerError, errors.New("empty chat response")
		}

		return response, common.CodeSuccess, nil
	}

	if modelInfo.Status == "active" {
		// For local deployed models
		providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
		if providerInfo == nil {
			return nil, common.CodeNotFound, errors.New("provider not found")
		}

		var extra map[string]string
		err = json.Unmarshal([]byte(instance.Extra), &extra)
		if err != nil {
			return nil, common.CodeServerError, err
		}

		region := extra["region"]
		apiConfig.Region = &region
		apiConfig.ApiKey = &instance.APIKey

		newURL := map[string]string{
			region: extra["base_url"],
		}
		newProviderInfo := providerInfo.ModelDriver.NewInstance(newURL)

		var response *modelModule.TTSResponse
		response, err = newProviderInfo.AudioSpeech(&modelName, audioContent, apiConfig, ttsConfig)
		if err != nil {
			return nil, common.CodeServerError, err
		}
		if response == nil {
			return nil, common.CodeServerError, errors.New("empty chat response")
		}

		return response, common.CodeSuccess, nil
	}

	return nil, common.CodeServerError, errors.New("model is disabled")
}

func (m *ModelProviderService) AudioSpeechStream(providerName, instanceName, modelName, userID string, audioContent *string, apiConfig *modelModule.APIConfig, ttsConfig *modelModule.TTSConfig, sender func(*string, *string) error) (common.ErrorCode, error) {
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

	modelInfo, err := m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(provider.ID, instance.ID, modelName)
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

		err = providerInfo.ModelDriver.AudioSpeechWithSender(&modelName, audioContent, apiConfig, ttsConfig, sender)
		if err != nil {
			return common.CodeServerError, err
		}

		return common.CodeSuccess, nil
	}

	if modelInfo.Status == "active" {
		// For local deployed models
		providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
		if providerInfo == nil {
			return common.CodeNotFound, errors.New("provider not found")
		}

		var extra map[string]string
		err = json.Unmarshal([]byte(instance.Extra), &extra)
		if err != nil {
			return common.CodeServerError, err
		}

		region := extra["region"]
		apiConfig.Region = &region
		apiConfig.ApiKey = &instance.APIKey

		newURL := map[string]string{
			region: extra["base_url"],
		}
		newProviderInfo := providerInfo.ModelDriver.NewInstance(newURL)

		err = newProviderInfo.AudioSpeechWithSender(&modelName, audioContent, apiConfig, ttsConfig, sender)
		if err != nil {
			return common.CodeServerError, err
		}
		return common.CodeSuccess, nil
	}

	return common.CodeServerError, errors.New("model is disabled")
}

func (m *ModelProviderService) OCRFile(providerName, instanceName, modelName, userID string, fileContent *string, apiConfig *modelModule.APIConfig, ocrConfig *modelModule.OCRConfig) (*modelModule.OCRResponse, common.ErrorCode, error) {
	if apiConfig == nil {
		apiConfig = &modelModule.APIConfig{}
	}
	if ocrConfig == nil {
		ocrConfig = &modelModule.OCRConfig{}
	}

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

	modelInfo, err := m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(provider.ID, instance.ID, modelName)
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

		var response *modelModule.OCRResponse
		response, err = providerInfo.ModelDriver.OCRFile(&modelName, fileContent, apiConfig, ocrConfig)
		if err != nil {
			return nil, common.CodeServerError, err
		}
		if response == nil {
			return nil, common.CodeServerError, errors.New("empty chat response")
		}

		return response, common.CodeSuccess, nil
	}

	if modelInfo.Status == "active" {
		// For local deployed models
		providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
		if providerInfo == nil {
			return nil, common.CodeNotFound, errors.New("provider not found")
		}

		var extra map[string]string
		err = json.Unmarshal([]byte(instance.Extra), &extra)
		if err != nil {
			return nil, common.CodeServerError, err
		}

		region := extra["region"]
		apiConfig.Region = &region
		apiConfig.ApiKey = &instance.APIKey

		newURL := map[string]string{
			region: extra["base_url"],
		}
		newProviderInfo := providerInfo.ModelDriver.NewInstance(newURL)

		var response *modelModule.OCRResponse
		response, err = newProviderInfo.OCRFile(&modelName, fileContent, apiConfig, ocrConfig)
		if err != nil {
			return nil, common.CodeServerError, err
		}
		if response == nil {
			return nil, common.CodeServerError, errors.New("empty chat response")
		}

		return response, common.CodeSuccess, nil
	}

	return nil, common.CodeServerError, errors.New("model is disabled")
}

// GetEmbeddingModel returns an EmbeddingModel wrapper for the given tenant
func (m *ModelProviderService) GetEmbeddingModel(tenantID, compositeModelName string) (*modelModule.EmbeddingModel, error) {
	driver, modelName, apiConfig, maxTokens, err := m.getModelConfig(tenantID, compositeModelName)
	if err != nil {
		return nil, err
	}
	return modelModule.NewEmbeddingModel(driver, &modelName, apiConfig, maxTokens), nil
}

// GetRerankModel returns a RerankModel wrapper for the given tenant
func (m *ModelProviderService) GetRerankModel(tenantID, compositeModelName string) (*modelModule.RerankModel, error) {
	driver, modelName, apiConfig, _, err := m.getModelConfig(tenantID, compositeModelName)
	if err != nil {
		return nil, err
	}
	return modelModule.NewRerankModel(driver, &modelName, apiConfig), nil
}

// GetChatModel returns a ChatModel wrapper for the given tenant
func (m *ModelProviderService) GetChatModel(tenantID, compositeModelName string) (*modelModule.ChatModel, error) {
	driver, modelName, apiConfig, _, err := m.getModelConfig(tenantID, compositeModelName)
	if err != nil {
		return nil, err
	}
	return modelModule.NewChatModel(driver, &modelName, apiConfig), nil
}

type AddCustomModelRequest struct {
	ProviderName string   `json:"provider_name"`
	InstanceName string   `json:"instance_name"`
	ModelName    string   `json:"model_name"`
	ModelTypes   []string `json:"model_types"`
	MaxTokens    int      `json:"max_tokens"`
	Thinking     *bool    `json:"thinking"`
}

func (m *ModelProviderService) AddCustomModel(request *AddCustomModelRequest, userID string) (common.ErrorCode, error) {
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
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, request.ProviderName)
	if err != nil {
		return common.CodeServerError, err
	}

	instance, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, request.InstanceName)
	if err != nil {
		return common.CodeServerError, err
	}

	_, err = m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(provider.ID, instance.ID, request.ModelName)
	if err == nil {
		return common.CodeConflict, errors.New("model already exists")
	}

	modelID, err := generateUUID1Hex()
	if err != nil {
		return common.CodeServerError, errors.New("fail to get UUID")
	}

	extra := make(map[string]interface{})
	extra["max_tokens"] = request.MaxTokens
	if request.Thinking != nil {
		extra["thinking"] = *request.Thinking
	}
	extra["model_types"] = request.ModelTypes
	// convert extra to string
	extraByte, err := json.Marshal(extra)
	if err != nil {
		return common.CodeServerError, errors.New("fail to marshal extra")
	}
	extraStr := string(extraByte)

	model := &entity.TenantModel{
		ID:         modelID,
		ModelName:  request.ModelName,
		ModelType:  request.ModelTypes[0],
		ProviderID: provider.ID,
		InstanceID: instance.ID,
		Status:     "active",
		Extra:      extraStr,
	}

	err = m.modelDAO.Create(model)
	if err != nil {
		return common.CodeServerError, err
	}

	return common.CodeSuccess, nil
}

// getModelConfig returns the model driver, model name, API config, and max tokens for a model
func (m *ModelProviderService) getModelConfig(tenantID, compositeModelName string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	modelName, instanceName, providerName, err := parseModelName(compositeModelName)
	if err != nil {
		return nil, "", nil, 0, err
	}

	// Check if provider exists
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return nil, "", nil, 0, err
	}
	if provider == nil {
		return nil, "", nil, 0, fmt.Errorf("provider %s not found", providerName)
	}

	instance, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return nil, "", nil, 0, err
	}
	if instance == nil {
		return nil, "", nil, 0, fmt.Errorf("instance %s not found for provider %s", instanceName, providerName)
	}

	var extra map[string]string
	err = json.Unmarshal([]byte(instance.Extra), &extra)
	if err != nil {
		return nil, "", nil, 0, err
	}
	region := extra["region"]

	providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
	if providerInfo == nil {
		return nil, "", nil, 0, fmt.Errorf("provider %s not found", providerName)
	}

	// Get model info to extract max_tokens
	modelInfo, err := dao.GetModelProviderManager().GetModelByName(providerName, modelName)
	maxTokens := 0
	if err == nil && modelInfo != nil {
		maxTokens = modelInfo.MaxTokens
	}

	_, err = m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(provider.ID, instance.ID, modelName)
	if err != nil {
		_, err = dao.GetModelProviderManager().GetModelByName(providerName, modelName)
		if err != nil {
			return nil, "", nil, 0, fmt.Errorf("provider %s model %s not found", providerName, modelName)
		}

		apiConfig := &modelModule.APIConfig{ApiKey: &instance.APIKey, Region: &region}
		return providerInfo.ModelDriver, modelName, apiConfig, maxTokens, nil
	}

	apiConfig := &modelModule.APIConfig{ApiKey: &instance.APIKey, Region: &region}
	return providerInfo.ModelDriver, modelName, apiConfig, maxTokens, nil
}
