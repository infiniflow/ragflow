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

	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// ListKnowledgeCompileEntries returns the routing metadata of every queue
// entry for datasetID. Variants and TaskTypes use serializer:json, so Find
// loads and deserializes them; callers apply kind filtering in Go to stay
// portable across databases.
func ListKnowledgeCompileEntries(ctx context.Context, db *gorm.DB, table, datasetID string) ([]entity.KnowledgeCompileEntry, error) {
	var entries []entity.KnowledgeCompileEntry
	err := db.WithContext(ctx).Table(table).
		Where("dataset_id = ?", datasetID).
		Select("variants", "task_types").
		Find(&entries).Error
	return entries, err
}

// CountKnowledgeCompileEntries counts all queue entries for datasetID using
// a SQL aggregate. Duplicates are intentional and must be counted separately.
func CountKnowledgeCompileEntries(ctx context.Context, db *gorm.DB, table, datasetID string) (int, error) {
	var count int64
	err := db.WithContext(ctx).Table(table).Where("dataset_id = ?", datasetID).Count(&count).Error
	return int(count), err
}
