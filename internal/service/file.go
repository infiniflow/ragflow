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
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/parser/parser"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
	"regexp"
	"strings"
	"time"
)

// FileService file service
type FileService struct {
	fileDAO          *dao.FileDAO
	file2DocumentDAO *dao.File2DocumentDAO
	documentService  *DocumentService
}

// NewFileService create file service
func NewFileService() *FileService {
	return &FileService{
		fileDAO:          dao.NewFileDAO(),
		file2DocumentDAO: dao.NewFile2DocumentDAO(),
		documentService:  NewDocumentService(),
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
// This method includes init_dataset_docs initialization when parent_id is empty
func (s *FileService) ListFiles(tenantID, pfID string, page, pageSize int, orderby string, desc bool, keywords string) (*ListFilesResponse, error) {
	// If pfID is empty, get root folder and initialize dataset docs
	if pfID == "" {
		rootFolder, err := s.fileDAO.GetRootFolder(tenantID)
		if err != nil {
			return nil, fmt.Errorf("failed to get root folder: %w", err)
		}
		pfID = rootFolder.ID

		// Initialize dataset docs (matching Python init_knowledgebase_docs logic)
		if err := s.initDatasetDocs(pfID, tenantID); err != nil {
			return nil, fmt.Errorf("failed to initialize dataset docs: %w", err)
		}

		// Initialize skills folder (matching Python init_skills_folder logic)
		if err := s.initSkillsFolder(pfID, tenantID); err != nil {
			return nil, fmt.Errorf("failed to initialize skills folder: %w", err)
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

	// Process files to add additional info, deduplicating by ID as a safety net
	// against any leftover duplicate rows (e.g. duplicate 'skills' or '.knowledgebase' folders).
	fileResponses := make([]map[string]interface{}, 0, len(files))
	seenIDs := make(map[string]struct{})
	for _, file := range files {
		if _, ok := seenIDs[file.ID]; ok {
			continue
		}
		seenIDs[file.ID] = struct{}{}
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

// initDatasetDocs initializes dataset documents for tenant
// This matches Python's FileService.init_dataset_docs method
func (s *FileService) initDatasetDocs(rootID, tenantID string) error {
	return s.fileDAO.InitDatasetDocs(rootID, tenantID, s.file2DocumentDAO)
}

// DatasetFolderName is the folder name for dataset
const DatasetFolderName = ".knowledgebase"

// SkillsFolderName is the folder name for skills
const SkillsFolderName = "skills"

// initSkillsFolder initializes the skills folder under the root folder.
// Deduplicates duplicate entries that may have been created by
// concurrent race conditions (TOCTOU).
func (s *FileService) initSkillsFolder(rootID, tenantID string) error {
	existing := s.fileDAO.Query(SkillsFolderName, rootID, tenantID)
	if len(existing) > 0 {
		if len(existing) > 1 {
			common.Logger.Warn(fmt.Sprintf(
				"Found %d duplicate '%s' folders under root %s, keeping only the first",
				len(existing), SkillsFolderName, rootID,
			))
			keepID := existing[0].ID
			for _, dup := range existing[1:] {
				children, _ := s.fileDAO.ListAllFilesByParentID(dup.ID)
				for _, child := range children {
					s.fileDAO.UpdateByID(child.ID, map[string]interface{}{"parent_id": keepID})
				}
				if delErr := s.fileDAO.Delete(dup.ID); delErr != nil {
					common.Logger.Warn(fmt.Sprintf("Failed to delete duplicate skills folder %s: %v", dup.ID, delErr))
				}
			}
		}
		return nil
	}

	folder := &entity.File{
		ID:         utility.GenerateToken(),
		ParentID:   rootID,
		TenantID:   tenantID,
		CreatedBy:  tenantID,
		Name:       SkillsFolderName,
		Type:       FileTypeFolder,
		Size:       0,
		SourceType: "",
	}
	return s.fileDAO.Insert(folder)
}

// FileSourceDataset represents dataset as file source
const FileSourceDataset = "knowledgebase"

var (
	assertURLSafe    = utility.AssertURLSafe
	pinnedHTTPClient = utility.PinnedHTTPClient
)

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

// GetParentFolder gets parent folder of a file with permission check
func (s *FileService) GetParentFolder(userID, fileID string) (map[string]interface{}, error) {
	// Get file
	file, err := s.fileDAO.GetByID(fileID)
	if err != nil {
		return nil, err
	}

	// Permission check
	if !s.checkFileTeamPermission(file, userID) {
		return nil, fmt.Errorf("No authorization.")
	}

	// Get parent folder
	parentFolder, err := s.fileDAO.GetParentFolder(fileID)
	if err != nil {
		return nil, err
	}

	return s.toFileResponse(parentFolder), nil
}

// GetAllParentFolders gets all parent folders in path with permission check
func (s *FileService) GetAllParentFolders(userID, fileID string) ([]map[string]interface{}, error) {
	// Get file
	file, err := s.fileDAO.GetByID(fileID)
	if err != nil {
		return nil, err
	}

	// Permission check
	if !s.checkFileTeamPermission(file, userID) {
		return nil, fmt.Errorf("No authorization.")
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

		fileType := utility.FilenameType(filename)

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

		data, err := io.ReadAll(src)
		if err != nil {
			return nil, fmt.Errorf("failed to read file data: %w", err)
		}

		if err := storageImpl.Put(lastFolder.ID, location, data); err != nil {
			return nil, fmt.Errorf("failed to store file: %w", err)
		}

		uniqueName := s.getUniqueFilename(fileObjNames[len(fileObjNames)-1], lastFolder.ID, tenantID)

		fileRecord := &entity.File{
			ID:         utility.GenerateToken(),
			ParentID:   lastFolder.ID,
			TenantID:   tenantID,
			CreatedBy:  tenantID,
			Name:       uniqueName,
			Location:   &location,
			Size:       int64(len(data)),
			Type:       string(fileType),
			SourceType: "",
		}

		if err := s.fileDAO.Insert(fileRecord); err != nil {
			return nil, fmt.Errorf("failed to insert file record: %w", err)
		}

		result = append(result, s.toFileResponse(fileRecord))
	}

	return result, nil
}

// UploadInfos mirrors Python's upload_info file branch: store raw bytes in the
// per-user downloads bucket and return lightweight upload descriptors instead
// of creating full File rows in the file-management tree.
func (s *FileService) UploadInfos(userID string, files []*multipart.FileHeader) ([]map[string]interface{}, error) {
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	results := make([]map[string]interface{}, 0, len(files))
	for _, fileHeader := range files {
		filename := fileHeader.Filename
		if err := s.checkUploadInfoHealth(userID, filename); err != nil {
			return nil, err
		}
		src, err := fileHeader.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open uploaded file: %w", err)
		}
		data, readErr := readUploadInfoData(src)
		src.Close()
		if readErr != nil {
			return nil, fmt.Errorf("failed to read file data: %w", readErr)
		}

		contentType := fileHeader.Header.Get("Content-Type")
		if contentType == "" {
			contentType = http.DetectContentType(data)
		}
		filename, contentType, data = normalizeUploadInfoContent(filename, contentType, data)
		resp, err := s.storeUploadInfoBlob(storageImpl, userID, filename, contentType, data)
		if err != nil {
			return nil, err
		}
		results = append(results, resp)
	}
	return results, nil
}

func readUploadInfoData(r io.Reader) ([]byte, error) {
	limited := &io.LimitedReader{R: r, N: maxRemoteFileSize + 1}
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxRemoteFileSize {
		return nil, fmt.Errorf("file size exceeds %d bytes", maxRemoteFileSize)
	}
	return data, nil
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

func (s *FileService) getUniqueFilename(name, parentID, tenantID string) string {
	existingFiles := s.fileDAO.Query(name, parentID, tenantID)
	if len(existingFiles) == 0 {
		return name
	}

	base := filepath.Base(name)
	ext := filepath.Ext(name)
	nameWithoutExt := strings.TrimSuffix(base, ext)

	counter := 1
	for {
		newName := fmt.Sprintf("%s_%d%s", nameWithoutExt, counter, ext)
		existingFiles = s.fileDAO.Query(newName, parentID, tenantID)
		if len(existingFiles) == 0 {
			return newName
		}
		counter++
	}
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

	existingFiles := s.fileDAO.Query(name, parentID, tenantID)
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

// DeleteFiles deletes files by IDs
// Returns (success, message) where success is true if all files were deleted
func (s *FileService) DeleteFiles(ctx context.Context, uid string, fileIDs []string) (bool, string) {
	for _, fileID := range fileIDs {
		// 1. Get file
		file, err := s.fileDAO.GetByID(fileID)
		if err != nil || file == nil {
			return false, "File or Folder not found!"
		}

		// 2. Check tenant_id
		if file.TenantID == "" {
			return false, "Tenant not found!"
		}

		// Block root-folder deletion (root folders have parent_id == id)
		if file.ParentID == file.ID {
			return false, "Root folder cannot be deleted."
		}

		// 3. Permission check
		if !s.checkFileTeamPermission(file, uid) {
			return false, "No authorization."
		}

		// 4. Skip dataset source files
		if file.SourceType == FileSourceDataset {
			continue
		}

		// 5. Delete based on type
		if file.Type == FileTypeFolder {
			if err := s.deleteFolderRecursive(ctx, file, uid); err != nil {
				return false, fmt.Sprintf("Failed to delete folder: %v", err)
			}
		} else {
			if err := s.deleteSingleFile(ctx, file); err != nil {
				return false, fmt.Sprintf("Failed to delete file: %v", err)
			}
		}
	}

	return true, ""
}

// checkFileTeamPermission checks if user has permission to access the file
// Matches Python's check_file_team_permission function
func (s *FileService) checkFileTeamPermission(file *entity.File, uid string) bool {
	// File's tenant directly authorized
	if file.TenantID == uid {
		return true
	}

	// Check KB permissions
	datasetIDs, err := s.fileDAO.GetDatasetIDByFileID(file.ID)
	if err != nil || len(datasetIDs) == 0 {
		return false
	}

	kbDAO := dao.NewKnowledgebaseDAO()
	for _, datasetID := range datasetIDs {
		ds, err := kbDAO.GetByID(datasetID)
		if err != nil || ds == nil {
			continue
		}

		// Check KB tenant permission
		if s.checkDatasetTeamPermission(ds, uid) {
			return true
		}
	}

	return false
}

// checkDatasetTeamPermission checks if user has permission to access the dataset
// Matches Python's check_kb_team_permission function
func (s *FileService) checkDatasetTeamPermission(ds *entity.Knowledgebase, uid string) bool {
	return hasKBTeamPermission(ds, uid, dao.NewTenantDAO())
}

// deleteSingleFile deletes a single file (not folder)
// Matches Python's _delete_single_file function
func (s *FileService) deleteSingleFile(ctx context.Context, file *entity.File) error {
	// 1. Delete storage object
	if file.Location != nil && *file.Location != "" {
		storageImpl := storage.GetStorageFactory().GetStorage()
		if storageImpl != nil {
			if err := storageImpl.Remove(file.ParentID, *file.Location); err != nil {
				common.Logger.Error(fmt.Sprintf("Fail to remove object: %s/%s, error: %v", file.ParentID, *file.Location, err))
			}
		}
	}

	// 2. Handle associated documents
	informs, err := s.file2DocumentDAO.GetByFileID(file.ID)
	if err != nil {
		return fmt.Errorf("failed to get file2document mappings: %w", err)
	}
	if len(informs) > 0 {
		for _, inform := range informs {
			if inform.DocumentID == nil {
				continue
			}
			docID := *inform.DocumentID
			if err := s.documentService.RemoveDocumentKeepFile(docID); err != nil {
				common.Logger.Error(fmt.Sprintf("Fail to remove document: %s, error: %v", docID, err))
			}
		}

		// Delete file2document mapping (outside the loop, called once - matching Python behavior)
		if err := s.file2DocumentDAO.DeleteByFileID(file.ID); err != nil {
			return fmt.Errorf("failed to delete file2document mapping: %w", err)
		}
	}

	// 3. Delete file record
	if err := s.fileDAO.Delete(file.ID); err != nil {
		return err
	}

	return nil
}

// deleteFolderRecursive recursively deletes a folder and its contents
// Matches Python's _delete_folder_recursive function
func (s *FileService) deleteFolderRecursive(ctx context.Context, folder *entity.File, uid string) error {
	// Get all sub-files
	subFiles, err := s.fileDAO.ListByParentID(folder.ID)
	if err != nil {
		return err
	}

	for _, subFile := range subFiles {
		if subFile.Type == FileTypeFolder {
			// Recursively delete subfolder
			if err := s.deleteFolderRecursive(ctx, subFile, uid); err != nil {
				return err
			}
		} else {
			// Delete single file
			if err := s.deleteSingleFile(ctx, subFile); err != nil {
				return err
			}
		}
	}

	// Delete the folder itself
	if err := s.fileDAO.Delete(folder.ID); err != nil {
		return err
	}

	return nil
}

// MoveFileReq represents the request body for move files operation
type MoveFileReq struct {
	SrcFileIDs []string `json:"src_file_ids" binding:"required,min=1"`
	DestFileID string   `json:"dest_file_id"`
	NewName    string   `json:"new_name"`
}

// MoveFiles moves and/or renames files
// Follows Linux mv semantics:
// - new_name only: rename in place (no storage operation)
// - dest_file_id only: move to new folder (keep names)
// - both: move and rename simultaneously
func (s *FileService) MoveFiles(uid string, srcFileIDs []string, destFileID string, newName string) (bool, string) {
	// 1. Get all source files
	files, err := s.fileDAO.GetByIDs(srcFileIDs)
	if err != nil || len(files) == 0 {
		return false, "Source files not found!"
	}

	// Create a map for quick lookup
	filesMap := make(map[string]*entity.File)
	for _, f := range files {
		filesMap[f.ID] = f
	}

	// 2. Validate all source files
	for _, fileID := range srcFileIDs {
		file, ok := filesMap[fileID]
		if !ok {
			return false, "File or folder not found!"
		}
		if file.TenantID == "" {
			return false, "Tenant not found!"
		}
		// 3. Permission check
		if !s.checkFileTeamPermission(file, uid) {
			return false, "No authorization."
		}
	}

	// 4. Validate destination folder if provided
	var destFolder *entity.File
	if destFileID != "" {
		destFolder, err = s.fileDAO.GetByID(destFileID)
		if err != nil || destFolder == nil {
			return false, "Parent folder not found!"
		}
		// Check destination folder permission
		if !s.checkFileTeamPermission(destFolder, uid) {
			return false, "No authorization to write to destination folder."
		}

		if destFolder.Type != FileTypeFolder {
			return false, "Destination is not a folder."
		}

		destAncestors, err := s.fileDAO.GetAllParentFolders(destFolder.ID)
		if err != nil {
			return false, "Parent folder not found!"
		}

		destAncestorIDs := make(map[string]struct{}, len(destAncestors))
		for _, folder := range destAncestors {
			destAncestorIDs[folder.ID] = struct{}{}
		}

		for _, file := range files {
			if file.Type != FileTypeFolder {
				continue
			}

			if file.ID == destFolder.ID {
				return false, "Cannot move a folder to itself."
			}

			if _, ok := destAncestorIDs[file.ID]; ok {
				return false, "Cannot move a folder into its own subfolder."
			}
		}
	}

	// 5. Validate new_name if provided
	if newName != "" {
		if len(srcFileIDs) > 1 {
			return false, "new_name can only be used with a single file"
		}

		file := filesMap[srcFileIDs[0]]
		// Check extension for non-folder files
		if file.Type != FileTypeFolder {
			oldExt := utility.GetFileExtension(file.Name)
			newExt := utility.GetFileExtension(newName)
			if oldExt != newExt {
				return false, "The extension of file can't be changed"
			}
		}

		// Check for duplicate names in target folder
		targetParentID := file.ParentID
		if destFolder != nil {
			targetParentID = destFolder.ID
		}
		existingFiles := s.fileDAO.Query(newName, targetParentID, file.TenantID)
		for _, f := range existingFiles {
			if f.Name == newName {
				return false, "Duplicated file name in the same folder."
			}
		}
	} else if destFolder != nil {
		// Plain move (no rename): check for duplicate names in destination folder
		for _, file := range files {
			existingFiles := s.fileDAO.Query(file.Name, destFolder.ID, file.TenantID)
			for _, f := range existingFiles {
				// Ignore the source file itself
				if f.ID != file.ID {
					return false, "Duplicated file name in the same folder."
				}
			}
		}
	}

	// 6. Perform the move operation
	if destFolder != nil {
		// Move to destination folder
		for _, file := range files {
			if err := s.moveEntryRecursive(file, destFolder, newName); err != nil {
				return false, err.Error()
			}
		}
	} else {
		// Pure rename: no storage operation needed
		if newName == "" {
			return false, "new_name is required for rename"
		}
		if len(srcFileIDs) == 0 {
			return false, "Source files not found!"
		}
		file := filesMap[srcFileIDs[0]]
		if err := s.fileDAO.UpdateByID(file.ID, map[string]interface{}{"name": newName}); err != nil {
			return false, "Database error (File rename)!"
		}

		// Update associated document name if exists
		informs, err := s.file2DocumentDAO.GetByFileID(file.ID)
		if err == nil && len(informs) > 0 && informs[0].DocumentID != nil {
			docID := *informs[0].DocumentID
			documentDAO := dao.NewDocumentDAO()
			if err := documentDAO.UpdateByID(docID, map[string]interface{}{"name": newName}); err != nil {
				return false, "Database error (Document rename)!"
			}
		}
	}

	return true, ""
}

// moveEntryRecursive recursively moves a file or folder entry
func (s *FileService) moveEntryRecursive(sourceFile *entity.File, destFolder *entity.File, overrideName string) error {
	effectiveName := overrideName
	if effectiveName == "" {
		effectiveName = sourceFile.Name
	}

	if sourceFile.Type == FileTypeFolder {
		// Handle folder move
		existingFolders := s.fileDAO.Query(effectiveName, destFolder.ID, sourceFile.TenantID)
		var newFolder *entity.File
		if len(existingFolders) > 0 {
			// Prevent moving a folder into itself (self-target merge)
			if existingFolders[0].ID == sourceFile.ID {
				return fmt.Errorf("cannot move folder into itself")
			}
			newFolder = existingFolders[0]
		} else {
			// Create new folder
			var err error
			newFolder, err = s.fileDAO.CreateFolder(destFolder.ID, sourceFile.TenantID, effectiveName, FileTypeFolder)
			if err != nil {
				return fmt.Errorf("failed to create destination folder: %w", err)
			}
		}

		// Recursively move sub-files
		subFiles, err := s.fileDAO.ListAllFilesByParentID(sourceFile.ID)
		if err != nil {
			return err
		}
		for _, subFile := range subFiles {
			if err := s.moveEntryRecursive(subFile, newFolder, ""); err != nil {
				return err
			}
		}

		// Delete the source folder
		return s.fileDAO.Delete(sourceFile.ID)
	}

	// Handle non-folder file move
	needStorageMove := destFolder.ID != sourceFile.ParentID
	updates := map[string]interface{}{}

	if needStorageMove {
		// Get storage
		storageImpl := storage.GetStorageFactory().GetStorage()
		if storageImpl == nil {
			return fmt.Errorf("storage not initialized")
		}

		// Calculate new location
		newLocation := effectiveName
		for storageImpl.ObjExist(destFolder.ID, newLocation) {
			newLocation += "_"
		}

		// Perform storage move (copy + delete)
		if sourceFile.Location == nil || *sourceFile.Location == "" {
			return fmt.Errorf("file location is empty")
		}

		if !storageImpl.Move(sourceFile.ParentID, *sourceFile.Location, destFolder.ID, newLocation) {
			return fmt.Errorf("move file failed at storage layer")
		}

		updates["parent_id"] = destFolder.ID
		updates["location"] = newLocation
	}

	if overrideName != "" {
		updates["name"] = overrideName
	}

	if len(updates) > 0 {
		if err := s.fileDAO.UpdateByID(sourceFile.ID, updates); err != nil {
			return fmt.Errorf("database error (File update): %w", err)
		}
	}

	// Update associated document name if renamed
	if overrideName != "" {
		informs, err := s.file2DocumentDAO.GetByFileID(sourceFile.ID)
		if err == nil && len(informs) > 0 && informs[0].DocumentID != nil {
			docID := *informs[0].DocumentID
			documentDAO := dao.NewDocumentDAO()
			if err := documentDAO.UpdateByID(docID, map[string]interface{}{"name": overrideName}); err != nil {
				return fmt.Errorf("database error (Document rename): %w", err)
			}
		}
	}

	return nil
}

// GetFileContent gets file metadata and checks permission for download
// Matches Python's file_api_service.get_file_content function
func (s *FileService) GetFileContent(uid, fileID string) (*entity.File, error) {
	file, err := s.fileDAO.GetByID(fileID)
	if err != nil || file == nil {
		return nil, fmt.Errorf("Document not found!")
	}
	if !s.checkFileTeamPermission(file, uid) {
		return nil, fmt.Errorf("No authorization.")
	}
	return file, nil
}

// StorageAddress represents bucket and object name for storage
type StorageAddress struct {
	Bucket string
	Name   string
}

// GetStorageAddress gets storage address for a file (fallback for when direct blob is empty)
// Matches Python's File2DocumentService.get_storage_address function
func (s *FileService) GetStorageAddress(fileID string) (*StorageAddress, error) {
	// Get file2document mapping
	f2d, err := s.file2DocumentDAO.GetByFileID(fileID)
	if err != nil || len(f2d) == 0 {
		return nil, fmt.Errorf("file2document mapping not found")
	}

	// Get the file
	if f2d[0].FileID == nil {
		return nil, fmt.Errorf("file_id is nil in file2document mapping")
	}
	file, err := s.fileDAO.GetByID(*f2d[0].FileID)
	if err != nil || file == nil {
		return nil, fmt.Errorf("file not found")
	}

	// If source_type is empty or local, return file's parent_id and location
	if file.SourceType == "" || entity.FileSource(file.SourceType) == entity.FileSourceLocal {
		if file.Location == nil || *file.Location == "" {
			return nil, fmt.Errorf("file location is empty")
		}
		return &StorageAddress{
			Bucket: file.ParentID,
			Name:   *file.Location,
		}, nil
	}

	// Otherwise, use document's kb_id and location
	if f2d[0].DocumentID == nil {
		return nil, fmt.Errorf("document_id is required")
	}

	documentDAO := dao.NewDocumentDAO()
	doc, err := documentDAO.GetByID(*f2d[0].DocumentID)
	if err != nil || doc == nil {
		return nil, fmt.Errorf("document not found")
	}

	if doc.Location == nil || *doc.Location == "" {
		return nil, fmt.Errorf("document location is empty")
	}

	return &StorageAddress{
		Bucket: doc.KbID,
		Name:   *doc.Location,
	}, nil
}

// DownloadAgentFile downloads an agent-generated file directly from MinIO without querying the database.
func (s *FileService) DownloadAgentFile(tenantID, location string) ([]byte, error) {
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	bucketName := fmt.Sprintf("%s-downloads", tenantID)

	blob, err := storageImpl.Get(bucketName, location)
	if err != nil {
		return nil, fmt.Errorf("failed to read file from storage: %w", err)
	}

	return blob, nil
}

// GetFileContents fetches file contents (text + image) from storage
// for the given file dicts.
//   - raw=false: images returned as base64 data URIs in images; non-images parsed and returned as text.
//   - raw=true:  images returned as raw bytes in images; non-images parsed and returned as text.
func (s *FileService) GetFileContents(uid string, fileDicts []map[string]interface{}, raw bool) (texts []string, images []string, err error) {
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, nil, fmt.Errorf("storage not initialized")
	}

	for _, fd := range fileDicts {
		id, _ := fd["id"].(string)
		if id == "" {
			continue
		}
		file, ferr := s.fileDAO.GetByID(id)
		if ferr != nil || file == nil || file.Location == nil || *file.Location == "" {
			continue
		}
		if !s.checkFileTeamPermission(file, uid) {
			return nil, nil, fmt.Errorf("No authorization.")
		}
		data, derr := storageImpl.Get(file.ParentID, *file.Location)
		if derr != nil || len(data) == 0 {
			continue
		}
		ft := utility.FilenameType(file.Name)
		if ft == utility.FileTypeVISUAL {
			if raw {
				images = append(images, string(data))
			} else {
				ext := utility.GetFileExtension(file.Name)
				mime := utility.GetContentType(ext, string(ft))
				images = append(images, "data:"+mime+";base64,"+base64.StdEncoding.EncodeToString(data))
			}
		} else {
			texts = append(texts, parseFileContent(file.Name, data))
		}
	}
	return texts, images, nil
}

// parseFileContent tries to parse a file's contents using the appropriate parser.
// Falls back to returning raw text if no parser is available.
func parseFileContent(filename string, data []byte) string {
	fileType := utility.GetFileType(filename)
	if fileType == utility.FileTypeOTHER {
		return string(data)
	}
	// Parser config — office_oxide for MS Office formats; other parsers ignore it.
	parserCfg := map[string]string{"lib_type": "office_oxide"}
	fp, err := parser.GetParser(fileType, parserCfg)
	if err != nil {
		return string(data)
	}
	res := fp.ParseWithResult(filename, data)
	if res.Err != nil {
		return string(data)
	}
	switch res.OutputFormat {
	case "text":
		return res.Text
	case "markdown":
		return res.Markdown
	case "html":
		return res.HTML
	case "json":
		return string(data)
	default:
		return string(data)
	}
}

// toUploadInfoResponse converts a newly-uploaded file record to the shape
// Python's upload_info endpoint returns.
func (s *FileService) toUploadInfoResponse(file *entity.File, mimeType string) map[string]interface{} {
	ext := ""
	if idx := strings.LastIndex(file.Name, "."); idx >= 0 {
		ext = strings.ToLower(file.Name[idx+1:])
	}
	return map[string]interface{}{
		"id":          file.ID,
		"name":        file.Name,
		"size":        file.Size,
		"extension":   ext,
		"mime_type":   mimeType,
		"created_by":  file.CreatedBy,
		"created_at":  float64(time.Now().UnixMilli()) / 1000.0,
		"preview_url": nil,
	}
}

// maxRemoteFileSize bounds the body of a ?url= upload (100 MB).
const maxRemoteFileSize = 100 << 20

// UploadFromURL fetches a remote URL, saves the content to the tenant's root
// folder, and returns the file metadata map — mirroring Python
// FileService.upload_info(tenant_id, None, url).
//
// The remote fetch is SSRF-guarded (mirrors Python's assert_url_is_safe): the
// scheme must be http/https and every address the host resolves to must be
// globally routable; the validated IP is pinned for the actual connection — and
// re-validated on each redirect hop — to defeat DNS-rebinding. The HTTP client
// carries connect and overall timeouts, and the response body is bounded with
// truncation detection so an oversized file is rejected rather than silently
// clipped.
func (s *FileService) UploadFromURL(tenantID, rawURL string) (map[string]interface{}, error) {
	if rawURL == "" {
		return nil, fmt.Errorf("url is required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return nil, fmt.Errorf("invalid or unsafe URL")
	}

	data, headers, finalURL, err := fetchRemoteFileSafely(rawURL, maxRemoteFileSize)
	if err != nil {
		return nil, err
	}

	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	contentType := headers.Get("Content-Type")
	filename := normalizeRemoteUploadFilename(finalURL, contentType, data)
	if err := s.checkUploadInfoHealth(tenantID, filename); err != nil {
		return nil, err
	}
	filename, contentType, data = normalizeUploadInfoContent(filename, contentType, data)
	return s.storeUploadInfoBlob(storageImpl, tenantID, filename, contentType, data)
}

// fetchRemoteFileSafely downloads rawURL with SSRF protection, connect/overall
// timeouts, and a hard size cap that rejects (rather than truncates) oversized
// bodies.
func fetchRemoteFileSafely(rawURL string, maxSize int64) ([]byte, http.Header, string, error) {
	currentURL := rawURL
	for redirects := 0; redirects < 10; redirects++ {
		hostname, resolvedIP, err := assertURLSafe(currentURL)
		if err != nil {
			return nil, nil, "", err
		}
		client := pinnedHTTPClient(hostname, resolvedIP, 10*time.Second)
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}

		// runs assertURLSafe(currentURL) on every iteration (including
		// redirects), which rejects private/loopback IPs and other
		// SSRF targets. The "nosec G107" comment is for gosec;
		// CodeQL needs an explicit suppression.
		// codeql[go/request-forgery] False positive: the loop above
		resp, err := client.Get(currentURL) // #nosec G107
		if err != nil {
			return nil, nil, "", fmt.Errorf("failed to fetch URL: %w", err)
		}

		if resp.StatusCode == http.StatusMovedPermanently ||
			resp.StatusCode == http.StatusFound ||
			resp.StatusCode == http.StatusSeeOther ||
			resp.StatusCode == http.StatusTemporaryRedirect ||
			resp.StatusCode == http.StatusPermanentRedirect {
			location := resp.Header.Get("Location")
			resp.Body.Close()
			if location == "" {
				return nil, nil, "", fmt.Errorf("redirect response missing Location header")
			}
			baseURL, parseErr := url.Parse(currentURL)
			if parseErr != nil {
				return nil, nil, "", parseErr
			}
			nextURL, resolveErr := baseURL.Parse(location)
			if resolveErr != nil {
				return nil, nil, "", resolveErr
			}
			currentURL = nextURL.String()
			continue
		}

		if resp.StatusCode >= 400 {
			resp.Body.Close()
			return nil, nil, "", fmt.Errorf("remote URL returned HTTP %d", resp.StatusCode)
		}

		data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
		resp.Body.Close()
		if readErr != nil {
			return nil, nil, "", fmt.Errorf("failed to read remote content: %w", readErr)
		}
		if int64(len(data)) > maxSize {
			return nil, nil, "", fmt.Errorf("remote file exceeds the maximum allowed size of %d bytes", maxSize)
		}
		return data, resp.Header.Clone(), currentURL, nil
	}
	return nil, nil, "", fmt.Errorf("stopped after too many redirects")
}

// isPublicIP reports whether ip is a globally routable address. It mirrors the
// allowlist intent of Python's assert_url_is_safe (which requires ip.is_global)
// by rejecting loopback, private, link-local, multicast, unspecified, and
// carrier-grade NAT ranges. IPv4-mapped IPv6 addresses are handled by the
// stdlib predicates.
func isPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	// Carrier-grade NAT 100.64.0.0/10 (RFC 6598) — not covered by IsPrivate.
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 100 && ip4[1]&0xc0 == 0x40 {
		return false
	}
	return true
}

func (s *FileService) checkUploadInfoHealth(userID, filename string) error {
	if filename == "" {
		return fmt.Errorf("No file selected!")
	}
	maxFileNumPerUser := os.Getenv("MAX_FILE_NUM_PER_USER")
	if maxFileNumPerUser != "" {
		var maxNum int64
		if _, err := fmt.Sscanf(maxFileNumPerUser, "%d", &maxNum); err == nil && maxNum > 0 {
			docCount, err := s.GetDocCount(userID)
			if err != nil {
				return fmt.Errorf("failed to get document count: %w", err)
			}
			if docCount >= maxNum {
				return fmt.Errorf("Exceed the maximum file number of a free user!")
			}
		}
	}
	if len([]byte(filename)) > 255 {
		return fmt.Errorf("Exceed the maximum length of file name!")
	}
	return nil
}

func (s *FileService) storeUploadInfoBlob(storageImpl storage.Storage, userID, filename, contentType string, data []byte) (map[string]interface{}, error) {
	location := utility.GenerateUUID()
	bucket := fmt.Sprintf("%s-downloads", userID)
	if err := storageImpl.Put(bucket, location, data); err != nil {
		return nil, fmt.Errorf("failed to store file: %w", err)
	}
	ext := ""
	if idx := strings.LastIndex(filename, "."); idx >= 0 {
		ext = strings.ToLower(filename[idx+1:])
	}
	return map[string]interface{}{
		"id":          location,
		"name":        filename,
		"size":        int64(len(data)),
		"extension":   ext,
		"mime_type":   contentType,
		"created_by":  userID,
		"created_at":  float64(time.Now().UnixMilli()) / 1000.0,
		"preview_url": nil,
	}, nil
}

func normalizeRemoteUploadFilename(rawURL, contentType string, data []byte) string {
	parsed, err := url.Parse(rawURL)
	filename := "download"
	if err == nil {
		filename = sanitizeFilename(filepath.Base(parsed.Path))
	}
	ct := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if ct == "application/pdf" || bytesLooksLikePDF(data) {
		if !strings.HasSuffix(strings.ToLower(filename), ".pdf") {
			filename += ".pdf"
		}
	}
	return filename
}

func normalizeUploadInfoContent(filename, contentType string, data []byte) (string, string, []byte) {
	lowerCT := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if lowerCT == "" {
		lowerCT = http.DetectContentType(data)
	}

	if lowerCT == "application/pdf" || bytesLooksLikePDF(data) {
		if !strings.HasSuffix(strings.ToLower(filename), ".pdf") {
			filename += ".pdf"
		}
		lowerCT = "application/pdf"
	}
	if lowerCT == "text/html" || lowerCT == "application/xhtml+xml" || looksLikeHTML(data) {
		data = htmlToReadableMarkdown(data)
		if lowerCT == "" {
			lowerCT = "text/html"
		}
	}
	return filename, lowerCT, data
}

func bytesLooksLikePDF(data []byte) bool {
	return len(data) >= 4 && string(data[:4]) == "%PDF"
}

func looksLikeHTML(data []byte) bool {
	snippet := strings.ToLower(string(data))
	return strings.Contains(snippet, "<html") || strings.Contains(snippet, "<body") || strings.Contains(snippet, "<div")
}

var (
	htmlScriptStyleRE = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)
	htmlTagRE         = regexp.MustCompile(`(?s)<[^>]+>`)
	multiSpaceRE      = regexp.MustCompile(`[ \t]+`)
	multiNewlineRE    = regexp.MustCompile(`\n{3,}`)
)

func htmlToReadableMarkdown(data []byte) []byte {
	text := string(data)
	text = htmlScriptStyleRE.ReplaceAllString(text, " ")
	text = strings.ReplaceAll(text, "<br>", "\n")
	text = strings.ReplaceAll(text, "<br/>", "\n")
	text = strings.ReplaceAll(text, "<br />", "\n")
	text = strings.ReplaceAll(text, "</p>", "\n\n")
	text = strings.ReplaceAll(text, "</div>", "\n")
	text = strings.ReplaceAll(text, "</li>", "\n")
	text = htmlTagRE.ReplaceAllString(text, " ")
	text = html.UnescapeString(text)
	text = strings.ReplaceAll(text, "\r", "\n")
	text = multiSpaceRE.ReplaceAllString(text, " ")
	text = multiNewlineRE.ReplaceAllString(text, "\n\n")
	text = strings.TrimSpace(text)
	return []byte(text)
}

// reservedDeviceNames are Windows reserved filenames that must never be used.
var reservedDeviceNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// sanitizeFilename produces a safe, filesystem-friendly filename from an
// arbitrary URL path segment: it strips directory components, replaces unsafe /
// control characters, rejects reserved names, bounds the length, and falls back
// to "download".
func sanitizeFilename(name string) string {
	name = filepath.Base(name)
	name = strings.TrimSpace(name)

	name = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', 0:
			return '_'
		}
		if r < 0x20 { // control characters
			return '_'
		}
		return r
	}, name)

	// Strip leading/trailing dots and spaces to avoid hidden or reserved forms.
	name = strings.Trim(name, ". ")

	if name == "" || name == "." || name == ".." {
		return "download"
	}
	if stem := strings.SplitN(strings.ToUpper(name), ".", 2)[0]; reservedDeviceNames[stem] {
		return "download"
	}
	if len(name) > 255 {
		name = name[:255]
	}
	return name
}
