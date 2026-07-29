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

package dao

import (
	"context"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// FileCommitDAO file commit data access object
type FileCommitDAO struct{}

// NewFileCommitDAO create file commit DAO
func NewFileCommitDAO() *FileCommitDAO {
	return &FileCommitDAO{}
}

// GetByID gets a file commit by ID
func (dao *FileCommitDAO) GetByID(ctx context.Context, db *gorm.DB, id string) (*entity.FileCommit, error) {
	var commit entity.FileCommit
	err := db.WithContext(ctx).Where("id = ?", id).First(&commit).Error
	if err != nil {
		return nil, err
	}
	return &commit, nil
}

// Create creates a new file commit record
func (dao *FileCommitDAO) Create(ctx context.Context, db *gorm.DB, commit *entity.FileCommit) error {
	return db.WithContext(ctx).Create(commit).Error
}

// UpdateTreeState updates the tree_state field for a commit
func (dao *FileCommitDAO) UpdateTreeState(ctx context.Context, db *gorm.DB, id string, treeState string) error {
	return db.WithContext(ctx).Model(&entity.FileCommit{}).Where("id = ?", id).Update("tree_state", treeState).Error
}

// GetLatestByFolderID gets the latest (most recent) commit for a folder
func (dao *FileCommitDAO) GetLatestByFolderID(ctx context.Context, db *gorm.DB, folderID string) (*entity.FileCommit, error) {
	var commit entity.FileCommit
	err := db.WithContext(ctx).Where("folder_id = ?", folderID).
		Order("create_time DESC").
		First(&commit).Error
	if err != nil {
		return nil, err
	}
	return &commit, nil
}

// allowedFileCommitSorts is the whitelist of safe column names for
// ListByFolderID's orderBy parameter to prevent SQL injection.
var allowedFileCommitSorts = map[string]string{
	"create_time": "create_time",
	"create_date": "create_date",
	"update_time": "update_time",
	"update_date": "update_date",
}

// ListByFolderID lists commits for a folder with pagination
func (dao *FileCommitDAO) ListByFolderID(ctx context.Context, db *gorm.DB, folderID string, page, pageSize int, orderBy string, desc bool) ([]*entity.FileCommit, int64, error) {
	var commits []*entity.FileCommit
	var total int64

	query := db.WithContext(ctx).Model(&entity.FileCommit{}).Where("folder_id = ?", folderID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Sanitize orderBy against whitelist; fall back to create_time.
	safeCol, ok := allowedFileCommitSorts[orderBy]
	if !ok {
		safeCol = "create_time"
	}

	orderDirection := "DESC"
	if !desc {
		orderDirection = "ASC"
	}

	orderClause := safeCol + " " + orderDirection

	if page > 0 && pageSize > 0 {
		offset := (page - 1) * pageSize
		if err := query.Order(orderClause).Offset(offset).Limit(pageSize).Find(&commits).Error; err != nil {
			return nil, 0, err
		}
	} else {
		if err := query.Order(orderClause).Find(&commits).Error; err != nil {
			return nil, 0, err
		}
	}

	return commits, total, nil
}

// FileCommitItemDAO file commit item data access object
type FileCommitItemDAO struct{}

// NewFileCommitItemDAO create file commit item DAO
func NewFileCommitItemDAO() *FileCommitItemDAO {
	return &FileCommitItemDAO{}
}

// Create creates a new file commit item record
func (dao *FileCommitItemDAO) Create(ctx context.Context, db *gorm.DB, item *entity.FileCommitItem) error {
	return db.WithContext(ctx).Create(item).Error
}

// ListByCommitID lists all items for a commit
func (dao *FileCommitItemDAO) ListByCommitID(ctx context.Context, db *gorm.DB, commitID string) ([]*entity.FileCommitItem, error) {
	var items []*entity.FileCommitItem
	err := db.WithContext(ctx).Where("commit_id = ?", commitID).Order("create_time ASC").Find(&items).Error
	return items, err
}

// ListByFileID lists all commit items for a specific file (for version history)
func (dao *FileCommitItemDAO) ListByFileID(ctx context.Context, db *gorm.DB, fileID string) ([]*entity.FileCommitItem, error) {
	var items []*entity.FileCommitItem
	err := db.WithContext(ctx).Where("file_id = ?", fileID).Order("create_time DESC").Find(&items).Error
	return items, err
}

// GetByCommitIDAndFileID gets a single commit item by commit and file ID
func (dao *FileCommitItemDAO) GetByCommitIDAndFileID(ctx context.Context, db *gorm.DB, commitID, fileID string) (*entity.FileCommitItem, error) {
	var item entity.FileCommitItem
	err := db.WithContext(ctx).Where("commit_id = ? AND file_id = ?", commitID, fileID).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}
