package service

import (
	"context"
	"testing"

	"ragflow/internal/entity"
)

// TestEmitLifecycleEventPropagatesEmptyNames guards the OSS call site of
// BuildIngestLog. The OSS build forwards empty user/tenant names (the EE build
// resolves them from the user/tenant DAOs before emitting), so the emitted event
// must carry UserName == "" and TenantName == "" while still propagating the
// task identity. This also pins the call shape: BuildIngestLog must be invoked
// with the 8-arg form and EmitIngestLog must receive exactly one
// common.IngestionLog. The original bug passed the two extra "" args to
// EmitIngestLog (which takes only ctx+log) and omitted them from BuildIngestLog,
// which broke compilation of the whole service package.
func TestEmitLifecycleEventPropagatesEmptyNames(t *testing.T) {
	rec := &recordingEmitter{}
	svc := &IngestionTaskService{eventEmitter: rec}

	task := &entity.IngestionTask{
		ID:         "task-n",
		DocumentID: "doc-n",
		DatasetID:  "kb-n",
		UserID:     "u-n",
	}
	svc.emitLifecycleEvent(context.Background(), task, "completed", "COMPLETED", "")

	if len(rec.events) != 1 {
		t.Fatalf("expected exactly 1 emitted event, got %d", len(rec.events))
	}
	got := rec.last()
	if got.UserName != "" {
		t.Errorf("UserName = %q, want empty (OSS forwards empty names)", got.UserName)
	}
	if got.TenantName != "" {
		t.Errorf("TenantName = %q, want empty (OSS forwards empty names)", got.TenantName)
	}
	if got.UserID != "u-n" {
		t.Errorf("UserID = %q, want u-n", got.UserID)
	}
}
