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

	"go.uber.org/zap"
)

// DeleteFiles deletes files and folders by ID after authorizing every entry in
// each selected folder tree.
func (s *FileService) DeleteFiles(ctx context.Context, userID string, fileIDs []string) error {
	files, err := s.fileDAO.GetByIDs(ctx, dao.DB, fileIDs)
	if err != nil {
		return err
	}
	filesByID := make(map[string]*entity.File, len(files))
	for _, file := range files {
		filesByID[file.ID] = file
	}

	for _, fileID := range fileIDs {
		file, ok := filesByID[fileID]
		if !ok {
			return errors.New("File or Folder not found!")
		}
		if file.TenantID == "" {
			return errors.New("Tenant not found!")
		}
		if file.ParentID == file.ID {
			return errors.New("Root folder cannot be deleted.")
		}
		if err := s.checkFileTreeAccess(ctx, userID, file, permission.OperationDelete); err != nil {
			return err
		}
	}

	for _, fileID := range fileIDs {
		file := filesByID[fileID]
		if file.SourceType == FileSourceDataset {
			continue
		}
		if file.Type == FileTypeFolder {
			if err := s.deleteFolderRecursive(ctx, file); err != nil {
				return fmt.Errorf("Failed to delete folder: %w", err)
			}
		} else if err := s.deleteSingleFile(ctx, file); err != nil {
			return fmt.Errorf("Failed to delete file: %w", err)
		}
	}
	return nil
}

// deleteSingleFile deletes a single file (not folder)
// Matches Python's _delete_single_file function
func (s *FileService) deleteSingleFile(ctx context.Context, file *entity.File) error {
	storageImpl := storage.GetStorageFactory().GetStorage()
	if err := removeFileObject(ctx, storageImpl, file); err != nil {
		return err
	}
	return s.deleteSingleFileRecords(ctx, file)
}

func removeFileObject(ctx context.Context, storageImpl storage.Storage, file *entity.File) error {
	if file.Location != nil && *file.Location != "" {
		if storageImpl == nil {
			return fmt.Errorf("storage is not configured for file %s", file.ID)
		}
		exists, err := storageImpl.ObjectExists(ctx, file.ParentID, *file.Location)
		if err != nil {
			return fmt.Errorf("check file %s: %w", file.ID, err)
		}
		if !exists {
			common.Warn("File object already missing", zap.String("file", fileNameAndID(file)), zap.String("file_id", file.ID), zap.String("bucket", file.ParentID))
			return nil
		}
		if err := storageImpl.Remove(ctx, file.ParentID, *file.Location); err != nil {
			return fmt.Errorf("remove file %s: %w", file.ID, err)
		}
		common.Info("Removed file object", zap.String("file", fileNameAndID(file)), zap.String("file_id", file.ID), zap.String("bucket", file.ParentID))
	}
	return nil
}

func (s *FileService) deleteSingleFileRecords(ctx context.Context, file *entity.File) error {
	// Handle associated documents
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
					return fmt.Errorf("remove document %s: %w", docID, err)
				}
			}
		}

		// Delete file2document mapping (outside the loop, called once - matching Python behavior)
		if err = s.file2DocumentDAO.DeleteByFileID(ctx, dao.DB, file.ID); err != nil {
			return fmt.Errorf("failed to delete file2document mapping: %w", err)
		}
	}

	// Delete file record
	if err = s.fileDAO.Delete(ctx, dao.DB, file.ID); err != nil {
		return err
	}
	common.Info("Deleted file", zap.String("file", fileNameAndID(file)), zap.String("file_id", file.ID))

	return nil
}

// deleteFolderRecursive recursively deletes a folder and its contents
// Matches Python's _delete_folder_recursive function
func (s *FileService) deleteFolderRecursive(ctx context.Context, folder *entity.File) error {
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return fmt.Errorf("storage is not configured for folder %s", folder.ID)
	}
	if err := s.removeFolderObjectsRecursive(ctx, folder, storageImpl); err != nil {
		return err
	}
	return s.deleteFolderRecordsRecursive(ctx, folder)
}

func (s *FileService) removeFolderObjectsRecursive(ctx context.Context, folder *entity.File, storageImpl storage.Storage) error {
	subFiles, err := s.fileDAO.ListByParentID(ctx, dao.DB, folder.ID)
	if err != nil {
		return err
	}
	for _, subFile := range subFiles {
		if subFile.Type == FileTypeFolder {
			if err = s.removeFolderObjectsRecursive(ctx, subFile, storageImpl); err != nil {
				return err
			}
		} else {
			if err = removeFileObject(ctx, storageImpl, subFile); err != nil {
				return err
			}
		}
	}
	exists, err := storageImpl.BucketExistsWithError(ctx, folder.ID)
	if err != nil {
		return fmt.Errorf("check folder bucket %s: %w", folder.ID, err)
	}
	if !exists {
		common.Warn("Folder bucket already missing", zap.String("folder", fileNameAndID(folder)), zap.String("bucket", folder.ID))
		return nil
	}
	if err := storageImpl.RemoveEmptyBucket(ctx, folder.ID); err != nil {
		common.Warn("Unable to remove empty folder bucket", zap.String("folder", fileNameAndID(folder)), zap.String("bucket", folder.ID), zap.Error(err))
	} else {
		common.Info("Removed empty folder bucket", zap.String("folder", fileNameAndID(folder)), zap.String("bucket", folder.ID))
	}
	return nil
}

func (s *FileService) deleteFolderRecordsRecursive(ctx context.Context, folder *entity.File) error {
	subFiles, err := s.fileDAO.ListByParentID(ctx, dao.DB, folder.ID)
	if err != nil {
		return err
	}
	for _, subFile := range subFiles {
		if subFile.Type == FileTypeFolder {
			if err = s.deleteFolderRecordsRecursive(ctx, subFile); err != nil {
				return err
			}
		} else if err = s.deleteSingleFileRecords(ctx, subFile); err != nil {
			return err
		}
	}
	if err = s.fileDAO.Delete(ctx, dao.DB, folder.ID); err != nil {
		return err
	}
	common.Info("Deleted folder", zap.String("folder", fileNameAndID(folder)), zap.String("folder_id", folder.ID))
	return nil
}

func fileNameAndID(file *entity.File) string {
	if file.Name == "" {
		return file.ID
	}
	return fmt.Sprintf("%s (%s)", file.Name, file.ID)
}
