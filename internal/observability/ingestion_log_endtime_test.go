package observability

import (
	"testing"
	"time"

	"ragflow/internal/entity"
)

// TestBuildIngestLogDerivesEndTimeFromBeginAndDuration covers the requirement
// that process_end_at need not be persisted: the authoritative end time is
// recovered from ProcessBeginAt + ProcessDuration.
func TestBuildIngestLogDerivesEndTimeFromBeginAndDuration(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	begin := base.Add(10 * time.Second)
	duration := 30.0
	run := &entity.PipelineOperationLog{
		RunCount:        ptrInt(1),
		CreateDate:      ptrTime(base),
		ProcessBeginAt:  ptrTime(begin),
		ProcessDuration: duration,
	}
	task := &entity.IngestionTask{ID: "task-1"}
	doc := &entity.Document{Name: ptrStr("d"), CreateDate: ptrTime(base)}

	ev := BuildIngestLog(task, doc, run, "completed", "COMPLETED", "", "", "")
	want := begin.Add(time.Duration(duration * float64(time.Second)))
	if !ev.EndTime.Equal(want) {
		t.Fatalf("EndTime = %v, want %v (ProcessBeginAt + ProcessDuration)", ev.EndTime, want)
	}
}

// TestBuildIngestLogZeroDurationFallsBackToEventTime guards against a naive
// "always derive end from begin" bug: when ProcessDuration is 0 (run not yet processed),
// EndTime must NOT collapse to ProcessBeginAt, and instead fall back to the transition
// EventTime for a terminal event.
func TestBuildIngestLogZeroDurationFallsBackToEventTime(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	begin := base.Add(10 * time.Second)
	run := &entity.PipelineOperationLog{
		RunCount:        ptrInt(1),
		CreateDate:      ptrTime(base),
		ProcessBeginAt:  ptrTime(begin),
		ProcessDuration: 0, // not yet processed
	}
	task := &entity.IngestionTask{ID: "task-1"}
	doc := &entity.Document{Name: ptrStr("d"), CreateDate: ptrTime(base), ProcessBeginAt: ptrTime(begin)}

	ev := BuildIngestLog(task, doc, run, "completed", "COMPLETED", "", "", "")
	if !ev.EndTime.Equal(ev.EventTime) {
		t.Fatalf("EndTime = %v, want EventTime %v (duration 0 fallback)", ev.EndTime, ev.EventTime)
	}
}
