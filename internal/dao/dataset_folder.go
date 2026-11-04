package dao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// ErrDatasetFolderConflict indicates that a folder cannot be associated with one dataset.
var ErrDatasetFolderConflict = errors.New("dataset folder association is ambiguous or conflicts with existing records")

// isUnreferencedEmptyFolder preserves bindings and history when reclaiming a folder.
func isUnreferencedEmptyFolder(ctx context.Context, db *gorm.DB, folderID string) (bool, error) {
	hasRows := func(q *gorm.DB) (bool, error) {
		var ids []string
		err := q.WithContext(ctx).Limit(1).Pluck("id", &ids).Error
		return len(ids) != 0, err
	}
	queries := []*gorm.DB{
		db.Model(&entity.File{}).Where("parent_id = ?", folderID),
		db.Model(&entity.File2Document{}).Where("file_id = ?", folderID),
		db.Model(&entity.FileCommit{}).Where("folder_id = ?", folderID),
		db.Model(&entity.FileCommitItem{}).Where("file_id = ?", folderID),
	}
	for _, q := range queries {
		found, err := hasRows(q)
		if err != nil {
			return false, err
		}
		if found {
			return false, nil
		}
	}
	if db.Migrator().HasTable(&entity.SkillSpace{}) {
		found, err := hasRows(db.Model(&entity.SkillSpace{}).Where("folder_id = ?", folderID))
		if err != nil {
			return false, err
		}
		if found {
			return false, nil
		}
	}
	token, err := json.Marshal(folderID)
	if err != nil {
		return false, err
	}
	pattern := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(string(token)) + "%"
	found, err := hasRows(db.Model(&entity.FileCommit{}).Where("tree_state LIKE ? ESCAPE '!'", pattern))
	if err != nil {
		return false, err
	}
	return !found, nil
}

func findDatasetChild(ctx context.Context, db *gorm.DB, parentID, tenantID, name string) (*entity.File, error) {
	var files []entity.File
	if err := db.WithContext(ctx).Where("parent_id = ? AND tenant_id = ? AND LOWER(name) = LOWER(?)", parentID, tenantID, name).Limit(2).Find(&files).Error; err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, nil
	}
	if len(files) != 1 || files[0].Type != "folder" || (files[0].SourceType != "" && files[0].SourceType != string(entity.FileSourceKnowledgebase)) {
		return nil, ErrDatasetFolderConflict
	}
	return &files[0], nil
}

func datasetRoot(ctx context.Context, db *gorm.DB, tenantID string, create bool) (*entity.File, error) {
	var roots []entity.File
	if err := db.WithContext(ctx).Where("tenant_id = ? AND parent_id = id", tenantID).Limit(2).Find(&roots).Error; err != nil {
		return nil, err
	}
	var root *entity.File
	if len(roots) > 1 {
		return nil, ErrDatasetFolderConflict
	}
	if len(roots) == 0 {
		if !create {
			return nil, nil
		}
		id := common.GenerateToken()
		root = &entity.File{ID: id, ParentID: id, TenantID: tenantID, CreatedBy: tenantID, Name: "/", Type: "folder"}
		if err := db.WithContext(ctx).Create(root).Error; err != nil {
			return nil, err
		}
	} else {
		root = &roots[0]
		if root.Type != "folder" {
			return nil, ErrDatasetFolderConflict
		}
	}
	parent, err := findDatasetChild(ctx, db, root.ID, tenantID, DatasetFolderName)
	if err != nil {
		return nil, err
	}
	if parent != nil || !create {
		return parent, nil
	}
	parent = &entity.File{ID: common.GenerateToken(), ParentID: root.ID, TenantID: tenantID, CreatedBy: tenantID, Name: DatasetFolderName, Type: "folder", SourceType: string(entity.FileSourceKnowledgebase)}
	if err := db.WithContext(ctx).Create(parent).Error; err != nil {
		return nil, err
	}
	return parent, nil
}

func checkDatasetFolder(ctx context.Context, db *gorm.DB, kb *entity.Knowledgebase, folder *entity.File) error {
	descendants, err := NewFileDAO().ListDescendants(ctx, db, folder.ID)
	if err != nil {
		return err
	}
	if len(descendants) == 0 {
		available, err := isUnreferencedEmptyFolder(ctx, db, folder.ID)
		if err != nil {
			return err
		}
		if !available {
			return ErrDatasetFolderConflict
		}
		return nil
	}
	var direct int64
	if err := db.WithContext(ctx).Model(&entity.File2Document{}).Where("file_id = ?", folder.ID).Count(&direct).Error; err != nil {
		return err
	}
	if direct != 0 {
		return ErrDatasetFolderConflict
	}
	if db.Migrator().HasTable(&entity.SkillSpace{}) {
		var count int64
		if err := db.WithContext(ctx).Model(&entity.SkillSpace{}).Where("folder_id = ?", folder.ID).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return ErrDatasetFolderConflict
		}
	}
	const batchSize = 500
	for start := 0; start < len(descendants); start += batchSize {
		end := start + batchSize
		if end > len(descendants) {
			end = len(descendants)
		}
		ids := make([]string, 0, end-start)
		for _, descendant := range descendants[start:end] {
			ids = append(ids, descendant.ID)
		}
		var mappings []struct {
			FileID string  `gorm:"column:file_id"`
			KBID   *string `gorm:"column:kb_id"`
		}
		if err := db.WithContext(ctx).Table("file2document AS mapping").Select("mapping.file_id, doc.kb_id").Joins("LEFT JOIN document AS doc ON doc.id = mapping.document_id").Where("mapping.file_id IN ?", ids).Scan(&mappings).Error; err != nil {
			return err
		}
		current := make(map[string]bool)
		others := make(map[string]bool)
		for _, mapping := range mappings {
			if mapping.KBID == nil {
				return ErrDatasetFolderConflict
			}
			if *mapping.KBID == kb.ID {
				current[mapping.FileID] = true
			} else {
				others[mapping.FileID] = true
			}
		}
		for fileID := range others {
			if !current[fileID] {
				return ErrDatasetFolderConflict
			}
		}
	}
	return nil
}

func loadDatasetFolder(ctx context.Context, db *gorm.DB, request *entity.Knowledgebase) (*entity.Knowledgebase, *entity.File, error) {
	var current entity.Knowledgebase
	if err := db.WithContext(ctx).Select("id", "tenant_id", "name", "folder_id").Where("id = ? AND tenant_id = ?", request.ID, request.TenantID).Take(&current).Error; err != nil {
		return nil, nil, err
	}
	if current.FolderID == nil || *current.FolderID == "" {
		return &current, nil, nil
	}
	var folder entity.File
	if err := db.WithContext(ctx).Where("id = ? AND tenant_id = ? AND type = ?", *current.FolderID, current.TenantID, "folder").Take(&folder).Error; err != nil {
		return &current, nil, fmt.Errorf("bound dataset folder %q is missing or invalid: %w", *current.FolderID, err)
	}
	return &current, &folder, nil
}

func checkOtherFolderBinding(ctx context.Context, db *gorm.DB, kbID, folderID string) error {
	var ids []string
	if err := db.WithContext(ctx).Model(&entity.Knowledgebase{}).Where("folder_id = ? AND id <> ?", folderID, kbID).Limit(1).Pluck("id", &ids).Error; err != nil {
		return err
	}
	if len(ids) != 0 {
		return ErrDatasetFolderConflict
	}
	return nil
}

func bindDatasetFolder(ctx context.Context, db *gorm.DB, kb *entity.Knowledgebase, folder *entity.File) error {
	if err := checkOtherFolderBinding(ctx, db, kb.ID, folder.ID); err != nil {
		return err
	}
	result := db.WithContext(ctx).Model(&entity.Knowledgebase{}).Where("id = ? AND tenant_id = ?", kb.ID, kb.TenantID).Where("folder_id IS NULL OR folder_id = ''").Update("folder_id", folder.ID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrDatasetFolderConflict
	}
	return nil
}

func findUnboundDatasetFolder(ctx context.Context, db *gorm.DB, kb *entity.Knowledgebase) (*entity.File, error) {
	parent, err := datasetRoot(ctx, db, kb.TenantID, false)
	if err != nil || parent == nil {
		return nil, err
	}
	folder, err := findDatasetChild(ctx, db, parent.ID, kb.TenantID, kb.Name)
	if err != nil || folder == nil {
		return folder, err
	}
	if err := checkOtherFolderBinding(ctx, db, kb.ID, folder.ID); err != nil {
		return nil, err
	}
	return folder, nil
}

// GetDatasetFolder resolves the live binding and live dataset name without changing either.
func (dao *FileDAO) GetDatasetFolder(ctx context.Context, db *gorm.DB, request *entity.Knowledgebase) (*entity.File, error) {
	current, folder, err := loadDatasetFolder(ctx, db, request)
	if err != nil || folder != nil {
		return folder, err
	}
	return findUnboundDatasetFolder(ctx, db, current)
}

// BindExistingDatasetFolder preserves directory identity before document references change.
// It never creates directories or force-adopts ambiguous existing content.
func (dao *FileDAO) BindExistingDatasetFolder(ctx context.Context, db *gorm.DB, request *entity.Knowledgebase) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, folder, err := loadDatasetFolder(ctx, tx, request)
		if err != nil || folder != nil {
			return err
		}
		folder, err = findUnboundDatasetFolder(ctx, tx, current)
		if err != nil || folder == nil {
			return err
		}
		if err := checkDatasetFolder(ctx, tx, current, folder); err != nil {
			return err
		}
		return bindDatasetFolder(ctx, tx, current, folder)
	})
}

// EnsureDatasetFolder creates or adopts a directory and saves its binding atomically.
func (dao *FileDAO) EnsureDatasetFolder(ctx context.Context, db *gorm.DB, request *entity.Knowledgebase) (*entity.File, error) {
	var result *entity.File
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, folder, err := loadDatasetFolder(ctx, tx, request)
		if err != nil {
			return err
		}
		if folder != nil {
			result = folder
			return nil
		}
		folder, err = findUnboundDatasetFolder(ctx, tx, current)
		if err != nil {
			return err
		}
		if folder != nil {
			if err := checkDatasetFolder(ctx, tx, current, folder); err != nil {
				return err
			}
		} else {
			parent, err := datasetRoot(ctx, tx, current.TenantID, true)
			if err != nil {
				return err
			}
			folder = &entity.File{ID: common.GenerateToken(), ParentID: parent.ID, TenantID: current.TenantID, CreatedBy: current.TenantID, Name: current.Name, Type: "folder", SourceType: string(entity.FileSourceKnowledgebase)}
			if err := tx.WithContext(ctx).Create(folder).Error; err != nil {
				return err
			}
		}
		if err := bindDatasetFolder(ctx, tx, current, folder); err != nil {
			return err
		}
		result = folder
		return nil
	})
	return result, err
}

// RenameDatasetFolder updates the associated directory in the caller's transaction.
func (dao *FileDAO) RenameDatasetFolder(ctx context.Context, db *gorm.DB, request *entity.Knowledgebase, name string) error {
	current, folder, err := loadDatasetFolder(ctx, db, request)
	if err != nil {
		return err
	}
	if folder == nil {
		folder, err = findUnboundDatasetFolder(ctx, db, current)
		if err != nil {
			return err
		}
		if folder != nil {
			if err := checkDatasetFolder(ctx, db, current, folder); err != nil {
				return err
			}
			if err := bindDatasetFolder(ctx, db, current, folder); err != nil {
				return err
			}
		}
	}
	var parentID string
	if folder != nil {
		parentID = folder.ParentID
	} else {
		parent, err := datasetRoot(ctx, db, current.TenantID, false)
		if err != nil {
			return err
		}
		if parent != nil {
			parentID = parent.ID
		}
	}
	if parentID != "" {
		var count int64
		q := db.WithContext(ctx).Model(&entity.File{}).Where("parent_id = ? AND tenant_id = ? AND LOWER(name) = LOWER(?)", parentID, current.TenantID, name)
		if folder != nil {
			q = q.Where("id <> ?", folder.ID)
		}
		if err := q.Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return ErrDatasetFolderConflict
		}
	}
	if folder == nil {
		return nil
	}
	result := db.WithContext(ctx).Model(&entity.File{}).Where("id = ?", folder.ID).Update("name", name)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrDatasetFolderConflict
	}
	return nil
}

// DeleteEmptyDatasetFolder removes a previously resolved folder only if it is unreferenced.
func (dao *FileDAO) DeleteEmptyDatasetFolder(ctx context.Context, db *gorm.DB, folderID string) error {
	ok, err := isUnreferencedEmptyFolder(ctx, db, folderID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	return db.WithContext(ctx).Unscoped().Where("id = ? AND type = ?", folderID, "folder").Delete(&entity.File{}).Error
}
