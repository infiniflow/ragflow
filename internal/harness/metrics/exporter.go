package metrics

import (
	"fmt"
	"strings"
)

// Exporter formats agent metrics for output. This is a lightweight
// alternative to a full Prometheus client, avoiding the dependency.
type Exporter struct {
	namespace string
}

// NewExporter creates a new metrics exporter.
func NewExporter(namespace string) *Exporter {
	return &Exporter{namespace: namespace}
}

// ExportText formats metrics as Prometheus-style text output.
func (e *Exporter) ExportText(m *AgentMetrics) string {
	snap := m.Snapshot()
	var b strings.Builder

	ns := e.namespace
	if ns != "" {
		ns += "_"
	}

	// Tool metrics.
	fmt.Fprintf(&b, "# HELP %stool_calls_total Total tool invocations\n", ns)
	fmt.Fprintf(&b, "# TYPE %stool_calls_total counter\n", ns)
	fmt.Fprintf(&b, "%stool_calls_total %d\n", ns, snap.ToolCalls)

	fmt.Fprintf(&b, "# HELP %stool_success_rate Tool success rate\n", ns)
	fmt.Fprintf(&b, "# TYPE %stool_success_rate gauge\n", ns)
	fmt.Fprintf(&b, "%stool_success_rate %.4f\n", ns, snap.ToolSuccessRate)

	fmt.Fprintf(&b, "# HELP %stool_retry_rate Tool retry rate\n", ns)
	fmt.Fprintf(&b, "# TYPE %stool_retry_rate gauge\n", ns)
	fmt.Fprintf(&b, "%stool_retry_rate %.4f\n", ns, snap.ToolRetryRate)

	// Checkpoint metrics.
	fmt.Fprintf(&b, "# HELP %scheckpoint_saves_total Total checkpoint saves\n", ns)
	fmt.Fprintf(&b, "# TYPE %scheckpoint_saves_total counter\n", ns)
	fmt.Fprintf(&b, "%scheckpoint_saves_total %d\n", ns, snap.CheckpointSaves)

	fmt.Fprintf(&b, "# HELP %scheckpoint_restore_success Checkpoint restore success rate\n", ns)
	fmt.Fprintf(&b, "# TYPE %scheckpoint_restore_success gauge\n", ns)
	fmt.Fprintf(&b, "%scheckpoint_restore_success %.4f\n", ns, snap.CheckpointRestoreSuccess)

	// Execution metrics.
	fmt.Fprintf(&b, "# HELP %ssteps_total Total supersteps executed\n", ns)
	fmt.Fprintf(&b, "# TYPE %ssteps_total counter\n", ns)
	fmt.Fprintf(&b, "%ssteps_total %d\n", ns, snap.Steps)

	fmt.Fprintf(&b, "# HELP %snodes_executed_total Total nodes executed\n", ns)
	fmt.Fprintf(&b, "# TYPE %snodes_executed_total counter\n", ns)
	fmt.Fprintf(&b, "%snodes_executed_total %d\n", ns, snap.NodesExecuted)

	fmt.Fprintf(&b, "# HELP %sinterrupts_total Total interrupts\n", ns)
	fmt.Fprintf(&b, "# TYPE %sinterrupts_total counter\n", ns)
	fmt.Fprintf(&b, "%sinterrupts_total %d\n", ns, snap.InterruptCount)

	return b.String()
}

// ExportCSV formats metrics as a single CSV row.
func (e *Exporter) ExportCSV(m *AgentMetrics) string {
	snap := m.Snapshot()
	return fmt.Sprintf("%s,%d,%d,%d,%.4f,%.4f,%d,%d,%.4f,%d,%d,%d,%.6f",
		snap.TraceID,
		snap.ToolCalls, snap.ToolSuccesses, snap.ToolFailures,
		snap.ToolSuccessRate, snap.ToolRetryRate,
		snap.CheckpointSaves, snap.CheckpointRestores,
		snap.CheckpointRestoreSuccess,
		snap.Steps, snap.NodesExecuted, snap.InterruptCount,
		snap.CostPerTask)
}
