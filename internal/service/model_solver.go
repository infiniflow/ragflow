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
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
)

// ModelTarget contains the resolved model configuration.
// It is intentionally separate from the runtime ChatConfig, EmbeddingConfig,
// and RerankConfig types. ModelTarget is selected by modelRef and carries the
// provider-side objects needed to make a model call.
type ModelTarget struct {
	ModelID       string
	ModelName     string
	ModelType     entity.ModelType
	ProviderName  string
	InstanceName  string
	Driver        modelModule.ModelDriver
	APIConfig     *modelModule.APIConfig
	ModelInfo     *modelModule.Model
	ContextLength int
	MaxTokens     int
}

// ModelSolver is the new model-resolution entry point. The existing
// ModelProviderService API remains untouched so callers can migrate to this
// resolver independently.
type ModelSolver struct {
	service *ModelProviderService
}

// NewModelSolver creates a model resolver backed by the standard model
// provider service.
func NewModelSolver() *ModelSolver {
	return &ModelSolver{service: NewModelProviderService()}
}

func (m *ModelProviderService) modelSolver() *ModelSolver {
	return &ModelSolver{service: m}
}

// ResolveModelConfig resolves a model by modelRef for the requested modelType.
// modelRef accepts either a tenant model ID or a composite reference in the
// form "model@instance@provider" ("model@provider" also uses the default
// instance).
func (s *ModelSolver) ResolveModelConfig(ctx context.Context, tenantID string, modelType entity.ModelType, modelRef string) (*ModelTarget, error) {
	modelRef = strings.TrimSpace(modelRef)
	if modelRef == "" {
		return nil, fmt.Errorf("%w: model ref is required", errModelConfigUnavailable)
	}
	if modelType == 0 {
		return nil, fmt.Errorf("%w: model type is required", errModelConfigUnavailable)
	}

	model, err := s.resolveModel(ctx, tenantID, modelType, modelRef)
	if err != nil {
		return nil, err
	}

	maxTokens := model.maxTokens
	if model.modelEntity != nil {
		maxTokens = maxTokensFromModelInfo(model.modelInfo, modelType)
		maxTokens, err = maxTokensFromTenantModelExtra(model.modelEntity, maxTokens)
		if err != nil {
			return nil, fmt.Errorf("%w: read model limits: %v", errModelConfigUnavailable, err)
		}
	}

	contextLength := 0
	if model.modelInfo != nil && model.modelInfo.ContextLength != nil {
		contextLength = *model.modelInfo.ContextLength
	}
	if resolved := dao.ResolveModelContentLength(ctx, dao.DB, tenantID, modelRef, "", ""); resolved > 0 {
		contextLength = resolved
	}

	return &ModelTarget{
		ModelID:       model.modelID,
		ModelName:     model.modelName,
		ModelType:     modelType,
		ProviderName:  model.providerEntity.ProviderName,
		InstanceName:  model.instanceName,
		Driver:        model.driver,
		APIConfig:     model.apiConfig,
		ModelInfo:     model.modelInfo,
		ContextLength: contextLength,
		MaxTokens:     maxTokens,
	}, nil
}

// ResolveDefaultModelConfig resolves the tenant's configured default model
// for modelType. It first uses the tenant model ID when present and falls back
// to the stored model reference if that ID is unavailable.
func (s *ModelSolver) ResolveDefaultModelConfig(ctx context.Context, tenantID string, modelType entity.ModelType) (*ModelTarget, error) {
	if s == nil || s.service == nil {
		return nil, fmt.Errorf("%w: model solver is not initialized", errModelConfigUnavailable)
	}
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, fmt.Errorf("%w: tenant id is required", errModelConfigUnavailable)
	}
	if modelType == 0 {
		return nil, fmt.Errorf("%w: model type is required", errModelConfigUnavailable)
	}
	if modelType == entity.ModelTypeOCR {
		return nil, fmt.Errorf("OCR model name is required")
	}
	if s.service.tenantDAO == nil {
		return nil, fmt.Errorf("%w: tenant service is not initialized", errModelConfigUnavailable)
	}

	tenant, err := s.service.tenantDAO.GetByID(ctx, dao.DB, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant: %s type %s: %w", tenantID, modelType, err)
	}
	modelName, modelID := defaultModelRefs(tenant, modelType)
	if modelID != "" {
		if target, idErr := s.ResolveModelConfig(ctx, tenantID, modelType, modelID); idErr == nil {
			return target, nil
		}
	}
	if strings.TrimSpace(modelName) == "" {
		return nil, fmt.Errorf("no default %s model is set", modelType)
	}
	return s.ResolveModelConfig(ctx, tenantID, modelType, modelName)
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
		return ptrStringValue(tenant.TTSID), ptrStringValue(tenant.TenantTTSID)
	case entity.ModelTypeOCR:
		return ptrStringValue(tenant.OCRID), ptrStringValue(tenant.TenantOCRID)
	default:
		return "", ""
	}
}

// ResolveModelType returns every model category supported by modelRef. A
// model may expose multiple categories, such as chat and vision.
func (s *ModelSolver) ResolveModelType(ctx context.Context, tenantID, modelRef string) ([]entity.ModelType, error) {
	modelRef = strings.TrimSpace(modelRef)
	if modelRef == "" {
		return nil, fmt.Errorf("%w: model ref is required", errModelConfigUnavailable)
	}

	model, err := s.lookupTenantModel(ctx, tenantID, modelRef)
	if err == nil {
		identity, identityErr := s.modelIdentity(ctx, tenantID, model, modelRef)
		if identityErr != nil {
			return nil, identityErr
		}
		return modelTypesFromBitmask(identity.modelType), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	modelType, err := s.resolveCompositeModelType(ctx, tenantID, modelRef)
	if err != nil {
		return nil, err
	}
	return modelTypesFromBitmask(modelType), nil
}

type modelIdentity struct {
	modelEntity    *entity.TenantModel
	providerEntity *entity.TenantModelProvider
	modelType      entity.ModelType
}

type resolvedModel struct {
	modelEntity    *entity.TenantModel
	providerEntity *entity.TenantModelProvider
	modelInfo      *modelModule.Model
	modelType      entity.ModelType
	modelID        string
	modelName      string
	instanceName   string
	driver         modelModule.ModelDriver
	apiConfig      *modelModule.APIConfig
	maxTokens      int
}

func (s *ModelSolver) lookupTenantModel(ctx context.Context, tenantID, modelRef string) (*entity.TenantModel, error) {
	if s == nil || s.service == nil {
		return nil, fmt.Errorf("%w: model solver is not initialized", errModelConfigUnavailable)
	}
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, fmt.Errorf("%w: tenant id is required", errModelConfigUnavailable)
	}
	if modelRef == "" {
		return nil, fmt.Errorf("%w: model ref is required", errModelConfigUnavailable)
	}
	if s.service.modelDAO == nil {
		return nil, fmt.Errorf("%w: model service is not initialized", errModelConfigUnavailable)
	}

	return s.service.modelDAO.GetByID(ctx, dao.DB, modelRef)
}

func (s *ModelSolver) modelIdentity(ctx context.Context, tenantID string, modelEntity *entity.TenantModel, modelRef string) (*modelIdentity, error) {
	if modelEntity == nil {
		return nil, fmt.Errorf("%w: tenant model %q not found", errModelConfigUnavailable, modelRef)
	}
	if modelEntity.Status != "active" {
		return nil, fmt.Errorf("%w: tenant model %q is disabled", errModelConfigUnavailable, modelRef)
	}

	modelType := entity.ModelType(modelEntity.ModelType)
	if modelType == 0 {
		return nil, fmt.Errorf("%w: tenant model %q has no model type", errModelConfigUnavailable, modelRef)
	}
	if s.service.modelProviderDAO == nil || s.service.modelInstanceDAO == nil {
		return nil, fmt.Errorf("%w: model service is not initialized", errModelConfigUnavailable)
	}

	providerEntity, err := s.service.modelProviderDAO.GetByID(ctx, dao.DB, modelEntity.ProviderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: provider id=%s not found for model %q", errModelConfigUnavailable, modelEntity.ProviderID, modelRef)
		}
		return nil, err
	}
	if providerEntity == nil {
		return nil, fmt.Errorf("%w: provider id=%s not found for model %q", errModelConfigUnavailable, modelEntity.ProviderID, modelRef)
	}

	allowed, err := s.service.tenantCanReachProviderTenant(ctx, tenantID, providerEntity.TenantID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, fmt.Errorf(
			"%w: tenant %s has no access to provider owned by tenant %s",
			errModelConfigUnavailable,
			tenantID,
			providerEntity.TenantID,
		)
	}

	return &modelIdentity{
		modelEntity:    modelEntity,
		providerEntity: providerEntity,
		modelType:      modelType,
	}, nil
}

func (s *ModelSolver) resolveModel(ctx context.Context, tenantID string, modelType entity.ModelType, modelRef string) (*resolvedModel, error) {
	modelEntity, err := s.lookupTenantModel(ctx, tenantID, modelRef)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.resolveCompositeModel(ctx, tenantID, modelType, modelRef)
		}
		return nil, err
	}
	identity, err := s.modelIdentity(ctx, tenantID, modelEntity, modelRef)
	if err != nil {
		return nil, err
	}
	if !identity.modelType.Has(modelType) {
		return nil, fmt.Errorf("%w: tenant model %q cannot be used as %s model", errModelConfigUnavailable, modelRef, modelType.String())
	}
	return s.resolveTenantModel(ctx, tenantID, modelType, modelRef, identity)
}

func (s *ModelSolver) resolveTenantModel(ctx context.Context, tenantID string, modelType entity.ModelType, modelRef string, identity *modelIdentity) (*resolvedModel, error) {
	modelEntity := identity.modelEntity
	providerEntity := identity.providerEntity

	instanceEntity, err := s.service.modelInstanceDAO.GetByID(ctx, dao.DB, modelEntity.InstanceID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: instance id=%s not found for model %q", errModelConfigUnavailable, modelEntity.InstanceID, modelRef)
		}
		return nil, err
	}
	if instanceEntity == nil {
		return nil, fmt.Errorf("%w: instance id=%s not found for model %q", errModelConfigUnavailable, modelEntity.InstanceID, modelRef)
	}

	providerInfo := dao.GetModelProviderManager().FindProvider(providerEntity.ProviderName)
	if providerInfo == nil {
		return nil, fmt.Errorf("%w: provider %q driver not found", errModelConfigUnavailable, providerEntity.ProviderName)
	}

	modelInfo, _ := dao.GetModelProviderManager().GetModelByName(providerEntity.ProviderName, modelEntity.ModelName)
	if modelInfo != nil {
		modelInfo, err = modelInfoWithTenantExtra(modelInfo, modelEntity)
		if err != nil {
			return nil, fmt.Errorf("%w: read model metadata: %v", errModelConfigUnavailable, err)
		}
	}

	extra, err := decodeModelInstanceExtra(instanceEntity.Extra)
	if err != nil {
		return nil, fmt.Errorf("%w: decode model instance configuration: %v", errModelConfigUnavailable, err)
	}
	apiKey := instanceEntity.APIKey
	region := extra.Region
	baseURL := extra.BaseURL
	driver, err := newModelDriverForBaseURL(providerInfo.ModelDriver, providerEntity.ProviderName, region, baseURL)
	if err != nil {
		return nil, fmt.Errorf("%w: create model driver: %v", errModelConfigUnavailable, err)
	}

	return &resolvedModel{
		modelEntity:    modelEntity,
		providerEntity: providerEntity,
		modelInfo:      modelInfo,
		modelType:      modelType,
		modelID:        modelEntity.ID,
		modelName:      modelEntity.ModelName,
		instanceName:   instanceEntity.InstanceName,
		driver:         driver,
		apiConfig:      &modelModule.APIConfig{ApiKey: &apiKey, Region: &region, BaseURL: &baseURL},
	}, nil
}

func (s *ModelSolver) resolveCompositeModel(ctx context.Context, tenantID string, modelType entity.ModelType, modelRef string) (*resolvedModel, error) {
	availableTypes, err := s.resolveCompositeModelType(ctx, tenantID, modelRef)
	if err != nil {
		return nil, err
	}
	if !availableTypes.Has(modelType) {
		return nil, fmt.Errorf("%w: model %q cannot be used as %s model", errModelConfigUnavailable, modelRef, modelType.String())
	}
	resolved, err := s.resolveProviderInstanceModel(ctx, tenantID, modelType, modelRef)
	if err != nil {
		return nil, err
	}
	return resolved, nil
}

func (s *ModelSolver) resolveProviderInstanceModel(ctx context.Context, tenantID string, modelType entity.ModelType, modelRef string) (*resolvedModel, error) {
	// TEI builtin embedding short-circuit.
	if modelType == entity.ModelTypeEmbedding && strings.Contains(common.GetEnv(common.EnvComposeProfiles), "tei-") {
		teiModel := common.GetEnv(common.EnvTEIModel)
		teiBaseURL := common.GetEnv(common.EnvTEIBaseURL)
		if modelRef == teiModel {
			driver := modelModule.GetBuiltinEmbeddingModel(modelRef)
			if driver == nil {
				return nil, fmt.Errorf("%w: builtin (TEI) embedding model %q not found", errModelConfigUnavailable, modelRef)
			}
			return &resolvedModel{providerEntity: &entity.TenantModelProvider{ProviderName: "Builtin"}, modelType: modelType, modelName: modelRef, driver: driver, apiConfig: &modelModule.APIConfig{BaseURL: &teiBaseURL}}, nil
		}
		teiPure, _, teiProvider := splitRightAnchoredModelName(modelRef)
		if teiPure == teiModel && (teiProvider == "Builtin" || teiProvider == "") {
			driver := modelModule.GetBuiltinEmbeddingModel(teiPure)
			if driver == nil {
				return nil, fmt.Errorf("%w: builtin (TEI) embedding model %q not found", errModelConfigUnavailable, teiPure)
			}
			return &resolvedModel{providerEntity: &entity.TenantModelProvider{ProviderName: "Builtin"}, modelType: modelType, modelName: teiPure, driver: driver, apiConfig: &modelModule.APIConfig{BaseURL: &teiBaseURL}}, nil
		}
	}

	pureModelName, instanceName, providerName, err := parseModelName(modelRef)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errModelConfigUnavailable, err)
	}
	if modelType == entity.ModelTypeEmbedding {
		var builtinName string
		switch {
		case strings.HasSuffix(modelRef, "@default@Builtin"):
			builtinName = strings.TrimSuffix(modelRef, "@default@Builtin")
		case strings.HasSuffix(modelRef, "@Builtin"):
			builtinName = strings.TrimSuffix(modelRef, "@Builtin")
		}
		if builtinName != "" {
			driver := modelModule.GetBuiltinEmbeddingModel(builtinName)
			if driver == nil {
				return nil, fmt.Errorf("%w: builtin embedding model %q not found", errModelConfigUnavailable, builtinName)
			}
			apiKey, region := "", ""
			maxTokens := 0
			modelInfo, _ := dao.GetModelProviderManager().GetModelByName("Builtin", builtinName)
			if modelInfo != nil {
				maxTokens = maxTokensFromModelInfo(modelInfo, modelType)
			}
			return &resolvedModel{providerEntity: &entity.TenantModelProvider{ProviderName: "Builtin"}, modelInfo: modelInfo, modelType: modelType, modelName: builtinName, driver: driver, apiConfig: &modelModule.APIConfig{ApiKey: &apiKey, Region: &region}, maxTokens: maxTokens}, nil
		}
	}

	provider, err := s.service.modelProviderDAO.GetByTenantIDAndProviderName(ctx, dao.DB, tenantID, providerName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: provider %q lookup failed: %w", errModelConfigUnavailable, providerName, err)
		}
		return nil, fmt.Errorf("provider %q lookup failed: %w", providerName, err)
	}
	if provider == nil {
		return nil, fmt.Errorf("%w: provider %q not found for model %q", errModelConfigUnavailable, providerName, modelRef)
	}
	instance, err := s.service.modelInstanceDAO.GetByProviderIDAndInstanceName(ctx, dao.DB, provider.ID, instanceName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: instance %q lookup failed: %w", errModelConfigUnavailable, instanceName, err)
		}
		return nil, fmt.Errorf("instance %q lookup failed: %w", instanceName, err)
	}
	if instance == nil {
		return nil, fmt.Errorf("%w: instance %q not found for model %q", errModelConfigUnavailable, instanceName, modelRef)
	}
	extra, err := decodeModelInstanceExtra(instance.Extra)
	if err != nil {
		return nil, fmt.Errorf("%w: decode model instance configuration: %v", errModelConfigUnavailable, err)
	}
	region, baseURL, apiKey := extra.Region, extra.BaseURL, instance.APIKey
	modelEntity, err := s.service.modelDAO.GetByProviderIDAndInstanceIDAndModelTypeAndModelName(ctx, dao.DB, provider.ID, instance.ID, int(modelType), pureModelName)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("model %q lookup failed: %w", modelRef, err)
	}
	providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
	if modelEntity != nil {
		if modelEntity.Status == "inactive" {
			return nil, fmt.Errorf("%w: model %q is disabled", errModelConfigUnavailable, modelRef)
		}
		if providerInfo == nil {
			return nil, fmt.Errorf("%w: provider %q driver not found", errModelConfigUnavailable, providerName)
		}
		driver, driverErr := newModelDriverForBaseURL(providerInfo.ModelDriver, providerName, region, baseURL)
		if driverErr != nil {
			return nil, fmt.Errorf("%w: create model driver: %v", errModelConfigUnavailable, driverErr)
		}
		modelInfo, _ := dao.GetModelProviderManager().GetModelByName(providerName, pureModelName)
		maxTokens := maxTokensFromModelInfo(modelInfo, modelType)
		maxTokens, err = maxTokensFromTenantModelExtra(modelEntity, maxTokens)
		if err != nil {
			return nil, fmt.Errorf("%w: read model limits: %v", errModelConfigUnavailable, err)
		}
		return &resolvedModel{modelEntity: modelEntity, providerEntity: provider, modelInfo: modelInfo, modelType: modelType, modelName: modelEntity.ModelName, instanceName: instanceName, driver: driver, apiConfig: &modelModule.APIConfig{ApiKey: &apiKey, Region: &region, BaseURL: &baseURL}, maxTokens: maxTokens}, nil
	}
	if providerInfo == nil {
		return nil, fmt.Errorf("%w: model provider config not found: %s", errModelConfigUnavailable, providerName)
	}
	targetFactoryName := providerName
	if region == "intl" && strings.EqualFold(providerName, "siliconflow") {
		targetFactoryName = "siliconflow_intl"
	}
	targetProvider := dao.GetModelProviderManager().FindProvider(targetFactoryName)
	if targetProvider == nil {
		return nil, fmt.Errorf("%w: model provider config not found: %s", errModelConfigUnavailable, providerName)
	}
	var modelInfo *modelModule.Model
	for _, candidate := range targetProvider.Models {
		if strings.EqualFold(candidate.Name, pureModelName) {
			modelInfo = candidate
			break
		}
	}
	if modelInfo == nil {
		return nil, fmt.Errorf("%w: model config not found: %s", errModelConfigUnavailable, modelRef)
	}
	driver, err := newModelDriverForBaseURL(targetProvider.ModelDriver, providerName, region, baseURL)
	if err != nil {
		return nil, fmt.Errorf("%w: create model driver: %v", errModelConfigUnavailable, err)
	}
	return &resolvedModel{providerEntity: provider, modelInfo: modelInfo, modelType: modelType, modelName: modelInfo.Name, instanceName: instanceName, driver: driver, apiConfig: &modelModule.APIConfig{ApiKey: &apiKey, Region: &region, BaseURL: &baseURL}, maxTokens: maxTokensFromModelInfo(modelInfo, modelType)}, nil
}

func (s *ModelSolver) resolveCompositeModelType(ctx context.Context, tenantID, modelRef string) (entity.ModelType, error) {
	pureModelName, instanceName, providerName, err := parseModelName(modelRef)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", errModelConfigUnavailable, err)
	}

	if strings.EqualFold(providerName, "Builtin") {
		if modelModule.GetBuiltinEmbeddingModel(pureModelName) == nil {
			return 0, fmt.Errorf("%w: builtin model %q not found", errModelConfigUnavailable, pureModelName)
		}
		return entity.ModelTypeEmbedding, nil
	}
	if s == nil || s.service == nil || s.service.modelProviderDAO == nil || s.service.modelInstanceDAO == nil || s.service.modelDAO == nil {
		return 0, fmt.Errorf("%w: model service is not initialized", errModelConfigUnavailable)
	}

	provider, err := s.service.modelProviderDAO.GetByTenantIDAndProviderName(ctx, dao.DB, tenantID, providerName)
	if err != nil {
		return 0, fmt.Errorf("%w: provider %q lookup failed: %w", errModelConfigUnavailable, providerName, err)
	}
	if provider == nil {
		return 0, fmt.Errorf("%w: provider %q not found for model %q", errModelConfigUnavailable, providerName, modelRef)
	}
	instance, err := s.service.modelInstanceDAO.GetByProviderIDAndInstanceName(ctx, dao.DB, provider.ID, instanceName)
	if err != nil {
		return 0, fmt.Errorf("%w: instance %q lookup failed: %w", errModelConfigUnavailable, instanceName, err)
	}
	if instance == nil {
		return 0, fmt.Errorf("%w: instance %q not found for model %q", errModelConfigUnavailable, instanceName, modelRef)
	}

	models, err := s.service.modelDAO.GetModelsByProviderIDAndInstanceIDAndModelName(ctx, dao.DB, provider.ID, instance.ID, pureModelName)
	if err != nil {
		return 0, fmt.Errorf("%w: model %q lookup failed: %v", errModelConfigUnavailable, modelRef, err)
	}
	var modelType entity.ModelType
	for _, model := range models {
		if model != nil && model.Status == "active" {
			modelType |= entity.ModelType(model.ModelType)
		}
	}
	if modelType != 0 {
		return modelType, nil
	}

	providerInfo := dao.GetModelProviderManager().FindProvider(providerName)
	if providerInfo == nil {
		return 0, fmt.Errorf("%w: model provider config not found: %s", errModelConfigUnavailable, providerName)
	}
	for _, model := range providerInfo.Models {
		if model != nil && strings.EqualFold(model.Name, pureModelName) {
			modelType = entity.ModelTypeFromStrings(model.ModelTypes)
			if modelType != 0 {
				return modelType, nil
			}
			break
		}
	}
	return 0, fmt.Errorf("%w: model %q not found for provider %q", errModelConfigUnavailable, pureModelName, providerName)
}

func modelTypesFromBitmask(modelType entity.ModelType) []entity.ModelType {
	modelTypes := make([]entity.ModelType, 0, 7)
	for _, candidate := range []entity.ModelType{
		entity.ModelTypeChat,
		entity.ModelTypeEmbedding,
		entity.ModelTypeSpeech2Text,
		entity.ModelTypeImage2Text,
		entity.ModelTypeRerank,
		entity.ModelTypeTTS,
		entity.ModelTypeOCR,
	} {
		if modelType.Has(candidate) {
			modelTypes = append(modelTypes, candidate)
		}
	}
	return modelTypes
}
