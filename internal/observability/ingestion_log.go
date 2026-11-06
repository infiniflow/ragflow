// Package observability builds structured lifecycle events for ingestion
// (document-parse) tasks and forwards them to the common ingest-log sink
// (internal/common.LogIngestEvent). The default emitter routes straight to that
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
	EmitIngestLog(ctx context.Context, log common.IngestionLog)
}

// CommonLogEmitter is the default OSS emitter. It forwards the event to
// common.LogIngestEvent, which writes one JSON line to the configured sink.
type CommonLogEmitter struct{}

// EmitIngestLog forwards the event to the common ingest-log sink.
func (e *CommonLogEmitter) EmitIngestLog(_ context.Context, log common.IngestionLog) {
	common.LogIngest(&log)
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
	// A zero begin means the worker-claim time is unknown (e.g. the document
	// lookup failed and claimTime returned nil). There is no parse window, so the
	// duration must clamp to 0 rather than blow up to end - year-0001.
	if !begin.IsZero() && end.After(begin) {
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
// ProcessBeginAt (set when the worker actually starts, in PrepareValidatedRun).
//
// doc.ProcessBeginAt is never cleared between runs, so on a run that never claimed
// the document (RequestStop/supersede on a queued task, or a publish failure) or a
// retry that reuses a stale document value, it can hold a PRIOR run's instant.
// We therefore only trust it when it falls at or after this run's open time
// (run.CreateDate); otherwise the run never started and must not report a
// claim/parse time. The run row's ProcessBeginAt is the open time (set in
// CreateOpenLog), not a claim time, so it is intentionally not used here.
func claimTime(doc *entity.Document, run *entity.PipelineOperationLog) *time.Time {
	if doc != nil && doc.ProcessBeginAt != nil && run != nil && run.CreateDate != nil {
		if !doc.ProcessBeginAt.Before(*run.CreateDate) {
			return doc.ProcessBeginAt
		}
		return nil
	}
	// No run to validate against: fall back to the document value as-is.
	if doc != nil && doc.ProcessBeginAt != nil {
		return doc.ProcessBeginAt
	}
	return nil
}

// BuildIngestLog assembles a common.IngestionLog from the task, its document, and
// the bound pipeline run. eventType is one of
// created|scheduled|running|completed|failed|stopped.
func BuildIngestLog(task *entity.IngestionTask, doc *entity.Document, run *entity.PipelineOperationLog, eventType, status, errorInfo, userName, tenantName string) common.IngestionLog {
	ev := common.IngestionLog{
		EventTime:  time.Now().UTC(),
		TaskID:     task.ID,
		DocID:      task.DocumentID,
		DatasetID:  task.DatasetID,
		UserID:     task.UserID,
		UserName:   userName,
		TenantName: tenantName,
		EventType:  eventType,
		Status:     status,
		ErrorInfo:  errorInfo,
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
		if run.ProcessBeginAt != nil && run.ProcessDuration > 0 {
			// The process_end_at column was removed; recover the authoritative end
			// time from the persisted begin time and duration so parse duration is
			// still accurate without a dedicated column.
			ev.EndTime = run.ProcessBeginAt.Add(time.Duration(run.ProcessDuration * float64(time.Second))).UTC()
		}
		// BeginTime only for running/terminal events; created/scheduled must not
		// leak a (possibly stale) claim time from a previous run.
		if ct := claimTime(doc, run); ct != nil && isBeginRelevant(eventType) {
			ev.BeginTime = ct.UTC()
		}
		// When the end time still could not be derived (the run had no
		// ProcessDuration, e.g. a stop on a still-queued task), fall back to the
		// transition time for a terminal event so a parse duration is still reported
		// if a begin time is present.
		if ev.EndTime.IsZero() && !ev.BeginTime.IsZero() &&
			(eventType == "completed" || eventType == "failed" || eventType == "stopped") {
			ev.EndTime = ev.EventTime
		}
	}
	ev.WaitDurationS, ev.ParseDurationS = ComputeDurations(ev.EnqueueTime, ev.BeginTime, ev.EndTime)
	ev.DurationMS = int64(ev.ParseDurationS * 1000)
	return ev
}
