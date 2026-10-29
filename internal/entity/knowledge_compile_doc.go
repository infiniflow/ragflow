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

// Dataset-level compile lifecycle states. Shared source of truth for the
// scheduler (which writes State on the knowledge_compile_docs row) and the
// dataset compilation-status API (which reads it back).
const (
	DatasetStateIdle      = "idle"      // no scheduling row / nothing to do
	DatasetStatePending   = "pending"   // backlog non-empty, awaiting claim
	DatasetStateRunning   = "running"   // a worker holds the lease and is merging
	DatasetStateCompleted = "completed" // backlog drained to empty
)

// KnowledgeCompileDataset is the MySQL scheduling row for the dataset-level
// post-processing consumer (knowledge_compile_design.md §11.4, Option E). It is
// the scheduling system of record together with KnowledgeCompileBacklog and
// KnowledgeCompileInflight. The claim_* fields hold the owner/lease used for
// crash recovery. NATS notify is only a wake-up; same-KB serialization comes
// from locking this row before changing either queue, not from the broker.
type KnowledgeCompileDataset struct {
	DatasetID      string     `gorm:"primaryKey;column:dataset_id;size:64" json:"dataset_id"`
	TenantID       string     `gorm:"column:tenant_id;size:64;not null;default:''" json:"tenant_id"`
	ClaimOwner     string     `gorm:"column:claim_owner;size:64;not null;default:''" json:"claim_owner"`
	ClaimToken     string     `gorm:"column:claim_token;size:64;not null;default:''" json:"claim_token"`
	ClaimExpiresAt *time.Time `gorm:"column:claim_expires_at;default:null" json:"claim_expires_at"`
	Priority       int        `gorm:"column:priority;not null;default:0" json:"priority"`
	// State is the dataset-level compile lifecycle state surfaced to the API:
	// idle | pending | running | completed. It is written by the scheduler and
	// consumer; the API never derives it from the backlog alone. Default is a
	// scalar string literal on a varchar column, which MySQL allows (Error 1101
	// only affects TEXT/BLOB, not varchar).
	State string `gorm:"column:state;size:16;not null;default:'idle'" json:"state"`
	// ErrorMsg is the most recent failure/retry diagnostic. It is TEXT with no
	// DDL default (MySQL 8.0.13+ rejects a literal default on TEXT, Error 1101);
	// the application always writes it explicitly when set.
	ErrorMsg     string  `gorm:"column:error_msg;type:text;not null" json:"error_msg"`
	Progress     float64 `gorm:"column:progress;not null;default:0" json:"progress"`
	CurrentPhase string  `gorm:"column:current_phase;size:64;not null;default:''" json:"current_phase"`
	ProgressMsg  string  `gorm:"column:progress_msg;type:text;not null" json:"progress_msg"`
	// LastCompletedAt records the last time the backlog drained to empty.
	LastCompletedAt *time.Time `gorm:"column:last_completed_at;default:null" json:"last_completed_at"`
	CreatedAt       time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName pins the scheduling table name.
func (KnowledgeCompileDataset) TableName() string { return "knowledge_compile_docs" }

// KnowledgeCompileEntry is an ordered queue event. Duplicate document events
// are allowed; neither queue has a primary key. Positions are allocated while
// holding the dataset scheduling row lock.
type KnowledgeCompileEntry struct {
	DatasetID     string   `gorm:"column:dataset_id;size:64;not null;index:,composite:queue,priority:1" json:"dataset_id"`
	DocID         string   `gorm:"column:doc_id;size:64;not null" json:"doc_id"`
	QueuePosition int64    `gorm:"column:queue_position;not null;index:,composite:queue,priority:2" json:"queue_position"`
	EventType     string   `gorm:"column:event_type;size:32;not null" json:"event_type"`
	Variants      []string `gorm:"column:variants;type:longtext;serializer:json" json:"variants,omitempty"`
	TaskTypes     []string `gorm:"column:task_types;type:longtext;serializer:json" json:"task_types,omitempty"`
}

type KnowledgeCompileBacklog struct {
	KnowledgeCompileEntry `gorm:"embedded"`
}

func (KnowledgeCompileBacklog) TableName() string { return "knowledge_compile_docs_backlog" }

type KnowledgeCompileInflight struct {
	KnowledgeCompileEntry `gorm:"embedded"`
}

func (KnowledgeCompileInflight) TableName() string { return "knowledge_compile_docs_inflight" }
