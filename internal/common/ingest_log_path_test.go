//go:build !ee

package common

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestInitIngestLogPathTreatedAsFile is the regression test for P1b: the configured
// ingestor.log.path is a FILE name (per conf/service_conf.yaml comment), so InitIngestLog
// must write to that exact file and must NOT treat it as a directory.
func TestInitIngestLogPathTreatedAsFile(t *testing.T) {
	t.Cleanup(resetIngestLog)

	dir := t.TempDir()
	filePath := filepath.Join(dir, "ingest_log.log")

	if err := InitIngestLog("file", FileOutput{Path: filePath}, "api_server_9381"); err != nil {
		t.Fatalf("InitIngestLog: %v", err)
	}

	LogIngestEvent(IngestionLog{EventTime: time.Now().UTC(), TaskID: "t1", EventType: "created", Status: "CREATED"})
	SyncIngestLog()

	fi, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("expected log file at %s: %v", filePath, err)
	}
	if fi.IsDir() {
		t.Fatalf("%s was created as a DIRECTORY; path must be treated as a file", filePath)
	}

	if stray, err := os.Stat(filepath.Join(dir, "ingest_log.log")); err == nil && stray.IsDir() {
		t.Fatalf("stray directory %s/ created; path was misinterpreted as a directory", dir)
	}
}

// TestInitIngestLogEmptyPathNoFile asserts the documented default: an empty path emits to
// stdout only and opens no rotated file. This guards the off-by-default contract.
func TestInitIngestLogEmptyPathNoFile(t *testing.T) {
	t.Cleanup(resetIngestLog)

	dir := t.TempDir()
	if err := InitIngestLog("file", FileOutput{}, "api_server_9381"); err != nil {
		t.Fatalf("InitIngestLog: %v", err)
	}
	LogIngestEvent(IngestionLog{EventTime: time.Now().UTC(), TaskID: "t1"})
	SyncIngestLog()

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if nameContains(e.Name(), "ingest") {
			t.Fatalf("empty path opened a file under %s: %s", dir, e.Name())
		}
	}
}

func nameContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
