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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	"ragflow/internal/logger"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// SkillsHubService handles business logic for skills hub operations
type SkillsHubService struct {
	hubDAO             *dao.SkillsHubDAO
	fileDAO            *dao.FileDAO
	configDAO          *dao.SkillSearchConfigDAO
	tenantDAO          *dao.TenantDAO
	skillsFolderCache  map[string]string   // tenant-keyed cache for skills folder ID
	skillsFolderMu     sync.RWMutex        // protects skillsFolderCache
	skillsFolderCreateMu sync.Map          // tenant-scoped locks for folder creation
	hubCreateMu        sync.Map            // tenant-scoped locks for hub creation (prevents TOCTOU races)
}

// NewSkillsHubService creates a new SkillsHubService instance
func NewSkillsHubService() *SkillsHubService {
	return &SkillsHubService{
		hubDAO:            dao.NewSkillsHubDAO(),
		fileDAO:           dao.NewFileDAO(),
		configDAO:         dao.NewSkillSearchConfigDAO(),
		tenantDAO:         dao.NewTenantDAO(),
		skillsFolderCache: make(map[string]string),
	}
}

// CreateHubRequest represents the request to create a skills hub
type CreateHubRequest struct {
	TenantID    string `json:"tenant_id" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	EmbdID      string `json:"embd_id"`
	RerankID    string `json:"rerank_id"`
}

// UpdateHubRequest represents the request to update a skills hub
type UpdateHubRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	EmbdID      string `json:"embd_id"`
	RerankID    string `json:"rerank_id"`
	TopK        int    `json:"top_k"`
}

// getSkillsFolderID gets or creates the skills folder for a tenant
// Uses tenant-scoped locking to prevent duplicate folder creation
func (s *SkillsHubService) getSkillsFolderID(tenantID string) (string, error) {
	// Return cached value if available (read lock)
	s.skillsFolderMu.RLock()
	if cachedID, ok := s.skillsFolderCache[tenantID]; ok && cachedID != "" {
		s.skillsFolderMu.RUnlock()
		return cachedID, nil
	}
	s.skillsFolderMu.RUnlock()

	// Acquire tenant-scoped creation lock
	lock, _ := s.skillsFolderCreateMu.LoadOrStore(tenantID, &sync.Mutex{})
	lock.(*sync.Mutex).Lock()
	defer lock.(*sync.Mutex).Unlock()

	// Double-check cache after acquiring lock
	s.skillsFolderMu.RLock()
	if cachedID, ok := s.skillsFolderCache[tenantID]; ok && cachedID != "" {
		s.skillsFolderMu.RUnlock()
		return cachedID, nil
	}
	s.skillsFolderMu.RUnlock()

	// Get root folder
	rootFolder, err := s.fileDAO.GetRootFolder(tenantID)
	if err != nil {
		return "", fmt.Errorf("failed to get root folder: %w", err)
	}

	// Look for skills folder under root
	files, _, err := s.fileDAO.GetByPfID(tenantID, rootFolder.ID, 0, 0, "name", false, "")
	if err != nil {
		return "", fmt.Errorf("failed to list root folder contents: %w", err)
	}

	for _, file := range files {
		if file.Type == "folder" && file.Name == "skills" {
			// Cache the result (write lock)
			s.skillsFolderMu.Lock()
			s.skillsFolderCache[tenantID] = file.ID
			s.skillsFolderMu.Unlock()
			return file.ID, nil
		}
	}

	// Skills folder not found, create it
	logger.Info("Creating skills folder", zap.String("tenant_id", tenantID))
	folderID := generateHubID()
	now := time.Now()
	createTime := now.UnixMilli()
	folder := &entity.File{
		ID:         folderID,
		ParentID:   rootFolder.ID,
		TenantID:   tenantID,
		CreatedBy:  tenantID,
		Name:       "skills",
		Type:       "folder",
		Size:       0,
		SourceType: "system",
		BaseModel: entity.BaseModel{
			CreateTime: &createTime,
			UpdateTime: &createTime,
			CreateDate: &now,
			UpdateDate: &now,
		},
	}

	if err := s.fileDAO.Create(folder); err != nil {
		return "", fmt.Errorf("failed to create skills folder: %w", err)
	}

	// Cache the result (write lock)
	s.skillsFolderMu.Lock()
	s.skillsFolderCache[tenantID] = folderID
	s.skillsFolderMu.Unlock()

	return folderID, nil
}

// CreateHub creates a new skills hub with associated folder
func (s *SkillsHubService) CreateHub(req *CreateHubRequest) (map[string]interface{}, common.ErrorCode, error) {
	// Validate name
	if req.Name == "" {
		return nil, common.CodeDataError, fmt.Errorf("hub name is required")
	}

	// Tenant-scoped serialization to prevent concurrent create/delete races
	tenantKey := req.TenantID + ":" + req.Name
	mu, _ := s.hubCreateMu.LoadOrStore(tenantKey, &sync.Mutex{})
	tenantMu := mu.(*sync.Mutex)
	tenantMu.Lock()
	defer func() {
		tenantMu.Unlock()
		s.hubCreateMu.Delete(tenantKey)
	}()

	// Double-check after acquiring lock: Check if hub with same name already exists (active status)
	existingHub, err := s.hubDAO.GetByTenantAndName(req.TenantID, req.Name)
	if err != nil {
		// Hub doesn't exist, continue
	} else if existingHub != nil {
		return nil, common.CodeDataError, fmt.Errorf("hub with name '%s' already exists", req.Name)
	}

	// Check if there's a deleted/non-active hub with the same name and permanently delete it
	// This handles the case where a previous creation failed partially
	// Only delete non-active hubs (status != '1') to prevent TOCTOU race
	if err := s.hubDAO.DeletePermanentByName(req.TenantID, req.Name); err != nil {
		logger.Warn("Failed to delete permanent hub by name", zap.Error(err))
	}

	// Get skills folder ID
	skillsFolderID, err := s.getSkillsFolderID(req.TenantID)
	if err != nil {
		logger.Error("Failed to get skills folder ID", err)
		return nil, common.CodeOperatingError, err
	}

	// Check if there's an existing folder with the same name under skills folder
	// If exists, delete it to prevent duplicate folder names
	existingFolders := s.fileDAO.Query(req.Name, skillsFolderID)
	for _, f := range existingFolders {
		if f.Type == "folder" && f.Name == req.Name {
			logger.Info("Deleting existing hub folder with same name", zap.String("folderID", f.ID), zap.String("name", req.Name))
			if err := s.deleteFolderRecursive(f.ID); err != nil {
				logger.Warn("Failed to delete existing folder", zap.String("folderID", f.ID), zap.Error(err))
			}
			break
		}
	}

	// Generate hub ID and folder ID
	hubID := generateHubID()
	folderID := generateHubID()
	timestamp := time.Now().UnixMilli()
	now := time.Now()

	// Create folder for the hub under skills folder
	folder := &entity.File{
		ID:         folderID,
		ParentID:   skillsFolderID,
		TenantID:   req.TenantID,
		CreatedBy:  req.TenantID,
		Name:       req.Name,
		Type:       "folder",
		Size:       0,
		SourceType: "skills_hub",
	}

	if err := s.fileDAO.Create(folder); err != nil {
		logger.Error("Failed to create hub folder", err)
		return nil, common.CodeOperatingError, fmt.Errorf("failed to create hub folder: %w", err)
	}

	// Create the hub
	hub := &entity.SkillsHub{
		ID:          hubID,
		TenantID:    req.TenantID,
		Name:        req.Name,
		FolderID:    folderID,
		Description: req.Description,
		EmbdID:      req.EmbdID,
		RerankID:    req.RerankID,
		TopK:        10,
		Status:      "1",
		CreateTime:  &timestamp,
		UpdateTime:  &now,
	}

	if err := s.hubDAO.Create(hub); err != nil {
		// Rollback: delete the created folder
		logger.Error("Failed to create hub in database", err)
		s.fileDAO.DeleteByIDs([]string{folderID})
		return nil, common.CodeOperatingError, fmt.Errorf("failed to create hub: %w", err)
	}

	// Create default search config for this hub
	// Use tenant's default embedding model if not provided
	defaultEmbdID := req.EmbdID
	if defaultEmbdID == "" {
		tenant, err := s.tenantDAO.GetByID(req.TenantID)
		if err == nil && tenant != nil && tenant.EmbdID != "" {
			defaultEmbdID = tenant.EmbdID
			logger.Info("Using tenant default embedding model", zap.String("tenantID", req.TenantID), zap.String("embdID", defaultEmbdID))
		} else {
			logger.Warn("Tenant has no default embedding model, skill search will not work until configured", zap.String("tenantID", req.TenantID))
		}
	}
	if defaultEmbdID != "" {
		_, _ = s.configDAO.GetOrCreate(req.TenantID, hubID, defaultEmbdID)
	}

	return hub.ToMap(), common.CodeSuccess, nil
}

// ListHubs lists all skills hubs for a tenant
func (s *SkillsHubService) ListHubs(tenantID string) (map[string]interface{}, common.ErrorCode, error) {
	hubs, err := s.hubDAO.GetByTenantID(tenantID)
	if err != nil {
		return nil, common.CodeOperatingError, fmt.Errorf("failed to list hubs: %w", err)
	}

	// Convert to maps
	hubList := make([]map[string]interface{}, len(hubs))
	for i, hub := range hubs {
		hubList[i] = hub.ToMap()
	}

	return map[string]interface{}{
		"hubs":  hubList,
		"total": len(hubList),
	}, common.CodeSuccess, nil
}

// GetHub retrieves a skills hub by ID
func (s *SkillsHubService) GetHub(hubID, tenantID string) (map[string]interface{}, common.ErrorCode, error) {
	hub, err := s.hubDAO.GetByID(hubID)
	if err != nil {
		return nil, common.CodeDataError, fmt.Errorf("hub not found")
	}

	// Verify tenant ownership
	if hub.TenantID != tenantID {
		return nil, common.CodeDataError, fmt.Errorf("hub not found")
	}

	return hub.ToMap(), common.CodeSuccess, nil
}

// UpdateHub updates a skills hub
func (s *SkillsHubService) UpdateHub(hubID string, tenantID string, req *UpdateHubRequest) (map[string]interface{}, common.ErrorCode, error) {
	hub, err := s.hubDAO.GetByID(hubID)
	if err != nil {
		return nil, common.CodeDataError, fmt.Errorf("hub not found")
	}

	// Verify tenant ownership
	if hub.TenantID != tenantID {
		return nil, common.CodeDataError, fmt.Errorf("hub not found")
	}

	// Build updates
	updates := make(map[string]interface{})
	
	if req.Name != "" && req.Name != hub.Name {
		// Check if name already exists
		existingHub, _ := s.hubDAO.GetByTenantAndName(tenantID, req.Name)
		if existingHub != nil && existingHub.ID != hubID {
			return nil, common.CodeDataError, fmt.Errorf("hub with name '%s' already exists", req.Name)
		}

		originalName := hub.Name
		updates["name"] = req.Name

		// Update hub first, then folder (atomic-like behavior with rollback on failure)
		if err := s.hubDAO.UpdateByID(hubID, updates); err != nil {
			return nil, common.CodeOperatingError, fmt.Errorf("failed to update hub name: %w", err)
		}

		// Update folder name as well - if this fails, rollback hub name
		if err := s.fileDAO.UpdateByID(hub.FolderID, map[string]interface{}{"name": req.Name}); err != nil {
			logger.Error("Failed to update folder name, rolling back hub name", err)
			// Rollback hub name
			if rollbackErr := s.hubDAO.UpdateByID(hubID, map[string]interface{}{"name": originalName}); rollbackErr != nil {
				logger.Error("Failed to rollback hub name after folder rename failure", rollbackErr)
			}
			return nil, common.CodeOperatingError, fmt.Errorf("failed to update folder name: %w", err)
		}

		// Clear updates map since we've already applied name change
		delete(updates, "name")
	}
	
	if req.Description != hub.Description {
		updates["description"] = req.Description
	}
	if req.EmbdID != "" && req.EmbdID != hub.EmbdID {
		updates["embd_id"] = req.EmbdID
	}
	if req.RerankID != hub.RerankID {
		updates["rerank_id"] = req.RerankID
	}
	if req.TopK > 0 && req.TopK != hub.TopK {
		updates["top_k"] = req.TopK
	}

	if len(updates) > 0 {
		if err := s.hubDAO.UpdateByID(hubID, updates); err != nil {
			return nil, common.CodeOperatingError, fmt.Errorf("failed to update hub: %w", err)
		}
	}

	// Refresh hub data
	hub, _ = s.hubDAO.GetByID(hubID)
	return hub.ToMap(), common.CodeSuccess, nil
}

// deleteFolderViaPythonAPI calls Python backend API to delete folder and its storage
func (s *SkillsHubService) deleteFolderViaPythonAPI(folderID, tenantID, authHeader string) error {
	// Python service runs on port 9380 (Go runs on 9384)
	pythonURL := "http://127.0.0.1:9380/api/v1/files"

	reqBody := map[string]interface{}{
		"ids": []string{folderID},
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("DELETE", pythonURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	// Extract raw token from "Bearer <token>" format if present
	// Python backend needs the raw token for authentication
	authToken := authHeader
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		authToken = strings.TrimSpace(authHeader[7:])
	}
	req.Header.Set("Authorization", authToken)
	// Set tenant ID header for Python backend
	req.Header.Set("X-tenant-id", tenantID)

	logger.Info("Calling Python API to delete folder", zap.String("folderID", folderID), zap.String("tenantID", tenantID))

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call Python API: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	logger.Info("Python API delete folder response", zap.String("folderID", folderID), zap.Int("status", resp.StatusCode), zap.String("body", string(body)))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Python API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response to check if deletion was successful
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if code, ok := result["code"].(float64); !ok || int(code) != 0 {
		message := "unknown error"
		if msg, ok := result["message"].(string); ok {
			message = msg
		}
		return fmt.Errorf("Python API returned error: %s", message)
	}

	logger.Info("Successfully deleted folder via Python API", zap.String("folderID", folderID))
	return nil
}

// DeleteHub deletes a skills hub and its associated folder
func (s *SkillsHubService) DeleteHub(hubID, tenantID string, docEngine engine.DocEngine, authHeader string) (common.ErrorCode, error) {
	hub, err := s.hubDAO.GetByID(hubID)
	if err != nil {
		return common.CodeDataError, fmt.Errorf("hub not found")
	}

	// Verify tenant ownership
	if hub.TenantID != tenantID {
		return common.CodeDataError, fmt.Errorf("hub not found")
	}

	// Delete the hub index if docEngine is provided
	if docEngine != nil {
		indexName := getSkillIndexName(tenantID, hubID)
		logger.Info("Deleting hub index", zap.String("index", indexName), zap.String("hubID", hubID), zap.String("tenantID", tenantID))
		// Create a timeout context for index deletion
		deleteCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := docEngine.DeleteIndex(deleteCtx, indexName); err != nil {
			logger.Warn("Failed to delete hub index", zap.String("index", indexName), zap.Error(err))
			// Don't return error, continue to delete hub data
		} else {
			logger.Info("Successfully deleted hub index", zap.String("index", indexName))
		}
	} else {
		logger.Warn("docEngine is nil, skipping index deletion")
	}

	// Delete the associated folder and all its contents via Python API (hard delete with storage)
	logger.Info("Starting to delete hub folder via Python API", zap.String("folderID", hub.FolderID))
	if err := s.deleteFolderViaPythonAPI(hub.FolderID, tenantID, authHeader); err != nil {
		logger.Error("Failed to delete hub folder via Python API", err)
		return common.CodeOperatingError, fmt.Errorf("failed to delete hub folder: %w", err)
	}
	logger.Info("Successfully deleted hub folder via Python API", zap.String("folderID", hub.FolderID))

	// Delete the hub (soft delete)
	if err := s.hubDAO.Delete(hubID); err != nil {
		return common.CodeOperatingError, fmt.Errorf("failed to delete hub: %w", err)
	}
	logger.Info("Soft deleted hub record", zap.String("hubID", hubID))

	return common.CodeSuccess, nil
}

// deleteFolderRecursive recursively deletes a folder and all its contents
func (s *SkillsHubService) deleteFolderRecursive(folderID string) error {
	// Get all children
	children, err := s.fileDAO.ListByParentID(folderID)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to list children for folder %s", folderID), err)
		return err
	}

	logger.Info("Deleting folder contents", zap.String("folder_id", folderID), zap.Int("child_count", len(children)))

	// Collect file IDs (non-folder) and recurse into subfolders
	var fileIDs []string
	for _, child := range children {
		if child.Type == "folder" {
			logger.Debug("Recursively deleting child folder", zap.String("folder_id", child.ID), zap.String("folder_name", child.Name))
			if err := s.deleteFolderRecursive(child.ID); err != nil {
				logger.Warn("Failed to delete child folder", zap.String("folder_id", child.ID), zap.Error(err))
			}
		} else {
			// Collect non-folder files for batch deletion
			logger.Debug("Collecting file for deletion", zap.String("file_id", child.ID), zap.String("file_name", child.Name))
			fileIDs = append(fileIDs, child.ID)
		}
	}

	// Delete all non-folder files in batch
	if len(fileIDs) > 0 {
		logger.Info("Deleting files in folder", zap.String("folder_id", folderID), zap.Int("file_count", len(fileIDs)))
		if _, err := s.fileDAO.DeleteByIDs(fileIDs); err != nil {
			logger.Warn("Failed to delete files in folder", zap.String("folder_id", folderID), zap.Strings("file_ids", fileIDs), zap.Error(err))
			// Continue to delete folder even if file deletion fails
		}
	}

	// Delete the folder itself
	logger.Info("Deleting folder", zap.String("folder_id", folderID))
	_, err = s.fileDAO.DeleteByIDs([]string{folderID})
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to delete folder %s", folderID), err)
	}
	return err
}

// GetHubByFolderID retrieves a skills hub by its folder ID
func (s *SkillsHubService) GetHubByFolderID(folderID, tenantID string) (map[string]interface{}, common.ErrorCode, error) {
	hub, err := s.hubDAO.GetByFolderID(folderID)
	if err != nil {
		return nil, common.CodeDataError, fmt.Errorf("hub not found for folder")
	}

	// Verify tenant ownership
	if hub.TenantID != tenantID {
		return nil, common.CodeDataError, fmt.Errorf("hub not found")
	}

	return hub.ToMap(), common.CodeSuccess, nil
}

// generateHubID generates a unique ID for hub
func generateHubID() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")[:32]
}
