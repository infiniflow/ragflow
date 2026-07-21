package file

import (
	"fmt"
	"path/filepath"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
	"strings"
)

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
	if !s.checkFilePerm(s.fileDAO, file, userID) {
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
	if !s.checkFilePerm(s.fileDAO, file, userID) {
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

// GetDocCount gets document count for a tenant
func (s *FileService) GetDocCount(tenantID string) (int64, error) {
	documentDAO := dao.NewDocumentDAO()
	return documentDAO.CountByTenantID(tenantID)
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
		if !s.checkFilePerm(s.fileDAO, file, uid) {
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
		if !s.checkFilePerm(s.fileDAO, destFolder, uid) {
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
