package metrics

import (
	"math"
	"sort"
	"sync"
	"time"
)

// MetricsAggregator aggregates metrics across multiple execution traces.
type MetricsAggregator struct {
	mu      sync.Mutex
	metrics []*AgentMetrics
}

// NewMetricsAggregator creates a new MetricsAggregator.
func NewMetricsAggregator() *MetricsAggregator {
	return &MetricsAggregator{}
}

// Add adds a metrics snapshot to the aggregator.
func (a *MetricsAggregator) Add(m *AgentMetrics) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.metrics = append(a.metrics, m)
}

// AggregatedMetrics contains summary statistics across multiple traces.
type AggregatedMetrics struct {
	TotalTraces   int
	TotalDuration time.Duration

	// Tool metrics (averages).
	AvgToolCalls       float64
	AvgToolSuccessRate float64
	AvgToolRetryRate   float64
	P50ToolLatencyMs   float64
	P95ToolLatencyMs   float64
	P99ToolLatencyMs   float64

	// Checkpoint metrics.
	AvgCheckpointSaves          float64
	AvgCheckpointRestores       float64
	AvgCheckpointRestoreSuccess float64

	// Execution metrics.
	AvgSteps           float64
	AvgNodesExecuted   float64
	AvgRecoveredErrors float64
	AvgInterrupts      float64

	// Cost metrics.
	AvgCostPerTask        float64
	AvgForkReplayPassRate float64
}

// Aggregate computes summary statistics across all collected metrics.
func (a *MetricsAggregator) Aggregate() *AggregatedMetrics {
	a.mu.Lock()
	defer a.mu.Unlock()

	result := &AggregatedMetrics{
		TotalTraces: len(a.metrics),
	}

	if len(a.metrics) == 0 {
		return result
	}

	var allLatencies []float64

	for _, m := range a.metrics {
		result.TotalDuration += m.Duration
		result.AvgToolCalls += float64(m.ToolCalls)
		result.AvgToolSuccessRate += m.ToolSuccessRate
		result.AvgToolRetryRate += m.ToolRetryRate
		result.AvgCheckpointSaves += float64(m.CheckpointSaves)
		result.AvgCheckpointRestores += float64(m.CheckpointRestores)
		result.AvgCheckpointRestoreSuccess += m.CheckpointRestoreSuccess
		result.AvgSteps += float64(m.Steps)
		result.AvgNodesExecuted += float64(m.NodesExecuted)
		result.AvgRecoveredErrors += float64(m.RecoveredErrors)
		result.AvgInterrupts += float64(m.InterruptCount)
		result.AvgCostPerTask += m.CostPerTask
		result.AvgForkReplayPassRate += m.ForkReplayPassRate

		for _, latencies := range m.ToolLatencyMs {
			for _, l := range latencies {
				allLatencies = append(allLatencies, float64(l))
			}
		}
	}

	n := float64(len(a.metrics))
	result.AvgToolCalls /= n
	result.AvgToolSuccessRate /= n
	result.AvgToolRetryRate /= n
	result.AvgCheckpointSaves /= n
	result.AvgCheckpointRestores /= n
	result.AvgCheckpointRestoreSuccess /= n
	result.AvgSteps /= n
	result.AvgNodesExecuted /= n
	result.AvgRecoveredErrors /= n
	result.AvgInterrupts /= n
	result.AvgCostPerTask /= n
	result.AvgForkReplayPassRate /= n

	// Compute latency percentiles.
	if len(allLatencies) > 0 {
		sort.Float64s(allLatencies)
		result.P50ToolLatencyMs = percentile(allLatencies, 50)
		result.P95ToolLatencyMs = percentile(allLatencies, 95)
		result.P99ToolLatencyMs = percentile(allLatencies, 99)
	}

	return result
}

// Reset clears all collected metrics.
func (a *MetricsAggregator) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.metrics = nil
}

// MetricsWindow tracks metrics over a sliding time window.
type MetricsWindow struct {
	mu      sync.Mutex
	window  time.Duration
	entries []windowEntry
}

type windowEntry struct {
	timestamp time.Time
	metrics   *AgentMetrics
}

// NewMetricsWindow creates a metrics window with the given duration.
func NewMetricsWindow(window time.Duration) *MetricsWindow {
	return &MetricsWindow{window: window}
}

// Add adds metrics at the current time.
func (w *MetricsWindow) Add(m *AgentMetrics) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.entries = append(w.entries, windowEntry{
		timestamp: time.Now(),
		metrics:   m,
	})
	w.prune()
}

// Aggregate returns aggregated metrics for the current window.
func (w *MetricsWindow) Aggregate() *AggregatedMetrics {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.prune()

	agg := NewMetricsAggregator()
	for _, entry := range w.entries {
		agg.Add(entry.metrics)
	}
	return agg.Aggregate()
}

// prune removes entries outside the window.
func (w *MetricsWindow) prune() {
	cutoff := time.Now().Add(-w.window)
	keep := make([]windowEntry, 0, len(w.entries))
	for _, e := range w.entries {
		if e.timestamp.After(cutoff) {
			keep = append(keep, e)
		}
	}
	w.entries = keep
}

// percentile computes the p-th percentile from a sorted slice.
func percentile(sorted []float64, p int) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(float64(p)/100.0*float64(len(sorted))) - 1)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
