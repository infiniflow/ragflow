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
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	"ragflow/internal/logger"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
	"strings"
	// "time"

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

// initDatasetDocs initializes dataset documents for tenant
// This matches Python's FileService.init_dataset_docs method
func (s *FileService) initDatasetDocs(rootID, tenantID string) error {
	return s.fileDAO.InitDatasetDocs(rootID, tenantID, s.file2DocumentDAO)
}

// DatasetFolderName is the folder name for dataset
const DatasetFolderName = ".knowledgebase"

// FileSourceDataset represents dataset as file source
const FileSourceDataset = "knowledgebase"

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
	userTenantDAO := dao.NewUserTenantDAO()

	for _, datasetID := range datasetIDs {
		ds, err := kbDAO.GetByID(datasetID)
		if err != nil || ds == nil {
			continue
		}

		// Check KB tenant permission
		if s.checkDatasetTeamPermission(ds, uid, userTenantDAO) {
			return true
		}
	}

	return false
}

// checkDatasetTeamPermission checks if user has permission to access the dataset
// Matches Python's check_kb_team_permission function
func (s *FileService) checkDatasetTeamPermission(ds *entity.Knowledgebase, uid string, userTenantDAO *dao.UserTenantDAO) bool {
	// KB's tenant directly authorized
	if ds.TenantID == uid {
		return true
	}

	// Check permission type
	permission := ds.Permission
	if permission != string(entity.TenantPermissionTeam) {
		return false
	}

	// Check if user joined the tenant
	joinedTenantIDs, err := userTenantDAO.GetTenantIDsByUserID(uid)
	if err != nil || len(joinedTenantIDs) == 0 {
		return false
	}

	for _, tenantID := range joinedTenantIDs {
		if tenantID == ds.TenantID {
			return true
		}
	}

	return false
}

// deleteSingleFile deletes a single file (not folder)
// Matches Python's _delete_single_file function
func (s *FileService) deleteSingleFile(ctx context.Context, file *entity.File) error {
	// 1. Delete storage object
	if file.Location != nil && *file.Location != "" {
		storageImpl := storage.GetStorageFactory().GetStorage()
		if storageImpl != nil {
			if err := storageImpl.Remove(file.ParentID, *file.Location); err != nil {
				logger.Logger.Error(fmt.Sprintf("Fail to remove object: %s/%s, error: %v", file.ParentID, *file.Location, err))
			}
		}
	}

	// 2. Handle associated documents
	informs, err := s.file2DocumentDAO.GetByFileID(file.ID)
	if err == nil && len(informs) > 0 {
		documentDAO := dao.NewDocumentDAO()
		datasetDAO := dao.NewKnowledgebaseDAO()

		for _, inform := range informs {
			if inform.DocumentID == nil {
				continue
			}
			docID := *inform.DocumentID

			doc, err := documentDAO.GetByID(docID)
			if err == nil && doc != nil {
				// Get tenant ID from KB
				ds, err := datasetDAO.GetByID(doc.KbID)
				if err == nil && ds != nil {
					tenantID := ds.TenantID
					if tenantID != "" {
						// Delete from document engine
						if err := s.deleteDocumentFromEngine(ctx, doc, tenantID); err != nil {
							logger.Logger.Error(fmt.Sprintf("Fail to delete document from engine: %s, error: %v", doc.ID, err))
						}
					}
				}

				// Delete document record
				if err := documentDAO.Delete(docID); err != nil {
					logger.Logger.Error(fmt.Sprintf("Fail to delete document: %s, error: %v", docID, err))
				}
			}

		}

		// Delete file2document mapping (outside the loop, called once - matching Python behavior)
		s.file2DocumentDAO.DeleteByFileID(file.ID)
	}

	// 3. Delete file record (unconditional, matching Python)
	if err := s.fileDAO.Delete(file.ID); err != nil {
		return err
	}

	return nil
}

// deleteDocumentFromEngine deletes a document from the document engine
func (s *FileService) deleteDocumentFromEngine(ctx context.Context, doc *entity.Document, tenantID string) error {
	// Get document engine
	docEngine := engine.Get()
	if docEngine == nil {
		return nil
	}

	// Build index name: ragflow_<tenant_id>_<kb_id>
	indexName := fmt.Sprintf("ragflow_%s_%s", tenantID, doc.KbID)


	// Delete document from engine with timeout
	// reqCtx, cancel := context.WithTimeout(ctx, 300*time.Second)
	// defer cancel()
	if err := docEngine.DeleteDocument(ctx, indexName, doc.ID); err != nil {
		return fmt.Errorf("delete document from engine: %w", err)
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
		existingFiles := s.fileDAO.Query(newName, targetParentID)
		for _, f := range existingFiles {
			if f.Name == newName {
				return false, "Duplicated file name in the same folder."
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
		if len(srcFileIDs) == 0 {
			return false, "Source files not found!"
		}
		file := filesMap[srcFileIDs[0]]
		if !s.fileDAO.UpdateByID(file.ID, map[string]interface{}{"name": newName}) {
			return false, "Database error (File rename)!"
		}

		// Update associated document name if exists
		informs, err := s.file2DocumentDAO.GetByFileID(file.ID)
		if err == nil && len(informs) > 0 && informs[0].DocumentID != nil {
			docID := *informs[0].DocumentID
			documentDAO := dao.NewDocumentDAO()
			if !documentDAO.UpdateByID(docID, map[string]interface{}{"name": newName}) {
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
		existingFolders := s.fileDAO.Query(effectiveName, destFolder.ID)
		var newFolder *entity.File
		if len(existingFolders) > 0 {
			newFolder = existingFolders[0]
		} else {
			// Create new folder
			newFolder, _ = s.fileDAO.CreateFolder(destFolder.ID, sourceFile.TenantID, effectiveName, FileTypeFolder)
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
		if !s.fileDAO.UpdateByID(sourceFile.ID, updates) {
			return fmt.Errorf("database error (File update)")
		}
	}

	// Update associated document name if renamed
	if overrideName != "" {
		informs, err := s.file2DocumentDAO.GetByFileID(sourceFile.ID)
		if err == nil && len(informs) > 0 && informs[0].DocumentID != nil {
			docID := *informs[0].DocumentID
			documentDAO := dao.NewDocumentDAO()
			if !documentDAO.UpdateByID(docID, map[string]interface{}{"name": overrideName}) {
				return fmt.Errorf("database error (Document rename)")
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
