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
	"ragflow/internal/entity"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
)

var (
	datasetAllowedChunkMethods = map[string]struct{}{
		"naive":        {},
		"book":         {},
		"email":        {},
		"laws":         {},
		"manual":       {},
		"one":          {},
		"paper":        {},
		"picture":      {},
		"presentation": {},
		"qa":           {},
		"resume":       {},
		"table":        {},
		"tag":          {},
	}
	datasetSupportedAvatarMIMETypes = map[string]struct{}{
		"image/jpeg": {},
		"image/png":  {},
	}
	datasetAllowedOrderByFields = map[string]struct{}{
		"create_time": {},
		"update_time": {},
	}
	datasetChunkMethodErrorMessage = "Input should be 'naive', 'book', 'email', 'laws', 'manual', 'one', 'paper', 'picture', 'presentation', 'qa', 'resume', 'table' or 'tag'"
)

// DatasetsService implements the RESTful dataset APIs from dataset_api.py.
type DatasetsService struct {
	kbDAO        *dao.KnowledgebaseDAO
	tenantDAO    *dao.TenantDAO
	tenantLLMDAO *dao.TenantLLMDAO
}

// NewDatasetsService creates a new datasets service.
func NewDatasetsService() *DatasetsService {
	return &DatasetsService{
		kbDAO:        dao.NewKnowledgebaseDAO(),
		tenantDAO:    dao.NewTenantDAO(),
		tenantLLMDAO: dao.NewTenantLLMDAO(),
	}
}

// AutoMetadataField mirrors the REST dataset auto metadata field schema.
type AutoMetadataField struct {
	Name           string      `json:"name"`
	Type           string      `json:"type"`
	Description    *string     `json:"description,omitempty"`
	Examples       interface{} `json:"examples,omitempty"`
	RestrictValues bool        `json:"restrict_values,omitempty"`
}

// AutoMetadataConfig mirrors the REST dataset auto metadata schema.
type AutoMetadataConfig struct {
	Enabled *bool               `json:"enabled,omitempty"`
	Fields  []AutoMetadataField `json:"fields,omitempty"`
}

// CreateDatasetRequest represents the request for creating a dataset.
type CreateDatasetRequest struct {
	Name               string                 `json:"name" binding:"required"`
	Avatar             *string                `json:"avatar,omitempty"`
	Description        *string                `json:"description,omitempty"`
	EmbeddingModel     *string                `json:"embedding_model,omitempty"`
	Permission         *string                `json:"permission,omitempty"`
	ChunkMethod        *string                `json:"chunk_method,omitempty"`
	ParseType          *int                   `json:"parse_type,omitempty"`
	PipelineID         *string                `json:"pipeline_id,omitempty"`
	ParserConfig       map[string]interface{} `json:"parser_config,omitempty"`
	AutoMetadataConfig *AutoMetadataConfig    `json:"auto_metadata_config,omitempty"`
	Ext                map[string]interface{} `json:"ext,omitempty"`
}

// ListDatasets lists datasets with pagination and filtering.
func (s *DatasetsService) ListDatasets(id, name string, page, pageSize int, orderby string, desc bool, keywords string, ownerIDs []string, parserID, userID string) ([]map[string]interface{}, int64, common.ErrorCode, error) {
	id = strings.TrimSpace(id)
	if id != "" {
		normalizedID, err := normalizeDatasetUUID1(id)
		if err != nil {
			return nil, 0, common.CodeDataError, err
		}
		id = normalizedID

		kbs, err := s.kbDAO.GetKBByIDAndUserID(id, userID)
		if err != nil {
			return nil, 0, common.CodeServerError, errors.New("Database operation failed")
		}
		if len(kbs) == 0 {
			return nil, 0, common.CodeDataError, fmt.Errorf("User '%s' lacks permission for dataset '%s'", userID, id)
		}
	}

	name = strings.TrimSpace(name)
	if name != "" {
		kbs, err := s.kbDAO.GetKBByNameAndUserID(name, userID)
		if err != nil {
			return nil, 0, common.CodeServerError, errors.New("Database operation failed")
		}
		if len(kbs) == 0 {
			return nil, 0, common.CodeDataError, fmt.Errorf("User '%s' lacks permission for dataset '%s'", userID, name)
		}
	}

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 30
	}

	orderby = strings.TrimSpace(orderby)
	if _, ok := datasetAllowedOrderByFields[orderby]; !ok {
		orderby = "create_time"
	}

	keywords = strings.TrimSpace(keywords)
	parserID = strings.TrimSpace(parserID)

	// Empty owner ids do not change the query, so only keep the meaningful ones.
	tenantIDs := make([]string, 0, len(ownerIDs))
	for _, ownerID := range ownerIDs {
		ownerID = strings.TrimSpace(ownerID)
		if ownerID != "" {
			tenantIDs = append(tenantIDs, ownerID)
		}
	}
	if len(tenantIDs) == 0 {
		joinedTenants, err := s.tenantDAO.GetJoinedTenantsByUserID(userID)
		if err != nil {
			return nil, 0, common.CodeServerError, errors.New("Database operation failed")
		}
		for _, joinedTenant := range joinedTenants {
			if joinedTenant == nil || joinedTenant.TenantID == "" {
				continue
			}
			tenantIDs = append(tenantIDs, joinedTenant.TenantID)
		}
	}

	kbs, total, err := s.kbDAO.GetByTenantIDs(tenantIDs, userID, page, pageSize, orderby, desc, keywords, parserID)
	if err != nil {
		return nil, 0, common.CodeServerError, errors.New("Database operation failed")
	}

	data := make([]map[string]interface{}, 0, len(kbs))
	for _, kb := range kbs {
		if kb == nil {
			continue
		}
		data = append(data, datasetListItemToMap(kb))
	}

	return data, total, common.CodeSuccess, nil
}

// CreateDataset creates a new dataset.
func (s *DatasetsService) CreateDataset(req *CreateDatasetRequest, tenantID string) (map[string]interface{}, common.ErrorCode, error) {
	if !isValidString(req.Name) {
		return nil, common.CodeDataError, errors.New("Dataset name must be string.")
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, common.CodeDataError, errors.New("Dataset name can't be empty.")
	}
	if len(name) > entity.DatasetNameLimit {
		return nil, common.CodeDataError, fmt.Errorf("Dataset name length is %d which is large than %d", len(name), entity.DatasetNameLimit)
	}

	tenant, err := s.tenantDAO.GetByID(tenantID)
	if err != nil || tenant == nil {
		return nil, common.CodeDataError, errors.New("Tenant not found.")
	}

	parserID := ""
	permission := "me"
	embeddingModel := ""
	parserConfig := req.ParserConfig
	pipelineID := req.PipelineID
	description := req.Description
	avatar := req.Avatar
	var language *string

	if req.Description != nil && len(*req.Description) > 65535 {
		return nil, common.CodeDataError, errors.New("String should have at most 65535 characters")
	}
	if req.Avatar != nil {
		if len(*req.Avatar) > 65535 {
			return nil, common.CodeDataError, errors.New("String should have at most 65535 characters")
		}
		if err := validateDatasetAvatar(*req.Avatar); err != nil {
			return nil, common.CodeDataError, err
		}
	}
	if req.Permission != nil {
		permission = strings.TrimSpace(*req.Permission)
		if permission != "me" && permission != "team" {
			return nil, common.CodeDataError, errors.New("Input should be 'me' or 'team'")
		}
	}
	if req.ChunkMethod != nil {
		parserID = strings.TrimSpace(*req.ChunkMethod)
		if err := validateDatasetChunkMethod(parserID); err != nil {
			return nil, common.CodeDataError, err
		}
		pipelineID = nil
	}
	if req.ParseType != nil && (*req.ParseType < 0 || *req.ParseType > 64) {
		return nil, common.CodeDataError, fmt.Errorf("Input should be between 0 and 64")
	}
	if req.PipelineID != nil {
		normalizedPipelineID, err := normalizeDatasetPipelineID(*req.PipelineID)
		if err != nil {
			return nil, common.CodeDataError, err
		}
		pipelineID = normalizedPipelineID
	}
	if req.EmbeddingModel != nil {
		embeddingModel = strings.TrimSpace(*req.EmbeddingModel)
		if err := validateDatasetEmbeddingModel(embeddingModel); err != nil {
			return nil, common.CodeDataError, err
		}
	}
	if err := validateDatasetParserConfigSize(parserConfig); err != nil {
		return nil, common.CodeDataError, err
	}

	// ext mirrors the Python REST implementation and overrides known top-level fields.
	for key, value := range req.Ext {
		switch key {
		case "name":
			nameValue, ok := value.(string)
			if !ok {
				return nil, common.CodeDataError, errors.New("Dataset name must be string.")
			}
			nameValue = strings.TrimSpace(nameValue)
			if nameValue == "" {
				return nil, common.CodeDataError, errors.New("Dataset name can't be empty.")
			}
			if len(nameValue) > entity.DatasetNameLimit {
				return nil, common.CodeDataError, fmt.Errorf("Dataset name length is %d which is large than %d", len(nameValue), entity.DatasetNameLimit)
			}
			name = nameValue
		case "description":
			descriptionValue, ok := value.(string)
			if !ok {
				return nil, common.CodeDataError, errors.New("Description must be string.")
			}
			if len(descriptionValue) > 65535 {
				return nil, common.CodeDataError, errors.New("String should have at most 65535 characters")
			}
			description = &descriptionValue
		case "avatar":
			avatarValue, ok := value.(string)
			if !ok {
				return nil, common.CodeDataError, errors.New("Avatar must be string.")
			}
			if len(avatarValue) > 65535 {
				return nil, common.CodeDataError, errors.New("String should have at most 65535 characters")
			}
			if err := validateDatasetAvatar(avatarValue); err != nil {
				return nil, common.CodeDataError, err
			}
			avatar = &avatarValue
		case "language":
			languageValue, ok := value.(string)
			if !ok {
				return nil, common.CodeDataError, errors.New("Language must be string.")
			}
			languageValue = strings.TrimSpace(languageValue)
			language = &languageValue
		case "permission":
			permissionValue, ok := value.(string)
			if !ok {
				return nil, common.CodeDataError, errors.New("Permission must be string.")
			}
			permissionValue = strings.TrimSpace(permissionValue)
			if permissionValue != "me" && permissionValue != "team" {
				return nil, common.CodeDataError, errors.New("Input should be 'me' or 'team'")
			}
			permission = permissionValue
		case "embedding_model", "embd_id":
			embeddingModelValue, ok := value.(string)
			if !ok {
				return nil, common.CodeDataError, errors.New("Embedding model identifier must follow <model_name>@<provider> format")
			}
			embeddingModelValue = strings.TrimSpace(embeddingModelValue)
			if err := validateDatasetEmbeddingModel(embeddingModelValue); err != nil {
				return nil, common.CodeDataError, err
			}
			embeddingModel = embeddingModelValue
		case "chunk_method", "parser_id":
			parserIDValue, ok := value.(string)
			if !ok {
				return nil, common.CodeDataError, errors.New(datasetChunkMethodErrorMessage)
			}
			parserIDValue = strings.TrimSpace(parserIDValue)
			if err := validateDatasetChunkMethod(parserIDValue); err != nil {
				return nil, common.CodeDataError, err
			}
			parserID = parserIDValue
			pipelineID = nil
		case "pipeline_id":
			pipelineIDValue, ok := value.(string)
			if !ok {
				return nil, common.CodeDataError, errors.New("pipeline_id must be 32 hex characters")
			}
			normalizedPipelineID, err := normalizeDatasetPipelineID(pipelineIDValue)
			if err != nil {
				return nil, common.CodeDataError, err
			}
			pipelineID = normalizedPipelineID
		case "parser_config":
			parserConfigValue, ok := value.(map[string]interface{})
			if !ok {
				return nil, common.CodeDataError, errors.New("parser_config must be valid JSON")
			}
			if err := validateDatasetParserConfigSize(parserConfigValue); err != nil {
				return nil, common.CodeDataError, err
			}
			parserConfig = parserConfigValue
		}
	}

	// parser_id wins when it is present; otherwise parse_type and pipeline_id must arrive together.
	if parserID == "" {
		if req.ParseType == nil && pipelineID == nil {
			parserID = "naive"
		} else if req.ParseType == nil || pipelineID == nil {
			missingFields := make([]string, 0, 2)
			if req.ParseType == nil {
				missingFields = append(missingFields, "parse_type")
			}
			if pipelineID == nil {
				missingFields = append(missingFields, "pipeline_id")
			}
			return nil, common.CodeDataError, fmt.Errorf("parser_id omitted -> required fields missing: %s", strings.Join(missingFields, ", "))
		}
	}

	if req.AutoMetadataConfig != nil {
		parserConfig = applyAutoMetadataConfig(parserConfig, req.AutoMetadataConfig)
	}

	parserConfig = common.GetParserConfig(parserID, parserConfig)
	parserConfig["llm_id"] = tenant.LLMID

	embdID := tenant.EmbdID
	if embeddingModel != "" {
		ok, message := s.verifyEmbeddingAvailability(embeddingModel, tenantID)
		if !ok {
			return nil, common.CodeDataError, errors.New(message)
		}
		embdID = embeddingModel
	}

	kbID, err := generateUUID1Hex()
	if err != nil {
		return nil, common.CodeServerError, errors.New("Internal server error")
	}

	now := time.Now().Unix()
	nowDate := time.Now().Truncate(time.Second)
	status := string(entity.StatusValid)
	// Deduplicate name within tenant
	duplicateName, err := common.DuplicateName(func(n, tid string) bool {
		existing, err := s.kbDAO.GetByName(n, tid)
		return err == nil && existing != nil
	}, name, tenantID)
	if err != nil {
		return nil, common.CodeDataError, err
	}

	kb := &entity.Knowledgebase{
		ID:           kbID,
		Name:         duplicateName,
		TenantID:     tenantID,
		CreatedBy:    tenantID,
		ParserID:     parserID,
		PipelineID:   pipelineID,
		ParserConfig: parserConfig,
		Permission:   permission,
		EmbdID:       embdID,
		Status:       &status,
	}
	kb.CreateTime = &now
	kb.UpdateTime = &now
	kb.CreateDate = &nowDate
	kb.UpdateDate = &nowDate

	if description != nil {
		kb.Description = description
	}
	if avatar != nil {
		kb.Avatar = avatar
	}
	if language != nil {
		kb.Language = language
	}

	if err := s.kbDAO.Create(kb); err != nil {
		return nil, common.CodeServerError, errors.New("Failed to save dataset")
	}

	createdKB, err := s.kbDAO.GetByID(kbID)
	if err != nil || createdKB == nil {
		return nil, common.CodeServerError, errors.New("Dataset created failed")
	}

	return datasetToMap(createdKB), common.CodeSuccess, nil
}

// DeleteDatasets deletes multiple datasets.
func (s *DatasetsService) DeleteDatasets(ids []string, deleteAll bool, tenantID string) (map[string]interface{}, common.ErrorCode, error) {
	normalizedIDs := make([]string, 0, len(ids))
	seenIDs := make(map[string]struct{}, len(ids))

	// Canonicalize ids once so every downstream DAO call sees the same UUID1 hex format.
	for _, id := range ids {
		normalizedID, err := normalizeDatasetUUID1(strings.TrimSpace(id))
		if err != nil {
			return nil, common.CodeDataError, err
		}
		if _, exists := seenIDs[normalizedID]; exists {
			return nil, common.CodeDataError, fmt.Errorf("Duplicate ids: '%s'", normalizedID)
		}
		seenIDs[normalizedID] = struct{}{}
		normalizedIDs = append(normalizedIDs, normalizedID)
	}

	if len(normalizedIDs) == 0 {
		if !deleteAll {
			return map[string]interface{}{"success_count": 0}, common.CodeSuccess, nil
		}

		kbs, err := s.kbDAO.Query(map[string]interface{}{"tenant_id": tenantID})
		if err != nil {
			return nil, common.CodeServerError, errors.New("Database operation failed")
		}
		for _, kb := range kbs {
			normalizedIDs = append(normalizedIDs, kb.ID)
		}
	}

	kbs := make([]*entity.Knowledgebase, 0, len(normalizedIDs))
	unauthorizedIDs := make([]string, 0)
	for _, id := range normalizedIDs {
		kb, err := s.kbDAO.GetByIDAndTenantID(id, tenantID)
		if err != nil || kb == nil {
			unauthorizedIDs = append(unauthorizedIDs, id)
			continue
		}
		kbs = append(kbs, kb)
	}
	if len(unauthorizedIDs) > 0 {
		return nil, common.CodeDataError, fmt.Errorf("User '%s' lacks permission for datasets: '%s'", tenantID, strings.Join(unauthorizedIDs, ", "))
	}

	errorsList := make([]string, 0)
	successCount := 0
	for _, kb := range kbs {
		if err := s.deleteDataset(tenantID, kb); err != nil {
			errorsList = append(errorsList, err.Error())
			continue
		}
		successCount++
	}

	if len(errorsList) == 0 {
		return map[string]interface{}{"success_count": successCount}, common.CodeSuccess, nil
	}

	details := strings.Join(errorsList, "; ")
	if len(details) > 128 {
		details = details[:128]
	}
	errorMessage := fmt.Sprintf(
		"Successfully deleted %d datasets, %d failed. Details: %s...",
		successCount,
		len(errorsList),
		details,
	)
	if successCount == 0 {
		return nil, common.CodeDataError, errors.New(errorMessage)
	}

	return map[string]interface{}{
		"success_count": successCount,
		"errors":        limitStrings(errorsList, 5),
	}, common.CodeSuccess, nil
}

func (s *DatasetsService) deleteDataset(tenantID string, kb *entity.Knowledgebase) error {
	return dao.DB.Transaction(func(tx *gorm.DB) error {
		var documents []entity.Document
		if err := tx.Where("kb_id = ?", kb.ID).Find(&documents).Error; err != nil {
			return fmt.Errorf("Delete dataset error for %s", kb.ID)
		}

		docIDs := make([]string, 0, len(documents))
		for _, document := range documents {
			docIDs = append(docIDs, document.ID)
		}

		if len(docIDs) > 0 {
			var mappings []entity.File2Document
			if err := tx.Where("document_id IN ?", docIDs).Find(&mappings).Error; err != nil {
				return fmt.Errorf("Delete dataset error for %s", kb.ID)
			}

			fileIDs := make([]string, 0, len(mappings))
			seenFileIDs := make(map[string]struct{}, len(mappings))
			for _, mapping := range mappings {
				if mapping.FileID == nil || *mapping.FileID == "" {
					continue
				}
				if _, exists := seenFileIDs[*mapping.FileID]; exists {
					continue
				}
				seenFileIDs[*mapping.FileID] = struct{}{}
				fileIDs = append(fileIDs, *mapping.FileID)
			}

			if err := tx.Where("doc_id IN ?", docIDs).Delete(&entity.Task{}).Error; err != nil {
				return fmt.Errorf("Delete dataset error for %s", kb.ID)
			}
			if err := tx.Where("document_id IN ?", docIDs).Delete(&entity.File2Document{}).Error; err != nil {
				return fmt.Errorf("Delete dataset error for %s", kb.ID)
			}
			if len(fileIDs) > 0 {
				if err := tx.Unscoped().Where("id IN ?", fileIDs).Delete(&entity.File{}).Error; err != nil {
					return fmt.Errorf("Delete dataset error for %s", kb.ID)
				}
			}
			if err := tx.Where("id IN ?", docIDs).Delete(&entity.Document{}).Error; err != nil {
				return fmt.Errorf("Delete dataset error for %s", kb.ID)
			}
		}

		if err := tx.Unscoped().
			Where("source_type = ? AND type = ? AND name = ? AND tenant_id = ?", string(entity.FileSourceKnowledgebase), "folder", kb.Name, tenantID).
			Delete(&entity.File{}).Error; err != nil {
			return fmt.Errorf("Delete dataset error for %s", kb.ID)
		}

		if err := tx.Where("id = ?", kb.ID).Delete(&entity.Knowledgebase{}).Error; err != nil {
			return fmt.Errorf("Delete dataset error for %s", kb.ID)
		}

		return nil
	})
}

func validateDatasetChunkMethod(chunkMethod string) error {
	if _, ok := datasetAllowedChunkMethods[chunkMethod]; !ok {
		return errors.New(datasetChunkMethodErrorMessage)
	}
	return nil
}

func validateDatasetAvatar(avatar string) error {
	if !strings.Contains(avatar, ",") {
		return errors.New("Missing MIME prefix. Expected format: data:<mime>;base64,<data>")
	}

	prefix, _, _ := strings.Cut(avatar, ",")
	if !strings.HasPrefix(prefix, "data:") {
		return errors.New("Invalid MIME prefix format. Must start with 'data:'")
	}

	mimeType, _, _ := strings.Cut(strings.TrimPrefix(prefix, "data:"), ";")
	if _, ok := datasetSupportedAvatarMIMETypes[mimeType]; !ok {
		return errors.New("Unsupported MIME type. Allowed: [image/jpeg image/png]")
	}

	return nil
}

func validateDatasetEmbeddingModel(embeddingModel string) error {
	if embeddingModel == "" {
		return errors.New("Embedding model identifier must follow <model_name>@<provider> format")
	}

	modelName, provider, ok := strings.Cut(embeddingModel, "@")
	if !ok {
		return errors.New("Embedding model identifier must follow <model_name>@<provider> format")
	}
	if strings.TrimSpace(modelName) == "" || strings.TrimSpace(provider) == "" {
		return errors.New("Both model_name and provider must be non-empty strings")
	}

	return nil
}

func normalizeDatasetPipelineID(pipelineID string) (*string, error) {
	pipelineID = strings.TrimSpace(pipelineID)
	if pipelineID == "" {
		return nil, nil
	}
	if len(pipelineID) != 32 {
		return nil, errors.New("pipeline_id must be 32 hex characters")
	}
	for _, char := range pipelineID {
		if !strings.ContainsRune("0123456789abcdefABCDEF", char) {
			return nil, errors.New("pipeline_id must be hexadecimal")
		}
	}

	normalized := strings.ToLower(pipelineID)
	return &normalized, nil
}

func validateDatasetParserConfigSize(parserConfig map[string]interface{}) error {
	if len(parserConfig) == 0 {
		return nil
	}

	data, err := json.Marshal(parserConfig)
	if err != nil {
		return errors.New("parser_config must be valid JSON")
	}
	if len(data) > 65535 {
		return fmt.Errorf("Parser config exceeds size limit (max 65,535 characters). Current size: %d", len(data))
	}

	return nil
}

func normalizeDatasetUUID1(id string) (string, error) {
	parsedUUID, err := uuid.Parse(id)
	if err != nil {
		return "", errors.New("Invalid UUID1 format")
	}
	if parsedUUID.Version() != 1 {
		return "", errors.New("Must be a UUID1 format")
	}
	return strings.ReplaceAll(parsedUUID.String(), "-", ""), nil
}

func (s *DatasetsService) verifyEmbeddingAvailability(embdID string, tenantID string) (bool, string) {
	modelName, provider, err := parseModelName(embdID)
	if err != nil {
		return false, "Embedding model identifier must follow <model_name>@<provider> format"
	}

	if provider == "Builtin" {
		return true, ""
	}

	tenantLLMs, err := s.tenantLLMDAO.ListValidByTenant(tenantID)
	if err != nil {
		return false, "Database operation failed"
	}

	for _, tenantLLM := range tenantLLMs {
		if tenantLLM == nil || tenantLLM.LLMName == nil || tenantLLM.ModelType == nil {
			continue
		}
		if *tenantLLM.LLMName == modelName &&
			tenantLLM.LLMFactory == provider &&
			*tenantLLM.ModelType == string(entity.ModelTypeEmbedding) {
			return true, ""
		}
	}

	return false, fmt.Sprintf("Unauthorized model: <%s>", embdID)
}

func generateUUID1Hex() (string, error) {
	generatedUUID, err := uuid.NewUUID()
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(generatedUUID.String(), "-", ""), nil
}

func applyAutoMetadataConfig(parserConfig map[string]interface{}, config *AutoMetadataConfig) map[string]interface{} {
	if parserConfig == nil {
		parserConfig = make(map[string]interface{})
	}

	fields := make([]map[string]interface{}, 0, len(config.Fields))
	for _, field := range config.Fields {
		fields = append(fields, map[string]interface{}{
			"name":            field.Name,
			"type":            field.Type,
			"description":     field.Description,
			"examples":        field.Examples,
			"restrict_values": field.RestrictValues,
		})
	}
	parserConfig["metadata"] = fields
	enableMetadata := true
	if config.Enabled != nil {
		enableMetadata = *config.Enabled
	}
	parserConfig["enable_metadata"] = enableMetadata
	return parserConfig
}

func datasetListItemToMap(kb *entity.KnowledgebaseListItem) map[string]interface{} {
	item := map[string]interface{}{
		"id":              kb.ID,
		"name":            kb.Name,
		"tenant_id":       kb.TenantID,
		"permission":      kb.Permission,
		"document_count":  kb.DocNum,
		"token_num":       kb.TokenNum,
		"chunk_count":     kb.ChunkNum,
		"chunk_method":    kb.ParserID,
		"embedding_model": kb.EmbdID,
		"nickname":        kb.Nickname,
	}

	if kb.Avatar != nil {
		item["avatar"] = *kb.Avatar
	}
	if kb.Language != nil {
		item["language"] = *kb.Language
	}
	if kb.Description != nil {
		item["description"] = *kb.Description
	}
	if kb.TenantAvatar != nil {
		item["tenant_avatar"] = *kb.TenantAvatar
	}
	if kb.UpdateTime != nil {
		item["update_time"] = *kb.UpdateTime
	}

	return item
}

func datasetToMap(kb *entity.Knowledgebase) map[string]interface{} {
	item := map[string]interface{}{
		"id":                       kb.ID,
		"tenant_id":                kb.TenantID,
		"name":                     kb.Name,
		"embedding_model":          kb.EmbdID,
		"permission":               kb.Permission,
		"created_by":               kb.CreatedBy,
		"document_count":           kb.DocNum,
		"token_num":                kb.TokenNum,
		"chunk_count":              kb.ChunkNum,
		"similarity_threshold":     kb.SimilarityThreshold,
		"vector_similarity_weight": kb.VectorSimilarityWeight,
		"chunk_method":             kb.ParserID,
		"parser_config":            kb.ParserConfig,
		"pagerank":                 kb.Pagerank,
		"create_time":              kb.CreateTime,
	}

	if kb.Avatar != nil {
		item["avatar"] = *kb.Avatar
	}
	if kb.Language != nil {
		item["language"] = *kb.Language
	}
	if kb.Description != nil {
		item["description"] = *kb.Description
	}
	if kb.PipelineID != nil {
		item["pipeline_id"] = *kb.PipelineID
	}
	if kb.GraphragTaskID != nil {
		item["graphrag_task_id"] = *kb.GraphragTaskID
	}
	if kb.GraphragTaskFinishAt != nil {
		item["graphrag_task_finish_at"] = kb.GraphragTaskFinishAt.Format("2006-01-02 15:04:05")
	}
	if kb.RaptorTaskID != nil {
		item["raptor_task_id"] = *kb.RaptorTaskID
	}
	if kb.RaptorTaskFinishAt != nil {
		item["raptor_task_finish_at"] = kb.RaptorTaskFinishAt.Format("2006-01-02 15:04:05")
	}
	if kb.MindmapTaskID != nil {
		item["mindmap_task_id"] = *kb.MindmapTaskID
	}
	if kb.MindmapTaskFinishAt != nil {
		item["mindmap_task_finish_at"] = kb.MindmapTaskFinishAt.Format("2006-01-02 15:04:05")
	}
	if kb.UpdateTime != nil {
		item["update_time"] = *kb.UpdateTime
	}

	return item
}

func limitStrings(values []string, limit int) []string {
	if len(values) <= limit {
		return values
	}
	return values[:limit]
}
