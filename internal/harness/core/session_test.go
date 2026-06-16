package core

import (
	"context"
	"sync"
	"testing"

	"ragflow/internal/harness/core/schema"
)

// ======================== Session Values Tests ========================

func TestSessionValues_Basic(t *testing.T) {
	ctx, rc := initRunCtx(context.Background(), "test", &AgentInput{})
	AddSessionValues(ctx, map[string]any{"key1": "val1", "key2": 42})

	rc.mu.Lock()
	v1 := rc.Session.Values["key1"]
	v2 := rc.Session.Values["key2"]
	rc.mu.Unlock()

	if v1 != "val1" {
		t.Errorf("expected 'val1', got %v", v1)
	}
	if v2 != 42 {
		t.Errorf("expected 42, got %v", v2)
	}
}

func TestSessionValues_EmptyContext(t *testing.T) {
	AddSessionValues(context.Background(), map[string]any{"key": "val"})
	// Should not panic
}

func TestSessionValues_NilValues(t *testing.T) {
	ctx, _ := initRunCtx(context.Background(), "test", &AgentInput{})
	AddSessionValues(ctx, nil)
	// Should not panic
}

func TestSessionValues_EmptyMap(t *testing.T) {
	ctx, rc := initRunCtx(context.Background(), "test", &AgentInput{})
	AddSessionValues(ctx, map[string]any{})
	rc.mu.Lock()
	l := len(rc.Session.Values)
	rc.mu.Unlock()
	if l != 0 {
		t.Errorf("expected empty values, got %d", l)
	}
}

func TestSessionValues_ComplexTypes(t *testing.T) {
	ctx, _ := initRunCtx(context.Background(), "test", &AgentInput{})
	AddSessionValues(ctx, map[string]any{
		"str":   "hello",
		"int":   42,
		"float": 3.14,
		"bool":  true,
	})

	rc := getRunCtx(ctx)
	rc.mu.Lock()
	s := rc.Session.Values
	rc.mu.Unlock()
	if s["str"] != "hello" {
		t.Errorf("str value mismatch")
	}
	if s["int"] != 42 {
		t.Errorf("int value mismatch")
	}
	if s["float"] != 3.14 {
		t.Errorf("float value mismatch")
	}
	if s["bool"] != true {
		t.Errorf("bool value mismatch")
	}
}

func TestSessionValues_Overwrite(t *testing.T) {
	ctx, rc := initRunCtx(context.Background(), "test", &AgentInput{})
	AddSessionValues(ctx, map[string]any{"a": 1, "b": 2})
	AddSessionValues(ctx, map[string]any{"b": 99, "c": 3})

	rc.mu.Lock()
	v := rc.Session.Values
	rc.mu.Unlock()
	if v["a"] != 1 {
		t.Errorf("expected a=1, got %v", v["a"])
	}
	if v["b"] != 99 {
		t.Errorf("expected b=99 (overwritten), got %v", v["b"])
	}
	if v["c"] != 3 {
		t.Errorf("expected c=3, got %v", v["c"])
	}
}

func TestSessionValues_Concurrent(t *testing.T) {
	ctx, rc := initRunCtx(context.Background(), "test", &AgentInput{})

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := string(rune('a' + n%26))
			AddSessionValues(ctx, map[string]any{key: n})
		}(i)
	}
	wg.Wait()

	rc.Session.valuesMx.Lock()
	count := len(rc.Session.Values)
	rc.Session.valuesMx.Unlock()
	if count == 0 {
		t.Error("expected some values after concurrent writes")
	}
}

// ======================== RunPath Tests ========================

func TestRunPath_Append(t *testing.T) {
	_, rc := initRunCtx(context.Background(), "agent_a", &AgentInput{})
	rc.appendRunPath(RunStep{agentName: "agent_b"})

	path := rc.getRunPath()
	if len(path) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(path))
	}
	if path[0].String() != "agent_a" {
		t.Errorf("expected first step 'agent_a', got %s", path[0].String())
	}
	if path[1].String() != "agent_b" {
		t.Errorf("expected second step 'agent_b', got %s", path[1].String())
	}
}

func TestRunPath_InitRunCtx(t *testing.T) {
	_, rc := initRunCtx(context.Background(), "root", &AgentInput{})
	if rc == nil {
		t.Fatal("expected non-nil runContext")
	}
	path := rc.getRunPath()
	if len(path) != 1 {
		t.Errorf("expected 1 step, got %d", len(path))
	}
	if path[0].String() != "root" {
		t.Errorf("expected 'root' in run path, got %s", path[0].String())
	}
}

func TestRunPath_SharedParentSession(t *testing.T) {
	ctx, _ := initRunCtx(context.Background(), "parent", &AgentInput{})
	AddSessionValues(ctx, map[string]any{"shared": true})

	childCtxA := forkRunCtx(ctx)
	childCtxB := forkRunCtx(ctx)

	AddSessionValues(childCtxA, map[string]any{"child_a": "val_a"})
	AddSessionValues(childCtxB, map[string]any{"child_b": "val_b"})

	joinRunCtxs(ctx, childCtxA, childCtxB)

	rc := getRunCtx(ctx)
	rc.mu.Lock()
	shared := rc.Session.Values["shared"]
	rc.mu.Unlock()
	if shared != true {
		t.Error("expected shared=true")
	}
}

// ======================== Fork/Join Tests ========================

func TestForkJoinRunCtx_Basic(t *testing.T) {
	ctx, rc := initRunCtx(context.Background(), "root", &AgentInput{})

	childCtx := forkRunCtx(ctx)
	child := getRunCtx(childCtx)
	if child == nil {
		t.Fatal("expected child runCtx")
	}
	// forkRunCtx creates a new session with its own BranchEvents for parallel isolation.
	if child.Session == rc.Session {
		t.Error("fork should create a new session with BranchEvents")
	}
	if child.Session.BranchEvents == nil {
		t.Error("fork should set BranchEvents on child session")
	}

	// Events in the child lane go to BranchEvents.Events.
	child.Session.addEvent("child_event")

	// joinRunCtxs collects lane events and commits them to the parent.
	joinRunCtxs(ctx, childCtx)

	events := rc.Session.getEvents()
	if len(events) == 0 {
		t.Error("expected at least 1 event after join")
	}
	t.Logf("events after fork/join: %d", len(events))
}

func TestForkJoinRunCtx_Nested(t *testing.T) {
	ctx, rc := initRunCtx(context.Background(), "A", &AgentInput{})

	ctxB := forkRunCtx(ctx)
	ctxC := forkRunCtx(ctx)
	ctxD := forkRunCtx(ctxB)

	getRunCtx(ctxB).Session.addEvent("event_B")
	getRunCtx(ctxC).Session.addEvent("event_C")
	getRunCtx(ctxD).Session.addEvent("event_D")

	joinRunCtxs(ctxB, ctxD)
	joinRunCtxs(ctx, ctxB, ctxC)

	events := rc.Session.getEvents()
	if len(events) == 0 {
		t.Error("expected at least 1 event")
	}
	t.Logf("nested events: %d", len(events))
}

// ======================== GobEncode/StreamErrors Tests ========================

func TestEventWrapEntry_GobEncodeNilEvent(t *testing.T) {
	entry := &eventWrapEntry{Event: nil, Timestamp: 0}

	data, err := entry.GobEncode()
	if err != nil {
		t.Fatalf("GobEncode nil event: %v", err)
	}

	var decoded eventWrapEntry
	if err := decoded.GobDecode(data); err != nil {
		t.Fatalf("GobDecode nil event: %v", err)
	}
	if decoded.Event != nil {
		t.Error("expected nil event after decode")
	}
}

func TestEventWrapEntry_ConsumeStream(t *testing.T) {
	stream := schema.NewStreamReader[Message]()
	go func() {
		defer stream.Close()
		stream.Send(&schema.Message{Content: "chunk1"}, nil)
		stream.Send(&schema.Message{Content: "chunk2"}, nil)
	}()

	entry := &eventWrapEntry{
		Event: &AgentEvent{
			Output: &TypedAgentOutput[*schema.Message]{
				MessageOutput: &TypedMessageVariant[*schema.Message]{
					MessageStream: stream,
					IsStreaming:   true,
				},
			},
		},
	}

	entry.consumeStream()

	ae := entry.Event.(*AgentEvent)
	mv := ae.Output.MessageOutput
	if mv.IsStreaming {
		t.Error("expected IsStreaming=false after consume")
	}
	if mv.Message == nil {
		t.Error("expected non-nil Message after consume")
	}
	if mv.MessageStream != nil {
		t.Error("expected nil MessageStream after consume")
	}
}

func TestEventWrapEntry_ConsumeStreamNilEvent(t *testing.T) {
	entry := &eventWrapEntry{Event: nil}
	entry.consumeStream()
}

// ======================== Integration Tests ========================

func TestRunCtx_IntegrationWithRunPath(t *testing.T) {
	ctx, rc := initRunCtx(context.Background(), "first", &AgentInput{})
	AddSessionValues(ctx, map[string]any{"user_id": "u-123"})

	ctx2, _ := initRunCtx(ctx, "second", &AgentInput{})
	AddSessionValues(ctx2, map[string]any{"step": 2})

	path := rc.getRunPath()
	if len(path) != 2 {
		t.Errorf("expected 2 run path steps, got %d", len(path))
	}
	rc.mu.Lock()
	uid := rc.Session.Values["user_id"]
	st := rc.Session.Values["step"]
	rc.mu.Unlock()
	if uid != "u-123" {
		t.Errorf("expected user_id preserved")
	}
	if st != 2 {
		t.Errorf("expected step=2")
	}
}

func TestGobEncode_NonStreamingEvent(t *testing.T) {
	// Verify the GobEncode path handles non-streaming events
	entry := &eventWrapEntry{
		Event:     nil,
		Timestamp: 100,
	}

	data, err := entry.GobEncode()
	if err != nil {
		t.Fatalf("gob encode: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty encoded data")
	}
}
