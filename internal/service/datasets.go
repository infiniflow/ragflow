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
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/model"
	"ragflow/internal/utility"
)

var (
	datasetChunkMethods = map[string]struct{}{
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
	supportedAvatarMIMETypes = map[string]struct{}{
		"image/jpeg": {},
		"image/png":  {},
	}
	datasetOrderByFields = map[string]struct{}{
		"create_time": {},
		"update_time": {},
	}
	datasetResponseKeyAliases = map[string]string{
		"chunk_num": "chunk_count",
		"doc_num":   "document_count",
		"parser_id": "chunk_method",
		"embd_id":   "embedding_model",
	}
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

// CreateDatasetRequest mirrors POST /api/v1/datasets.
type CreateDatasetRequest struct {
	Name               string                 `json:"name"`
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

	ChunkMethodProvided bool `json:"-"`
}

// DeleteDatasetsRequest mirrors DELETE /api/v1/datasets.
type DeleteDatasetsRequest struct {
	IDs       *[]string `json:"ids"`
	DeleteAll bool      `json:"delete_all,omitempty"`
}

// ListDatasetsExt mirrors the supported ext query fields of GET /api/v1/datasets.
type ListDatasetsExt struct {
	Keywords string   `json:"keywords,omitempty"`
	OwnerIDs []string `json:"owner_ids,omitempty"`
	ParserID string   `json:"parser_id,omitempty"`
}

// ListDatasetsRequest mirrors GET /api/v1/datasets.
type ListDatasetsRequest struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`

	Page     int    `json:"page,omitempty"`
	PageSize int    `json:"page_size,omitempty"`
	OrderBy  string `json:"orderby,omitempty"`
	Desc     bool   `json:"desc,omitempty"`

	IncludeParsingStatus bool            `json:"include_parsing_status,omitempty"`
	Ext                  ListDatasetsExt `json:"ext,omitempty"`
}

// ListDatasetsResult contains the dataset list payload and total count.
type ListDatasetsResult struct {
	Data  []map[string]interface{}
	Total int64
}

// ValidateCreateDatasetRequest validates request-shape rules before service execution.
func (s *DatasetsService) ValidateCreateDatasetRequest(req *CreateDatasetRequest) error {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return fmt.Errorf("Field required: name")
	}
	if len(name) > model.DatasetNameLimit {
		return fmt.Errorf("String should have at most %d characters", model.DatasetNameLimit)
	}
	if req.Avatar != nil {
		if err := validateDatasetAvatar(*req.Avatar); err != nil {
			return err
		}
	}
	if req.EmbeddingModel != nil {
		trimmed := strings.TrimSpace(*req.EmbeddingModel)
		if err := validateEmbeddingModelIdentifier(trimmed); err != nil {
			return err
		}
		req.EmbeddingModel = &trimmed
	}
	if req.Permission != nil {
		if *req.Permission != "me" && *req.Permission != "team" {
			return fmt.Errorf("Input should be 'me' or 'team'")
		}
	}
	if req.ChunkMethodProvided {
		if req.ChunkMethod == nil {
			return fmt.Errorf("Input should be 'naive', 'book', 'email', 'laws', 'manual', 'one', 'paper', 'picture', 'presentation', 'qa', 'resume', 'table' or 'tag'")
		}
		chunkMethod := strings.TrimSpace(*req.ChunkMethod)
		if _, ok := datasetChunkMethods[chunkMethod]; !ok {
			return fmt.Errorf("Input should be 'naive', 'book', 'email', 'laws', 'manual', 'one', 'paper', 'picture', 'presentation', 'qa', 'resume', 'table' or 'tag'")
		}
		req.ChunkMethod = &chunkMethod
		req.ParseType = nil
		req.PipelineID = nil
	} else {
		if req.ParseType != nil {
			if *req.ParseType < 0 || *req.ParseType > 64 {
				return fmt.Errorf("Input should be between 0 and 64")
			}
		}
		if req.PipelineID != nil {
			normalized, err := normalizePipelineID(*req.PipelineID)
			if err != nil {
				return err
			}
			req.PipelineID = normalized
		}
		if req.ParseType == nil && req.PipelineID == nil {
			chunkMethod := "naive"
			req.ChunkMethod = &chunkMethod
		} else if req.ParseType == nil || req.PipelineID == nil {
			missingFields := make([]string, 0, 2)
			if req.ParseType == nil {
				missingFields = append(missingFields, "parse_type")
			}
			if req.PipelineID == nil {
				missingFields = append(missingFields, "pipeline_id")
			}
			return fmt.Errorf("parser_id omitted -> required fields missing: %s", strings.Join(missingFields, ", "))
		}
	}
	if len(req.ParserConfig) > 0 {
		data, err := json.Marshal(req.ParserConfig)
		if err != nil {
			return fmt.Errorf("parser_config must be valid JSON")
		}
		if len(data) > 65535 {
			return fmt.Errorf("Parser config exceeds size limit (max 65,535 characters). Current size: %d", len(data))
		}
	}
	return nil
}

// ValidateDeleteDatasetsRequest validates request-shape rules before service execution.
func (s *DatasetsService) ValidateDeleteDatasetsRequest(req *DeleteDatasetsRequest) error {
	if req.IDs == nil {
		return nil
	}

	seen := make(map[string]struct{}, len(*req.IDs))
	for idx, rawID := range *req.IDs {
		normalizedID, err := normalizeUUID1Hex(rawID)
		if err != nil {
			return err
		}
		if _, exists := seen[normalizedID]; exists {
			return fmt.Errorf("Duplicate ids: '%s'", normalizedID)
		}
		seen[normalizedID] = struct{}{}
		(*req.IDs)[idx] = normalizedID
	}
	return nil
}

// ValidateListDatasetsRequest validates query-shape rules before service execution.
func (s *DatasetsService) ValidateListDatasetsRequest(req *ListDatasetsRequest) error {
	req.Name = strings.TrimSpace(req.Name)

	if strings.TrimSpace(req.ID) != "" {
		normalizedID, err := normalizeUUID1Hex(req.ID)
		if err != nil {
			return err
		}
		req.ID = normalizedID
	}

	if req.Page < 1 {
		return fmt.Errorf("Input should be greater than or equal to 1")
	}
	if req.PageSize < 1 {
		return fmt.Errorf("Input should be greater than or equal to 1")
	}

	req.OrderBy = strings.TrimSpace(req.OrderBy)
	if _, ok := datasetOrderByFields[req.OrderBy]; !ok {
		return fmt.Errorf("Input should be 'create_time' or 'update_time'")
	}

	req.Ext.Keywords = strings.TrimSpace(req.Ext.Keywords)
	req.Ext.ParserID = strings.TrimSpace(req.Ext.ParserID)

	filteredOwnerIDs := make([]string, 0, len(req.Ext.OwnerIDs))
	for _, ownerID := range req.Ext.OwnerIDs {
		trimmedOwnerID := strings.TrimSpace(ownerID)
		if trimmedOwnerID == "" {
			continue
		}
		filteredOwnerIDs = append(filteredOwnerIDs, trimmedOwnerID)
	}
	req.Ext.OwnerIDs = filteredOwnerIDs

	return nil
}

// ListDatasets ports dataset_api_service.list_datasets.
func (s *DatasetsService) ListDatasets(userID string, req *ListDatasetsRequest) (*ListDatasetsResult, error) {
	if req.ID != "" {
		kbs, err := s.kbDAO.GetKBByIDAndUserID(req.ID, userID)
		if err != nil {
			return nil, fmt.Errorf("Database operation failed")
		}
		if len(kbs) == 0 {
			return nil, fmt.Errorf("User '%s' lacks permission for dataset '%s'", userID, req.ID)
		}
	}

	if req.Name != "" {
		kbs, err := s.kbDAO.GetKBByNameAndUserID(req.Name, userID)
		if err != nil {
			return nil, fmt.Errorf("Database operation failed")
		}
		if len(kbs) == 0 {
			return nil, fmt.Errorf("User '%s' lacks permission for dataset '%s'", userID, req.Name)
		}
	}

	tenantIDs := req.Ext.OwnerIDs
	if len(tenantIDs) == 0 {
		joinedTenants, err := s.tenantDAO.GetJoinedTenantsByUserID(userID)
		if err != nil {
			return nil, fmt.Errorf("Database operation failed")
		}

		tenantIDs = make([]string, 0, len(joinedTenants))
		for _, joinedTenant := range joinedTenants {
			if joinedTenant == nil || joinedTenant.TenantID == "" {
				continue
			}
			tenantIDs = append(tenantIDs, joinedTenant.TenantID)
		}
	}

	kbs, total, err := s.kbDAO.GetByTenantIDs(
		tenantIDs,
		userID,
		req.Page,
		req.PageSize,
		req.OrderBy,
		req.Desc,
		req.Ext.Keywords,
		req.Ext.ParserID,
	)
	if err != nil {
		return nil, fmt.Errorf("Database operation failed")
	}

	data := make([]map[string]interface{}, 0, len(kbs))
	for _, kb := range kbs {
		if kb == nil {
			continue
		}
		data = append(data, datasetListItemToMap(kb))
	}

	return &ListDatasetsResult{
		Data:  data,
		Total: total,
	}, nil
}

// CreateDataset ports dataset_api_service.create_dataset.
func (s *DatasetsService) CreateDataset(tenantID string, req *CreateDatasetRequest) (map[string]interface{}, error) {
	nameValue := interface{}(req.Name)
	description := req.Description
	avatar := req.Avatar
	language := (*string)(nil)
	permission := "me"
	if req.Permission != nil && *req.Permission != "" {
		permission = *req.Permission
	}
	embdID := ""
	if req.EmbeddingModel != nil {
		embdID = strings.TrimSpace(*req.EmbeddingModel)
	}
	parserID := "naive"
	if req.ChunkMethod != nil && *req.ChunkMethod != "" {
		parserID = *req.ChunkMethod
	}
	pipelineID := req.PipelineID
	parserConfig := utility.CloneMap(req.ParserConfig)
	if len(parserConfig) == 0 {
		parserConfig = nil
	}

	if req.AutoMetadataConfig != nil {
		parserConfig = applyAutoMetadataConfig(parserConfig, req.AutoMetadataConfig)
	}

	if len(req.Ext) > 0 {
		if value, ok := req.Ext["name"]; ok {
			nameValue = value
		}
		if value, ok := getStringFromMap(req.Ext, "description"); ok {
			description = &value
		}
		if value, ok := getStringFromMap(req.Ext, "avatar"); ok {
			avatar = &value
		}
		if value, ok := getStringFromMap(req.Ext, "language"); ok {
			language = &value
		}
		if value, ok := getStringFromMap(req.Ext, "permission"); ok {
			permission = value
		}
		if value, ok := getStringFromMap(req.Ext, "embedding_model"); ok {
			embdID = value
		}
		if value, ok := getStringFromMap(req.Ext, "embd_id"); ok {
			embdID = value
		}
		if value, ok := getStringFromMap(req.Ext, "chunk_method"); ok {
			parserID = value
		}
		if value, ok := getStringFromMap(req.Ext, "parser_id"); ok {
			parserID = value
		}
		if value, ok := getStringFromMap(req.Ext, "pipeline_id"); ok {
			pipelineID = &value
		}
		if value, ok := getMapFromMap(req.Ext, "parser_config"); ok {
			parserConfig = utility.CloneMap(value)
		}
	}

	name, ok := nameValue.(string)
	if !ok {
		return nil, fmt.Errorf("Dataset name must be string.")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("Dataset name can't be empty.")
	}
	if len(name) > model.DatasetNameLimit {
		return nil, fmt.Errorf("Dataset name length is %d which is large than %d", len(name), model.DatasetNameLimit)
	}

	tenant, err := s.tenantDAO.GetByID(tenantID)
	if err != nil {
		return nil, fmt.Errorf("Tenant not found")
	}

	if strings.TrimSpace(parserID) == "" {
		parserID = "naive"
	}
	parserConfig = utility.GetParserConfig(parserID, parserConfig)
	parserConfig["llm_id"] = tenant.LLMID

	if strings.TrimSpace(embdID) == "" {
		embdID = tenant.EmbdID
	} else {
		ok, message := s.verifyEmbeddingAvailability(strings.TrimSpace(embdID), tenantID)
		if !ok {
			return nil, fmt.Errorf("%s", message)
		}
	}

	kbID, err := generateUUID1Hex()
	if err != nil {
		return nil, fmt.Errorf("Internal server error")
	}

	now := time.Now().Unix()
	nowDate := time.Now().Truncate(time.Second)
	status := string(model.StatusValid)
	kb := &model.Knowledgebase{
		ID:           kbID,
		Name:         s.kbDAO.DuplicateName(name, tenantID),
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
		return nil, fmt.Errorf("Failed to save dataset")
	}

	createdKB, err := s.kbDAO.GetByID(kbID)
	if err != nil {
		return nil, fmt.Errorf("Dataset created failed")
	}

	return utility.RemapKeys(createdKB.ToMap(), datasetResponseKeyAliases), nil
}

// DeleteDatasets ports dataset_api_service.delete_datasets.
func (s *DatasetsService) DeleteDatasets(tenantID string, req *DeleteDatasetsRequest) (map[string]interface{}, error) {
	var ids []string
	if req.IDs != nil {
		ids = append(ids, (*req.IDs)...)
	}

	if len(ids) == 0 {
		if !req.DeleteAll {
			return map[string]interface{}{"success_count": 0}, nil
		}

		kbs, err := s.kbDAO.Query(map[string]interface{}{"tenant_id": tenantID})
		if err != nil {
			return nil, fmt.Errorf("Database operation failed")
		}
		ids = make([]string, 0, len(kbs))
		for _, kb := range kbs {
			ids = append(ids, kb.ID)
		}
	}

	kbs := make([]*model.Knowledgebase, 0, len(ids))
	unauthorizedIDs := make([]string, 0)
	for _, id := range ids {
		kb, err := s.kbDAO.GetByIDAndTenantID(id, tenantID)
		if err != nil || kb == nil {
			unauthorizedIDs = append(unauthorizedIDs, id)
			continue
		}
		kbs = append(kbs, kb)
	}
	if len(unauthorizedIDs) > 0 {
		return nil, fmt.Errorf("User '%s' lacks permission for datasets: '%s'", tenantID, strings.Join(unauthorizedIDs, ", "))
	}

	errors := make([]string, 0)
	successCount := 0
	for _, kb := range kbs {
		if err := s.deleteDataset(tenantID, kb); err != nil {
			errors = append(errors, err.Error())
			continue
		}
		successCount++
	}

	if len(errors) == 0 {
		return map[string]interface{}{"success_count": successCount}, nil
	}

	details := strings.Join(errors, "; ")
	if len(details) > 128 {
		details = details[:128]
	}
	errorMessage := fmt.Sprintf(
		"Successfully deleted %d datasets, %d failed. Details: %s...",
		successCount,
		len(errors),
		details,
	)
	if successCount == 0 {
		return nil, fmt.Errorf("%s", errorMessage)
	}

	return map[string]interface{}{
		"success_count": successCount,
		"errors":        limitStrings(errors, 5),
	}, nil
}

func (s *DatasetsService) deleteDataset(tenantID string, kb *model.Knowledgebase) error {
	return dao.DB.Transaction(func(tx *gorm.DB) error {
		var documents []model.Document
		if err := tx.Where("kb_id = ?", kb.ID).Find(&documents).Error; err != nil {
			return fmt.Errorf("Delete dataset error for %s", kb.ID)
		}

		docIDs := make([]string, 0, len(documents))
		for _, document := range documents {
			docIDs = append(docIDs, document.ID)
		}

		if len(docIDs) > 0 {
			var mappings []model.File2Document
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

			if err := tx.Where("doc_id IN ?", docIDs).Delete(&model.Task{}).Error; err != nil {
				return fmt.Errorf("Delete dataset error for %s", kb.ID)
			}
			if err := tx.Where("document_id IN ?", docIDs).Delete(&model.File2Document{}).Error; err != nil {
				return fmt.Errorf("Delete dataset error for %s", kb.ID)
			}
			if len(fileIDs) > 0 {
				if err := tx.Unscoped().Where("id IN ?", fileIDs).Delete(&model.File{}).Error; err != nil {
					return fmt.Errorf("Delete dataset error for %s", kb.ID)
				}
			}
			if err := tx.Where("id IN ?", docIDs).Delete(&model.Document{}).Error; err != nil {
				return fmt.Errorf("Delete dataset error for %s", kb.ID)
			}
		}

		if err := tx.Unscoped().
			Where("source_type = ? AND type = ? AND name = ? AND tenant_id = ?", string(model.FileSourceKnowledgebase), "folder", kb.Name, tenantID).
			Delete(&model.File{}).Error; err != nil {
			return fmt.Errorf("Delete dataset error for %s", kb.ID)
		}

		if err := tx.Where("id = ?", kb.ID).Delete(&model.Knowledgebase{}).Error; err != nil {
			return fmt.Errorf("Delete dataset error for %s", kb.ID)
		}

		return nil
	})
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
			*tenantLLM.ModelType == string(model.ModelTypeEmbedding) {
			return true, ""
		}
	}

	return false, fmt.Sprintf("Unauthorized model: <%s>", embdID)
}

func validateDatasetAvatar(value string) error {
	if !strings.Contains(value, ",") {
		return fmt.Errorf("Missing MIME prefix. Expected format: data:<mime>;base64,<data>")
	}

	prefix, _, _ := strings.Cut(value, ",")
	if !strings.HasPrefix(prefix, "data:") {
		return fmt.Errorf("Invalid MIME prefix format. Must start with 'data:'")
	}

	mimeType, _, _ := strings.Cut(strings.TrimPrefix(prefix, "data:"), ";")
	if _, ok := supportedAvatarMIMETypes[mimeType]; !ok {
		return fmt.Errorf("Unsupported MIME type. Allowed: [image/jpeg image/png]")
	}
	return nil
}

func validateEmbeddingModelIdentifier(value string) error {
	modelName, provider, err := parseModelName(value)
	if err != nil {
		return fmt.Errorf("Embedding model identifier must follow <model_name>@<provider> format")
	}
	if strings.TrimSpace(modelName) == "" || strings.TrimSpace(provider) == "" {
		return fmt.Errorf("Both model_name and provider must be non-empty strings")
	}
	return nil
}

func normalizePipelineID(value string) (*string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	if len(trimmed) != 32 {
		return nil, fmt.Errorf("pipeline_id must be 32 hex characters")
	}
	for _, char := range trimmed {
		if !strings.ContainsRune("0123456789abcdefABCDEF", char) {
			return nil, fmt.Errorf("pipeline_id must be hexadecimal")
		}
	}
	normalized := strings.ToLower(trimmed)
	return &normalized, nil
}

func normalizeUUID1Hex(value string) (string, error) {
	parsedUUID, err := uuid.Parse(value)
	if err != nil {
		return "", fmt.Errorf("Invalid UUID1 format")
	}
	if parsedUUID.Version() != 1 {
		return "", fmt.Errorf("Must be a UUID1 format")
	}
	return strings.ReplaceAll(parsedUUID.String(), "-", ""), nil
}

func generateUUID1Hex() (string, error) {
	generatedUUID, err := uuid.NewUUID()
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(generatedUUID.String(), "-", ""), nil
}

func applyAutoMetadataConfig(parserConfig map[string]interface{}, config *AutoMetadataConfig) map[string]interface{} {
	merged := utility.CloneMap(parserConfig)
	if merged == nil {
		merged = make(map[string]interface{})
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
	merged["metadata"] = fields
	enableMetadata := true
	if config.Enabled != nil {
		enableMetadata = *config.Enabled
	}
	merged["enable_metadata"] = enableMetadata
	return merged
}

func datasetListItemToMap(kb *model.KnowledgebaseListItem) map[string]interface{} {
	item := map[string]interface{}{
		"id":         kb.ID,
		"name":       kb.Name,
		"tenant_id":  kb.TenantID,
		"permission": kb.Permission,
		"doc_num":    kb.DocNum,
		"token_num":  kb.TokenNum,
		"chunk_num":  kb.ChunkNum,
		"parser_id":  kb.ParserID,
		"embd_id":    kb.EmbdID,
		"nickname":   kb.Nickname,
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

	return utility.RemapKeys(item, datasetResponseKeyAliases)
}

func getStringFromMap(source map[string]interface{}, key string) (string, bool) {
	value, ok := source[key]
	if !ok {
		return "", false
	}
	stringValue, ok := value.(string)
	if !ok {
		return "", false
	}
	return stringValue, true
}

func getMapFromMap(source map[string]interface{}, key string) (map[string]interface{}, bool) {
	value, ok := source[key]
	if !ok {
		return nil, false
	}
	mapValue, ok := value.(map[string]interface{})
	if !ok {
		return nil, false
	}
	return mapValue, true
}

func limitStrings(values []string, limit int) []string {
	if len(values) <= limit {
		return values
	}
	return values[:limit]
}
