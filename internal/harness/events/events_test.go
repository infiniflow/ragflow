package events

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func TestMemoryEventStore_AppendAndStream(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryEventStore()

	ev1 := &Event{ID: "e1", Type: EventGraphStart, Clock: 1, Timestamp: time.Now(), TraceID: "trace-1"}
	ev2 := &Event{ID: "e2", Type: EventGraphEnd, Clock: 2, Timestamp: time.Now(), TraceID: "trace-1"}

	if err := s.Append(ctx, ev1, ev2); err != nil {
		t.Fatal(err)
	}

	// Stream all.
	iter := s.Stream(ctx, EventFilter{})
	var got []*Event
	for {
		ev, ok := iter.Next(ctx)
		if !ok {
			break
		}
		got = append(got, ev)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 events, got %d", len(got))
	}
	if got[0].ID != "e1" || got[1].ID != "e2" {
		t.Fatal("events out of order")
	}
}

func TestMemoryEventStore_Get(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryEventStore()
	s.Append(ctx, &Event{ID: "find-me", Type: EventNodeStart, Clock: 1, TraceID: "t"})

	ev, err := s.Get(ctx, "find-me")
	if err != nil {
		t.Fatal(err)
	}
	if ev == nil {
		t.Fatal("expected event, got nil")
	}

	ev, err = s.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if ev != nil {
		t.Fatal("expected nil for nonexistent")
	}
}

func TestMemoryEventStore_Range(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryEventStore()
	for i := 1; i <= 10; i++ {
		s.Append(ctx, &Event{ID: EventID(fmt.Sprintf("e%d", i)), Clock: uint64(i), Type: EventStepStart, Timestamp: time.Now(), TraceID: "t"})
	}

	events, err := s.Range(ctx, 3, 7, EventFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}
	if events[0].Clock != 3 {
		t.Fatalf("expected clock 3 first, got %d", events[0].Clock)
	}
}

func TestMemoryEventStore_Seek(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryEventStore()
	for i := 1; i <= 5; i++ {
		s.Append(ctx, &Event{ID: EventID(fmt.Sprintf("e%d", i)), Clock: uint64(i), Type: EventStepStart, TraceID: "t"})
	}

	iter, err := s.Seek(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	ev, ok := iter.Next(ctx)
	if !ok || ev.Clock != 3 {
		t.Fatalf("expected clock 3, got %v", ev)
	}
}

func TestMemoryEventStore_Length(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryEventStore()
	n, _ := s.Length(ctx)
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
	s.Append(ctx, &Event{ID: "e1", Clock: 1, Type: EventGraphStart, TraceID: "t"})
	n, _ = s.Length(ctx)
	if n != 1 {
		t.Fatalf("expected 1, got %d", n)
	}
}

func TestMemoryEventStore_Subscribe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := NewMemoryEventStore()

	ch, err := s.Subscribe(ctx, EventFilter{})
	if err != nil {
		t.Fatal(err)
	}

	s.Append(ctx, &Event{ID: "live", Clock: 1, Type: EventNodeStart, TraceID: "t"})

	select {
	case ev := <-ch:
		if ev.ID != "live" {
			t.Fatalf("expected live event, got %s", ev.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for subscribed event")
	}
}

func TestMemoryEventStore_Filter(t *testing.T) {
	f := EventFilter{TraceID: "trace-1", Types: []EventType{EventNodeStart}}
	if !f.Matches(&Event{TraceID: "trace-1", Type: EventNodeStart}) {
		t.Fatal("should match")
	}
	if f.Matches(&Event{TraceID: "trace-2", Type: EventNodeStart}) {
		t.Fatal("should not match different trace")
	}
	if f.Matches(&Event{TraceID: "trace-1", Type: EventNodeEnd}) {
		t.Fatal("should not match different type")
	}
}

func TestLocalFileEventStore_GC_RetainsSurvivingEvents(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	s, err := NewLocalFileEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Write one old event (outside retention) and one recent event.
	oldEv := &Event{ID: "old", Clock: 1, Type: EventGraphStart, TraceID: "gc-test", Timestamp: time.Now().Add(-2 * time.Hour)}
	oldEv.Seal()
	recentEv := &Event{ID: "recent", Clock: 2, Type: EventGraphEnd, TraceID: "gc-test", Timestamp: time.Now()}
	recentEv.Seal()
	if err := s.Append(ctx, oldEv, recentEv); err != nil {
		t.Fatal(err)
	}

	// GC with 1-hour retention.
	if err := s.GC(ctx, time.Hour); err != nil {
		t.Fatal(err)
	}

	// In-memory: old should be gone, recent should remain.
	ev, _ := s.Get(ctx, "old")
	if ev != nil {
		t.Error("old event should be removed from cache")
	}
	ev, _ = s.Get(ctx, "recent")
	if ev == nil {
		t.Fatal("recent event should survive in cache")
	}

	// Reopen the store from disk and verify retained events survived.
	s2, err := NewLocalFileEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	ev, _ = s2.Get(ctx, "recent")
	if ev == nil {
		t.Fatal("recent event should survive GC on disk")
	}
	ev, _ = s2.Get(ctx, "old")
	if ev != nil {
		t.Error("old event should be absent from disk after GC")
	}
}

func TestMemoryEventStore_GC(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryEventStore()
	old := &Event{ID: "old", Clock: 1, Type: EventGraphStart, Timestamp: time.Now().Add(-2 * time.Hour), TraceID: "t"}
	s.Append(ctx, old)
	s.Append(ctx, &Event{ID: "new", Clock: 2, Type: EventGraphEnd, Timestamp: time.Now(), TraceID: "t"})

	s.GC(ctx, time.Hour)

	ev, _ := s.Get(ctx, "old")
	if ev != nil {
		t.Fatal("old event should have been GC'd")
	}
	ev, _ = s.Get(ctx, "new")
	if ev == nil {
		t.Fatal("new event should survive GC")
	}
}

func TestLocalFileEventStore_AppendAndReopen(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	s, err := NewLocalFileEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	for i := 1; i <= 5; i++ {
		s.Append(ctx, &Event{ID: EventID(fmt.Sprintf("f%d", i)), Clock: uint64(i), Type: EventStepStart, TraceID: "reopen", Timestamp: time.Now()})
	}

	// Reopen — should load existing events.
	s2, err := NewLocalFileEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	iter := s2.Stream(ctx, EventFilter{TraceID: "reopen"})
	count := 0
	for {
		_, ok := iter.Next(ctx)
		if !ok {
			break
		}
		count++
	}
	if count != 5 {
		t.Fatalf("expected 5 events after reopen, got %d", count)
	}
}

func TestLocalFileEventStore_SegmentRotation(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	// Use small maxSize to force rotation.
	s := &LocalFileEventStore{
		dir:     dir,
		segment: 0,
		maxSize: 100, // 100 bytes per segment
		cached:  make([]*Event, 0),
	}

	// Write events large enough to trigger rotation.
	for i := 0; i < 20; i++ {
		ev := &Event{
			ID:        EventID(fmt.Sprintf("seg-%d", i)),
			Clock:     uint64(i + 1),
			Type:      EventGraphStart,
			TraceID:   "rotation-test",
			Timestamp: time.Now(),
			Metadata: map[string]any{
				"data": fmt.Sprintf("x=%s", string(make([]byte, 50))),
			},
		}
		ev.Seal()
		s.Append(ctx, ev)
	}

	// Verify all events survive rotation.
	s.mu.RLock()
	count := len(s.cached)
	s.mu.RUnlock()
	if count != 20 {
		t.Fatalf("expected 20 events, got %d", count)
	}

	// Check multiple segment files exist.
	entries, _ := os.ReadDir(dir)
	segCount := 0
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".jsonl" {
			segCount++
		}
	}
	if segCount < 2 {
		t.Fatalf("expected at least 2 segment files, got %d", segCount)
	}

	// Reopen and verify.
	s2, err := NewLocalFileEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	iter := s2.Stream(ctx, EventFilter{})
	var all []*Event
	for {
		ev, ok := iter.Next(ctx)
		if !ok {
			break
		}
		all = append(all, ev)
	}
	if len(all) != 20 {
		t.Fatalf("expected 20 events after reopen, got %d", len(all))
	}
	// Verify event order by Clock.
	sort.Slice(all, func(i, j int) bool { return all[i].Clock < all[j].Clock })
	for i, ev := range all {
		if ev.Clock != uint64(i+1) {
			t.Fatalf("event %d: expected clock %d, got %d", i, i+1, ev.Clock)
		}
	}
}

func TestEvent_Seal(t *testing.T) {
	ev := NewEvent(EventNodeStart, 1)
	ev.Payload, _ = json.Marshal(StateTransitionPayload{Channel: "msg"})
	ev.Seal()
	if ev.Hash == "" {
		t.Fatal("hash should be set after Seal")
	}
	// Same payload should produce same hash.
	ev2 := NewEvent(EventNodeStart, 1)
	ev2.Payload = ev.Payload
	ev2.Seal()
	if ev.Hash != ev2.Hash {
		t.Fatal("identical events should have identical hashes")
	}
}

func TestMemoryEventStore_Snapshot(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryEventStore()
	s.Append(ctx, &Event{ID: "s1", Clock: 1, Type: EventGraphStart, TraceID: "snap-trace", Timestamp: time.Now()})
	s.Append(ctx, &Event{ID: "s2", Clock: 2, Type: EventGraphEnd, TraceID: "snap-trace", Timestamp: time.Now()})

	snap, err := s.CreateSnapshot(ctx, "snap-trace")
	if err != nil {
		t.Fatal(err)
	}
	if snap.TraceID != "snap-trace" {
		t.Fatalf("expected snap-trace, got %s", snap.TraceID)
	}
	if snap.Data == nil {
		t.Fatal("snapshot data should not be nil")
	}
}

func TestLogicalClock(t *testing.T) {
	c := NewLogicalClock()
	if c.Now() != 0 {
		t.Fatalf("expected 0, got %d", c.Now())
	}
	v1 := c.Tick()
	if v1 != 1 {
		t.Fatalf("expected 1, got %d", v1)
	}
	v2 := c.Tick()
	if v2 != 2 {
		t.Fatalf("expected 2, got %d", v2)
	}
	c.Reset()
	if c.Now() != 0 {
		t.Fatalf("expected 0 after reset, got %d", c.Now())
	}
}

func TestEventRecorder_GraphCallback(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryEventStore()
	r := NewEventRecorder(store, WithTraceID("rec-trace"), WithThreadID("rec-thread"))

	// Dispatch all GraphCallback methods.
	r.OnRunStart(ctx, "test-graph", "rec-thread")
	r.OnStepStart(ctx, 0, 3)
	r.OnNodeStart(ctx, "node-a", 0)
	r.OnNodeEnd(ctx, "node-a", 0, "output", nil)
	r.OnCheckpointSave(ctx, "rec-thread", "cp-1", 0)
	r.OnCheckpointLoad(ctx, "rec-thread", "cp-1", 0)
	r.OnInterrupt(ctx, []string{"node-a"}, 0)
	r.OnResume(ctx, "rec-thread")
	r.OnStepEnd(ctx, 0, nil)
	r.OnRunEnd(ctx, "test-graph", "rec-thread", nil)

	// Verify all events recorded.
	iter := store.Stream(ctx, EventFilter{TraceID: "rec-trace"})
	var events []*Event
	for {
		ev, ok := iter.Next(ctx)
		if !ok {
			break
		}
		events = append(events, ev)
	}
	if len(events) != 10 {
		t.Fatalf("expected 10 events, got %d: %v", len(events), collectTypes(events))
	}
}

func TestEventRecorder_ModelAndToolCalls(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryEventStore()
	r := NewEventRecorder(store, WithTraceID("mt-trace"))

	r.RecordModelCall(ctx, "gpt-4", "openai", []any{"hello"}, "world", TokenUsage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30}, 500, 0.002)
	r.RecordModelCall(ctx, "gpt-4", "openai", []any{"hello2"}, "world2", TokenUsage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30}, 500, 0.002)
	r.RecordLLMChunk(ctx, "gpt-4", "wor")
	r.RecordLLMChunk(ctx, "gpt-4", "ld")
	r.RecordToolCall(ctx, "get_weather", map[string]any{"city": "NYC"}, "sunny", 200, 0, "")
	r.RecordToolCall(ctx, "fail_tool", map[string]any{}, nil, 100, 2, "timeout")

	iter := store.Stream(ctx, EventFilter{TraceID: "mt-trace"})
	count := 0
	for {
		_, ok := iter.Next(ctx)
		if !ok {
			break
		}
		count++
	}
	expected := 10 // 2 model calls × 2 + 2 chunks + 2 tool calls × 2
	if count != expected {
		t.Fatalf("expected %d events, got %d", expected, count)
	}
}

func TestEventRecorder_FineGrained(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryEventStore()
	r := NewEventRecorder(store, WithTraceID("fine"))

	r.RecordStateWrite(ctx, "messages", nil, []any{"hello"}, "append")
	r.RecordMemoryWrite(ctx, "vector", "insert", "doc1", "content", 0.95)
	r.RecordMemoryRead(ctx, "vector", "doc1", 0.92)
	r.RecordApproval(ctx, "req-1", "execute_tool", map[string]any{"tool": "search"}, "granted", 3000)
	r.RecordError(ctx, "connection refused")
	r.RecordRetry(ctx, "attempt 2/3")
	r.RecordSubAgentCall(ctx, "researcher", "query1", "result1", 1, 500, "")
	r.RecordSessionValue(ctx, "mode", "fast")
	r.RecordSessionTransfer(ctx, "planner", "executor", "plan ready", nil)

	iter := store.Stream(ctx, EventFilter{TraceID: "fine"})
	count := 0
	for {
		_, ok := iter.Next(ctx)
		if !ok {
			break
		}
		count++
	}
	expected := 6 + 4 // original 6 + sub-start/sub-end + session-value + session-transfer
	if count != expected {
		t.Fatalf("expected %d events, got %d", expected, count)
	}
}

func TestEventRecorder_ContextHelpers(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryEventStore()
	r := NewEventRecorder(store, WithTraceID("ctx-test"))

	// Store recorder in context.
	ctx = ContextWithRecorder(ctx, r)

	// Retrieve and verify.
	got := RecorderFromContext(ctx)
	if got == nil {
		t.Fatal("expected non-nil recorder from context")
	}

	// Context without recorder should return nil.
	ctx2 := context.Background()
	if RecorderFromContext(ctx2) != nil {
		t.Fatal("expected nil from context without recorder")
	}
}

func TestSubAgentAndSessionEvents(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryEventStore()
	r := NewEventRecorder(store, WithTraceID("sub-session"))

	r.RecordSubAgentCall(ctx, "researcher", "query1", "result1", 1, 500, "")
	r.RecordSubAgentCall(ctx, "researcher", "query2", "", 2, 0, "timeout")
	r.RecordSessionValue(ctx, "mode", "fast")
	r.RecordSessionValue(ctx, "count", 42)
	r.RecordSessionTransfer(ctx, "a", "b", "reason", "input")

	iter := store.Stream(ctx, EventFilter{TraceID: "sub-session"})
	var evts []*Event
	for {
		ev, ok := iter.Next(ctx)
		if !ok {
			break
		}
		evts = append(evts, ev)
	}
	if len(evts) != 7 { // 2 sub-agent calls × 2 events + 2 values + 1 transfer
		t.Fatalf("expected 7 events, got %d", len(evts))
	}
}

// ---- helpers ----

func collectTypes(events []*Event) []EventType {
	var types []EventType
	for _, ev := range events {
		types = append(types, ev.Type)
	}
	return types
}
