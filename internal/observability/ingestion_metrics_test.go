package observability

import (
	"strings"
	"testing"
)

func TestIngestionLogMetricsPrometheusTextUsesClosedLabels(t *testing.T) {
	ResetIngestionLogMetricsForTest()
	t.Cleanup(ResetIngestionLogMetricsForTest)

	RecordIngestionLogFoldSuccess(3)
	RecordIngestionLogFoldFailure()
	RecordIngestionLogFoldSkipped("run_not_terminal")
	RecordIngestionLogFoldSkipped("user-controlled-label")
	RecordIngestionLogRunDelete(2, 7)
	RecordIngestionLogEventRejected("missing_pipeline_log_id")
	RecordIngestionLogEventRejected("task-id-attacker")

	output := IngestionLogMetricsPrometheusText()
	for _, want := range []string{
		`ingestion_log_events_deleted_total 10`,
		`ingestion_log_runs_deleted_total 2`,
		`ingestion_log_folds_total{result="success"} 1`,
		`ingestion_log_folds_total{result="failure"} 1`,
		`ingestion_log_fold_skipped_total{reason="run_not_terminal"} 1`,
		`ingestion_log_fold_skipped_total{reason="unknown"} 1`,
		`ingestion_log_event_rejected_total{reason="missing_pipeline_log_id"} 1`,
		`ingestion_log_event_rejected_total{reason="unknown"} 1`,
	} {
		if !strings.Contains(output, want) {
			t.Errorf("metrics output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "task-id-attacker") || strings.Contains(output, "user-controlled-label") {
		t.Fatalf("metrics output contains unbounded label value:\n%s", output)
	}
}
