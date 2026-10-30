//go:build !ee

package common

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

// resetIngestLog restores the default no-op logger so tests are isolated.
func resetIngestLog() {
	ingestLogger.Store(zap.NewNop())
}

func TestIngestLogJSONShape(t *testing.T) {
	log := IngestLog{
		EventTime: time.Now().UTC(),
		TenantID:  "t1",
		DatasetID: "kb1",
		DocID:     "d1",
		UserID:    "u1",
		UserName:  "alice",
		Worker:    "w1",
		TaskID:    "task1",
		EventType: "completed",
		Status:    "COMPLETED",

		ErrorInfo:      "boom",
		Filename:       "report.pdf",
		FileType:       "pdf",
		FileSize:       12345,
		ParserID:       "pdf",
		UploadTime:     time.Now().UTC(),
		EnqueueTime:    time.Now().UTC(),
		BeginTime:      time.Now().UTC(),
		EndTime:        time.Now().UTC(),
		DurationMS:     1500,
		WaitDurationS:  30,
		ParseDurationS: 120,
		Progress:       1,
		RunCount:       2,
		Operation:      map[string]any{"k": "v"},
	}
	b, err := json.Marshal(log)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{
		"event_time", "tenant_id", "dataset_id", "doc_id", "user_id", "user_name",
		"worker", "task_id", "event_type", "status", "error_info",
		"filename", "file_type", "file_size", "parser_id", "upload_time",
		"enqueue_time", "begin_time", "end_time", "duration_ms", "wait_duration_s",
		"parse_duration_s", "progress", "run_count", "operation",
	} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing key %q in emitted JSON", k)
		}
	}
	if _, ok := m["error"]; ok {
		t.Errorf("legacy `error` key must not be present; only `error_info` is allowed")
	}
	if v, ok := m["error_info"].(string); !ok || v != "boom" {
		t.Errorf("error_info = %v (%T), want string \"boom\"", m["error_info"], m["error_info"])
	}
}

func TestLogIngestTransmitsOneFlatLine(t *testing.T) {
	defer resetIngestLog()
	dir := t.TempDir()
	if err := InitIngestLog("json", FileOutput{Path: dir, Filename: "ingest.log"}, "svc"); err != nil {
		t.Fatalf("InitIngestLog: %v", err)
	}
	LogIngest(IngestLog{TaskID: "task1", EventType: "completed", Status: "COMPLETED", ErrorInfo: "boom"})

	SyncIngestLog()
	raw, err := os.ReadFile(filepath.Join(dir, "ingest.log"))
	if err != nil {
		t.Fatalf("read sink: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("LogIngest wrote no output")
	}
	if countLines(raw) != 1 {
		t.Fatalf("expected 1 line, got %d (raw=%q)", countLines(raw), raw)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("emitted line not JSON: %v (raw=%q)", err, raw)
	}
	if m["event_type"] != "completed" {
		t.Errorf("event_type = %v, want completed (must be flat top-level)", m["event_type"])
	}
	if m["error_info"] != "boom" {
		t.Errorf("error_info = %v, want boom", m["error_info"])
	}
}

func TestLogIngestDefaultIsNoop(t *testing.T) {
	defer resetIngestLog()
	ingestLogger.Store(zap.NewNop())
	dir := t.TempDir()
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("LogIngest panicked on no-op logger: %v", r)
			}
		}()
		LogIngest(IngestLog{TaskID: "x", EventType: "created"})
	}()
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("no-op LogIngest must not create output files, got %v", entries)
	}
}

func TestLogIngestZeroStructStillEmitsValidJSON(t *testing.T) {
	defer resetIngestLog()
	dir := t.TempDir()
	if err := InitIngestLog("json", FileOutput{Path: dir, Filename: "ingest.log"}, "svc"); err != nil {
		t.Fatalf("InitIngestLog: %v", err)
	}
	LogIngest(IngestLog{})
	SyncIngestLog()
	raw, err := os.ReadFile(filepath.Join(dir, "ingest.log"))
	if err != nil {
		t.Fatalf("read sink: %v", err)
	}
	if err := json.Unmarshal(raw, &map[string]any{}); err != nil {
		t.Fatalf("zero-value event did not emit valid JSON: %v (raw=%q)", err, raw)
	}
}

func TestLogIngestConcurrentNoInterleaveAndNoPanic(t *testing.T) {
	defer resetIngestLog()
	dir := t.TempDir()
	if err := InitIngestLog("json", FileOutput{Path: dir, Filename: "ingest.log"}, "svc"); err != nil {
		t.Fatalf("InitIngestLog: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			LogIngest(IngestLog{TaskID: "task", EventType: "completed"})
		}()
	}
	wg.Wait()
	SyncIngestLog()
	raw, err := os.ReadFile(filepath.Join(dir, "ingest.log"))
	if err != nil {
		t.Fatalf("read sink: %v", err)
	}
	n := 0
	for _, line := range bytes.Split(raw, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		if err := json.Unmarshal(line, &map[string]any{}); err != nil {
			t.Fatalf("concurrent emit produced non-JSON line: %v (raw=%q)", err, line)
		}
		n++
	}
	if n != 50 {
		t.Fatalf("expected 50 valid JSON lines, got %d", n)
	}
}

func TestInitIngestLogReinitSwitchesSink(t *testing.T) {
	defer resetIngestLog()
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	if err := InitIngestLog("json", FileOutput{Path: dir1, Filename: "a.log"}, "svc"); err != nil {
		t.Fatal(err)
	}
	LogIngest(IngestLog{TaskID: "first"})
	if err := InitIngestLog("json", FileOutput{Path: dir2, Filename: "b.log"}, "svc"); err != nil {
		t.Fatal(err)
	}
	LogIngest(IngestLog{TaskID: "second"})
	if _, err := os.Stat(filepath.Join(dir1, "a.log")); err != nil {
		t.Fatalf("first sink missing: %v", err)
	}
	SyncIngestLog()
	raw, err := os.ReadFile(filepath.Join(dir2, "b.log"))
	if err != nil {
		t.Fatalf("second sink missing: %v", err)
	}
	if !bytes.Contains(raw, []byte("second")) {
		t.Errorf("re-init did not switch sink; second file = %q", raw)
	}
	if bytes.Contains(raw, []byte("first")) {
		t.Errorf("re-init leaked into new sink: %q", raw)
	}
}

func countLines(b []byte) int {
	n := 0
	for _, c := range b {
		if c == 10 {
			n++
		}
	}
	return n
}

func TestLogIngestOmitsZeroTimes(t *testing.T) {
	defer resetIngestLog()
	dir := t.TempDir()
	if err := InitIngestLog("json", FileOutput{Path: dir, Filename: "ingest.log"}, "svc"); err != nil {
		t.Fatalf("InitIngestLog: %v", err)
	}
	// Only EventTime is set; every other time field is the zero value.
	LogIngest(IngestLog{EventTime: time.Now().UTC(), TaskID: "t1", EventType: "created", Status: "CREATED"})
	SyncIngestLog()
	raw, err := os.ReadFile(filepath.Join(dir, "ingest.log"))
	if err != nil {
		t.Fatalf("read sink: %v", err)
	}
	if bytes.Contains(raw, []byte("0001-01-01")) {
		t.Errorf("emitted zero-time sentinel 0001-01-01: %s", raw)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("emitted line not JSON: %v (raw=%s)", err, raw)
	}
	for _, k := range []string{"begin_time", "end_time", "upload_time", "enqueue_time"} {
		if _, ok := m[k]; ok {
			t.Errorf("zero-time field %q must be omitted, got %v", k, m[k])
		}
	}
	if _, ok := m["event_time"]; !ok {
		t.Errorf("event_time must always be present")
	}
}

func TestIngestLoggerConcurrentInitAndEmit(t *testing.T) {
	defer resetIngestLog()
	dir := t.TempDir()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = InitIngestLog("json", FileOutput{Path: dir, Filename: "ingest.log"}, "svc")
			LogIngest(IngestLog{EventTime: time.Now().UTC(), TaskID: "t", EventType: "created"})
			SyncIngestLog()
		}(i)
	}
	wg.Wait()
	resetIngestLog()
}
