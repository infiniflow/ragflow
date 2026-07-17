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
	"sort"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// parseModelName parses a composite model name in format "model@instance@provider" or "model@provider"
// Returns modelName, instanceName, providerName separately.
//
// The composite key is right-anchored: providerName is always the *last*
// '@'-separated field, instanceName is the second-to-last (when present),
// and everything to the left is the bare model name. Some model names
// legitimately contain '@' characters themselves (e.g. LM Studio embedding
// model IDs such as `text-embedding-nomic-embed-text-v1.5@q8_0`), which
// produces composite keys like
// `text-embedding-nomic-embed-text-v1.5@q8_0@lmstudio@LM-Studio`. When the
// split yields more than 3 fields we rejoin the leading fields back into the
// modelName so any embedded '@' characters are preserved verbatim.
func parseModelName(compositeName string) (modelName, instanceName, providerName string, err error) {
	parts := strings.Split(compositeName, "@")
	switch len(parts) {
	case 3:
		// Format: model@instance@provider
		return parts[0], parts[1], parts[2], nil
	case 2:
		// Format: model@provider -> instance defaults to "default"
		return parts[0], "default", parts[1], nil
	case 1:
		return parts[0], "", "", fmt.Errorf("provider name missing in model name: %s", compositeName)
	}
	// len(parts) > 3: any '@' characters embedded in the leftmost modelName
	// component must be preserved in that component instead of being dropped
	// or assigned to the instance/provider fields.
	n := len(parts)
	return strings.Join(parts[:n-2], "@"), parts[n-2], parts[n-1], nil
}

// splitRightAnchoredModelName is a bare-name-tolerant variant of
// parseModelName used by the Builtin / TEI short-circuit branches in
// GetModelConfigFromProviderInstance.
//
// Those branches must accept a bare model name (no provider suffix) where
// parseModelName would return an error, while still preserving any '@'
// characters embedded in the modelName portion of a multi-segment key.
// Returns the modelName, instanceName ("default" for the 2-segment form),
// and providerName ("" for the 1-segment form).
func splitRightAnchoredModelName(compositeName string) (modelName, instanceName, providerName string) {
	parts := strings.Split(compositeName, "@")
	switch len(parts) {
	case 3:
		return parts[0], parts[1], parts[2]
	case 2:
		// The 2-segment form "model@X" is ambiguous: X could be a provider
		// suffix (only "Builtin" is recognised by the TEI / Builtin
		// short-circuits that consume this helper) or part of the model
		// name itself (e.g. a quantization tag like "q8_0" in
		// "text-embedding-nomic-embed-text-v1.5@q8_0"). Treat the last
		// token as a provider only when it actually is one; otherwise
		// the whole string is the bare model name and the caller falls
		// through to its non-short-circuit path. The TEI short-circuit's
		// `modelName == teiModel` exact-match fast path already covers
		// the bare-default case where the embedded '@' happens to match
		// the TEI model identifier verbatim.
		if parts[1] == "Builtin" {
			return parts[0], "default", parts[1]
		}
		return compositeName, "", ""
	case 1:
		return parts[0], "", ""
	}
	n := len(parts)
	return strings.Join(parts[:n-2], "@"), parts[n-2], parts[n-1]
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
		tenantDAO:            dao.NewTenantDAO(),
		userTenantDAO:        dao.NewUserTenantDAO(),
	}
}

type ModelProviderService struct {
	modelProviderDAO     *dao.TenantModelProviderDAO
	modelInstanceDAO     *dao.TenantModelInstanceDAO
	modelDAO             *dao.TenantModelDAO
	modelGroupDAO        *dao.TenantModelGroupDAO
	modelGroupMappingDAO *dao.TenantModelGroupMappingDAO
	tenantDAO            *dao.TenantDAO
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
	providerName = strings.TrimSpace(providerName)

	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	existing, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return common.CodeServerError, err
	}
	if existing != nil {
		return common.CodeSuccess, nil
	}

	providerID := utility.GenerateToken()

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
		// Mirror Python's list_providers tenant branch: silently skip system-excluded
		// factory names (e.g. "Builtin", "Youdao", "FastEmbed", "BAAI",
		// "siliconflow_intl") and any stale entries whose factory is no longer in
		// the system pool. See api/apps/services/provider_api_service.py:108.
		if isExcludedTenantProvider(providerName) {
			continue
		}
		provider, err := dao.GetModelProviderManager().GetProviderByName(providerName)
		if err != nil {
			// Treat "provider not found in system pool" as a stale tenant entry
			// rather than a 500. Mirrors Python's factory_info_mapping.get(name)
			// truthy gate in api/apps/services/provider_api_service.py:108.
			if strings.Contains(err.Error(), "not found") {
				continue
			}
			return nil, common.CodeServerError, err
		}
		// provider["name"] is the catalog name from GetProviderByName (e.g.,
		// "SILICONFLOW"), which matches Python's factory_info["name"]. Do NOT
		// override it with providerName (the DB value), which may differ in
		// case from historical ToLower writes.

		// Set has_instance flag. Mirrors Python's:
		//   provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, name)
		//   has_instance = bool(provider_obj and TenantModelInstanceService.get_all_by_provider_id(provider_obj.id))
		provider["has_instance"] = m.providerHasInstance(tenantID, providerName)

		result = append(result, provider)
	}

	return result, common.CodeSuccess, nil
}

// providerHasInstance checks whether the given tenant's provider has any
// configured instances. Mirrors the Python pattern:
//
//	provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, name)
//	has_instance = bool(provider_obj and TenantModelInstanceService.get_all_by_provider_id(provider_obj.id))
func (m *ModelProviderService) providerHasInstance(tenantID, providerName string) bool {
	providerObj, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return false
	}
	instances, err := m.modelInstanceDAO.GetAllInstancesByProviderID(providerObj.ID)
	if err != nil {
		return false
	}
	return len(instances) > 0
}

// isExcludedTenantProvider returns true for system-pool names that the Python
// implementation (api/apps/services/provider_api_service.py:108) intentionally
// filters out when listing a tenant's providers.
func isExcludedTenantProvider(name string) bool {
	switch name {
	case "Youdao", "FastEmbed", "BAAI", "Builtin", "siliconflow_intl":
		return true
	}
	return false
}

func (m *ModelProviderService) DeleteModelProvider(userID, providerName string) (common.ErrorCode, error) {
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return common.CodeServerError, err
	}
	if len(tenants) == 0 {
		return common.CodeNotFound, errors.New("user has no tenants")
	}
	tenantID := tenants[0].TenantID

	// Find the provider first.
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return common.CodeNotFound, fmt.Errorf("provider %s not found", providerName)
	}

	// Delete all models and instances under this provider.
	instances, err := m.modelInstanceDAO.GetAllInstancesByProviderID(provider.ID)
	if err != nil {
		return common.CodeServerError, err
	}
	if len(instances) > 0 {
		instanceIDs := make([]string, len(instances))
		for i, inst := range instances {
			instanceIDs[i] = inst.ID
		}
		if _, err := m.modelDAO.DeleteByInstanceIDs(instanceIDs); err != nil {
			return common.CodeServerError, err
		}
		if _, err := m.modelInstanceDAO.DeleteByProviderID(provider.ID); err != nil {
			return common.CodeServerError, err
		}
	}

	_, err = m.modelProviderDAO.DeleteByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return common.CodeServerError, err
	}

	return common.CodeSuccess, nil
}

func (m *ModelProviderService) ListSupportedModels(providerName, instanceName, userID string) ([]map[string]interface{}, error) {
	providerName = strings.TrimSpace(providerName)

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
	}
	return result, nil
}

type CreateInstanceModelInfo struct {
	ModelName  string                 `json:"model_name"`
	ModelTypes []string               `json:"model_type"`
	MaxTokens  int                    `json:"max_tokens"`
	Extra      map[string]interface{} `json:"extra"`
}

func (m *ModelProviderService) getProviderByIDOrName(tenantID, providerIDOrName string) (*entity.TenantModelProvider, error) {
	provider, err := m.modelProviderDAO.GetByID(providerIDOrName)
	if err == nil && provider.TenantID == tenantID {
		return provider, nil
	}
	return m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, strings.TrimSpace(providerIDOrName))
}

func (m *ModelProviderService) CreateProviderInstance(providerIDOrName, instanceName, apiKey, baseURL, region, userID string, modelInfo []CreateInstanceModelInfo) (common.ErrorCode, error) {
	providerIDOrName = strings.TrimSpace(providerIDOrName)

	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	provider, err := m.getProviderByIDOrName(tenantID, providerIDOrName)
	if err != nil {
		return common.CodeNotFound, fmt.Errorf("provider '%s' does not exist", providerIDOrName)
	}
	providerName := provider.ProviderName

	// Normalize api_key: VLLM with empty api_key defaults to "x".
	// Mirrors Python's _normalize_provider_api_key.
	if strings.EqualFold(providerName, "VLLM") && apiKey == "" {
		apiKey = "x"
	}

	// Verify the API key against the provider.
	// Mirrors Python's verify_api_key (provider_api_service.py:596).
	modelVerifyResult := m.verifyProviderAPIKey(providerName, apiKey, region, baseURL, modelInfo)

	instanceID := utility.GenerateToken()

	extra := make(map[string]string)
	extra["region"] = region
	extra["base_url"] = baseURL
	extraByte, err := json.Marshal(extra)
	if err != nil {
		return common.CodeServerError, errors.New("fail to marshal extra")
	}
	extraStr := string(extraByte)

	tenantModelInstance := &entity.TenantModelInstance{
		ID:           instanceID,
		InstanceName: instanceName,
		ProviderID:   provider.ID,
		APIKey:       apiKey,
		Status:       "active",
		Extra:        extraStr,
	}
	err = m.modelInstanceDAO.Create(tenantModelInstance)
	if err != nil {
		return common.CodeServerError, fmt.Errorf("fail to create model instance: %s", err.Error())
	}

	// Add models with verify result in extra.
	if len(modelInfo) > 0 {
		for _, model := range modelInfo {
			if model.Extra == nil {
				model.Extra = make(map[string]interface{})
			}
			verifyStatus := modelVerifyResult[model.ModelName]
			if verifyStatus == "" {
				verifyStatus = entity.ModelVerifyUnknown
			}
			model.Extra["verify"] = verifyStatus
			if err := m.addModelToInstance(tenantID, providerName, instanceName, model); err != nil {
				return common.CodeServerError, err
			}
		}
	} else {
		// model_info not provided — add all factory default models.
		// Mirrors Python's create_provider_instance
		// (api/apps/services/provider_api_service.py:506-531).
		targetFactoryName := providerName
		if region == "intl" && strings.EqualFold(providerName, "siliconflow") {
			targetFactoryName = "siliconflow_intl"
		}
		factoryProvider := dao.GetModelProviderManager().FindProvider(targetFactoryName)
		if factoryProvider != nil {
			for _, llm := range factoryProvider.Models {
				verifyStatus := modelVerifyResult[llm.Name]
				if verifyStatus == "" {
					verifyStatus = entity.ModelVerifyUnknown
				}
				extra := map[string]interface{}{
					"verify": verifyStatus,
				}
				if llm.Tools != nil {
					extra["is_tools"] = llm.Tools.Support
				}
				if llm.Thinking != nil {
					extra["thinking"] = llm.Thinking.DefaultValue
				}
				if err := m.addModelToInstance(tenantID, providerName, instanceName, CreateInstanceModelInfo{
					ModelName:  llm.Name,
					ModelTypes: llm.ModelTypes,
					MaxTokens: func() int {
						if llm.MaxTokens != nil {
							return *llm.MaxTokens
						}
						return 8192
					}(),
					Extra: extra,
				}); err != nil {
					return common.CodeServerError, err
				}
			}
		}
	}

	return common.CodeSuccess, nil
}

// CreateNameOnlyProviderInstance creates a provider instance with only a name,
// skipping API key validation and model creation.
func (m *ModelProviderService) CreateNameOnlyProviderInstance(providerIDOrName, instanceName, userID string) (common.ErrorCode, error) {
	providerIDOrName = strings.TrimSpace(providerIDOrName)

	if instanceName == "default" {
		return common.CodeBadRequest, errors.New("instance name cannot be 'default'")
	}

	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return common.CodeServerError, err
	}
	if len(tenants) == 0 {
		return common.CodeNotFound, errors.New("user has no tenants")
	}
	tenantID := tenants[0].TenantID

	provider, err := m.getProviderByIDOrName(tenantID, providerIDOrName)
	if err != nil {
		return common.CodeNotFound, fmt.Errorf("provider '%s' does not exist", providerIDOrName)
	}

	instanceID := utility.GenerateToken()

	tenantModelInstance := &entity.TenantModelInstance{
		ID:           instanceID,
		InstanceName: instanceName,
		ProviderID:   provider.ID,
		APIKey:       "",
		Status:       "active",
		Extra:        "{}",
	}
	err = m.modelInstanceDAO.Create(tenantModelInstance)
	if err != nil {
		return common.CodeServerError, fmt.Errorf("fail to create model instance: %s", err.Error())
	}

	return common.CodeSuccess, nil
}

// verifyProviderAPIKey verifies the API key against the provider by calling
// the driver's CheckConnection. It returns a map from model name to verify
// status (success/fail/unknown).
func (m *ModelProviderService) verifyProviderAPIKey(providerName, apiKey, region, baseURL string, modelInfo []CreateInstanceModelInfo) map[string]string {
	result := make(map[string]string)

	providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
	if providerInfo == nil {
		// Provider not in system pool — mark all models as unknown.
		for _, model := range modelInfo {
			result[model.ModelName] = entity.ModelVerifyUnknown
		}
		return result
	}

	apiKey = strings.TrimSpace(apiKey)
	region = strings.TrimSpace(region)
	baseURL = strings.TrimSpace(baseURL)
	if region == "" {
		region = "default"
	}

	driver := providerInfo.ModelDriver
	if strings.EqualFold(providerInfo.Class, "local") {
		var err error
		driver, err = newModelDriverForBaseURL(driver, providerName, region, baseURL)
		if err != nil {
			for _, model := range modelInfo {
				result[model.ModelName] = entity.ModelVerifyFail
			}
			return result
		}
	}

	apiConfig := &modelModule.APIConfig{
		ApiKey:  &apiKey,
		Region:  &region,
		BaseURL: &baseURL,
	}

	verifyErr := driver.CheckConnection(apiConfig)
	verifyStatus := entity.ModelVerifySuccess
	if verifyErr != nil {
		verifyStatus = entity.ModelVerifyFail
	}

	for _, model := range modelInfo {
		result[model.ModelName] = verifyStatus
	}
	return result
}

// addModelToInstance creates a single model under the given provider instance.
func (m *ModelProviderService) addModelToInstance(tenantID, providerName, instanceName string, model CreateInstanceModelInfo) error {
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return fmt.Errorf("no provider found for provider '%s'", providerName)
	}

	instance, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return fmt.Errorf("no instance found for provider '%s' and instance '%s'", providerName, instanceName)
	}

	// Check for duplicate model.
	_, err = m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(provider.ID, instance.ID, model.ModelName)
	if err == nil {
		return fmt.Errorf("model '%s' already exists for provider '%s' and instance '%s'", model.ModelName, providerName, instanceName)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	// Compute model type bitmask.
	combinedType := entity.ModelType(0)
	for _, t := range model.ModelTypes {
		combinedType |= entity.ModelTypeFromString(t)
	}

	// Build extra fields. Mirrors Python's extra_fields in add_model_to_instance.
	extraFields := map[string]interface{}{
		"max_tokens": model.MaxTokens,
	}
	if model.Extra != nil {
		for k, v := range model.Extra {
			extraFields[k] = v
		}
	}
	extraBytes, err := json.Marshal(extraFields)
	if err != nil {
		return fmt.Errorf("fail to marshal extra: %s", err.Error())
	}

	modelID := utility.GenerateToken()
	tenantModel := &entity.TenantModel{
		ID:         modelID,
		ModelName:  model.ModelName,
		ModelType:  int(combinedType),
		ProviderID: provider.ID,
		InstanceID: instance.ID,
		Status:     "active",
		Extra:      string(extraBytes),
	}

	if err := m.modelDAO.Create(tenantModel); err != nil {
		return fmt.Errorf("fail to create model '%s': %s", model.ModelName, err.Error())
	}

	return nil
}

func (m *ModelProviderService) ListProviderInstances(providerIDOrName, userID string) ([]map[string]interface{}, common.ErrorCode, error) {
	providerIDOrName = strings.TrimSpace(providerIDOrName)

	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return nil, common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return nil, common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists — try by ID first, then by name.
	provider, err := m.getProviderByIDOrName(tenantID, providerIDOrName)
	if err != nil {
		return nil, common.CodeDataError, fmt.Errorf("No provider found for provider '%s'", providerIDOrName)
	}

	// Check if provider exists
	instances, err := m.modelInstanceDAO.GetAllInstancesByProviderID(provider.ID)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	// Always emit a non-nil slice so the JSON encoder serializes [] rather
	// than null when the tenant has no instances on this provider. The
	// front-end calls .map() / .forEach() on the response array (see
	// web/src/pages/user-setting/setting-model/...) and would otherwise
	// crash on a freshly created tenant.
	result := make([]map[string]interface{}, 0, len(instances))
	for _, instance := range instances {
		// Parse extra to extract region
		var extraFields map[string]string
		if instance.Extra != "" {
			if err := json.Unmarshal([]byte(instance.Extra), &extraFields); err != nil {
				return nil, common.CodeServerError, err
			}
		}
		if extraFields == nil {
			extraFields = make(map[string]string)
		}

		// Emit snake_case keys to match the Python
		// list_provider_instances response shape
		// (provider_api_service.py:563).
		result = append(result, map[string]interface{}{
			"id":            instance.ID,
			"instance_name": instance.InstanceName,
			"provider_id":   instance.ProviderID,
			"region":        extraFields["region"],
			"status":        instance.Status,
		})
	}

	return result, common.CodeSuccess, nil
}

func (m *ModelProviderService) ShowProviderInstance(providerName, instanceIDOrName, userID string) (map[string]interface{}, common.ErrorCode, error) {
	providerName = strings.TrimSpace(providerName)
	providerName = strings.ToLower(providerName)

	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return nil, common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return nil, common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists. Mirrors Python's:
	//   provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_id(tenant_id, provider_id_or_name)
	//   if not provider_obj:
	//       provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_id_or_name)
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return nil, common.CodeDataError, fmt.Errorf("No provider found for provider '%s'", providerName)
	}

	// Find the instance — try by ID first, then by name.
	// Mirrors Python's:
	//   _, instance_obj = TenantModelInstanceService.get_by_id(instance_id_or_name)
	//   if instance_obj and instance_obj.provider_id != provider_id: instance_obj = None
	//   if not instance_obj:
	//       instance_obj = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider_id, instance_id_or_name)
	var instance *entity.TenantModelInstance
	instance, err = m.modelInstanceDAO.GetByID(instanceIDOrName)
	if err != nil || instance.ProviderID != provider.ID {
		instance, err = m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceIDOrName)
	}
	if err != nil {
		return nil, common.CodeDataError, fmt.Errorf("No instance found for provider '%s' and instance '%s'", providerName, instanceIDOrName)
	}

	// Parse extra fields. Mirrors Python's:
	//   extra_fields = json.loads(instance_obj.extra) if instance_obj.extra else {}
	var extraFields map[string]string
	if instance.Extra != "" {
		if err := json.Unmarshal([]byte(instance.Extra), &extraFields); err != nil {
			return nil, common.CodeServerError, err
		}
	}
	if extraFields == nil {
		extraFields = make(map[string]string)
	}

	result := map[string]interface{}{
		"id":            instance.ID,
		"instance_name": instance.InstanceName,
		"provider_id":   provider.ID,
		"region":        extraFields["region"],
		"base_url":      extraFields["base_url"],
		"api_key":       instance.APIKey,
		"status":        instance.Status,
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

// ListTenantAddedModels returns the list of models the tenant has "added"
// across all of their provider instances. It is the Go port of Python's
// models_api_service.list_tenant_added_models
// (api/apps/services/models_api_service.py:300) and is the response
// contract for GET /api/v1/models — the endpoint that
// web/src/hooks/use-llm-request.tsx → useFetchAllAddedModels consumes.
//
// Per the Python algorithm, for each (provider × instance) we cross-reference the factory catalog (internal/entity/models/model.go
// ProviderManager.Providers) with the per-tenant overrides in
// tenant_model:
//
//	active_model_types   = tenant_model rows with status='active'
//	inactive_model_types = tenant_model rows with status='inactive'
//	factory_model_types  = provider.Models[i].ModelTypes
//	model_types = (factory ∪ active) \ inactive
//
// The Go port never WRITES to tenant_model, so in practice every model
// from the factory catalog is treated as added unless explicitly
// disabled (which today can only happen via SQL — the Go port has no
// enable/disable endpoint path that mutates tenant_model). This is
// intentional: the previous Go contract mistakenly routed /api/v1/models
// to ListTenantDefaultModels (which only enumerates the 6-7 default
// tenant fields and returned `[]` for any tenant without defaults),
// breaking the front-end's "View Models" list entirely.
func (m *ModelProviderService) ListTenantAddedModels(userID, ownerTenantID, modelTypeFilter string) ([]map[string]interface{}, common.ErrorCode, error) {
	tenant, code, err := m.resolveModelListTenant(userID, ownerTenantID)
	if err != nil {
		return nil, code, err
	}
	if tenant == nil {
		return []map[string]interface{}{}, common.CodeSuccess, nil
	}
	tenantID := tenant.ID
	tenantName := ""
	if tenant.Name != nil {
		tenantName = *tenant.Name
	}

	if modelTypeFilter != "" {
		modelTypeFilter = strings.ToLower(strings.TrimSpace(modelTypeFilter))
	}

	// Mirror Python's ensure_*_from_env calls.
	_ = m.ensureMineruFromEnv(tenantID)
	_ = m.ensurePaddleOCREnabledFromEnv(tenantID)
	_ = m.ensureOpenDataLoaderFromEnv(tenantID)

	var modelTypeFilterBin entity.ModelType
	if modelTypeFilter != "" {
		modelTypeFilterBin = entity.ModelTypeFromString(modelTypeFilter)
	}

	providers, err := m.modelProviderDAO.GetByTenantID(tenantID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if len(providers) == 0 {
		return []map[string]interface{}{}, common.CodeSuccess, nil
	}

	providerIDs := make([]string, 0, len(providers))
	providerInfoByID := make(map[string]*entity.TenantModelProvider, len(providers))
	for _, p := range providers {
		providerIDs = append(providerIDs, p.ID)
		providerInfoByID[p.ID] = p
	}

	instances, err := m.modelInstanceDAO.GetByProviderIDs(providerIDs)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if len(instances) == 0 {
		return []map[string]interface{}{}, common.CodeSuccess, nil
	}

	instanceIDs := make([]string, 0, len(instances))
	instanceInfoByID := make(map[string]*entity.TenantModelInstance, len(instances))
	for _, inst := range instances {
		instanceIDs = append(instanceIDs, inst.ID)
		instanceInfoByID[inst.ID] = inst
	}

	// Fetch tenant model records and filter by model_type if needed.
	modelRecords, err := m.modelDAO.GetModelsByProviderIDsAndInstanceIDs(providerIDs, instanceIDs)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	// Repair records that have model_type == 0. Before the ModelTypeFromString
	// fix that added "asr" support, models whose factory catalog used the
	// canonical name "asr" (instead of the legacy "speech2text") were stored
	// with model_type = 0. Re-derive the correct bitmask from the factory
	// catalog and persist it so the model shows up in typed queries.
	providerManager := dao.GetModelProviderManager()
	for _, rec := range modelRecords {
		if rec.ModelType != 0 {
			continue
		}
		provInfo := providerInfoByID[rec.ProviderID]
		if provInfo == nil {
			continue
		}
		factoryModel, lookupErr := providerManager.GetModelByName(provInfo.ProviderName, rec.ModelName)
		if lookupErr != nil || len(factoryModel.ModelTypes) == 0 {
			continue
		}
		combinedType := entity.ModelType(0)
		for _, t := range factoryModel.ModelTypes {
			combinedType |= entity.ModelTypeFromString(t)
		}
		if combinedType != 0 {
			rec.ModelType = int(combinedType)
			_ = m.modelDAO.UpdateByID(rec.ID, map[string]interface{}{"model_type": int(combinedType)})
		}
	}

	var targetRecords []*entity.TenantModel
	if modelTypeFilterBin != 0 {
		for _, rec := range modelRecords {
			if entity.ModelType(rec.ModelType)&modelTypeFilterBin != 0 {
				targetRecords = append(targetRecords, rec)
			}
		}
	} else {
		targetRecords = modelRecords
	}

	// Build model rank map from factory catalog (mirrors Python's model_rank_map).
	modelRankMap := make(map[string]int) // key: "providerName@modelName"
	factoryRankMapping := make(map[string]int)
	for i := range providerManager.Providers {
		factory := &providerManager.Providers[i]
		factoryRankMapping[factory.Name] = -factory.Rank
		for _, llm := range factory.Models {
			rank := 500
			if llm.Rank != nil {
				rank = *llm.Rank
			}
			modelRankMap[factory.Name+"@"+llm.Name] = rank
		}
	}

	added := make([]map[string]interface{}, 0, len(targetRecords))
	for _, modelRecord := range targetRecords {
		provInfo, provOK := providerInfoByID[modelRecord.ProviderID]
		instInfo, instOK := instanceInfoByID[modelRecord.InstanceID]
		if !provOK || !instOK {
			continue
		}
		rank := modelRankMap[provInfo.ProviderName+"@"+modelRecord.ModelName]
		if rank == 0 {
			rank = 500
		}
		added = append(added, map[string]interface{}{
			"model_id":      modelRecord.ID,
			"tenant_id":     provInfo.TenantID,
			"tenant_name":   tenantName,
			"model_type":    entity.ModelType(modelRecord.ModelType).HumanReadable(),
			"name":          modelRecord.ModelName,
			"provider_id":   modelRecord.ProviderID,
			"provider_name": provInfo.ProviderName,
			"instance_id":   modelRecord.InstanceID,
			"instance_name": instInfo.InstanceName,
			"rank":          rank,
		})
	}

	// Add TEI Builtin embedding model if configured (mirrors Python).
	composeProfiles := common.GetEnv(common.EnvComposeProfiles)
	teiModel := common.GetEnv(common.EnvTEIModel)
	if strings.Contains(composeProfiles, "tei-") && teiModel != "" {
		if modelTypeFilter == "" || modelTypeFilter == "embedding" {
			teiAlreadyAdded := false
			for _, m := range added {
				if pn, _ := m["provider_name"].(string); pn == "Builtin" {
					if n, _ := m["name"].(string); n == teiModel {
						teiAlreadyAdded = true
						break
					}
				}
			}
			if !teiAlreadyAdded {
				rank := modelRankMap["Builtin@"+teiModel]
				if rank == 0 {
					rank = 500
				}
				added = append(added, map[string]interface{}{
					"model_id":      "",
					"tenant_id":     tenant.ID,
					"tenant_name":   tenantName,
					"model_type":    []string{"embedding"},
					"name":          teiModel,
					"provider_id":   "",
					"provider_name": "Builtin",
					"instance_id":   "",
					"instance_name": "default",
					"rank":          rank,
				})
			}
		}
	}

	// Sort by factory rank desc, provider_name, instance_name, model rank desc, name.
	sort.Slice(added, func(i, j int) bool {
		pi, _ := added[i]["provider_name"].(string)
		pj, _ := added[j]["provider_name"].(string)
		fi := factoryRankMapping[pi]
		fj := factoryRankMapping[pj]
		if fi != fj {
			return fi < fj // factory rank: smaller (more negative) = higher priority
		}
		if pi != pj {
			return pi < pj
		}
		ini, _ := added[i]["instance_name"].(string)
		inj, _ := added[j]["instance_name"].(string)
		if ini != inj {
			return ini < inj
		}
		ri, _ := added[i]["rank"].(int)
		rj, _ := added[j]["rank"].(int)
		if ri != rj {
			return ri > rj // model rank desc
		}
		ni, _ := added[i]["name"].(string)
		nj, _ := added[j]["name"].(string)
		return ni < nj
	})

	return added, common.CodeSuccess, nil
}

func (m *ModelProviderService) resolveModelListTenant(userID, ownerTenantID string) (*entity.Tenant, common.ErrorCode, error) {
	if ownerTenantID == "" {
		tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
		if err != nil {
			return nil, common.CodeServerError, err
		}
		if len(tenants) == 0 {
			return nil, common.CodeSuccess, nil
		}
		ownerTenantID = tenants[0].TenantID
	} else {
		relations, err := m.userTenantDAO.GetByUserID(userID)
		if err != nil {
			return nil, common.CodeServerError, err
		}
		allowed := false
		for _, rel := range relations {
			if rel.TenantID == ownerTenantID {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, common.CodeAuthenticationError, fmt.Errorf("permission denied")
		}
	}

	tenant, err := m.tenantDAO.GetByID(ownerTenantID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeNotFound, fmt.Errorf("tenant %s not found", ownerTenantID)
		}
		return nil, common.CodeServerError, err
	}
	return tenant, common.CodeSuccess, nil
}

// ensureMineruFromEnv mirrors Python's ensure_mineru_from_env.
// It ensures a MinerU OCR provider instance exists when env vars are configured.
func (m *ModelProviderService) ensureMineruFromEnv(tenantID string) error {
	config := collectEnvConfig(mineruEnvKeys, mineruDefaultConfig)
	return m.ensureOCRProviderFromEnv(tenantID, "MinerU", "mineru-from-env", config)
}

// ensurePaddleOCREnabledFromEnv mirrors Python's ensure_paddleocr_from_env.
func (m *ModelProviderService) ensurePaddleOCREnabledFromEnv(tenantID string) error {
	config := collectEnvConfig(paddleOCREnvKeys, paddleOCRDefaultConfig)
	return m.ensureOCRProviderFromEnv(tenantID, "PaddleOCR", "paddleocr-from-env", config)
}

// ensureOpenDataLoaderFromEnv mirrors Python's ensure_opendataloader_from_env.
func (m *ModelProviderService) ensureOpenDataLoaderFromEnv(tenantID string) error {
	config := collectEnvConfig(openDataLoaderEnvKeys, openDataLoaderDefaultConfig)
	return m.ensureOCRProviderFromEnv(tenantID, "OpenDataLoader", "opendataloader-from-env", config)
}

// env key / default config tables for the three OCR providers.
// Mirrors common/constants.py MINERU_ENV_KEYS, PADDLEOCR_ENV_KEYS, OPENDATALOADER_ENV_KEYS.
var (
	mineruEnvKeys = []string{
		common.EnvMineruApiServer,
		"MINERU_OUTPUT_DIR",
		common.EnvMineruBackend,
		"MINERU_SERVER_URL",
		"MINERU_DELETE_OUTPUT",
	}
	mineruDefaultConfig = map[string]interface{}{
		common.EnvMineruApiServer: "",
		"MINERU_OUTPUT_DIR":       "",
		common.EnvMineruBackend:   "pipeline",
		"MINERU_SERVER_URL":       "",
		"MINERU_DELETE_OUTPUT":    1,
	}
	paddleOCREnvKeys = []string{
		common.EnvPaddleOCRBaseUrl,
		common.EnvPaddleOCRApiURL,
		common.EnvPaddleOCRAccessToken,
		common.EnvPaddleOCRAlgorithm,
	}
	paddleOCRDefaultConfig = map[string]interface{}{
		common.EnvPaddleOCRBaseUrl:     "",
		common.EnvPaddleOCRApiURL:      "",
		common.EnvPaddleOCRAccessToken: nil,
		common.EnvPaddleOCRAlgorithm:   "PaddleOCR-VL",
	}
	openDataLoaderEnvKeys = []string{
		common.EnvOpenDataLoaderApiServer,
	}
	openDataLoaderDefaultConfig = map[string]interface{}{
		common.EnvOpenDataLoaderApiServer: "",
	}
)

// collectEnvConfig collects environment variable values for the given keys
// into a map pre-populated with defaultConfig. Returns nil if none of the
// env vars are actually set, matching Python's _collect_env_config.
func collectEnvConfig(envKeys []string, defaultConfig map[string]interface{}) map[string]interface{} {
	config := make(map[string]interface{}, len(defaultConfig))
	for k, v := range defaultConfig {
		config[k] = v
	}
	found := false
	for _, key := range envKeys {
		value := os.Getenv(key)
		if value != "" {
			found = true
			config[key] = value
		}
	}
	if !found {
		return nil
	}
	return config
}

// ensureOCRProviderFromEnv mirrors Python's _ensure_ocr_provider_from_env.
// It finds or creates a provider, instance, and model for the given OCR provider.
func (m *ModelProviderService) ensureOCRProviderFromEnv(tenantID, providerName, modelName string, config map[string]interface{}) error {
	if config == nil {
		return nil
	}

	// 1. Find or create the provider.
	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		if !dao.IsNotFoundErr(err) {
			return fmt.Errorf("failed to get provider %s: %w", providerName, err)
		}
		providerID := utility.GenerateToken()
		provider = &entity.TenantModelProvider{
			ID:           providerID,
			TenantID:     tenantID,
			ProviderName: providerName,
		}
		if err := m.modelProviderDAO.Create(provider); err != nil {
			return fmt.Errorf("failed to create provider %s: %w", providerName, err)
		}
	}

	// 2. Find or create the instance (api_key stores the JSON config for dedup).
	apiKeyBytes, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config for %s: %w", providerName, err)
	}
	apiKey := string(apiKeyBytes)

	instance, err := m.modelInstanceDAO.GetInstanceByApiKey(apiKey, provider.ID)
	if err != nil {
		if !dao.IsNotFoundErr(err) {
			return fmt.Errorf("failed to get instance for %s: %w", providerName, err)
		}
		instanceID := utility.GenerateToken()
		instance = &entity.TenantModelInstance{
			ID:           instanceID,
			ProviderID:   provider.ID,
			InstanceName: modelName,
			APIKey:       apiKey,
			Extra:        "{}",
		}
		if err := m.modelInstanceDAO.Create(instance); err != nil {
			return fmt.Errorf("failed to create instance for %s: %w", providerName, err)
		}
	}

	// 3. Find or create the model.
	_, err = m.modelDAO.GetByProviderIDAndInstanceIDAndModelTypeAndModelName(
		provider.ID,
		instance.ID,
		int(entity.ModelTypeOCR),
		modelName,
	)
	if err != nil {
		if !dao.IsNotFoundErr(err) {
			return fmt.Errorf("failed to get model for %s: %w", providerName, err)
		}
		extraBytes, err := json.Marshal(map[string]int{"max_tokens": 0})
		if err != nil {
			return fmt.Errorf("failed to marshal extra for %s model: %w", providerName, err)
		}
		modelID := utility.GenerateToken()
		tenantModel := &entity.TenantModel{
			ID:         modelID,
			ModelName:  modelName,
			ProviderID: provider.ID,
			InstanceID: instance.ID,
			ModelType:  int(entity.ModelTypeOCR),
			Status:     "active",
			Extra:      string(extraBytes),
		}
		if err := m.modelDAO.Create(tenantModel); err != nil {
			return fmt.Errorf("failed to create model for %s: %w", providerName, err)
		}
	}

	return nil
}

func (m *ModelProviderService) AlterProviderInstance(userID, providerIDOrName, instanceIDOrName, newInstanceName, apiKey, baseURL, region string, modelInfo []CreateInstanceModelInfo, verify bool) (common.ErrorCode, error) {
	providerIDOrName = strings.TrimSpace(providerIDOrName)

	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return common.CodeServerError, err
	}
	if len(tenants) == 0 {
		return common.CodeNotFound, errors.New("user has no tenants")
	}
	tenantID := tenants[0].TenantID

	provider, err := m.getProviderByIDOrName(tenantID, providerIDOrName)
	if err != nil {
		return common.CodeNotFound, fmt.Errorf("provider '%s' does not exist", providerIDOrName)
	}
	providerName := provider.ProviderName

	// Find the instance — try by ID first, then by name.
	instance, err := m.modelInstanceDAO.GetByID(instanceIDOrName)
	if err != nil || instance.ProviderID != provider.ID {
		instance, err = m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceIDOrName)
	}
	if err != nil {
		return common.CodeNotFound, fmt.Errorf("no instance found for provider '%s' and instance '%s'", providerName, instanceIDOrName)
	}

	// Normalize api_key: VLLM with empty api_key defaults to "x".
	if strings.EqualFold(providerName, "vllm") && apiKey == "" {
		apiKey = "x"
	}

	// Verify API key if requested.
	modelVerifyResult := make(map[string]string)
	if verify {
		modelVerifyResult = m.verifyProviderAPIKey(providerName, apiKey, region, baseURL, modelInfo)
	}

	// Update instance record.
	instanceUpdates := map[string]interface{}{
		"api_key": apiKey,
	}
	if newInstanceName != "" && newInstanceName != instance.InstanceName {
		instanceUpdates["instance_name"] = newInstanceName
	}

	extraFields := make(map[string]interface{})
	if baseURL != "" {
		extraFields["base_url"] = baseURL
	}
	if region != "" {
		extraFields["region"] = region
	}
	// Preserve existing extra fields not overwritten.
	existingExtra := make(map[string]interface{})
	if instance.Extra != "" {
		if err := json.Unmarshal([]byte(instance.Extra), &existingExtra); err != nil {
			return common.CodeServerError, err
		}
	}
	for k, v := range extraFields {
		existingExtra[k] = v
	}
	extraBytes, err := json.Marshal(existingExtra)
	if err != nil {
		return common.CodeServerError, err
	}
	instanceUpdates["extra"] = string(extraBytes)
	if err := m.modelInstanceDAO.UpdateByID(instance.ID, instanceUpdates); err != nil {
		return common.CodeServerError, fmt.Errorf("fail to update instance: %s", err.Error())
	}

	// Use the (possibly updated) instance_name for model operations.
	effectiveInstanceName := instance.InstanceName
	if newInstanceName != "" {
		effectiveInstanceName = newInstanceName
	}

	// Upsert models: add new ones, update existing ones, remove ones no longer selected.
	existingModels, err := m.modelDAO.GetModelsByInstanceID(instance.ID)
	if err != nil {
		return common.CodeServerError, err
	}
	existingModelMap := make(map[string]*entity.TenantModel)
	for _, mdl := range existingModels {
		existingModelMap[mdl.ModelName] = mdl
	}

	// Delete models that are no longer in the submitted model_info.
	submittedModelNames := make(map[string]bool)
	if modelInfo != nil {
		for _, mdl := range modelInfo {
			if mdl.ModelName != "" {
				submittedModelNames[mdl.ModelName] = true
			}
		}
	}
	var idsToRemove []string
	for name, mdl := range existingModelMap {
		if !submittedModelNames[name] {
			idsToRemove = append(idsToRemove, mdl.ID)
		}
	}
	if len(idsToRemove) > 0 {
		if _, err := m.modelDAO.DeleteByIDs(idsToRemove); err != nil {
			return common.CodeServerError, err
		}
	}

	// Add or update models from model_info.
	if modelInfo != nil {
		for _, mdl := range modelInfo {
			if mdl.ModelName == "" {
				continue
			}
			// Attach verify status.
			if verify {
				verifyStatus := modelVerifyResult[mdl.ModelName]
				if verifyStatus == "" {
					verifyStatus = entity.ModelVerifyUnknown
				}
				if mdl.Extra == nil {
					mdl.Extra = make(map[string]interface{})
				}
				mdl.Extra["verify"] = verifyStatus
			}

			if existingMdl, exists := existingModelMap[mdl.ModelName]; exists {
				// Update existing model.
				updates := make(map[string]interface{})
				if len(mdl.ModelTypes) > 0 {
					combinedType := entity.ModelType(0)
					for _, t := range mdl.ModelTypes {
						combinedType |= entity.ModelTypeFromString(t)
					}
					if int(combinedType) != existingMdl.ModelType {
						updates["model_type"] = int(combinedType)
					}
				}
				mergedExtra := make(map[string]interface{})
				if existingMdl.Extra != "" {
					json.Unmarshal([]byte(existingMdl.Extra), &mergedExtra)
				}
				if mdl.Extra != nil {
					for k, v := range mdl.Extra {
						mergedExtra[k] = v
					}
				}
				if mdl.MaxTokens > 0 {
					mergedExtra["max_tokens"] = mdl.MaxTokens
				}
				extraBytes, _ := json.Marshal(mergedExtra)
				updates["extra"] = string(extraBytes)
				if len(updates) > 0 {
					if err := m.modelDAO.UpdateByID(existingMdl.ID, updates); err != nil {
						return common.CodeServerError, err
					}
				}
			} else {
				// Add new model.
				if err := m.addModelToInstance(tenantID, providerName, effectiveInstanceName, mdl); err != nil {
					return common.CodeServerError, err
				}
			}
		}
	}

	return common.CodeSuccess, nil
}

func (m *ModelProviderService) DropProviderInstances(providerIDOrName, userID string, instanceIDOrNames []string) (common.ErrorCode, error) {
	if len(instanceIDOrNames) == 0 {
		return common.CodeBadRequest, errors.New("instances is required")
	}

	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Find provider by ID or name
	provider, err := m.getProviderByIDOrName(tenantID, providerIDOrName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return common.CodeNotFound, fmt.Errorf("no provider found for provider %q", providerIDOrName)
		}
		return common.CodeServerError, err
	}

	// First pass: resolve all instance IDs and collect not-found instances.
	// Mirrors Python's drop_provider_instances which returns error immediately
	// if any instance does not exist, without performing partial deletion.
	notExistInstances := make([]string, 0)
	instanceIDs := make([]string, 0, len(instanceIDOrNames))
	for _, idOrName := range instanceIDOrNames {
		var instance *entity.TenantModelInstance
		// Try by ID first, then by name — same as Python.
		if idOrName != "" {
			instance, err = m.modelInstanceDAO.GetByID(idOrName)
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return common.CodeServerError, err
			}
			if instance != nil && instance.ProviderID != provider.ID {
				instance = nil
			}
		}
		if instance == nil {
			instance, err = m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, idOrName)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					notExistInstances = append(notExistInstances, idOrName)
					continue
				}
				return common.CodeServerError, err
			}
		}
		instanceIDs = append(instanceIDs, instance.ID)
	}

	if len(notExistInstances) > 0 {
		return common.CodeNotFound, fmt.Errorf("no instance found for provider %q and instances %q", providerIDOrName, notExistInstances)
	}

	// Second pass: delete models and instances by IDs.
	// Mirrors Python's: delete_models_by_instance_ids(instance_ids)
	//                   TenantModelInstanceService.delete_by_ids(instance_ids)
	if _, err := m.modelDAO.DeleteByInstanceIDs(instanceIDs); err != nil {
		return common.CodeServerError, err
	}
	if _, err := m.modelInstanceDAO.DeleteByIDs(instanceIDs); err != nil {
		return common.CodeServerError, err
	}

	return common.CodeSuccess, nil
}

func (m *ModelProviderService) DropInstanceModels(providerIDOrName, instanceIDOrName, userID string, modelNames []string) (common.ErrorCode, error) {
	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return common.CodeServerError, err
	}

	if len(tenants) == 0 {
		return common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Get provider by ID or name (matches Python's get_by_tenant_id_and_provider_id → get_by_tenant_id_and_provider_name fallback).
	provider, err := m.getProviderByIDOrName(tenantID, providerIDOrName)
	if err != nil {
		return common.CodeDataError, fmt.Errorf("No provider found for provider '%s'", providerIDOrName)
	}

	// Get instance by ID or name (matches Python's get_by_id → get_by_provider_id_and_instance_name fallback).
	instance, err := m.modelInstanceDAO.GetByID(instanceIDOrName)
	if err != nil || instance.ProviderID != provider.ID {
		instance, err = m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceIDOrName)
	}
	if err != nil {
		return common.CodeDataError, fmt.Errorf("No instance found for provider '%s' and instance '%s'", providerIDOrName, instanceIDOrName)
	}

	// Fetch all models under this instance and validate all requested names exist.
	modelObjs, err := m.modelDAO.GetModelsByInstanceID(instance.ID)
	if err != nil {
		return common.CodeServerError, err
	}

	existingNames := make(map[string]struct{}, len(modelObjs))
	for _, obj := range modelObjs {
		existingNames[obj.ModelName] = struct{}{}
	}

	notExist := make([]string, 0)
	for _, name := range modelNames {
		if _, ok := existingNames[name]; !ok {
			notExist = append(notExist, name)
		}
	}
	if len(notExist) > 0 {
		return common.CodeNotFound, fmt.Errorf("Models %v not found for provider '%s' and instance '%s'", notExist, providerIDOrName, instanceIDOrName)
	}

	// Collect IDs of only the requested models to delete.
	requestedNames := make(map[string]struct{}, len(modelNames))
	for _, name := range modelNames {
		requestedNames[name] = struct{}{}
	}
	idsToDelete := make([]string, 0, len(modelNames))
	for _, obj := range modelObjs {
		if _, ok := requestedNames[obj.ModelName]; ok {
			idsToDelete = append(idsToDelete, obj.ID)
		}
	}

	if _, err := m.modelDAO.DeleteByIDs(idsToDelete); err != nil {
		return common.CodeServerError, err
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

	// Find provider by ID or name
	provider, err := m.getProviderByIDOrName(tenantID, providerName)
	if err != nil {
		return nil, err
	}

	// Find instance by ID first, then by name
	instance, err := m.modelInstanceDAO.GetByID(instanceName)
	if err != nil || instance.ProviderID != provider.ID {
		instance, err = m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
		if err != nil {
			return nil, err
		}
	}

	// Get all models for this instance
	modelObjs, err := m.modelDAO.GetModelsByInstanceID(instance.ID)
	if err != nil {
		return nil, err
	}

	modelList := make([]map[string]interface{}, 0, len(modelObjs))
	for _, model := range modelObjs {
		modelExtra := make(map[string]interface{})
		if model.Extra != "" {
			_ = json.Unmarshal([]byte(model.Extra), &modelExtra)
		}

		maxTokens := 8192
		if v, ok := modelExtra["max_tokens"]; ok {
			switch val := v.(type) {
			case float64:
				maxTokens = int(val)
			case int:
				maxTokens = val
			}
		}

		verify := "unknown"
		if v, ok := modelExtra["verify"].(string); ok {
			verify = v
		}

		features := make([]string, 0)
		if v, ok := modelExtra["is_tools"]; ok && isTruthy(v) {
			features = append(features, "is_tools")
		}
		if v, ok := modelExtra["thinking"]; ok && isTruthy(v) {
			features = append(features, "thinking")
		}

		modelList = append(modelList, map[string]interface{}{
			"name":       model.ModelName,
			"model_type": entity.ModelType(model.ModelType).HumanReadable(),
			"max_tokens": maxTokens,
			"status":     model.Status,
			"verify":     verify,
			"features":   features,
		})
	}

	// Sort by name
	sort.Slice(modelList, func(i, j int) bool {
		return modelList[i]["name"].(string) < modelList[j]["name"].(string)
	})

	return modelList, nil
}

func (m *ModelProviderService) AlterModel(providerName, instanceName, modelName, userID, modelID string, updateDict map[string]interface{}) (common.ErrorCode, error) {
	modelName = strings.TrimSpace(modelName)
	modelID = strings.TrimSpace(modelID)
	if modelName == "" && modelID == "" {
		return common.CodeBadRequest, errors.New("model name or model ID is required")
	}

	// Validate status early, before any DB lookup.
	if status, ok := updateDict["status"].(string); ok && status != "" {
		if status != "active" && status != "inactive" {
			return common.CodeBadRequest, errors.New("status must be 'active' or 'inactive'")
		}
	}

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
	provider, err := m.getProviderByIDOrName(tenantID, providerName)
	if err != nil {
		return common.CodeServerError, err
	}

	instance, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return common.CodeServerError, err
	}

	var model *entity.TenantModel
	if modelName != "" {
		model, err = m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(provider.ID, instance.ID, modelName)
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return common.CodeServerError, err
			}
			return common.CodeNotFound, fmt.Errorf("model %q not found for provider %q and instance %q", modelName, providerName, instanceName)
		}
		if modelID != "" && model.ID != modelID {
			return common.CodeBadRequest, errors.New("model ID does not match model name")
		}
	} else {
		model, err = m.modelDAO.GetByID(modelID)
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return common.CodeServerError, err
			}
			return common.CodeNotFound, fmt.Errorf("model with ID %q not found", modelID)
		}
		if model.ProviderID != provider.ID || model.InstanceID != instance.ID {
			return common.CodeNotFound, fmt.Errorf("model with ID %q does not belong to provider %q and instance %q", modelID, providerName, instanceName)
		}
	}

	toUpdate := make(map[string]interface{})

	// Handle status (validation already done above, here we only apply the change)
	if status, ok := updateDict["status"].(string); ok && status != "" {
		if status != model.Status {
			toUpdate["status"] = status
		}
	}

	// Handle model_type
	if modelTypeVal, ok := updateDict["model_type"]; ok && modelTypeVal != nil {
		targetModelType := resolveModelType(modelTypeVal)
		if targetModelType != model.ModelType {
			toUpdate["model_type"] = targetModelType
		}
	}

	// Handle extra (max_tokens, extra)
	newExtra := make(map[string]interface{})
	if extraVal, ok := updateDict["extra"]; ok && extraVal != nil {
		if extraMap, ok := extraVal.(map[string]interface{}); ok {
			for k, v := range extraMap {
				newExtra[k] = v
			}
		}
	}
	if maxTokensVal, ok := updateDict["max_tokens"]; ok {
		switch v := maxTokensVal.(type) {
		case int:
			if v > 0 {
				newExtra["max_tokens"] = v
			}
		case float64:
			if v > 0 {
				newExtra["max_tokens"] = int(v)
			}
		}
	}
	if len(newExtra) > 0 {
		dbExtra := make(map[string]interface{})
		if model.Extra != "" {
			_ = json.Unmarshal([]byte(model.Extra), &dbExtra)
		}
		for k, v := range newExtra {
			dbExtra[k] = v
		}
		extraBytes, err := json.Marshal(dbExtra)
		if err != nil {
			return common.CodeServerError, err
		}
		toUpdate["extra"] = string(extraBytes)
	}

	if len(toUpdate) > 0 {
		if err := m.modelDAO.UpdateByID(model.ID, toUpdate); err != nil {
			return common.CodeServerError, err
		}
	}

	return common.CodeSuccess, nil
}

// resolveModelType converts a model_type value (string, []string, or float64) to an int bitmask.
func resolveModelType(val interface{}) int {
	switch v := val.(type) {
	case string:
		return int(entity.ModelTypeFromString(v))
	case []interface{}:
		types := make([]string, 0, len(v))
		for _, t := range v {
			if s, ok := t.(string); ok {
				types = append(types, s)
			}
		}
		return int(entity.ModelTypeFromStrings(types))
	case float64:
		return int(v)
	default:
		return 0
	}
}

type ModelInstanceAndProviderInfo struct {
	ProviderEntity *entity.TenantModelProvider
	ProviderInfo   *modelModule.Provider
	InstanceEntity *entity.TenantModelInstance
	ModelEntity    *entity.TenantModel
	ModelInfo      *modelModule.Model
	APIConfig      *modelModule.APIConfig
}

type tenantModelExtra struct {
	MaxTokens    *int     `json:"max_tokens"`
	ModelTypes   []string `json:"model_types"`
	MaxDimension *int     `json:"max_dimension"`
	Dimensions   []int    `json:"dimensions"`
	Thinking     *bool    `json:"thinking"`
}

func modelInfoWithTenantExtra(modelInfo *modelModule.Model, modelEntity *entity.TenantModel) (*modelModule.Model, error) {
	if modelInfo == nil || modelEntity == nil || strings.TrimSpace(modelEntity.Extra) == "" {
		return modelInfo, nil
	}

	var extra tenantModelExtra
	if err := json.Unmarshal([]byte(modelEntity.Extra), &extra); err != nil {
		return nil, err
	}

	model := *modelInfo
	model.ModelTypes = append([]string(nil), modelInfo.ModelTypes...)
	model.Dimensions = append([]int(nil), modelInfo.Dimensions...)
	model.Alias = append([]string(nil), modelInfo.Alias...)
	if modelInfo.ModelTypeMap != nil {
		model.ModelTypeMap = make(map[string]bool, len(modelInfo.ModelTypeMap))
		for modelType, enabled := range modelInfo.ModelTypeMap {
			model.ModelTypeMap[modelType] = enabled
		}
	}
	if modelInfo.Thinking != nil {
		thinking := *modelInfo.Thinking
		model.Thinking = &thinking
	}

	if extra.MaxTokens != nil && *extra.MaxTokens > 0 {
		model.MaxTokens = extra.MaxTokens
	}
	if len(extra.ModelTypes) > 0 {
		model.ModelTypes = append([]string(nil), extra.ModelTypes...)
		model.ModelTypeMap = make(map[string]bool, len(extra.ModelTypes))
		for _, modelType := range extra.ModelTypes {
			model.ModelTypeMap[modelType] = true
		}
	}
	if extra.MaxDimension != nil && *extra.MaxDimension > 0 {
		model.MaxDimension = extra.MaxDimension
	}
	if len(extra.Dimensions) > 0 {
		model.Dimensions = append([]int(nil), extra.Dimensions...)
	}
	if extra.Thinking != nil {
		if model.Thinking == nil {
			model.Thinking = &modelModule.ModelThinking{}
		}
		model.Thinking.DefaultValue = *extra.Thinking
	}

	return &model, nil
}

func maxTokensFromTenantModelExtra(modelEntity *entity.TenantModel, fallback int) (int, error) {
	if modelEntity == nil || strings.TrimSpace(modelEntity.Extra) == "" {
		return fallback, nil
	}
	var extra tenantModelExtra
	if err := json.Unmarshal([]byte(modelEntity.Extra), &extra); err != nil {
		return 0, err
	}
	if extra.MaxTokens != nil && *extra.MaxTokens > 0 {
		return *extra.MaxTokens, nil
	}
	return fallback, nil
}

func (m *ModelProviderService) getModelInstanceAndProviderByName(providerName, instanceName, modelName *string, userID string, apiConfig *modelModule.APIConfig) (*ModelInstanceAndProviderInfo, error) {
	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return nil, err
	}

	if len(tenants) == 0 {
		return nil, err
	}

	tenantID := tenants[0].TenantID

	// Check if provider exists
	providerEntity, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, *providerName)
	if err != nil {
		return nil, err
	}

	instanceEntity, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(providerEntity.ID, *instanceName)
	if err != nil {
		return nil, err
	}

	modelEntity, err := m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(providerEntity.ID, instanceEntity.ID, *modelName)
	if err != nil {
		// Not found model
		modelEntity = nil
	}

	providerInfo := dao.GetModelProviderManager().FindProvider(*providerName)
	if providerInfo == nil {
		return nil, errors.New("provider not found")
	}

	modelInfo, err := dao.GetModelProviderManager().GetModelByName(*providerName, *modelName)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("provider %s model %s not found", *providerName, *modelName))
	}
	modelInfo, err = modelInfoWithTenantExtra(modelInfo, modelEntity)
	if err != nil {
		return nil, err
	}

	var extra map[string]string
	err = json.Unmarshal([]byte(instanceEntity.Extra), &extra)
	if err != nil {
		return nil, err
	}

	if apiConfig == nil {
		apiConfig = &modelModule.APIConfig{}
	}

	region := extra["region"]
	baseURL := extra["base_url"]

	apiConfig.ApiKey = &instanceEntity.APIKey
	apiConfig.BaseURL = &baseURL
	apiConfig.Region = &region

	var result = &ModelInstanceAndProviderInfo{
		ProviderEntity: providerEntity,
		ProviderInfo:   providerInfo,
		InstanceEntity: instanceEntity,
		ModelEntity:    modelEntity,
		ModelInfo:      modelInfo,
		APIConfig:      apiConfig,
	}

	return result, nil
}

func (m *ModelProviderService) getModelInstanceAndProviderByID(modelID *string, userID string, apiConfig *modelModule.APIConfig) (*ModelInstanceAndProviderInfo, error) {
	// Get tenant ID from user
	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return nil, err
	}

	if len(tenants) == 0 {
		return nil, err
	}

	tenantID := tenants[0].TenantID

	modelEntity, err := m.modelDAO.GetByID(*modelID)
	if err != nil {
		return nil, err
	}

	instanceEntity, err := m.modelInstanceDAO.GetByID(modelEntity.InstanceID)
	if err != nil {
		return nil, err
	}

	providerEntity, err := m.modelProviderDAO.GetByID(instanceEntity.ProviderID)
	if err != nil {
		return nil, err
	}

	if providerEntity.TenantID != tenantID {
		return nil, errors.New("provider not found")
	}

	providerInfo := dao.GetModelProviderManager().FindProvider(providerEntity.ProviderName)
	if providerInfo == nil {
		return nil, errors.New("provider not found")
	}

	modelInfo, err := dao.GetModelProviderManager().GetModelByName(providerEntity.ProviderName, modelEntity.ModelName)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("provider %s model %s not found", providerEntity.ProviderName, modelEntity.ModelName))
	}
	modelInfo, err = modelInfoWithTenantExtra(modelInfo, modelEntity)
	if err != nil {
		return nil, err
	}

	var extra map[string]string
	err = json.Unmarshal([]byte(instanceEntity.Extra), &extra)
	if err != nil {
		return nil, err
	}

	if apiConfig == nil {
		apiConfig = &modelModule.APIConfig{}
	}

	region := extra["region"]
	baseURL := extra["base_url"]

	apiConfig.ApiKey = &instanceEntity.APIKey
	apiConfig.BaseURL = &baseURL
	apiConfig.Region = &region

	var result = &ModelInstanceAndProviderInfo{
		ProviderEntity: providerEntity,
		ProviderInfo:   providerInfo,
		InstanceEntity: instanceEntity,
		ModelEntity:    modelEntity,
		ModelInfo:      modelInfo,
		APIConfig:      apiConfig,
	}

	return result, nil
}

// ChatToModelWithMessages sends messages to the model with messages array
func (m *ModelProviderService) ChatToModelWithMessages(providerName, instanceName, modelName, modelID *string, userID string, messages []modelModule.Message, apiConfig *modelModule.APIConfig, modelConfig *modelModule.ChatConfig) (*modelModule.ChatResponse, common.ErrorCode, error) {

	var err error
	var info *ModelInstanceAndProviderInfo

	if modelID != nil {
		info, err = m.getModelInstanceAndProviderByID(modelID, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	} else {
		info, err = m.getModelInstanceAndProviderByName(providerName, instanceName, modelName, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	}

	if modelConfig == nil {
		modelConfig = &modelModule.ChatConfig{}
	}
	modelConfig.ModelClass = info.ModelInfo.Class
	if modelConfig.Thinking == nil && info.ModelInfo.Thinking != nil {
		thinking := info.ModelInfo.Thinking.DefaultValue
		modelConfig.Thinking = &thinking
	}
	resolvedModelName := info.ModelInfo.Name
	if info.ModelEntity != nil && info.ModelEntity.ModelName != "" {
		resolvedModelName = info.ModelEntity.ModelName
	}
	resolvedProviderName := info.ProviderEntity.ProviderName

	var response *modelModule.ChatResponse
	var modelDriver modelModule.ModelDriver

	if info.ModelEntity == nil {
		if !info.ModelInfo.ModelTypeMap["chat"] && !info.ModelInfo.ModelTypeMap["vision"] {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is a chat or multimodal model", resolvedModelName, resolvedProviderName))
		}
		modelDriver = info.ProviderInfo.ModelDriver
	} else {
		// model entity exists
		if info.ModelEntity.Status == "active" {
			if !entity.ModelType(info.ModelEntity.ModelType).Has(entity.ModelTypeChat) && !entity.ModelType(info.ModelEntity.ModelType).Has(entity.ModelTypeImage2Text) {
				return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is a chat or multimodal model", resolvedModelName, resolvedProviderName))
			}

			modelDriver, err = newModelDriverForBaseURL(info.ProviderInfo.ModelDriver, resolvedProviderName, *info.APIConfig.Region, *info.APIConfig.BaseURL)
			if err != nil {
				return nil, common.CodeServerError, err
			}
		} else {
			return nil, common.CodeServerError, errors.New("model is inactive")
		}
	}

	response, err = modelDriver.ChatWithMessages(resolvedModelName, messages, info.APIConfig, modelConfig)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if response == nil {
		return nil, common.CodeServerError, errors.New("empty chat response")
	}

	return response, common.CodeSuccess, nil
}

// ChatToModelStreamWithSender streams chat response directly via sender function ( the best performance, no channel)
func (m *ModelProviderService) ChatToModelStreamWithSender(providerName, instanceName, modelName, modelID *string, userID string, messages []modelModule.Message, apiConfig *modelModule.APIConfig, modelConfig *modelModule.ChatConfig, sender func(*string, *string) error) (common.ErrorCode, error) {

	var err error
	var info *ModelInstanceAndProviderInfo

	if modelID != nil {
		info, err = m.getModelInstanceAndProviderByID(modelID, userID, apiConfig)
		if err != nil || info == nil {
			return common.CodeNotFound, err
		}
	} else {
		info, err = m.getModelInstanceAndProviderByName(providerName, instanceName, modelName, userID, apiConfig)
		if err != nil || info == nil {
			return common.CodeNotFound, err
		}
	}

	if modelConfig == nil {
		modelConfig = &modelModule.ChatConfig{}
	}
	modelConfig.ModelClass = info.ModelInfo.Class
	if modelConfig.Thinking == nil && info.ModelInfo.Thinking != nil {
		thinking := info.ModelInfo.Thinking.DefaultValue
		modelConfig.Thinking = &thinking
	}
	resolvedModelName := info.ModelInfo.Name
	if info.ModelEntity != nil && info.ModelEntity.ModelName != "" {
		resolvedModelName = info.ModelEntity.ModelName
	}
	resolvedProviderName := info.ProviderEntity.ProviderName

	var modelDriver modelModule.ModelDriver

	if info.ModelEntity == nil {
		modelDriver = info.ProviderInfo.ModelDriver
	} else {
		// model entity exists
		if info.ModelEntity.Status == "active" {
			if !entity.ModelType(info.ModelEntity.ModelType).Has(entity.ModelTypeChat) && !entity.ModelType(info.ModelEntity.ModelType).Has(entity.ModelTypeImage2Text) {
				return common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is a chat or multimodal model", resolvedModelName, resolvedProviderName))
			}

			modelDriver, err = newModelDriverForBaseURL(info.ProviderInfo.ModelDriver, resolvedProviderName, *info.APIConfig.Region, *info.APIConfig.BaseURL)
			if err != nil {
				return common.CodeServerError, err
			}
		} else {
			return common.CodeServerError, errors.New("model is inactive")
		}
	}

	err = modelDriver.ChatStreamlyWithSender(resolvedModelName, messages, info.APIConfig, modelConfig, sender)
	if err != nil {
		return common.CodeServerError, err
	}
	return common.CodeSuccess, nil
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
func (m *ModelProviderService) EmbedText(providerName, instanceName, modelName, modelID *string, userID string, texts []string, apiConfig *modelModule.APIConfig, modelConfig *modelModule.EmbeddingConfig) ([]modelModule.EmbeddingData, common.ErrorCode, error) {

	var err error
	var info *ModelInstanceAndProviderInfo

	if modelID != nil {
		info, err = m.getModelInstanceAndProviderByID(modelID, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	} else {
		info, err = m.getModelInstanceAndProviderByName(providerName, instanceName, modelName, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	}

	if modelConfig == nil {
		modelConfig = &modelModule.EmbeddingConfig{}
	}
	resolvedModelName := info.ModelInfo.Name
	if info.ModelEntity != nil && info.ModelEntity.ModelName != "" {
		resolvedModelName = info.ModelEntity.ModelName
	}
	resolvedProviderName := info.ProviderEntity.ProviderName

	var modelDriver modelModule.ModelDriver

	if info.ModelEntity == nil {
		if !info.ModelInfo.ModelTypeMap["embedding"] {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is an embedding model", resolvedModelName, resolvedProviderName))
		}
		modelDriver = info.ProviderInfo.ModelDriver
	} else {
		// model entity exists
		if info.ModelEntity.Status == "active" {
			if !entity.ModelType(info.ModelEntity.ModelType).Has(entity.ModelTypeEmbedding) {
				return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is an embedding model", resolvedModelName, resolvedProviderName))
			}

			modelDriver, err = newModelDriverForBaseURL(info.ProviderInfo.ModelDriver, resolvedProviderName, *info.APIConfig.Region, *info.APIConfig.BaseURL)
			if err != nil {
				return nil, common.CodeServerError, err
			}
		} else {
			return nil, common.CodeServerError, errors.New("model is inactive")
		}
	}

	if err = validateEmbeddingDimension(info.ModelInfo, modelConfig.Dimension); err != nil {
		return nil, common.CodeBadRequest, err
	}

	var response []modelModule.EmbeddingData
	response, err = modelDriver.Embed(&resolvedModelName, texts, info.APIConfig, modelConfig)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if response == nil || len(response) == 0 {
		return nil, common.CodeServerError, errors.New("empty embed response")
	}

	return response, common.CodeSuccess, nil
}

// RerankDocument sends texts to the embedding model
func (m *ModelProviderService) RerankDocument(providerName, instanceName, modelName, modelID *string, userID, query string, documents []string, apiConfig *modelModule.APIConfig, modelConfig *modelModule.RerankConfig) (*modelModule.RerankResponse, common.ErrorCode, error) {

	var err error
	var info *ModelInstanceAndProviderInfo

	if modelID != nil {
		info, err = m.getModelInstanceAndProviderByID(modelID, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	} else {
		info, err = m.getModelInstanceAndProviderByName(providerName, instanceName, modelName, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	}

	if modelConfig == nil {
		modelConfig = &modelModule.RerankConfig{}
	}
	resolvedModelName := info.ModelInfo.Name
	if info.ModelEntity != nil && info.ModelEntity.ModelName != "" {
		resolvedModelName = info.ModelEntity.ModelName
	}
	resolvedProviderName := info.ProviderEntity.ProviderName

	var modelDriver modelModule.ModelDriver

	if info.ModelEntity == nil {
		if !info.ModelInfo.ModelTypeMap["rerank"] {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is a rerank model", resolvedModelName, resolvedProviderName))
		}
		modelDriver = info.ProviderInfo.ModelDriver
	} else {
		// model entity exists
		if info.ModelEntity.Status == "active" {
			if !entity.ModelType(info.ModelEntity.ModelType).Has(entity.ModelTypeRerank) {
				return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is a rerank model", resolvedModelName, resolvedProviderName))
			}

			modelDriver, err = newModelDriverForBaseURL(info.ProviderInfo.ModelDriver, resolvedProviderName, *info.APIConfig.Region, *info.APIConfig.BaseURL)
			if err != nil {
				return nil, common.CodeServerError, err
			}
		} else {
			return nil, common.CodeServerError, errors.New("model is inactive")
		}
	}

	var response *modelModule.RerankResponse
	response, err = modelDriver.Rerank(&resolvedModelName, query, documents, info.APIConfig, modelConfig)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	return response, common.CodeSuccess, nil
}

// TranscribeAudio transcribe audio file to text
func (m *ModelProviderService) TranscribeAudio(providerName, instanceName, modelName, modelID *string, userID string, audioFile *string, apiConfig *modelModule.APIConfig, modelConfig *modelModule.ASRConfig) (*modelModule.ASRResponse, common.ErrorCode, error) {

	var err error
	var info *ModelInstanceAndProviderInfo

	if modelID != nil {
		info, err = m.getModelInstanceAndProviderByID(modelID, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	} else {
		info, err = m.getModelInstanceAndProviderByName(providerName, instanceName, modelName, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	}

	if modelConfig == nil {
		modelConfig = &modelModule.ASRConfig{}
	}

	var modelDriver modelModule.ModelDriver

	if info.ModelEntity == nil {
		if !info.ModelInfo.ModelTypeMap["asr"] {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is an ASR model", *modelName, *providerName))
		}
		modelDriver = info.ProviderInfo.ModelDriver
	} else {
		// model entity exists
		if info.ModelEntity.Status == "active" {
			if !entity.ModelType(info.ModelEntity.ModelType).Has(entity.ModelTypeSpeech2Text) {
				return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is an ASR model", *modelName, *providerName))
			}

			modelDriver, err = newModelDriverForBaseURL(info.ProviderInfo.ModelDriver, *providerName, *info.APIConfig.Region, *info.APIConfig.BaseURL)
			if err != nil {
				return nil, common.CodeServerError, err
			}
		} else {
			return nil, common.CodeServerError, errors.New("model is inactive")
		}
	}

	var response *modelModule.ASRResponse
	response, err = modelDriver.TranscribeAudio(modelName, audioFile, apiConfig, modelConfig)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if response == nil {
		return nil, common.CodeServerError, errors.New("empty chat response")
	}

	return response, common.CodeSuccess, nil
}

// TranscribeAudioStream transcribe audio file to text stream directly via sender function ( the best performance, no channel)
func (m *ModelProviderService) TranscribeAudioStream(providerName, instanceName, modelName, modelID *string, userID string, audioFile *string, apiConfig *modelModule.APIConfig, modelConfig *modelModule.ASRConfig, sender func(*string, *string) error) (common.ErrorCode, error) {

	var err error
	var info *ModelInstanceAndProviderInfo

	if modelID != nil {
		info, err = m.getModelInstanceAndProviderByID(modelID, userID, apiConfig)
		if err != nil || info == nil {
			return common.CodeNotFound, err
		}
	} else {
		info, err = m.getModelInstanceAndProviderByName(providerName, instanceName, modelName, userID, apiConfig)
		if err != nil || info == nil {
			return common.CodeNotFound, err
		}
	}

	if modelConfig == nil {
		modelConfig = &modelModule.ASRConfig{}
	}

	var modelDriver modelModule.ModelDriver

	if info.ModelEntity == nil {
		if !info.ModelInfo.ModelTypeMap["asr"] {
			return common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is an ASR model", *modelName, *providerName))
		}
		modelDriver = info.ProviderInfo.ModelDriver
	} else {
		// model entity exists
		if info.ModelEntity.Status == "active" {
			if !entity.ModelType(info.ModelEntity.ModelType).Has(entity.ModelTypeSpeech2Text) {
				return common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is an ASR model", *modelName, *providerName))
			}

			modelDriver, err = newModelDriverForBaseURL(info.ProviderInfo.ModelDriver, *providerName, *info.APIConfig.Region, *info.APIConfig.BaseURL)
			if err != nil {
				return common.CodeServerError, err
			}
		} else {
			return common.CodeServerError, errors.New("model is inactive")
		}
	}

	err = modelDriver.TranscribeAudioWithSender(modelName, audioFile, apiConfig, modelConfig, sender)
	if err != nil {
		return common.CodeServerError, err
	}

	return common.CodeSuccess, nil
}

// AudioSpeech convert audio to speech
func (m *ModelProviderService) AudioSpeech(providerName, instanceName, modelName, modelID *string, userID string, audioContent *string, apiConfig *modelModule.APIConfig, modelConfig *modelModule.TTSConfig) (*modelModule.TTSResponse, common.ErrorCode, error) {

	var err error
	var info *ModelInstanceAndProviderInfo

	if modelID != nil {
		info, err = m.getModelInstanceAndProviderByID(modelID, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	} else {
		info, err = m.getModelInstanceAndProviderByName(providerName, instanceName, modelName, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	}

	if modelConfig == nil {
		modelConfig = &modelModule.TTSConfig{}
	}

	var modelDriver modelModule.ModelDriver

	if info.ModelEntity == nil {
		if !info.ModelInfo.ModelTypeMap["tts"] {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is a TTS model", *modelName, *providerName))
		}
		modelDriver = info.ProviderInfo.ModelDriver
	} else {
		// model entity exists
		if info.ModelEntity.Status == "active" {
			if !entity.ModelType(info.ModelEntity.ModelType).Has(entity.ModelTypeTTS) {
				return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is a TTS model", *modelName, *providerName))
			}

			modelDriver, err = newModelDriverForBaseURL(info.ProviderInfo.ModelDriver, *providerName, *info.APIConfig.Region, *info.APIConfig.BaseURL)
			if err != nil {
				return nil, common.CodeServerError, err
			}
		} else {
			return nil, common.CodeServerError, errors.New("model is inactive")
		}
	}

	var response *modelModule.TTSResponse
	response, err = modelDriver.AudioSpeech(modelName, audioContent, apiConfig, modelConfig)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if response == nil {
		return nil, common.CodeServerError, errors.New("empty chat response")
	}

	return response, common.CodeSuccess, nil
}

func (m *ModelProviderService) AudioSpeechStream(providerName, instanceName, modelName, modelID *string, userID string, audioContent *string, apiConfig *modelModule.APIConfig, modelConfig *modelModule.TTSConfig, sender func(*string, *string) error) (common.ErrorCode, error) {

	var err error
	var info *ModelInstanceAndProviderInfo

	if modelID != nil {
		info, err = m.getModelInstanceAndProviderByID(modelID, userID, apiConfig)
		if err != nil || info == nil {
			return common.CodeNotFound, err
		}
	} else {
		info, err = m.getModelInstanceAndProviderByName(providerName, instanceName, modelName, userID, apiConfig)
		if err != nil || info == nil {
			return common.CodeNotFound, err
		}
	}

	if modelConfig == nil {
		modelConfig = &modelModule.TTSConfig{}
	}

	var modelDriver modelModule.ModelDriver

	if info.ModelEntity == nil {
		if !info.ModelInfo.ModelTypeMap["tts"] {
			return common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is a TTS model", *modelName, *providerName))
		}
		modelDriver = info.ProviderInfo.ModelDriver
	} else {
		// model entity exists
		if info.ModelEntity.Status == "active" {
			if !entity.ModelType(info.ModelEntity.ModelType).Has(entity.ModelTypeTTS) {
				return common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is a TTS model", *modelName, *providerName))
			}

			modelDriver, err = newModelDriverForBaseURL(info.ProviderInfo.ModelDriver, *providerName, *info.APIConfig.Region, *info.APIConfig.BaseURL)
			if err != nil {
				return common.CodeServerError, err
			}
		} else {
			return common.CodeServerError, errors.New("model is inactive")
		}
	}

	err = modelDriver.AudioSpeechWithSender(modelName, audioContent, apiConfig, modelConfig, sender)
	if err != nil {
		return common.CodeServerError, err
	}

	return common.CodeSuccess, nil
}

func (m *ModelProviderService) OCRFile(providerName, instanceName, modelName, modelID *string, userID string, content []byte, url *string, apiConfig *modelModule.APIConfig, modelConfig *modelModule.OCRConfig) (*modelModule.OCRFileResponse, common.ErrorCode, error) {

	var err error
	var info *ModelInstanceAndProviderInfo

	if modelID != nil {
		info, err = m.getModelInstanceAndProviderByID(modelID, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	} else {
		info, err = m.getModelInstanceAndProviderByName(providerName, instanceName, modelName, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	}

	if modelConfig == nil {
		modelConfig = &modelModule.OCRConfig{}
	}

	var modelDriver modelModule.ModelDriver

	if info.ModelEntity == nil {
		if !info.ModelInfo.ModelTypeMap["ocr"] {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is an OCR model", *modelName, *providerName))
		}
		modelDriver = info.ProviderInfo.ModelDriver
	} else {
		// model entity exists
		if info.ModelEntity.Status == "active" {
			if !entity.ModelType(info.ModelEntity.ModelType).Has(entity.ModelTypeOCR) {
				return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is an OCR model", *modelName, *providerName))
			}

			modelDriver, err = newModelDriverForBaseURL(info.ProviderInfo.ModelDriver, *providerName, *info.APIConfig.Region, *info.APIConfig.BaseURL)
			if err != nil {
				return nil, common.CodeServerError, err
			}
		} else {
			return nil, common.CodeServerError, errors.New("model is inactive")
		}
	}

	var response *modelModule.OCRFileResponse
	response, err = modelDriver.OCRFile(modelName, content, url, apiConfig, modelConfig)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if response == nil {
		return nil, common.CodeServerError, errors.New("empty chat response")
	}

	return response, common.CodeSuccess, nil
}

func (m *ModelProviderService) ParseFile(providerName, instanceName, modelName, modelID *string, userID string, content []byte, url *string, apiConfig *modelModule.APIConfig, modelConfig *modelModule.ParseFileConfig) (*modelModule.ParseFileResponse, common.ErrorCode, error) {

	var err error
	var info *ModelInstanceAndProviderInfo

	if modelID != nil {
		info, err = m.getModelInstanceAndProviderByID(modelID, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	} else {
		info, err = m.getModelInstanceAndProviderByName(providerName, instanceName, modelName, userID, apiConfig)
		if err != nil || info == nil {
			return nil, common.CodeNotFound, err
		}
	}

	if modelConfig == nil {
		modelConfig = &modelModule.ParseFileConfig{}
	}

	var modelDriver modelModule.ModelDriver

	if info.ModelEntity == nil {
		if !info.ModelInfo.ModelTypeMap["doc_parse"] {
			return nil, common.CodeNotFound, errors.New(fmt.Sprintf("expect model %s@%s is a ParseFile model", *modelName, *providerName))
		}
		modelDriver = info.ProviderInfo.ModelDriver
	} else {
		// model entity exists
		if info.ModelEntity.Status == "active" {
			// Note: ParseFile model type is not in the ModelType enum; skip entity-type check
			// and rely on the factory ModelTypeMap check in the nil-entity branch.

			modelDriver, err = newModelDriverForBaseURL(info.ProviderInfo.ModelDriver, *providerName, *info.APIConfig.Region, *info.APIConfig.BaseURL)
			if err != nil {
				return nil, common.CodeServerError, err
			}
		} else {
			return nil, common.CodeServerError, errors.New("model is inactive")
		}
	}

	var response *modelModule.ParseFileResponse
	response, err = modelDriver.ParseFile(modelName, content, url, apiConfig, modelConfig)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if response == nil {
		return nil, common.CodeServerError, errors.New("empty chat response")
	}

	return response, common.CodeSuccess, nil
}

// GetEmbeddingModel returns an EmbeddingModel wrapper for the given tenant
func (m *ModelProviderService) GetEmbeddingModel(tenantID, compositeModelName string) (*modelModule.EmbeddingModel, error) {
	driver, modelName, apiConfig, maxTokens, err := m.ResolveModelConfig(tenantID, entity.ModelTypeEmbedding, compositeModelName)
	if err != nil {
		return nil, err
	}
	return modelModule.NewEmbeddingModel(driver, &modelName, apiConfig, maxTokens), nil
}

// GetChatModel  returns a ChatModel wrapper for the given tenant
func (m *ModelProviderService) GetChatModel(tenantID, compositeModelName string) (*modelModule.ChatModel, error) {
	driver, modelName, apiConfig, _, err := m.ResolveModelConfig(tenantID, entity.ModelTypeChat, compositeModelName)
	if err != nil {
		return nil, err
	}
	return modelModule.NewChatModel(driver, &modelName, apiConfig), nil
}

// GetRerankModel returns a RerankModel wrapper for the given tenant
func (m *ModelProviderService) GetRerankModel(tenantID, compositeModelName string) (*modelModule.RerankModel, error) {
	driver, modelName, apiConfig, _, err := m.ResolveModelConfig(tenantID, entity.ModelTypeRerank, compositeModelName)
	if err != nil {
		return nil, err
	}
	return modelModule.NewRerankModel(driver, &modelName, apiConfig), nil
}

type AddModelRequest struct {
	ProviderName string                 `json:"provider_name"`
	InstanceName string                 `json:"instance_name"`
	ModelName    string                 `json:"model_name"`
	ModelTypes   []string               `json:"model_type"`
	MaxTokens    int                    `json:"max_tokens"`
	Extra        map[string]interface{} `json:"extra"`
}

func (m *ModelProviderService) GetTenantDefaultModelByType(tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	if modelType == entity.ModelTypeOCR {
		return nil, "", nil, 0, fmt.Errorf("OCR model name is required")
	}

	tenant, err := m.tenantDAO.GetByID(tenantID)
	if err != nil {
		return nil, "", nil, 0, fmt.Errorf("failed to get tenant: %s type %s: %w", tenantID, modelType, err)
	}
	modelName, modelID := defaultModelRefs(tenant, modelType)
	if modelID != "" {
		driver, resolvedName, apiConfig, maxTokens, idErr := m.GetModelConfigByID(tenantID, modelType, modelID)
		if idErr == nil {
			return driver, resolvedName, apiConfig, maxTokens, nil
		}
		common.Warn("GetTenantDefaultModelByType: model_id lookup failed, falling back to model name",
			zap.String("tenantID", tenantID),
			zap.String("modelID", modelID),
			zap.Error(idErr))
	}
	if modelName == "" {
		return nil, "", nil, 0, fmt.Errorf("no default %s model is set", modelType)
	}

	return m.ResolveModelConfig(tenantID, modelType, modelName)
}

// GetModelConfigByID returns model driver and API config for a tenant_model row by its ID.
func (m *ModelProviderService) GetModelConfigByID(userID string, modelType entity.ModelType, modelID string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	common.Debug("GetModelConfigByID",
		zap.String("userID", userID),
		zap.String("modelType", modelType.String()),
		zap.String("modelID", modelID))

	modelEntity, err := m.modelDAO.GetByID(modelID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", nil, 0, fmt.Errorf("tenant model id=%s not found", modelID)
		}
		return nil, "", nil, 0, err
	}
	if modelEntity.Status != "active" {
		return nil, "", nil, 0, fmt.Errorf("tenant model id=%s is disabled", modelID)
	}
	if modelType != 0 && !entity.ModelType(modelEntity.ModelType).Has(modelType) {
		return nil, "", nil, 0, fmt.Errorf("tenant model id=%s cannot be used as %s model", modelID, modelType.String())
	}

	providerEntity, err := m.modelProviderDAO.GetByID(modelEntity.ProviderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", nil, 0, fmt.Errorf("provider id=%s not found for model id=%s", modelEntity.ProviderID, modelID)
		}
		return nil, "", nil, 0, err
	}
	if providerEntity == nil {
		return nil, "", nil, 0, fmt.Errorf("provider id=%s not found for model id=%s", modelEntity.ProviderID, modelID)
	}

	if providerEntity.TenantID != userID {
		userTenants, terr := NewUserTenantService().GetUserTenantRelationByUserID(userID)
		if terr != nil {
			return nil, "", nil, 0, terr
		}
		allowed := false
		for _, rel := range userTenants {
			if rel != nil && rel.TenantID == providerEntity.TenantID {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, "", nil, 0, fmt.Errorf("tenant %s has no access to provider owned by tenant %s", userID, providerEntity.TenantID)
		}
	}

	instanceEntity, err := m.modelInstanceDAO.GetByID(modelEntity.InstanceID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", nil, 0, fmt.Errorf("instance id=%s not found for model id=%s", modelEntity.InstanceID, modelID)
		}
		return nil, "", nil, 0, err
	}

	apiKey := instanceEntity.APIKey
	var extra map[string]string
	if err := json.Unmarshal([]byte(instanceEntity.Extra), &extra); err != nil {
		return nil, "", nil, 0, err
	}
	region := extra["region"]
	baseURL := extra["base_url"]

	providerInfo := dao.GetModelProviderManager().FindProvider(providerEntity.ProviderName)
	if providerInfo == nil {
		return nil, "", nil, 0, fmt.Errorf("provider %q driver not found", providerEntity.ProviderName)
	}
	modelDriver, err := newModelDriverForBaseURL(providerInfo.ModelDriver, providerEntity.ProviderName, region, baseURL)
	if err != nil {
		return nil, "", nil, 0, err
	}

	maxTokens := 0
	if mi, _ := dao.GetModelProviderManager().GetModelByName(providerEntity.ProviderName, modelEntity.ModelName); mi != nil {
		if mi.MaxTokens != nil {
			maxTokens = *mi.MaxTokens
		}
	}
	maxTokens, err = maxTokensFromTenantModelExtra(modelEntity, maxTokens)
	if err != nil {
		return nil, "", nil, 0, err
	}

	apiConfig := &modelModule.APIConfig{ApiKey: &apiKey, Region: &region, BaseURL: &baseURL}
	return modelDriver, modelEntity.ModelName, apiConfig, maxTokens, nil
}

func defaultModelRefs(tenant *entity.Tenant, modelType entity.ModelType) (string, string) {
	if tenant == nil {
		return "", ""
	}
	switch modelType {
	case entity.ModelTypeChat:
		return tenant.LLMID, ptrStringValue(tenant.TenantLLMID)
	case entity.ModelTypeEmbedding:
		return tenant.EmbdID, ptrStringValue(tenant.TenantEmbdID)
	case entity.ModelTypeRerank:
		return tenant.RerankID, ptrStringValue(tenant.TenantRerankID)
	case entity.ModelTypeSpeech2Text:
		return tenant.ASRID, ptrStringValue(tenant.TenantASRID)
	case entity.ModelTypeImage2Text:
		return tenant.Img2TxtID, ptrStringValue(tenant.TenantImg2TxtID)
	case entity.ModelTypeTTS:
		return *tenant.TTSID, ptrStringValue(tenant.TenantTTSID)
	case entity.ModelTypeOCR:
		return *tenant.OCRID, ptrStringValue(tenant.TenantOCRID)
	default:
		return "", ""
	}
}

func (m *ModelProviderService) ResolveModelConfig(tenantID string, modelType entity.ModelType, modelRef string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	if strings.TrimSpace(modelRef) == "" {
		return nil, "", nil, 0, fmt.Errorf("model ref is required")
	}
	if _, err := m.modelDAO.GetByID(modelRef); err == nil {
		return m.GetModelConfigByID(tenantID, modelType, modelRef)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, "", nil, 0, err
	}
	return m.GetModelConfigFromProviderInstance(tenantID, modelType, modelRef)
}

func (m *ModelProviderService) ResolveModelID(tenantID string, modelType entity.ModelType, modelName string) (string, error) {
	if modelObj, err := m.modelDAO.GetByID(modelName); err == nil {
		if modelObj.Status != "active" {
			return "", fmt.Errorf("tenant model id=%s is disabled", modelName)
		}
		if !entity.ModelType(modelObj.ModelType).Has(modelType) {
			return "", fmt.Errorf("tenant model id=%s cannot be used as %s model", modelName, modelType.String())
		}
		if _, _, _, _, err := m.GetModelConfigByID(tenantID, modelType, modelName); err != nil {
			return "", err
		}
		return modelObj.ID, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}

	pureModelName, instanceName, providerName, err := parseModelName(modelName)
	if err != nil {
		return "", err
	}

	// Builtin provider is a local service (TEI), not a tenant-enrolled
	// provider. There is no row in tenant_model_provider for it, so skip
	// database lookups. Mirrors GetModelConfigFromProviderInstance's
	// Builtin short-circuit and Python's resolve_model_id which returns
	// None for Builtin TEI embeddings.
	if modelType == entity.ModelTypeEmbedding && providerName == "Builtin" {
		return "", nil
	}

	provider, err := m.modelProviderDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return "", fmt.Errorf("provider %q lookup failed: %w", providerName, err)
	}
	if provider == nil {
		return "", fmt.Errorf("provider %q not found for model %q", providerName, modelName)
	}

	instance, err := m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return "", fmt.Errorf("instance %q lookup failed: %w", instanceName, err)
	}
	if instance == nil {
		return "", fmt.Errorf("instance %q not found for model %q", instanceName, modelName)
	}

	modelObj, err := m.modelDAO.GetByProviderIDAndInstanceIDAndModelTypeAndModelName(provider.ID, instance.ID, int(modelType), pureModelName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", fmt.Errorf("model %q not found for model type %s", modelName, modelType.String())
		}
		return "", err
	}
	if modelObj.Status != "active" {
		return "", fmt.Errorf("model %q is disabled", modelName)
	}
	return modelObj.ID, nil
}

func (m *ModelProviderService) ResolveModelType(tenantID, modelRef string) ([]entity.ModelType, error) {
	modelObj, err := m.modelDAO.GetByID(modelRef)
	if err == nil {
		if modelObj.Status != "active" {
			return nil, fmt.Errorf("tenant model id=%s is disabled", modelRef)
		}
		return []entity.ModelType{entity.ModelType(modelObj.ModelType)}, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return m.GetModelTypeByName(tenantID, modelRef)
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
	// conf/models/*.json files now use the same names as the Python's
	// conf/llm_factories.json (e.g. "SILICONFLOW"); but a strict
	// `==` here would still fail for any case mismatch. The rest of the Go codebase
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
			return []entity.ModelType{entity.ModelTypeFromString(targetProvider.Models[i].ModelTypes[0])}, nil
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
	ModelName    string   `json:"model_name"`
	ModelTypes   []string `json:"model_types"`
	MaxTokens    int      `json:"max_tokens"`
	MaxDimension int      `json:"max_dimension"`
	Dimensions   []int    `json:"dimensions"`
	Thinking     *bool    `json:"thinking"`
}

func (m *ModelProviderService) AddModel(request *AddModelRequest, userID string) (common.ErrorCode, error) {
	if request == nil {
		return common.CodeBadRequest, errors.New("request is required")
	}

	modelName := strings.TrimSpace(request.ModelName)
	if modelName == "" {
		return common.CodeBadRequest, errors.New("model_name is required")
	}

	if len(request.ModelTypes) == 0 {
		return common.CodeBadRequest, errors.New("model_type is required")
	}

	tenants, err := m.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return common.CodeServerError, err
	}
	if len(tenants) == 0 {
		return common.CodeNotFound, errors.New("user has no tenants")
	}

	tenantID := tenants[0].TenantID

	// Get provider by ID or name (matches Python's get_by_tenant_id_and_provider_id → get_by_tenant_id_and_provider_name fallback).
	provider, err := m.getProviderByIDOrName(tenantID, request.ProviderName)
	if err != nil {
		return common.CodeDataError, fmt.Errorf("No provider found for provider '%s'", request.ProviderName)
	}

	// Get instance by ID or name (matches Python's get_by_id → get_by_provider_id_and_instance_name fallback).
	instance, err := m.modelInstanceDAO.GetByID(request.InstanceName)
	if err != nil || instance.ProviderID != provider.ID {
		instance, err = m.modelInstanceDAO.GetByProviderIDAndInstanceName(provider.ID, request.InstanceName)
	}
	if err != nil {
		return common.CodeDataError, fmt.Errorf("No instance found for provider '%s' and instance '%s'", request.ProviderName, request.InstanceName)
	}

	// Check for duplicate model.
	_, err = m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(provider.ID, instance.ID, modelName)
	if err == nil {
		return common.CodeConflict, fmt.Errorf("Model '%s' already exists for provider '%s' and instance '%s'", modelName, request.ProviderName, request.InstanceName)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return common.CodeServerError, err
	}

	// Compute model type bitmask.
	combinedType := entity.ModelType(0)
	for _, rawType := range request.ModelTypes {
		mt := strings.TrimSpace(rawType)
		if mt == "" {
			continue
		}
		t := entity.ModelTypeFromString(mt)
		if t == 0 {
			return common.CodeBadRequest, fmt.Errorf("invalid model type: %s", mt)
		}
		combinedType |= t
	}

	maxTokens := request.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 8192
	}

	// Build extra fields. Mirrors Python's add_model_to_instance.
	extraFields := map[string]interface{}{
		"max_tokens": maxTokens,
	}

	// Look up factory info to populate is_tools and thinking (matches Python).
	targetProvider := dao.GetModelProviderManager().FindProvider(provider.ProviderName)
	if targetProvider != nil {
		targetModel := dao.GetModelProviderManager().FindModel(targetProvider, modelName)
		if targetModel != nil {
			if targetModel.Tools != nil && targetModel.Tools.Support {
				extraFields["is_tools"] = true
			}
			if targetModel.Thinking != nil {
				extraFields["thinking"] = true
			}
		}
	}

	if request.Extra != nil {
		for k, v := range request.Extra {
			extraFields[k] = v
		}
	}

	extraBytes, err := json.Marshal(extraFields)
	if err != nil {
		return common.CodeServerError, errors.New("fail to marshal extra")
	}

	modelID := utility.GenerateToken()
	tenantModel := &entity.TenantModel{
		ID:         modelID,
		ModelName:  modelName,
		ModelType:  int(combinedType),
		ProviderID: provider.ID,
		InstanceID: instance.ID,
		Status:     "active",
		Extra:      string(extraBytes),
	}

	if err := m.modelDAO.Create(tenantModel); err != nil {
		return common.CodeServerError, fmt.Errorf("fail to create model '%s': %s", modelName, err.Error())
	}

	return common.CodeSuccess, nil
}

// GetModelConfigFromProviderInstance get model config from provider instance
// modelName@instance@provider or
// "model@provider" — the provider is required and is looked up directly via
// tenant_model_provider. For 2-part names the instance defaults to "default".
// If the model is enrolled in tenant_model, that row is used (and INACTIVE rows
// raise). Otherwise, the factory's LLM catalog is consulted, with
// region=intl + siliconflow redirected to the siliconflow_intl factory.
func (m *ModelProviderService) GetModelConfigFromProviderInstance(tenantID string, modelType entity.ModelType, modelName string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	common.Debug("GetModelConfigFromProviderInstance",
		zap.String("tenantID", tenantID),
		zap.String("modelName", modelName),
		zap.String("modelType", modelType.String()))

	// TEI builtin embedding short-circuit
	if modelType == entity.ModelTypeEmbedding && strings.Contains(common.GetEnv(common.EnvComposeProfiles), "tei-") {
		teiModel := common.GetEnv(common.EnvTEIModel)
		teiBaseURL := common.GetEnv(common.EnvTEIBaseURL)

		// First try exact match: handles bare model IDs like "model@q8_0"
		// where '@' is part of the model name itself.
		if modelName == teiModel {
			builtinDriver := modelModule.GetBuiltinEmbeddingModel(modelName)
			if builtinDriver == nil {
				return nil, "", nil, 0, fmt.Errorf("builtin (TEI) embedding model %q not found", modelName)
			}
			apiConfig := &modelModule.APIConfig{ApiKey: nil, Region: nil, BaseURL: &teiBaseURL}
			return builtinDriver, modelName, apiConfig, 0, nil
		}

		// Then try right-anchored parsing for explicit "model@Builtin" or
		// "model@instance@Builtin" composite keys.
		teiPure, _, teiProvider := splitRightAnchoredModelName(modelName)
		if teiPure == teiModel && (teiProvider == "Builtin" || teiProvider == "") {
			builtinDriver := modelModule.GetBuiltinEmbeddingModel(teiPure)
			if builtinDriver == nil {
				return nil, "", nil, 0, fmt.Errorf("builtin (TEI) embedding model %q not found", teiPure)
			}
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
	if modelType == entity.ModelTypeEmbedding {
		// The Builtin provider is a local service, not a tenant-enrolled
		// provider. Extract the model name by looking for the @Builtin
		// suffix explicitly so model names with embedded '@' characters
		// (e.g. `model@q8_0@Builtin` or bare `model@q8_0` without any
		// provider suffix) are handled correctly.
		var pureModelName string
		switch {
		case strings.HasSuffix(modelName, "@default@Builtin"):
			pureModelName = modelName[:len(modelName)-len("@default@Builtin")]
		case strings.HasSuffix(modelName, "@Builtin"):
			pureModelName = modelName[:len(modelName)-len("@Builtin")]
		default:
			pureModelName = ""
		}
		if pureModelName != "" {
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
		provider.ID, instance.ID, int(modelType), pureModelName,
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
		maxTokens, driverErr = maxTokensFromTenantModelExtra(modelObj, maxTokens)
		if driverErr != nil {
			return nil, "", nil, 0, driverErr
		}
		apiConfig := &modelModule.APIConfig{ApiKey: &apiKey, Region: &region, BaseURL: &baseURL}
		return driver, modelObj.ModelName, apiConfig, maxTokens, nil
	case errors.Is(modelErr, gorm.ErrRecordNotFound):
		// Tenant hasn't enrolled this model. Fall through to the factory catalog.
		common.Debug("GetModelConfigFromProviderInstance: tenant has no row for model, falling back to factory catalog",
			zap.String("tenantID", tenantID),
			zap.String("providerName", providerName),
			zap.String("instanceName", instanceName),
			zap.String("modelType", modelType.String()),
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
	// conf/models/*.json files now use the same names as the Python's
	// conf/llm_factories.json (e.g. "SILICONFLOW"); but a strict
	// `==` here would still fail for any case mismatch even though the Python passes
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
	var baseURL string
	if instance != nil {
		err = json.Unmarshal([]byte(instance.Extra), &extra)
		if err != nil {
			return nil, "", nil, 0, err
		}
		region = extra["region"]
		baseURL = extra["base_url"]
	}

	providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
	if providerInfo == nil {
		if providerName != "Builtin" {
			return nil, "", nil, 0, fmt.Errorf("model provider config not found: %s", providerName)
		}
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
	}

	var modelRecord *entity.TenantModel
	modelRecord, err = m.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(providerID, instance.ID, modelName)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", nil, 0, fmt.Errorf("tenant model %q lookup failed: %w", modelName, err)
		}
		_, err = dao.GetModelProviderManager().GetModelByName(providerName, modelName)
		if err != nil {
			return nil, "", nil, 0, fmt.Errorf("provider %s model %s not found", providerName, modelName)
		}
	}
	maxTokens, err = maxTokensFromTenantModelExtra(modelRecord, maxTokens)
	if err != nil {
		return nil, "", nil, 0, err
	}
	apiKey = instance.APIKey

	driver, err := newModelDriverForBaseURL(providerInfo.ModelDriver, providerName, region, baseURL)
	if err != nil {
		return nil, "", nil, 0, err
	}

	apiConfig := &modelModule.APIConfig{ApiKey: &apiKey, Region: &region, BaseURL: &baseURL}
	return driver, modelName, apiConfig, maxTokens, nil
}

// ListAllModels list all models
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

// isImage2TextLLM returns true when the named LLM is registered as an
// image2text model for the tenant.
// Returns false on lookup error or empty LLM ID so callers fall back to
// chat — matches Python's branch order where only an EXPLICIT image2text
// registration switches the model type away from chat.
func (m *ModelProviderService) isImage2TextLLM(tenantID, llmID string) bool {
	if m == nil || llmID == "" {
		return false
	}
	modelTypes, err := m.ResolveModelType(tenantID, llmID)
	if err != nil {
		return false
	}
	for _, mt := range modelTypes {
		if mt == entity.ModelTypeImage2Text {
			return true
		}
	}
	return false
}

// GetChatModelConfig resolves the model configuration for a chat dialog.
// If llmID is empty, falls back to the tenant's default chat model.
// When the named LLM is registered as an image2text model, returns the
// IMAGE2TEXT driver/config instead of CHAT.
func (m *ModelProviderService) GetChatModelConfig(tenantID string, llmID string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	if llmID == "" {
		return m.GetTenantDefaultModelByType(tenantID, entity.ModelTypeChat)
	}
	modelType := entity.ModelTypeChat
	if m.isImage2TextLLM(tenantID, llmID) {
		modelType = entity.ModelTypeImage2Text
	}
	return m.ResolveModelConfig(tenantID, modelType, llmID)
}
