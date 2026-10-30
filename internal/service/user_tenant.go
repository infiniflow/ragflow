//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

// SetModelSettingsRequest represents default model selections for a user in the current tenant.
type SetModelSettingsRequest struct {
	TenantID     *string                `json:"tenant_id"`
	ASRID        *string                `json:"asr_id"`
	EmbdID       *string                `json:"embd_id"`
	Img2TxtID    *string                `json:"img2txt_id"`
	LLMID        *string                `json:"llm_id"`
	RerankID     *string                `json:"rerank_id"`
	TTSID        *string                `json:"tts_id"`
	Raw          map[string]interface{} `json:"-"`
	DefaultModel *DefaultModelSelection `json:"-"`
}

// DefaultModelSelection is the single-model form used by the default-model endpoints.
type DefaultModelSelection struct {
	ModelProvider string
	ModelInstance string
	ModelName     string
	ModelType     string
	ModelID       string
}

// UserTenantService manages user settings scoped to a tenant membership.
type UserTenantService struct {
	userTenantDAO *dao.UserTenantDAO
	tenantDAO     *dao.TenantDAO
}

// NewUserTenantService creates a UserTenantService.
func NewUserTenantService() *UserTenantService {
	return &UserTenantService{
		userTenantDAO: dao.NewUserTenantDAO(),
		tenantDAO:     dao.NewTenantDAO(),
	}
}

// TenantInfoResponse contains the current tenant and membership settings.
type TenantInfoResponse struct {
	TenantID  string  `json:"tenant_id"`
	Name      *string `json:"name,omitempty"`
	LLMID     string  `json:"llm_id"`
	EmbDID    string  `json:"embd_id"`
	RerankID  string  `json:"rerank_id"`
	ASRID     string  `json:"asr_id"`
	Img2TxtID string  `json:"img2txt_id"`
	TTSID     string  `json:"tts_id"`
	ParserIDs string  `json:"parser_ids"`
	Role      string  `json:"role"`
}

// GetCurrentTenantInfo returns the active tenant with the user's membership settings.
func (s *UserTenantService) GetCurrentTenantInfo(ctx context.Context, userID string) (*TenantInfoResponse, error) {
	tenantID, err := requireCurrentTenantID(ctx, userID)
	if err != nil {
		return nil, err
	}
	membership, err := s.userTenantDAO.FilterByUserIDAndTenantID(ctx, dao.DB, userID, tenantID)
	if err != nil {
		return nil, err
	}
	tenant, err := s.tenantDAO.GetByID(ctx, dao.DB, tenantID)
	if err != nil {
		return nil, err
	}
	return &TenantInfoResponse{
		TenantID:  tenant.ID,
		Name:      tenant.Name,
		LLMID:     membership.LLMID,
		EmbDID:    membership.EmbdID,
		RerankID:  membership.RerankID,
		ASRID:     membership.ASRID,
		Img2TxtID: membership.Img2TxtID,
		TTSID:     ptrStringValue(membership.TTSID),
		ParserIDs: tenant.ParserIDs,
		Role:      membership.Role,
	}, nil
}

// UserTenantRelation represents a user-tenant relationship response.
type UserTenantRelation struct {
	ID       string `json:"id"`
	UserID   string `json:"user_id"`
	TenantID string `json:"tenant_id"`
	Role     string `json:"role"`
}

// GetUserTenantRelationByUserIDWithContext retrieves a user's active tenant relationships.
func (s *UserTenantService) GetUserTenantRelationByUserIDWithContext(ctx context.Context, userID string) ([]*UserTenantRelation, error) {
	relations, err := s.userTenantDAO.GetByUserID(ctx, dao.DB, userID)
	if err != nil {
		return nil, err
	}

	result := make([]*UserTenantRelation, len(relations))
	for i, relation := range relations {
		result[i] = &UserTenantRelation{
			ID:       relation.ID,
			UserID:   relation.UserID,
			TenantID: relation.TenantID,
			Role:     relation.Role,
		}
	}
	return result, nil
}

var defaultModelTypeNames = [...]string{"chat", "embedding", "rerank", "asr", "vision", "tts", "ocr", "decision"}

// SetModelSettings validates and updates default models on the user's active tenant membership.
func (s *UserTenantService) SetModelSettings(ctx context.Context, userID string, req *SetModelSettingsRequest) (common.ErrorCode, error) {
	if req == nil || (req.Raw != nil && req.DefaultModel != nil) {
		return common.CodeArgumentError, fmt.Errorf("provide either model settings or one default model selection")
	}
	raw := req.Raw
	if selection := req.DefaultModel; selection != nil {
		legacyField, idField, _, err := defaultModelFields(selection.ModelType)
		if err != nil {
			return common.CodeArgumentError, err
		}
		provider := strings.TrimSpace(selection.ModelProvider)
		instance := strings.TrimSpace(selection.ModelInstance)
		name := strings.TrimSpace(selection.ModelName)
		modelID := strings.TrimSpace(selection.ModelID)
		anyNameField := provider != "" || instance != "" || name != ""
		if anyNameField && modelID == "" && (provider == "" || instance == "" || name == "") {
			return common.CodeArgumentError, fmt.Errorf("model provider, instance and name must be specified together")
		}
		modelRef := ""
		if anyNameField && provider != "" && instance != "" && name != "" {
			modelRef = fmt.Sprintf("%s@%s@%s", name, instance, provider)
		}
		var modelIDValue interface{}
		if modelID != "" {
			modelIDValue = modelID
		}
		raw = map[string]interface{}{legacyField: modelRef, idField: modelIDValue}
	}
	updates, err := modelSettingUpdates(raw)
	if err != nil {
		return common.CodeArgumentError, err
	}
	tenantID, err := requireCurrentTenantID(ctx, userID)
	if err != nil {
		return common.CodeForbidden, err
	}
	if req.TenantID != nil && strings.TrimSpace(*req.TenantID) != "" && strings.TrimSpace(*req.TenantID) != tenantID {
		return common.CodeForbidden, fmt.Errorf("tenant_id must match the active Tenant context")
	}
	if rawTenantID, ok := raw["tenant_id"].(string); ok && strings.TrimSpace(rawTenantID) != "" && strings.TrimSpace(rawTenantID) != tenantID {
		return common.CodeForbidden, fmt.Errorf("tenant_id must match the active Tenant context")
	}
	if s == nil || s.userTenantDAO == nil || dao.DB == nil {
		return common.CodeExceptionError, fmt.Errorf("User Tenant service is not initialized")
	}
	membership, err := s.userTenantDAO.FilterByUserIDAndTenantID(ctx, dao.DB, userID, tenantID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return common.CodeForbidden, err
		}
		return common.CodeExceptionError, err
	}

	factory := NewModelFactory()
	modelUpdates := make(map[string]interface{}, len(updates))
	for _, modelTypeName := range defaultModelTypeNames {
		legacyField, idField, modelType, _ := defaultModelFields(modelTypeName)
		legacyValue, hasLegacy := updates[legacyField]
		idValue, hasID := updates[idField]
		if !hasLegacy && !hasID {
			continue
		}
		legacyRef, _ := legacyValue.(string)
		idRef, _ := idValue.(string)
		legacyRef = strings.TrimSpace(legacyRef)
		idRef = strings.TrimSpace(idRef)
		modelRef := idRef
		if modelRef == "" {
			modelRef = legacyRef
		}
		if modelRef == "" {
			modelUpdates[legacyField] = ""
			modelUpdates[idField] = nil
			continue
		}

		info, err := factory.ResolveInfo(ctx, ModelAccess{UserID: userID, TenantID: tenantID}, modelType, modelRef)
		if err != nil {
			switch {
			case errors.Is(err, permission.ErrPermissionDenied), errors.Is(err, permission.ErrResourceNotFound), errors.Is(err, permission.ErrMembershipNotFound):
				return common.CodeForbidden, err
			case errors.Is(err, errModelConfigUnavailable):
				return common.CodeArgumentError, err
			default:
				return common.CodeExceptionError, err
			}
		}
		if info == nil {
			return common.CodeArgumentError, fmt.Errorf("model %q is unavailable", modelRef)
		}
		if info.InstanceName != "" {
			modelUpdates[legacyField] = fmt.Sprintf("%s@%s@%s", info.Name, info.InstanceName, info.ProviderName)
		} else if info.ProviderName != "" {
			modelUpdates[legacyField] = fmt.Sprintf("%s@%s", info.Name, info.ProviderName)
		} else {
			modelUpdates[legacyField] = info.Name
		}
		var resolvedModelID interface{}
		if info.ID != "" {
			resolvedModelID = info.ID
		}
		modelUpdates[idField] = resolvedModelID
	}
	if len(modelUpdates) == 0 {
		return common.CodeSuccess, nil
	}
	if err := dao.DB.WithContext(ctx).Model(&entity.UserTenant{}).Where("id = ?", membership.ID).Updates(modelUpdates).Error; err != nil {
		return common.CodeExceptionError, err
	}
	return common.CodeSuccess, nil
}

func modelSettingField(key string) (string, string, bool) {
	for _, modelType := range defaultModelTypeNames {
		legacyField, idField, _, err := defaultModelFields(modelType)
		if err != nil {
			continue
		}
		if key == legacyField || key == idField {
			nullable := key == idField || legacyField == "tts_id" || legacyField == "ocr_id" || legacyField == "decision_id"
			return legacyField, idField, nullable
		}
	}
	return "", "", false
}

func modelSettingUpdates(raw map[string]interface{}) (map[string]interface{}, error) {
	updates := make(map[string]interface{}, len(raw))
	for key, value := range raw {
		if key == "tenant_id" {
			if _, ok := value.(string); !ok {
				return nil, fmt.Errorf("tenant_id must be a string")
			}
			continue
		}
		legacyField, _, nullable := modelSettingField(key)
		if legacyField == "" {
			return nil, fmt.Errorf("unsupported model setting %q", key)
		}
		if value == nil {
			if !nullable {
				return nil, fmt.Errorf("%s cannot be null", key)
			}
			updates[key] = nil
			continue
		}
		if _, ok := value.(string); !ok {
			return nil, fmt.Errorf("%s must be a string", key)
		}
		updates[key] = value
	}
	return updates, nil
}

// ModelItem describes one configured default model.
type ModelItem struct {
	ModelProvider *string `json:"model_provider"`
	ModelInstance *string `json:"model_instance"`
	ModelName     *string `json:"model_name"`
	ModelID       string  `json:"model_id"`
	ModelType     string  `json:"model_type"`
	Enable        bool    `json:"enable"`
}

func defaultModelFields(modelType string) (string, string, entity.ModelType, error) {
	switch modelType {
	case "chat":
		return "llm_id", "tenant_llm_id", entity.ModelTypeChat, nil
	case "embedding":
		return "embd_id", "tenant_embd_id", entity.ModelTypeEmbedding, nil
	case "rerank":
		return "rerank_id", "tenant_rerank_id", entity.ModelTypeRerank, nil
	case "asr", "speech2text":
		return "asr_id", "tenant_asr_id", entity.ModelTypeSpeech2Text, nil
	case "vision", "image2text":
		return "img2txt_id", "tenant_img2txt_id", entity.ModelTypeImage2Text, nil
	case "tts":
		return "tts_id", "tenant_tts_id", entity.ModelTypeTTS, nil
	case "ocr":
		return "ocr_id", "tenant_ocr_id", entity.ModelTypeOCR, nil
	case "decision":
		return "decision_id", "tenant_decision_id", entity.ModelTypeDecision, nil
	default:
		return "", "", 0, fmt.Errorf("model type %s is invalid", modelType)
	}
}

func ptrStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// ListDefaultModels returns the models configured on the user's current tenant membership.
func (s *UserTenantService) ListDefaultModels(ctx context.Context, userID string) ([]ModelItem, error) {
	tenantID, err := requireCurrentTenantID(ctx, userID)
	if err != nil {
		return nil, err
	}
	membership, err := s.userTenantDAO.FilterByUserIDAndTenantID(ctx, dao.DB, userID, tenantID)
	if err != nil {
		return nil, err
	}
	type defaultModel struct {
		modelType string
		modelName string
		modelID   *string
	}
	models := []defaultModel{
		{modelType: "chat", modelName: membership.LLMID, modelID: membership.TenantLLMID},
		{modelType: "embedding", modelName: membership.EmbdID, modelID: membership.TenantEmbdID},
		{modelType: "rerank", modelName: membership.RerankID, modelID: membership.TenantRerankID},
		{modelType: "asr", modelName: membership.ASRID, modelID: membership.TenantASRID},
		{modelType: "vision", modelName: membership.Img2TxtID, modelID: membership.TenantImg2TxtID},
		{modelType: "ocr", modelName: ptrStringValue(membership.OCRID), modelID: membership.TenantOCRID},
		{modelType: "tts", modelName: ptrStringValue(membership.TTSID), modelID: membership.TenantTTSID},
		{modelType: "decision", modelName: ptrStringValue(membership.DecisionID), modelID: membership.TenantDecisionID},
	}
	factory := NewModelFactory()
	result := make([]ModelItem, 0, len(models))
	for _, model := range models {
		modelRef := ptrStringValue(model.modelID)
		if modelRef == "" {
			modelRef = model.modelName
		}
		if strings.TrimSpace(modelRef) == "" {
			continue
		}
		_, _, modelType, err := defaultModelFields(model.modelType)
		if err != nil {
			continue
		}
		info, err := factory.ResolveInfo(ctx, ModelAccess{UserID: userID, TenantID: tenantID}, modelType, modelRef)
		if err != nil || info == nil {
			continue
		}
		modelID := info.ID
		if modelID == "" {
			modelID = ptrStringValue(model.modelID)
		}
		result = append(result, ModelItem{
			ModelProvider: &info.ProviderName,
			ModelInstance: &info.InstanceName,
			ModelName:     &info.Name,
			ModelID:       modelID,
			ModelType:     model.modelType,
			Enable:        true,
		})
	}
	return result, nil
}
