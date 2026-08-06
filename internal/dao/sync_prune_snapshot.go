//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package dao

import (
	"context"
	syncerconnector "ragflow/internal/syncer/connector"

	"gorm.io/gorm"
)

// SyncPruneSnapshot stores one source ID from a complete prune snapshot.
type SyncPruneSnapshot struct {
	TaskID      string `gorm:"column:task_id;primaryKey;size:32"`
	SourceDocID string `gorm:"column:source_doc_id;primaryKey;size:512"`
}

// TableName returns the prune snapshot table name.
func (SyncPruneSnapshot) TableName() string {
	return "sync_prune_snapshot"
}

// SyncPruneSnapshotDAO manages temporary prune snapshots.
type SyncPruneSnapshotDAO struct {
	db *gorm.DB
}

// NewSyncPruneSnapshotDAO creates a prune snapshot DAO.
func NewSyncPruneSnapshotDAO(db *gorm.DB) *SyncPruneSnapshotDAO {
	return &SyncPruneSnapshotDAO{db: db}
}

// InsertBatch writes one prune snapshot batch.
func (d *SyncPruneSnapshotDAO) InsertBatch(ctx context.Context, taskID string, documents []syncerconnector.SlimDocument) error {
	if len(documents) == 0 {
		return nil
	}
	rows := make([]SyncPruneSnapshot, 0, len(documents))
	for _, doc := range documents {
		rows = append(rows, SyncPruneSnapshot{TaskID: taskID, SourceDocID: doc.SourceID})
	}
	return d.db.WithContext(ctx).Create(&rows).Error
}

// ListSourceIDs returns all source IDs for a task snapshot.
func (d *SyncPruneSnapshotDAO) ListSourceIDs(ctx context.Context, taskID string) ([]string, error) {
	var ids []string
	err := d.db.WithContext(ctx).Model(&SyncPruneSnapshot{}).Where("task_id = ?", taskID).Pluck("source_doc_id", &ids).Error
	return ids, err
}

// Clear removes all snapshot rows for a task.
func (d *SyncPruneSnapshotDAO) Clear(ctx context.Context, taskID string) error {
	return d.db.WithContext(ctx).Where("task_id = ?", taskID).Delete(&SyncPruneSnapshot{}).Error
}
