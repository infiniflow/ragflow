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

import "fmt"

// Tier identifies the priority lane an ingestion task is published to. The
// three lanes are physically separate JetStream streams so a full low-priority
// lane can never reject a high-priority publish.
type Tier string

const (
	TierHigh   Tier = "high"
	TierMedium Tier = "medium"
	TierLow    Tier = "low"
)

const (
	// TaskSubject is the canonical community/OSS ingestion subject. The
	// open-source edition publishes here for its single (low-priority) lane,
	// and the EE low tier lane reuses it so an OSS split needs no rename and
	// OSS and EE can share one NATS server.
	TaskSubject = "tasks.RAGFLOW"

	// TaskSubjectHigh / TaskSubjectMedium / TaskSubjectLow are the three
	// tier-specific subjects. Producers resolve a tenant's tier and publish to
	// exactly one of these; the consumer drains them high -> medium -> low. The
	// low subject equals TaskSubject: the low lane IS the community queue.
	TaskSubjectHigh   = "tasks.RAGFLOW.high"
	TaskSubjectMedium = "tasks.RAGFLOW.medium"
	TaskSubjectLow    = TaskSubject
)

// SubjectForTier maps a tier to its JetStream subject. An unknown tier is an
// error, never a silent fallback to a default lane.
func SubjectForTier(tier Tier) (string, error) {
	switch tier {
	case TierHigh:
		return TaskSubjectHigh, nil
	case TierMedium:
		return TaskSubjectMedium, nil
	case TierLow:
		return TaskSubject, nil
	default:
		return "", fmt.Errorf("unknown task tier %q", tier)
	}
}

const (
	// MaxManualPullMessages is the largest task batch the administrative queue
	// pull endpoint accepts.
	MaxManualPullMessages = 100

	TaskTypeIngestionTask = "ingestion_task"
	TaskTypeIngestionTest = "ingestion_test"
	// TaskTypeSyncer is the NATS wake-up message type for datasource sync_logs tasks.
	TaskTypeSyncer = "syncer"
	// TaskTypeMemory is the async memory-extraction task type. Memory tasks
	// share the tiered tasks.RAGFLOW.<tier> subjects and the Ingestor's
	// consumer + worker pool with ingestion tasks; handleAndExecute dispatches
	// them by TaskType. Their TaskMessage is only a wake-up; input lives in
	// memory_task.
	TaskTypeMemory = "memory"
)

// TaskMessage is a broker wake-up that identifies one durable task.
type TaskMessage struct {
	TaskID   string `json:"task_id" binding:"required"`
	TaskType string `json:"task_type" binding:"required"`
}

// TaskHandle controls settlement and heartbeat for a received task message.
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
