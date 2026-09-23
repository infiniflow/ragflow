package agentic_rag

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/rag/agentic-rag/runtime"
	"ragflow/internal/rag/agentic-rag/slots"
	"ragflow/internal/rag/prompts"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// Test doubles

// scriptedModel answers with a fixed set of canned replies, one per call,
// looping on the last if exhausted. It satisfies runtime.SessionModel. seen
// records every message slice handed to Complete, for assertions on retry turns.
type scriptedModel struct {
	mu      sync.Mutex
	replies []string
	idx     int
	seen    [][]schema.Message
}

func (m *scriptedModel) push(reply string) {
	m.replies = append(m.replies, reply)
}

func (m *scriptedModel) Complete(ctx context.Context, msgs []schema.Message, _ []runtime.ToolSpec) (*runtime.ModelReply, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	// The pre-flight probe (probeChatModel) is infrastructure, not script: it is
	// answered here and never consumes a scripted reply or enters `seen`, so the
	// call order each test documents (formalize → planner → slot table → draft →
	// SCA) stays what the test wrote. A test that wants the PROBE to fail uses
	// errModel instead.
	if len(msgs) == 1 && msgs[0].Role == schema.User && msgs[0].Content == probePrompt {
		return &runtime.ModelReply{Content: "ok"}, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seen = append(m.seen, msgs)
	if len(m.replies) == 0 {
		return &runtime.ModelReply{Content: ""}, nil
	}
	r := m.replies[m.idx]
	if m.idx < len(m.replies)-1 {
		m.idx++
	}
	return &runtime.ModelReply{Content: r}, nil
}

// promptRoutedModel answers by inspecting the prompt instead of by call order,
// so a test survives the graph adding or reordering node LLM calls. The SCA call
// returns insufficient on its FIRST invocation and sufficient afterwards, which
// is what drives one rewrite round.
type promptRoutedModel struct {
	mu      sync.Mutex
	scaSeen int
}

func (m *promptRoutedModel) Complete(ctx context.Context, msgs []schema.Message, _ []runtime.ToolSpec) (*runtime.ModelReply, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var text strings.Builder
	for _, msg := range msgs {
		text.WriteString(msg.Content)
	}
	switch {
	case strings.Contains(text.String(), "Break the user's question into 2 to 5"):
		return &runtime.ModelReply{Content: `{"fanouts": ["when built", "where located"]}`}, nil
	case strings.Contains(text.String(), "research strategist"):
		return &runtime.ModelReply{Content: `{"slots":[{"id":0,"type":"aspect","question":"when built","clues":["1865"]},{"id":1,"type":"aspect","question":"where located","clues":["geneva"]}], "first_queries":["when built","where located"]}`}, nil
	case strings.Contains(text.String(), "Sufficient Context Agent"):
		m.scaSeen++
		if m.scaSeen == 1 {
			return &runtime.ModelReply{Content: `{"is_sufficient": false, "score": 0.2, "contradictions": [], "reasoning": "missing the builder", "sub_queries": [{"missing_fact": "who built it", "search_hint": "builder", "satisfied": false}], "claims": {}}`}, nil
		}
		return &runtime.ModelReply{Content: `{"is_sufficient": true, "score": 0.9, "contradictions": [], "reasoning": "ok", "claims": {}}`}, nil
	case strings.Contains(text.String(), "Query Rewriter"):
		return &runtime.ModelReply{Content: `{"queries": [{"query": "who built it"}]}`}, nil
	default:
		return &runtime.ModelReply{Content: "Built in 1865, in Geneva."}, nil
	}
}

// stubExecutor is a deterministic ToolExecutor recording every call.
type stubExecutor struct {
	mu        sync.Mutex
	calls     []string
	responses map[string]runtime.ToolOutcome
}

func newStubExecutor() *stubExecutor {
	return &stubExecutor{responses: map[string]runtime.ToolOutcome{}}
}

func (e *stubExecutor) add(name, payload string, status string) {
	e.responses[name] = runtime.ToolOutcome{
		Status:  status,
		Payload: []any{payload},
	}
}

func (e *stubExecutor) Execute(ctx context.Context, name string, args map[string]any) (runtime.ToolOutcome, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = append(e.calls, name)
	if o, ok := e.responses[name]; ok {
		return o, nil
	}
	return runtime.ToolOutcome{Status: runtime.StatusOK, Payload: []any{"{}"}}, nil
}

func newToolset(exec *stubExecutor) *runtime.Toolset {
	return &runtime.Toolset{
		ThinkingMode:  "high",
		HasWebSearch:  true,
		DisabledTools: map[string]bool{},
		Exec:          exec,
	}
}

// countingRetriever is a no-op retriever recording the requested top-N. It
// satisfies runtime.Retriever (= tools.Retriever).
type countingRetriever struct {
	mu   sync.Mutex
	topN int
}

func (r *countingRetriever) Retrieve(_ context.Context, req runtime.RetrieveRequest) ([]map[string]any, error) {
	r.mu.Lock()
	r.topN = req.TopN
	r.mu.Unlock()
	return []map[string]any{{"doc_id": "d1", "docnm_kwd": "doc1", "content": "Saint Lawrence River; 14 April 1865; 9 December 2019."}}, nil
}

// routeResearch unit tests. This router is the loop's ONLY decision (see its doc comment), and
// every fact it reads is a record the round itself produced: the session's answer, the part the
// session said it could not establish, and whether the pool grew.

// TestRouteResearchClosesOutWhenTheSessionAnswered pins the paper's stop condition: the loop ends
// because the AGENT ANSWERED and named nothing open — even with fresh evidence in the pool, which
// is the fact that would otherwise ask for another round.
func TestRouteResearchClosesOutWhenTheSessionAnswered(t *testing.T) {
	st := NewAgenticState("关羽杀了多少有姓名的人物？", "", 3, nil)
	st.LastRoundNew = 40
	st.Deadline = time.Now().Add(120 * time.Second)

	st.CollectedAnswer = "关羽杀了十二人：华雄、颜良、文丑…"
	if n := routeResearch(st, 3); n != nodeFormalizeAnswer {
		t.Fatalf("answered: route = %v, want formalize_answer", n)
	}
}

// TestRouteResearchReopensOnAnAnswerThatNamesAnOpenPart pins the L2 rule: an answer that NAMES a
// part it could not establish is not a finished round.
//
// "The session answered ⇒ stop" holds when the agent answers only what it can support. A multi-hop
// question's first answer is very often "I read X, and X does not carry Y" — an answer AND a
// statement of work remaining — and treating it as final is how four questions stayed at zero while
// their sessions each held the bridge and said so, with ~150s of the round's 180s unspent
// (measured 2026-09-20, FRAMES: Quincy's mayors, Gifu's population, AP's law, Lahore in 1858).
func TestRouteResearchReopensOnAnAnswerThatNamesAnOpenPart(t *testing.T) {
	newState := func() *AgenticState {
		st := NewAgenticState("What is the population of the birthplace of the writer of Culdcept Saga?", "", 3, nil)
		st.LastRoundNew = 25
		st.Deadline = time.Now().Add(120 * time.Second)
		st.CollectedAnswer = "The writer is Tow Ubukata, born in Gifu Prefecture [ID:0]…"
		st.SessionUnresolved = "the population figure for Gifu Prefecture"
		return st
	}

	if n := routeResearch(newState(), 3); n != nodeRagAgentLoop {
		t.Fatalf("answer with an open part: route = %v, want another round", n)
	}

	// The same answer with nothing left open is final — the paper's rule, unchanged.
	answered := newState()
	answered.SessionUnresolved = ""
	if n := routeResearch(answered, 3); n != nodeFormalizeAnswer {
		t.Errorf("answer with nothing open: route = %v, want formalize_answer", n)
	}

	// Nor does an open part reopen a round whose rounds are spent.
	spent := newState()
	spent.SearchRounds = 3
	if n := routeResearch(spent, 3); n != nodeFormalizeAnswer {
		t.Errorf("open part but rounds spent: route = %v, want formalize_answer", n)
	}

	// Nor one whose clock cannot fit another round AND the finale it has to hand the question to.
	late := newState()
	late.Deadline = time.Now().Add(time.Duration(finaleShareS(70)+MinRoundS-1) * time.Second)
	if n := routeResearch(late, 3); n != nodeFormalizeAnswer {
		t.Errorf("open part but no room for the round and the finale: route = %v, want formalize_answer", n)
	}
}

// TestRouteResearchGivesANamedOpenPartOneZeroGrowthRound pins the single free hop: a round that
// added NO passage may still be followed by one more attempt when the session named a part — that
// is the bridge hop a multi-hop question is made of ("I read X, X does not carry Y") — and a SECOND
// such round is refused, so a stall cannot spend the question.
//
// This is what replaced the slot table as the continuation fact: the slot table is the session's
// scratchpad, and a round that read forty passages already in the pool used to look like one that
// had learned nothing (measured 2026-09-20: three of four zero-scoring questions ended on
// `no round is asked for (… +0 chunks this round)`).
func TestRouteResearchGivesANamedOpenPartOneZeroGrowthRound(t *testing.T) {
	st := NewAgenticState("When and where was it built?", "", 3, nil)
	st.CollectedAnswer = "It was built at Geneva."
	st.SessionUnresolved = "the year it was built"
	st.LastRoundNew = 0
	st.Deadline = time.Now().Add(150 * time.Second)

	if n := routeResearch(st, 3); n != nodeRagAgentLoop {
		t.Fatalf("first zero-growth round with a named part: route = %v, want another round", n)
	}
	// The next round learned nothing either: the hop is refused.
	st.LastRoundNew = 0
	if n := routeResearch(st, 3); n != nodeFormalizeAnswer {
		t.Errorf("second zero-growth round: route = %v, want formalize_answer", n)
	}
}

// TestRouteResearchTheSlotTableDoesNotDecide pins the 2026-09-20 decision that took the plan out of
// the routing: two states that differ ONLY in their unresolved slots must route the same way.
//
// The slot table is the session's scratchpad (the model fills it to remember what it found), so a
// table full of unresolved slots is not a statement that work remains — and a table empty of them
// is not a statement that the question is answered. What decides is the session's own <unresolved>
// statement and whether the round brought evidence in.
func TestRouteResearchTheSlotTableDoesNotDecide(t *testing.T) {
	withSlots := NewAgenticState("When and where was it built?", "", 3, nil)
	withSlots.UnresolvedSlots = []map[string]any{{"question_clues": []string{"when built", "where built"}}}
	withSlots.CollectedAnswer = "It was built in Geneva."
	withSlots.LastRoundNew = 40
	withSlots.Deadline = time.Now().Add(150 * time.Second)

	without := NewAgenticState("When and where was it built?", "", 3, nil)
	without.CollectedAnswer = "It was built in Geneva."
	without.LastRoundNew = 40
	without.Deadline = time.Now().Add(150 * time.Second)

	if a, b := routeResearch(withSlots, 3), routeResearch(without, 3); a != b {
		t.Errorf("unresolved slots changed the route: with=%v without=%v; the slot table must not decide", a, b)
	}
	if got := routeResearch(withSlots, 3); got != nodeFormalizeAnswer {
		t.Errorf("route = %v, want formalize_answer (an answer that names nothing open is final)", got)
	}
}

// TestRouteResearchRetriesARoundThatProducedNothingWhileEvidenceArrives pins the one continuation
// that needs neither an answer nor a named part: the round FAILED (it wrote no answer) while its own
// retrieval was still feeding the pool. A retry is the only way such a round can end in an answer,
// and a round that added nothing is not retried.
func TestRouteResearchRetriesARoundThatProducedNothingWhileEvidenceArrives(t *testing.T) {
	st := NewAgenticState("When and where was it built?", "", 3, nil)
	st.LastRoundNew = 40
	st.Deadline = time.Now().Add(150 * time.Second)
	if n := routeResearch(st, 3); n != nodeRagAgentLoop {
		t.Fatalf("no answer, nothing open, but evidence arrived: route = %v, want another round", n)
	}

	stalled := NewAgenticState("When and where was it built?", "", 3, nil)
	stalled.LastRoundNew = 0
	stalled.Deadline = time.Now().Add(150 * time.Second)
	if n := routeResearch(stalled, 3); n != nodeFormalizeAnswer {
		t.Errorf("no answer and nothing arrived: route = %v, want formalize_answer", n)
	}
}

// TestRouteResearchHonoursTheRoundBudgetAndTheClock pins the two bounds that are neither the
// answer nor the record: the mode's round count, and the room a round needs — the round itself PLUS
// the finale it hands the question over to (see canOpenRound).
func TestRouteResearchHonoursTheRoundBudgetAndTheClock(t *testing.T) {
	atMax := NewAgenticState("When and where was it built?", "", 3, nil)
	atMax.CollectedAnswer = "It was built in Geneva."
	atMax.SessionUnresolved = "the year"
	atMax.LastRoundNew = 40
	atMax.SearchRounds = 3
	atMax.Deadline = time.Now().Add(150 * time.Second)
	if n := routeResearch(atMax, 3); n != nodeFormalizeAnswer {
		t.Errorf("round budget spent: route = %v, want formalize_answer", n)
	}

	tight := NewAgenticState("When and where was it built?", "", 3, nil)
	tight.CollectedAnswer = "It was built in Geneva."
	tight.SessionUnresolved = "the year"
	tight.LastRoundNew = 40
	tight.Deadline = time.Now().Add(70 * time.Second)
	if n := routeResearch(tight, 3); n != nodeFormalizeAnswer {
		t.Errorf("no room for a round and the finale: route = %v, want formalize_answer", n)
	}
}

// TestResearchStatusNoteReportsTheRecordNotAVerdict pins what the "[Research status]" note says
// now: a round that ANSWERED has nothing to annotate, and one that did not states what it read and
// what the plan still lists as unresolved.
func TestResearchStatusNoteReportsTheRecordNotAVerdict(t *testing.T) {
	answered := NewAgenticState("q", "", 3, nil)
	answered.CollectedAnswer = "the answer"
	if got := researchStatusNote(answered); got != "" {
		t.Errorf("answered round note = %q, want empty", got)
	}

	st := NewAgenticState("q", "", 3, nil)
	st.KB = &runtime.Kbinfos{Chunks: []map[string]any{{"chunk_id": "c1"}, {"chunk_id": "c2"}}}
	st.UnresolvedSlots = []map[string]any{{"question_clues": []string{"a"}}}
	got := researchStatusNote(st)
	if !strings.Contains(got, "2 passages read") || !strings.Contains(got, "1 plan slot(s) still unresolved") {
		t.Errorf("note = %q, want the round's own record", got)
	}

	// Nothing read and nothing open: no note at all, because there is nothing to report.
	if got := researchStatusNote(NewAgenticState("q", "", 3, nil)); got != "" {
		t.Errorf("empty record note = %q, want empty", got)
	}
}

func TestMergeSlotPatchAdoptsStronger(t *testing.T) {
	strong := 0.95
	base := runtime.NewState([]runtime.Variable{
		{ID: 0, Type: "aspect", Candidate: strPtr("base"), CandidateStrength: &strong},
	}, 0, nil)
	// A weak branch (0.30) must NOT downgrade the strong base.
	weak := 0.30
	branch := runtime.NewState([]runtime.Variable{
		{ID: 0, Type: "aspect", Candidate: strPtr("weak-candidate"), CandidateStrength: &weak},
	}, 1, nil)
	merged := MergeSlotPatch(base, branch)
	// A weak branch does not downgrade the strong base. It is no longer a no-op
	// either: the claim that lost the comparison is kept as an alternate, so the
	// list two sessions disagreed about does not vanish (see
	// TestMergeSlotPatchKeepsTheLosingListAsAnAlternate).
	if merged != nil {
		if got := merged.ByID(0).Candidate; got == nil || *got != "base" {
			t.Fatalf("weak branch upgraded the slot: %v", got)
		}
		if alts := alternateCandidatesOf(*merged.ByID(0)); len(alts) != 1 || alts[0] != "weak-candidate" {
			t.Fatalf("alternates = %v, want the weak claim kept as the loser", alts)
		}
	}

	// A stronger branch wins.
	stronger := 0.99
	branch2 := runtime.NewState([]runtime.Variable{
		{ID: 0, Type: "aspect", Candidate: strPtr("strong-candidate"), CandidateStrength: &stronger},
	}, 1, nil)
	merged2 := MergeSlotPatch(base, branch2)
	if merged2 == nil {
		t.Fatal("expected a merge result")
	}
	if got := merged2.State[0].Candidate; got == nil || *got != "strong-candidate" {
		t.Fatalf("strong branch should upgrade, got %v", got)
	}
}

func TestMergeSlotPatchNoChangeReturnsNil(t *testing.T) {
	base := runtime.NewState([]runtime.Variable{
		{ID: 0, Type: "aspect", Candidate: strPtr("same")},
	}, 0, nil)
	branch := runtime.NewState([]runtime.Variable{
		{ID: 0, Type: "aspect", Candidate: strPtr("same")},
	}, 1, nil)
	if m := MergeSlotPatch(base, branch); m != nil {
		t.Fatalf("identical patch should return nil, got %+v", m)
	}
}

// TestComposeFallbackDraftPrompt pins the draft prompt byte for byte: the FOUND/MISSING
// instructions, the "Retrieved evidence:" label, the "\n"-joined numbered snippets, and
// the language clause appended without a separator.
func TestComposeFallbackDraftPrompt(t *testing.T) {
	mdl := &fakeModel{replies: []*runtime.ModelReply{{Content: "FOUND: x\nMISSING: none"}}}
	st := &AgenticState{
		Question: "谁开的？",
		KB:       &runtime.Kbinfos{Chunks: []map[string]any{{"content_with_weight": "证据一"}}},
	}
	if got := ComposeFallbackDraft(context.Background(), RAGTools{Model: mdl}, st); got != "FOUND: x\nMISSING: none" {
		t.Fatalf("draft = %q", got)
	}
	if len(mdl.messages) != 2 {
		t.Fatalf("model received %d message(s), want system+user", len(mdl.messages))
	}
	wantSystem := "You are a research assistant writing an INTERMEDIATE DRAFT toward answering the user's " +
		"question, using ONLY the retrieved evidence snippets below.\n" +
		"Requirements:\n" +
		"1. First list concrete FACTS FOUND in the snippets (exact numbers, dates, names preserved).\n" +
		"2. Then output a line starting with 'MISSING:' naming precisely which part(s) of the " +
		"question the snippets do NOT answer yet.\n" +
		"3. No conclusions beyond the evidence; no general knowledge.\n" +
		"Keep it under 250 words." +
		"Write your draft in the same language as the question."
	if got := mdl.messages[0].Content; got != wantSystem {
		t.Errorf("system prompt = %q\nwant          %q", got, wantSystem)
	}
	wantUser := "Question: 谁开的？\n\nRetrieved evidence:\n[1] 证据一"
	if got := mdl.messages[1].Content; got != wantUser {
		t.Errorf("user prompt = %q\nwant        %q", got, wantUser)
	}
}

// TestBuildSlotTableKeepsAllFanoutsOnFailure pins the failure path: when
// initialize_state fails, first_queries is the FULL fan-out list — the [:3] cap only
// applies to the empty-root path. Losing the list made the first prefetch narrower.
func TestBuildSlotTableKeepsAllFanoutsOnFailure(t *testing.T) {
	fanouts := []string{"q1", "q2", "q3", "q4", "q5"}
	// No model → initialize_state cannot run, so the failure path is taken.
	root, first := BuildSlotTable(context.Background(), runtime.SessionDeps{}, "raw question", fanouts, 60)
	if len(root.State) != 4 {
		t.Fatalf("fallback slots = %d, want 4 (queries[:4])", len(root.State))
	}
	if len(first) != len(fanouts) {
		t.Errorf("firstQueries = %v, want the full fan-out list on the failure path", first)
	}
}

// TestBuildSlotTableFallsBackOnSpentBudget pins the planner's deadline: the value is NOT
// floored, so once the round budget is spent the decomposition times out at once and the
// table falls back to the planner fan-outs.
func TestBuildSlotTableFallsBackOnSpentBudget(t *testing.T) {
	mdl := &fakeModel{replies: []*runtime.ModelReply{
		{Content: `{"slots":[{"id":0,"type":"aspect","clues":["a"]}]}`},
	}}
	fanouts := []string{"f1", "f2"}
	root, first := BuildSlotTable(context.Background(), runtime.SessionDeps{Model: mdl}, "q", fanouts, -15)
	if len(root.State) != len(fanouts) {
		t.Fatalf("slots = %d, want the %d fan-out fallback slots", len(root.State), len(fanouts))
	}
	if mdl.calls != 0 {
		t.Errorf("model called %d time(s); a spent budget must not reach the provider", mdl.calls)
	}
	if len(first) != len(fanouts) {
		t.Errorf("firstQueries = %v, want %v", first, fanouts)
	}
}

// TestBuildSlotTableFallsBackWhenIDIsNotAnInteger covers the end-to-end effect of the
// strict parser: a non-integer id fails the parse and the table is rebuilt from the
// planner fan-outs.
func TestBuildSlotTableFallsBackWhenIDIsNotAnInteger(t *testing.T) {
	fanouts := []string{"q1", "q2"}
	deps := runtime.SessionDeps{Model: &fakeModel{replies: []*runtime.ModelReply{
		{Content: `{"slots":[{"id":"abc","type":"aspect","clues":["a"]}]}`},
	}}}
	root, first := BuildSlotTable(context.Background(), deps, "raw question", fanouts, 60)
	if len(root.State) != len(fanouts) {
		t.Fatalf("slots = %d, want the %d fan-out fallback slots", len(root.State), len(fanouts))
	}
	for i, v := range root.State {
		if v.Type != "aspect" || len(v.QuestionClues) != 1 || v.QuestionClues[0] != fanouts[i] {
			t.Errorf("slot %d = %+v, want aspect/%s", i, v, fanouts[i])
		}
	}
	if len(first) != len(fanouts) {
		t.Errorf("firstQueries = %v, want %v", first, fanouts)
	}
}

// sessionStubExec answers every tool call with a hit, so a session runs its turns
// without a real retriever.
type sessionStubExec struct{}

func (sessionStubExec) Execute(_ context.Context, name string, _ map[string]any) (runtime.ToolOutcome, error) {
	return runtime.ToolOutcome{
		Status:      runtime.StatusOK,
		Payload:     []any{map[string]any{"kind": name, "content": "hit"}},
		EvidenceIDs: []string{"c-" + name},
	}, nil
}

// TestRunSlotResearchPassLedgerRecordsSessionHint drives a real round whose
// session produces no terminal answer, and asserts the ledger row still carries the
// QUERY of the pair (query, outcome): the direction this session was sent on.
//
// The row used to carry a langchain-style repr of the session's messages instead, and
// that repr is a CONSTANT in production: every session's history opens with the same
// system prompt, so its first 80 runes went into every row of every question.
func TestRunSlotResearchPassLedgerRecordsSessionHint(t *testing.T) {
	st := &AgenticState{
		Question: "who opened it?",
		SlotTable: runtime.NewState([]runtime.Variable{
			{ID: 0, Type: "aspect", QuestionClues: []string{"who opened it?"}},
		}, 0, nil),
	}
	deps := runtime.SessionDeps{
		Model: &fakeModel{replies: []*runtime.ModelReply{
			{Content: "I could not find any evidence about the opening date in the retrieved passages."},
		}},
		Tools: &runtime.Toolset{Exec: sessionStubExec{}},
		KB:    &runtime.Kbinfos{},
	}
	res := RunSlotResearchPass(context.Background(), context.Background(), deps, "who opened it?", st, 60)
	if res == nil {
		t.Fatal("RunSlotResearchPass returned nil")
	}
	if len(res.Attempted) == 0 {
		t.Fatal("no ledger entry recorded")
	}
	q, _ := res.Attempted[len(res.Attempted)-1]["q"].(string)
	if q != "who opened it?" {
		t.Errorf("ledger q = %q, want the direction this session was sent on", q)
	}
	if strings.HasPrefix(q, "[SystemMessage(") {
		t.Error("ledger q carries a message repr — that is the system prompt, not a query")
	}
}

// TestRunSlotResearchPassLogsSlotEvidenceBound pins: the round
// reports how many passages each slot's session bound, so a slot whose session
// retrieved nothing is visible in the run log.
func TestRunSlotResearchPassLogsSlotEvidenceBound(t *testing.T) {
	var buf bytes.Buffer
	orig := _LOG
	_LOG = log.New(&buf, "", 0)
	defer func() { _LOG = orig }()

	st := &AgenticState{
		Question: "who opened it?",
		SlotTable: runtime.NewState([]runtime.Variable{
			{ID: 0, Type: "aspect", QuestionClues: []string{"who opened it?"}},
		}, 0, nil),
	}
	deps := runtime.SessionDeps{
		Model: &fakeModel{replies: []*runtime.ModelReply{{
			Content: "Let me look that up.",
			ToolCalls: []runtime.ToolCall{
				{ID: "call_0", Name: "retrieve", Args: map[string]any{"query": "who opened it?"}},
			},
		}}},
		Tools: &runtime.Toolset{Exec: sessionStubExec{}},
		KB:    &runtime.Kbinfos{},
	}
	if res := RunSlotResearchPass(context.Background(), context.Background(), deps, "who opened it?", st, 60); res == nil {
		t.Fatal("RunSlotResearchPass returned nil")
	}
	if !strings.Contains(buf.String(), "slot evidence bound") {
		t.Errorf("missing the per-slot evidence report; log:\n%s", buf.String())
	}
}

// channelRetriever records the channel each retrieve call belongs to: the exact
// leg is keyword-only (weight 0, top_n=60, query terms folded into the query),
// the semantic-bypass leg has the vector weight on (top_n=30, plain query).
type channelRetriever struct {
	mu      sync.Mutex
	weights []float64
	topN    []int
	queries []string
}

func (r *channelRetriever) Retrieve(_ context.Context, req runtime.RetrieveRequest) ([]map[string]any, error) {
	// Each leg names its keyword weight explicitly; nil (the control was not supplied)
	// is recorded as -1 so the assertions below catch a leg that drops it.
	weight := -1.0
	if req.KeywordsSimilarityWeight != nil {
		weight = *req.KeywordsSimilarityWeight
	}
	// The fan-out runs its legs concurrently (see FanoutSearch): a fixture that appends to
	// its own fields without a lock is a race, not a stricter test.
	r.mu.Lock()
	r.weights = append(r.weights, weight)
	r.topN = append(r.topN, req.TopN)
	r.queries = append(r.queries, req.Query)
	r.mu.Unlock()
	return nil, nil
}

// TestFanoutSearchIsDualChannel pins the property the single-channel
// implementation could not express: each fan-out issues BOTH a keyword BM25 leg
// (narrowed, wide pool) and a dense semantic leg (narrowing BYPASSED, top_n=30).
// One hybrid call per query cannot distinguish exact from paraphrase-only hits,
// so it cannot apply the semantic quota that keeps the bypass low-precision hits
// from displacing exact ones.
func TestFanoutSearchIsDualChannel(t *testing.T) {
	ctx := context.Background()
	r := &channelRetriever{}
	deps := RAGTools{Search: runtime.SearchDeps{Backend: r, KbIDs: []string{"kb1"}, HasEmbedder: true}}
	st := &AgenticState{KB: &runtime.Kbinfos{}}
	FanoutSearch(ctx, deps, st, []string{"when was it built", "where located"}, 8, false)

	// Two fan-outs x two channels. The legs run CONCURRENTLY (see FanoutSearch), so what is
	// recorded first is the schedule's choice, not the query order: the assertion is on the
	// SHAPE of the four calls — two keyword legs and two semantic ones — not on their arrival.
	// The order that IS a contract (exact admitted before semantic) is pinned separately, by
	// the admission order in TestFanoutSearchPrefersExactOverSemantic.
	if len(r.weights) != 4 {
		t.Fatalf("expected 4 retrieve calls (2 fan-outs x 2 channels), got %d", len(r.weights))
	}
	keywordLegs, semanticLegs := 0, 0
	for i := range r.weights {
		if r.weights[i] >= 1 {
			// Channel A: keyword-only weight (KeywordsSimilarityWeight 1 = no dense leg), wide pool,
			// query terms folded in.
			keywordLegs++
			if r.topN[i] != fanoutBM25TopN {
				t.Errorf("keyword leg %d: TopN = %d, want %d", i, r.topN[i], fanoutBM25TopN)
			}
			if !strings.Contains(r.queries[i], "built") && !strings.Contains(r.queries[i], "located") {
				t.Errorf("keyword leg %d: query = %q, want the fan-out query (terms folded in)", i, r.queries[i])
			}
			continue
		}
		// Channel B: dense leg on, so the keyword weight is below 1, top_n=30 (narrowing bypassed).
		semanticLegs++
		if r.topN[i] != fanoutHybridTopN {
			t.Errorf("semantic leg %d: TopN = %d, want %d", i, r.topN[i], fanoutHybridTopN)
		}
	}
	if keywordLegs != 2 || semanticLegs != 2 {
		t.Errorf("legs = %d keyword, %d semantic; want two of each (one per fan-out)", keywordLegs, semanticLegs)
	}
}

// TestFanoutSearchPrefersExactOverSemantic pins the admission order across
// FAN-OUTS: every fan-out's exact channel is admitted before any fan-out's
// semantic channel. Two fan-outs each offer one exact and one semantic hit, and
// the two exact hits must lead — a single merged channel could not express this
// preference at all.
//
// The order used to be observable only through a pool ceiling of two (the exact
// hits took the two free seats, the semantic ones were refused). The pool has no
// ceiling now, so every hit lands and the ORDER is what the test reads.
func TestFanoutSearchPrefersExactOverSemantic(t *testing.T) {
	ctx := context.Background()
	r := &fanoutHitRetriever{}
	deps := RAGTools{Search: runtime.SearchDeps{Backend: r, KbIDs: []string{"kb1"}, HasEmbedder: true}}
	st := &AgenticState{KB: &runtime.Kbinfos{}}
	added, _ := FanoutSearch(ctx, deps, st, []string{"alpha", "beta"}, 8, false)

	if added != 4 {
		t.Fatalf("added = %d, want 4 (no ceiling: every hit a retrieval found is admitted)", added)
	}
	ids := make([]string, 0, len(st.KB.Chunks))
	for _, c := range st.KB.Chunks {
		ids = append(ids, runtime.ChunkIDOf(c))
	}
	if len(ids) != 4 {
		t.Fatalf("admitted %v, want all four hits", ids)
	}
	if ids[0] != "ex-alpha" || ids[1] != "ex-beta" {
		t.Errorf("first two admitted = %v, want the exact-leg hits [ex-alpha ex-beta]", ids[:2])
	}
	for i, id := range ids[2:] {
		if !strings.HasPrefix(id, "sem-") {
			t.Errorf("admitted[%d] = %q, want the semantic-leg hits after the exact ones", i+2, id)
		}
	}
}

// fanoutHitRetriever returns one exact hit and one semantic hit per fan-out,
// distinguished by the leg's top_n (30 = semantic bypass, 60 = exact BM25).
type fanoutHitRetriever struct{}

func (r *fanoutHitRetriever) Retrieve(_ context.Context, req runtime.RetrieveRequest) ([]map[string]any, error) {
	which := "beta"
	if strings.Contains(req.Query, "alpha") {
		which = "alpha"
	}
	if req.TopN == fanoutHybridTopN {
		return []map[string]any{
			{"chunk_id": "sem-" + which, "content": "paraphrase only", "doc_name": "semantic"},
		}, nil
	}
	return []map[string]any{
		{"chunk_id": "ex-" + which, "content": which + " term", "doc_name": "exact"},
	}, nil
}

// claimTopUpEngine stands in for the doc engine across the three store reads
// the channel-0 path makes: the has-compilation probe, the claim recall legs,
// and the directed source-chunk fetch.
type claimTopUpEngine struct {
	engine.DocEngine
}

func (e *claimTopUpEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	if _, ok := req.Filter["compile_kwd"]; ok {
		// DatasetHasCompilation probe: the dataset IS compiled.
		return &types.SearchResult{Chunks: []map[string]interface{}{{"id": "row"}}}, nil
	}
	if ids, ok := req.Filter["id"].([]string); ok {
		// LoadChunksForIDs: the source chunks behind the admitted claims.
		rows := make([]map[string]interface{}, 0, len(ids))
		for _, id := range ids {
			rows = append(rows, map[string]interface{}{
				"id":                  id,
				"content_with_weight": "full text of " + id,
				"doc_id":              "doc-1",
			})
		}
		return &types.SearchResult{Chunks: rows}, nil
	}
	// Claim recall leg: one claim citing src-1.
	payload, _ := json.Marshal(map[string]any{
		"name":             "The tower is 330m tall",
		"description":      "height",
		"evidence":         []any{map[string]any{"quote": "330 metres"}},
		"source_chunk_ids": []string{"src-1"},
	})
	return &types.SearchResult{Chunks: []map[string]interface{}{{
		"content_with_weight": string(payload),
		"source_chunk_ids":    []string{"src-1"},
		"doc_id":              "doc-1",
		"similarity":          0.9,
	}}}, nil
}

// TestFanoutSearchEvidenceTopUp pins the channel-0 directional top-up: after a claim
// pseudo chunk is admitted, the source chunk it cites is fetched by id and admitted too —
// the verbatim quote alone does not carry the surrounding passage.
func TestFanoutSearchEvidenceTopUp(t *testing.T) {
	r := &channelRetriever{}
	de := &claimTopUpEngine{}
	// Unique tenant per run: the runtime claim caches are process-global with a
	// 300s TTL, and this test must pay the store reads itself.
	tenant := fmt.Sprintf("tenant-topup-%d", time.Now().UnixNano())
	deps := RAGTools{Search: runtime.SearchDeps{
		Backend:   r,
		DocEngine: de,
		TenantID:  tenant,
		KbIDs:     []string{"kb-1"},
	}}
	st := &AgenticState{KB: &runtime.Kbinfos{}}
	added, _ := FanoutSearch(context.Background(), deps, st, []string{"tower height topup"}, 8, false)
	if added != 2 {
		t.Fatalf("added = %d, want 2 (the claim row + its source chunk)", added)
	}
	var claimEntry, srcEntry map[string]any
	for _, c := range st.KB.Chunks {
		id, _ := c["chunk_id"].(string)
		switch {
		case strings.HasPrefix(id, "claim_"):
			claimEntry = c
		case id == "src-1":
			srcEntry = c
		}
	}
	if claimEntry == nil {
		t.Fatalf("claim pseudo chunk not admitted; pool = %v", st.KB.Chunks)
	}
	if srcEntry == nil {
		t.Fatalf("source chunk behind the claim was not top-upped; pool = %v", st.KB.Chunks)
	}
	if got, _ := srcEntry["content_with_weight"].(string); got != "full text of src-1" {
		t.Errorf("top-up content = %q, want the fetched source chunk text", got)
	}
}

// draftModel returns a canned draft and records the prompt it was given.
type draftModel struct {
	reply    string
	err      error
	lastUser string
	lastSys  string
	calls    int
}

func (m *draftModel) Complete(_ context.Context, msgs []schema.Message, _ []runtime.ToolSpec) (*runtime.ModelReply, error) {
	m.calls++
	for _, msg := range msgs {
		switch msg.Role {
		case schema.System:
			m.lastSys = msg.Content
		case schema.User:
			m.lastUser = msg.Content
		}
	}
	if m.err != nil {
		return nil, m.err
	}
	return &runtime.ModelReply{Content: m.reply}, nil
}

// TestComposeFallbackDraftUsesLLM pins that the fallback draft is SYNTHESISED,
// not concatenated. The draft feeds PreSummary as "Research findings
// (authoritative)" and is what the SCA reviews, so its shape matters: it must
// ask for a MISSING: line, and a plain list of truncated snippets cannot express
// one.
func TestComposeFallbackDraftUsesLLM(t *testing.T) {
	st := &AgenticState{
		Question: "When was it built?",
		KB:       &runtime.Kbinfos{Chunks: []map[string]any{{"content": "It was built in 1874."}}},
	}
	mdl := &draftModel{reply: "Built in 1874.\nMISSING: architect"}
	got := ComposeFallbackDraft(context.Background(), RAGTools{Model: mdl}, st)

	if mdl.calls != 1 {
		t.Fatalf("model calls = %d, want 1 (the draft is synthesised)", mdl.calls)
	}
	if got != "Built in 1874.\nMISSING: architect" {
		t.Fatalf("draft = %q, want the model's synthesis", got)
	}
	// The system prompt must demand the MISSING line — that is the whole point.
	if !strings.Contains(mdl.lastSys, "MISSING:") {
		t.Errorf("system = %q, want it to require a MISSING: line", mdl.lastSys)
	}
	// numbered "[i]" evidence, 1-indexed.
	if !strings.Contains(mdl.lastUser, "[1] It was built in 1874.") {
		t.Errorf("user = %q, want '[1] <content>' evidence", mdl.lastUser)
	}
}

// TestComposeFallbackDraftFallsBackToEvidence pins -1486 and
// 1512-1514: with no model, or when the call fails, the draft degrades to the
// raw evidence capped at 4000 chars.
func TestComposeFallbackDraftFallsBackToEvidence(t *testing.T) {
	st := &AgenticState{
		Question: "When?",
		KB:       &runtime.Kbinfos{Chunks: []map[string]any{{"content": "Evidence body."}}},
	}
	if got := ComposeFallbackDraft(context.Background(), RAGTools{}, st); !strings.Contains(got, "Evidence body.") {
		t.Errorf("no-model draft = %q, want the raw evidence", got)
	}
	failing := &draftModel{err: errors.New("boom")}
	if got := ComposeFallbackDraft(context.Background(), RAGTools{Model: failing}, st); !strings.Contains(got, "Evidence body.") {
		t.Errorf("failed-call draft = %q, want the raw evidence", got)
	}
}

// TestComposeFallbackDraftOrdersByRelevance pins: the fixed 16-slot
// budget is spent on the STRONGEST evidence, not on insertion order.
func TestComposeFallbackDraftOrdersByRelevance(t *testing.T) {
	st := &AgenticState{
		Question: "When?",
		KB: &runtime.Kbinfos{Chunks: []map[string]any{
			{"content": "weak hit", "similarity": 0.1},
			{"content": "strong hit", "similarity": 0.9},
		}},
	}
	mdl := &draftModel{reply: "draft"}
	ComposeFallbackDraft(context.Background(), RAGTools{Model: mdl}, st)

	iWeak := strings.Index(mdl.lastUser, "weak hit")
	iStrong := strings.Index(mdl.lastUser, "strong hit")
	if iWeak < 0 || iStrong < 0 {
		t.Fatalf("evidence missing from prompt: %q", mdl.lastUser)
	}
	if iStrong > iWeak {
		t.Errorf("order = weak@%d strong@%d, want strongest evidence first", iWeak, iStrong)
	}
}

// TestRagCollectsPerPhaseUsage pins the wiring that makes per-phase LLM usage
// measurable: the collector must be built and the chat model wrapped, otherwise the
// counting machinery stays inert — CurrentStats(ctx) is nil, so the phase markers already
// in the graph record nothing and even the explicit deps.Stats.RecordCall sites are
// skipped by their nil guard.
func TestRagCollectsPerPhaseUsage(t *testing.T) {
	mdl := &countingModel{}
	Rag(context.Background(), RAGTools{
		Retriever: &corpusRetriever{},
		Model:     mdl,
	}, runtime.RunRequest{
		Question:     "Who created Culdcept?",
		ThinkingMode: "high",
		DatasetIDs:   []string{"kb1"},
	})

	// The decisive assertion: Rag must bind stats AND a phase onto the context,
	// because that is what makes every LLM call beneath attributable. The
	// explicit deps.Stats.RecordCall sites fire regardless, so counting calls
	// alone would pass even with the binding removed — hence the ctx probe.
	if mdl.nilStats > 0 {
		t.Errorf("CurrentStats(ctx) was nil on %d of %d calls: Rag did not bind "+
			"stats to the context, so per-phase usage is not collected", mdl.nilStats, mdl.calls)
	}
	if mdl.calls == 0 {
		t.Fatal("the model was never called; the probe proves nothing")
	}
	if mdl.unknownPhase == mdl.calls {
		t.Errorf("every call saw phase \"unknown\": no phase marker reached the " +
			"model, so usage cannot be attributed per phase")
	}
	for p := range mdl.phases {
		t.Logf("observed phase: %s", p)
	}
}

// countingModel records what the context looked like at each completion, so the
// test can prove stats and phase were actually bound.
type countingModel struct {
	calls        int
	nilStats     int
	unknownPhase int
	phases       map[string]bool
}

func (m *countingModel) Complete(ctx context.Context, msgs []schema.Message, _ []runtime.ToolSpec) (*runtime.ModelReply, error) {
	m.calls++
	if runtime.CurrentStats(ctx) == nil {
		m.nilStats++
	}
	if p := runtime.CurrentPhase(ctx); p == "unknown" {
		m.unknownPhase++
	} else {
		if m.phases == nil {
			m.phases = map[string]bool{}
		}
		m.phases[p] = true
	}
	return &runtime.ModelReply{Content: "ok"}, nil
}

// TestComposeFallbackDraftMirrorsLanguage pins: a non-English
// question gets an explicit same-language instruction.
func TestComposeFallbackDraftMirrorsLanguage(t *testing.T) {
	st := &AgenticState{
		Question: "它是什么时候建造的？",
		KB:       &runtime.Kbinfos{Chunks: []map[string]any{{"content": "1874年建成。"}}},
	}
	mdl := &draftModel{reply: "1874年建成。\nMISSING: 无"}
	ComposeFallbackDraft(context.Background(), RAGTools{Model: mdl}, st)

	if !strings.Contains(mdl.lastSys, "same language") {
		t.Errorf("system = %q, want the same-language instruction for a CJK question", mdl.lastSys)
	}
}

func TestRunReturnsNonNilState(t *testing.T) {
	ctx := context.Background()
	mdl := &scriptedModel{}
	mdl.push(`{"slots":[{"id":0,"type":"aspect","question":"when built","clues":["1865"]},{"id":1,"type":"aspect","question":"where located","clues":["geneva"]}], "first_queries":["when built","where located"]}`)
	mdl.push("Built in 1865.")
	mdl.push("In Geneva.")
	mdl.push(`{"is_sufficient": true, "score": 0.9, "contradictions": [], "reasoning": "ok", "claims": {}}`)

	exec := newStubExecutor()
	exec.add("search_chunks", `{"hit":[{"doc_id":"d1","docnm_kwd":"doc1","content":"Built 1865, Geneva."}],"doc_aggs":[]}`, runtime.StatusOK)

	st, err := BuildAgenticGraph(ctx, RAGTools{
		Model: mdl,
		Tools: newToolset(exec),
		// The dual-channel fan-out retrieves directly, so it needs the search
		// config; without it the prefetch contributes nothing.
		Search: runtime.SearchDeps{Backend: &corpusRetriever{}, KbIDs: []string{"kb1"}, HasEmbedder: true},
		Logger: log.Default(),
	}, "When and where was it built?", "", 3, nil)
	if err != nil {
		t.Fatalf("BuildAgenticGraph: %v", err)
	}

	if st == nil {
		t.Fatal("expected a non-nil AgenticState")
	}
	// The round's deliverable is what it READ, not a rendered draft: the run has to have put
	// evidence in the pool (the retrieval legs ran) and its record has to name the slots it is
	// working on. `RagAnswer` used to be asserted here; it held the SCA-facing draft, which is gone
	// (see the note where RenderSlotDraft lived).
	if len(st.KB.Chunks) == 0 {
		t.Fatal("expected the round to gather evidence into the pool")
	}
	if len(st.SlotTable.State) == 0 {
		t.Fatal("expected the plan's slot table to travel with the round")
	}
	// What a completed run reports is its own record — the answer its session wrote and the part
	// it named as open (see routeResearch); the NoProgress flag the router used to carry is gone
	// with the query-rewrite node that was its only writer.
}

// TestAgenticGraphPushesPhaseProgress asserts that the agentic loop forwards tagged
// engine-stage lines to the caller's Progress sink as it runs — planner, research round,
// SCA — so a streaming chat can show live research progress in the reasoning block.
func TestAgenticGraphPushesPhaseProgress(t *testing.T) {
	ctx := context.Background()
	mdl := &scriptedModel{}
	mdl.push(`{"slots":[{"id":0,"type":"aspect","question":"when built","clues":["1865"]},{"id":1,"type":"aspect","question":"where located","clues":["geneva"]}], "first_queries":["when built","where located"]}`)
	mdl.push("Built in 1865.")
	mdl.push("In Geneva.")
	mdl.push(`{"is_sufficient": true, "score": 0.9, "contradictions": [], "reasoning": "ok", "claims": {}}`)

	exec := newStubExecutor()
	exec.add("search_chunks", `{"hit":[{"doc_id":"d1","docnm_kwd":"doc1","content":"Built 1865, Geneva."}],"doc_aggs":[]}`, runtime.StatusOK)

	var linesMu sync.Mutex
	var lines []string
	// Mirror production: the run's step reporter carries the think-block text.
	// Each stage declares its own step, so everything asserted here is a step a
	// node reported — never a log line that happened to match a pattern.
	//
	// Locked: a research round's retrieval legs run concurrently (see FanoutSearch), so a
	// step can arrive from more than one goroutine at the same time.
	sink := func(line string) {
		linesMu.Lock()
		lines = append(lines, line)
		linesMu.Unlock()
	}
	ctx = runtime.WithSteps(ctx, runtime.StepReporter{Text: sink})
	st, err := BuildAgenticGraph(ctx, RAGTools{
		Model:  mdl,
		Tools:  newToolset(exec),
		Search: runtime.SearchDeps{Backend: &corpusRetriever{}, KbIDs: []string{"kb1"}, HasEmbedder: true},
		Logger: log.Default(),
	}, "When and where was it built?", "", 3, nil)
	if err != nil {
		t.Fatalf("BuildAgenticGraph: %v", err)
	}

	if st == nil {
		t.Fatal("expected a non-nil AgenticState")
	}
	joined := strings.Join(lines, "\n")
	// Each stage is announced by its node's own tagged log line (no separate push), so
	// assert on what the nodes actually log.
	for _, want := range []string{
		"[Agentic RAG] Starting research",
		"[Planner] Split the question into",
		// prefetchSummary's opener has two branches (a plan larger than the opening
		// queries vs not), so assert the half both share plus the closing step.
		"up front.",
		"[Prefetch] Added",
		"[RAGAgent] Round 1 begins",
		"[Finalize] Finalizing",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("think-log lines missing %q; got:\n%s", want, joined)
		}
	}
	// Nesting: the prefetch's search legs are indented under the prefetch line —
	// that is what makes "what did this line run" readable — while the graph's own
	// nodes stay flush (this run has no outer rag call above them).
	if !strings.Contains(joined, runtime.StepIndentUnit+"[BM25 search] Searching by keyword for") {
		t.Errorf("the prefetch's legs must be indented under it; got:\n%s", joined)
	}
	if !strings.Contains(joined, "\n[Planner] Split the question into") {
		t.Errorf("the graph's own steps must stay flush; got:\n%s", joined)
	}
}

// TestAgenticGraphTakesOneRoundWhenTheSessionNamesNothing drives the research loop end to end:
// the session writes no <unresolved>, so ONE round is taken and the run closes out. The loop is the
// only cycle in the graph — it is why the graph is compiled in Pregel mode (DAG mode rejects
// cycles) — and it is entered by the session's own statement about what is still open.
func TestAgenticGraphTakesOneRoundWhenTheSessionNamesNothing(t *testing.T) {
	ctx := context.Background()
	mdl := &promptRoutedModel{}

	exec := newStubExecutor()
	exec.add("search_chunks", `{"hit":[{"doc_id":"d1","docnm_kwd":"doc1","content":"Built 1865, Geneva."}],"doc_aggs":[]}`, runtime.StatusOK)

	var buf bytes.Buffer
	var linesMu sync.Mutex
	var lines []string
	// Locked: the fan-out legs report their steps concurrently.
	sink := func(line string) {
		linesMu.Lock()
		lines = append(lines, line)
		linesMu.Unlock()
	}
	ctx = runtime.WithSteps(ctx, runtime.StepReporter{Text: sink})
	st, err := BuildAgenticGraph(ctx, RAGTools{
		Model:  mdl,
		Tools:  newToolset(exec),
		Search: runtime.SearchDeps{Backend: &corpusRetriever{}, KbIDs: []string{"kb1"}, HasEmbedder: true},
		Logger: log.New(&buf, "", 0),
	}, "When and where was it built?", "", 3, nil)
	if err != nil {
		t.Fatalf("BuildAgenticGraph: %v", err)
	}

	if st == nil {
		t.Fatal("expected a non-nil AgenticState")
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		"[RAGAgent] Round 1 begins",
		"[Finalize] Finalizing",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("graph did not run: missing %q; got:\n%s", want, joined)
		}
	}
	// This run takes ONE round and closes out, which is the correct outcome: the scripted session
	// wrote no answer and named no open part, so there is nothing for another round to aim at. What
	// the slot table holds does not enter that decision (see routeResearch), and the router's own
	// tests cover both answers: TestRouteResearchReopensOnAnAnswerThatNamesAnOpenPart and
	// TestRouteResearchTheSlotTableDoesNotDecide.
	if strings.Contains(joined, "[RAGAgent] Round 2 begins") {
		t.Errorf("a round was taken with no record asking for one:\n%s", joined)
	}
}

// TestBuildLowGraphRunsFormalizeThenDirectSearch covers the low-mode graph. It has no planner
// and no research-round loop, so the only observable contract is: formalize rewrites the
// question, then direct_search merges retrieved evidence into the kbinfos.
func TestBuildLowGraphRunsFormalizeThenDirectSearch(t *testing.T) {
	ctx := context.Background()
	mdl := &scriptedModel{}
	// The multi-turn formalize asks for the rewrite AND the keywords in one JSON
	// reply, so the rewrite below is what the node actually searches for.
	mdl.push(`{"question": "When was Culdcept released?", "keywords": "Culdcept, released"}`) // Formalize
	mdl.push("Culdcept was released in 1999.")                                                // compose

	kb := &runtime.Kbinfos{}
	sd := runtime.SearchDeps{Backend: &corpusRetriever{}, KbIDs: []string{"kb1"}, HasEmbedder: true}
	resp := &RunResponse{Mode: runtime.ResolveMode(&runtime.Toolset{ThinkingMode: "low"})}
	req := runtime.RunRequest{Question: "when was it made?", TopN: 8}

	var buf bytes.Buffer
	BuildLowGraph(ctx, RAGTools{
		Model: mdl,
		Messages: []schema.Message{
			{Role: schema.User, Content: "Culdcept was made by OmiyaSoft."},
			{Role: schema.User, Content: "when was it made?"},
		},
		Search: sd,
		Logger: log.New(&buf, "", 0),
	}, req, sd, kb, resp, log.New(&buf, "", 0))

	// direct_search ran and merged evidence into the shared kbinfos.
	if !kb.HasChunks() {
		t.Fatalf("direct_search did not deposit any chunk; log:\n%s", buf.String())
	}
	// formalize ran and rewrote the follow-up (the graph's first node), reported
	// under its own [Formalize] stage with BOTH the question asked and the one
	// actually searched.
	if want := `[Formalize] Rewrote the follow-up into a standalone question: "when was it made?" → "When was Culdcept released?"`; !strings.Contains(buf.String(), want) {
		t.Errorf("formalize_question node did not narrate its rewrite; log:\n%s", buf.String())
	}
}

// TestLogComposeDone pins the compose completion line: it is a LOG line, not a
// step — a compose finishes after the answer has begun streaming, where the chat
// pipeline drops think lines — and it counts RUNES, so a Chinese answer is not
// reported at three times its length.
func TestLogComposeDone(t *testing.T) {
	var buf bytes.Buffer
	logComposeDone(log.New(&buf, "", 0), time.Now(), "曹操是谁？", 3)
	got := buf.String()
	// 5 runes, 15 bytes: the line must say 5.
	if !strings.Contains(got, "[Composing the answer] composed 5 chars from 3 gathered passages in ") {
		t.Errorf("compose completion line = %q", got)
	}
	if strings.Contains(got, "15 chars") {
		t.Errorf("byte count leaked into the line: %q", got)
	}
	// An answer that came back empty is exactly the case this line exists for: it
	// must be reported, not skipped.
	var empty bytes.Buffer
	logComposeDone(log.New(&empty, "", 0), time.Now(), "", 0)
	if !strings.Contains(empty.String(), "composed 0 chars from 0 gathered passages") {
		t.Errorf("empty answer = %q", empty.String())
	}
}

// TestBuildLowGraphNarratesAnUnchangedQuestion covers the common case end to end:
// single-turn input is never rewritten, so the formalize step names the question
// as asked instead of claiming a rewrite that did not happen.
func TestBuildLowGraphNarratesAnUnchangedQuestion(t *testing.T) {
	ctx := context.Background()
	mdl := &scriptedModel{}
	mdl.push(`{"keywords": "Culdcept, released"}`) // single-turn: keywords only
	mdl.push("Culdcept was released in 1999.")     // compose

	kb := &runtime.Kbinfos{}
	sd := runtime.SearchDeps{Backend: &corpusRetriever{}, KbIDs: []string{"kb1"}, HasEmbedder: true}
	resp := &RunResponse{Mode: runtime.ResolveMode(&runtime.Toolset{ThinkingMode: "low"})}
	req := runtime.RunRequest{Question: "when was Culdcept made?", TopN: 8}

	var buf bytes.Buffer
	BuildLowGraph(ctx, RAGTools{
		Model: mdl,
		Messages: []schema.Message{
			{Role: schema.User, Content: "when was Culdcept made?"},
		},
		Search: sd,
		Logger: log.New(&buf, "", 0),
	}, req, sd, kb, resp, log.New(&buf, "", 0))

	if want := `[Formalize] Kept the question as asked: "when was Culdcept made?"`; !strings.Contains(buf.String(), want) {
		t.Errorf("formalize did not name the unchanged question; log:\n%s", buf.String())
	}
	if strings.Contains(buf.String(), "Rewrote") {
		t.Errorf("no rewrite happened, the step must not say so; log:\n%s", buf.String())
	}
}

// TestFanoutSummary pins what the planner reports: the sub-questions themselves,
// not an announcement that it is about to decompose. The fallback case (the model
// refused to decompose, so the question is searched as asked) must not be dressed
// up as a sub-question.
//
// The wording carries no "first-hop": a reader has no hop to count, and these are
// just the questions the plan set out to research.
func TestFanoutSummary(t *testing.T) {
	cases := []struct {
		name     string
		question string
		fanouts  []string
		want     string
	}{
		{"three", "曹操是谁？", []string{"曹操是谁", "曹操生平简介", "曹操历史地位"},
			`Split the question into 3 sub-questions to research: "曹操是谁", "曹操生平简介", "曹操历史地位".`},
		{"one-sub-question", "曹操是谁？", []string{"曹操"},
			`Split the question into 1 sub-question to research: "曹操".`},
		{"kept-as-asked", "曹操是谁？", []string{"曹操是谁？"},
			"The question is already a single searchable sub-question; searching it as asked."},
		{"nothing", "曹操是谁？", nil,
			"Could not decompose the question; searching it as asked."},
	}
	for _, tc := range cases {
		if got := fanoutSummary(tc.question, tc.fanouts); got != tc.want {
			t.Errorf("%s: fanoutSummary = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestFinalizeSummary pins the finalize step's four states, including the evidence
// size on the two that have evidence to report. The compose step no longer repeats
// the count (it is suppressed after this node), so this line is where the reader
// learns what the answer is being written from.
func TestFinalizeSummary(t *testing.T) {
	cases := []struct {
		name           string
		partial, empty bool
		chunks         int
		want           string
	}{
		{"complete", false, false, 5, "Finalizing a complete answer from 5 passages."},
		{"complete-singular", false, false, 1, "Finalizing a complete answer from 1 passage."},
		{"partial", true, false, 5, "Finalizing a partial answer from 5 passages; some gaps remain unanswered."},
		{"empty", false, true, 0, "Finalizing with no supporting evidence: the answer will say so."},
		{"partial-and-empty", true, true, 0,
			"Finalizing a partial answer with no supporting evidence: research exhausted its attempts."},
	}
	for _, tc := range cases {
		if got := finalizeSummary(tc.partial, tc.empty, tc.chunks); got != tc.want {
			t.Errorf("%s: finalizeSummary = %q, want %q", tc.name, got, tc.want)
		}
	}
	if got := chunkCount(nil); got != 0 {
		t.Errorf("chunkCount(nil) = %d, want 0 (a failed graph still reports Finalize)", got)
	}
}

// TestPrefetchSummary pins the two numbers a reader sees back to back in high/ultra
// mode: what the planner decomposed the question into, and how many queries the
// upfront search runs (initialize_state caps first_queries at three, the rest stay
// as slots). A bare "searching 3" after "split into 5" reads like two sub-questions
// were dropped.
//
// The sentence must NOT call those queries the plan's sub-questions: they are the
// slot table's own first_queries, written by a model that only sees the plan as a
// hint and may reword it (see prefetchSummary). The two numbers are still both
// named, which is the property this test protects.
func TestPrefetchSummary(t *testing.T) {
	cases := []struct {
		name               string
		planned, searching int
		want               string
	}{
		{"all-searched", 3, 3, "Searching 3 opening queries up front."},
		{"singular", 1, 1, "Searching 1 opening query up front."},
		// The cap: the plan is bigger than the opening queries, so the line names
		// the plan's total (the rest stay as slots for later rounds).
		{"capped", 5, 3, "The plan lists 5 sub-questions; searching 3 opening queries up front."},
		{"capped-singular", 2, 1, "The plan lists 2 sub-questions; searching 1 opening query up front."},
		// Decomposition failed: no plan, one query as asked.
		{"no-plan", 0, 1, "Searching 1 opening query up front."},
	}
	for _, tc := range cases {
		if got := prefetchSummary(tc.planned, tc.searching); got != tc.want {
			t.Errorf("%s: prefetchSummary = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// The evidence-prefill summary and its test are gone with the session fan-out: a round opens
// ONE session, so "opening N research sessions" no longer names anything, and the slot table
// that could count "answered slots" is gone too (see RunSlotResearchPass).

// TestRagRoundEndLine pins the round's closing sentence, including the zero case:
// "0 slots still unresolved" read as a double negative.
//
// The clause says "slots", not "sub-questions": the number is the slot table's
// backlog, and the plan's sub-questions are a different count printed earlier in
// the same block.
func TestRagRoundEndLine(t *testing.T) {
	cases := []struct {
		name                                 string
		round, newPassages, pool, unresolved int
		want                                 string
	}{
		{"zero-unresolved", 1, 0, 6, 0,
			"Round 1 finished: 0 new passages added; the pool now holds 6 passages, and no unresolved slots."},
		{"some-unresolved", 2, 0, 6, 2,
			"Round 2 finished: 0 new passages added; the pool now holds 6 passages, and 2 slots still unresolved."},
		{"one-added-one-open", 3, 1, 7, 1,
			"Round 3 finished: 1 new passage added; the pool now holds 7 passages, and 1 slot still unresolved."},
	}
	for _, tc := range cases {
		if got := ragRoundEndLine(tc.round, tc.newPassages, tc.pool, tc.unresolved); got != tc.want {
			t.Errorf("%s: ragRoundEndLine = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestFormalizeStepLine pins the formalize step's wording per outcome: a question
// kept as asked names the question, a rewritten one gets the pair (asked →
// searched), and only a formalize that produced no question stays silent.
//
// The node used to announce `Formalized the question into "曹操是谁？".` for every
// outcome — which reads as a rewrite that did not happen, since single-turn runs
// never rewrite and the multi-turn prompt asks for the question unchanged "in
// most cases".
func TestFormalizeStepLine(t *testing.T) {
	cases := []struct {
		name       string
		asAsked    string
		standalone string
		want       string
	}{
		{"unchanged", "曹操是谁？", "曹操是谁？", `Kept the question as asked: "曹操是谁？"`},
		{"only-whitespace-differs", "曹操是谁？", "  曹操是谁？ ", `Kept the question as asked: "曹操是谁？"`},
		{"empty-standalone", "曹操是谁？", "", ""},
		{"nothing-asked-either", "", "", ""},
		{"rewritten", "他死于哪年？", "曹操死于哪年？",
			`Rewrote the follow-up into a standalone question: "他死于哪年？" → "曹操死于哪年？"`},
		{"nothing-asked", "", "曹操是谁？", `Standalone question for this turn: "曹操是谁？"`},
	}
	for _, tc := range cases {
		got := formalizeStepLine(tc.asAsked, tc.standalone)
		if got != tc.want {
			t.Errorf("%s: formalizeStepLine = %q, want %q", tc.name, got, tc.want)
		}
		if strings.HasSuffix(got, "。") || strings.HasSuffix(got, "？.") || strings.HasSuffix(got, "?.") {
			t.Errorf("%s: %q ends on a doubled terminator", tc.name, got)
		}
	}
}

// lowGraphCountingExpander counts compiled-expansion calls (runtime.CompiledExpander).
type lowGraphCountingExpander struct{ calls int }

func (c *lowGraphCountingExpander) Expand(context.Context, *runtime.Kbinfos, string, string, []string) error {
	c.calls++
	return nil
}

// TestBuildLowGraphAlwaysExpandsCompiled: mode: direct.py
// calls hybrid_search(..., use_compiled=True) unconditionally, so the low
// graph's direct_search must expand even when the caller's RunRequest leaves
// UseCompiled false — which production always does.
func TestBuildLowGraphAlwaysExpandsCompiled(t *testing.T) {
	ctx := context.Background()
	kb := &runtime.Kbinfos{}
	exp := &lowGraphCountingExpander{}
	sd := runtime.SearchDeps{Backend: &corpusRetriever{}, KbIDs: []string{"kb1"}, HasEmbedder: true, Expand: exp}
	resp := &RunResponse{Mode: runtime.ResolveMode(&runtime.Toolset{ThinkingMode: "low"})}
	req := runtime.RunRequest{Question: "when was it made?", TopN: 8, UseCompiled: false}

	var buf bytes.Buffer
	if err := BuildLowGraph(ctx, RAGTools{Search: sd}, req, sd, kb, resp, log.New(&buf, "", 0)); err != nil {
		t.Fatalf("BuildLowGraph: %v", err)
	}
	if exp.calls == 0 {
		t.Errorf("low mode's direct_search must expand compiled structure; log:\n%s",
			buf.String())
	}
}

// TestFinalizeRunsOnceFromInsideTheGraph locks in the terminal node's contract: both
// graphs end in a formalize_answer node that composes, so the composition happens INSIDE
// the graph — and exactly once, even though Rag also has a post-graph call for runs that
// never reach the last node. A double composition would overwrite the answer with the
// model's next reply, which is what this test watches for.
func TestFinalizeRunsOnceFromInsideTheGraph(t *testing.T) {
	ctx := context.Background()
	finalized := 0
	resp := &RunResponse{Mode: runtime.ResolveMode(&runtime.Toolset{ThinkingMode: "low"})}
	kb := &runtime.Kbinfos{}
	sd := runtime.SearchDeps{Backend: &corpusRetriever{}, KbIDs: []string{"kb1"}, HasEmbedder: true}
	req := runtime.RunRequest{Question: "when was it made?", TopN: 8}
	deps := RAGTools{
		Model:  &scriptedModel{},
		Search: sd,
	}
	// Mirror Rag: the hook is idempotent, so the graph's node and the
	// post-graph safety net cannot both compose.
	composed := false
	deps.Finalize = func(_ context.Context, _ bool, _ bool, question string) {
		if composed {
			return
		}
		composed = true
		finalized++
		// The low graph's formalize_answer must forward the FORMALIZED
		// question the formalize_question node wrote into the RunRequest
		// composing
		// from the outer tool argument collapses a multi-hop answer to its
		// first sub-answer.
		if question != "when was it made?" {
			t.Errorf("Finalize question = %q, want the formalized RunRequest question", question)
		}
	}

	var buf bytes.Buffer
	if err := BuildLowGraph(ctx, deps, req, sd, kb, resp, log.New(&buf, "", 0)); err != nil {
		t.Fatalf("BuildLowGraph: %v", err)
	}
	// The graph's last node composed...
	if finalized != 1 {
		t.Fatalf("formalize_answer node composed %d time(s), want 1; log:\n%s", finalized, buf.String())
	}
	// ...and the post-graph call is then a no-op.
	deps.Finalize(ctx, false, false, "")
	if finalized != 1 {
		t.Errorf("composition ran %d time(s) after the post-graph call, want 1", finalized)
	}
}

// TestUnresolvedClueGapsCapsAtTwoClues pins the [:2] per-slot cap.
func TestUnresolvedClueGapsCapsAtTwoClues(t *testing.T) {
	st := &AgenticState{UnresolvedSlots: []map[string]any{
		{"question_clues": []string{"a", "b", "c"}},
		{"question_clues": []string{"a", "d"}}, // clues are appended without dedupe
	}}
	gaps := unresolvedClueGaps(st)
	if len(gaps) != 4 {
		t.Fatalf("gaps = %d, want 4 (first two clues of each slot, duplicates kept)", len(gaps))
	}
	if gaps[0].What != "a" || gaps[0].SearchHint != "a" {
		t.Errorf("gaps[0] = %+v, want what=hint=\"a\"", gaps[0])
	}
	if gaps[2].What != "a" || gaps[3].What != "d" {
		t.Errorf("gaps = %+v, want the second slot's clues a,d appended verbatim", gaps)
	}
}

func TestAgenticNodeNameMapsRoutingToGraphKeys(t *testing.T) {
	cases := map[agenticNode]string{
		nodeRagAgentLoop:    "rag_agent",
		nodeFormalizeAnswer: "formalize_answer",
	}
	for n, want := range cases {
		if got := agenticNodeName(n); got != want {
			t.Errorf("agenticNodeName(%d) = %q, want %q", n, got, want)
		}
	}
}

// Explicit-wiring integration tests (replaces the old init()-based registration checks).
// Activation is explicit: the test registers the loop before calling Run.

// TestMain keeps the package's tests on in-memory doubles: graph exploration
// otherwise seeds its dense search from the tenant embedding model, which
// reaches a database these tests do not have. Returning nil keeps the keyword
// fallback, which is the path under test.
func TestMain(m *testing.M) {
	runtime.SetSeedEncoder(func(context.Context, string, string) []float64 { return nil })
	os.Exit(m.Run())
}

func TestNewAgenticLoopDrivesHarnessRun(t *testing.T) {
	SetAgenticLoop(NewAgenticLoop())
	defer SetAgenticLoop(nil)

	facts := `{"fact":"Saint Lawrence River; 14 April 1865; 9 December 2019."}`
	if err := json.Unmarshal([]byte(facts), &struct{}{}); err != nil {
		t.Fatalf("bad fixture: %v", err)
	}

	mdl := &scriptedModel{}
	mdl.push(`{"slots":[{"id":0,"type":"aspect","question":"when opened","clues":["1865"]},{"id":1,"type":"aspect","question":"which river","clues":["river"]}], "first_queries":["when opened","which river"]}`)
	mdl.push("It opened on 14 April 1865.")
	mdl.push("The Saint Lawrence River.")
	mdl.push(`{"is_sufficient": true, "score": 0.9, "contradictions": [], "reasoning": "ok", "claims": {}}`)

	deps := RAGTools{
		Retriever: &countingRetriever{},
		Model:     mdl,
		Prompts:   &fakePrompts{},
	}

	got := runHarnessRun(t, deps, "When did it open and on which river?")
	if got == nil {
		t.Fatal("expected a non-nil RunResponse")
	}
	if got.Mode.Label != "high" {
		t.Fatalf("expected high mode, got %q", got.Mode.Label)
	}
	// The agentic loop routed through the planner + slot research; at minimum the
	// response is produced without error.
	if len(got.Chunks) == 0 {
		t.Fatalf("expected >=1 chunk (retriever stub fills KB), got %d", len(got.Chunks))
	}
}

// rfRetriever records the rank feature every retrieve call carried.
type rfRetriever struct {
	mu        sync.Mutex
	features  []map[string]float64
	queries   []string
	docScopes [][]string
}

func (r *rfRetriever) Retrieve(_ context.Context, req runtime.RetrieveRequest) ([]map[string]any, error) {
	// Locked for the same reason channelRetriever is: the fan-out legs run concurrently.
	r.mu.Lock()
	r.features = append(r.features, req.RankFeature)
	r.queries = append(r.queries, req.Query)
	r.docScopes = append(r.docScopes, req.DocScope)
	r.mu.Unlock()
	return []map[string]any{{
		"chunk_id": "c1",
		"content":  "The bridge opened in 1865.",
		"doc_id":   "d1",
		"doc_name": "Bridge",
	}}, nil
}

type rfTagger struct{}

func (rfTagger) LabelQuestion(_ context.Context, _ string, _ []*entity.Knowledgebase) map[string]float64 {
	return map[string]float64{"location": 1.0}
}

// TestAgenticLoopRankFeaturePolicy pins the rank_feature policy: the search legs (the
// fan-out channels, search_chunks, retrieve) call the retriever WITHOUT rank_feature, and
// only the low-mode direct pass passes the question-type tag boost. The agentic loop's
// research retrievals must therefore carry NO tag boost, even though the Tagger/KBs
// projection is wired through.
func TestAgenticLoopRankFeaturePolicy(t *testing.T) {
	SetAgenticLoop(NewAgenticLoop())
	defer SetAgenticLoop(nil)

	mdl := &scriptedModel{}
	// Call order: formalize (single-turn keyword extraction) → planner fan-out → slot
	// table → draft → SCA.
	mdl.push(`{"entity": ["it"], "aliases": [], "fact_type": [], "qualifiers": []}`)
	mdl.push(`{"slots":[{"id":0,"type":"aspect","question":"when opened","clues":["1865"]}], "first_queries":["when opened"]}`)
	mdl.push("It opened in 1865.")
	mdl.push(`{"is_sufficient": true, "score": 0.9, "contradictions": [], "reasoning": "ok", "claims": {}}`)

	r := &rfRetriever{}
	got := runHarnessRun(t, RAGTools{
		Retriever: r,
		Model:     mdl,
		Prompts:   &fakePrompts{},
		KBs:       []*entity.Knowledgebase{{}},
		Tagger:    rfTagger{},
	}, "When did it open?")
	if got == nil {
		t.Fatal("expected a non-nil RunResponse")
	}
	if len(r.features) == 0 {
		t.Fatal("the agentic loop performed no retrieval")
	}
	// Scope to the fan-out / slot research retrievals — the hybrid/bm25 legs, none of
	// which pass rank_feature.
	//
	// Also exempt: the navigation tools' own recalls (chunk-agg routing via
	// chunkAggRetrieveFrom) — neither passes rank_feature there either.
	research := map[string]bool{"when was it opened": true, "when opened": true}
	checked := 0
	for i, q := range r.queries {
		if !research[q] {
			continue
		}
		checked++
		if len(r.features[i]) != 0 {
			t.Errorf("retrieve %q RankFeature = %v, want nil (search.py legs never pass rank_feature)", q, r.features[i])
		}
	}
	if checked == 0 {
		t.Fatalf("no fan-out/slot retrieval was performed (saw %v); assertion would be vacuous", r.queries)
	}
}

func TestNewAgenticLoopDegradesWithoutModel(t *testing.T) {
	SetAgenticLoop(NewAgenticLoop())
	defer SetAgenticLoop(nil)

	deps := RAGTools{
		Retriever: &countingRetriever{},
		Model:     nil, // no model => direct fallback
		Prompts:   &fakePrompts{},
	}

	got := runHarnessRun(t, deps, "Any question?")
	if got == nil {
		t.Fatal("expected a non-nil RunResponse")
	}
	// With no model the loop falls back to a single direct retrieval; no slots.
	if len(got.Slots) != 0 {
		t.Fatalf("expected 0 slots when no model, got %d", len(got.Slots))
	}
	if len(got.Chunks) == 0 {
		t.Fatalf("expected >=1 chunk from direct fallback, got %d", len(got.Chunks))
	}
}

func TestNewAgenticLoopRoundsToggleDeactivates(t *testing.T) {
	// After deregistering (nil), Run must NOT enter the agentic loop.
	SetAgenticLoop(nil)

	mdl := &scriptedModel{}
	deps := RAGTools{
		Retriever: &countingRetriever{},
		Model:     mdl,
		Prompts:   &fakePrompts{},
	}
	got := runHarnessRun(t, deps, "Question?")
	if got == nil {
		t.Fatal("expected a non-nil RunResponse")
	}
	// In single-session fallback the planner fan-out prompt is never issued, so
	// the scripted model's first reply is never consumed.
	if mdl.idx != 0 {
		t.Fatalf("expected agentic planner NOT to run (model idx %d, want 0)", mdl.idx)
	}
	_ = got
}

// runHarnessRun drives Run in high mode and returns the response. It is
// the Go analogue of dialog_service.py invoking RAGTools for an agentic answer.
func runHarnessRun(t *testing.T, deps RAGTools, question string) *RunResponse {
	t.Helper()
	req := runtime.RunRequest{
		Question:     question,
		DatasetIDs:   []string{"kb1"},
		ThinkingMode: "high",
	}
	resp := Rag(context.Background(), deps, req)
	if resp == nil {
		t.Fatalf("Run returned nil response")
	}
	return resp
}

// strPtr / fltPtr / fakePrompts are tiny helpers shared across the tests.
func strPtr(s string) *string { return &s }

type fakePrompts struct{}

func (f *fakePrompts) Load(name string) (string, error) { return "", nil }

// Test doubles for the RAGTools method tests. Kept local to the agentic_rag package so
// the tests can call Run/ComposeAnswer (which live here) without importing agent
// from the runtime test package (that would be an import cycle).

// TestComposeFinalAnswerUsesTheSessionAnswerOnTheStreamingPath pins the fix for the funnel every
// production answer goes through.
//
// The session-answer check used to live inside ComposeAnswerWith, which production never reaches:
// with an AnswerSink the run streams (ComposeAnswerStream) and returns from that branch. So 15 of
// 18 answered requests had a session answer that was silently replaced by a compose call which had
// not read the passages — the answers then cited the six blocks that call renders (rendered_blocks
// = 6) while the pool held up to 213 passages, and six questions came back "the evidence does not
// contain it" (measured 2026-09-20).
//
// The funnel must therefore take the session's answer BEFORE it chooses a path, publish the
// session's own registry as the citation list (its [ID:n] markers index into those numbers), and
// hand the text to the sink so a streaming client still receives it.
func TestComposeFinalAnswerUsesTheSessionAnswerOnTheStreamingPath(t *testing.T) {
	// The session writes prose; its numbers are dropped and the run's evidence list is what carries them
	// (see useSessionAnswer).
	// The handle is the model's own citation of the passage it read; it is renumbered (here it is already
	// the first cited passage), never dropped — dropping what the model cited is how an answer ends up
	// with no citations at all (see useSessionAnswer).
	const sessionAnswer = "关羽 did the deed [ID:0]"
	const deliveredAnswer = "关羽 did the deed [ID:0]"
	kb := &runtime.Kbinfos{
		Chunks:              []map[string]any{{"chunk_id": "c-hua", "content": "云长提华雄之头"}},
		SessionAnswer:       sessionAnswer,
		SessionEvidenceRefs: []string{"c-hua"},
	}
	var delivered []string
	sink := &AnswerSink{OnDelta: func(delta string, isThink bool) { delivered = append(delivered, delta) }}
	model := &fakeModel{}
	deps := RAGTools{Model: model, AnswerSink: sink}
	resp := &RunResponse{}

	composeFinalAnswer(context.Background(), deps, runtime.RunRequest{Question: "谁斩了华雄"},
		kb, resp, nil, true, false, "谁斩了华雄")

	if resp.Answer != deliveredAnswer {
		t.Errorf("Answer = %q, want the session's own prose with the handle it cited renumbered", resp.Answer)
	}
	if len(resp.CiteChunkIDs) != 1 || resp.CiteChunkIDs[0] != "c-hua" {
		t.Errorf("CiteChunkIDs = %v, want the session's registry", resp.CiteChunkIDs)
	}
	if !resp.Partial {
		t.Error("Partial = false; the graph's partial flag must be reflected back")
	}
	if len(delivered) != 1 || delivered[0] != deliveredAnswer {
		t.Errorf("sink received %v, want the answer delivered in one piece", delivered)
	}
	if model.messages != nil {
		t.Error("the model was called: an answer the session already wrote must not be re-composed")
	}
}

// TestSessionAnswerIsRefusedOnlyForAbstentionOrAnEmptyPool is the negative half: the shortcut is
// not a way to skip composition when there is no evidence, or when the run abstained.
//
// It is gated on the POOL and not on the caller's empty_result flag: that flag is the compose
// prompt's no-evidence hedge, and the graph passes it TRUE by construction on every round, so
// gating on it suppressed this answer in every run (measured 2026-09-20, 19:07: 20 of 20 rounds
// wrote an answer and "Using the answer the research session wrote" was still 0).
func TestSessionAnswerIsRefusedOnlyForAbstentionOrAnEmptyPool(t *testing.T) {
	kb := &runtime.Kbinfos{
		Chunks:        []map[string]any{{"chunk_id": "c1", "content": "x"}},
		SessionAnswer: "an answer",
	}
	if _, ok := sessionAnswer(kb, true); ok {
		t.Error("abstain: the session answer must not be used")
	}
	emptyPool := &runtime.Kbinfos{SessionAnswer: "an answer"}
	if _, ok := sessionAnswer(emptyPool, false); ok {
		t.Error("no evidence: the session answer must not be used")
	}
	blank := &runtime.Kbinfos{
		Chunks:        []map[string]any{{"chunk_id": "c1"}},
		SessionAnswer: "   \n ",
	}
	if _, ok := sessionAnswer(blank, false); ok {
		t.Error("whitespace answer: nothing to use")
	}
	if _, ok := sessionAnswer(nil, false); ok {
		t.Error("nil pool: nothing to use")
	}
	if ans, ok := sessionAnswer(kb, false); !ok || ans != "an answer" {
		t.Errorf("usable pool: (%q, %v), want the answer", ans, ok)
	}

	// The reason a written answer was skipped is reported, so a run can say why.
	if why := sessionAnswerBlocked(emptyPool, false); why != "the evidence pool is empty" {
		t.Errorf("blocked reason = %q, want the empty-pool reason", why)
	}
	if why := sessionAnswerBlocked(kb, true); why != "the run abstained" {
		t.Errorf("blocked reason = %q, want the abstention reason", why)
	}
	if why := sessionAnswerBlocked(kb, false); why != "" {
		t.Errorf("blocked reason = %q, want none for a usable answer", why)
	}
}

// fakeModel replays a scripted sequence of replies, so the session's control
// flow can be driven without a provider.
type fakeModel struct {
	replies  []*runtime.ModelReply
	calls    int
	messages []schema.Message
}

func (f *fakeModel) Complete(_ context.Context, msgs []schema.Message, _ []runtime.ToolSpec) (*runtime.ModelReply, error) {
	f.messages = msgs
	// The pre-flight probe (probeChatModel) opens every non-naive run with one
	// minimal "ping". It is infrastructure, not script: answer it without
	// consuming a scripted reply, so a test's reply sequence stays what the test
	// wrote. A test that wants the PROBE itself to fail uses errModel instead.
	if len(msgs) == 1 && msgs[0].Role == schema.User && msgs[0].Content == probePrompt {
		return &runtime.ModelReply{Content: "ok"}, nil
	}
	if f.calls >= len(f.replies) {
		return &runtime.ModelReply{Content: `<state>{"new_states": []}</state>`}, nil
	}
	r := f.replies[f.calls]
	f.calls++
	return r, nil
}

// lastUserPrompt returns the content of the most recent user message, so tests
// can assert on the prompt the model actually received.
func (f *fakeModel) lastUserPrompt() string {
	for i := len(f.messages) - 1; i >= 0; i-- {
		if f.messages[i].Role == schema.User {
			return f.messages[i].Content
		}
	}
	return ""
}

// corpusRetriever answers every query from a fixed corpus.
type corpusRetriever struct {
	mu    sync.Mutex
	calls []string
}

func (c *corpusRetriever) Retrieve(_ context.Context, req runtime.RetrieveRequest) ([]map[string]any, error) {
	// Locked: a fan-out's legs (and the search executor's own legs) run concurrently.
	c.mu.Lock()
	c.calls = append(c.calls, req.Query)
	c.mu.Unlock()
	return []map[string]any{{
		"chunk_id":   "c1",
		"content":    "Culdcept was created by OmiyaSoft and released in 1999.",
		"doc_id":     "doc-culdcept",
		"docnm_kwd":  "Culdcept History",
		"dataset_id": "kb1",
	}}, nil
}

// emptyRetriever simulates a corpus with no matches.
type emptyRetriever struct{}

func (emptyRetriever) Retrieve(_ context.Context, _ runtime.RetrieveRequest) ([]map[string]any, error) {
	return nil, nil
}

func TestRunLowModeIsSinglePass(t *testing.T) {
	r := &corpusRetriever{}
	resp := Rag(context.Background(), RAGTools{Retriever: r}, runtime.RunRequest{
		Question:     "Who created Culdcept?",
		ThinkingMode: "low",
		DatasetIDs:   []string{"kb1"},
		UseCompiled:  true,
	})
	if resp.EmptyResult {
		t.Fatal("low mode must retrieve evidence")
	}
	if len(resp.Chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(resp.Chunks))
	}
	if len(r.calls) != 1 {
		t.Errorf("retrieval calls = %d, want 1 (low is a single pass)", len(r.calls))
	}
	if resp.Mode.Agentic {
		t.Error("low must resolve to a non-agentic spec")
	}
	if len(resp.DocAggs) != 1 || resp.DocAggs[0]["doc_id"] != "doc-culdcept" {
		t.Fatalf("doc_aggs = %v", resp.DocAggs)
	}
	if resp.DocAggs[0]["doc_name"] != "Culdcept History" {
		t.Errorf("doc_name = %v, want 'Culdcept History'", resp.DocAggs[0]["doc_name"])
	}
	if runtime.MemorySize(resp.Kbinfos) != 1 {
		t.Errorf("memory size = %d, want 1", runtime.MemorySize(resp.Kbinfos))
	}
}

func TestRunUnknownModeFallsBackToNaive(t *testing.T) {
	r := &corpusRetriever{}
	resp := Rag(context.Background(), RAGTools{Retriever: r}, runtime.RunRequest{
		Question:     "Who created Culdcept?",
		ThinkingMode: "not-a-real-mode",
		DatasetIDs:   []string{"kb1"},
	})
	if resp.Mode.Label != "naive" {
		t.Errorf("mode = %q, want naive", resp.Mode.Label)
	}
	if resp.EmptyResult {
		t.Error("naive must still retrieve (it degrades, not fails)")
	}
}

// TestNaiveUsesFlatEvidenceAndOwnComposition pins the semantics that separate the naive
// path from low/direct. Naive is NOT "direct_search minus formalize": it does a plain
// retrieve with no weighted keyword extraction and composes under a short fixed system
// prompt from flat "[i] content" evidence, never FinalAnswerSystem / kb_prompt. Routing
// naive through runDirect + composeFinalAnswer silently
// substituted the agentic composition, which is what this test guards.
func TestNaiveUsesFlatEvidenceAndOwnComposition(t *testing.T) {
	mdl := &fakeModel{replies: []*runtime.ModelReply{{Content: "OmiyaSoft [1]."}}}
	r := &corpusRetriever{}
	resp := Rag(context.Background(), RAGTools{Retriever: r, Model: mdl}, runtime.RunRequest{
		Question:     "Who created Culdcept?",
		ThinkingMode: "not-a-real-mode",
		DatasetIDs:   []string{"kb1"},
		UseCompiled:  true, // naive must ignore compiled expansion
	})

	if resp.Mode.Label != "naive" {
		t.Fatalf("mode = %q, want naive", resp.Mode.Label)
	}
	if resp.EmptyResult || len(resp.Chunks) != 1 {
		t.Fatalf("naive must retrieve evidence (chunks=%d, empty=%v)", len(resp.Chunks), resp.EmptyResult)
	}
	// one PLAIN retrieve — no ExtractWeightedKeywords, so the model
	// is called exactly once. The old runDirect path spent a second call on
	// keyword extraction before composing.
	if mdl.calls != 1 {
		t.Fatalf("model calls = %d, want 1 (naive composes without extracting keywords)", mdl.calls)
	}

	// flat "[i] content" evidence, not kb_prompt's "ID: n" blocks.
	got := mdl.lastUserPrompt()
	if !strings.Contains(got, "[1] Culdcept was created") {
		t.Errorf("evidence = %q, want flat \"[1] <content>\"", got)
	}
	if strings.Contains(got, "ID: 1") {
		t.Errorf("evidence = %q, must not use kb_prompt's \"ID: n\" blocks", got)
	}
	if resp.Answer != "OmiyaSoft [1]." {
		t.Errorf("answer = %q, want the naive composition", resp.Answer)
	}
}

func TestRunAgenticDegradesToDirectWithoutModel(t *testing.T) {
	r := &corpusRetriever{}
	resp := Rag(context.Background(), RAGTools{Retriever: r}, runtime.RunRequest{
		Question:     "Who created Culdcept?",
		ThinkingMode: "high",
		DatasetIDs:   []string{"kb1"},
	})
	if resp.EmptyResult {
		t.Error("agentic mode without a model must degrade to direct search")
	}
}

// TestRouteResearchOutputIsInEveryBranchsDeclaredEnds pins the invariant Eino enforces and the
// invariant a merged router broke: Eino ABORTS the whole run when a branch returns a node that
// branch does not declare ("unintended end node"), and the question is then answered by a fallback
// composition that never saw the research. With the rewrite branch gone there is one router and one
// branch, and the ends it may name are that branch's own.
//
// The set is a literal here on purpose: this test exists to fail when the graph's declaration
// changes and the router's outputs stop fitting inside it.
func TestRouteResearchOutputIsInEveryBranchsDeclaredEnds(t *testing.T) {
	ragAgentEnds := map[string]bool{"stop": true, "rag_agent": true, "formalize_answer": true}

	for _, tc := range []struct {
		name  string
		state func() *AgenticState
	}{
		{"another round", func() *AgenticState {
			st := NewAgenticState("When and where was it built?", "", 3, nil)
			st.SessionUnresolved = "the year it was built"
			st.LastRoundNew = 40
			st.Deadline = time.Now().Add(150 * time.Second)
			return st
		}},
		{"closing out", func() *AgenticState {
			st := NewAgenticState("When and where was it built?", "", 3, nil)
			st.CollectedAnswer = "It was built in Geneva."
			st.LastRoundNew = 40
			st.Deadline = time.Now().Add(150 * time.Second)
			return st
		}},
	} {
		got := agenticNodeName(routeResearch(tc.state(), 3))
		if !ragAgentEnds[got] {
			t.Errorf("%s: routeResearch returned %q, which rag_agent does not declare", tc.name, got)
		}
	}
}

// growingRetriever returns a NEW chunk on every call.
//
// A fixed stub can never drive a second round: routeResearch reads LastRoundNew (the passages the
// round ADDED), so a retriever that keeps returning the same chunk makes every round look stalled
// and the loop closes out after one. Tests that need the cycle have to make the evidence grow.
type growingRetriever struct {
	mu sync.Mutex
	n  int
}

func (g *growingRetriever) Retrieve(_ context.Context, _ runtime.RetrieveRequest) ([]map[string]any, error) {
	g.mu.Lock()
	g.n++
	n := g.n
	g.mu.Unlock()
	return []map[string]any{{
		"doc_id":    fmt.Sprintf("d%d", n),
		"docnm_kwd": fmt.Sprintf("doc%d", n),
		"chunk_id":  fmt.Sprintf("c%d", n),
		"content":   "关羽斩华雄于汜水关；又斩颜良、文丑。[doc " + fmt.Sprintf("%d", n) + "]",
	}}, nil
}

// NOTE: an END-TO-END two-round test would need a session whose retrieval actually lands in the pool
// (a stubbed toolset returns a payload that never reaches it, so every round looks stalled and the loop
// closes out after one). It is not written here; the router fix is pinned by
// TestRouteResearchContinueTargetIsTheBranchesOwn and
// TestRouteResearchOutputIsInEveryBranchsDeclaredEnds, and the real proof is the next benchmark run.

func TestRunAgenticSeedsSlotsAndRunsSession(t *testing.T) {
	r := &corpusRetriever{}
	mdl := &fakeModel{replies: []*runtime.ModelReply{
		{Content: `{"slots": [{"id": 0, "type": "entity", "clues": ["creator"]}, {"id": 1, "type": "date", "clues": ["released"]}], "first_queries": ["creator"]}`},
		{Content: "", ToolCalls: []runtime.ToolCall{{ID: "c0", Name: "retrieve", Args: map[string]any{"query": []any{"Culdcept creator"}}}}},
		{Content: `<state>{"new_states":[{"state":[{"id":0,"candidate":"OmiyaSoft","candidate_strength":0.95}]}]}</state>`},
	}}
	resp := Rag(context.Background(), RAGTools{Retriever: r, Model: mdl}, runtime.RunRequest{
		Question:     "Who created Culdcept and when?",
		ThinkingMode: "medium",
		DatasetIDs:   []string{"kb1"},
	})
	if resp.EmptyResult {
		t.Fatal("the session's retrieval must land in kbinfos")
	}
	if len(resp.Slots) != 2 {
		t.Fatalf("slots = %d, want 2 (from the seeded table)", len(resp.Slots))
	}
	filled := 0
	for _, v := range resp.Slots {
		if v.Filled() {
			filled++
		}
	}
	if filled != 1 {
		t.Errorf("filled slots = %d, want 1 (the patched candidate)", filled)
	}
	if len(r.calls) == 0 {
		t.Error("the retrieve tool call never reached the retriever")
	}
}

func TestRunNeverPanicsWithoutBackend(t *testing.T) {
	resp := Rag(context.Background(), RAGTools{}, runtime.RunRequest{
		Question:     "anything",
		ThinkingMode: "low",
		DatasetIDs:   []string{"kb1"},
	})
	if !resp.EmptyResult {
		t.Error("an unavailable backend must yield an empty result, not an error")
	}
}

func TestRunLowReturnsComposedAnswer(t *testing.T) {
	mdl := &fakeModel{replies: []*runtime.ModelReply{
		// formalize (single-turn keyword extraction) and the low graph's direct search each
		// extract keywords in their own call, so
		// the composed answer is the third reply.
		{Content: `{"entity": ["Culdcept"], "aliases": [], "fact_type": [], "qualifiers": []}`},
		{Content: `{"entity": ["Culdcept"], "aliases": [], "fact_type": [], "qualifiers": []}`},
		{Content: "Culdcept was created by OmiyaSoft and released in 1999 [1]."},
	}}
	resp := Rag(context.Background(), RAGTools{
		Retriever: &corpusRetriever{},
		Model:     mdl,
	}, runtime.RunRequest{
		Question:     "Who created Culdcept?",
		ThinkingMode: "low",
		DatasetIDs:   []string{"kb1"},
	})
	if resp.EmptyResult {
		t.Fatal("low must retrieve evidence")
	}
	if resp.Answer == "" {
		t.Fatal("low must return a composed answer (was empty before the composition node)")
	}
	if !strings.Contains(resp.Answer, "OmiyaSoft") {
		t.Errorf("answer = %q, want it grounded in the retrieved evidence", resp.Answer)
	}
	if len(resp.Chunks) == 0 || len(resp.DocAggs) == 0 {
		t.Error("chunks/aggs must be reported alongside the answer")
	}
}

func TestRunUnknownModeReturnsComposedAnswer(t *testing.T) {
	mdl := &fakeModel{replies: []*runtime.ModelReply{{Content: "OmiyaSoft [1]"}}}
	resp := Rag(context.Background(), RAGTools{
		Retriever: &corpusRetriever{},
		Model:     mdl,
	}, runtime.RunRequest{
		Question:     "Who created Culdcept?",
		ThinkingMode: "not-a-mode",
		DatasetIDs:   []string{"kb1"},
	})
	if resp.Mode.Label != "naive" {
		t.Fatalf("mode = %q, want naive", resp.Mode.Label)
	}
	if resp.Answer == "" {
		t.Error("naive must compose an answer")
	}
}

func TestRunEmptyResponseShortCircuits(t *testing.T) {
	mdl := &fakeModel{}
	var buf bytes.Buffer
	resp := Rag(context.Background(), RAGTools{
		Retriever:     &emptyRetriever{},
		Model:         mdl,
		EmptyResponse: "I don't have enough information.",
		Logger:        log.New(&buf, "", 0),
	}, runtime.RunRequest{
		Question:     "Who created Culdcept?",
		ThinkingMode: "low",
		DatasetIDs:   []string{"kb1"},
	})
	if !resp.EmptyResult {
		t.Fatal("expected no evidence")
	}
	if resp.Answer != "I don't have enough information." {
		t.Errorf("answer = %q, want the configured empty response", resp.Answer)
	}
	if mdl.calls != 0 {
		t.Errorf("model calls = %d, want 0 (short-circuited before composition)", mdl.calls)
	}
	// The short circuit is narrated, and the line says what was returned AND that
	// no model was called — the two facts a reader needs to know why there is no
	// composed answer.
	want := "[Composing the answer] No supporting evidence was found, so the configured empty response is returned without calling the model."
	if !strings.Contains(buf.String(), want) {
		t.Errorf("think line missing %q; log:\n%s", want, buf.String())
	}
	// This is the LOW graph, whose last node has no [Finalize] step of its own, so
	// the compose kickoff is the only line saying an answer is being written — it
	// must stay. The question used to be quoted here too, which in this branch (no
	// question threaded through) rendered as an empty "".
	if got := buf.String(); !strings.Contains(got, "[Composing the answer] Composing the answer from 0 gathered passages.") {
		t.Errorf("compose kickoff line missing; log:\n%s", got)
	}
}

// TestAgenticFinalizeMarksItsComposition pins the first half of that split: the
// finalize node hands the compose closure a context that SAYS the [Finalize] step
// was reported, which is what lets the compose stay silent about the evidence.
func TestAgenticFinalizeMarksItsComposition(t *testing.T) {
	var announced []bool
	st := NewAgenticState("q", "", 3, nil)
	st.KB = &runtime.Kbinfos{}
	deps := RAGTools{Finalize: func(fctx context.Context, partial, empty bool, question string) {
		announced = append(announced, finalizeAnnounced(fctx))
	}}

	formalizeAnswerNode(context.Background(), deps, st, nil)

	if len(announced) != 1 || !announced[0] {
		t.Fatalf("the finalize node must mark the composition it triggers; got %v", announced)
	}
}

// TestComposeKickoffFollowsTheAnnouncement pins the second half, in both
// directions: when [Finalize] has been reported the compose must NOT repeat its
// numbers, and when it has not — the low graph's last node, every fallback — the
// kickoff line is the only thing saying an answer is being written, so it stays.
//
// The composition is driven into its no-evidence short circuit so the assertion
// needs no model: the kickoff step is emitted before that branch either way.
func TestComposeKickoffFollowsTheAnnouncement(t *testing.T) {
	cases := []struct {
		name string
		mark bool
		want bool
	}{
		{"after-finalize", true, false},
		{"without-finalize", false, true},
	}
	for _, tc := range cases {
		var linesMu sync.Mutex
		var lines []string
		ctx := runtime.WithSteps(context.Background(),
			runtime.StepReporter{Text: func(line string) {
				linesMu.Lock()
				lines = append(lines, line)
				linesMu.Unlock()
			}})
		if tc.mark {
			ctx = markFinalizeAnnounced(ctx)
		}
		ComposeAnswerWith(ctx, AnswerDeps{EmptyResponse: "nothing found"},
			&runtime.Kbinfos{}, "q", false, false, true)

		got := strings.Join(lines, "\n")
		has := strings.Contains(got, "Composing the answer from")
		if has != tc.want {
			t.Errorf("%s: kickoff line present = %v, want %v; steps:\n%s", tc.name, has, tc.want, got)
		}
		// The short circuit is reported on both paths: "no model was called" is a
		// fact nothing else carries.
		if !strings.Contains(got, "No supporting evidence was found") {
			t.Errorf("%s: the short circuit must be narrated; steps:\n%s", tc.name, got)
		}
	}
}

func TestRunCanDisableComposition(t *testing.T) {
	no := false
	resp := Rag(context.Background(), RAGTools{
		Retriever:     &corpusRetriever{},
		Model:         &fakeModel{},
		ComposeAnswer: &no,
	}, runtime.RunRequest{
		Question:     "Who created Culdcept?",
		ThinkingMode: "low",
		DatasetIDs:   []string{"kb1"},
	})
	if resp.Answer != "" {
		t.Errorf("answer = %q, want empty when composition is disabled", resp.Answer)
	}
	if len(resp.Chunks) == 0 {
		t.Error("evidence must still be returned")
	}
}

func TestRunFormalizesFromConversation(t *testing.T) {
	mdl := &fakeModel{replies: []*runtime.ModelReply{
		{Content: `{"question": "When was Culdcept released?", "keywords": "Culdcept, release, released"}`},
		{Content: "Culdcept was released in 1999 [1]."},
	}}
	retriever := &corpusRetriever{}
	resp := Rag(context.Background(), RAGTools{
		Retriever: retriever,
		Model:     mdl,
		Messages: []schema.Message{
			*schema.UserMessage("Who created Culdcept?"),
			*schema.AssistantMessage("OmiyaSoft.", nil),
			*schema.UserMessage("When was it released?"),
		},
	}, runtime.RunRequest{
		Question:     "When was it released?",
		ThinkingMode: "low",
		DatasetIDs:   []string{"kb1"},
	})
	if len(retriever.calls) == 0 {
		t.Fatal("retriever was never called")
	}
	if !strings.HasPrefix(retriever.calls[0], "When was Culdcept released?") {
		t.Errorf("retrieval query = %q, want it to start with the formalized question", retriever.calls[0])
	}
	if !strings.Contains(retriever.calls[0], "Culdcept") {
		t.Errorf("retrieval query = %q, want the formalized keywords appended", retriever.calls[0])
	}
	if resp.Answer == "" {
		t.Error("answer must be composed after formalization")
	}
}

func TestRunFormalizeDisabledByDefault(t *testing.T) {
	mdl := &fakeModel{replies: []*runtime.ModelReply{{Content: "answer [1]"}}}
	Rag(context.Background(), RAGTools{
		Retriever: &corpusRetriever{},
		Model:     mdl,
		Messages: []schema.Message{
			*schema.UserMessage("Who created Culdcept?"),
			*schema.AssistantMessage("OmiyaSoft.", nil),
			*schema.UserMessage("When was it released?"),
		},
	}, runtime.RunRequest{
		Question:     "When was it released?",
		ThinkingMode: "low",
		DatasetIDs:   []string{"kb1"},
	})
	if mdl.calls != 1 {
		t.Errorf("model calls = %d, want 1 (formalize is opt-in)", mdl.calls)
	}
}

func TestRunAgenticComposesFromResearchFindings(t *testing.T) {
	kb := &runtime.Kbinfos{
		Chunks:     []map[string]any{{"chunk_id": "c1", "content": "Culdcept: OmiyaSoft, 1999.", "similarity": 0.9}},
		PreSummary: "Culdcept was created by OmiyaSoft and released in 1999.",
	}
	mdl := &fakeModel{replies: []*runtime.ModelReply{{Content: "Culdcept was created by OmiyaSoft and released in 1999 [1]."}}}
	res := ComposeAnswerWith(context.Background(), AnswerDeps{Model: mdl}, kb, "Who created Culdcept?", false, false, false)
	if res.Failed {
		t.Fatal("composition must succeed")
	}
	if !strings.Contains(mdl.lastUserPrompt(), "Culdcept was created by OmiyaSoft and released in 1999.") {
		t.Errorf("prompt missing the research findings:\n%s", mdl.lastUserPrompt())
	}
}

// errModel fails every completion, standing in for a provider outage (an
// exhausted quota, a 5xx): the compose call gets no reply at all.
type errModel struct{ err error }

func (m errModel) Complete(context.Context, []schema.Message, []runtime.ToolSpec) (*runtime.ModelReply, error) {
	return nil, m.err
}

// TestRagPreflightFailsFastWithoutThinking pins the pre-flight: a provider that
// cannot answer AT ALL must be reported before the run narrates anything, in the
// same `**ERROR**: …` shape naive mode uses. Without it the failure surfaced
// only at compose time, after a whole research run's worth of think lines.
func TestRagPreflightFailsFastWithoutThinking(t *testing.T) {
	var think []string
	var streamed []string
	resp := Rag(context.Background(), RAGTools{
		Model: errModel{err: fmt.Errorf("minimax API error: insufficient balance")},
		Steps: runtime.StepReporter{Text: func(line string) { think = append(think, line) }},
		AnswerSink: &AnswerSink{OnDelta: func(delta string, isThink bool) {
			if !isThink {
				streamed = append(streamed, delta)
			}
		}},
	}, runtime.RunRequest{Question: "who?", ThinkingMode: "high"})

	if !strings.HasPrefix(resp.Answer, "**ERROR**: ") || !strings.Contains(resp.Answer, "insufficient balance") {
		t.Fatalf("answer = %q, want **ERROR** + the provider's message", resp.Answer)
	}
	if len(think) != 0 {
		t.Errorf("think block must stay closed, got %v", think)
	}
	if len(streamed) != 0 {
		t.Errorf("the error must not be streamed as an answer delta, got %v", streamed)
	}
}

// blockingModel hangs until its context is done, standing in for a provider
// that is slow to first token rather than dead.
type blockingModel struct{}

func (blockingModel) Complete(ctx context.Context, _ []schema.Message, _ []runtime.ToolSpec) (*runtime.ModelReply, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// TestProbeSlowProviderIsNotAFailure pins the timeout split: a probe that only
// ran out of ITS OWN budget must not read as a provider failure — a slow model
// is not a dead one, and failing the whole run here would turn a long first
// token into an outage.
func TestProbeSlowProviderIsNotAFailure(t *testing.T) {
	err := probeChatModelWithin(context.Background(), RAGTools{Model: blockingModel{}}, 5*time.Millisecond)
	if !errors.Is(err, errProbeTimeout) {
		t.Fatalf("err = %v, want errProbeTimeout (slow, not dead)", err)
	}
}

// TestComposeFailureIsNotStreamedAsAnAnswerDelta pins the delivery rule: the
// error answer goes out once, in the final result. Sending it through the sink
// as well made the client render it twice (measured: 221 answer bytes for one
// error + the research-status note, shown as two concatenated ERROR blocks).
func TestComposeFailureIsNotStreamedAsAnAnswerDelta(t *testing.T) {
	var streamed []string
	deps := RAGTools{
		Model:      errModel{err: fmt.Errorf("minimax API error: insufficient balance")},
		AnswerSink: &AnswerSink{OnDelta: func(delta string, isThink bool) { streamed = append(streamed, delta) }},
	}
	resp := &RunResponse{}
	kb := &runtime.Kbinfos{Chunks: []map[string]any{{"chunk_id": "c1", "content": "body"}}}

	composeFinalAnswer(context.Background(), deps, runtime.RunRequest{Question: "who?"}, kb, resp,
		_LOG, false, false, "who?")

	if !strings.HasPrefix(resp.Answer, "**ERROR**: ") {
		t.Fatalf("answer = %q, want **ERROR** + the provider's message", resp.Answer)
	}
	if len(streamed) != 0 {
		t.Errorf("a failed compose must not stream an answer delta, got %v", streamed)
	}
}

// TestComposeFailureReportsProviderErrorInThinkBlock pins the Go-only deviation
// from Python: when the compose call fails, the think block carries a bounded
// summary of the provider's own error. Python prints the bare fallback and keeps
// the cause in the log, which made an exhausted quota look like a RAG bug to the
// user waiting for an answer.
func TestComposeFailureReportsProviderErrorInThinkBlock(t *testing.T) {
	var think []string
	ctx := runtime.WithSteps(context.Background(), runtime.StepReporter{
		Text: func(line string) { think = append(think, line) },
	})
	kb := &runtime.Kbinfos{Chunks: []map[string]any{{"chunk_id": "c1", "content": "body"}}}
	mdl := errModel{err: fmt.Errorf("minimax API error: 已达到 Token Plan 用量上限\n请升级 Token Plan 套餐或购买积分补充用量")}

	res := ComposeAnswerWith(ctx, AnswerDeps{Model: mdl}, kb, "who?", false, false, false)
	if !res.Failed {
		t.Fatalf("res = %+v, want Failed=true", res)
	}
	// The answer reads like a naive-mode failure: the classic `**ERROR**: …`
	// shape carrying the provider's own words, not Python's generic sentence.
	if !strings.HasPrefix(res.Answer, "**ERROR**: ") ||
		!strings.Contains(res.Answer, "已达到 Token Plan 用量上限") {
		t.Errorf("answer = %q, want **ERROR** + the provider's message", res.Answer)
	}

	joined := strings.Join(think, "")
	if !strings.Contains(joined, "已达到 Token Plan 用量上限") {
		t.Errorf("think block must carry the provider's error, got %q", joined)
	}
	if strings.Contains(joined, "\n") {
		t.Errorf("the summary must be a single line, got %q", joined)
	}
}

// TestComposePublishesCiteChunkIDs pins the evidence contract the chat pipeline
// resolves citations against: the compose numbers the blocks 0-based (the client
// indexes reference.chunks with the marker's number, and the answer is streamed
// before any rewrite could apply) and publishes THAT order — similarity-ranked
// and capped — on runtime.Kbinfos.CiteChunkIDs, because the reference is built in
// that order and a marker against the pool by position would land on the wrong
// chunk (or past the end when the pool is shorter than the render cap, which is
// how an agentic answer came back with no reference at all).
func TestComposePublishesCiteChunkIDs(t *testing.T) {
	kb := &runtime.Kbinfos{
		Chunks: []map[string]any{
			{"chunk_id": "c1", "content": "the wolf is grey", "similarity": 0.1},
			{"chunk_id": "c2", "content": "the wolf is small", "similarity": 0.9},
			// No content: kbpBlock renders no block for it, so it must not take a
			// slot in the published list either — an id for it would put every
			// marker after it one block off the passage the model cited.
			{"chunk_id": "c3", "content": "   ", "similarity": 0.5},
		},
	}
	mdl := &fakeModel{replies: []*runtime.ModelReply{{Content: "it is grey [ID:0]."}}}
	if res := ComposeAnswerWith(context.Background(), AnswerDeps{Model: mdl}, kb, "what colour is it?", false, false, false); res.Failed {
		t.Fatal("composition must succeed")
	}
	got := mdl.lastUserPrompt()
	for _, want := range []string{"ID: 0", "ID: 1"} {
		if !strings.Contains(got, want) {
			t.Errorf("evidence missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "ID: 2") {
		t.Errorf("evidence numbering ran past the rendered blocks:\n%s", got)
	}
	// The rendered order is the similarity ranking, not the pool order: the
	// higher-similarity chunk is block 0. The blank chunk sits between the two
	// rendered blocks and contributes no id.
	want := []string{"c2", "c1"}
	if !reflect.DeepEqual(kb.CiteChunkIDs, want) {
		t.Fatalf("CiteChunkIDs = %v, want one id per rendered block in render order %v", kb.CiteChunkIDs, want)
	}
}

// TestComposeSystemDeclaresZeroBasedEvidence pins the prompt-side guard for the
// evidence numbering: the compose renders 0-based block ids and the client indexes
// reference.chunks with the marker's number, so the model has to be told — one on
// "the first source is 1" cites the second passage and never the first. The rule
// belongs to the callers that render 0-based blocks, not to CitationPrompt:
// CitationPrompt is shared with the agent canvas, which numbers its blocks by hash
// id, so a base declared there would be wrong.
func TestComposeSystemDeclaresZeroBasedEvidence(t *testing.T) {
	sys := AnswerDeps{}.composeSystem()
	if !strings.Contains(sys, "the FIRST block is [ID:0]") {
		t.Fatalf("compose system prompt must state the 0-based evidence numbering:\n%s", sys)
	}
	if strings.Contains(prompts.CitationPrompt(""), "the FIRST block is [ID:0]") {
		t.Fatal("CitationPrompt must not declare a base: the canvas render numbers its blocks by hash id")
	}
}

// TestAnswerPromptCarriesTargetContract pins the EXTREME-SELECTION guardrail
// . Without it a "longest/shortest/most…" question tends to be
// answered with the most common or first-listed candidate rather than the
// extreme one, because nothing asks the model to compare.
func TestAnswerPromptCarriesTargetContract(t *testing.T) {
	kb := &runtime.Kbinfos{
		Chunks: []map[string]any{{"chunk_id": "c1", "content": "river A is 300km; river B is 900km"}},
	}
	mdl := &fakeModel{replies: []*runtime.ModelReply{{Content: "river B [1]."}}}
	if res := ComposeAnswerWith(context.Background(), AnswerDeps{Model: mdl}, kb,
		"Which river is the longest?", false, false, false); res.Failed {
		t.Fatal("composition must succeed")
	}
	got := mdl.lastUserPrompt()
	if !strings.Contains(got, "Answer Target Contract:") {
		t.Errorf("prompt missing the Answer Target Contract:\n%s", got)
	}
	if !strings.Contains(got, "EXTREME-SELECTION") || !strings.Contains(got, "EXTREME one") {
		t.Errorf("prompt missing the EXTREME-SELECTION guardrail:\n%s", got)
	}
}

// TestAnswerPromptNoEvidenceInstructions pins: when the call is
// not short-circuited by empty_response, the prompt must still tell the model how
// to degrade — from the research summary if there is one, otherwise with an
// explicit insufficiency statement.
func TestAnswerPromptNoEvidenceInstructions(t *testing.T) {
	// No chunks, no summary -> insufficiency statement.
	mdl := &fakeModel{replies: []*runtime.ModelReply{{Content: "unknown [1]."}}}
	ComposeAnswerWith(context.Background(), AnswerDeps{Model: mdl}, &runtime.Kbinfos{}, "q?", false, false, false)
	if got := mdl.lastUserPrompt(); !strings.Contains(got, "No supporting evidence was retrieved") {
		t.Errorf("prompt missing the no-evidence instruction:\n%s", got)
	}

	// No chunks but a draft exists -> answer from the summary, do not refuse.
	kb := &runtime.Kbinfos{PreSummary: "partial findings"}
	mdl2 := &fakeModel{replies: []*runtime.ModelReply{{Content: "x [1]."}}}
	ComposeAnswerWith(context.Background(), AnswerDeps{Model: mdl2}, kb, "q?", false, false, false)
	if got := mdl2.lastUserPrompt(); !strings.Contains(got, "The retrieved passages are limited") {
		t.Errorf("prompt missing the limited-evidence instruction:\n%s", got)
	}
}

// TestAnswerPromptOrdersPartialPreamble pins: the partial
// preamble sits AFTER the research summary and BEFORE the evidence — not at the
// front of the prompt, where it would precede the question.
func TestAnswerPromptOrdersPartialPreamble(t *testing.T) {
	kb := &runtime.Kbinfos{
		Chunks:     []map[string]any{{"chunk_id": "c1", "content": "body"}},
		PreSummary: "the draft",
	}
	mdl := &fakeModel{replies: []*runtime.ModelReply{{Content: "x [1]."}}}
	ComposeAnswerWith(context.Background(), AnswerDeps{Model: mdl}, kb, "q?", true, false, false)

	got := mdl.lastUserPrompt()
	iQ := strings.Index(got, "Question:")
	iSummary := strings.Index(got, "Research Summary")
	iPartial := strings.Index(got, strings.TrimSpace(runtime.PartialAnswerPreamble))
	iEvidence := strings.Index(got, "Evidence:")
	if iQ < 0 || iSummary < 0 || iPartial < 0 || iEvidence < 0 {
		t.Fatalf("prompt missing one of the parts:\n%s", got)
	}
	if !(iQ < iSummary && iSummary < iPartial && iPartial < iEvidence) {
		t.Errorf("order = question@%d summary@%d partial@%d evidence@%d, want question < summary < partial < evidence",
			iQ, iSummary, iPartial, iEvidence)
	}
}

// TestComposeAnswerWithAttachesUserImages pins the non-outer path fix: when
// AnswerDeps.UserImages carries vision-gated data URIs surviving gateImageAttachments,
// the final-answer user message must be multimodal — an image_url content block beside the
// text question — so the compose model sees the images on the direct fallback (called with
// the original multimodal messages). Without this the
// direct (no-outer) path composed from a text-only prompt and silently dropped
// the picture.
func TestComposeAnswerWithAttachesUserImages(t *testing.T) {
	mdl := &fakeModel{replies: []*runtime.ModelReply{{Content: "answer [1]."}}}
	kb := &runtime.Kbinfos{Chunks: []map[string]any{{"chunk_id": "c1", "content": "evidence"}}}
	if res := ComposeAnswerWith(context.Background(), AnswerDeps{
		Model:      mdl,
		UserImages: []string{"data:image/png;base64,AAAA"},
	}, kb, "What is in this image?", false, false, false); res.Failed {
		t.Fatal("composition must succeed")
	}
	var user *schema.Message
	for i := len(mdl.messages) - 1; i >= 0; i-- {
		if mdl.messages[i].Role == schema.User {
			user = &mdl.messages[i]
			break
		}
	}
	if user == nil {
		t.Fatal("no user message captured")
	}
	if len(user.UserInputMultiContent) == 0 {
		t.Fatalf("expected multimodal content blocks on the user message; content=%q", user.Content)
	}
	hasImage := false
	for _, p := range user.UserInputMultiContent {
		if p.Type == schema.ChatMessagePartTypeImageURL {
			hasImage = true
		}
	}
	if !hasImage {
		t.Errorf("expected an image_url block in the user message; got %+v", user.UserInputMultiContent)
	}
	// The text question must accompany the image (leading text block), so the
	// model reads both the picture and the question.
	sawText := user.Content != ""
	for _, p := range user.UserInputMultiContent {
		if p.Type == schema.ChatMessagePartTypeText && p.Text != "" {
			sawText = true
		}
	}
	if !sawText {
		t.Errorf("expected the question text to accompany the image; content=%q blocks=%+v", user.Content, user.UserInputMultiContent)
	}
}

// TestComposeAnswerWithNoImagesStaysTextOnly pins the default: without
// UserImages the prompt is the plain-text question (no multimodal block), so
// non-vision callers are unaffected.
func TestComposeAnswerWithNoImagesStaysTextOnly(t *testing.T) {
	mdl := &fakeModel{replies: []*runtime.ModelReply{{Content: "answer [1]."}}}
	kb := &runtime.Kbinfos{Chunks: []map[string]any{{"chunk_id": "c1", "content": "evidence"}}}
	if res := ComposeAnswerWith(context.Background(), AnswerDeps{Model: mdl}, kb, "plain?", false, false, false); res.Failed {
		t.Fatal("composition must succeed")
	}
	user := mdl.messages[len(mdl.messages)-1]
	if len(user.UserInputMultiContent) != 0 {
		t.Errorf("expected no multimodal blocks when there are no images; got %+v", user.UserInputMultiContent)
	}
}

// stubDocChunks implements runtime.DocChunkLister for injection tests.
type stubDocChunks struct{}

func (stubDocChunks) DocChunks(context.Context, runtime.DocChunksRequest) ([]map[string]any, error) {
	return nil, nil
}

// TestSearchDepsForProjectsDocChunks locks the injection seam: RAGTools.DocChunks
// must be projected onto SearchDeps so the document-level tools (summarize_document
// / fetch_full_document) can reach a real chunk reader once the caller wires one.
func TestSearchDepsForProjectsDocChunks(t *testing.T) {
	got := searchDepsFor(context.Background(), RAGTools{
		DocChunks: stubDocChunks{},
	}, runtime.RunRequest{}, nil, "", &runtime.Kbinfos{}, log.New(os.Stderr, "", 0))

	if got.DocChunks == nil {
		t.Fatal("RAGTools.DocChunks was not projected onto SearchDeps")
	}
	if _, ok := got.DocChunks.(stubDocChunks); !ok {
		t.Fatalf("SearchDeps.DocChunks = %T, want the injected stubDocChunks", got.DocChunks)
	}

	// Nil RAGTools.DocChunks must stay nil (disables whole-document reading).
	gotNil := searchDepsFor(context.Background(), RAGTools{}, runtime.RunRequest{}, nil, "", &runtime.Kbinfos{}, log.New(os.Stderr, "", 0))
	if gotNil.DocChunks != nil {
		t.Fatal("nil RAGTools.DocChunks must stay nil on SearchDeps")
	}
}

// These benchmarks isolate the cost Eino charges for declaring + compiling a
// graph — exactly what the "build once, inject deps via state" alternative
// (B2) would save. The per-run Invoke cost is identical in both designs, so it
// is deliberately not measured here.

func benchCompile(b *testing.B, build func() error) {
	b.Helper()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := build(); err != nil {
			b.Fatal(err)
		}
	}
}

// low graph shape: 3 nodes, 3 edges, no branch.
func BenchmarkCompileLowGraphShape(b *testing.B) {
	ctx := context.Background()
	benchCompile(b, func() error {
		g := compose.NewGraph[*runtime.RunRequest, *runtime.RunRequest]()
		node := func(c context.Context, r *runtime.RunRequest) (*runtime.RunRequest, error) { return r, nil }
		if err := g.AddLambdaNode("formalize_question", compose.InvokableLambda(node)); err != nil {
			return err
		}
		if err := g.AddLambdaNode("direct_search", compose.InvokableLambda(node)); err != nil {
			return err
		}
		if err := g.AddLambdaNode("formalize_answer", compose.InvokableLambda(node)); err != nil {
			return err
		}
		if err := g.AddEdge(compose.START, "formalize_question"); err != nil {
			return err
		}
		if err := g.AddEdge("formalize_question", "direct_search"); err != nil {
			return err
		}
		if err := g.AddEdge("direct_search", "formalize_answer"); err != nil {
			return err
		}
		if err := g.AddEdge("formalize_answer", compose.END); err != nil {
			return err
		}
		_, err := g.Compile(ctx, compose.WithGraphName("bench_low"))
		return err
	})
}

// action session shape: 3 nodes, 2 edges, 2 branches (with cycles).
func BenchmarkCompileActionSessionShape(b *testing.B) {
	ctx := context.Background()
	benchCompile(b, func() error {
		g := compose.NewGraph[*AgenticState, *AgenticState]()
		node := func(c context.Context, s *AgenticState) (*AgenticState, error) { return s, nil }
		for _, n := range []string{"run_action", "tool", "finalize"} {
			if err := g.AddLambdaNode(n, compose.InvokableLambda(node)); err != nil {
				return err
			}
		}
		if err := g.AddEdge(compose.START, "run_action"); err != nil {
			return err
		}
		if err := g.AddEdge("finalize", compose.END); err != nil {
			return err
		}
		ends := map[string]bool{"tool": true, "finalize": true, "run_action": true, compose.END: true}
		if err := g.AddBranch("run_action", compose.NewGraphBranch(func(context.Context, *AgenticState) (string, error) {
			return "run_action", nil
		}, ends)); err != nil {
			return err
		}
		if err := g.AddBranch("tool", compose.NewGraphBranch(func(context.Context, *AgenticState) (string, error) {
			return "run_action", nil
		}, map[string]bool{"run_action": true, "finalize": true})); err != nil {
			return err
		}
		_, err := g.Compile(ctx, compose.WithGraphName("bench_session"), compose.WithMaxRunSteps(32))
		return err
	})
}

// agentic graph shape: 6 nodes, 4 edges, 4 branches.
func BenchmarkCompileAgenticShape(b *testing.B) {
	ctx := context.Background()
	benchCompile(b, func() error {
		g := compose.NewGraph[*AgenticState, *AgenticState]()
		node := func(c context.Context, s *AgenticState) (*AgenticState, error) { return s, nil }
		for _, n := range []string{
			"formalize_question", "planner", "prefetch", "rag_agent",
			"formalize_answer", "stop",
		} {
			if err := g.AddLambdaNode(n, compose.InvokableLambda(node)); err != nil {
				return err
			}
		}
		for _, e := range [][2]string{
			{compose.START, "formalize_question"},
			{"prefetch", "rag_agent"},
			{"formalize_answer", compose.END},
			{"stop", compose.END},
		} {
			if err := g.AddEdge(e[0], e[1]); err != nil {
				return err
			}
		}
		branches := map[string]map[string]bool{
			"formalize_question": {"stop": true, "planner": true, "rag_agent": true},
			"planner":            {"stop": true, "prefetch": true, "rag_agent": true},
			"prefetch":           {"stop": true, "rag_agent": true},
			"rag_agent":          {"stop": true, "rag_agent": true, "formalize_answer": true},
		}
		for from, ends := range branches {
			if err := g.AddBranch(from, compose.NewGraphBranch(func(context.Context, *AgenticState) (string, error) {
				return "rag_agent", nil
			}, ends)); err != nil {
				return err
			}
		}
		_, err := g.Compile(ctx, compose.WithGraphName("bench_agentic"), compose.WithMaxRunSteps(136))
		return err
	})
}

func TestGraphRecursionLimitBounds(t *testing.T) {
	// 60 for the agentic graph, else max(25, max_loops*8).
	if got := graphRecursionLimit(true, 3); got != 60 {
		t.Fatalf("agentic limit = %d, want 60", got)
	}
	if got := graphRecursionLimit(false, 3); got != 25 {
		t.Fatalf("low limit with 3 loops = %d, want 25", got)
	}
	if got := graphRecursionLimit(false, 5); got != 40 {
		t.Fatalf("low limit with 5 loops = %d, want 40", got)
	}
}

func TestAgenticResearchRoundCostsOneVisit(t *testing.T) {
	// One research round is ONE node (rag_agent). It used to be three (rag_agent → draft → sca),
	// so a round now costs a third of what it did — and the guard bounds WORK, so the constant
	// has to follow the node count or the loop would be allowed 3x the rounds it should have.
	if got, want := agenticRoundVisits, 1; got != want {
		t.Fatalf("round cost = %d, want %d", got, want)
	}
}

func TestNewAgenticStateDoesNotArmBudget(t *testing.T) {
	// the budget is armed in the formalize_question node's
	// return, not at state creation, so formalization is not charged to it.
	st := NewAgenticState("q", "", 3, nil)

	if !st.Deadline.IsZero() {
		t.Fatalf("Deadline = %v, want zero until formalize_question runs", st.Deadline)
	}
}

func TestFormalizeQuestionNodeArmsBudget(t *testing.T) {
	st := NewAgenticState("when did it open?", "", 3, nil)
	// No model: the node still arms the budget, mirroring.
	formalizeQuestionNode(context.Background(), RAGTools{}, st, nil)

	if st.Deadline.IsZero() {
		t.Fatal("formalize_question must arm the budget")
	}
	if st.Question != "when did it open?" {
		t.Fatalf("Question = %q, want it unchanged without a model", st.Question)
	}
}

// delayedFormalizeModel answers a formalization call only after a delay, so a
// test can tell whether that call was charged to the research budget.
type delayedFormalizeModel struct {
	delay time.Duration
}

func (m delayedFormalizeModel) Complete(_ context.Context, _ []schema.Message, _ []runtime.ToolSpec) (*runtime.ModelReply, error) {
	time.Sleep(m.delay)
	return &runtime.ModelReply{Content: `{"keywords": ["曹操"]}`}, nil
}

// TestFormalizeQuestionNodeDoesNotChargeTheBudget pins WHEN the budget starts:
// Python stamps the deadline in formalize_question's return dict
// (agentic_rag_graph.py:1061) — after the formalization call — so that call is
// not charged to the research budget. Arming on entry shortened every downstream
// timeout by one LLM call, which is exactly enough to flip the
// canOpenRound guard near the boundary and skip a round Python would run.
func TestFormalizeQuestionNodeDoesNotChargeTheBudget(t *testing.T) {
	const delay = 300 * time.Millisecond
	st := NewAgenticState("曹操是谁？", "", 3, []schema.Message{*schema.UserMessage("曹操是谁？")})
	deps := RAGTools{Model: delayedFormalizeModel{delay: delay}}

	started := time.Now()
	formalizeQuestionNode(context.Background(), deps, st, nil)
	if elapsed := time.Since(started); elapsed < delay {
		t.Fatalf("the stub model did not run (%v elapsed): the assertion below would be vacuous", elapsed)
	}

	// The clock is armed on the way OUT, so what remains is the whole budget
	// minus the moment the defer ran — not minus the model call.
	if got := st.RemainingS(); got < TotalBudgetS-0.15 {
		t.Errorf("RemainingS = %.2fs after a %v formalization, want ~%.0fs: the call must not be charged to the budget",
			got, delay, TotalBudgetS)
	}
}

// TestRecordConsecutiveUnanswerableAcrossOuterRagCalls mirrors the Python
// outer react loop in dialog_service.rag_agent: the model may call rag()
// several times within one user turn, and each unsatisfying verdict should
// bump the shared _consecutive_unanswerable counter. Go keeps that counter on
// the *RAGCache that Rag() now builds before the outer-react branch, so the
// same cache is reused across the outer loop's multiple rag() calls. This test
// simulates two such outer rag() calls with INSUFFICIENT verdicts and asserts
// the counter reaches 2 — the threshold at which Rag() tells the outer agent to
// STOP calling rag again.
func TestRecordConsecutiveUnanswerableAcrossOuterRagCalls(t *testing.T) {
	cache := NewRAGCache()

	// First outer rag() call — unsatisfying verdict bumps the counter to 1.
	cache.NoteUnanswerable(false)
	if cache.ConsecutiveUnanswerable() != 1 {
		t.Fatalf("after 1st outer rag() call: ConsecutiveUnanswerable = %d, want 1",
			cache.ConsecutiveUnanswerable())
	}

	// Second outer rag() call — still unsatisfying: counter must reach 2 so the
	// STOP guard can fire (the state the pre-fix code could never reach on the
	// outer path, because deps.Cache was nil and the increment was skipped).
	cache.NoteUnanswerable(false)
	if cache.ConsecutiveUnanswerable() != 2 {
		t.Fatalf("after 2nd outer rag() call: ConsecutiveUnanswerable = %d, want 2",
			cache.ConsecutiveUnanswerable())
	}

	// A satisfying verdict resets the streak.
	cache.NoteUnanswerable(true)
	if cache.ConsecutiveUnanswerable() != 0 {
		t.Fatalf("after a SUFFICIENT verdict: ConsecutiveUnanswerable = %d, want 0",
			cache.ConsecutiveUnanswerable())
	}
}

// TestRecordConsecutiveUnanswerableNoCacheIsNoOp makes sure the guard is safe
// when no cache is wired (nil deps.Cache): the update must not panic and the
// outer loop simply loses the cross-call STOP protection — which is exactly
// why Rag() now auto-builds a cache before branching into the outer react loop.
func TestRecordConsecutiveUnanswerableNoCacheIsNoOp(t *testing.T) {
	// Must not panic with a nil cache.
	(*RAGCache)(nil).NoteUnanswerable(false)
	(*RAGCache)(nil).NoteUnanswerable(true)
}

// TestRewriteContextShowsThePassageBehindAConfirmedMember and
// TestRewriteRoundCanStillAdmitOnARichPool used to live here: they pinned the input side of the
// gap→query rewrite round (what the rewriter was shown, and that a rich pool did not stop it from
// admitting its own retrieval). Both went with the node — with no machine writing the next round's
// queries, there is no context to render and no rewrite round to admit anything.

// TestRenderSlotRecordCarriesNoMachineFields pins the contract that survived every redesign here: the
// record is the FACTS the research settled, with no strength, no evidence ids and no clue tails — the
// answer quotes the bookkeeping verbatim when it is handed one (see answerPromptWithEvidence).
func TestRenderSlotRecordCarriesNoMachineFields(t *testing.T) {
	strength := 0.9
	tbl := runtime.State{State: []runtime.Variable{
		{ID: 0, Type: "aspect", Candidate: strPtr("answer A"), CandidateStrength: &strength},
		{ID: 1, Type: "aspect"},
	}}
	rec := RenderSlotRecord(tbl, "the collected answer")
	for _, banned := range []string{"strength=", "evidence_ids", "terminal=", "discovered_clues"} {
		if strings.Contains(rec, banned) {
			t.Errorf("record %q carries the machine field %q", rec, banned)
		}
	}
	if !strings.Contains(rec, "answer A") || !strings.Contains(rec, "NOT RESOLVED") {
		t.Errorf("record %q, want the slots' own values", rec)
	}
	if !strings.Contains(rec, "the collected answer") {
		t.Errorf("record %q, want the session's own draft carried", rec)
	}
}

// TestAnswerPromptLabelsTheRecordAndForbidsQuoting pins the prompt contract: the
// slot record reaches the answer labelled as an INTERNAL record (not evidence),
// with an explicit instruction not to quote its lines and labels — and the
// machine fields never reach the prompt at all.
func TestAnswerPromptLabelsTheRecordAndForbidsQuoting(t *testing.T) {
	strong := 0.9
	record := RenderSlotRecord(runtime.NewState([]runtime.Variable{
		{ID: 1, Type: "entity", Candidate: strPtr("华雄、颜良"), CandidateStrength: &strong},
	}, 0, nil), "")

	withRecord := &runtime.Kbinfos{Record: record, PreSummary: "- slot 1 [entity]: 华雄、颜良 (strength=0.90)"}
	prompt := AnswerDeps{}.answerPromptWithEvidence(withRecord, "q", false, false)
	for _, want := range []string{recordContract, "Research Record (INTERNAL", "华雄、颜良"} {
		if !strings.Contains(prompt.user, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	for _, banned := range []string{"strength=", "evidence_ids", "Research Summary (primary evidence)"} {
		if strings.Contains(prompt.user, banned) {
			t.Errorf("prompt must not carry %q when a slot record exists", banned)
		}
	}

	// No slot record (a prose summary instead): the prose block is used, and the
	// record contract stays out of it.
	prose := &runtime.Kbinfos{PreSummary: "the research found A and B"}
	prompt = AnswerDeps{}.answerPromptWithEvidence(prose, "q", false, false)
	if !strings.Contains(prompt.user, "Research Summary (primary evidence)") {
		t.Errorf("prose prompt = %q, want the summary block", prompt.user)
	}
	if strings.Contains(prompt.user, recordContract) {
		t.Error("the record contract must not label a prose summary")
	}
}

// TestSessionPatchLogNamesBothSidesOfTheTournament pins P13 for the one place a
// list can vanish without a trace: MergeSlotPatch keeps ONE candidate per slot,
// chosen by the strength the model reported, and the losing candidate is then in
// no table, no draft and no record.
//
// The log line is the only place that fact can exist, so it has to name both the
// claim and the base it lost to.
func TestSessionPatchLogNamesBothSidesOfTheTournament(t *testing.T) {
	var buf bytes.Buffer
	orig := _LOG
	_LOG = log.New(&buf, "", 0)
	defer func() { _LOG = orig }()

	strong, weak := 0.97, 0.85
	base := runtime.NewState([]runtime.Variable{
		{ID: 1, Type: "entity", Candidate: strPtr("孔秀、孟坦"), CandidateStrength: &strong},
	}, 0, nil)
	patch := runtime.NewState([]runtime.Variable{
		{ID: 1, Type: "entity", Candidate: strPtr("华雄、颜良、庞德"), CandidateStrength: &weak},
	}, 0, nil)

	// The weaker claim loses: the slot still holds the base afterwards.
	logSessionPatch(2, base, base, patch)
	got := buf.String()
	for _, want := range []string{"华雄、颜良、庞德", "0.85", "孔秀、孟坦", "0.97", "NOT ADOPTED"} {
		if !strings.Contains(got, want) {
			t.Errorf("log %q missing %q", got, want)
		}
	}

	// Adopted: the slot holds the patch's candidate afterwards.
	buf.Reset()
	merged := runtime.NewState([]runtime.Variable{
		{ID: 1, Type: "entity", Candidate: strPtr("华雄、颜良、庞德"), CandidateStrength: &weak},
	}, 1, nil)
	logSessionPatch(2, base, merged, patch)
	if got := buf.String(); !strings.Contains(got, "adopted") || strings.Contains(got, "NOT ADOPTED") {
		t.Errorf("log %q, want the adopted verdict", got)
	}
}

// TestMergeSlotPatchKeepsTheStrongestClaim is the behaviour the log above exists
// to expose: a weaker branch candidate must NOT overwrite a stronger base, and a
// stronger one must.
// TestMergeSlotPatchKeepsTheStrongestClaim pins the rule for a slot that holds
// ONE value: sessions fold in completion order, so a weak tentative claim must
// not downgrade what another session already proved. Sets are exempt from this
// rule on purpose — see TestMergeSlotPatchMergesSetCandidates.
func TestMergeSlotPatchKeepsTheStrongestClaim(t *testing.T) {
	strong, weak := 0.97, 0.85
	base := runtime.NewState([]runtime.Variable{
		{ID: 1, Type: "entity", Candidate: strPtr("白马坡"), CandidateStrength: &strong},
	}, 0, nil)
	weaker := runtime.NewState([]runtime.Variable{
		{ID: 1, Type: "entity", Candidate: strPtr("延津"), CandidateStrength: &weak},
	}, 0, nil)
	if merged := MergeSlotPatch(base, weaker); merged != nil {
		if got := *merged.ByID(1).Candidate; got != "白马坡" {
			t.Errorf("slot 1 = %q, want the stronger base candidate to stand", got)
		}
	}

	stronger := runtime.NewState([]runtime.Variable{
		{ID: 1, Type: "entity", Candidate: strPtr("延津"), CandidateStrength: &strong},
	}, 0, nil)
	base = runtime.NewState([]runtime.Variable{
		{ID: 1, Type: "entity", Candidate: strPtr("白马坡"), CandidateStrength: &weak},
	}, 0, nil)
	if merged := MergeSlotPatch(base, stronger); merged == nil || *merged.ByID(1).Candidate != "延津" {
		t.Error("a strictly stronger branch candidate must be adopted")
	}
}

// TestMergeSlotPatchMergesSetCandidates pins the set rule: a slot that holds a
// SET is unioned, because "the stronger claim wins" DISCARDS the members only the
// weaker list held.
//
// Measured (2026-09-15, 三国演义/关羽): one session enumerated twelve members into
// slot 0, a second had already written "10" there at strength 0.90, the twelve
// lost at 0.85, and the answer was the 10 — while 管亥 and 车胄 (two of the
// twelve) had each returned ten passages of their own.
func TestMergeSlotPatchMergesSetCandidates(t *testing.T) {
	strong, weak := 0.90, 0.85

	// A list beats a NUMBER: the number is a claim ABOUT the list, and the list is
	// the members. The number is not lost — it stays as an alternate clue.
	base := runtime.NewState([]runtime.Variable{
		typedCountVar(1, "count", 10, strong),
	}, 0, nil)
	enumeration := runtime.NewState([]runtime.Variable{
		typedMembersVarS(1, "count", weak, "华雄", "颜良", "管亥", "车胄", "蔡阳"),
	}, 0, nil)
	merged := MergeSlotPatch(base, enumeration)
	if merged == nil {
		t.Fatal("the enumeration must change the slot: five members are not a count of ten")
	}
	if got := *merged.ByID(1).Candidate; got != "华雄、颜良、管亥、车胄、蔡阳" {
		t.Fatalf("slot 1 = %q, want the enumerated list to hold the slot", got)
	}
	if alts := alternateCandidatesOf(*merged.ByID(1)); len(alts) != 1 || alts[0] != "10" {
		t.Fatalf("alternates = %v, want the losing count kept", alts)
	}

	// Two lists union, and neither side's members are dropped.
	twoLists := MergeSlotPatch(
		runtime.NewState([]runtime.Variable{
			typedMembersVarS(1, "entity", strong, "孔秀", "孟坦"),
		}, 0, nil),
		runtime.NewState([]runtime.Variable{
			typedMembersVarS(1, "entity", weak, "华雄", "颜良", "庞德"),
		}, 0, nil),
	)
	if twoLists == nil {
		t.Fatal("two lists must union")
	}
	for _, want := range []string{"孔秀", "孟坦", "华雄", "颜良", "庞德"} {
		if got := *twoLists.ByID(1).Candidate; !strings.Contains(got, want) {
			t.Errorf("slot 1 = %q, want %s kept", got, want)
		}
	}

	// Two numbers keep the larger: a set that shrinks when a second source agrees
	// with it is a set that loses members.
	numbers := MergeSlotPatch(
		runtime.NewState([]runtime.Variable{
			typedCountVar(1, "count", 10, strong),
		}, 0, nil),
		runtime.NewState([]runtime.Variable{
			typedCountVar(1, "count", 12, weak),
		}, 0, nil),
	)
	if numbers == nil || *numbers.ByID(1).Candidate != "12" {
		t.Fatalf("two counts must keep the larger one, got %v", numbers)
	}
}

// TestValueQuestionRecordKeepsItsOwnDraft pins that the record carries the session's draft with NO
// shape gate: it used to lead with the draft only off a "value table" (a member count computed from
// the table's text), which is the runtime deciding what the table means (see RenderSlotRecord).
func TestValueQuestionRecordKeepsItsOwnDraft(t *testing.T) {
	tbl := runtime.State{State: []runtime.Variable{{ID: 0, Type: "person"}, {ID: 1, Type: "person"}}}
	rec := RenderSlotRecord(tbl, "The person is Richard Coxon, crewed with Colin Beashel in the Soling class.")
	if !strings.Contains(rec, "Richard Coxon") {
		t.Fatalf("record %q, want the run's own draft kept", rec)
	}
	if !strings.Contains(rec, "UNVERIFIED") {
		t.Errorf("record %q, want the draft labelled for what it is (one session, before the merge)", rec)
	}
}

// TestMemberLineCarriesTheItemsOwnWords pins that an enumerated member reaches the answer WITH its
// words, not as a bare name.
//
// The evidence block is token-budgeted, so the passage behind a member may not fit: measured
// (2026-09-17, 三国/关羽) 18 members against the 13 passages the budget carried, and the answer had
// to name five of them as "listed in the record, no original text provided". The record's own
// contract accepts a member "quoted above", so the quote travels with the name.
func TestMemberLineCarriesTheItemsOwnWords(t *testing.T) {
	items := slots.Items(
		slots.Item{Value: "华雄", ChunkID: "c1", Quote: "云长提华雄之头，掷于地上"},
		slots.Item{Value: "刘延"},
	)
	line := memberLine(runtime.Variable{ID: 1, Type: "person", Value: &items})
	if !strings.Contains(line, "华雄 ←c1 “云长提华雄之头，掷于地上”") {
		t.Fatalf("line = %q, want the member's own words beside the passage that states it", line)
	}
	if !strings.Contains(line, "刘延 ←(no passage)") {
		t.Fatalf("line = %q, want a member with no passage still marked as a claim", line)
	}
}

// TestRunCoverageResolveEnrollsTheEnumerationThePlannerSkipped pins the wiring that the measured
// run (2026-09-17, 三国/关羽) needed: the planner typed the answer slot "count", the sessions
// patched the members into it, and no type word ever asked for a set.
//
// The VALUE is the fact, so the resolve — the last node that can complete the set — runs the
// direction's enumeration itself instead of judging only what a session happened to write down.
// Ten names were the whole answer that day; the corpus stated more.
// The two resolve-node tests that lived here are gone with the node itself. They pinned when the
// runtime decided to run an enumeration at the answer node — enrolled because the planner had
// skipped it, or skipped because the direction's two words had never met in one passage — and both
// halves of that decision belonged to the coverage engine.
//
// What replaces the whole mechanism: the session searches (its tool descriptions already carry the
// probe shape), every tool result says how much of each document it has read, and list_chunks pages a
// document to its end. The completeness of an enumeration is then a fact about what the model read,
// not a window count the runtime computed.

// TestMergeSlotPatchKeepsTheLosingClaimAsAnAlternate pins the bookkeeping that
// survives every rule in MergeSlotPatch: one slot holds one candidate, so the
// claim that lost the comparison used to leave no trace at all — not in the
// table, not in the draft, not in the record. The framework does not decide which
// claim is true; it stops discarding the one that lost.
//
// Sets no longer lose at all (they union, see
// TestMergeSlotPatchMergesSetCandidates), so this pins the single-value case.
func TestMergeSlotPatchKeepsTheLosingClaimAsAnAlternate(t *testing.T) {
	strong, weak := 0.97, 0.85
	base := runtime.NewState([]runtime.Variable{
		{ID: 1, Type: "entity", Candidate: strPtr("白马坡"), CandidateStrength: &strong},
	}, 0, nil)
	branch := runtime.NewState([]runtime.Variable{
		{ID: 1, Type: "entity", Candidate: strPtr("延津"), CandidateStrength: &weak},
	}, 0, nil)

	merged := MergeSlotPatch(base, branch)
	if merged == nil {
		t.Fatal("the losing claim must still change the state: it is kept as an alternate")
	}
	v := merged.ByID(1)
	if v.Candidate == nil || *v.Candidate != "白马坡" {
		t.Fatalf("slot 1 = %v, want the stronger claim to hold the slot", v.Candidate)
	}
	alts := alternateCandidatesOf(*v)
	if len(alts) != 1 || alts[0] != "延津" {
		t.Fatalf("alternates = %v, want the losing claim kept", alts)
	}

	// It reaches the ANSWER-facing record, which is the whole point: the answer
	// must be able to see that two claims exist.
	rec := RenderSlotRecord(*merged, "")
	if !strings.Contains(rec, "alternate") || !strings.Contains(rec, "延津") {
		t.Fatalf("record = %q, want the alternate rendered", rec)
	}

	// And it survives a SECOND merge: alternates accumulate, they do not compete.
	strongerAgain := runtime.NewState([]runtime.Variable{
		{ID: 1, Type: "entity", Candidate: strPtr("斜谷"), CandidateStrength: &strong},
	}, 0, nil)
	merged2 := MergeSlotPatch(*merged, strongerAgain)
	if merged2 == nil {
		t.Fatal("a third claim must fold in")
	}
	if got := alternateCandidatesOf(*merged2.ByID(1)); len(got) != 2 {
		t.Fatalf("alternates = %v, want both losing claims kept", got)
	}
}

// TestTheRecordStatesNoSizeAndReconcilesNothing pins what the record does NOT do: no
// "enumerated members across the slots above: N" computed from the table, no count-vs-list warning, no
// "claimed WITHOUT a passage" list. Those lines were the runtime reading the table's own text to decide
// what counted as a member (see RenderSlotRecord) — the answer reconciles the values against the
// passages it was shown, and the record hands it both, verbatim.
func TestTheRecordStatesNoSizeAndReconcilesNothing(t *testing.T) {
	tbl := runtime.State{State: []runtime.Variable{
		{ID: 0, Type: "count", Candidate: strPtr("10")},
		{ID: 1, Type: "person", Candidate: strPtr("华雄、颜良")},
	}}
	rec := RenderSlotRecord(tbl, "关羽共杀了10人。")
	for _, banned := range []string{"enumerated members", "claimed WITHOUT a passage", "NOTE:",
		"State the count and the members TOGETHER"} {
		if strings.Contains(rec, banned) {
			t.Errorf("record %q carries the removed derivation (%q)", rec, banned)
		}
	}
	for _, want := range []string{"10", "华雄、颜良", "关羽共杀了10人。"} {
		if !strings.Contains(rec, want) {
			t.Errorf("record %q is missing %q", rec, want)
		}
	}
}

// TestAValueTableHasNoSizeLineEither pins the other side of the same fact: nothing in the record is
// computed from a value's shape, so a value table is not "spared" a size line — no table has one
// (see RenderSlotRecord).
func TestAValueTableHasNoSizeLineEither(t *testing.T) {
	tbl := runtime.State{State: []runtime.Variable{
		// One value with separators inside it, under a non-member type: the permissive reading of this
		// once produced "enumerated members: 16" for ONE waterfall.
		{ID: 0, Type: "dataset", Candidate: strPtr("Grace's、High、Falls、Colonial、Creek")},
		{ID: 1, Type: "number", Candidate: strPtr("133 feet")},
	}}
	rec := RenderSlotRecord(tbl, "Alabama's tallest waterfall is 133 feet tall.")
	if strings.Contains(rec, "enumerated members") {
		t.Errorf("record %q states a size of its own", rec)
	}
	if !strings.Contains(rec, "Grace's、High、Falls、Colonial、Creek") {
		t.Errorf("record %q, want the value verbatim (never split)", rec)
	}
}

// TestUnionSetCandidatesIsOrderIndependent pins the bug that cost the most members
// in the 2026-09-16 enumeration runs.
//
// The union must not depend on which side is the base: with the base the LONGER
// list, "the union adds nothing to the base" was answered as "no conclusion", the
// caller fell through to the strength rule, and a SHORTER list — the one the model
// was sure of, so the one it called stronger — replaced the list containing it.
//
// Measured in the run this pins: twenty-four names probed and each reached, slot 1
// holding thirteen members, a session patching its own eleven at 0.95 against the
// base's 0.90, and a final record of eleven.
// typedMembersValue is a slot value that DECLARES a member list — the only shape the
// runtime counts (see package slots): names with no declared kind are opaque text.
func typedMembersValue(list string) slots.Value {
	items := strings.Split(list, "、")
	members := make([]slots.Item, 0, len(items))
	for _, n := range items {
		members = append(members, slots.Item{Value: strings.TrimSpace(n)})
	}
	return slots.Items(members...)
}

// typedMembersVar is typedMembersValue as the slot variable a session would patch.
func typedMembersVar(id int, typ, list string) runtime.Variable {
	return typedMembersVarS(id, typ, 0, strings.Split(list, "、")...)
}

// typedMembersVarS is the same with an explicit strength, the way a session's patch
// carries one.
func typedMembersVarS(id int, typ string, strength float64, names ...string) runtime.Variable {
	items := make([]slots.Item, 0, len(names))
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" {
			items = append(items, slots.Item{Value: n})
		}
	}
	v := slots.Items(items...)
	rendered := slots.Render(v)
	out := runtime.Variable{ID: id, Type: typ, Candidate: &rendered, Value: &v}
	if strength > 0 {
		out.CandidateStrength = &strength
	}
	return out
}

// typedAnchoredMembersVar is typedMembersVarS with the passage each member rests on: the shape a
// member must have to be COUNTED (see runtime.AnchoredMembers). Without the chunk id the member
// is a claim — visible in the record, absent from the number.
func typedAnchoredMembersVar(id int, typ string, lists ...string) runtime.Variable {
	items := make([]slots.Item, 0, len(lists))
	for _, list := range lists {
		for _, n := range strings.Split(list, "、") {
			if n = strings.TrimSpace(n); n != "" {
				items = append(items, slots.Item{Value: n, ChunkID: "c-" + n, Quote: "…" + n + "…"})
			}
		}
	}
	v := slots.Items(items...)
	rendered := slots.Render(v)
	return runtime.Variable{ID: id, Type: typ, Candidate: &rendered, Value: &v}
}

// anchoredItemsVar builds a slot holding items with the anchors and quotes a session chose, which is
// what typedAnchoredMembersVar's synthetic "c-<name>" anchors cannot express — a metadata selection's
// anchors are doc ids and its quotes are the metadata lines it was shown.
func anchoredItemsVar(id int, typ string, items ...slots.Item) runtime.Variable {
	v := slots.Items(items...)
	rendered := slots.Render(v)
	return runtime.Variable{ID: id, Type: typ, Candidate: &rendered, Value: &v}
}

func typedCountVar(id int, typ string, n int, strength float64) runtime.Variable {
	v := slots.Number(n)
	rendered := slots.Render(v)
	out := runtime.Variable{ID: id, Type: typ, Candidate: &rendered, Value: &v}
	if strength > 0 {
		out.CandidateStrength = &strength
	}
	return out
}

// TestRecordContractStatesTheSettledValueIsTheAnswer pins the half of the record
// contract that the measured "not found" answer needed: a record that is only a
// check is droppable when the passages do not repeat its value.
//
// Measured (2026-09-16, FRAMES, mode high): the slot table held
// `slot 0 [person]: Colin Beashel and Richard Coxon (Australia, Star class 1984
// Olympics)` and the composed answer was "not found in the knowledge base" — the
// passages around it were about other competitions, and the chat configuration
// requires that sentence when the information is unavailable. The record has to
// outrank the absence, or the research that found the answer is discarded.
func TestRecordContractStatesTheSettledValueIsTheAnswer(t *testing.T) {
	for _, want := range []string{
		"that value IS the answer",
		"do not report that the answer was not found while a slot holds one",
		"evidence plainly contradicts a settled value",
		"NEVER quote these lines",
		"probed-and-answered",
	} {
		if !strings.Contains(recordContract, want) {
			t.Errorf("recordContract is missing %q", want)
		}
	}
}

// The one-shot budget extension and its test are gone. It gave a set-shaped pass a second
// slice of the question's clock (measured 2026-09-16, 三国/关羽: the run ended at
// `ROUND 1 end (unresolved=0)` with the reached-but-unpatched 管亥 gone), which meant the
// clock one question had was decided by the shape of its plan rather than by the caller.
// There is one clock per question and nothing inside a run widens it.

// TestMergeSlotPatchKeepsTheDeclaration pins what the fold must NOT drop.
//
// Terms/Subject are the planner's declaration of how the source words the deed, and the
// completeness pass is built from them (runtime.ScanPatterns / RunCompletenessPass).
// Rebuilding the slot without them is why one run's second round had no act patterns at
// all: measured (2026-09-16, 三国/关羽) round 1's session seeds carried 6325 characters
// (method + the declared patterns) and round 2's carried 4150 (method only) — the recovery
// round, opened by the routing precisely because the record was still short, ran with the
// enumeration machinery switched off.
func TestMergeSlotPatchKeepsTheDeclaration(t *testing.T) {
	base := runtime.NewState([]runtime.Variable{
		{ID: 0, Type: "count", Terms: []string{"斩", "杀"}, Subjects: []string{"关羽", "云长"}},
		{ID: 1, Type: "dataset"},
	}, 0, nil)
	strong := 0.9
	branch := runtime.NewState([]runtime.Variable{
		{ID: 1, Type: "dataset", Candidate: strPtr("孔秀、孟坦"), CandidateStrength: &strong},
	}, 1, nil)

	merged := MergeSlotPatch(base, branch)
	if merged == nil {
		t.Fatal("MergeSlotPatch returned nil for a branch that carries a candidate")
	}
	if got := merged.State[0].Terms; len(got) != 2 || got[0] != "斩" || got[1] != "杀" {
		t.Errorf("merged Terms = %v, want the declaration to travel with the slot", got)
	}
	if got := strings.Join(merged.State[0].Subjects, "|"); got != "关羽|云长" {
		t.Errorf("merged Subject = %q, want the declared actor", got)
	}
	// The declaration must SURVIVE the fold: it is the only place the deed's words live, and a fold
	// that drops them leaves the next turn with nothing to search by. The assertions above are the
	// test — the old gate that read the fold's result as "is this still an enumeration" is gone with
	// the coverage engine.
}

// TestRunSlotResearchPassRunsOneSession pins the round's shape: ONE session, seeded with the
// question and the plan's clues, and NOTHING enumerated by the runtime in code.
//
// The act words used to be turned into windows by the runtime before any session started
// (runtime.EnumerateCoverage), on the argument that "a list of queries in a prompt is advice,
// and advice may simply not be taken". What that cost was the whole coverage engine: a shape
// gate, a window judge, a member write-back, a per-shape budget extension and a second
// enrolment path at the answer node — all of them reading a slot table to decide what to run.
//
// The same evidence is reached the other way round now: the session searches with the corpus's
// own wording (retrieve's description already teaches the probe shape), every tool result says
// how much of each document has been read, and list_chunks pages a document to its end. The
// enumeration is the model reading, not the runtime asking.
func TestRunSlotResearchPassRunsOneSession(t *testing.T) {
	table := func() runtime.State {
		return runtime.NewState([]runtime.Variable{
			{ID: 0, Type: "count", Terms: []string{"斩", "杀"}, Subjects: []string{"关羽", "云长"}},
			{ID: 1, Type: "dataset", QuestionClues: []string{"who did he kill?"}},
		}, 0, nil)
	}
	exec := newStubExecutor()
	exec.add("search_chunks", `{"hit":[{"doc_id":"d1","docnm_kwd":"doc1","content":"云长手起刀落，斩孔秀于马下。"}],"doc_aggs":[]}`, runtime.StatusOK)
	kb := &runtime.Kbinfos{}
	st := &AgenticState{
		Question:  "关羽杀了多少有姓名的人物？",
		KB:        kb,
		SlotTable: table(),
		Plan:      []string{"关羽 斩 名单", "关羽 杀 武将"},
	}
	mdl := &scriptedModel{replies: []string{"I could not find any evidence about that."}}
	deps := runtime.SessionDeps{
		Model: mdl,
		Tools: newToolset(exec),
		KB:    kb,
	}
	res := RunSlotResearchPass(context.Background(), context.Background(), deps, st.Question, st, 60)
	if res == nil {
		t.Fatal("expected a result")
	}
	// The session ran. It is the round's research AND its answer, so a round always runs one.
	if got := len(mdl.seen); got == 0 {
		t.Fatal("model calls = 0, want the one session this round seeds")
	}
	// ONE round is ONE session: the ledger has a single row, and the rows are the round's own
	// record of what each session was asked (see RunSlotResearchPass).
	if got := len(res.Attempted); got != 1 {
		t.Fatalf("ledger rows = %d, want exactly one (one session per round)", got)
	}
}

// The question's clock is DIVIDED, and these pin the division (see policy.go). The phases used to
// carry a cap each — planner 45, prefetch 90, round 120, finale 60 — whose sum was 315s against a
// 180s question, so whichever phase ran last was cut off by the wall: measured 2026-09-20, 5
// sessions died at exactly 110s and 4 openings spent 90s with zero model calls, leaving the
// research 30-90s of a 180s question.

// TestTheOpeningShareIsBoundedAndInsideTheQuestion pins the opening's share: it has a ceiling (the
// whole point), a floor (a question that is almost out of time still gets one decomposition), and
// it is always smaller than the question.
func TestTheOpeningShareIsBoundedAndInsideTheQuestion(t *testing.T) {
	if got := openingShareS(TotalBudgetS); got != OpeningMaxS {
		t.Errorf("openingShareS(%v) = %v, want the ceiling %v", TotalBudgetS, got, OpeningMaxS)
	}
	if got := openingShareS(2 * OpeningMinS); got != OpeningMinS {
		t.Errorf("openingShareS(%v) = %v, want the floor %v", 2*OpeningMinS, got, OpeningMinS)
	}
	for _, remaining := range []float64{30, 60, 90, 120, 180} {
		if got := openingShareS(remaining); got > remaining {
			t.Errorf("openingShareS(%v) = %v, which is more than the question has left", remaining, got)
		}
	}
}

// TestTheFinaleKeepsItsShareAndTheResearchGetsWhatIsLeft pins the two shares that are always in
// tension: the answer turn's floor (one slow call may not be raced by the wall) and the research
// room the round is allowed to spend.
func TestTheFinaleKeepsItsShareAndTheResearchGetsWhatIsLeft(t *testing.T) {
	if want := min(FinaleMaxS, 0.25*TotalBudgetS); finaleShareS(TotalBudgetS) != want {
		t.Errorf("finaleShareS(%v) = %v, want %v", TotalBudgetS, finaleShareS(TotalBudgetS), want)
	}
	if got := finaleShareS(40); got != FinaleMinS {
		t.Errorf("finaleShareS(40) = %v, want the floor %v (the answer call's own tail)", got, FinaleMinS)
	}
	// The research room is exactly the clock minus that share — no second reserve anywhere.
	for _, remaining := range []float64{80, 120, 180} {
		if got, want := researchRoomS(remaining), remaining-finaleShareS(remaining); got != want {
			t.Errorf("researchRoomS(%v) = %v, want %v", remaining, got, want)
		}
	}
}

// TestARoundNeedsRoomForItselfAndTheFinale pins the ONE gate the router uses for time: a round is
// opened only when the question can pay for the round AND for the answer that follows it.
//
// It replaced a bare headroom check that asked only whether a round could START — which is how a
// second round was opened with less time left than the answer turn needs.
func TestARoundNeedsRoomForItselfAndTheFinale(t *testing.T) {
	if !canOpenRound(150) {
		t.Error("canOpenRound(150) = false, want a round when a quarter of the question is left")
	}
	if canOpenRound(finaleShareS(80) + MinRoundS - 1) {
		t.Error("canOpenRound opened a round that leaves the finale short of its share")
	}
	// A question that is nearly out of time has room for the answer and nothing else.
	if canOpenRound(45) {
		t.Error("canOpenRound(45) = true, want the answer's clock kept when that is all that is left")
	}
}

// TestTheSharesFitInsideTheQuestionTogether pins the invariant the old per-node caps broke: opening
// + finale, the two things every question pays, must fit inside the question with room for the
// research between them.
func TestTheSharesFitInsideTheQuestionTogether(t *testing.T) {
	for _, remaining := range []float64{90, 120, 150, 180} {
		opening, finale := openingShareS(remaining), finaleShareS(remaining)
		if opening+finale >= remaining {
			t.Errorf("at %vs left: opening %.0fs + finale %.0fs leaves no research room", remaining, opening, finale)
		}
		if researchRoomS(remaining) < MinRoundS {
			t.Errorf("at %vs left: research room %.0fs is below the round minimum %.0fs", remaining, researchRoomS(remaining), MinRoundS)
		}
	}
}

// TestPrefetchSpendsWhatThePlannerLeftOfTheOpening pins the opening as ONE deadline rather than two
// caps: the planner arms it, and the fan-out gets whatever the decomposition left of it.
//
// When the decomposition used ALL of it the fan-out still runs, on its own floor: upstream's fix
// (47a444039) removed nodeClock's "room gone -> zero" branch, because returning zero built a
// context.WithTimeout(ctx, 0), whose first ctx.Done() check admitted nothing — the run then reached
// the answer with an empty pool. See TestNodeClockKeepsItsFloorOnASpentBudget for the property.
func TestPrefetchSpendsWhatThePlannerLeftOfTheOpening(t *testing.T) {
	st := NewAgenticState("q", "", 3, nil)
	st.Deadline = time.Now().Add(TotalBudgetS * time.Second)

	if got := st.openingLeftS(); got < OpeningMaxS-0.01 || got > OpeningMaxS {
		t.Errorf("without a planner-set deadline the opening reports %v, want its own share %v", got, OpeningMaxS)
	}
	st.OpeningStarted = time.Now()
	st.OpeningDeadline = st.OpeningStarted.Add(7 * time.Second)
	if got := st.openingLeftS(); got < 6 || got > 7.5 {
		t.Errorf("openingLeftS() = %v, want the ~7s left of the opening", got)
	}
	st.OpeningDeadline = st.OpeningStarted.Add(-time.Second)
	if got := nodeClock(PrefetchTimeoutS, OpeningMinS, st.openingLeftS()); got != OpeningMinS {
		t.Errorf("prefetch clock = %v after the opening was spent, want its floor %v (the floor wins)",
			got, OpeningMinS)
	}
}

// TestTheSessionRegistryIsRecordedWithoutAnAnswer pins the difference between the answer and the
// REGISTRY: SessionEvidenceRefs is the runtime's list of what the session was SHOWN as [ID:n], and
// the answer stage resolves its markers against it.
//
// It used to be assigned only when the session answered, so a round that read forty passages and
// wrote no answer left the pool with no registry at all. Measured 2026-09-20 (三国/关羽): 44
// passages read, 0 patches, no answer — and the composed answer carried no citation for any member.
//
// The last two assertions are the ones that keep it honest in both directions: an empty list must
// not CLEAR a registry an earlier round wrote, and a nil pool must not panic.
func TestTheSessionRegistryIsRecordedWithoutAnAnswer(t *testing.T) {
	kb := &runtime.Kbinfos{}
	recordSessionEvidence(kb, []string{"c-hua", "c-yan"})
	if got := strings.Join(kb.SessionEvidenceRefs, ","); got != "c-hua,c-yan" {
		t.Fatalf("SessionEvidenceRefs = %v, want the passages the session was shown", kb.SessionEvidenceRefs)
	}

	recordSessionEvidence(kb, nil)
	if len(kb.SessionEvidenceRefs) != 2 {
		t.Errorf("an empty registry cleared the run's record: %v", kb.SessionEvidenceRefs)
	}
	recordSessionEvidence(nil, []string{"c-hua"})
}

// TestTheOpeningRankingFusesTheChannelsByReciprocalRank pins the fusion that turns the opening's
// parallel legs into ONE order.
//
// The legs' scores are not comparable (a BM25 score, a vector similarity and a claim's fused rank
// live on different scales), so the fusion is reciprocal rank — the idiom the claim rows already
// use. Two facts are pinned here: the keyword channel outranks the semantic one at equal rank (it
// is the deliberate surface probe, and its hits carry the corpus's own wording), and a passage
// several clues reached outranks one a single clue reached.
func TestTheOpeningRankingFusesTheChannelsByReciprocalRank(t *testing.T) {
	one := []fanoutPair{{
		exact:    []map[string]any{{"chunk_id": "c-keyword"}},
		semantic: []map[string]any{{"chunk_id": "c-semantic"}},
	}}
	got := rankOpening([]string{"关羽 斩"}, one, nil)
	if len(got) != 2 || got[0] != "c-keyword" {
		t.Errorf("ranking = %v, want the keyword hit first", got)
	}

	// Reached by TWO clues vs once: the fusion sums over (clue × channel).
	two := []fanoutPair{
		{exact: []map[string]any{{"chunk_id": "c-both"}}},
		{exact: []map[string]any{{"chunk_id": "c-both"}}, semantic: []map[string]any{{"chunk_id": "c-once"}}},
	}
	got = rankOpening([]string{"q1", "q2"}, two, nil)
	if len(got) != 2 || got[0] != "c-both" {
		t.Errorf("ranking = %v, want the passage two clues reached first", got)
	}

	// A claim row leads both channels: it is the same text, verbatim and compact.
	withClaim := rankOpening([]string{"q"}, []fanoutPair{{
		exact: []map[string]any{{"chunk_id": "c-keyword"}},
	}}, [][]map[string]any{{{"chunk_id": "claim_1"}}})
	if len(withClaim) != 2 || withClaim[0] != "claim_1" {
		t.Errorf("ranking = %v, want the claim row first", withClaim)
	}

	// Every id appears once, whatever the legs did, and an empty opening is empty.
	dup := rankOpening([]string{"q"}, []fanoutPair{{
		exact:    []map[string]any{{"chunk_id": "c-1"}, {"chunk_id": "c-1"}},
		semantic: []map[string]any{{"chunk_id": "c-1"}},
	}}, nil)
	if len(dup) != 1 {
		t.Errorf("ranking = %v, want one entry for a passage three legs returned", dup)
	}
	if len(rankOpening(nil, nil, nil)) != 0 {
		t.Error("an empty opening produced a ranking")
	}
}

// TestTheOpeningDepthMatchesTheQueryCountItAsksFor pins the ONE coupling that two A/Bs failed on.
//
// The opening's depth and the number of queries the planner writes are one decision. Measured over the
// FRAMES set on this machine:
//
//	                              queries  bm25/hybrid/quota/budget   accuracy
//	a2110c7af (opening spec)      1-4      60 / 30 /  4 /  30          0.900
//	4dac9a06a → 2026-09-21 14:17  8-12     200 / 60 /  8 / 400         0.850
//	revert WIDTHS ONLY            8-12     60 / 30 /  4 /  30          0.700
//	revert WIDTHS + QUERIES       1-4      60 / 30 /  4 /  30          0.684
//
// The third row is the trap: 8-12 queries against a 30-passage admission is ~3 passages per query, so a
// question whose evidence is one table (758's mayor list, 25's tale-of-the-tape, 692's second waterfall)
// loses it while the plan still looks complete. Pinned EQUAL, both sides, so a later edit that changes
// one without the other fails here instead of in a benchmark.
func TestTheOpeningDepthMatchesTheQueryCountItAsksFor(t *testing.T) {
	if fanoutBM25TopN != 200 || fanoutHybridTopN != 60 || fanoutSemanticQuota != 8 || rawSnippetQuota != 400 {
		t.Errorf("opening depth = bm25 %d / hybrid %d / semantic-only %d / raw budget %d, want 200/60/8/400 "+
			"(the 8-12 first_queries the planner asks for — see action_initialize_state.md)",
			fanoutBM25TopN, fanoutHybridTopN, fanoutSemanticQuota, rawSnippetQuota)
	}
}

// TestThePlanCarriesTheProbesTheTableDeclared pins the one place the planner's act words are USED.
//
// A member-set slot declares the actor and the words the SOURCE uses for the deed (Variable.Terms /
// Subject). Nothing read those fields after the coverage engine was removed, so the plan was whatever
// queries the model happened to write — measured 2026-09-20 (三国/关羽): three queries, none of them the
// act-word probes the table had already declared, while a passage phrased "砍为两段" is reached by the
// probe 关羽 砍 and by nothing else.
//
// The combination is mechanical: the words are the planner's own, and the values stay opaque (no
// splitting, counting or comparison of a slot's text).
func TestThePlanCarriesTheProbesTheTableDeclared(t *testing.T) {
	table := runtime.State{State: []runtime.Variable{
		{ID: 0, Type: "count", Candidate: strPtr("16")},
		{ID: 1, Type: "entity", Terms: []string{"斩", "砍"}, Subjects: []string{"关羽", "云长"}},
		{ID: 2, Type: "date", Candidate: strPtr("1858")},
	}}
	got := runtime.DeclaredProbes(table)
	// The ENTITY's spellings, and nothing else. The act words are gone: which verb a source uses for a
	// deed is a fact about the source ("挥为两段", "劈管亥于马下", "手起一刀，刺于马下", "关公刀起处"), and a
	// probe built from a guessed verb reaches the passages phrased the way the guess predicted and misses
	// the rest. Measured 2026-09-21 (三国演义.txt, 1718 chunks): the verb-guessed probes reached 162 of the
	// 266 chunks naming the actor, and the same question answered eleven, fourteen, seventeen or four
	// members on consecutive runs of the SAME probe set.
	want := []string{"关羽", "云长"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("declaredProbes = %v, want %v (the entity's spellings, as declared)", got, want)
	}

	// No entity declared: nothing is asked. The act words are not a substitute — they ARE the guess this
	// function no longer makes.
	bare := runtime.State{State: []runtime.Variable{{ID: 0, Type: "entity", Terms: []string{"斩", "砍"}}}}
	if got := runtime.DeclaredProbes(bare); len(got) != 0 {
		t.Errorf("declaredProbes = %v from a table that declares no entity, want none", got)
	}

	// Every spelling the plan declares is asked: the plan writes the spellings the QUESTION uses, so there
	// is nothing here to select, rank or drop.
	wide := runtime.State{State: []runtime.Variable{{ID: 0, Type: "entity", Subjects: []string{"关羽", "关公", "云长", "关云长"}}}}
	if got, want := runtime.DeclaredProbes(wide), []string{"关羽", "关公", "云长", "关云长"}; !reflect.DeepEqual(got, want) {
		t.Errorf("declaredProbes = %v, want %v (as declared)", got, want)
	}

	// A table that declares nothing contributes nothing — never a guess.
	if got := runtime.DeclaredProbes(runtime.State{State: []runtime.Variable{{ID: 0, Type: "date", Candidate: strPtr("1858")}}}); len(got) != 0 {
		t.Errorf("declaredProbes = %v from a table that declares no act words, want none", got)
	}

	// The legs are bounded: each probe is a retrieval leg, and the opening has one clock.
	var many []runtime.Variable
	for i := 0; i < 12; i++ {
		many = append(many, runtime.Variable{ID: i, Type: "entity", Terms: []string{"斩", "砍"}, Subjects: []string{"关羽", "云长"}})
	}
	if got := runtime.DeclaredProbes(runtime.State{State: many}); len(got) > runtime.DeclaredProbesMax {
		t.Errorf("declaredProbes = %d probe(s), want at most %d", len(got), runtime.DeclaredProbesMax)
	}
}

// TestTheAnswerCitesWithTheHandlesTheModelWroteRenumberedCompactly pins the citation step.
//
// The model cites with the handles the run printed beside its evidence, and those numbers are the
// RETRIEVAL's, not the answer's: the seed numbers every window the scan delivered (up to 265 in one
// 三国/关羽 run), so an answer resting on sixteen passages came back carrying [ID:265] — a number that
// resolves, and still reads as nonsense. The code does one mechanical thing: the first passage the ANSWER
// cites becomes [ID:0], the next passage it has not cited yet [ID:1], and so on, and the client resolves
// those numbers against exactly that list.
//
// Nothing is matched or inferred: the handle the model wrote IS the claim about which passage it rests
// on. Only what cannot resolve is dropped — a marker that is not a handle at all (a chunk id copied out
// of a tool result) and a number that names no passage this run published.
func TestTheAnswerCitesWithTheHandlesTheModelWroteRenumberedCompactly(t *testing.T) {
	kb := &runtime.Kbinfos{}
	kb.Chunks = []map[string]any{
		{"chunk_id": "c-a", "doc_id": "d1", "content": "云长提华雄之头，掷于地上。"},
		{"chunk_id": "c-b", "doc_id": "d1", "content": "颜良措手不及，被云长手起一刀，刺于马下。"},
		{"chunk_id": "c-c", "doc_id": "d1", "content": "云长舞动大刀，纵马飞迎。"},
	}
	kb.SessionEvidenceRefs = []string{"c-a", "c-b", "c-c"}

	ans := "1. 丙——飞迎 [ID:2]" + "\n" +
		"2. 甲——掷于地上 [ID:0]" + "\n" +
		"3. 丙再说一次 [ID:2]" + "\n" +
		"4. 某——[ID:ffd9977ab2ef7071]" + "\n" +
		"5. 无此人——[ID:99]"
	resp := &RunResponse{}
	useSessionAnswer(kb, resp, ans)

	if strings.Contains(resp.Answer, "265") || strings.Contains(resp.Answer, "ffd9977ab2ef7071") {
		t.Errorf("answer = %q, want no handle the run did not publish", resp.Answer)
	}
	lines := strings.Split(resp.Answer, "\n")
	if !strings.HasSuffix(strings.TrimSpace(lines[0]), "[ID:0]") ||
		!strings.HasSuffix(strings.TrimSpace(lines[1]), "[ID:1]") {
		t.Errorf("answer = %q, want the cited passages numbered in the order the answer cites them", resp.Answer)
	}
	// The same passage cited twice keeps the same number, and the two unresolvable markers are gone.
	if !strings.HasSuffix(strings.TrimSpace(lines[2]), "[ID:0]") {
		t.Errorf("answer = %q, want a repeated passage to keep its number", resp.Answer)
	}
	if strings.Contains(lines[3], "[ID:") || strings.Contains(lines[4], "[ID:") {
		t.Errorf("answer = %q, want the unresolvable markers dropped", resp.Answer)
	}
	if want := []string{"c-c", "c-a"}; !reflect.DeepEqual(kb.CiteChunkIDs, want) {
		t.Errorf("CiteChunkIDs = %v, want %v (the passages the answer cites, in answer order)", kb.CiteChunkIDs, want)
	}

	// An answer that cites nothing is left exactly as written, and the registry stays the citation list.
	bare := &RunResponse{}
	useSessionAnswer(kb, bare, "关羽斩将甚多。")
	if bare.Answer != "关羽斩将甚多。" {
		t.Errorf("answer = %q, want it unchanged when nothing is cited", bare.Answer)
	}
}

// TestAnAnswerThatNamesPassagesByIdsStillGetsCitations pins the fallback (see resolveLooseHandles).
//
// A DOCUMENT-LEVEL answer has no passage handle to write — metadata_search returns doc_ids and no
// passages — so the model writes the id it was given. Measured 2026-09-22, two runs in a row came back
// with zero citations (raw_markers=[], resolved_blocks=0) while every source the answer named was in
// the pool: one answer wrote chunk ids out of a tool result, the other wrote `doc <id>`. The fallback
// resolves both against the pool, and leaves an id the run never published exactly as written.
func TestAnAnswerThatNamesPassagesByIdsStillGetsCitations(t *testing.T) {
	kb := &runtime.Kbinfos{}
	kb.Chunks = []map[string]any{
		{"chunk_id": "6e9e890eb6d944fda75d71ed5e6f8802", "doc_id": "0ee41271ba9b42e9b73618054fc351c7", "content": "05_网络安全日志审计与电子证据保留规范"},
		{"chunk_id": "28569a3e11e2490eb16c5bf415391f89", "doc_id": "057f5d7af5d743418e055ddef7b1e867", "content": "06_事件复盘与升级规则"},
	}
	kb.SessionEvidenceRefs = []string{"6e9e890eb6d944fda75d71ed5e6f8802", "28569a3e11e2490eb16c5bf415391f89"}

	ans := "1. 日志审计规范（doc 0ee41271ba9b42e9b73618054fc351c7）\n" +
		"2. 事件升级流程 `28569a3e11e2490eb16c5bf415391f89`\n" +
		"3. 某篇未发布文档（doc deadbeefdeadbeefdeadbeefdeadbeef）"
	resp := &RunResponse{}
	useSessionAnswer(kb, resp, ans)

	lines := strings.Split(resp.Answer, "\n")
	// The replacement is a BARE [ID:n]: the model's own "doc" wording and its backticks go WITH the
	// id, because the citation contract has no such form (citation_prompt.md: [ID:i], nothing else).
	if got := strings.TrimSpace(lines[0]); got != "1. 日志审计规范（[ID:0]）" {
		t.Errorf("line 1 = %q, want the doc id replaced by a bare citation number", got)
	}
	if got := strings.TrimSpace(lines[1]); got != "2. 事件升级流程 [ID:1]" {
		t.Errorf("line 2 = %q, want the backticked chunk id replaced by a bare citation number", got)
	}
	if strings.Contains(resp.Answer, "0ee41271ba9b42e9b73618054fc351c7") ||
		strings.Contains(resp.Answer, "28569a3e11e2490eb16c5bf415391f89") {
		t.Errorf("answer = %q, want the resolved ids replaced by numbers, not echoed", resp.Answer)
	}
	// An id this run never published is left exactly as the model wrote it — prefix and all.
	if !strings.Contains(lines[2], "doc deadbeefdeadbeefdeadbeefdeadbeef") {
		t.Errorf("answer = %q, want an unpublished id left as written", resp.Answer)
	}
	// The doc id resolves to the first chunk of that document: the unit the client can open.
	want := []string{"6e9e890eb6d944fda75d71ed5e6f8802", "28569a3e11e2490eb16c5bf415391f89"}
	if !reflect.DeepEqual(kb.CiteChunkIDs, want) {
		t.Errorf("CiteChunkIDs = %v, want %v (the passages the answer names, in answer order)", kb.CiteChunkIDs, want)
	}
}

// metadataScopedRetriever serves a hit ONLY to a doc-scoped request, so a test can tell
// which fan-out channel admitted what: channels A/B search the whole corpus (no scope),
// channel C searches inside the documents the metadata filter selected.
type metadataScopedRetriever struct {
	scopes [][]string
}

func (r *metadataScopedRetriever) Retrieve(_ context.Context, req runtime.RetrieveRequest) ([]map[string]any, error) {
	if len(req.DocScope) == 0 {
		return nil, nil
	}
	r.scopes = append(r.scopes, req.DocScope)
	return []map[string]any{{"id": "meta-1", "doc_id": "d1", "content": "Culdcept tower height is 42 m."}}, nil
}

// fanoutMetadataResolver is a scripted MetadataResolver: the push-down always answers with
// the configured documents, the flattened view answers with metas, and the filters/logic it
// was handed are recorded.
type fanoutMetadataResolver struct {
	ids     []string
	metas   common.MetaData
	calls   int
	filters []map[string]any
	logic   string
	// docMeta backs the metadata_search context block (doc_id → fields).
	docMeta map[string]map[string]any
}

func (m *fanoutMetadataResolver) FilterDocIDsByMetaPushdown(_ context.Context, _ []string, filters []map[string]any, logic string) ([]string, bool) {
	m.calls++
	m.filters = filters
	m.logic = logic
	return m.ids, true
}

func (m *fanoutMetadataResolver) GetFlattedMetaByKBs(context.Context, []string) (common.MetaData, error) {
	return m.metas, nil
}

// MetadataForDocIDs answers the metadata_search context block. These tests observe the
// selection, not the context block, so it stays empty unless a case sets docMeta.
func (m *fanoutMetadataResolver) MetadataForDocIDs(context.Context, []string, []string) (map[string]map[string]any, error) {
	return m.docMeta, nil
}

// TestFanoutSearchMetadataChannelUsesCatalogFields pins channel C end to end with the
// catalog-driven vocabulary: the model reads a sub-question and names a field the SESSION
// offers — here `author`, not the historical hard-coded `title` — and the retrieval runs inside
// the documents that field matches, even though the corpus-wide channels returned nothing.
func TestFanoutSearchMetadataChannelUsesCatalogFields(t *testing.T) {
	ctx := context.Background()
	r := &metadataScopedRetriever{}
	resolver := &fanoutMetadataResolver{ids: []string{"d1"}}
	mdl := &scriptedModel{}
	mdl.push(`{"filters": [[{"key":"author","op":"contains","value":"OmiyaSoft"}]]}`)
	deps := RAGTools{
		Search: runtime.SearchDeps{
			Backend:          r,
			KbIDs:            []string{"kb1"},
			HasEmbedder:      true,
			MetadataResolver: resolver,
		},
		Model: mdl,
		Tools: &runtime.Toolset{ThinkingMode: "high", MetadataFields: &runtime.MetadataCatalog{
			Keys: []string{"author", "title"},
		}},
	}
	st := &AgenticState{KB: &runtime.Kbinfos{}}

	added, _ := FanoutSearch(ctx, deps, st, []string{"what is the Culdcept tower height"}, 8, true)

	if added != 1 || len(st.KB.Chunks) != 1 {
		t.Fatalf("added = %d, pool = %d; want the metadata channel's one hit", added, len(st.KB.Chunks))
	}
	if got := runtime.ChunkIDOf(st.KB.Chunks[0]); got != "meta-1" {
		t.Errorf("pooled chunk = %q, want the channel-C hit", got)
	}
	if len(mdl.seen) != 1 {
		t.Errorf("model calls = %d, want exactly ONE extraction call for the batch", len(mdl.seen))
	}
	if resolver.calls != 1 || resolver.logic != "or" {
		t.Errorf("resolver calls = %d logic = %q, want one OR-combined lookup", resolver.calls, resolver.logic)
	}
	if len(resolver.filters) != 1 || resolver.filters[0]["key"] != "author" ||
		resolver.filters[0]["op"] != "contains" || resolver.filters[0]["value"] != "OmiyaSoft" {
		t.Errorf("filters = %v, want the author-contains condition the catalog offered", resolver.filters)
	}
	if len(r.scopes) != 1 || len(r.scopes[0]) != 1 || r.scopes[0][0] != "d1" {
		t.Errorf("scoped requests = %v, want the search restricted to the matched document", r.scopes)
	}
}

// TestFanoutSearchSkipsMetadataChannelUnlessAsked pins the gate: the rewrite round (and
// every other caller that does not ask for the metadata channel) must not pay the
// entity-extraction model call, and must not resolve documents.
func TestFanoutSearchSkipsMetadataChannelUnlessAsked(t *testing.T) {
	ctx := context.Background()
	r := &metadataScopedRetriever{}
	resolver := &fanoutMetadataResolver{ids: []string{"d1"}}
	mdl := &scriptedModel{}
	mdl.push(`{"entities": [["Culdcept"]]}`)
	deps := RAGTools{
		Search: runtime.SearchDeps{
			Backend:          r,
			KbIDs:            []string{"kb1"},
			HasEmbedder:      true,
			MetadataResolver: resolver,
		},
		Model: mdl,
	}
	st := &AgenticState{KB: &runtime.Kbinfos{}}

	if added, _ := FanoutSearch(ctx, deps, st, []string{"what is the Culdcept tower height"}, 8, false); added != 0 {
		t.Errorf("added = %d, want 0 without the metadata channel", added)
	}
	if len(mdl.seen) != 0 {
		t.Errorf("model calls = %d, want none: entity extraction must not run", len(mdl.seen))
	}
	if resolver.calls != 0 || len(r.scopes) != 0 {
		t.Errorf("resolver calls = %d scoped searches = %d, want none", resolver.calls, len(r.scopes))
	}
}

// TestFanoutSearchGatesMetadataChannelOnFilterableFields pins the gate that keeps Channel C
// from being pure cost: a session whose catalog offers NO filterable field pays neither the
// extraction call nor one guaranteed-empty scoped retrieval per sub-question.
//
// The gate is the field VOCABULARY, not one particular field: a catalog that offers `author`
// and no `title` is perfectly usable, because the model may name whichever field the
// sub-question fits. Nothing is baked in as a fallback — a caller that wired no toolset, or a
// session whose catalog is empty because the dataset carries no metadata, is skipped too.
func TestFanoutSearchGatesMetadataChannelOnFilterableFields(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name      string
		cat       *runtime.MetadataCatalog
		noToolset bool
		want      int
	}{
		{
			name: "wired session, no metadata",
			cat:  nil,
			want: 0,
		},
		{
			name: "wired session, empty catalog",
			cat:  &runtime.MetadataCatalog{},
			want: 0,
		},
		{
			name:      "caller wired no tool surface",
			noToolset: true,
			want:      0,
		},
		{
			name: "catalog offers a non-title field",
			cat: &runtime.MetadataCatalog{
				Keys:    []string{"author"},
				Samples: map[string][]runtime.MetadataSample{"author": {{Value: "OmiyaSoft", Docs: 1}}},
			},
			want: 1,
		},
	}
	for _, c := range cases {
		r := &metadataScopedRetriever{}
		resolver := &fanoutMetadataResolver{ids: []string{"d1"}}
		mdl := &scriptedModel{}
		mdl.push(`{"filters": [[{"key":"author","op":"contains","value":"OmiyaSoft"}]]}`)
		deps := RAGTools{
			Search: runtime.SearchDeps{
				Backend:          r,
				KbIDs:            []string{"kb1"},
				HasEmbedder:      true,
				MetadataResolver: resolver,
			},
			Model: mdl,
		}
		if !c.noToolset {
			deps.Tools = &runtime.Toolset{ThinkingMode: "high", MetadataFields: c.cat}
		}
		st := &AgenticState{KB: &runtime.Kbinfos{}}

		added, _ := FanoutSearch(ctx, deps, st, []string{"what is the Culdcept tower height"}, 8, true)
		if added != c.want {
			t.Errorf("%s: added = %d, want %d", c.name, added, c.want)
		}
		if c.want == 0 {
			if len(mdl.seen) != 0 {
				t.Errorf("%s: model calls = %d, want none — the extraction must not run when no field is filterable", c.name, len(mdl.seen))
			}
			if resolver.calls != 0 || len(r.scopes) != 0 {
				t.Errorf("%s: resolver calls = %d scoped searches = %d, want none", c.name, resolver.calls, len(r.scopes))
			}
		}
	}
}

// TestParseFanoutFiltersShapesAndGuards pins the reply shapes and the guards. The guard set is
// what keeps the channel from filtering on something the session never offered: a field outside
// the catalog (which is also how the blacklist holds here), a WIDENING operator, a copied
// sub-question, an answer sentence, an over-long value, a duplicate and an empty value are all
// dropped, and at most three conditions survive per sub-question.
func TestParseFanoutFiltersShapesAndGuards(t *testing.T) {
	fanouts := []string{"q one", "q two"}
	allowed := map[string]bool{"title": true, "author": true}

	// Positional arrays: one condition-group per sub-question, in order.
	got := parseFanoutFilters(`{"filters": [[`+
		`{"key":"title","op":"contains","value":"Alpha Corp"},`+
		`{"key":"author","op":"=","value":"Beta"}],`+
		`[{"key":"title","op":"contains","value":"Gamma"}]]}`, fanouts, allowed)
	if len(got) != 2 || len(got[0]) != 2 || got[0][0]["value"] != "Alpha Corp" ||
		got[0][1]["key"] != "author" || got[1][0]["value"] != "Gamma" {
		t.Errorf("positional = %v", got)
	}

	// Dict form matched by sub-question text: the unmatched one stays empty.
	got = parseFanoutFilters(`{"filters": [{"sub_question":"q two","filters":[{"key":"title","op":"contains","value":"Delta"}]}]}`, fanouts, allowed)
	if len(got[0]) != 0 || len(got[1]) != 1 || got[1][0]["value"] != "Delta" {
		t.Errorf("by-text = %v", got)
	}

	// A flat list of conditions is read POSITIONALLY: one condition per sub-question, so a
	// single-condition list only fills the first one.
	got = parseFanoutFilters(`{"filters": [{"key":"author","op":"=","value":"Alice"}]}`, fanouts, allowed)
	if len(got[0]) != 1 || got[0][0]["value"] != "Alice" || len(got[1]) != 0 {
		t.Errorf("flat = %v", got)
	}

	// Guards: an unoffered field, a widening operator, an over-long value, an 11-word value, a
	// prose/citation value, a duplicate and an empty value are dropped; the cap is three.
	long := strings.Repeat("x", metadataValueMaxChars+1)
	reply := `{"filters": [[` +
		`{"key":"question_id","op":"contains","value":"444"},` +
		`{"key":"title","op":"not contains","value":"Alpha"},` +
		`{"key":"title","op":"contains","value":"` + long + `"},` +
		`{"key":"title","op":"contains","value":"one two three four five six seven eight nine ten eleven"},` +
		`{"key":"title","op":"contains","value":"see https://example.com"},` +
		`{"key":"title","op":"contains","value":"Dup"},` +
		`{"key":"title","op":"contains","value":"Dup"},` +
		`{"key":"title","op":"contains","value":""},` +
		`{"key":"title","op":"contains","value":"e1"},` +
		`{"key":"title","op":"contains","value":"e2"},` +
		`{"key":"title","op":"contains","value":"e3"},` +
		`{"key":"title","op":"contains","value":"e4"}]]}`
	got = parseFanoutFilters(reply, fanouts, allowed)
	want := []string{"Dup", "e1", "e2"}
	if len(got[0]) != len(want) {
		t.Fatalf("guarded = %v, want %v", got[0], want)
	}
	for i, w := range want {
		if got[0][i]["value"] != w {
			t.Errorf("guarded[%d] = %v, want %q", i, got[0][i], w)
		}
	}

	// 'in' keeps its list (the value SET) and needs at least one entry.
	got = parseFanoutFilters(`{"filters": [[{"key":"title","op":"in","value":["A","B"]}]]}`, fanouts, allowed)
	if len(got[0]) != 1 {
		t.Fatalf("in = %v, want one condition", got)
	}
	if list, ok := got[0][0]["value"].([]any); !ok || len(list) != 2 {
		t.Errorf("in value = %v, want the list kept", got[0][0]["value"])
	}
	if got := parseFanoutFilters(`{"filters": [[{"key":"title","op":"in","value":[]}]]}`, fanouts, allowed); len(got[0]) != 0 {
		t.Errorf("empty 'in' = %v, want it dropped", got[0])
	}

	// No JSON at all: nothing parsed, and the fan-outs are still order-aligned.
	got = parseFanoutFilters("no json here", fanouts, allowed)
	if len(got) != 2 || len(got[0]) != 0 || len(got[1]) != 0 {
		t.Errorf("non-JSON = %v", got)
	}
}

// TestExtractFanoutFiltersDegradesToNothing pins the best-effort contract: no chat model, no
// sub-questions, or a reply with nothing usable all mean NO conditions (nil, the channel
// simply skipped), never a failure.
func TestExtractFanoutFiltersDegradesToNothing(t *testing.T) {
	if got := ExtractFanoutFilters(context.Background(), RAGTools{}, []string{"q"}); got != nil {
		t.Errorf("filters = %v, want nil without a model", got)
	}
	if got := ExtractFanoutFilters(context.Background(), RAGTools{Model: &scriptedModel{}}, nil); got != nil {
		t.Errorf("filters = %v, want nil without sub-questions", got)
	}
	if got := ExtractFanoutFilters(context.Background(), RAGTools{Model: &scriptedModel{}}, []string{"q"}); got != nil {
		t.Errorf("filters = %v, want nil when the caller wired no tool surface", got)
	}
}

// slotResearchWithMetadataTool builds one action session whose tool executor is the REAL
// metadata_search executor, so a tool call made by the model is observed end to end: the
// resolver records the filter and answers with the documents the selector returns.
func slotResearchWithMetadataTool(resolver *fanoutMetadataResolver, retriever *corpusRetriever, cat *runtime.MetadataCatalog, replies []*runtime.ModelReply) (*AgenticState, *runtime.Kbinfos, *fakeModel) {
	kb := &runtime.Kbinfos{}
	sd := runtime.SearchDeps{
		Backend:          retriever,
		KbIDs:            []string{"kb1"},
		HasEmbedder:      true,
		KB:               kb,
		MetadataResolver: resolver,
	}
	mdl := &fakeModel{replies: replies}
	st := &AgenticState{
		Question: "who wrote it?",
		KB:       kb,
		SlotTable: runtime.NewState([]runtime.Variable{
			{ID: 0, Type: "aspect", QuestionClues: []string{"who wrote it?"}},
		}, 0, nil),
	}
	deps := runtime.SessionDeps{
		Model: mdl,
		Tools: &runtime.Toolset{
			ThinkingMode:   "high",
			MetadataFields: cat,
			Exec:           runtime.NewSearchExecutor(sd, runtime.RunRequest{ThinkingMode: "high"}),
		},
		KB: kb,
	}
	RunSlotResearchPass(context.Background(), context.Background(), deps, "who wrote it?", st, 60)
	return st, kb, mdl
}

// TestSlotResearchSessionUsesCatalogFieldForMetadataSearch pins the whole chain in one run:
// the catalog advertises `author`, the model spends its one metadata_search on that field,
// and the real executor resolves documents by it. The resolver seeing the author condition
// is the observation that the advertised field is the one that reaches the index — the
// schema, the tool call and the executor all agreeing.
func TestSlotResearchSessionUsesCatalogFieldForMetadataSearch(t *testing.T) {
	resolver := &fanoutMetadataResolver{
		ids:   []string{"doc-culdcept"},
		metas: common.MetaData{"author": {"Alice": {"doc-culdcept"}}, "title": {"Culdcept History": {"doc-culdcept"}}},
	}
	cat := &runtime.MetadataCatalog{
		Keys:    []string{"author", "title"},
		Samples: map[string][]runtime.MetadataSample{"author": {{Value: "Alice", Docs: 1}}},
	}
	_, _, mdl := slotResearchWithMetadataTool(resolver, &corpusRetriever{}, cat, []*runtime.ModelReply{
		{
			Content: "Narrowing by author.",
			ToolCalls: []runtime.ToolCall{{
				ID:   "call_0",
				Name: "metadata_search",
				Args: map[string]any{
					"filters": []any{map[string]any{"key": "author", "op": "contains", "value": "Alice"}},
				},
			}},
		},
		{Content: `<state>{"new_states": []}</state>`},
	})

	if resolver.calls == 0 {
		t.Fatal("the session never resolved documents by metadata; the advertised field did not reach the executor")
	}
	if len(resolver.filters) != 1 || resolver.filters[0]["key"] != "author" {
		t.Errorf("resolver filters = %v, want the author condition the catalog advertised", resolver.filters)
	}
	if mdl.calls < 2 {
		t.Errorf("model calls = %d, want the session to have continued after the tool result", mdl.calls)
	}
}

// TestSlotResearchSessionSurvivesMetadataSearchWithoutMetadata pins the requirement that a
// metadata-free dataset must not derail the run: the index carries NO field, the model still
// spends its one metadata_search on `title`, and the session must read that as a query-level
// miss and carry on with the next tool — never abort, never disable the run.
func TestSlotResearchSessionSurvivesMetadataSearchWithoutMetadata(t *testing.T) {
	resolver := &fanoutMetadataResolver{} // no ids, no metas: a dataset with no metadata
	retriever := &corpusRetriever{}
	// No catalog: the shipped title-only schema is what the model sees.
	_, _, mdl := slotResearchWithMetadataTool(resolver, retriever, nil, []*runtime.ModelReply{
		{
			Content: "Trying a title filter.",
			ToolCalls: []runtime.ToolCall{{
				ID:   "call_0",
				Name: "metadata_search",
				Args: map[string]any{
					"filters": []any{map[string]any{"key": "title", "op": "contains", "value": "Culdcept"}},
				},
			}},
		},
		{
			Content: "Falling back to retrieval.",
			ToolCalls: []runtime.ToolCall{{
				ID:   "call_1",
				Name: "retrieve",
				Args: map[string]any{"query": []any{"who wrote it?"}},
			}},
		},
		{Content: `<state>{"new_states": []}</state>`},
	})

	// The metadata miss must not have stopped the loop: the session went on to retrieval.
	if len(retriever.calls) == 0 {
		t.Fatalf("retriever calls = 0; a metadata-free dataset aborted the session instead of degrading to retrieval")
	}
	// The call COUNT is this branch's own clock model, not upstream's: the session computes how many
	// search turns the round's clock affords (pace measured, the answer's reserve subtracted), and with
	// the 60s this fixture passes it affords ONE — so the turn that the metadata miss did not derail is
	// spent on retrieval instead of on a third model call. What the fix owns is asserted above: the miss
	// is a query-level miss, the session degrades to retrieval, and it never aborts.
	if mdl.calls < 2 {
		t.Errorf("model calls = %d, want the session to have continued past the metadata miss", mdl.calls)
	}
	if resolver.calls != 0 {
		t.Errorf("push-down calls = %d, want none: an unknown key is rejected before the index is touched", resolver.calls)
	}
}
