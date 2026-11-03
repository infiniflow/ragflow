//go:build !ee

package common

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func resetIngestLog() {
	ingestLogger.Store(zap.NewNop())
}

// TestIngestLogUnifiedStruct asserts the single unified type: renamed fields
// (DocID not DocumentID, EventType not TaskType) and the absence of the
// un-serializable Error error interface. Structural guard for 方案B unification.
func TestIngestLogUnifiedStruct(t *testing.T) {
	typ := reflect.TypeOf(IngestionLog{})
	for _, f := range []string{"DocID", "EventType", "ErrorInfo", "Code"} {
		if _, ok := typ.FieldByName(f); !ok {
			t.Errorf("IngestionLog missing expected unified field %q", f)
		}
	}
	for _, bad := range []string{"DocumentID", "TaskType", "Error"} {
		if _, ok := typ.FieldByName(bad); ok {
			t.Errorf("IngestionLog must not keep legacy field %q after unification", bad)
		}
	}
}

// TestIngestLogEmitsUnifiedJSON happy path: flat JSON line with error_info (string) and code.
func TestIngestLogEmitsUnifiedJSON(t *testing.T) {
	defer resetIngestLog()
	dir := t.TempDir()
	if err := InitIngestLog("json", FileOutput{Path: dir, Filename: "ingest.log"}, "svc"); err != nil {
		t.Fatalf("InitIngestLog: %v", err)
	}
	LogIngestEvent(IngestionLog{
		EventTime: time.Now().UTC(),
		TenantID:  "t1",
		DatasetID: "kb1",
		DocID:     "d1",
		UserID:    "u1",
		TaskID:    "task1",
		EventType: "completed",
		Status:    "COMPLETED",
		ErrorInfo: "boom",
		Filename:  "report.pdf",
		FileType:  "pdf",
		FileSize:  12345,
		ParserID:  "pdf",
		Code:      7,
	})
	SyncIngestLog()
	raw, err := os.ReadFile(filepath.Join(dir, "ingest.log"))
	if err != nil {
		t.Fatalf("read sink: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("emitted line not JSON: %v (raw=%q)", err, raw)
	}
	if m["error_info"] != "boom" {
		t.Errorf("error_info = %v, want boom (string, not error interface)", m["error_info"])
	}
	if c, ok := m["code"].(float64); !ok || c != 7 {
		t.Errorf("code = %v (%T), want 7", m["code"], m["code"])
	}
	if _, ok := m["error"]; ok {
		t.Errorf("legacy error key must not be present; only error_info is allowed")
	}
}

// TestIngestLogCodeRoundTrips adversarial: Code is a first-class struct field,
// not hard-coded 0 via a secondary mapping layer.
func TestIngestLogCodeRoundTrips(t *testing.T) {
	defer resetIngestLog()
	dir := t.TempDir()
	if err := InitIngestLog("json", FileOutput{Path: dir, Filename: "ingest.log"}, "svc"); err != nil {
		t.Fatalf("InitIngestLog: %v", err)
	}
	LogIngestEvent(IngestionLog{TaskID: "task1", EventType: "failed", Status: "FAILED", Code: 7})
	SyncIngestLog()
	raw, _ := os.ReadFile(filepath.Join(dir, "ingest.log"))
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("emitted line not JSON: %v (raw=%q)", err, raw)
	}
	if c, ok := m["code"].(float64); !ok || c != 7 {
		t.Errorf("code = %v (%T), want 7 (first-class struct field, not hard-coded 0)", m["code"], m["code"])
	}
}

// TestIngestLogOmitsZeroTimes adversarial: zero-value times must not leak.
func TestIngestLogOmitsZeroTimes(t *testing.T) {
	defer resetIngestLog()
	dir := t.TempDir()
	if err := InitIngestLog("json", FileOutput{Path: dir, Filename: "ingest.log"}, "svc"); err != nil {
		t.Fatalf("InitIngestLog: %v", err)
	}
	LogIngestEvent(IngestionLog{EventTime: time.Now().UTC(), TaskID: "t1", EventType: "created", Status: "CREATED"})
	SyncIngestLog()
	raw, _ := os.ReadFile(filepath.Join(dir, "ingest.log"))
	if strings.Contains(string(raw), "0001-01-01") {
		t.Errorf("emitted zero-time sentinel 0001-01-01: %s", raw)
	}
	var m map[string]any
	json.Unmarshal(raw, &m)
	for _, k := range []string{"begin_time", "end_time", "upload_time", "enqueue_time"} {
		if _, ok := m[k]; ok {
			t.Errorf("zero-time field %q must be omitted, got %v", k, m[k])
		}
	}
	if _, ok := m["event_time"]; !ok {
		t.Errorf("event_time must always be present")
	}
}

// TestIngestLogOmitsEmptyErrorInfo adversarial: empty ErrorInfo omitted.
func TestIngestLogOmitsEmptyErrorInfo(t *testing.T) {
	defer resetIngestLog()
	dir := t.TempDir()
	if err := InitIngestLog("json", FileOutput{Path: dir, Filename: "ingest.log"}, "svc"); err != nil {
		t.Fatalf("InitIngestLog: %v", err)
	}
	LogIngestEvent(IngestionLog{TaskID: "t1", EventType: "created", Status: "CREATED"})
	SyncIngestLog()
	raw, _ := os.ReadFile(filepath.Join(dir, "ingest.log"))
	var m map[string]any
	json.Unmarshal(raw, &m)
	if _, ok := m["error_info"]; ok {
		t.Errorf("empty error_info must be omitted, got %v", m["error_info"])
	}
}

// TestIngestLogOmitsEmptyUserName corner.
func TestIngestLogOmitsEmptyUserName(t *testing.T) {
	defer resetIngestLog()
	dir := t.TempDir()
	if err := InitIngestLog("json", FileOutput{Path: dir, Filename: "ingest.log"}, "svc"); err != nil {
		t.Fatalf("InitIngestLog: %v", err)
	}
	LogIngestEvent(IngestionLog{TaskID: "task1", EventType: "created", Status: "CREATED"})
	SyncIngestLog()
	raw, _ := os.ReadFile(filepath.Join(dir, "ingest.log"))
	if strings.Contains(string(raw), "user_name") {
		t.Fatalf("emitted line contains user_name when UserName is empty: %s", raw)
	}
}

// TestIngestLogOneFlatLine adversarial: each event is exactly one JSON line.
func TestIngestLogOneFlatLine(t *testing.T) {
	defer resetIngestLog()
	dir := t.TempDir()
	if err := InitIngestLog("json", FileOutput{Path: dir, Filename: "ingest.log"}, "svc"); err != nil {
		t.Fatalf("InitIngestLog: %v", err)
	}
	LogIngestEvent(IngestionLog{TaskID: "task1", EventType: "completed", Status: "COMPLETED", ErrorInfo: "boom"})
	SyncIngestLog()
	raw, _ := os.ReadFile(filepath.Join(dir, "ingest.log"))
	if len(bytes.TrimSpace(raw)) == 0 {
		t.Fatal("LogIngest wrote no output")
	}
	lines := 0
	for _, l := range bytes.Split(raw, []byte("\n")) {
		if len(bytes.TrimSpace(l)) == 0 {
			continue
		}
		lines++
		var m map[string]any
		if err := json.Unmarshal(l, &m); err != nil {
			t.Fatalf("non-JSON line: %v (raw=%q)", err, l)
		}
	}
	if lines != 1 {
		t.Fatalf("expected 1 line, got %d (raw=%q)", lines, raw)
	}
}
