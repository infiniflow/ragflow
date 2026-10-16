package file

import (
	"context"
	"errors"
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
	"strings"

	"gorm.io/gorm"
)

// GetRootFolder gets or creates root folder for tenant
func (s *FileService) GetRootFolder(ctx context.Context, tenantID string) (map[string]interface{}, error) {
	file, err := s.fileDAO.GetRootFolder(ctx, dao.DB, tenantID)
	if err != nil {
		return nil, err
	}
	if err := checkFileAccess(ctx, tenantID, file, permission.OperationRead); err != nil {
		return nil, err
	}
	return s.toFileResponse(file), nil
}

// lookupError classifies a file lookup failure: a missing row is a client-facing
// not-found (CodeNotFound), while any other error (connection loss, timeout,
// etc.) is returned unchanged so handlers keep reporting it as a server error.
func lookupError(err error, message string) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return common.NewCodedError(common.CodeNotFound, message)
	}
	return err
}

// ListFiles lists files by parent folder ID (matching Python /files endpoint)
// This method includes init_dataset_docs initialization when parent_id is empty
func (s *FileService) ListFiles(ctx context.Context, tenantID, pfID string, page, pageSize int, terms []dao.OrderTerm, keywords string) (*ListFilesResponse, error) {
	return s.listFiles(ctx, tenantID, tenantID, pfID, page, pageSize, terms, keywords, true)
}

// ListFilesForTenant allows active tenant members to browse the tenant's files.
func (s *FileService) ListFilesForTenant(ctx context.Context, userID, tenantID, pfID string, page, pageSize int, terms []dao.OrderTerm, keywords string) (*ListFilesResponse, error) {
	checkFilePermissions := userID == tenantID
	if !checkFilePermissions {
		if err := permission.NewDatabaseChecker(dao.DB).CheckTenant(ctx, permission.Subject{UserID: userID}, tenantID, permission.TenantMember); err != nil {
			return nil, err
		}
	}
	return s.listFiles(ctx, userID, tenantID, pfID, page, pageSize, terms, keywords, checkFilePermissions)
}

func (s *FileService) listFiles(ctx context.Context, userID, tenantID, pfID string, page, pageSize int, terms []dao.OrderTerm, keywords string, checkFilePermissions bool) (*ListFilesResponse, error) {
	if tenantID == "" {
		return nil, permission.ErrInvalidSubject
	}
	// If pfID is empty, get root folder and initialize dataset docs
	if pfID == "" {
		rootFolder, err := s.fileDAO.GetRootFolder(ctx, dao.DB, tenantID)
		if err != nil {
			return nil, fmt.Errorf("failed to get root folder: %w", err)
		}
		if checkFilePermissions {
			if err := checkFileAccess(ctx, userID, rootFolder, permission.OperationRead); err != nil {
				return nil, err
			}
		}
		pfID = rootFolder.ID

		// Keep tenant-scoped lookups read-only. The default self-owned file
		// listing retains its existing folder initialization.
		if tenantID == userID {
			// Initialize dataset docs (matching Python init_knowledgebase_docs logic)
			if err = s.initDatasetDocs(ctx, pfID, tenantID); err != nil {
				return nil, fmt.Errorf("failed to initialize dataset docs: %w", err)
			}

			// Initialize skills folder (matching Python init_skills_folder logic)
			if err = s.initSkillsFolder(ctx, pfID, tenantID); err != nil {
				return nil, fmt.Errorf("failed to initialize skills folder: %w", err)
			}
		}
	}

	// Check if parent folder exists
	folder, err := s.fileDAO.GetByID(ctx, dao.DB, pfID)
	if err != nil {
		return nil, lookupError(err, "folder not found")
	}
	if folder.Type != FileTypeFolder {
		return nil, permission.ErrResourceNotFound
	}
	if checkFilePermissions {
		if err := checkFileAccess(ctx, userID, folder, permission.OperationRead); err != nil {
			return nil, err
		}
	}
	if folder.TenantID != tenantID {
		return nil, permission.ErrResourceNotFound
	}

	// Get files by parent folder ID
	excludeSkills := folder.ID == folder.ParentID
	files, total, err := s.fileDAO.GetByPfID(ctx, dao.DB, tenantID, pfID, page, pageSize, terms, keywords, excludeSkills)
	if err != nil {
		return nil, err
	}

	// Get parent folder
	parentFolder, err := s.fileDAO.GetParentFolder(ctx, dao.DB, pfID)
	if err != nil {
		return nil, fmt.Errorf("folder not found")
	}
	if parentFolder.TenantID != tenantID {
		return nil, permission.ErrResourceNotFound
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
			folderSize, err := s.fileDAO.GetFolderSize(ctx, dao.DB, file.ID)
			if err == nil {
				fileInfo.Size = folderSize
			}
			hasChild, err := s.fileDAO.HasChildFolder(ctx, dao.DB, file.ID)
			if err == nil {
				fileInfo.HasChildFolder = hasChild
			}
			fileInfo.KbsInfo = []map[string]interface{}{}
		} else {
			// Get KB info for non-folder files
			kbsInfo, err := s.file2DocumentDAO.GetKBInfoByFileID(ctx, dao.DB, file.ID)
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
func (s *FileService) initDatasetDocs(ctx context.Context, rootID, tenantID string) error {
	return s.fileDAO.InitDatasetDocs(ctx, dao.DB, rootID, tenantID, s.file2DocumentDAO)
}

// initSkillsFolder initializes the skills folder under the root folder.
// Deduplicates duplicate entries that may have been created by
// concurrent race conditions (TOCTOU).
func (s *FileService) initSkillsFolder(ctx context.Context, rootID, tenantID string) error {
	existing, err := s.fileDAO.Query(ctx, dao.DB, SkillsFolderName, rootID, tenantID)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		if len(existing) > 1 {
			common.Logger.Warn(fmt.Sprintf(
				"Found %d duplicate '%s' folders under root %s, keeping only the first",
				len(existing), SkillsFolderName, rootID,
			))
			keepID := existing[0].ID
			for _, dup := range existing[1:] {
				children, _ := s.fileDAO.ListAllFilesByParentID(ctx, dao.DB, dup.ID)
				for _, child := range children {
					if err := s.fileDAO.UpdateByID(ctx, dao.DB, child.ID, map[string]interface{}{"parent_id": keepID}); err != nil {
						common.Logger.Warn(fmt.Sprintf("Failed to update child folder %s: %v", child.ID, err))
					}
				}
				if err := s.fileDAO.Delete(ctx, dao.DB, dup.ID); err != nil {
					common.Logger.Warn(fmt.Sprintf("Failed to delete duplicate skills folder %s: %v", dup.ID, err))
				}
			}
		}
		return nil
	}

	folder := &entity.File{
		ID:         common.GenerateToken(),
		ParentID:   rootID,
		TenantID:   tenantID,
		CreatedBy:  tenantID,
		Name:       SkillsFolderName,
		Type:       FileTypeFolder,
		Size:       0,
		SourceType: "",
	}
	return s.fileDAO.Insert(ctx, dao.DB, folder)
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
func (s *FileService) GetParentFolder(ctx context.Context, userID, fileID string) (map[string]interface{}, error) {
	// Get file
	file, err := s.fileDAO.GetByID(ctx, dao.DB, fileID)
	if err != nil {
		return nil, lookupError(err, "file or folder not found")
	}

	// Permission check
	if err := checkFileAccess(ctx, userID, file, permission.OperationRead); err != nil {
		return nil, err
	}

	// Get parent folder
	parentFolder, err := s.fileDAO.GetParentFolder(ctx, dao.DB, fileID)
	if err != nil {
		return nil, err
	}
	if err := checkFileAccess(ctx, userID, parentFolder, permission.OperationRead); err != nil {
		return nil, err
	}

	return s.toFileResponse(parentFolder), nil
}

// GetAllParentFolders gets all parent folders in path with permission check
func (s *FileService) GetAllParentFolders(ctx context.Context, userID, fileID string) ([]map[string]interface{}, error) {
	return s.getAllParentFolders(ctx, userID, "", fileID, true)
}

// GetAllParentFoldersForTenant allows active tenant members to resolve file paths.
// The file and every ancestor must belong to tenantID.
func (s *FileService) GetAllParentFoldersForTenant(ctx context.Context, userID, tenantID, fileID string) ([]map[string]interface{}, error) {
	if tenantID == "" {
		return nil, permission.ErrInvalidSubject
	}
	checkFilePermissions := userID == tenantID
	if !checkFilePermissions {
		if err := permission.NewDatabaseChecker(dao.DB).CheckTenant(ctx, permission.Subject{UserID: userID}, tenantID, permission.TenantMember); err != nil {
			return nil, err
		}
	}
	return s.getAllParentFolders(ctx, userID, tenantID, fileID, checkFilePermissions)
}

func (s *FileService) getAllParentFolders(ctx context.Context, userID, tenantID, fileID string, checkFilePermissions bool) ([]map[string]interface{}, error) {
	// Get file
	file, err := s.fileDAO.GetByID(ctx, dao.DB, fileID)
	if err != nil {
		return nil, lookupError(err, "file or folder not found")
	}
	if tenantID != "" && file.TenantID != tenantID {
		return nil, permission.ErrResourceNotFound
	}

	// Permission check
	if checkFilePermissions {
		if err := checkFileAccess(ctx, userID, file, permission.OperationRead); err != nil {
			return nil, err
		}
	}

	// Get all parent folders
	parentFolders, err := s.fileDAO.GetAllParentFolders(ctx, dao.DB, fileID)
	if err != nil {
		return nil, err
	}
	for _, folder := range parentFolders {
		if tenantID != "" && folder.TenantID != tenantID {
			return nil, permission.ErrResourceNotFound
		}
		if checkFilePermissions {
			if err := checkFileAccess(ctx, userID, folder, permission.OperationRead); err != nil {
				return nil, err
			}
		}
	}

	// Convert to response format
	result := make([]map[string]interface{}, len(parentFolders))
	for i, folder := range parentFolders {
		result[i] = s.toFileResponse(folder)
	}

	return result, nil
}

// GetDocCount gets document count for a tenant
func (s *FileService) GetDocCount(ctx context.Context, tenantID string) (int64, error) {
	documentDAO := dao.NewDocumentDAO()
	return documentDAO.CountByTenantID(ctx, dao.DB, tenantID)
}

func (s *FileService) createFolderRecursive(ctx context.Context, parentFolder *entity.File, names []string, count int, tenantID string) (*entity.File, error) {
	if count > len(names)-2 {
		return parentFolder, nil
	}

	newFolder, err := s.fileDAO.CreateFolder(ctx, dao.DB, parentFolder.ID, tenantID, names[count], FileTypeFolder)
	if err != nil {
		return nil, err
	}

	return s.createFolderRecursive(ctx, newFolder, names, count+1, tenantID)
}

// getUniqueFilename returns a non-colliding file name within the folder,
// appending (1), (2), ... when the requested name is already taken.
func (s *FileService) getUniqueFilename(ctx context.Context, name, parentID, tenantID string) (string, error) {
	return common.UniqueFileName(name, 255, func(candidate string) (bool, error) {
		return s.fileDAO.NameExists(ctx, dao.DB, candidate, parentID, tenantID, "")
	})
}

// CreateFolder creates a new folder or virtual file
func (s *FileService) CreateFolder(ctx context.Context, tenantID, name, parentID, fileType string) (map[string]interface{}, error) {
	// "/" is the root folder name and the path separator used by recursive
	// folder creation, so names containing it collide with the root folder
	// and break path-based lookups.
	if strings.Contains(name, "/") {
		return nil, errors.New(`Folder name cannot contain "/"`)
	}

	if parentID == "" {
		rootFolder, err := s.fileDAO.GetRootFolder(ctx, dao.DB, tenantID)
		if err != nil {
			return nil, fmt.Errorf("failed to get root folder: %w", err)
		}
		parentID = rootFolder.ID
	}

	parentFolder, err := s.fileDAO.GetByID(ctx, dao.DB, parentID)
	if err != nil || parentFolder == nil || parentFolder.Type != FileTypeFolder {
		return nil, fmt.Errorf("parent folder not found")
	}
	if err := checkFileAccess(ctx, tenantID, parentFolder, permission.OperationCreate); err != nil {
		return nil, err
	}

	if fileType == "" {
		fileType = FileTypeVirtual
	}
	if fileType != FileTypeFolder {
		fileType = FileTypeVirtual
	}

	existsInParent := func(candidate string) (bool, error) {
		return s.fileDAO.NameExists(ctx, dao.DB, candidate, parentID, tenantID, "")
	}
	var uniqueName string
	if fileType == FileTypeFolder {
		// A folder is not a file: append the counter to the whole name.
		uniqueName, err = common.UniqueName(name, 255, existsInParent)
	} else {
		uniqueName, err = common.UniqueFileName(name, 255, existsInParent)
	}
	if err != nil {
		return nil, err
	}
	name = uniqueName

	folder, err := s.fileDAO.CreateFolder(ctx, dao.DB, parentID, tenantID, name, fileType)
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
func (s *FileService) MoveFiles(ctx context.Context, userID string, srcFileIDs []string, destFileID string, newName string) error {
	files, err := s.fileDAO.GetByIDs(ctx, dao.DB, srcFileIDs)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return errors.New("source files not found")
	}
	filesByID := make(map[string]*entity.File, len(files))
	for _, file := range files {
		filesByID[file.ID] = file
	}

	for _, fileID := range srcFileIDs {
		file, ok := filesByID[fileID]
		if !ok {
			return errors.New("file or folder not found")
		}
		if file.TenantID == "" {
			return errors.New("tenant not found")
		}
		if err := s.checkFileTreeAccess(ctx, userID, file, permission.OperationUpdate); err != nil {
			return err
		}
	}

	if ok, msg := s.checkSourcesDisjoint(ctx, files); !ok {
		return errors.New(msg)
	}

	var destFolder *entity.File
	if destFileID != "" {
		destFolder, err = s.fileDAO.GetByID(ctx, dao.DB, destFileID)
		if err != nil || destFolder == nil || destFolder.Type != FileTypeFolder {
			return errors.New("parent folder not found")
		}
		if err := checkFileAccess(ctx, userID, destFolder, permission.OperationCreate); err != nil {
			return err
		}

		destAncestors, err := s.fileDAO.GetAllParentFolders(ctx, dao.DB, destFolder.ID)
		if err != nil {
			return errors.New("parent folder not found")
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
				return errors.New("cannot move a folder to itself")
			}
			if _, ok := destAncestorIDs[file.ID]; ok {
				return errors.New("cannot move a folder into its own subfolder")
			}
		}
	}

	if newName != "" {
		if len(srcFileIDs) > 1 {
			return errors.New("new name can only be used with a single file")
		}
		if strings.Contains(newName, "/") {
			return errors.New(`Name cannot contain "/"`)
		}

		file := filesByID[srcFileIDs[0]]
		if file.Type != FileTypeFolder && utility.GetFileExtension(file.Name) != utility.GetFileExtension(newName) {
			return errors.New("The extension of file can't be changed")
		}

		targetParentID := file.ParentID
		if destFolder != nil {
			targetParentID = destFolder.ID
		}
		existsInTarget := func(candidate string) (bool, error) {
			return s.fileDAO.NameExists(ctx, dao.DB, candidate, targetParentID, file.TenantID, file.ID)
		}
		if targetParentID == file.ParentID {
			available, err := common.NameAvailable(file.Name, newName, existsInTarget)
			if err != nil {
				return fmt.Errorf("failed to query existing files: %w", err)
			}
			if !available {
				return errors.New("duplicated file name in the same folder")
			}
		} else {
			taken, err := existsInTarget(newName)
			if err != nil {
				return fmt.Errorf("failed to query existing files: %w", err)
			}
			if taken {
				return errors.New("duplicated file name in the same folder")
			}
		}
	} else if destFolder != nil {
		for _, file := range files {
			exists, err := s.fileDAO.NameExists(ctx, dao.DB, file.Name, destFolder.ID, file.TenantID, file.ID)
			if err != nil {
				return fmt.Errorf("failed to query existing files: %w", err)
			}
			if exists {
				return errors.New("Duplicated file name in the same folder.")
			}
		}
	}

	if destFolder != nil {
		for _, fileID := range srcFileIDs {
			if err := s.moveEntryRecursive(ctx, userID, filesByID[fileID], destFolder, newName); err != nil {
				return err
			}
		}
		return nil
	}
	if newName == "" {
		return errors.New("new_name is required for rename")
	}
	if len(srcFileIDs) == 0 {
		return errors.New("Source files not found!")
	}
	file := filesByID[srcFileIDs[0]]
	if err := s.fileDAO.UpdateByID(ctx, dao.DB, file.ID, map[string]interface{}{"name": newName}); err != nil {
		return errors.New("Database error (File rename)!")
	}
	if err := s.renameLinkedDocuments(ctx, file.ID, newName); err != nil {
		return errors.New("Database error (Document rename)!")
	}
	return nil
}

// checkSourcesDisjoint reports false with a message when a selected source
// entry is an ancestor or descendant of another selected source entry.
// moveEntryRecursive already relocates and deletes an entire subtree, so a
// descendant selected alongside its ancestor would otherwise be processed
// twice, recreating a duplicate folder or moving an already-removed object.
func (s *FileService) checkSourcesDisjoint(ctx context.Context, files []*entity.File) (bool, string) {
	if len(files) < 2 {
		return true, ""
	}

	srcIDs := make(map[string]struct{}, len(files))
	for _, f := range files {
		srcIDs[f.ID] = struct{}{}
	}

	for _, file := range files {
		ancestors, err := s.fileDAO.GetAllParentFolders(ctx, dao.DB, file.ID)
		if err != nil {
			return false, "failed to check source folder hierarchy"
		}
		for _, ancestor := range ancestors {
			if ancestor.ID == file.ID {
				// GetAllParentFolders includes the entry itself.
				continue
			}
			if _, ok := srcIDs[ancestor.ID]; ok {
				return false, "cannot move a folder together with its descendant"
			}
		}
	}

	return true, ""
}

// renameLinkedDocuments renames every knowledgebase document linked to the
// given file so the new name is reflected in all linked datasets. Only the
// document name is synced: the Python reference (_rename_linked_documents in
// api/apps/services/file_api_service.py) updates no other denormalized field
// on rename.
func (s *FileService) renameLinkedDocuments(ctx context.Context, fileID, newName string) error {
	informs, err := s.file2DocumentDAO.GetByFileID(ctx, dao.DB, fileID)
	if err != nil {
		return err
	}
	documentDAO := dao.NewDocumentDAO()
	for _, inform := range informs {
		if inform.DocumentID == nil {
			continue
		}
		if err := documentDAO.UpdateByID(ctx, dao.DB, *inform.DocumentID, map[string]interface{}{"name": newName}); err != nil {
			return err
		}
	}
	return nil
}

// moveEntryRecursive recursively moves a file or folder entry
func (s *FileService) moveEntryRecursive(ctx context.Context, userID string, sourceFile *entity.File, destFolder *entity.File, overrideName string) error {
	effectiveName := overrideName
	if effectiveName == "" {
		effectiveName = sourceFile.Name
	}

	if sourceFile.Type == FileTypeFolder {
		// Handle folder move
		existingFolders, err := s.fileDAO.Query(ctx, dao.DB, effectiveName, destFolder.ID, sourceFile.TenantID)
		if err != nil {
			return fmt.Errorf("failed to query existing folders: %w", err)
		}
		var newFolder *entity.File
		if len(existingFolders) > 0 {
			// Prevent moving a folder into itself (self-target merge)
			if existingFolders[0].ID == sourceFile.ID {
				return fmt.Errorf("cannot move folder into itself")
			}
			newFolder = existingFolders[0]
			if err := checkFileAccess(ctx, userID, newFolder, permission.OperationCreate); err != nil {
				return err
			}
		} else {
			// Create new folder
			var err error
			newFolder, err = s.fileDAO.CreateFolder(ctx, dao.DB, destFolder.ID, sourceFile.TenantID, effectiveName, FileTypeFolder)
			if err != nil {
				return fmt.Errorf("failed to create destination folder: %w", err)
			}
		}

		// Recursively move sub-files
		subFiles, err := s.fileDAO.ListAllFilesByParentID(ctx, dao.DB, sourceFile.ID)
		if err != nil {
			return err
		}
		for _, subFile := range subFiles {
			if err = s.moveEntryRecursive(ctx, userID, subFile, newFolder, ""); err != nil {
				return err
			}
		}

		// Delete the source folder
		return s.fileDAO.Delete(ctx, dao.DB, sourceFile.ID)
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
		for storageImpl.ObjExist(ctx, destFolder.ID, newLocation) {
			newLocation += "_"
		}

		// Perform storage move (copy + delete)
		if sourceFile.Location == nil || *sourceFile.Location == "" {
			return fmt.Errorf("file location is empty")
		}

		if !storageImpl.Move(ctx, sourceFile.ParentID, *sourceFile.Location, destFolder.ID, newLocation) {
			return fmt.Errorf("move file failed at storage layer")
		}

		updates["parent_id"] = destFolder.ID
		updates["location"] = newLocation
	}

	if overrideName != "" {
		updates["name"] = overrideName
	}

	if len(updates) > 0 {
		if err := s.fileDAO.UpdateByID(ctx, dao.DB, sourceFile.ID, updates); err != nil {
			return fmt.Errorf("database error (File update): %w", err)
		}
	}

	// Update names of all linked documents if renamed
	if overrideName != "" {
		if err := s.renameLinkedDocuments(ctx, sourceFile.ID, overrideName); err != nil {
			return fmt.Errorf("database error (Document rename): %w", err)
		}
	}

	return nil
}
