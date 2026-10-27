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
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	permissionresponse "ragflow/internal/permission/response"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TenantService tenant service
type TenantService struct {
	tenantDAO        *dao.TenantDAO
	userTenantDAO    *dao.UserTenantDAO
	userDAO          *dao.UserDAO
	modelProviderDAO *dao.TenantModelProviderDAO
	modelInstanceDAO *dao.TenantModelInstanceDAO
	modelDAO         *dao.TenantModelDAO
	kbDAO            *dao.KnowledgebaseDAO
	docEngine        engine.DocEngine
}

type tenantInviteRole struct {
	value    string
	roleID   *int32
	roleName string
}

// tenantMemberPermission keeps community membership policy in shared code and
// lets Enterprise apply its Tenant permission grants through the EE hook.
func tenantMemberPermission(ctx context.Context, db *gorm.DB, userID, tenantID string, required int32) (*entity.UserTenant, int32, error) {
	membership, mask, handled, err := tenantMemberPermissionEE(ctx, db, userID, tenantID, required)
	if err != nil || handled {
		return membership, mask, err
	}

	requirement := permission.TenantMember
	if required != 0 {
		requirement = permission.TenantOwner
	}
	checker := permission.NewDatabaseChecker(db)
	if err := checker.CheckTenant(ctx, permission.Subject{UserID: userID}, tenantID, requirement); err != nil {
		return nil, 0, err
	}
	var communityMembership entity.UserTenant
	if err := db.WithContext(ctx).
		Where("user_id = ? AND tenant_id = ? AND status = ?", userID, tenantID, string(entity.StatusValid)).
		Take(&communityMembership).Error; err != nil {
		return nil, 0, err
	}
	return &communityMembership, 0, nil
}

func tenantRoleErrorCode(err error) common.ErrorCode {
	var coded *common.CodedError
	if errors.As(err, &coded) {
		return coded.Code
	}
	code, _ := permissionresponse.Normalize(err)
	return code
}

func recordTenantPermissionChange(tx *gorm.DB, tenantID, actorUserID, eventType, targetType, targetID, resourceType, resourceID, actionType string, beforePermission, afterPermission int32, beforeData, afterData interface{}) error {
	return recordTenantPermissionChangeEE(tx, tenantID, actorUserID, eventType, targetType, targetID, resourceType, resourceID, actionType, beforePermission, afterPermission, beforeData, afterData)
}

func resolveTenantInviteRole(tx *gorm.DB, tenantID string) (tenantInviteRole, error) {
	role, handled, err := resolveTenantInviteRoleEE(tx, tenantID)
	if err != nil || handled {
		return role, err
	}
	return tenantInviteRole{value: TenantRoleNormal}, nil
}

func (s *TenantService) checkTenantInviteEnabled(ctx context.Context, tenantID string) (common.ErrorCode, error) {
	code, handled, err := s.checkTenantInviteEnabledEE(ctx, tenantID)
	if err != nil || handled {
		return code, err
	}
	return common.CodeSuccess, nil
}

// NewTenantService create tenant service
func NewTenantService() *TenantService {
	return &TenantService{
		tenantDAO:        dao.NewTenantDAO(),
		userTenantDAO:    dao.NewUserTenantDAO(),
		userDAO:          dao.NewUserDAO(),
		modelProviderDAO: dao.NewTenantModelProviderDAO(),
		modelInstanceDAO: dao.NewTenantModelInstanceDAO(),
		modelDAO:         dao.NewTenantModelDAO(),
		kbDAO:            dao.NewKnowledgebaseDAO(),
		docEngine:        engine.Get(),
	}
}

// TenantInfoResponse tenant information response
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

// GetTenantInfo returns the user's own tenant and its owner membership settings.
func (s *TenantService) GetTenantInfo(ctx context.Context, userID string) (*TenantInfoResponse, error) {
	membership, tenantID, err := s.currentUserTenant(ctx, userID)
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

// TenantListItem tenant list item response
type TenantListItem struct {
	TenantID     string  `json:"tenant_id"`
	Role         string  `json:"role"`
	Nickname     string  `json:"nickname"`
	Email        string  `json:"email"`
	Avatar       string  `json:"avatar"`
	UpdateDate   string  `json:"update_date"`
	DeltaSeconds float64 `json:"delta_seconds"`
}

// TenantLLMService tenant LLM service
// This service handles operations related to tenant-specific LLM configurations
type TenantLLMService struct {
	modelProviderDAO *dao.TenantModelProviderDAO
	modelInstanceDAO *dao.TenantModelInstanceDAO
	modelDAO         *dao.TenantModelDAO
}

// NewTenantLLMService creates a new TenantLLMService instance
func NewTenantLLMService() *TenantLLMService {
	return &TenantLLMService{
		modelProviderDAO: dao.NewTenantModelProviderDAO(),
		modelInstanceDAO: dao.NewTenantModelInstanceDAO(),
		modelDAO:         dao.NewTenantModelDAO(),
	}
}

// GetAPIKeyFromInstance returns the API key for the given composite model name
// by looking it up in the tenant_model_instance table. compositeModelName is in
// "model@instance@provider" or "model@provider" format.
func (s *TenantLLMService) GetAPIKeyFromInstance(ctx context.Context, tenantID, compositeModelName string) (string, error) {
	parts := strings.Split(compositeModelName, "@")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid model name format: %s", compositeModelName)
	}

	var providerName, instanceName string
	switch len(parts) {
	case 2:
		instanceName = "default"
		providerName = parts[1]
	case 3:
		instanceName = parts[1]
		providerName = parts[2]
	default:
		return "", fmt.Errorf("invalid model name format: %s", compositeModelName)
	}

	provider, err := s.modelProviderDAO.GetByTenantIDAndProviderName(ctx, dao.DB, tenantID, providerName)
	if err != nil {
		return "", fmt.Errorf("provider %q not found: %w", providerName, err)
	}
	if provider == nil {
		return "", fmt.Errorf("provider %q not found", providerName)
	}

	instance, err := s.modelInstanceDAO.GetByProviderIDAndInstanceName(ctx, dao.DB, provider.ID, instanceName)
	if err != nil {
		return "", fmt.Errorf("instance %q not found: %w", instanceName, err)
	}
	if instance == nil {
		return "", fmt.Errorf("instance %q not found", instanceName)
	}

	if instance.APIKey == "" {
		return "", fmt.Errorf("no API key configured for model %s", compositeModelName)
	}
	return instance.APIKey, nil
}

// GetTenantList get tenant list for a user
func (s *TenantService) GetTenantList(ctx context.Context, userID string) ([]*TenantListItem, error) {
	tenants, err := s.userTenantDAO.GetTenantsByUserID(ctx, dao.DB, userID)
	if err != nil {
		return nil, err
	}

	result := make([]*TenantListItem, len(tenants))

	for i, t := range tenants {
		// Parse update_date and calculate delta_seconds
		var deltaSeconds float64
		if t.UpdateDate != "" {
			deltaSeconds, err = common.DeltaSeconds(t.UpdateDate)
			if err != nil {
				return nil, err
			}
		}

		result[i] = &TenantListItem{
			TenantID:     t.TenantID,
			Role:         t.Role,
			Nickname:     t.Nickname,
			Email:        t.Email,
			Avatar:       t.Avatar,
			UpdateDate:   t.UpdateDate,
			DeltaSeconds: deltaSeconds,
		}
	}

	return result, nil
}

// CreateMetadataStore creates the metadata store for a tenant
func (s *TenantService) CreateMetadataStore(ctx context.Context, tenantID string) (common.ErrorCode, error) {
	// Call document engine to create doc meta table
	err := s.docEngine.CreateMetadataStore(ctx, tenantID)
	if err != nil {
		return common.CodeServerError, fmt.Errorf("failed to create metadata table: %w", err)
	}

	return common.CodeSuccess, nil
}

// DeleteMetadataStore deletes the metadata store for a tenant
func (s *TenantService) DeleteMetadataStore(ctx context.Context, tenantID string) (common.ErrorCode, error) {
	// Call document engine to delete doc meta table
	err := s.docEngine.DropMetadataStore(ctx, tenantID)
	if err != nil {
		return common.CodeServerError, fmt.Errorf("failed to delete doc meta table: %w", err)
	}

	return common.CodeSuccess, nil
}

// CreateDatasetTableRequest represents the request for creating a dataset table
type CreateDatasetTableRequest struct {
	KBID       string `json:"kb_id" binding:"required"`
	VectorSize int    `json:"vector_size" binding:"required"`
	ParserID   string `json:"parser_id,omitempty"`
}

// CreateChunkStoreResponse represents the response for creating a chunk store
type CreateChunkStoreResponse struct {
	KBID       string `json:"kb_id"`
	TableName  string `json:"table_name"`
	VectorSize int    `json:"vector_size"`
}

// CreateChunkStore creates a chunk store in the document engine for a knowledge base
func (s *TenantService) CreateChunkStore(ctx context.Context, req *CreateDatasetTableRequest) (*CreateChunkStoreResponse, common.ErrorCode, error) {
	if req == nil {
		return nil, common.CodeDataError, fmt.Errorf("request is required")
	}
	// Get KB to find tenant_id for building table name
	kb, err := s.kbDAO.GetByID(ctx, dao.DB, req.KBID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, fmt.Errorf("knowledge base not found: %s", req.KBID)
		}
		return nil, common.CodeServerError, fmt.Errorf("failed to query knowledge base %s: %w", req.KBID, err)
	}

	// vector_size is required
	vecSize := req.VectorSize
	if vecSize <= 0 {
		return nil, common.CodeDataError, fmt.Errorf("vector_size must be positive")
	}

	// Build table name prefix: ragflow_<tenant_id>
	tableName := fmt.Sprintf("ragflow_%s", kb.TenantID)

	// The dataset language picks the fulltext analyzer on engines that fix it at
	// index creation, so it has to travel with the table, not the query.
	language := ""
	if kb.Language != nil {
		language = *kb.Language
	}

	// Call document engine to create table
	// Full table name will be built as "{tableName}_{kb_id}"
	err = s.docEngine.CreateChunkStore(ctx, tableName, req.KBID, vecSize, req.ParserID, language)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to create dataset: %w", err)
	}

	return &CreateChunkStoreResponse{
		KBID:       req.KBID,
		TableName:  tableName,
		VectorSize: vecSize,
	}, common.CodeSuccess, nil
}

// DeleteChunkStore deletes the chunk store in the document engine for a knowledge base
func (s *TenantService) DeleteChunkStore(ctx context.Context, kbID string) (common.ErrorCode, error) {
	// Get KB to find tenant_id for building table name
	kb, err := s.kbDAO.GetByID(ctx, dao.DB, kbID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return common.CodeDataError, fmt.Errorf("knowledge base not found: %s", kbID)
		}
		return common.CodeServerError, fmt.Errorf("failed to query knowledge base %s: %w", kbID, err)
	}

	// Call document engine to delete table
	err = s.docEngine.DropChunkStore(ctx, fmt.Sprintf("ragflow_%s", kb.TenantID), kbID)
	if err != nil {
		return common.CodeServerError, fmt.Errorf("failed to delete table: %w", err)
	}

	return common.CodeSuccess, nil
}

type ModelItem struct {
	ModelProvider *string `json:"model_provider"`
	ModelInstance *string `json:"model_instance"`
	ModelName     *string `json:"model_name"`
	ModelID       string  `json:"model_id"`
	ModelType     string  `json:"model_type"`
	Enable        bool    `json:"enable"`
}

func tenantDefaultModelFields(modelType string) (string, string, entity.ModelType, error) {
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
		// The decision model is its own type rather than a chat model: it is
		// spoken to in the System One protocol (one request, several questions)
		// and is chosen independently of the answering model.
		return "decision_id", "tenant_decision_id", entity.ModelTypeDecision, nil
	default:
		return "", "", 0, fmt.Errorf("model type %s is invalid", modelType)
	}
}

func ptrStringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// GetDefaultModelName returns the selected model reference for a tenant. Request
// contexts use that user's membership; trusted background work uses the owner.
func (s *TenantService) GetDefaultModelName(ctx context.Context, tenantID string, modelType entity.ModelType) (string, error) {
	return s.GetDefaultModelNameForTenant(ctx, tenantID, modelType)
}

// GetDefaultModelNameForUser returns the user's setting in the current Tenant.
func (s *TenantService) GetDefaultModelNameForUser(ctx context.Context, userID string, modelType entity.ModelType) (string, error) {
	membership, _, err := s.currentUserTenant(ctx, userID)
	if err != nil {
		return "", err
	}
	if !isDefaultModelType(modelType) {
		return "", fmt.Errorf("invalid model type: %s", modelType)
	}
	modelName, _ := defaultModelRefs(membership, modelType)
	return modelName, nil
}

// GetDefaultModelNameForTenant resolves the request user's membership setting
// when present and falls back to the tenant owner for background work.
func (s *TenantService) GetDefaultModelNameForTenant(ctx context.Context, tenantID string, modelType entity.ModelType) (string, error) {
	if !isDefaultModelType(modelType) {
		return "", fmt.Errorf("invalid model type: %s", modelType)
	}
	membership, err := s.defaultModelMembershipForTenant(ctx, tenantID)
	if err != nil {
		return "", err
	}
	modelName, _ := defaultModelRefs(membership, modelType)
	return modelName, nil
}

// GetDefaultModelRefsForUser returns the model reference and resolved tenant
// model ID configured by userID in tenantID.
func (s *TenantService) GetDefaultModelRefsForUser(ctx context.Context, userID, tenantID string, modelType entity.ModelType) (string, string, error) {
	if !isDefaultModelType(modelType) {
		return "", "", fmt.Errorf("invalid model type: %s", modelType)
	}
	membership, err := s.defaultModelMembership(ctx, userID, tenantID)
	if err != nil {
		return "", "", err
	}
	modelName, modelID := defaultModelRefs(membership, modelType)
	return modelName, modelID, nil
}

// GetDefaultModelRefsForTenant resolves model settings for the request user or,
// without a request identity, the tenant owner.
func (s *TenantService) GetDefaultModelRefsForTenant(ctx context.Context, tenantID string, modelType entity.ModelType) (string, string, error) {
	if !isDefaultModelType(modelType) {
		return "", "", fmt.Errorf("invalid model type: %s", modelType)
	}
	membership, err := s.defaultModelMembershipForTenant(ctx, tenantID)
	if err != nil {
		return "", "", err
	}
	modelName, modelID := defaultModelRefs(membership, modelType)
	return modelName, modelID, nil
}

func isDefaultModelType(modelType entity.ModelType) bool {
	switch modelType {
	case entity.ModelTypeChat, entity.ModelTypeEmbedding, entity.ModelTypeRerank,
		entity.ModelTypeSpeech2Text, entity.ModelTypeImage2Text, entity.ModelTypeTTS,
		entity.ModelTypeOCR, entity.ModelTypeDecision:
		return true
	default:
		return false
	}
}

func (s *TenantService) defaultModelMembership(ctx context.Context, userID, tenantID string) (*entity.UserTenant, error) {
	if strings.TrimSpace(userID) == "" {
		return s.userTenantDAO.GetOwnerByTenantID(ctx, dao.DB, tenantID)
	}
	membership, err := s.userTenantDAO.FilterByUserIDAndTenantID(ctx, dao.DB, userID, tenantID)
	if err == nil {
		return membership, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) || userID != tenantID {
		return nil, err
	}
	return s.userTenantDAO.GetOwnerByTenantID(ctx, dao.DB, tenantID)
}

func (s *TenantService) defaultModelMembershipForTenant(ctx context.Context, tenantID string) (*entity.UserTenant, error) {
	if subject, ok := permission.SubjectFromContext(ctx); ok && strings.TrimSpace(subject.UserID) != "" &&
		(strings.TrimSpace(subject.TenantID) == "" || strings.TrimSpace(subject.TenantID) == tenantID) {
		return s.defaultModelMembership(ctx, subject.UserID, tenantID)
	}
	return s.userTenantDAO.GetOwnerByTenantID(ctx, dao.DB, tenantID)
}

func (s *TenantService) GetModelInfo(ctx context.Context, tenantID string, defaultModel string, modelType string) (*string, *string, *string, bool, error) {
	// Mirror Python's _get_model_info: right-anchored rsplit so that model
	// names containing '@' (e.g. LM Studio IDs like
	// "text-embedding-nomic-embed-text-v1.5@q8_0") remain intact.
	// The composite key is: modelName@instanceName@providerName or
	// modelName@providerName.
	parts := rsplitN(defaultModel, "@", 2)
	var modelName, instanceName, providerName string
	switch len(parts) {
	case 3:
		modelName, instanceName, providerName = parts[0], parts[1], parts[2]
	case 2:
		modelName, providerName = parts[0], parts[1]
		instanceName = "default"
	default:
		modelName = parts[0]
		providerName = ""
		instanceName = "default"
	}

	// Special case: OCR with infiniflow@default@deepdoc is always enabled.
	if modelType == "ocr" && providerName == "infiniflow" && instanceName == "default" && modelName == "deepdoc" {
		return &providerName, &instanceName, &modelName, true, nil
	}

	// Special case: TEI Builtin embedding model.
	composeProfiles := common.GetEnv(common.EnvComposeProfiles)
	teiModel := common.GetEnv(common.EnvTEIModel)
	if modelType == "embedding" && strings.Contains(composeProfiles, "tei-") && teiModel != "" &&
		modelName == teiModel && (providerName == "" || providerName == "Builtin") {
		return &providerName, &instanceName, &modelName, true, nil
	}

	// Check if the provider exists for the tenant.
	modelProvider, err := s.modelProviderDAO.GetByTenantIDAndProviderName(ctx, dao.DB, tenantID, providerName)
	if err != nil {
		return nil, nil, nil, false, err
	}

	// Check if the instance exists.
	modelInstance, err := s.modelInstanceDAO.GetByProviderIDAndInstanceName(ctx, dao.DB, modelProvider.ID, instanceName)
	if err != nil {
		return nil, nil, nil, false, err
	}

	// Check if the model exists and is active. Model type validity is
	// enforced on the save path against the tenant_model record, so models
	// added online that are absent from the static catalog still resolve.
	modelEntity, err := s.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(ctx, dao.DB, modelProvider.ID, modelInstance.ID, modelName)
	if err != nil {
		if !dao.IsNotFoundErr(err) {
			return nil, nil, nil, false, err
		}
	}
	if modelEntity == nil {
		return nil, nil, nil, false, fmt.Errorf("model %s isn't available", modelName)
	}
	if modelEntity.Status != "active" {
		return nil, nil, nil, false, fmt.Errorf("model %s isn't available", modelName)
	}

	return &providerName, &instanceName, &modelName, true, nil
}

// rsplitN splits s by sep from the right, limiting to n+1 parts (mirrors
// Python's str.rsplit(sep, maxsplit)).
func rsplitN(s, sep string, n int) []string {
	if n <= 0 {
		return []string{s}
	}
	result := make([]string, 0, n+1)
	remaining := s
	for i := 0; i < n; i++ {
		idx := strings.LastIndex(remaining, sep)
		if idx < 0 {
			result = append(result, remaining)
			// Reverse the collected parts.
			for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
				result[left], result[right] = result[right], result[left]
			}
			return result
		}
		result = append(result, remaining[idx+len(sep):])
		remaining = remaining[:idx]
	}
	result = append(result, remaining)
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

func (s *TenantService) ListTenantDefaultModels(ctx context.Context, userID string) ([]ModelItem, error) {
	membership, tenantID, err := s.currentUserTenant(ctx, userID)
	if err != nil {
		return nil, err
	}
	return listDefaultModels(ctx, s, membership, tenantID), nil
}

func listDefaultModels(ctx context.Context, s *TenantService, membership *entity.UserTenant, tenantID string) []ModelItem {
	type defaultModel struct {
		modelType string
		modelID   string
		rowID     *string
	}
	models := []defaultModel{
		{modelType: "chat", modelID: membership.LLMID, rowID: membership.TenantLLMID},
		{modelType: "embedding", modelID: membership.EmbdID, rowID: membership.TenantEmbdID},
		{modelType: "rerank", modelID: membership.RerankID, rowID: membership.TenantRerankID},
		{modelType: "asr", modelID: membership.ASRID, rowID: membership.TenantASRID},
		{modelType: "vision", modelID: membership.Img2TxtID, rowID: membership.TenantImg2TxtID},
		{modelType: "ocr", modelID: ptrStringValue(membership.OCRID), rowID: membership.TenantOCRID},
		{modelType: "tts", modelID: ptrStringValue(membership.TTSID), rowID: membership.TenantTTSID},
		{modelType: "decision", modelID: ptrStringValue(membership.DecisionID), rowID: membership.TenantDecisionID},
	}
	result := make([]ModelItem, 0, len(models))
	for _, model := range models {
		if strings.TrimSpace(model.modelID) == "" {
			continue
		}
		provider, instance, name, enabled, err := s.GetModelInfo(ctx, tenantID, model.modelID, model.modelType)
		if err != nil {
			continue
		}
		result = append(result, ModelItem{
			ModelProvider: provider,
			ModelInstance: instance,
			ModelName:     name,
			ModelID:       ptrStringValue(model.rowID),
			ModelType:     model.modelType,
			Enable:        enabled,
		})
	}
	return result
}

func (s *TenantService) checkModelAvailable(ctx context.Context, tenantID, providerName, instanceName, modelName, modelType string) error {
	_, _, modelTypeBit, err := tenantDefaultModelFields(modelType)
	if err != nil {
		return err
	}

	// Static bypass: deepdoc is a built-in model that doesn't need DB checks (mirrors Python _check_model_available).
	if providerName == "infiniflow" && instanceName == "default" && modelName == "deepdoc" {
		return nil
	}

	// Static bypass: OCR with infiniflow@default@deepdoc is always enabled (mirrors Python _check_model_available).
	if modelType == "ocr" && providerName == "infiniflow" && instanceName == "default" {
		return nil
	}

	// Static bypass: TEI Builtin embedding model when COMPOSE_PROFILES includes tei- (mirrors Python _check_model_available).
	composeProfiles := common.GetEnv(common.EnvComposeProfiles)
	teiModel := common.GetEnv(common.EnvTEIModel)
	if modelType == "embedding" && strings.Contains(composeProfiles, "tei-") && teiModel != "" &&
		modelName == teiModel && (providerName == "" || providerName == "Builtin") {
		return nil
	}

	// Check if the provider and instance exists
	modelProvider, err := s.modelProviderDAO.GetByTenantIDAndProviderName(ctx, dao.DB, tenantID, providerName)
	if err != nil {
		return err
	}

	modelInstance, err := s.modelInstanceDAO.GetByProviderIDAndInstanceName(ctx, dao.DB, modelProvider.ID, instanceName)
	if err != nil {
		return err
	}

	// Validate model availability through the DB (TenantModel table)
	modelEntity, err := s.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(ctx, dao.DB, modelProvider.ID, modelInstance.ID, modelName)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return fmt.Errorf("model %s isn't available", modelName)
		}
		return err
	}
	if modelEntity.Status != "active" {
		return fmt.Errorf("model %s isn't available", modelName)
	}
	if !entity.ModelType(modelEntity.ModelType).Has(modelTypeBit) {
		return fmt.Errorf("model %s isn't a %s model", modelName, modelType)
	}

	return nil
}

func (s *TenantService) SetTenantDefaultModels(ctx context.Context, userID, modelProvider, modelInstance, modelName, modelType, modelID string) error {
	membership, tenantID, err := s.currentUserTenant(ctx, userID)
	if err != nil {
		return err
	}
	var defaultModel string
	modelTypeID, tenantModelTypeID, modelTypeBit, err := tenantDefaultModelFields(modelType)
	if err != nil {
		return err
	}

	var tenantModelID interface{}
	if modelID != "" {
		modelEntity, err := s.modelDAO.GetByID(ctx, dao.DB, modelID)
		if err != nil {
			return fmt.Errorf("model ID %s is invalid", modelID)
		}
		instanceEntity, err := s.modelInstanceDAO.GetByID(ctx, dao.DB, modelEntity.InstanceID)
		if err != nil {
			return fmt.Errorf("instance for model %s not found: %w", modelID, err)
		}
		providerEntity, err := s.modelProviderDAO.GetByID(ctx, dao.DB, instanceEntity.ProviderID)
		if err != nil {
			return fmt.Errorf("provider for model %s not found: %w", modelID, err)
		}
		if providerEntity.TenantID != tenantID {
			return fmt.Errorf("model %s does not belong to your tenant", modelID)
		}
		if modelEntity.Status != "active" {
			return fmt.Errorf("model %s isn't available", modelEntity.ModelName)
		}
		if !entity.ModelType(modelEntity.ModelType).Has(modelTypeBit) {
			return fmt.Errorf("model %s isn't a %s model", modelEntity.ModelName, modelType)
		}
		modelProvider = providerEntity.ProviderName
		modelInstance = instanceEntity.InstanceName
		modelName = modelEntity.ModelName
		tenantModelID = modelID
	}

	if modelProvider == "" && modelInstance == "" && modelName == "" {
		defaultModel = ""
		tenantModelID = nil
	} else if modelProvider != "" && modelInstance != "" && modelName != "" {
		if err := s.checkModelAvailable(ctx, tenantID, modelProvider, modelInstance, modelName, modelType); err != nil {
			return err
		}
		if modelID == "" {
			if modelProvider == "Builtin" {
				tenantModelID = nil
			} else {
				provider, err := s.modelProviderDAO.GetByTenantIDAndProviderName(ctx, dao.DB, tenantID, modelProvider)
				if err != nil {
					return err
				}
				instance, err := s.modelInstanceDAO.GetByProviderIDAndInstanceName(ctx, dao.DB, provider.ID, modelInstance)
				if err != nil {
					return err
				}
				model, err := s.modelDAO.GetModelByProviderIDAndInstanceIDAndModelName(ctx, dao.DB, provider.ID, instance.ID, modelName)
				if err != nil {
					return err
				}
				tenantModelID = model.ID
			}
		}
		defaultModel = fmt.Sprintf("%s@%s@%s", modelName, modelInstance, modelProvider)
	} else {
		return fmt.Errorf("model provider, instance and name must be specified together")
	}

	return dao.DB.WithContext(ctx).Model(&entity.UserTenant{}).Where("id = ?", membership.ID).Updates(map[string]interface{}{
		modelTypeID:       defaultModel,
		tenantModelTypeID: tenantModelID,
	}).Error
}

func (s *TenantService) currentUserTenant(ctx context.Context, userID string) (*entity.UserTenant, string, error) {
	membership, handled, err := resolveCurrentUserTenantEE(ctx, userID)
	if err != nil {
		return nil, "", err
	}
	if handled {
		return membership, membership.TenantID, nil
	}
	membership, err = s.userTenantDAO.GetOwnerByUserID(ctx, dao.DB, userID)
	if err != nil {
		return nil, "", err
	}
	return membership, membership.TenantID, nil
}

// Tenant member role constants.
const (
	TenantRoleOwner  = "owner"
	TenantRoleNormal = "normal"
	TenantRoleAdmin  = "admin"
)

// TenantMemberResponse is one entry in the member list response.
type TenantMemberResponse struct {
	ID              string  `json:"id"`
	UserID          string  `json:"user_id"`
	Role            string  `json:"role"`
	Status          string  `json:"status"`
	Nickname        string  `json:"nickname"`
	Email           string  `json:"email"`
	Avatar          string  `json:"avatar"`
	IsAuthenticated bool    `json:"is_authenticated"`
	IsActive        string  `json:"is_active"`
	IsAnonymous     bool    `json:"is_anonymous"`
	IsSuperuser     bool    `json:"is_superuser"`
	UpdateDate      string  `json:"update_date"`
	DeltaSeconds    float64 `json:"delta_seconds"`
}

// ListMembers returns all non-owner members of tenantID.
func (s *TenantService) ListMembers(ctx context.Context, userID, tenantID string) ([]*TenantMemberResponse, common.ErrorCode, error) {
	if _, _, err := tenantMemberPermission(ctx, dao.DB.WithContext(ctx), userID, tenantID, entity.TenantPermissionManageMembers); err != nil {
		return nil, tenantRoleErrorCode(err), err
	}
	rows, err := s.userTenantDAO.GetMembersByTenantID(ctx, dao.DB, tenantID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	result := make([]*TenantMemberResponse, 0, len(rows))
	for _, r := range rows {
		delta, _ := common.DeltaSeconds(r.UpdateDate)
		result = append(result, &TenantMemberResponse{
			ID:              r.ID,
			UserID:          r.UserID,
			Role:            r.Role,
			Status:          r.Status,
			Nickname:        r.Nickname,
			Email:           r.Email,
			Avatar:          r.Avatar,
			IsAuthenticated: r.IsAuthenticated,
			IsActive:        r.IsActive,
			IsAnonymous:     r.IsAnonymous,
			IsSuperuser:     r.IsSuperuser,
			UpdateDate:      r.UpdateDate,
			DeltaSeconds:    delta,
		})
	}
	return result, common.CodeSuccess, nil
}

// AddMemberRequest holds the invite payload.
type AddMemberRequest struct {
	Email string `json:"email"`
}

// AddMemberResponse holds the new member's public data.
type AddMemberResponse struct {
	ID       string `json:"id"`
	Avatar   string `json:"avatar"`
	Email    string `json:"email"`
	Nickname string `json:"nickname"`
}

// AddMember invites a user (by email) to the tenant.
func (s *TenantService) AddMember(ctx context.Context, userID, tenantID string, req *AddMemberRequest) (*AddMemberResponse, common.ErrorCode, error) {
	if _, _, err := tenantMemberPermission(ctx, dao.DB.WithContext(ctx), userID, tenantID, entity.TenantPermissionManageMembers); err != nil {
		return nil, tenantRoleErrorCode(err), err
	}
	if req == nil {
		return nil, common.CodeArgumentError, fmt.Errorf("request is required")
	}
	if strings.TrimSpace(req.Email) == "" {
		return nil, common.CodeArgumentError, fmt.Errorf("email is required")
	}
	if code, err := s.checkTenantInviteEnabled(ctx, tenantID); err != nil {
		return nil, code, err
	}

	invitee, err := s.userDAO.GetByEmail(ctx, dao.DB, req.Email)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, fmt.Errorf("user not found")
		}
		return nil, common.CodeDataError, err
	}
	var membership entity.UserTenant
	err = dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing entity.UserTenant
		loadErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND tenant_id = ?", invitee.ID, tenantID).Take(&existing).Error
		if loadErr != nil && !errors.Is(loadErr, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load existing Tenant membership: %w", loadErr)
		}
		if errors.Is(loadErr, gorm.ErrRecordNotFound) {
			status := entity.TenantMembershipStatusInvited
			membership = entity.UserTenant{ID: common.GenerateUUID(), UserID: invitee.ID, TenantID: tenantID, InvitedBy: userID, Status: &status}
			if err := tx.Create(&membership).Error; err != nil {
				return fmt.Errorf("failed to create invitation: %w", err)
			}
			return recordTenantPermissionChange(tx, tenantID, userID, "tenant.member.invited", "user", invitee.ID, "tenant_membership", membership.ID, "invite", 0, 0, nil,
				map[string]interface{}{"user_id": invitee.ID, "email": invitee.Email, "role": membership.Role, "invited_by": userID, "status": status})
		}
		oldStatus := tenantMembershipStatus(existing.Status)
		switch oldStatus {
		case entity.TenantMembershipStatusJoined:
			if existing.Role == TenantRoleOwner {
				return common.NewCodedError(common.CodeDataError, "user is already the tenant owner")
			}
			return common.NewCodedError(common.CodeDataError, "user is already a member")
		case entity.TenantMembershipStatusInvited:
			return common.NewCodedError(common.CodeDataError, "user already has a pending invitation")
		case entity.TenantMembershipStatusRequested, entity.TenantMembershipStatusRejected, entity.TenantMembershipStatusLeft, entity.TenantMembershipStatusRemoved:
		default:
			return common.NewCodedError(common.CodeConflict, "Tenant membership has an unsupported status")
		}
		before := map[string]interface{}{"user_id": invitee.ID, "role": existing.Role, "status": oldStatus, "invited_by": existing.InvitedBy}
		status := entity.TenantMembershipStatusInvited
		if err := tx.Model(&entity.UserTenant{}).Where("id = ?", existing.ID).Updates(map[string]interface{}{"role": "", "status": status, "invited_by": userID}).Error; err != nil {
			return fmt.Errorf("reinvite Tenant member: %w", err)
		}
		existing.Role, existing.InvitedBy, existing.Status = "", userID, &status
		membership = existing
		return recordTenantPermissionChange(tx, tenantID, userID, "tenant.member.invited", "user", invitee.ID, "tenant_membership", membership.ID, "invite", 0, 0, before,
			map[string]interface{}{"user_id": invitee.ID, "email": invitee.Email, "role": membership.Role, "invited_by": userID, "status": status})
	})
	if err != nil {
		return nil, tenantRoleErrorCode(err), err
	}

	return tenantMemberResponse(invitee), common.CodeSuccess, nil
}

// RemoveMember removes a user from the tenant. The owner may remove members,
// and a member may remove themselves. An invitee may decline their own pending
// invitation. The tenant owner cannot be removed.
func (s *TenantService) RemoveMember(ctx context.Context, userID, tenantID, targetUserID string) (common.ErrorCode, error) {
	if s.userTenantDAO == nil {
		return common.CodeServerError, fmt.Errorf("userTenantDAO not initialized")
	}
	if userID != targetUserID {
		if _, _, err := tenantMemberPermission(ctx, dao.DB.WithContext(ctx), userID, tenantID, entity.TenantPermissionManageMembers); err != nil {
			return tenantRoleErrorCode(err), err
		}
	}
	err := dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var membership entity.UserTenant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND tenant_id = ?", targetUserID, tenantID).Take(&membership).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return common.NewCodedError(common.CodeDataError, "user has no Tenant membership or invitation")
			}
			return fmt.Errorf("load Tenant membership: %w", err)
		}
		oldStatus := tenantMembershipStatus(membership.Status)
		if oldStatus == entity.TenantMembershipStatusRemoved || oldStatus == entity.TenantMembershipStatusRejected || oldStatus == entity.TenantMembershipStatusLeft {
			return nil
		}
		if membership.Role == TenantRoleOwner {
			return common.NewCodedError(common.CodeArgumentError, "cannot remove or leave the tenant owner")
		}

		newStatus := entity.TenantMembershipStatusRemoved
		eventType, actionType := "tenant.member.removed", "remove"
		if userID == targetUserID {
			switch oldStatus {
			case entity.TenantMembershipStatusInvited:
				newStatus = entity.TenantMembershipStatusRejected
				eventType, actionType = "tenant.invitation.declined", "reject"
			case entity.TenantMembershipStatusJoined:
				newStatus = entity.TenantMembershipStatusLeft
				eventType, actionType = "tenant.member.left", "leave"
			default:
				return common.NewCodedError(common.CodeDataError, "user has no active Tenant membership or invitation")
			}
		} else if oldStatus == entity.TenantMembershipStatusInvited {
			eventType, actionType = "tenant.invitation.revoked", "revoke"
		}

		before := map[string]interface{}{
			"user_id": membership.UserID, "role": membership.Role,
			"invited_by": membership.InvitedBy, "status": oldStatus,
		}
		if err := tx.Model(&entity.UserTenant{}).Where("id = ?", membership.ID).
			Updates(map[string]interface{}{"role": "", "status": newStatus}).Error; err != nil {
			return fmt.Errorf("update Tenant membership status: %w", err)
		}
		after := map[string]interface{}{
			"user_id": membership.UserID, "role": "",
			"invited_by": membership.InvitedBy, "status": newStatus,
		}
		return recordTenantPermissionChange(tx, tenantID, userID, eventType, "user", targetUserID, "tenant_membership", membership.ID, actionType, 0, 0, before, after)
	})
	if err != nil {
		return tenantRoleErrorCode(err), err
	}
	return common.CodeSuccess, nil
}

// AcceptInvite assigns the Tenant default role when a pending invitation is accepted.
func (s *TenantService) AcceptInvite(ctx context.Context, userID, tenantID string) (common.ErrorCode, error) {
	if s.userTenantDAO == nil {
		return common.CodeServerError, fmt.Errorf("userTenantDAO not initialized")
	}
	err := dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var membership entity.UserTenant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND tenant_id = ?", userID, tenantID).Take(&membership).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return common.NewCodedError(common.CodeDataError, "no pending invitation found")
			}
			return fmt.Errorf("load pending Tenant invitation: %w", err)
		}
		if tenantMembershipStatus(membership.Status) != entity.TenantMembershipStatusInvited {
			return common.NewCodedError(common.CodeArgumentError, "no pending invitation to accept")
		}
		role, err := resolveTenantInviteRole(tx, tenantID)
		if err != nil {
			return err
		}
		if strings.TrimSpace(role.value) == "" {
			return common.NewCodedError(common.CodeConflict, "Tenant default role is invalid")
		}
		status := entity.TenantMembershipStatusJoined
		if err := tx.Model(&entity.UserTenant{}).Where("id = ?", membership.ID).
			Updates(map[string]interface{}{"role": role.value, "status": status}).Error; err != nil {
			return fmt.Errorf("accept Tenant invitation: %w", err)
		}
		before := map[string]interface{}{"user_id": userID, "role": membership.Role, "status": tenantMembershipStatus(membership.Status)}
		after := map[string]interface{}{"user_id": userID, "role": role.value, "status": status}
		if role.roleID != nil {
			after["role_id"] = *role.roleID
		}
		if role.roleName != "" {
			after["role_name"] = role.roleName
		}
		return recordTenantPermissionChange(tx, tenantID, userID, "tenant.member.joined", "user", userID, "tenant_membership", membership.ID, "join", 0, 0, before, after)
	})
	if err != nil {
		return tenantRoleErrorCode(err), err
	}
	return common.CodeSuccess, nil
}

func tenantMembershipStatus(status *string) string {
	if status == nil {
		return ""
	}
	return *status
}

func tenantMemberResponse(user *entity.User) *AddMemberResponse {
	avatar := ""
	if user.Avatar != nil {
		avatar = *user.Avatar
	}
	return &AddMemberResponse{ID: user.ID, Avatar: avatar, Email: user.Email, Nickname: user.Nickname}
}
