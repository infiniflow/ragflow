package file

import (
	"context"
	"errors"
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
)

// DeleteFiles deletes files by IDs
// Returns (success, message) where success is true if all files were deleted
func (s *FileService) DeleteFiles(ctx context.Context, uid string, fileIDs []string) (bool, string) {
	for _, fileID := range fileIDs {
		// 1. Get file
		file, err := s.fileDAO.GetByID(ctx, dao.DB, fileID)
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
		if !s.checkFilePerm(ctx, s.fileDAO, file, uid) {
			return false, "No authorization."
		}

		// 4. Skip dataset source files
		if file.SourceType == FileSourceDataset {
			continue
		}

		// 5. Delete based on type
		if file.Type == FileTypeFolder {
			if err = s.deleteFolderRecursive(ctx, file, uid); err != nil {
				return false, fmt.Sprintf("Failed to delete folder: %v", err)
			}
		} else {
			if err = s.deleteSingleFile(ctx, file); err != nil {
				return false, fmt.Sprintf("Failed to delete file: %v", err)
			}
		}
	}

	return true, ""
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
	informs, err := s.file2DocumentDAO.GetByFileID(ctx, dao.DB, file.ID)
	if err != nil {
		return fmt.Errorf("failed to get file2document mappings: %w", err)
	}
	if len(informs) > 0 {
		for _, inform := range informs {
			if inform.DocumentID == nil {
				continue
			}
			docID := *inform.DocumentID
			if s.documentService != nil {
				if err = s.documentService.RemoveDocumentKeepFile(ctx, docID); err != nil {
					if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
						return fmt.Errorf("context cancelled while removing document %s: %w", docID, err)
					}
					common.Logger.Error(fmt.Sprintf("Fail to remove document: %s, error: %v", docID, err))
				}
			}
		}

		// Delete file2document mapping (outside the loop, called once - matching Python behavior)
		if err = s.file2DocumentDAO.DeleteByFileID(ctx, dao.DB, file.ID); err != nil {
			return fmt.Errorf("failed to delete file2document mapping: %w", err)
		}
	}

	// 3. Delete file record
	if err = s.fileDAO.Delete(ctx, dao.DB, file.ID); err != nil {
		return err
	}

	return nil
}

// deleteFolderRecursive recursively deletes a folder and its contents
// Matches Python's _delete_folder_recursive function
func (s *FileService) deleteFolderRecursive(ctx context.Context, folder *entity.File, uid string) error {
	// Get all sub-files
	subFiles, err := s.fileDAO.ListByParentID(ctx, dao.DB, folder.ID)
	if err != nil {
		return err
	}

	for _, subFile := range subFiles {
		if subFile.Type == FileTypeFolder {
			// Recursively delete subfolder
			if err = s.deleteFolderRecursive(ctx, subFile, uid); err != nil {
				return err
			}
		} else {
			// Delete single file
			if err = s.deleteSingleFile(ctx, subFile); err != nil {
				return err
			}
		}
	}

	// Delete the folder itself
	if err = s.fileDAO.Delete(ctx, dao.DB, folder.ID); err != nil {
		return err
	}

	return nil
}
