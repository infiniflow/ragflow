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
	"os"
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

// SkillSpaceService handles business logic for skills space operations
type SkillSpaceService struct {
	spaceDAO           *dao.SkillSpaceDAO
	fileDAO            *dao.FileDAO
	configDAO          *dao.SkillSearchConfigDAO
	tenantDAO          *dao.TenantDAO
	skillsFolderCache  map[string]string   // tenant-keyed cache for skills folder ID
	skillsFolderMu     sync.RWMutex        // protects skillsFolderCache
	skillsFolderCreateMu sync.Map          // tenant-scoped locks for folder creation
	spaceCreateMu      sync.Map            // tenant-scoped locks for space creation (prevents TOCTOU races)
}

// NewSkillSpaceService creates a new SkillSpaceService instance
func NewSkillSpaceService() *SkillSpaceService {
	return &SkillSpaceService{
		spaceDAO:          dao.NewSkillSpaceDAO(),
		fileDAO:           dao.NewFileDAO(),
		configDAO:         dao.NewSkillSearchConfigDAO(),
		tenantDAO:         dao.NewTenantDAO(),
		skillsFolderCache: make(map[string]string),
	}
}

// CreateSpaceRequest represents the request to create a skills space
type CreateSpaceRequest struct {
	TenantID    string `json:"tenant_id" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	EmbdID      string `json:"embd_id"`
	RerankID    string `json:"rerank_id"`
}

// UpdateSpaceRequest represents the request to update a skills space
type UpdateSpaceRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	EmbdID      string `json:"embd_id"`
	RerankID    string `json:"rerank_id"`
	TopK        int    `json:"top_k"`
}

// getSkillsFolderID gets or creates the skills folder for a tenant
// Uses tenant-scoped locking to prevent duplicate folder creation
func (s *SkillSpaceService) getSkillsFolderID(tenantID string) (string, error) {
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
	folderID := generateSpaceID()
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

// CreateSpace creates a new skills space with associated folder
func (s *SkillSpaceService) CreateSpace(req *CreateSpaceRequest) (map[string]interface{}, common.ErrorCode, error) {
	// Validate name
	if req.Name == "" {
		return nil, common.CodeDataError, fmt.Errorf("space name is required")
	}

	// Tenant-scoped serialization to prevent concurrent create/delete races
	tenantKey := req.TenantID + ":" + req.Name
	mu, _ := s.spaceCreateMu.LoadOrStore(tenantKey, &sync.Mutex{})
	tenantMu := mu.(*sync.Mutex)
	tenantMu.Lock()
	defer func() {
		tenantMu.Unlock()
		s.spaceCreateMu.Delete(tenantKey)
	}()

	// Double-check after acquiring lock: Check if space with same name already exists (active status)
	existingSpace, err := s.spaceDAO.GetByTenantAndName(req.TenantID, req.Name)
	if err != nil {
		// Space doesn't exist, continue
	} else if existingSpace != nil {
		return nil, common.CodeDataError, fmt.Errorf("space with name '%s' already exists", req.Name)
	}

	// Check if there's a space with the same name that is currently being deleted
	existingSpaceAny, err := s.spaceDAO.GetByTenantAndNameAnyStatus(req.TenantID, req.Name)
	if err == nil && existingSpaceAny != nil && existingSpaceAny.Status == entity.SpaceStatusDeleting {
		return nil, common.CodeDataError, fmt.Errorf("space with name '%s' is being deleted, please try again later", req.Name)
	}

	// Check if there's a deleted/non-active space with the same name and permanently delete it
	// This handles the case where a previous creation failed partially
	// Only delete non-active spaces (status != '1') to prevent TOCTOU race
	if err := s.spaceDAO.DeletePermanentByName(req.TenantID, req.Name); err != nil {
		logger.Warn("Failed to delete permanent space by name", zap.Error(err))
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
			logger.Info("Deleting existing space folder with same name", zap.String("folderID", f.ID), zap.String("name", req.Name))
			if err := s.deleteFolderRecursive(f.ID); err != nil {
				logger.Warn("Failed to delete existing folder", zap.String("folderID", f.ID), zap.Error(err))
			}
			break
		}
	}

	// Generate space ID and folder ID
	spaceID := generateSpaceID()
	folderID := generateSpaceID()
	timestamp := time.Now().UnixMilli()
	now := time.Now()

	// Create folder for the space under skills folder
	folder := &entity.File{
		ID:         folderID,
		ParentID:   skillsFolderID,
		TenantID:   req.TenantID,
		CreatedBy:  req.TenantID,
		Name:       req.Name,
		Type:       "folder",
		Size:       0,
		SourceType: "skill_space",
	}

	if err := s.fileDAO.Create(folder); err != nil {
		logger.Error("Failed to create space folder", err)
		return nil, common.CodeOperatingError, fmt.Errorf("failed to create space folder: %w", err)
	}

	// Create the space
	space := &entity.SkillSpace{
		ID:          spaceID,
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

	if err := s.spaceDAO.Create(space); err != nil {
		// Rollback: delete the created folder
		logger.Error("Failed to create space in database", err)
		s.fileDAO.DeleteByIDs([]string{folderID})
		return nil, common.CodeOperatingError, fmt.Errorf("failed to create space: %w", err)
	}

	// Create default search config for this space
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
		if _, err := s.configDAO.GetOrCreate(req.TenantID, spaceID, defaultEmbdID); err != nil {
			logger.Warn("Failed to create skill search config for new space",
				zap.String("tenantID", req.TenantID),
				zap.String("spaceID", spaceID),
				zap.String("embdID", defaultEmbdID),
				zap.Error(err))
		}
	}

	return space.ToMap(), common.CodeSuccess, nil
}

// ListSpaces lists all skills spaces for a tenant
func (s *SkillSpaceService) ListSpaces(tenantID string) (map[string]interface{}, common.ErrorCode, error) {
	spaces, err := s.spaceDAO.GetByTenantID(tenantID)
	if err != nil {
		return nil, common.CodeOperatingError, fmt.Errorf("failed to list spaces: %w", err)
	}

	// Convert to maps
	spaceList := make([]map[string]interface{}, len(spaces))
	for i, space := range spaces {
		spaceList[i] = space.ToMap()
	}

	return map[string]interface{}{
		"spaces": spaceList,
		"total":  len(spaceList),
	}, common.CodeSuccess, nil
}

// GetSpace retrieves a skills space by ID (includes deleting status for visibility)
func (s *SkillSpaceService) GetSpace(spaceID, tenantID string) (map[string]interface{}, common.ErrorCode, error) {
	space, err := s.spaceDAO.GetByIDAnyStatus(spaceID)
	if err != nil {
		return nil, common.CodeDataError, fmt.Errorf("space not found")
	}

	// Verify tenant ownership
	if space.TenantID != tenantID {
		return nil, common.CodeDataError, fmt.Errorf("space not found")
	}

	// Return deleted spaces as not found
	if space.Status == entity.SpaceStatusDeleted {
		return nil, common.CodeDataError, fmt.Errorf("space not found")
	}

	return space.ToMap(), common.CodeSuccess, nil
}

// UpdateSpace updates a skills space
func (s *SkillSpaceService) UpdateSpace(spaceID string, tenantID string, req *UpdateSpaceRequest) (map[string]interface{}, common.ErrorCode, error) {
	space, err := s.spaceDAO.GetByID(spaceID)
	if err != nil {
		return nil, common.CodeDataError, fmt.Errorf("space not found")
	}

	// Verify tenant ownership
	if space.TenantID != tenantID {
		return nil, common.CodeDataError, fmt.Errorf("space not found")
	}

	// Build updates
	updates := make(map[string]interface{})
	
	if req.Name != "" && req.Name != space.Name {
		// Check if name already exists
		existingSpace, _ := s.spaceDAO.GetByTenantAndName(tenantID, req.Name)
		if existingSpace != nil && existingSpace.ID != spaceID {
			return nil, common.CodeDataError, fmt.Errorf("space with name '%s' already exists", req.Name)
		}

		originalName := space.Name
		updates["name"] = req.Name

		// Update space first, then folder (atomic-like behavior with rollback on failure)
		if err := s.spaceDAO.UpdateByID(spaceID, updates); err != nil {
			return nil, common.CodeOperatingError, fmt.Errorf("failed to update space name: %w", err)
		}

		// Update folder name as well - if this fails, rollback space name
		if err := s.fileDAO.UpdateByID(space.FolderID, map[string]interface{}{"name": req.Name}); err != nil {
			logger.Error("Failed to update folder name, rolling back space name", err)
			// Rollback space name
			if rollbackErr := s.spaceDAO.UpdateByID(spaceID, map[string]interface{}{"name": originalName}); rollbackErr != nil {
				logger.Error("Failed to rollback space name after folder rename failure", rollbackErr)
			}
			return nil, common.CodeOperatingError, fmt.Errorf("failed to update folder name: %w", err)
		}

		// Clear updates map since we've already applied name change
		delete(updates, "name")
	}
	
	if req.Description != space.Description {
		updates["description"] = req.Description
	}
	if req.EmbdID != "" && req.EmbdID != space.EmbdID {
		updates["embd_id"] = req.EmbdID
	}
	if req.RerankID != space.RerankID {
		updates["rerank_id"] = req.RerankID
	}
	if req.TopK > 0 && req.TopK != space.TopK {
		updates["top_k"] = req.TopK
	}

	if len(updates) > 0 {
		if err := s.spaceDAO.UpdateByID(spaceID, updates); err != nil {
			return nil, common.CodeOperatingError, fmt.Errorf("failed to update space: %w", err)
		}
	}

	// Refresh space data
	space, _ = s.spaceDAO.GetByID(spaceID)
	return space.ToMap(), common.CodeSuccess, nil
}

// getPythonServiceURL returns the Python service URL from environment or default
func getPythonServiceURL() string {
	url := os.Getenv("PYTHON_SERVICE_URL")
	if url == "" {
		url = "http://127.0.0.1:9380"
	}
	// Ensure URL has scheme
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "http://" + url
	}
	// Ensure URL has the API path
	if !strings.HasSuffix(url, "/api/v1/files") {
		url = strings.TrimSuffix(url, "/")
		url = url + "/api/v1/files"
	}
	return url
}

// deleteFolderViaPythonAPI calls Python backend API to delete folder and its storage
func (s *SkillSpaceService) deleteFolderViaPythonAPI(folderID, tenantID, authHeader string) error {
	pythonURL := getPythonServiceURL()

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

	// Use request context with timeout to prevent indefinite blocking
	deleteCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	req = req.WithContext(deleteCtx)

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

// DeleteSpace starts asynchronous deletion of a skills space and returns immediately.
// The space status is set to "deleting" and the actual cleanup runs in a background goroutine.
func (s *SkillSpaceService) DeleteSpace(spaceID, tenantID string, docEngine engine.DocEngine, authHeader string) (common.ErrorCode, error) {
	// Get space regardless of status (could be retrying a failed delete)
	space, err := s.spaceDAO.GetByIDAnyStatus(spaceID)
	if err != nil {
		return common.CodeDataError, fmt.Errorf("space not found")
	}

	// Verify tenant ownership
	if space.TenantID != tenantID {
		return common.CodeDataError, fmt.Errorf("space not found")
	}

	// If already deleting, return success (idempotent)
	if space.Status == entity.SpaceStatusDeleting {
		logger.Info("Space is already being deleted", zap.String("spaceID", spaceID))
		return common.CodeSuccess, nil
	}

	// If already deleted, return success (idempotent)
	if space.Status == entity.SpaceStatusDeleted {
		logger.Info("Space is already deleted", zap.String("spaceID", spaceID))
		return common.CodeSuccess, nil
	}

	// CAS: status must be "1" (active) → "2" (deleting) to prevent concurrent deletes
	swapped, err := s.spaceDAO.CASStatus(spaceID, entity.SpaceStatusActive, entity.SpaceStatusDeleting)
	if err != nil {
		return common.CodeOperatingError, fmt.Errorf("failed to update space status: %w", err)
	}
	if !swapped {
		// Another request already changed the status
		return common.CodeOperatingError, fmt.Errorf("space is being modified by another request")
	}

	logger.Info("Space marked as deleting, starting async cleanup", zap.String("spaceID", spaceID), zap.String("tenantID", tenantID))

	// Launch async deletion in background goroutine
	go s.asyncDeleteSpace(spaceID, space.FolderID, tenantID, docEngine, authHeader)

	return common.CodeSuccess, nil
}

// asyncDeleteSpace performs the actual deletion work in the background.
// It deletes the search index, removes files via Python API, and soft-deletes the space record.
func (s *SkillSpaceService) asyncDeleteSpace(spaceID, folderID, tenantID string, docEngine engine.DocEngine, authHeader string) {
	defer func() {
		if r := recover(); r != nil {
			logger.Warn("Panic in asyncDeleteSpace, marking space as deleted", zap.Any("recover", r), zap.String("spaceID", spaceID))
			_, _ = s.spaceDAO.CASStatus(spaceID, entity.SpaceStatusDeleting, entity.SpaceStatusDeleted)
		}
	}()

	// Step 1: Delete the search index
	if docEngine != nil {
		indexName := getSkillIndexName(tenantID, spaceID)
		logger.Info("Async deleting space index", zap.String("index", indexName), zap.String("spaceID", spaceID))
		deleteCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		if err := docEngine.DropTable(deleteCtx, indexName); err != nil {
			logger.Warn("Failed to delete space index during async delete", zap.String("index", indexName), zap.Error(err))
			// Continue with other cleanup steps
		} else {
			logger.Info("Successfully deleted space index", zap.String("index", indexName))
		}
		cancel()
	}

	// Step 2: Delete folder and storage via Python API
	logger.Info("Async deleting space folder via Python API", zap.String("folderID", folderID), zap.String("spaceID", spaceID))
	if err := s.deleteFolderViaPythonAPI(folderID, tenantID, authHeader); err != nil {
		logger.Error(fmt.Sprintf("Failed to delete space folder via Python API during async delete, spaceID=%s", spaceID), err)
		// Retry once with a delay
		time.Sleep(5 * time.Second)
		if retryErr := s.deleteFolderViaPythonAPI(folderID, tenantID, authHeader); retryErr != nil {
			logger.Error(fmt.Sprintf("Retry failed to delete space folder, marking space as deleted anyway, spaceID=%s", spaceID), retryErr)
			// Mark as deleted even if folder deletion fails - orphaned folders can be cleaned up later
		}
	} else {
		logger.Info("Successfully deleted space folder via Python API", zap.String("folderID", folderID))
	}

	// Step 3: Soft delete the space record (status "2" → "0")
	// First, permanently remove any previously deleted spaces with the same tenant+name
	// to avoid UNIQUE INDEX constraint violation when changing status from "2" to "0"
	space, err := s.spaceDAO.GetByIDAnyStatus(spaceID)
	if err == nil && space != nil {
		_ = s.spaceDAO.DeletePermanentByName(space.TenantID, space.Name)
	}

	swapped, err := s.spaceDAO.CASStatus(spaceID, entity.SpaceStatusDeleting, entity.SpaceStatusDeleted)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to update space status to deleted, spaceID=%s", spaceID), err)
		return
	}
	if !swapped {
		logger.Warn("Space status was not 'deleting' when trying to mark as deleted", zap.String("spaceID", spaceID))
		return
	}

	logger.Info("Successfully completed async space deletion", zap.String("spaceID", spaceID))
}

// deleteFolderRecursive recursively deletes a folder and all its contents
func (s *SkillSpaceService) deleteFolderRecursive(folderID string) error {
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

// GetSpaceByFolderID retrieves a skills space by its folder ID
func (s *SkillSpaceService) GetSpaceByFolderID(folderID, tenantID string) (map[string]interface{}, common.ErrorCode, error) {
	space, err := s.spaceDAO.GetByFolderID(folderID)
	if err != nil {
		return nil, common.CodeDataError, fmt.Errorf("space not found for folder")
	}

	// Verify tenant ownership
	if space.TenantID != tenantID {
		return nil, common.CodeDataError, fmt.Errorf("space not found")
	}

	return space.ToMap(), common.CodeSuccess, nil
}

// generateSpaceID generates a unique ID for space
func generateSpaceID() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")[:32]
}
