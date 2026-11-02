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
	"ragflow/internal/common"
	"strings"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/permission"

	"gorm.io/gorm"
)

type tenantModelExtra struct {
	MaxTokens *int `json:"max_tokens"`
}

type resolvedModelConfig struct {
	driver        modelModule.ModelDriver
	model         *entity.TenantModel
	instance      *entity.TenantModelInstance
	apiConfig     *modelModule.APIConfig
	contextLength int
}

var resolveTenantModelByType = defaultResolveTenantModelByType

// resolveModelConfig resolves a specific model reference (tenant-model ID or
// "name@instance@provider" composite) to a driver. Exposed as a package var so
// per-call model-selection tests can inject a fake without a live MySQL.
var resolveModelConfig = defaultResolveModelConfig

// configuredAudioModelID reads the audio setup's ASR model reference.
func configuredAudioModelID(setup schema.ParserSetup) string {
	if vlm, ok := setup["vlm"].(map[string]any); ok {
		if ref, _ := vlm["llm_id"].(string); ref != "" {
			return ref
		}
	}
	return getStringOr(setup, "llm_id", "")
}

// defaultResolveTenantModelByType resolves the current member's configured default.
func defaultResolveTenantModelByType(ctx context.Context, db *gorm.DB, tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	membership, err := resolveDefaultModelMembership(ctx, db, tenantID)
	if err != nil {
		return nil, "", nil, 0, fmt.Errorf("load default model settings for tenant %s: %w", tenantID, err)
	}
	var modelID string
	switch modelType {
	case entity.ModelTypeChat:
		modelID = membership.LLMID
	case entity.ModelTypeEmbedding:
		modelID = membership.EmbdID
	case entity.ModelTypeRerank:
		modelID = membership.RerankID
	case entity.ModelTypeSpeech2Text:
		modelID = membership.ASRID
	case entity.ModelTypeImage2Text:
		modelID = membership.Img2TxtID
	case entity.ModelTypeTTS:
		modelID = stringValue(membership.TTSID)
	case entity.ModelTypeOCR:
		modelID = stringValue(membership.OCRID)
	default:
		return nil, "", nil, 0, fmt.Errorf("invalid model type: %s", modelType)
	}
	if modelID == "" {
		return nil, "", nil, 0, fmt.Errorf("no default %s model is set", modelType)
	}
	if tenantModelID := tenantModelIDByType(membership, modelType); tenantModelID != "" {
		resolved, err := resolveModelConfigByID(ctx, db, tenantID, modelType, tenantModelID)
		if err == nil {
			return resolved.driver, resolved.model.ModelName, resolved.apiConfig, resolved.contextLength, nil
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

// defaultResolveTenantOCRModelByProvider searches the tenant's named provider,
// trying its aliases only if the provider lookup fails. The first active OCR
// model is resolved by ID; its resolution error is returned rather than trying
// another model.
func defaultResolveTenantOCRModelByProvider(ctx context.Context, db *gorm.DB, tenantID string, providerName string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	providerDAO := dao.NewTenantModelProviderDAO()
	provider, err := providerDAO.GetByTenantIDAndProviderName(ctx, db, tenantID, providerName)
	if err != nil {
		// Some OCR capabilities are registered under sibling provider names
		// (cloud vs. local). Tolerate the alternate spelling before giving up
		// so a tenant configured with either name resolves.
		for _, alias := range ocrProviderAliases(providerName) {
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
			resolved, err := resolveModelConfigByID(ctx, db, tenantID, entity.ModelTypeOCR, model.ID)
			if err != nil {
				return nil, "", nil, 0, err
			}
			return resolved.driver, resolved.model.ModelName, resolved.apiConfig, resolved.contextLength, nil
		}
	}
	return nil, "", nil, 0, fmt.Errorf("tenant %s has no active %s OCR model", tenantID, providerName)
}

// ocrProviderAliases returns the registered provider names that expose the
// same OCR capability as providerName: the cloud "PaddleOCR" provider and
// the local "PaddleOCR.local" provider are interchangeable, and so are the
// local "MinerU" provider and the remote "MinerU.Net" provider. The PDF
// dispatch resolves a tenant's OCR model regardless of which spelling was
// configured.
func ocrProviderAliases(providerName string) []string {
	switch providerName {
	case "PaddleOCR", "PaddleOCR.local":
		return []string{"PaddleOCR", "PaddleOCR.local"}
	case "MinerU", "MinerU.Net":
		return []string{"MinerU", "MinerU.Net"}
	default:
		return []string{providerName}
	}
}

func resolveDefaultModelMembership(ctx context.Context, db *gorm.DB, tenantID string) (*entity.UserTenant, error) {
	membershipDAO := dao.NewUserTenantDAO()
	if subject, ok := permission.SubjectFromContext(ctx); ok && subject.UserID != "" && (subject.TenantID == "" || subject.TenantID == tenantID) {
		return membershipDAO.FilterByUserIDAndTenantID(ctx, db, subject.UserID, tenantID)
	}
	return membershipDAO.GetOwnerByTenantID(ctx, db, tenantID)
}

func tenantModelIDByType(membership *entity.UserTenant, modelType entity.ModelType) string {
	if membership == nil {
		return ""
	}
	switch modelType {
	case entity.ModelTypeChat:
		return stringValue(membership.TenantLLMID)
	case entity.ModelTypeEmbedding:
		return stringValue(membership.TenantEmbdID)
	case entity.ModelTypeRerank:
		return stringValue(membership.TenantRerankID)
	case entity.ModelTypeSpeech2Text:
		return stringValue(membership.TenantASRID)
	case entity.ModelTypeImage2Text:
		return stringValue(membership.TenantImg2TxtID)
	case entity.ModelTypeTTS:
		return stringValue(membership.TenantTTSID)
	case entity.ModelTypeOCR:
		return stringValue(membership.TenantOCRID)
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

// defaultResolveModelConfig treats a reference without "@" as an exact tenant
// model ID and a reference containing "@" as a provider-instance composite.
// Failed ID lookups never fall through to composite or catalog resolution.
func defaultResolveModelConfig(ctx context.Context, db *gorm.DB, tenantID string, modelType entity.ModelType, modelRef string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	if !strings.Contains(modelRef, "@") {
		resolved, err := resolveModelConfigByID(ctx, db, tenantID, modelType, modelRef)
		if err != nil {
			return nil, "", nil, 0, err
		}
		return resolved.driver, resolved.model.ModelName, resolved.apiConfig, resolved.contextLength, nil
	}
	return resolveModelConfigFromProviderInstance(ctx, db, tenantID, modelType, modelRef)
}

// resolveModelConfigByID resolves an enrolled model within tenantID before
// checking status or type or reading instance credentials. Missing and foreign
// models share one public error; other database failures remain errors.
// Query sessions preserve caller scopes without mutating the caller's Statement.
// Instance/provider consistency and instance status are caller responsibilities.
func resolveModelConfigByID(ctx context.Context, db *gorm.DB, tenantID string, modelType entity.ModelType, modelID string) (*resolvedModelConfig, error) {
	modelDAO := dao.NewTenantModelDAO()
	instanceDAO := dao.NewTenantModelInstanceDAO()
	providerDAO := dao.NewTenantModelProviderDAO()

	// Scope the lookup itself so missing and foreign models are indistinguishable
	// before exposing status, type, instance configuration, or driver details.
	providers := db.Session(&gorm.Session{}).Model(&entity.TenantModelProvider{}).
		Select("id").Where("tenant_id = ?", tenantID)
	scopedDB := db.Session(&gorm.Session{}).Where("provider_id IN (?)", providers)
	modelObj, err := modelDAO.GetByID(ctx, scopedDB, modelID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("model not found or inaccessible")
		}
		return nil, err
	}
	provider, err := providerDAO.GetByID(ctx, db, modelObj.ProviderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("model not found or inaccessible")
		}
		return nil, err
	}
	if provider.TenantID != tenantID {
		return nil, errors.New("model not found or inaccessible")
	}
	if modelObj.Status != "active" {
		return nil, fmt.Errorf("model %q is disabled", modelID)
	}
	if !entity.ModelType(modelObj.ModelType).Has(modelType) {
		return nil, fmt.Errorf("model %q cannot be used as %s model", modelID, modelType.String())
	}
	instance, err := instanceDAO.GetByID(ctx, db, modelObj.InstanceID)
	if err != nil {
		return nil, err
	}
	// A disabled instance must not be used: its credentials are no longer
	// accepted by the provider, and running against it would hide that the
	// selected deployment is unavailable. Mirrors the service-layer check.
	if !strings.EqualFold(instance.Status, "active") {
		return nil, fmt.Errorf("model instance %q is disabled", instance.InstanceName)
	}

	apiKey := instance.APIKey
	var extra map[string]string
	_ = json.Unmarshal([]byte(instance.Extra), &extra)
	region := extra["region"]
	baseURL := extra["base_url"]

	catalogName := modelModule.CatalogProviderName(provider.ProviderName)
	providerInfo := dao.GetModelProviderManager().FindProvider(catalogName)
	if providerInfo == nil {
		return nil, fmt.Errorf("provider %q driver not found", provider.ProviderName)
	}
	driver, err := newModelDriverForBaseURLLocal(providerInfo.ModelDriver, provider.ProviderName, region, baseURL)
	if err != nil {
		return nil, err
	}
	contextLength := 0
	if mi, _ := dao.GetModelProviderManager().GetModelByName(catalogName, modelObj.ModelName); mi != nil {
		if mi.ContextLength != nil {
			contextLength = *mi.ContextLength
		} else if mi.MaxTokens != nil {
			contextLength = *mi.MaxTokens
		}
	}
	if strings.TrimSpace(modelObj.Extra) != "" {
		var tenantExtra tenantModelExtra
		if err := json.Unmarshal([]byte(modelObj.Extra), &tenantExtra); err != nil {
			return nil, err
		}
		if tenantExtra.MaxTokens != nil && *tenantExtra.MaxTokens > 0 {
			contextLength = *tenantExtra.MaxTokens
		}
	}
	apiConfig := &modelModule.APIConfig{ApiKey: &apiKey, Region: &region, BaseURL: &baseURL}
	return &resolvedModelConfig{
		driver: driver, model: modelObj, instance: instance,
		apiConfig: apiConfig, contextLength: contextLength,
	}, nil
}

func resolveModelConfigFromProviderInstance(ctx context.Context, db *gorm.DB, tenantID string, modelType entity.ModelType, modelName string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
	pureModelName, instanceName, providerName, err := common.ParseCompositeModelName(modelName)
	if err != nil {
		return nil, "", nil, 0, err
	}
	catalogName := modelModule.CatalogProviderName(providerName)

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
	// The instance named by the reference must be usable. A two-part
	// "model@provider" reference names the "default" instance implicitly, so
	// an inactive default fails here instead of being answered by another
	// instance of the same provider.
	if !strings.EqualFold(instance.Status, "active") {
		return nil, "", nil, 0, fmt.Errorf("model instance %q is disabled", instanceName)
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
		providerInfo := dao.GetModelProviderManager().FindProvider(catalogName)
		if providerInfo == nil {
			return nil, "", nil, 0, fmt.Errorf("provider %q driver not found", providerName)
		}
		driver, err := newModelDriverForBaseURLLocal(providerInfo.ModelDriver, providerName, region, baseURL)
		if err != nil {
			return nil, "", nil, 0, err
		}
		contextLength := 0
		if mi, _ := dao.GetModelProviderManager().GetModelByName(catalogName, pureModelName); mi != nil {
			if mi.ContextLength != nil {
				contextLength = *mi.ContextLength
			} else if mi.MaxTokens != nil {
				contextLength = *mi.MaxTokens
			}
		}
		if modelObj != nil && strings.TrimSpace(modelObj.Extra) != "" {
			var tenantExtra tenantModelExtra
			if err := json.Unmarshal([]byte(modelObj.Extra), &tenantExtra); err != nil {
				return nil, "", nil, 0, err
			}
			if tenantExtra.MaxTokens != nil && *tenantExtra.MaxTokens > 0 {
				contextLength = *tenantExtra.MaxTokens
			}
		}
		apiConfig := &modelModule.APIConfig{ApiKey: &apiKey, Region: &region, BaseURL: &baseURL}
		return driver, modelObj.ModelName, apiConfig, contextLength, nil
	case !errorsIsRecordNotFound(modelErr):
		return nil, "", nil, 0, fmt.Errorf("model %q lookup failed: %w", modelName, modelErr)
	}

	targetProvider := dao.GetModelProviderManager().FindProvider(catalogName)
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
	contextLength := 0
	if llmInfo.ContextLength != nil {
		contextLength = *llmInfo.ContextLength
	} else if llmInfo.MaxTokens != nil {
		contextLength = *llmInfo.MaxTokens
	}
	return driver, llmInfo.Name, apiConfig, contextLength, nil
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
