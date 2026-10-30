package observability

import (
	"testing"
	"time"

	"ragflow/internal/entity"
)

func TestBuildIngestLogPrefersWorkerClaimTime(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	enqueue := base
	openTime := base.Add(1 * time.Minute)
	claimTime := base.Add(10 * time.Minute)
	end := base.Add(20 * time.Minute)
	task := &entity.IngestionTask{ID: "t", DocumentID: "d", DatasetID: "kb", UserID: "u"}
	doc := &entity.Document{Name: ptrStr("r"), Suffix: ".pdf", Type: "pdf", Size: 10, ParserID: "pdf", ProcessBeginAt: ptrTime(claimTime)}
	run := &entity.PipelineOperationLog{RunCount: ptrInt(1), CreateDate: ptrTime(enqueue), ProcessBeginAt: ptrTime(openTime), ProcessEndAt: ptrTime(end)}

	log := BuildIngestLog(task, doc, run, "completed", "COMPLETED", "")
	if !log.BeginTime.Equal(claimTime) {
		t.Errorf("BeginTime = %v, want worker-claim time %v", log.BeginTime, claimTime)
	}
	if log.WaitDurationS <= 120 {
		t.Errorf("WaitDurationS = %v, want ~600 (enqueue -> worker claim), got near-zero", log.WaitDurationS)
	}
}

func TestBuildIngestLogPopulatesDurationMS(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	task := &entity.IngestionTask{ID: "t", DocumentID: "d", DatasetID: "kb", UserID: "u"}
	doc := &entity.Document{Name: ptrStr("r"), Suffix: ".pdf", Type: "pdf", Size: 10, ParserID: "pdf", ProcessBeginAt: ptrTime(base.Add(10 * time.Minute))}
	run := &entity.PipelineOperationLog{RunCount: ptrInt(1), CreateDate: ptrTime(base), ProcessBeginAt: ptrTime(base.Add(1 * time.Minute)), ProcessEndAt: ptrTime(base.Add(20 * time.Minute))}

	log := BuildIngestLog(task, doc, run, "completed", "COMPLETED", "")
	want := int64(log.ParseDurationS * 1000)
	if want <= 0 {
		t.Fatalf("ParseDurationS = %v, want > 0 for setup", log.ParseDurationS)
	}
	if log.DurationMS != want {
		t.Errorf("DurationMS = %d, want %d", log.DurationMS, want)
	}

	created := BuildIngestLog(task, doc, &entity.PipelineOperationLog{RunCount: ptrInt(1)}, "created", "CREATED", "")
	if created.DurationMS != 0 {
		t.Errorf("created DurationMS = %d, want 0", created.DurationMS)
	}
}
