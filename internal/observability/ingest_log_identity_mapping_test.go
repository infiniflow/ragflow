package observability

import (
	"context"
	"testing"

	"ragflow/internal/entity"
)

// TestBuildIngestLogMapsIngestorAndQueue guards that the identity carried on the
// context is mapped onto the struct fields, and that the existing name mapping
// is untouched.
func TestBuildIngestLogMapsIngestorAndQueue(t *testing.T) {
	name := "report.pdf"
	task := &entity.IngestionTask{ID: "t1", DocumentID: "d1", DatasetID: "k1", UserID: "u1"}
	doc := &entity.Document{Name: &name, Suffix: ".pdf"}
	run := &entity.PipelineOperationLog{RunCount: ptrInt(1)}

	ctx := WithIngestIdentity(context.Background(), IngestIdentity{
		UserName:     "alice",
		TenantName:   "Acme",
		IngestorName: "ing-7",
		NatsQueue:    "tasks.RAGFLOW",
	})
	ev := BuildIngestLog(ctx, task, doc, run, "running", "RUNNING", "")
	if ev.IngestorName != "ing-7" {
		t.Errorf("IngestorName = %q, want ing-7", ev.IngestorName)
	}
	if ev.NatsQueue != "tasks.RAGFLOW" {
		t.Errorf("NatsQueue = %q, want tasks.RAGFLOW", ev.NatsQueue)
	}
	if ev.UserName != "alice" || ev.TenantName != "Acme" {
		t.Errorf("name mapping regressed: user=%q tenant=%q", ev.UserName, ev.TenantName)
	}

	// Adversarial: empty identity must map to empty fields, no panic.
	ev2 := BuildIngestLog(context.Background(), task, doc, run, "running", "RUNNING", "")
	if ev2.IngestorName != "" || ev2.NatsQueue != "" {
		t.Errorf("empty identity should map to empty fields, got %q/%q", ev2.IngestorName, ev2.NatsQueue)
	}
}
