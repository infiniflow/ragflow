package observability

import (
	"testing"

	"ragflow/internal/entity"
)

// TestBuildIngestLogPropagatesUserAndTenantName asserts the caller-supplied
// display names are carried onto the event verbatim. This keeps the OSS
// IngestionLog / BuildIngestLog contract aligned with the EE build, which
// resolves the names from the user/tenant DAOs before emitting.
func TestBuildIngestLogPropagatesUserAndTenantName(t *testing.T) {
	task := &entity.IngestionTask{ID: "t", DocumentID: "d", DatasetID: "kb", UserID: "u1"}
	doc := &entity.Document{Name: ptrStr("r"), Suffix: ".pdf", Type: "pdf", Size: 10, ParserID: "pdf"}
	run := &entity.PipelineOperationLog{RunCount: ptrInt(1), TenantID: "tenant-1"}

	ev := BuildIngestLog(task, doc, run, "completed", "COMPLETED", "", "alice", "Acme Corp")

	if ev.UserName != "alice" {
		t.Errorf("UserName = %q, want %q", ev.UserName, "alice")
	}
	if ev.TenantName != "Acme Corp" {
		t.Errorf("TenantName = %q, want %q", ev.TenantName, "Acme Corp")
	}
	if ev.UserID != "u1" {
		t.Errorf("UserID = %q, want u1", ev.UserID)
	}
	if ev.TenantID != "tenant-1" {
		t.Errorf("TenantID = %q, want tenant-1", ev.TenantID)
	}
}

// TestBuildIngestLogEmptyNamesStaysEmpty verifies that empty display names do
// not corrupt the event (the EE-side best-effort resolution may yield "").
func TestBuildIngestLogEmptyNamesStaysEmpty(t *testing.T) {
	task := &entity.IngestionTask{ID: "t", DocumentID: "d", DatasetID: "kb", UserID: "u1"}
	doc := &entity.Document{Name: ptrStr("r"), Suffix: ".pdf", Type: "pdf", Size: 10, ParserID: "pdf"}
	run := &entity.PipelineOperationLog{RunCount: ptrInt(1), TenantID: "tenant-1"}

	ev := BuildIngestLog(task, doc, run, "running", "RUNNING", "", "", "")

	if ev.UserName != "" {
		t.Errorf("UserName = %q, want empty", ev.UserName)
	}
	if ev.TenantName != "" {
		t.Errorf("TenantName = %q, want empty", ev.TenantName)
	}
	if ev.EventTime.IsZero() {
		t.Errorf("EventTime should be set, got zero")
	}
}
