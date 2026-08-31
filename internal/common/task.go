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

package common

import "encoding/json"

const (
	// TaskSubject is the NATS subject on which ingestion and memory tasks are
	// published and consumed. Producer and consumer must reference this single
	// symbol so the routing contract cannot diverge (mirrors the RAGFLOW_TASKS
	// JetStream subject in internal/engine/nats).
	TaskSubject = "tasks.RAGFLOW"

	TaskTypeIngestionTask = "ingestion_task"
	TaskTypeIngestionTest = "ingestion_test"
	// TaskTypeSyncer is the NATS wake-up message type for datasource sync_logs tasks.
	TaskTypeSyncer = "syncer"
	// TaskTypeMemory is the async memory-extraction task type. Memory tasks
	// share the tasks.RAGFLOW subject and the Ingestor's consumer + worker
	// pool with ingestion tasks; processMessage dispatches them by TaskType.
	// The memory-specific payload (message_dict/memory_id/source_id) is
	// carried in TaskMessage.Payload.
	TaskTypeMemory = "memory"
)

type TaskMessage struct {
	TaskID   string `json:"task_id" binding:"required"`
	TaskType string `json:"task_type" binding:"required"`
	// Payload carries the task-specific body for non-ingestion task types
	// (e.g. the memory extraction payload). It is left empty for ingestion
	// tasks and old messages, so existing consumers are unaffected.
	Payload json.RawMessage `json:"payload,omitempty"`
}

type TaskHandle interface {
	GetMessage() TaskMessage
	Ack() error
	Nack() error

	// InProgress resets the AckWait timer without acknowledging the message,
	// signalling the broker that the worker is still processing. Call
	// periodically during long tasks to avoid in-flight redelivery.
	InProgress() error
}

// RawMessage is a broker message carrying opaque bytes (used by the dataset-level
// compile consumer, which publishes arbitrary JSON payloads rather than the
// TaskMessage shape). The NATS engine returns RawMessage from FetchKnowledgeCompileMessages.
type RawMessage interface {
	Data() []byte
	Ack() error
	Nak() error
}
