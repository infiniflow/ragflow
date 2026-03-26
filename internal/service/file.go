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
	"strings"

	"github.com/google/uuid"

	"ragflow/internal/dao"
	"ragflow/internal/model"
)

// FileService file service
type FileService struct {
	fileDAO         *dao.FileDAO
	file2DocumentDAO *dao.File2DocumentDAO
}

// NewFileService create file service
func NewFileService() *FileService {
	return &FileService{
		fileDAO:          dao.NewFileDAO(),
		file2DocumentDAO: dao.NewFile2DocumentDAO(),
	}
}

// FileInfo file info with additional fields
type FileInfo struct {
	*model.File
	Size           int64                    `json:"size"`
	KbsInfo        []map[string]interface{} `json:"kbs_info"`
	HasChildFolder bool                     `json:"has_child_folder,omitempty"`
}

// ListFilesResponse list files response
type ListFilesResponse struct {
	Total          int64              `json:"total"`
	Files          []map[string]interface{} `json:"files"`
	ParentFolder   map[string]interface{}   `json:"parent_folder"`
}

// GetRootFolder gets or creates root folder for tenant
func (s *FileService) GetRootFolder(tenantID string) (map[string]interface{}, error) {
	file, err := s.fileDAO.GetRootFolder(tenantID)
	if err != nil {
		return nil, err
	}
	return s.toFileResponse(file), nil
}

// ListFiles lists files by parent folder ID
func (s *FileService) ListFiles(tenantID, pfID string, page, pageSize int, orderby string, desc bool, keywords string) (*ListFilesResponse, error) {
	// If pfID is empty, get root folder
	if pfID == "" {
		rootFolder, err := s.fileDAO.GetRootFolder(tenantID)
		if err != nil {
			return nil, err
		}
		pfID = rootFolder.ID
	}

	// Check if parent folder exists
	if _, err := s.fileDAO.GetByID(pfID); err != nil {
		return nil, err
	}

	// Get files by parent folder ID
	files, total, err := s.fileDAO.GetByPfID(tenantID, pfID, page, pageSize, orderby, desc, keywords)
	if err != nil {
		return nil, err
	}

	// Get parent folder
	parentFolder, err := s.fileDAO.GetParentFolder(pfID)
	if err != nil {
		return nil, err
	}

	// Process files to add additional info
	fileResponses := make([]map[string]interface{}, len(files))
	for i, file := range files {
		fileInfo := s.toFileInfo(file)
		
		// If folder, calculate size and check for child folders
		if file.Type == "folder" {
			folderSize, err := s.fileDAO.GetFolderSize(file.ID)
			if err == nil {
				fileInfo.Size = folderSize
			}
			hasChild, err := s.fileDAO.HasChildFolder(file.ID)
			if err == nil {
				fileInfo.HasChildFolder = hasChild
			}
			fileInfo.KbsInfo = []map[string]interface{}{}
		} else {
			// Get KB info for non-folder files
			kbsInfo, err := s.file2DocumentDAO.GetKBInfoByFileID(file.ID)
			if err != nil {
				kbsInfo = []map[string]interface{}{}
			}
			fileInfo.KbsInfo = kbsInfo
		}
		
		fileResponses[i] = s.fileInfoToResponse(fileInfo)
	}

	return &ListFilesResponse{
		Total:        total,
		Files:        fileResponses,
		ParentFolder: s.toFileResponse(parentFolder),
	}, nil
}

// toFileResponse converts file model to response format
func (s *FileService) toFileResponse(file *model.File) map[string]interface{} {
	result := map[string]interface{}{
		"id":         file.ID,
		"parent_id":  file.ParentID,
		"tenant_id":  file.TenantID,
		"created_by": file.CreatedBy,
		"name":       file.Name,
		"size":       file.Size,
		"type":       file.Type,
		"create_time": file.CreateTime,
		"update_time": file.UpdateTime,
	}
	
	if file.Location != nil {
		result["location"] = *file.Location
	}
	result["source_type"] = file.SourceType
	
	return result
}

// toFileInfo converts file model to FileInfo
func (s *FileService) toFileInfo(file *model.File) *FileInfo {
	return &FileInfo{
		File:           file,
		Size:           file.Size,
		KbsInfo:        []map[string]interface{}{},
		HasChildFolder: false,
	}
}

// fileInfoToResponse converts FileInfo to response map
func (s *FileService) fileInfoToResponse(info *FileInfo) map[string]interface{} {
	result := map[string]interface{}{
		"id":          info.File.ID,
		"parent_id":   info.File.ParentID,
		"tenant_id":   info.File.TenantID,
		"created_by":  info.File.CreatedBy,
		"name":        info.File.Name,
		"size":        info.Size,
		"type":        info.File.Type,
		"create_time": info.File.CreateTime,
		"update_time": info.File.UpdateTime,
		"kbs_info":    info.KbsInfo,
	}

	if info.File.Location != nil {
		result["location"] = *info.File.Location
	}
	result["source_type"] = info.File.SourceType

	if info.File.Type == "folder" {
		result["has_child_folder"] = info.HasChildFolder
	}

	return result
}

// GetParentFolder gets parent folder of a file
func (s *FileService) GetParentFolder(fileID string) (map[string]interface{}, error) {
	// Check if file exists
	if _, err := s.fileDAO.GetByID(fileID); err != nil {
		return nil, err
	}

	// Get parent folder
	parentFolder, err := s.fileDAO.GetParentFolder(fileID)
	if err != nil {
		return nil, err
	}

	return s.toFileResponse(parentFolder), nil
}

// GetAllParentFolders gets all parent folders in path
func (s *FileService) GetAllParentFolders(fileID string) ([]map[string]interface{}, error) {
	// Check if file exists
	if _, err := s.fileDAO.GetByID(fileID); err != nil {
		return nil, err
	}

	// Get all parent folders
	parentFolders, err := s.fileDAO.GetAllParentFolders(fileID)
	if err != nil {
		return nil, err
	}

	// Convert to response format
	result := make([]map[string]interface{}, len(parentFolders))
	for i, folder := range parentFolders {
		result[i] = s.toFileResponse(folder)
	}

	return result, nil
}

// GetFileByID gets file by ID
func (s *FileService) GetFileByID(fileID string) (map[string]interface{}, error) {
	file, err := s.fileDAO.GetByID(fileID)
	if err != nil {
		return nil, err
	}

	fileInfo := s.toFileInfo(file)

	// If folder, calculate size and check for child folders
	if file.Type == "folder" {
		folderSize, err := s.fileDAO.GetFolderSize(file.ID)
		if err == nil {
			fileInfo.Size = folderSize
		}
		hasChild, err := s.fileDAO.HasChildFolder(file.ID)
		if err == nil {
			fileInfo.HasChildFolder = hasChild
		}
		fileInfo.KbsInfo = []map[string]interface{}{}
	} else {
		// Get KB info for non-folder files
		kbsInfo, err := s.file2DocumentDAO.GetKBInfoByFileID(file.ID)
		if err != nil {
			kbsInfo = []map[string]interface{}{}
		}
		fileInfo.KbsInfo = kbsInfo
	}

	return s.fileInfoToResponse(fileInfo), nil
}

// CreateFolder creates a new folder
func (s *FileService) CreateFolder(tenantID, name, parentID, fileType string) (map[string]interface{}, error) {
	// If parent_id is empty, get root folder
	if parentID == "" {
		rootFolder, err := s.fileDAO.GetRootFolder(tenantID)
		if err != nil {
			return nil, err
		}
		parentID = rootFolder.ID
	}

	// Check if parent folder exists
	if _, err := s.fileDAO.GetByID(parentID); err != nil {
		return nil, err
	}

	// Determine file type
	ft := fileType
	if ft == "" {
		ft = "folder"
	}

	// Create file model
	file := &model.File{
		ID:        generateFileUUID(),
		ParentID:  parentID,
		TenantID:  tenantID,
		CreatedBy: tenantID,
		Name:      name,
		Type:      ft,
		Size:      0,
	}
	file.SourceType = ""

	// Save to database
	if err := s.fileDAO.Create(file); err != nil {
		return nil, err
	}

	return s.toFileResponse(file), nil
}

// DeleteFiles deletes files by IDs
func (s *FileService) DeleteFiles(fileIDs []string) error {
	if len(fileIDs) == 0 {
		return nil
	}
	_, err := s.fileDAO.DeleteByIDs(fileIDs)
	return err
}

// generateFileUUID generates a UUID for file
func generateFileUUID() string {
	id := uuid.New().String()
	return strings.ReplaceAll(id, "-", "")
}
