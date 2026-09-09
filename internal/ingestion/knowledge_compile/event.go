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

// defaultPublisher is the package-level Publisher used by the publishing path
// (PublishCompleted / PublishDeleted). It is installed by Provision (called
// once by the owning Ingestor at startup). Until then, publishing is a no-op.
var defaultPublisher Publisher

// defaultClaimer is the package-level Claimer handed to the consumer workers.
// It is the same underlying instance as defaultPublisher (a *mysqlScheduler
// satisfies both roles), set together by SetScheduler.
var defaultClaimer Claimer

// SetScheduler installs the package-level Publisher and Claimer used for
// publishing and consuming. The same *mysqlScheduler satisfies both interfaces.
func SetScheduler(s Publisher) {
	defaultPublisher = s
	if c, ok := s.(Claimer); ok {
		defaultClaimer = c
	}
}

// DefaultPublisher returns the package-level Publisher used by the producer path.
func DefaultPublisher() Publisher { return defaultPublisher }

// DefaultClaimer returns the package-level Claimer used by consumer workers.
func DefaultClaimer() Claimer { return defaultClaimer }

// PublishCompleted records a doc_completed event: it appends the doc to the
// KB's durable MySQL backlog and wakes idle workers over NATS. variants carries
// the doc-level compile types (tree/structure/wiki/mindmap) produced by this
// doc so the consumer can dispatch to the matching dataset-level path. It is a
// no-op when no Publisher has been installed (e.g. DB unavailable). A failure is
// returned so callers can log but never fail the pipeline on it.
func PublishCompleted(ctx context.Context, tenantID, datasetID, docID string, variants []string) error {
	if defaultPublisher == nil {
		return nil
	}
	return defaultPublisher.Publish(ctx, tenantID, datasetID, docID, string(EventTypeCompleted), variants)
}

// PublishDeleted records a doc_deleted event the same way (Publish handles the
// append + notify pairing). Deleted events carry no compile types.
func PublishDeleted(ctx context.Context, tenantID, datasetID, docID string) error {
	if defaultPublisher == nil {
		return nil
	}
	return defaultPublisher.Publish(ctx, tenantID, datasetID, docID, string(EventTypeDeleted), nil)
}
