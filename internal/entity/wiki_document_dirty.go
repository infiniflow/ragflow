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

package entity

import "time"

const (
	WikiDirtyStatePending = "pending"
	WikiDirtyStateRunning = "running"
)

// WikiDocumentDirty is the durable trailing-edge debounce row for direct chunk
// mutations. Each document owns one row; another mutation increments Revision,
// merges the affected chunk ids, and moves RunAfter twenty seconds forward.
type WikiDocumentDirty struct {
	DocumentID       string     `gorm:"primaryKey;column:document_id;size:64" json:"document_id"`
	DatasetID        string     `gorm:"column:dataset_id;size:64;not null;index" json:"dataset_id"`
	TenantID         string     `gorm:"column:tenant_id;size:64;not null;index" json:"tenant_id"`
	Revision         uint64     `gorm:"column:revision;not null;default:1" json:"revision"`
	AffectedChunkIDs string     `gorm:"column:affected_chunk_ids;type:text;not null" json:"affected_chunk_ids"`
	RunAfter         time.Time  `gorm:"column:run_after;not null;index" json:"run_after"`
	State            string     `gorm:"column:state;size:16;not null;default:'pending';index" json:"state"`
	ClaimOwner       string     `gorm:"column:claim_owner;size:64;not null;default:''" json:"claim_owner"`
	ClaimExpiresAt   *time.Time `gorm:"column:claim_expires_at;default:null;index" json:"claim_expires_at"`
	ErrorMsg         string     `gorm:"column:error_msg;type:text;not null" json:"error_msg"`
	CreatedAt        time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (WikiDocumentDirty) TableName() string { return "wiki_document_dirty" }
