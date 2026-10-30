// Package observability builds structured lifecycle events for ingestion
// (document-parse) tasks and forwards them to the common ingest-log sink
// (internal/common.LogIngest). The default emitter routes straight to that
// sink; tests inject a recording emitter.
package observability

import (
	"context"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

// IngestionEventEmitter receives lifecycle events. Implementations must be
// non-blocking and must never return an error that could fail the parse; the
// caller ignores errors by design.
type IngestionEventEmitter interface {
	EmitIngestLog(ctx context.Context, log common.IngestLog)
}

// CommonLogEmitter is the default OSS emitter. It forwards the event to
// common.LogIngest, which writes one JSON line to the configured sink.
type CommonLogEmitter struct{}

// EmitIngestLog forwards the event to the common ingest-log sink.
func (e *CommonLogEmitter) EmitIngestLog(_ context.Context, log common.IngestLog) {
	common.LogIngest(log)
}

// ComputeDurations derives wait (begin-enqueue) and parse (end-begin) seconds.
// Durations are clamped to >= 0 so clock skew never produces negative values
// downstream.
func ComputeDurations(enqueue, begin, end time.Time) (float64, float64) {
	wait := 0.0
	if begin.After(enqueue) {
		wait = begin.Sub(enqueue).Seconds()
	}
	parse := 0.0
	if end.After(begin) {
		parse = end.Sub(begin).Seconds()
	}
	return wait, parse
}

// isBeginRelevant reports whether the worker-claim time should be attached to
// this event. created/scheduled events happen before any worker has claimed the
// document, so they must not carry a (possibly stale, from a prior run) claim
// time.
func isBeginRelevant(eventType string) bool {
	switch eventType {
	case "running", "completed", "failed", "stopped":
		return true
	default:
		return false
	}
}

// claimTime returns the worker-claim time. It prefers the document's own
// ProcessBeginAt (set when the worker actually starts, in PrepareValidatedRun)
// over the run row's ProcessBeginAt (set in CreateOpenLog at open time, before
// the worker claims). This keeps wait_duration_s measuring real queue wait
// rather than ~0. A retry's new run overwrites doc.ProcessBeginAt at its own
// claim, so the current run never inherits a previous run's claim time here.
func claimTime(doc *entity.Document, run *entity.PipelineOperationLog) *time.Time {
	if doc != nil && doc.ProcessBeginAt != nil {
		return doc.ProcessBeginAt
	}
	if run != nil && run.ProcessBeginAt != nil {
		return run.ProcessBeginAt
	}
	return nil
}

// BuildIngestLog assembles a common.IngestLog from the task, its document, and
// the bound pipeline run. eventType is one of
// created|scheduled|running|completed|failed|stopped.
func BuildIngestLog(task *entity.IngestionTask, doc *entity.Document, run *entity.PipelineOperationLog, eventType, status, errorInfo string) common.IngestLog {
	ev := common.IngestLog{
		EventTime: time.Now().UTC(),
		TaskID:    task.ID,
		DocID:     task.DocumentID,
		DatasetID: task.DatasetID,
		UserID:    task.UserID,
		EventType: eventType,
		Status:    status,
		ErrorInfo: errorInfo,
	}
	if doc != nil {
		name := ""
		if doc.Name != nil {
			name = *doc.Name
		}
		ev.Filename = name + doc.Suffix
		ev.FileType = doc.Type
		ev.FileSize = doc.Size
		ev.ParserID = doc.ParserID
		if doc.CreateDate != nil {
			ev.UploadTime = doc.CreateDate.UTC()
		}
	}
	if run != nil {
		if run.RunCount != nil {
			ev.RunCount = *run.RunCount
		}
		if run.TenantID != "" {
			ev.TenantID = run.TenantID
		}
		ev.Progress = run.Progress
		if run.CreateDate != nil {
			ev.EnqueueTime = run.CreateDate.UTC()
		}
		if run.ProcessEndAt != nil {
			ev.EndTime = run.ProcessEndAt.UTC()
		}
		// BeginTime only for running/terminal events; created/scheduled must not
		// leak a (possibly stale) claim time from a previous run.
		if ct := claimTime(doc, run); ct != nil && isBeginRelevant(eventType) {
			ev.BeginTime = ct.UTC()
		}
		// ProcessEndAt is not always persisted on the run; for a terminal event
		// whose run actually started, fall back to the transition time so parse
		// duration is still reported (a still-queued stop gets no duration).
		if ev.EndTime.IsZero() && !ev.BeginTime.IsZero() &&
			(eventType == "completed" || eventType == "failed" || eventType == "stopped") {
			ev.EndTime = ev.EventTime
		}
	}
	ev.WaitDurationS, ev.ParseDurationS = ComputeDurations(ev.EnqueueTime, ev.BeginTime, ev.EndTime)
	ev.DurationMS = int64(ev.ParseDurationS * 1000)
	return ev
}
