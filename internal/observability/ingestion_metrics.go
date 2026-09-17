// Package observability contains process-level metrics that are safe to
// expose to a Prometheus scraper. Labels are intentionally closed sets: task,
// document, and run identifiers belong in structured logs, never metric
// labels.
package observability

import (
	"fmt"
	"strings"
	"sync/atomic"
)

const (
	metricResultSuccess = "success"
	metricResultFailure = "failure"
	metricReasonUnknown = "unknown"
)

var (
	foldSkipReasons = []string{
		"run_not_terminal",
		"protected_rows_reach_limit",
		metricReasonUnknown,
	}
	eventRejectReasons = []string{
		"missing_pipeline_log_id",
		"missing_task_id",
		"invalid_kind",
		"invalid_phase",
		"missing_component",
		metricReasonUnknown,
	}
)

type ingestionLogMetrics struct {
	eventsDeleted atomic.Uint64
	runsDeleted   atomic.Uint64
	foldsSuccess  atomic.Uint64
	foldsFailure  atomic.Uint64
	foldSkipped   [3]atomic.Uint64
	eventRejected [6]atomic.Uint64
}

var ingestionMetrics ingestionLogMetrics

// RecordIngestionLogFoldSuccess records one successful fold and its deleted
// event rows. The deleted count is allowed to be zero for an idempotent fold.
func RecordIngestionLogFoldSuccess(deletedEvents int) {
	ingestionMetrics.foldsSuccess.Add(1)
	if deletedEvents > 0 {
		ingestionMetrics.eventsDeleted.Add(uint64(deletedEvents))
	}
}

// RecordIngestionLogFoldFailure records a fold that could not be completed.
func RecordIngestionLogFoldFailure() {
	ingestionMetrics.foldsFailure.Add(1)
}

// RecordIngestionLogFoldSkipped records a fold skipped for a bounded reason.
func RecordIngestionLogFoldSkipped(reason string) {
	ingestionMetrics.foldSkipped[closedReasonIndex(foldSkipReasons, reason)].Add(1)
}

// RecordIngestionLogRunDelete records complete run deletion and its event-row
// count. Run deletion is distinct from single-run folding in the dashboard.
func RecordIngestionLogRunDelete(deletedRuns, deletedEvents int) {
	if deletedRuns > 0 {
		ingestionMetrics.runsDeleted.Add(uint64(deletedRuns))
	}
	if deletedEvents > 0 {
		ingestionMetrics.eventsDeleted.Add(uint64(deletedEvents))
	}
}

// RecordIngestionLogEventRejected records an event rejected by the writer's
// identity/type invariant checks. Unknown reasons are intentionally grouped.
func RecordIngestionLogEventRejected(reason string) {
	ingestionMetrics.eventRejected[closedReasonIndex(eventRejectReasons, reason)].Add(1)
}

// IngestionLogMetricsPrometheusText returns the metrics in the Prometheus
// text exposition format. It is a snapshot; counters may advance while the
// response is being assembled, which is acceptable for cumulative metrics.
func IngestionLogMetricsPrometheusText() string {
	var b strings.Builder
	writeCounter(&b, "ingestion_log_events_deleted_total", "Deleted ingestion event rows.", ingestionMetrics.eventsDeleted.Load())
	writeCounter(&b, "ingestion_log_runs_deleted_total", "Deleted completed ingestion runs.", ingestionMetrics.runsDeleted.Load())

	b.WriteString("# HELP ingestion_log_folds_total Completed ingestion log folds by result.\n")
	b.WriteString("# TYPE ingestion_log_folds_total counter\n")
	fmt.Fprintf(&b, "ingestion_log_folds_total{result=\"success\"} %d\n", ingestionMetrics.foldsSuccess.Load())
	fmt.Fprintf(&b, "ingestion_log_folds_total{result=\"failure\"} %d\n", ingestionMetrics.foldsFailure.Load())

	b.WriteString("# HELP ingestion_log_fold_skipped_total Skipped ingestion log folds by bounded reason.\n")
	b.WriteString("# TYPE ingestion_log_fold_skipped_total counter\n")
	for i, reason := range foldSkipReasons {
		fmt.Fprintf(&b, "ingestion_log_fold_skipped_total{reason=\"%s\"} %d\n", reason, ingestionMetrics.foldSkipped[i].Load())
	}

	b.WriteString("# HELP ingestion_log_event_rejected_total Rejected ingestion events by bounded reason.\n")
	b.WriteString("# TYPE ingestion_log_event_rejected_total counter\n")
	for i, reason := range eventRejectReasons {
		fmt.Fprintf(&b, "ingestion_log_event_rejected_total{reason=\"%s\"} %d\n", reason, ingestionMetrics.eventRejected[i].Load())
	}
	return b.String()
}

func writeCounter(b *strings.Builder, name, help string, value uint64) {
	fmt.Fprintf(b, "# HELP %s %s\n# TYPE %s counter\n%s %d\n", name, help, name, name, value)
}

func closedReasonIndex(reasons []string, reason string) int {
	for i, candidate := range reasons {
		if candidate == reason {
			return i
		}
	}
	return len(reasons) - 1
}

// ResetIngestionLogMetricsForTest clears process counters. Production code
// must never reset cumulative metrics; this hook is intentionally named for
// tests so it is not mistaken for a runtime operation.
func ResetIngestionLogMetricsForTest() {
	ingestionMetrics = ingestionLogMetrics{}
}
