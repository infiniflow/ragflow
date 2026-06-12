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
	"os"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/utility"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"
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

func newModelDriverForBaseURL(driver modelModule.ModelDriver, providerName, region, baseURL string) (modelModule.ModelDriver, error) {
	if driver == nil {
		return nil, fmt.Errorf("provider %s driver not found", providerName)
	}

	if strings.TrimSpace(baseURL) == "" {
		return driver, nil
	}

	baseURLByRegion := map[string]string{
		region: baseURL,
	}
	if region == "" {
		baseURLByRegion["default"] = baseURL
	}

	newDriver := driver.NewInstance(baseURLByRegion)
	if newDriver == nil {
		return nil, fmt.Errorf("provider %s does not support custom base_url", providerName)
	}

	return newDriver, nil
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

// CheckConnectionRequest carries the credentials and optional instance selector
// for checking provider connectivity without creating a new model instance.
type CheckConnectionRequest struct {
	APIKey  string `json:"api_key"`
	Region  string `json:"region"`
	BaseURL string `json:"base_url"`
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

	providerID, err := utility.GenerateUUID1()
	if err != nil {
		return common.CodeServerError, errors.New("fail to get UUID")
	}

	tenantModelProvider := &entity.TenantModelProvider{
		ID:           providerID,
		ProviderName: providerName,
		TenantID:     tenantID,
	}
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

func (m *ModelProviderService) ListSupportedModels(providerName, instanceName, userID string) ([]map[string]interface{}, error) {

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
		driver, err = newModelDriverForBaseURL(driver, providerName, region, baseURL)
		if err != nil {
			return nil, err
		}
	}

	modelList, err := driver.ListModels(apiConfig)
	if err != nil {
		return nil, err
	}

	var result []map[string]interface{}
	for _, model := range modelList {
		result = append(result, map[string]interface{}{
			"name":          model.Name,
			"max_dimension": model.MaxDimension,
			"dimensions":    model.Dimensions,
			"max_tokens":    model.MaxTokens,
			"model_types":   model.ModelTypes,
			"thinking":      model.Thinking,
		})
		modelData := map[string]interface{}{
			"name":        model.Name,
			"dimension":   model.MaxDimension,
			"max_tokens":  model.MaxTokens,
			"model_types": model.ModelTypes,
			"thinking":    model.Thinking,
		}
		if len(model.Dimensions) > 0 {
			modelData["dimensions"] = model.Dimensions
		}
		result = append(result, modelData)
	}
	return result, nil
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

	instanceID, err := utility.GenerateUUID1()
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

	tenantModelProvider := &entity.TenantModelInstance{
		ID:           instanceID,
		InstanceName: instanceName,
		ProviderID:   provider.ID,
		APIKey:       apiKey,
		Status:       "active",
		Extra:        extraStr,
	}
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
		"apikey":       instance.APIKey,
		"region":       extra["region"],
		"base_url":     extra["base_url"],
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
	baseURL := extra["base_url"]
	apiConfig.Region = &region
	apiConfig.ApiKey = &instance.APIKey
	apiConfig.BaseURL = &baseURL

	var result map[string]interface{}
	result, err = providerInfo.ModelDriver.Balance(apiConfig)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	return result, common.CodeSuccess, nil
}

func (m *ModelProviderService) CheckConnection(providerName, apiKey, region, baseURL string, userID string) (common.ErrorCode, error) {
	providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
	if providerInfo == nil {
		return common.CodeServerError, fmt.Errorf("provider %s not found", providerName)
	}

	apiKey = strings.TrimSpace(apiKey)
	region = strings.TrimSpace(region)
	baseURL = strings.TrimSpace(baseURL)
	if region == "" {
		region = "default"
	}

	driver := providerInfo.ModelDriver
	if strings.EqualFold(providerInfo.Class, "local") {
		if baseURL == "" {
			return common.CodeDataError, fmt.Errorf("base_url is required for local provider %s", providerName)
		}

		var err error
		driver, err = newModelDriverForBaseURL(driver, providerName, region, baseURL)
		if err != nil {
			return common.CodeServerError, err
		}
	}

	apiConfig := &modelModule.APIConfig{
		ApiKey: &apiKey,
		Region: &region,
	}

	err := driver.CheckConnection(apiConfig)
	if err != nil {
		return common.CodeServerError, err
	}

	return common.CodeSuccess, nil
}

func (m *ModelProviderService) CheckInstanceConnection(providerName, instanceName, userID string) (common.ErrorCode, error) {

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
		driver, err = newModelDriverForBaseURL(driver, providerName, region, baseURL)
		if err != nil {
			return common.CodeServerError, err
		}
	}

	err = driver.CheckConnection(apiConfig)
	if err != nil {
		return common.CodeServerError, err
	}
	return common.CodeSuccess, nil
}

func (m *ModelProviderService) ListTasks(providerName, instanceName, userID string) ([]modelModule.ListTaskStatus, common.ErrorCode, error) {

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

	driver := providerInfo.ModelDriver
	if baseURL, ok := extra["base_url"]; ok && baseURL != "" {
		driver, err = newModelDriverForBaseURL(driver, providerName, region, baseURL)
		if err != nil {
			return nil, common.CodeServerError, err
		}
	}

	var listTaskResponse []modelModule.ListTaskStatus
	listTaskResponse, err = driver.ListTasks(apiConfig)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	return listTaskResponse, common.CodeSuccess, nil
}

func (m *ModelProviderService) ShowTask(providerName, instanceName, taskID, userID string) (*modelModule.TaskResponse, common.ErrorCode, error) {

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

	driver := providerInfo.ModelDriver
	if baseURL, ok := extra["base_url"]; ok && baseURL != "" {
		driver, err = newModelDriverForBaseURL(driver, providerName, region, baseURL)
		if err != nil {
			return nil, common.CodeServerError, err
		}
	}

	var taskResponse *modelModule.TaskResponse
	taskResponse, err = driver.ShowTask(taskID, apiConfig)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	return taskResponse, common.CodeSuccess, nil
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
		modelID, err = utility.GenerateUUID1()
		if err != nil {
			return common.CodeServerError, errors.New("fail to get UUID")
		}

		var modelSchema *modelModule.Model
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

		var model *modelModule.Model = nil
		model, err = dao.GetModelProviderManager().GetModelByName(providerName, modelName)
		if err != nil {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("provider %s model %s not found", providerName, modelName))
		}

		if !model.ModelTypeMap["chat"] && !model.ModelTypeMap["vision"] {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is a chat or multimodal model", modelName, providerName))
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
		if modelInfo.ModelType != "chat" && modelInfo.ModelType != "vision" {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is a chat or multimodal model", modelName, providerName))
		}
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

		newProviderInfo, err := newModelDriverForBaseURL(providerInfo.ModelDriver, providerName, region, extra["base_url"])
		if err != nil {
			return nil, common.CodeServerError, err
		}

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

		var model *modelModule.Model = nil
		model, err = dao.GetModelProviderManager().GetModelByName(providerName, modelName)
		if err != nil {
			return common.CodeNotFound, err
		}

		if !model.ModelTypeMap["chat"] && !model.ModelTypeMap["vision"] {
			return common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is a chat or multimodal model", modelName, providerName))
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
		if modelInfo.ModelType != "chat" && modelInfo.ModelType != "vision" {
			return common.CodeServerError, errors.New(fmt.Sprintf("expect model %s@%s is a chat or multimodal model", modelName, providerName))
		}
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

		newProviderInfo, err := newModelDriverForBaseURL(providerInfo.ModelDriver, providerName, region, extra["base_url"])
		if err != nil {
			return common.CodeServerError, err
		}

		err = newProviderInfo.ChatStreamlyWithSender(modelName, messages, apiConfig, modelConfig, sender)
		if err != nil {
			return common.CodeServerError, err
		}
		return common.CodeSuccess, nil
	}

	return common.CodeServerError, errors.New("model is disabled")
}

func validateEmbeddingDimension(model *modelModule.Model, requested int) error {
	if requested <= 0 || model == nil {
		return nil
	}

	if len(model.Dimensions) > 0 {
		for _, dim := range model.Dimensions {
			if dim == requested {
				return nil
			}
		}
		return fmt.Errorf(
			"dimension %d is not supported by model %s, supported dimensions: %v",
			requested,
			model.Name,
			model.Dimensions,
		)
	}
	if model.MaxDimension != nil && requested > *model.MaxDimension {
		return fmt.Errorf(
			"dimension %d is not supported by model %s, max dimension: %d",
			requested,
			model.Name,
			*model.MaxDimension,
		)
	}

	return nil
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

		var model *modelModule.Model = nil
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

		if err := validateEmbeddingDimension(model, modelConfig.Dimension); err != nil {
			return nil, common.CodeBadRequest, err
		}

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
		if modelInfo.ModelType != "embedding" {
			return nil, common.CodeServerError, errors.New(fmt.Sprintf("expect model %s@%s is an embedding model", modelName, providerName))
		}
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

		newProviderInfo, err := newModelDriverForBaseURL(providerInfo.ModelDriver, providerName, region, extra["base_url"])
		if err != nil {
			return nil, common.CodeServerError, err
		}

		modelSchema, _ := dao.GetModelProviderManager().GetModelByName(providerName, modelName)
		if err := validateEmbeddingDimension(modelSchema, modelConfig.Dimension); err != nil {
			return nil, common.CodeBadRequest, err
		}

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

		var model *modelModule.Model = nil
		model, err = dao.GetModelProviderManager().GetModelByName(providerName, modelName)
		if err != nil {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("provider %s model %s not found", providerName, modelName))
		}

		if !model.ModelTypeMap["rerank"] {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("provider %s model %s is not a rerank model", providerName, modelName))
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
		if modelInfo.ModelType != "rerank" {
			return nil, common.CodeServerError, errors.New(fmt.Sprintf("expect model %s@%s is a rerank model", modelName, providerName))
		}
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

		newProviderInfo, err := newModelDriverForBaseURL(providerInfo.ModelDriver, providerName, region, extra["base_url"])
		if err != nil {
			return nil, common.CodeServerError, err
		}

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

		var model *modelModule.Model = nil
		model, err = dao.GetModelProviderManager().GetModelByName(providerName, modelName)
		if err != nil {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("provider %s model %s not found", providerName, modelName))
		}

		if !model.ModelTypeMap["asr"] {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("provider %s model %s is not an ASR model", providerName, modelName))
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
		if modelInfo.ModelType != "asr" {
			return nil, common.CodeServerError, errors.New(fmt.Sprintf("expect model %s@%s is an ASR model", modelName, providerName))
		}
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

		newProviderInfo, err := newModelDriverForBaseURL(providerInfo.ModelDriver, providerName, region, extra["base_url"])
		if err != nil {
			return nil, common.CodeServerError, err
		}

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

		var model *modelModule.Model = nil
		model, err = dao.GetModelProviderManager().GetModelByName(providerName, modelName)
		if err != nil {
			return common.CodeNotFound, err
		}
		if !model.ModelTypeMap["asr"] {
			return common.CodeNotFound, errors.New(fmt.Sprintf("provider %s model %s is not an ASR model", providerName, modelName))
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
		if modelInfo.ModelType != "asr" {
			return common.CodeServerError, errors.New(fmt.Sprintf("expect model %s@%s is an ASR model", modelName, providerName))
		}
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

		newProviderInfo, err := newModelDriverForBaseURL(providerInfo.ModelDriver, providerName, region, extra["base_url"])
		if err != nil {
			return common.CodeServerError, err
		}

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

		var model *modelModule.Model = nil
		model, err = dao.GetModelProviderManager().GetModelByName(providerName, modelName)
		if err != nil {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("provider %s model %s not found", providerName, modelName))
		}

		if !model.ModelTypeMap["tts"] {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("provider %s model %s is not a TTS model", providerName, modelName))
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
		if modelInfo.ModelType != "tts" {
			return nil, common.CodeServerError, errors.New(fmt.Sprintf("expect model %s@%s is a TTS model", modelName, providerName))
		}
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

		newProviderInfo, err := newModelDriverForBaseURL(providerInfo.ModelDriver, providerName, region, extra["base_url"])
		if err != nil {
			return nil, common.CodeServerError, err
		}

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

		var model *modelModule.Model = nil
		model, err = dao.GetModelProviderManager().GetModelByName(providerName, modelName)
		if err != nil {
			return common.CodeNotFound, err
		}

		if !model.ModelTypeMap["tts"] {
			return common.CodeNotFound, errors.New(fmt.Sprintf("provider %s model %s is not a TTS model", providerName, modelName))
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
		if modelInfo.ModelType != "tts" {
			return common.CodeServerError, errors.New(fmt.Sprintf("expect model %s@%s is a TTS model", modelName, providerName))
		}
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

		newProviderInfo, err := newModelDriverForBaseURL(providerInfo.ModelDriver, providerName, region, extra["base_url"])
		if err != nil {
			return common.CodeServerError, err
		}

		err = newProviderInfo.AudioSpeechWithSender(&modelName, audioContent, apiConfig, ttsConfig, sender)
		if err != nil {
			return common.CodeServerError, err
		}
		return common.CodeSuccess, nil
	}

	return common.CodeServerError, errors.New("model is disabled")
}

func (m *ModelProviderService) OCRFile(providerName, instanceName, modelName, userID string, content []byte, url *string, apiConfig *modelModule.APIConfig, ocrConfig *modelModule.OCRConfig) (*modelModule.OCRFileResponse, common.ErrorCode, error) {
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

		var model *modelModule.Model = nil
		model, err = dao.GetModelProviderManager().GetModelByName(providerName, modelName)
		if err != nil {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("provider %s model %s not found", providerName, modelName))
		}

		if !model.ModelTypeMap["ocr"] {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("provider %s model %s is not an OCR model", providerName, modelName))
		}

		var extra map[string]string
		err = json.Unmarshal([]byte(instance.Extra), &extra)
		if err != nil {
			return nil, common.CodeServerError, err
		}

		region := extra["region"]
		apiConfig.Region = &region
		apiConfig.ApiKey = &instance.APIKey

		var response *modelModule.OCRFileResponse
		response, err = providerInfo.ModelDriver.OCRFile(&modelName, content, url, apiConfig, ocrConfig)
		if err != nil {
			return nil, common.CodeServerError, err
		}
		if response == nil {
			return nil, common.CodeServerError, errors.New("empty chat response")
		}

		return response, common.CodeSuccess, nil
	}

	if modelInfo.Status == "active" {
		if modelInfo.ModelType != "ocr" {
			return nil, common.CodeServerError, errors.New(fmt.Sprintf("expect model %s@%s is an OCR model", modelName, providerName))
		}
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

		newProviderInfo, err := newModelDriverForBaseURL(providerInfo.ModelDriver, providerName, region, extra["base_url"])
		if err != nil {
			return nil, common.CodeServerError, err
		}

		var response *modelModule.OCRFileResponse
		response, err = newProviderInfo.OCRFile(&modelName, content, url, apiConfig, ocrConfig)
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

func (m *ModelProviderService) ParseFile(providerName, instanceName, modelName, userID string, content []byte, url *string, apiConfig *modelModule.APIConfig, parseFileConfig *modelModule.ParseFileConfig) (*modelModule.ParseFileResponse, common.ErrorCode, error) {
	if apiConfig == nil {
		apiConfig = &modelModule.APIConfig{}
	}
	if parseFileConfig == nil {
		parseFileConfig = &modelModule.ParseFileConfig{}
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

		var model *modelModule.Model = nil
		model, err = dao.GetModelProviderManager().GetModelByName(providerName, modelName)
		if err != nil {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("provider %s model %s not found", providerName, modelName))
		}

		if !model.ModelTypeMap["doc_parse"] {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("provider %s model %s is not a Document Parse model", providerName, modelName))
		}

		var extra map[string]string
		err = json.Unmarshal([]byte(instance.Extra), &extra)
		if err != nil {
			return nil, common.CodeServerError, err
		}

		region := extra["region"]
		apiConfig.Region = &region
		apiConfig.ApiKey = &instance.APIKey

		var response *modelModule.ParseFileResponse
		response, err = providerInfo.ModelDriver.ParseFile(&modelName, content, url, apiConfig, parseFileConfig)
		if err != nil {
			return nil, common.CodeServerError, err
		}
		if response == nil {
			return nil, common.CodeServerError, errors.New("empty chat response")
		}

		return response, common.CodeSuccess, nil
	}

	if modelInfo.Status == "active" {
		if modelInfo.ModelType != "doc_parse" {
			return nil, common.CodeServerError, errors.New(fmt.Sprintf("expect model %s@%s is a Document Parse model", modelName, providerName))
		}
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

		newProviderInfo, err := newModelDriverForBaseURL(providerInfo.ModelDriver, providerName, region, extra["base_url"])
		if err != nil {
			return nil, common.CodeServerError, err
		}

		var response *modelModule.ParseFileResponse
		response, err = newProviderInfo.ParseFile(&modelName, content, url, apiConfig, parseFileConfig)
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

// GetChatModel  returns a ChatModel wrapper for the given tenant
func (m *ModelProviderService) GetChatModel(tenantID, compositeModelName string) (*modelModule.ChatModel, error) {
	driver, modelName, apiConfig, _, err := m.getModelConfig(tenantID, compositeModelName)
	if err != nil {
		return nil, err
	}
	return modelModule.NewChatModel(driver, &modelName, apiConfig), nil
}

// GetRerankModel returns a RerankModel wrapper for the given tenant
func (m *ModelProviderService) GetRerankModel(tenantID, compositeModelName string) (*modelModule.RerankModel, error) {
	driver, modelName, apiConfig, _, err := m.getModelConfig(tenantID, compositeModelName)
	if err != nil {
		return nil, err
	}
	return modelModule.NewRerankModel(driver, &modelName, apiConfig), nil
}

type AddModelRequest struct {
	ProviderName string         `json:"provider_name"`
	InstanceName string         `json:"instance_name"`
	Models       []ModelRequest `json:"models"`
}

func (m *ModelProviderService) GetTenantDefaultModelByType(tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	if modelType == entity.ModelTypeOCR {
		return nil, "", nil, 0, fmt.Errorf("OCR model name is required")
	}

	tenantSvc := NewTenantService()
	modelName, err := tenantSvc.GetDefaultModelName(tenantID, modelType)
	if err != nil {
		return nil, "", nil, 0, fmt.Errorf("failed to get default model name for type %s: %w", modelType, err)
	}
	if modelName == "" {
		return nil, "", nil, 0, fmt.Errorf("no default %s model is set", modelType)
	}

	return m.GetModelConfigFromProviderInstance(tenantID, modelType, modelName)
}

// GetModelTypeByName returns the list of model types the given model is enrolled as.
func (m *ModelProviderService) GetModelTypeByName(tenantID, modelName string) ([]entity.ModelType, error) {
	common.Debug("GetModelTypeByName",
		zap.String("tenantID", tenantID),
		zap.String("modelName", modelName))

	pureModelName, instanceName, providerName, err := parseModelName(modelName)
	if err != nil {
		return nil, err
	}

	// Direct provider lookup
	provider, provErr := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if provErr != nil {
		return nil, fmt.Errorf("provider %q lookup failed: %w", providerName, provErr)
	}
	if provider == nil {
		return nil, fmt.Errorf("provider %q not found for model %q", providerName, modelName)
	}

	// Direct instance lookup
	instance, instErr := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if instErr != nil {
		return nil, fmt.Errorf("instance %q lookup failed: %w", instanceName, instErr)
	}
	if instance == nil {
		return nil, fmt.Errorf("instance %q not found for model %q", instanceName, modelName)
	}

	// Direct model lookup
	modelObjs, modelErr := m.modelDAO.GetModelsByProviderIDAndInstanceIDAndModelName(provider.ID, instance.ID, pureModelName)
	if modelErr == nil && len(modelObjs) > 0 {
		types := make([]entity.ModelType, 0, len(modelObjs))
		for _, obj := range modelObjs {
			types = append(types, entity.ModelType(obj.ModelType))
		}
		return types, nil
	}

	// Fallback: factory LLM catalog
	var extra map[string]string
	_ = json.Unmarshal([]byte(instance.Extra), &extra)
	region := extra["region"]

	targetFactoryName := providerName
	if region == "intl" && strings.EqualFold(providerName, "siliconflow") {
		targetFactoryName = "siliconflow_intl"
	}
	// Use the case-insensitive FindProvider / findModel helpers. The Go's
	// conf/models/*.json files use mixed case ("SiliconFlow") while the
	// Python's conf/llm_factories.json uses all-caps ("SILICONFLOW"); a strict
	// `==` here would fail for any case mismatch. The rest of the Go codebase
	// uses these helpers too.
	targetProvider := dao.GetModelProviderManager().FindProvider(targetFactoryName)
	if targetProvider == nil {
		return nil, fmt.Errorf("model provider config not found: %s", providerName)
	}
	for i := range targetProvider.Models {
		if strings.EqualFold(targetProvider.Models[i].Name, pureModelName) {
			if len(targetProvider.Models[i].ModelTypes) == 0 {
				return nil, fmt.Errorf("model %q has no model_types in factory catalog", pureModelName)
			}
			return []entity.ModelType{entity.ModelType(targetProvider.Models[i].ModelTypes[0])}, nil
		}
	}
	return nil, fmt.Errorf("model %q not found for model %q", pureModelName, modelName)
}

type AddCustomModelRequest struct {
	ProviderName string   `json:"provider_name"`
	InstanceName string   `json:"instance_name"`
	ModelName    string   `json:"model_name"`
	ModelTypes   []string `json:"model_types"`
	MaxTokens    int      `json:"max_tokens"`
	Thinking     *bool    `json:"thinking"`
}

type ModelRequest struct {
	ModelName  string   `json:"model_name"`
	ModelTypes []string `json:"model_types"`
	MaxTokens  int      `json:"max_tokens"`
	Thinking   *bool    `json:"thinking"`
}

func (m *ModelProviderService) AddModel(request *AddModelRequest, userID string) (common.ErrorCode, error) {
	if request == nil {
		return common.CodeBadRequest, errors.New("request is required")
	}
	if len(request.Models) == 0 {
		return common.CodeBadRequest, errors.New("models is required")
	}

	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return common.CodeServerError, err
	}
	if len(tenants) == 0 {
		return common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, request.ProviderName)
	if err != nil {
		return common.CodeServerError, err
	}

	instance, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, request.InstanceName)
	if err != nil {
		return common.CodeServerError, err
	}

	seen := make(map[string]struct{})
	models := make([]*entity.TenantModel, 0, len(request.Models))

	for _, model := range request.Models {
		modelName := strings.TrimSpace(model.ModelName)
		if len(model.ModelTypes) == 0 {
			return common.CodeBadRequest, errors.New("model types is required")
		}
		modelType := strings.TrimSpace(model.ModelTypes[0])

		if modelName == "" {
			return common.CodeBadRequest, errors.New("model name is required")
		}
		if modelType == "" {
			return common.CodeBadRequest, errors.New("model type is required")
		}

		duplicateKey := strings.ToLower(modelName)
		if _, ok := seen[duplicateKey]; ok {
			return common.CodeConflict, fmt.Errorf("duplicate model in request: %s", modelName)
		}
		seen[duplicateKey] = struct{}{}

		_, err = m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(provider.ID, instance.ID, modelName)
		if err == nil {
			return common.CodeConflict, fmt.Errorf("model already exists: %s", modelName)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return common.CodeServerError, err
		}

		var modelID string
		modelID, err = utility.GenerateUUID1()
		if err != nil {
			return common.CodeServerError, errors.New("fail to get UUID")
		}

		extra := map[string]interface{}{
			"max_tokens":  model.MaxTokens,
			"model_types": []string{modelType},
		}
		if model.Thinking != nil {
			extra["thinking"] = *model.Thinking
		}

		var extraByte []byte
		extraByte, err = json.Marshal(extra)
		if err != nil {
			return common.CodeServerError, errors.New("fail to marshal extra")
		}

		models = append(models, &entity.TenantModel{
			ID:         modelID,
			ModelName:  modelName,
			ModelType:  modelType,
			ProviderID: provider.ID,
			InstanceID: instance.ID,
			Status:     "active",
			Extra:      string(extraByte),
		})
	}

	if err = m.modelDAO.CreateBatch(models); err != nil {
		return common.CodeServerError, err
	}

	return common.CodeSuccess, nil
}

// modelName must be a composite name of the form "model@instance@provider" or
// "model@provider" — the provider is required and is looked up directly via
// tenant_model_provider. For 2-part names the instance defaults to "default".
// If the model is enrolled in tenant_model, that row is used (and INACTIVE rows
// raise). Otherwise the factory's LLM catalog is consulted, with
// region=intl + siliconflow redirected to the siliconflow_intl factory.
func (m *ModelProviderService) GetModelConfigFromProviderInstance(tenantID string, modelType entity.ModelType, modelName string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	common.Debug("GetModelConfigFromProviderInstance",
		zap.String("tenantID", tenantID),
		zap.String("modelName", modelName),
		zap.String("modelType", string(modelType)))

	// TEI builtin embedding short-circuit
	if modelType == entity.ModelTypeEmbedding && strings.Contains(os.Getenv("COMPOSE_PROFILES"), "tei-") {
		parts := strings.Split(modelName, "@")
		teiPure := parts[0]
		teiProvider := ""
		switch len(parts) {
		case 2:
			teiProvider = parts[1]
		case 3:
			teiProvider = parts[2]
		}
		if teiPure == os.Getenv("TEI_MODEL") && (teiProvider == "Builtin" || teiProvider == "") {
			builtinDriver := modelModule.GetBuiltinEmbeddingModel(teiPure)
			if builtinDriver == nil {
				return nil, "", nil, 0, fmt.Errorf("builtin (TEI) embedding model %q not found", teiPure)
			}
			teiBaseURL := os.Getenv("TEI_BASE_URL")
			apiConfig := &modelModule.APIConfig{ApiKey: nil, Region: nil, BaseURL: &teiBaseURL}
			return builtinDriver, teiPure, apiConfig, 0, nil
		}
	}

	// Generic Builtin provider short-circuit. The tenant_model_provider /
	// tenant_model_instance tables don't have a row for "Builtin" because it's
	// a local service, not a tenant-enrolled provider — so the direct lookups
	// below would raise. Mirrors the private getModelConfig's Builtin branch.
	//
	// Gated on ModelTypeEmbedding because the Builtin driver here is the TEI
	// embedding endpoint; the underlying BuiltinModel's Chat/Rerank/AudioSpeech
	// /OCR methods all return hard "not supported" errors. A chat/rerank/etc.
	// request that names a Builtin provider must fall through to the standard
	// branch, which surfaces an accurate "provider not found" instead of
	// handing back an embedding-only driver.
	parts := strings.Split(modelName, "@")
	if modelType == entity.ModelTypeEmbedding && len(parts) >= 2 && parts[len(parts)-1] == "Builtin" {
		pureModelName := parts[0]
		builtinDriver := modelModule.GetBuiltinEmbeddingModel(pureModelName)
		if builtinDriver == nil {
			return nil, "", nil, 0, fmt.Errorf("builtin embedding model %q not found", pureModelName)
		}
		apiKey := ""
		region := ""
		apiConfig := &modelModule.APIConfig{ApiKey: &apiKey, Region: &region}
		maxTokens := 0
		if mi, _ := dao.GetModelProviderManager().GetModelByName("Builtin", pureModelName); mi != nil {
			if mi.MaxTokens == nil {
				maxTokens = 0
			} else {
				maxTokens = *mi.MaxTokens
			}
		}
		return builtinDriver, pureModelName, apiConfig, maxTokens, nil
	}

	pureModelName, instanceName, providerName, err := parseModelName(modelName)
	if err != nil {
		return nil, "", nil, 0, err
	}

	// Direct provider lookup
	provider, provErr := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if provErr != nil {
		return nil, "", nil, 0, fmt.Errorf("provider %q lookup failed: %w", providerName, provErr)
	}
	if provider == nil {
		return nil, "", nil, 0, fmt.Errorf("provider %q not found for model %q", providerName, modelName)
	}

	// Direct instance lookup
	instance, instErr := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if instErr != nil {
		return nil, "", nil, 0, fmt.Errorf("instance %q lookup failed: %w", instanceName, instErr)
	}
	if instance == nil {
		return nil, "", nil, 0, fmt.Errorf("instance %q not found for model %q", instanceName, modelName)
	}

	// Decode api_key and extra fields from the instance row
	apiKey := instance.APIKey
	var extra map[string]string
	_ = json.Unmarshal([]byte(instance.Extra), &extra)
	region := extra["region"]
	baseURL := extra["base_url"]

	// Direct model lookup
	modelObj, modelErr := m.modelDAO.GetByProviderIDAndInstanceIDAndModelTypeAndModelName(
		provider.ID, instance.ID, string(modelType), pureModelName,
	)
	switch {
	case modelErr == nil:
		// Happy path: tenant enrolled this model.
		// INACTIVE check
		if modelObj.Status == "inactive" {
			return nil, "", nil, 0, fmt.Errorf("model %q is disabled", modelName)
		}

		providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
		if providerInfo == nil {
			return nil, "", nil, 0, fmt.Errorf("provider %q driver not found", providerName)
		}
		driver, driverErr := newModelDriverForBaseURL(providerInfo.ModelDriver, providerName, region, baseURL)
		if driverErr != nil {
			return nil, "", nil, 0, driverErr
		}
		maxTokens := 0
		if mi, _ := dao.GetModelProviderManager().GetModelByName(providerName, pureModelName); mi != nil {
			if mi.MaxTokens == nil {
				maxTokens = 0
			} else {
				maxTokens = *mi.MaxTokens
			}
		}
		apiConfig := &modelModule.APIConfig{ApiKey: &apiKey, Region: &region, BaseURL: &baseURL}
		return driver, modelObj.ModelName, apiConfig, maxTokens, nil
	case errors.Is(modelErr, gorm.ErrRecordNotFound):
		// Tenant hasn't enrolled this model. Fall through to the factory catalog.
		common.Debug("GetModelConfigFromProviderInstance: tenant has no row for model, falling back to factory catalog",
			zap.String("tenantID", tenantID),
			zap.String("providerName", providerName),
			zap.String("instanceName", instanceName),
			zap.String("modelType", string(modelType)),
			zap.String("modelName", pureModelName))
	default:
		// Surface unexpected DAO errors (e.g. transient DB failure) instead of
		// silently resolving from the factory catalog — that would mask disabled
		// or tenant-specific configurations.
		return nil, "", nil, 0, fmt.Errorf("model %q lookup failed: %w", modelName, modelErr)
	}

	// Fallback: factory LLM catalog
	targetFactoryName := providerName
	if region == "intl" && strings.EqualFold(providerName, "siliconflow") {
		targetFactoryName = "siliconflow_intl"
	}
	// Use the case-insensitive FindProvider / findModel helpers. The Go's
	// conf/models/*.json files use mixed case ("SiliconFlow") while the
	// Python's conf/llm_factories.json uses all-caps ("SILICONFLOW"); a strict
	// `==` here would fail for any case mismatch even though the Python passes
	// (its FACTORY_LLM_INFOS and the parsed provider_name happen to agree on
	// casing). The rest of the Go codebase uses these helpers too.
	targetProvider := dao.GetModelProviderManager().FindProvider(targetFactoryName)
	if targetProvider == nil {
		return nil, "", nil, 0, fmt.Errorf("model provider config not found: %s", providerName)
	}
	var llmInfo *modelModule.Model
	for i := range targetProvider.Models {
		if strings.EqualFold(targetProvider.Models[i].Name, pureModelName) {
			llmInfo = targetProvider.Models[i]
			break
		}
	}
	if llmInfo == nil {
		return nil, "", nil, 0, fmt.Errorf("model config not found: %s", modelName)
	}
	driver, driverErr := newModelDriverForBaseURL(targetProvider.ModelDriver, providerName, region, baseURL)
	if driverErr != nil {
		return nil, "", nil, 0, driverErr
	}
	apiConfig := &modelModule.APIConfig{ApiKey: &apiKey, Region: &region, BaseURL: &baseURL}
	maxTokens := 0
	if llmInfo.MaxTokens != nil {
		maxTokens = *llmInfo.MaxTokens
	}
	return driver, llmInfo.Name, apiConfig, maxTokens, nil
}

// getModelConfig returns the model driver, model name, API config, and max tokens for a model
func (m *ModelProviderService) getModelConfig(tenantID, compositeModelName string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	modelName, instanceName, providerName, err := parseModelName(compositeModelName)
	if err != nil {
		return nil, "", nil, 0, err
	}

	// Check if provider exists (skip for Builtin provider)
	var providerID string
	if providerName != "Builtin" {
		provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
		if err != nil {
			return nil, "", nil, 0, err
		}
		if provider == nil {
			return nil, "", nil, 0, fmt.Errorf("provider %s not found", providerName)
		}
		providerID = provider.ID
	} else {
		common.Debug("getModelConfig skipping provider lookup for Builtin")
	}

	// Get instance (skip for Builtin provider since it doesn't use tenant_model_instance)
	var instance *entity.TenantModelInstance
	if providerName != "Builtin" {
		instance, err = m.modelInstanceDAO.GetByProviderIDAndInstanceName(providerID, instanceName)
		if err != nil {
			return nil, "", nil, 0, err
		}
		if instance == nil {
			return nil, "", nil, 0, fmt.Errorf("instance %s not found for provider %s", instanceName, providerName)
		}
		common.Debug("getModelConfig instance found", zap.String("instanceName", instanceName))
	}

	var extra map[string]string
	var region string
	if instance != nil {
		err = json.Unmarshal([]byte(instance.Extra), &extra)
		if err != nil {
			return nil, "", nil, 0, err
		}
		region = extra["region"]
	}

	providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
	if providerName != "Builtin" && providerInfo == nil {
		return nil, "", nil, 0, fmt.Errorf("provider %s not found", providerName)
	}

	// Get model info to extract max_tokens
	modelInfo, err := dao.GetModelProviderManager().GetModelByName(providerName, modelName)
	maxTokens := 0
	if err == nil && modelInfo != nil {
		if modelInfo.MaxTokens == nil {
			maxTokens = 0
		} else {
			maxTokens = *modelInfo.MaxTokens
		}
	}

	// For Builtin provider, use empty APIKey and skip tenant_model lookup
	var apiKey string
	if providerName == "Builtin" {
		apiKey = ""
		// For Builtin, we need to get the ModelDriver from somewhere
		// Since Builtin models are handled locally, we return a special driver
		builtinDriver := modelModule.GetBuiltinEmbeddingModel(modelName)
		if builtinDriver == nil {
			return nil, "", nil, 0, fmt.Errorf("builtin embedding model %s not found", modelName)
		}
		apiConfig := &modelModule.APIConfig{ApiKey: &apiKey, Region: &region}
		return builtinDriver, modelName, apiConfig, maxTokens, nil
	} else {
		_, err = m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(providerID, instance.ID, modelName)
		if err != nil {
			_, err = dao.GetModelProviderManager().GetModelByName(providerName, modelName)
			if err != nil {
				return nil, "", nil, 0, fmt.Errorf("provider %s model %s not found", providerName, modelName)
			}
		}
		apiKey = instance.APIKey
	}

	apiConfig := &modelModule.APIConfig{ApiKey: &apiKey, Region: &region}
	return providerInfo.ModelDriver, modelName, apiConfig, maxTokens, nil
}

// getModelConfig returns the model driver, model name, API config, and max tokens for a model
func (m *ModelProviderService) ListAllModels(pageIndex, pageSize int) ([]map[string]interface{}, error) {
	models, err := dao.GetModelProviderManager().ListAllModels()
	if err != nil {
		return nil, err
	}
	if pageSize > 0 && pageIndex >= 0 && pageIndex*pageSize < len(models) {
		return models[pageIndex*pageSize : (pageIndex+1)*pageSize], nil
	}
	return models, nil
}

func (m *ModelProviderService) ShowModel(modelName string) (*modelModule.Model, error) {
	return dao.GetModelProviderManager().GetModelByNameOrAlias(modelName), nil
}
