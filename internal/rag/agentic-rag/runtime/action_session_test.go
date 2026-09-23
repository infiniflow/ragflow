package runtime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"
	"ragflow/internal/agent/chat"
	"ragflow/internal/rag/prompts"
)

// ladderExec answers the tools the navigation ladder can reach. Every call is
// recorded so the test can prove the continuation actually ran.
type ladderExec struct {
	calls []string
	// emptyFor makes the named tool come back empty, which is what drives a
	// weak-rung continuation.
	emptyFor map[string]bool
}

func (e *ladderExec) Execute(_ context.Context, name string, _ map[string]any) (ToolOutcome, error) {
	e.calls = append(e.calls, name)
	if e.emptyFor[name] {
		return ToolOutcome{Status: StatusEmpty, Reason: reasonNoStructure}, nil
	}
	return ToolOutcome{
		Status:      StatusOK,
		Payload:     []any{map[string]any{"kind": name, "content": "hit for " + name, "chunk_id": "c-" + name}},
		EvidenceIDs: []string{"c-" + name},
	}, nil
}

// bigPayloadExec answers every tool with one oversized passage, so the session's cumulative
// tool-payload budget is what decides how much of it the model ever sees.
type bigPayloadExec struct{ size int }

func (e *bigPayloadExec) Execute(_ context.Context, name string, _ map[string]any) (ToolOutcome, error) {
	return ToolOutcome{
		Status:  StatusOK,
		Payload: []any{map[string]any{"kind": name, "chunk_id": "c1", "content": strings.Repeat("字", e.size)}},
	}, nil
}

// TestToolPayloadCutIsAnnounced pins the truncation notice: when a tool result does not fit the
// session's cumulative budget, what the model receives must SAY how much was withheld and how to
// ask for the rest. A silent cut is what let an 8275-code-point standings table reach the model as
// its first ~800, after which the answer reported the table had only the top four finishers.
func TestToolPayloadCutIsAnnounced(t *testing.T) {
	exec := &bigPayloadExec{size: 5000}
	st := &sessionState{
		Tools:        &Toolset{Exec: exec, ThinkingMode: "high"},
		DeadlineLeft: 60,
		ToolCache:    NewToolCache(),
		CtxBudget:    1200, // smaller than one passage: forces the cut
		PendingCalls: []ToolCall{{ID: "call-1", Name: "search_chunks", Args: map[string]any{"query": "x"}}},
	}
	if err := st.toolNode(context.Background()); err != nil {
		t.Fatalf("toolNode: %v", err)
	}
	last := st.Messages[len(st.Messages)-1]
	if !strings.Contains(last.Content, "TRUNCATED") {
		t.Errorf("a cut tool result is not announced:\n%.200s", last.Content)
	}
}

// TestConsumeExchangeUsesIdPrefix pins the id-prefix tagging: the prefix and the
// in-session ladder fallback share rule ids, so their tool_call ids must be tagged
// differently ("nav_<rule>" vs "ladder_<rule>") or the provider rejects the history as a
// duplicate id.
func TestConsumeExchangeUsesIdPrefix(t *testing.T) {
	cases := []struct {
		idPrefix string
		want     string
	}{
		{"nav", "nav_r1"},
		{"ladder", "ladder_r1"},
	}
	for _, c := range cases {
		ex := &navExchange{}
		ex.consumeExchange("r1", "navigate_tree", map[string]any{"q": 1}, ToolOutcome{Payload: []any{}}, c.idPrefix, 0)
		if len(ex.Messages) != 2 {
			t.Fatalf("prefix %q: want 2 messages, got %d", c.idPrefix, len(ex.Messages))
		}
		if got := ex.Messages[0].ToolCalls[0].ID; got != c.want {
			t.Errorf("prefix %q: tool_call id = %q, want %q", c.idPrefix, got, c.want)
		}
		if got := ex.Messages[1].ToolCallID; got != c.want {
			t.Errorf("prefix %q: tool message id = %q, want %q", c.idPrefix, got, c.want)
		}
	}
}

// TestToolNodeContinuesLadderInCode pins the in-code ladder continuation: when the rung
// the model just ran came back weak, the ladder keeps advancing IN CODE through the
// remaining (cheaper, wider) rungs, instead of leaving the model to rediscover the
// fallback one turn at a time. The continuation must also update where the ladder rests,
// so a later call in the SAME batch resumes from there rather than from the original
// resting point.
//
// Unreachable with the shipped navRules (all modeAuto), so this test installs an LLM rung
// to exercise it — the continuation code is what makes a future LLM rung work without
// another change.
func TestToolNodeContinuesLadderInCode(t *testing.T) {
	// Install an LLM rung that falls through to "global" (retrieve) on an empty
	// result, and remove it afterwards.
	const llmRung = "test-llm-rung"
	navRuleByID[llmRung] = &navRule{
		ID:   llmRung,
		Tool: "navigate_structure",
		Mode: modeLLM,
		Next: map[string]string{StatusEmpty: "global", StatusMiss: "global"},
	}
	defer delete(navRuleByID, llmRung)

	exec := &ladderExec{emptyFor: map[string]bool{"navigate_structure": true}}
	ts := &Toolset{Exec: exec, ThinkingMode: "high"}

	// The model issues one call on the rung the chain handed back.
	st := &sessionState{
		Tools:        ts,
		DeadlineLeft: 60,
		Direction:    "who created Culdcept",
		NavRuleID:    llmRung,
		ToolCache:    NewToolCache(),
		PendingCalls: []ToolCall{
			{ID: "call-1", Name: "navigate_structure"},
		},
	}

	if err := st.toolNode(context.Background()); err != nil {
		t.Fatalf("toolNode: %v", err)
	}

	// The model's own call ran...
	if len(exec.calls) == 0 || exec.calls[0] != "navigate_structure" {
		t.Fatalf("calls = %v, want the model's navigate_structure call first", exec.calls)
	}
	// ...and the ladder then continued on its own to the "global" rung.
	var sawRetrieve bool
	for _, c := range exec.calls[1:] {
		if c == "retrieve" {
			sawRetrieve = true
		}
	}
	if !sawRetrieve {
		t.Errorf("ladder did not continue: calls = %v, want a continuation to retrieve", exec.calls)
	}
	// The ladder finished (the "global" rung has no Next), so the resting point
	// is "" — recording "" is correct and distinct from "never advanced".
	if st.NavRuleID != "" {
		t.Errorf("NavRuleID = %q, want \"\" (the ladder ran to completion)", st.NavRuleID)
	}
	// The continuation's evidence is folded into the session, not discarded.
	if len(st.RetrievedEvidenceIDs) == 0 {
		t.Error("continuation evidence was dropped from RetrievedEvidenceIDs")
	}
	// The continuation's tool message reaches the model, so it does not repeat
	// the work on its next turn.
	toolMsgs := 0
	for _, m := range st.Messages {
		if m.Role == schema.Tool {
			toolMsgs++
		}
	}
	if toolMsgs < 2 {
		t.Errorf("tool messages = %d, want >= 2 (the call plus the ladder's)", toolMsgs)
	}
}

// TestToolNodeLeavesLadderAloneWithNoPendingRule pins the default: with the
// shipped all-AUTO ladder there is nothing to continue, so no extra ladder work
// happens.
func TestToolNodeLeavesLadderAloneWithNoPendingRule(t *testing.T) {
	exec := &ladderExec{emptyFor: map[string]bool{"retrieve": true}}
	ts := &Toolset{Exec: exec, ThinkingMode: "high"}
	st := &sessionState{
		Tools:        ts,
		DeadlineLeft: 60,
		Direction:    "who created Culdcept",
		NavRuleID:    "", // ladder finished
		ToolCache:    NewToolCache(),
		PendingCalls: []ToolCall{
			{ID: "call-1", Name: "retrieve"},
		},
	}
	if err := st.toolNode(context.Background()); err != nil {
		t.Fatalf("toolNode: %v", err)
	}
	if len(exec.calls) != 1 {
		t.Errorf("calls = %v, want exactly the model's one call", exec.calls)
	}
	if st.NavRuleID != "" {
		t.Errorf("NavRuleID = %q, want \"\"", st.NavRuleID)
	}
}

// TestSessionGraphCompilesOnce pins the compile-once contract: the session graph is built
// once and every session invokes THAT graph, so repeated calls must hand back the same
// runnable instead of compiling a fresh one per session.
func TestSessionGraphCompilesOnce(t *testing.T) {
	first, err := sessionGraph()
	if err != nil {
		t.Fatalf("sessionGraph: %v", err)
	}
	second, err := sessionGraph()
	if err != nil {
		t.Fatalf("sessionGraph (second call): %v", err)
	}
	if first != second {
		t.Error("sessionGraph compiled a new graph per call; want one shared runnable")
	}
}

// TestSessionGraphHasNoCheckpointStore guards precondition 2 of the shared
// graph: with a checkpoint store the shared runner would carry per-run
// persistence state, and concurrency would become a property of that store.
// Eino rejects a CheckPointID when no store is configured, so a successful
// invoke with one means someone wired a store in.
func TestSessionGraphHasNoCheckpointStore(t *testing.T) {
	runnable, err := sessionGraph()
	if err != nil {
		t.Fatalf("sessionGraph: %v", err)
	}
	st := &sessionState{
		Tools:        &Toolset{Exec: &ladderExec{}},
		Model:        &fixedReplyModel{reply: &ModelReply{Content: "ok"}},
		DeadlineLeft: 30,
	}
	_, err = runnable.Invoke(context.Background(), st, compose.WithCheckPointID("session-1"))
	if err == nil {
		t.Fatal("session graph accepted a CheckPointID; it must stay checkpoint-free for shared concurrent use")
	}
}

// TestAppendMessagesKeepsRepeatToolCallIDs locks the reason appendMessages is a
// plain append and NOT a tool_call_id dedupe: fallback ids are assigned per turn
// (ensureToolCallIDs -> call_0, call_1…), so two turns can legitimately carry
// the same tool_call_id. Dropping the earlier response would orphan that turn's
// assistant tool_calls and get the next request rejected.
func TestAppendMessagesKeepsRepeatToolCallIDs(t *testing.T) {
	msgs := appendMessages(nil,
		*schema.ToolMessage("turn-1 answer", "call_0"),
		*schema.ToolMessage("turn-2 answer", "call_0"),
	)
	if len(msgs) != 2 {
		t.Fatalf("len = %d, want 2 — a repeated tool_call_id must not replace", len(msgs))
	}
	if msgs[0].Content != "turn-1 answer" || msgs[1].Content != "turn-2 answer" {
		t.Errorf("messages = %q / %q, want both turns kept in order", msgs[0].Content, msgs[1].Content)
	}
}

// TestSessionGraphIsSharedAcrossConcurrentSessions runs many sessions through
// the shared graph at once — RunSlotResearchPass drives sessions in parallel, so
// the runnable must be safe to invoke concurrently and must keep each session's
// state separate. Each session's model echoes its own direction, so a state that
// picks up another session's reply is detectable.
func TestSessionGraphIsSharedAcrossConcurrentSessions(t *testing.T) {
	const sessions = 16

	states := make([]*sessionState, sessions)
	for i := 0; i < sessions; i++ {
		dir := fmt.Sprintf("direction-%02d", i)
		states[i] = &sessionState{
			Direction:    dir,
			Tools:        &Toolset{Exec: &ladderExec{}},
			Model:        &fixedReplyModel{reply: &ModelReply{Content: "answer for " + dir}},
			DeadlineLeft: 60,
		}
	}

	var wg sync.WaitGroup
	errs := make([]error, sessions)
	for i := range states {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = states[i].sessionLoop(context.Background())
		}(i)
	}
	wg.Wait()

	for i, st := range states {
		if errs[i] != nil {
			t.Fatalf("session %d: %v", i, errs[i])
		}
		if st.Direction != fmt.Sprintf("direction-%02d", i) {
			t.Errorf("session %d direction = %q; another session's state leaked in", i, st.Direction)
		}
		// The session must have run: attempts incremented and its own model
		// echoed back into its own message list.
		if st.Attempts == 0 {
			t.Errorf("session %d never ran (attempts=0)", i)
		}
		want := "answer for " + st.Direction
		sawOwn := false
		for _, m := range st.Messages {
			if m.Role == schema.Assistant && m.Content == want {
				sawOwn = true
			}
			// No reply belonging to a different session may appear here.
			for j := 0; j < sessions; j++ {
				if j == i {
					continue
				}
				other := fmt.Sprintf("answer for direction-%02d", j)
				if m.Content == other {
					t.Errorf("session %d carries session %d's reply %q", i, j, other)
				}
			}
		}
		if !sawOwn {
			t.Errorf("session %d never recorded its own reply %q", i, want)
		}
	}
}

// failingModel fails every Complete call, standing in for the timeout /
// provider-exception branch of runActionNode.
type failingModel struct{ err error }

func (m *failingModel) Complete(_ context.Context, _ []schema.Message, _ []ToolSpec) (*ModelReply, error) {
	return nil, m.err
}

// TestRunActionNodeKeepsTheRecordOnAPromptFailure pins what a failed turn means: no more
// SEARCHING, not the end of the session.
//
// It used to set Done and wipe NewStates — so one provider hiccup or slow call erased every patch
// the session had written on earlier turns, and the round reported no work at all (its own comment
// described the empty Result the entry point then returned). What the session still has is its
// reserve and the answer turn that reserve exists for, so it goes there instead (see route), which
// is also what the reference loops do: the paper stops the RETRIEVAL and demands an answer
// (submit-now), WeKnora calls the model once more with ToolChoice "none".
func TestRunActionNodeKeepsTheRecordOnAPromptFailure(t *testing.T) {
	s := &sessionState{
		Messages:     []schema.Message{*schema.UserMessage("q")},
		Tools:        &Toolset{},
		Model:        &failingModel{err: errors.New("provider down")},
		DeadlineLeft: 90,
		ParentState:  State{State: []Variable{{ID: 0, Type: "aspect"}}},
		// A patch written on an earlier turn: the failed turn must not take it away.
		NewStates: []State{NewState([]Variable{{ID: 0, Type: "aspect", Candidate: strPtr("Lahore")}}, 0, nil)},
	}
	if err := s.runActionNode(context.Background()); err != nil {
		t.Fatalf("runActionNode = %v; the session must not abort the graph", err)
	}
	if s.Done {
		t.Error("Done = true; a failed turn stops the SEARCHING, it does not end the session")
	}
	if !s.ForceAnswer {
		t.Error("ForceAnswer = false; the failed turn must send the session to its answer turn")
	}
	if len(s.NewStates) != 1 {
		t.Errorf("NewStates = %d, want the patch the session had already written", len(s.NewStates))
	}
	if s.FoundAnswer != nil {
		t.Errorf("FoundAnswer = %v; a failed turn answers nothing itself", s.FoundAnswer)
	}
	if s.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1 (the failed turn still counts)", s.Attempts)
	}
	// The route must take it to the answer turn rather than end the session.
	if got := s.route(); got != routeFinalize {
		t.Fatalf("route after a failed turn = %v, want routeFinalize (the answer turn)", got)
	}
}

// TestFinalizeNodeHarvestsNarrativeWhenSalvageFails pins the finalize behaviour: a failed
// salvage call is logged and the node still runs the deterministic loose-clue harvest, so
// the last narration survives as a breadcrumb on the first unresolved slot.
func TestFinalizeNodeHarvestsNarrativeWhenSalvageFails(t *testing.T) {
	const narration = "The entity was founded in 1865 by a consortium of local merchants."
	s := &sessionState{
		Messages: []schema.Message{
			*schema.UserMessage("q"),
			*schema.AssistantMessage(narration, nil),
		},
		Tools:        &Toolset{},
		Model:        &failingModel{err: errors.New("salvage unavailable")},
		DeadlineLeft: 30,
		ParentState:  State{State: []Variable{{ID: 7, Type: "aspect"}}},
	}
	if err := s.finalizeNode(context.Background()); err != nil {
		t.Fatalf("finalizeNode = %v", err)
	}
	if !s.Done {
		t.Error("Done = false; finalize must mark the session done")
	}
	if len(s.NewStates) != 1 {
		t.Fatalf("NewStates = %d, want the narrative breadcrumb patch", len(s.NewStates))
	}
	clues := s.NewStates[0].State[0].DiscoveredClues
	if len(clues) != 1 || !strings.HasPrefix(clues[0], "narrative: ") {
		t.Errorf("discovered clues = %v, want a narrative breadcrumb", clues)
	}
}

// TestInitRetryTimeout pins the retry window: the slot table decomposition retry gets a
// longer window than the first attempt, floored at the first attempt's budget and clamped
// by the round deadline.
func TestInitRetryTimeout(t *testing.T) {
	cases := []struct {
		name         string
		first, deadl float64
		want         float64
	}{
		{"no deadline: 2x capped at 90", 45, 0, 90},
		{"deadline leaves 5s of headroom", 45, 60, 55},
		{"floor is the first attempt's budget", 45, 40, 45},
		{"2x ceiling below 90", 30, 200, 60},
		{"deadline above the ceiling", 45, 300, 90},
	}
	for _, c := range cases {
		if got := initRetryTimeout(c.first, c.deadl); got != c.want {
			t.Errorf("%s: initRetryTimeout(%v, %v) = %v, want %v", c.name, c.first, c.deadl, got, c.want)
		}
	}
}

// TestInitializeStateKeepsEmptyFirstQueries pins the first_queries handling: the first
// three entries are taken and stripped, and an entry that strips to "" is KEPT — filtering
// it made Go pick a later entry instead.
func TestInitializeStateKeepsEmptyFirstQueries(t *testing.T) {
	mdl := &fixedReplyModel{reply: &ModelReply{
		Content: `{"slots":[{"id":0,"type":"aspect","clues":["a"]}],"first_queries":["  ", "b", "c", "d"]}`,
	}}
	res := InitializeState(context.Background(), SessionDeps{Model: mdl}, "q", nil, 60)
	want := []string{"", "b", "c"}
	if len(res.FirstQueries) != len(want) {
		t.Fatalf("firstQueries = %#v, want %#v", res.FirstQueries, want)
	}
	for i := range want {
		if res.FirstQueries[i] != want[i] {
			t.Errorf("firstQueries[%d] = %q, want %q", i, res.FirstQueries[i], want[i])
		}
	}
}

// TestAsIntConvertsSlotValues pins the conversion a slot table needs: a bool is an integer,
// floats truncate toward zero, a quoted integer may carry whitespace/underscores, and
// anything that cannot be converted reports failure — which discards the whole
// decomposition in initialize_state rather than guessing an id.
func TestAsIntConvertsSlotValues(t *testing.T) {
	ok := []struct {
		in   any
		want int
	}{
		{7, 7},
		{float64(7), 7},
		{float64(1.7), 1},
		{float64(-1.7), -1},
		{true, 1},
		{false, 0},
		{"5", 5},
		{"  5  ", 5},
		{"+5", 5},
		{"1_000", 1000},
	}
	for _, c := range ok {
		got, converted := asInt(c.in)
		if !converted || got != c.want {
			t.Errorf("asInt(%#v) = %d, %v; want %d, true", c.in, got, converted, c.want)
		}
	}
	// These are hard failures, not coercions: a decimal string, a bare word, nil, and
	// non-scalar containers.
	for _, bad := range []any{"5.5", "abc", "", nil, []any{1}, map[string]any{}} {
		if _, converted := asInt(bad); converted {
			t.Errorf("asInt(%#v) reported success; it must be refused", bad)
		}
	}
}

// TestAsStringListReadsListShapes pins the coercion of a list-ish slot value: a falsy value
// yields nothing, a list yields its items as text, a string yields its characters, a map its
// keys (sorted), and a value that is not list-like is an error rather than a guess.
func TestAsStringListReadsListShapes(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want []string
		ok   bool
	}{
		{"nil is falsy", nil, nil, true},
		{"empty string is falsy", "", nil, true},
		{"zero is falsy", float64(0), nil, true},
		{"list items are stringified", []any{"a", 7}, []string{"a", "7"}, true},
		{"a string iterates its characters", "ab", []string{"a", "b"}, true},
		{"a map iterates its keys", map[string]any{"b": 1, "a": 2}, []string{"a", "b"}, true},
		{"a number is not iterable", float64(5), nil, false},
		{"true is not iterable", true, nil, false},
	}
	for _, c := range cases {
		got, converted := asStringList(c.in)
		if converted != c.ok {
			t.Errorf("%s: ok = %v, want %v", c.name, converted, c.ok)
			continue
		}
		if !converted {
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("%s: got %#v, want %#v", c.name, got, c.want)
			continue
		}
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Errorf("%s: got %#v, want %#v", c.name, got, c.want)
				break
			}
		}
	}
}

// TestInitializeStateDiscardsOnUnconvertibleValues pins the failure path: a bad id or a
// non-iterable clues list fails the parse, and the whole decomposition is dropped in favour
// of the planner fan-outs. Go signals that by returning an empty root.
func TestInitializeStateDiscardsOnUnconvertibleValues(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"non-integer id", `{"slots":[{"id":"abc","type":"aspect","clues":["a"]}]}`},
		{"decimal id", `{"slots":[{"id":"5.5","type":"aspect"}]}`},
		{"non-iterable clues", `{"slots":[{"id":0,"type":"aspect","clues":5}]}`},
		{"non-iterable first_queries", `{"slots":[{"id":0,"type":"aspect"}],"first_queries":7}`},
	}
	for _, c := range cases {
		mdl := &fixedReplyModel{reply: &ModelReply{Content: c.body}}
		res := InitializeState(context.Background(), SessionDeps{Model: mdl}, "q", nil, 60)
		if len(res.Root.State) != 0 {
			t.Errorf("%s: root has %d slot(s); the decomposition must be discarded", c.name, len(res.Root.State))
		}
	}
}

// TestInitializeStateStringifiesType pins `str(s.get("type") or "entity")`: a
// falsy type becomes "entity", any other value is stringified rather than
// rejected.
func TestInitializeStateStringifiesType(t *testing.T) {
	mdl := &fixedReplyModel{reply: &ModelReply{
		Content: `{"slots":[{"id":0,"type":7,"clues":["a"]},{"id":1,"type":"","clues":[]}]}`,
	}}
	res := InitializeState(context.Background(), SessionDeps{Model: mdl}, "q", nil, 60)
	if len(res.Root.State) != 2 {
		t.Fatalf("slots = %d, want 2", len(res.Root.State))
	}
	if got := res.Root.State[0].Type; got != "7" {
		t.Errorf("type = %q, want %q (a non-string type is stringified)", got, "7")
	}
	if got := res.Root.State[1].Type; got != "entity" {
		t.Errorf("type = %q, want %q (falsy -> entity)", got, "entity")
	}
}

// TestInitializeStateSpentBudgetTimesOutImmediately pins the deadline semantics: 0 means
// "unset", but a NEGATIVE deadline is used as-is, so the timeout fires on the next tick and
// NEITHER attempt reaches the provider — the caller then falls back to the planner
// fan-outs.
func TestInitializeStateSpentBudgetTimesOutImmediately(t *testing.T) {
	mdl := &fixedReplyModel{reply: &ModelReply{
		Content: `{"slots":[{"id":0,"type":"aspect","clues":["a"]}]}`,
	}}
	res := InitializeState(context.Background(), SessionDeps{Model: mdl}, "q", nil, -15)
	if mdl.calls != 0 {
		t.Errorf("model called %d time(s); a spent budget must not reach the provider", mdl.calls)
	}
	// No fan-out hint → the single "answer" fallback slot, not a decomposition.
	if len(res.Root.State) != 1 || res.Root.State[0].Type != "answer" {
		t.Errorf("root = %+v, want the single answer fallback slot", res.Root.State)
	}
	if len(res.Root.State[0].QuestionClues) != 1 || res.Root.State[0].QuestionClues[0] != "q" {
		t.Errorf("clues = %v, want [q]", res.Root.State[0].QuestionClues)
	}
}

// TestInitializeStateZeroDeadlineUsesFullBudget is the other half of the same
// line: an UNSET deadline (0/None) still gets the full _INIT_TIMEOUT_S budget.
func TestInitializeStateZeroDeadlineUsesFullBudget(t *testing.T) {
	mdl := &fixedReplyModel{reply: &ModelReply{
		Content: `{"slots":[{"id":0,"type":"aspect","clues":["a"]}]}`,
	}}
	res := InitializeState(context.Background(), SessionDeps{Model: mdl}, "q", nil, 0)
	if len(res.Root.State) != 1 {
		t.Fatalf("root has %d slot(s), want 1", len(res.Root.State))
	}
	if mdl.calls != 1 {
		t.Errorf("model called %d time(s), want 1", mdl.calls)
	}
}

// TestConsumeExchangeCapsPayload pins the payload cap: the in-session ladder passes its
// remaining context budget down, so the emitted tool payload is truncated; 0 keeps the
// prefix path uncapped.
func TestConsumeExchangeCapsPayload(t *testing.T) {
	big := strings.Repeat("x", 5000)
	payload := []any{map[string]any{"kind": "navigate_tree", "content": big}}

	capped := &navExchange{}
	capped.consumeExchange("r1", "navigate_tree", map[string]any{"q": 1}, ToolOutcome{Payload: payload}, "ladder", 1000)
	if got := len(capped.Messages[1].Content); got > 1000 {
		t.Errorf("tool payload = %d chars, want it capped at 1000", got)
	}

	uncapped := &navExchange{}
	uncapped.consumeExchange("r1", "navigate_tree", map[string]any{"q": 1}, ToolOutcome{Payload: payload}, "nav", 0)
	if got := len(uncapped.Messages[1].Content); got <= 1000 {
		t.Errorf("tool payload = %d chars, want 0 to mean uncapped", got)
	}
}

// TestExtractJSONLenient: fallback
// through json.loads(strict=False) and json_repair.loads: models routinely emit
// trailing commas, single-quoted strings, bare NaN/Infinity and stray control
// characters. ExtractJSON should salvage these instead of dropping the whole
// candidate and moving to the next "{".

func eqJSONMap(t *testing.T, got any, want map[string]any) {
	t.Helper()
	g, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("got %T %v, want map %v", got, got, want)
	}
	if !reflect.DeepEqual(g, want) {
		t.Fatalf("got %v, want %v", g, want)
	}
}

func TestExtractJSONLenient(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want map[string]any
	}{
		{"valid passthrough", `{"a": 1}`, map[string]any{"a": float64(1)}},
		{"trailing comma", `{"a": 1,}`, map[string]any{"a": float64(1)}},
		{"single quotes", `{'a': 'b'}`, map[string]any{"a": "b"}},
		{"single quotes and trailing comma", `{'a': 1,}`, map[string]any{"a": float64(1)}},
		{"NaN literal", `{"x": NaN}`, map[string]any{"x": nil}},
		{"Infinity literal", `{"x": Infinity}`, map[string]any{"x": nil}},
		{"negative Infinity", `{"x": -Infinity}`, map[string]any{"x": nil}},
		{"control char in string", "{\"a\": \"x\x01y\"}", map[string]any{"a": "xy"}},
		{"prefixed text", `blah {"a": 1} blah`, map[string]any{"a": float64(1)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			eqJSONMap(t, ExtractJSON(c.in), c.want)
		})
	}
}

func TestExtractJSONLenientUnrecoverable(t *testing.T) {
	// The brace-matched loop tries each "{" candidate; the deep repair fallback salvages
	// the FIRST candidate even when it is malformed (a deformed object yields null for the
	// incomplete key). It must not panic, hang, or return nil for a salvageable object.
	v := ExtractJSON(`{"a":} {"b": 2}`)
	if v == nil {
		t.Fatalf("first candidate should be salvaged, got nil")
	}
	// Empty input has nothing to salvage and must return nil (not panic).
	if v := ExtractJSON(``); v != nil {
		t.Fatalf("empty input: got %v, want nil", v)
	}
}

// TestExtractJSONWholeTextRepair repairs the WHOLE text — deformities that no single
// brace-matched candidate can fix.
func TestExtractJSONWholeTextRepair(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want map[string]any
	}{
		// Unquoted object keys.
		{"unquoted key", `{a: 1}`, map[string]any{"a": float64(1)}},
		{"unquoted keys multiple", `{name: foo, age: 3}`, map[string]any{"name": "foo", "age": float64(3)}},
		// Dropped separator between adjacent values.
		{"missing separator obj", `{"a": 1 "b": 2}`, map[string]any{"a": float64(1), "b": float64(2)}},
		{"missing separator after array", `{"a": [1,2] "b": 3}`, map[string]any{"a": []any{float64(1), float64(2)}, "b": float64(3)}},
		// Outer braces present but keys/values deformed (json_repair recovers).
		{"deformed object", `{a: 1, b: 2}`, map[string]any{"a": float64(1), "b": float64(2)}},
		// Combination: unquoted key + trailing comma + missing separator.
		{"combined deformity", `{name: foo, "age": 3 "city": bar,}`, map[string]any{"name": "foo", "age": float64(3), "city": "bar"}},
		// Prefixed prose with a deformed object.
		{"prose plus unquoted key", `Here is the answer: {entity: "x", relation: "y"}`, map[string]any{"entity": "x", "relation": "y"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			eqJSONMap(t, ExtractJSON(c.in), c.want)
		})
	}
}

// TestExtractJSONThinkFenceStillPreprocessed ensures the whole-text fallback
// still strips <think> preambles and ```json fences before deep repair.
func TestExtractJSONThinkFenceStillPreprocessed(t *testing.T) {
	in := "```json\n{name: foo, value: 1}\n```"
	eqJSONMap(t, ExtractJSON(in), map[string]any{"name": "foo", "value": float64(1)})
	think := "<think>reasoning</think>{\"a\": 1,}"
	eqJSONMap(t, ExtractJSON(think), map[string]any{"a": float64(1)})
}

// recordingInvoker captures the outgoing chat.Request so the test can assert
// what the model was actually shown.
type recordingInvoker struct{ req chat.Request }

func (r *recordingInvoker) Invoke(_ context.Context, _ *gorm.DB, req chat.Request) (*chat.Response, error) {
	r.req = req
	return &chat.Response{Content: "ok"}, nil
}

// fixedReplyModel returns a canned ModelReply so tests can drive the session
// nodes without a real model.
type fixedReplyModel struct {
	reply *ModelReply
	calls int
}

// promptRecordingModel returns a canned reply and keeps the LAST prompt it was shown, so a test can
// assert what a turn ASKED for rather than only what it did with the reply.
type promptRecordingModel struct {
	reply *ModelReply
	last  []schema.Message
	calls int
}

func (m *promptRecordingModel) Complete(_ context.Context, msgs []schema.Message, _ []ToolSpec) (*ModelReply, error) {
	m.calls++
	m.last = msgs
	return m.reply, nil
}

// TestFinalizeTurnAsksForTheAnswer pins the last turn's contract: the session is asked to ANSWER,
// and the answer it writes is what the round hands back.
//
// The turn used to ask for a state patch only ("output now: <state>{...}"), which is why a session
// that had read everything still reported no answer and the run re-composed the deliverable from a
// renderer that had never read the passages (measured 2026-09-20: 三国's record held twelve members
// with the quoted line behind each, and the answer carried citations for a third of them).
func TestFinalizeTurnAsksForTheAnswer(t *testing.T) {
	mdl := &promptRecordingModel{reply: &ModelReply{Content: "<answer>关羽杀了十二人 [ID:0]</answer>"}}
	s := &sessionState{
		Messages: []schema.Message{
			*schema.UserMessage("关羽杀了多少有姓名的人物？"),
			*schema.AssistantMessage("查得：华雄、颜良、文丑。", nil),
		},
		Tools:        &Toolset{},
		Model:        mdl,
		DeadlineLeft: 30,
		ParentState:  State{State: []Variable{{ID: 0, Type: "count"}}},
	}
	if err := s.finalizeNode(context.Background()); err != nil {
		t.Fatalf("finalizeNode = %v", err)
	}
	if s.FoundAnswer == nil || !strings.Contains(*s.FoundAnswer, "关羽杀了十二人") {
		t.Fatalf("FoundAnswer = %v, want the answer the turn wrote", s.FoundAnswer)
	}

	var joined strings.Builder
	for _, m := range mdl.last {
		joined.WriteString(m.Content)
	}
	for _, want := range []string{"<answer>", "[ID:n]", "REQUIRED"} {
		if !strings.Contains(joined.String(), want) {
			t.Errorf("the last prompt must ASK for the answer (%q missing); got:\n%s", want, joined.String())
		}
	}
}

func (m *fixedReplyModel) Complete(_ context.Context, _ []schema.Message, _ []ToolSpec) (*ModelReply, error) {
	m.calls++
	return m.reply, nil
}

func declaredTools() []ToolSpec {
	return []ToolSpec{{
		Function: toolFunction{
			Name:        "retrieve",
			Description: "run retrieval",
			Parameters:  map[string]any{"type": "object"},
		},
	}}
}

// TestNoPromptBasedToolInstruction pins the contract: there is NO prompt-based tool-call
// instruction of any kind — tools are bound natively and the reply's tool_calls are read
// directly. Such a protocol must not be sent, least of all one whose "at most one tool per
// reply" clause contradicts the native tool list.
func TestNoPromptBasedToolInstruction(t *testing.T) {
	inv := &recordingInvoker{}
	m := &InvokerSessionModel{Invoker: inv}
	if _, err := m.Complete(context.Background(),
		[]schema.Message{*schema.UserMessage("q")}, declaredTools()); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if len(inv.req.Tools) == 0 {
		t.Fatal("native tools were not declared")
	}
	// The fallback prompt must never be sent, with native tools or without.
	if _, err := m.Complete(context.Background(),
		[]schema.Message{*schema.UserMessage("q")},
		[]ToolSpec{{Function: toolFunction{Description: "unnamed"}}}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	for _, msg := range inv.req.Messages {
		if strings.Contains(msg.Content, "at most one tool per reply") {
			t.Errorf("prompt-based tool instruction sent: %q", msg.Content)
		}
	}
}

// TestAssistantMessageCarriesToolCalls pins the pairing contract: the assistant message
// carries the model's tool_calls verbatim so the tool responses that follow can be paired by
// id. Passing nil leaves a dangling tool_call, and the next request is rejected with "tool
// call result does not follow tool call".
func TestAssistantMessageCarriesToolCalls(t *testing.T) {
	st := &sessionState{
		Messages: []schema.Message{*schema.SystemMessage("s")},
		Tools:    &Toolset{Exec: &ladderExec{}},
		Model: &fixedReplyModel{reply: &ModelReply{Content: "thinking", ToolCalls: []ToolCall{
			{ID: "", Name: "retrieve", Args: map[string]any{"query": "q1"}},
			{ID: "", Name: "retrieve", Args: map[string]any{"query": "q2"}},
		}}},
	}
	if err := st.runActionNode(context.Background()); err != nil {
		t.Fatalf("runActionNode: %v", err)
	}

	var assistant *schema.Message
	for i := range st.Messages {
		if st.Messages[i].Role == schema.Assistant {
			assistant = &st.Messages[i]
		}
	}
	if assistant == nil {
		t.Fatal("no assistant message was appended")
	}
	if len(assistant.ToolCalls) != 2 {
		t.Fatalf("assistant tool_calls = %d, want 2 (pass-through, and both kept)", len(assistant.ToolCalls))
	}
	// A call_{i} id is synthesized when the provider omits one, so the tool responses have
	// ids to reference.
	if assistant.ToolCalls[0].ID != "call_0" || assistant.ToolCalls[1].ID != "call_1" {
		t.Errorf("ids = %q, %q; want call_0, call_1", assistant.ToolCalls[0].ID, assistant.ToolCalls[1].ID)
	}
	if assistant.ToolCalls[0].Function.Name != "retrieve" {
		t.Errorf("name = %q, want retrieve", assistant.ToolCalls[0].Function.Name)
	}
	// The pending calls must carry the SAME ids the assistant message declared.
	for i, c := range st.PendingCalls {
		if c.ID == "" {
			t.Errorf("pending call %d has no id; tool response cannot pair with the assistant message", i)
		}
	}
}

// TestDisabledToolIsNotUnknown pins the known-tool check: it is against the STATIC full
// set, not the active surface, so a tool this session disabled (or hid for lack of a web
// provider) is still a known tool: the model must get the "unavailable, use this instead"
// note from executeTool, not an "unknown tool" correction.
func TestDisabledToolIsNotUnknown(t *testing.T) {
	// Sanity: the name must be a real tool for the test to mean anything.
	if _, ok := toolMap["graph_explore"]; !ok {
		t.Skip("graph_explore is not a registered tool")
	}
	ts := &Toolset{ThinkingMode: "high"}
	// Disable it: it now drops out of the active surface but stays in toolMap.
	ts.DisableTool("graph_explore")
	for _, spec := range ts.ActiveToolSpecs() {
		if spec.Function.Name == "graph_explore" {
			t.Fatal("graph_explore must be absent from the active surface for this test")
		}
	}
	calls := nativeToHarnessCalls([]chat.ToolCall{{Name: "graph_explore"}})
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(calls))
	}
	if calls[0].Unknown {
		t.Error("a disabled tool must NOT be Unknown: the check judges against the static " +
			"tool map, so the call gets the 'unavailable, use this instead' note")
	}
}

// TestUnknownToolStillFlagged pins the other side — a name that is not a real tool at all
// IS flagged Unknown, so it is answered with a correction instead of being executed.
func TestUnknownToolStillFlagged(t *testing.T) {
	calls := nativeToHarnessCalls([]chat.ToolCall{{Name: "definitely_not_a_tool"}})
	if len(calls) != 1 || !calls[0].Unknown {
		t.Fatalf("calls = %+v, want one Unknown call", calls)
	}
}

// TestNativeToolCallsPreserveAll pins the execution contract: every native tool call is
// mapped, not just the first — all of them are executed.
func TestNativeToolCallsPreserveAll(t *testing.T) {
	calls := nativeToHarnessCalls([]chat.ToolCall{
		{ID: "1", Name: "retrieve"},
		{ID: "2", Name: "retrieve"},
		{ID: "3", Name: "bogus"},
	})
	if len(calls) != 3 {
		t.Fatalf("mapped %d calls, want 3 (all of them, not just the first)", len(calls))
	}
	if !calls[2].Unknown {
		t.Error("an undeclared name must be flagged Unknown")
	}
}

// streamingInvoker streams a fixed set of pieces.
type streamingInvoker struct {
	pieces []piece
	err    error
}

type piece struct {
	text    string
	isThink bool
}

func (s *streamingInvoker) Invoke(context.Context, *gorm.DB, chat.Request) (*chat.Response, error) {
	return &chat.Response{Content: "one-shot"}, nil
}

func (s *streamingInvoker) Stream(_ context.Context, _ *gorm.DB, _ chat.Request, onDelta func(string, bool) error) (*chat.Response, error) {
	if s.err != nil {
		return nil, s.err
	}
	var content, reasoning string
	for _, p := range s.pieces {
		if onDelta != nil {
			if err := onDelta(p.text, p.isThink); err != nil {
				return nil, err
			}
		}
		if p.isThink {
			reasoning += p.text
		} else {
			content += p.text
		}
	}
	return &chat.Response{Content: content, Thinking: reasoning}, nil
}

// plainInvoker only implements the non-streaming seam.
type plainInvoker struct{}

func (p *plainInvoker) Invoke(context.Context, *gorm.DB, chat.Request) (*chat.Response, error) {
	return &chat.Response{Content: "one-shot"}, nil
}

func TestStreamCompleteForwardsDeltas(t *testing.T) {
	m := &InvokerSessionModel{Invoker: &streamingInvoker{pieces: []piece{
		{text: "think", isThink: true},
		{text: "Hello ", isThink: false},
		{text: "world", isThink: false},
	}}}

	var got []piece
	reply, err := m.StreamComplete(context.Background(), []schema.Message{*schema.UserMessage("q")}, nil,
		func(delta string, isThink bool) error {
			got = append(got, piece{text: delta, isThink: isThink})
			return nil
		})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply.Content != "Hello world" {
		t.Fatalf("Content = %q, want %q", reply.Content, "Hello world")
	}
	if len(got) != 3 || !got[0].isThink || got[0].text != "think" {
		t.Fatalf("deltas = %+v, want reasoning flagged first", got)
	}
}

func TestStreamCompleteFailsWithoutStreamingInvoker(t *testing.T) {
	// A non-streaming invoker must report failure so the caller falls back to
	// the one-shot call rather than silently producing nothing.
	m := &InvokerSessionModel{Invoker: &plainInvoker{}}

	if _, err := m.StreamComplete(context.Background(), nil, nil, nil); err == nil {
		t.Fatal("want an error when the invoker cannot stream")
	}
}

func TestStreamCompletePropagatesError(t *testing.T) {
	m := &InvokerSessionModel{Invoker: &streamingInvoker{err: errors.New("provider down")}}

	if _, err := m.StreamComplete(context.Background(), nil, nil, nil); err == nil {
		t.Fatal("want the provider error")
	}
}

func TestStreamCompleteRequiresInvoker(t *testing.T) {
	m := &InvokerSessionModel{}

	if _, err := m.StreamComplete(context.Background(), nil, nil, nil); err == nil {
		t.Fatal("want an error when no invoker is configured")
	}
}

type capturingInvoker struct {
	lastReq *chat.Request
	hasTool bool
	tool    chat.ToolCall
	content string
}

func (c *capturingInvoker) Invoke(_ context.Context, _ *gorm.DB, req chat.Request) (*chat.Response, error) {
	c.lastReq = &req
	resp := &chat.Response{Content: c.content}
	if c.hasTool {
		resp.ToolCalls = []chat.ToolCall{c.tool}
	}
	return resp, nil
}

func TestCompleteForwardsNativeToolsAndPrefersNativeCalls(t *testing.T) {
	tools := []ToolSpec{{
		Type: "function",
		Function: toolFunction{
			Name:        "search",
			Description: "run a search",
			Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		},
	}}
	inv := &capturingInvoker{hasTool: true, tool: chat.ToolCall{ID: "c1", Name: "search", Arguments: map[string]any{"q": "x"}}, content: "ok"}
	m := &InvokerSessionModel{Invoker: inv}

	reply, err := m.Complete(context.Background(), []schema.Message{*schema.UserMessage("q")}, tools)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inv.lastReq == nil || len(inv.lastReq.Tools) != 1 {
		t.Fatalf("tools not forwarded to invoker: req=%+v", inv.lastReq)
	}
	if inv.lastReq.Tools[0].Function.Name != "search" {
		t.Fatalf("forwarded tool name = %q, want search", inv.lastReq.Tools[0].Function.Name)
	}
	if inv.lastReq.ToolChoice != chat.ToolChoiceAuto {
		t.Fatalf("tool choice = %q, want auto", inv.lastReq.ToolChoice)
	}
	if len(reply.ToolCalls) != 1 || reply.ToolCalls[0].Name != "search" || reply.ToolCalls[0].Args["q"] != "x" {
		t.Fatalf("native tool call not preferred: %+v", reply.ToolCalls)
	}
}

func TestCompleteReportsNoCallWhenModelSendsNoNativeCall(t *testing.T) {
	inv := &capturingInvoker{hasTool: false, content: ""}
	m := &InvokerSessionModel{Invoker: inv}
	tools := []ToolSpec{{
		Type:     "function",
		Function: toolFunction{Name: "search", Description: "d", Parameters: map[string]any{}},
	}}

	// No native tool_calls => the reply stays tool-less. This confirms the runtime still
	// declares tools on the request (verified above) but does not hallucinate a call: there
	// is no other parsing path.
	reply, err := m.Complete(context.Background(), []schema.Message{*schema.UserMessage("q")}, tools)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reply.ToolCalls) != 0 {
		t.Fatalf("expected no tool calls, got %+v", reply.ToolCalls)
	}
}

// streamingCapturingInvoker implements chat.StreamingInvoker and records the
// streaming request, returning a native tool call so we can verify the runtime
// reads streamed calls out of the native tool_calls field.
type streamingCapturingInvoker struct {
	capturingInvoker
	onDelta func(delta string, isThink bool) error
}

func (s *streamingCapturingInvoker) Stream(_ context.Context, _ *gorm.DB, req chat.Request, onDelta func(delta string, isThink bool) error) (*chat.Response, error) {
	s.lastReq = &req
	if onDelta != nil {
		_ = onDelta("partial", false)
	}
	resp := &chat.Response{Content: "streamed"}
	if s.hasTool {
		resp.ToolCalls = []chat.ToolCall{s.tool}
	}
	return resp, nil
}

func TestStreamCompleteForwardsNativeToolsAndPrefersNativeCalls(t *testing.T) {
	tools := []ToolSpec{{
		Type: "function",
		Function: toolFunction{
			Name:        "search",
			Description: "run a search",
			Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		},
	}}
	inv := &streamingCapturingInvoker{
		capturingInvoker: capturingInvoker{
			hasTool: true,
			tool:    chat.ToolCall{ID: "c2", Name: "search", Arguments: map[string]any{"q": "y"}},
		},
	}
	m := &InvokerSessionModel{Invoker: inv}

	reply, err := m.StreamComplete(context.Background(), []schema.Message{*schema.UserMessage("q")}, tools, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inv.lastReq == nil || len(inv.lastReq.Tools) != 1 || inv.lastReq.Tools[0].Function.Name != "search" {
		t.Fatalf("streaming tools not forwarded: req=%+v", inv.lastReq)
	}
	if inv.lastReq.ToolChoice != chat.ToolChoiceAuto {
		t.Fatalf("streaming tool choice = %q, want auto", inv.lastReq.ToolChoice)
	}
	if len(reply.ToolCalls) != 1 || reply.ToolCalls[0].Name != "search" || reply.ToolCalls[0].Args["q"] != "y" {
		t.Fatalf("streamed native tool call not preferred: %+v", reply.ToolCalls)
	}
}

func TestStreamCompleteIgnoresFencedBlockInContent(t *testing.T) {
	tools := []ToolSpec{{
		Type:     "function",
		Function: toolFunction{Name: "search", Description: "d", Parameters: map[string]any{}},
	}}
	// A fenced block in the content is NOT a tool call: only the reply's tool_calls are
	// read, and there is no prompt-based parsing path.
	inv := &streamingCapturingInvoker{
		capturingInvoker: capturingInvoker{
			hasTool: false,
			content: "```json\n{\"tool\": \"search\", \"args\": {\"q\": \"ignored\"}}\n```",
		},
	}
	m := &InvokerSessionModel{Invoker: inv}

	reply, err := m.StreamComplete(context.Background(), []schema.Message{*schema.UserMessage("q")}, tools, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reply.ToolCalls) != 0 {
		t.Fatalf("fenced block must not be parsed as a tool call: %+v", reply.ToolCalls)
	}
}

func TestRenderPromptUsesLoader(t *testing.T) {
	loader := stringPromptLoader{"tpl": "hello {{ name }}!"}
	// Loader resolves the name: variable substituted.
	if got := prompts.Render(loader, "tpl", "", map[string]string{"name": "world"}); got != "hello world!" {
		t.Errorf("prompts.Render = %q", got)
	}
	// Unknown template with empty fallback -> empty (there is no degraded constant; a
	// missing template fails fast rather than degrading).
	if got := prompts.Render(loader, "missing", "", nil); got != "" {
		t.Errorf("unknown template with empty fallback = %q", got)
	}
	// Nil loader + a known embedded name still resolves the canonical .md. (sca_select was the
	// template here until the sufficiency review was removed, then sca_query_rewrite until the
	// rewriter was; the ACTION prompts are the surviving embedded set. action_set carries no
	// placeholders, so the assertion is that it renders — not that a variable came back.)
	if got := prompts.Render(nil, "action_set", "", nil); got == "" {
		t.Error("nil loader did not resolve the embedded action_set template")
	}
	// Both {{k}} and {{ k }} are substituted (templates use the spaced form).
	if got := prompts.Render(stringPromptLoader{"tpl": "a={{x}} b={{ y }}"}, "tpl", "",
		map[string]string{"x": "1", "y": "2"}); got != "a=1 b=2" {
		t.Errorf("spaced placeholder not substituted: %q", got)
	}
}

// Helpers

// Tool spec / schema contract (moved here from action_session_schema_test.go)

// playbookAnchors are the 5-section contract every tool description must carry.
var playbookAnchors = []string{"WHEN TO CALL", "DO NOT CALL", "ARGUMENTS", "OUTPUT", "IF IT FAILS"}

// executorSupportedParams lists the params the executor actually consumes per
// tool (mirrors _EXECUTOR_SUPPORTED). The schema MUST NOT declare any param
// outside this set — otherwise the model is told to fill an argument the runtime
// silently ignores (the list_chunks(chunk_ids) ghost bug). Note: keywords are
// supported by the executor but intentionally NOT declared in the schema (keep
// the description honest with the implementation); doc_scope IS declared on the
// two tools that consume it (retrieve, graph_explore).
var executorSupportedParams = map[string]map[string]bool{
	"retrieve":           {"query": true, "doc_scope": true, "reason": true},
	"metadata_search":    {"filters": true, "logic": true},
	"search_chunks":      {"query": true, "reason": true},
	"list_chunks":        {"doc_id": true, "offset": true},
	"navigate_tree":      {"query": true, "keywords": true},
	"navigate_structure": {"doc_id": true, "query": true, "kind": true},
	"calculate":          {"question": true, "facts": true},
	"web_search":         {"query": true},
}

// TestToolSpecsHavePlaybookSections pins the PR: every tool documents the
// 5-section contract, within the token budget (cap 1200 chars).
func TestToolSpecsHavePlaybookSections(t *testing.T) {
	for _, name := range allTools {
		desc := toolMap[name].Function.Description
		for _, anchor := range playbookAnchors {
			if !strings.Contains(desc, anchor) {
				t.Errorf("%s description missing anchor %q", name, anchor)
			}
		}
		if n := utf8.RuneCountInString(desc); n > 1200 {
			t.Errorf("%s description too long: %d chars (cap 1200)", name, n)
		}
	}
}

// TestActiveToolSpecsToolSurface pins mode -> exposed tool count: low=0,
// medium/high=8, ultra=9, web-hidden=7.
func TestActiveToolSpecsToolSurface(t *testing.T) {
	cases := []struct {
		mode string
		web  bool
		want int
	}{
		{"low", true, 0},
		{"medium", true, 8},
		{"high", true, 8},
		{"ultra", true, 9},
		{"medium", false, 7},
	}
	for _, c := range cases {
		ts := &Toolset{ThinkingMode: c.mode, HasWebSearch: c.web}
		if got := len(ts.ActiveToolSpecs()); got != c.want {
			t.Errorf("mode=%s web=%t: tools = %d, want %d", c.mode, c.web, got, c.want)
		}
	}
}

// TestSchemaParamsMatchExecutor pins that no schema declares a param the
// executor cannot consume (no ghost args).
func TestSchemaParamsMatchExecutor(t *testing.T) {
	for _, name := range allTools {
		props, _ := toolMap[name].Function.Parameters["properties"].(map[string]any)
		supported := executorSupportedParams[name]
		for param := range props {
			if param == "decision" {
				continue
			}
			if !supported[param] {
				t.Errorf("%s declares unsupported param %q", name, param)
			}
		}
	}
}

// TestActionRunPromptHasPlaybook pins that action_run.md exposes the TOOL
// PLAYBOOK and only references real tools.
func TestActionRunPromptHasPlaybook(t *testing.T) {
	prompt := loadPrompt(nil, "action_run")
	if !strings.Contains(prompt, "TOOL PLAYBOOK") {
		t.Error("action_run prompt missing TOOL PLAYBOOK")
	}
	for _, tool := range []string{"navigate_structure", "calculate", "graph_explore"} {
		if !strings.Contains(prompt, tool) {
			t.Errorf("action_run prompt missing tool %q", tool)
		}
	}
	for _, name := range append(append([]string{}, allTools...), graphExploreTool) {
		if _, ok := toolMap[name]; !ok {
			t.Errorf("%q referenced by the playbook is not in ToolMap", name)
		}
	}
}

// TestParseTerminalEmptyAnswerNotFound pins the empty-answer rule: an empty or whitespace
// <answer> payload is NOT a found answer. The session then only ends when the <answer>
// carried a new_state patch; with neither an answer nor a patch it NUDGES and continues, so
// a bare <answer> block can never end a session with an empty collected answer..
func TestParseTerminalEmptyAnswerNotFound(t *testing.T) {
	parent := NewState([]Variable{{ID: 0, Type: "answer"}}, 0, nil)

	// Whitespace answer, no patch: no found answer, no branches — the session
	// keeps running (runActionNode falls through to the nudge).
	states, found, tt, _ := parseTerminal("<answer>{\"answer\": \"   \"}</answer>", parent)
	if found != nil {
		t.Errorf("whitespace answer: FoundAnswer = %q, want nil", *found)
	}
	if len(states) != 0 {
		t.Errorf("whitespace answer: %d branch(es), want 0", len(states))
	}
	if tt == nil || *tt != "answer" {
		t.Errorf("whitespace answer: terminal type = %v, want answer", tt)
	}

	// Missing answer field, no patch: same.
	states, found, _, _ = parseTerminal("<answer>{}</answer>", parent)
	if found != nil || len(states) != 0 {
		t.Errorf("missing answer: FoundAnswer=%v branches=%d, want nil/0", found, len(states))
	}

	// A real answer still parses.
	_, found, _, _ = parseTerminal("<answer>{\"answer\": \"  74  \"}</answer>", parent)
	if found == nil || *found != "74" {
		t.Errorf("real answer: FoundAnswer = %v, want 74 (stripped)", found)
	}

	// Empty answer WITH a new_state patch: the patch is returned as the final state,
	// terminal type stays "answer", found stays nil.
	states, found, _, _ = parseTerminal(
		"<answer>{\"answer\": \"\", \"new_state\": [{\"id\": 0, \"candidate\": \"74\", \"candidate_strength\": 0.9}]}</answer>",
		parent)
	if found != nil {
		t.Errorf("empty answer with patch: FoundAnswer = %q, want nil", *found)
	}
	if len(states) != 1 || states[0].State[0].Candidate == nil || *states[0].State[0].Candidate != "74" {
		t.Errorf("empty answer with patch: branches = %+v, want one state with candidate 74", states)
	}
}

// TestParseTerminalReadsTheAnswerWrittenBesideTheStatePatch pins the shape the finalize turn
// ASKS FOR, and the reason it used to lose the answer.
//
// finalizeNode's salvage prompt ends with "Format: <state>{...}</state> followed by
// <answer>your answer</answer> ... An answer is REQUIRED even when the state block is empty."
// The parser used to look for <state> first and RETURN, so every session that obeyed the
// protocol was recorded as having produced no answer: the round reported
// collected_answer=false, and the run's real answer — written against passages the model had
// read — was replaced by a composition call that had read none of them (measured 2026-09-20:
// 7 of 21 rounds, 6 of them through the salvage path that shares this parser).
//
// A state block is bookkeeping and is applied either way; it cannot suppress the answer beside
// it. Both blocks are read, in whichever order the model wrote them.
func TestParseTerminalReadsTheAnswerWrittenBesideTheStatePatch(t *testing.T) {
	parent := NewState([]Variable{{ID: 0, Type: "answer"}}, 0, nil)

	// The finalize shape, state first: both the patch and the answer come back.
	reply := `<state>{"new_states": [{"state": [{"id": 0, "candidate": "关羽", "candidate_strength": 0.9}]}]}</state>` +
		"\n<answer>十二人：关羽 [ID:0]</answer>"
	states, found, tt, payload := parseTerminal(reply, parent)
	if found == nil || *found != "十二人：关羽 [ID:0]" {
		t.Fatalf("FoundAnswer = %v, want the prose answer beside the patch", found)
	}
	if tt == nil || *tt != "answer" {
		t.Errorf("terminal type = %v, want answer (the answer is what decides the round)", tt)
	}
	if len(states) != 1 || states[0].State[0].Candidate == nil || *states[0].State[0].Candidate != "关羽" {
		t.Errorf("branches = %+v, want the patch applied as well (bookkeeping is not dropped)", states)
	}
	if payload == nil {
		t.Error("payload = nil, want the answer block's data")
	}

	// Same two blocks, answer written first: order in the reply must not matter.
	states, found, tt, _ = parseTerminal(
		"<answer>关羽 [ID:0]</answer>\n<state>{\"new_states\": []}</state>", parent)
	if found == nil || *found != "关羽 [ID:0]" || tt == nil || *tt != "answer" {
		t.Errorf("answer-first reply: found=%v type=%v, want the answer", found, tt)
	}
	if len(states) != 0 {
		t.Errorf("empty new_states: %d branch(es), want 0", len(states))
	}

	// A state block ALONE still reports a state terminal and no answer (unchanged).
	_, found, tt, _ = parseTerminal(`<state>{"new_states": []}</state>`, parent)
	if found != nil || tt == nil || *tt != "state" {
		t.Errorf("state-only reply: found=%v type=%v, want nil/state", found, tt)
	}

	// An answer alone is unchanged.
	_, found, tt, _ = parseTerminal("<answer>关羽</answer>", parent)
	if found == nil || *found != "关羽" || tt == nil || *tt != "answer" {
		t.Errorf("answer-only reply: found=%v type=%v, want 关羽/answer", found, tt)
	}

	// No terminal at all stays "nothing" — the session nudges and keeps working.
	states, found, tt, _ = parseTerminal("still thinking about it", parent)
	if states != nil || found != nil || tt != nil {
		t.Errorf("no terminal: states=%v found=%v type=%v, want all nil", states, found, tt)
	}
}

// TestSnippetsPerQueryRisesWithModeAndFallsBack pins the per-query snippet cap.
//
// The cap decides how much of ONE query's candidate list the session reads, and
// that list is where a themed query's fact-bearing passage sits: the engine
// returns 30-60 candidates per leg, and on a 64-candidate leg the passage that
// carried the answer ranked 20th and 38th — both discarded by a flat cap of four,
// while handing the model those same passages produced the complete list.
//
// A deeper mode issues more queries and owns a larger budget, so it reads deeper;
// a mode that declares no cap keeps the flat fallback, which is what every mode
// did before this existed. Nothing here depends on a corpus or a language.
func TestSnippetsPerQueryRisesWithModeAndFallsBack(t *testing.T) {
	for _, mode := range []string{"medium", "high", "ultra"} {
		spec := GetMode(mode)
		if spec.SnippetsPerQuery <= snippetsPerQuery {
			t.Errorf("%s cap = %d, want it above the flat fallback %d", mode, spec.SnippetsPerQuery, snippetsPerQuery)
		}
		if got := snippetsPerQueryFor(mode); got != spec.SnippetsPerQuery {
			t.Errorf("snippetsPerQueryFor(%s) = %d, want %d", mode, got, spec.SnippetsPerQuery)
		}
	}
	if GetMode("medium").SnippetsPerQuery >= GetMode("high").SnippetsPerQuery ||
		GetMode("high").SnippetsPerQuery >= GetMode("ultra").SnippetsPerQuery {
		t.Error("the per-query cap must rise with the mode")
	}
	// Unset (low, or an unknown label) falls back to the flat cap.
	for _, mode := range []string{"low", "no-such-mode"} {
		if got := snippetsPerQueryFor(mode); got != snippetsPerQuery {
			t.Errorf("snippetsPerQueryFor(%q) = %d, want the fallback %d", mode, got, snippetsPerQuery)
		}
	}
}

// TestRetrieveDescriptionCarriesTheEnumerationContract pins the recall path in
// the tool description the model reads while deciding how to call it.
//
// Measured (2026-09-16): the same corpus and question answered sixteen members
// with this contract present and nine-to-fourteen without it, while the runtime
// underneath was identical — the seats, the batch weaving and the reach notes
// exist in BOTH trees. What the description carries is how to ASK: probe the
// names themselves in an alternation, in small batches, guess the next batch
// yourself, and read a miss as a RESULT rather than as silence.
func TestRetrieveDescriptionCarriesTheEnumerationContract(t *testing.T) {
	desc := toolMap["retrieve"].Function.Description
	for _, want := range []string{
		"ENUMERATING A SET",
		"alternated with |",
		"4-6 per query",
		"guess the next batch yourself",
		"in an enumeration that is a RESULT",
	} {
		if !strings.Contains(desc, want) {
			t.Errorf("retrieve description is missing %q", want)
		}
	}
	// It must not send the opposite instruction. "Do not conclude the corpus lacks
	// the fact" is right for a fuzzy query and wrong for an exact-term probe: a
	// model told that stops probing names and settles for the ones it already
	// holds, which is the shape of the nine-member answer.
	if strings.Contains(desc, "do not conclude the corpus lacks the fact") {
		t.Error("retrieve description must not contradict the enumeration semantics of a miss")
	}
}

// TestParseUnresolvedReadsOnlyTheNamedBlock pins the signal the router reopens a round on.
//
// "Did the session answer?" and "is the answer finished?" are two facts, and the loop only had the
// first. The second is taken from a block the model must write itself, because prose cannot be
// tested for meaning: a part named as still open reopens the question (see routeResearch), an empty
// block ends it, and no block at all means the answer is final.
func TestParseUnresolvedReadsOnlyTheNamedBlock(t *testing.T) {
	cases := []struct {
		name  string
		reply string
		want  string
	}{
		{"named part",
			"<answer>The writer is Tow Ubukata [ID:0]</answer>\n<unresolved>the population figure for Gifu Prefecture</unresolved>",
			"the population figure for Gifu Prefecture"},
		{"empty block", "<answer>a [ID:0]</answer><unresolved></unresolved>", ""},
		{"whitespace block", "<answer>a</answer><unresolved>   \n  </unresolved>", ""},
		{"no block", "<answer>a</answer>", ""},
		{"prose alone", "I could not find the population anywhere.", ""},
		{"trimmed", "<unresolved>  two open parts  </unresolved>", "two open parts"},
		{"block before the answer", "<unresolved>Gifu's population</unresolved><answer>a</answer>", "Gifu's population"},
	}
	for _, tc := range cases {
		if got := ParseUnresolved(tc.reply); got != tc.want {
			t.Errorf("%s: ParseUnresolved = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestTheSessionsClockAndToolBudgetEndBeforeItsContext pins the guard that keeps the answer call
// from losing its race with the context deadline.
//
// The session's clock and its context deadline used to be the same number, so every budget derived
// from the clock could consume exactly up to the wall: a tool call or the answer call would return
// at the boundary, the context would fire, and the round was reported as cut with the answer it had
// just written never parsed (measured 2026-09-20, 20:39: 4 of 20 rounds, one right after
// `turn timed out after 62s; … asking for the answer` had already routed it to the answer turn).
func TestTheSessionsClockAndToolBudgetEndBeforeItsContext(t *testing.T) {
	// The clock is strictly inside the budget the context is armed with...
	for _, budget := range []float64{180, 60, 25} {
		if got := sessionClockFor(budget); got >= budget {
			t.Errorf("sessionClockFor(%v) = %v, want it inside the context deadline", budget, got)
		}
	}
	// ... and never negative, however small the budget.
	if got := sessionClockFor(1); got != minSessionClockS {
		t.Errorf("sessionClockFor(1) = %v, want the floor %v", got, minSessionClockS)
	}
	// A tool call may spend the SEARCHING clock and never the answering reserve.
	if got := toolWallS(100).Seconds(); got > 100-answerReserveS {
		t.Errorf("tool wall = %vs for a 100s clock, want at most %vs (the reserve is the answer's)",
			got, 100-answerReserveS)
	}
	if got := toolWallS(1).Seconds(); got < 5 {
		t.Errorf("tool wall = %vs with almost no clock, want the 5s floor", got)
	}
}

// TestSessionResultCarriesWhatACutSessionRead pins the mapping BOTH session exits use.
//
// A session cut by its clock used to return an empty Result, which threw away every state patch it
// had written, every passage it had read, and its answer if it had one: 5 of 19 sessions in one
// FRAMES run (measured 2026-09-20, 20:15) were cut, and each of those rounds then had no answer AND
// a thin citation registry, so the composition that replaced it answered from whatever ranked
// first. A cut is how a session that spends its clock ends; it is not a reason to discard research.
func TestSessionResultCarriesWhatACutSessionRead(t *testing.T) {
	st := &sessionState{
		Messages:     []schema.Message{*schema.UserMessage("q")},
		NewStates:    []State{NewState([]Variable{{ID: 0, Type: "date", Candidate: strPtr("1858")}}, 0, nil)},
		FoundAnswer:  strPtr("1858"),
		EvidenceRefs: []string{"c1", "c2"},
		Unresolved:   "  the Crown's takeover year  ",
		TerminalType: strPtr("answer"),
	}
	res := sessionResult(st)
	if len(res.NewStates) != 1 {
		t.Errorf("NewStates = %d, want the patch the session wrote before the cut", len(res.NewStates))
	}
	if res.FoundAnswer == nil || *res.FoundAnswer != "1858" {
		t.Errorf("FoundAnswer = %v, want the answer it had reached", res.FoundAnswer)
	}
	if len(res.EvidenceRefs) != 2 {
		t.Errorf("EvidenceRefs = %v, want the passages it read", res.EvidenceRefs)
	}
	if res.Unresolved != "the Crown's takeover year" {
		t.Errorf("Unresolved = %q, want the trimmed open part", res.Unresolved)
	}
	if res.TerminalType == nil || *res.TerminalType != "answer" {
		t.Errorf("TerminalType = %v, want the terminal it reached", res.TerminalType)
	}
	// The registry is copied, not aliased: nothing that happens to the session afterwards may
	// rewrite what the round was handed.
	st.EvidenceRefs[0] = "mutated"
	if res.EvidenceRefs[0] != "c1" {
		t.Error("EvidenceRefs is aliased to the session's own slice")
	}
}

// TestEverySessionGetsTheModesTurnFloor pins the removal of the shape gate on the turn floor.
//
// The floor used to be halved for a session that was not "enumerating" (a caller-written batch of
// terms in one query), on the argument that a question which is not assembling a set has nothing
// to spend those turns on. The questions that come back empty are multi-hop VALUE questions, and
// their sessions were stopped by their TURN count while their clock was still full (measured
// 2026-09-20: "turn floor 8 → 4" 15 times in one 20-question FRAMES run, 8 rounds finalized by the
// salvage prompt, and "167 seconds of research budget left" beside "TOOL BUDGET EXHAUSTED"; the
// four questions that stayed at zero — Quincy's mayors, Gifu's population, AP's law, Lahore in
// 1858 — each hit that wall). The constraint that binds is the clock, so every session runs on the
// mode's own count and the run cap stays the only hard turn ceiling.
func TestEverySessionGetsTheModesTurnFloor(t *testing.T) {
	ts := &Toolset{ThinkingMode: "high"}
	modeFloor := ResolveMode(ts).ActionMaxTurns
	if modeFloor <= valueTurnFloor {
		t.Fatalf("mode high ActionMaxTurns = %d, want > %d (so the test can see the difference)",
			modeFloor, valueTurnFloor)
	}

	// A value session: no batch written, and the table asks for a value. Same floor as a set.
	value := &sessionState{
		Tools:        ts,
		Direction:    "in what year did the Sikh Empire's capital come under the British Crown",
		ParentState:  State{State: []Variable{{ID: 0, Type: "date", Candidate: strPtr("1849")}}},
		DeadlineLeft: 70,
	}
	if got := value.actionMaxTurns(); got != modeFloor {
		t.Errorf("value session turn floor = %d, want the mode's %d", got, modeFloor)
	}

	// A session that wrote a batch pays the same floor (the enumeration extras are elsewhere: the
	// SET method and the pool excerpt, both still gated).
	value.SearchQueries = []string{"华雄|颜良|文丑"}
	if got := value.actionMaxTurns(); got != modeFloor {
		t.Errorf("session that wrote a batch turn floor = %d, want %d", got, modeFloor)
	}

	// A mode that declares no count at all still falls back to the value floor.
	blank := &sessionState{Direction: "q", DeadlineLeft: 70}
	if got := blank.actionMaxTurns(); got != valueTurnFloor {
		t.Errorf("mode with no count turn floor = %d, want the fallback %d", got, valueTurnFloor)
	}
}

// TestSessionWallFollowsTheEnumeration pins the clock an enumeration session gets.
//
// The wall clock is spent differently by the two strategies: an enumeration session is
// midway through a batch when its clock runs out, and a batch that is cut loses the
// members it had already reached — measured (2026-09-16, 三国/关羽) a session was
// cancelled one millisecond before the patch that recorded 管亥, a member it had
// already found and whose passages were already in the shared pool. A value session
// has no work in flight at its deadline, so it keeps the tighter clock.
//
// What follows from that measurement is ONE clock, not two: the reason a set direction wanted
// more time is a fact about the session's WORK in flight, and no reading of the table can decide
// it (see the note where SessionWallS used to live). The session's own batch — the signal that
// survives, see wroteBatch — extends the session's protocol, and the budget is the caller's.

// TestDigestShowsAPassageWholeEnoughToNameSomeone pins the seed digest's per-chunk
// cap.
//
// The digest is how a session SEES the pool without re-querying it, and its cap used to
// be a hard 300-code-point cut. A 300-code-point window is shorter than the sentence a deed
// is reported in: the passage names the actor early and the person at the end, so the cut
// removed exactly the clause that makes a member a member — and a model told to answer only
// from what it was shown then excluded them.
//
// Measured (2026-09-14, fixrecall): with the cap at 1200 an enumeration reached
// seventeen members. Measured here (2026-09-16, 三国/关羽): 管亥 / 杨龄 / 程远志
// appeared ZERO times in whole runs whose pool held their passages.
func TestDigestShowsAPassageWholeEnoughToNameSomeone(t *testing.T) {
	narrative := strings.Repeat("却说曹兵势大", 60) + "云长手起刀落，砍杨龄于马下。"
	kb := &Kbinfos{}
	kb.Admit(func(p *PoolAdmitter) {
		p.Add(map[string]any{"chunk_id": "c1", "content": narrative})
	})

	digest := extractRelevantEvidence(kb, "关羽杀了多少有姓名的人物", 4)
	if !strings.Contains(digest, "砍杨龄于马下") {
		t.Fatalf("digest = %q…, want the clause that names the member (cap %d)", TruncateRunes(digest, 120),
			stageBudgets[stageDigest].MaxCharsPerItem)
	}
	if got := len([]rune(digest)); got > stageBudgets[stageDigest].MaxCharsPerItem+64 {
		t.Errorf("digest = %d runes, want no more than one capped chunk plus its id", got)
	}
	if stageBudgets[stageDigest].MaxCharsPerItem < 1200 {
		t.Errorf("digest cap = %d, want at least the 1200 an admitted passage carries",
			stageBudgets[stageDigest].MaxCharsPerItem)
	}
}

// TestReadPassagesAreMarkedReadAndSearchedOnesPreview pins the read/preview distinction: the same
// passage is "read" once a document page delivered it, and "preview" while all the run has is a
// search snippet.
//
// The two arrive in one shape, and a model that cannot tell them apart answers from a ranked guess
// as if it were the document's text. Nothing else in the run knows the difference once both are in
// the pool, so the label is attached where the result is rendered.
func TestReadPassagesAreMarkedReadAndSearchedOnesPreview(t *testing.T) {
	kb := &Kbinfos{}
	kb.NoteChunksRead("doc-a", 0, []string{"c-read"}, true)

	chunks := []any{
		map[string]any{"id": "c-read", "content": "the page"},
		map[string]any{"id": "c-only-previewed", "content": "a snippet"},
	}
	markReadState(kb, chunks)

	if got := chunks[0].(map[string]any)["seen"]; got != "read" {
		t.Errorf("a chunk delivered by list_chunks is marked %v, want read", got)
	}
	if got := chunks[1].(map[string]any)["seen"]; got != "preview" {
		t.Errorf("a chunk only ever searched is marked %v, want preview", got)
	}
}

// TestTheReadLedgerCountsDistinctChunksPerDocument pins what the ledger's numbers mean: PAGES is
// how many list_chunks calls a document was read in, CHUNKS is how many DISTINCT chunks those pages
// delivered, and the last page's offset and "continues" flag are what a session needs to know
// whether reading on is worth a call.
//
// An overlapping page (the model re-reading from an earlier offset) must not inflate the chunk
// count: the number says how much of the document the run actually has.
func TestTheReadLedgerCountsDistinctChunksPerDocument(t *testing.T) {
	kb := &Kbinfos{}
	kb.NoteChunksRead("doc-a", 0, []string{"c1", "c2"}, true)
	kb.NoteChunksRead("doc-a", 30, []string{"c2", "c3"}, false)

	progress := kb.ReadProgress()
	if len(progress) != 1 {
		t.Fatalf("ReadProgress = %v, want one line", progress)
	}
	line := progress[0]
	for _, want := range []string{"doc-a", "2 page(s)", "3 chunk(s) read", "offset 30"} {
		if !strings.Contains(line, want) {
			t.Errorf("ReadProgress line = %q, want %q", line, want)
		}
	}
	if strings.Contains(line, "more behind") {
		t.Errorf("ReadProgress line = %q, want no 'more behind' after the page that ended the document", line)
	}
	if kb.WasRead("c1") != true || kb.WasRead("c3") != true || kb.WasRead("c9") != false {
		t.Errorf("WasRead = %v/%v/%v, want true/true/false", kb.WasRead("c1"), kb.WasRead("c3"), kb.WasRead("c9"))
	}
	if kb.ReadProgress() == nil || (&Kbinfos{}).ReadProgress() != nil {
		t.Error("an empty pool must report no read progress, and a nil pool must not panic")
	}
}

// TestTheRouteKeepsTheAnswerClockBeforeTheLastToolCall pins the ORDER of the two checks that decide
// what happens at the end of a session's budget.
//
// Pending tool calls used to run FIRST, so that the provider never saw a dangling tool_calls — and
// the answer turn strips those anyway (finalizeNode → stripUnpairedToolCalls). Measured 2026-09-20
// (三国): the last reply declared three probes, the tool node ran them into the wall, and the round
// ended with "session cut … returning the 44 passage(s) and 0 patch(es)" — the answer turn never
// ran, which is how a run loses its count and every citation at once.
func TestTheRouteKeepsTheAnswerClockBeforeTheLastToolCall(t *testing.T) {
	s := &sessionState{
		BudgetS:      100,
		DeadlineLeft: answerReserveS - 1,
		PendingCalls: []ToolCall{{ID: "c1", Name: "retrieve"}},
	}
	if got := s.route(); got != routeFinalize {
		t.Fatalf("route = %v with the answer's reserve reached and a tool call pending, want finalize", got)
	}

	// Above the reserve the pending call still runs: the protocol needs its response.
	s = &sessionState{
		BudgetS:      100,
		DeadlineLeft: answerReserveS + 30,
		PendingCalls: []ToolCall{{ID: "c1", Name: "retrieve"}},
	}
	if got := s.route(); got != routeTool {
		t.Errorf("route = %v with clock to spare, want the tool node", got)
	}
}

// TestTheSessionsClockIsReadNotRemembered pins the fix for the clock that was a number: DeadlineLeft
// used to be written ONCE (after the navigation prefix) and never again, so nothing inside the loop
// knew how much time was left — turns were bounded only by the caller's context, which expires
// mid-call. Measured 2026-09-20 (三国/关羽): three turns of 36s and 74s inside a 110s clock,
// `session cut` inside the third call, 0 patches, no answer, and a composed answer that named six of
// the seventeen members.
func TestTheSessionsClockIsReadNotRemembered(t *testing.T) {
	s := &sessionState{DeadlineLeft: 99}
	s.refreshClock()
	if s.DeadlineLeft != 99 {
		t.Errorf("refreshClock without an armed deadline changed the clock to %v", s.DeadlineLeft)
	}

	s.SessionDeadline = time.Now().Add(5 * time.Second)
	s.refreshClock()
	if s.DeadlineLeft < 4 || s.DeadlineLeft > 5 {
		t.Errorf("clock = %v, want the ~5s left to the session's own deadline", s.DeadlineLeft)
	}
	s.SessionDeadline = time.Now().Add(-time.Second)
	if got := s.clockLeftS(); got != 0 {
		t.Errorf("clockLeftS() = %v after the deadline, want 0", got)
	}
}

// TestTheTurnBudgetFollowsTheClockAndTheMeasuredPace pins the turn floor as a CLOCK fact: how many
// search turns fit before the answer's own reserve, at the pace this session's calls actually take.
//
// The floor used to be the mode's constant (8 on high) whatever the clock or the provider's latency,
// so a session whose calls take 74s ran three turns into a 110s clock and was cut with nothing
// written (measured 2026-09-20, 三国). The pace is measured, not assumed: the longest turn the
// session has completed.
func TestTheTurnBudgetFollowsTheClockAndTheMeasuredPace(t *testing.T) {
	// One 74s call already spent: the question's 110s clock fits ONE more search turn at that pace,
	// and after it the answer turn has its reserve.
	slowPace, slowLeft := 74.0, 90.0
	slow := &sessionState{DeadlineLeft: slowLeft, expectedTurnS: slowPace}
	wantSlow := int((slowLeft - answerReserveS) / slowPace)
	if wantSlow < 1 {
		wantSlow = 1 // the floor: a session always gets a turn to work with
	}
	if got, want := slow.affordableSearchTurns(), wantSlow; got != want {
		t.Errorf("affordableSearchTurns = %d at a 74s pace, want %d", got, want)
	}
	if got := slow.effectiveTurnFloor(); got != slow.affordableSearchTurns() {
		t.Errorf("effectiveTurnFloor = %d, want the clock's %d (the mode says %d)",
			got, slow.affordableSearchTurns(), slow.actionMaxTurns())
	}
	if slow.canAffordAnotherTurn() {
		t.Error("canAffordAnotherTurn = true with 90s left at a 74s pace, want the answer's clock kept")
	}

	// A faster provider fits more search turns in the same clock.
	fastPace, fastLeft := 30.0, 130.0
	fast := &sessionState{DeadlineLeft: fastLeft, expectedTurnS: fastPace}
	if got, want := fast.affordableSearchTurns(), int((fastLeft-answerReserveS)/fastPace); got != want {
		t.Errorf("affordableSearchTurns = %d at a 30s pace, want %d", got, want)
	}
	if got := fast.effectiveTurnFloor(); got != fast.affordableSearchTurns() {
		t.Errorf("effectiveTurnFloor = %d, want the clock's %d", got, fast.affordableSearchTurns())
	}
	if !fast.canAffordAnotherTurn() {
		t.Error("canAffordAnotherTurn = false with 130s left at a 30s pace, want one more turn")
	}

	// Without a completed turn the pace is floored, so a session is never planned around a 3s call.
	blindLeft := 100.0
	blind := &sessionState{DeadlineLeft: blindLeft}
	if got, want := blind.affordableSearchTurns(), int((blindLeft-answerReserveS)/minExpectedTurnS); got != want {
		t.Errorf("affordableSearchTurns = %d with no measured pace, want %d", got, want)
	}

	// No room at all: the answer turn is what is left.
	late := &sessionState{DeadlineLeft: answerReserveS, expectedTurnS: answerReserveS}
	if got := late.affordableSearchTurns(); got != 0 {
		t.Errorf("affordableSearchTurns = %d with only the reserve left, want 0", got)
	}
	if late.canAffordAnotherTurn() {
		t.Error("canAffordAnotherTurn = true with only the answer's reserve left")
	}
}

// TestTheOpeningIsHandedToTheSessionInRankOrder pins the opening's delivery: the session's first
// observation is the RANKED union of the opening's legs, as a bounded list of previews, EACH WITH THE
// HANDLE IT CAN BE CITED BY.
//
// Before this, the session was handed the pool's tail or a direction-token scan of it (see
// extractRelevantEvidence), and the rank order the opening had already computed was thrown away:
// with a clock that affords two model calls, the order decides what the session reads first.
//
// The handle half is not cosmetic. The registry a citation resolves against is written by the tool
// loop, and the opening's previews are shown BEFORE any tool runs — so a session that answered from
// them (which is what the ranked list is FOR) had nothing to cite: measured 2026-09-20 (三国/关羽), a
// correct sixteen-member answer with 0 passages in the citation registry.
func TestTheOpeningIsHandedToTheSessionInRankOrder(t *testing.T) {
	kb := &Kbinfos{Chunks: []map[string]any{
		{"chunk_id": "c-a", "doc_id": "d1", "content": "云长提华雄之头，掷于地上。"},
		{"chunk_id": "c-b", "doc_id": "d2", "content": "关公斩颜良于万军之中。"},
	}}
	kb.NoteOpening([]string{"c-b", "c-a"})

	got := renderOpening(kb, "", 0)
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) != 2 {
		t.Fatalf("opening rendered %d line(s), want 2:\n%s", len(lines), got)
	}
	// The RANK is what the lines carry: the best-ranked preview comes first.
	for i, want := range []string{"关公斩颜良", "云长提华雄"} {
		if !strings.Contains(lines[i], want) {
			t.Errorf("line %d = %q, want the rank-%d passage (%s)", i+1, lines[i], i+1, want)
		}
	}
	if !strings.Contains(lines[0], "d2") {
		t.Errorf("the opening does not name the document: %q", lines[0])
	}
	// The handle a line prints is the number the registry holds, and the previews are IN the registry
	// in the order they were shown — the seed is rendered before the session exists, so the pool is
	// the only place that can hand out a number both halves agree on.
	if want := []string{"c-b", "c-a"}; !reflect.DeepEqual(kb.SessionEvidenceRefs, want) {
		t.Fatalf("published registry = %v, want the previews in rank order (%v)", kb.SessionEvidenceRefs, want)
	}
	for i, line := range lines {
		if want := fmt.Sprintf("[ID:%d]", i); !strings.Contains(line, want) {
			t.Errorf("line %d = %q, want the citation handle %s", i+1, line, want)
		}
	}

	// A rank entry the pool no longer holds is skipped rather than rendered as a hole — and it does
	// not take a handle with it; an empty opening renders nothing at all.
	kb.NoteOpening([]string{"c-gone", "c-a"})
	// c-a keeps the number it was published with, and the missing id takes no number at all.
	again := renderOpening(kb, "", 0)
	if !strings.Contains(again, "云长提华雄") || strings.Contains(again, "c-gone") {
		t.Errorf("renderOpening = %q, want only the chunk the pool holds", again)
	}
	if !strings.Contains(again, "[ID:1]") || strings.Count(again, "[ID:") != 1 {
		t.Errorf("renderOpening = %q, want c-a's published handle and no other line", again)
	}
	if got := renderOpening(&Kbinfos{}, "", 0); got != "" {
		t.Errorf("an empty opening rendered %q", got)
	}

	// The list is BOUNDED: the head of the ranking is what the session pays for.
	big := &Kbinfos{}
	var ids []string
	for i := 0; i < stageMaxItems(stageOpening)+5; i++ {
		id := fmt.Sprintf("c-%02d", i)
		big.Chunks = append(big.Chunks, map[string]any{"chunk_id": id, "doc_id": "d1", "content": "text " + id})
		ids = append(ids, id)
	}
	big.NoteOpening(ids)
	if got := strings.Count(renderOpening(big, "", 0), "\n"); got != stageMaxItems(stageOpening) {
		t.Errorf("opening rendered %d preview(s), want the cap %d", got, stageMaxItems(stageOpening))
	}
	if len(big.SessionEvidenceRefs) != stageMaxItems(stageOpening) {
		t.Errorf("opening published %d passage(s), want the cap %d", len(big.SessionEvidenceRefs), stageMaxItems(stageOpening))
	}

	// A DECLARED set raises that bound: an enumerated question has to show every member the plan
	// named, and those members sit in the ranking past the stage's own number (measured 2026-09-22,
	// the ten Oscar nominees: eight previews, two members never shown, no child count for either).
	declared := stageMaxItems(stageOpening) + 4
	wide := &Kbinfos{}
	var wideIDs []string
	for i := 0; i < declared+3; i++ {
		id := fmt.Sprintf("w-%02d", i)
		wide.Chunks = append(wide.Chunks, map[string]any{"chunk_id": id, "doc_id": "d1", "content": "text " + id})
		wideIDs = append(wideIDs, id)
	}
	wide.NoteOpening(wideIDs)
	if got := strings.Count(renderOpening(wide, "", declared), "\n"); got != declared {
		t.Errorf("opening rendered %d preview(s) for a declared set of %d, want the declared count", got, declared)
	}
}

// TestWhatTheSeedShowedIsInTheCitationRegistry pins the other half of the same fact: the opening's
// previews are REGISTERED, so an answer written from a preview before any tool call still cites.
//
// The nav prefix is the sharper case. Its payloads are whole tool results with no handle printed
// beside them, and the ladder's retrieve is not a tool LOOP call, so nothing stamped them: measured
// 2026-09-20 (三国/关羽) the prefix's retrieve brought the ten passages holding every name, the session
// answered from them on its first turn, and the run shipped a correct answer with 0 passages in the
// citation registry — not one member citable.
func TestWhatTheSeedShowedIsInTheCitationRegistry(t *testing.T) {
	kb := &Kbinfos{Chunks: []map[string]any{
		{"chunk_id": "c-opening", "doc_id": "d1", "content": "云长提华雄之头，掷于地上。"},
		{"chunk_id": "c-prefix", "doc_id": "d2", "content": "关公手起刀落，带头连肩，斩于马下。"},
		{"chunk_id": "c-tool", "doc_id": "d3", "content": "关公斩蔡阳于古城之下。"},
	}}
	st := &sessionState{KB: kb}

	// The opening's previews are published while the seed is built, and the session starts from that
	// registry: its numbering continues the run's instead of restarting at zero.
	kb.PublishEvidence([]string{"c-opening"})
	st.loadEvidenceRefs(kb)
	if want := []string{"c-opening"}; !reflect.DeepEqual(st.EvidenceRefs, want) {
		t.Fatalf("EvidenceRefs = %v, want the run's registry (%v)", st.EvidenceRefs, want)
	}
	// The prefix's passages come next, and their lines carry the number the registry holds.
	lines := st.seedEvidenceRefs([]string{"c-prefix"})
	if len(lines) != 1 || !strings.Contains(lines[0], "[ID:1]") || !strings.Contains(lines[0], "斩于马下") {
		t.Fatalf("prefix lines = %v, want [ID:1] with the passage's words", lines)
	}
	// The next tool result continues the numbering rather than restarting it.
	st.stampEvidenceRefs([]any{map[string]any{"chunk_id": "c-tool"}})
	if want := []string{"c-opening", "c-prefix", "c-tool"}; !reflect.DeepEqual(st.EvidenceRefs, want) {
		t.Fatalf("EvidenceRefs = %v, want %v", st.EvidenceRefs, want)
	}
	// The tool loop's own stamps stay in the SESSION until the round hands them over: that merge is
	// what publishes them (see recordSessionEvidence in the graph), and it must not renumber.
	if want := []string{"c-opening", "c-prefix"}; !reflect.DeepEqual(kb.SessionEvidenceRefs, want) {
		t.Fatalf("the run's registry = %v, want the seed's two passages (%v)", kb.SessionEvidenceRefs, want)
	}
	kb.PublishEvidence(st.EvidenceRefs)
	if want := []string{"c-opening", "c-prefix", "c-tool"}; !reflect.DeepEqual(kb.SessionEvidenceRefs, want) {
		t.Fatalf("after the round's merge the registry = %v, want %v", kb.SessionEvidenceRefs, want)
	}
	// A document id (navigate_tree's evidence) is not a passage and gets no handle.
	if lines := st.seedEvidenceRefs([]string{"doc-abc"}); len(lines) != 0 {
		t.Errorf("a doc id was numbered: %v", lines)
	}
	// Re-registering the same passage keeps its number, so a citation written earlier still resolves.
	st.seedEvidenceRefs([]string{"c-prefix"})
	if want := []string{"c-opening", "c-prefix", "c-tool"}; !reflect.DeepEqual(st.EvidenceRefs, want) {
		t.Fatalf("EvidenceRefs = %v after a repeat, want %v", st.EvidenceRefs, want)
	}

	// A SECOND round is where the numbering used to break. Its session must open on the run's registry
	// — an answer written in round 1 cites round 1's numbers, and the published list is what those
	// markers are resolved against. Measured 2026-09-20 (三国/关羽): the last round's session had made
	// no tool calls, so it published only its prefix's eight passages while the answer carried
	// [ID:45], and every anchored member came back "->(not-published)".
	round2 := &sessionState{KB: kb}
	round2.loadEvidenceRefs(kb)
	if want := []string{"c-opening", "c-prefix", "c-tool"}; !reflect.DeepEqual(round2.EvidenceRefs, want) {
		t.Fatalf("round 2 started from %v, want the run's registry %v", round2.EvidenceRefs, want)
	}
	if n, c, ok := round2.publishEvidenceID("c-tool"); !ok || n != 2 || c["chunk_id"] != "c-tool" {
		t.Errorf("publishEvidenceID(c-tool) = (%d, %v, %t), want the run's number 2", n, c["chunk_id"], ok)
	}
	if len(kb.SessionEvidenceRefs) != 3 {
		t.Errorf("the run's registry = %v, want no duplicate of a passage it already holds", kb.SessionEvidenceRefs)
	}
}

// TestTheLadderSearchesTheQuestionNotTheSeedBlock pins what the navigation ladder probes with.
//
// Every rung handed `nav.Direction` to its tool verbatim, and the direction is the QUESTION plus the
// seed's own scaffolding ("Clues to cover:", the clues, the open part). Retrieval tokenizes what it
// is given, so the retrieve the ladder ran on the session's behalf searched "Clues", "to" and
// "cover" beside the question (measured 2026-09-20, 三国/关羽) — and the sanitizer that exists for
// exactly this was only wired into the MODEL's calls.
func TestTheLadderSearchesTheQuestionNotTheSeedBlock(t *testing.T) {
	nav := &navContext{Direction: "在《三国演义》中，关羽一共杀了多少有姓名的人物？\n\nClues to cover:\n- 关羽 杀\n- 云长 斩\n\nAn earlier round could not establish: 最后一个被杀者"}
	got := navQuery(nav)
	if got != "在《三国演义》中，关羽一共杀了多少有姓名的人物？" {
		t.Errorf("navQuery = %q, want the direction's question", got)
	}
	// Every rung uses it — the ladder's own retrieve must not search the scaffolding either.
	for _, r := range navRules {
		if q, ok := r.Args(nav)["query"]; ok {
			if fmt.Sprint(q) != got && fmt.Sprint(q) != "["+got+"]" {
				t.Errorf("rule %q searched %v, want %q", r.ID, q, got)
			}
		}
	}
	// A direction that is only a question passes through unchanged.
	if got := navQuery(&navContext{Direction: "巴黎 2019 人口"}); got != "巴黎 2019 人口" {
		t.Errorf("navQuery = %q, want the question as-is", got)
	}
	// No direction at all is not a reason to search the empty string twice.
	if got := navQuery(nil); got != "" {
		t.Errorf("navQuery(nil) = %q, want \"\"", got)
	}
}

// flakyModel fails a fixed number of Complete calls and then answers: the provider stalls seen in
// production (`failed to send request`) are transient.
type flakyModel struct {
	reply *ModelReply
	fails int
	calls int
}

func (m *flakyModel) Complete(_ context.Context, _ []schema.Message, _ []ToolSpec) (*ModelReply, error) {
	m.calls++
	if m.calls <= m.fails {
		return nil, errors.New("Post \"https://api.example/v1/text/chat\": failed to send request")
	}
	return m.reply, nil
}

// TestAFailedAnswerCallIsRetriedOnce pins the retry the answer turn needs.
//
// Measured 2026-09-20 (三国/关羽): `salvage call failed: … failed to send request` cost a run the
// sixteen-member answer whose evidence the session had already read; the fallback composition named
// the six members its own budget happened to admit, and that is what the user saw.
func TestAFailedAnswerCallIsRetriedOnce(t *testing.T) {
	mdl := &flakyModel{reply: &ModelReply{Content: "<answer>关羽杀了十六人 [ID:0]</answer>"}, fails: 1}
	s := &sessionState{
		Messages:     []schema.Message{*schema.UserMessage("关羽杀了多少有姓名的人物？")},
		Tools:        &Toolset{},
		Model:        mdl,
		DeadlineLeft: 40,
		ParentState:  State{State: []Variable{{ID: 0, Type: "count"}}},
	}
	if err := s.finalizeNode(context.Background()); err != nil {
		t.Fatalf("finalizeNode = %v", err)
	}
	if mdl.calls != 2 {
		t.Fatalf("the answer call ran %d time(s), want the failed one retried once", mdl.calls)
	}
	if s.FoundAnswer == nil || !strings.Contains(*s.FoundAnswer, "十六") {
		t.Errorf("FoundAnswer = %v, want the retried call's answer", s.FoundAnswer)
	}

	// With nothing left but the answer's own margin, a second attempt would only race the wall.
	none := &flakyModel{reply: &ModelReply{Content: "<answer>16 [ID:0]</answer>"}, fails: 1}
	s2 := &sessionState{
		Messages:     []schema.Message{*schema.UserMessage("q")},
		Tools:        &Toolset{},
		Model:        none,
		DeadlineLeft: minSalvageRetryS + finalizeMarginS,
		ParentState:  State{State: []Variable{{ID: 0, Type: "count"}}},
	}
	_ = s2.finalizeNode(context.Background())
	if none.calls != 1 {
		t.Errorf("the answer call ran %d time(s) with only the margin left, want 1", none.calls)
	}
}

// TestTheLedgerIsRenderedWholeWhereTheModelDecidesAndAnswers pins the general shape of the record:
// the notes are the MODEL's (its <state> patches are this loop's scratchpad), they are rendered
// VERBATIM wherever the model has to decide or answer, and the runtime adds no list of its own.
//
// Recall was lost by DERIVING one: a one-line record reading "…华雄、颜良、文丑…+15" asked the model to
// account for names it had never been shown, and the answer enumerated ten of the nineteen names the
// derived ledger held (measured 2026-09-20, 三国/关羽). The fix is not a longer derived list — it is not
// deriving one: what counts as a member is a semantic judgement, and the runtime does not make it.
func TestTheLedgerIsRenderedWholeWhereTheModelDecidesAndAnswers(t *testing.T) {
	names := []string{"华雄", "颜良", "文丑", "车胄", "孔秀", "孟坦", "韩福", "卞喜", "王植",
		"秦琪", "蔡阳", "荀正", "庞德", "夏侯存", "成何", "翟元", "关平", "周仓", "关羽"}
	vars := make([]Variable, 0, len(names))
	for i, n := range names {
		vars = append(vars, Variable{ID: i, Type: "person", Candidate: strPtr(n)})
	}
	kb := &Kbinfos{}
	kb.Admit(func(p *PoolAdmitter) {
		// A passage that mentions a name the MODEL never wrote down: the runtime must not add it to
		// the record, and must not score the model for omitting it. Deciding that is the model's job,
		// from the passages it is shown (see sessionRecord).
		p.Add(map[string]any{"chunk_id": "c-x", "content": "关羽斩华雄之外，尚有他人。"})
	})
	s := &sessionState{
		KB: kb, BudgetS: 120, DeadlineLeft: 100, Attempts: 2,
		ParentState:   State{State: vars},
		SearchQueries: []string{"关羽 斩将 名单"},
	}
	rec := s.sessionRecord()
	if len(rec.Notes) != len(names) {
		t.Fatalf("notes = %d, want the %d the model wrote", len(rec.Notes), len(names))
	}
	// The DECISION carries every note, verbatim — the whole ledger, not a sample of it.
	ask := continuationAsk(2, 16, rec.Brief(), rec.Verbose())
	for _, n := range names {
		if !strings.Contains(ask, n) {
			t.Errorf("the decision offer does not carry the model's own note %q", n)
		}
	}
	// The per-turn line is counts only, and it never elides the model's own writing.
	line := rec.Line()
	if strings.Contains(line, "…") {
		t.Errorf("the record line elides the model's record: %q", line)
	}
	// The passages the run holds are NOT the ledger: what the model did not write down stays unclaimed,
	// which is exactly why the answer is written from the notes plus the passages it was shown.
	if got := s.sessionRecord().Notes; len(got) != len(names) {
		t.Errorf("notes = %d after a passage naming another member, want the %d the model wrote", len(got), len(names))
	}
	// Writing a note is what moves the "shown since your last note" counter (see runActionNode).
	s.notesAtEvidence, s.RetrievedEvidenceIDs = 0, []string{"c-a", "c-b"}
	if got := s.sessionRecord().ShownSinceNote; got != 2 {
		t.Errorf("ShownSinceNote = %d, want 2", got)
	}
	s.notesAtEvidence = len(s.RetrievedEvidenceIDs)
	if got := s.sessionRecord().ShownSinceNote; got != 0 {
		t.Errorf("ShownSinceNote = %d after a note, want 0", got)
	}
}
