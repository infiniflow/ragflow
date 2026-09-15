package harness

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"

	"ragflow/internal/agent/chat"
)

// TestLogHierarchicalRounds verifies that Log mirrors Python LLMUsageStats.log:
// the orchestrator phase expands into one "orchestrator round N" row per round,
// with the in-loop sub-phases (claim_research / sufficiency) nested underneath,
// while the out-of-loop phases (route / planner / finalize) stay flat.
//
// NOTE: this test cannot run while the unrelated pre-existing break in
// internal/agent/tool/retrieval_nlp.go (RankFeature) blocks compilation of the
// harness test binary. It will run once that is resolved.
func TestLogHierarchicalRounds(t *testing.T) {
	s := NewLLMUsageStats()
	ctx := WithStats(context.Background(), s)

	// Flat phases before the orchestrator loop.
	func() {
		_, done := Phase(ctx, PhaseRoute)
		defer done()
		s.RecordCall(PhaseRoute)
	}()
	func() {
		_, done := Phase(ctx, PhasePlanner)
		defer done()
		s.RecordCall(PhasePlanner)
	}()

	// Orchestrator loop: two rounds, each doing claim_research + sufficiency.
	func() {
		c, done := Phase(ctx, PhaseOrchestrator)
		defer done()

		RecordRound(c, PhaseOrchestrator)
		func() {
			_, d := Phase(c, PhaseClaimResearch)
			defer d()
			s.RecordCall(PhaseClaimResearch)
			s.RecordUsage(PhaseClaimResearch, 0, 0, 120)
		}()
		func() {
			_, d := Phase(c, PhaseSufficiency)
			defer d()
			s.RecordCall(PhaseSufficiency)
		}()

		RecordRound(c, PhaseOrchestrator)
		func() {
			_, d := Phase(c, PhaseClaimResearch)
			defer d()
			s.RecordCall(PhaseClaimResearch)
			s.RecordUsage(PhaseClaimResearch, 0, 0, 80)
		}()
	}()

	// Flat phase after the loop.
	func() {
		_, done := Phase(ctx, PhaseFinalize)
		defer done()
		s.RecordCall(PhaseFinalize)
	}()

	var buf bytes.Buffer
	s.Log(log.New(&buf, "", 0))
	out := buf.String()

	for _, want := range []string{
		"orchestrator round 1",
		"orchestrator round 2",
		"route",
		"planner",
		"finalize",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q\n---\n%s", want, out)
		}
	}
	if !strings.Contains(out, "sufficiency") {
		t.Errorf("log output missing sufficiency\n---\n%s", out)
	}
	// claim_research runs inside every round, so it must appear at least once
	// per round as a nested (indented) row, not just as a flat top-level row.
	if n := strings.Count(out, "claim_research"); n < 2 {
		t.Errorf("expected claim_research at least twice (per round), got %d\n---\n%s", n, out)
	}
}

// TestProgressCtxRoundTrip verifies WithProgress binds a per-request progress
// sink on ctx and CurrentProgress retrieves the same callback.
func TestProgressCtxRoundTrip(t *testing.T) {
	ctx := context.Background()
	if p := CurrentProgress(ctx); p != nil {
		t.Fatal("no progress bound on a bare ctx")
	}
	var got string
	ctx = WithProgress(ctx, func(line string) { got = line })
	p := CurrentProgress(ctx)
	if p == nil {
		t.Fatal("WithProgress must bind a sink")
	}
	p("[Hybrid search] test")
	if got != "[Hybrid search] test" {
		t.Fatalf("sink did not fire: got %q", got)
	}
}

// TestCountingInvokerCountsStreaming mirrors Python CountingChatModel
// async_chat_streamly / async_chat_streamly_delta: a wrapped invoker that DOES
// stream must keep streaming AND be counted. Before CountingInvoker gained a
// Stream method, StreamComplete's type assertion (m.Invoker.(chat.StreamingInvoker))
// failed on the wrapper, so the whole pipeline fell back to a one-shot Invoke and
// the stream was lost — the streaming branch was effectively uncredited.
func TestCountingInvokerCountsStreaming(t *testing.T) {
	stats := NewLLMUsageStats()
	wrapped := &CountingInvoker{
		Inner: &streamingInvoker{pieces: []piece{{text: "hi", isThink: false}}},
		Stats: stats,
	}
	ctx, done := Phase(context.Background(), "finalize")
	defer done()

	resp, err := wrapped.Stream(ctx, nil, chat.Request{}, func(string, bool) error { return nil })
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}
	if resp.Content != "hi" {
		t.Fatalf("Content = %q, want %q", resp.Content, "hi")
	}
	calls, _ := stats.Snapshot()["finalize"]["calls"].(int)
	if calls != 1 {
		t.Fatalf("streaming call not counted: calls = %v, want 1", stats.Snapshot()["finalize"]["calls"])
	}
}

// TestCountingInvokerStreamDeclinesWithoutStreamingInner mirrors Python's
// invariant that the wrapper only streams when the inner model can: a non-
// streaming inner returns an explicit error rather than silently downgrading to
// a blocking Invoke (which would look like a one-shot call in the stats).

// TestCountingInvokerStreamDeclinesWithoutStreamingInner mirrors Python's
// invariant that the wrapper only streams when the inner model can: a non-
// streaming inner returns an explicit error rather than silently downgrading to
// a blocking Invoke (which would look like a one-shot call in the stats).
func TestCountingInvokerStreamDeclinesWithoutStreamingInner(t *testing.T) {
	wrapped := &CountingInvoker{Inner: &plainInvoker{}}
	if _, err := wrapped.Stream(context.Background(), nil, chat.Request{}, nil); err == nil {
		t.Fatal("want an error when the inner invoker cannot stream")
	}
}

// capturingInvoker records the last Request and returns a native tool call so we
// can verify the seam forwards the declared tools (Python native tools) and reads
// the model's calls back out of the native tool_calls field.
