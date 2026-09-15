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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"

	"gorm.io/gorm"
)

type tenantModelExtra struct {
	MaxTokens *int `json:"max_tokens"`
}

var resolveTenantModelByType = defaultResolveTenantModelByType

// resolveModelConfig resolves a specific model reference (tenant-model ID or
// "name@instance@provider" composite) to a driver. Exposed as a package var so
// per-call model-selection tests can inject a fake without a live MySQL.
var resolveModelConfig = defaultResolveModelConfig

// configuredMediaModelID extracts a per-call model reference from a parser setup.
// Image parsing stores the VLM model reference in parse_method when it is not
// "ocr" (mirroring Python rag/flow/parser/parser.py:_image). Other media
// families use vlm.llm_id, matching the frontend parser form.
func configuredMediaModelID(setup schema.ParserSetup, family string) string {
	if family == "image" {
		if ref := getStringOr(setup, "parse_method", ""); ref != "" && !strings.EqualFold(ref, "ocr") {
			return ref
		}
	}
	if vlm, ok := setup["vlm"].(map[string]any); ok {
		if ref, _ := vlm["llm_id"].(string); ref != "" {
			return ref
		}
	}
	return getStringOr(setup, "llm_id", "")
}

func defaultResolveTenantModelByType(ctx context.Context, db *gorm.DB, tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	tenantDAO := dao.NewTenantDAO()
	tenant, err := tenantDAO.GetByID(ctx, db, tenantID)
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
		modelID = *tenant.TTSID
	case entity.ModelTypeOCR:
		modelID = *tenant.OCRID
	default:
		return nil, "", nil, 0, fmt.Errorf("invalid model type: %s", modelType)
	}
	if modelID == "" {
		return nil, "", nil, 0, fmt.Errorf("no default %s model is set", modelType)
	}
	if tenantModelID := tenantModelIDByType(tenant, modelType); tenantModelID != "" {
		driver, modelName, apiConfig, maxTokens, err := resolveModelConfigByID(ctx, db, tenantID, modelType, tenantModelID)
		if err == nil {
			return driver, modelName, apiConfig, maxTokens, nil
		}
	}
	return resolveModelConfig(ctx, db, tenantID, modelType, modelID)
}

// resolveTenantOCRModelByProvider resolves the tenant's first active OCR
// model under the named provider (e.g. "PaddleOCR"), mirroring Python's
// get_first_provider_model_name(tenant_id, provider_name, LLMType.OCR).
// It walks provider -> instances -> models and returns the first model whose
// model_type includes the OCR bit, resolved through resolveModelConfigByID.
var resolveTenantOCRModelByProvider = defaultResolveTenantOCRModelByProvider

func defaultResolveTenantOCRModelByProvider(ctx context.Context, db *gorm.DB, tenantID string, providerName string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	providerDAO := dao.NewTenantModelProviderDAO()
	provider, err := providerDAO.GetByTenantIDAndProviderName(ctx, db, tenantID, providerName)
	if err != nil {
		// Some OCR providers are registered under sibling names: the cloud
		// "PaddleOCR" provider and the local "PaddleOCR.local" provider expose
		// the same OCR capability. Tolerate the alternate spelling before
		// giving up so a tenant configured with either name resolves.
		for _, alias := range paddleOCRProviderAliases() {
			if alias == providerName {
				continue
			}
			if p, aerr := providerDAO.GetByTenantIDAndProviderName(ctx, db, tenantID, alias); aerr == nil {
				provider, err = p, nil
				break
			}
		}
		if err != nil {
			return nil, "", nil, 0, fmt.Errorf("tenant %s has no %s provider: %w", tenantID, providerName, err)
		}
	}
	instanceDAO := dao.NewTenantModelInstanceDAO()
	instances, err := instanceDAO.GetAllInstancesByProviderID(ctx, db, provider.ID)
	if err != nil {
		return nil, "", nil, 0, err
	}
	modelDAO := dao.NewTenantModelDAO()
	for _, instance := range instances {
		models, err := modelDAO.GetModelsByInstanceID(ctx, db, instance.ID)
		if err != nil {
			return nil, "", nil, 0, err
		}
		for _, model := range models {
			if model.Status != "active" || !entity.ModelType(model.ModelType).Has(entity.ModelTypeOCR) {
				continue
			}
			return resolveModelConfigByID(ctx, db, tenantID, entity.ModelTypeOCR, model.ID)
		}
	}
	return nil, "", nil, 0, fmt.Errorf("tenant %s has no active %s OCR model", tenantID, providerName)
}

// paddleOCRProviderAliases returns the registered provider names that expose
// PaddleOCR OCR models: the cloud "PaddleOCR" provider and the local
// "PaddleOCR.local" provider. Both carry OCR-typed models and route through
// the PaddleOCR PDF dispatch regardless of which spelling a tenant configured.
func paddleOCRProviderAliases() []string {
	return []string{"PaddleOCR", "PaddleOCR.local"}
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

func defaultResolveModelConfig(ctx context.Context, db *gorm.DB, tenantID string, modelType entity.ModelType, modelRef string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	modelDAO := dao.NewTenantModelDAO()
	if _, err := modelDAO.GetByID(ctx, db, modelRef); err == nil {
		return resolveModelConfigByID(ctx, db, tenantID, modelType, modelRef)
	} else if !errorsIsRecordNotFound(err) {
		return nil, "", nil, 0, err
	}
	return resolveModelConfigFromProviderInstance(ctx, db, tenantID, modelType, modelRef)
}

func resolveModelConfigByID(ctx context.Context, db *gorm.DB, tenantID string, modelType entity.ModelType, modelID string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	modelDAO := dao.NewTenantModelDAO()
	instanceDAO := dao.NewTenantModelInstanceDAO()
	providerDAO := dao.NewTenantModelProviderDAO()

	modelObj, err := modelDAO.GetByID(ctx, db, modelID)
	if err != nil {
		return nil, "", nil, 0, err
	}
	if modelObj.Status != "active" {
		return nil, "", nil, 0, fmt.Errorf("model %q is disabled", modelID)
	}
	if !entity.ModelType(modelObj.ModelType).Has(modelType) {
		return nil, "", nil, 0, fmt.Errorf("model %q cannot be used as %s model", modelID, modelType.String())
	}
	instance, err := instanceDAO.GetByID(ctx, db, modelObj.InstanceID)
	if err != nil {
		return nil, "", nil, 0, err
	}
	provider, err := providerDAO.GetByID(ctx, db, modelObj.ProviderID)
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
	if mi, _ := dao.GetModelProviderManager().GetModelByName(provider.ProviderName, modelObj.ModelName); mi != nil && mi.MaxOutput != nil {
		maxTokens = *mi.MaxOutput
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

func resolveModelConfigFromProviderInstance(ctx context.Context, db *gorm.DB, tenantID string, modelType entity.ModelType, modelName string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	pureModelName, instanceName, providerName, err := parseCompositeModelName(modelName)
	if err != nil {
		return nil, "", nil, 0, err
	}

	providerDAO := dao.NewTenantModelProviderDAO()
	instanceDAO := dao.NewTenantModelInstanceDAO()
	modelDAO := dao.NewTenantModelDAO()

	provider, err := providerDAO.GetByTenantIDAndProviderName(ctx, db, tenantID, providerName)
	if err != nil {
		return nil, "", nil, 0, fmt.Errorf("provider %q lookup failed: %w", providerName, err)
	}
	instance, err := instanceDAO.GetByProviderIDAndInstanceName(ctx, db, provider.ID, instanceName)
	if err != nil {
		return nil, "", nil, 0, fmt.Errorf("instance %q lookup failed: %w", instanceName, err)
	}

	apiKey := instance.APIKey
	var extra map[string]string
	_ = json.Unmarshal([]byte(instance.Extra), &extra)
	region := extra["region"]
	baseURL := extra["base_url"]

	modelObj, modelErr := modelDAO.GetByProviderIDAndInstanceIDAndModelTypeAndModelName(
		ctx, db, provider.ID, instance.ID, int(modelType), pureModelName,
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
		if mi, _ := dao.GetModelProviderManager().GetModelByName(providerName, pureModelName); mi != nil && mi.MaxOutput != nil {
			maxTokens = *mi.MaxOutput
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
	if llmInfo.MaxOutput != nil {
		maxTokens = *llmInfo.MaxOutput
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
	return err != nil && (errors.Is(err, gorm.ErrRecordNotFound) || strings.Contains(err.Error(), gorm.ErrRecordNotFound.Error()))
}
