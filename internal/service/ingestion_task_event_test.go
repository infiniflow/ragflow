package service

import (
	"context"
	"sync"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

// recordingEmitter captures emitted ingest logs for assertions.
type recordingEmitter struct {
	mu     sync.Mutex
	events []common.IngestLog
}

func (r *recordingEmitter) EmitIngestLog(_ context.Context, log common.IngestLog) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, log)
}

func (r *recordingEmitter) last() common.IngestLog {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.events) == 0 {
		return common.IngestLog{}
	}
	return r.events[len(r.events)-1]
}

// TestEmitLifecycleEventForwardsToEmitter verifies the service builds an
// observability event from the task and forwards it to the configured emitter.
func TestEmitLifecycleEventForwardsToEmitter(t *testing.T) {
	rec := &recordingEmitter{}
	svc := &IngestionTaskService{eventEmitter: rec}

	task := &entity.IngestionTask{
		ID:         "task-1",
		DocumentID: "doc-1",
		DatasetID:  "kb-1",
		UserID:     "u-1",
	}
	svc.emitLifecycleEvent(context.Background(), task, "completed", "COMPLETED", "boom")

	got := rec.last()
	if got.EventType != "completed" {
		t.Errorf("EventType = %q, want completed", got.EventType)
	}
	if got.Status != "COMPLETED" {
		t.Errorf("Status = %q, want COMPLETED", got.Status)
	}
	if got.ErrorInfo != "boom" {
		t.Errorf("ErrorInfo = %q, want boom", got.ErrorInfo)
	}
	if got.TaskID != "task-1" || got.DocID != "doc-1" || got.DatasetID != "kb-1" || got.UserID != "u-1" {
		t.Errorf("identity mismatch: %+v", got)
	}
}

// TestEmitLifecycleEventNilEmitterIsNoop verifies a nil emitter never panics and
// a nil task is ignored (best-effort contract).
func TestEmitLifecycleEventNilEmitterIsNoop(t *testing.T) {
	svc := &IngestionTaskService{}
	task := &entity.IngestionTask{ID: "t", DocumentID: "d"}
	svc.emitLifecycleEvent(context.Background(), task, "created", "CREATED", "")
	svc.emitLifecycleEvent(context.Background(), nil, "created", "CREATED", "")
}

// TestSetEventEmitterSwapsSink verifies SetEventEmitter installs a new sink and
// that a nil argument is ignored (cannot disable emission by accident).
func TestSetEventEmitterSwapsSink(t *testing.T) {
	svc := &IngestionTaskService{}
	rec := &recordingEmitter{}
	svc.SetEventEmitter(nil) // ignored
	svc.SetEventEmitter(rec)

	task := &entity.IngestionTask{ID: "t2", DocumentID: "d2", DatasetID: "kb2", UserID: "u2"}
	svc.emitLifecycleEvent(context.Background(), task, "running", "RUNNING", "")

	if len(rec.events) != 1 {
		t.Fatalf("expected 1 emitted event, got %d", len(rec.events))
	}
	if rec.last().EventType != "running" {
		t.Errorf("EventType = %q, want running", rec.last().EventType)
	}
}
