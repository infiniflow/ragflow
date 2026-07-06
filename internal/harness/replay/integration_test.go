package replay

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"ragflow/internal/harness/events"
	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/checkpoint"
	"ragflow/internal/harness/graph/constants"
	"ragflow/internal/harness/graph/graph"
	"ragflow/internal/harness/graph/pregel"
	"ragflow/internal/harness/graph/types"
)

// ============================================================================
// Integration: Tool calls — record → verify → replay
// ============================================================================

func TestIntegration_ToolCalls(t *testing.T) {
	ctx := context.Background()
	eventStore := events.NewMemoryEventStore()
	recorder := events.NewEventRecorder(eventStore, events.WithTraceID("tool-int"))

	// Build a StateGraph with tool-calling nodes.
	sg := graph.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))

	sg.AddNode("search", func(ctx context.Context, state any) (any, error) {
		recorder.RecordToolCall(ctx, "web_search", map[string]any{"q": "RAG architecture"},
			"Search results: RAG = Retrieval Augmented Generation", 350, 0, "")
		m, _ := state.(map[string]any)
		m["value"] = "searched"
		return m, nil
	})

	sg.AddNode("calculator", func(ctx context.Context, state any) (any, error) {
		recorder.RecordToolCall(ctx, "calculator", map[string]any{"expr": "2+2"}, "4", 50, 0, "")
		m, _ := state.(map[string]any)
		m["value"] = "calculated"
		return m, nil
	})

	sg.AddNode("fail_tool", func(ctx context.Context, state any) (any, error) {
		recorder.RecordToolCall(ctx, "failing_tool", map[string]any{}, nil, 100, 2, "permission denied")
		m, _ := state.(map[string]any)
		m["value"] = "failed"
		return m, nil
	})

	if err := sg.AddEdge(constants.Start, "search"); err != nil {
		t.Fatal(err)
	}
	if err := sg.AddEdge("search", "calculator"); err != nil {
		t.Fatal(err)
	}
	if err := sg.AddEdge("calculator", "fail_tool"); err != nil {
		t.Fatal(err)
	}
	if err := sg.AddEdge("fail_tool", constants.End); err != nil {
		t.Fatal(err)
	}

	cb := pregel.NewCallbackManager()
	cb.AddCallback(recorder)

	engine := pregel.NewEngine(sg,
		pregel.WithCheckpointer(checkpoint.NewMemorySaver()),
		pregel.WithCallbacks(cb),
		pregel.WithRecursionLimit(10),
	)

	outputCh, errCh := engine.Run(ctx, map[string]any{"value": ""}, types.StreamModeValues)
	drainOutput(outputCh)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}

	// --- Phase 2: Verify events ---
	iter := eventStore.Stream(ctx, events.EventFilter{TraceID: "tool-int"})
	var recordedEvents []*events.Event
	for {
		ev, ok := iter.Next(ctx)
		if !ok {
			break
		}
		recordedEvents = append(recordedEvents, ev)
	}

	toolStarts := countByType(recordedEvents, events.EventToolCallStart)
	toolResults := countByType(recordedEvents, events.EventToolCallResult)
	graphEvents := countByType(recordedEvents, events.EventGraphStart) +
		countByType(recordedEvents, events.EventGraphEnd)

	if graphEvents != 2 {
		t.Fatalf("expected 2 graph events (start+end), got %d", graphEvents)
	}
	if toolStarts != 3 {
		t.Fatalf("expected 3 tool call starts, got %d", toolStarts)
	}
	if toolResults != 3 {
		t.Fatalf("expected 3 tool call results, got %d", toolResults)
	}

	// Check tool names in payloads.
	for _, ev := range recordedEvents {
		if ev.Type == events.EventToolCallResult {
			var pl events.ToolCallPayload
			if ev.Payload != nil {
				json.Unmarshal(ev.Payload, &pl)
			}
			switch pl.ToolName {
			case "web_search", "calculator", "failing_tool":
			default:
				t.Fatalf("unexpected tool name: %s", pl.ToolName)
			}
			if pl.ToolName == "failing_tool" && pl.Error != "permission denied" {
				t.Fatalf("expected 'permission denied' error, got %q", pl.Error)
			}
			if pl.ToolName == "calculator" && pl.Result != "4" {
				t.Fatalf("expected result '4', got %v", pl.Result)
			}
		}
	}

	// ---- Phase 3: Replay ----
	replayEngine := NewReplayEngine(eventStore)
	replayResult, err := replayEngine.Replay(ctx, &ReplayConfig{
		TraceID:     "tool-int",
		DiffEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(replayResult.Divergences) != 0 {
		t.Fatalf("expected 0 divergences, got %d", len(replayResult.Divergences))
	}
	if replayResult.OriginalLen != len(recordedEvents) {
		t.Fatalf("OriginalLen %d != recorded %d", replayResult.OriginalLen, len(recordedEvents))
	}
	if replayResult.ReplayMetrics.MatchCount != replayResult.ReplayMetrics.TotalEvents {
		t.Fatalf("MatchCount != TotalEvents: %d divergences", replayResult.ReplayMetrics.DivergenceCount)
	}

	// ---- Phase 4: Replay with tool override ----
	overrideCalled := false
	_, err = replayEngine.Replay(ctx, &ReplayConfig{
		TraceID: "tool-int",
		ToolOverride: func(name string, args map[string]any, recorded any) (any, error) {
			overrideCalled = true
			return "OVERRIDDEN", nil
		},
		DiffEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !overrideCalled {
		t.Fatal("ToolOverride was never called")
	}
}

// ============================================================================
// Integration: Sub-agents — record → verify → replay
// ============================================================================

func TestIntegration_SubAgents(t *testing.T) {
	ctx := context.Background()
	eventStore := events.NewMemoryEventStore()
	recorder := events.NewEventRecorder(eventStore, events.WithTraceID("sub-agent-int"))

	sg := graph.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))

	sg.AddNode("researcher", func(ctx context.Context, state any) (any, error) {
		recorder.RecordSubAgentCall(ctx, "researcher", "user query: RAG vs Fine-tuning",
			"RAG is better for dynamic knowledge", 1, 1500, "")
		m, _ := state.(map[string]any)
		m["value"] = "researched"
		return m, nil
	})
	sg.AddNode("writer", func(ctx context.Context, state any) (any, error) {
		recorder.RecordSubAgentCall(ctx, "writer", "summarize research results",
			"Final summary: ...", 1, 800, "")
		m, _ := state.(map[string]any)
		m["value"] = "written"
		return m, nil
	})
	sg.AddNode("aggregator", func(ctx context.Context, state any) (any, error) {
		recorder.RecordSubAgentCall(ctx, "aggregator", "merge results",
			nil, 2, 300, "deadline exceeded")
		m, _ := state.(map[string]any)
		m["value"] = "aggregated"
		return m, nil
	})

	if err := sg.AddEdge(constants.Start, "researcher"); err != nil {
		t.Fatal(err)
	}
	if err := sg.AddEdge("researcher", "writer"); err != nil {
		t.Fatal(err)
	}
	if err := sg.AddEdge("writer", "aggregator"); err != nil {
		t.Fatal(err)
	}
	if err := sg.AddEdge("aggregator", constants.End); err != nil {
		t.Fatal(err)
	}

	cb := pregel.NewCallbackManager()
	cb.AddCallback(recorder)
	engine := pregel.NewEngine(sg,
		pregel.WithCheckpointer(checkpoint.NewMemorySaver()),
		pregel.WithCallbacks(cb),
		pregel.WithRecursionLimit(10),
	)

	outputCh, errCh := engine.Run(ctx, map[string]any{"value": ""}, types.StreamModeValues)
	drainOutput(outputCh)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}

	// Verify sub-agent events.
	iter := eventStore.Stream(ctx, events.EventFilter{TraceID: "sub-agent-int"})
	var all []*events.Event
	for {
		ev, ok := iter.Next(ctx)
		if !ok {
			break
		}
		all = append(all, ev)
	}

	saStarts := countByType(all, events.EventSubAgentCallStart)
	saEnds := countByType(all, events.EventSubAgentCallEnd)
	if saStarts != 3 {
		t.Fatalf("expected 3 sub-agent starts, got %d", saStarts)
	}
	if saEnds != 3 {
		t.Fatalf("expected 3 sub-agent ends, got %d", saEnds)
	}

	for _, ev := range all {
		if ev.Type == events.EventSubAgentCallEnd {
			var pl events.SubAgentCallPayload
			if ev.Payload != nil {
				json.Unmarshal(ev.Payload, &pl)
			}
			switch pl.SubAgentName {
			case "researcher":
				if pl.DurationMs != 1500 {
					t.Fatalf("expected researcher duration 1500, got %d", pl.DurationMs)
				}
			case "aggregator":
				if pl.Error != "deadline exceeded" {
					t.Fatalf("expected aggregator error, got %q", pl.Error)
				}
			}
		}
	}

	// Replay.
	replayEngine := NewReplayEngine(eventStore)
	replayResult, err := replayEngine.Replay(ctx, &ReplayConfig{
		TraceID:     "sub-agent-int",
		DiffEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(replayResult.Divergences) != 0 {
		t.Fatalf("expected 0 divergences, got %d", len(replayResult.Divergences))
	}
	_ = replayResult
}

// ============================================================================
// Integration: Loop / Deep Research — record → verify → replay
// ============================================================================

func TestIntegration_DeepResearchLoop(t *testing.T) {
	ctx := context.Background()
	eventStore := events.NewMemoryEventStore()
	recorder := events.NewEventRecorder(eventStore, events.WithTraceID("deep-research"))

	sg := graph.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))

	sg.AddNode("research_step", func(ctx context.Context, state any) (any, error) {
		m, _ := state.(map[string]any)
		iteration := 1
		if iter, ok := m["iteration"].(int); ok {
			iteration = iter
		}

		var toolName, query, result string
		switch iteration {
		case 1:
			toolName = "web_search"
			query = "deep learning fundamentals"
			result = "DL is a subset of ML using neural networks"
		case 2:
			toolName = "academic_search"
			query = "transformer architecture 2024"
			result = "Transformer: attention is all you need"
		case 3:
			toolName = "code_search"
			query = "Python implementation of RAG"
			result = "langchain + chromadb example"
		default:
			toolName = "summarize"
			query = fmt.Sprintf("iteration %d wrap-up", iteration)
			result = fmt.Sprintf("Final summary after %d iterations", iteration)
		}

		recorder.RecordToolCall(ctx, toolName, map[string]any{"q": query}, result, 200, 0, "")
		recorder.RecordStateWrite(ctx, fmt.Sprintf("result_%d", iteration), nil, result, "set")

		m["iteration"] = iteration + 1
		m["value"] = result
		return m, nil
	})

	if err := sg.AddEdge(constants.Start, "research_step"); err != nil {
		t.Fatal(err)
	}
	if err := sg.AddConditionalEdges("research_step",
		func(ctx context.Context, state any) (any, error) {
			m, _ := state.(map[string]any)
			iteration := 1
			if iter, ok := m["iteration"].(int); ok {
				iteration = iter
			}
			if iteration < 4 {
				return "continue", nil
			}
			return "done", nil
		},
		map[string]string{
			"continue": "research_step",
			"done":     constants.End,
		},
	); err != nil {
		t.Fatal(err)
	}

	cb := pregel.NewCallbackManager()
	cb.AddCallback(recorder)
	engine := pregel.NewEngine(sg,
		pregel.WithCheckpointer(checkpoint.NewMemorySaver()),
		pregel.WithCallbacks(cb),
		pregel.WithRecursionLimit(20),
	)

	outputCh, errCh := engine.Run(ctx, map[string]any{"value": "", "iteration": 1}, types.StreamModeValues)
	drainOutput(outputCh)
	if err := <-errCh; err != nil {
		t.Fatalf("engine run error: %v", err)
	}

	// ---- Phase 2: Verify events ----
	iter := eventStore.Stream(ctx, events.EventFilter{TraceID: "deep-research"})
	var allEvents []*events.Event
	for {
		ev, ok := iter.Next(ctx)
		if !ok {
			break
		}
		allEvents = append(allEvents, ev)
	}

	toolStarts := countByType(allEvents, events.EventToolCallStart)
	toolResults := countByType(allEvents, events.EventToolCallResult)
	stateWrites := countByType(allEvents, events.EventStateWrite)

	if toolStarts < 3 {
		t.Fatalf("expected at least 3 tool calls, got %d", toolStarts)
	}
	if toolStarts != toolResults {
		t.Fatalf("tool starts %d != tool results %d", toolStarts, toolResults)
	}
	if stateWrites < 3 {
		t.Fatalf("expected at least 3 state writes, got %d", stateWrites)
	}

	// Verify tool names across iterations.
	var toolNames []string
	for _, ev := range allEvents {
		if ev.Type == events.EventToolCallStart {
			if name, ok := ev.Metadata["tool"].(string); ok {
				toolNames = append(toolNames, name)
			}
		}
	}
	if len(toolNames) < 3 {
		t.Fatalf("expected at least 3 tools (3 research iterations), got %d", len(toolNames))
	}
	if toolNames[0] != "web_search" {
		t.Fatalf("expected first tool 'web_search', got %q", toolNames[0])
	}
	if toolNames[1] != "academic_search" {
		t.Fatalf("expected second tool 'academic_search', got %q", toolNames[1])
	}
	if toolNames[2] != "code_search" {
		t.Fatalf("expected third tool 'code_search', got %q", toolNames[2])
	}

	// ---- Phase 3: Replay ----
	replayEngine := NewReplayEngine(eventStore)
	replayResult, err := replayEngine.Replay(ctx, &ReplayConfig{
		TraceID:     "deep-research",
		DiffEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(replayResult.Divergences) != 0 {
		t.Fatalf("expected 0 divergences, got %d", len(replayResult.Divergences))
	}
	if replayResult.OriginalLen != replayResult.ReplayLen {
		t.Fatalf("OriginalLen %d != ReplayLen %d", replayResult.OriginalLen, replayResult.ReplayLen)
	}

	// ---- Phase 4: Replay with tool override on first tool ----
	overrideReplay, err := replayEngine.Replay(ctx, &ReplayConfig{
		TraceID:     "deep-research",
		DiffEnabled: true,
		ToolOverride: func(name string, args map[string]any, recorded any) (any, error) {
			if name == "web_search" {
				return "OVERRIDDEN RESEARCH", nil
			}
			return recorded, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(overrideReplay.Divergences) == 0 {
		t.Fatal("expected divergences with tool override")
	}
}

// ============================================================================
// Helpers
// ============================================================================

func countByType(evts []*events.Event, typ events.EventType) int {
	n := 0
	for _, ev := range evts {
		if ev.Type == typ {
			n++
		}
	}
	return n
}

func drainOutput(ch <-chan interface{}) {
	for range ch {
	}
}
