package service

import (
	"context"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/observability"
)

// TestEmitLifecycleEventCarriesContextIdentity guards the OSS call site of
// BuildIngestLog: ingest identity is read from the context, not positional
// params. When no identity is injected the emitted event must carry empty
// name/ingestor/queue fields (the EE build resolves them before emitting), while
// still propagating the task identity. When identity IS injected on the context,
// it must flow through to the emitted event verbatim. This pins the call shape:
// BuildIngestLog is invoked with ctx as its first arg and EmitIngestLog receives
// exactly one common.IngestionLog.
func TestEmitLifecycleEventCarriesContextIdentity(t *testing.T) {
	task := &entity.IngestionTask{
		ID:         "task-n",
		DocumentID: "doc-n",
		DatasetID:  "kb-n",
		UserID:     "u-n",
	}

	// Case 1: no identity on ctx -> empty name/identity fields.
	rec := &recordingEmitter{}
	svc := &IngestionTaskService{eventEmitter: rec}
	svc.emitLifecycleEvent(context.Background(), task, "completed", "COMPLETED", "")

	if !waitForEmitCount(rec.count, 1) {
		t.Fatalf("event not emitted (async) within timeout")
	}
	if rec.count() != 1 {
		t.Fatalf("expected exactly 1 emitted event, got %d", rec.count())
	}
	got := rec.last()
	if got.UserName != "" {
		t.Errorf("UserName = %q, want empty (no identity injected)", got.UserName)
	}
	if got.TenantName != "" {
		t.Errorf("TenantName = %q, want empty (no identity injected)", got.TenantName)
	}
	if got.IngestorName != "" || got.NatsQueue != "" || got.Worker != "" {
		t.Errorf("identity fields = %q/%q/%q, want empty (no identity injected)", got.IngestorName, got.NatsQueue, got.Worker)
	}
	if got.UserID != "u-n" {
		t.Errorf("UserID = %q, want u-n", got.UserID)
	}

	// Case 2: identity injected on ctx -> flows through verbatim.
	rec2 := &recordingEmitter{}
	svc2 := &IngestionTaskService{eventEmitter: rec2}
	ctx := observability.WithIngestIdentity(context.Background(), observability.IngestIdentity{
		UserName:     "alice",
		TenantName:   "Acme",
		IngestorName: "ing-7",
		NatsQueue:    "tasks.RAGFLOW",
		WorkerName:   "worker-3",
	})
	svc2.emitLifecycleEvent(ctx, task, "running", "RUNNING", "")

	if !waitForEmitCount(rec2.count, 1) {
		t.Fatalf("event not emitted (async) within timeout")
	}
	if rec2.count() != 1 {
		t.Fatalf("expected exactly 1 emitted event, got %d", rec2.count())
	}
	got2 := rec2.last()
	if got2.UserName != "alice" || got2.TenantName != "Acme" {
		t.Errorf("names = %q/%q, want alice/Acme", got2.UserName, got2.TenantName)
	}
	if got2.IngestorName != "ing-7" || got2.NatsQueue != "tasks.RAGFLOW" || got2.Worker != "worker-3" {
		t.Errorf("identity = %q/%q/%q, want ing-7/tasks.RAGFLOW/worker-3", got2.IngestorName, got2.NatsQueue, got2.Worker)
	}
}
