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
	"ragflow/internal/model"
)

// DocumentDAO document data access object
type DocumentDAO struct{}

// NewDocumentDAO create document DAO
func NewDocumentDAO() *DocumentDAO {
	return &DocumentDAO{}
}

// Create create document
func (dao *DocumentDAO) Create(document *model.Document) error {
	return DB.Create(document).Error
}

// GetByID get document by ID
func (dao *DocumentDAO) GetByID(id string) (*model.Document, error) {
	var document model.Document
	err := DB.Preload("Author").First(&document, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &document, nil
}

// GetByAuthorID get documents by author ID
func (dao *DocumentDAO) GetByAuthorID(authorID string, offset, limit int) ([]*model.Document, int64, error) {
	var documents []*model.Document
	var total int64

	query := DB.Model(&model.Document{}).Where("created_by = ?", authorID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Preload("Author").Offset(offset).Limit(limit).Find(&documents).Error
	return documents, total, err
}

// Update update document
func (dao *DocumentDAO) Update(document *model.Document) error {
	return DB.Save(document).Error
}

// Delete delete document
func (dao *DocumentDAO) Delete(id string) error {
	return DB.Delete(&model.Document{}, "id = ?", id).Error
}

// List list documents
func (dao *DocumentDAO) List(offset, limit int) ([]*model.Document, int64, error) {
	var documents []*model.Document
	var total int64

	if err := DB.Model(&model.Document{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := DB.Preload("Author").Offset(offset).Limit(limit).Find(&documents).Error
	return documents, total, err
}
