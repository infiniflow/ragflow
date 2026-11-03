package observability

import (
	"testing"
	"time"

	"ragflow/internal/entity"
)

func TestComputeDurationsHappy(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	enqueue := base
	begin := base.Add(5 * time.Minute)
	end := base.Add(15 * time.Minute)
	wait, parse := ComputeDurations(enqueue, begin, end)
	if wait != 300 {
		t.Errorf("wait_duration_s = %v, want 300", wait)
	}
	if parse != 600 {
		t.Errorf("parse_duration_s = %v, want 600", parse)
	}
}

func TestComputeDurationsClampsSkew(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	enqueue := base.Add(10 * time.Minute)
	begin := base
	end := base.Add(20 * time.Minute)
	wait, parse := ComputeDurations(enqueue, begin, end)
	if wait < 0 {
		t.Errorf("wait_duration_s = %v, want >= 0 (clock skew must clamp)", wait)
	}
	if parse != 1200 {
		t.Errorf("parse_duration_s = %v, want 1200", parse)
	}
}

func TestComputeDurationsClampsEndBeforeBegin(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	enqueue := base
	begin := base.Add(10 * time.Minute)
	end := base.Add(5 * time.Minute)
	wait, parse := ComputeDurations(enqueue, begin, end)
	if wait != 600 {
		t.Errorf("wait_duration_s = %v, want 600", wait)
	}
	if parse < 0 {
		t.Errorf("parse_duration_s = %v, want >= 0", parse)
	}
}

func ptrStr(s string) *string        { return &s }
func ptrTime(t time.Time) *time.Time { return &t }
func ptrInt(i int) *int              { return &i }

func TestBuildIngestLogHappy(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	task := &entity.IngestionTask{
		ID:         "task-1",
		DocumentID: "doc-1",
		DatasetID:  "kb-1",
		UserID:     "user-1",
	}
	doc := &entity.Document{
		Name:           ptrStr("report"),
		Suffix:         ".pdf",
		Type:           "pdf",
		Size:           12345,
		ParserID:       "pdf",
		CreateDate:     ptrTime(now.Add(-time.Hour)),
		ProcessBeginAt: ptrTime(now.Add(2 * time.Minute)),
	}
	run := &entity.PipelineOperationLog{
		RunCount:     ptrInt(1),
		TenantID:     "tenant-1",
		Progress:     1,
		CreateDate:   ptrTime(now),
		ProcessEndAt: ptrTime(now.Add(12 * time.Minute)),
	}
	log := BuildIngestLog(task, doc, run, "completed", "COMPLETED", "")

	if log.TaskID != "task-1" {
		t.Errorf("TaskID = %q, want task-1", log.TaskID)
	}
	if log.DocID != "doc-1" {
		t.Errorf("DocID = %q, want doc-1", log.DocID)
	}
	if log.DatasetID != "kb-1" {
		t.Errorf("DatasetID = %q, want kb-1", log.DatasetID)
	}
	if log.Filename != "report.pdf" {
		t.Errorf("Filename = %q, want report.pdf", log.Filename)
	}
	if log.FileType != "pdf" {
		t.Errorf("FileType = %q, want pdf", log.FileType)
	}
	if log.FileSize != 12345 {
		t.Errorf("FileSize = %d, want 12345", log.FileSize)
	}
	if log.ParserID != "pdf" {
		t.Errorf("ParserID = %q, want pdf", log.ParserID)
	}
	if log.EventType != "completed" {
		t.Errorf("EventType = %q, want completed", log.EventType)
	}
	if log.Status != "COMPLETED" {
		t.Errorf("Status = %q, want COMPLETED", log.Status)
	}
	if log.RunCount != 1 {
		t.Errorf("RunCount = %d, want 1", log.RunCount)
	}
	if log.TenantID != "tenant-1" {
		t.Errorf("TenantID = %q, want tenant-1", log.TenantID)
	}
	if log.WaitDurationS <= 0 {
		t.Errorf("WaitDurationS = %v, want > 0 (claim after enqueue)", log.WaitDurationS)
	}
	if log.ParseDurationS <= 0 {
		t.Errorf("ParseDurationS = %v, want > 0", log.ParseDurationS)
	}
}

func TestBuildIngestLogCreatedSkipsClaimTime(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	task := &entity.IngestionTask{ID: "task-1", DocumentID: "doc-1", DatasetID: "kb-1", UserID: "user-1"}
	doc := &entity.Document{
		Name:           ptrStr("report"),
		Suffix:         ".pdf",
		Type:           "pdf",
		Size:           10,
		ParserID:       "pdf",
		ProcessBeginAt: ptrTime(now.Add(2 * time.Minute)),
	}
	run := &entity.PipelineOperationLog{RunCount: ptrInt(1), CreateDate: ptrTime(now)}
	log := BuildIngestLog(task, doc, run, "created", "CREATED", "")
	if !log.BeginTime.IsZero() {
		t.Errorf("created event BeginTime = %v, want zero", log.BeginTime)
	}
}

func TestBuildIngestLogFailedCarriesErrorInfo(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	task := &entity.IngestionTask{ID: "task-1", DocumentID: "doc-1", DatasetID: "kb-1", UserID: "user-1"}
	doc := &entity.Document{Name: ptrStr("docname"), Suffix: ".pdf", Type: "pdf", Size: 10, ParserID: "pdf"}
	run := &entity.PipelineOperationLog{RunCount: ptrInt(1), CreateDate: ptrTime(now)}
	log := BuildIngestLog(task, doc, run, "failed", "FAILED", "boom")
	if log.ErrorInfo != "boom" {
		t.Errorf("ErrorInfo = %q, want boom", log.ErrorInfo)
	}
}
