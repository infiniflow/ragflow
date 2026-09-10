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

package entity

import "time"

// MemoryTaskState is a durable checkpoint in memory extraction.
type MemoryTaskState string

const (
	MemoryTaskStatePending   MemoryTaskState = "pending"
	MemoryTaskStateExtracted MemoryTaskState = "extracted"
	MemoryTaskStateStored    MemoryTaskState = "stored"
	MemoryTaskStateCompleted MemoryTaskState = "completed"
	MemoryTaskStateFailed    MemoryTaskState = "failed"
)

// MemoryTask stores the authoritative execution state for an asynchronous
// memory extraction task. Task.Progress remains a UI projection and is not an
// execution checkpoint.
type MemoryTask struct {
	TaskID         string          `gorm:"column:task_id;primaryKey;size:32" json:"task_id"`
	MemoryID       string          `gorm:"column:memory_id;size:32;not null;index" json:"memory_id"`
	SourceID       int64           `gorm:"column:source_id;not null" json:"source_id"`
	Input          JSONMap         `gorm:"column:input;type:longtext;not null" json:"input"`
	State          MemoryTaskState `gorm:"column:state;size:16;not null;default:'pending';index:idx_memory_task_due,priority:1" json:"state"`
	Extraction     JSONSlice       `gorm:"column:extraction;type:longtext" json:"extraction,omitempty"`
	NextRetryAt    *time.Time      `gorm:"column:next_retry_at;default:null;index:idx_memory_task_due,priority:2" json:"next_retry_at,omitempty"`
	AttemptCount   int             `gorm:"column:attempt_count;not null;default:0" json:"attempt_count"`
	LeaseOwner     string          `gorm:"column:lease_owner;size:128;not null;default:''" json:"lease_owner"`
	LeaseExpiresAt *time.Time      `gorm:"column:lease_expires_at;default:null;index" json:"lease_expires_at,omitempty"`
	LastError      string          `gorm:"column:last_error;type:text;not null" json:"last_error"`
	BaseModel
}

// TableName returns the durable memory task table name.
func (MemoryTask) TableName() string { return "memory_task" }
