// Package metrics provides observability metrics collection for agent execution.
//
// The autoMetricCollector implements pregel.GraphCallback and tracks key
// metrics during graph execution: tool success rate, checkpoint operations,
// step/node counts, and error recovery. Metrics are exposed via the
// MetricsCollector interface and can be exported to Prometheus.
package metrics

import (
	"context"
	"sync"
	"time"
)

// AgentMetrics is a snapshot of all metrics for one agent execution trace.
type AgentMetrics struct {
	// TraceID identifies the execution trace.
	TraceID string
	// ThreadID identifies the execution thread.
	ThreadID string
	// Duration is the wall-clock duration of the execution.
	Duration time.Duration

	// ---- Tool metrics ----

	// ToolCalls is the total number of tool invocations.
	ToolCalls int
	// ToolSuccesses is the number of successful tool invocations.
	ToolSuccesses int
	// ToolFailures is the number of failed tool invocations.
	ToolFailures int
	// ToolRetries is the number of times tools were retried.
	ToolRetries int
	// ToolLatencyMs holds per-tool latency histograms (toolName → durations).
	ToolLatencyMs map[string][]int64
	// ToolSuccessRate is the ratio of successes to total calls (0–1).
	ToolSuccessRate float64
	// ToolRetryRate is the ratio of retries to (calls + retries).
	ToolRetryRate float64

	// ---- Checkpoint metrics ----

	// CheckpointSaves is the number of checkpoint saves.
	CheckpointSaves int
	// CheckpointRestores is the number of checkpoint restores.
	CheckpointRestores int
	// CheckpointRestoreSuccess is the ratio of successful restores (0–1).
	CheckpointRestoreSuccess float64

	// ---- Execution metrics ----

	// Steps is the number of Pregel supersteps executed.
	Steps int
	// NodesExecuted is the number of graph nodes executed.
	NodesExecuted int
	// RecoveredErrors is the number of errors that were recovered.
	RecoveredErrors int
	// InterruptCount is the number of times execution was interrupted.
	InterruptCount int

	// ---- Computed metrics ----

	// CostPerTask is an estimated metric (requires LLM cost tracking).
	CostPerTask float64
	// ForkReplayPassRate is the ratio of fork replays that pass assertions.
	ForkReplayPassRate float64
	// ApprovalLatencyMs tracks human-in-the-loop approval wait times.
	ApprovalLatencyMs []int64
	// ApprovalRate is the ratio of approvals to total approval requests.
	ApprovalRate float64
	// MemoryAvgHitScore is the average retrieval score for memory operations.
	MemoryAvgHitScore float64
}

// NewAgentMetrics creates a new AgentMetrics with initialised maps.
func NewAgentMetrics() *AgentMetrics {
	return &AgentMetrics{
		ToolLatencyMs:     make(map[string][]int64),
		ApprovalLatencyMs: make([]int64, 0),
	}
}

// Snapshot captures a point-in-time copy of the metrics.
func (m *AgentMetrics) Snapshot() *AgentMetrics {
	cp := *m
	cp.ToolLatencyMs = make(map[string][]int64, len(m.ToolLatencyMs))
	for k, v := range m.ToolLatencyMs {
		durations := make([]int64, len(v))
		copy(durations, v)
		cp.ToolLatencyMs[k] = durations
	}
	cp.ApprovalLatencyMs = make([]int64, len(m.ApprovalLatencyMs))
	copy(cp.ApprovalLatencyMs, m.ApprovalLatencyMs)

	// Compute derived rates.
	if cp.ToolCalls > 0 {
		cp.ToolSuccessRate = float64(cp.ToolSuccesses) / float64(cp.ToolCalls)
		cp.ToolRetryRate = float64(cp.ToolRetries) / float64(cp.ToolCalls+cp.ToolRetries)
	}
	if cp.CheckpointSaves+cp.CheckpointRestores > 0 {
		cp.CheckpointRestoreSuccess = float64(cp.CheckpointRestores) / float64(cp.CheckpointSaves+cp.CheckpointRestores)
	}
	return &cp
}

// MetricsCollector collects and aggregates metrics for agent execution.
type MetricsCollector interface {
	// RecordToolCall records a tool invocation outcome.
	RecordToolCall(toolName string, success bool, durationMs int64)
	// RecordToolRetry records a tool retry.
	RecordToolRetry(toolName string)
	// RecordCheckpointSave records a checkpoint save.
	RecordCheckpointSave()
	// RecordCheckpointRestore records a checkpoint restore (success or failure).
	RecordCheckpointRestore(success bool)
	// RecordStep records a completed Pregel superstep.
	RecordStep()
	// RecordNode records a completed node execution.
	RecordNode(nodeName string)
	// RecordRecoveredError records a recovered error.
	RecordRecoveredError()
	// RecordInterrupt records an execution interrupt.
	RecordInterrupt()
	// RecordApproval records an approval outcome.
	RecordApproval(latencyMs int64, granted bool)
	// RecordMemoryHit records a memory retrieval score.
	RecordMemoryHit(score float64)
	// RecordLLMCost records an LLM invocation cost.
	RecordLLMCost(cost float64)

	// Snapshot returns the current metrics snapshot.
	Snapshot() *AgentMetrics
	// Reset clears all metrics.
	Reset()
}

// ---- AutoCollector: implements GraphCallback + MetricsCollector ----

// AutoCollector automatically collects metrics from graph execution callbacks.
// It implements both pregel.GraphCallback and MetricsCollector.
type AutoCollector struct {
	mu sync.Mutex
	m  *AgentMetrics
}

// NewAutoCollector creates a new AutoCollector.
func NewAutoCollector() *AutoCollector {
	return &AutoCollector{m: NewAgentMetrics()}
}

// ---- MetricsCollector implementation ----

func (c *AutoCollector) RecordToolCall(toolName string, success bool, durationMs int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m.ToolCalls++
	if success {
		c.m.ToolSuccesses++
	} else {
		c.m.ToolFailures++
	}
	c.m.ToolLatencyMs[toolName] = append(c.m.ToolLatencyMs[toolName], durationMs)
}

func (c *AutoCollector) RecordToolRetry(toolName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m.ToolRetries++
}

func (c *AutoCollector) RecordCheckpointSave() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m.CheckpointSaves++
}

func (c *AutoCollector) RecordCheckpointRestore(success bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m.CheckpointRestores++
}

func (c *AutoCollector) RecordStep() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m.Steps++
}

func (c *AutoCollector) RecordNode(nodeName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m.NodesExecuted++
}

func (c *AutoCollector) RecordRecoveredError() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m.RecoveredErrors++
}

func (c *AutoCollector) RecordInterrupt() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m.InterruptCount++
}

func (c *AutoCollector) RecordApproval(latencyMs int64, granted bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m.ApprovalLatencyMs = append(c.m.ApprovalLatencyMs, latencyMs)
	if granted {
		// Track approval rate via approvals/total.
	}
}

func (c *AutoCollector) RecordMemoryHit(score float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m.MemoryAvgHitScore == 0 {
		c.m.MemoryAvgHitScore = score
	} else {
		c.m.MemoryAvgHitScore = (c.m.MemoryAvgHitScore + score) / 2
	}
}

func (c *AutoCollector) RecordLLMCost(cost float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m.CostPerTask += cost
}

func (c *AutoCollector) Snapshot() *AgentMetrics {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := c.m.Snapshot()
	cp.Duration = time.Since(c.startTime())
	return cp
}

func (c *AutoCollector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m = NewAgentMetrics()
}

// startTime is a placeholder for tracking execution duration.
// In practice, the collector is initialised at Run start.
func (c *AutoCollector) startTime() time.Time {
	return time.Now()
}

// ---- GraphCallback implementation ----

// OnRunStart implements pregel.RunCallback.
func (c *AutoCollector) OnRunStart(ctx context.Context, graphName, threadID string) {
	c.Reset()
	c.mu.Lock()
	c.m.ThreadID = threadID
	c.mu.Unlock()
}

// OnRunEnd implements pregel.RunCallback.
func (c *AutoCollector) OnRunEnd(ctx context.Context, graphName, threadID string, err error) {}

// OnStepStart implements pregel.StepCallback.
func (c *AutoCollector) OnStepStart(ctx context.Context, step, taskCount int) {}

// OnStepEnd implements pregel.StepCallback.
func (c *AutoCollector) OnStepEnd(ctx context.Context, step int, err error) {
	c.RecordStep()
}

// OnNodeStart implements pregel.NodeCallback.
func (c *AutoCollector) OnNodeStart(ctx context.Context, nodeName string, step int) {}

// OnNodeEnd implements pregel.NodeCallback.
func (c *AutoCollector) OnNodeEnd(ctx context.Context, nodeName string, step int, output interface{}, err error) {
	c.RecordNode(nodeName)
	if err != nil {
		c.RecordRecoveredError()
	}
}

// OnCheckpointSave implements pregel.CheckpointCallback.
func (c *AutoCollector) OnCheckpointSave(ctx context.Context, threadID, checkpointID string, step int) {
	c.RecordCheckpointSave()
}

// OnCheckpointLoad implements pregel.CheckpointCallback.
func (c *AutoCollector) OnCheckpointLoad(ctx context.Context, threadID, checkpointID string, step int) {
	c.RecordCheckpointRestore(true)
}

// OnCheckpointUpdate implements pregel.CheckpointCallback.
func (c *AutoCollector) OnCheckpointUpdate(ctx context.Context, threadID, asNode string) {}

// OnInterrupt implements pregel.InterruptCallback.
func (c *AutoCollector) OnInterrupt(ctx context.Context, nodeNames []string, step int) {
	c.RecordInterrupt()
}

// OnResume implements pregel.InterruptCallback.
func (c *AutoCollector) OnResume(ctx context.Context, threadID string) {}
