package service

import (
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/ingestion"
	"ragflow/internal/observability"
)

// TestNewIngestorSeedsContextIdentity locks that the ingestor seeds its
// process-level ingest identity (ingestor name + NATS subject) onto the root
// context. Every lifecycle event emitted through this ingestor therefore
// attributes to it, including scheduled/created events raised outside a worker.
// The per-worker name is added per task and must NOT be present here.
func TestNewIngestorSeedsContextIdentity(t *testing.T) {
	ing := NewIngestor("ing-test-1", 1, []string{"pdf"}, &ingestion.Deps{})

	if got := observability.IngestorName(ing.ctx); got != "ing-test-1" {
		t.Errorf("IngestorName on root ctx = %q, want ing-test-1", got)
	}
	if got := observability.NatsQueue(ing.ctx); got != common.TaskSubject {
		t.Errorf("NatsQueue on root ctx = %q, want %q", got, common.TaskSubject)
	}
	if got := observability.WorkerName(ing.ctx); got != "" {
		t.Errorf("WorkerName on root ctx = %q, want empty (added per task)", got)
	}
}
