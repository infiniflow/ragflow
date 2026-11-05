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
		Code:      0,
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
		"parser_id", "progress", "run_count", "code",
	} {
		if _, ok := m[key]; !ok {
			t.Errorf("expected key %q present in JSON, got %s", key, string(b))
		}
	}

	// Optional fields must be omitted when empty (omitempty contract).
	for _, key := range []string{"user_name", "tenant_name", "error_info", "operation"} {
		if _, ok := m[key]; ok {
			t.Errorf("expected optional key %q omitted when empty, got %s", key, string(b))
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
	for _, key := range []string{"user_name", "tenant_name", "error_info", "operation"} {
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
