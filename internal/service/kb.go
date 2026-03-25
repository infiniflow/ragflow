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
	"errors"
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/model"
	"ragflow/internal/utility"
	"strings"
	"time"

	"github.com/google/uuid"
)

// KnowledgebaseService service class for managing dataset operations
type KnowledgebaseService struct {
	kbDAO         *dao.KnowledgebaseDAO
	userTenantDAO *dao.UserTenantDAO
	userDAO       *dao.UserDAO
	tenantDAO     *dao.TenantDAO
	connectorDAO  *dao.ConnectorDAO
}

// NewKnowledgebaseService creates a new knowledge base service
func NewKnowledgebaseService() *KnowledgebaseService {
	return &KnowledgebaseService{
		kbDAO:         dao.NewKnowledgebaseDAO(),
		userTenantDAO: dao.NewUserTenantDAO(),
		userDAO:       dao.NewUserDAO(),
		tenantDAO:     dao.NewTenantDAO(),
		connectorDAO:  dao.NewConnectorDAO(),
	}
}

// CreateKBRequest represents the request for creating a knowledge base
type CreateKBRequest struct {
	Name         string                 `json:"name" binding:"required"`
	ParserID     *string                `json:"parser_id,omitempty"`
	Description  *string                `json:"description,omitempty"`
	Language     *string                `json:"language,omitempty"`
	Permission   *string                `json:"permission,omitempty"`
	Avatar       *string                `json:"avatar,omitempty"`
	ParserConfig map[string]interface{} `json:"parser_config,omitempty"`
}

// CreateKBResponse represents the response for creating a knowledge base
type CreateKBResponse struct {
	KBID string `json:"kb_id"`
}

// UpdateKBRequest represents the request for updating a knowledge base
type UpdateKBRequest struct {
	KBID         string                 `json:"kb_id" binding:"required"`
	Name         string                 `json:"name" binding:"required"`
	Description  *string                `json:"description"`
	ParserID     string                 `json:"parser_id" binding:"required"`
	Permission   *string                `json:"permission,omitempty"`
	Language     *string                `json:"language,omitempty"`
	Avatar       *string                `json:"avatar,omitempty"`
	Pagerank     *int64                 `json:"pagerank,omitempty"`
	ParserConfig map[string]interface{} `json:"parser_config,omitempty"`
	Connectors   []string               `json:"connectors,omitempty"`
}

// UpdateMetadataSettingRequest represents the request for updating metadata settings
type UpdateMetadataSettingRequest struct {
	KBID           string                 `json:"kb_id" binding:"required"`
	Metadata       map[string]interface{} `json:"metadata" binding:"required"`
	EnableMetadata *bool                  `json:"enable_metadata,omitempty"`
}

// ListKbsRequest represents the request for listing knowledge bases
type ListKbsRequest struct {
	Keywords *string   `json:"keywords,omitempty"`
	Page     *int      `json:"page,omitempty"`
	PageSize *int      `json:"page_size,omitempty"`
	ParserID *string   `json:"parser_id,omitempty"`
	Orderby  *string   `json:"orderby,omitempty"`
	Desc     *bool     `json:"desc,omitempty"`
	OwnerIDs *[]string `json:"owner_ids,omitempty"`
}

// ListKbsResponse represents the response for listing knowledge bases
type ListKbsResponse struct {
	KBs   []map[string]interface{} `json:"kbs"`
	Total int64                    `json:"total"`
}

// CreateKB creates a new knowledge base
// This matches the Python create endpoint in kb_app.py
func (s *KnowledgebaseService) CreateKB(req *CreateKBRequest, tenantID string) (*CreateKBResponse, common.ErrorCode, error) {
	// Validate name is a string
	if !isValidString(req.Name) {
		return nil, common.CodeDataError, errors.New("Dataset name must be string.")
	}

	// Trim and validate name
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, common.CodeDataError, errors.New("Dataset name can't be empty.")
	}

	// Check name length (using UTF-8 byte length like Python)
	if len(name) > model.DatasetNameLimit {
		return nil, common.CodeDataError, fmt.Errorf("Dataset name length is %d which is large than %d", len(name), model.DatasetNameLimit)
	}

	// Verify tenant exists
	tenant, err := s.tenantDAO.GetByID(tenantID)
	if err != nil {
		return nil, common.CodeDataError, errors.New("Tenant not found.")
	}

	// Deduplicate name within tenant
	duplicateName := s.kbDAO.DuplicateName(name, tenantID)

	// Get parser ID (default to "naive")
	parserID := "naive"
	if req.ParserID != nil && *req.ParserID != "" {
		parserID = *req.ParserID
	}

	// Get parser config with defaults
	parserConfig := getParserConfig(parserID, req.ParserConfig)
	parserConfig["llm_id"] = tenant.LLMID

	// Generate KB ID
	kbID := strings.ReplaceAll(uuid.New().String(), "-", "")

	// Create knowledge base model
	now := time.Now().Unix()
	nowDate := time.Now().Truncate(time.Second)
	kb := &model.Knowledgebase{
		ID:           kbID,
		Name:         duplicateName,
		TenantID:     tenantID,
		CreatedBy:    tenantID,
		ParserID:     parserID,
		ParserConfig: parserConfig,
		Permission:   "me",
		EmbdID:       "",
	}
	kb.CreateTime = &now
	kb.UpdateTime = &now
	kb.CreateDate = &nowDate
	kb.UpdateDate = &nowDate
	status := string(model.StatusValid)
	kb.Status = &status

	// Set optional fields
	if req.Description != nil {
		kb.Description = req.Description
	}
	if req.Language != nil {
		kb.Language = req.Language
	}
	if req.Permission != nil {
		kb.Permission = *req.Permission
	}
	if req.Avatar != nil {
		kb.Avatar = req.Avatar
	}

	// Create in database
	if err := s.kbDAO.Create(kb); err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to create knowledge base: %w", err)
	}

	return &CreateKBResponse{KBID: kbID}, common.CodeSuccess, nil
}

// UpdateKB updates an existing knowledge base
// This matches the Python update endpoint in kb_app.py
func (s *KnowledgebaseService) UpdateKB(req *UpdateKBRequest, userID string) (map[string]interface{}, common.ErrorCode, error) {
	// Validate name is a string
	if !isValidString(req.Name) {
		return nil, common.CodeDataError, errors.New("Dataset name must be string.")
	}

	// Trim and validate name
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, common.CodeDataError, errors.New("Dataset name can't be empty.")
	}

	// Check name length
	if len(name) > model.DatasetNameLimit {
		return nil, common.CodeDataError, fmt.Errorf("Dataset name length is %d which is large than %d", len(name), model.DatasetNameLimit)
	}

	// Check authorization
	if !s.kbDAO.Accessible4Deletion(req.KBID, userID) {
		return nil, common.CodeAuthenticationError, errors.New("No authorization.")
	}

	// Verify ownership
	kbs, err := s.kbDAO.Query(map[string]interface{}{"created_by": userID, "id": req.KBID})
	if err != nil || len(kbs) == 0 {
		return nil, common.CodeOperatingError, errors.New("only owner of dataset authorized for this operation")
	}

	// Get existing KB
	kb, err := s.kbDAO.GetByID(req.KBID)
	if err != nil {
		return nil, common.CodeDataError, errors.New("can't find this dataset")
	}

	// Check for duplicate name
	if strings.ToLower(name) != strings.ToLower(kb.Name) {
		existingKB, _ := s.kbDAO.GetByName(name, userID)
		if existingKB != nil {
			return nil, common.CodeDataError, errors.New("duplicated dataset name")
		}
	}

	// Build updates
	updates := map[string]interface{}{
		"name":      name,
		"parser_id": req.ParserID,
	}

	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Permission != nil {
		updates["permission"] = *req.Permission
	}
	if req.Language != nil {
		updates["language"] = *req.Language
	}
	if req.Avatar != nil {
		updates["avatar"] = *req.Avatar
	}
	if req.Pagerank != nil {
		updates["pagerank"] = *req.Pagerank
	}
	if req.ParserConfig != nil {
		updates["parser_config"] = req.ParserConfig
	}

	now := time.Now().Unix()
	nowDate := time.Now().Truncate(time.Second)
	updates["update_time"] = now
	updates["update_date"] = nowDate

	// Update in database
	if err := s.kbDAO.UpdateByID(req.KBID, updates); err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to update knowledge base: %w", err)
	}

	// Get updated KB
	updatedKB, err := s.kbDAO.GetByID(req.KBID)
	if err != nil {
		return nil, common.CodeDataError, errors.New("database error (knowledgebase rename)")
	}

	result := updatedKB.ToMap()
	result["connectors"] = req.Connectors

	return result, common.CodeSuccess, nil
}

// UpdateMetadataSetting updates the metadata settings for a knowledge base
func (s *KnowledgebaseService) UpdateMetadataSetting(req *UpdateMetadataSettingRequest) (map[string]interface{}, common.ErrorCode, error) {
	kb, err := s.kbDAO.GetByID(req.KBID)
	if err != nil {
		return nil, common.CodeDataError, errors.New("database error (knowledgebase not found)")
	}

	parserConfig := kb.ParserConfig
	if parserConfig == nil {
		parserConfig = make(map[string]interface{})
	}

	parserConfig["metadata"] = req.Metadata
	enableMetadata := true
	if req.EnableMetadata != nil {
		enableMetadata = *req.EnableMetadata
	}
	parserConfig["enable_metadata"] = enableMetadata

	if err := s.kbDAO.UpdateParserConfig(req.KBID, parserConfig); err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to update metadata setting: %w", err)
	}

	result := kb.ToMap()
	result["parser_config"] = parserConfig

	return result, common.CodeSuccess, nil
}

// GetDetail retrieves detailed information about a knowledge base
// This matches the Python kb_detail endpoint in kb_app.py
func (s *KnowledgebaseService) GetDetail(kbID, userID string) (*model.KnowledgebaseDetail, common.ErrorCode, error) {
	// Check authorization
	if !s.kbDAO.Accessible(kbID, userID) {
		return nil, common.CodeOperatingError, errors.New("only owner of dataset authorized for this operation")
	}

	// Get detail
	detail, err := s.kbDAO.GetDetail(kbID)
	if err != nil {
		return nil, common.CodeDataError, errors.New("can't find this dataset")
	}

	// Set connectors (empty for now)
	detail.Connectors = []string{}

	return detail, common.CodeSuccess, nil
}

// ListKbs lists knowledge bases with pagination and filtering
// This matches the Python list endpoint in kb_app.py
func (s *KnowledgebaseService) ListKbs(keywords string, page int, pageSize int, parserID string, orderby string, desc bool, ownerIDs []string, userID string) (*ListKbsResponse, common.ErrorCode, error) {
	var kbs []*model.KnowledgebaseListItem
	var total int64
	var err error

	if len(ownerIDs) > 0 {
		// List by owner IDs
		kbs, total, err = s.kbDAO.GetByTenantIDs(ownerIDs, userID, page, pageSize, orderby, desc, keywords, parserID)
	} else {
		// Get tenant IDs for user
		tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
		if err != nil {
			return nil, common.CodeServerError, err
		}

		kbs, total, err = s.kbDAO.GetByTenantIDs(tenantIDs, userID, page, pageSize, orderby, desc, keywords, parserID)
	}

	if err != nil {
		return nil, common.CodeServerError, err
	}

	// Convert to map slice
	kbMaps := make([]map[string]interface{}, len(kbs))
	for i, kb := range kbs {
		kbMaps[i] = map[string]interface{}{
			"id":            kb.ID,
			"avatar":        kb.Avatar,
			"name":          kb.Name,
			"language":      kb.Language,
			"description":   kb.Description,
			"tenant_id":     kb.TenantID,
			"permission":    kb.Permission,
			"doc_num":       kb.DocNum,
			"token_num":     kb.TokenNum,
			"chunk_num":     kb.ChunkNum,
			"parser_id":     kb.ParserID,
			"embd_id":       kb.EmbdID,
			"nickname":      kb.Nickname,
			"tenant_avatar": kb.TenantAvatar,
			"update_time":   kb.UpdateTime,
		}
	}

	return &ListKbsResponse{
		KBs:   kbMaps,
		Total: total,
	}, common.CodeSuccess, nil
}

// DeleteKB soft deletes a knowledge base
// This matches the Python rm endpoint in kb_app.py
func (s *KnowledgebaseService) DeleteKB(kbID, userID string) (common.ErrorCode, error) {
	// Check authorization
	if !s.kbDAO.Accessible4Deletion(kbID, userID) {
		return common.CodeAuthenticationError, errors.New("No authorization.")
	}

	// Verify ownership
	kbs, err := s.kbDAO.Query(map[string]interface{}{"created_by": userID, "id": kbID})
	if err != nil || len(kbs) == 0 {
		return common.CodeOperatingError, errors.New("only owner of dataset authorized for this operation")
	}

	// Soft delete
	if err := s.kbDAO.Delete(kbID); err != nil {
		return common.CodeServerError, fmt.Errorf("database error (knowledgebase removal): %w", err)
	}

	return common.CodeSuccess, nil
}

// Accessible checks if a knowledge base is accessible by a user
func (s *KnowledgebaseService) Accessible(kbID, userID string) bool {
	return s.kbDAO.Accessible(kbID, userID)
}

// GetByID retrieves a knowledge base by ID
func (s *KnowledgebaseService) GetByID(kbID string) (*model.Knowledgebase, error) {
	return s.kbDAO.GetByID(kbID)
}

// GetKBIDsByTenantID retrieves all knowledge base IDs for a tenant
func (s *KnowledgebaseService) GetKBIDsByTenantID(tenantID string) ([]string, error) {
	return s.kbDAO.GetKBIDsByTenantID(tenantID)
}

// isValidString checks if a value is a non-empty string
func isValidString(v interface{}) bool {
	str, ok := v.(string)
	return ok && str != ""
}

// getParserConfig returns the parser configuration with defaults
// This matches the Python get_parser_config function
func getParserConfig(parserID string, customConfig map[string]interface{}) map[string]interface{} {
	config := map[string]interface{}{
		"pages":              [][]int{{1, 1000000}},
		"table_context_size": 0,
		"image_context_size": 0,
	}

	switch parserID {
	case "table":
		config["layout_recognize"] = false
		config["chunk_token_num"] = 128
		config["delimiter"] = "\n!?;。；！？"
		config["html4excel"] = false
	case "naive":
		config["chunk_token_num"] = 128
		config["delimiter"] = "\n!?;。；！？"
		config["html4excel"] = false
	default:
		config["raptor"] = map[string]interface{}{
			"use_raptor": false,
		}
	}

	// Merge custom config if provided
	if customConfig != nil {
		config = mergeParserConfig(config, customConfig)
	}

	return config
}

// mergeParserConfig merges two parser configurations
func mergeParserConfig(base, override map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range base {
		result[k] = v
	}

	for k, v := range override {
		if existing, ok := result[k]; ok {
			if existingMap, ok := existing.(map[string]interface{}); ok {
				if newMap, ok := v.(map[string]interface{}); ok {
					result[k] = mergeParserConfig(existingMap, newMap)
					continue
				}
			}
		}
		result[k] = v
	}

	return result
}

// GenerateUUID generates a UUID string without dashes
func GenerateUUID() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")
}

// GetUserByToken gets user by authorization token
func (s *KnowledgebaseService) GetUserByToken(authorization string) (*model.User, common.ErrorCode, error) {
	userService := NewUserService()
	return userService.GetUserByToken(authorization)
}

// GetUserByID gets user by ID
func (s *KnowledgebaseService) GetUserByID(id string) (*model.User, error) {
	return s.userDAO.GetByAccessToken(id)
}

// GetTenantIDsByUserID gets tenant IDs for a user
func (s *KnowledgebaseService) GetTenantIDsByUserID(userID string) ([]string, error) {
	return s.userTenantDAO.GetTenantIDsByUserID(userID)
}

// GetConnectorsByTenantID gets connectors for a tenant
func (s *KnowledgebaseService) GetConnectorsByTenantID(tenantID string) ([]*dao.ConnectorListItem, error) {
	return s.connectorDAO.ListByTenantID(tenantID)
}

// GetKBList retrieves knowledge bases with ID and name filtering
func (s *KnowledgebaseService) GetKBList(tenantIDs []string, userID string, page, pageSize int, orderby string, desc bool, id, name string) ([]*model.Knowledgebase, int64, common.ErrorCode, error) {
	kbs, total, err := s.kbDAO.GetList(tenantIDs, userID, page, pageSize, orderby, desc, id, name)
	if err != nil {
		return nil, 0, common.CodeServerError, err
	}
	return kbs, total, common.CodeSuccess, nil
}

// GetKBByIDAndUserID retrieves a knowledge base by ID and user ID
func (s *KnowledgebaseService) GetKBByIDAndUserID(kbID, userID string) ([]*model.Knowledgebase, error) {
	return s.kbDAO.GetKBByIDAndUserID(kbID, userID)
}

// GetKBByNameAndUserID retrieves a knowledge base by name and user ID
func (s *KnowledgebaseService) GetKBByNameAndUserID(kbName, userID string) ([]*model.Knowledgebase, error) {
	return s.kbDAO.GetKBByNameAndUserID(kbName, userID)
}

// AtomicIncreaseDocNumByID atomically increments the document count
func (s *KnowledgebaseService) AtomicIncreaseDocNumByID(kbID string) error {
	return s.kbDAO.AtomicIncreaseDocNumByID(kbID)
}

// DecreaseDocumentNum decreases document, chunk, and token counts
func (s *KnowledgebaseService) DecreaseDocumentNum(kbID string, docNum, chunkNum, tokenNum int64) error {
	return s.kbDAO.DecreaseDocumentNum(kbID, docNum, chunkNum, tokenNum)
}

// UpdateParserConfig updates the parser configuration
func (s *KnowledgebaseService) UpdateParserConfig(id string, config map[string]interface{}) error {
	return s.kbDAO.UpdateParserConfig(id, config)
}

// DeleteFieldMap removes the field_map from parser_config
func (s *KnowledgebaseService) DeleteFieldMap(id string) error {
	return s.kbDAO.DeleteFieldMap(id)
}

// GetFieldMap retrieves field mappings from multiple knowledge bases
func (s *KnowledgebaseService) GetFieldMap(ids []string) (map[string]interface{}, error) {
	return s.kbDAO.GetFieldMap(ids)
}

// GetAllIDs retrieves all knowledge base IDs
func (s *KnowledgebaseService) GetAllIDs() ([]string, error) {
	return s.kbDAO.GetAllIDs()
}

// ExtractAccessToken extracts access token from authorization header
func ExtractAccessToken(authorization, secretKey string) (string, error) {
	return utility.ExtractAccessToken(authorization, secretKey)
}
