//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

// Package agentic_rag is the outer agentic-search loop (medium / high / ultra).
//
// This file is the five-phase pipeline that sits ABOVE the action session:
//
//	formalize_question → [planner → prefetch] → rag_agent → draft → sca
//	    ├─ sufficient ──────────────────────────→ formalize_answer
//	    └─ insufficient → query_rewrite ────────→ rag_agent (next round)
//
// The graph is Eino's compose.NewGraph, compiled in Pregel mode (the research loop is a
// cycle: sca → query_rewrite → rag_agent). Node bodies and routing predicates follow the
// five phases above. Node-visit accounting stays in this file rather than the framework's:
// a research round costs three node visits, whereas Eino counts run steps.
//
// The run configuration and entry points live in agentic_rag.go, and the leaf primitives
// in runtime/.
package agentic_rag

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"ragflow/internal/agent/chat"
	"ragflow/internal/common"
	"ragflow/internal/rag/agentic-rag/runtime"
	"ragflow/internal/rag/agentic-rag/runtime/orchestrator"
	"ragflow/internal/rag/prompts"
)

var _LOG = common.StdLogger()

// The sufficiency VERDICT is gone with the reviewer that produced it. It was three-valued
// (SUFFICIENT / INSUFFICIENT / UNKNOWN) because "the review could not run" is not a judgement
// about the evidence — and the loop needed that distinction to avoid marking an answer partial
// on no evidence at all (measured 2026-09-15). With no review, no verdict exists to be wrong:
// the round either wrote an answer or it did not, and what it read is a number in its record.

// Local text helpers shared by the nodes below.

var tokenPattern = regexp.MustCompile(`[A-Za-z0-9_]+`)

// ViewTerms: terms describing what the SCA
// should look FOR this round.
func ViewTerms(st *AgenticState) []string {
	if st == nil {
		return nil
	}
	terms := queryToTerms(st.Question)
	for _, q := range st.CurrentQueries {
		terms = append(terms, queryToTerms(q)...)
	}
	return dedupe(terms)
}

// RemainingS: seconds left in the global budget.
// Never negative.
func (s *AgenticState) RemainingS() float64 {
	if s.Deadline.IsZero() {
		return TotalBudgetS
	}
	d := time.Until(s.Deadline).Seconds()
	if d < 0 {
		return 0
	}
	return d
}

// The budget extension used to live here: ExtendDeadline bought ONE extra slice of the
// question's clock, and only an "enumeration table" could ask for it (see the note in
// policy.go). It is gone — there is one clock per question, the caller sets it, and nothing
// inside a run widens it. Widening it was how a shape went from "a rule that decides what to
// do" to "a rule that also decides how long everything else has".
// ctxRoomS reports how many seconds a context still has, and whether it is bounded
// at all: an unbounded caller owns no deadline, so nothing here can overrun it.
func ctxRoomS(ctx context.Context) (float64, bool) {
	if ctx == nil {
		return 0, false
	}
	dl, ok := ctx.Deadline()
	if !ok {
		return 0, false
	}
	return time.Until(dl).Seconds(), true
}

// ctxLeftS is ctxRoomS for a log line: seconds left, zero when unbounded.
func ctxLeftS(ctx context.Context) float64 {
	room, _ := ctxRoomS(ctx)
	return room
}

// AgenticState — the outer loop's mutable state.
//
// Deliberately absent: an earlier design declared `fills_found`, `research_feedback` and
// a `verdict` dict carrying `missing_claims` / `hard_violations` / `agent_confidence` /
// `feedback` keys, but nothing ever writes them — the "focus" injected from them would
// always be empty. Carrying them here would add dead state for no behaviour. Verify
// against the WRITE site, not the declaration, before re-adding any of them.

type AgenticState struct {
	// ── conversation input ──
	// Messages is the conversation history the formalize_question node reads to
	// resolve pronouns and ellipses.
	Messages []schema.Message
	Question string
	Keywords string

	// ── evolving research state ──
	Plan            []string // planner fan-outs (Phase 1)
	CurrentQueries  []string // active research targets
	SlotTable       runtime.State
	CollectedAnswer string // the answer the round's SESSION wrote ("" when it wrote none)
	UnresolvedSlots []map[string]any
	SlotEvidence    map[string]SlotEvidence
	KB              *runtime.Kbinfos
	PartialAnswer   bool
	Abstain         bool
	EmptyResult     bool

	// ── budgets & counters ──
	MaxLoops     int
	Deadline     time.Time // wall-clock expiry of the research budget
	SearchRounds int       // completed research rounds
	Attempted    []map[string]any
	NoProgress   bool
	// LastRoundNew is how many chunks the last research round ADDED to the pool.
	//
	// It is the loop's one non-subjective signal about whether to keep going: a
	// round that is still adding evidence is still learning, whatever the
	// reviewer thinks of the passages it holds (see routeSCA). Zero means the
	// round learned nothing, which is what NoProgress already means.
	LastRoundNew int
}

// NewAgenticState builds the initial state. It does NOT arm the global budget: the
// budget is armed by the formalize_question node's return, so formalization is not charged
// to it.
//
// maxLoops caps the research rounds.
func NewAgenticState(question, keywords string, maxLoops int, messages []schema.Message) *AgenticState {
	if maxLoops <= 0 {
		maxLoops = 3
	}
	return &AgenticState{
		Question:    question,
		Keywords:    keywords,
		MaxLoops:    maxLoops,
		Messages:    messages,
		KB:          &runtime.Kbinfos{},
		EmptyResult: true,
	}
}

// Slot-table depth/size limits.
const (
	maxSlotDepth  = 3
	maxSlotsTotal = 8
)

// fanoutPrompt, fanoutStrictRetry, the shape guards (fanoutMaxWords / fanoutMaxChars /
// fanoutLooseMaxWords) and fanoutAnswerMarks are gone with the opening decomposition stage: they
// existed to ask for sub-questions and to police what came back. The slot table's own call asks
// for the queries directly (see plannerNode), so there is no "did this line look like a query"
// question left for code to answer.

// Fanout search tuning .
const (
	// fanoutBM25TopN is the keyword-leg candidate pool.
	fanoutBM25TopN = 60
	// fanoutHybridTopN is the semantic-leg candidate pool.
	fanoutHybridTopN = 30
	// fanoutSemanticQuota caps narrow-BYPASS hits admitted per fan-out.
	fanoutSemanticQuota = 4
	// evidenceTopUp caps how many of the chunks cited by the channel-0 claim rows to pull
	// in verbatim. A directed fetch by id, not another recall.
	evidenceTopUp = 8
	// Narrowing budget for the exact leg.
	fanoutNarrowMaxOutPerChunk = 1200
	fanoutNarrowMaxOutTotal    = 16000
)

// BuildLowGraph: the lightweight
// low-mode path — formalize → direct_search → answer — with no planner, no
// fan-out, and no SCA loop.
//
// The low graph declares the same two nodes on Eino's compose.NewGraph and invokes the
// compiled runnable.
//
// A low-graph failure is reported the same way as an agentic one: both graphs are wrapped
// by the same error path.
func BuildLowGraph(ctx context.Context, deps RAGTools, req runtime.RunRequest, sd runtime.SearchDeps, kb *runtime.Kbinfos, resp *RunResponse, logger *log.Logger) error {
	// The graph is declared per run: deps / sd / resp are request-scoped and Eino
	// captures them in closures, so the graph must be declared per run rather than shared.
	g := compose.NewGraph[*runtime.RunRequest, *runtime.RunRequest]()

	var buildErr error
	addNode := func(name string, fn func(context.Context, *runtime.RunRequest) (*runtime.RunRequest, error)) {
		if buildErr != nil {
			return
		}
		buildErr = g.AddLambdaNode(name, compose.InvokableLambda(fn))
	}
	addEdge := func(from, to string) {
		if buildErr != nil {
			return
		}
		buildErr = g.AddEdge(from, to)
	}

	// Node 1: formalize_question — rewrites req.Question/Keywords in place.
	addNode("formalize_question", func(c context.Context, r *runtime.RunRequest) (*runtime.RunRequest, error) {
		formalizeQuestion(c, deps, r, logger)
		return r, nil
	})
	// Node 2: direct_search (direct_search_node).
	addNode("direct_search", func(c context.Context, r *runtime.RunRequest) (*runtime.RunRequest, error) {
		// The direct search runs with compiled expansion ON unconditionally. The RunRequest
		// field only gates the L1 direct retrieve elsewhere, so it must not be able to turn
		// expansion off here.
		low := *r
		low.UseCompiled = true
		runDirect(c, deps, low, sd, kb, resp, logger)
		return r, nil
	})
	// Node 3: formalize_answer — the last node, so the answer is produced inside the graph.
	addNode("formalize_answer", func(c context.Context, r *runtime.RunRequest) (*runtime.RunRequest, error) {
		if deps.Finalize != nil {
			// The state at this node: partial_answer=False (formalize_question) and
			// empty_result=True (direct search) — the values the compose prompt reads from the
			// state. The question
			// is the one formalize_question wrote into this same RunRequest
			// the low
			// graph's compose must use the formalized question too.
			deps.Finalize(c, false, true, r.Question)
		}
		return r, nil
	})

	addEdge(compose.START, "formalize_question")
	addEdge("formalize_question", "direct_search")
	addEdge("direct_search", "formalize_answer")
	addEdge("formalize_answer", compose.END)

	if buildErr != nil {
		logger.Printf("[Low RAG] graph build failed: %v", buildErr)
		return buildErr
	}

	runnable, err := g.Compile(ctx, compose.WithGraphName("low_graph"))
	if err != nil {
		logger.Printf("[Low RAG] graph compile failed: %v", err)
		return err
	}
	if _, err := runnable.Invoke(ctx, &req); err != nil {
		logger.Printf("[Low RAG] graph run failed: %v", err)
		return err
	}
	return nil
}

// Routing

type agenticNode int

// The nodes a ROUTE can name. The graph's entry, the planner and the prefetch are not
// among them: they are edges of the graph (see BuildAgenticGraph), never the target of a
// routing decision, and a node no route can return is a case no switch can reach.
const (
	nodeFormalizeAnswer agenticNode = iota
	nodeQueryRewrite
	nodeRagAgentLoop
)

// plannerNode mirrors the `planner` node: decompose into fan-outs,
// then build the slot table from them.
func plannerNode(ctx context.Context, deps RAGTools, st *AgenticState, logger *log.Logger) {
	ctx, done := runtime.Phase(ctx, "planner")
	defer done()

	// ONE call produces both the queries and the fact slots.
	//
	// This used to be two: `ExpandFanouts` asked a model for "candidate aspects", and the
	// slot-table call then asked a model — the same model, on the same question — for the slots
	// AND for `first_queries`, which are the very queries the first call had just produced.
	// Measured 2026-09-20: the planner phase cost 2 calls per question and was the second-biggest
	// output-token consumer of the run (50 calls / 50,450 output tokens over 25 questions). The
	// paper's opening move is ONE call — `first_move`: decompose, then retrieve the queries it
	// produced — and the slot table's own call already produces them.
	//
	// The `-15.0` is the reservation the old planner made, not a cap: once the budget is spent
	// the value goes NEGATIVE and InitializeState uses it as-is, timing out immediately, so an
	// exhausted round cannot spend 15 more seconds on a decomposition it cannot afford.
	sd := deps.sessionDeps()
	root, firstQueries := BuildSlotTable(ctx, sd, st.Question, nil, st.RemainingS()-15.0)
	st.SlotTable = root

	// The plan IS what the round will search: the table's own queries, and the slots' clues when
	// it produced none (a table built from the fallback path carries clues without queries). It
	// used to be the fan-out model's wording, which prefetch then overrode whenever the table
	// produced queries of its own — so the two log lines described different searches.
	var plan []string
	for _, q := range firstQueries {
		if q = strings.TrimSpace(q); q != "" {
			plan = append(plan, q)
		}
	}
	plan = dedupe(plan)
	if len(plan) == 0 {
		plan = planFromSlots(root)
	}
	st.Plan = plan
	st.CurrentQueries = append([]string(nil), plan...)
	runtime.StepsFrom(ctx).StageLine(logger, "Planner", fanoutSummary(st.Question, plan))
}

// planFromSlots is the plan of last resort: the slots' own question clues, in slot order.
func planFromSlots(root runtime.State) []string {
	var out []string
	for _, v := range root.State {
		for _, c := range v.QuestionClues {
			if c = strings.TrimSpace(c); c != "" {
				out = append(out, c)
			}
		}
	}
	return dedupe(out)
}

// prefetchNode mirrors the `prefetch` node: programmatic fan-out
// retrieval into the snippet pool.
func prefetchNode(ctx context.Context, deps RAGTools, st *AgenticState, logger *log.Logger) {
	ctx, done := runtime.Phase(ctx, "orchestrator")
	defer done()

	queries := st.CurrentQueries
	if len(queries) == 0 {
		if st.Question == "" {
			return
		}
		queries = []string{st.Question}
	}
	timeout := nodeClock(PrefetchTimeoutS, 10.0, st.RemainingS()-MinRoundHeadroomS)
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout*float64(time.Second)))
	defer cancel()

	// The opening line brackets the leg lines below: FanoutSearch reports each leg
	// under its own tag ("[BM25 search]", "[Hybrid search]") and its ONLY other
	// caller — the query rewriter, further down this file — produces lines that
	// look identical, so without this step a reader cannot tell which block the
	// searches belong to. What is searched is said here; what came back is said
	// after, and neither is repeated.
	step(ctx, logger, "Prefetch", "%s", prefetchSummary(len(st.Plan), len(queries)))
	// The legs report one level deeper: they are what this prefetch runs, not
	// sibling steps of it.
	added := FanoutSearch(runtime.Nested(callCtx), deps, st, queries, FanoutTopN)
	for _, q := range queries {
		st.Attempted = append(st.Attempted, map[string]any{"q": q, "r": 0, "new": added})
	}
	// The upfront search is a phase of its own (Python surfaces it as
	// "[Preliminary search]"): without this step the trace jumps from the plan
	// straight to round 1, and the passages the pool already held look like they
	// came from nowhere.
	step(ctx, logger, "Prefetch", "Added %s to the evidence pool.", runtime.CountOf(added, "new passage"))
}

// The evidence-prefill summary used to live here: it promised the number of research
// sessions a round would open (min(open slots, slotSessionsPerRound)) and said how many of
// the plan's slots the pooled evidence already answered. It is gone with the session fan-out
// — a round opens ONE session, so there is no number to promise — and with the slot table,
// which was the only thing that could say how many slots were "answered".

// prefetchSummary renders what the upfront search is about to cover.
//
// The plan's decomposition and the queries prefetch runs are DIFFERENT things, and
// in a high/ultra trace they print one after the other: the queries are the slot
// table's own first_queries, which the slot-table model writes fresh (the plan's
// sub-questions are only a HINT in that call, so it may reword them — a plan entry
// "曹操 生平简介" comes back as the query "曹操 简介"). initialize_state caps them at
// three; the rest stay on the plan as slots for later rounds. Naming the plan's
// total here is what keeps "split into 5 sub-questions" followed by "searching 3
// opening queries" from reading like two sub-questions were dropped.
//
// Calling those queries "the plan's sub-questions" claimed a subset relation that
// does not hold: the two lines then contradicted each other, since the legs below
// search the model's wording and not the plan's.
func prefetchSummary(planned, searching int) string {
	if planned <= searching {
		return fmt.Sprintf("Searching %s up front.",
			runtime.CountOf(searching, "opening query"))
	}
	return fmt.Sprintf("The plan lists %d sub-questions; searching %s up front.",
		planned, runtime.CountOf(searching, "opening query"))
}

// fanoutSummary renders the planner's decomposition: the sub-questions the
// question was split into.
//
// The step used to announce the decomposition without ever showing it
// ("Decomposing the question into first-hop fan-outs."), which left "first-hop
// fan-out" an internal phrase — the sub-questions themselves only surfaced later,
// mixed into the prefetch legs as search queries. expandFanouts falls back to the
// question itself when the model will not decompose, and the sentence says so
// rather than presenting that fallback as a sub-question.
//
// "first-hop" is gone from the wording as well: it is fan-out jargon (a reader has
// no "hop" to count) and these are simply the questions the plan set out to
// research.
func fanoutSummary(question string, fanouts []string) string {
	switch {
	case len(fanouts) == 0:
		return "Could not decompose the question; searching it as asked."
	case len(fanouts) == 1 && fanouts[0] == question:
		return "The question is already a single searchable sub-question; searching it as asked."
	}
	quoted := make([]string, 0, len(fanouts))
	for _, f := range fanouts {
		quoted = append(quoted, fmt.Sprintf("%q", f))
	}
	return fmt.Sprintf("Split the question into %s to research: %s.",
		runtime.CountOf(len(fanouts), "sub-question"), strings.Join(quoted, ", "))
}

// ragRoundEndLine renders one research round's outcome: what the round added,
// what the pool holds now, and what is still open.
//
// The last clause is phrased per count — "0 slots still unresolved" reads as a
// double negative, so the zero case says it outright.
//
// It counts SLOTS, not the plan's sub-questions. The two are different numbers in
// the same block (a 5-sub-question plan builds a 6-slot table), so saying
// "sub-questions" here made a reader reconcile 5, 3 and 2 as one series — and read
// a slot backlog as plan coverage.
func ragRoundEndLine(roundNo, newPassages, pool, unresolved int) string {
	clause := "and no unresolved slots"
	if unresolved > 0 {
		clause = "and " + runtime.CountOf(unresolved, "slot") + " still unresolved"
	}
	return fmt.Sprintf("Round %d finished: %s added; the pool now holds %s, %s.",
		roundNo, runtime.CountOf(newPassages, "new passage"), runtime.CountOf(pool, "passage"), clause)
}

// ragAgentNode mirrors the `rag_agent` node: one slot research pass.
func ragAgentNode(ctx context.Context, deps RAGTools, st *AgenticState, logger *log.Logger) {
	ctx, done := runtime.Phase(ctx, "dynamic")
	defer done()

	timeLeft := st.RemainingS()
	if timeLeft < MinRoundHeadroomS {
		step(ctx, logger, "RAGAgent", "Only %.0f seconds of the research budget are left, so no further research pass will run.", timeLeft)
		return
	}
	roundNo := st.SearchRounds + 1
	poolBefore := len(st.KB.Chunks)
	// The round number already says how many passes preceded it (roundNo is
	// st.SearchRounds+1), so the counter is not repeated here.
	step(ctx, logger, "RAGAgent", "Round %d begins with %s in the evidence pool; %.0f seconds of research budget left.",
		roundNo, runtime.CountOf(poolBefore, "passage"), timeLeft)

	t := max(20.0, nodeClock(PassTimeoutS, 0, timeLeft-25.0))
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(t*float64(time.Second)))
	defer cancel()

	res := RunSlotResearchPass(callCtx, ctx, deps.sessionDeps(), st.Question, st, t)
	if res == nil {
		// "Nothing to do" is still a ROUND, and the round's growth fact belongs to
		// this round, not the previous one: leaving LastRoundNew alone made the
		// routing read a stale `grew` (measured: round 2 reported round 1's +40
		// chunks and asked for a third round on evidence it had not gathered).
		if st.KB != nil {
			st.LastRoundNew = len(st.KB.Chunks) - poolBefore
		}
		return
	}
	st.SlotTable = res.SlotTable
	st.CollectedAnswer = res.CollectedAnswer
	st.UnresolvedSlots = res.UnresolvedSlots
	st.SlotEvidence = res.SlotEvidence
	// The SCA-facing draft used to travel out of the round here (SlotDraft → RagAnswer) and the
	// previous round's was kept when the new one was empty. Neither exists any more: the round's
	// record is what it found, and the ANSWER is written by the session that read the passages.
	// The ANSWER prompt reads this one, not the draft: a draft carries the SCA's
	// machine fields, and handing them over as a "summary" put the runtime's
	// bookkeeping into the answer (see RenderSlotRecord).
	if res.SlotRecord != "" && st.KB != nil {
		st.KB.Record = res.SlotRecord
	}
	st.Attempted = res.Attempted
	// The session IS the answerer (see the design's R2): it read the evidence, so the answer it
	// wrote — and the registry its [ID:n] markers index into — are recorded on the pool, where
	// the terminal composition (which runs outside the graph) can read them.
	if st.KB != nil {
		if ans := strings.TrimSpace(st.CollectedAnswer); ans != "" {
			st.KB.SessionAnswer = ans
			st.KB.SessionEvidenceRefs = res.EvidenceRefs
			// The fallback composition reads PreSummary as "the research findings". The round
			// used to render a slot draft for the reviewer to read; with no reviewer, the
			// findings ARE the answer the session wrote.
			st.KB.PreSummary = ans
		}
	}

	// The round's growth is the loop's continuation fact (see routeSCA): stored
	// on the state rather than only printed, so the routing decision reads the
	// same number the log shows.
	st.LastRoundNew = len(st.KB.Chunks) - poolBefore
	runtime.StepsFrom(ctx).StageLine(logger, "RAGAgent",
		ragRoundEndLine(roundNo, st.LastRoundNew, len(st.KB.Chunks), len(res.UnresolvedSlots)))
}

// The draft node used to live here: it rendered an intermediate answer for the reviewer to
// judge (and, on a round that reported nothing, synthesized one from snippets). It existed only
// because the SCA needed something to read; with no reviewer there is no draft — the round's
// session writes the answer itself (see the note on routeResearch), and a round that ends
// without one leaves the evidence pool for the closing composition to use.

// queryRewriteNode mirrors the `query_rewrite` node: Phase-4
// targeted gap pursuit.
func queryRewriteNode(ctx context.Context, deps RAGTools, st *AgenticState, logger *log.Logger) {
	ctx, done := runtime.Phase(ctx, "rewrite")
	defer done()

	// The gaps come from the round's OWN record, not from a reviewer: the router only sends a
	// round here when it found work left (unresolved slots) and evidence still arriving, and
	// `unresolvedClueGaps` is that record turned into retrieval directions.
	gaps := unresolvedClueGaps(st)
	if len(gaps) == 0 {
		step(ctx, logger, "QueryRewriter", "The evidence check came back insufficient but named no concrete gap; accepting the draft.")
		st.NoProgress = true
		return
	}

	// Information-augmented rewriting: give the rewriter FULL VISIBILITY — what
	// was tried (with outcomes), what the evidence pool holds — so it aims at
	// uncovered angles itself, instead of rule-based dedupe.
	researchContext := renderResearchContext(st)

	t := nodeClock(RewriteTimeoutS, 10.0, st.RemainingS()-10.0)
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(t*float64(time.Second)))
	defer cancel()

	rewritten := orchestrator.RewriteGapToQuery(callCtx, orchestrator.RewriteDeps{
		Model:           &jsonModelAdapter{inner: deps.Model, maxLength: deps.MaxLength},
		Prompts:         deps.RewritePrompts,
		ResearchContext: researchContext,
	}, st.Question, gaps)

	var queries []string
	for _, q := range rewritten {
		if s := strings.TrimSpace(q["query"]); s != "" {
			queries = append(queries, s)
		}
	}
	// Fold the still-unresolved slots into the rewrite queries so the next slot
	// research pass targets exactly those unknowns. This makes an insufficient
	// verdict drive slot completion, not a blind re-search.
	for _, us := range st.UnresolvedSlots {
		if clues, ok := us["question_clues"].([]string); ok {
			for i, qc := range clues {
				if i >= 2 {
					break
				}
				if s := strings.TrimSpace(qc); s != "" {
					queries = append(queries, s)
				}
			}
		}
	}
	queries = dedupe(queries)
	if len(queries) == 0 {
		// The rewriter is a MODEL and it can decline, which used to end the round: the routing above
		// had already judged another round worth its budget (gaps exist, the review view changed),
		// and one empty reply cancelled that judgement. On a set question the other fallback
		// (unresolved slots' clues) is empty too, because its unknowns are names nobody has
		// proposed yet. So the round is kept alive with queries taken from the run's own
		// bookkeeping instead of the model.
		//
		// The SCA's gaps are NOT usable here: a gap derived from a draft carries the draft's TEXT
		// (MissingPiece.What is the whole paragraph), which as a retrieval query is noise.
		queries = fallbackQueries(st)
		if len(queries) > 0 {
			step(ctx, logger, "QueryRewriter",
				"The rewriter produced no query; re-asking the run's own open terms (%d) instead.", len(queries))
		}
	}
	if len(queries) == 0 {
		step(ctx, logger, "QueryRewriter", "No actionable query was produced; accepting the draft.")
		st.NoProgress = true
		return
	}

	// Dual-track pursuit of the gap: (a) programmatically pre-fetch new snippets
	// into the SCA pool; (b) the next slot research pass picks these up via the
	// persisted slot_table + unresolved_slots.
	// Room is measured against the EVIDENCE POOL's ceiling, the same number the
	// admitter enforces. Sized against the smaller snippet-pool constant, a rich
	// round got room <= 0 and admitted nothing, so the round reported "retrieval
	// saturated" and discarded itself while the pool still had room to take the
	// evidence it had just asked for.
	added := FanoutSearch(callCtx, deps, st, queries, FanoutTopNRewrite)
	// Retrieval saturation early-exit: a rewrite round that produced ZERO new
	// snippets means further full research passes just burn latency.
	if added == 0 && st.SearchRounds >= 1 {
		step(ctx, logger, "QueryRewriter", "Retrieval saturated: another insufficient round added no new passages, so iteration stops.")
		st.NoProgress = true
		st.CurrentQueries = queries
		return
	}

	// DECOMPOSE: promote SCA gaps to new
	// slots so the next research pass gets a typed unknown with its own
	// action session — that is how the plan actually expands. No extra LLM
	// call: the SCA already told us what is missing (missing_fact + hint).
	//
	// ORDER: promotion runs AFTER the rewrite LLM call, the
	// empty-query early return (:1326-1328) and the saturation early-exit
	// (:1337-1339) — both of which DISCARD the promotion by returning before it.
	// Promoting before the rewrite (the earlier Go port's order) mutated the slot
	// table even on rounds that were then dropped, desyncing the table from the
	// queries actually pursued.
	if st.SlotTable.Depth < maxSlotDepth {
		slots := st.SlotTable.State
		known := map[string]bool{}
		nextID := 0
		for _, v := range slots {
			for _, c := range v.QuestionClues {
				known[strings.ToLower(strings.TrimSpace(c))] = true
			}
			if v.ID >= nextID {
				nextID = v.ID + 1
			}
		}
		promoted := 0
		for _, g := range gaps {
			if len(slots) >= maxSlotsTotal {
				break
			}
			key := strings.ToLower(strings.TrimSpace(g.What))
			if key == "" || known[key] {
				continue
			}
			clues := []string{truncateRunes(g.What, 200)}
			if strings.TrimSpace(g.SearchHint) != "" {
				clues = append(clues, truncateRunes(g.SearchHint, 200))
			}
			slots = append(slots, runtime.Variable{ID: nextID, Type: "entity", QuestionClues: clues})
			known[key] = true
			nextID++
			promoted++
		}
		if promoted > 0 {
			st.SlotTable.State = slots
			step(ctx, logger, "QueryRewriter", "Promoted %s to research slots (depth %d).",
				runtime.CountOf(promoted, "gap"), st.SlotTable.Depth)
		}
	}

	step(ctx, logger, "QueryRewriter", "Round %d came back insufficient; rewriting it into %s: %v.",
		st.SearchRounds+1, runtime.CountOf(len(queries), "targeted query"), queries)
	st.NoProgress = false
	st.CurrentQueries = queries
	st.SearchRounds++
	for _, q := range queries {
		st.Attempted = append(st.Attempted, map[string]any{"q": q, "r": st.SearchRounds, "new": added})
	}
}

// fallbackQueries is what the run can still ask WITHOUT a model: the terms it probed and got
// nothing back for — a fact about the QUERY, which is what the probe ledger is for — and the
// direction's own subject forms.
func fallbackQueries(st *AgenticState) []string {
	var out []string
	if st.KB != nil {
		out = append(out, st.KB.ProbedAbsentTerms()...)
	}
	out = append(out, runtime.ActorForms(st.SlotTable)...)
	kept := make([]string, 0, len(out))
	for _, q := range out {
		if q = strings.TrimSpace(q); q != "" {
			kept = append(kept, q)
		}
	}
	return dedupe(kept)
}

// chunkCount is the pool size, nil-safe: a graph that failed before a pool existed still reports its
// [Finalize] step.
func chunkCount(kb *runtime.Kbinfos) int {
	if kb == nil {
		return 0
	}
	return len(kb.Chunks)
}

// finalizeSummary renders the finalize step's state as a sentence. The verdict comes first and the
// evidence it is built from last, so the line that closes the research phase says both: this is the
// last trace line before the answer is written, and the compose step no longer repeats the count.
func finalizeSummary(partial, empty bool, chunks int) string {
	switch {
	case partial && empty:
		return "Finalizing a partial answer with no supporting evidence: research exhausted its attempts."
	case empty:
		return "Finalizing with no supporting evidence: the answer will say so."
	case partial:
		return fmt.Sprintf("Finalizing a partial answer from %s; some gaps remain unanswered.",
			runtime.CountOf(chunks, "passage"))
	}
	return fmt.Sprintf("Finalizing a complete answer from %s.", runtime.CountOf(chunks, "passage"))
}

// onOff renders a boolean switch as the word the run's opening line uses ("self-check on").
func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// formalizeAnswerNode mirrors the `formalize_answer` node's state mutation
// (query_rewrite). The answer composition itself (Phase 5 synthesis) is out of
// scope here — it needs the report prompt templates; the caller reads the
// approved draft from st.KB.PreSummary.
// researchStatusNote is the body of the "[Research status]" note the outer tool loop folds into
// its answer: the round's OWN record, or "" when the round answered.
//
// It replaces the reviewer's verdict text. The verdict carried a judgement ("insufficient") plus
// the reviewer's view of what was missing; a judgement is exactly what this design removes from
// the answer path, so the note states facts instead — what the round read, and what the plan
// still lists as unresolved. Both are numbers a reader can check against the run's own log.
func researchStatusNote(st *AgenticState) string {
	if strings.TrimSpace(st.CollectedAnswer) != "" {
		return ""
	}
	parts := make([]string, 0, 2)
	if st.KB != nil && len(st.KB.Chunks) > 0 {
		parts = append(parts, runtime.CountOf(len(st.KB.Chunks), "passage")+" read")
	}
	if n := len(st.UnresolvedSlots); n > 0 {
		parts = append(parts, fmt.Sprintf("%d plan slot(s) still unresolved", n))
	}
	if len(parts) == 0 {
		return ""
	}
	return "this round did not settle the question (" + strings.Join(parts, ", ") + ")"
}

func formalizeAnswerNode(ctx context.Context, deps RAGTools, st *AgenticState, logger *log.Logger) {
	// The enumeration's own last node used to run here — a window-by-window judge whose
	// verdicts were written back as members. It is gone with the coverage engine, and what
	// replaced it is the same evidence reached from the other side: the session that did the
	// reading is the one that writes the answer, so the members it found are already in what
	// it wrote (see the design's R1/R2 and the note in kbinfos.go).

	// "Partial" is a statement about the EVIDENCE, decided by facts: unresolved slots are the
	// table's own list of what it could not fill, and NoProgress means the last round learned
	// nothing. A reviewer's verdict used to be the third input, and it made the flag unreliable
	// in BOTH directions — a review that could not run (VerdictUnknown) shipped "partial answer,
	// some gaps remain" on no evidence at all (2026-09-15: the SCA had timed out), while a
	// satisfied reviewer said nothing about whether the record was finished.
	if st.NoProgress || len(st.UnresolvedSlots) > 0 {
		// All research attempts exhausted without a satisfying context — surface
		// the residual findings honestly instead of refusing.
		st.PartialAnswer = true
	}
	st.EmptyResult = len(st.KB.Chunks) == 0
	step(ctx, logger, "Finalize", "%s", finalizeSummary(st.PartialAnswer, st.EmptyResult, chunkCount(st.KB)))
	// formalize_answer — the node itself composes and streams the answer
	// (_compose_answer_from_evidence); it does not just flag the state.
	// Composition uses THIS node's state values: partial_answer was set right above and
	// empty_result is still the True formalize_question wrote (never reset anywhere in the
	// graph), so the compose prompt carries the no-evidence hedge on every round and, when
	// INSUFFICIENT, the partial preamble. Forwarding the state beats the caller's response
	// flags, which are only copied after the graph returns.
	if deps.Finalize != nil {
		// question = state["question"] (Python :834): the FORMALIZED question
		// the formalize_question node wrote — composing from the outer tool
		// argument instead collapses the final answer to the first completed
		// sub-answer of a multi-hop question.
		//
		// Marked first: this node has just told the reader the verdict and the
		// evidence, so the compose that follows suppresses its own kickoff step.
		deps.Finalize(markFinalizeAnnounced(ctx), st.PartialAnswer, true, st.Question)
	}
}

// SCA helpers: view selection, the terms a view is scored on, gaps→rewrite, and the sca
// node's claim construction.

// genJSONMaxRetry: the first call, plus one corrective round that feeds the malformed
// answer and the parse error back to the model.
//
// Deliberately NO reply cache: the contract is "the same prompt still calls the LLM",
// which also rules out replaying a verdict computed against evidence a later round has
// already superseded.
const genJSONMaxRetry = 2

// genJSONTailFenceRE matches a trailing ``` fence followed by any newlines, the
// "```\n*$" alternative of gen_json's cleanup regex.
var genJSONTailFenceRE = regexp.MustCompile("```\\n*$")

// GenJSON implements orchestrator.JSONModel.
func (a *jsonModelAdapter) GenJSON(ctx context.Context, prompt string) (any, error) {
	if a.inner == nil {
		return nil, fmt.Errorf("agentic: no model configured")
	}
	// No reply cache: see the genJSONMaxRetry note — every call reaches the model.
	const userPrompt = "Output:\n"
	// The prompt is fitted ONCE, before the retry loop: msg[0] becomes the system turn and
	// msg[1:] the history, with the corrective text appended to the LAST user turn WITHOUT
	// re-fitting. A zero/negative max_length is normalised to 8192 by
	// chat.EffectiveContextLength.
	baseUser := userPrompt
	systemTurn := prompt
	if fitted, _ := chat.FitMessages("", []schema.Message{
		*schema.SystemMessage(prompt),
		*schema.UserMessage(baseUser),
	}, a.maxLength); len(fitted) > 0 {
		if fitted[0].Role == schema.System {
			systemTurn = fitted[0].Content
		}
		if last := fitted[len(fitted)-1]; last.Role == schema.User {
			baseUser = last.Content
		}
	}
	var lastAns, errText string
	for attempt := 0; attempt < genJSONMaxRetry; attempt++ {
		userTurn := baseUser
		if attempt > 0 && lastAns != "" && errText != "" {
			// gen_json appends the corrective prompt to the user turn only once
			// the previous round produced both an answer and a parse error.
			userTurn = baseUser + fmt.Sprintf("\nGenerated JSON is as following:\n%s\nBut exception while loading:\n%s\nPlease reconsider and correct it.", lastAns, errText)
		}
		reply, err := a.inner.Complete(ctx, []schema.Message{
			*schema.SystemMessage(systemTurn),
			*schema.UserMessage(userTurn),
		}, nil)
		if err != nil {
			// gen_json propagates chat/transport errors immediately; only JSON
			// parsing failures are retried. Mirror that.
			return nil, err
		}
		cleaned := stripGenJSONWrappers(reply.Content)
		// The corrective prompt re-sends the CLEANED answer: the next round's "Generated
		// JSON is as following:" carries the stripped text, not the raw reply with its fences
		// / think block.
		lastAns = cleaned
		if v, ok := parseGenJSONReply(cleaned); ok {
			return v, nil
		}
		// Deliberately log length, not content: the reply may embed retrieved
		// material (PII), so the raw body is excluded, as in jsonchat.GenJSON.
		errText = fmt.Sprintf("model reply (%d bytes) is not parseable JSON", len(cleaned))
	}
	return nil, fmt.Errorf("agentic: no parseable JSON in the model output after %d attempts", genJSONMaxRetry)
}

// MemberWindowMax bounds how many confirmed-member passages the rewrite context
// carries, and MemberWindowChars how much of each.
const (
	MemberWindowMax   = 12
	MemberWindowChars = 220
)

// routeResearch decides whether the run takes another research round — the ONE router, used
// after every research round (see the two branches in the graph builder).
//
// It replaces the SCA verdict as the thing that decides. The paper's loop stops when the agent
// ANSWERS, and that is the first check here: the session reads the evidence and writes the
// answer, so once it has written one the run is over. Everything below is the fallback for a
// round that ended without an answer: the round's own RECORD says what is still open, and the
// round count and the clock bound how long the run may keep asking.
//
// The facts it reads are records, not judgements:
//
//	answered       — the session wrote an <answer>. Stop.
//	work left      — the plan still has unresolved slots, or the review named gaps.
//	still learning — the round added evidence (+N chunks). An empty unresolved list is NOT
//	                 proof of completeness (2026-09-15: unresolved=0 while the answer was six
//	                 members short), and a satisfied reviewer was a judgement about the passages
//	                 it read rather than about whether the record was finished.
//	stalled        — NoProgress: the same evidence twice, no actionable query, or a rewrite that
//	                 retrieved nothing new.
//
// So: answered ⇒ stop; nothing left AND nothing learned ⇒ stop; work left AND still learning ⇒
// another round; otherwise the round count and the clock decide.
func routeResearch(st *AgenticState, maxRounds int) agenticNode {
	if ans := strings.TrimSpace(st.CollectedAnswer); ans != "" {
		_LOG.Printf("[Routing] closing out: the session wrote an answer (%d rune(s)).", utf8.RuneCountInString(ans))
		return nodeFormalizeAnswer
	}
	if st.NoProgress {
		_LOG.Printf("[Routing] closing out: the round made no progress (unresolved=%d, +%d chunks).",
			len(st.UnresolvedSlots), st.LastRoundNew)
		return nodeFormalizeAnswer
	}
	gaps := unresolvedClueGaps(st)
	work := len(gaps) > 0 || len(st.UnresolvedSlots) > 0
	grew := st.LastRoundNew > 0
	if !work || !grew {
		_LOG.Printf("[Routing] closing out: no round is asked for (unresolved=%d, gaps=%d, +%d chunks this round).",
			len(st.UnresolvedSlots), len(gaps), st.LastRoundNew)
		return nodeFormalizeAnswer
	}
	if st.SearchRounds >= maxRounds {
		_LOG.Printf("[Routing] work remains (unresolved=%d, gaps=%d, +%d chunks this round) but the round budget is spent (%d/%d); closing out.",
			len(st.UnresolvedSlots), len(gaps), st.LastRoundNew, st.SearchRounds, maxRounds)
		return nodeFormalizeAnswer
	}
	if st.RemainingS() <= MinRoundHeadroomS {
		_LOG.Printf("[Routing] work remains (unresolved=%d, gaps=%d, +%d chunks this round) but only %.0fs left (need %.0fs); closing out.",
			len(st.UnresolvedSlots), len(gaps), st.LastRoundNew, st.RemainingS(), MinRoundHeadroomS)
		return nodeFormalizeAnswer
	}
	_LOG.Printf("[Routing] another round: unresolved=%d, gaps=%d, +%d chunks this round, rounds=%d/%d, %.0fs left.",
		len(st.UnresolvedSlots), len(gaps), st.LastRoundNew, st.SearchRounds, maxRounds, st.RemainingS())
	return nodeQueryRewrite
}

// composedRecord is the record block the ANSWER prompt carries: the slot record
// plus the probe ledger.
//
// The prompt builder and the log both call it, because they disagreed once: the
// log reported `record_len=225` (the slots alone) while the prompt carried the
// ledger too, so the one line meant to show what the answer was given could not
// answer the only question asked of it.
func composedRecord(kb *runtime.Kbinfos) string {
	if kb == nil {
		return ""
	}
	record := strings.TrimSpace(kb.Record)
	if ledger := probeLedger(kb); ledger != "" {
		if record != "" {
			record += "\n"
		}
		record += ledger
	}
	if kb.SufficiencyUnchecked() {
		// The review that judges completeness never produced a verdict (see graph_sca), so the count
		// is what the evidence supports rather than a checked total.
		record += "- NOTE: the sufficiency review could not be completed for this question, so nobody checked whether the members above are complete. State the count as what the evidence supports and do not present it as exhaustive."
	}
	return record
}

// ledgerNonNameMax bounds the "NOT names" line: enough that the run's probing stays visible on
// the record, few enough that the terms nobody may use do not crowd out the ones they may.
const ledgerNonNameMax = 10

// probeNameRunes bounds what a probed term can be if it is to pass as a name. The corpus's names
// are short (华雄, 程远志, 太史慈) and a longer term is a phrase a session built rather than a name
// it found (荥阳太守王植, 令左右推出斩之).
const probeNameRunes = 5

// probeLedgerTerms splits the reached terms into the ones that can be a name and the ones that
// cannot: an act word names the deed, so does a term carrying one, a term with a separator, a space
// or too many runes is a query, and the actor's own forms are not elements of what he did. What is
// name-shaped is left to judgement, which is what the section is for.
func probeLedgerTerms(kb *runtime.Kbinfos, reached []runtime.ReachedTerm) (named []runtime.ReachedTerm, others []string) {
	// The planner's declaration (the deed's act words, the actor's own forms) is gone with the
	// coverage engine, so the filter keeps the SHAPE test alone — length, separators, an embedded
	// space — which is the half that never guessed anything about the question.
	var acts, actors []string
	for _, rt := range reached {
		if probeNameShaped(rt.Term, acts, actors) {
			named = append(named, rt)
			continue
		}
		others = append(others, rt.Term)
	}
	return named, others
}

// probeNameShaped reports whether a probed term is shaped like a name (see probeLedgerTerms).
func probeNameShaped(term string, acts, actors []string) bool {
	t := strings.TrimSpace(term)
	if t == "" || utf8.RuneCountInString(t) > probeNameRunes {
		return false
	}
	for _, r := range t {
		if unicode.IsSpace(r) || unicode.IsDigit(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			return false
		}
	}
	lowered := strings.ToLower(t)
	// An act word is rejected even when it is one rune and only contained: a name never carries the
	// verb of the deed.
	for _, act := range acts {
		if act = strings.ToLower(strings.TrimSpace(act)); act != "" && strings.Contains(lowered, act) {
			return false
		}
	}
	// The actor's forms go through the same predicate as the member set (runtime.IsActorForm).
	return !runtime.IsActorForm(t, actors)
}

// probeLedger renders the names the research PROBED — the terms the model itself
// asked the corpus about, with what came back.
//
// It exists because the slots are the model's own bookkeeping and the ledger is
// the framework's: a run can probe twenty-four terms, get a passage for each, and
// record eleven of them in a slot, and then the answer is composed from the
// eleven while the other thirteen exist only inside the pool. Measured
// (fixrecall2, 2026-09-15): `members=0 reached=24` at the last turn, eleven names
// in the final record, and 车胄 / 管亥 both proven present by the run's own probes
// (`carry: 车胄(7) 管亥(3)`) yet absent from the answer — which meanwhile anchored
// on the model's own PRIOR, delivered through a slot nobody could verify.
//
// The wording keeps the two lists apart on purpose. "Reached" is a fact about the
// corpus; "belongs in the answer" is a judgement the answer still has to make.
// And a term that was probed with nothing back is a fact about the QUERY — the
// distinction the whole loop depends on, since reading it as absence is what
// stops an enumeration short.
func probeLedger(kb *runtime.Kbinfos) string {
	if kb == nil {
		return ""
	}
	var b strings.Builder
	if reached := kb.ReachedTerms(); len(reached) > 0 {
		// Per-member quotes are for a SET answer, which is the only answer that has
		// to point at a passage per item. On a value direction they would be prompt
		// tokens bought for nothing, so the quotes ride the same shape gate as the
		// rest of the set machinery (the direction declares itself once, see
		// Kbinfos.MarkSetDirection). The names stay in both cases: they are what the
		// ledger has always been.
		quoted := kb.IsSetDirection()
		named, others := probeLedgerTerms(kb, reached)
		terms := make([]string, 0, len(named))
		for _, rt := range named {
			// The name AND the words that prove it. Without the words the answer has
			// a list of names and no way to point at a passage for any one of them,
			// which is what a set answer needs to carry per member: measured
			// (2026-09-16) an answer that was handed names only reported "21 listed,
			// four counted but not listed" and cited one evidence RANGE for all of
			// them, instead of one citation per member.
			if quoted && len(terms) < ledgerQuoteMembers {
				if quote := ledgerQuote(kb, rt.ChunkID, rt.Term); quote != "" {
					terms = append(terms, fmt.Sprintf("%s — %s", rt.Term, quote))
					continue
				}
			}
			terms = append(terms, rt.Term)
		}
		if len(terms) > 0 {
			fmt.Fprintf(&b, "Probed and answered (a passage came back for each of these, so they OCCUR in the corpus; whether each belongs in the answer is still your judgement — any name here that the record above does not mention is a finding nobody recorded; the words behind each name are its evidence, and a member without words is a member nobody can point at): %s",
				strings.Join(terms, "；"))
		}
		if len(others) > 0 {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			if len(others) > ledgerNonNameMax {
				others = append(others[:ledgerNonNameMax], "…")
			}
			fmt.Fprintf(&b, "Probed, NOT names (the act words and query-shaped terms this run also asked about — listed so its own probing is visible on the record; do NOT list them as members): %s",
				strings.Join(others, "；"))
		}
	}
	if absent := kb.ProbedAbsentTerms(); len(absent) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "Probed with NOTHING back (a fact about the query, not about the corpus — re-word it or change the angle; this is not \"absent\"): %s",
			strings.Join(absent, "、"))
	}
	return b.String()
}

// ledgerQuoteRunes bounds one member's quoted evidence, and ledgerQuoteMembers
// how many members carry a quote. Both are bounded because the ledger is part of
// the ANSWER prompt: a couple of dozen members at a few dozen runes each is the
// most a record may spend on evidence before it crowds out the question.
const (
	ledgerQuoteRunes   = 40
	ledgerQuoteMembers = 24
)

// ledgerQuote is the words behind one reached term: a short window of the pool
// chunk that carries it, in the reading order of that chunk.
//
// The ledger names the members; this is what lets an answer show, member by
// member, the passage each one rests on — and lets a reader see which member has
// no words behind it at all.
func ledgerQuote(kb *runtime.Kbinfos, chunkID, term string) string {
	c := kb.ChunkByID(chunkID)
	if c == nil {
		return ""
	}
	text := runtime.ChunkTextOf(c)
	if text == "" {
		return ""
	}
	runes := []rune(text)
	start := 0
	if at := strings.Index(text, term); at > 0 {
		if r := utf8.RuneCountInString(text[:at]) - ledgerQuoteRunes/3; r > 0 {
			start = r
		}
	}
	end := min(start+ledgerQuoteRunes, len(runes))
	if end-start < ledgerQuoteRunes && end == len(runes) {
		start = max(0, end-ledgerQuoteRunes)
	}
	q := strings.TrimSpace(strings.ReplaceAll(string(runes[start:end]), "\n", " "))
	if q == "" {
		return ""
	}
	return "“" + q + "”"
}

// recordSource names which block the answer prompt carried, so a log reader can
// tell "the model had a slot record" from "the model had a prose summary" without
// inferring it from lengths.
func recordSource(record string) string {
	if strings.TrimSpace(record) == "" {
		return "prose-summary"
	}
	return "slot-record"
}

// recordContract labels the answer prompt's slot-record block.
//
// The line exists because "Research Summary (primary evidence)" described a
// runtime record as evidence: the model then treated its lines as findings and
// copied them. A record is not evidence — it is what the research settled — and
// the prompt has to say so, or the labels end up in the answer.
//
// "the value IS the answer" is the other half, and it is measured. A record that is
// only a check is a record the answer may drop when the passages do not repeat it,
// and the RUN's passages often do not: they were gathered to FIND the value, so the
// session that found it recorded it while the chunk that carried it never entered
// the cited evidence. Measured (2026-09-16, FRAMES, mode high): the slot table held
// `slot 0 [person]: Colin Beashel and Richard Coxon (Australia, Star class 1984
// Olympics)` — the answer — and the composed answer was "not found in the knowledge
// base", because the passages around it were about other competitions and the chat
// configuration requires that sentence when the information is unavailable (see
// web/src/locales/zh.ts).
const recordContract = "Research Record (INTERNAL — your own slot table plus the terms your probes " +
	"reached; NEVER quote these lines, their `slot N [type]` labels, or any id into the answer). " +
	"The slots are what the research SETTLED: when a slot holds a value for what the question asks, " +
	"that value IS the answer — state it, and do not report that the answer was not found while a " +
	"slot holds one; a passage that merely fails to repeat the value does not contradict it. If the " +
	"evidence plainly contradicts a settled value, answer what the evidence supports and say so. " +
	"A name listed as probed-and-answered is one the corpus was asked about and produced: if the answer " +
	"is a list or a count and that name is not in it, say why."

// literalList renders a string slice as the protocol's list of quoted terms (['a', 'b']):
// the draft text is prompt content the SCA reads, and Go's fmt.Sprint form ([a b]) does not
// read as a list of terms.
func literalList(in []string) string {
	quoted := make([]string, 0, len(in))
	for _, s := range in {
		quoted = append(quoted, "'"+s+"'")
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// Slot-table research: the executor behind the outer loop's `rag_agent` node.
//
// Covers the slot-table build, the research pass, patch merging and draft rendering.
//
// One research round = one action session per unresolved slot, run concurrently
// under a semaphore, with the resulting branches folded back into the shared
// table.

// SlotEvidence records which passages produced a slot's candidate, so the SCA
// can verify the candidate against the passages that actually produced it.
type SlotEvidence struct {
	EvidenceIDs  []string
	TerminalType string
	Candidate    string
}

// SlotResearchResult is one research round's output.
type SlotResearchResult struct {
	SlotTable       runtime.State
	CollectedAnswer string
	UnresolvedSlots []map[string]any
	SlotEvidence    map[string]SlotEvidence
	// EvidenceRefs is the session's evidence registry in first-seen order: the chunk ids the
	// model was shown as [ID:0], [ID:1], … (see SessionState.EvidenceRefs). The answer the
	// session wrote cites THESE numbers, so a caller that lets that answer stand must pass this
	// list on as the citation list.
	EvidenceRefs []string
	// SlotRecord is the same table rendered for the ANSWER prompt: the facts the
	// research settled, without the machine fields the SCA needs (see
	// RenderSlotRecord). The two are produced together so they cannot drift.
	SlotRecord string
	Attempted  []map[string]any
}

// buildSlotTableFrom wraps InitializeState, converting its non-error result into
// an error so BuildSlotTable can apply the single fallback path.
func buildSlotTableFrom(ctx context.Context, deps runtime.SessionDeps, question string, fanouts []string, deadlineLeft float64) (runtime.State, []string, error) {
	if deps.Model == nil {
		return runtime.State{}, nil, fmt.Errorf("no model configured")
	}
	init := runtime.InitializeState(ctx, deps, question, fanouts, deadlineLeft)
	if len(init.Root.State) == 0 {
		// Covers both an empty reply and a reply the strict parser rejected (non-integer id
		// / non-iterable clues).
		return runtime.State{}, nil, fmt.Errorf("decomposition failed")
	}
	return init.Root, init.FirstQueries, nil
}

// ── Evidence-guided batched answer generation (APT-RAG) ─────────────────────

// evidenceChunkPrefix: （agentic_rag_graph.py:537）:
// only the claim pseudo-chunks are atomic evidence rows.
const evidenceChunkPrefix = "claim_"

// Evidence-guided batching constants.
//
// The threshold is deliberately conservative: APT-RAG ships sim_threshold=0.0
// (any overlap), which merges weakly related slots and, worse, lets a single
// parse failure blank every answer in the batch at once. Both conditions must
// hold. The absolute floor stops two tiny evidence sets from merging on a
// single coincidental hit; the ratio gate stays LOW enough to actually fire —
// measured FRAMES overlap was 0.03-0.2 mean, so the old 0.5 never triggered at
// all. (APT-RAG ships 0.0; that is too loose for us because one parse failure
// would blank every answer in the batch.)
const (
	EvidenceBatchMinSim    = 0.15
	EvidenceBatchMinShared = 2
	EvidenceBatchMaxSlots  = 4
	EvidenceBatchMaxChars  = 12000
)

// EvidenceBatchPrompt
// verbatim.
const EvidenceBatchPrompt = "Answer each listed sub-question using ONLY the shared evidence below.\n" +
	"These sub-questions share this evidence, so read it once and answer all of them.\n" +
	"Return JSON only, mapping each sub-question id to its answer: " +
	`{"<id>": "<answer>", ...}. Use an empty string when the evidence does not ` +
	"answer that sub-question. Never invent facts."

// intersectionSize counts the shared members of two evidence-id sets.
func intersectionSize(a, b map[string]bool) int {
	n := 0
	for id := range a {
		if b[id] {
			n++
		}
	}
	return n
}

// Word-level coverage a pooled evidence row must reach before it is allowed to
// answer a slot on its own. Deliberately strict: a wrong prefill costs
// accuracy, while a missed prefill only costs one session (which still runs).
// Strict on purpose: a wrong prefill costs accuracy, a missed one costs a session.
const EvidencePrefillCoverage = 0.6

// The literal helpers this file's prompts render with (retrieval.formatLiteral /
// retrieval.quoteLiteral) live in the runtime package: initialize_state needs the same
// syntax to read the model's reply, so there is one implementation.

func strengthOf(v runtime.Variable) float64 {
	if v.CandidateStrength == nil {
		return 0.0
	}
	return *v.CandidateStrength
}

// Fallback-draft synthesis tuning.
const (
	// draftChunkCap / draftChunkChars: the evidence budget.
	draftChunkCap   = 16
	draftChunkChars = 1200
	// draftFallbackChars: raw-evidence fallback when no model is available or the
	// call fails.
	draftFallbackChars = 4000
	// draftMaxChars caps the composed draft.
	draftMaxChars = 6000
)

// containsNonASCII: a question carrying non-ASCII characters is answered in its own
// language.
func containsNonASCII(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII {
			return true
		}
	}
	return false
}

// BuildAgenticGraph drives the agentic-search loop and returns the terminal state.
// Declares and compiles the graph, then invokes the runnable.
//
// The caller reads the answer from st.KB.PreSummary / st.CollectedAnswer and composes it
// (see Run for the single-shot entry point).
//
// The returned error mirrors run_agentic_rag's holder["error"] — the graph itself
// failed. The state is still returned so the caller can surface whatever the run
// produced; an empty result alone is not a failure.
//
// Activation is EXPLICIT: call this directly, with no init()-based auto-registration.
// This is the agentic planner (medium/high/ultra); the mode dispatch lives in
// RAGTools.Run, and agentic modes register this as the AgenticLoop.
func BuildAgenticGraph(ctx context.Context, deps RAGTools, question, keywords string, maxLoops int, messages []schema.Message) (*AgenticState, error) {
	logger := deps.logger()
	// formalize_question — the graph starts at formalize_question, which both
	// initializes the state and arms the budget, so the budget is NOT set here.
	st := NewAgenticState(question, keywords, maxLoops, messages)
	if deps.KB != nil {
		st.KB = deps.KB
	}
	spec := runtime.ResolveMode(deps.Tools)
	maxRounds := spec.MaxRounds

	step(ctx, logger, "Agentic RAG", "Starting research in %s mode (%s).",
		spec.Label, runtime.CountOf(maxRounds, "follow-up round"))

	// run_agentic_rag — there is NO whole-graph wall clock. Research stays
	// bounded by the per-node timeouts (bounded / PassTimeoutS / SCATimeoutS…),
	// the routing guards (MinRoundHeadroomS) and the visit limit; and because
	// formalize_answer now composes inside the graph, the answer stream must be allowed to
	// run until the model finishes. Capping the whole graph at TotalBudgetS+30s used to cut
	// the composition short.

	// run_agentic_rag — LangGraph counts NODE VISITS, not loop iterations: one
	// research round is rag_agent → draft → sca = three visits. Counting run
	// steps instead would let Go do roughly 3x the work before the guard trips,
	// so the counter is kept here and each node adds the visits it costs.
	limit := graphRecursionLimit(true, maxLoops)
	visits := 0

	// The graph is declared per run: deps / logger / spec are request-scoped and
	// Eino captures them in closures, so a compiled graph cannot be shared
	// across requests. The graph is 7 nodes; compilation is validation plus
	// channel wiring.
	g := compose.NewGraph[*AgenticState, *AgenticState]()
	var buildErr error
	visit := func(n int) { visits += n }

	addNode := func(name string, fn func(context.Context, *AgenticState) (*AgenticState, error)) {
		if buildErr != nil {
			return
		}
		buildErr = g.AddLambdaNode(name, compose.InvokableLambda(fn))
	}
	addEdge := func(from, to string) {
		if buildErr != nil {
			return
		}
		buildErr = g.AddEdge(from, to)
	}
	addBranch := func(from string, cond func(context.Context, *AgenticState) (string, error), ends map[string]bool) {
		if buildErr != nil {
			return
		}
		buildErr = g.AddBranch(from, compose.NewGraphBranch(cond, ends))
	}
	// guard routes to "stop" once the visit budget is spent. The guards ABORT the run
	// rather than running formalize_answer, so the stop node reports the failure instead of
	// composing an answer from whatever partial research the round had gathered.
	guard := func(next string) string {
		if visits >= limit {
			return "stop"
		}
		return next
	}

	addNode("formalize_question", func(ctx context.Context, s *AgenticState) (*AgenticState, error) {
		visit(1)
		formalizeQuestionNode(ctx, deps, s, logger)
		return s, nil
	})
	addNode("planner", func(ctx context.Context, s *AgenticState) (*AgenticState, error) {
		visit(1)
		plannerNode(ctx, deps, s, logger)
		return s, nil
	})
	addNode("prefetch", func(ctx context.Context, s *AgenticState) (*AgenticState, error) {
		visit(1)
		prefetchNode(ctx, deps, s, logger)
		return s, nil
	})
	// rag_agent is one research ROUND: one session reads the evidence and writes its answer.
	// Both the first pass and the rewrite-driven passes enter this node. It used to be three
	// nodes — rag_agent → draft → sca — where the draft existed only to be reviewed and the
	// review existed only to judge the draft (see the note on routeResearch).
	addNode("rag_agent", func(ctx context.Context, s *AgenticState) (*AgenticState, error) {
		visit(agenticRoundVisits)
		ragAgentNode(ctx, deps, s, logger)
		return s, nil
	})
	addNode("query_rewrite", func(ctx context.Context, s *AgenticState) (*AgenticState, error) {
		visit(1)
		queryRewriteNode(ctx, deps, s, logger)
		return s, nil
	})
	addNode("formalize_answer", func(c context.Context, s *AgenticState) (*AgenticState, error) {
		visit(1)
		formalizeAnswerNode(c, deps, s, logger)
		return s, nil
	})
	// stop is the visit-budget backstop and has no graceful path: exhausting the budget is
	// recorded as a graph failure and, when the run also produced nothing, surfaces as
	// graphFailureFallback. Returning the state as-is would compose a partial answer where
	// the internal error belongs, so the error is raised here and the caller drops the
	// state (see below).
	addNode("stop", func(_ context.Context, s *AgenticState) (*AgenticState, error) {
		logger.Printf("[Agentic RAG] stopping after %d node visits (limit=%d)", visits, limit)
		return s, fmt.Errorf("graph recursion limit reached after %d node visits", visits)
	})

	// START → formalize_question.
	addEdge(compose.START, "formalize_question")
	addEdge("prefetch", "rag_agent")
	addEdge("formalize_answer", compose.END)
	addEdge("stop", compose.END)

	// Every mode walks the same graph: formalize_question → planner → prefetch → rag_agent →
	// … The planner used to be skipped (and prefetch with it) for the modes whose spec said
	// `UseFanout: false`, so the QUESTION's own structure — which mode it happened to run
	// under — decided whether the plan step existed at all. A mode now differs only in how
	// much it may spend (see runtime/config.go): same nodes, same edges, same semantics.
	addBranch("formalize_question", func(_ context.Context, _ *AgenticState) (string, error) {
		return guard("planner"), nil
	}, map[string]bool{"stop": true, "planner": true})

	addBranch("planner", func(_ context.Context, _ *AgenticState) (string, error) {
		return guard("prefetch"), nil
	}, map[string]bool{"stop": true, "prefetch": true})

	addBranch("prefetch", func(_ context.Context, _ *AgenticState) (string, error) {
		return guard("rag_agent"), nil
	}, map[string]bool{"stop": true, "rag_agent": true})

	// ONE router after both research nodes: a round either ends the run (the session answered)
	// or asks for another one, and there is no separate reviewer whose verdict could ask for it
	// instead. The round count is the mode's, and nothing about the QUESTION shortens it: it
	// used to be cut to two rounds for a table the planner had typed as a set, and that bound
	// cannot be told from a slot type.
	addBranch("rag_agent", func(_ context.Context, s *AgenticState) (string, error) {
		return guard(agenticNodeName(routeResearch(s, maxRounds))), nil
	}, map[string]bool{"stop": true, "query_rewrite": true, "formalize_answer": true})

	addBranch("query_rewrite", func(_ context.Context, s *AgenticState) (string, error) {
		return guard(agenticNodeName(routeResearch(s, maxRounds))), nil
	}, map[string]bool{"stop": true, "rag_agent": true, "formalize_answer": true})

	if buildErr != nil {
		logger.Printf("[Agentic RAG] graph build failed: %v", buildErr)
		return st, buildErr
	}

	runnable, err := g.Compile(ctx,
		compose.WithGraphName("agentic_rag"),
		// The visit counter above is the authoritative guard; this is only a backstop so a
		// mis-wired cycle cannot spin.
		compose.WithMaxRunSteps(limit*2+16),
	)
	if err != nil {
		logger.Printf("[Agentic RAG] graph compile failed: %v", err)
		return st, err
	}
	out, err := runnable.Invoke(ctx, st)
	if err != nil {
		logger.Printf("[Agentic RAG] graph run failed: %v", err)
		// A failed run keeps NO state: an empty state is handed back so everything
		// downstream sees nothing. Returning the mutated state would surface slots/draft the
		// caller must discard.
		return &AgenticState{}, err
	}
	if out != nil {
		return out, nil
	}
	return st, nil
}

// agenticNodeName maps the routing predicates' node ids onto the Eino graph's
// node keys.
func agenticNodeName(n agenticNode) string {
	switch n {
	case nodeQueryRewrite:
		return "query_rewrite"
	case nodeRagAgentLoop:
		return "rag_agent"
	default:
		return "formalize_answer"
	}
}

// Explicit wiring into the RAGTools.Run loop registration.
//
// There is NO init()-based auto-registration: the caller constructs the object and invokes
// it, activating the full medium/high/ultra pipeline by explicitly registering this
// loop:
//
//	agentic_rag.SetAgenticLoop(agentic_rag.NewAgenticLoop())
//
// Without registration, RAGTools.Run falls back to a single action session for
// agentic modes (see Run).

// NewAgenticLoop returns the outer agentic loop adapter for RAGTools.Run. It
// converts the RAGTools run config into the planner's state and maps the result
// back onto the
// RunResponse.
func NewAgenticLoop() AgenticLoop {
	return func(ctx context.Context, deps RAGTools, req runtime.RunRequest, kb *runtime.Kbinfos, resp *RunResponse, logger *log.Logger) {
		if logger == nil {
			logger = _LOG
		}
		if deps.Model == nil {
			logger.Printf("[Agentic RAG] no model configured for mode %q; degrading to a direct search", resp.Mode.Label)
			runDirectFallback(ctx, deps, req, kb, logger)
			return
		}

		// The loop needs an executor for its programmatic fan-out fetches, built from the same
		// retrieval backend the single-session path uses. It must project the FULL RAGTools
		// config, otherwise fan-out and action-session retrieval silently lose tuning —
		// notably the tag boost (Tagger/KBs).
		sd := searchDepsFor(ctx, deps, req, req.DatasetIDs, req.TenantID, kb, logger)
		sd.WebSearch = deps.WebSearch
		sd.CiteRules = deps.CiteRules

		toolset := &runtime.Toolset{
			ThinkingMode: resp.Mode.Label,
			// web_search is visible only when the mode exposes it AND a provider is actually
			// wired. Advertising it without a provider leaves the model calling a tool that can
			// only return an infra error.
			HasWebSearch:  resp.Mode.HasTool("web_search") && deps.WebSearch != nil,
			DisabledTools: map[string]bool{},
			Exec:          runtime.NewSearchExecutor(sd, req),
		}

		st, runErr := BuildAgenticGraph(ctx, RAGTools{
			Tools:          toolset,
			Search:         sd, // dual-channel fan-out retrieves directly
			Model:          deps.Model,
			ModelName:      deps.ModelName,
			Prompts:        deps.Prompts,
			KB:             kb,
			SCAPrompts:     deps.Prompts,
			RewritePrompts: deps.Prompts,
			MaxLength:      deps.MaxLength,
			Logger:         logger,
			// The terminal composition is the formalize_answer node body; it must reach the
			// graph even though this deps copy is rebuilt here.
			Finalize: deps.Finalize,
			// Steps is deliberately not copied here: Rag already bound the caller's
			// reporter on ctx, and every stage and tool reads it from there
			// (runtime.StepsFrom), so a second copy on this rebuilt RAGTools would be
			// dead weight that could drift.
		}, req.Question, req.Keywords, 3, deps.Messages)

		// A graph exception is recorded separately from "research found nothing". The state is
		// still surfaced below: the internal-error message is only swapped in when the failed
		// run ALSO produced nothing.
		if runErr != nil {
			resp.GraphFailed = true
		}

		// Surface the loop's outcome on the RunResponse.
		resp.Slots = append(resp.Slots, st.SlotTable.State...)
		// Slot citations: the chat pipeline's citation decoration rewrites
		// leaked "[ID:Slot N]" markers with these evidence chunk ids. Plain
		// ids (not positions) so the mapping survives the caller's later
		// evidence-pool narrowing (selectEvidence).
		if len(st.SlotEvidence) > 0 {
			sc := make(map[string][]string, len(st.SlotEvidence))
			for sid, ev := range st.SlotEvidence {
				sc[sid] = ev.EvidenceIDs
			}
			resp.SlotCitations = sc
		}
		// Research findings: the SCA-reviewed draft. NOTE: this is the research
		// draft, NOT the final answer — RAGTools.Run composes the final cited
		// answer afterwards from KB.PreSummary (see composeFinalAnswer).
		if st.CollectedAnswer != "" {
			resp.CollectedAnswer = st.CollectedAnswer
		}
		resp.Partial = st.PartialAnswer
		resp.SearchRounds = st.SearchRounds
		// SCAFeedback is the body of the "[Research status]" note that rag() folds into the
		// answer when the round did NOT answer. It is the round's own record (what it read, what
		// the plan still lists as unresolved) rather than a reviewer's verdict. Rag() appends the
		// trailing "STOP" vs "call rag again" sentence based on the consecutive-unanswerable count.
		resp.SCAFeedback = researchStatusNote(st)
		// Update the consecutive-unanswerable guardrail on the shared per-turn *RAGCache.
		// Rag() builds deps.Cache before the outer react branch, so this counter accumulates
		// across the outer loop's multiple rag() calls within a single turn.
		//
		// A round that ANSWERED resets the counter, one that did not bumps it, and the update is
		// locked because those calls run concurrently.
		deps.Cache.NoteUnanswerable(strings.TrimSpace(st.CollectedAnswer) != "")
	}
}

// graphRecursionLimit: the graph aborts after this many node visits: 60 for the agentic
// graph, else max(25, max_loops*8). Eino counts run steps, not node visits, so the graph
// keeps its own visit counter and checks it in every branch.
func graphRecursionLimit(agentic bool, maxLoops int) int {
	if agentic {
		return AgenticRecursionLimit
	}
	if n := maxLoops * 8; n > lowRecursionLimitBase {
		return n
	}
	return lowRecursionLimitBase
}

// formalizeStepLine renders the formalize phase's user-visible step, or "" when
// there is nothing to report at all.
//
// The phase used to announce `Formalized the question into %q` unconditionally,
// which reads as a rewrite even when Formalize returned the question VERBATIM —
// the common case: a single-turn run skips the rewrite entirely, and the
// multi-turn prompt asks for the question unchanged "in most cases".
//
// Both outcomes are reported, because either way a reader wants to see the
// question this turn was actually researched under: unchanged is "kept the
// question as asked", a rewrite gets the pair — seeing the asked question NEXT TO
// the searched one is what lets a reader check that a pronoun or an ellipsis was
// resolved the way they meant. Only a formalize that produced no question at all
// stays silent. No trailing period: the sentence ends on the quoted question,
// which carries its own full-width "？".
func formalizeStepLine(asAsked, standalone string) string {
	asAsked = strings.TrimSpace(asAsked)
	standalone = strings.TrimSpace(standalone)
	if standalone == "" {
		return ""
	}
	if standalone == asAsked {
		if asAsked == "" {
			return ""
		}
		return fmt.Sprintf("Kept the question as asked: %q", trunc(asAsked, 80))
	}
	if asAsked == "" {
		return fmt.Sprintf("Standalone question for this turn: %q", trunc(standalone, 80))
	}
	return fmt.Sprintf("Rewrote the follow-up into a standalone question: %q → %q",
		trunc(asAsked, 80), trunc(standalone, 80))
}

// formalizeQuestionNode is the graph's first node. It resolves pronouns and ellipses from
// the conversation into a standalone question plus search keywords, and arms the global
// budget in its return — so the formalization work itself is NOT charged to that budget.
//
// Single-turn input costs no LLM call (Formalize returns early).
func formalizeQuestionNode(ctx context.Context, deps RAGTools, st *AgenticState, logger *log.Logger) {
	// formalize_question — the budget is armed as the node RETURNS, i.e. after any
	// formalization work has already happened (Python stamps the deadline in this
	// node's return dict, agentic_rag_graph.py:1061). Arming on ENTRY instead
	// charged the formalization call — a full rewrite on multi-turn, a keyword
	// extraction on single-turn — to the research budget, so every downstream
	// timeout and the MinRoundHeadroomS guard saw a budget short by one LLM call.
	//
	// Deferred so that every return path arming it stays impossible to forget:
	// Python's node has a single return (agentic_rag_graph.py:1041-1062, the
	// deadline at :1061), so it arms the budget unconditionally — even for an empty
	// conversation — and a Go early exit must not become the one path that leaves
	// the graph with no deadline at all.
	defer func() {
		st.Deadline = time.Now().Add(time.Duration(TotalBudgetS * float64(time.Second)))
	}()

	q, kw := formalizeConversation(ctx, deps, st.Messages)
	if q != "" {
		st.Question = q
	}
	if kw != "" && st.Keywords == "" {
		st.Keywords = kw
	}
	if logger != nil {
		asAsked, _ := transcriptOf(st.Messages)
		if line := formalizeStepLine(asAsked, q); line != "" {
			runtime.StepsFrom(ctx).StageLine(logger, "Formalize", line)
		}
	}
}

// formalizeQuestion resolves pronouns and ellipses from the conversation into a standalone
// question plus search keywords — the same work the agentic graph's first node does.
// Single-turn input costs no LLM call (Formalize returns early), and the graph budget is
// NOT started here: the deadline is armed in that node's return, so this cost is not
// charged to it. The low graph has no scheduler, so it runs as a plain step of
// BuildLowGraph rather than as a scheduled node; the naive path has no such step.
func formalizeQuestion(ctx context.Context, deps RAGTools, req *runtime.RunRequest, logger *log.Logger) {
	// The guard is repeated here (it is also inside formalizeConversation) because the
	// STATS record belongs to "there was something to formalize": a history-less call
	// spends no call and must not count as one.
	if len(deps.Messages) == 0 || deps.Model == nil {
		return
	}
	if deps.Stats != nil {
		deps.Stats.RecordCall("formalize")
	}
	q, kw := formalizeConversation(ctx, deps, deps.Messages)
	if deps.Stats != nil && q == "" {
		deps.Stats.RecordFailed("formalize")
	}
	if q != "" {
		req.Question = q
	}
	if kw != "" && req.Keywords == "" {
		req.Keywords = kw
	}
	if logger != nil {
		asAsked, _ := transcriptOf(deps.Messages)
		if line := formalizeStepLine(asAsked, q); line != "" {
			runtime.StepsFrom(ctx).StageLine(logger, "Formalize", line)
		}
	}
}

// formalizeConversation is the step the two formalize_question entry points share (the
// graph node and the low-graph pass): the conversation resolved into a standalone question
// plus search keywords. Both empty when there is nothing to formalize — no model, or no
// history, which the single-turn fast path returns on without a call.
func formalizeConversation(ctx context.Context, deps RAGTools, messages []schema.Message) (string, string) {
	if len(messages) == 0 || deps.Model == nil {
		return "", ""
	}
	return Formalize(ctx, runtime.SessionDeps{Model: deps.Model, Prompts: deps.Prompts}, messages, deps.MaxLength)
}

// RunAgenticRAG: the mode
// dispatch plus execution of the selected pipeline. It is the counterpart of
// RAGTools.rag (Go: Run), which owns the conversation-level concerns around it — cache
// lookup, the effective question, attachments and storing the result.
//
// The dispatch is run_agentic_rag:
//
//	mode is NAIVE      → _naive_rag            (plain retrieval)
//	mode.Agentic       → build_agentic_graph   (medium / high / ultra)
//	else               → build_low_graph       (low: formalize → direct_search)
//
// Formalization belongs to the graphs, not to Run: it is the first node of both the
// agentic and low graphs, while the naive path has none, so each path below runs it where
// its counterpart does.
func RunAgenticRAG(ctx context.Context, deps RAGTools, req runtime.RunRequest, sd runtime.SearchDeps, kb *runtime.Kbinfos, resp *RunResponse, logger *log.Logger, spec runtime.ModeSpec) {
	// The graph budget is NOT started here: the deadline is armed inside the first node's
	// return, i.e. after formalization has run, so formalization is not charged to it.
	switch {
	case spec.Agentic:
		// build_agentic_graph — formalization is the graph's first node,
		// so it happens inside runAgentic.
		runAgentic(ctx, deps, req, sd, kb, resp, logger)
	case spec.Label != "naive":
		// build_low_graph — formalize → direct_search.
		if err := BuildLowGraph(ctx, deps, req, sd, kb, resp, logger); err != nil {
			// run_agentic_rag — a graph exception is recorded separately from
			// "research found nothing"; the fallback below only fires when the
			// run also produced nothing.
			resp.GraphFailed = true
		}
	default:
		// _naive_rag: no formalize node, a plain retrieve with no
		// keyword extraction, and its own composition (flat "[i] content"
		// evidence under naiveAnswerSystem). It must therefore not fall
		// through to composeFinalAnswer.
		NaiveRAG(ctx, deps, req, kb, resp, logger)
	}

	// run_agentic_rag — only when research produced NOTHING *and* the graph
	// itself failed. An empty result without a failure is not an error: that is
	// EmptyResponse's job, and overwriting it here would hide the real reason.
	if resp.GraphFailed && resp.Answer == "" && len(resp.Slots) == 0 {
		step(ctx, logger, "Agentic RAG", "Research failed without producing anything; returning the fallback answer.")
		resp.Answer = graphFailureFallback
	}
}

var reAnswerThink = regexp.MustCompile(`(?s)^.*</think>`)

// AnswerResult is the composed final answer.
type AnswerResult struct {
	// Answer is the composed text.
	Answer string
	// Partial is true when the underlying research ended without a satisfying
	// verdict; the answer carries a partial-information preamble.
	Partial bool
	// NoEvidence is true when composition was skipped for lack of evidence.
	NoEvidence bool
	// Failed is true when the composition call failed and the fallback message
	// was returned.
	Failed bool
}

// ComposeAnswer: turn the gathered
// evidence into a grounded, cited answer.
//
// Behaviour, in order:
//  1. no evidence + configured empty_response → return it WITHOUT calling the LLM;
//  2. rank chunks by similarity, keep the top citeChunkCap as citation reference;
//  3. render the evidence block under the token budget (kb_prompt);
//  4. prepend the fact-preserving pre_summary (the SCA-reviewed draft) when set;
//  5. call the model with FINAL_ANSWER_SYSTEM + the composed user content.
func ComposeAnswer(ctx context.Context, deps AnswerDeps, kb *runtime.Kbinfos, question string, partial, abstain bool) AnswerResult {
	return ComposeAnswerWith(ctx, deps, kb, question, partial, abstain, false)
}

// multimodalUserMsg builds a user message that carries the given text plus any
// vision-gated image data URIs. Returns nil when there is no text and no
// images, so callers fall back to schema.UserMessage. Used by the non-outer
// final-answer path so the compose model sees images even without the outer
// react loop.
func multimodalUserMsg(text string, images []string) *schema.Message {
	parts := make([]schema.MessageInputPart, 0, 1+len(images))
	if text != "" {
		parts = append(parts, schema.MessageInputPart{Type: schema.ChatMessagePartTypeText, Text: text})
	}
	for i := range images {
		uri := images[i]
		parts = append(parts, schema.MessageInputPart{
			Type:  schema.ChatMessagePartTypeImageURL,
			Image: &schema.MessageInputImage{MessagePartCommon: schema.MessagePartCommon{URL: &uri}},
		})
	}
	if len(parts) == 0 {
		return nil
	}
	return &schema.Message{Role: schema.User, UserInputMultiContent: parts}
}

// answerTemperature: default sampling temperature:
// build_agentic_graph uses gen_conf or {"temperature": 0.3} (/:1021)
// and run_agentic_rag invokes the graph with gen_conf unset, so the compose
// call always samples at 0.3.
const answerTemperature = 0.3

// answerTargetContract is the static answer-target guardrail. It costs no LLM call — it is
// injected into every final-answer prompt.
//
// The EXTREME-SELECTION clause is the load-bearing part: for
// shortest/longest/smallest/largest/most/least/最 questions, a model otherwise
// tends to name the most common or first-listed candidate. Naming the clause
// explicitly forces a comparison against the evidence instead.
const answerTargetContract = "Answer Target Contract:\n" +
	"Final answer must directly satisfy the user's top-level who/what request. " +
	"Use bridge entities only as clues, and verify any proposed answer against " +
	"the evidence. In EXTREME-SELECTION questions (shortest/longest/smallest/" +
	"largest/most/least/最), compare the alternatives in the evidence and name " +
	"the EXTREME one rather than the most common or first-listed.\n"

// withCitedChunks puts the passage behind every enumerated item FIRST, then the top-scoring
// remainder. An enumerated answer cites one passage per element, and those passages are in the pool
// but not necessarily among the few that score highest: a sixteen-member table whose answer could
// name only three of them was reading the same six blocks as any other question.
//
// Members first also means a token budget that truncates can only drop the scored extras, never an
// element's own passage (see KBPrompt, which stops at the budget).
func withCitedChunks(ranked []map[string]any, kb *runtime.Kbinfos, cap int) []map[string]any {
	if kb == nil || len(ranked) == 0 {
		return ranked
	}
	cited := make([]map[string]any, 0, len(ranked))
	used := map[string]bool{}
	for _, id := range kb.CitedChunks() {
		if c := kb.ChunkByID(id); c != nil && !used[id] {
			used[id] = true
			cited = append(cited, c)
		}
	}
	for _, c := range ranked {
		if len(cited) >= len(kb.CitedChunks())+cap {
			break
		}
		if id := runtime.ChunkIDOf(c); id != "" && used[id] {
			continue
		}
		cited = append(cited, c)
	}
	if len(cited) == 0 {
		return ranked
	}
	return cited
}

// compactAnchored renders each anchored member's block from the quote the naming node matched it
// to, leaving every other chunk as it is. The block is what the budget is spent on, so whole
// passages put only the first handful of members inside the render — and a member outside it has
// no block number the model could cite. The quote carries the same words the record already
// states, so no evidence is lost: the passage keeps its place in the pool and in the reference.
func compactAnchored(chunks []map[string]any, kb *runtime.Kbinfos) []map[string]any {
	if kb == nil {
		return chunks
	}
	quotes := map[string]string{}
	for _, ref := range kb.AnchoredRefs() {
		if ref.ChunkID == "" || ref.Quote == "" {
			continue
		}
		if _, dup := quotes[ref.ChunkID]; !dup {
			quotes[ref.ChunkID] = ref.Quote
		}
	}
	if len(quotes) == 0 {
		return chunks
	}
	out := make([]map[string]any, 0, len(chunks))
	for _, c := range chunks {
		q, ok := quotes[runtime.ChunkIDOf(c)]
		if !ok {
			out = append(out, c)
			continue
		}
		cp := make(map[string]any, len(c)+1)
		for k, v := range c {
			cp[k] = v
		}
		cp["content"] = q
		out = append(out, cp)
	}
	return out
}

// anchoredPoolEntries rebuilds a pool record for every enumerated member whose passage is NOT in
// the pool, from the quote the naming node took from that passage.
//
// The citation chain is: marker number → position in the published list → the pool entry that id
// resolves to. A member whose passage is gone therefore cannot be cited at all — the marker is
// dropped (citePoolIdx answers -1) or opens nothing. Rebuilding the record from the member's own
// quote keeps that chain whole, and the content it opens is exactly the text the answer states.
func anchoredPoolEntries(kb *runtime.Kbinfos) []map[string]any {
	if kb == nil {
		return nil
	}
	var out []map[string]any
	for _, r := range kb.AnchoredRefs() {
		id := strings.TrimSpace(r.ChunkID)
		quote := strings.TrimSpace(r.Quote)
		if id == "" || quote == "" || kb.ChunkByID(id) != nil {
			continue
		}
		out = append(out, map[string]any{
			"chunk_id":            id,
			"content":             quote,
			"content_with_weight": quote,
		})
	}
	return out
}

// appendMissingIDs appends the ids not already present, in order: a passage the render left out
// (the budget admits only the first few whole chunks) still has to hold a position in the
// published list, or nothing can cite it.
func appendMissingIDs(ids, extra []string) []string {
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		seen[id] = true
	}
	for _, id := range extra {
		if id = strings.TrimSpace(id); id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

// answerPromptWithEvidence renders the parts list in order: question, answer-target
// contract, optional no-evidence instruction, research summary, partial preamble,
// evidence. The no-evidence flag is `abstain or empty_result or not chunks`.
func (d AnswerDeps) answerPromptWithEvidence(kb *runtime.Kbinfos, question string, partial, noEvidence bool) answerPrompt {
	chunks := []map[string]any{}
	if kb != nil {
		// An enumerated member whose passage is no longer in the pool is rebuilt from the quote the
		// naming node took from it BEFORE anything is rendered or published: the marker's number
		// indexes the published list and the client opens the pool entry behind it, so a member
		// whose passage was evicted (the pool cap, or selectEvidence narrowing to another call)
		// would otherwise lose its citation in the chat pipeline (citePoolIdx answers -1 and the
		// marker is dropped).
		if extra := anchoredPoolEntries(kb); len(extra) > 0 {
			kb.Admit(func(p *runtime.PoolAdmitter) {
				for _, c := range extra {
					p.Add(c)
				}
			})
		}
		chunks = kb.Chunks
	}
	ranked := rankByScore(chunks)
	citeChunks := withCitedChunks(ranked, kb, citeChunkCap)
	if len(citeChunks) == 0 {
		citeChunks = chunks
	}
	// An anchored member's block is rendered from its QUOTE: the budget is spent per block, so a
	// whole ~1200-char passage per member fits only the first handful of them, and every member
	// past that one is a member neither the model nor the answer can cite. The quote is the same
	// words the record states, and the passage itself stays in the pool and in the reference.
	citeChunks = compactAnchored(citeChunks, kb)
	maxTokens := d.MaxTokens
	if maxTokens <= 0 {
		maxTokens = evidenceBudgetTokens
	}
	// Publish the ordered evidence list the model is about to see so the chat
	// pipeline can resolve the answer's [ID:n] markers against THE SAME list and put
	// it in front of the reference (runtime.Kbinfos.CiteChunkIDs →
	// RunResponse.CiteChunkIDs → decorateHarnessAnswer). The blocks are numbered
	// 0-based because the client renders a marker by indexing reference.chunks with
	// its number, which the model has to write itself; citeChunks is the
	// similarity-ranked, capped selection (not the pool in order), so the reference
	// is ordered to match it.
	//
	// The ids come from the render's SOURCE indices, not from citeChunks: a chunk
	// with no content renders no block, so walking citeChunks would publish an id for
	// the skipped chunk and put every marker past it one block off.
	//
	// The enumerated members then follow whether or not their block rendered: the evidence
	// budget admits only the first few whole chunks, so a member past them holds no block
	// number the model could write — and a member with no position is a member the answer
	// cannot cite and the user cannot open. Appending keeps the rendered prefix intact
	// (block n is still position n) and gives the completion its target
	// (runtime.CiteAnchoredMembers).
	blocks, sources := prompts.KBPromptZeroBasedWithSourceIndices(citeChunks, maxTokens)
	if kb != nil {
		// Only ids the POOL still holds are appended: the chat pipeline resolves a marker by
		// looking its chunk up in the pool, and a member whose passage is gone would publish a
		// nil reference entry — the marker is then dropped and the line loses its citation
		// entirely. A member the pool no longer holds must lose its citation, not break the one
		// the user can open.
		present := make([]string, 0, len(kb.CitedChunks()))
		for _, id := range kb.CitedChunks() {
			if kb.ChunkByID(id) != nil {
				present = append(present, id)
			}
		}
		kb.CiteChunkIDs = appendMissingIDs(citeChunkIDsAt(citeChunks, sources), present)
	}
	evidence := strings.Join(blocks, "\n")

	summary := ""
	record := composedRecord(kb)
	if kb != nil {
		summary = strings.TrimSpace(kb.PreSummary)
	}

	parts := []string{fmt.Sprintf("Question:\n%s\n", question)}

	// The static guardrail, always present.
	parts = append(parts, answerTargetContract)

	// How to degrade when there is no evidence (reached only when no empty_response
	// short-circuited the call).
	if noEvidence {
		if summary != "" {
			parts = append(parts, noEvidenceWithSummary)
		} else {
			parts = append(parts, noEvidenceWithoutSummary)
		}
	}

	if record != "" {
		// The slot table of an agentic round: facts to check the answer against,
		// explicitly NOT evidence and NOT answer text (see recordContract).
		parts = append(parts, fmt.Sprintf("%s\n%s\n", recordContract, record))
	} else if summary != "" {
		// A prose summary (no slot table behind it): it stands in as findings.
		parts = append(parts, fmt.Sprintf("Research Summary (primary evidence):\n%s\n", summary))
	}
	// _compose_answer_from_evidence: the partial preamble goes HERE — after the research
	// summary and before the evidence — not at the very front of the prompt.
	if partial {
		parts = append(parts, fmt.Sprintf("%s\n", runtime.PartialAnswerPreamble))
	}
	parts = append(parts, fmt.Sprintf("Evidence:\n%s", evidence))

	return answerPrompt{system: d.composeSystem(), user: strings.Join(parts, "\n"), partial: partial}
}

// naiveAnswerSystem is the fixed system prompt the naive path sends. It is deliberately
// NOT FinalAnswerSystem: the naive path is a
// plain single-pass answer and carries neither the citation contract nor the
// partial-answer preamble the agentic modes compose.
const naiveAnswerSystem = "Answer the question using ONLY the numbered evidence below. Cite with [n] markers. If the evidence does not answer it, say so plainly — do not use outside knowledge."

// naiveAnswerTemperature is the sampling temperature of the naive answer call
// (: answer_conf = gen_conf or {"temperature": 0.3}). Go's RunRequest
// carries no per-call generation config, so this is always the no-gen_conf
// branch.
const naiveAnswerTemperature = 0.3

// naiveFallbackChars caps the evidence echoed back when the naive answer call
// fails (: evidence[:4000]).
const naiveFallbackChars = 4000

// modelWithTemperature returns a view of mdl that samples at temp, falling back
// to the model's default when it does not support per-call temperature.
func modelWithTemperature(mdl runtime.SessionModel, temp float64) runtime.SessionModel {
	if tm, ok := mdl.(runtime.TemperatureModel); ok {
		return &temperatureModel{mdl: tm, temp: temp}
	}
	return mdl
}

type temperatureModel struct {
	mdl  runtime.TemperatureModel
	temp float64
}

func (m *temperatureModel) Complete(ctx context.Context, messages []schema.Message, tools []runtime.ToolSpec) (*runtime.ModelReply, error) {
	return m.mdl.CompleteWithTemperature(ctx, messages, tools, m.temp)
}
