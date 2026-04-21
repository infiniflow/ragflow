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
	"ragflow/internal/entity"
	"strings"

	"github.com/google/uuid"
)

// FileDAO file data access object
type FileDAO struct{}

// NewFileDAO create file DAO
func NewFileDAO() *FileDAO {
	return &FileDAO{}
}

// GetByID gets file by ID
func (dao *FileDAO) GetByID(id string) (*entity.File, error) {
	var file entity.File
	err := DB.Where("id = ?", id).First(&file).Error
	if err != nil {
		return nil, err
	}
	return &file, nil
}

// GetByPfID gets files by parent folder ID with pagination and filtering
func (dao *FileDAO) GetByPfID(tenantID, pfID string, page, pageSize int, orderby string, desc bool, keywords string) ([]*entity.File, int64, error) {
	var files []*entity.File
	var total int64

	query := DB.Model(&entity.File{}).
		Where("tenant_id = ? AND parent_id = ? AND id != ?", tenantID, pfID, pfID)

	// Apply keyword filter
	if keywords != "" {
		query = query.Where("LOWER(name) LIKE ?", "%"+strings.ToLower(keywords)+"%")
	}

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply ordering (allowlist only; never concatenate raw user input)
	orderDirection := "ASC"
	if desc {
		orderDirection = "DESC"
	}
	orderCol := sanitizeFileListOrderBy(orderby)
	query = query.Order(orderCol + " " + orderDirection)

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
func (dao *FileDAO) GetRootFolder(tenantID string) (*entity.File, error) {
	var file entity.File
	err := DB.Where("tenant_id = ? AND parent_id = id", tenantID).First(&file).Error
	if err == nil {
		return &file, nil
	}

	// Create root folder if not exists
	fileID := generateUUID()
	file = entity.File{
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
func (dao *FileDAO) GetParentFolder(fileID string) (*entity.File, error) {
	var file entity.File
	err := DB.Where("id = ?", fileID).First(&file).Error
	if err != nil {
		return nil, err
	}

	var parentFile entity.File
	err = DB.Where("id = ?", file.ParentID).First(&parentFile).Error
	if err != nil {
		return nil, err
	}
	return &parentFile, nil
}

// ListByParentID lists all files by parent ID (including subfolders)
func (dao *FileDAO) ListByParentID(parentID string) ([]*entity.File, error) {
	var files []*entity.File
	err := DB.Where("parent_id = ? AND id != ?", parentID, parentID).Find(&files).Error
	return files, err
}

// GetFolderSize calculates folder size recursively
func (dao *FileDAO) GetFolderSize(folderID string) (int64, error) {
	var size int64

	var dfs func(parentID string) error
	dfs = func(parentID string) error {
		var files []*entity.File
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
	err := DB.Model(&entity.File{}).
		Where("parent_id = ? AND id != ? AND type = ?", folderID, folderID, "folder").
		Count(&count).Error
	return count > 0, err
}

// GetAllParentFolders gets all parent folders in path (from current to root)
func (dao *FileDAO) GetAllParentFolders(startID string) ([]*entity.File, error) {
	var parentFolders []*entity.File
	currentID := startID

	for currentID != "" {
		var file entity.File
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

// Create creates a new file
func (dao *FileDAO) Create(file *entity.File) error {
	return DB.Create(file).Error
}

// DeleteByTenantID deletes all files by tenant ID (hard delete)
func (dao *FileDAO) DeleteByTenantID(tenantID string) (int64, error) {
	result := DB.Unscoped().Where("tenant_id = ?", tenantID).Delete(&entity.File{})
	return result.RowsAffected, result.Error
}

// DeleteByIDs deletes files by IDs (hard delete)
func (dao *FileDAO) DeleteByIDs(ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	result := DB.Unscoped().Where("id IN ?", ids).Delete(&entity.File{})
	return result.RowsAffected, result.Error
}

// GetAllIDsByTenantID gets all file IDs by tenant ID
func (dao *FileDAO) GetAllIDsByTenantID(tenantID string) ([]string, error) {
	var ids []string
	err := DB.Model(&entity.File{}).Where("tenant_id = ?", tenantID).Pluck("id", &ids).Error
	return ids, err
}

// GetByIDs gets files by multiple IDs
func (dao *FileDAO) GetByIDs(ids []string) ([]*entity.File, error) {
	var files []*entity.File
	if len(ids) == 0 {
		return files, nil
	}
	err := DB.Where("id IN ?", ids).Find(&files).Error
	return files, err
}

// ListAllFilesByParentID lists all files by parent folder ID
func (dao *FileDAO) ListAllFilesByParentID(parentID string) ([]*entity.File, error) {
	var files []*entity.File
	err := DB.Where("parent_id = ? AND id != ?", parentID, parentID).Find(&files).Error
	return files, err
}

// GetByParentIDAndName gets file by parent folder ID and name
func (dao *FileDAO) GetByParentIDAndName(parentID, name string) (*entity.File, error) {
	var file entity.File
	err := DB.Where("parent_id = ? AND name = ?", parentID, name).First(&file).Error
	if err != nil {
		return nil, err
	}
	return &file, nil
}

// GetIDListByID recursively gets list of file IDs by traversing folder structure
func (dao *FileDAO) GetIDListByID(id string, names []string, count int, res []string) ([]string, error) {
	if count < len(names) {
		file, err := dao.GetByParentIDAndName(id, names[count])
		if err != nil {
			return res, nil
		}
		res = append(res, file.ID)
		return dao.GetIDListByID(file.ID, names, count+1, res)
	}
	return res, nil
}

// CreateFolder creates a folder in the database
func (dao *FileDAO) CreateFolder(parentID, tenantID, name, fileType string) (*entity.File, error) {
	file := &entity.File{
		ID:         generateUUID(),
		ParentID:   parentID,
		TenantID:   tenantID,
		CreatedBy:  tenantID,
		Name:       name,
		Type:       fileType,
		Size:       0,
		SourceType: "",
	}
	if err := DB.Create(file).Error; err != nil {
		return nil, err
	}
	return file, nil
}

// Insert inserts a new file record
func (dao *FileDAO) Insert(file *entity.File) error {
	return DB.Create(file).Error
}

// IsParentFolderExist checks if parent folder exists
func (dao *FileDAO) IsParentFolderExist(parentID string) bool {
	var count int64
	err := DB.Model(&entity.File{}).Where("id = ?", parentID).Count(&count).Error
	if err != nil || count == 0 {
		return false
	}
	return true
}

// Query retrieves files by conditions
func (dao *FileDAO) Query(name string, parentID string) []*entity.File {
	var files []*entity.File
	query := DB.Model(&entity.File{})
	if name != "" {
		query = query.Where("name = ?", name)
	}
	if parentID != "" {
		query = query.Where("parent_id = ?", parentID)
	}
	query.Find(&files)
	return files
}

// UpdateByID updates file by ID with the given fields
func (dao *FileDAO) UpdateByID(id string, updates map[string]interface{}) error {
	return DB.Model(&entity.File{}).Where("id = ?", id).Updates(updates).Error
}

// Delete deletes a file by ID (hard delete)
func (dao *FileDAO) Delete(id string) error {
	return DB.Unscoped().Where("id = ?", id).Delete(&entity.File{}).Error
}

// GetDatasetIDByFileID gets dataset ID by file ID
func (dao *FileDAO) GetDatasetIDByFileID(fileID string) ([]string, error) {
	var datasetIDs []string
	rows, err := DB.Model(&entity.File{}).
		Select("knowledgebase.id").
		Joins("JOIN file2document ON file2document.file_id = ?", fileID).
		Joins("JOIN document ON document.id = file2document.document_id").
		Joins("JOIN knowledgebase ON knowledgebase.id = document.kb_id").
		Where("file.id = ?", fileID).
		Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var kbID string
		if err := rows.Scan(&kbID); err != nil {
			continue
		}
		datasetIDs = append(datasetIDs, kbID)
	}

	return datasetIDs, nil
}

// generateUUID generates a UUID
func generateUUID() string {
	id := uuid.New().String()
	return strings.ReplaceAll(id, "-", "")
}

// DatasetFolderName is the folder name for dataset
const DatasetFolderName = ".knowledgebase"

// InitDatasetDocs initializes dataset documents for tenant
// This matches Python's FileService.init_dataset_docs method
func (dao *FileDAO) InitDatasetDocs(rootID, tenantID string, file2DocumentDAO *File2DocumentDAO) error {
	var count int64
	err := DB.Model(&entity.File{}).
		Where("name = ? AND parent_id = ?", DatasetFolderName, rootID).
		Count(&count).Error
	if err != nil {
		return err
	}

	if count > 0 {
		return nil
	}

	datasetFolder, err := dao.newAFileFromDataset(tenantID, DatasetFolderName, rootID)
	if err != nil {
		return err
	}

	var datasets []entity.Knowledgebase
	err = DB.Select("id", "name").
		Where("tenant_id = ?", tenantID).
		Find(&datasets).Error
	if err != nil {
		return err
	}

	for _, ds := range datasets {
		datasetFolderForDataset, err := dao.newAFileFromDataset(tenantID, ds.Name, datasetFolder.ID)
		if err != nil {
			continue
		}

		var documents []entity.Document
		err = DB.Where("kb_id = ?", ds.ID).Find(&documents).Error
		if err != nil {
			continue
		}

		for _, doc := range documents {
			if err := dao.addFileFromKB(&doc, datasetFolderForDataset.ID, tenantID, file2DocumentDAO); err != nil {
				return err
			}
		}
	}

	return nil
}

// newAFileFromDataset creates a new file from knowledgebase
func (dao *FileDAO) newAFileFromDataset(tenantID, name, parentID string) (*entity.File, error) {
	var existingFiles []*entity.File
	err := DB.Where("tenant_id = ? AND parent_id = ? AND name = ?", tenantID, parentID, name).Find(&existingFiles).Error
	if err != nil {
		return nil, err
	}

	if len(existingFiles) > 0 {
		return existingFiles[0], nil
	}

	fileID := generateUUID()
	file := &entity.File{
		ID:         fileID,
		ParentID:   parentID,
		TenantID:   tenantID,
		CreatedBy:  tenantID,
		Name:       name,
		Type:       "folder",
		Size:       0,
		SourceType: "knowledgebase",
	}

	if err := DB.Create(file).Error; err != nil {
		return nil, err
	}
	return file, nil
}

// addFileFromKB adds a file record from knowledgebase document
func (dao *FileDAO) addFileFromKB(doc *entity.Document, datasetFolderID, tenantID string, file2DocumentDAO *File2DocumentDAO) error {
	var f2dCount int64
	err := DB.Model(&entity.File2Document{}).
		Where("document_id = ?", doc.ID).
		Count(&f2dCount).Error
	if err != nil {
		return err
	}

	if f2dCount > 0 {
		return nil
	}

	docName := ""
	if doc.Name != nil {
		docName = *doc.Name
	}

	docLocation := ""
	if doc.Location != nil {
		docLocation = *doc.Location
	}

	fileID := generateUUID()
	file := &entity.File{
		ID:         fileID,
		ParentID:   datasetFolderID,
		TenantID:   tenantID,
		CreatedBy:  tenantID,
		Name:       docName,
		Type:       doc.Type,
		Size:       doc.Size,
		Location:   &docLocation,
		SourceType: "knowledgebase",
	}

	if err := DB.Create(file).Error; err != nil {
		return err
	}

	f2dID := generateUUID()
	f2d := &entity.File2Document{
		ID:         f2dID,
		FileID:     &fileID,
		DocumentID: &doc.ID,
	}

	if err := DB.Create(f2d).Error; err != nil {
		return err
	}

	return nil
}
