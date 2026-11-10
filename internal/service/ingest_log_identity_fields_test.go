package service

import (
	"context"
	"testing"

	"ragflow/internal/entity"
)

// TestEmitLifecycleEventLeavesIdentityFieldsEmpty guards the OSS contract: the
// OSS emit path forwards empty ingestor/queue identity (the EE build populates
// them from its own context). It must keep propagating task identity without
// panicking and leave the new fields empty.
func TestEmitLifecycleEventLeavesIdentityFieldsEmpty(t *testing.T) {
	rec := &recordingEmitter{}
	svc := &IngestionTaskService{eventEmitter: rec}
	task := &entity.IngestionTask{ID: "t1", DocumentID: "d1", DatasetID: "k1", UserID: "u1"}
	svc.emitLifecycleEvent(context.Background(), task, "completed", "COMPLETED", "")

	if len(rec.events) != 1 {
		t.Fatalf("expected exactly 1 emitted event, got %d", len(rec.events))
	}
	got := rec.last()
	if got.IngestorName != "" {
		t.Errorf("IngestorName = %q, want empty (OSS forwards empty identity)", got.IngestorName)
	}
	if got.NatsQueue != "" {
		t.Errorf("NatsQueue = %q, want empty (OSS forwards empty identity)", got.NatsQueue)
	}
}
