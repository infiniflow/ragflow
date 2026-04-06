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

// File2DocumentDAO file to document mapping data access object
type File2DocumentDAO struct{}

// NewFile2DocumentDAO create file2document DAO
func NewFile2DocumentDAO() *File2DocumentDAO {
	return &File2DocumentDAO{}
}

// GetKBInfoByFileID gets knowledge base info by file ID
func (dao *File2DocumentDAO) GetKBInfoByFileID(fileID string) ([]map[string]interface{}, error) {
	var results []map[string]interface{}

	rows, err := DB.Model(&entity.File{}).
		Select("knowledgebase.id, knowledgebase.name, file2document.document_id").
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
		var kbID, kbName, docID string
		if err := rows.Scan(&kbID, &kbName, &docID); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"kb_id":       kbID,
			"kb_name":     kbName,
			"document_id": docID,
		})
	}

	return results, nil
}
