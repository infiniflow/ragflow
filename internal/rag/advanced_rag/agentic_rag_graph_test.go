package advanced_rag

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"ragflow/internal/entity"
	"ragflow/internal/rag/advanced_rag/harness"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// scriptedModel answers with a fixed set of canned replies, one per call,
// looping on the last if exhausted. It satisfies harness.SessionModel. seen
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

func (m *scriptedModel) Complete(ctx context.Context, msgs []schema.Message, _ []harness.ToolSpec) (*harness.ModelReply, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seen = append(m.seen, msgs)
	if len(m.replies) == 0 {
		return &harness.ModelReply{Content: ""}, nil
	}
	r := m.replies[m.idx]
	if m.idx < len(m.replies)-1 {
		m.idx++
	}
	return &harness.ModelReply{Content: r}, nil
}

// promptRoutedModel answers by inspecting the prompt instead of by call order,
// so a test survives the graph adding or reordering node LLM calls. The SCA call
// returns insufficient on its FIRST invocation and sufficient afterwards, which
// is what drives one rewrite round.
type promptRoutedModel struct {
	mu      sync.Mutex
	scaSeen int
}

func (m *promptRoutedModel) Complete(ctx context.Context, msgs []schema.Message, _ []harness.ToolSpec) (*harness.ModelReply, error) {
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
		return &harness.ModelReply{Content: `{"fanouts": ["when built", "where located"]}`}, nil
	case strings.Contains(text.String(), "research strategist"):
		return &harness.ModelReply{Content: `{"slots":[{"id":0,"type":"aspect","question":"when built","clues":["1865"]},{"id":1,"type":"aspect","question":"where located","clues":["geneva"]}], "first_queries":["when built","where located"]}`}, nil
	case strings.Contains(text.String(), "Sufficient Context Agent"):
		m.scaSeen++
		if m.scaSeen == 1 {
			return &harness.ModelReply{Content: `{"is_sufficient": false, "score": 0.2, "contradictions": [], "reasoning": "missing the builder", "sub_queries": [{"missing_fact": "who built it", "search_hint": "builder", "satisfied": false}], "claims": {}}`}, nil
		}
		return &harness.ModelReply{Content: `{"is_sufficient": true, "score": 0.9, "contradictions": [], "reasoning": "ok", "claims": {}}`}, nil
	case strings.Contains(text.String(), "Query Rewriter"):
		return &harness.ModelReply{Content: `{"queries": [{"query": "who built it"}]}`}, nil
	default:
		return &harness.ModelReply{Content: "Built in 1865, in Geneva."}, nil
	}
}

// stubExecutor is a deterministic ToolExecutor recording every call.
type stubExecutor struct {
	mu        sync.Mutex
	calls     []string
	responses map[string]harness.ToolOutcome
}

func newStubExecutor() *stubExecutor {
	return &stubExecutor{responses: map[string]harness.ToolOutcome{}}
}

func (e *stubExecutor) add(name, payload string, status string) {
	e.responses[name] = harness.ToolOutcome{
		Status:  status,
		Payload: []any{payload},
	}
}

func (e *stubExecutor) Execute(ctx context.Context, name string, args map[string]any) (harness.ToolOutcome, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = append(e.calls, name)
	if o, ok := e.responses[name]; ok {
		return o, nil
	}
	return harness.ToolOutcome{Status: harness.StatusOK, Payload: []any{"{}"}}, nil
}

func newToolset(exec *stubExecutor) *harness.Toolset {
	return &harness.Toolset{
		ThinkingMode:  "high",
		HasWebSearch:  true,
		DisabledTools: map[string]bool{},
		Exec:          exec,
	}
}

// countingRetriever is a no-op retriever recording the requested top-N. It
// satisfies harness.Retriever (= tools.Retriever).
type countingRetriever struct {
	mu   sync.Mutex
	topN int
}

func (r *countingRetriever) Retrieve(_ context.Context, req harness.RetrieveRequest) ([]map[string]any, error) {
	r.mu.Lock()
	r.topN = req.TopN
	r.mu.Unlock()
	return []map[string]any{{"doc_id": "d1", "docnm_kwd": "doc1", "content": "Saint Lawrence River; 14 April 1865; 9 December 2019."}}, nil
}

// ---------------------------------------------------------------------------
// Unit tests for the routing / helpers (no init() required).
// ---------------------------------------------------------------------------

func TestRouteSCAAlwaysCloseoutOnNoProgress(t *testing.T) {
	// NoProgress is the hard stop: regardless of SCA enablement or verdict, the
	// loop must close out rather than spin another round.
	st := &AgenticState{Verdict: VerdictInsufficient, MaxLoops: 3, NoProgress: true}
	if n := routeSCA(st, true, 3); n != nodeFormalizeAnswer {
		t.Fatalf("SCA on: expected formalize on no-progress, got %v", n)
	}
	if n := routeSCA(st, false, 3); n != nodeFormalizeAnswer {
		t.Fatalf("SCA off: expected formalize on no-progress, got %v", n)
	}
}

func TestRouteSCADisabledSCAClosesOut(t *testing.T) {
	// With SCA disabled the single research pass' verdict is informational; we
	// never start a rewrite round.
	st := &AgenticState{Verdict: VerdictInsufficient, MaxLoops: 3}
	if n := routeSCA(st, false, 3); n != nodeFormalizeAnswer {
		t.Fatalf("expected formalize when SCA disabled, got %v", n)
	}
}

func TestRouteSCAInsufficientStartsRewrite(t *testing.T) {
	// Sufficient verdict closes out.
	st := &AgenticState{Verdict: VerdictSufficient, MaxLoops: 3}
	if n := routeSCA(st, true, 3); n != nodeFormalizeAnswer {
		t.Fatalf("expected formalize on sufficient, got %v", n)
	}
	// Insufficient + rounds under cap + headroom remaining => rewrite.
	st = &AgenticState{Verdict: VerdictInsufficient, MaxLoops: 3, SearchRounds: 0}
	st.Deadline = time.Now().Add(120 * time.Second) // 120s remaining >> MinRoundHeadroomS
	if n := routeSCA(st, true, 3); n != nodeQueryRewrite {
		t.Fatalf("expected rewrite on insufficient+headroom, got %v", n)
	}
	// At max rounds => formalize.
	st = &AgenticState{Verdict: VerdictInsufficient, MaxLoops: 3, SearchRounds: 3}
	st.Deadline = time.Now().Add(120 * time.Second)
	if n := routeSCA(st, true, 3); n != nodeFormalizeAnswer {
		t.Fatalf("expected formalize at max rounds, got %v", n)
	}
}

func TestSelectSCAViewIdentity(t *testing.T) {
	chunks := make([]map[string]any, 0, 80)
	for i := 0; i < 80; i++ {
		chunks = append(chunks, map[string]any{
			"chunk_id":   fmt.Sprintf("c%d", i),
			"content":    fmt.Sprintf("the quick brown fox %d", i),
			"similarity": 1.0 - float64(i)/100.0,
		})
	}
	v1, id1 := SelectSCAView(chunks, []string{"quick", "brown"})
	if len(v1) != SCAViewCap {
		t.Fatalf("expected view cap %d, got %d", SCAViewCap, len(v1))
	}
	// Same inputs => same identity (no Process-randomised hash).
	_, id2 := SelectSCAView(chunks, []string{"quick", "brown"})
	if id1 != id2 {
		t.Fatalf("view identity should be deterministic, got %q vs %q", id1, id2)
	}
	// An enlarged pool with the SAME selected ids keeps the same identity — this
	// is exactly the "review view unchanged" early-exit signal.
	more := append(chunks, map[string]any{"chunk_id": "extra", "content": "unrelated tail", "similarity": 0.0})
	_, id3 := SelectSCAView(more, []string{"quick", "brown"})
	if id1 != id3 {
		t.Fatalf("adding non-selected chunks should NOT change the view identity, got %q vs %q", id1, id3)
	}
}

func TestMergeSlotPatchAdoptsStronger(t *testing.T) {
	strong := 0.95
	base := harness.NewState([]harness.Variable{
		{ID: 0, Type: "aspect", Candidate: strPtr("base"), CandidateStrength: &strong},
	}, 0, nil)
	// A weak branch (0.30) must NOT downgrade the strong base.
	weak := 0.30
	branch := harness.NewState([]harness.Variable{
		{ID: 0, Type: "aspect", Candidate: strPtr("weak-candidate"), CandidateStrength: &weak},
	}, 1, nil)
	merged := MergeSlotPatch(base, branch)
	// A weak branch does not downgrade the strong base, AND produces no change
	// at all — MergeSlotPatch returns nil to signal "no new patch".
	if merged != nil {
		t.Fatalf("weak branch over strong base should yield NO change (nil patch), got %+v", merged)
	}

	// A stronger branch wins.
	stronger := 0.99
	branch2 := harness.NewState([]harness.Variable{
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
	base := harness.NewState([]harness.Variable{
		{ID: 0, Type: "aspect", Candidate: strPtr("same")},
	}, 0, nil)
	branch := harness.NewState([]harness.Variable{
		{ID: 0, Type: "aspect", Candidate: strPtr("same")},
	}, 1, nil)
	if m := MergeSlotPatch(base, branch); m != nil {
		t.Fatalf("identical patch should return nil, got %+v", m)
	}
}

func TestExpandFanoutsParsesJSON(t *testing.T) {
	mdl := &scriptedModel{}
	mdl.push(`{"fanouts": ["who discovered it", "when was it discovered"]}`)
	got := ExpandFanouts(context.Background(), RAGTools{Model: mdl}, "What was discovered and by whom?")
	if len(got) != 2 {
		t.Fatalf("expected 2 fan-outs, got %v", got)
	}
	if got[0] != "who discovered it" || got[1] != "when was it discovered" {
		t.Fatalf("unexpected fan-outs: %v", got)
	}
}

func TestExpandFanoutsFallbackOnBadJSON(t *testing.T) {
	mdl := &scriptedModel{}
	mdl.push("I cannot break this down.")
	got := ExpandFanouts(context.Background(), RAGTools{Model: mdl}, "single question")
	// Bad JSON: the loop falls back to line-splitting the model's reply, which
	// yields the reply itself (single line), not the raw question.
	if len(got) != 1 || got[0] != "I cannot break this down." {
		t.Fatalf("expected line-split fallback, got %v", got)
	}
}

func TestExpandFanoutsNilModelReturnsQuestion(t *testing.T) {
	got := ExpandFanouts(context.Background(), RAGTools{}, "single question")
	if len(got) != 1 || got[0] != "single question" {
		t.Fatalf("expected raw question when no model, got %v", got)
	}
}

// TestExpandFanoutsRetriesWithStrictJSON covers Python's retry (:478-488): a
// prose answer must not be line-split into fan-outs (it poisons the slot table),
// so the prompt is re-sent with the strict JSON instruction instead.
func TestExpandFanoutsRetriesWithStrictJSON(t *testing.T) {
	mdl := &scriptedModel{}
	mdl.push("The woman was **Rocio Restrepo**.\nSupporting sources: https://example.com")
	mdl.push(`{"fanouts": ["who was she", "when did it happen"]}`)
	got := ExpandFanouts(context.Background(), RAGTools{Model: mdl}, "Who was she?")
	if len(got) != 2 || got[0] != "who was she" || got[1] != "when did it happen" {
		t.Fatalf("expected the strict-retry fan-outs, got %v", got)
	}
	if len(mdl.seen) != 2 {
		t.Fatalf("model calls = %d, want 2 (original + one strict retry)", len(mdl.seen))
	}
}

// TestExpandFanoutsDropsAnswerLikeEntries covers _fanout_looks_like_query: a
// fan-out is used verbatim as a retrieval query and as the slot hint, so an
// answered fact must never survive into either.
func TestExpandFanoutsDropsAnswerLikeEntries(t *testing.T) {
	mdl := &scriptedModel{}
	mdl.push(`{"fanouts": ["who won the medal", "The winner was **Ann** according to the citation", "when was the race"]}`)
	got := ExpandFanouts(context.Background(), RAGTools{Model: mdl}, "Who won the medal and when?")
	if len(got) != 2 || got[0] != "who won the medal" || got[1] != "when was the race" {
		t.Fatalf("expected the answer-like entry to be dropped, got %v", got)
	}
}

func TestFanoutLooksLikeQueryBounds(t *testing.T) {
	// The loose (non-JSON) path holds a tighter bound than the JSON path.
	long := "who opened the new library downtown last spring and who paid for the building"
	if !fanoutLooksLikeQuery(long, false) {
		t.Errorf("JSON-path fan-out %q should pass the %d-word bound", long, fanoutMaxWords)
	}
	if fanoutLooksLikeQuery(long, true) {
		t.Errorf("loose fan-out %q should exceed the %d-word bound", long, fanoutLooseMaxWords)
	}
	// A question keeps its full length even on the loose path.
	if !fanoutLooksLikeQuery(long+"?", true) {
		t.Error("a question-shaped line should survive the loose bound")
	}
	if fanoutLooksLikeQuery(strings.Repeat("word ", fanoutMaxWords+2), false) {
		t.Errorf("fan-outs over %d words must be rejected", fanoutMaxWords)
	}
	if fanoutLooksLikeQuery(strings.Repeat("a", fanoutMaxChars+1), false) {
		t.Errorf("fan-outs over %d chars must be rejected", fanoutMaxChars)
	}
}

// TestRenderSlotDraftMatchesPythonFormat pins Python _render_slot_draft
// (agentic_rag_graph.py:1278-1326) exactly: the collected answer leads with its
// evidence metadata, a resolved slot carries strength + terminal + evidence ids +
// its discovered-clue tail, and an unresolved slot is rendered as
// "NOT RESOLVED (<question clues>)". This text is the SCA's claim context, so a
// different shape changes what the reviewer sees.
func TestRenderSlotDraftMatchesPythonFormat(t *testing.T) {
	strong := 0.9
	st := harness.NewState([]harness.Variable{
		{
			ID:                0,
			Type:              "aspect",
			Candidate:         strPtr("answer A"),
			CandidateStrength: &strong,
			DiscoveredClues:   []string{"clue one", "clue two"},
		},
		{ID: 1, Type: "aspect", QuestionClues: []string{"who opened it?"}},
	}, 0, nil)
	evidence := map[string]SlotEvidence{
		"0": {EvidenceIDs: []string{"c1", "c2"}, TerminalType: "state"},
	}

	draft := RenderSlotDraft(st, "the collected answer", evidence)
	want := "Candidate answer: the collected answer\n" +
		"\n" +
		"- slot 0 [aspect]: answer A (strength=0.90) [terminal=state, evidence_ids=['c1', 'c2']] — clue one; clue two\n" +
		"- slot 1 [aspect]: NOT RESOLVED (who opened it?)"
	if draft != want {
		t.Fatalf("draft = %q\nwant   %q", draft, want)
	}
}

// TestSelectSCAViewPrefersScoreWhenSimilarityIsZero pins Python
// _select_sca_view:140 (`float(c.get("similarity") or c.get("score") or 0.0)`):
// a similarity of 0.0 is FALSY, so the score must be used instead. The
// presence-based read ranked such a chunk last instead of first.
func TestSelectSCAViewPrefersScoreWhenSimilarityIsZero(t *testing.T) {
	chunks := []map[string]any{
		{"chunk_id": "zero-sim", "content": "irrelevant", "similarity": 0.0, "score": 0.9},
		{"chunk_id": "half", "content": "irrelevant", "similarity": 0.5},
	}
	view, _ := SelectSCAView(chunks, nil)
	if len(view) != 2 {
		t.Fatalf("view size = %d, want 2", len(view))
	}
	if got := harness.ChunkIDOf(view[0]); got != "zero-sim" {
		t.Errorf("first chunk = %q, want zero-sim (score 0.9 must beat similarity 0.5)", got)
	}
}

// TestComposeFallbackDraftMatchesPythonPrompt pins the draft prompt of Python
// _compose_fallback_draft (:1585-1598) byte for byte: the FOUND/MISSING
// instructions, the "Retrieved evidence:" label, the "\n"-joined numbered
// snippets, and the language clause appended without a separator.
func TestComposeFallbackDraftMatchesPythonPrompt(t *testing.T) {
	mdl := &fakeModel{replies: []*harness.ModelReply{{Content: "FOUND: x\nMISSING: none"}}}
	st := &AgenticState{
		Question: "谁开的？",
		KB:       &harness.Kbinfos{Chunks: []map[string]any{{"content_with_weight": "证据一"}}},
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

// TestBuildSlotTableKeepsAllFanoutsOnFailure pins Python _build_slot_table's
// exception branch (:1347-1350): when initialize_state fails, first_queries is
// the FULL fan-out list — the [:3] cap only applies to the empty-root path
// (:1360). Losing the list made the first prefetch narrower than Python's.
func TestBuildSlotTableKeepsAllFanoutsOnFailure(t *testing.T) {
	fanouts := []string{"q1", "q2", "q3", "q4", "q5"}
	// No model → initialize_state cannot run, so the failure path is taken.
	root, first := BuildSlotTable(context.Background(), harness.SessionDeps{}, "raw question", fanouts, 60)
	if len(root.State) != 4 {
		t.Fatalf("fallback slots = %d, want 4 (Python queries[:4])", len(root.State))
	}
	if len(first) != len(fanouts) {
		t.Errorf("firstQueries = %v, want the full fan-out list on the failure path", first)
	}
}

// TestBuildSlotTableFallsBackOnSpentBudget pins the planner's deadline
// (Python agentic_rag_graph.py:917, `lambda: _remaining_s(state) - 15.0`): the
// value is NOT floored, so once the round budget is spent the decomposition times
// out at once and the table falls back to the planner fan-outs.
func TestBuildSlotTableFallsBackOnSpentBudget(t *testing.T) {
	mdl := &fakeModel{replies: []*harness.ModelReply{
		{Content: `{"slots":[{"id":0,"type":"aspect","clues":["a"]}]}`},
	}}
	fanouts := []string{"f1", "f2"}
	root, first := BuildSlotTable(context.Background(), harness.SessionDeps{Model: mdl}, "q", fanouts, -15)
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

// TestBuildSlotTableFallsBackWhenIDIsNotAnInteger covers the end-to-end effect of
// the strict parser: Python's int(s["id"]) raises, _build_slot_table catches it
// and rebuilds the table from the planner fan-outs.
func TestBuildSlotTableFallsBackWhenIDIsNotAnInteger(t *testing.T) {
	fanouts := []string{"q1", "q2"}
	deps := harness.SessionDeps{Model: &fakeModel{replies: []*harness.ModelReply{
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

// TestPythonMessageListPrefixMatchesLangchain pins the ledger's q fallback
// (Python _run_slot_research_pass:1446 `(r.found_answer or str(r.messages))[:80]`).
// The expectation is the byte-exact output of the installed langchain_core:
//
//	>>> str([SystemMessage(content='You are a slot-filling research agent.\n'
//	...                      'Direction: who opened it?')])[:80]
//	"[SystemMessage(content='You are a slot-filling research agent.\\nDirection: who o"
func TestPythonMessageListPrefixMatchesLangchain(t *testing.T) {
	msgs := []schema.Message{
		*schema.SystemMessage("You are a slot-filling research agent.\nDirection: who opened it?"),
		*schema.UserMessage("Direction: who opened it?\n\nState:\n- slot 0 [aspect]: NOT RESOLVED"),
	}
	want := "[SystemMessage(content='You are a slot-filling research agent.\\nDirection: who o"
	if got := pythonMessageListPrefix(msgs, 80); got != want {
		t.Errorf("prefix = %q\nwant    %q", got, want)
	}

	// The full (untruncated) repr must match langchain's field list, including the
	// normalized tool_call dict and the ToolMessage shape.
	full := []schema.Message{
		*schema.SystemMessage("sys"),
		*schema.UserMessage("usr"),
		*schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call_0",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "retrieve",
				Arguments: `{"query":"who opened it?"}`,
			},
		}}),
		*schema.ToolMessage(`{"passages": []}`, "call_0"),
	}
	wantFull := "[SystemMessage(content='sys', additional_kwargs={}, response_metadata={}), " +
		"HumanMessage(content='usr', additional_kwargs={}, response_metadata={}), " +
		"AIMessage(content='', additional_kwargs={}, response_metadata={}, " +
		"tool_calls=[{'name': 'retrieve', 'args': {'query': 'who opened it?'}, 'id': 'call_0', 'type': 'tool_call'}], " +
		"invalid_tool_calls=[]), " +
		`ToolMessage(content='{"passages": []}', tool_call_id='call_0')]`
	if got := pythonMessageListPrefix(full, 4096); got != wantFull {
		t.Errorf("full repr = %q\nwant       %q", got, wantFull)
	}
	if got := pythonMessageListPrefix(nil, 80); got != "[]" {
		t.Errorf("nil messages = %q, want []", got)
	}
}

// sessionStubExec answers every tool call with a hit, so a session runs its turns
// without a real retriever.
type sessionStubExec struct{}

func (sessionStubExec) Execute(_ context.Context, name string, _ map[string]any) (harness.ToolOutcome, error) {
	return harness.ToolOutcome{
		Status:      harness.StatusOK,
		Payload:     []any{map[string]any{"kind": name, "content": "hit"}},
		EvidenceIDs: []string{"c-" + name},
	}, nil
}

// TestRunSlotResearchPassLedgerRecordsSessionHint drives a real round whose
// session produces no terminal answer, and asserts the ledger row still carries a
// query hint (the transcript digest) rather than an empty string.
func TestRunSlotResearchPassLedgerRecordsSessionHint(t *testing.T) {
	st := &AgenticState{
		Question: "who opened it?",
		SlotTable: harness.NewState([]harness.Variable{
			{ID: 0, Type: "aspect", QuestionClues: []string{"who opened it?"}},
		}, 0, nil),
	}
	deps := harness.SessionDeps{
		Model: &fakeModel{replies: []*harness.ModelReply{
			{Content: "I could not find any evidence about the opening date in the retrieved passages."},
		}},
		Tools: &harness.Toolset{Exec: sessionStubExec{}},
		KB:    &harness.Kbinfos{},
	}
	res := RunSlotResearchPass(context.Background(), deps, "who opened it?", st, 60)
	if res == nil {
		t.Fatal("RunSlotResearchPass returned nil")
	}
	if len(res.Attempted) == 0 {
		t.Fatal("no ledger entry recorded")
	}
	q, _ := res.Attempted[len(res.Attempted)-1]["q"].(string)
	if !strings.HasPrefix(q, "[SystemMessage(content='") {
		t.Errorf("ledger q = %q, want the langchain repr prefix (Python :1446)", q)
	}
}

// TestRunSlotResearchPassLogsSlotEvidenceBound pins Python :1459-1463: the round
// reports how many passages each slot's session bound, so a slot whose session
// retrieved nothing is visible in the run log.
func TestRunSlotResearchPassLogsSlotEvidenceBound(t *testing.T) {
	var buf bytes.Buffer
	orig := _LOG
	_LOG = log.New(&buf, "", 0)
	defer func() { _LOG = orig }()

	st := &AgenticState{
		Question: "who opened it?",
		SlotTable: harness.NewState([]harness.Variable{
			{ID: 0, Type: "aspect", QuestionClues: []string{"who opened it?"}},
		}, 0, nil),
	}
	deps := harness.SessionDeps{
		Model: &fakeModel{replies: []*harness.ModelReply{{
			Content: "Let me look that up.",
			ToolCalls: []harness.ToolCall{
				{ID: "call_0", Name: "retrieve", Args: map[string]any{"query": "who opened it?"}},
			},
		}}},
		Tools: &harness.Toolset{Exec: sessionStubExec{}},
		KB:    &harness.Kbinfos{},
	}
	if res := RunSlotResearchPass(context.Background(), deps, "who opened it?", st, 60); res == nil {
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
	weights []float64
	topN    []int
	queries []string
}

func (r *channelRetriever) Retrieve(_ context.Context, req harness.RetrieveRequest) ([]map[string]any, error) {
	r.weights = append(r.weights, req.KeywordsSimilarityWeight)
	r.topN = append(r.topN, req.TopN)
	r.queries = append(r.queries, req.Query)
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
	deps := RAGTools{Search: harness.SearchDeps{Backend: r, KbIDs: []string{"kb1"}, HasEmbedder: true}}
	st := &AgenticState{KB: &harness.Kbinfos{}}
	FanoutSearch(ctx, deps, st, []string{"when was it built", "where located"}, 8, 60)

	// Two fan-outs x two channels.
	if len(r.weights) != 4 {
		t.Fatalf("expected 4 retrieve calls (2 fan-outs x 2 channels), got %d", len(r.weights))
	}
	for i := 0; i < 4; i += 2 {
		// Channel A: keyword-only weight, wide pool, query terms folded in.
		if r.weights[i] != 0 {
			t.Errorf("fan-out %d channel A: weight = %v, want 0 (BM25 keyword leg)", i/2, r.weights[i])
		}
		if r.topN[i] != fanoutBM25TopN {
			t.Errorf("fan-out %d channel A: TopN = %d, want %d", i/2, r.topN[i], fanoutBM25TopN)
		}
		if !strings.Contains(r.queries[i], "built") && !strings.Contains(r.queries[i], "located") {
			t.Errorf("fan-out %d channel A: query = %q, want the fan-out query (terms folded in)", i/2, r.queries[i])
		}
		// Channel B: vector weight on, top_n=30 (narrowing bypassed).
		if r.weights[i+1] <= 0 {
			t.Errorf("fan-out %d channel B: weight = %v, want > 0 (semantic bypass leg)", i/2, r.weights[i+1])
		}
		if r.topN[i+1] != fanoutHybridTopN {
			t.Errorf("fan-out %d channel B: TopN = %d, want %d", i/2, r.topN[i+1], fanoutHybridTopN)
		}
	}
}

// TestFanoutSearchPrefersExactOverSemantic pins the admission order across
// FAN-OUTS: every fan-out's exact channel is admitted before any fan-out's
// semantic channel. With room for only two snippets and two fan-outs each
// offering one exact and one semantic hit, the two exact hits must win — a
// single merged channel could not express this preference at all.
func TestFanoutSearchPrefersExactOverSemantic(t *testing.T) {
	ctx := context.Background()
	r := &fanoutHitRetriever{}
	deps := RAGTools{Search: harness.SearchDeps{Backend: r, KbIDs: []string{"kb1"}, HasEmbedder: true}}
	st := &AgenticState{KB: &harness.Kbinfos{}}
	added := FanoutSearch(ctx, deps, st, []string{"alpha", "beta"}, 8, 2)

	if added != 2 {
		t.Fatalf("added = %d, want 2 (the pool's remaining room)", added)
	}
	ids := make([]string, 0, len(st.KB.Chunks))
	for _, c := range st.KB.Chunks {
		ids = append(ids, harness.ChunkIDOf(c))
	}
	if len(ids) != 2 || ids[0] != "ex-alpha" || ids[1] != "ex-beta" {
		t.Errorf("admitted %v, want the two exact-leg hits [ex-alpha ex-beta]", ids)
	}
}

// fanoutHitRetriever returns one exact hit and one semantic hit per fan-out,
// distinguished by the leg's top_n (30 = semantic bypass, 60 = exact BM25).
type fanoutHitRetriever struct{}

func (r *fanoutHitRetriever) Retrieve(_ context.Context, req harness.RetrieveRequest) ([]map[string]any, error) {
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

// draftModel returns a canned draft and records the prompt it was given.
type draftModel struct {
	reply    string
	err      error
	lastUser string
	lastSys  string
	calls    int
}

func (m *draftModel) Complete(_ context.Context, msgs []schema.Message, _ []harness.ToolSpec) (*harness.ModelReply, error) {
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
	return &harness.ModelReply{Content: m.reply}, nil
}

// TestComposeFallbackDraftUsesLLM pins that the fallback draft is SYNTHESISED,
// not concatenated. The draft feeds PreSummary as "Research findings
// (authoritative)" and is what the SCA reviews, so its shape matters: it must
// ask for a MISSING: line, and a plain list of truncated snippets cannot express
// one.
func TestComposeFallbackDraftUsesLLM(t *testing.T) {
	st := &AgenticState{
		Question: "When was it built?",
		KB:       &harness.Kbinfos{Chunks: []map[string]any{{"content": "It was built in 1874."}}},
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
	// Python :1479-1481: numbered "[i]" evidence, 1-indexed.
	if !strings.Contains(mdl.lastUser, "[1] It was built in 1874.") {
		t.Errorf("user = %q, want '[1] <content>' evidence", mdl.lastUser)
	}
}

// TestComposeFallbackDraftFallsBackToEvidence pins Python :1485-1486 and
// :1512-1514: with no model, or when the call fails, the draft degrades to the
// raw evidence capped at 4000 chars.
func TestComposeFallbackDraftFallsBackToEvidence(t *testing.T) {
	st := &AgenticState{
		Question: "When?",
		KB:       &harness.Kbinfos{Chunks: []map[string]any{{"content": "Evidence body."}}},
	}
	if got := ComposeFallbackDraft(context.Background(), RAGTools{}, st); !strings.Contains(got, "Evidence body.") {
		t.Errorf("no-model draft = %q, want the raw evidence", got)
	}
	failing := &draftModel{err: errors.New("boom")}
	if got := ComposeFallbackDraft(context.Background(), RAGTools{Model: failing}, st); !strings.Contains(got, "Evidence body.") {
		t.Errorf("failed-call draft = %q, want the raw evidence", got)
	}
}

// TestComposeFallbackDraftOrdersByRelevance pins Python :1478: the fixed 16-slot
// budget is spent on the STRONGEST evidence, not on insertion order.
func TestComposeFallbackDraftOrdersByRelevance(t *testing.T) {
	st := &AgenticState{
		Question: "When?",
		KB: &harness.Kbinfos{Chunks: []map[string]any{
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
// measurable: Python builds LLMUsageStats and wraps the chat model at
// agentic_rag.py:266-267, then logs at :930. Without the Go equivalent the
// counting machinery stays inert — CurrentStats(ctx) is nil, so the phase
// markers already in the graph record nothing and even the explicit
// deps.Stats.RecordCall sites are skipped by their nil guard.
func TestRagCollectsPerPhaseUsage(t *testing.T) {
	mdl := &countingModel{}
	Rag(context.Background(), RAGTools{
		Retriever: &corpusRetriever{},
		Model:     mdl,
	}, harness.RunRequest{
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

func (m *countingModel) Complete(ctx context.Context, msgs []schema.Message, _ []harness.ToolSpec) (*harness.ModelReply, error) {
	m.calls++
	if harness.CurrentStats(ctx) == nil {
		m.nilStats++
	}
	if p := harness.CurrentPhase(ctx); p == "unknown" {
		m.unknownPhase++
	} else {
		if m.phases == nil {
			m.phases = map[string]bool{}
		}
		m.phases[p] = true
	}
	return &harness.ModelReply{Content: "ok"}, nil
}

// TestSCAFeedbackIsStatusOnly pins Python agentic_rag.py:902-929: the
// "[Research status]" note is the status hint ALONE. Python's verdict dict
// carries only a "status" key, so its missing_claims / hard_violations /
// agent_confidence / feedback segments are always empty — a Go port that
// rendered them (from the separate SCA payload) diverged from upstream and
// invented a "0.00" confidence fallback Python never emits.
func TestSCAFeedbackIsStatusOnly(t *testing.T) {
	rich := map[string]any{
		"contradictions": []any{"A says 1874, B says 1881"},
		"confidence":     0.42,
		"claims": map[string]any{
			"c1": map[string]any{"grounded": false, "missing_information": []any{
				map[string]any{"what": "the architect"},
			}},
		},
	}
	got := scaFeedback(rich, VerdictInsufficient)
	if got != "evidence is not yet sufficient" {
		t.Errorf("scaFeedback = %q, want the status hint alone", got)
	}
	// Empty (and thus un-appended) for a SUFFICIENT verdict, per :910.
	if got := scaFeedback(rich, VerdictSufficient); got != "" {
		t.Errorf("scaFeedback(SUFFICIENT) = %q, want empty", got)
	}
	// The unused-status fallback mirrors :915.
	if got := scaFeedback(rich, "WEIRD"); got != "sufficiency status: WEIRD" {
		t.Errorf("scaFeedback(WEIRD) = %q, want the :915 fallback", got)
	}
}

// TestComposeFallbackDraftMirrorsLanguage pins Python :1505-1506: a non-English
// question gets an explicit same-language instruction.
func TestComposeFallbackDraftMirrorsLanguage(t *testing.T) {
	st := &AgenticState{
		Question: "它是什么时候建造的？",
		KB:       &harness.Kbinfos{Chunks: []map[string]any{{"content": "1874年建成。"}}},
	}
	mdl := &draftModel{reply: "1874年建成。\nMISSING: 无"}
	ComposeFallbackDraft(context.Background(), RAGTools{Model: mdl}, st)

	if !strings.Contains(mdl.lastSys, "same language") {
		t.Errorf("system = %q, want the same-language instruction for a CJK question", mdl.lastSys)
	}
}

func TestSCAGapsToRewriteFromSubQueries(t *testing.T) {
	sca := map[string]any{
		"sub_queries": []any{
			map[string]any{"sub_query": "q1", "satisfied": true, "missing_fact": "", "search_hint": ""},
			map[string]any{"sub_query": "q2", "satisfied": false, "missing_fact": "the year", "search_hint": "search for dates"},
		},
	}
	gaps := SCAGapsToRewrite(sca)
	if len(gaps) != 1 {
		t.Fatalf("expected 1 gap, got %d: %+v", len(gaps), gaps)
	}
	if gaps[0].What != "the year" || gaps[0].SearchHint != "search for dates" {
		t.Fatalf("unexpected gap: %+v", gaps[0])
	}
}

func TestSCAGapsToRewriteFallbackToClaims(t *testing.T) {
	sca := map[string]any{
		"claims": map[string]any{
			"c1": map[string]any{
				"verdict": "unverified",
				"missing_information": []any{
					map[string]any{"what": "population", "search_hint": "demographics"},
				},
			},
		},
	}
	gaps := SCAGapsToRewrite(sca)
	if len(gaps) != 1 {
		t.Fatalf("expected 1 gap from claims fallback, got %d: %+v", len(gaps), gaps)
	}
	if gaps[0].What != "population" {
		t.Fatalf("unexpected gap: %+v", gaps[0])
	}
}

func TestRunReturnsNonNilState(t *testing.T) {
	ctx := context.Background()
	mdl := &scriptedModel{}
	mdl.push(`{"fanouts": ["when built", "where located"]}`)
	mdl.push(`{"slots":[{"id":0,"type":"aspect","question":"when built","clues":["1865"]},{"id":1,"type":"aspect","question":"where located","clues":["geneva"]}], "first_queries":["when built","where located"]}`)
	mdl.push("Built in 1865.")
	mdl.push("In Geneva.")
	mdl.push(`{"is_sufficient": true, "score": 0.9, "contradictions": [], "reasoning": "ok", "claims": {}}`)

	exec := newStubExecutor()
	exec.add("search_chunks", `{"hit":[{"doc_id":"d1","docnm_kwd":"doc1","content":"Built 1865, Geneva."}],"doc_aggs":[]}`, harness.StatusOK)

	st, err := BuildAgenticGraph(ctx, RAGTools{
		Model: mdl,
		Tools: newToolset(exec),
		// The dual-channel fan-out retrieves directly, so it needs the search
		// config; without it the prefetch contributes nothing.
		Search: harness.SearchDeps{Backend: &corpusRetriever{}, KbIDs: []string{"kb1"}, HasEmbedder: true},
		Logger: log.Default(),
	}, "When and where was it built?", "", 3, nil)
	if err != nil {
		t.Fatalf("BuildAgenticGraph: %v", err)
	}

	if st == nil {
		t.Fatal("expected a non-nil AgenticState")
	}
	if strings.TrimSpace(st.Draft) == "" {
		t.Fatal("expected a non-empty research draft")
	}
	if st.NoProgress {
		t.Fatal("a completed run should not report NoProgress")
	}
}

// TestAgenticGraphPushesPhaseProgress asserts that the agentic loop forwards
// tagged engine-stage lines to the caller's Progress sink (Python think_log
// counterpart) as it runs — planner, research round, SCA — so a streaming chat
// can show live research progress in the reasoning block.
func TestAgenticGraphPushesPhaseProgress(t *testing.T) {
	ctx := context.Background()
	mdl := &scriptedModel{}
	mdl.push(`{"fanouts": ["when built", "where located"]}`)
	mdl.push(`{"slots":[{"id":0,"type":"aspect","question":"when built","clues":["1865"]},{"id":1,"type":"aspect","question":"where located","clues":["geneva"]}], "first_queries":["when built","where located"]}`)
	mdl.push("Built in 1865.")
	mdl.push("In Geneva.")
	mdl.push(`{"is_sufficient": true, "score": 0.9, "contradictions": [], "reasoning": "ok", "claims": {}}`)

	exec := newStubExecutor()
	exec.add("search_chunks", `{"hit":[{"doc_id":"d1","docnm_kwd":"doc1","content":"Built 1865, Geneva."}],"doc_aggs":[]}`, harness.StatusOK)

	var lines []string
	// Mirror production: Rag wraps the run logger with thinkLogger using the
	// same Progress sink. Python has no explicit "push" API at all — the think
	// block is fed purely by intercepting tagged logger lines — so everything
	// asserted here must come from a logger.Printf.
	sink := func(line string) { lines = append(lines, line) }
	st, err := BuildAgenticGraph(ctx, RAGTools{
		Model:    mdl,
		Tools:    newToolset(exec),
		Search:   harness.SearchDeps{Backend: &corpusRetriever{}, KbIDs: []string{"kb1"}, HasEmbedder: true},
		Logger:   thinkLogger(log.Default(), sink),
		Progress: sink,
	}, "When and where was it built?", "", 3, nil)
	if err != nil {
		t.Fatalf("BuildAgenticGraph: %v", err)
	}

	if st == nil {
		t.Fatal("expected a non-nil AgenticState")
	}
	joined := strings.Join(lines, "\n")
	// Each stage is announced by its node's own tagged log line (Python parity:
	// no separate push), so assert on what the nodes actually log.
	for _, want := range []string{
		"[Agentic RAG] Starting research",
		"[Planner] Decomposing",
		"[RAGAgent] ROUND 1 start",
		"[Draft] intermediate draft",
		"[SCA] verdict=",
		"[Finalize] partial=",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("think-log lines missing %q; got:\n%s", want, joined)
		}
	}
}

// TestAgenticGraphCyclesBackThroughQueryRewrite drives the research loop: the
// SCA returns insufficient once, so the graph must run query_rewrite and re-enter
// rag_agent. The loop is the only cycle in the graph — it is why the graph is
// compiled in Pregel mode (DAG mode rejects cycles).
func TestAgenticGraphCyclesBackThroughQueryRewrite(t *testing.T) {
	ctx := context.Background()
	mdl := &promptRoutedModel{}

	exec := newStubExecutor()
	exec.add("search_chunks", `{"hit":[{"doc_id":"d1","docnm_kwd":"doc1","content":"Built 1865, Geneva."}],"doc_aggs":[]}`, harness.StatusOK)

	var buf bytes.Buffer
	var lines []string
	sink := func(line string) { lines = append(lines, line) }
	st, err := BuildAgenticGraph(ctx, RAGTools{
		Model:    mdl,
		Tools:    newToolset(exec),
		Search:   harness.SearchDeps{Backend: &corpusRetriever{}, KbIDs: []string{"kb1"}, HasEmbedder: true},
		Logger:   thinkLogger(log.New(&buf, "", 0), sink),
		Progress: sink,
	}, "When and where was it built?", "", 3, nil)
	if err != nil {
		t.Fatalf("BuildAgenticGraph: %v", err)
	}

	if st == nil {
		t.Fatal("expected a non-nil AgenticState")
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		"[QueryRewriter]",
		"[RAGAgent] ROUND 2 start",
		"[Finalize] partial=",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("cycle not driven: missing %q; got:\n%s", want, joined)
		}
	}
	// The second research round must actually run: the SCA is only reached again
	// through query_rewrite → rag_agent, i.e. the graph's only cycle.
	if !strings.Contains(joined, "[SCA] verdict=INSUFFICIENT") {
		t.Errorf("expected the first SCA to be insufficient:\n%s", joined)
	}
}

// TestBuildLowGraphRunsFormalizeThenDirectSearch covers the low-mode graph
// (Python build_low_graph:733). It has no planner and no SCA loop, so the only
// observable contract is: formalize rewrites the question, then direct_search
// merges retrieved evidence into the kbinfos.
func TestBuildLowGraphRunsFormalizeThenDirectSearch(t *testing.T) {
	ctx := context.Background()
	mdl := &scriptedModel{}
	mdl.push("When was Culdcept released?")        // Formalize
	mdl.push(`{"keywords": "Culdcept, released"}`) // weighted keyword extraction

	kb := &harness.Kbinfos{}
	sd := harness.SearchDeps{Backend: &corpusRetriever{}, KbIDs: []string{"kb1"}, HasEmbedder: true}
	resp := &RunResponse{Mode: harness.ResolveMode(&harness.Toolset{ThinkingMode: "low"})}
	req := harness.RunRequest{Question: "when was it made?", TopN: 8}

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
	// formalize ran and rewrote the question (the graph's first node).
	if !strings.Contains(buf.String(), "[Agentic RAG] formalized the question") {
		t.Errorf("formalize_question node did not run; log:\n%s", buf.String())
	}
}

// lowGraphCountingExpander counts compiled-expansion calls (harness.CompiledExpander).
type lowGraphCountingExpander struct{ calls int }

func (c *lowGraphCountingExpander) Expand(context.Context, *harness.Kbinfos, string, string, []string) error {
	c.calls++
	return nil
}

// TestBuildLowGraphAlwaysExpandsCompiled mirrors Python low mode: direct.py
// calls hybrid_search(..., use_compiled=True) unconditionally, so the low
// graph's direct_search must expand even when the caller's RunRequest leaves
// UseCompiled false — which production always does.
func TestBuildLowGraphAlwaysExpandsCompiled(t *testing.T) {
	ctx := context.Background()
	kb := &harness.Kbinfos{}
	exp := &lowGraphCountingExpander{}
	sd := harness.SearchDeps{Backend: &corpusRetriever{}, KbIDs: []string{"kb1"}, HasEmbedder: true, Expand: exp}
	resp := &RunResponse{Mode: harness.ResolveMode(&harness.Toolset{ThinkingMode: "low"})}
	req := harness.RunRequest{Question: "when was it made?", TopN: 8, UseCompiled: false}

	var buf bytes.Buffer
	if err := BuildLowGraph(ctx, RAGTools{Search: sd}, req, sd, kb, resp, log.New(&buf, "", 0)); err != nil {
		t.Fatalf("BuildLowGraph: %v", err)
	}
	if exp.calls == 0 {
		t.Errorf("low mode's direct_search must expand compiled structure "+
			"(Python direct.py use_compiled=True); log:\n%s", buf.String())
	}
}

// TestFinalizeRunsOnceFromInsideTheGraph locks in the Python parity of the
// terminal node: both graphs end in a formalize_answer node that composes
// (:1214 / :834), so the composition happens INSIDE the graph — and exactly
// once, even though Rag also has a post-graph call for runs that never reach
// the last node. A double composition would overwrite the answer with the
// model's next reply, which is what this test watches for.
func TestFinalizeRunsOnceFromInsideTheGraph(t *testing.T) {
	ctx := context.Background()
	finalized := 0
	resp := &RunResponse{Mode: harness.ResolveMode(&harness.Toolset{ThinkingMode: "low"})}
	kb := &harness.Kbinfos{}
	sd := harness.SearchDeps{Backend: &corpusRetriever{}, KbIDs: []string{"kb1"}, HasEmbedder: true}
	req := harness.RunRequest{Question: "when was it made?", TopN: 8}
	deps := RAGTools{
		Model:  &scriptedModel{},
		Search: sd,
	}
	// Mirror Rag: the hook is idempotent, so the graph's node and the
	// post-graph safety net cannot both compose.
	composed := false
	deps.Finalize = func(context.Context) {
		if composed {
			return
		}
		composed = true
		finalized++
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
	deps.Finalize(ctx)
	if finalized != 1 {
		t.Errorf("composition ran %d time(s) after the post-graph call, want 1", finalized)
	}
}

// TestSCAUnavailableMarksInsufficient pins Python :1096-1098: when the SCA
// produces no usable result (timeout, unparsable reply, no model) the verdict is
// INSUFFICIENT so unresolved slots can drive another research round — NOT
// SUFFICIENT, which would ship the unverified draft as if it had passed review.
func TestSCAUnavailableMarksInsufficient(t *testing.T) {
	st := NewAgenticState("When was it built?", "", 3, nil)
	st.Draft = "Built in 1865."
	st.KB = &harness.Kbinfos{Chunks: []map[string]any{
		{"chunk_id": "c1", "content": "Built 1865, Geneva."},
	}}
	// A model that never returns JSON: GenJSON fails, so the SCA has no result.
	mdl := &scriptedModel{}
	mdl.push("this is not json at all")

	scaNode(context.Background(), RAGTools{Model: mdl}, st, log.New(&bytes.Buffer{}, "", 0))

	if st.Verdict != VerdictInsufficient {
		t.Errorf("Verdict = %q, want INSUFFICIENT (Python marks an unavailable SCA insufficient)", st.Verdict)
	}
	if st.SCA == nil {
		t.Error("SCA payload must stay an (empty) map, like Python's sca={}")
	}
}

// TestQueryRewriteFoldsUnresolvedCluesWhenNoGaps pins Python :1114-1128: with
// no structured SCA gaps, the unresolved slots' question_clues become the gaps,
// so the loop keeps going instead of accepting an unresolved draft.
func TestQueryRewriteFoldsUnresolvedCluesWhenNoGaps(t *testing.T) {
	st := NewAgenticState("When and where was it built?", "", 3, nil)
	st.SCA = map[string]any{} // no sub_queries, no claims -> no gaps
	st.UnresolvedSlots = []map[string]any{
		{"question_clues": []string{"when opened", "where located", "ignored third clue"}},
	}
	st.Deadline = time.Now().Add(60 * time.Second)

	mdl := &scriptedModel{}
	mdl.push(`{"queries": [{"query": "when was it opened"}]}`)

	queryRewriteNode(context.Background(), RAGTools{
		Model:  mdl,
		Search: harness.SearchDeps{Backend: &corpusRetriever{}, KbIDs: []string{"kb1"}, HasEmbedder: true},
	}, st, log.New(&bytes.Buffer{}, "", 0))

	if st.NoProgress {
		t.Fatal("NoProgress = true; Python folds unresolved slot clues into the gaps and keeps researching")
	}
	joined := strings.Join(st.CurrentQueries, "|")
	if !strings.Contains(joined, "when opened") {
		t.Errorf("CurrentQueries = %v, want the unresolved slot clue to be pursued", st.CurrentQueries)
	}
}

// TestUnresolvedClueGapsCapsAtTwoClues mirrors Python's `[:2]` per slot.
func TestUnresolvedClueGapsCapsAtTwoClues(t *testing.T) {
	st := &AgenticState{UnresolvedSlots: []map[string]any{
		{"question_clues": []string{"a", "b", "c"}},
		{"question_clues": []string{"a", "d"}}, // duplicate "a" is dropped
	}}
	gaps := unresolvedClueGaps(st)
	if len(gaps) != 3 {
		t.Fatalf("gaps = %d, want 3 (2 from the first slot + 1 new from the second)", len(gaps))
	}
	if gaps[0].What != "a" || gaps[0].SearchHint != "a" {
		t.Errorf("gaps[0] = %+v, want what=hint=\"a\"", gaps[0])
	}
}

// TestBuildSCAClaimsFallsBackToEvidenceOnlyClaim pins Python :1072-1073: with no
// draft and no slot evidence, the SCA still gets one claim carrying EVERY chunk
// in the view, so it can judge the evidence instead of the node short-circuiting.
func TestBuildSCAClaimsFallsBackToEvidenceOnlyClaim(t *testing.T) {
	view := []map[string]any{
		{"chunk_id": "c1", "content": "Built 1865."},
		{"chunk_id": "c2", "content": "In Geneva."},
	}
	claims, _ := buildSCAClaims("", view, view, nil)
	if len(claims) != 1 {
		t.Fatalf("claims = %d, want 1", len(claims))
	}
	if claims[0].Draft != "(no draft)" {
		t.Errorf("draft = %q, want %q", claims[0].Draft, "(no draft)")
	}
	// Evidence is addressed by POSITION (renderClaimContext keys by index), so
	// the fallback claim must carry 0..len(view)-1.
	if len(claims[0].EvidenceIDs) != 2 || claims[0].EvidenceIDs[0] != "0" || claims[0].EvidenceIDs[1] != "1" {
		t.Errorf("EvidenceIDs = %v, want [0 1]", claims[0].EvidenceIDs)
	}
}

// TestBuildSCAClaimsAppendsMissingSlotEvidence pins Python :1057-1071: a slot
// whose evidence chunk is NOT in the view has that chunk appended, and the claim
// carries its new POSITION. Python appends to the same list and reviews against
// it, so buildSCAClaims returns the grown view.
func TestBuildSCAClaimsAppendsMissingSlotEvidence(t *testing.T) {
	chunks := []map[string]any{
		{"chunk_id": "c1", "content": "Built 1865."},
		{"chunk_id": "c2", "content": "In Geneva."},
		{"chunk_id": "c3", "content": "Designed by OmiyaSoft."},
	}
	view := []map[string]any{chunks[0]}
	slotEvidence := map[string]SlotEvidence{
		"0": {EvidenceIDs: []string{"2"}, Candidate: "designer"}, // pool position 2
	}

	claims, grown := buildSCAClaims("", view, chunks, slotEvidence)
	if len(claims) != 1 {
		t.Fatalf("claims = %d, want 1", len(claims))
	}
	if len(grown) != 2 {
		t.Fatalf("grown view = %d, want 2 (the missing chunk must be appended)", len(grown))
	}
	if got := claims[0].EvidenceIDs; len(got) != 1 || got[0] != "1" {
		t.Errorf("EvidenceIDs = %v, want [1] (its position in the grown view)", got)
	}
}

// TestResolveEvidenceChunkAcceptsBothIdSpaces covers the heterogeneous ids:
// retrieval tools emit pool positions, navigation tools emit doc ids.
func TestResolveEvidenceChunkAcceptsBothIdSpaces(t *testing.T) {
	chunks := []map[string]any{
		{"chunk_id": "c1", "doc_id": "d1", "content": "a"},
		{"chunk_id": "c2", "doc_id": "d2", "content": "b"},
	}
	if got := resolveEvidenceChunk("1", chunks); got["chunk_id"] != "c2" {
		t.Errorf("position id \"1\" resolved to %v, want chunk c2", got)
	}
	if got := resolveEvidenceChunk("c1", chunks); got["chunk_id"] != "c1" {
		t.Errorf("chunk id \"c1\" resolved to %v, want chunk c1", got)
	}
	if got := resolveEvidenceChunk("d2", chunks); got["chunk_id"] != "c2" {
		t.Errorf("doc id \"d2\" resolved to %v, want chunk c2", got)
	}
	if got := resolveEvidenceChunk("nope", chunks); got != nil {
		t.Errorf("unknown id resolved to %v, want nil", got)
	}
}

func TestAgenticNodeNameMapsRoutingToGraphKeys(t *testing.T) {
	cases := map[agenticNode]string{
		nodeQueryRewrite:      "query_rewrite",
		nodeRagAgentFirst:     "rag_agent",
		nodeRagAgentLoop:      "rag_agent",
		nodeFormalizeAnswer:   "formalize_answer",
		nodeFormalizeQuestion: "formalize_answer",
	}
	for n, want := range cases {
		if got := agenticNodeName(n); got != want {
			t.Errorf("agenticNodeName(%d) = %q, want %q", n, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Explicit-wiring integration tests (replaces the old init()-based registration
// checks). Activation is explicit, mirroring Python's dialog_service.py
// instantiating RAGTools: the test registers the loop before calling Run.
// ---------------------------------------------------------------------------

// TestMain keeps the package's tests on in-memory doubles: graph exploration
// otherwise seeds its dense search from the tenant embedding model, which
// reaches a database these tests do not have. Returning nil keeps the keyword
// fallback, which is the path under test.
func TestMain(m *testing.M) {
	harness.SetSeedEncoder(func(context.Context, string, string) []float64 { return nil })
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
	mdl.push(`{"fanouts": ["when was it opened", "what river"]}`)
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
	features  []map[string]float64
	queries   []string
	docScopes [][]string
}

func (r *rfRetriever) Retrieve(_ context.Context, req harness.RetrieveRequest) ([]map[string]any, error) {
	r.features = append(r.features, req.RankFeature)
	r.queries = append(r.queries, req.Query)
	r.docScopes = append(r.docScopes, req.DocScope)
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

// TestAgenticLoopProjectsRankFeature guards the projection that Python gets for
// free: tools.retrieve reads every setting (incl.
// rank_feature=label_question(question, self.kbs), agentic_rag.py:668) off the
// RAGTools instance, so the agentic loop's fan-out and action-session retrieval
// must see the same Tagger/KBs. A partial SearchDeps projection silently drops
// the tag boost and degrades ranking with no error.
func TestAgenticLoopProjectsRankFeature(t *testing.T) {
	SetAgenticLoop(NewAgenticLoop())
	defer SetAgenticLoop(nil)

	mdl := &scriptedModel{}
	// Call order: formalize (single-turn keyword extraction, Python
	// agentic_rag.py:446) → planner fan-out → slot table → draft → SCA.
	mdl.push(`{"entity": ["it"], "aliases": [], "fact_type": [], "qualifiers": []}`)
	mdl.push(`{"fanouts": ["when was it opened"]}`)
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
	// Scope to the fan-out / slot research retrievals — the ones Python issues
	// through tools.retrieve / search_chunks and therefore the ones that must
	// carry rank_feature.
	//
	// Deliberately exempt: the navigation tools' own recalls (chunk-agg routing
	// via chunkAggRetrieveFrom, and _recall_chunk_ids_in_doc) which mirror
	// Python's _search_layers_nav_chunk_agg / _recall_chunk_ids_in_doc
	// (navigation.py:1399) — neither passes rank_feature there.
	research := map[string]bool{"when was it opened": true, "when opened": true}
	checked := 0
	for i, q := range r.queries {
		if !research[q] {
			continue
		}
		checked++
		if r.features[i]["location"] != 1.0 {
			t.Errorf("retrieve %q RankFeature = %v, want the label_question tag boost", q, r.features[i])
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
	mdl.push(`{"fanouts": ["x", "y"]}`) // would only be produced by the loop's planner
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
	req := harness.RunRequest{
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

// Test doubles for the RAGTools method tests. Kept local to the advanced_rag package so
// the tests can call Run/ComposeAnswer (which live here) without importing agent
// from the harness test package (that would be an import cycle).

// fakeModel replays a scripted sequence of replies, so the session's control
// flow can be driven without a provider.
type fakeModel struct {
	replies  []*harness.ModelReply
	calls    int
	messages []schema.Message
}

func (f *fakeModel) Complete(_ context.Context, msgs []schema.Message, _ []harness.ToolSpec) (*harness.ModelReply, error) {
	f.messages = msgs
	if f.calls >= len(f.replies) {
		return &harness.ModelReply{Content: `<state>{"new_states": []}</state>`}, nil
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
type corpusRetriever struct{ calls []string }

func (c *corpusRetriever) Retrieve(_ context.Context, req harness.RetrieveRequest) ([]map[string]any, error) {
	c.calls = append(c.calls, req.Query)
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

func (emptyRetriever) Retrieve(_ context.Context, _ harness.RetrieveRequest) ([]map[string]any, error) {
	return nil, nil
}

func TestRunLowModeIsSinglePass(t *testing.T) {
	r := &corpusRetriever{}
	resp := Rag(context.Background(), RAGTools{Retriever: r}, harness.RunRequest{
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
	if harness.MemorySize(resp.Kbinfos) != 1 {
		t.Errorf("memory size = %d, want 1", harness.MemorySize(resp.Kbinfos))
	}
}

func TestRunUnknownModeFallsBackToNaive(t *testing.T) {
	r := &corpusRetriever{}
	resp := Rag(context.Background(), RAGTools{Retriever: r}, harness.RunRequest{
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

// TestNaiveUsesFlatEvidenceAndOwnComposition pins the semantics that separate
// the naive path from low/direct. Python's _naive_rag (:1517) is NOT
// "direct_search minus formalize": it does a plain retrieve with no weighted
// keyword extraction (:1533) and composes under a short fixed system prompt
// from flat "[i] content" evidence (:1555-1568), never FinalAnswerSystem /
// kb_prompt. Routing naive through runDirect + composeFinalAnswer silently
// substituted the agentic composition, which is what this test guards.
func TestNaiveUsesFlatEvidenceAndOwnComposition(t *testing.T) {
	mdl := &fakeModel{replies: []*harness.ModelReply{{Content: "OmiyaSoft [1]."}}}
	r := &corpusRetriever{}
	resp := Rag(context.Background(), RAGTools{Retriever: r, Model: mdl}, harness.RunRequest{
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
	// Python 1533: one PLAIN retrieve — no ExtractWeightedKeywords, so the model
	// is called exactly once. The old runDirect path spent a second call on
	// keyword extraction before composing.
	if mdl.calls != 1 {
		t.Fatalf("model calls = %d, want 1 (naive composes without extracting keywords)", mdl.calls)
	}

	// Python 1555: flat "[i] content" evidence, not kb_prompt's "ID: n" blocks.
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
	resp := Rag(context.Background(), RAGTools{Retriever: r}, harness.RunRequest{
		Question:     "Who created Culdcept?",
		ThinkingMode: "high",
		DatasetIDs:   []string{"kb1"},
	})
	if resp.EmptyResult {
		t.Error("agentic mode without a model must degrade to direct search")
	}
}

func TestRunAgenticSeedsSlotsAndRunsSession(t *testing.T) {
	r := &corpusRetriever{}
	mdl := &fakeModel{replies: []*harness.ModelReply{
		{Content: `{"slots": [{"id": 0, "type": "entity", "clues": ["creator"]}, {"id": 1, "type": "date", "clues": ["released"]}], "first_queries": ["creator"]}`},
		{Content: "", ToolCalls: []harness.ToolCall{{ID: "c0", Name: "retrieve", Args: map[string]any{"query": []any{"Culdcept creator"}}}}},
		{Content: `<state>{"new_states":[{"state":[{"id":0,"candidate":"OmiyaSoft","candidate_strength":0.95}]}]}</state>`},
	}}
	resp := Rag(context.Background(), RAGTools{Retriever: r, Model: mdl}, harness.RunRequest{
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
	resp := Rag(context.Background(), RAGTools{}, harness.RunRequest{
		Question:     "anything",
		ThinkingMode: "low",
		DatasetIDs:   []string{"kb1"},
	})
	if !resp.EmptyResult {
		t.Error("an unavailable backend must yield an empty result, not an error")
	}
}

func TestRunLowReturnsComposedAnswer(t *testing.T) {
	mdl := &fakeModel{replies: []*harness.ModelReply{
		// formalize (single-turn keyword extraction, Python
		// agentic_rag.py:446) and the low graph's direct search each extract
		// keywords in their own call (Python direct.py:22 does the same), so
		// the composed answer is the third reply.
		{Content: `{"entity": ["Culdcept"], "aliases": [], "fact_type": [], "qualifiers": []}`},
		{Content: `{"entity": ["Culdcept"], "aliases": [], "fact_type": [], "qualifiers": []}`},
		{Content: "Culdcept was created by OmiyaSoft and released in 1999 [1]."},
	}}
	resp := Rag(context.Background(), RAGTools{
		Retriever: &corpusRetriever{},
		Model:     mdl,
	}, harness.RunRequest{
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
	mdl := &fakeModel{replies: []*harness.ModelReply{{Content: "OmiyaSoft [1]"}}}
	resp := Rag(context.Background(), RAGTools{
		Retriever: &corpusRetriever{},
		Model:     mdl,
	}, harness.RunRequest{
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
	resp := Rag(context.Background(), RAGTools{
		Retriever:     &emptyRetriever{},
		Model:         mdl,
		EmptyResponse: "I don't have enough information.",
	}, harness.RunRequest{
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
}

func TestRunCanDisableComposition(t *testing.T) {
	no := false
	resp := Rag(context.Background(), RAGTools{
		Retriever:     &corpusRetriever{},
		Model:         &fakeModel{},
		ComposeAnswer: &no,
	}, harness.RunRequest{
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
	mdl := &fakeModel{replies: []*harness.ModelReply{
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
	}, harness.RunRequest{
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
	mdl := &fakeModel{replies: []*harness.ModelReply{{Content: "answer [1]"}}}
	Rag(context.Background(), RAGTools{
		Retriever: &corpusRetriever{},
		Model:     mdl,
		Messages: []schema.Message{
			*schema.UserMessage("Who created Culdcept?"),
			*schema.AssistantMessage("OmiyaSoft.", nil),
			*schema.UserMessage("When was it released?"),
		},
	}, harness.RunRequest{
		Question:     "When was it released?",
		ThinkingMode: "low",
		DatasetIDs:   []string{"kb1"},
	})
	if mdl.calls != 1 {
		t.Errorf("model calls = %d, want 1 (formalize is opt-in)", mdl.calls)
	}
}

func TestRunAgenticComposesFromResearchFindings(t *testing.T) {
	kb := &harness.Kbinfos{
		Chunks:     []map[string]any{{"chunk_id": "c1", "content": "Culdcept: OmiyaSoft, 1999.", "similarity": 0.9}},
		PreSummary: "Culdcept was created by OmiyaSoft and released in 1999.",
	}
	mdl := &fakeModel{replies: []*harness.ModelReply{{Content: "Culdcept was created by OmiyaSoft and released in 1999 [1]."}}}
	res := ComposeAnswer(context.Background(), AnswerDeps{Model: mdl}, kb, "Who created Culdcept?", false, false)
	if res.Failed {
		t.Fatal("composition must succeed")
	}
	if !strings.Contains(mdl.lastUserPrompt(), "Culdcept was created by OmiyaSoft and released in 1999.") {
		t.Errorf("prompt missing the research findings:\n%s", mdl.lastUserPrompt())
	}
}

// TestAnswerPromptCarriesTargetContract pins the EXTREME-SELECTION guardrail
// (Python :643-652). Without it a "longest/shortest/most…" question tends to be
// answered with the most common or first-listed candidate rather than the
// extreme one, because nothing asks the model to compare.
func TestAnswerPromptCarriesTargetContract(t *testing.T) {
	kb := &harness.Kbinfos{
		Chunks: []map[string]any{{"chunk_id": "c1", "content": "river A is 300km; river B is 900km"}},
	}
	mdl := &fakeModel{replies: []*harness.ModelReply{{Content: "river B [1]."}}}
	if res := ComposeAnswer(context.Background(), AnswerDeps{Model: mdl}, kb,
		"Which river is the longest?", false, false); res.Failed {
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

// TestAnswerPromptNoEvidenceInstructions pins Python :654-663: when the call is
// not short-circuited by empty_response, the prompt must still tell the model how
// to degrade — from the research summary if there is one, otherwise with an
// explicit insufficiency statement.
func TestAnswerPromptNoEvidenceInstructions(t *testing.T) {
	// No chunks, no summary -> insufficiency statement.
	mdl := &fakeModel{replies: []*harness.ModelReply{{Content: "unknown [1]."}}}
	ComposeAnswer(context.Background(), AnswerDeps{Model: mdl}, &harness.Kbinfos{}, "q?", false, false)
	if got := mdl.lastUserPrompt(); !strings.Contains(got, "No supporting evidence was retrieved") {
		t.Errorf("prompt missing the no-evidence instruction:\n%s", got)
	}

	// No chunks but a draft exists -> answer from the summary, do not refuse.
	kb := &harness.Kbinfos{PreSummary: "partial findings"}
	mdl2 := &fakeModel{replies: []*harness.ModelReply{{Content: "x [1]."}}}
	ComposeAnswer(context.Background(), AnswerDeps{Model: mdl2}, kb, "q?", false, false)
	if got := mdl2.lastUserPrompt(); !strings.Contains(got, "The retrieved passages are limited") {
		t.Errorf("prompt missing the limited-evidence instruction:\n%s", got)
	}
}

// TestAnswerPromptOrdersPartialPreamble pins Python :668-671: the partial
// preamble sits AFTER the research summary and BEFORE the evidence — not at the
// front of the prompt, where it would precede the question.
func TestAnswerPromptOrdersPartialPreamble(t *testing.T) {
	kb := &harness.Kbinfos{
		Chunks:     []map[string]any{{"chunk_id": "c1", "content": "body"}},
		PreSummary: "the draft",
	}
	mdl := &fakeModel{replies: []*harness.ModelReply{{Content: "x [1]."}}}
	ComposeAnswer(context.Background(), AnswerDeps{Model: mdl}, kb, "q?", true, false)

	got := mdl.lastUserPrompt()
	iQ := strings.Index(got, "Question:")
	iSummary := strings.Index(got, "Research Summary")
	iPartial := strings.Index(got, strings.TrimSpace(harness.PartialAnswerPreamble))
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
// AnswerDeps.UserImages carries vision-gated data URIs (Python image_attachments
// surviving gateImageAttachments), the final-answer user message must be
// multimodal — an image_url content block beside the text question — so the
// compose model sees the images exactly like Python's direct async_chat
// fallback (called with the original multimodal messages). Without this the
// direct (no-outer) path composed from a text-only prompt and silently dropped
// the picture.
func TestComposeAnswerWithAttachesUserImages(t *testing.T) {
	mdl := &fakeModel{replies: []*harness.ModelReply{{Content: "answer [1]."}}}
	kb := &harness.Kbinfos{Chunks: []map[string]any{{"chunk_id": "c1", "content": "evidence"}}}
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
	mdl := &fakeModel{replies: []*harness.ModelReply{{Content: "answer [1]."}}}
	kb := &harness.Kbinfos{Chunks: []map[string]any{{"chunk_id": "c1", "content": "evidence"}}}
	if res := ComposeAnswerWith(context.Background(), AnswerDeps{Model: mdl}, kb, "plain?", false, false, false); res.Failed {
		t.Fatal("composition must succeed")
	}
	user := mdl.messages[len(mdl.messages)-1]
	if len(user.UserInputMultiContent) != 0 {
		t.Errorf("expected no multimodal blocks when there are no images; got %+v", user.UserInputMultiContent)
	}
}

// userTurnAt returns the Content of the last message of the idx-th Complete
// call (0-based), i.e. the user turn of that attempt.
func userTurnAt(m *scriptedModel, idx int) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if idx >= len(m.seen) || len(m.seen[idx]) == 0 {
		return ""
	}
	return m.seen[idx][len(m.seen[idx])-1].Content
}

// TestGenJSONUsesOutputNewlineUserTurn locks the contract that every JSONModel
// round mirrors Python gen_json's "Output:\n" user turn. It was previously an
// invisible convention buried inside jsonModelAdapter.GenJSON — a second JSONModel
// implementation could silently drop it and diverge from Python without any test
// catching it. See query_rewriter.go parity note (diff ②).
func TestGenJSONUsesOutputNewlineUserTurn(t *testing.T) {
	mdl := &scriptedModel{}
	mdl.push("{\"ok\": true}")

	if _, err := (&jsonModelAdapter{inner: mdl}).GenJSON(context.Background(), "Render JSON."); err != nil {
		t.Fatalf("GenJSON failed: %v", err)
	}
	if got := userTurnAt(mdl, 0); got != "Output:\n" {
		t.Errorf("first-round user turn = %q, want exactly %q (Python gen_json separator)", got, "Output:\n")
	}
}

func TestGenJSONRetriesMalformedReplyWithCorrection(t *testing.T) {
	mdl := &scriptedModel{}
	// First round: fenced but unparseable JSON. Second round: well-formed.
	mdl.push("```json\n{\"queries\": [broken")
	mdl.push("{\"queries\": [{\"query\": \"q1\"}]}")

	got, err := (&jsonModelAdapter{inner: mdl}).GenJSON(context.Background(), "Rewrite the query.")
	if err != nil {
		t.Fatalf("GenJSON failed after corrective retry: %v", err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("GenJSON returned %T, want map[string]any", got)
	}
	queries, ok := m["queries"].([]any)
	if !ok || len(queries) != 1 {
		t.Fatalf("queries = %#v, want a 1-element list", m["queries"])
	}

	// The retry must feed the previous answer and the parse error back to the
	// model, mirroring gen_json's corrective prompt.
	turn := userTurnAt(mdl, 1)
	for _, want := range []string{"Generated JSON is as following:", "broken", "But exception while loading:", "Please reconsider and correct it."} {
		if !strings.Contains(turn, want) {
			t.Errorf("retry user turn missing %q; got: %s", want, turn)
		}
	}
}

func TestGenJSONStripsThinkAndFenceOnFirstTry(t *testing.T) {
	mdl := &scriptedModel{}
	mdl.push("Sure.\n<thinking>drafting...</thinking>\n```json\n{\"ok\": true}\n```\n")

	got, err := (&jsonModelAdapter{inner: mdl}).GenJSON(context.Background(), "Render JSON.")
	if err != nil {
		t.Fatalf("GenJSON failed on a wrapper-padded reply: %v", err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("GenJSON returned %T, want map[string]any", got)
	}
	if m["ok"] != true {
		t.Errorf(`m["ok"] = %#v, want true`, m["ok"])
	}
	if len(mdl.seen) != 1 {
		t.Errorf("expected a single attempt, got %d", len(mdl.seen))
	}
}

// fakeLLMCache is an in-memory LLMCache double.
type fakeLLMCache struct {
	entries map[string]string
	gets    int
	sets    int
}

func newFakeLLMCache() *fakeLLMCache { return &fakeLLMCache{entries: map[string]string{}} }

func (c *fakeLLMCache) Get(_ context.Context, key string) (string, bool) {
	c.gets++
	v, ok := c.entries[key]
	return v, ok
}

func (c *fakeLLMCache) Set(_ context.Context, key, value string) {
	c.sets++
	c.entries[key] = value
}

// TestGenJSONCachesFirstSuccessfulReply pins gen_json's 24h reply cache
// (Python get_llm_cache / set_llm_cache): the CLEANED answer of the first
// successful parse is stored, and an identical (model, system, user) triple is
// served from the cache WITHOUT a second model call.
func TestGenJSONCachesFirstSuccessfulReply(t *testing.T) {
	cache := newFakeLLMCache()
	mdl := &scriptedModel{}
	mdl.push("Sure.\n```json\n{\"ok\": true}\n```\n")

	adapter := &jsonModelAdapter{inner: mdl, modelName: "gpt-4o", cache: cache}
	first, err := adapter.GenJSON(context.Background(), "Render JSON.")
	if err != nil {
		t.Fatalf("first GenJSON: %v", err)
	}
	if m, ok := first.(map[string]any); !ok || m["ok"] != true {
		t.Fatalf("first GenJSON = %#v", first)
	}
	if len(mdl.seen) != 1 || cache.sets != 1 {
		t.Fatalf("first call: model calls = %d, cache sets = %d, want 1/1", len(mdl.seen), cache.sets)
	}
	wantKey := genJSONCacheKey("gpt-4o", "Render JSON.", "Output:\n")
	// The cleanup regex removes only the ```json fence and the trailing fence:
	// the "Sure.\n" prose prefix has no </think> terminator, so it survives —
	// exactly as it does in Python.
	if got := cache.entries[wantKey]; got != "Sure.\n{\"ok\": true}\n" {
		t.Errorf("cached value = %q, want the cleaned answer", got)
	}

	second, err := adapter.GenJSON(context.Background(), "Render JSON.")
	if err != nil {
		t.Fatalf("second GenJSON: %v", err)
	}
	if m, ok := second.(map[string]any); !ok || m["ok"] != true {
		t.Fatalf("second GenJSON = %#v", second)
	}
	if len(mdl.seen) != 1 {
		t.Errorf("cache hit still called the model: %d calls", len(mdl.seen))
	}
}

// TestGenJSONCacheKeyMatchesPython locks the key SHAPE: %x over the xxh64 of
// model+system+user+"{}" (str(gen_conf) of the default {} is "{}"), i.e. the 16
// lowercase hex digits Python's xxhash.xxh64(...).hexdigest() produces. An empty
// model name yields "" so the cache is skipped rather than collapsing models.
func TestGenJSONCacheKeyMatchesPython(t *testing.T) {
	got := genJSONCacheKey("gpt-4o", "SYS", "Output:\n")
	// Reference value produced by Python itself:
	//   xxhash.xxh64(('gpt-4o' + 'SYS' + 'Output:\n' + '{}').encode('utf-8')).hexdigest()
	//   -> '02dffaad8465c0c9'
	// Note the LEADING ZERO: a plain %x prints 15 digits ('2dffaad8465c0c9'),
	// which would silently stop sharing the cache with Python.
	if got != "02dffaad8465c0c9" {
		t.Fatalf("key = %q, want 02dffaad8465c0c9 (Python xxh64 hexdigest)", got)
	}
	// Empty model name yields "" so callers SKIP the cache rather than collapse
	// every model onto one bucket.
	if genJSONCacheKey("", "SYS", "Output:\n") != "" {
		t.Error("empty model name must disable the cache key")
	}
	if genJSONCacheKey("a", "SYS", "u") == genJSONCacheKey("b", "SYS", "u") {
		t.Error("distinct model names produced the same key")
	}
}

// TestGenJSONFitsPromptToContextWindow pins Python gen_json's
// message_fit_in(form_message(system_prompt, user_prompt), chat_mdl.max_length):
// an oversized system prompt must be trimmed to the model window BEFORE the
// first call instead of being sent verbatim (the provider would reject it).
func TestGenJSONFitsPromptToContextWindow(t *testing.T) {
	huge := strings.Repeat("token ", 40000) // far past any window
	mdl := &scriptedModel{}
	mdl.push(`{"ok": true}`)

	if _, err := (&jsonModelAdapter{inner: mdl, maxLength: 4096}).GenJSON(context.Background(), huge); err != nil {
		t.Fatalf("GenJSON failed: %v", err)
	}
	if len(mdl.seen) != 1 || len(mdl.seen[0]) == 0 {
		t.Fatalf("expected one call with messages, got %#v", mdl.seen)
	}
	sent := mdl.seen[0][0]
	if sent.Role != schema.System {
		t.Fatalf("first message role = %v, want system", sent.Role)
	}
	if len(sent.Content) >= len(huge) {
		t.Errorf("system prompt not fitted: %d bytes sent of %d", len(sent.Content), len(huge))
	}
}

// TestGenJSONAcceptsTopLevelArrayAndScalar pins json_repair parity: gen_json
// parses ANY top-level JSON value, so an array or scalar reply must come back
// as-is instead of being treated as a parse failure (which a map-only unmarshal
// did before).
func TestGenJSONAcceptsTopLevelArrayAndScalar(t *testing.T) {
	mdl := &scriptedModel{}
	mdl.push(`[{"query": "q1"}]`)
	got, err := (&jsonModelAdapter{inner: mdl}).GenJSON(context.Background(), "Render JSON.")
	if err != nil {
		t.Fatalf("array reply: %v", err)
	}
	arr, ok := got.([]any)
	if !ok || len(arr) != 1 {
		t.Fatalf("array reply = %#v, want a 1-element list", got)
	}
	if len(mdl.seen) != 1 {
		t.Errorf("array reply made %d attempt(s), want 1", len(mdl.seen))
	}

	mdl = &scriptedModel{}
	mdl.push(`42`)
	got, err = (&jsonModelAdapter{inner: mdl}).GenJSON(context.Background(), "Render JSON.")
	if err != nil {
		t.Fatalf("scalar reply: %v", err)
	}
	if n, ok := got.(float64); !ok || n != 42 {
		t.Fatalf("scalar reply = %#v, want 42", got)
	}
}

func TestGenJSONGivesUpAfterMaxRetries(t *testing.T) {
	mdl := &scriptedModel{}
	mdl.push("I cannot produce JSON today.")
	mdl.push("Still not JSON.")

	_, err := (&jsonModelAdapter{inner: mdl}).GenJSON(context.Background(), "Rewrite the query.")
	if err == nil {
		t.Fatal("GenJSON succeeded, want error after exhausting retries")
	}
	if len(mdl.seen) != 2 {
		t.Errorf("expected 2 attempts (genJSONMaxRetry), got %d", len(mdl.seen))
	}
}

// errOnceModel fails the underlying chat call and counts invocations, proving
// chat/transport errors are NOT retried (gen_json propagates them immediately).
type errOnceModel struct {
	n int
}

func (m *errOnceModel) Complete(_ context.Context, _ []schema.Message, _ []harness.ToolSpec) (*harness.ModelReply, error) {
	m.n++
	return nil, errors.New("provider down")
}

func TestGenJSONPropagatesChatErrorWithoutRetry(t *testing.T) {
	mdl := &errOnceModel{}
	_, err := (&jsonModelAdapter{inner: mdl}).GenJSON(context.Background(), "Rewrite the query.")
	if err == nil {
		t.Fatal("GenJSON succeeded, want the chat error")
	}
	if mdl.n != 1 {
		t.Errorf("chat invoked %d times, want exactly 1 (no retry on transport error)", mdl.n)
	}
}

func TestGenJSONNilModel(t *testing.T) {
	if _, err := (&jsonModelAdapter{inner: nil}).GenJSON(context.Background(), "x"); err == nil {
		t.Fatal("GenJSON with a nil inner model succeeded, want error")
	}
}

// stubDocChunks implements harness.DocChunkLister for injection tests.
type stubDocChunks struct{}

func (stubDocChunks) DocChunks(context.Context, harness.DocChunksRequest) ([]map[string]any, error) {
	return nil, nil
}

// TestSearchDepsForProjectsDocChunks locks the injection seam: RAGTools.DocChunks
// must be projected onto SearchDeps so the document-level tools (summarize_document
// / fetch_full_document) can reach a real chunk reader once the caller wires one.
func TestSearchDepsForProjectsDocChunks(t *testing.T) {
	got := searchDepsFor(context.Background(), RAGTools{
		DocChunks: stubDocChunks{},
	}, harness.RunRequest{}, nil, "", &harness.Kbinfos{}, log.New(os.Stderr, "", 0))

	if got.DocChunks == nil {
		t.Fatal("RAGTools.DocChunks was not projected onto SearchDeps")
	}
	if _, ok := got.DocChunks.(stubDocChunks); !ok {
		t.Fatalf("SearchDeps.DocChunks = %T, want the injected stubDocChunks", got.DocChunks)
	}

	// Nil RAGTools.DocChunks must stay nil (disables whole-document reading).
	gotNil := searchDepsFor(context.Background(), RAGTools{}, harness.RunRequest{}, nil, "", &harness.Kbinfos{}, log.New(os.Stderr, "", 0))
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
		g := compose.NewGraph[*harness.RunRequest, *harness.RunRequest]()
		node := func(c context.Context, r *harness.RunRequest) (*harness.RunRequest, error) { return r, nil }
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

// agentic graph shape: 7 nodes, 4 edges, 5 branches.
func BenchmarkCompileAgenticShape(b *testing.B) {
	ctx := context.Background()
	benchCompile(b, func() error {
		g := compose.NewGraph[*AgenticState, *AgenticState]()
		node := func(c context.Context, s *AgenticState) (*AgenticState, error) { return s, nil }
		for _, n := range []string{
			"formalize_question", "planner", "prefetch", "rag_agent",
			"query_rewrite", "formalize_answer", "stop",
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
			"rag_agent":          {"stop": true, "query_rewrite": true, "formalize_answer": true},
			"query_rewrite":      {"stop": true, "rag_agent": true, "formalize_answer": true},
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

func TestGraphRecursionLimitMatchesPython(t *testing.T) {
	// Python :1619 — 60 for the agentic graph, else max(25, max_loops*8).
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

func TestAgenticResearchRoundCostsThreeVisits(t *testing.T) {
	// One research round is rag_agent → draft → sca. If the loop counted
	// iterations instead of node visits, 20 rounds would cost 20 instead of 60
	// and the guard would trip ~3x later than Python's.
	if got, want := agenticRoundVisits, 3; got != want {
		t.Fatalf("round cost = %d, want %d", got, want)
	}
}

func TestNewAgenticStateDoesNotArmBudget(t *testing.T) {
	// Python :823 — the budget is armed in the formalize_question node's
	// return, not at state creation, so formalization is not charged to it.
	st := NewAgenticState("q", "", 3, nil)

	if !st.Deadline.IsZero() {
		t.Fatalf("Deadline = %v, want zero until formalize_question runs", st.Deadline)
	}
}

func TestFormalizeQuestionNodeArmsBudget(t *testing.T) {
	st := NewAgenticState("when did it open?", "", 3, nil)
	// No model: the node still arms the budget, mirroring Python :823.
	formalizeQuestionNode(context.Background(), RAGTools{}, st, nil)

	if st.Deadline.IsZero() {
		t.Fatal("formalize_question must arm the budget (Python :823)")
	}
	if st.Question != "when did it open?" {
		t.Fatalf("Question = %q, want it unchanged without a model", st.Question)
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
	recordConsecutiveUnanswerable(cache, VerdictInsufficient)
	if cache.ConsecutiveUnanswerable != 1 {
		t.Fatalf("after 1st outer rag() call: ConsecutiveUnanswerable = %d, want 1",
			cache.ConsecutiveUnanswerable)
	}

	// Second outer rag() call — still unsatisfying: counter must reach 2 so the
	// STOP guard can fire (the state the pre-fix code could never reach on the
	// outer path, because deps.Cache was nil and the increment was skipped).
	recordConsecutiveUnanswerable(cache, VerdictInsufficient)
	if cache.ConsecutiveUnanswerable != 2 {
		t.Fatalf("after 2nd outer rag() call: ConsecutiveUnanswerable = %d, want 2",
			cache.ConsecutiveUnanswerable)
	}

	// A satisfying verdict resets the streak, matching Python rag (:921-924).
	recordConsecutiveUnanswerable(cache, VerdictSufficient)
	if cache.ConsecutiveUnanswerable != 0 {
		t.Fatalf("after a SUFFICIENT verdict: ConsecutiveUnanswerable = %d, want 0",
			cache.ConsecutiveUnanswerable)
	}
}

// TestRecordConsecutiveUnanswerableNoCacheIsNoOp makes sure the guard is safe
// when no cache is wired (nil deps.Cache): the helper must not panic and the
// outer loop simply loses the cross-call STOP protection — which is exactly
// why Rag() now auto-builds a cache before branching into the outer react loop.
func TestRecordConsecutiveUnanswerableNoCacheIsNoOp(t *testing.T) {
	// Must not panic with a nil cache.
	recordConsecutiveUnanswerable(nil, VerdictInsufficient)
	recordConsecutiveUnanswerable(nil, VerdictSufficient)
}
