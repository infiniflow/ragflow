package core

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ragflow/internal/harness/core/schema"
)

// fhcAgent is a minimal Agent used to exercise flowAgent.genInput directly.
type fhcAgent struct{ name string }

func (a *fhcAgent) Name(context.Context) string        { return a.name }
func (a *fhcAgent) Description(context.Context) string { return a.name }
func (a *fhcAgent) Run(context.Context, *AgentInput, ...RunOption) *AsyncIterator[*AgentEvent] {
	panic("fhcAgent.Run must not be called by genInput tests")
}

func fhcEvent(agentName, content string) *AgentEvent {
	return &AgentEvent{
		AgentName: agentName,
		Output: &AgentOutput{MessageOutput: &MessageVariant{
			Message: &schema.Message{Role: schema.RoleAssistant, Content: content},
		}},
	}
}

func fhcToolEvent(agentName, content, toolName string) *AgentEvent {
	return &AgentEvent{
		AgentName: agentName,
		Output: &AgentOutput{MessageOutput: &MessageVariant{
			Role:     schema.RoleTool,
			ToolName: toolName,
			Message:  &schema.Message{Role: schema.RoleTool, Content: content, ToolName: toolName},
		}},
	}
}

func fhcStreamingEvent(agentName string) *AgentEvent {
	return &AgentEvent{
		AgentName: agentName,
		Output: &AgentOutput{MessageOutput: &MessageVariant{
			IsStreaming: true,
			Message:     &schema.Message{Role: schema.RoleAssistant, Content: "partial"},
		}},
	}
}

func fhcTransferEvent(agentName, dest string) *AgentEvent {
	ev := EventFromMessage(
		&schema.Message{Role: schema.RoleTool, Content: `{"agent":"` + dest + `"}`, Name: dest, ToolName: "transfer_to_" + dest},
		nil, schema.RoleTool, "transfer_to_"+dest,
	)
	ev.AgentName = agentName
	ev.Action = &AgentAction{TransferToAgent: &TransferToAgentAction{DestAgentName: dest}}
	return ev
}

// fhcFullReplayEntries reconstructs entries by replaying the whole event log.
// Every incremental result is checked against it.
func fhcFullReplayEntries(root *AgentInput, events []any, skipTransfer bool) []*HistoryEntry {
	entries := make([]*HistoryEntry, 0, len(root.Messages)+len(events))
	for _, m := range root.Messages {
		entries = append(entries, &HistoryEntry{IsUserInput: true, Message: m})
	}
	for _, ev := range events {
		ae, ok := ev.(*AgentEvent)
		if !ok {
			continue
		}
		if skipTransfer && ae.Action != nil && ae.Action.TransferToAgent != nil {
			if ae.Output != nil && ae.Output.MessageOutput != nil && ae.Output.MessageOutput.Role == schema.RoleTool && len(entries) > 0 {
				entries = entries[:len(entries)-1]
			}
			continue
		}
		msg := msgFromEvent(ae)
		if msg == nil {
			continue
		}
		entries = append(entries, &HistoryEntry{AgentName: ae.AgentName, Message: msg})
	}
	return entries
}

func fhcAssertMessages(t *testing.T, want, got []Message) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("message count = %d, want %d\n got: %v\nwant: %v", len(got), len(want), fhcContents(got), fhcContents(want))
	}
	for i := range want {
		if !equalMessages(want[i], got[i]) {
			t.Fatalf("message %d mismatch:\n got: %+v\nwant: %+v", i, got[i], want[i])
		}
	}
}

func fhcContents(msgs []Message) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		if m == nil {
			out[i] = "<nil>"
			continue
		}
		out[i] = fmt.Sprintf("%s:%q", m.Role, m.Content)
	}
	return out
}

func fhcCacheState(rc *runContext, skipTransfer bool) (scanned, lastIndex int, version uint64) {
	rc.historyMu.Lock()
	defer rc.historyMu.Unlock()
	c := rc.historyCacheFor(skipTransfer)
	return c.scannedEvents, c.lastEventIndex, c.entriesVersion
}

func TestFlowAgentGenInput_IncrementalMatchesFullReplay(t *testing.T) {
	ctx := t.Context()
	root := &AgentInput{Messages: []Message{schema.UserMessage("root question"), schema.UserMessage("follow-up")}}
	rc := &runContext{RootInput: root, Session: newRunSession()}
	fa := toFlowAgent(ctx, &fhcAgent{name: "supervisor"})
	if !fa.builtinHistory {
		t.Fatal("expected built-in history rewriter")
	}

	batches := [][]any{
		{fhcEvent("worker", "first"), fhcEvent("worker", "second")},
		{fhcEvent("supervisor", "supervisor reply"), fhcToolEvent("worker", "tool result", "search")},
		{},
		{"not an agent event", fhcEvent("worker", "third")},
		{fhcStreamingEvent("worker"), fhcEvent("worker", "")},
	}

	var events []any
	for i, batch := range batches {
		for _, ev := range batch {
			rc.Session.addEvent(ev)
			events = append(events, ev)
		}
		got, err := fa.genInput(ctx, rc, false)
		if err != nil {
			t.Fatalf("batch %d: genInput: %v", i, err)
		}
		want := defaultRewriteEntries("supervisor", fhcFullReplayEntries(root, events, false))
		fhcAssertMessages(t, want, got.Messages)
	}
}

func TestFlowAgentGenInput_OnlyScansNewEvents(t *testing.T) {
	ctx := t.Context()
	root := &AgentInput{Messages: []Message{schema.UserMessage("question")}}
	rc := &runContext{RootInput: root, Session: newRunSession()}
	fa := toFlowAgent(ctx, &fhcAgent{name: "supervisor"})

	const initial = 100
	for i := 0; i < initial; i++ {
		rc.Session.addEvent(fhcEvent("worker", fmt.Sprintf("event %d", i)))
	}
	if _, err := fa.genInput(ctx, rc, false); err != nil {
		t.Fatalf("initial genInput: %v", err)
	}
	scanned, last, version := fhcCacheState(rc, false)
	if scanned != initial || last != initial {
		t.Fatalf("initial cache state = scanned %d, last %d; want %d, %d", scanned, last, initial, initial)
	}

	// Regression: normal agent steps append through runSession.addEvent, which
	// must not bump the epoch or force a rebuild. The next transfer may only
	// scan the newly appended events.
	const appended = 5
	for i := 0; i < appended; i++ {
		rc.Session.addEvent(fhcEvent("worker", fmt.Sprintf("new event %d", i)))
	}
	if _, err := fa.genInput(ctx, rc, false); err != nil {
		t.Fatalf("incremental genInput: %v", err)
	}
	scanned, last, versionAfter := fhcCacheState(rc, false)
	if scanned != initial+appended {
		t.Fatalf("scanned events = %d, want %d (full replay re-scanned the log)", scanned, initial+appended)
	}
	if last != initial+appended {
		t.Fatalf("lastProcessedEventIndex = %d, want %d", last, initial+appended)
	}
	if versionAfter != version {
		t.Fatalf("entriesVersion changed from %d to %d after plain addEvent; cache was rebuilt", version, versionAfter)
	}
}

func TestFlowAgentGenInput_InvalidatedOnSessionMutation(t *testing.T) {
	ctx := t.Context()
	root := &AgentInput{Messages: []Message{schema.UserMessage("question")}}
	rc := &runContext{RootInput: root, Session: newRunSession()}
	fa := toFlowAgent(ctx, &fhcAgent{name: "supervisor"})

	const initial = 20
	for i := 0; i < initial; i++ {
		rc.Session.addEvent(fhcEvent("worker", fmt.Sprintf("event %d", i)))
	}
	if _, err := fa.genInput(ctx, rc, false); err != nil {
		t.Fatalf("initial genInput: %v", err)
	}
	if scanned, _, _ := fhcCacheState(rc, false); scanned != initial {
		t.Fatalf("initial scanned = %d, want %d", scanned, initial)
	}

	// Plain appends stay incremental.
	rc.Session.addEvent(fhcEvent("worker", "appended"))
	if _, err := fa.genInput(ctx, rc, false); err != nil {
		t.Fatalf("append genInput: %v", err)
	}
	if scanned, _, _ := fhcCacheState(rc, false); scanned != initial+1 {
		t.Fatalf("scanned after append = %d, want %d", scanned, initial+1)
	}

	// A parallel lane join mutates the event log outside addEvent and must
	// invalidate the cache.
	commitEvents(rc, []*eventWrapEntry{{
		Event:     fhcEvent("worker", "joined lane event"),
		Timestamp: time.Now().UnixNano(),
	}})
	if _, err := fa.genInput(ctx, rc, false); err != nil {
		t.Fatalf("join genInput: %v", err)
	}
	allEvents := rc.Session.getEvents()
	scanned, last, _ := fhcCacheState(rc, false)
	if scanned != 2*(initial+1)+1 {
		t.Fatalf("scanned after join = %d, want full rebuild of %d events", scanned, 2*(initial+1)+1)
	}
	if last != len(allEvents) {
		t.Fatalf("lastProcessedEventIndex after join = %d, want %d", last, len(allEvents))
	}
	want := defaultRewriteEntries("supervisor", fhcFullReplayEntries(root, allEvents, false))
	got, err := fa.genInput(ctx, rc, false)
	if err != nil {
		t.Fatalf("post-join genInput: %v", err)
	}
	fhcAssertMessages(t, want, got.Messages)

	// A rewind (truncating the log) must also invalidate the cache and rebuild
	// from the truncated history.
	rc.Session.mu.Lock()
	rc.Session.events = rc.Session.events[:initial/2]
	rc.Session.epoch++
	rc.Session.mu.Unlock()

	if _, err := fa.genInput(ctx, rc, false); err != nil {
		t.Fatalf("rewind genInput: %v", err)
	}
	rewound := rc.Session.getEvents()
	if len(rewound) != initial/2 {
		t.Fatalf("rewound event count = %d, want %d", len(rewound), initial/2)
	}
	scannedAfterRewrite, lastAfterRewrite, _ := fhcCacheState(rc, false)
	if lastAfterRewrite != len(rewound) {
		t.Fatalf("lastProcessedEventIndex after rewind = %d, want %d", lastAfterRewrite, len(rewound))
	}
	if scannedAfterRewrite-lastAfterRewrite <= 0 {
		t.Fatalf("rewind did not rescan: scanned=%d last=%d", scannedAfterRewrite, lastAfterRewrite)
	}
	got, err = fa.genInput(ctx, rc, false)
	if err != nil {
		t.Fatalf("rewound genInput: %v", err)
	}
	want = defaultRewriteEntries("supervisor", fhcFullReplayEntries(root, rewound, false))
	fhcAssertMessages(t, want, got.Messages)
}

func TestFlowAgentGenInput_SkipTransferMatchesFullReplay(t *testing.T) {
	ctx := t.Context()
	root := &AgentInput{Messages: []Message{schema.UserMessage("question")}}
	rc := &runContext{RootInput: root, Session: newRunSession()}
	fa := toFlowAgent(ctx, &fhcAgent{name: "supervisor"})

	batches := [][]*AgentEvent{
		{fhcEvent("worker", "before transfer"), fhcTransferEvent("worker", "next")},
		{fhcEvent("next", "next reply")},
		{fhcTransferEvent("supervisor", "worker"), fhcEvent("worker", "after second transfer")},
	}

	var events []any
	for i, batch := range batches {
		for _, ev := range batch {
			rc.Session.addEvent(ev)
			events = append(events, ev)
		}
		for _, skip := range []bool{false, true} {
			got, err := fa.genInput(ctx, rc, skip)
			if err != nil {
				t.Fatalf("batch %d skip=%v: genInput: %v", i, skip, err)
			}
			want := defaultRewriteEntries("supervisor", fhcFullReplayEntries(root, events, skip))
			fhcAssertMessages(t, want, got.Messages)
		}
	}
}

func TestFlowAgentGenInput_NestedSupervisorWorker(t *testing.T) {
	ctx := t.Context()
	root := &AgentInput{Messages: []Message{schema.UserMessage("question")}}
	rc := &runContext{RootInput: root, Session: newRunSession()}

	sup, err := SetSubAgents(ctx, &fhcAgent{name: "supervisor"}, []Agent{&fhcAgent{name: "worker"}})
	if err != nil {
		t.Fatalf("SetSubAgents: %v", err)
	}
	faSup := sup.(*flowAgent)
	if len(faSup.subAgents) != 1 {
		t.Fatalf("sub-agent count = %d, want 1", len(faSup.subAgents))
	}
	faWorker := faSup.subAgents[0]
	if faWorker.historyName != "worker" {
		t.Fatalf("worker history name = %q, want worker", faWorker.historyName)
	}

	batches := [][]*AgentEvent{
		{fhcEvent("supervisor", "routing to worker"), fhcTransferEvent("supervisor", "worker")},
		{fhcEvent("worker", "worker result"), fhcToolEvent("worker", "tool output", "lookup")},
		{fhcEvent("supervisor", "final answer")},
	}

	var events []any
	for i, batch := range batches {
		for _, ev := range batch {
			rc.Session.addEvent(ev)
			events = append(events, ev)
		}
		for _, tc := range []struct {
			name    string
			fa      *flowAgent
			rewrite string
		}{
			{"supervisor", faSup, "supervisor"},
			{"worker", faWorker, "worker"},
		} {
			got, err := tc.fa.genInput(ctx, rc, false)
			if err != nil {
				t.Fatalf("batch %d %s: genInput: %v", i, tc.name, err)
			}
			want := defaultRewriteEntries(tc.rewrite, fhcFullReplayEntries(root, events, false))
			fhcAssertMessages(t, want, got.Messages)
		}
	}
}

func TestFlowAgentGenInput_CustomRewriterStillCallsEveryTransfer(t *testing.T) {
	ctx := t.Context()
	root := &AgentInput{Messages: []Message{schema.UserMessage("question")}}
	rc := &runContext{RootInput: root, Session: newRunSession()}

	var calls int
	custom := func(_ context.Context, entries []*HistoryEntry) ([]Message, error) {
		calls++
		// Mutating the received entries must not corrupt the shared cache.
		if len(entries) > 0 {
			if entries[0].Message != nil {
				entries[0].Message.Content = "mutated"
			}
			entries[0].Message = nil
		}
		return []Message{schema.UserMessage("custom")}, nil
	}
	fa := toFlowAgent(ctx, &fhcAgent{name: "custom"}, WithHistoryRewriter(custom))
	if fa.builtinHistory {
		t.Fatal("custom rewriter must not be marked built-in")
	}

	rc.Session.addEvent(fhcEvent("worker", "one"))
	for i := 0; i < 3; i++ {
		got, err := fa.genInput(ctx, rc, false)
		if err != nil {
			t.Fatalf("genInput %d: %v", i, err)
		}
		fhcAssertMessages(t, []Message{schema.UserMessage("custom")}, got.Messages)
	}
	if calls != 3 {
		t.Fatalf("custom rewriter calls = %d, want 3", calls)
	}

	rc.historyMu.Lock()
	cached := rc.historyCacheFor(false).entries
	rc.historyMu.Unlock()
	if len(cached) == 0 || cached[0].Message == nil {
		t.Fatal("custom rewriter corrupted the cached history entries")
	}
	if cached[0].Message.Content != "question" {
		t.Fatalf("custom rewriter mutated cached message: got %q", cached[0].Message.Content)
	}
}

func TestFlowAgentGenInput_ReturnedMessagesDoNotPolluteCache(t *testing.T) {
	ctx := t.Context()
	root := &AgentInput{Messages: []Message{schema.UserMessage("question")}}
	rc := &runContext{RootInput: root, Session: newRunSession()}
	fa := toFlowAgent(ctx, &fhcAgent{name: "supervisor"})

	withExtra := fhcEvent("worker", "worker reply")
	withExtra.Output.MessageOutput.Message.Extra = map[string]any{"key": "original"}
	rc.Session.addEvent(withExtra)
	rc.Session.addEvent(fhcEvent("supervisor", "supervisor reply"))

	got, err := fa.genInput(ctx, rc, false)
	if err != nil {
		t.Fatalf("genInput: %v", err)
	}
	for _, m := range got.Messages {
		if m == nil {
			continue
		}
		m.Content = "mutated"
		m.Role = schema.RoleTool
		if m.Extra != nil {
			m.Extra["key"] = "mutated"
		}
	}

	want := defaultRewriteEntries("supervisor", fhcFullReplayEntries(root, rc.Session.getEvents(), false))
	got, err = fa.genInput(ctx, rc, false)
	if err != nil {
		t.Fatalf("second genInput: %v", err)
	}
	fhcAssertMessages(t, want, got.Messages)

	var extra any
	for _, m := range got.Messages {
		if m != nil && m.Extra != nil {
			extra = m.Extra["key"]
		}
	}
	if extra != "original" {
		t.Fatalf("cached Extra map was polluted: got %v, want original", extra)
	}
}

func TestFlowAgentGenInput_LongSessionDoesNotRescan(t *testing.T) {
	ctx := t.Context()
	root := &AgentInput{Messages: []Message{schema.UserMessage("question")}}
	rc := &runContext{RootInput: root, Session: newRunSession()}
	fa := toFlowAgent(ctx, &fhcAgent{name: "supervisor"})

	const total = 200
	const batchSize = 5
	var events []any
	for i := 0; i < total/batchSize; i++ {
		for j := 0; j < batchSize; j++ {
			ev := fhcEvent("worker", fmt.Sprintf("event %d", i*batchSize+j))
			rc.Session.addEvent(ev)
			events = append(events, ev)
		}
		got, err := fa.genInput(ctx, rc, false)
		if err != nil {
			t.Fatalf("batch %d: genInput: %v", i, err)
		}
		want := defaultRewriteEntries("supervisor", fhcFullReplayEntries(root, events, false))
		fhcAssertMessages(t, want, got.Messages)
	}
	scanned, last, _ := fhcCacheState(rc, false)
	if scanned != total {
		t.Fatalf("scanned events = %d, want %d; long session was replayed", scanned, total)
	}
	if last != total {
		t.Fatalf("lastProcessedEventIndex = %d, want %d", last, total)
	}
}

func BenchmarkFlowAgentGenInput(b *testing.B) {
	ctx := context.Background()
	build := func(size int) (*runContext, *flowAgent) {
		root := &AgentInput{Messages: []Message{schema.UserMessage("question")}}
		rc := &runContext{RootInput: root, Session: newRunSession()}
		fa := toFlowAgent(ctx, &fhcAgent{name: "supervisor"})
		for i := 0; i < size; i++ {
			rc.Session.addEvent(fhcEvent("worker", "event"))
		}
		return rc, fa
	}

	// Each measured transfer happens against a history of size events, with
	// exactly one event appended since the previous transfer.
	for _, size := range []int{10, 50, 200} {
		b.Run(fmt.Sprintf("incremental_%d", size), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				rc, fa := build(size)
				fa.genInput(ctx, rc, false) // prime the cache over the full log
				b.StartTimer()

				rc.Session.addEvent(fhcEvent("worker", "event"))
				if _, err := fa.genInput(ctx, rc, false); err != nil {
					b.Fatalf("genInput: %v", err)
				}
			}
		})
		b.Run(fmt.Sprintf("full_replay_%d", size), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				rc, fa := build(size)
				b.StartTimer()

				rc.Session.addEvent(fhcEvent("worker", "event"))
				if _, err := fa.genInput(ctx, rc, false); err != nil {
					b.Fatalf("genInput: %v", err)
				}
			}
		})
	}
}
