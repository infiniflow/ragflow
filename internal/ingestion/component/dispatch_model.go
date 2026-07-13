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

// Tenant model resolution helpers shared by pdf_vision, media_dispatch,
// docx_vision, and markdown_vision dispatchers.

package component

import (
	"encoding/json"
	"fmt"
	"strings"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"

	"gorm.io/gorm"
)

type tenantModelExtra struct {
	MaxTokens *int `json:"max_tokens"`
}

var resolveTenantModelByType = defaultResolveTenantModelByType

func defaultResolveTenantModelByType(tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	tenantDAO := dao.NewTenantDAO()
	tenant, err := tenantDAO.GetByID(tenantID)
	if err != nil {
		return nil, "", nil, 0, err
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
		modelID = tenant.TTSID
	case entity.ModelTypeOCR:
		modelID = tenant.OCRID
	default:
		return nil, "", nil, 0, fmt.Errorf("invalid model type: %s", modelType)
	}
	if modelID == "" {
		return nil, "", nil, 0, fmt.Errorf("no default %s model is set", modelType)
	}
	if tenantModelID := tenantModelIDByType(tenant, modelType); tenantModelID != "" {
		driver, modelName, apiConfig, maxTokens, err := resolveModelConfigByID(tenantID, modelType, tenantModelID)
		if err == nil {
			return driver, modelName, apiConfig, maxTokens, nil
		}
	}
	return resolveModelConfig(tenantID, modelType, modelID)
}

func tenantModelIDByType(tenant *entity.Tenant, modelType entity.ModelType) string {
	if tenant == nil {
		return ""
	}
	switch modelType {
	case entity.ModelTypeChat:
		return stringValue(tenant.TenantLLMID)
	case entity.ModelTypeEmbedding:
		return stringValue(tenant.TenantEmbdID)
	case entity.ModelTypeRerank:
		return stringValue(tenant.TenantRerankID)
	case entity.ModelTypeSpeech2Text:
		return stringValue(tenant.TenantASRID)
	case entity.ModelTypeImage2Text:
		return stringValue(tenant.TenantImg2TxtID)
	case entity.ModelTypeTTS:
		return stringValue(tenant.TenantTTSID)
	case entity.ModelTypeOCR:
		return stringValue(tenant.TenantOCRID)
	default:
		return ""
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func resolveModelConfig(tenantID string, modelType entity.ModelType, modelRef string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	modelDAO := dao.NewTenantModelDAO()
	if _, err := modelDAO.GetByID(modelRef); err == nil {
		return resolveModelConfigByID(tenantID, modelType, modelRef)
	} else if !errorsIsRecordNotFound(err) {
		return nil, "", nil, 0, err
	}
	return resolveModelConfigFromProviderInstance(tenantID, modelType, modelRef)
}

func resolveModelConfigByID(tenantID string, modelType entity.ModelType, modelID string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	modelDAO := dao.NewTenantModelDAO()
	instanceDAO := dao.NewTenantModelInstanceDAO()
	providerDAO := dao.NewTenantModelProviderDAO()

	modelObj, err := modelDAO.GetByID(modelID)
	if err != nil {
		return nil, "", nil, 0, err
	}
	if modelObj.Status != "active" {
		return nil, "", nil, 0, fmt.Errorf("model %q is disabled", modelID)
	}
	if !entity.ModelType(modelObj.ModelType).Has(modelType) {
		return nil, "", nil, 0, fmt.Errorf("model %q cannot be used as %s model", modelID, modelType.String())
	}
	instance, err := instanceDAO.GetByID(modelObj.InstanceID)
	if err != nil {
		return nil, "", nil, 0, err
	}
	provider, err := providerDAO.GetByID(modelObj.ProviderID)
	if err != nil {
		return nil, "", nil, 0, err
	}
	if provider.TenantID != tenantID {
		return nil, "", nil, 0, fmt.Errorf("tenant %s has no access to provider owned by tenant %s", tenantID, provider.TenantID)
	}

	apiKey := instance.APIKey
	var extra map[string]string
	_ = json.Unmarshal([]byte(instance.Extra), &extra)
	region := extra["region"]
	baseURL := extra["base_url"]

	providerInfo := dao.GetModelProviderManager().FindProvider(provider.ProviderName)
	if providerInfo == nil {
		return nil, "", nil, 0, fmt.Errorf("provider %q driver not found", provider.ProviderName)
	}
	driver, err := newModelDriverForBaseURLLocal(providerInfo.ModelDriver, provider.ProviderName, region, baseURL)
	if err != nil {
		return nil, "", nil, 0, err
	}
	maxTokens := 0
	if mi, _ := dao.GetModelProviderManager().GetModelByName(provider.ProviderName, modelObj.ModelName); mi != nil && mi.MaxTokens != nil {
		maxTokens = *mi.MaxTokens
	}
	if strings.TrimSpace(modelObj.Extra) != "" {
		var tenantExtra tenantModelExtra
		if err := json.Unmarshal([]byte(modelObj.Extra), &tenantExtra); err != nil {
			return nil, "", nil, 0, err
		}
		if tenantExtra.MaxTokens != nil && *tenantExtra.MaxTokens > 0 {
			maxTokens = *tenantExtra.MaxTokens
		}
	}
	apiConfig := &modelModule.APIConfig{ApiKey: &apiKey, Region: &region, BaseURL: &baseURL}
	return driver, modelObj.ModelName, apiConfig, maxTokens, nil
}

func resolveModelConfigFromProviderInstance(tenantID string, modelType entity.ModelType, modelName string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	pureModelName, instanceName, providerName, err := parseCompositeModelName(modelName)
	if err != nil {
		return nil, "", nil, 0, err
	}

	providerDAO := dao.NewTenantModelProviderDAO()
	instanceDAO := dao.NewTenantModelInstanceDAO()
	modelDAO := dao.NewTenantModelDAO()

	provider, err := providerDAO.GetByTenantIDAndProviderName(tenantID, providerName)
	if err != nil {
		return nil, "", nil, 0, fmt.Errorf("provider %q lookup failed: %w", providerName, err)
	}
	instance, err := instanceDAO.GetByProviderIDAndInstanceName(provider.ID, instanceName)
	if err != nil {
		return nil, "", nil, 0, fmt.Errorf("instance %q lookup failed: %w", instanceName, err)
	}

	apiKey := instance.APIKey
	var extra map[string]string
	_ = json.Unmarshal([]byte(instance.Extra), &extra)
	region := extra["region"]
	baseURL := extra["base_url"]

	modelObj, modelErr := modelDAO.GetByProviderIDAndInstanceIDAndModelTypeAndModelName(
		provider.ID, instance.ID, int(modelType), pureModelName,
	)
	switch {
	case modelErr == nil:
		if modelObj.Status == "inactive" {
			return nil, "", nil, 0, fmt.Errorf("model %q is disabled", modelName)
		}
		providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
		if providerInfo == nil {
			return nil, "", nil, 0, fmt.Errorf("provider %q driver not found", providerName)
		}
		driver, err := newModelDriverForBaseURLLocal(providerInfo.ModelDriver, providerName, region, baseURL)
		if err != nil {
			return nil, "", nil, 0, err
		}
		maxTokens := 0
		if mi, _ := dao.GetModelProviderManager().GetModelByName(providerName, pureModelName); mi != nil && mi.MaxTokens != nil {
			maxTokens = *mi.MaxTokens
		}
		if modelObj != nil && strings.TrimSpace(modelObj.Extra) != "" {
			var tenantExtra tenantModelExtra
			if err := json.Unmarshal([]byte(modelObj.Extra), &tenantExtra); err != nil {
				return nil, "", nil, 0, err
			}
			if tenantExtra.MaxTokens != nil && *tenantExtra.MaxTokens > 0 {
				maxTokens = *tenantExtra.MaxTokens
			}
		}
		apiConfig := &modelModule.APIConfig{ApiKey: &apiKey, Region: &region, BaseURL: &baseURL}
		return driver, modelObj.ModelName, apiConfig, maxTokens, nil
	case !errorsIsRecordNotFound(modelErr):
		return nil, "", nil, 0, fmt.Errorf("model %q lookup failed: %w", modelName, modelErr)
	}

	targetFactoryName := providerName
	if region == "intl" && strings.EqualFold(providerName, "siliconflow") {
		targetFactoryName = "siliconflow_intl"
	}
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
	driver, err := newModelDriverForBaseURLLocal(targetProvider.ModelDriver, providerName, region, baseURL)
	if err != nil {
		return nil, "", nil, 0, err
	}
	apiConfig := &modelModule.APIConfig{ApiKey: &apiKey, Region: &region, BaseURL: &baseURL}
	maxTokens := 0
	if llmInfo.MaxTokens != nil {
		maxTokens = *llmInfo.MaxTokens
	}
	return driver, llmInfo.Name, apiConfig, maxTokens, nil
}

func parseCompositeModelName(compositeName string) (modelName, instanceName, providerName string, err error) {
	parts := strings.Split(compositeName, "@")
	switch len(parts) {
	case 3:
		return parts[0], parts[1], parts[2], nil
	case 2:
		return parts[0], "default", parts[1], nil
	case 1:
		return parts[0], "", "", fmt.Errorf("provider name missing in model name: %s", compositeName)
	}
	n := len(parts)
	return strings.Join(parts[:n-2], "@"), parts[n-2], parts[n-1], nil
}

func newModelDriverForBaseURLLocal(driver modelModule.ModelDriver, providerName, region, baseURL string) (modelModule.ModelDriver, error) {
	if driver == nil {
		return nil, fmt.Errorf("provider %s driver not found", providerName)
	}
	if strings.TrimSpace(baseURL) == "" {
		return driver, nil
	}
	baseURLByRegion := map[string]string{region: baseURL}
	if region == "" {
		baseURLByRegion["default"] = baseURL
	}
	newDriver := driver.NewInstance(baseURLByRegion)
	if newDriver == nil {
		return nil, fmt.Errorf("provider %s does not support custom base_url", providerName)
	}
	return newDriver, nil
}

func errorsIsRecordNotFound(err error) bool {
	return err != nil && (err == gorm.ErrRecordNotFound || strings.Contains(err.Error(), gorm.ErrRecordNotFound.Error()))
}
