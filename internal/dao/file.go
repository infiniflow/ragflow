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
	"strings"

	"github.com/google/uuid"

	"ragflow/internal/model"
)

// FileDAO file data access object
type FileDAO struct{}

// NewFileDAO create file DAO
func NewFileDAO() *FileDAO {
	return &FileDAO{}
}

// GetByID gets file by ID
func (dao *FileDAO) GetByID(id string) (*model.File, error) {
	var file model.File
	err := DB.Where("id = ?", id).First(&file).Error
	if err != nil {
		return nil, err
	}
	return &file, nil
}

// GetByPfID gets files by parent folder ID with pagination and filtering
func (dao *FileDAO) GetByPfID(tenantID, pfID string, page, pageSize int, orderby string, desc bool, keywords string) ([]*model.File, int64, error) {
	var files []*model.File
	var total int64

	query := DB.Model(&model.File{}).
		Where("tenant_id = ? AND parent_id = ? AND id != ?", tenantID, pfID, pfID)

	// Apply keyword filter
	if keywords != "" {
		query = query.Where("LOWER(name) LIKE ?", "%"+strings.ToLower(keywords)+"%")
	}

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply ordering
	orderDirection := "ASC"
	if desc {
		orderDirection = "DESC"
	}
	query = query.Order(orderby + " " + orderDirection)

	// Apply pagination
	if page > 0 && pageSize > 0 {
		offset := (page - 1) * pageSize
		if err := query.Offset(offset).Limit(pageSize).Find(&files).Error; err != nil {
			return nil, 0, err
		}
	} else {
		if err := query.Find(&files).Error; err != nil {
			return nil, 0, err
		}
	}

	return files, total, nil
}

// GetRootFolder gets or creates root folder for tenant
func (dao *FileDAO) GetRootFolder(tenantID string) (*model.File, error) {
	var file model.File
	err := DB.Where("tenant_id = ? AND parent_id = id", tenantID).First(&file).Error
	if err == nil {
		return &file, nil
	}

	// Create root folder if not exists
	fileID := generateUUID()
	file = model.File{
		ID:        fileID,
		ParentID:  fileID,
		TenantID:  tenantID,
		CreatedBy: tenantID,
		Name:      "/",
		Type:      "folder",
		Size:      0,
	}
	file.SourceType = ""

	if err := DB.Create(&file).Error; err != nil {
		return nil, err
	}
	return &file, nil
}

// GetParentFolder gets parent folder of a file
func (dao *FileDAO) GetParentFolder(fileID string) (*model.File, error) {
	var file model.File
	err := DB.Where("id = ?", fileID).First(&file).Error
	if err != nil {
		return nil, err
	}

	var parentFile model.File
	err = DB.Where("id = ?", file.ParentID).First(&parentFile).Error
	if err != nil {
		return nil, err
	}
	return &parentFile, nil
}

// ListByParentID lists all files by parent ID (including subfolders)
func (dao *FileDAO) ListByParentID(parentID string) ([]*model.File, error) {
	var files []*model.File
	err := DB.Where("parent_id = ? AND id != ?", parentID, parentID).Find(&files).Error
	return files, err
}

// GetFolderSize calculates folder size recursively
func (dao *FileDAO) GetFolderSize(folderID string) (int64, error) {
	var size int64

	var dfs func(parentID string) error
	dfs = func(parentID string) error {
		var files []*model.File
		if err := DB.Select("id", "size", "type").
			Where("parent_id = ? AND id != ?", parentID, parentID).
			Find(&files).Error; err != nil {
			return err
		}

		for _, f := range files {
			size += f.Size
			if f.Type == "folder" {
				if err := dfs(f.ID); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if err := dfs(folderID); err != nil {
		return 0, err
	}
	return size, nil
}

// HasChildFolder checks if folder has child folders
func (dao *FileDAO) HasChildFolder(folderID string) (bool, error) {
	var count int64
	err := DB.Model(&model.File{}).
		Where("parent_id = ? AND id != ? AND type = ?", folderID, folderID, "folder").
		Count(&count).Error
	return count > 0, err
}

// GetAllParentFolders gets all parent folders in path (from current to root)
func (dao *FileDAO) GetAllParentFolders(startID string) ([]*model.File, error) {
	var parentFolders []*model.File
	currentID := startID

	for currentID != "" {
		var file model.File
		err := DB.Where("id = ?", currentID).First(&file).Error
		if err != nil {
			return nil, err
		}

		parentFolders = append(parentFolders, &file)

		// Stop if we've reached the root folder (parent_id == id)
		if file.ParentID == file.ID {
			break
		}
		currentID = file.ParentID
	}

	return parentFolders, nil
}

// generateUUID generates a UUID
func generateUUID() string {
	id := uuid.New().String()
	return strings.ReplaceAll(id, "-", "")
}
