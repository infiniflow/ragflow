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
)

// DocumentDAO document data access object
type DocumentDAO struct{}

// NewDocumentDAO create document DAO
func NewDocumentDAO() *DocumentDAO {
	return &DocumentDAO{}
}

// Create create document
func (dao *DocumentDAO) Create(document *entity.Document) error {
	return DB.Create(document).Error
}

// GetByID get document by ID
func (dao *DocumentDAO) GetByID(id string) (*entity.Document, error) {
	var document entity.Document
	err := DB.First(&document, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &document, nil
}

// GetByAuthorID get documents by author ID
func (dao *DocumentDAO) GetByAuthorID(authorID string, offset, limit int) ([]*entity.Document, int64, error) {
	var documents []*entity.Document
	var total int64

	query := DB.Model(&entity.Document{}).Where("created_by = ?", authorID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Preload("Author").Offset(offset).Limit(limit).Find(&documents).Error
	return documents, total, err
}

// Update update document
func (dao *DocumentDAO) Update(document *entity.Document) error {
	return DB.Save(document).Error
}

// UpdateByID updates document by ID with the given fields
func (dao *DocumentDAO) UpdateByID(id string, updates map[string]interface{}) error {
	return DB.Model(&entity.Document{}).Where("id = ?", id).Updates(updates).Error
}

// Delete delete document
func (dao *DocumentDAO) Delete(id string) error {
	return DB.Delete(&entity.Document{}, "id = ?", id).Error
}

// List list documents
func (dao *DocumentDAO) List(offset, limit int) ([]*entity.Document, int64, error) {
	var documents []*entity.Document
	var total int64

	if err := DB.Model(&entity.Document{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := DB.Preload("Author").Offset(offset).Limit(limit).Find(&documents).Error
	return documents, total, err
}

// ListByKBID list documents by knowledge base ID
func (dao *DocumentDAO) ListByKBID(kbID string, offset, limit int) ([]*entity.DocumentListItem, int64, error) {
	var documents []*entity.DocumentListItem
	var total int64

	if err := DB.Model(&entity.Document{}).Where("kb_id = ?", kbID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := DB.Table("document").
		Select(`document.*, user_canvas.title as pipeline_name, user.nickname`).
		Joins("JOIN file2document ON file2document.document_id = document.id").
		Joins("JOIN file ON file.id = file2document.file_id").
		Joins("LEFT JOIN user_canvas ON document.pipeline_id = user_canvas.id").
		Joins("LEFT JOIN user ON document.created_by = user.id").
		Where("document.kb_id = ?", kbID).
		Order("document.create_time DESC").
		Offset(offset).
		Limit(limit).
		Scan(&documents).Error
	return documents, total, err
}

// DeleteByTenantID deletes all documents by tenant ID (hard delete)
func (dao *DocumentDAO) DeleteByTenantID(tenantID string) (int64, error) {
	result := DB.Unscoped().Where("tenant_id = ?", tenantID).Delete(&entity.Document{})
	return result.RowsAffected, result.Error
}

// GetAllDocIDsByKBIDs gets all document IDs by knowledge base IDs
func (dao *DocumentDAO) GetAllDocIDsByKBIDs(kbIDs []string) ([]map[string]string, error) {
	var docs []struct {
		ID   string `gorm:"column:id"`
		KbID string `gorm:"column:kb_id"`
	}
	err := DB.Model(&entity.Document{}).Select("id, kb_id").Where("kb_id IN ?", kbIDs).Find(&docs).Error
	if err != nil {
		return nil, err
	}

	result := make([]map[string]string, len(docs))
	for i, doc := range docs {
		result[i] = map[string]string{"id": doc.ID, "kb_id": doc.KbID}
	}
	return result, nil
}

// GetByIDs retrieves documents by multiple IDs
func (dao *DocumentDAO) GetByIDs(ids []string) ([]*entity.Document, error) {
	var documents []*entity.Document
	err := DB.Where("id IN ?", ids).Find(&documents).Error
	if err != nil {
		return nil, err
	}
	return documents, nil
}

// CountByTenantID counts documents by tenant ID
func (dao *DocumentDAO) CountByTenantID(tenantID string) (int64, error) {
	var count int64
	err := DB.Model(&entity.Document{}).Where("created_by = ?", tenantID).Count(&count).Error
	return count, err
}

// SumSizeByDatasetID returns the total document size for a dataset.
func (dao *DocumentDAO) SumSizeByDatasetID(datasetID string) (int64, error) {
	var total int64
	err := DB.Model(&entity.Document{}).
		Select("COALESCE(SUM(size), 0)").
		Where("kb_id = ?", datasetID).
		Scan(&total).Error
	return total, err
}

// GetParsingStatusByKBID aggregates document parsing status counts for a
// dataset, mirroring DocumentService.get_parsing_status_by_kb_ids in Python.
func (dao *DocumentDAO) GetParsingStatusByKBID(kbID string) (map[string]int64, error) {
	result := map[string]int64{
		"unstart_count": 0,
		"running_count": 0,
		"cancel_count":  0,
		"done_count":    0,
		"fail_count":    0,
	}

	var rows []struct {
		Run *string `gorm:"column:run"`
		Cnt int64   `gorm:"column:cnt"`
	}
	err := DB.Model(&entity.Document{}).
		Select("run, COUNT(id) as cnt").
		Where("kb_id = ?", kbID).
		Group("run").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	statusFieldMap := map[string]string{
		string(entity.TaskStatusUnstart): "unstart_count",
		string(entity.TaskStatusRunning): "running_count",
		string(entity.TaskStatusCancel):  "cancel_count",
		string(entity.TaskStatusDone):    "done_count",
		string(entity.TaskStatusFail):    "fail_count",
	}
	for _, row := range rows {
		if row.Run == nil {
			continue
		}
		if field, ok := statusFieldMap[*row.Run]; ok {
			result[field] = row.Cnt
		}
	}
	return result, nil
}
