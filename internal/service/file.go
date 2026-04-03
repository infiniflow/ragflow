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
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
	"ragflow/internal/util"
	"strings"

	"github.com/google/uuid"
)

// FileService file service
type FileService struct {
	fileDAO          *dao.FileDAO
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
	*entity.File
	Size           int64                    `json:"size"`
	KbsInfo        []map[string]interface{} `json:"kbs_info"`
	HasChildFolder bool                     `json:"has_child_folder,omitempty"`
}

// ListFilesResponse list files response
type ListFilesResponse struct {
	Total        int64                    `json:"total"`
	Files        []map[string]interface{} `json:"files"`
	ParentFolder map[string]interface{}   `json:"parent_folder"`
}

// GetRootFolder gets or creates root folder for tenant
func (s *FileService) GetRootFolder(tenantID string) (map[string]interface{}, error) {
	file, err := s.fileDAO.GetRootFolder(tenantID)
	if err != nil {
		return nil, err
	}
	return s.toFileResponse(file), nil
}

// ListFiles lists files by parent folder ID (matching Python /files endpoint)
// This method includes init_knowledgebase_docs initialization when parent_id is empty
func (s *FileService) ListFiles(tenantID, pfID string, page, pageSize int, orderby string, desc bool, keywords string) (*ListFilesResponse, error) {
	// If pfID is empty, get root folder and initialize knowledgebase docs
	if pfID == "" {
		rootFolder, err := s.fileDAO.GetRootFolder(tenantID)
		if err != nil {
			return nil, fmt.Errorf("failed to get root folder: %w", err)
		}
		pfID = rootFolder.ID

		// Initialize knowledgebase docs (matching Python init_knowledgebase_docs logic)
		if err := s.initKnowledgebaseDocs(pfID, tenantID); err != nil {
			return nil, fmt.Errorf("failed to initialize knowledgebase docs: %w", err)
		}
	}

	// Check if parent folder exists
	if _, err := s.fileDAO.GetByID(pfID); err != nil {
		return nil, fmt.Errorf("Folder not found!")
	}

	// Get files by parent folder ID
	files, total, err := s.fileDAO.GetByPfID(tenantID, pfID, page, pageSize, orderby, desc, keywords)
	if err != nil {
		return nil, err
	}

	// Get parent folder
	parentFolder, err := s.fileDAO.GetParentFolder(pfID)
	if err != nil {
		return nil, fmt.Errorf("File not found!")
	}

	// Process files to add additional info
	fileResponses := make([]map[string]interface{}, 0, len(files))
	for _, file := range files {
		fileInfo := s.toFileInfo(file)

		// If folder, calculate size and check for child folders
		if file.Type == FileTypeFolder {
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

		fileResponses = append(fileResponses, s.fileInfoToResponse(fileInfo))
	}

	return &ListFilesResponse{
		Total:        total,
		Files:        fileResponses,
		ParentFolder: s.toFileResponse(parentFolder),
	}, nil
}

// initKnowledgebaseDocs initializes knowledgebase documents for tenant
// This matches Python's FileService.init_knowledgebase_docs method
func (s *FileService) initKnowledgebaseDocs(rootID, tenantID string) error {
	return s.fileDAO.InitKnowledgebaseDocs(rootID, tenantID, s.file2DocumentDAO)
}

// KnowledgebaseFolderName is the folder name for knowledgebase
const KnowledgebaseFolderName = ".knowledgebase"

// FileSourceKnowledgebase represents knowledgebase as file source
const FileSourceKnowledgebase = "knowledgebase"

// toFileResponse converts file model to response format
func (s *FileService) toFileResponse(file *entity.File) map[string]interface{} {
	result := map[string]interface{}{
		"id":          file.ID,
		"parent_id":   file.ParentID,
		"tenant_id":   file.TenantID,
		"created_by":  file.CreatedBy,
		"name":        file.Name,
		"size":        file.Size,
		"type":        file.Type,
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
func (s *FileService) toFileInfo(file *entity.File) *FileInfo {
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

const (
	FileTypeFolder  = "folder"
	FileTypeVirtual = "virtual"
)

// GetDocCount gets document count for a tenant
func (s *FileService) GetDocCount(tenantID string) (int64, error) {
	documentDAO := dao.NewDocumentDAO()
	return documentDAO.CountByTenantID(tenantID)
}

// UploadFile uploads files to a folder
func (s *FileService) UploadFile(tenantID, parentID string, files []*multipart.FileHeader) ([]map[string]interface{}, error) {
	if parentID == "" {
		rootFolder, err := s.fileDAO.GetRootFolder(tenantID)
		if err != nil {
			return nil, fmt.Errorf("failed to get root folder: %w", err)
		}
		parentID = rootFolder.ID
	}

	_, err := s.fileDAO.GetByID(parentID)
	if err != nil {
		return nil, fmt.Errorf("Can't find this folder!")
	}

	maxFileNumPerUser := os.Getenv("MAX_FILE_NUM_PER_USER")
	if maxFileNumPerUser != "" {
		var maxNum int64
		if _, err := fmt.Sscanf(maxFileNumPerUser, "%d", &maxNum); err == nil && maxNum > 0 {
			docCount, err := s.GetDocCount(tenantID)
			if err != nil {
				return nil, fmt.Errorf("failed to get document count: %w", err)
			}
			if docCount >= maxNum {
				return nil, fmt.Errorf("Exceed the maximum file number of a free user!")
			}
		}
	}

	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	var result []map[string]interface{}

	for _, fileHeader := range files {
		filename := fileHeader.Filename
		if filename == "" {
			return nil, fmt.Errorf("No file selected!")
		}

		fileType := util.FilenameType(filename)

		fileObjNames := s.parseFilePath(filename)

		idList, err := s.fileDAO.GetIDListByID(parentID, fileObjNames, 1, []string{parentID})
		if err != nil {
			return nil, fmt.Errorf("failed to get file ID list: %w", err)
		}

		var lastFolder *entity.File
		if len(fileObjNames) != len(idList)-1 {
			lastID := idList[len(idList)-1]
			lastFolder, err = s.fileDAO.GetByID(lastID)
			if err != nil {
				return nil, fmt.Errorf("Folder not found!")
			}
			createdFolder, err := s.createFolderRecursive(lastFolder, fileObjNames, len(idList), tenantID)
			if err != nil {
				return nil, fmt.Errorf("failed to create folder: %w", err)
			}
			lastFolder = createdFolder
		} else {
			lastID := idList[len(idList)-2]
			lastFolder, err = s.fileDAO.GetByID(lastID)
			if err != nil {
				return nil, fmt.Errorf("Folder not found!")
			}
		}

		location := fileObjNames[len(fileObjNames)-1]
		for storageImpl.ObjExist(lastFolder.ID, location) {
			location += "_"
		}

		src, err := fileHeader.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open uploaded file: %w", err)
		}
		defer src.Close()

		data := make([]byte, fileHeader.Size)
		if _, err := src.Read(data); err != nil {
			return nil, fmt.Errorf("failed to read file data: %w", err)
		}

		if err := storageImpl.Put(lastFolder.ID, location, data); err != nil {
			return nil, fmt.Errorf("failed to store file: %w", err)
		}

		uniqueName := s.getUniqueFilename(fileObjNames[len(fileObjNames)-1], lastFolder.ID)

		fileRecord := &entity.File{
			ID:         s.generateUUID(),
			ParentID:   lastFolder.ID,
			TenantID:   tenantID,
			CreatedBy:  tenantID,
			Name:       uniqueName,
			Location:   &location,
			Size:       int64(len(data)),
			Type:       fileType,
			SourceType: "",
		}

		if err := s.fileDAO.Insert(fileRecord); err != nil {
			return nil, fmt.Errorf("failed to insert file record: %w", err)
		}

		result = append(result, s.toFileResponse(fileRecord))
	}

	return result, nil
}

func (s *FileService) parseFilePath(filename string) []string {
	filename = strings.TrimPrefix(filename, "/")
	parts := strings.Split(filename, "/")
	var result []string
	for _, part := range parts {
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func (s *FileService) createFolderRecursive(parentFolder *entity.File, names []string, count int, tenantID string) (*entity.File, error) {
	if count > len(names)-2 {
		return parentFolder, nil
	}

	newFolder, err := s.fileDAO.CreateFolder(parentFolder.ID, tenantID, names[count], FileTypeFolder)
	if err != nil {
		return nil, err
	}

	return s.createFolderRecursive(newFolder, names, count+1, tenantID)
}

func (s *FileService) getUniqueFilename(name, parentID string) string {
	existingFiles := s.fileDAO.Query(name, parentID)
	if len(existingFiles) == 0 {
		return name
	}

	base := filepath.Base(name)
	ext := filepath.Ext(name)
	nameWithoutExt := strings.TrimSuffix(base, ext)

	counter := 1
	for {
		newName := fmt.Sprintf("%s_%d%s", nameWithoutExt, counter, ext)
		existingFiles = s.fileDAO.Query(newName, parentID)
		if len(existingFiles) == 0 {
			return newName
		}
		counter++
	}
}

func (s *FileService) generateUUID() string {
	id := uuid.New().String()
	return strings.ReplaceAll(id, "-", "")
}

// CreateFolder creates a new folder or virtual file
func (s *FileService) CreateFolder(tenantID, name, parentID, fileType string) (map[string]interface{}, error) {
	if parentID == "" {
		rootFolder, err := s.fileDAO.GetRootFolder(tenantID)
		if err != nil {
			return nil, fmt.Errorf("failed to get root folder: %w", err)
		}
		parentID = rootFolder.ID
	}

	if !s.fileDAO.IsParentFolderExist(parentID) {
		return nil, fmt.Errorf("Parent Folder Doesn't Exist!")
	}

	existingFiles := s.fileDAO.Query(name, parentID)
	if len(existingFiles) > 0 {
		return nil, fmt.Errorf("Duplicated folder name in the same folder.")
	}

	if fileType == "" {
		fileType = FileTypeVirtual
	}

	if fileType == FileTypeFolder {
		fileType = FileTypeFolder
	} else {
		fileType = FileTypeVirtual
	}

	folder, err := s.fileDAO.CreateFolder(parentID, tenantID, name, fileType)
	if err != nil {
		return nil, fmt.Errorf("failed to create folder: %w", err)
	}

	return s.toFileResponse(folder), nil
}
