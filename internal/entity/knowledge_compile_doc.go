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
// the scheduling system of record: backlog_doc_ids holds the not-yet-processed
// doc entries for the KB, inflight_doc_ids the ones a worker has claimed (the
// closed batch), and the claim_* fields the owner/lease used for crash
// recovery. NATS notify is only a wake-up; same-KB serialization comes from
// these rows, not from the broker.
//
// The *_doc_ids columns store a JSON array of knowledge_compile.BacklogEntry
// (doc_id + event_type + seq) as TEXT so the consumer can re-apply the same
// out-of-order / tombstone guards as the broker-based design without re-reading
// the queue.
type KnowledgeCompileDataset struct {
	DatasetID string `gorm:"primaryKey;column:dataset_id;size:64" json:"dataset_id"`
	TenantID  string `gorm:"column:tenant_id;size:64;not null;default:''" json:"tenant_id"`
	// The *_doc_ids columns store a JSON array as TEXT. No DDL default is set:
	// MySQL (8.0.13+) rejects a literal DEFAULT on TEXT/BLOB columns (Error
	// 1101), and the application always writes "[]" explicitly on insert/update
	// (scheduler.go FirstOrCreate / release paths), so the default is redundant.
	BacklogDocIDs  string     `gorm:"column:backlog_doc_ids;type:text;not null" json:"backlog_doc_ids"`
	InflightDocIDs string     `gorm:"column:inflight_doc_ids;type:text;not null" json:"inflight_doc_ids"`
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
	ErrorMsg string `gorm:"column:error_msg;type:text;not null" json:"error_msg"`
	// LastCompletedAt records the last time the backlog drained to empty.
	LastCompletedAt *time.Time `gorm:"column:last_completed_at;default:null" json:"last_completed_at"`
	CreatedAt       time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName pins the scheduling table name.
func (KnowledgeCompileDataset) TableName() string { return "knowledge_compile_docs" }
