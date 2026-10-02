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
	"strings"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
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

// ModelFactory resolves model references, checks access, and constructs typed
// model instances. Provider and instance details stay inside this boundary.
type ModelFactory struct {
	tenantDAO        *dao.TenantDAO
	modelProviderDAO *dao.TenantModelProviderDAO
	modelInstanceDAO *dao.TenantModelInstanceDAO
	modelDAO         *dao.TenantModelDAO
	userTenantDAO    *dao.UserTenantDAO
}

// NewModelFactory creates a factory backed by the standard model DAOs.
func NewModelFactory() *ModelFactory {
	return &ModelFactory{
		tenantDAO:        dao.NewTenantDAO(),
		modelProviderDAO: dao.NewTenantModelProviderDAO(),
		modelInstanceDAO: dao.NewTenantModelInstanceDAO(),
		modelDAO:         dao.NewTenantModelDAO(),
		userTenantDAO:    dao.NewUserTenantDAO(),
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
	target, err := f.resolveConfig(ctx, access, modelType, modelRef)
	if err != nil {
		return nil, err
	}
	return modelInfoFromTarget(target), nil
}

// ResolveDefaultInfo returns metadata for the tenant's default model without
// exposing its provider driver or API credentials.
func (f *ModelFactory) ResolveDefaultInfo(ctx context.Context, access ModelAccess, modelType entity.ModelType) (*modelModule.ModelInfo, error) {
	target, err := f.resolveDefaultConfig(ctx, access, modelType)
	if err != nil {
		return nil, err
	}
	return modelInfoFromTarget(target), nil
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
// model resolution.
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

type tenantModelExtra struct {
	MaxTokens    *int     `json:"max_tokens"`
	ModelTypes   []string `json:"model_types"`
	MaxDimension *int     `json:"max_dimension"`
	MaxBatchSize *int     `json:"max_batch_size"`
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
		contextLength := *extra.MaxTokens
		model.ContextLength = &contextLength
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

// modelTargetRef renders a resolved model as the lookups' reference: its
// tenant_model id, or the composite "model@instance@provider" form.
func modelTargetRef(target *modelTarget) string {
	if target == nil {
		return ""
	}
	if target.ModelID != "" {
		return target.ModelID
	}
	return fmt.Sprintf("%s@%s@%s", target.ModelName, target.InstanceName, target.ProviderName)
}

// tenantCanReachProviderTenant reports whether userID owns the provider's tenant
// or is a joined member of it. Mirrors Python's tenant_model_service
// get_model_config_by_id tenant check (:342-347).
func (s *ModelFactory) tenantCanReachProviderTenant(ctx context.Context, userID, ownerTenantID string) (bool, error) {
	if userID == ownerTenantID {
		return true, nil
	}
	if s == nil || s.userTenantDAO == nil {
		return false, fmt.Errorf("%w: user tenant DAO is not initialized", errModelConfigUnavailable)
	}
	userTenants, err := s.userTenantDAO.GetByUserID(ctx, dao.DB, userID)
	if err != nil {
		return false, err
	}
	for _, rel := range userTenants {
		if rel != nil && rel.TenantID == ownerTenantID {
			return true, nil
		}
	}
	return false, nil
}

func (f *ModelFactory) resolveConfig(ctx context.Context, access ModelAccess, modelType entity.ModelType, modelRef string) (*modelTarget, error) {
	if err := f.authorize(ctx, access); err != nil {
		return nil, err
	}
	modelRef = strings.TrimSpace(modelRef)
	if modelRef == "" {
		return nil, fmt.Errorf("%w: model ref is required", errModelConfigUnavailable)
	}
	if modelType == 0 {
		return nil, fmt.Errorf("%w: model type is required", errModelConfigUnavailable)
	}

	model, err := f.resolveModel(ctx, access, modelType, modelRef)
	if err != nil {
		return nil, err
	}

	contextLength := model.contextLength
	if resolved := dao.ResolveModelContentLength(ctx, dao.DB, access.TenantID, modelRef, "", ""); resolved > 0 {
		contextLength = resolved
	}
	if contextLength == 0 {
		contextLength = contextLengthFromModelInfo(model.modelInfo)
	}

	return &modelTarget{
		ModelID:       model.modelID,
		ModelName:     model.modelName,
		ModelType:     modelType,
		ProviderName:  model.providerEntity.ProviderName,
		InstanceID:    model.instanceID,
		InstanceName:  model.instanceName,
		Driver:        model.driver,
		APIConfig:     model.apiConfig,
		ModelInfo:     model.modelInfo,
		ContextLength: contextLength,
		MaxOutput:     maxOutputFromModelInfo(model.modelInfo),
		SupportsTools: model.supportsTools(),
	}, nil
}

func (f *ModelFactory) authorize(ctx context.Context, access ModelAccess) error {
	if f == nil {
		return fmt.Errorf("%w: model factory is not initialized", errModelConfigUnavailable)
	}
	access.TenantID = strings.TrimSpace(access.TenantID)
	access.UserID = strings.TrimSpace(access.UserID)
	if access.TenantID == "" {
		return fmt.Errorf("%w: tenant id is required", errModelConfigUnavailable)
	}
	if access.UserID == "" {
		return nil
	}
	if f == nil || f.userTenantDAO == nil {
		return fmt.Errorf("%w: user tenant DAO is not initialized", errModelConfigUnavailable)
	}
	if _, err := f.userTenantDAO.FilterByUserIDAndTenantID(ctx, dao.DB, access.UserID, access.TenantID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: user %s has no access to tenant %s", errModelConfigUnavailable, access.UserID, access.TenantID)
		}
		return fmt.Errorf("check model access: %w", err)
	}
	return nil
}

// ResolveChatModelType returns the output type used by the chat pipeline for
// attachment dispatch. A model enrolled as both chat and image2text is rendered
// as image2text so image content can be passed to it; a chat-only model remains
// chat. An image2text-only enrollment remains chat here so NewChatModel
// can reject it as an invalid chat model before this display type is used.
//
// Probe failures are conservative and yield chat: that is the type a plain chat
// model is enrolled as, and it keeps image attachments out of a model whose
// vision support could not be established.
func (s *ModelFactory) ResolveChatModelType(ctx context.Context, tenantID, modelRef string) entity.ModelType {
	if s == nil || strings.TrimSpace(modelRef) == "" {
		return entity.ModelTypeChat
	}
	modelTypes, err := s.ResolveModelType(ctx, tenantID, modelRef)
	if err != nil {
		return entity.ModelTypeChat
	}
	hasChat := false
	hasImage2Text := false
	for _, mt := range modelTypes {
		hasChat = hasChat || mt.Has(entity.ModelTypeChat)
		hasImage2Text = hasImage2Text || mt.Has(entity.ModelTypeImage2Text)
	}
	if hasChat && hasImage2Text {
		return entity.ModelTypeImage2Text
	}
	return entity.ModelTypeChat
}

func (f *ModelFactory) resolveDefaultConfig(ctx context.Context, access ModelAccess, modelType entity.ModelType) (*modelTarget, error) {
	if err := f.authorize(ctx, access); err != nil {
		return nil, err
	}
	if modelType == 0 {
		return nil, fmt.Errorf("%w: model type is required", errModelConfigUnavailable)
	}
	if modelType == entity.ModelTypeOCR {
		return nil, fmt.Errorf("OCR model name is required")
	}
	if f.tenantDAO == nil {
		return nil, fmt.Errorf("%w: tenant DAO is not initialized", errModelConfigUnavailable)
	}

	tenant, err := f.tenantDAO.GetByID(ctx, dao.DB, access.TenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant: %s type %s: %w", access.TenantID, modelType, err)
	}
	modelName, modelID := defaultModelRefs(tenant, modelType)
	if modelID != "" {
		if target, idErr := f.resolveConfig(ctx, access, modelType, modelID); idErr == nil {
			return target, nil
		}
	}
	if strings.TrimSpace(modelName) == "" {
		return nil, fmt.Errorf("no default %s model is set", modelType)
	}
	return f.resolveConfig(ctx, access, modelType, modelName)
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
func (s *ModelFactory) ResolveModelType(ctx context.Context, tenantID, modelRef string) ([]entity.ModelType, error) {
	modelRef = strings.TrimSpace(modelRef)
	if modelRef == "" {
		return nil, fmt.Errorf("%w: model ref is required", errModelConfigUnavailable)
	}

	model, err := s.lookupTenantModel(ctx, tenantID, modelRef)
	if err == nil {
		identity, identityErr := s.modelIdentity(ctx, tenantID, "", model, modelRef)
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
	instanceID     string
	instanceName   string
	driver         modelModule.ModelDriver
	apiConfig      *modelModule.APIConfig
	contextLength  int
}

// supportsTools reports whether the resolved model can emit tool calls. The
// enrollment flag takes precedence over the instance credential and provider
// catalog defaults.
//
// It is a method on the resolution rather than a separate lookup so that the
// capability is answered by the row that was just loaded, instead of by a second
// lookup the caller has to key on a type it may get wrong (see
// ResolveChatModelType).
func (m *resolvedModel) supportsTools() bool {
	if m == nil {
		return false
	}
	var extra, providerName, modelName, instanceAPIKey string
	if m.modelEntity != nil {
		extra = m.modelEntity.Extra
		modelName = m.modelEntity.ModelName
	}
	if m.providerEntity != nil {
		providerName = m.providerEntity.ProviderName
	}
	if modelName == "" {
		modelName = m.modelName
	}
	if m.apiConfig != nil && m.apiConfig.ApiKey != nil {
		instanceAPIKey = *m.apiConfig.ApiKey
	}
	return toolSupportFromEnrollment(extra, instanceAPIKey, providerName, modelName)
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
	provider := providerManager.FindProvider(providerName)
	if provider == nil {
		return false
	}
	model := providerManager.FindModel(provider, modelName)
	return model != nil && model.Tools != nil && model.Tools.Support
}

func (s *ModelFactory) lookupTenantModel(ctx context.Context, tenantID, modelRef string) (*entity.TenantModel, error) {
	if s == nil {
		return nil, fmt.Errorf("%w: model factory is not initialized", errModelConfigUnavailable)
	}
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, fmt.Errorf("%w: tenant id is required", errModelConfigUnavailable)
	}
	if modelRef == "" {
		return nil, fmt.Errorf("%w: model ref is required", errModelConfigUnavailable)
	}
	if s.modelDAO == nil {
		return nil, fmt.Errorf("%w: model factory is not initialized", errModelConfigUnavailable)
	}

	return s.modelDAO.GetByID(ctx, dao.DB, modelRef)
}

func (s *ModelFactory) modelIdentity(ctx context.Context, tenantID, userID string, modelEntity *entity.TenantModel, modelRef string) (*modelIdentity, error) {
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
	if s.modelProviderDAO == nil || s.modelInstanceDAO == nil {
		return nil, fmt.Errorf("%w: model factory is not initialized", errModelConfigUnavailable)
	}

	providerEntity, err := s.modelProviderDAO.GetByID(ctx, dao.DB, modelEntity.ProviderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: provider id=%s not found for model %q", errModelConfigUnavailable, modelEntity.ProviderID, modelRef)
		}
		return nil, err
	}
	if providerEntity == nil {
		return nil, fmt.Errorf("%w: provider id=%s not found for model %q", errModelConfigUnavailable, modelEntity.ProviderID, modelRef)
	}

	allowed := providerEntity.TenantID == tenantID
	if !allowed && userID != "" {
		var err error
		allowed, err = s.tenantCanReachProviderTenant(ctx, userID, providerEntity.TenantID)
		if err != nil {
			return nil, err
		}
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

func (s *ModelFactory) resolveModel(ctx context.Context, access ModelAccess, modelType entity.ModelType, modelRef string) (*resolvedModel, error) {
	modelEntity, err := s.lookupTenantModel(ctx, access.TenantID, modelRef)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// A bare ref that is BOTH unknown as a tenant model id AND rejected as
			// a composite name is almost always a DANGLING REFERENCE — a knowledge
			// base or chat pointing at a tenant_model row that was deleted. Say so
			// explicitly: the underlying "provider name missing in model name:
			// <uuid>" reads like a naming-format mistake and sends the operator to
			// look at the model's name instead of at the row that no longer exists.
			composite, compositeErr := s.resolveCompositeModel(ctx, access.TenantID, modelType, modelRef)
			if compositeErr != nil && !strings.Contains(modelRef, "@") {
				return nil, fmt.Errorf("model %q is neither a tenant model id (no tenant_model row) nor a valid composite name — the reference is dangling: %w", modelRef, compositeErr)
			}
			return composite, compositeErr
		}
		return nil, err
	}
	identity, err := s.modelIdentity(ctx, access.TenantID, access.UserID, modelEntity, modelRef)
	if err != nil {
		return nil, err
	}
	if !identity.modelType.Has(modelType) {
		return nil, fmt.Errorf("%w: tenant model %q cannot be used as %s model", errModelConfigUnavailable, modelRef, modelType.String())
	}
	return s.resolveTenantModel(ctx, access.TenantID, modelType, modelRef, identity)
}

func (s *ModelFactory) resolveTenantModel(ctx context.Context, tenantID string, modelType entity.ModelType, modelRef string, identity *modelIdentity) (*resolvedModel, error) {
	modelEntity := identity.modelEntity
	providerEntity := identity.providerEntity

	instanceEntity, err := s.modelInstanceDAO.GetByID(ctx, dao.DB, modelEntity.InstanceID)
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
		instanceID:     instanceEntity.ID,
		instanceName:   instanceEntity.InstanceName,
		driver:         driver,
		apiConfig:      &modelModule.APIConfig{ApiKey: &apiKey, Region: &region, BaseURL: &baseURL},
	}, nil
}

func (s *ModelFactory) resolveCompositeModel(ctx context.Context, tenantID string, modelType entity.ModelType, modelRef string) (*resolvedModel, error) {
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

func (s *ModelFactory) resolveProviderInstanceModel(ctx context.Context, tenantID string, modelType entity.ModelType, modelRef string) (*resolvedModel, error) {
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
				maxTokens = contextLengthFromModelInfo(modelInfo)
			}
			return &resolvedModel{providerEntity: &entity.TenantModelProvider{ProviderName: "Builtin"}, modelInfo: modelInfo, modelType: modelType, modelName: builtinName, driver: driver, apiConfig: &modelModule.APIConfig{ApiKey: &apiKey, Region: &region}, contextLength: maxTokens}, nil
		}
	}

	provider, err := s.modelProviderDAO.GetByTenantIDAndProviderName(ctx, dao.DB, tenantID, providerName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: provider %q lookup failed: %w", errModelConfigUnavailable, providerName, err)
		}
		return nil, fmt.Errorf("provider %q lookup failed: %w", providerName, err)
	}
	if provider == nil {
		return nil, fmt.Errorf("%w: provider %q not found for model %q", errModelConfigUnavailable, providerName, modelRef)
	}
	instance, err := s.modelInstanceDAO.GetByProviderIDAndInstanceName(ctx, dao.DB, provider.ID, instanceName)
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
	modelEntity, err := s.modelDAO.GetByProviderIDAndInstanceIDAndModelTypeAndModelName(ctx, dao.DB, provider.ID, instance.ID, int(modelType), pureModelName)
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
		if modelInfo != nil {
			modelInfo, err = modelInfoWithTenantExtra(modelInfo, modelEntity)
			if err != nil {
				return nil, fmt.Errorf("%w: read model metadata: %v", errModelConfigUnavailable, err)
			}
		}
		contextLength := contextLengthFromModelInfo(modelInfo)
		return &resolvedModel{modelEntity: modelEntity, providerEntity: provider, modelInfo: modelInfo, modelType: modelType, modelID: modelEntity.ID, modelName: modelEntity.ModelName, instanceID: instance.ID, instanceName: instanceName, driver: driver, apiConfig: &modelModule.APIConfig{ApiKey: &apiKey, Region: &region, BaseURL: &baseURL}, contextLength: contextLength}, nil
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
	return &resolvedModel{providerEntity: provider, modelInfo: modelInfo, modelType: modelType, modelName: modelInfo.Name, instanceID: instance.ID, instanceName: instanceName, driver: driver, apiConfig: &modelModule.APIConfig{ApiKey: &apiKey, Region: &region, BaseURL: &baseURL}, contextLength: contextLengthFromModelInfo(modelInfo)}, nil
}

func (s *ModelFactory) resolveCompositeModelType(ctx context.Context, tenantID, modelRef string) (entity.ModelType, error) {
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
	if s == nil || s.modelProviderDAO == nil || s.modelInstanceDAO == nil || s.modelDAO == nil {
		return 0, fmt.Errorf("%w: model factory is not initialized", errModelConfigUnavailable)
	}

	provider, err := s.modelProviderDAO.GetByTenantIDAndProviderName(ctx, dao.DB, tenantID, providerName)
	if err != nil {
		return 0, fmt.Errorf("%w: provider %q lookup failed: %w", errModelConfigUnavailable, providerName, err)
	}
	if provider == nil {
		return 0, fmt.Errorf("%w: provider %q not found for model %q", errModelConfigUnavailable, providerName, modelRef)
	}
	instance, err := s.modelInstanceDAO.GetByProviderIDAndInstanceName(ctx, dao.DB, provider.ID, instanceName)
	if err != nil {
		return 0, fmt.Errorf("%w: instance %q lookup failed: %w", errModelConfigUnavailable, instanceName, err)
	}
	if instance == nil {
		return 0, fmt.Errorf("%w: instance %q not found for model %q", errModelConfigUnavailable, instanceName, modelRef)
	}

	models, err := s.modelDAO.GetModelsByProviderIDAndInstanceIDAndModelName(ctx, dao.DB, provider.ID, instance.ID, pureModelName)
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
