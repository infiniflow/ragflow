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

//go:build !ee

package common

import (
	"encoding/json"
	"testing"
)

// TestIngestionLogJSONContract pins the JSON shape of the OSS IngestionLog type
// so a future refactor (e.g. moving it out of stub.go into its own file, or
// changing field tags) is caught if it alters the wire format that the EE sink
// and downstream consumers rely on.
func TestIngestionLogJSONContract(t *testing.T) {
	log := IngestionLog{
		TenantID:  "t1",
		DatasetID: "d1",
		DocID:     "doc1",
		UserID:    "u1",
		TaskID:    "task1",
		EventType: "completed",
		Status:    "success",
		Filename:  "a.pdf",
		FileType:  "pdf",
		FileSize:  1024,
		ParserID:  "parser1",
		Progress:  1.0,
		RunCount:  1,
	}

	b, err := json.Marshal(log)
	if err != nil {
		t.Fatalf("marshal IngestionLog: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Required, always-present keys.
	for _, key := range []string{
		"tenant_id", "dataset_id", "doc_id", "user_id", "task_id",
		"event_type", "status", "filename", "file_type", "file_size",
		"parser_id", "progress", "run_count",
	} {
		if _, ok := m[key]; !ok {
			t.Errorf("expected key %q present in JSON, got %s", key, string(b))
		}
	}

	// Optional fields must be omitted when empty (omitempty contract).
	for _, key := range []string{"user_name", "tenant_name", "error_info", "operation", "worker"} {
		if _, ok := m[key]; ok {
			t.Errorf("expected optional key %q omitted when empty, got %s", key, string(b))
		}
	}
	// Granularity fields are optional too — a plain task event omits them.
	for _, key := range []string{"pipeline_log_id", "component", "phase", "unit", "current", "total", "message"} {
		if _, ok := m[key]; ok {
			t.Errorf("expected granularity key %q omitted when empty, got %s", key, string(b))
		}
	}
}

// TestIngestionLogGranularityFieldsEmitted is the adversarial counterpart: every
// granularity field that is set must serialize. uint8/uint32 zero-value handling
// is the boundary — Phase/Current/Total=0 are omitted by omitempty, so we set
// non-zero values to assert they survive the JSON contract.
func TestIngestionLogGranularityFieldsEmitted(t *testing.T) {
	log := IngestionLog{
		TaskID:        "task1",
		PipelineLogID: "run-1",
		Component:     "Tokenizer",
		Phase:         2,
		Unit:          "chunk",
		Current:       257,
		Total:         3000,
		Message:       "batch 3",
	}
	b, err := json.Marshal(log)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for key, want := range map[string]any{
		"pipeline_log_id": "run-1",
		"component":       "Tokenizer",
		"phase":           float64(2),
		"unit":            "chunk",
		"current":         float64(257),
		"total":           float64(3000),
		"message":         "batch 3",
	} {
		if got, ok := m[key]; !ok || got != want {
			t.Errorf("granularity key %q = %v (present=%v), want %v; json=%s", key, got, ok, want, string(b))
		}
	}
}

// TestIngestionLogOptionalFieldsEmitted verifies that non-empty optional fields
// are serialized (the omitempty contract works both ways).
func TestIngestionLogOptionalFieldsEmitted(t *testing.T) {
	log := IngestionLog{
		TaskID:     "task1",
		UserName:   "alice",
		TenantName: "acme",
		Worker:     "worker-3",
		ErrorInfo:  "boom",
		Operation:  map[string]any{"step": "parse"},
	}
	b, err := json.Marshal(log)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"user_name", "tenant_name", "worker", "error_info", "operation"} {
		if _, ok := m[key]; !ok {
			t.Errorf("expected optional key %q emitted, got %s", key, string(b))
		}
	}
}

// TestIngestLogSinkNoop pins the OSS no-op behavior of the ingest-log sink:
// InitIngestLog returns nil, LogIngest is callable without panicking, and
// SyncIngestLog is callable without panicking. The real sink lives in the EE
// build; the OSS binary must keep the seam compiling and running safely.
func TestIngestLogSinkNoop(t *testing.T) {
	if err := InitIngestLog("file", FileOutput{}, "svc"); err != nil {
		t.Fatalf("InitIngestLog returned error in OSS build: %v", err)
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("LogIngest panicked: %v", r)
			}
		}()
		LogIngest(&IngestionLog{TaskID: "t", EventType: "completed"})
	}()

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("SyncIngestLog panicked: %v", r)
			}
		}()
		SyncIngestLog()
	}()
}
