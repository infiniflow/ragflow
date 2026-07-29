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

// Package knowledge_compile implements the dataset-level post-processing consumer described in
// docs/develop/knowledge_compile_design.md §11 (Option E).
//
// Pipeline (KnowledgeCompiler, per document) writes compiled chunks with
// available_int=0. Its completion/deletion is recorded by appending a
// BacklogEntry to the KB's durable MySQL scheduling row
// (knowledge_compile_docs), then waking idle workers over NATS. A cluster of
// competing workers claims a closed batch (backlog -> inflight) per KB, runs
// dataset-level dedup on that batch, and writes the merged dataset-level
// products with available_int=1. The MySQL row — not the broker — is the
// scheduling system of record and the source of same-KB serialization.
package knowledge_compile

import (
	"context"
	"encoding/json"
)

// Subjects and event types for the knowledge-compile stream. Both sit under the
// knowledge.compile.events.> prefix declared on the NATS stream/consumer.
const (
	SubjectCompleted = "knowledge.compile.events.completed"
	SubjectDeleted   = "knowledge.compile.events.deleted"
)

// EventType enumerates the KC event kinds.
type EventType string

const (
	EventTypeCompleted EventType = "doc_completed"
	EventTypeDeleted   EventType = "doc_deleted"
)

// KCCompileEvent is the payload published when a document's pipeline finishes
// (doc_completed) or is deleted (doc_deleted). It is intentionally decoupled
// from common.TaskMessage because the consumer reads the raw JSON body.
type KCCompileEvent struct {
	TenantID  string `json:"tenant_id"`
	DatasetID string `json:"dataset_id"` // the KB scope
	DocID     string `json:"doc_id"`     // the contributing document
	EventType string `json:"event_type"` // EventType value
	Seq       uint64 `json:"seq"`        // per-doc monotonic sequence, for out-of-order correction
	Timestamp int64  `json:"ts"`
}

// Subject returns the NATS subject for this event.
func (e KCCompileEvent) Subject() string {
	if EventType(e.EventType) == EventTypeDeleted {
		return SubjectDeleted
	}
	return SubjectCompleted
}

// Marshal serializes the event to JSON.
func (e KCCompileEvent) Marshal() ([]byte, error) { return json.Marshal(e) }

// ParseEvent deserializes a KCCompileEvent from raw message bytes.
func ParseEvent(data []byte) (KCCompileEvent, error) {
	var e KCCompileEvent
	if err := json.Unmarshal(data, &e); err != nil {
		return KCCompileEvent{}, err
	}
	return e, nil
}

// defaultScheduler is the package-level Scheduler used by the publishing path
// (PublishCompleted / PublishDeleted). It is installed by Provision (called
// once by the owning Ingestor at startup). Until then, publishing is a no-op.
var defaultScheduler Scheduler

// SetScheduler installs the package-level Scheduler used for publishing.
func SetScheduler(s Scheduler) { defaultScheduler = s }

// DefaultScheduler returns the package-level Scheduler (nil until Provision).
func DefaultScheduler() Scheduler { return defaultScheduler }

// PublishCompleted records a doc_completed event: it appends the doc to the
// KB's durable MySQL backlog and wakes idle workers over NATS. It is a no-op
// when no Scheduler has been installed (e.g. DB unavailable). A failure is
// returned so callers can log but never fail the pipeline on it.
func PublishCompleted(ctx context.Context, tenantID, datasetID, docID string, seq uint64) error {
	if defaultScheduler == nil {
		return nil
	}
	if err := defaultScheduler.AppendBacklog(ctx, tenantID, datasetID, docID, string(EventTypeCompleted), seq); err != nil {
		return err
	}
	return defaultScheduler.Notify(ctx, datasetID)
}

// PublishDeleted records a doc_deleted event the same way (append + notify).
func PublishDeleted(ctx context.Context, tenantID, datasetID, docID string, seq uint64) error {
	if defaultScheduler == nil {
		return nil
	}
	if err := defaultScheduler.AppendBacklog(ctx, tenantID, datasetID, docID, string(EventTypeDeleted), seq); err != nil {
		return err
	}
	return defaultScheduler.Notify(ctx, datasetID)
}
