//go:build !ee

package observability

import (
	"testing"
	"time"

	"ragflow/internal/entity"
)

// TestClaimTimeRejectsStaleDocumentValue verifies that a document claim time left
// over from a PREVIOUS run (i.e. before this run even opened) is NOT treated as
// this run's claim time. This is the root cause of P1a: doc.ProcessBeginAt is
// stamped by PrepareValidatedRun and never cleared between runs, so a task that
// never reaches RUNNING in the current run must not inherit a stale value.
func TestClaimTimeRejectsStaleDocumentValue(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	staleClaim := base.Add(-48 * time.Hour) // two days before this run opened
	runOpen := base.Add(-1 * time.Hour)
	doc := &entity.Document{ProcessBeginAt: ptrTime(staleClaim)}
	run := &entity.PipelineOperationLog{CreateDate: ptrTime(runOpen)}

	if got := claimTime(doc, run); got != nil {
		t.Fatalf("claimTime returned %v for a stale document value; want nil", got)
	}
}

// TestClaimTimeAcceptsValidClaimTime verifies a claim time that falls within this
// run's window is still honored (the happy path must keep working).
func TestClaimTimeAcceptsValidClaimTime(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	runOpen := base
	claim := base.Add(10 * time.Minute)
	doc := &entity.Document{ProcessBeginAt: ptrTime(claim)}
	run := &entity.PipelineOperationLog{CreateDate: ptrTime(runOpen)}

	got := claimTime(doc, run)
	if got == nil || !got.Equal(claim) {
		t.Fatalf("claimTime = %v, want %v", got, claim)
	}
}

// TestBuildIngestLogPreClaimStoppedOmitsStaleClaimTime is the direct regression
// for P1a: a task stopped (by user / superseded) before any worker claimed it
// must not emit a BeginTime inherited from a prior run, and must not report a
// fabricated parse_duration_s of hours/days.
func TestBuildIngestLogPreClaimStoppedOmitsStaleClaimTime(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	staleClaim := base.Add(-48 * time.Hour)
	runOpen := base.Add(-1 * time.Hour)
	task := &entity.IngestionTask{ID: "t", DocumentID: "d", DatasetID: "kb", UserID: "u"}
	doc := &entity.Document{Name: ptrStr("r"), Suffix: ".pdf", Type: "pdf", Size: 10, ParserID: "pdf", ProcessBeginAt: ptrTime(staleClaim)}
	run := &entity.PipelineOperationLog{RunCount: ptrInt(1), CreateDate: ptrTime(runOpen)}

	ev := BuildIngestLog(task, doc, run, "stopped", "STOPPED", "stopped by user", "", "")
	if !ev.BeginTime.IsZero() {
		t.Errorf("pre-claim stopped must omit stale BeginTime, got %v", ev.BeginTime)
	}
	if ev.ParseDurationS != 0 {
		t.Errorf("pre-claim stopped parse_duration_s must be 0, got %v", ev.ParseDurationS)
	}
	if ev.WaitDurationS != 0 {
		t.Errorf("pre-claim stopped wait_duration_s must be 0, got %v", ev.WaitDurationS)
	}
}

// TestBuildIngestLogPreClaimFailedOmitsStaleClaimTime covers the settlePublishFailure
// path: a run that fails before a worker claims it must not report a stale claim time.
func TestBuildIngestLogPreClaimFailedOmitsStaleClaimTime(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	staleClaim := base.Add(-48 * time.Hour)
	runOpen := base.Add(-1 * time.Hour)
	task := &entity.IngestionTask{ID: "t", DocumentID: "d", DatasetID: "kb", UserID: "u"}
	doc := &entity.Document{Name: ptrStr("r"), Suffix: ".pdf", Type: "pdf", Size: 10, ParserID: "pdf", ProcessBeginAt: ptrTime(staleClaim)}
	run := &entity.PipelineOperationLog{RunCount: ptrInt(1), CreateDate: ptrTime(runOpen)}

	ev := BuildIngestLog(task, doc, run, "failed", "FAILED", "publish failed", "", "")
	if !ev.BeginTime.IsZero() {
		t.Errorf("pre-claim failed must omit stale BeginTime, got %v", ev.BeginTime)
	}
	if ev.ParseDurationS != 0 {
		t.Errorf("pre-claim failed parse_duration_s must be 0, got %v", ev.ParseDurationS)
	}
}

// TestBuildIngestLogRunningKeepsValidClaimTime guards the happy path: a task that
// actually started keeps its real claim time and a sane parse/wait duration.
func TestBuildIngestLogRunningKeepsValidClaimTime(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	runOpen := base
	claim := base.Add(10 * time.Minute)
	task := &entity.IngestionTask{ID: "t", DocumentID: "d", DatasetID: "kb", UserID: "u"}
	doc := &entity.Document{Name: ptrStr("r"), Suffix: ".pdf", Type: "pdf", Size: 10, ParserID: "pdf", ProcessBeginAt: ptrTime(claim)}
	run := &entity.PipelineOperationLog{RunCount: ptrInt(1), CreateDate: ptrTime(runOpen), ProcessBeginAt: ptrTime(runOpen), ProcessDuration: 1200}

	ev := BuildIngestLog(task, doc, run, "running", "RUNNING", "", "", "")
	if !ev.BeginTime.Equal(claim) {
		t.Errorf("running BeginTime = %v, want %v", ev.BeginTime, claim)
	}
	if ev.ParseDurationS <= 0 {
		t.Errorf("running parse_duration_s = %v, want > 0", ev.ParseDurationS)
	}
}
