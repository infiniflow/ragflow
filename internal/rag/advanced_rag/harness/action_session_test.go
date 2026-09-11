package harness

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

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
		return ToolOutcome{Status: StatusEmpty, Reason: ReasonNoStructure}, nil
	}
	return ToolOutcome{
		Status:      StatusOK,
		Payload:     []any{map[string]any{"kind": name, "content": "hit for " + name, "chunk_id": "c-" + name}},
		EvidenceIDs: []string{"c-" + name},
	}, nil
}

// TestConsumeExchangeUsesIdPrefix pins Python _run_nav_chain(id_prefix=...): the
// prefix and the in-session ladder fallback share rule ids, so their tool_call
// ids must be tagged differently ("nav_<rule>" vs "ladder_<rule>") or the
// provider rejects the history as a duplicate id.
func TestConsumeExchangeUsesIdPrefix(t *testing.T) {
	cases := []struct {
		idPrefix string
		want     string
	}{
		{"nav", "nav_r1"},
		{"ladder", "ladder_r1"},
	}
	for _, c := range cases {
		ex := &NavExchange{}
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

// TestToolNodeContinuesLadderInCode pins Python action_session.py:_tool_node:
// when the rung the model just ran came back weak, the ladder keeps advancing IN
// CODE through the remaining (cheaper, wider) rungs, instead of leaving the model
// to rediscover the fallback one turn at a time. The continuation must also
// update where the ladder rests, so a later call in the SAME batch resumes from
// there rather than from the original resting point.
//
// Unreachable with the shipped NavRules (all ModeAuto), so this test installs an
// LLM rung to exercise it — the ported code is what makes a future LLM rung
// work without another change.
func TestToolNodeContinuesLadderInCode(t *testing.T) {
	// Install an LLM rung that falls through to "global" (retrieve) on an empty
	// result, and remove it afterwards.
	const llmRung = "test-llm-rung"
	navRuleByID[llmRung] = &NavRule{
		ID:   llmRung,
		Tool: "navigate_structure",
		Mode: ModeLLM,
		Next: map[string]string{StatusEmpty: "global", StatusMiss: "global"},
	}
	defer delete(navRuleByID, llmRung)

	exec := &ladderExec{emptyFor: map[string]bool{"navigate_structure": true}}
	ts := &Toolset{Exec: exec, ThinkingMode: "high"}

	// The model issues one call on the rung the chain handed back.
	st := &SessionState{
		Tools:        ts,
		DeadlineLeft: 60,
		Direction:    "who created Culdcept",
		NavRuleID:    llmRung,
		ToolCache:    map[string]ToolOutcome{},
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
	st := &SessionState{
		Tools:        ts,
		DeadlineLeft: 60,
		Direction:    "who created Culdcept",
		NavRuleID:    "", // ladder finished
		ToolCache:    map[string]ToolOutcome{},
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

// TestSessionGraphCompilesOnce pins the Python shape: _SESSION_GRAPH is built
// at import (:1936) and every session invokes THAT graph, so repeated calls must
// hand back the same runnable instead of compiling a fresh one per session.
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
	st := &SessionState{
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

	states := make([]*SessionState, sessions)
	for i := 0; i < sessions; i++ {
		dir := fmt.Sprintf("direction-%02d", i)
		states[i] = &SessionState{
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

// failingModel fails every Complete call, standing in for Python's
// TimeoutError / provider-exception branch of _run_action_node.
type failingModel struct{ err error }

func (m *failingModel) Complete(_ context.Context, _ []schema.Message, _ []ToolSpec) (*ModelReply, error) {
	return nil, m.err
}

// TestRunActionNodeConvergesOnLLMError pins Python _run_action_node:1215-1220:
// a timed-out or failed turn converges the session EMPTY ({"_done": True,
// "new_states": [], "found_answer": None}) so the graph ends through _route/END
// and run_action_session still returns the messages/evidence gathered so far.
// Returning the error instead aborted the whole eino run, which RunActionSession
// reports as a failed session and answers with an empty Result.
func TestRunActionNodeConvergesOnLLMError(t *testing.T) {
	s := &SessionState{
		Messages:     []schema.Message{*schema.UserMessage("q")},
		Tools:        &Toolset{},
		Model:        &failingModel{err: errors.New("provider down")},
		DeadlineLeft: 30,
		ParentState:  State{State: []Variable{{ID: 0, Type: "aspect"}}},
	}
	if err := s.runActionNode(context.Background()); err != nil {
		t.Fatalf("runActionNode = %v; Python converges the session instead of aborting the graph", err)
	}
	if !s.Done {
		t.Error("Done = false; the session must converge so _route returns END")
	}
	if len(s.NewStates) != 0 || s.FoundAnswer != nil {
		t.Errorf("converged session carried states/answer: %d / %v", len(s.NewStates), s.FoundAnswer)
	}
	if s.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1 (the failed turn still counts)", s.Attempts)
	}
	// The session loop must terminate on the converged state rather than abort.
	if err := s.sessionLoop(context.Background()); err != nil {
		t.Fatalf("sessionLoop = %v; a converged session must end normally", err)
	}
}

// TestFinalizeNodeHarvestsNarrativeWhenSalvageFails pins Python
// _finalize_node:1494-1518: a failed salvage call is logged and the node still
// runs the deterministic loose-clue harvest, so the last narration survives as a
// breadcrumb on the first unresolved slot.
func TestFinalizeNodeHarvestsNarrativeWhenSalvageFails(t *testing.T) {
	const narration = "The entity was founded in 1865 by a consortium of local merchants."
	s := &SessionState{
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

// TestInitRetryTimeout pins Python _init_retry_timeout (:2080-2096): the slot
// table decomposition retry gets a longer window than the first attempt, floored
// at the first attempt's budget and clamped by the round deadline.
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

// TestInitializeStateKeepsEmptyFirstQueries pins Python initialize_state:2130
// (`[str(q).strip() for q in (...)] [:3]`): the first three entries are taken
// and stripped, and an entry that strips to "" is KEPT — filtering it made Go
// pick a later entry instead.
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

// TestPyIntMatchesPython pins Python's int() semantics: bool is an int subclass,
// floats truncate toward zero, strings may carry whitespace/underscores, and
// anything Python cannot convert reports failure (which discards the whole
// decomposition in initialize_state).
func TestPyIntMatchesPython(t *testing.T) {
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
		got, converted := pyInt(c.in)
		if !converted || got != c.want {
			t.Errorf("pyInt(%#v) = %d, %v; want %d, true", c.in, got, converted, c.want)
		}
	}
	// Python raises for these: int("5.5") is a ValueError, int(None) a TypeError.
	for _, bad := range []any{"5.5", "abc", "", nil, []any{1}, map[string]any{}} {
		if _, converted := pyInt(bad); converted {
			t.Errorf("pyInt(%#v) reported success; Python raises", bad)
		}
	}
}

// TestPyStringListMatchesPython pins `[str(c) for c in (value or [])]`: falsy
// values yield nothing, a string yields its characters, a map its keys, and a
// non-iterable value is an error (TypeError in Python).
func TestPyStringListMatchesPython(t *testing.T) {
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
		got, converted := pyStringList(c.in)
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

// TestInitializeStateDiscardsOnUnconvertibleValues pins Python's exception path:
// int(s["id"]) and the clues comprehension raise on bad values, _build_slot_table
// catches it, and the whole decomposition is dropped in favour of the planner
// fan-outs. Go signals the same by returning an empty root.
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
			t.Errorf("%s: root has %d slot(s); Python discards the decomposition", c.name, len(res.Root.State))
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
		t.Errorf("type = %q, want %q (Python str(7))", got, "7")
	}
	if got := res.Root.State[1].Type; got != "entity" {
		t.Errorf("type = %q, want %q (falsy -> entity)", got, "entity")
	}
}

// TestInitializeStateSpentBudgetTimesOutImmediately pins Python
// initialize_state:2106 (`min(_INIT_TIMEOUT_S, deadline_left or _INIT_TIMEOUT_S)`):
// 0 means "unset", but a NEGATIVE deadline is used as-is, so asyncio.timeout fires
// on the next tick and NEITHER attempt reaches the provider — the caller then
// falls back to the planner fan-outs.
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

// TestConsumeExchangeCapsPayload pins Python _emit_nav_pair:1815-1828 — the
// in-session ladder passes its remaining context budget down, so the emitted
// tool payload is truncated; 0 keeps the prefix path uncapped.
func TestConsumeExchangeCapsPayload(t *testing.T) {
	big := strings.Repeat("x", 5000)
	payload := []any{map[string]any{"kind": "navigate_tree", "content": big}}

	capped := &NavExchange{}
	capped.consumeExchange("r1", "navigate_tree", map[string]any{"q": 1}, ToolOutcome{Payload: payload}, "ladder", 1000)
	if got := len(capped.Messages[1].Content); got > 1000 {
		t.Errorf("tool payload = %d chars, want it capped at 1000", got)
	}

	uncapped := &NavExchange{}
	uncapped.consumeExchange("r1", "navigate_tree", map[string]any{"q": 1}, ToolOutcome{Payload: payload}, "nav", 0)
	if got := len(uncapped.Messages[1].Content); got <= 1000 {
		t.Errorf("tool payload = %d chars, want 0 to mean uncapped", got)
	}
}

// TestExtractJSONLenient mirrors Python action_session.extract_json's fallback
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
	// The brace-matched loop tries each "{" candidate; the deep json_repair
	// fallback salvages the FIRST candidate even when it is malformed (mirroring
	// Python, where json_repair.loads({"a":}) yields {"a": null}). It must not
	// panic, hang, or return nil for a salvageable object.
	v := ExtractJSON(`{"a":} {"b": 2}`)
	if v == nil {
		t.Fatalf("first candidate should be salvaged, got nil")
	}
	// Empty input has nothing to salvage and must return nil (not panic).
	if v := ExtractJSON(``); v != nil {
		t.Fatalf("empty input: got %v, want nil", v)
	}
}

// TestExtractJSONWholeTextRepair mirrors Python json_repair.loads(text) on the
// WHOLE text — deformities that no single brace-matched candidate can fix.
func TestExtractJSONWholeTextRepair(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want map[string]any
	}{
		// Unquoted object keys (Python json_repair quotes them).
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

func (m *fixedReplyModel) Complete(_ context.Context, _ []schema.Message, _ []ToolSpec) (*ModelReply, error) {
	m.calls++
	return m.reply, nil
}

func declaredTools() []ToolSpec {
	return []ToolSpec{{
		Function: ToolFunction{
			Name:        "retrieve",
			Description: "run retrieval",
			Parameters:  map[string]any{"type": "object"},
		},
	}}
}

// TestNoPromptBasedToolInstruction pins the alignment with Python: Python has NO
// tool-call instruction of any kind — it binds tools natively
// (action_session.py:_acompletion) and reads msg.tool_calls (:1101-1114). A prompt-based
// protocol is a Go-only addition and must not be sent, least of all one whose
// "at most one tool per reply" clause contradicts the native tool list.
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
		[]ToolSpec{{Function: ToolFunction{Description: "unnamed"}}}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	for _, msg := range inv.req.Messages {
		if strings.Contains(msg.Content, "at most one tool per reply") {
			t.Errorf("prompt-based tool instruction sent: %q", msg.Content)
		}
	}
}

// TestAssistantMessageCarriesToolCalls pins Python action_session.py:_run_action_node:
// the assistant message carries the model's tool_calls verbatim so the tool
// responses that follow can be paired by id. Passing nil leaves a dangling
// tool_call, and the next request is rejected with "tool call result does not
// follow tool call".
func TestAssistantMessageCarriesToolCalls(t *testing.T) {
	st := &SessionState{
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
	// Python synthesizes call_{i} when the provider omits the id
	// (:1134/:1136); the tool responses reference the same ids.
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

// TestDisabledToolIsNotUnknown pins Python action_session.py:_parse_tool_calls —
// `if name not in _TOOL_MAP`. The check is against the STATIC full set, not the
// active surface, so a tool this session disabled (or hid for lack of a web
// provider) is still a known tool: the model must get the "unavailable, use
// this instead" note from ExecuteTool, not an "unknown tool" correction.
func TestDisabledToolIsNotUnknown(t *testing.T) {
	// Sanity: the name must be a real tool for the test to mean anything.
	if _, ok := ToolMap["graph_explore"]; !ok {
		t.Skip("graph_explore is not a registered tool")
	}
	ts := &Toolset{ThinkingMode: "high"}
	// Disable it: it now drops out of the active surface but stays in ToolMap.
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
		t.Error("a disabled tool must NOT be Unknown: Python judges against _TOOL_MAP, " +
			"so the call gets the 'unavailable, use this instead' note")
	}
}

// TestUnknownToolStillFlagged pins the other side of :1124 — a name that is not
// a real tool at all IS flagged Unknown, so it is answered with a correction
// instead of being executed.
func TestUnknownToolStillFlagged(t *testing.T) {
	calls := nativeToHarnessCalls([]chat.ToolCall{{Name: "definitely_not_a_tool"}})
	if len(calls) != 1 || !calls[0].Unknown {
		t.Fatalf("calls = %+v, want one Unknown call", calls)
	}
}

// TestNativeToolCallsPreserveAll pins the execution contract: every native tool
// call is mapped, not just the first — Python executes all of them
// (action_session.py:_tool_node `for c in pending`).
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
		Function: ToolFunction{
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
		Function: ToolFunction{Name: "search", Description: "d", Parameters: map[string]any{}},
	}}

	// No native tool_calls => the reply stays tool-less. This confirms the
	// harness still declares tools on the request (verified above) but does not
	// hallucinate a call, matching Python, which has no other parsing path.
	reply, err := m.Complete(context.Background(), []schema.Message{*schema.UserMessage("q")}, tools)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reply.ToolCalls) != 0 {
		t.Fatalf("expected no tool calls, got %+v", reply.ToolCalls)
	}
}

// streamingCapturingInvoker implements chat.StreamingInvoker and records the
// streaming request, returning a native tool call so we can verify the harness
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
		Function: ToolFunction{
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
		Function: ToolFunction{Name: "search", Description: "d", Parameters: map[string]any{}},
	}}
	// A fenced block in the content is NOT a tool call: Python only reads
	// msg.tool_calls, and this port has no prompt-based parsing path.
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
	loader := StringPromptLoader{"tpl": "hello {{ name }}!"}
	// Loader resolves the name: variable substituted.
	if got := prompts.Render(loader, "tpl", "", map[string]string{"name": "world"}); got != "hello world!" {
		t.Errorf("prompts.Render = %q", got)
	}
	// Unknown template with empty fallback -> empty (Python has no degraded
	// constant; a missing template fails fast rather than degrading).
	if got := prompts.Render(loader, "missing", "", nil); got != "" {
		t.Errorf("unknown template with empty fallback = %q", got)
	}
	// Nil loader + known embedded name still resolves the canonical .md.
	if got := prompts.Render(nil, "sca_select", "", map[string]string{"question": "Q"}); !strings.Contains(got, "Q") {
		t.Errorf("nil loader did not resolve embedded sca_select: %q", got)
	}
	// Both {{k}} and {{ k }} are substituted (templates use the spaced form).
	if got := prompts.Render(StringPromptLoader{"tpl": "a={{x}} b={{ y }}"}, "tpl", "",
		map[string]string{"x": "1", "y": "2"}); got != "a=1 b=2" {
		t.Errorf("spaced placeholder not substituted: %q", got)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------
