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

// KnowledgeCompileDoc is the MySQL scheduling row for the dataset-level
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
type KnowledgeCompileDoc struct {
	DatasetID      string     `gorm:"primaryKey;column:dataset_id;size:64" json:"dataset_id"`
	TenantID       string     `gorm:"column:tenant_id;size:64;not null;default:''" json:"tenant_id"`
	BacklogDocIDs  string     `gorm:"column:backlog_doc_ids;type:text;not null;default:'[]'" json:"backlog_doc_ids"`
	InflightDocIDs string     `gorm:"column:inflight_doc_ids;type:text;not null;default:'[]'" json:"inflight_doc_ids"`
	ClaimOwner     string     `gorm:"column:claim_owner;size:64;not null;default:''" json:"claim_owner"`
	ClaimToken     string     `gorm:"column:claim_token;size:64;not null;default:''" json:"claim_token"`
	ClaimExpiresAt *time.Time `gorm:"column:claim_expires_at;default:null" json:"claim_expires_at"`
	Priority       int        `gorm:"column:priority;not null;default:0" json:"priority"`
	CreatedAt      time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName pins the scheduling table name.
func (KnowledgeCompileDoc) TableName() string { return "knowledge_compile_docs" }
