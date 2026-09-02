package metrics

import (
	"strings"
	"testing"
)

func TestNewAgentMetrics(t *testing.T) {
	m := NewAgentMetrics()
	if m.ToolLatencyMs == nil {
		t.Fatal("ToolLatencyMs should be initialised")
	}
	if m.ApprovalLatencyMs == nil {
		t.Fatal("ApprovalLatencyMs should be initialised")
	}
}

func TestAutoCollector_ToolMetrics(t *testing.T) {
	c := NewAutoCollector()

	c.RecordToolCall("search", true, 100)
	c.RecordToolCall("search", true, 200)
	c.RecordToolCall("calc", false, 50)
	c.RecordToolRetry("search")

	snap := c.Snapshot()
	if snap.ToolCalls != 3 {
		t.Fatalf("expected 3 calls, got %d", snap.ToolCalls)
	}
	if snap.ToolSuccesses != 2 {
		t.Fatalf("expected 2 successes, got %d", snap.ToolSuccesses)
	}
	if snap.ToolFailures != 1 {
		t.Fatalf("expected 1 failure, got %d", snap.ToolFailures)
	}
	if snap.ToolRetries != 1 {
		t.Fatalf("expected 1 retry, got %d", snap.ToolRetries)
	}
	if snap.ToolSuccessRate != 2.0/3.0 {
		t.Fatalf("expected success rate %.4f, got %.4f", 2.0/3.0, snap.ToolSuccessRate)
	}

	// Check latency tracking.
	if len(snap.ToolLatencyMs["search"]) != 2 {
		t.Fatalf("expected 2 search latencies, got %d", len(snap.ToolLatencyMs["search"]))
	}
	if len(snap.ToolLatencyMs["calc"]) != 1 {
		t.Fatalf("expected 1 calc latency, got %d", len(snap.ToolLatencyMs["calc"]))
	}
}

func TestAutoCollector_CheckpointMetrics(t *testing.T) {
	c := NewAutoCollector()

	c.RecordCheckpointSave()
	c.RecordCheckpointSave()
	c.RecordCheckpointRestore(true)
	c.RecordCheckpointSave()

	snap := c.Snapshot()
	if snap.CheckpointSaves != 3 {
		t.Fatalf("expected 3 saves, got %d", snap.CheckpointSaves)
	}
	if snap.CheckpointRestores != 1 {
		t.Fatalf("expected 1 restore, got %d", snap.CheckpointRestores)
	}
}

func TestAutoCollector_ExecutionMetrics(t *testing.T) {
	c := NewAutoCollector()

	c.RecordStep()
	c.RecordStep()
	c.RecordStep()
	c.RecordNode("a")
	c.RecordNode("b")
	c.RecordNode("c")
	c.RecordRecoveredError()
	c.RecordInterrupt()

	snap := c.Snapshot()
	if snap.Steps != 3 {
		t.Fatalf("expected 3 steps, got %d", snap.Steps)
	}
	if snap.NodesExecuted != 3 {
		t.Fatalf("expected 3 nodes, got %d", snap.NodesExecuted)
	}
	if snap.RecoveredErrors != 1 {
		t.Fatalf("expected 1 error, got %d", snap.RecoveredErrors)
	}
	if snap.InterruptCount != 1 {
		t.Fatalf("expected 1 interrupt, got %d", snap.InterruptCount)
	}
}

func TestAutoCollector_LLMCost(t *testing.T) {
	c := NewAutoCollector()
	c.RecordLLMCost(0.002)
	c.RecordLLMCost(0.001)
	snap := c.Snapshot()
	if snap.CostPerTask != 0.003 {
		t.Fatalf("expected cost 0.003, got %.6f", snap.CostPerTask)
	}
}

func TestAutoCollector_MemoryHit(t *testing.T) {
	c := NewAutoCollector()
	c.RecordMemoryHit(0.9)
	c.RecordMemoryHit(0.7)
	snap := c.Snapshot()
	if snap.MemoryAvgHitScore < 0.79 || snap.MemoryAvgHitScore > 0.81 {
		t.Fatalf("expected avg ~0.80, got %.4f", snap.MemoryAvgHitScore)
	}
}

func TestAutoCollector_SnapshotCopy(t *testing.T) {
	c := NewAutoCollector()
	c.RecordToolCall("search", true, 100)

	snap1 := c.Snapshot()
	c.RecordToolCall("search", true, 200)
	snap2 := c.Snapshot()

	if snap1.ToolCalls != 1 {
		t.Fatalf("snap1 should have 1 call, got %d", snap1.ToolCalls)
	}
	if snap2.ToolCalls != 2 {
		t.Fatalf("snap2 should have 2 calls, got %d", snap2.ToolCalls)
	}
}

func TestAutoCollector_Reset(t *testing.T) {
	c := NewAutoCollector()
	c.RecordToolCall("search", true, 100)
	c.Reset()

	snap := c.Snapshot()
	if snap.ToolCalls != 0 {
		t.Fatalf("expected 0 after reset, got %d", snap.ToolCalls)
	}
}

func TestAggregator_SingleTrace(t *testing.T) {
	a := NewMetricsAggregator()
	m := NewAgentMetrics()
	m.ToolCalls = 10
	m.ToolSuccesses = 8
	m.ToolSuccessRate = 0.8
	m.Steps = 5

	a.Add(m)
	agg := a.Aggregate()

	if agg.TotalTraces != 1 {
		t.Fatalf("expected 1 trace, got %d", agg.TotalTraces)
	}
	if agg.AvgToolCalls != 10 {
		t.Fatalf("expected avg 10, got %.2f", agg.AvgToolCalls)
	}
	if agg.AvgSteps != 5 {
		t.Fatalf("expected avg 5, got %.2f", agg.AvgSteps)
	}
}

func TestAggregator_MultipleTraces(t *testing.T) {
	a := NewMetricsAggregator()

	m1 := NewAgentMetrics()
	m1.ToolCalls = 10
	m1.ToolSuccessRate = 1.0
	m1.Steps = 2

	m2 := NewAgentMetrics()
	m2.ToolCalls = 20
	m2.ToolSuccessRate = 0.5
	m2.Steps = 4

	a.Add(m1)
	a.Add(m2)
	agg := a.Aggregate()

	if agg.TotalTraces != 2 {
		t.Fatalf("expected 2 traces, got %d", agg.TotalTraces)
	}
	if agg.AvgToolCalls != 15 {
		t.Fatalf("expected avg 15, got %.2f", agg.AvgToolCalls)
	}
	if agg.AvgToolSuccessRate != 0.75 {
		t.Fatalf("expected avg success rate 0.75, got %.4f", agg.AvgToolSuccessRate)
	}
	if agg.AvgSteps != 3 {
		t.Fatalf("expected avg 3 steps, got %.2f", agg.AvgSteps)
	}
}

func TestAggregator_Empty(t *testing.T) {
	a := NewMetricsAggregator()
	agg := a.Aggregate()
	if agg.TotalTraces != 0 {
		t.Fatalf("expected 0 traces, got %d", agg.TotalTraces)
	}
}

func TestMetricsWindow(t *testing.T) {
	w := NewMetricsWindow(24 * 3600 * 1000000000) // 24h in ns
	m := NewAgentMetrics()
	m.ToolCalls = 5
	m.Steps = 3

	w.Add(m)
	agg := w.Aggregate()
	if agg.TotalTraces != 1 {
		t.Fatalf("expected 1 trace, got %d", agg.TotalTraces)
	}
}

func TestExporter_Text(t *testing.T) {
	e := NewExporter("test")
	m := NewAgentMetrics()
	m.ToolCalls = 10
	m.ToolSuccesses = 8
	m.ToolSuccessRate = 0.8
	m.CheckpointSaves = 5
	m.Steps = 4
	m.NodesExecuted = 12

	text := e.ExportText(m)
	if !strings.Contains(text, "test_tool_calls_total 10") {
		t.Fatalf("expected tool call metric in output:\n%s", text)
	}
	if !strings.Contains(text, "test_tool_success_rate 0.8000") {
		t.Fatalf("expected success rate in output:\n%s", text)
	}
	if !strings.Contains(text, "test_checkpoint_saves_total 5") {
		t.Fatalf("expected checkpoint metric in output:\n%s", text)
	}
}

func TestExporter_CSV(t *testing.T) {
	e := NewExporter("")
	m := NewAgentMetrics()
	m.TraceID = "t1"
	m.ToolCalls = 5
	m.ToolSuccesses = 4
	m.Steps = 3

	csv := e.ExportCSV(m)
	if !strings.HasPrefix(csv, "t1,") {
		t.Fatalf("expected CSV starting with trace ID, got: %s", csv)
	}
}

func TestPercentile(t *testing.T) {
	data := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	if p := percentile(data, 50); p != 5 {
		t.Fatalf("P50: expected 5, got %.0f", p)
	}
	if p := percentile(data, 95); p != 10 {
		t.Fatalf("P95: expected 10, got %.0f", p)
	}
	if p := percentile(data, 99); p != 10 {
		t.Fatalf("P99: expected 10, got %.0f", p)
	}
}

func TestPercentile_Empty(t *testing.T) {
	if p := percentile(nil, 50); p != 0 {
		t.Fatalf("expected 0 for empty data, got %.0f", p)
	}
}
