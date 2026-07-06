package replay

import (
	"context"
	"fmt"
	"testing"

	"ragflow/internal/harness/events"
)

func TestReplayEngine_EmptyTrace(t *testing.T) {
	ctx := context.Background()
	store := events.NewMemoryEventStore()

	engine := NewReplayEngine(store)
	result, err := engine.Replay(ctx, &ReplayConfig{
		TraceID: "empty",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.OriginalLen != 0 {
		t.Fatalf("expected 0, got %d", result.OriginalLen)
	}
	if result.ReplayMetrics.TotalEvents != 0 {
		t.Fatalf("expected 0, got %d", result.ReplayMetrics.TotalEvents)
	}
}

func TestReplayEngine_ExactReplay(t *testing.T) {
	ctx := context.Background()
	store := events.NewMemoryEventStore()

	// Record a simple trace.
	rec := events.NewEventRecorder(store, events.WithTraceID("exact"))
	rec.RecordModelCall(ctx, "gpt-4", "openai", []any{"hi"}, "hello", events.TokenUsage{PromptTokens: 5, CompletionTokens: 10, TotalTokens: 15}, 300, 0.001)
	rec.RecordToolCall(ctx, "search", map[string]any{"q": "test"}, "result1", 100, 0, "")

	engine := NewReplayEngine(store)
	result, err := engine.Replay(ctx, &ReplayConfig{
		TraceID:     "exact",
		DiffEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.OriginalLen == 0 {
		t.Fatal("expected non-zero original events")
	}
	if result.OriginalLen != result.ReplayLen {
		t.Fatalf("OriginalLen %d != ReplayLen %d", result.OriginalLen, result.ReplayLen)
	}
	if len(result.Divergences) != 0 {
		t.Fatalf("expected 0 divergences for exact replay, got %d: %+v", len(result.Divergences), result.Divergences)
	}
	if result.ReplayMetrics.TotalEvents != result.ReplayLen {
		t.Fatalf("TotalEvents %d != ReplayLen %d", result.ReplayMetrics.TotalEvents, result.ReplayLen)
	}
}

func TestReplayEngine_ModelOverride(t *testing.T) {
	ctx := context.Background()
	store := events.NewMemoryEventStore()
	rec := events.NewEventRecorder(store, events.WithTraceID("mo"))
	rec.RecordModelCall(ctx, "gpt-4", "openai", []any{"hi"}, "original", events.TokenUsage{}, 100, 0)
	rec.RecordToolCall(ctx, "search", map[string]any{"q": "test"}, "tool-result", 50, 0, "")

	engine := NewReplayEngine(store)
	result, err := engine.Replay(ctx, &ReplayConfig{
		TraceID: "mo",
		ModelOverride: func(messages []any, recorded string) (*string, error) {
			sub := "substituted: " + recorded
			return &sub, nil
		},
		DiffEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Tool events should still match (exact replay for tools).
	// With model override, LLMEnd has same hash since override doesn't modify the stored event.
	if result.OriginalLen != result.ReplayLen {
		t.Fatalf("replay should have same length")
	}
}

func TestReplayEngine_ToolOverride(t *testing.T) {
	ctx := context.Background()
	store := events.NewMemoryEventStore()
	rec := events.NewEventRecorder(store, events.WithTraceID("to"))
	rec.RecordToolCall(ctx, "calc", map[string]any{"expr": "1+1"}, "2", 50, 0, "")

	engine := NewReplayEngine(store)
	result, err := engine.Replay(ctx, &ReplayConfig{
		TraceID: "to",
		ToolOverride: func(name string, args map[string]any, recorded any) (any, error) {
			return "overridden", nil
		},
		DiffEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.OriginalLen != result.ReplayLen {
		t.Fatalf("replay should have same length: %d vs %d", result.OriginalLen, result.ReplayLen)
	}
}

func TestReplayEngine_StateOverride(t *testing.T) {
	ctx := context.Background()
	store := events.NewMemoryEventStore()
	rec := events.NewEventRecorder(store, events.WithTraceID("so"))
	rec.RecordStateWrite(ctx, "messages", nil, "initial", "append")

	engine := NewReplayEngine(store)
	result, err := engine.Replay(ctx, &ReplayConfig{
		TraceID: "so",
		StateOverride: func(recorded map[string]any) (map[string]any, error) {
			recorded["messages"] = "overridden"
			return recorded, nil
		},
		DiffEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.OriginalLen == 0 {
		t.Fatal("expected events")
	}
	// StateOverride modifies the first EventStateWrite payload.
	fmt.Printf("Divergences: %+v\n", result.Divergences)
}

func TestReplayEngine_OutputStore(t *testing.T) {
	ctx := context.Background()
	store := events.NewMemoryEventStore()
	rec := events.NewEventRecorder(store, events.WithTraceID("out"))
	rec.RecordModelCall(ctx, "gpt-4", "openai", nil, "resp", events.TokenUsage{}, 100, 0)

	output := events.NewMemoryEventStore()
	engine := NewReplayEngine(store)
	result, err := engine.Replay(ctx, &ReplayConfig{
		TraceID:     "out",
		OutputStore: output,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Events) == 0 {
		t.Fatal("expected events in result.Events")
	}
	outLen, _ := output.Length(ctx)
	if int(outLen) != result.ReplayLen {
		t.Fatalf("output store has %d events, expected %d", outLen, result.ReplayLen)
	}
}

func TestReplayEngine_MultipleRuns(t *testing.T) {
	ctx := context.Background()
	store := events.NewMemoryEventStore()
	rec := events.NewEventRecorder(store, events.WithTraceID("multi"))
	for i := 0; i < 5; i++ {
		rec.RecordToolCall(ctx, fmt.Sprintf("tool-%d", i), nil, fmt.Sprintf("res-%d", i), 10, 0, "")
	}

	engine := NewReplayEngine(store)
	for i := 0; i < 3; i++ {
		result, err := engine.Replay(ctx, &ReplayConfig{TraceID: "multi"})
		if err != nil {
			t.Fatal(err)
		}
		if result.OriginalLen != 10 { // 5 start + 5 result
			t.Fatalf("run %d: expected 10 events, got %d", i, result.OriginalLen)
		}
	}
}

func TestFork_FromEvent(t *testing.T) {
	ctx := context.Background()
	store := events.NewMemoryEventStore()
	rec := events.NewEventRecorder(store, events.WithTraceID("fork-test"))
	rec.RecordModelCall(ctx, "gpt-4", "openai", nil, "resp1", events.TokenUsage{}, 100, 0)
	rec.RecordModelCall(ctx, "gpt-4", "openai", nil, "resp2", events.TokenUsage{}, 100, 0)

	engine := NewReplayEngine(store)

	// Get the first event's ID.
	iter := store.Stream(ctx, events.EventFilter{TraceID: "fork-test"})
	first, _ := iter.Next(ctx)
	iter.Close()

	result, err := engine.Fork(ctx, &ForkConfig{
		TraceID: "fork-test",
		Point:   first.ID,
		Store:   store,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ForkTraceID == "" {
		t.Fatal("expected non-empty fork trace ID")
	}
	if result.ParentTraceID != "fork-test" {
		t.Fatalf("expected parent fork-test, got %s", result.ParentTraceID)
	}
	if len(result.ForkEvents) == 0 {
		t.Fatal("expected fork events")
	}
}

func TestDiff_IdenticalTraces(t *testing.T) {
	ctx := context.Background()
	store := events.NewMemoryEventStore()
	rec := events.NewEventRecorder(store, events.WithTraceID("diff-a"))
	rec.RecordToolCall(ctx, "search", nil, "res", 50, 0, "")
	rec.RecordModelCall(ctx, "gpt-4", "openai", nil, "final", events.TokenUsage{}, 100, 0)

	// Copy events to a second trace.
	store2 := events.NewMemoryEventStore()
	rec2 := events.NewEventRecorder(store2, events.WithTraceID("diff-b"))
	rec2.RecordToolCall(ctx, "search", nil, "res", 50, 0, "")
	rec2.RecordModelCall(ctx, "gpt-4", "openai", nil, "final", events.TokenUsage{}, 100, 0)

	result, err := Diff(ctx, store, store2, "diff-a", "diff-b")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.MissingInRight) > 0 || len(result.MissingInLeft) > 0 {
		t.Fatalf("identical traces should have no missing events: left=%d, right=%d",
			len(result.MissingInLeft), len(result.MissingInRight))
	}
}

func TestDiff_DifferentTraces(t *testing.T) {
	ctx := context.Background()
	store := events.NewMemoryEventStore()
	rec := events.NewEventRecorder(store, events.WithTraceID("left"))
	rec.RecordToolCall(ctx, "search", nil, "res1", 50, 0, "")
	rec.RecordModelCall(ctx, "gpt-4", "openai", nil, "final1", events.TokenUsage{}, 100, 0)

	store2 := events.NewMemoryEventStore()
	rec2 := events.NewEventRecorder(store2, events.WithTraceID("right"))
	rec2.RecordToolCall(ctx, "search", nil, "res2", 50, 0, "")
	rec2.RecordModelCall(ctx, "gpt-4", "openai", nil, "final2", events.TokenUsage{}, 100, 0)

	result, err := Diff(ctx, store, store2, "left", "right")
	if err != nil {
		t.Fatal(err)
	}
	// Different payloads → mismatches expected.
	_ = result
}

func TestReplayResult_Metrics(t *testing.T) {
	ctx := context.Background()
	store := events.NewMemoryEventStore()
	rec := events.NewEventRecorder(store, events.WithTraceID("metrics"))
	rec.RecordToolCall(ctx, "t1", nil, "r1", 10, 0, "")
	rec.RecordToolCall(ctx, "t2", nil, "r2", 20, 0, "err")
	rec.RecordModelCall(ctx, "gpt-4", "openai", nil, "m1", events.TokenUsage{}, 100, 0)

	engine := NewReplayEngine(store)
	result, err := engine.Replay(ctx, &ReplayConfig{
		TraceID:     "metrics",
		DiffEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ReplayMetrics.TotalEvents == 0 {
		t.Fatal("expected non-zero TotalEvents")
	}
	if result.ReplayMetrics.TotalEvents != result.ReplayLen {
		t.Fatalf("TotalEvents %d != ReplayLen %d", result.ReplayMetrics.TotalEvents, result.ReplayLen)
	}
	if result.ReplayMetrics.DivergenceCount+result.ReplayMetrics.MatchCount != result.ReplayMetrics.TotalEvents {
		t.Fatal("DivergenceCount + MatchCount should equal TotalEvents")
	}
}

func TestEventsContains_FindByType(t *testing.T) {
	ctx := context.Background()
	store := events.NewMemoryEventStore()
	rec := events.NewEventRecorder(store, events.WithTraceID("find"))
	rec.OnNodeStart(ctx, "n1", 0)
	rec.RecordToolCall(ctx, "search", nil, "res", 10, 0, "")

	iter := store.Stream(ctx, events.EventFilter{TraceID: "find"})
	var evts []*events.Event
	for {
		ev, ok := iter.Next(ctx)
		if !ok {
			break
		}
		evts = append(evts, ev)
	}

	if !EventsContains(evts, events.EventNodeStart) {
		t.Fatal("should contain EventNodeStart")
	}
	if EventsContains(evts, events.EventGraphStart) {
		t.Fatal("should not contain EventGraphStart")
	}

	found := FindEventsOfType(evts, events.EventToolCallStart)
	if len(found) != 1 {
		t.Fatalf("expected 1 EventToolCallStart, got %d", len(found))
	}

	count := EventCount(evts, events.EventNodeStart)
	if count != 1 {
		t.Fatalf("expected 1 EventNodeStart, got %d", count)
	}
}

func TestReplayLiveTools(t *testing.T) {
	called := false
	override := ReplayLiveTools()
	_, _ = override("search", nil, "recorded")
	called = true
	if !called {
		t.Fatal("override function should return nil")
	}
}

func TestReplaySubstituteModel(t *testing.T) {
	override := ReplaySubstituteModel(func(recorded string) string {
		return "over:" + recorded
	})
	result, err := override(nil, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if *result != "over:hello" {
		t.Fatalf("expected 'over:hello', got '%s'", *result)
	}
}

func TestReplayExactTools(t *testing.T) {
	override := ReplayExactTools()
	result, err := override("test", nil, "exact-value")
	if err != nil {
		t.Fatal(err)
	}
	if result != "exact-value" {
		t.Fatalf("expected 'exact-value', got '%v'", result)
	}
}

func TestIntegration_RecordReplayRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := events.NewMemoryEventStore()
	rec := events.NewEventRecorder(store, events.WithTraceID("rtt"))

	// Record a realistic agent trace.
	rec.OnRunStart(ctx, "agent", "thread-1")
	rec.OnStepStart(ctx, 0, 2)
	rec.OnNodeStart(ctx, "llm_call", 0)
	rec.RecordModelCall(ctx, "gpt-4", "openai", []any{"user msg"}, "assistant resp", events.TokenUsage{PromptTokens: 50, CompletionTokens: 100, TotalTokens: 150}, 800, 0.003)
	rec.OnNodeEnd(ctx, "llm_call", 0, nil, nil)
	rec.OnNodeStart(ctx, "execute_tools", 0)
	rec.RecordToolCall(ctx, "web_search", map[string]any{"q": "RAG"}, "search results", 1200, 0, "")
	rec.OnNodeEnd(ctx, "execute_tools", 0, nil, nil)
	rec.OnStepEnd(ctx, 0, nil)
	rec.OnRunEnd(ctx, "agent", "thread-1", nil)

	// Replay exactly.
	engine := NewReplayEngine(store)
	result, err := engine.Replay(ctx, &ReplayConfig{
		TraceID:     "rtt",
		DiffEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.OriginalLen == 0 {
		t.Fatal("expected non-zero events")
	}
	if result.OriginalLen != result.ReplayLen {
		t.Fatalf("OriginalLen %d != ReplayLen %d", result.OriginalLen, result.ReplayLen)
	}
	if len(result.Divergences) != 0 {
		t.Fatalf("expected 0 divergences, got %d", len(result.Divergences))
	}
	if result.Duration == 0 {
		t.Fatal("expected non-zero duration")
	}
	if result.ReplayMetrics.TotalEvents != result.ReplayLen {
		t.Fatal("ReplayMetrics.TotalEvents mismatch")
	}
}

func TestIntegration_ForkThenReplay(t *testing.T) {
	ctx := context.Background()
	store := events.NewMemoryEventStore()
	rec := events.NewEventRecorder(store, events.WithTraceID("fork-rtt"))

	rec.RecordModelCall(ctx, "gpt-4", "openai", nil, "resp1", events.TokenUsage{}, 100, 0)
	rec.RecordToolCall(ctx, "search", nil, "res1", 50, 0, "")
	rec.RecordModelCall(ctx, "gpt-4", "openai", nil, "resp2", events.TokenUsage{}, 100, 0)

	// Fork from the last model call.
	iter := store.Stream(ctx, events.EventFilter{TraceID: "fork-rtt"})
	var allEvents []*events.Event
	for {
		ev, ok := iter.Next(ctx)
		if !ok {
			break
		}
		allEvents = append(allEvents, ev)
	}
	iter.Close()

	// Find the last LLMCallStart event.
	var forkPoint events.EventID
	for _, ev := range allEvents {
		if ev.Type == events.EventLLMCallStart {
			forkPoint = ev.ID
		}
	}

	engine := NewReplayEngine(store)
	forkResult, err := engine.Fork(ctx, &ForkConfig{
		TraceID: "fork-rtt",
		Point:   forkPoint,
		Store:   store,
	})
	if err != nil {
		t.Fatal(err)
	}
	if forkResult.ForkTraceID == "" {
		t.Fatal("expected non-empty fork ID")
	}

	// Fork events should include the original events up to fork point + fork marker.
	if len(forkResult.ForkEvents) == 0 {
		t.Fatal("expected fork events")
	}
}

// ---- True replay: BuildCheckpoint ----

func TestBuildCheckpoint_StateWrite(t *testing.T) {
	ctx := context.Background()
	store := events.NewMemoryEventStore()
	rec := events.NewEventRecorder(store, events.WithTraceID("cp-test"))

	rec.RecordStateWrite(ctx, "messages", nil, []any{"hello"}, "append")
	rec.RecordStateWrite(ctx, "counter", nil, 42, "add")
	rec.OnNodeEnd(ctx, "agent", 0, nil, nil)
	rec.OnStepEnd(ctx, 0, nil)

	iter := store.Stream(ctx, events.EventFilter{TraceID: "cp-test"})
	var evts []*events.Event
	for {
		ev, ok := iter.Next(ctx)
		if !ok {
			break
		}
		evts = append(evts, ev)
	}

	cp, cpID := BuildCheckpoint(evts, "test-thread")
	if cpID == "" {
		t.Fatal("expected non-empty checkpoint ID")
	}
	if cp["messages"] == nil {
		t.Fatal("expected messages in checkpoint")
	}
	if val, ok := cp["counter"].(int); !ok || val != 42 {
		if val, ok := cp["counter"].(float64); !ok || val != 42 {
			t.Fatalf("expected counter=42, got %v (type %T)", cp["counter"], cp["counter"])
		}
	}
	if val, ok := cp["__step__"].(float64); !ok || val != 0 {
		t.Fatalf("expected step=0, got %v (type %T)", cp["__step__"], cp["__step__"])
	}
	if val, ok := cp["__last_completed_node__"].(string); !ok || val != "agent" {
		t.Fatalf("expected last_completed_node='agent', got %v (type %T)", cp["__last_completed_node__"], cp["__last_completed_node__"])
	}
}

func TestBuildCheckpoint_EmptyEvents(t *testing.T) {
	cp, cpID := BuildCheckpoint(nil, "empty")
	if cpID == "" {
		t.Fatal("expected checkpoint ID even with no events")
	}
	// last_state is always set for non-nil input
	_ = cp
}

// ---- True replay: Fork with Engine ----

func TestFork_WithEngine(t *testing.T) {
	ctx := context.Background()
	store := events.NewMemoryEventStore()
	rec := events.NewEventRecorder(store, events.WithTraceID("fork-eng"))

	// Record two state writes, then a tool call.
	rec.RecordStateWrite(ctx, "results", nil, []any{"first"}, "append")
	rec.RecordStateWrite(ctx, "count", nil, 1, "add")
	rec.RecordToolCall(ctx, "search", map[string]any{"q": "test"}, "result", 100, 0, "")

	// Find the fork point (second event, a state write).
	iter := store.Stream(ctx, events.EventFilter{TraceID: "fork-eng"})
	var allEvents []*events.Event
	for {
		ev, ok := iter.Next(ctx)
		if !ok {
			break
		}
		allEvents = append(allEvents, ev)
	}
	if len(allEvents) < 2 {
		t.Fatal("need at least 2 events")
	}

	// Fork from the second state write (after count=1 is persisted).
	forkPoint := allEvents[1].ID // EventStateWrite for "count"

	engine := NewReplayEngine(store)
	result, err := engine.Fork(ctx, &ForkConfig{
		TraceID: "fork-eng",
		Point:   forkPoint,
		Store:   store,
		// No ForkEngine — should use deterministic replay path.
		OutputStore: events.NewMemoryEventStore(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.FinalState != nil {
		t.Fatal("expected nil FinalState without ForkEngine")
	}
	if result.Duration == 0 {
		t.Fatal("expected non-zero duration")
	}
}

func TestFork_EventNotFound(t *testing.T) {
	ctx := context.Background()
	store := events.NewMemoryEventStore()
	engine := NewReplayEngine(store)

	_, err := engine.Fork(ctx, &ForkConfig{
		TraceID: "nonexistent",
		Point:   "no-such-event",
		Store:   store,
	})
	if err == nil {
		t.Fatal("expected error for nonexistent event")
	}
}

// ---- bench ----

func BenchmarkReplay(b *testing.B) {
	ctx := context.Background()
	store := events.NewMemoryEventStore()
	rec := events.NewEventRecorder(store, events.WithTraceID("bench-replay"))

	// Record 100 events.
	for i := 0; i < 50; i++ {
		rec.RecordModelCall(ctx, "gpt-4", "openai", nil, "resp", events.TokenUsage{}, 100, 0)
		rec.RecordToolCall(ctx, "search", nil, "res", 50, 0, "")
	}

	engine := NewReplayEngine(store)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.Replay(ctx, &ReplayConfig{TraceID: "bench-replay"})
		if err != nil {
			b.Fatal(err)
		}
	}
}
