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
// this event. It keys on the task-level status (uppercase), not event_type —
// event_type now carries the event scope (task|component|progress).
// created/scheduled events happen before any worker has claimed the document,
// so they must not carry a (possibly stale, from a prior run) claim time.
func isBeginRelevant(status string) bool {
	switch status {
	case "RUNNING", "COMPLETED", "FAILED", "STOPPED":
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

// BuildIngestLog assembles a common.IngestionLog from the task, its document,
// and the bound pipeline run. scope is the event scope (task|component|progress);
// the lifecycle state lives in status. Component/progress events are built by
// the dedicated granularity builders instead.
//
// Ingest identity (ingestor name, NATS subject, worker name, user/tenant
// display names) is read from ctx via the WithXxx/IngestXxx accessors in
// ingest_ctx.go; it is execution/routing metadata not derivable from the task
// or document and varies per execution. A missing key yields an empty field
// that serializes as omitted/zero.
func BuildIngestLog(ctx context.Context, task *entity.IngestionTask, doc *entity.Document, run *entity.PipelineOperationLog, scope, status, errorInfo string) common.IngestionLog {
	ev := common.IngestionLog{
		EventTime:    time.Now().UTC(),
		UserName:     UserName(ctx),
		TenantName:   TenantName(ctx),
		IngestorName: IngestorName(ctx),
		NatsQueue:    NatsQueue(ctx),
		Worker:       WorkerName(ctx),
		EventType:    scope,
		Status:       status,
		ErrorInfo:    errorInfo,
	}
	// task may be absent (e.g. a best-effort granularity event where the task
	// row could not be loaded); keep the builder nil-safe so instrumentation
	// never panics the parse path. Identity fields simply stay empty.
	if task != nil {
		ev.TaskID = task.ID
		ev.DocID = task.DocumentID
		ev.DatasetID = task.DatasetID
		ev.UserID = task.UserID
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
		// Run identity is part of every scope (task/component/progress): task ids
		// are reused across retries, so the run id is what disambiguates a run.
		ev.PipelineLogID = run.ID
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
	}
	// The run row can be unavailable at emit time (lookup failure or not yet
	// committed). Fall back to the task's run id so every scope still carries run
	// identity; task ids are reused across retries and only the run id tells them
	// apart. A loaded run always wins over this fallback.
	if ev.PipelineLogID == "" && task != nil && task.PipelineLogID != nil {
		ev.PipelineLogID = *task.PipelineLogID
	}
	// Claim and terminal timing belong to task-scope lifecycle events only.
	// Component/progress snapshots are emitted mid-run; inheriting a terminal
	// EndTime would fabricate a full-run parse duration for a single snapshot.
	if scope == "task" && run != nil {
		// BeginTime only for running/terminal events; created/scheduled must not
		// leak a (possibly stale) claim time from a previous run.
		if ct := claimTime(doc, run); ct != nil && isBeginRelevant(status) {
			ev.BeginTime = ct.UTC()
		}
		// When the end time still could not be derived (the run had no
		// ProcessDuration, e.g. a stop on a still-queued task), fall back to the
		// transition time for a terminal event so a parse duration is still reported
		// if a begin time is present.
		if ev.EndTime.IsZero() && !ev.BeginTime.IsZero() &&
			(status == common.COMPLETED || status == common.FAILED || status == common.STOPPED) {
			ev.EndTime = ev.EventTime
		}
	}
	ev.WaitDurationS, ev.ParseDurationS = ComputeDurations(ev.EnqueueTime, ev.BeginTime, ev.EndTime)
	ev.DurationMS = int64(ev.ParseDurationS * 1000)
	return ev
}
