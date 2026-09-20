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
	"sort"
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

// Unit tests for the routing / helpers (no init() required).

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

// TestRouteSCAPoolSizeIsNotTheStop pins the DECOUPLED stop: the pool's size is
// not what ends the research loop — whether the SCA can READ anything new is, and
// that fact is the view identity (scaNode sets NoProgress when the view matches
// the previous review's, and routeSCA honours it on its first check).
//
// The old guard finalized as soon as the pool reached SCAViewCap on a
// second-or-later review, which is a different claim: a pool past the view cap
// whose view CHANGED has new evidence the reviewer can read, and it was being
// thrown away (measured: a name-probe round ended at 117 chunks with members
// still arriving).
func TestRouteSCAPoolSizeIsNotTheStop(t *testing.T) {
	full := func() *AgenticState {
		st := &AgenticState{
			Verdict:      VerdictInsufficient,
			MaxLoops:     3,
			SearchRounds: 1,
			// Work left in the table AND evidence still arriving last round —
			// the two facts the loop is driven by (see routeSCA).
			LastRoundNew:    12,
			UnresolvedSlots: []map[string]any{{"question_clues": []string{"who else"}}},
			KB:              &runtime.Kbinfos{Chunks: make([]map[string]any, SCAViewCap)},
		}
		st.Deadline = time.Now().Add(120 * time.Second)
		return st
	}

	// Full pool, view changed (scaNode did NOT set NoProgress) => research on.
	if n := routeSCA(full(), true, 3); n != nodeQueryRewrite {
		t.Fatalf("full pool with a changed view: expected rewrite, got %v", n)
	}

	// Full pool, view UNCHANGED: scaNode's detector owns this case, and routeSCA
	// honours it before anything else.
	stale := full()
	stale.NoProgress = true
	if n := routeSCA(stale, true, 3); n != nodeFormalizeAnswer {
		t.Fatalf("full pool with an unchanged view: expected formalize, got %v", n)
	}

	// The loop is still bounded by the round count...
	atMax := full()
	atMax.SearchRounds = 3
	if n := routeSCA(atMax, true, 3); n != nodeFormalizeAnswer {
		t.Fatalf("at max rounds with a full pool: expected formalize, got %v", n)
	}
	// ...and by the round-headroom guard.
	tight := full()
	tight.Deadline = time.Now().Add(MinRoundHeadroomS / 2 * time.Second)
	if n := routeSCA(tight, true, 3); n != nodeFormalizeAnswer {
		t.Fatalf("no headroom with a full pool: expected formalize, got %v", n)
	}
}

// TestRouteSCARoundRecordDrivesTheLoop pins the DECOUPLED stop/continue rule: the
// loop's own record (work left in the table + evidence still arriving) decides,
// and the reviewer's verdict is one of its inputs rather than the switch.
//
// The distinction it encodes: a SUFFICIENT verdict is a judgement about the
// PASSAGES the reviewer read. It is not a statement that the question is done —
// measured 2026-09-15, a run ended with unresolved=0 while the answer was six
// members short — so a table that still lists unresolved slots while the round is
// still adding evidence keeps its next round, and a settled table closes out
// however much evidence arrived.
func TestRouteSCARoundRecordDrivesTheLoop(t *testing.T) {
	base := func() *AgenticState {
		st := &AgenticState{MaxLoops: 3, SearchRounds: 1}
		st.Deadline = time.Now().Add(120 * time.Second)
		return st
	}
	work := []map[string]any{{"question_clues": []string{"who else"}}}

	// Work left + still learning + a SATISFIED reviewer: another round. The
	// reviewer's view of the passages does not settle the table's own gaps.
	st := base()
	st.Verdict = VerdictSufficient
	st.LastRoundNew = 9
	st.UnresolvedSlots = work
	if n := routeSCA(st, true, 3); n != nodeQueryRewrite {
		t.Errorf("SUFFICIENT with work left and evidence still arriving: expected rewrite, got %v", n)
	}

	// Nothing left in the table: growth alone is not a reason to keep going.
	st = base()
	st.Verdict = VerdictSufficient
	st.LastRoundNew = 9
	if n := routeSCA(st, true, 3); n != nodeFormalizeAnswer {
		t.Errorf("settled table with nothing against it: expected formalize, got %v", n)
	}

	// Work left, stalled round, reviewer names a gap: another round.
	st = base()
	st.Verdict = VerdictInsufficient
	st.UnresolvedSlots = work
	if n := routeSCA(st, true, 3); n != nodeQueryRewrite {
		t.Errorf("INSUFFICIENT with a concrete gap: expected rewrite, got %v", n)
	}

	// Work can ALSO come from the review's own gaps, with an empty unresolved
	// table — the two records are independent. Ignoring the reviewer's gaps is
	// what made a run close out with INSUFFICIENT (confidence 1.00) and four
	// derived gaps while 60s and two rounds went unspent (2026-09-15).
	st = base()
	st.Verdict = VerdictInsufficient
	st.SCA = map[string]any{"claims": map[string]any{
		"c1": map[string]any{"missing_information": []any{
			map[string]any{"what": "还有谁", "search_hint": "关羽 斩将 名单"},
		}},
	}}
	if n := routeSCA(st, true, 3); n != nodeQueryRewrite {
		t.Errorf("INSUFFICIENT with the review's own gaps and no unresolved slots: expected rewrite, got %v", n)
	}

	// Work left, stalled round, NO judgement (the review could not run): nothing
	// names a direction and the round learned nothing — close out. This is the
	// case a fallback INSUFFICIENT used to promise a round to.
	st = base()
	st.Verdict = VerdictUnknown
	st.UnresolvedSlots = work
	if n := routeSCA(st, true, 3); n != nodeFormalizeAnswer {
		t.Errorf("UNKNOWN with a stalled round: expected formalize, got %v", n)
	}

	// Work left, stalled round, satisfied reviewer: close out.
	st = base()
	st.Verdict = VerdictSufficient
	st.UnresolvedSlots = work
	if n := routeSCA(st, true, 3); n != nodeFormalizeAnswer {
		t.Errorf("SUFFICIENT with a stalled round: expected formalize, got %v", n)
	}

	// At max rounds => formalize, whatever the record says.
	st = base()
	st.Verdict = VerdictInsufficient
	st.LastRoundNew = 3
	st.UnresolvedSlots = work
	st.SearchRounds = 3
	if n := routeSCA(st, true, 3); n != nodeFormalizeAnswer {
		t.Errorf("at max rounds: expected formalize, got %v", n)
	}
	// No headroom => formalize.
	st = base()
	st.Verdict = VerdictInsufficient
	st.LastRoundNew = 3
	st.UnresolvedSlots = work
	st.Deadline = time.Now().Add(MinRoundHeadroomS / 2 * time.Second)
	if n := routeSCA(st, true, 3); n != nodeFormalizeAnswer {
		t.Errorf("no headroom: expected formalize, got %v", n)
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
	// Bad JSON: the model answered in prose, so the loop line-splits the reply (yielding
	// the reply itself, not the raw question). The loose path strips the
	// "-•0123456789. " cutset from both ends, so the trailing period is dropped.
	if len(got) != 1 || got[0] != "I cannot break this down" {
		t.Fatalf("expected line-split fallback without the trailing period, got %v", got)
	}
}

func TestExpandFanoutsNilModelReturnsQuestion(t *testing.T) {
	got := ExpandFanouts(context.Background(), RAGTools{}, "single question")
	if len(got) != 1 || got[0] != "single question" {
		t.Fatalf("expected raw question when no model, got %v", got)
	}
}

// TestExpandFanoutsRetriesWithStrictJSON covers the retry: a prose answer must not be
// line-split into fan-outs (it poisons the slot table), so the prompt is re-sent with the
// strict JSON instruction instead.
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

// TestFanoutLooksLikeQueryCountsRunes: the 160-char cap counts code points, so a CJK
// fan-out at the limit must survive even though it exceeds 160 BYTES, and one over the
// limit must still be rejected.
func TestFanoutLooksLikeQueryCountsRunes(t *testing.T) {
	atLimit := strings.Repeat("中", fanoutMaxChars) // 160 chars / 480 bytes
	if !fanoutLooksLikeQuery(atLimit, false) {
		t.Errorf("a %d-character fan-out must pass: the cap counts characters, not bytes", fanoutMaxChars)
	}
	if fanoutLooksLikeQuery(atLimit+"中", false) {
		t.Errorf("a %d-character fan-out must be rejected", fanoutMaxChars+1)
	}
}

// TestParseFanoutsLooseStripsBothEnds: the bullet/numbering cutset is stripped from BOTH
// ends, so a trailing period is not carried into the retrieval query (the leading "1."
// already was not).
func TestParseFanoutsLooseStripsBothEnds(t *testing.T) {
	got := parseFanouts("1. who opened the library.\n- when did it open?")
	want := []string{"who opened the library", "when did it open?"}
	if len(got) != len(want) {
		t.Fatalf("parseFanouts = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("parseFanouts[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestParseFanoutsSplitsOnAllLineBreaks: a carriage-return separated reply yields one
// fan-out per line, not a single fused blob (splitting on "\n" alone would keep the lone
// "\r" inline).
func TestParseFanoutsSplitsOnAllLineBreaks(t *testing.T) {
	got := parseFanouts("who opened it\rwhen did it open?")
	want := []string{"who opened it", "when did it open?"}
	if len(got) != len(want) {
		t.Fatalf("parseFanouts = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("parseFanouts[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestRenderSlotDraftFormat pins the draft format exactly: the collected answer leads
// with its evidence metadata, a resolved slot carries strength + terminal + evidence ids +
// its discovered-clue tail, and an unresolved slot is rendered as
// "NOT RESOLVED (<question clues>)". This text is the SCA's claim context, so a different
// shape changes what the reviewer sees.
func TestRenderSlotDraftFormat(t *testing.T) {
	strong := 0.9
	st := runtime.NewState([]runtime.Variable{
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

// TestSelectSCAViewPrefersScoreWhenSimilarityIsZero: a similarity of 0.0 is FALSY, so the
// score must be used instead. The presence-based read ranked such a chunk last instead of
// first.
func TestSelectSCAViewPrefersScoreWhenSimilarityIsZero(t *testing.T) {
	chunks := []map[string]any{
		{"chunk_id": "zero-sim", "content": "irrelevant", "similarity": 0.0, "score": 0.9},
		{"chunk_id": "half", "content": "irrelevant", "similarity": 0.5},
	}
	view, _ := SelectSCAView(chunks, nil)
	if len(view) != 2 {
		t.Fatalf("view size = %d, want 2", len(view))
	}
	if got := runtime.ChunkIDOf(view[0]); got != "zero-sim" {
		t.Errorf("first chunk = %q, want zero-sim (score 0.9 must beat similarity 0.5)", got)
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
	keywordsWeights []float64
	topN            []int
	queries         []string
}

func (r *channelRetriever) Retrieve(_ context.Context, req runtime.RetrieveRequest) ([]map[string]any, error) {
	// Each leg names its keyword weight explicitly; nil (the control was not supplied)
	// is recorded as -1 so the assertions below catch a leg that drops it.
	weight := -1.0
	if req.KeywordsSimilarityWeight != nil {
		weight = *req.KeywordsSimilarityWeight
	}
	r.keywordsWeights = append(r.keywordsWeights, weight)
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
	deps := RAGTools{Search: runtime.SearchDeps{Backend: r, KbIDs: []string{"kb1"}, HasEmbedder: true}}
	st := &AgenticState{KB: &runtime.Kbinfos{}}
	FanoutSearch(ctx, deps, st, []string{"when was it built", "where located"}, 8, 60, false)

	// Two fan-outs x two channels.
	if len(r.keywordsWeights) != 4 {
		t.Fatalf("expected 4 retrieve calls (2 fan-outs x 2 channels), got %d", len(r.keywordsWeights))
	}
	for i := 0; i < 4; i += 2 {
		// Channel A: keyword-only weight, wide pool, query terms folded in.
		if r.keywordsWeights[i] != 1 {
			t.Errorf("fan-out %d channel A: keyword weight = %v, want 1 (BM25 keyword leg)", i/2, r.keywordsWeights[i])
		}
		if r.topN[i] != fanoutBM25TopN {
			t.Errorf("fan-out %d channel A: TopN = %d, want %d", i/2, r.topN[i], fanoutBM25TopN)
		}
		if !strings.Contains(r.queries[i], "built") && !strings.Contains(r.queries[i], "located") {
			t.Errorf("fan-out %d channel A: query = %q, want the fan-out query (terms folded in)", i/2, r.queries[i])
		}
		// Channel B: vector weight on, top_n=30 (narrowing bypassed).
		if r.keywordsWeights[i+1] >= 1 {
			t.Errorf("fan-out %d channel B: keyword weight = %v, want < 1 (semantic bypass leg)", i/2, r.keywordsWeights[i+1])
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
	deps := RAGTools{Search: runtime.SearchDeps{Backend: r, KbIDs: []string{"kb1"}, HasEmbedder: true}}
	st := &AgenticState{KB: &runtime.Kbinfos{}}
	added := FanoutSearch(ctx, deps, st, []string{"alpha", "beta"}, 8, 2, false)

	if added != 2 {
		t.Fatalf("added = %d, want 2 (the pool's remaining room)", added)
	}
	ids := make([]string, 0, len(st.KB.Chunks))
	for _, c := range st.KB.Chunks {
		ids = append(ids, runtime.ChunkIDOf(c))
	}
	if len(ids) != 2 || ids[0] != "ex-alpha" || ids[1] != "ex-beta" {
		t.Errorf("admitted %v, want the two exact-leg hits [ex-alpha ex-beta]", ids)
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
	added := FanoutSearch(context.Background(), deps, st, []string{"tower height topup"}, 8, 60, false)
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

// TestSCAFeedbackIsStatusOnly: the "[Research status]" note is the status hint ALONE.
// The verdict dict carries only a "status" key, so its missing_claims / hard_violations /
// agent_confidence / feedback segments are always empty — rendering them (from the
// separate SCA payload) would invent a "0.00" confidence fallback that is never
// emitted.
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
	if strings.TrimSpace(st.Draft) == "" {
		t.Fatal("expected a non-empty research draft")
	}
	if st.NoProgress {
		t.Fatal("a completed run should not report NoProgress")
	}
}

// TestAgenticGraphPushesPhaseProgress asserts that the agentic loop forwards tagged
// engine-stage lines to the caller's Progress sink as it runs — planner, research round,
// SCA — so a streaming chat can show live research progress in the reasoning block.
func TestAgenticGraphPushesPhaseProgress(t *testing.T) {
	ctx := context.Background()
	mdl := &scriptedModel{}
	mdl.push(`{"fanouts": ["when built", "where located"]}`)
	mdl.push(`{"slots":[{"id":0,"type":"aspect","question":"when built","clues":["1865"]},{"id":1,"type":"aspect","question":"where located","clues":["geneva"]}], "first_queries":["when built","where located"]}`)
	mdl.push("Built in 1865.")
	mdl.push("In Geneva.")
	mdl.push(`{"is_sufficient": true, "score": 0.9, "contradictions": [], "reasoning": "ok", "claims": {}}`)

	exec := newStubExecutor()
	exec.add("search_chunks", `{"hit":[{"doc_id":"d1","docnm_kwd":"doc1","content":"Built 1865, Geneva."}],"doc_aggs":[]}`, runtime.StatusOK)

	var lines []string
	// Mirror production: the run's step reporter carries the think-block text.
	// Each stage declares its own step, so everything asserted here is a step a
	// node reported — never a log line that happened to match a pattern.
	sink := func(line string) { lines = append(lines, line) }
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
		"[Draft] Drafted an intermediate answer",
		"[SCA] Evidence check:",
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

// TestAgenticGraphCyclesBackThroughQueryRewrite drives the research loop: the
// SCA returns insufficient once, so the graph must run query_rewrite and re-enter
// rag_agent. The loop is the only cycle in the graph — it is why the graph is
// compiled in Pregel mode (DAG mode rejects cycles).
func TestAgenticGraphCyclesBackThroughQueryRewrite(t *testing.T) {
	ctx := context.Background()
	mdl := &promptRoutedModel{}

	exec := newStubExecutor()
	exec.add("search_chunks", `{"hit":[{"doc_id":"d1","docnm_kwd":"doc1","content":"Built 1865, Geneva."}],"doc_aggs":[]}`, runtime.StatusOK)

	var buf bytes.Buffer
	var lines []string
	sink := func(line string) { lines = append(lines, line) }
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
		"[QueryRewriter]",
		"[RAGAgent] Round 2 begins",
		"[Finalize] Finalizing",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("cycle not driven: missing %q; got:\n%s", want, joined)
		}
	}
	// The second research round must actually run: the SCA is only reached again
	// through query_rewrite → rag_agent, i.e. the graph's only cycle.
	if !strings.Contains(joined, "[SCA] Evidence check: INSUFFICIENT") {
		t.Errorf("expected the first SCA to be insufficient:\n%s", joined)
	}
}

// TestBuildLowGraphRunsFormalizeThenDirectSearch covers the low-mode graph
// . It has no planner and no SCA loop, so the only
// observable contract is: formalize rewrites the question, then direct_search
// merges retrieved evidence into the kbinfos.
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

// TestSlotPrefillSummary pins what the evidence prefill leaves to research. The
// zero case matters most: it is the one that explains a round re-searching queries
// the upfront prefetch already ran.
//
// The count in the sentence is the number of sessions that will RUN
// (slotSessionsPerRound), not the number of open slots: the round opens a bounded
// number of them and re-answers the rest from the pooled evidence, so promising the
// open count sent a reader looking for searches that were never going to happen.
func TestSlotPrefillSummary(t *testing.T) {
	cases := []struct {
		name                        string
		prefilled, total, remaining int
		want                        string
	}{
		{"nothing-prefilled", 0, 5, 5,
			"The pooled evidence answers none of the 5 slots; opening 3 research sessions this round."},
		{"some-prefilled", 1, 5, 4,
			"The pooled evidence already answers 1 of the 5 slots; opening 3 research sessions for the rest."},
		// Fewer open slots than the per-round cap: 1 is what runs, not 1 capped.
		{"one-open", 4, 5, 1,
			"The pooled evidence already answers 4 of the 5 slots; opening 1 research session for the rest."},
		{"all-prefilled", 5, 5, 0,
			"The pooled evidence already answers all 5 slots; no session to run."},
	}
	for _, tc := range cases {
		if got := slotPrefillSummary(tc.prefilled, tc.total, tc.remaining); got != tc.want {
			t.Errorf("%s: slotPrefillSummary = %q, want %q", tc.name, got, tc.want)
		}
	}
}

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

// TestSCAUnavailableMarksInsufficient pins: when the SCA
// produces no usable result (timeout, unparsable reply, no model) the verdict is
// INSUFFICIENT so unresolved slots can drive another research round — NOT
// SUFFICIENT, which would ship the unverified draft as if it had passed review.
func TestSCAUnavailableIsNotAVerdict(t *testing.T) {
	st := NewAgenticState("When was it built?", "", 3, nil)
	st.Draft = "Built in 1865."
	st.KB = &runtime.Kbinfos{Chunks: []map[string]any{
		{"chunk_id": "c1", "content": "Built 1865, Geneva."},
	}}
	// A model that never returns JSON: GenJSON fails, so the SCA has no result.
	mdl := &scriptedModel{}
	mdl.push("this is not json at all")

	scaNode(context.Background(), RAGTools{Model: mdl}, st, log.New(&bytes.Buffer{}, "", 0))

	if st.Verdict != VerdictUnknown {
		t.Errorf("Verdict = %q, want UNKNOWN: a review that could not run is the ABSENCE of a verdict, not a judgement of insufficiency", st.Verdict)
	}
	if st.Verdict == VerdictInsufficient {
		t.Error("an unavailable review must not read as INSUFFICIENT: that is what marked the answer partial on no evidence and promised a research round the budget could not pay for")
	}
	if st.SCA == nil {
		t.Error("SCA payload must stay an (empty) map")
	}
	// An un-run review must not make the answer PARTIAL either: with nothing
	// unresolved and no stalled round, the deliverable is not dressed up as
	// unverified.
	st.Deadline = time.Now().Add(60 * time.Second)
	formalizeAnswerNode(context.Background(), RAGTools{}, st, log.New(&bytes.Buffer{}, "", 0))
	if st.PartialAnswer {
		t.Error("PartialAnswer = true on an UNKNOWN verdict with no unresolved slots; partial is a statement about the evidence")
	}
	// The FACT still has to reach the answer: nobody checked whether the record is complete.
	if !st.KB.SufficiencyUnchecked() {
		t.Error("an un-run review must be recorded on the run (see NoteSufficiencyUnchecked): the record has to say nobody checked, or the count reads as a checked total")
	}
	if rec := composedRecord(st.KB); !strings.Contains(rec, "nobody checked whether the members above are complete") {
		t.Errorf("record = %q, want the unchecked review stated where the answer reads it", rec)
	}
}

// TestProbeLedgerKeepsTheDeedsWordsOutOfTheNames pins what the probed list may be READ as: names.
//
// Measured (2026-09-17, 三国/关羽) the list handed the answer `杀 斩 武将 名单 亲斩 关羽 太史慈 …
// 赚城斩车胄 令左右推出斩之 庞德`, under the instruction "any name here that the record above does
// not mention is a finding nobody recorded" — an invitation to count the actor among the people he
// killed, and the deed's own verbs among its objects. The act words and query-shaped terms come
// from the run's own probes and are still shown (the ledger's job is to make the probing visible),
// but they are shown as what they are.
func TestProbeLedgerKeepsTheDeedsWordsOutOfTheNames(t *testing.T) {
	kb := &runtime.Kbinfos{}
	kb.Admit(func(p *runtime.PoolAdmitter) {
		p.Add(map[string]any{"chunk_id": "c1", "content": "关公马快，赶上文丑，脑后一刀，将文丑斩下马来。"})
	})
	// The direction's own declaration: what the run is searching WITH (actor forms + act words).
	kb.MarkCoverage(runtime.Coverage{Acts: []string{"斩", "杀"}, Actor: "关羽|云长"})
	for _, term := range []string{
		"文丑", "太史慈", "关羽", "斩孔秀", "亲斩", "关羽斩华雄|温酒斩华雄", "令左右推出斩之 庞德",
	} {
		kb.RecordReachedTerm(term, "c1")
	}

	ledger := probeLedger(kb)
	names, others, split := strings.Cut(ledger, "Probed, NOT names")
	if !split {
		t.Fatalf("ledger = %q, want the non-names said apart from the names", ledger)
	}
	if !strings.Contains(names, "文丑") || !strings.Contains(names, "太史慈") {
		t.Errorf("names = %q, want the name-shaped terms kept whatever they name (文丑 is a kill, 太史慈 a judgement the answer makes)", names)
	}
	for _, banned := range []string{"关羽", "斩孔秀", "亲斩", "关羽斩华雄|温酒斩华雄", "令左右推出斩之 庞德"} {
		if strings.Contains(names, banned) {
			t.Errorf("names = %q, must not offer %q as a name", names, banned)
		}
		if !strings.Contains(others, banned) {
			t.Errorf("non-names = %q, want %q shown as probed-but-not-a-name", others, banned)
		}
	}
}

// TestComposedRecordSaysWhenNobodyCheckedCompleteness pins the note's default and its wording: a
// run whose review ran says nothing extra, and one whose review never ran says the one thing the
// answer needs — nobody checked.
func TestComposedRecordSaysWhenNobodyCheckedCompleteness(t *testing.T) {
	kb := &runtime.Kbinfos{Record: "- slot 0 [count]: 18"}
	if rec := composedRecord(kb); strings.Contains(rec, "nobody checked") {
		t.Fatalf("record = %q, want no note while the review ran", rec)
	}
	kb.NoteSufficiencyUnchecked()
	rec := composedRecord(kb)
	if !strings.Contains(rec, "- slot 0 [count]: 18") {
		t.Fatalf("record = %q, want the slots kept", rec)
	}
	if !strings.Contains(rec, "nobody checked whether the members above are complete") ||
		!strings.Contains(rec, "do not present it as exhaustive") {
		t.Fatalf("record = %q, want the unchecked review stated", rec)
	}
}

// TestQueryRewriteKeepsTheRoundAliveWhenTheRewriterDeclines pins the asymmetry that used to end the
// loop: the routing had already judged another round worth its budget (gaps exist, the review view
// changed) and one empty reply from the rewriter cancelled that judgement — while the other
// fallback, the unresolved slots' clues, is empty on a set question whose unknowns are names nobody
// has proposed yet. Measured (2026-09-17, 三国/关羽): INSUFFICIENT, 3 gaps, +182 chunks, 130s left,
// and no round.
func TestQueryRewriteKeepsTheRoundAliveWhenTheRewriterDeclines(t *testing.T) {
	st := NewAgenticState("关羽杀了多少有姓名的人物？", "", 3, nil)
	st.SCA = map[string]any{"claims": map[string]any{
		"c1": map[string]any{"verdict": "unverified", "missing_information": []any{
			map[string]any{"what": "the count", "search_hint": "count the passages"},
		}},
	}}
	st.KB = &runtime.Kbinfos{}
	st.KB.RecordProbedAbsent("斩孟坦")
	st.SlotTable = runtime.NewState([]runtime.Variable{
		{ID: 0, Type: "count", Terms: []string{"斩"}, Subject: "关羽|云长"},
	}, 0, nil)
	st.Deadline = time.Now().Add(120 * time.Second)

	mdl := &scriptedModel{}
	mdl.push(`{"queries": []}`)
	queryRewriteNode(context.Background(), RAGTools{
		Model:  mdl,
		Search: runtime.SearchDeps{Backend: &corpusRetriever{}, KbIDs: []string{"kb1"}, HasEmbedder: true},
	}, st, log.New(&bytes.Buffer{}, "", 0))

	if st.NoProgress {
		t.Fatal("NoProgress = true: the rewriter declined, and the round had already been judged worth its budget")
	}
	joined := strings.Join(st.CurrentQueries, "|")
	if !strings.Contains(joined, "斩孟坦") || !strings.Contains(joined, "关羽") {
		t.Fatalf("CurrentQueries = %v, want the run's own open terms", st.CurrentQueries)
	}
}

// TestQueryRewriteFoldsUnresolvedCluesWhenNoGaps pins: with
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
		Search: runtime.SearchDeps{Backend: &corpusRetriever{}, KbIDs: []string{"kb1"}, HasEmbedder: true},
	}, st, log.New(&bytes.Buffer{}, "", 0))

	if st.NoProgress {
		t.Fatal("NoProgress = true; unresolved slot clues must be folded into the gaps and researched")
	}
	joined := strings.Join(st.CurrentQueries, "|")
	if !strings.Contains(joined, "when opened") {
		t.Errorf("CurrentQueries = %v, want the unresolved slot clue to be pursued", st.CurrentQueries)
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

// TestBuildSCAClaimsFallsBackToEvidenceOnlyClaim pins: with no
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

// TestBuildSCAClaimsAppendsMissingSlotEvidence pins: a slot
// whose evidence chunk is NOT in the view has that chunk appended, and the claim
// carries its new POSITION. It is appended to the same list and reviewed against, so
// buildSCAClaims returns the grown view.
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
		nodeQueryRewrite:    "query_rewrite",
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

func (r *rfRetriever) Retrieve(_ context.Context, req runtime.RetrieveRequest) ([]map[string]any, error) {
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
type corpusRetriever struct{ calls []string }

func (c *corpusRetriever) Retrieve(_ context.Context, req runtime.RetrieveRequest) ([]map[string]any, error) {
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
		var lines []string
		ctx := runtime.WithSteps(context.Background(),
			runtime.StepReporter{Text: func(line string) { lines = append(lines, line) }})
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
	res := ComposeAnswer(context.Background(), AnswerDeps{Model: mdl}, kb, "Who created Culdcept?", false, false)
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
	if res := ComposeAnswer(context.Background(), AnswerDeps{Model: mdl}, kb, "what colour is it?", false, false); res.Failed {
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

// TestAnswerPromptNoEvidenceInstructions pins: when the call is
// not short-circuited by empty_response, the prompt must still tell the model how
// to degrade — from the research summary if there is one, otherwise with an
// explicit insufficiency statement.
func TestAnswerPromptNoEvidenceInstructions(t *testing.T) {
	// No chunks, no summary -> insufficiency statement.
	mdl := &fakeModel{replies: []*runtime.ModelReply{{Content: "unknown [1]."}}}
	ComposeAnswer(context.Background(), AnswerDeps{Model: mdl}, &runtime.Kbinfos{}, "q?", false, false)
	if got := mdl.lastUserPrompt(); !strings.Contains(got, "No supporting evidence was retrieved") {
		t.Errorf("prompt missing the no-evidence instruction:\n%s", got)
	}

	// No chunks but a draft exists -> answer from the summary, do not refuse.
	kb := &runtime.Kbinfos{PreSummary: "partial findings"}
	mdl2 := &fakeModel{replies: []*runtime.ModelReply{{Content: "x [1]."}}}
	ComposeAnswer(context.Background(), AnswerDeps{Model: mdl2}, kb, "q?", false, false)
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
	ComposeAnswer(context.Background(), AnswerDeps{Model: mdl}, kb, "q?", true, false)

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
// round: "Output:\n" user turn. It was previously an
// invisible convention buried inside jsonModelAdapter.GenJSON — a second JSONModel
// implementation could silently drop it without any test catching it. See the
// query_rewriter.go note (diff ②).
func TestGenJSONUsesOutputNewlineUserTurn(t *testing.T) {
	mdl := &scriptedModel{}
	mdl.push("{\"ok\": true}")

	if _, err := (&jsonModelAdapter{inner: mdl}).GenJSON(context.Background(), "Render JSON."); err != nil {
		t.Fatalf("GenJSON failed: %v", err)
	}
	if got := userTurnAt(mdl, 0); got != "Output:\n" {
		t.Errorf("first-round user turn = %q, want exactly %q (the gen_json separator)", got, "Output:\n")
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

// TestGenJSONFitsPromptToContextWindow: the fitted prompt is bounded by the model's
// context window, so an oversized system prompt must be trimmed BEFORE the first call
// instead of being sent verbatim (the provider would reject it).
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

func (m *errOnceModel) Complete(_ context.Context, _ []schema.Message, _ []runtime.ToolSpec) (*runtime.ModelReply, error) {
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

func TestAgenticResearchRoundCostsThreeVisits(t *testing.T) {
	// One research round is rag_agent → draft → sca. If the loop counted iterations
	// instead of node visits, 20 rounds would cost 20 instead of 60 and the guard would
	// trip ~3x later.
	if got, want := agenticRoundVisits, 3; got != want {
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
// MinRoundHeadroomS guard near the boundary and skip a round Python would run.
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
	cache.NoteUnanswerable(VerdictInsufficient)
	if cache.ConsecutiveUnanswerable() != 1 {
		t.Fatalf("after 1st outer rag() call: ConsecutiveUnanswerable = %d, want 1",
			cache.ConsecutiveUnanswerable())
	}

	// Second outer rag() call — still unsatisfying: counter must reach 2 so the
	// STOP guard can fire (the state the pre-fix code could never reach on the
	// outer path, because deps.Cache was nil and the increment was skipped).
	cache.NoteUnanswerable(VerdictInsufficient)
	if cache.ConsecutiveUnanswerable() != 2 {
		t.Fatalf("after 2nd outer rag() call: ConsecutiveUnanswerable = %d, want 2",
			cache.ConsecutiveUnanswerable())
	}

	// A satisfying verdict resets the streak.
	cache.NoteUnanswerable(VerdictSufficient)
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
	(*RAGCache)(nil).NoteUnanswerable(VerdictInsufficient)
	(*RAGCache)(nil).NoteUnanswerable(VerdictSufficient)
}

// TestRewriteContextShowsThePassageBehindAConfirmedMember pins the input side of
// the rewrite round: what the rewriter is shown is the PASSAGE that carries each
// confirmed name, not the first line of a stored chunk.
//
// The difference is the whole point of that round. The wording a text uses for the
// relation (and the other names it mentions) lives in the middle of a passage, so
// a first-line digest cannot carry it — and a rewrite that cannot see it can only
// re-ask what was already asked.
func TestRewriteContextShowsThePassageBehindAConfirmedMember(t *testing.T) {
	st := NewAgenticState("关羽杀了多少有姓名的人物？", "", 3, nil)
	st.KB = &runtime.Kbinfos{Chunks: []map[string]any{
		{"chunk_id": "w1", "content": "荀正 引军来战，被关公一刀斩于马下。"},
		{"chunk_id": "big", "content": "第一行与本题无关\n第二行才提到关公"},
	}}
	st.KB.RecordReachedTerm("荀正", "w1")

	ctx := renderResearchContext(st)
	if !strings.Contains(ctx, "荀正") || !strings.Contains(ctx, "斩于马下") {
		t.Errorf("rewrite context does not carry the passage behind the confirmed member:\n%s", ctx)
	}
	if strings.Contains(ctx, "第一行与本题无关") {
		t.Errorf("rewrite context fell back to first-line digests although a confirmed member's passage was available:\n%s", ctx)
	}
	if !strings.Contains(ctx, "ALREADY confirmed") {
		t.Errorf("the member section is not labelled:\n%s", ctx)
	}
}

// TestRewriteRoundCanStillAdmitOnARichPool pins the room a rewrite round is given:
// it is measured against the EVIDENCE POOL's ceiling, the same number the admitter
// enforces.
//
// Sized against the smaller snippet-pool constant, a pool past 60 chunks left the
// round with room <= 0: it admitted nothing, declared "retrieval saturated" and
// discarded the queries it had just built — on exactly the rich rounds where more
// evidence was still arriving.
func TestRewriteRoundCanStillAdmitOnARichPool(t *testing.T) {
	pool := make([]map[string]any, 0, runtime.EvidencePoolCap())
	for i := 0; i < 71; i++ {
		pool = append(pool, map[string]any{"chunk_id": fmt.Sprintf("pre-%d", i), "content": "already pooled"})
	}
	st := NewAgenticState("关羽杀了多少有姓名的人物？", "", 3, nil)
	st.KB = &runtime.Kbinfos{Chunks: pool}
	st.UnresolvedSlots = []map[string]any{{"question_clues": []string{"还有谁被关羽所杀"}}}
	st.Deadline = time.Now().Add(60 * time.Second)

	mdl := &scriptedModel{}
	mdl.push(`{"queries": [{"query": "关羽 斩 名单 其余"}]}`)

	queryRewriteNode(context.Background(), RAGTools{
		Model:  mdl,
		Search: runtime.SearchDeps{Backend: &corpusRetriever{}, KbIDs: []string{"kb1"}, HasEmbedder: true},
	}, st, log.New(&bytes.Buffer{}, "", 0))

	if st.NoProgress {
		t.Fatal("NoProgress = true on a pool of 71 with room left: the round's room must be measured against the evidence pool's ceiling")
	}
	if len(st.KB.Chunks) <= 71 {
		t.Errorf("pool = %d, want the rewrite's own query admitted (cap %d, pool was 71)", len(st.KB.Chunks), runtime.EvidencePoolCap())
	}
}

// TestRenderSlotRecordCarriesNoMachineFields pins the split between the record the
// ANSWER is composed from and the draft the SCA reviews.
//
// The draft deliberately carries the machine fields that make verification
// possible (strength, terminal type, evidence ids). Handing those to the answer
// model as a "summary" is a different act with a different failure mode, and it
// was measured: the composed answer quoted the bookkeeping verbatim
// ("slot 1 [entity] … (strength=0.90) [terminal=state, evidence_ids=[…]]").
func TestRenderSlotRecordCarriesNoMachineFields(t *testing.T) {
	strong := 0.9
	st := runtime.NewState([]runtime.Variable{
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

	record := RenderSlotRecord(st, "the collected answer")
	// A table that asks for no SET leads with the session's own answer, exactly as
	// it did before the answer-layer demotion existed — see
	// TestSlotRecordLeadsWithFactsNotWithASessionsProse for the SET case, and
	// TestValueRecordCarriesNoEnumeratedMembers for why the two differ.
	if !strings.HasPrefix(record, "Candidate answer: the collected answer\n\n- slot 0 [aspect]: answer A\n- slot 1 [aspect]: NOT RESOLVED") {
		t.Fatalf("record = %q, want the session's answer first on a value table", record)
	}
	if strings.Contains(record, "enumerated members") {
		t.Fatalf("record = %q, want no enumerated line on a value table", record)
	}
	for _, banned := range []string{"strength=", "terminal=", "evidence_ids", "c1"} {
		if strings.Contains(record, banned) {
			t.Errorf("record %q must not carry the machine field %q", record, banned)
		}
	}
	// The SCA's draft keeps them: the split must not disarm verification.
	draft := RenderSlotDraft(st, "the collected answer", evidence)
	for _, want := range []string{"strength=0.90", "terminal=state", "evidence_ids=['c1', 'c2']"} {
		if !strings.Contains(draft, want) {
			t.Errorf("draft %q must keep %q for the SCA", draft, want)
		}
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

// TestAnswerRecordCarriesTheProbeLedger pins the block that closes the measured
// write-back gap: the slots are the model's own bookkeeping, so a run can probe
// twenty-four terms, get a passage for every one, and record eleven — and the
// answer then sees only the eleven while the rest exist merely inside the pool.
//
// The two lists stay apart on purpose: "reached" is a fact about the corpus,
// "nothing back" is a fact about the QUERY (not an absence).
func TestAnswerRecordCarriesTheProbeLedger(t *testing.T) {
	kb := &runtime.Kbinfos{Record: "- slot 0 [count]: 11"}
	kb.RecordReachedTerm("车胄", "c1")
	kb.RecordReachedTerm("管亥", "c2")
	kb.RecordProbedAbsent("温酒")

	prompt := AnswerDeps{}.answerPromptWithEvidence(kb, "q", false, false)
	for _, want := range []string{
		"- slot 0 [count]: 11", // the slots come first
		"Probed and answered",
		"车胄", "管亥",
		"Probed with NOTHING back",
		"温酒",
	} {
		if !strings.Contains(prompt.user, want) {
			t.Errorf("answer prompt missing %q", want)
		}
	}
	// "Nothing back" may not be described as absence: that reading is what stops
	// an enumeration short.
	if strings.Contains(prompt.user, "absent from the corpus") && !strings.Contains(prompt.user, "this is not \"absent\"") {
		t.Error("the not-reached list must refuse the absence reading")
	}
	// The ledger belongs to the answer prompt, not to the SCA-facing draft.
	if strings.Contains(kb.PreSummary, "Probed and answered") {
		t.Error("the probe ledger must not leak into the SCA draft")
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

// TestSyncCountSlotsTakesTheEnumeratedSize pins the other half of the same
// loss: the count slot and the list slots are written by different sessions and
// nothing kept them in step. The slot takes the DERIVED number, because a count is a
// claim the table can check.
//
// The direction that matters is the one an answer repeats: measured (2026-09-16) a
// count slot left holding a session's 28 against thirteen enumerated members
// produced "killed 28 named people" over a list of a handful. An over-claim is kept
// as an alternate clue, so the record still shows what was claimed.
//
// A slot that does not CLAIM a number is left alone — a date, a phrase or a sentence
// is nobody's count, and reading the slot's declared TYPE to guess otherwise was the
// rule this replaces (see slots.Value.Number).
func TestSyncCountSlotsTakesTheEnumeratedSize(t *testing.T) {
	table := runtime.NewState([]runtime.Variable{
		typedCountVar(0, "count", 10, 0),
		typedAnchoredMembersVar(1, "person", "华雄、颜良、文丑、孔秀、孟坦、韩福、卞喜、王植、秦琪、蔡阳、车胄、管亥"),
	}, 0, nil)
	raised := syncCountSlots(&table)
	if len(raised) != 1 || raised[0] != 0 {
		t.Fatalf("raised = %v, want the count slot reconciled", raised)
	}
	if got := *table.ByID(0).Candidate; got != "12" {
		t.Fatalf("count slot = %q, want 12 (the list's size)", got)
	}

	// An over-claim does not stand: the members are what the table can check, and
	// the claim is kept beside them as an alternate.
	bigger := runtime.NewState([]runtime.Variable{
		typedCountVar(0, "count", 16, 0),
		typedAnchoredMembersVar(1, "person", "华雄、颜良、文丑、孔秀、孟坦、韩福"),
	}, 0, nil)
	if changed := syncCountSlots(&bigger); len(changed) != 1 {
		t.Fatalf("changed = %v, want the over-claim reconciled to the members", changed)
	}
	if got := *bigger.ByID(0).Candidate; got != "6" {
		t.Fatalf("count slot = %q, want 6 (the members' size)", got)
	}
	alts := alternateCandidatesOf(*bigger.ByID(0))
	if len(alts) != 1 || alts[0] != "16" {
		t.Fatalf("alternates = %v, want the over-claim kept as the record's alternate", alts)
	}

	// A count that already agrees is left alone.
	agreed := runtime.NewState([]runtime.Variable{
		typedCountVar(0, "count", 6, 0),
		typedAnchoredMembersVar(1, "person", "华雄、颜良、文丑、孔秀、孟坦、韩福"),
	}, 0, nil)
	if changed := syncCountSlots(&agreed); len(changed) != 0 {
		t.Fatalf("changed = %v, want an agreeing count untouched", changed)
	}

	// A count slot whose text is NOT a number is left alone: the slot made no checkable
	// claim, so there is nothing here to correct — and the answer's count comes from the
	// members either way (measured 2026-09-16, 三国/关羽: a session wrote "约 17-19 人"
	// into the count slot, and the members beside it are what the answer must repeat).
	unreadable := runtime.NewState([]runtime.Variable{
		{ID: 0, Type: "count", Candidate: strPtr("约 17-19 人")},
		typedAnchoredMembersVar(1, "person", "华雄、颜良、文丑、孔秀、孟坦、韩福"),
	}, 0, nil)
	if changed := syncCountSlots(&unreadable); len(changed) != 0 {
		t.Fatalf("changed = %v, want an unreadable claim left alone", changed)
	}

	// The count of a table whose member slot is TEXT is left alone: prose claims no
	// number and no members, and nothing here guesses at either.
	textOnly := runtime.NewState([]runtime.Variable{
		{ID: 0, Type: "count", Candidate: strPtr("约 17-19 人")},
		{ID: 1, Type: "person", Candidate: strPtr("华雄、颜良、文丑")},
	}, 0, nil)
	if changed := syncCountSlots(&textOnly); len(changed) != 0 {
		t.Fatalf("changed = %v, want nothing derived from text", changed)
	}
}

// TestMemberCountIgnoresProseFragments pins the count's unit: a member is a NAME,
// not a fragment of the sentence around it.
//
// The fixture is a measured record shape — the slots held the right names wrapped
// in chapter prose, SplitCandidateNames cut that prose at its separators, and the
// enumerated size came out 28 against thirteen real names. That number was then
// raised into the count slot and reported by the answer as its own.
func TestMemberCountIgnoresProseFragments(t *testing.T) {
	// The prose is Text and the names are declared members. The fixture that used to
	// prove this (names wrapped in chapter prose, with the parser cutting the prose
	// into "members") cannot be written any more: an undeclared slot contributes
	// nothing, so the count is the members or it is nothing.
	table := runtime.NewState([]runtime.Variable{
		typedCountVar(0, "count", 28, 0),
		typedAnchoredMembersVar(1, "web", "孔秀、孟坦、韩福、卞喜、王植、秦琪、蔡阳"),
		{ID: 2, Type: "web", Candidate: strPtr("第五回（发矫诏诸镇应曹公、破关兵三英战吕布）、第二十五回（屯土山关公约三事、救白马曹操解重围）")},
		typedAnchoredMembersVar(3, "web", "华雄、颜良、文丑、庞德"),
	}, 0, nil)

	// Eleven distinct real names: the prose pieces around them are not members.
	if got := enumeratedSize(table); got != 11 {
		t.Fatalf("enumerated size = %d, want 11 (the declared names, not the chapter prose)", got)
	}
	// And the count slot takes that number rather than the session's claim.
	syncCountSlots(&table)
	if got := *table.ByID(0).Candidate; got != "11" {
		t.Fatalf("count slot = %q, want 11 (the enumerated members)", got)
	}
}

// TestSyncCountSlotsCountsOnlyMembersWithAPassage pins the count's unit one level further down:
// a member counts when it carries the passage that states it.
//
// The fixture is the measured case (2026-09-17, 三国/关羽): a session's member list merged with the
// resolve's, and among the names two that no passage in hand quotes — 于禁 (taken at 水淹七军 and
// released, never killed) and 刘延 (who outlives the actor). The union was 18, the answer stated
// 18, and sixteen of those names had a passage behind them. A claim is not a member: the number
// is what the table can point at, and the claim stays visible as the slot's alternate.
func TestSyncCountSlotsCountsOnlyMembersWithAPassage(t *testing.T) {
	table := runtime.NewState([]runtime.Variable{
		typedCountVar(0, "count", 18, 0),
		typedAnchoredMembersVar(1, "person", "华雄、荀正"),
		typedMembersVar(2, "person", "于禁、刘延"),
	}, 0, nil)
	raised := syncCountSlots(&table)
	if len(raised) != 1 || raised[0] != 0 {
		t.Fatalf("raised = %v, want the count slot reconciled to the members with a passage", raised)
	}
	if got := *table.ByID(0).Candidate; got != "2" {
		t.Fatalf("count slot = %q, want 2 (the members the table can quote)", got)
	}
	if alts := alternateCandidatesOf(*table.ByID(0)); len(alts) != 1 || alts[0] != "18" {
		t.Fatalf("alternates = %v, want the 18 kept as the record's alternate", alts)
	}
}

// TestValueQuestionRecordKeepsItsOwnDraft pins the FRAMES regression of 2026-09-17 (q759): a table
// whose slots hold one person each is NOT an enumeration, and the record is the only place the run's
// own draft answer reaches the answer model.
//
// Rendered as a set, the record dropped "Candidate answer:" — the one line carrying the run's own
// finding — and asked the answer to state a count of members instead. That is the failure the
// record's gate was written against ("the session's own draft answer is then demoted below a line
// that does not apply to it"), and it is why Set stays the planner's declaration.
func TestValueQuestionRecordKeepsItsOwnDraft(t *testing.T) {
	items := slots.Items(slots.Item{Value: "Lanee Butler", ChunkID: "c1", Quote: "Mistral (sailboard) | Lanee Butler United States"})
	table := runtime.NewState([]runtime.Variable{
		{ID: 0, Type: "person", Value: &items},
		{ID: 1, Type: "person"},
	}, 0, nil)
	rec := RenderSlotRecord(table, "The person is Richard Coxon, crewed with Colin Beashel in the Soling class.")
	if !strings.Contains(rec, "Candidate answer: The person is Richard Coxon") {
		t.Fatalf("record = %q, want the run's own draft kept for a question whose answer is one value", rec)
	}
	if strings.Contains(rec, "enumerated members across the slots above") {
		t.Fatalf("record = %q, want no set block on a value question", rec)
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
func TestRunCoverageResolveEnrollsTheEnumerationThePlannerSkipped(t *testing.T) {
	items := slots.Items(
		slots.Item{Value: "华雄", ChunkID: "c-华雄"},
		slots.Item{Value: "颜良", ChunkID: "c-颜良"},
	)
	rendered := slots.Render(items)
	table := runtime.NewState([]runtime.Variable{
		{ID: 0, Type: "count", Terms: []string{"斩"}, Candidate: &rendered, Value: &items},
	}, 0, nil)

	exec := &coverageStubExec{}
	st := NewAgenticState("关羽杀了多少有姓名的人物？", "", 3, nil)
	st.KB = &runtime.Kbinfos{}
	// The direction's words must already have MET in what the run holds, or the enrollment is spent
	// on a recall whose windows the enumeration's own filter would drop (see
	// CoverageActsMeetActor). Here 斩 is held; the stub's window then survives.
	st.KB.Admit(func(p *runtime.PoolAdmitter) {
		p.Add(map[string]any{"chunk_id": "held", "content_with_weight": "云长提刀直取，斩之"})
	})
	st.SlotTable = table
	st.Deadline = time.Now().Add(120 * time.Second)

	model := &scriptedModel{}
	model.push(`{"members": [{"i": 0, "name": "孔秀"}], "not_members": []}`)
	RunCoverageResolve(context.Background(), RAGTools{
		Model: model,
		Tools: &runtime.Toolset{Exec: exec},
	}, st, nil)

	if exec.ran() != 1 {
		t.Fatalf("enumeration ran %d time(s), want the resolve to enroll it when no type word asked for it", exec.ran())
	}
	names := strings.Join(runtime.ItemValues(&st.SlotTable), "、")
	if !strings.Contains(names, "孔秀") {
		t.Fatalf("items = %q, want the window the enrolled enumeration admitted judged into the table", names)
	}
}

// TestRunCoverageResolveSkipsTheEnumerationWhenTheWordsNeverMet pins the guard the measured waste
// bought: on 2026-09-17 (FRAMES, resolve node) the enrollment asked 13 operands, recalled 724
// passages and produced ZERO windows, because every window the enumeration builds must carry an act
// word AND the actor, and this direction's two words had never met in anything the run held.
//
// The enrollment spends the ANSWER's clock, so the same conjunction is now probed against the
// passages in hand first: a recall that cannot survive the filter is not made.
func TestRunCoverageResolveSkipsTheEnumerationWhenTheWordsNeverMet(t *testing.T) {
	items := slots.Items(slots.Item{Value: "Lanee Butler", ChunkID: "c-Lanee"})
	rendered := slots.Render(items)
	table := runtime.NewState([]runtime.Variable{
		{ID: 0, Type: "person", Terms: []string{"partner"}, Subject: "Colin Beashel", Candidate: &rendered, Value: &items},
	}, 0, nil)

	exec := &coverageStubExec{}
	st := NewAgenticState("who is the partner of the 1984 keelboat sailor?", "", 3, nil)
	st.KB = &runtime.Kbinfos{}
	// Held passages mention the actor, and passages carry the act word — never both in one.
	st.KB.Admit(func(p *runtime.PoolAdmitter) {
		p.Add(map[string]any{"chunk_id": "held-actor", "content_with_weight": "Colin Beashel sailed the Soling class."})
		p.Add(map[string]any{"chunk_id": "held-act", "content_with_weight": "Their partner was crewing that year."})
	})
	st.SlotTable = table
	st.Deadline = time.Now().Add(120 * time.Second)

	RunCoverageResolve(context.Background(), RAGTools{
		Model: &scriptedModel{},
		Tools: &runtime.Toolset{Exec: exec},
	}, st, nil)

	if exec.ran() != 0 {
		t.Fatalf("enumeration ran %d time(s), want no recall for a direction whose words have never met in one passage", exec.ran())
	}
}

// TestCiteChunksPutTheItemsPassagesFirst pins the citation set of an enumerated answer.
//
// The answer has to cite one passage per element, and those passages are in the pool but not
// necessarily among the few that score highest: a sixteen-member table whose answer named only
// three of them was reading the same handful of blocks as any other question. Members first also
// means a token budget that truncates drops scored extras, never an element's own passage.
func TestCiteChunksPutTheItemsPassagesFirst(t *testing.T) {
	chunks := []map[string]any{
		{"chunk_id": "w1", "content": "低分段落", "similarity": 0.1},
		{"chunk_id": "w2", "content": "高分段落", "similarity": 0.9},
		{"chunk_id": "w3", "content": "成员自己的段落", "similarity": 0.2},
	}
	kb := &runtime.Kbinfos{Chunks: chunks}
	kb.NoteCitedChunks([]string{"w3"})

	got := withCitedChunks(rankByScore(chunks), kb, citeChunkCap)
	if len(got) != 3 || runtime.ChunkIDOf(got[0]) != "w3" {
		t.Fatalf("citeChunks = %v, want the items' own passage first and the rest after it", got)
	}
	if runtime.ChunkIDOf(got[1]) != "w2" {
		t.Fatalf("citeChunks = %v, want the scored chunks after the items' passages", got)
	}

	// Without items the selection is unchanged: the top-scoring chunk, capped.
	plain := withCitedChunks(rankByScore(chunks), &runtime.Kbinfos{Chunks: chunks}, 1)
	if len(plain) != 1 || runtime.ChunkIDOf(plain[0]) != "w2" {
		t.Fatalf("citeChunks = %v, want the plain top-N selection when no items carry passages", plain)
	}
}

// TestSlotRecordShowsEachItemWithItsPassage pins the citation half of the record: an item's chunk id
// is in the value, and the record used to print names only — so the instruction to cite each member
// ("every member you list carries the words behind it") had nothing to cite.
func TestSlotRecordShowsEachItemWithItsPassage(t *testing.T) {
	table := runtime.NewState([]runtime.Variable{
		typedAnchoredMembersVar(1, "person", "华雄、荀正"),
		typedMembersVar(2, "person", "于禁"),
	}, 0, nil)
	rec := RenderSlotRecord(table, "")
	for _, want := range []string{"华雄 ←c-华雄", "荀正 ←c-荀正", "于禁 ←(no passage)"} {
		if !strings.Contains(rec, want) {
			t.Fatalf("record = %q, want %q", rec, want)
		}
	}
}

// TestSlotRecordStatesACountSlotThatHoldsWords pins the silent half of the count disagreement.
//
// A count slot whose value is TEXT passes in silence: it claims no number, so nothing is derived
// from it (by contract), and the record used to say nothing either — its words sat beside the
// enumerated size and the answer took whichever it liked. Measured (2026-09-17, 三国/关羽): a
// session's "14" answered a table that enumerated 16. The words are not parsed here either; the
// disagreement is stated.
func TestSlotRecordStatesACountSlotThatHoldsWords(t *testing.T) {
	table := runtime.NewState([]runtime.Variable{
		{ID: 0, Type: "count", Candidate: strPtr("14")},
		typedAnchoredMembersVar(1, "person", "华雄、荀正、杨龄"),
	}, 0, nil)
	rec := RenderSlotRecord(table, "")
	if !strings.Contains(rec, "enumerated members across the slots above: 3") {
		t.Fatalf("record = %q, want the enumerated size stated", rec)
	}
	if !strings.Contains(rec, `holds "14" as text, not a number`) {
		t.Fatalf("record = %q, want a count slot holding words stated as a disagreement", rec)
	}

	// The declared-number branch keeps its own wording.
	numbered := runtime.NewState([]runtime.Variable{
		typedCountVar(0, "count", 14, 0),
		typedAnchoredMembersVar(1, "person", "华雄、荀正、杨龄"),
	}, 0, nil)
	if rec := RenderSlotRecord(numbered, ""); !strings.Contains(rec, "says 14 while the slots above enumerate 3") {
		t.Fatalf("record = %q, want a declared number's disagreement stated as before", rec)
	}
}

// TestSyncCountSlotsReadsTheWholeTable pins the ONE assumption the derivation carries: the size is
// read ACROSS the table, so a question that enumerates two sets gets one number for both count
// slots (`斩杀` and `生擒` both read 4 here).
//
// It is pinned rather than fixed because the fix is a DECLARATION — the planner saying which count
// counts which set — and an inference (adjacency, wording) would guess exactly where the record has
// to be exact. Until that declaration exists, the record's own line is the honest reading: it says
// "across the slots above", not per slot.
func TestSyncCountSlotsReadsTheWholeTable(t *testing.T) {
	table := runtime.NewState([]runtime.Variable{
		typedCountVar(0, "count", 9, 0),
		typedAnchoredMembersVar(1, "person", "华雄、荀正"),
		typedCountVar(2, "count", 4, 0),
		typedAnchoredMembersVar(3, "person", "于禁、庞德"),
	}, 0, nil)
	syncCountSlots(&table)
	for _, id := range []int{0, 2} {
		if got := *table.ByID(id).Candidate; got != "4" {
			t.Fatalf("slot %d = %q, want the table-wide size: two sets per table are a declaration the planner does not make yet", id, got)
		}
	}
}

// TestSyncCountSlotsLeavesWhatClaimsNoNumber pins the kind-keyed half: the derivation touches the
// slots that CLAIM a number and nothing else — a text claim is not a number anybody can check, and
// the floor keeps a barely-filled table from collapsing a claim to one.
func TestSyncCountSlotsLeavesWhatClaimsNoNumber(t *testing.T) {
	table := runtime.NewState([]runtime.Variable{
		{ID: 0, Type: "count", Candidate: strPtr("约 17-19 人")}, // prose: claims no number
		typedAnchoredMembersVar(1, "person", "华雄、荀正、杨龄"),
	}, 0, nil)
	if changed := syncCountSlots(&table); len(changed) != 0 {
		t.Fatalf("changed = %v, want a claim that is not a number left alone", changed)
	}
	// One quotable element is below the floor: nothing to derive from, so the claim stands.
	thin := runtime.NewState([]runtime.Variable{
		typedCountVar(0, "count", 9, 0),
		typedAnchoredMembersVar(1, "person", "华雄"),
	}, 0, nil)
	if changed := syncCountSlots(&thin); len(changed) != 0 {
		t.Fatalf("changed = %v, want the floor respected (one element is not an enumeration)", changed)
	}
	if got := *thin.ByID(0).Candidate; got != "9" {
		t.Fatalf("count slot = %q, want the claim standing below the floor", got)
	}
}

// TestSlotRecordNamesTheClaimsWithoutAPassage pins the other half of that rule: leaving a claim
// out of the number must not hide it. An answer told only "the members are two" cannot tell that
// two more names were claimed — nor that its number may therefore be short.
func TestSlotRecordNamesTheClaimsWithoutAPassage(t *testing.T) {
	table := runtime.NewState([]runtime.Variable{
		typedCountVar(0, "count", 4, 0),
		typedAnchoredMembersVar(1, "person", "荀正、杨龄"),
		typedMembersVar(2, "person", "于禁、刘延"),
	}, 0, nil)
	rec := RenderSlotRecord(table, "")
	if !strings.Contains(rec, "enumerated members across the slots above: 2") {
		t.Fatalf("record = %q, want the size stated over the members with a passage", rec)
	}
	if !strings.Contains(rec, "claimed WITHOUT a passage in hand") ||
		!strings.Contains(rec, "于禁") || !strings.Contains(rec, "刘延") {
		t.Fatalf("record = %q, want the claims named beside the number", rec)
	}
}

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

// TestSlotRecordLeadsWithFactsNotWithASessionsProse pins the answer-layer fix.
//
// The record used to LEAD with one session's prose draft answer, and the answer
// copied it: measured twice (2026-09-15, 三国演义/关羽) — a record whose slots
// enumerated seventeen members produced a fifteen-member answer, and a record
// enumerating fourteen produced a ten-member answer, in both cases exactly the
// number written in that prose. The prose is one session's recollection, written
// before the other sessions were merged; it is a claim to reconcile with the
// members, not the record.
func TestSlotRecordLeadsWithFactsNotWithASessionsProse(t *testing.T) {
	table := runtime.NewState([]runtime.Variable{
		typedCountVar(0, "count", 10, 0),
		typedAnchoredMembersVar(1, "person", "华雄、颜良、文丑、孔秀、孟坦、韩福、卞喜、王植、秦琪、蔡阳、车胄、程远志、夏侯存、庞德"),
		// A second slot of the same table holds REFERENCES, not members — and it is
		// TEXT, so it contributes none: counting them is how this record once
		// answered its own count with a nineteen on a twelve-member list.
		{ID: 2, Type: "dataset", Candidate: strPtr("第5回(华雄)、第21回(车胄)、第25回(颜良)、第27回(五关六将)、第74回(庞德)")},
	}, 0, nil)
	rec := RenderSlotRecord(table, "关羽在《三国演义》中斩杀的有姓名人物共10人，名单如下：……")

	if !strings.Contains(rec, "enumerated members across the slots above: 14") {
		t.Fatalf("record = %q, want the enumerated size stated as a fact", rec)
	}
	// A count that disagrees with the members has to be SAID: this record once
	// produced a nineteen-person answer out of a count slot reading 19, which the
	// answer explained as seven more people "the material does not list".
	if !strings.Contains(rec, "slot 0 [count] says 10 while the slots above enumerate 14") {
		t.Fatalf("record = %q, want the disagreement stated", rec)
	}
	// Stated, not ordered: an order was tried and reverted — measured (2026-09-16,
	// 三国/关羽) a record that said "take the number from the enumerated members" got
	// exactly that number taken (nine), when its slots enumerated the wrong things and
	// its sessions had found fourteen. Either number can be the wrong one, and only the
	// passages decide, so the note must state the disagreement and leave the judgement
	// to the stage that reads them.
	if strings.Contains(rec, "take the number from") {
		t.Fatalf("record = %q, must not order the answer to take either number", rec)
	}
	if !strings.Contains(rec, "disagree") {
		t.Fatalf("record = %q, want the disagreement stated as a disagreement", rec)
	}
	facts := strings.Index(rec, "slot 1 [person]")
	draft := strings.Index(rec, "One session's own draft answer")
	if facts < 0 || draft < 0 || facts > draft {
		t.Fatalf("record = %q, want the slots BEFORE the session's own draft", rec)
	}
	if !strings.Contains(rec, "UNVERIFIED") {
		t.Fatalf("record = %q, want the draft labelled as a claim", rec)
	}
}

// TestValueRecordCarriesNoEnumeratedMembers pins the other half of the gate: the
// enumerated size, the count-vs-members warning and the demotion of the session's
// prose are SET-question machinery, and a record whose answer is one date or one
// number must render exactly as it did before any of it existed.
//
// The table is the measured case. A "how much shorter is A than B" question's
// record carried `- slot 1 [number]: 133 feet` beside `- enumerated members across
// the slots above: 16`, where the 16 was `Grace's、High、Falls、Colonial、Creek` —
// one waterfall's name cut at its separators by whoever wrote it into the slot —
// and the session's draft answer was labelled UNVERIFIED below it. Rendered
// ungated, those two lines appeared in 21 of a 20-question FRAMES run's records.
func TestValueRecordCarriesNoEnumeratedMembers(t *testing.T) {
	table := runtime.NewState([]runtime.Variable{
		{ID: 0, Type: "dataset", Candidate: strPtr("Grace's、High、Falls、Colonial、Creek")},
		{ID: 1, Type: "number", Candidate: strPtr("133 feet")},
		{ID: 2, Type: "web", Candidate: strPtr("Colonial Creek Falls, Washington — 788 m (2,585 ft)")},
	}, 0, nil)
	rec := RenderSlotRecord(table, "Alabama's tallest waterfall is 133 feet tall.")

	if strings.Contains(rec, "enumerated members") {
		t.Fatalf("record = %q, want no enumerated line on a value table", rec)
	}
	if strings.Contains(rec, "UNVERIFIED") {
		t.Fatalf("record = %q, want the session's answer NOT demoted on a value table", rec)
	}
	if !strings.HasPrefix(rec, "Candidate answer: Alabama's tallest waterfall is 133 feet tall.") {
		t.Fatalf("record = %q, want the session's answer first on a value table", rec)
	}
	for _, want := range []string{"- slot 0 [dataset]:", "- slot 1 [number]: 133 feet", "788 m (2,585 ft)"} {
		if !strings.Contains(rec, want) {
			t.Errorf("record %q missing the slot fact %q", rec, want)
		}
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

func typedCountVar(id int, typ string, n int, strength float64) runtime.Variable {
	v := slots.Number(n)
	rendered := slots.Render(v)
	out := runtime.Variable{ID: id, Type: typ, Candidate: &rendered, Value: &v}
	if strength > 0 {
		out.CandidateStrength = &strength
	}
	return out
}

func TestUnionIsOrderIndependent(t *testing.T) {
	eleven := typedMembersValue("华雄、管亥、颜良、文丑、孔秀、孟坦、韩福、卞喜、王植、秦琪、蔡阳")
	thirteen := typedMembersValue("华雄、颜良、文丑、孔秀、孟坦、韩福、卞喜、王植、秦琪、蔡阳、庞德、成何、管亥")
	names := func(v slots.Value) string {
		out := v.ItemValues()
		sort.Strings(out)
		return strings.Join(out, "、")
	}
	want := names(thirteen)

	up, _, ok := slots.Union(eleven, thirteen)
	if !ok || names(up) != want {
		t.Fatalf("small→big: union = %q ok = %v, want the thirteen-name superset", slots.Render(up), ok)
	}
	down, _, ok := slots.Union(thirteen, eleven)
	if !ok {
		t.Fatal("big→small must resolve too: a subset is a resolution, not a tiebreaker for strength")
	}
	if names(down) != want {
		t.Fatalf("big→small: union = %q, want the base kept whole (a subset adds nothing)", slots.Render(down))
	}

	// Two numbers keep the larger one, in both orders.
	if got, _, ok := slots.Union(slots.Number(13), slots.Number(11)); !ok || got.Count != 13 {
		t.Fatalf("13→11 = %v ok=%v, want the larger count", got, ok)
	}
	if got, _, ok := slots.Union(slots.Number(11), slots.Number(13)); !ok || got.Count != 13 {
		t.Fatalf("11→13 = %v ok=%v, want the larger count", got, ok)
	}

	// A number never outvotes the members it counts: the number is the claim that
	// loses, kept as the dropped value.
	union, dropped, ok := slots.Union(thirteen, slots.Number(11))
	if !ok || union.Kind != slots.KindItems {
		t.Fatalf("members vs count = %v ok=%v, want the members", union, ok)
	}
	if dropped.Count != 11 {
		t.Fatalf("dropped = %v, want the losing count", dropped)
	}

	// Text is not the union's business: a value question merges by strength, exactly
	// as it did before values were typed.
	if _, _, ok := slots.Union(slots.Text("1858"), slots.Text("1849")); ok {
		t.Error("text must not merge by union — that is what keeps a value question on its strength rule")
	}
}

// TestMemberUnionDropsPlaceAndEventPhrases pins the last way a count inflates: a
// piece that CONTAINS a member is not a second member.
//
// A session writing a place beside its owner (`洛阳关孟坦`) or an event beside its
// object (`温酒斩华雄`) writes a token that is short, digit-free and unpunctuated —
// it passes every shape test a name passes — while the member it names is already
// in the table. Measured (2026-09-16): a record enumerating 21 names counted 25,
// the four extra being place-qualified copies of names already listed.
func TestMemberUnionDropsPlaceAndEventPhrases(t *testing.T) {
	table := runtime.NewState([]runtime.Variable{
		typedAnchoredMembersVar(1, "person", "程远志、华雄、管亥、颜良、文丑、杨龄、孔秀、孟坦、韩福、卞喜、王植、秦琪、车胄、成何、蔡阳、吕旷、吕翔、荀正、纪灵、夏侯存、庞德"),
		typedAnchoredMembersVar(2, "person", "孟坦、韩福、卞喜、王植、秦琪、洛阳关孟坦、汜水关卞喜、荥阳王植、黄河渡口秦琪"),
	}, 0, nil)

	union := runtime.ItemValues(&table)
	if len(union) != 21 {
		t.Fatalf("member count = %d, want the 21 names (place-qualified copies are not members): %v", len(union), union)
	}
	for _, extra := range []string{"洛阳关孟坦", "汜水关卞喜", "荥阳王植", "黄河渡口秦琪"} {
		for _, item := range union {
			if item == extra {
				t.Errorf("member union kept %q, a name with its place attached", extra)
			}
		}
	}
	if got := enumeratedSize(table); got != 21 {
		t.Fatalf("enumerated size = %d, want 21", got)
	}
}

// TestLedgerQuoteCarriesTheWordsBehindAMember pins per-member evidence in the
// answer-facing ledger: a name without the words behind it is a member nobody can
// point at, and an answer handed names only cannot cite one passage per member.
func TestLedgerQuoteCarriesTheWordsBehindAMember(t *testing.T) {
	kb := &runtime.Kbinfos{}
	kb.Admit(func(p *runtime.PoolAdmitter) {
		p.Add(map[string]any{"chunk_id": "c1", "content": "关公马快，赶上文丑，脑后一刀，将文丑斩下马来。"})
	})
	kb.RecordReachedTerm("文丑", "c1")

	// The names are in the ledger either way; the words ride the set gate, so a
	// value direction does not pay for them.
	if ledger := probeLedger(kb); strings.Contains(ledger, "斩下马来") {
		t.Fatalf("ledger = %q, want no per-member quotes before a set direction declares itself", ledger)
	}

	kb.MarkSetDirection()
	ledger := probeLedger(kb)
	if !strings.Contains(ledger, "文丑 — “") {
		t.Fatalf("ledger = %q, want the member followed by the words that carry it", ledger)
	}
	if !strings.Contains(ledger, "斩下马来") {
		t.Fatalf("ledger = %q, want the quoted passage itself", ledger)
	}
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

// TestBudgetExtensionIsBoughtOncePerQuestion pins the one-shot budget extension a
// set-shaped pass buys.
//
// The budget fits ONE pass, and an enumeration needs a second one to pick up the
// members a cut first pass never patched (measured 2026-09-16, 三国/关羽: the run
// ended at `ROUND 1 end (unresolved=0)` with the reached-but-unpatched 管亥 gone).
// One extension, not a per-round top-up: the point is a second look, not an
// unbounded run for any table that keeps declaring a set.
func TestBudgetExtensionIsBoughtOncePerQuestion(t *testing.T) {
	st := &AgenticState{Deadline: time.Now().Add(30 * time.Second)}
	before := st.RemainingS()
	if !st.ExtendDeadline(SetBudgetExtensionS) {
		t.Fatal("the first extension must apply")
	}
	after := st.RemainingS()
	if after < before+SetBudgetExtensionS-1 {
		t.Errorf("remaining %.0fs after the extension, want ~%.0fs", after, before+SetBudgetExtensionS)
	}
	if st.ExtendDeadline(SetBudgetExtensionS) {
		t.Error("a second extension must be refused: the budget is bought once per question")
	}
	if got := st.RemainingS(); got > after+1 {
		t.Errorf("remaining %.0fs after the refused extension, want ~%.0f", got, after)
	}
}

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
		{ID: 0, Type: "count", Terms: []string{"斩", "杀"}, Subject: "关羽|云长"},
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
	if got := merged.State[0].Subject; got != "关羽|云长" {
		t.Errorf("merged Subject = %q, want the declared actor", got)
	}
	// The declaration is what the enumeration is built from: a fold that drops it leaves the
	// next round with nothing to enumerate, which is the measured failure above.
	if !runtime.CoverageOf(*merged).Ok() {
		t.Error("the merged table is no longer an enumeration: the declaration was lost in the fold")
	}
}

// coverageStubExec answers tool calls like sessionStubExec and is ALSO a
// runtime.CoverageRunner, so a round's enumeration runs without a retriever.
type coverageStubExec struct {
	mu    sync.Mutex
	calls []runtime.Coverage
}

func (e *coverageStubExec) Execute(_ context.Context, name string, _ map[string]any) (runtime.ToolOutcome, error) {
	return runtime.ToolOutcome{
		Status:      runtime.StatusOK,
		Payload:     []any{map[string]any{"kind": name, "content": "hit"}},
		EvidenceIDs: []string{"c-" + name},
	}, nil
}

// EnumerateCoverage stands in for the corpus: one window stating the deed.
func (e *coverageStubExec) EnumerateCoverage(_ context.Context, cov runtime.Coverage, kb *runtime.Kbinfos) runtime.CoverageSet {
	e.mu.Lock()
	e.calls = append(e.calls, cov)
	e.mu.Unlock()
	quote := "云长手起刀落，斩孔秀于马下"
	if kb != nil {
		kb.Admit(func(p *runtime.PoolAdmitter) {
			p.Add(map[string]any{"chunk_id": "w1", "content_with_weight": quote})
		})
	}
	return runtime.CoverageSet{
		Operands: cov.Operands(),
		Recalled: 1,
		Windows:  []runtime.CoverageWindow{{ChunkID: "w1", Quote: quote, Act: "斩"}},
	}
}

func (e *coverageStubExec) ran() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.calls)
}

// TestRunSlotResearchPassEnumeratesOnce pins the wiring, and the run-once rule.
//
// The act words used to be seeded as a list of queries TO MAKE, and a whole round ran
// without a single one of them being made: measured (2026-09-16, 三国/关羽) 2175 characters
// of patterns in every session's seed and zero `.*` queries in the run's log, with the
// sessions re-probing names by hand in the next round. Now the round asks the corpus
// itself — one call, one recall per operand — admits what comes back, and seeds it.
func TestRunSlotResearchPassEnumeratesOnce(t *testing.T) {
	table := func() runtime.State {
		return runtime.NewState([]runtime.Variable{
			{ID: 0, Type: "count", Terms: []string{"斩", "杀"}, Subject: "关羽|云长"},
			{ID: 1, Type: "dataset", QuestionClues: []string{"who did he kill?"}},
		}, 0, nil)
	}
	exec := &coverageStubExec{}
	kb := &runtime.Kbinfos{}
	st := &AgenticState{Question: "关羽杀了多少有姓名的人物？", KB: kb, SlotTable: table()}
	deps := runtime.SessionDeps{
		Model: &scriptedModel{replies: []string{"I could not find any evidence about that."}},
		Tools: &runtime.Toolset{Exec: exec},
		KB:    kb,
	}
	RunSlotResearchPass(context.Background(), context.Background(), deps, st.Question, st, 60)

	if got := exec.ran(); got != 1 {
		t.Fatalf("ran %d enumeration(s), want exactly one per question", got)
	}
	if got := exec.calls[0].Operands(); len(got) != 4 {
		t.Errorf("operands = %v, want one entry per actor form and act word", got)
	}
	set, done := kb.CoverageSet()
	if !done || len(set.Windows) != 1 || set.Windows[0].ChunkID != "w1" {
		t.Fatalf("stored set = %+v (done=%v), want the window the enumeration found", set, done)
	}
	if !strings.Contains(set.Render(), "斩孔秀于马下") {
		t.Errorf("seed = %q, want the window with the chunk id a member is cited by", set.Render())
	}
	if kb.PoolSize() == 0 {
		t.Error("the enumeration's window never reached the pool: the win cannot be cited")
	}

	// A second round REUSES the set: the windows are in the pool under the same ids, so
	// asking the corpus the same operand queries again spends the store legs for nothing.
	second := &AgenticState{Question: st.Question, KB: kb, SlotTable: table()}
	RunSlotResearchPass(context.Background(), context.Background(), deps, second.Question, second, 60)
	if got := exec.ran(); got != 1 {
		t.Errorf("second round ran the enumeration again (%d), want the stored set reused", got)
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

	added := FanoutSearch(ctx, deps, st, []string{"what is the Culdcept tower height"}, 8, 60, true)

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

	if added := FanoutSearch(ctx, deps, st, []string{"what is the Culdcept tower height"}, 8, 60, false); added != 0 {
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

		added := FanoutSearch(ctx, deps, st, []string{"what is the Culdcept tower height"}, 8, 60, true)
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
// resolver records the filter, and the retriever records the scoped search.
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
					"query":   []any{"who wrote it?"},
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
					"query":   []any{"who wrote it?"},
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
	if mdl.calls < 3 {
		t.Errorf("model calls = %d, want the session to have kept running to its own empty patch", mdl.calls)
	}
	if resolver.calls != 0 {
		t.Errorf("push-down calls = %d, want none: an unknown key is rejected before the index is touched", resolver.calls)
	}
}
