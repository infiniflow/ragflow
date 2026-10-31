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
	"strconv"
	"strings"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/permission"
)

// modelTarget is the private result of resolving a model reference. Callers
// should use ModelFactory to construct a typed model or request public metadata.
type modelTarget struct {
	ModelID       string
	ModelName     string
	ModelType     entity.ModelType
	ProviderName  string
	InstanceID    string
	InstanceName  string
	Driver        modelModule.ModelDriver
	APIConfig     *modelModule.APIConfig
	ModelInfo     *modelModule.Model
	ContextLength int
	MaxOutput     int
	SupportsTools bool
}

// ModelAccess identifies the user and active tenant requesting a model.
// UserID may be empty for trusted background work scoped to TenantID.
type ModelAccess struct {
	UserID   string
	TenantID string
}

// ErrNoDefaultModel reports that a membership has no configured model for the
// requested capability.
var ErrNoDefaultModel = errors.New("no default model configured")

// ModelAccessForUser resolves the current tenant context for an authenticated
// user. Request handlers should use this when they need to resolve the user's
// default model rather than assuming the user's personal tenant.
func ModelAccessForUser(ctx context.Context, userID string) (ModelAccess, error) {
	tenantID, err := requireCurrentTenantID(ctx, userID)
	if err != nil {
		return ModelAccess{}, err
	}
	return ModelAccess{UserID: userID, TenantID: tenantID}, nil
}

// ModelReference returns the internal ID when available, falling back to the
// composite name for built-in or catalog-only models.
func ModelReference(info *modelModule.ModelInfo) string {
	if info == nil {
		return ""
	}
	if id := strings.TrimSpace(info.ID); id != "" {
		return id
	}
	name := strings.TrimSpace(info.Name)
	if name == "" {
		return ""
	}
	if provider := strings.TrimSpace(info.ProviderName); provider != "" {
		instance := strings.TrimSpace(info.InstanceName)
		if instance == "" {
			return name + "@" + provider
		}
		return name + "@" + instance + "@" + provider
	}
	return name
}

// ModelFactory resolves model references, checks access, and constructs typed
// model instances. Provider and instance details stay inside this boundary.
type ModelFactory struct {
	userTenantDAO    *dao.UserTenantDAO
	modelProviderDAO *dao.TenantModelProviderDAO
	modelInstanceDAO *dao.TenantModelInstanceDAO
	modelDAO         *dao.TenantModelDAO
}

// NewModelFactory creates a factory backed by the standard model DAOs.
func NewModelFactory() *ModelFactory {
	return &ModelFactory{
		userTenantDAO:    dao.NewUserTenantDAO(),
		modelProviderDAO: dao.NewTenantModelProviderDAO(),
		modelInstanceDAO: dao.NewTenantModelInstanceDAO(),
		modelDAO:         dao.NewTenantModelDAO(),
	}
}

// NewChatModel resolves and constructs a chat model for the given access scope.
func (f *ModelFactory) NewChatModel(ctx context.Context, access ModelAccess, modelRef string) (*modelModule.ChatModel, error) {
	target, err := f.resolveConfig(ctx, access, entity.ModelTypeChat, modelRef)
	if err != nil {
		return nil, err
	}
	return modelModule.NewChatModelWithInfo(target.Driver, &target.ModelName, target.APIConfig, modelInfoFromTarget(target)), nil
}

// NewDefaultChatModel constructs the tenant's configured default chat model.
func (f *ModelFactory) NewDefaultChatModel(ctx context.Context, access ModelAccess) (*modelModule.ChatModel, error) {
	target, err := f.resolveDefaultConfig(ctx, access, entity.ModelTypeChat)
	if err != nil {
		return nil, err
	}
	return modelModule.NewChatModelWithInfo(target.Driver, &target.ModelName, target.APIConfig, modelInfoFromTarget(target)), nil
}

// NewEmbeddingModel resolves and constructs an embedding model.
func (f *ModelFactory) NewEmbeddingModel(ctx context.Context, access ModelAccess, modelRef string) (*modelModule.EmbeddingModel, error) {
	target, err := f.resolveConfig(ctx, access, entity.ModelTypeEmbedding, modelRef)
	if err != nil {
		return nil, err
	}
	return modelModule.NewEmbeddingModelWithInfo(target.Driver, &target.ModelName, target.APIConfig, inputTokenLimit(target), modelInfoFromTarget(target)), nil
}

// NewDefaultEmbeddingModel constructs the tenant's configured default embedding model.
func (f *ModelFactory) NewDefaultEmbeddingModel(ctx context.Context, access ModelAccess) (*modelModule.EmbeddingModel, error) {
	target, err := f.resolveDefaultConfig(ctx, access, entity.ModelTypeEmbedding)
	if err != nil {
		return nil, err
	}
	return modelModule.NewEmbeddingModelWithInfo(target.Driver, &target.ModelName, target.APIConfig, inputTokenLimit(target), modelInfoFromTarget(target)), nil
}

// NewRerankModel resolves and constructs a rerank model.
func (f *ModelFactory) NewRerankModel(ctx context.Context, access ModelAccess, modelRef string) (*modelModule.RerankModel, error) {
	target, err := f.resolveConfig(ctx, access, entity.ModelTypeRerank, modelRef)
	if err != nil {
		return nil, err
	}
	return modelModule.NewRerankModelWithInfo(target.Driver, &target.ModelName, target.APIConfig, inputTokenLimit(target), modelInfoFromTarget(target)), nil
}

// NewDefaultRerankModel constructs the tenant's configured default rerank model.
func (f *ModelFactory) NewDefaultRerankModel(ctx context.Context, access ModelAccess) (*modelModule.RerankModel, error) {
	target, err := f.resolveDefaultConfig(ctx, access, entity.ModelTypeRerank)
	if err != nil {
		return nil, err
	}
	return modelModule.NewRerankModelWithInfo(target.Driver, &target.ModelName, target.APIConfig, inputTokenLimit(target), modelInfoFromTarget(target)), nil
}

// NewOCRModel resolves and constructs an OCR model.
func (f *ModelFactory) NewOCRModel(ctx context.Context, access ModelAccess, modelRef string) (*modelModule.OCRModel, error) {
	target, err := f.resolveConfig(ctx, access, entity.ModelTypeOCR, modelRef)
	if err != nil {
		return nil, err
	}
	return modelModule.NewOCRModelWithInfo(target.Driver, &target.ModelName, target.APIConfig, modelInfoFromTarget(target)), nil
}

// NewASRModel resolves and constructs a speech-to-text model.
func (f *ModelFactory) NewASRModel(ctx context.Context, access ModelAccess, modelRef string) (*modelModule.ASRModel, error) {
	target, err := f.resolveConfig(ctx, access, entity.ModelTypeSpeech2Text, modelRef)
	if err != nil {
		return nil, err
	}
	return modelModule.NewASRModelWithInfo(target.Driver, &target.ModelName, target.APIConfig, modelInfoFromTarget(target)), nil
}

// NewDefaultASRModel constructs the tenant's configured default speech-to-text model.
func (f *ModelFactory) NewDefaultASRModel(ctx context.Context, access ModelAccess) (*modelModule.ASRModel, error) {
	target, err := f.resolveDefaultConfig(ctx, access, entity.ModelTypeSpeech2Text)
	if err != nil {
		return nil, err
	}
	return modelModule.NewASRModelWithInfo(target.Driver, &target.ModelName, target.APIConfig, modelInfoFromTarget(target)), nil
}

// NewTTSModel resolves and constructs a text-to-speech model.
func (f *ModelFactory) NewTTSModel(ctx context.Context, access ModelAccess, modelRef string) (*modelModule.TTSModel, error) {
	target, err := f.resolveConfig(ctx, access, entity.ModelTypeTTS, modelRef)
	if err != nil {
		return nil, err
	}
	return modelModule.NewTTSModelWithInfo(target.Driver, &target.ModelName, target.APIConfig, modelInfoFromTarget(target)), nil
}

// NewDefaultTTSModel constructs the tenant's configured default text-to-speech model.
func (f *ModelFactory) NewDefaultTTSModel(ctx context.Context, access ModelAccess) (*modelModule.TTSModel, error) {
	target, err := f.resolveDefaultConfig(ctx, access, entity.ModelTypeTTS)
	if err != nil {
		return nil, err
	}
	return modelModule.NewTTSModelWithInfo(target.Driver, &target.ModelName, target.APIConfig, modelInfoFromTarget(target)), nil
}

// ResolveInfo returns model metadata without exposing API credentials or a driver.
func (f *ModelFactory) ResolveInfo(ctx context.Context, access ModelAccess, modelType entity.ModelType, modelRef string) (*modelModule.ModelInfo, error) {
	access = modelAccessFromContext(ctx, access)
	if modelType == entity.ModelTypeOCR {
		modelName, instanceName, providerName := splitRightAnchoredModelName(modelRef)
		if modelName == "deepdoc" && instanceName == "default" && providerName == "infiniflow" {
			if err := f.authorize(ctx, access); err != nil {
				return nil, err
			}
			return &modelModule.ModelInfo{
				Name:         modelName,
				ProviderName: providerName,
				InstanceName: instanceName,
				ModelTypes:   []string{entity.ModelTypeOCR.String()},
			}, nil
		}
	}
	if err := f.authorize(ctx, access); err != nil {
		return nil, err
	}
	modelRef = strings.TrimSpace(modelRef)
	if name, ok := builtinEmbeddingName(modelRef); ok && modelType == entity.ModelTypeEmbedding {
		info, _ := dao.GetModelProviderManager().GetModelByName("Builtin", name)
		return modelInfoFromTarget(&modelTarget{ModelName: name, ModelType: modelType, ProviderName: "Builtin", ModelInfo: info}), nil
	}
	identity, err := f.resolveModelIdentity(ctx, access, modelType, modelRef)
	if err != nil {
		return nil, err
	}
	target, err := modelTargetFromIdentity(identity, modelType)
	if err != nil {
		return nil, err
	}

	return modelInfoFromTarget(target), nil
}

// ResolveDefaultInfo returns metadata for the tenant's default model without
// exposing its provider driver or API credentials.
func (f *ModelFactory) ResolveDefaultInfo(ctx context.Context, access ModelAccess, modelType entity.ModelType) (*modelModule.ModelInfo, error) {
	access = modelAccessFromContext(ctx, access)
	if err := f.authorize(ctx, access); err != nil {
		return nil, err
	}
	ref, err := f.defaultModelReference(ctx, access, modelType)
	if err != nil {
		return nil, err
	}
	return f.ResolveInfo(ctx, access, modelType, ref)
}

func modelInfoFromTarget(target *modelTarget) *modelModule.ModelInfo {
	if target == nil {
		return nil
	}
	modelTypes := []string{target.ModelType.String()}
	if target.ModelInfo != nil && len(target.ModelInfo.ModelTypes) > 0 {
		modelTypes = append([]string(nil), target.ModelInfo.ModelTypes...)
	}
	info := &modelModule.ModelInfo{
		ID:            target.ModelID,
		Name:          target.ModelName,
		ProviderName:  target.ProviderName,
		InstanceID:    target.InstanceID,
		InstanceName:  target.InstanceName,
		ModelTypes:    modelTypes,
		ContextLength: target.ContextLength,
		MaxOutput:     target.MaxOutput,
		SupportsTools: target.SupportsTools,
		Catalog:       target.ModelInfo,
	}
	if target.ModelInfo != nil {
		if target.ModelInfo.Class != nil {
			info.ModelClass = *target.ModelInfo.Class
		}
		if target.ModelInfo.Thinking != nil {
			thinking := *target.ModelInfo.Thinking
			info.Thinking = &thinking
		}
	}
	return info
}

func inputTokenLimit(target *modelTarget) int {
	if target == nil {
		return 0
	}
	if target.ContextLength > 0 {
		return target.ContextLength
	}
	if target.ModelInfo != nil && target.ModelInfo.MaxTokens != nil {
		return *target.ModelInfo.MaxTokens
	}
	return 0
}

// modelInstanceExtra contains the instance fields consumed during model
// resolution. Other provider-specific fields remain valid and are ignored.
type modelInstanceExtra struct {
	Region  string `json:"region"`
	BaseURL string `json:"base_url"`
}

// decodeModelInstanceExtra reads only the endpoint fields used for model
// resolution, allowing existing rows to retain provider-specific JSON values.
func decodeModelInstanceExtra(raw string) (modelInstanceExtra, error) {
	if strings.TrimSpace(raw) == "" {
		return modelInstanceExtra{}, nil
	}

	var extra modelInstanceExtra
	if err := json.Unmarshal([]byte(raw), &extra); err != nil {
		return modelInstanceExtra{}, err
	}
	return extra, nil
}

// splitRightAnchoredModelName is a bare-name-tolerant variant of
// common.ParseCompositeModelName used by the Builtin / TEI short-circuit branches in
// model resolution.
//
// Those branches must accept a bare model name (no provider suffix) where
// common.ParseCompositeModelName would return an error, while still preserving any '@'
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

type tenantModelExtra struct {
	MaxTokens    json.RawMessage `json:"max_tokens"`
	ModelTypes   []string        `json:"model_types"`
	MaxDimension *int            `json:"max_dimension"`
	MaxBatchSize *int            `json:"max_batch_size"`
	Dimensions   []int           `json:"dimensions"`
	Thinking     *bool           `json:"thinking"`
}

func modelInfoWithTenantExtra(modelInfo *modelModule.Model, modelEntity *entity.TenantModel) (*modelModule.Model, error) {
	if modelEntity == nil {
		return modelInfo, nil
	}

	model := modelModule.Model{Name: modelEntity.ModelName}
	if modelInfo != nil {
		model = *modelInfo
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
	}
	documentParseSupported := model.ModelTypeMap["doc_parse"]

	if modelEntity.ModelType != 0 {
		setModelInfoTypes(&model, entity.ModelType(modelEntity.ModelType))
		if documentParseSupported {
			model.ModelTypeMap["doc_parse"] = true
		}
	}

	if strings.TrimSpace(modelEntity.Extra) == "" {
		return &model, nil
	}
	var extra tenantModelExtra
	if err := json.Unmarshal([]byte(modelEntity.Extra), &extra); err != nil {
		return nil, err
	}

	if maxTokens := maxTokensFromJSON(extra.MaxTokens); maxTokens > 0 {
		contextLength := maxTokens
		model.ContextLength = &contextLength
	}
	if modelEntity.ModelType == 0 && len(extra.ModelTypes) > 0 {
		model.ModelTypes = append([]string(nil), extra.ModelTypes...)
		model.ModelTypeMap = make(map[string]bool, len(extra.ModelTypes))
		for _, modelType := range extra.ModelTypes {
			model.ModelTypeMap[modelType] = true
		}
		if documentParseSupported {
			model.ModelTypeMap["doc_parse"] = true
		}
	}
	if extra.MaxDimension != nil && *extra.MaxDimension > 0 {
		model.MaxDimension = extra.MaxDimension
	}
	if extra.MaxBatchSize != nil && *extra.MaxBatchSize > 0 {
		model.MaxBatchSize = extra.MaxBatchSize
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

func maxTokensFromJSON(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		if value, err := strconv.ParseFloat(number.String(), 64); err == nil && value > 0 {
			return int(value)
		}
		return 0
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		value, err := strconv.Atoi(strings.TrimSpace(text))
		if err == nil && value > 0 {
			return value
		}
	}
	return 0
}

func setModelInfoTypes(modelInfo *modelModule.Model, modelTypes entity.ModelType) {
	if modelInfo == nil {
		return
	}
	modelInfo.ModelTypes = modelTypes.HumanReadable()
	modelInfo.ModelTypeMap = make(map[string]bool, len(modelInfo.ModelTypes))
	for _, modelType := range modelInfo.ModelTypes {
		modelInfo.ModelTypeMap[modelType] = true
	}
}

func contextLengthFromModelInfo(modelInfo *modelModule.Model) int {
	if modelInfo == nil {
		return 0
	}
	if modelInfo.ContextLength != nil && *modelInfo.ContextLength > 0 {
		return *modelInfo.ContextLength
	}
	// max_tokens on persisted tenant/catalog model rows predates the split
	// between context_length and max_output. It represented the model's
	// context/input window, so never treat it as an output-generation cap.
	if modelInfo.MaxTokens != nil && *modelInfo.MaxTokens > 0 {
		return *modelInfo.MaxTokens
	}
	return 0
}

func maxOutputFromModelInfo(modelInfo *modelModule.Model) int {
	if modelInfo == nil || modelInfo.MaxOutput == nil || *modelInfo.MaxOutput <= 0 {
		return 0
	}
	return *modelInfo.MaxOutput
}

func (f *ModelFactory) resolveConfig(ctx context.Context, access ModelAccess, modelType entity.ModelType, modelRef string) (*modelTarget, error) {
	access = modelAccessFromContext(ctx, access)
	if err := f.authorize(ctx, access); err != nil {
		return nil, err
	}
	return f.resolveConfigAuthorized(ctx, access, modelType, modelRef)
}

func (f *ModelFactory) resolveConfigAuthorized(ctx context.Context, access ModelAccess, modelType entity.ModelType, modelRef string) (*modelTarget, error) {
	modelRef = strings.TrimSpace(modelRef)
	if modelRef == "" {
		return nil, fmt.Errorf("%w: model ref is required", errModelConfigUnavailable)
	}
	// A zero type skips category filtering for catalog-only capabilities such
	// as doc_parse that are not represented by entity.ModelType.
	return f.resolveModel(ctx, access, modelType, modelRef)
}

func normalizeModelAccess(access ModelAccess) ModelAccess {
	access.TenantID = strings.TrimSpace(access.TenantID)
	access.UserID = strings.TrimSpace(access.UserID)
	return access
}

func modelAccessFromContext(ctx context.Context, access ModelAccess) ModelAccess {
	access = normalizeModelAccess(access)
	if access.UserID == "" {
		if subject, ok := permission.SubjectFromContext(ctx); ok && strings.TrimSpace(subject.UserID) != "" &&
			(strings.TrimSpace(subject.TenantID) == "" || strings.TrimSpace(subject.TenantID) == access.TenantID) {
			access.UserID = strings.TrimSpace(subject.UserID)
		}
	}
	return access
}

func (f *ModelFactory) authorize(ctx context.Context, access ModelAccess) error {
	if f == nil {
		return fmt.Errorf("%w: model factory is not initialized", errModelConfigUnavailable)
	}
	if access.TenantID == "" {
		return fmt.Errorf("%w: tenant id is required", errModelConfigUnavailable)
	}
	// A user's personal tenant uses the user ID as its tenant ID. The owner
	// does not need a user_tenant membership row for that tenant; modelIdentity
	// still checks whether the requested provider belongs to a reachable tenant.
	if access.UserID == "" || access.UserID == access.TenantID {
		return nil
	}
	if err := permission.NewDatabaseChecker(dao.DB).CheckTenant(ctx, permission.Subject{UserID: access.UserID}, access.TenantID, permission.TenantMember); err != nil {
		return fmt.Errorf("%w: user %s has no access to tenant %s: %w", errModelConfigUnavailable, access.UserID, access.TenantID, err)
	}
	return nil
}

func (f *ModelFactory) tenantOwnerMembership(ctx context.Context, tenantID string) (*entity.UserTenant, error) {
	if f == nil || f.userTenantDAO == nil {
		return nil, fmt.Errorf("%w: user tenant DAO is not initialized", errModelConfigUnavailable)
	}
	membership, err := f.userTenantDAO.GetOwnerByTenantID(ctx, dao.DB, tenantID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &entity.UserTenant{UserID: tenantID, TenantID: tenantID}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant owner: %s: %w", tenantID, err)
	}
	return membership, nil
}

func (f *ModelFactory) resolveDefaultConfig(ctx context.Context, access ModelAccess, modelType entity.ModelType) (*modelTarget, error) {
	access = modelAccessFromContext(ctx, access)
	if err := f.authorize(ctx, access); err != nil {
		return nil, err
	}
	ref, err := f.defaultModelReference(ctx, access, modelType)
	if err != nil {
		return nil, err
	}
	return f.resolveConfigAuthorized(ctx, access, modelType, ref)
}

func (f *ModelFactory) defaultModelReference(ctx context.Context, access ModelAccess, modelType entity.ModelType) (string, error) {
	if modelType == 0 {
		return "", fmt.Errorf("%w: model type is required", errModelConfigUnavailable)
	}
	var membership *entity.UserTenant
	var err error
	if access.UserID == "" {
		membership, err = f.tenantOwnerMembership(ctx, access.TenantID)
	} else {
		membership, err = f.userTenantDAO.FilterByUserIDAndTenantID(ctx, dao.DB, access.UserID, access.TenantID)
		if errors.Is(err, gorm.ErrRecordNotFound) && access.UserID == access.TenantID {
			membership, err = f.tenantOwnerMembership(ctx, access.TenantID)
		}
	}
	if err != nil {
		return "", fmt.Errorf("failed to get user tenant model settings: %s type %s: %w", access.TenantID, modelType, err)
	}
	modelName, modelID := defaultModelRefs(membership, modelType)
	if modelID != "" {
		return modelID, nil
	}
	if strings.TrimSpace(modelName) == "" {
		return "", fmt.Errorf("%w: %s", ErrNoDefaultModel, modelType)
	}
	return modelName, nil
}

func defaultModelRefs(membership *entity.UserTenant, modelType entity.ModelType) (string, string) {
	if membership == nil {
		return "", ""
	}
	switch modelType {
	case entity.ModelTypeChat:
		return membership.LLMID, ptrStringValue(membership.TenantLLMID)
	case entity.ModelTypeEmbedding:
		return membership.EmbdID, ptrStringValue(membership.TenantEmbdID)
	case entity.ModelTypeRerank:
		return membership.RerankID, ptrStringValue(membership.TenantRerankID)
	case entity.ModelTypeSpeech2Text:
		return membership.ASRID, ptrStringValue(membership.TenantASRID)
	case entity.ModelTypeImage2Text:
		return membership.Img2TxtID, ptrStringValue(membership.TenantImg2TxtID)
	case entity.ModelTypeTTS:
		return ptrStringValue(membership.TTSID), ptrStringValue(membership.TenantTTSID)
	case entity.ModelTypeOCR:
		return ptrStringValue(membership.OCRID), ptrStringValue(membership.TenantOCRID)
	case entity.ModelTypeDecision:
		return ptrStringValue(membership.DecisionID), ptrStringValue(membership.TenantDecisionID)
	default:
		return "", ""
	}
}

// ResolveModelType returns every model category supported by modelRef. A
// model may expose multiple categories, such as chat and vision.
func (s *ModelFactory) ResolveModelType(ctx context.Context, access ModelAccess, modelRef string) ([]entity.ModelType, error) {
	access = modelAccessFromContext(ctx, access)
	if err := s.authorize(ctx, access); err != nil {
		return nil, err
	}
	modelRef = strings.TrimSpace(modelRef)
	if _, ok := builtinEmbeddingName(modelRef); ok {
		return []entity.ModelType{entity.ModelTypeEmbedding}, nil
	}
	identity, err := s.resolveModelIdentity(ctx, access, 0, modelRef)
	if err != nil {
		return nil, err
	}
	return modelTypesFromBitmask(identity.modelType), nil
}

type modelIdentity struct {
	modelName      string
	modelEntity    *entity.TenantModel
	providerEntity *entity.TenantModelProvider
	instanceEntity *entity.TenantModelInstance
	modelType      entity.ModelType
}

func toolSupportFromEnrollment(extra, instanceAPIKey, providerName, modelName string) bool {
	if supported, ok := extraToolSupport(extra); ok {
		return supported
	}
	if supported, ok := extraToolSupport(instanceAPIKey); ok {
		return supported
	}
	return catalogToolSupport(providerName, modelName)
}

func extraToolSupport(extra string) (bool, bool) {
	if strings.TrimSpace(extra) == "" {
		return false, false
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(extra), &fields); err != nil {
		return false, false
	}
	value, ok := fields["is_tools"]
	if !ok {
		return false, false
	}
	switch value := value.(type) {
	case bool:
		return value, true
	case string:
		return strings.EqualFold(strings.TrimSpace(value), "true"), true
	case float64:
		return value != 0, true
	default:
		return false, false
	}
}

func catalogToolSupport(providerName, modelName string) bool {
	providerManager := dao.GetModelProviderManager()
	provider := providerManager.FindProvider(modelModule.CatalogProviderName(providerName))
	if provider == nil {
		return false
	}
	model := providerManager.FindModel(provider, modelName)
	return model != nil && model.Tools != nil && model.Tools.Support
}

// resolveModelIdentity resolves saved IDs and composite names before applying the
// same ownership, relationship, status and resource permission checks.
func (s *ModelFactory) resolveModelIdentity(ctx context.Context, access ModelAccess, requestedType entity.ModelType, modelRef string) (*modelIdentity, error) {
	if s == nil || s.modelDAO == nil || s.modelProviderDAO == nil || s.modelInstanceDAO == nil {
		return nil, fmt.Errorf("%w: model factory is not initialized", errModelConfigUnavailable)
	}
	if modelRef == "" {
		return nil, fmt.Errorf("%w: model ref is required", errModelConfigUnavailable)
	}
	model, err := s.modelDAO.GetByID(ctx, dao.DB, modelRef)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	owner, err := s.tenantOwnerMembership(ctx, access.TenantID)
	if err != nil {
		return nil, err
	}
	identity := &modelIdentity{modelEntity: model}
	var candidates []*entity.TenantModel
	if model != nil {
		identity.modelName = model.ModelName
		identity.modelType = entity.ModelType(model.ModelType)
		identity.providerEntity, err = s.modelProviderDAO.GetByID(ctx, dao.DB, model.ProviderID)
		if err != nil {
			return nil, fmt.Errorf("%w: provider id=%s lookup failed: %w", errModelConfigUnavailable, model.ProviderID, err)
		}
		identity.instanceEntity, err = s.modelInstanceDAO.GetByID(ctx, dao.DB, model.InstanceID)
		if err != nil {
			return nil, fmt.Errorf("%w: instance id=%s lookup failed: %w", errModelConfigUnavailable, model.InstanceID, err)
		}
	} else {
		name, instanceName, providerName, parseErr := common.ParseCompositeModelName(modelRef)
		if parseErr != nil {
			return nil, fmt.Errorf("%w: model %q has no saved model row or valid composite name: %v", errModelConfigUnavailable, modelRef, parseErr)
		}
		identity.modelName = name
		identity.providerEntity, err = s.modelProviderDAO.GetByTenantIDAndUserIDAndProviderName(ctx, dao.DB, access.TenantID, owner.UserID, providerName)
		if err != nil {
			return nil, fmt.Errorf("%w: provider %q lookup failed: %w", errModelConfigUnavailable, providerName, err)
		}
		identity.instanceEntity, err = s.modelInstanceDAO.GetByProviderIDAndInstanceName(ctx, dao.DB, identity.providerEntity.ID, instanceName)
		if err != nil {
			return nil, fmt.Errorf("%w: instance %q lookup failed: %w", errModelConfigUnavailable, instanceName, err)
		}
		candidates, err = s.modelDAO.GetModelsByProviderIDAndInstanceIDAndModelName(ctx, dao.DB, identity.providerEntity.ID, identity.instanceEntity.ID, name)
		if err != nil {
			return nil, fmt.Errorf("model %q lookup failed: %w", modelRef, err)
		}
		for _, candidate := range candidates {
			if !strings.EqualFold(candidate.Status, "active") {
				continue
			}
			identity.modelType |= entity.ModelType(candidate.ModelType)
			if identity.modelEntity == nil && (requestedType == 0 || entity.ModelType(candidate.ModelType).Has(requestedType)) {
				identity.modelEntity = candidate
			}
		}
		if len(candidates) == 0 {
			catalog, catalogErr := dao.GetModelProviderManager().GetModelByName(modelModule.CatalogProviderName(providerName), name)
			if catalogErr == nil && catalog != nil {
				identity.modelType = entity.ModelTypeFromStrings(catalog.ModelTypes)
			}
		}
		if requestedType != 0 && identity.modelType != 0 && !identity.modelType.Has(requestedType) {
			return nil, fmt.Errorf("%w: model %q cannot be used as %s model", errModelConfigUnavailable, modelRef, requestedType.String())
		}
		if identity.modelEntity == nil && (len(candidates) > 0 || identity.modelType == 0) {
			return nil, fmt.Errorf("%w: model %q is disabled or unavailable", errModelConfigUnavailable, modelRef)
		}
	}
	provider, instance := identity.providerEntity, identity.instanceEntity
	if provider.TenantID != access.TenantID || provider.UserID != owner.UserID {
		return nil, fmt.Errorf("%w: tenant %s can use only its owner's model configuration", errModelConfigUnavailable, access.TenantID)
	}
	if instance.ProviderID != provider.ID {
		return nil, fmt.Errorf("%w: instance id=%s does not belong to provider id=%s", errModelConfigUnavailable, instance.ID, provider.ID)
	}
	if !strings.EqualFold(instance.Status, "active") {
		return nil, fmt.Errorf("%w: model instance %q is disabled", errModelConfigUnavailable, instance.InstanceName)
	}
	if identity.modelEntity != nil && !strings.EqualFold(identity.modelEntity.Status, "active") {
		return nil, fmt.Errorf("%w: tenant model %q is disabled", errModelConfigUnavailable, modelRef)
	}
	if access.UserID != "" {
		checker := permission.NewDatabaseChecker(dao.DB)
		subject := permission.Subject{UserID: access.UserID}
		if err := checker.CheckResource(ctx, subject, permission.ResourceRef{Kind: permission.ResourceKindModelProvider, ID: provider.ID}, permission.OperationRead); err != nil {
			return nil, fmt.Errorf("%w: user %s cannot read model provider %q: %w", errModelConfigUnavailable, access.UserID, provider.ProviderName, err)
		}
		if err := checker.CheckResource(ctx, subject, permission.ResourceRef{Kind: permission.ResourceKindModelInstance, ID: instance.ID}, permission.OperationUse); err != nil {
			return nil, fmt.Errorf("%w: user %s cannot use model instance %q: %w", errModelConfigUnavailable, access.UserID, instance.InstanceName, err)
		}
		checkedModels := candidates
		if requestedType != 0 || model != nil {
			checkedModels = nil
			if identity.modelEntity != nil {
				checkedModels = []*entity.TenantModel{identity.modelEntity}
			}
		}
		for _, candidate := range checkedModels {
			if !strings.EqualFold(candidate.Status, "active") {
				continue
			}
			if err := checker.CheckResource(ctx, subject, permission.ResourceRef{Kind: permission.ResourceKindModel, ID: candidate.ID}, permission.OperationUse); err != nil {
				return nil, fmt.Errorf("%w: user %s cannot use model %q: %w", errModelConfigUnavailable, access.UserID, modelRef, err)
			}
		}
	}
	if requestedType != 0 && !identity.modelType.Has(requestedType) {
		return nil, fmt.Errorf("%w: model %q cannot be used as %s model", errModelConfigUnavailable, modelRef, requestedType.String())
	}
	return identity, nil
}

func (s *ModelFactory) resolveModel(ctx context.Context, access ModelAccess, modelType entity.ModelType, modelRef string) (*modelTarget, error) {
	if name, ok := builtinEmbeddingName(modelRef); ok && modelType == entity.ModelTypeEmbedding {
		driver := modelModule.GetBuiltinEmbeddingModel(name)
		if driver == nil {
			return nil, fmt.Errorf("%w: builtin embedding model %q not found", errModelConfigUnavailable, name)
		}
		info, _ := dao.GetModelProviderManager().GetModelByName("Builtin", name)
		config := &modelModule.APIConfig{}
		if strings.Contains(common.GetEnv(common.EnvComposeProfiles), "tei-") && name == common.GetEnv(common.EnvTEIModel) {
			baseURL := common.GetEnv(common.EnvTEIBaseURL)
			config.BaseURL = &baseURL
		}
		return builtinModelTarget(modelType, name, driver, config, info), nil
	}
	identity, err := s.resolveModelIdentity(ctx, access, modelType, modelRef)
	if err != nil {
		return nil, err
	}
	return s.resolveTenantModel(ctx, modelType, identity)
}

func builtinEmbeddingName(modelRef string) (string, bool) {
	if strings.Contains(common.GetEnv(common.EnvComposeProfiles), "tei-") {
		name := common.GetEnv(common.EnvTEIModel)
		pure, _, provider := splitRightAnchoredModelName(modelRef)
		if name != "" && pure == name && (provider == "Builtin" || provider == "") {
			return name, true
		}
	}
	name, _, provider, err := common.ParseCompositeModelName(modelRef)
	return name, err == nil && strings.EqualFold(provider, "Builtin")
}

func (s *ModelFactory) resolveTenantModel(ctx context.Context, modelType entity.ModelType, identity *modelIdentity) (*modelTarget, error) {
	providerEntity := identity.providerEntity
	instanceEntity := identity.instanceEntity

	catalogName := modelModule.CatalogProviderName(providerEntity.ProviderName)
	providerInfo := dao.GetModelProviderManager().FindProvider(catalogName)
	if providerInfo == nil {
		return nil, fmt.Errorf("%w: provider %q driver not found", errModelConfigUnavailable, providerEntity.ProviderName)
	}

	target, err := modelTargetFromIdentity(identity, modelType)
	if err != nil {
		return nil, err
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

	target.Driver = driver
	target.APIConfig = &modelModule.APIConfig{ApiKey: &apiKey, Region: &region, BaseURL: &baseURL}
	return target, nil
}

func modelTargetFromIdentity(identity *modelIdentity, modelType entity.ModelType) (*modelTarget, error) {
	modelEntity, providerEntity, instanceEntity := identity.modelEntity, identity.providerEntity, identity.instanceEntity
	catalogName := modelModule.CatalogProviderName(providerEntity.ProviderName)
	modelInfo, _ := dao.GetModelProviderManager().GetModelByName(catalogName, identity.modelName)
	modelInfo, err := modelInfoWithTenantExtra(modelInfo, modelEntity)
	if err != nil {
		return nil, fmt.Errorf("%w: read model metadata: %v", errModelConfigUnavailable, err)
	}

	if modelEntity != nil && identity.modelType != 0 {
		documentParseSupported := modelInfo.ModelTypeMap["doc_parse"]
		setModelInfoTypes(modelInfo, identity.modelType)
		if documentParseSupported {
			modelInfo.ModelTypeMap["doc_parse"] = true
		}
	}

	modelExtra := ""
	modelID := ""
	if modelEntity != nil {
		modelExtra = modelEntity.Extra
		modelID = modelEntity.ID
	}
	return &modelTarget{
		ModelID:       modelID,
		ModelName:     identity.modelName,
		ModelType:     modelType,
		ProviderName:  providerEntity.ProviderName,
		InstanceID:    instanceEntity.ID,
		InstanceName:  instanceEntity.InstanceName,
		ModelInfo:     modelInfo,
		ContextLength: contextLengthFromModelInfo(modelInfo),
		MaxOutput:     maxOutputFromModelInfo(modelInfo),
		SupportsTools: toolSupportFromEnrollment(modelExtra, instanceEntity.APIKey, providerEntity.ProviderName, identity.modelName),
	}, nil
}

func builtinModelTarget(modelType entity.ModelType, modelName string, driver modelModule.ModelDriver, apiConfig *modelModule.APIConfig, modelInfo *modelModule.Model) *modelTarget {
	var apiKey string
	if apiConfig != nil && apiConfig.ApiKey != nil {
		apiKey = *apiConfig.ApiKey
	}
	return &modelTarget{
		ModelName:     modelName,
		ModelType:     modelType,
		ProviderName:  "Builtin",
		Driver:        driver,
		APIConfig:     apiConfig,
		ModelInfo:     modelInfo,
		ContextLength: contextLengthFromModelInfo(modelInfo),
		MaxOutput:     maxOutputFromModelInfo(modelInfo),
		SupportsTools: toolSupportFromEnrollment("", apiKey, "Builtin", modelName),
	}
}

func modelTypesFromBitmask(modelType entity.ModelType) []entity.ModelType {
	modelTypes := make([]entity.ModelType, 0, 8)
	for _, candidate := range []entity.ModelType{
		entity.ModelTypeChat,
		entity.ModelTypeEmbedding,
		entity.ModelTypeSpeech2Text,
		entity.ModelTypeImage2Text,
		entity.ModelTypeRerank,
		entity.ModelTypeTTS,
		entity.ModelTypeOCR,
		entity.ModelTypeDecision,
	} {
		if modelType.Has(candidate) {
			modelTypes = append(modelTypes, candidate)
		}
	}
	return modelTypes
}
