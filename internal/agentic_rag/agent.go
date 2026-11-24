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

package agentic_rag

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/entity/models"
	"ragflow/internal/retrievalaccount"
	"ragflow/internal/tokenizer"
)

// Input carries everything Run needs to spin up a one-shot ReAct conversation
// turn. It is intentionally decoupled from the canvas runtime: the caller
// builds the model and messages directly.
type Input struct {
	// Model is the chat model backing the agent. It must support tool calling
	// (model.ToolCallingChatModel) — models.EinoChatModel does.
	Model *models.EinoChatModel
	// SynthModel, when non-nil, is the model the last-resort synthesis
	// (finalizeAnswer) calls. It must be a SEPARATE instance over the same
	// failover chain as Model: a failover instance caches the error of its
	// last full-chain failure and short-circuits every later call with it for
	// 30s, so sharing the agent's instance hands the synthesis a stale error
	// from whatever malformed call tripped the cooldown — which is how a
	// request with no tool messages in it once "failed" with
	// "tool result's tool id ... not found". Nil falls back to Model.
	SynthModel *models.EinoChatModel
	// AuditModel, when non-nil, is the model the delivery gate's auditor runs
	// on. It must be a SEPARATE instance over the same failover chain as Model
	// — and, unlike Model, one whose sampling is pinned (see
	// AuditTemperature) — because the auditor is not a second producer: its
	// verdict is machine-parsed and decides whether the deliverable ships, so
	// it must not inherit the producer's sampling noise, and a failover
	// instance's cached last error must not flow between the two. Nil falls
	// back to Model.
	AuditModel *models.EinoChatModel
	// PlanAuditModel, when non-nil, is the model the independent plan auditor
	// runs on: same reasoning as AuditModel (the plan_auditor template pins
	// 0.1 — its findings are machine-parsed and each sampled FAIL burns a
	// repair turn), separate failover state for the same reason. Nil falls
	// back to Model.
	PlanAuditModel *models.EinoChatModel
	// PlanStageModel, when non-nil, is the model the plan stage's planner
	// runs on. The planner template declares its own temperature (0.5): the
	// stage's product is machine-verified by the plan auditor and the
	// mechanical check, so sampled dropout burns repair turns and the shape
	// variance of a hot sample shows up as run-to-run plan churn. Nil falls
	// back to Model.
	PlanStageModel *models.EinoChatModel
	// Messages are the conversation history plus the current user message —
	// one turn per Run. The next user input is the NEXT Run (AsyncChat is
	// re-entered per request), so the history the caller passes in must
	// already be compact: one final answer per earlier turn, never the ReAct
	// trajectory that produced it.
	Messages []*schema.Message
	// MaxIterations caps the ReAct loop. Zero falls back to a sane default.
	MaxIterations int
	// Stream controls whether model output is streamed into the event iterator.
	Stream bool
	// TenantID is the conversation's tenant (the chat's bound tenant, decided by
	// the session created in the UI). It is injected into the retrieval tools so
	// they search the right index without a canvas runtime.
	TenantID string
	// UserID identifies whose per-Tenant default models apply to this run.
	UserID string
	// DatasetIDs is the conversation's bound dataset scope, decided by the
	// session created in the UI. It is injected into the retrieval tools.
	DatasetIDs []string
	// DocIDs is the server-bound document scope. Nil leaves retrieval unrestricted;
	// a non-nil empty slice restricts all corpus tools to no documents.
	DocIDs []string
	// ConversationKey is the conversation's stable identifier (the chat id). When
	// set, the compiled-knowledge tools (navigate_tree / graph_explore) record a
	// dataset-level absence once and drop themselves from the tool list until the
	// 30-minute mark expires, so the agent stops spending calls on a tool the
	// corpus cannot satisfy. When empty, a per-run key scopes the decision to this
	// single ReAct turn. The chat pipeline sets it from the conversation id; see
	// nav_tool_enablement.go.
	ConversationKey string
	// Tools are the eino tools the agent may call. When empty, the tool set of
	// the resolved template (TemplateID, else the config's default) is used.
	// The web_search tool is NOT passed here: it is injected from the run's
	// context (see WithWebSearch), which is what keeps one conversation's
	// internet capability from leaking into another's.
	Tools []tool.BaseTool
	// TemplateID selects the agent template from conf/agentic_rag.yaml by id
	// (e.g. "smart-reasoning"). The caller names it; a request cannot, because a
	// turn is selected by reasoning level and the level names no config id. It is
	// mandatory: an empty or unknown id fails the run with a clear error — there is
	// no default/env/first-template fallback.
	TemplateID string
	// Template is the per-run resolved configuration, shared with model setup.
	Template *Template
	// OnDelta, when non-nil, receives each incremental (content, reasoning)
	// delta as it streams. When nil, deltas fall back to runtime.EmitAgentMessage.
	OnDelta func(contentDelta, thinkingDelta string)
	// ToolCallCounts, when non-nil, is filled with the number of times each tool
	// was invoked during this agent turn, keyed by tool name. Tools never called
	// are omitted. Useful for per-question usage accounting.
	ToolCallCounts map[string]int
	// ToolCallErrors, when non-nil, tallies how many tool calls returned a
	// failure notice (<tool_error> / severity="error") during this turn,
	// keyed by tool name. A backend outage or repeated invalid arguments
	// shows up here instead of hiding inside the call counts.
	ToolCallErrors map[string]int
	// ToolErrorSamples, when non-nil, records one representative failure
	// message per tool (the first occurrence, truncated) so a benchmark row
	// is diagnosable without opening the server logs.
	ToolErrorSamples map[string]string
	// ToolCallDurations, when non-nil, accumulates the total wall-clock duration
	// of every tool invocation during this agent turn, keyed by tool name. Pair
	// this with ToolCallCounts to derive per-call average latency per tool.
	ToolCallDurations *durationAccumulator
	// RetrievedDocIDs, when non-nil, collects every document identifier any
	// retrieval tool surfaced during this agent turn (both doc_id and doc_name,
	// deduplicated). Citation payloads only carry what the final answer quotes,
	// but benchmarks score retrieval recall against everything the agent ever
	// pulled, so the tally has to be taken per tool call.
	RetrievedDocIDs *docIDLedger
	// ChunkReads, when non-nil, accumulates how many chunks the retrieval
	// tools put in front of the model, split by read depth (full content =
	// deep, snippet window = shallow). Benchmarks report both totals per
	// question next to ToolCallCounts.
	ChunkReads *chunkReadLedger
	// Searches, when non-nil, records every retrieval action the run actually
	// executed - tool name and verbatim arguments, in execution order. It is
	// the ground truth of WHAT WAS SEARCHED: the Candidate Matrix is the
	// model's own end-of-run reconstruction and measurably underreports (a
	// #30 run executed 59 distinct searches and its matrix carried 8), so
	// coverage judgments that ask "was this anchor ever queried" must read
	// this ledger, not the matrix. The delivery gate hands it to the auditor
	// as the read_search_ledger tool.
	Searches *searchLedger
	// Serves, when non-nil, records every chunk the retrieval tools put in
	// front of the model, keyed by chunk id with the document it belongs to.
	// The delivery gate diffs it against the deep-read set: documents that
	// were served but never deep-read are exactly the candidates a failed
	// chain never looked at.
	Serves *servedLedger
	// GateAudit, when non-nil, is filled by the delivery gate with the
	// answer auditor's per-round suspect accounting (see GateAuditRecord).
	// Benchmarks report it next to ToolCallCounts to show how much audit
	// effort a question consumed.
	GateAudit *GateAuditRecord
}

// GateAuditRecord carries the delivery gate's per-round audit accounting for
// one agent turn. Suspects holds the suspect count of EVERY audit the gate
// ran, oldest first — the whole curve, pass rounds included (a PASS verdict
// reports 0 suspects) — so a consumer can see how the count evolved
// (15→11→6→5→0 reads very differently from a flat 1,1,1,1). Passed reports
// whether the LAST audit the gate ran concluded PASS; a gate that exited on
// stall, budget or time leaves it false.
type GateAuditRecord struct {
	Suspects []int `json:"suspects,omitempty"`
	Passed   bool  `json:"passed"`
	// Rejections counts the deliverables the gate refused BEFORE an audit ran —
	// an ungrounded value, a list-only name, a value-less answer line, an
	// answer-less continuation. Without it, a run whose gate never reached the
	// auditor (both q350 and q784 short-circuited this way) carried an EMPTY
	// Suspects on a never-PASSed deliverable, which read as "nothing to report"
	// and let a `Final Answer` label survive unaudited.
	Rejections int `json:"rejections,omitempty"`
	// AuditFailures counts audit passes the auditor itself could not complete
	// (LLM timeout, tool outage). It is the third way "the gate never concluded
	// PASS" becomes true, and the only one that records nothing else: the gate
	// returns the standing deliverable on an audit error, so without this count
	// the record stayed EMPTY on a never-audited deliverable and the label
	// governance read it as "no audit was needed" — shipping an unverified
	// `Final Answer`. It doubles as an operator signal: a per-question audit
	// outage is a provider problem, not a reasoning one.
	AuditFailures int `json:"audit_failures,omitempty"`
	// CitationGroundings counts the deliverables the gate rejected because their
	// candidate lines rested on a BIBLIOGRAPHIC ENTRY (a citation names a work and
	// states nothing about it). It is the mechanical half of the sibling discipline:
	// the auditor is ASKED to widen its read, but whether a round does it is up to
	// the model, and 10 of 17 measured q221 runs shipped the wrong member of a
	// two-book family — every one of them "grounded" by a citation line. Recorded
	// separately from Rejections so a benchmark can tell a citation-only matrix from
	// a missing-value one.
	CitationGroundings int `json:"citation_groundings,omitempty"`
	// UnreadCitations counts the deliverables the gate refused because they cited a
	// chunk the run never received (neither a deep read nor a triage snippet). It is
	// the observable half of the citation discipline: a cited id outside the read
	// ledger is a fabricated or mis-transcribed identifier, and unlike a value
	// precheck this reading is membership, not phrasing - see unreadCitations for
	// why that makes it admissible where the value heuristics were not. Recorded
	// apart from Rejections so a benchmark can tell "cited evidence I never read"
	// from "no deliverable at all".
	UnreadCitations int `json:"unread_citations,omitempty"`
	// ModelKilledTurns counts the gate-loop turns (repair turns, audit rounds)
	// that a model-chain failure killed outright — the failover chain exhausted
	// every model on a 529/overload. It is the causal link between a question's
	// failure and the provider: a FAIL record with ModelKilledTurns > 0 died to
	// the model chain, not to the deliverable's content, and the run's row
	// should be read as infrastructure-shaped even though it ships an answer.
	ModelKilledTurns int `json:"model_killed_turns,omitempty"`
	// LastModelKill carries the provider-facing reason of the most recent
	// model-killed turn, so the run's own error can describe itself
	// ("model failure killed 2 gate turn(s): provider overloaded (HTTP 529)")
	// instead of the row reading like a content refusal.
	LastModelKill string `json:"last_model_kill,omitempty"`
	// ChainBreaks counts the deliverables the gate refused because a
	// Survivors line filled a downstream variable while an upstream `From`
	// dependency still read empty, or the answer filler traced to no upstream
	// set - the "look ahead, then come back" discipline enforced mechanically
	// (see survivorsChainBreaks). Recorded apart from Rejections so a
	// benchmark can tell a chain-order failure from other refusals.
	ChainBreaks int `json:"chain_breaks,omitempty"`
	// AuditVerdicts holds an excerpt of EVERY audit verdict, oldest first, in
	// step with Suspects. The counts alone say a curve moved 5→3→1→0 but never
	// WHAT was contested, so a failure could only be explained by re-reading the
	// shipped deliverable and guessing — which is how the #221 analysis had to
	// proceed, and how it managed to infer the wrong cause twice. One excerpt
	// per round makes the audit's own words the record.
	AuditVerdicts []string `json:"audit_verdicts,omitempty"`
}

// defaultMaxIterations caps the ReAct loop before the agent must answer. It is
// raised beyond the original 50 because multi-step benchmark questions (multiple
// hops, off-by-one arithmetic) routinely need more than 50 model turns, and the
// duplicate-retrieval guard (todo 3) prevents the extra budget from being wasted
// on repeated identical searches.
const defaultMaxIterations = 120

// agenticMaxIterations resolves the loop cap: an explicit request wins, then
// RAGFLOW_AGENTIC_MAX_ITERATIONS, then the default. Clamped at 1 — zero would
// mean "never loop", turning every run into an immediate failure.
func agenticMaxIterations(explicit int) int {
	if explicit > 0 {
		return explicit
	}
	n := envIntDefault(common.EnvAgenticMaxIterations, defaultMaxIterations)
	if n < 1 {
		return 1
	}
	return n
}

var errNilModel = errors.New("agentic_rag: model is required")

// runSeq issues a unique key for each Run that carries no ConversationKey, so a
// disable decision is scoped to that turn and does not collide with a concurrent
// one.
var runSeq uint64

// auditModelFor is the model the auditor runs on: its own instance when the
// caller built one (pinned sampling, separate failover state), else the
// producer's. A named rule rather than an inline nil check, so the fallback
// stays testable.
func auditModelFor(in Input) *models.EinoChatModel {
	if in.AuditModel != nil {
		return in.AuditModel
	}
	return in.Model
}

// llmRetryMax bounds retry attempts per model call on top of the initial one
// (adk semantics: MaxRetries=1 → up to 2 calls). With the backoff below the
// worst-case added latency per call is ~2s.
//
// Lowered from 3 to 1 on 2026-09-17 for the slow-plan experiment: a slow
// endpoint that fails at the send phase fails the same way on every attempt,
// so the extra attempts only bought latency (and, with the old 300s per-call
// budget, ran whole questions into the wall-clock deadline).
const llmRetryMax = 1

// agentModelRetryConfig returns the eino-native retry policy attached to
// every ChatModelAgent this package builds (explorer, repair turns, and the
// answer_auditor auditor): provider hiccups — MiniMax 529 overload, 429 rate
// limits, 5xx, connection resets — are retried with seconds-scale backoff,
// while client errors (400/401) and exhausted deadlines fail fast.
//
// Two design guards:
//  1. Retries are refused once partial content has streamed out
//     (OutputMessage != nil): adk forwards stream deltas to the client in
//     real time and only defers the retry decision, so a retried mid-stream
//     failure would duplicate already-emitted text.
//  2. context.Canceled / DeadlineExceeded are never retried — the 300s
//     Generate budget already spent is not doubled by another attempt.
func agentModelRetryConfig() *adk.ModelRetryConfig {
	return &adk.ModelRetryConfig{
		MaxRetries: llmRetryMax,
		ShouldRetry: func(_ context.Context, rc *adk.RetryContext) *adk.RetryDecision {
			if rc.Err == nil || rc.OutputMessage != nil {
				return nil // success, or partial output already delivered: accept as-is
			}
			if errors.Is(rc.Err, context.Canceled) || errors.Is(rc.Err, context.DeadlineExceeded) {
				// A send/transport-phase failure is transient even though it
				// wraps a deadline ("failed to send request: Post ...:
				// context deadline exceeded" is the endpoint being briefly
				// unreachable, ~seconds); only the Generate-level budget
				// ("EinoChatModel.Generate(...): context deadline exceeded
				// (300s...)") has already spent the turn's allowance and must
				// fail fast instead of doubling it.
				if !transientLLMError(rc.Err) {
					return nil
				}
			}
			if transientLLMError(rc.Err) {
				return &adk.RetryDecision{Retry: true}
			}
			return nil // 400/401/403 and friends: retrying cannot help
		},
		BackoffFunc: func(_ context.Context, attempt int) time.Duration {
			// Overload (529) recovery needs seconds, not adk's default
			// 100ms ramp: 2s → 5s → 10s.
			switch {
			case attempt <= 1:
				return 2 * time.Second
			case attempt == 2:
				return 5 * time.Second
			default:
				return 10 * time.Second
			}
		},
	}
}

// transientLLMError reports whether an LLM error is worth retrying: provider
// overload (529), rate limiting (429), other 5xx server errors, and
// network-level hiccups. Matched by message substrings because the drivers
// wrap provider bodies as text (e.g. "status 529: ... overloaded_error").
//
// 用量上限 IS included even though it names MiniMax's plan wall: the gateway
// reports short TPM/RPM bursts with the SAME wording it uses for true plan
// exhaustion, and the message carries no HTTP status to tell them apart. A
// burst recovers within seconds, so backing off is right; a genuine wall costs
// at most llmRetryMax retries (2s → 5s → 10s, ~17s) before failing fast, which
// is the price of not aborting whole benchmark runs on a momentary burst. The
// alternative - treating the string as terminal - is what made concurrent runs
// report "quota exhausted" while the plan still had headroom.
func transientLLMError(err error) bool {
	msg := err.Error()
	for _, marker := range []string{
		"529", "overloaded_error", "429", "rate limit", "status 5",
		// MiniMax's plan limits arrive as Chinese prose with no status code.
		// 速率限制 is the 429 wording and 用量上限 the wall/burst wording;
		// both are retryable - see the comment above for the burst rationale.
		"速率限制", "用量上限",
		// transport phase (safe to retry even when it wraps a deadline)
		"failed to send request", "dial tcp", "i/o timeout", "tls handshake",
		"connection reset", "connection refused",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// durationAccumulator is a concurrency-safe total-duration tally keyed by tool
// name. The agent executes same-round tool_calls in parallel, so callers may
// invoke Add from multiple goroutines; the mutex guards the shared map.
type durationAccumulator struct {
	mu  sync.Mutex
	dur map[string]time.Duration
}

func NewDurationAccumulator() *durationAccumulator {
	return &durationAccumulator{dur: make(map[string]time.Duration)}
}

func (a *durationAccumulator) Add(name string, d time.Duration) {
	if a == nil || name == "" {
		return
	}
	a.mu.Lock()
	a.dur[name] += d
	a.mu.Unlock()
}

func (a *durationAccumulator) Snapshot() map[string]time.Duration {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make(map[string]time.Duration, len(a.dur))
	for k, v := range a.dur {
		out[k] = v
	}
	return out
}

// docIDLedger accumulates the set of documents a run's FULL-CONTENT tools
// (search_chunks, list_chunks) surfaced. Benchmarks (BrowseComp-Plus) score
// retrieval recall against this union, not just the cited subset, so the
// tally is taken as results come back. Only the tools' rendered doc_name is
// stored, normalized to its file stem (see recordedDocIDs): a benchmark
// corpus keys its evidence on file stems (e.g. "5412" from "5412.md"), which
// the opaque doc_id cannot match. Locate-only tools (search_bm25_chunks,
// grep_chunks) do NOT feed the ledger — they surface a snippet window, not
// the document.
type docIDLedger struct {
	mu  sync.Mutex
	ids map[string]struct{}
}

// NewDocIDLedger returns an empty document ledger for one run.
func NewDocIDLedger() *docIDLedger {
	return &docIDLedger{ids: make(map[string]struct{})}
}

// Add records the given document identifiers. Empty values are dropped and
// duplicates collapse, so callers can pass raw tool output matches as-is.
func (l *docIDLedger) Add(ids ...string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, id := range ids {
		if id == "" {
			continue
		}
		l.ids[id] = struct{}{}
	}
}

// Snapshot returns the recorded identifiers in sorted order. Sorted so the
// payload a benchmark archives is byte-stable across runs.
func (l *docIDLedger) Snapshot() []string {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, 0, len(l.ids))
	for id := range l.ids {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// chunkReadLedger accumulates HOW MANY chunks a run's retrieval tools put in
// front of the model, split by read depth:
//   - DEEP reads (list_chunks, search_chunks): full chunk content rendered —
//     the expensive reads;
//   - SHALLOW reads (grep_chunks, search_bm25_chunks): <match_snippet> windows
//     only — the cheap triage reads.
//
// Benchmarks report the two totals per question next to tool_call_counts: the
// split shows whether a run's context weight came from reading documents or
// from triage traffic, which raw tool call counts cannot distinguish.
type chunkReadLedger struct {
	mu      sync.Mutex
	deep    int
	shallow int
	// deepIDs / shallowIDs record WHICH chunks the two counts refer to.
	//
	// The counts alone cannot answer the question a failed run actually poses:
	// did the passage that holds the answer ever reach the model? Measured on
	// the browsecomp retry batch, 13 of the 20 evidence_in_hand failures had a
	// gold-bearing document served to the run, only 6 of those cited it and 0
	// shipped it — the difference between "read and silently dropped" and
	// "never read", which a total (q1021: 458 chunks) cannot make. The
	// identifiers are the raw fact; whether an uncited read chunk MATTERS is
	// the auditor's judgement, not this ledger's.
	deepIDs    map[string]struct{}
	shallowIDs map[string]struct{}
}

// NewChunkReadLedger returns an empty chunk-read ledger for one run.
func NewChunkReadLedger() *chunkReadLedger {
	return &chunkReadLedger{
		deepIDs:    make(map[string]struct{}),
		shallowIDs: make(map[string]struct{}),
	}
}

// AddDeep records n deep-read chunks (full chunk content).
func (l *chunkReadLedger) AddDeep(n int) {
	if l == nil || n <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.deep += n
}

// AddShallow records n shallow-read chunks (snippet windows).
func (l *chunkReadLedger) AddShallow(n int) {
	if l == nil || n <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.shallow += n
}

// searchLedger records every retrieval action a run actually executed - tool
// name and verbatim arguments, in execution order, append-only. It is the
// ground truth of WHAT WAS SEARCHED:
//
// The Candidate Matrix is the model's own end-of-run reconstruction, and it
// measurably underreports - a #30 run executed 59 distinct searches while its
// shipped matrix carried 8 Searched lines, so an auditor judging coverage from
// the matrix alone misread a thoroughly-searched anchor as "never queried"
// (five stale suspects across as many passes). The ledger is recorded at tool
// execution by the same wrapper that accounts durations, so no reconstruction
// sits between the behavior and the record.
//
// Entries are append-only and keep duplicates: a repeated query is itself
// behavioral signal (the run searched the same thing twice). The order is
// COMPLETION order, not the model's tool-call order: one assistant turn can
// issue several searches in a batch and eino runs them concurrently, so the
// sequence records which search RETURNED first. The first entry is therefore
// "the first search that completed", not necessarily the first the model
// issued, and the candidate-free opening rule must read it as such. The
// auditor reads the ledger through the read_search_ledger tool (a cursor-style
// after argument returns the delta), which keeps the audit payload small while
// the ground truth stays a tool call away.
type searchLedgerEntry struct {
	seq  int    // 1-based COMPLETION order across the whole run
	tool string // e.g. search_bm25_chunks, grep_chunks
	args string // verbatim tool arguments (the query or query list)
}

type searchLedger struct {
	mu      sync.Mutex
	seq     int
	entries []searchLedgerEntry
}

// NewSearchLedger returns an empty search ledger for one run.
func NewSearchLedger() *searchLedger {
	return &searchLedger{}
}

// Add records one retrieval action. The arguments are stored verbatim
// (single-line trimmed) - no parsing, no normalization: the raw bytes are the
// fact, and how to read them is the auditor's judgement.
func (l *searchLedger) Add(tool, args string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seq++
	l.entries = append(l.entries, searchLedgerEntry{
		seq:  l.seq,
		tool: tool,
		args: strings.TrimSpace(args),
	})
}

// servedLedger records the documents the retrieval tools served to the model,
// counting - per document - the DISTINCT QUERIES that returned it. The delivery
// gate diffs it against the deep-read document set (RetrievedDocIDs, filled only
// by full-content tools) and reports the documents that kept surfacing and were
// never opened: those are the candidates a failing chain never looked at. A
// document served once in a passing sweep is noise; one several different
// searches returned is a candidate.
//
// The unit is the QUERY, never the chunk. Counting serves per chunk made the
// weight a function of how many chunks of one document a single result carried:
// measured 2026-10-05 on q1093 of a ten-question run, the run's own latched
// document reached 35 chunk-serves while every unread lead it was supposed to
// consider stood at 2-4, so the census's top five was one document repeated and
// the leads it exists to name never made the list. Counting distinct queries
// drops that same document to 8 - still the loudest, because the run really did
// ask about it eight times, but the inflation is gone.
type servedLedger struct {
	mu   sync.Mutex
	docs map[string]map[string]struct{} // document stem -> the distinct queries that served it
}

// NewServedLedger returns an empty served ledger for one run.
func NewServedLedger() *servedLedger {
	return &servedLedger{docs: map[string]map[string]struct{}{}}
}

// Add records one served document (the benchmark's file stem) under the query
// that returned it. Repeats of the same (query, document) pair collapse: the
// weight is how many different searches insisted on the document, not how many
// times one search was re-run.
func (l *servedLedger) Add(query, docID string) {
	if l == nil || docID == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	queries, ok := l.docs[docID]
	if !ok {
		queries = map[string]struct{}{}
		l.docs[docID] = queries
	}
	queries[query] = struct{}{}
}

// Serves returns how many DISTINCT queries returned this document during the run
// (0 when none did). Callers that report a lead's weight read it through this
// accessor rather than the document map, which Docs() hands out a copy of,
// unlocked.
func (l *servedLedger) Serves(docID string) int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.docs[docID])
}

// Docs returns the served documents, most-served first (ties broken by id).
func (l *servedLedger) Docs() []string {
	if l == nil {
		return nil
	}
	type pair struct {
		id string
		n  int
	}
	var ps []pair
	l.mu.Lock()
	for d, queries := range l.docs {
		ps = append(ps, pair{d, len(queries)})
	}
	l.mu.Unlock()
	sort.Slice(ps, func(i, j int) bool {
		if ps[i].n != ps[j].n {
			return ps[i].n > ps[j].n
		}
		return ps[i].id < ps[j].id
	})
	out := make([]string, 0, len(ps))
	for _, x := range ps {
		out = append(out, x.id)
	}
	return out
}

// How many DISTINCT QUERIES make a document a named lead, and how many leads
// each consumer is handed. The auditor gets a short list; the RE-ANCHOR round
// gets a longer one, because that round exists to put the right unread documents
// in front of the producer and the answer-bearing ones can rank below the
// auditor's cap: measured on the smoke53 run, q1093's two documents carrying the
// answer's own series had been served twice each and never opened, and neither
// reached the auditor's ten while ten louder leads did. The unit is the query on
// purpose - see servedLedger - because the run's own latched candidate is what
// pumps a document's count, and one query returning many chunks of it must not
// be allowed to crowd the list of leads the run has NOT considered.
const (
	servedUnreadMinServes   = 2
	servedUnreadAuditorMax  = 10
	servedUnreadReanchorMax = 20
)

// repairGateBudget bounds the gate's repair turns independently of
// auditMaxPass. The two used to share one counter, so a repair could spend an
// audit's budget; this cap keeps repeated precheck failures from spinning the
// loop now that audits are counted on their own.
const repairGateBudget = 16

// servedUnreadDocs returns the served documents that were never deep-read and
// that the retrieval kept surfacing (at least servedUnreadMinServes serve
// events), MOST-SERVED FIRST - Docs() ranks them - labelled with the count the
// retrieval served them ("<doc> (xN)"), and capped at limit.
//
// The label is the point of the census: the count is how a reader (the auditor,
// or whoever reads the log) tells a lead the retrieval insisted on from one it
// mentioned once, and it is the same "<doc> (xN)" phrasing the mid-run
// watchdog's notice uses.
func servedUnreadDocs(served *servedLedger, deepDocs *docIDLedger, limit int) []string {
	if served == nil || limit <= 0 {
		return nil
	}
	deep := map[string]struct{}{}
	for _, d := range deepDocs.Snapshot() {
		deep[d] = struct{}{}
	}
	unread := []string{}
	for _, d := range served.Docs() { // most-served first, ties broken by id
		if _, read := deep[d]; read {
			continue
		}
		serves := served.Serves(d)
		if serves < servedUnreadMinServes {
			break // Docs() is ranked: every remaining document is quieter still
		}
		// The serve count rides along in the auditor's payload and in the log:
		// the number is what tells a reader how hard the retrieval insisted on
		// this lead, and the auditor cites it in the opinion. Same "<doc> (xN)"
		// convention as the mid-run watchdog's notice.
		unread = append(unread, fmt.Sprintf("%s (x%d)", d, serves))
		if len(unread) == limit {
			break
		}
	}
	return unread
}

// servedLedgerContextKey carries one run's served ledger through the
// context the tools execute under.
type servedLedgerContextKey struct{}

// withServedLedger returns ctx carrying the run's served ledger.
func withServedLedger(ctx context.Context, l *servedLedger) context.Context {
	return context.WithValue(ctx, servedLedgerContextKey{}, l)
}

// servedLedgerFrom returns the run's served ledger, or nil when the context
// carries none (a caller that built its own context for a probe).
func servedLedgerFrom(ctx context.Context) *servedLedger {
	l, _ := ctx.Value(servedLedgerContextKey{}).(*servedLedger)
	return l
}

// Count returns the number of recorded actions.
func (l *searchLedger) Count() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}

// Snapshot returns the recorded actions with seq > after (pass after=0 for
// the full ledger), capped at limit entries starting from the oldest match.
func (l *searchLedger) Snapshot(after, limit int) []searchLedgerEntry {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []searchLedgerEntry
	for _, e := range l.entries {
		if e.seq <= after {
			continue
		}
		out = append(out, e)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

// AddDeepIDs records the identifiers of deep-read chunks. Empty values are
// dropped and repeats collapse (the same chunk read twice is one read), so
// callers can pass raw tool-output matches as-is.
func (l *chunkReadLedger) AddDeepIDs(ids ...string) {
	l.addIDs(true, ids)
}

// AddShallowIDs records the identifiers of shallow-read chunks, same contract
// as AddDeepIDs.
func (l *chunkReadLedger) AddShallowIDs(ids ...string) {
	l.addIDs(false, ids)
}

// addIDs records one tool result's chunk identifiers under the lock, creating
// the id set on first use — so a bare &chunkReadLedger{} records ids too, not
// only one built by the constructor. deep selects which of the two sets.
func (l *chunkReadLedger) addIDs(deep bool, ids []string) {
	if l == nil || len(ids) == 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	set := &l.shallowIDs
	if deep {
		set = &l.deepIDs
	}
	if *set == nil {
		*set = make(map[string]struct{})
	}
	for _, id := range ids {
		if id == "" {
			continue
		}
		(*set)[id] = struct{}{}
	}
}

// Snapshot returns (deepReadChunks, shallowReadChunks) accumulated so far.
func (l *chunkReadLedger) Snapshot() (deep, shallow int) {
	if l == nil {
		return 0, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.deep, l.shallow
}

// ChunkIDs returns the identifiers of the chunks the run read, split by read
// depth and sorted so the payload a benchmark archives is byte-stable across
// runs (same rule as docIDLedger.Snapshot).
//
// A deep-read id means the chunk's FULL content was put in front of the model;
// a shallow one means it appeared as a <match_snippet> window during triage.
// The split mirrors the counts, and neither list is filtered against what the
// deliverable ended up citing: the difference between the two is exactly the
// measurement this exists for, and it is the caller's to draw.
func (l *chunkReadLedger) ChunkIDs() (deep, shallow []string) {
	if l == nil {
		return nil, nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return sortedIDSet(l.deepIDs), sortedIDSet(l.shallowIDs)
}

// sortedIDSet flattens an id set into a sorted slice.
func sortedIDSet(ids map[string]struct{}) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// instrumentedTool decorates a tool.InvokableTool, accumulating the total
// wall-clock duration of every InvokableRun into a shared accumulator keyed by
// the wrapped tool's name, and (when docs is non-nil) recording the documents
// a FULL-CONTENT tool surfaced into the run's retrieval ledger. It embeds
// tool.InvokableTool (which itself embeds tool.BaseTool), promoting Info, then
// overrides InvokableRun to time the call.

// searchActionTools are the corpus tools whose invocations are recorded in
// the run's searchLedger: every retrieval action with query-bearing
// arguments. list_chunks is a read of already-known identifiers (it lands in
// chunkReads instead); web_search is not a corpus action.
var searchActionTools = map[string]struct{}{
	"grep_chunks":            {},
	"search_bm25_chunks":     {},
	"search_semantic_chunks": {},
}

// The ledger is fed by regex over the rendered output rather than by each tool
// reporting its chunks: every locate tool and list_chunks already render
// doc_id= / doc_name= attributes, so one wrapper covers the whole toolset
// without touching any tool implementation. web_search renders url/title
// instead, so web hits are correctly not counted as corpus documents.
type instrumentedTool struct {
	tool.InvokableTool
	acc    *durationAccumulator
	docs   *docIDLedger
	chunks *chunkReadLedger
	// searches records every search action into the run's append-only
	// ledger (see searchLedger) - the ground truth the delivery gate's
	// auditor reads through read_search_ledger.
	searches *searchLedger
	// watch is the run's locate watchdog (see locate_watch.go): it counts
	// locate calls and, when the meaning-based leg is held but never called,
	// appends one reminder to the tool result the model is about to read.
	watch *locateWatch
	// unread is the run's served-unread watchdog (see served_unread_watch.go):
	// when documents pile up served-but-never-read, it names them on the tool
	// result the model is about to read.
	unread *servedUnreadWatch
}

func (t *instrumentedTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	start := time.Now()
	out, err := t.InvokableTool.InvokableRun(ctx, args, opts...)
	cost := time.Since(start)
	name := ""
	if info, ierr := t.InvokableTool.Info(ctx); ierr == nil && info != nil {
		name = info.Name
	}
	t.acc.Add(name, cost)
	if err == nil {
		// Search ledger: record the retrieval action verbatim BEFORE the
		// result is read - the ledger is what WAS searched, append-only,
		// and the auditor reads it through read_search_ledger to judge
		// coverage against facts instead of the deliverable's own
		// reconstruction.
		if _, isSearch := searchActionTools[name]; isSearch && t.searches != nil {
			t.searches.Add(name, args)
		}
		// Retrieval accounting, one implementation for every engine: see
		// retrievalaccount.AccountToolOutput. The ledger split (retrieved vs served, deep
		// vs shallow) and the regexes over the rendered output live in that leaf package,
		// so the mode-8 runner — which only has the MCP result text — classifies
		// identically. It is a leaf because this package pulls in dao, which imports
		// codexagent, so codexagent cannot import this one back.
		acc := retrievalaccount.AccountToolOutput(name, out)
		t.docs.Add(acc.RetrievedDocs...)
		if acc.DeepChunks > 0 {
			t.chunks.AddDeep(acc.DeepChunks)
			t.chunks.AddDeepIDs(acc.DeepChunkIDs...)
		}
		if acc.ShallowChunks > 0 {
			t.chunks.AddShallow(acc.ShallowChunks)
			t.chunks.AddShallowIDs(acc.ShallowChunkIDs...)
		}
	}
	fields := []zap.Field{
		zap.String("tool", name),
		zap.Float64("cost_ms", float64(cost.Milliseconds())),
		// Args are logged IN FULL: the stage-one plan rides check_decomposition's
		// args, and it is the run's single variable state - a truncated copy makes
		// stage-one-vs-delivered comparisons impossible after the fact. Tool args
		// are bounded by construction (queries, ids, the plan), so the volume cost
		// is a few kilobytes per call at most.
		zap.String("args", args),
	}
	if err == nil && t.watch != nil {
		// The locate watchdog rides the result, not the prompt: a rule read
		// once at the top of the system prompt competes with everything else
		// there, while this line appears in the tool result the model is
		// looking at when it decides what to query next.
		if nudge := t.watch.observe(name); nudge != "" {
			out += "\n" + nudge
			fields = append(fields, zap.Int("locate_watchdog", 1))
		}
		if nudge := t.unread.observe(name); nudge != "" {
			out += "\n" + nudge
			fields = append(fields, zap.Int("served_unread_watchdog", 1))
		}
	}
	if err != nil {
		fields = append(fields, zap.Error(err))
	}
	common.DebugCtx(ctx, "agentic_rag: tool call", fields...)
	return out, err
}

// Run executes a smart-reasoning (ReAct) turn against the given model and
// returns the final assistant message content, streaming incremental deltas
// through in.OnDelta (or runtime.EmitAgentMessage when OnDelta is nil).
//
// One call answers ONE user turn: the caller's stored conversation comes in
// through in.Messages and the next user input arrives as the NEXT call
// (ChatPipelineService.AsyncChat is re-entered per request). Keeping the
// history compact is therefore the caller's job at the turn boundary — a
// stored assistant message must hold the turn's final answer, not the ReAct
// trajectory that produced it (see appendAssistantToSession's callers).
//
// It uses eino ADK's adk.ChatModelAgent (NOT flow/agent/react, and never
// adk/react.go directly), which provides the ReAct loop plus robustness
// (retry/failover/cancel monitoring) and recoverability (checkpoint/resume).
func Run(ctx context.Context, in Input) (string, error) {
	if in.Model == nil {
		return "", errNilModel
	}
	if in.DocIDs != nil {
		ctx = withDocumentScope(ctx, in.DocIDs)
	}
	// Resolve the corpus scope ONCE per turn: every corpus tool of this run must
	// read the indexes that actually hold the bound datasets, which is decided by
	// each dataset's own tenant (a team-shared dataset lives in the sharing
	// tenant), not by the conversation's tenant.
	ctx = WithDatasetTenants(ctx, ResolveDatasetTenants(ctx, in.DatasetIDs))

	// Conversation identity for per-conversation compiled-knowledge tool
	// availability: the chat pipeline passes the chat id; when absent, a unique
	// per-run key keeps any availability decision scoped to this turn and
	// isolated from concurrent runs.
	convKey := in.ConversationKey
	generatedKey := convKey == ""
	if generatedKey {
		convKey = fmt.Sprintf("run-%d", atomic.AddUint64(&runSeq, 1))
	}
	ctx = WithConversationKey(ctx, convKey)
	// The turn's compiled-evidence state: the scope claims are looked up in,
	// the corpus-level existence probe, and the claims already shown. Installed
	// here so the pre-loop probe and every locate tool share one scope.
	ctx = withCompiledRun(ctx, in.TenantID, in.DatasetIDs)

	// Identity the judge is chosen for: the decision model is configured per chat
	// and per tenant in the UI, so the filter has to know which run it is
	// serving. The generated key above is a per-turn placeholder and must not be
	// used here — a run with no real chat id falls back to the tenant default,
	// exactly as a chat that never overrode it does.
	ctx = withDecisionScope(ctx, in.TenantID, in.ConversationKey, in.UserID)
	if generatedKey {
		// The generated key belongs to this turn alone: no later run reuses it,
		// so release its availability marks on the way out instead of leaking
		// one dead entry per keyless run into the process-wide map.
		defer forgetConversation(convKey)
	}

	var tmpl Template
	if in.Template != nil {
		tmpl = *in.Template
	} else {
		var err error
		tmpl, err = ResolveTemplate(in.TemplateID, nil)
		if err != nil {
			common.ErrorCtx(ctx, "agentic_rag: resolve template", err)
			return "", err
		}
	}

	// Answer-audit gate state: created for templates that declare
	// `audit_max_pass: N` (N > 0), which is both the switch and the budget —
	// how many audit passes the gate may spend. The auditor is a STANDALONE
	// agent deliberately kept out of the main agent's toolset — the agent only
	// produces the FINAL message, and the delivery gate drives the auditor
	// itself (see runDeliveryGate), so compliance cannot be defied. An explicit
	// tool set means the caller owns the toolset, so the template's auditor
	// does not apply.
	auditMaxPass := tmpl.AuditMaxPass
	if len(in.Tools) > 0 {
		auditMaxPass = 0
	}
	var auditorAgent adk.Agent

	// Shared per-run tool ledgers: initialized BEFORE the auditor is built so
	// its deep-reads land in the same accumulators as the explorer's calls.
	if in.ToolCallDurations == nil {
		in.ToolCallDurations = &durationAccumulator{dur: make(map[string]time.Duration)}
	}
	if in.RetrievedDocIDs == nil {
		in.RetrievedDocIDs = NewDocIDLedger()
	}
	if in.ChunkReads == nil {
		in.ChunkReads = NewChunkReadLedger()
	}
	if in.Searches == nil {
		in.Searches = NewSearchLedger()
	}
	if in.Serves == nil {
		in.Serves = NewServedLedger()
	}

	// The serve ledger rides the context: every retrieval tool's serve point
	// records what it put in front of the model, so the delivery gate can
	// diff served documents against the deep-read set.
	ctx = withServedLedger(ctx, in.Serves)

	// Stage one: the planner writes the block plan and
	// has it verified BEFORE the explorer runs, so planning is never left
	// to the explorer's discipline - two measured runs (#11/#12) showed an
	// in-prompt call contract the model never exercised (zero
	// check_decomposition calls) and shipped a merged two-variable block (q875)
	// and a plan of four disconnected roots (q1005). It runs after the shared
	// ledgers exist so the stage's checker calls land in the same duration
	// accounting as the explorer's. The stage is skipped for caller-owned
	// toolsets (a different product shape, like the auditor) and for templates
	// that do not decompose.
	// plannedRun decides the WHOLE shape of this turn: whether the plan stage
	// runs, whether the explorer's input is the plan or the raw question,
	// whether a missing plan is an error, and whether the FOS answer-line
	// contract applies. It must be one condition, not five similar ones: it
	// used to be `TemplateID == PlanProducerTemplateID` at some sites and
	// `... && len(in.Tools) == 0` at others, so a plan-producer template with a
	// caller-owned toolset skipped planning and then failed with "plan stage
	// returned no plan" before it ever invoked the caller's tools.
	//
	// A caller-owned toolset means the caller supplies its own product shape
	// (the auditor is one), and that shape has no plan to render — which is
	// also why such a run gets the raw history, not the plan sections.
	plannedRun := in.TemplateID == PlanProducerTemplateID && len(in.Tools) == 0
	plan, conversational, planStageErr := "", false, error(nil)
	if plannedRun {
		plan, conversational, planStageErr = runPlanStage(ctx, in)
	}
	if conversational {
		// The stage classified the message as purely conversational: the
		// explorer-auditor pipeline does not run for a greeting - no ReAct
		// loop, no retrieval tools, no delivery gate. One direct generation
		// over the caller's history answers it in plain prose.
		return runConversationalReply(ctx, in)
	}
	resolvedQuestion := extractResolvedQuestion(plan)
	if resolvedQuestion == "" {
		// Single-turn run, or a planner that skipped the line: the raw last
		// user message is the best question on file.
		resolvedQuestion = lastUserQuestion(in.Messages)
	}

	// With no explicit tools, use the requested template. The config reloads
	// when its mtime changes so operators can change the tool list and prompt.
	// toolsFor (inside toolsForRun) also injects web_search from the context
	// when the conversation has a search provider, so that tool is never
	// declared in the template.
	tools := toolsForRun(ctx, tmpl, in, engine.Get())
	agentModel, err := modelForTools(ctx, in.Model, tools, in.DatasetIDs)
	if err != nil {
		return "", err
	}

	// The auditor is built for templates that declare `audit_max_pass: N` (N > 0);
	// an explicit tool set already zeroed auditMaxPass above, because the caller
	// then owns the toolset. It runs on the same per-run ledgers as the explorer
	// (initialized above), and its system prompt pins THIS run's resolved question.
	if auditMaxPass > 0 {
		inner, aerr := NewAnswerAuditorAgent(ctx, auditModelFor(in),
			in.TenantID, in.DatasetIDs, resolvedQuestion,
			in.ToolCallDurations, in.RetrievedDocIDs, in.ChunkReads,
			in.Searches)
		if aerr != nil {
			// auditMaxPass drops to 0: no gate auditing.
			common.WarnCtx(ctx, "agentic_rag: answer auditor unavailable", zap.Error(aerr))
			auditMaxPass = 0
		} else {
			auditorAgent = inner
		}
	}

	maxIter := agenticMaxIterations(in.MaxIterations)

	// The compiled-evidence probe runs BEFORE the loop: search once, then ask the
	// model whether the claims COMPILED FROM what that search returned already
	// answer the question. A refusal falls back to the loop, which is the only
	// place that decides what to search next.
	//
	// The ordering is the point. The search is the same hybrid chunk retrieval
	// the locate tools run, so it works identically on a corpus with compiled
	// artifacts and one without; what differs is only what is offered next. With
	// claims, the turn can terminate on a single model call over them - the
	// claims carry their own verbatim evidence, so no raw chunk has to be read.
	// Without them (or when the model judges them insufficient) the loop runs
	// exactly as it would have, and the model reads chunks itself.
	//
	// The probe is a control signal, not prompt text: an insufficient block is
	// dropped here rather than pinned into the system instruction, where it
	// would be re-billed on every call of the turn.
	if len(in.Tools) == 0 && !tmpl.strictTools {
		pf := prefetchCompiledEvidence(ctx, in.TenantID, in.DatasetIDs, in.Messages)
		if len(pf.claims) > 0 {
			answer, aerr := answerFromClaims(ctx, synthModelFor(in), tmpl.Content, in.Messages, pf)
			switch {
			case aerr != nil:
				// Falling through keeps the turn answerable: the claims are
				// dropped from the registry's numbering (nothing was shown) and
				// the loop retrieves what it needs itself.
				common.WarnCtx(ctx, "agentic_rag: compiled claim answer failed; falling back to the loop", zap.Error(aerr))
			case strings.TrimSpace(answer) != "":
				// The loop is skipped entirely, so this line is the only record
				// that the turn never read a chunk: a review of tool usage must
				// be able to tell it from a turn the model gave up on.
				common.InfoCtx(ctx, "agentic_rag: answered from compiled claims",
					zap.Int("claims", len(pf.claims)),
					zap.Int("chunks", len(pf.chunks)),
					zap.Strings("docs", claimDocsInOrder(pf.claims)),
					zap.Int("handles", claimHandleCount(answer)),
					zap.Int("answer_bytes", len(answer)))
				emit(ctx, in.OnDelta, answer, "")
				return answer, nil
			default:
				// The claims were judged (or proved) not to answer the question:
				// the loop retrieves what the claims could not supply.
				common.InfoCtx(ctx, "agentic_rag: falling back to the loop after an unusable claim answer",
					zap.Int("claims", len(pf.claims)))
			}
		}
	}

	// Wrap every invokable tool so each call is timed, emitted as a debug log,
	// accumulated into in.ToolCallDurations, and (for the full-content tools
	// search_chunks/list_chunks) has the documents it surfaced recorded into
	// in.RetrievedDocIDs. Tools that aren't InvokableTool (e.g.
	// streamable-only) are passed through unwrapped — all agent tools here are
	// InvokableTool.
	// The locate watchdog may only advertise the meaning-based leg if THIS run
	// holds it, so availability is established while the tools are wrapped —
	// before the model can call anything (see locate_watch.go).
	watch := newLocateWatch()
	unreadWatch := newServedUnreadWatch(in.Serves, in.RetrievedDocIDs)
	wrapped := make([]tool.BaseTool, len(tools))
	for i, t := range tools {
		if it, ok := t.(tool.InvokableTool); ok {
			// The locate watchdog may only advertise the meaning-based leg if
			// THIS run actually holds it (see locate_watch.go).
			if info, ierr := it.Info(ctx); ierr == nil && info != nil && info.Name == searchSemanticChunksToolName {
				watch.markSemanticAvailable()
			}
			timed := &instrumentedTool{
				InvokableTool: it,
				acc:           in.ToolCallDurations,
				docs:          in.RetrievedDocIDs,
				chunks:        in.ChunkReads,
				searches:      in.Searches,
				watch:         watch,
				unread:        unreadWatch,
			}
			wrapped[i] = timed
		} else {
			wrapped[i] = t
		}
	}
	tools = wrapped

	// Drop compiled-knowledge tools this conversation has already proven
	// unusable (dataset-level absence), so the model cannot keep calling them
	// and wasting turns. The tool still short-circuits on its own for any call
	// that slips through, but removing it from the list is the clean stop.
	if unavailable := navUnavailableToolSet(ctx); len(unavailable) > 0 {
		kept := tools[:0]
		for _, t := range tools {
			if info, ierr := t.Info(ctx); ierr == nil && info != nil && unavailable[info.Name] {
				continue
			}
			kept = append(kept, t)
		}
		tools = kept
	}
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		if info, err := t.Info(ctx); err == nil && info != nil {
			names = append(names, info.Name)
		}
	}
	common.InfoCtx(ctx, "agentic_rag: configured runtime tools",
		zap.String("template", tmpl.ID), zap.Bool("strict_tools", tmpl.strictTools), zap.Strings("tools", names))

	common.DebugCtx(ctx, "agentic_rag: run start",
		zap.Int("max_iterations", maxIter),
		zap.Int("messages", len(in.Messages)),
		zap.Int("tools", len(tools)),
	)

	cfg := &adk.ChatModelAgentConfig{
		Name:          "smart-reasoning",
		Instruction:   tmpl.Content,
		Model:         agentModel,
		MaxIterations: maxIter,
		// Retry transient provider failures (MiniMax 529 overload, 429, 5xx,
		// network hiccups) with seconds-scale backoff on EVERY model call of
		// this agent — main-loop research turns, repair turns, and finalize
		// alike. Without it one 529 mid-research aborts a 100-iteration run.
		ModelRetryConfig: agentModelRetryConfig(),
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: tools,
				// Execute same-round tool calls (multiple tool_calls in one model
				// response) concurrently rather than one-after-another. This is
				// eino's default (false), but we state it explicitly so the
				// parallel intent is not accidental. All of these tools
				// (think/todo_write/grep_chunks/search_bm25_chunks/search_semantic_chunks/list_chunks/run_javascript/search_metadata)
				// are concurrency-safe: they keep no shared mutable state across
				// calls (run_javascript creates a fresh goja VM per invocation).
				ExecuteSequentially: false,
				// A tool that is not registered is not a reason to abort the run.
				//
				// web_search is only registered when the conversation enabled
				// internet, but the shared prompt mentions it, so the model
				// sometimes calls it anyway. Without this handler eino raises
				// "tool web_search not found in toolsNode indexes" and the whole
				// turn dies — the question then scores zero even though the
				// corpus tools were working. Answering with a tool result lets
				// the model switch legs and keep reading.
				UnknownToolsHandler: unknownToolResult,
			},
		},
	}

	explorerAgent, err := adk.NewChatModelAgent(ctx, cfg)
	if err != nil {
		return "", err
	}

	// explorer/auditor are driven as MANAGED SESSIONS: each turn hands over
	// nothing but its new messages and the Runner replays everything older from
	// the session's event log (see run_session.go). The caller above therefore
	// never reassembles a transcript — no history concatenation, no tail
	// trimming, no tool-result reordering.
	sess := newRunSession()

	// EnableStreaming lives on RunnerConfig, not ChatModelAgentConfig.
	explorerHead := sess.explorer.head(ctx)
	// The plan pipeline has ALREADY produced the plan in the stage above:
	// reaching this point without one means the stage failed after its retries.
	// Assert the plan instead of degrading to an unpinned explorer - the
	// unpinned run is exactly how the merged-block and guessed-answer defects
	// reached the deliverable in #11/#12. The plain-explorer shape (every other
	// template, and a caller-owned toolset) legitimately has no plan: it takes
	// the caller's own question as its input below.
	//
	// planBody is the plan section the delivery gate re-reads; it stays empty
	// for the plain-explorer shape.
	planBody := ""
	var runMessages []adk.Message
	if plannedRun {
		if plan == "" {
			// The stage's own error rides along: a model-chain death (529
			// overload exhausting the failover chain) makes the run error
			// self-describing instead of a bare "returned no plan" (#44: 22
			// such rows whose provider cause was buried in a rotated log).
			// The question itself never enters the error text: the serving
			// layers log this error at ERROR/INFO level, so it would put the
			// user's private question into general service logs. The wrapped
			// cause (the provider error) is what makes the message useful.
			if planStageErr != nil {
				return "", fmt.Errorf("agentic_rag: plan stage returned no plan: %w", planStageErr)
			}
			return "", fmt.Errorf("agentic_rag: plan stage returned no plan")
		}
		// The run's input is ONE user message in a fixed two-section format the
		// producer prompt declares: `## Resolved question` (the stage-one
		// resolved question — the caller's question with every conversational
		// reference resolved — falling back to the raw last user message when
		// the plan carries no resolved line) and `## The question decomposition`
		// (the stage-one plan). Both sections are DATA - no instructions ride
		// along - so the format itself tells the explorer what it is looking at.
		planBody = plan
		if q := extractResolvedQuestion(plan); q != "" && strings.HasPrefix(plan, resolvedQuestionHeader) {
			// The resolved question moves to the input's own heading; the plan
			// section starts at its first block.
			if nl := strings.Index(planBody, "\n"); nl >= 0 {
				planBody = strings.TrimSpace(planBody[nl+1:])
			}
		}
		formatted := "## Resolved question\n\n" + resolvedQuestion + "\n\n## The question decomposition\n\n" + planBody
		planMsg := schema.UserMessage(formatted)
		// The plan is text, but the turn may carry IMAGE attachments, and the
		// step above replaces the caller's messages with the plan alone. The
		// retrieval tools search the corpus and cannot read an attachment, so
		// dropping the image here would lose evidence the model can only see.
		// Carry the caller's multimodal parts onto the plan message.
		planMsg.UserInputMultiContent = carryImageParts(in.Messages)
		runMessages = []adk.Message{planMsg}
	} else {
		// Plain explorer: the turn hands over only the current question and lets
		// the managed session replay the caller's history.
		question := schema.UserMessage(lastUserQuestion(in.Messages))
		seed := in.Messages
		if len(seed) > 0 {
			question = seed[len(seed)-1]
			// The final user message is the new turn and must be appended exactly
			// once by turnMessages.
			seed = seed[:len(seed)-1]
		}
		runMessages = sess.explorer.turnMessages(seed, "", question)
	}
	// Everything the ReAct loop calls the model for is the exploration phase:
	// the loop is where a turn's token budget is actually spent, so it must be
	// attributable on its own and not merge into the final synthesis.
	exploreCtx := tokenizer.WithUsagePhase(ctx, "explore")
	iter := sess.explorer.runner(exploreCtx, explorerAgent, in.Stream).Run(exploreCtx, runMessages)
	// Runner assigns IDs to input messages before persisting them. Exclude
	// these from recovery so a failed turn cannot reuse a historical answer.
	inputMessageIDs := make(map[string]bool, len(runMessages))
	for _, msg := range runMessages {
		inputMessageIDs[adk.GetMessageID(msg)] = true
	}
	final, evidence, runErr := consumeAgentEvents(ctx, iter, in.OnDelta, in.ToolCallCounts, in.ToolCallErrors, in.ToolErrorSamples)
	if runErr != nil {
		runErr = fmt.Errorf("agentic research failed: %w", withCancellationCause(ctx, runErr))
	}

	// Recovery, cheapest and most faithful first. A run that ended on a
	// narration tail or on a bare tool call still holds the model's own answer
	// somewhere in its turns — shipping it beats a synthesis call, which
	// discards that work and, once a failover cooldown has set in, may not
	// even reach a provider (it would be short-circuited with the stale error
	// that tripped the cooldown).
	// abortedTurn is captured BEFORE recovery clears runErr: an aborted turn
	// whose answer was recovered is still an aborted turn. It issued a tool
	// call whose result never arrived, and that unmatched call replays as a
	// provider error on the next turn, so the rollback below must run even
	// when the carried answer makes the turn's RESULT usable.
	abortedTurn := runErr != nil
	if final == "" || runErr != nil {
		if carried := sess.explorer.lastAssistant(ctx, func(m *schema.Message) bool {
			return !inputMessageIDs[adk.GetMessageID(m)] && strings.TrimSpace(m.Content) != ""
		}); carried != "" {
			final = carried
			runErr = nil
		}
	}
	if abortedTurn {
		// The aborted turn's events must not reach the next one: a tool call
		// whose result never arrived is a provider error on replay. This also
		// drops the caller's history from the session, so the first repair
		// turn has to carry it again (conversation.turnMessages).
		sess.explorer.discardFailedTurn(ctx, explorerHead)
	} else {
		sess.explorer.markSeeded()
	}

	// Delivery gate. The agent's job ended with its FINAL message; the gate
	// OWNS verification: every pass audits the standing final with
	// answer_auditor, ships on PASS, and on FAIL hands the audit findings back
	// to the agent as one repair directive. Nothing depends on the model
	// choosing to call the auditor.
	//
	// The gate runs even when the main loop aborted (runErr != nil): an aborted
	// run typically leaves a narration tail as the final, which is exactly the
	// answer-less output the gate exists to catch. Skipping it there handed
	// process narration straight to the user (observed on q55, where one
	// grep_chunks error aborted the loop right before the synthesis step).
	preGateFinal := final
	// A planned repair re-renders the SAME deliverable the first turn was asked
	// for, so it seeds from that turn's input; a plain repair seeds from the
	// caller's history. See deliveryGateInput.seedMessages.
	repairSeedMessages := in.Messages
	if plannedRun {
		repairSeedMessages = runMessages
	}
	gateFinal, auditedFinal, gateRejected, gateModelErr := runDeliveryGate(ctx, deliveryGateInput{
		explorer:         explorerAgent,
		auditor:          auditorAgent,
		sess:             sess,
		baseMessages:     in.Messages,
		seedMessages:     repairSeedMessages,
		final:            final,
		auditMaxPass:     auditMaxPass,
		toolCallCounts:   in.ToolCallCounts,
		toolCallErrors:   in.ToolCallErrors,
		toolErrorSamples: in.ToolErrorSamples,
		reads:            in.ChunkReads,
		searches:         in.Searches,
		serves:           in.Serves,
		deepDocs:         in.RetrievedDocIDs,
		planBody:         planBody,
		audit:            in.GateAudit,
	})
	if runErr != nil && gateFinal != preGateFinal {
		runErr = nil
	}
	final = gateFinal
	if !hasAnswerLine(final) {
		// Recovery ladder, cheapest and most faithful first. A run that ends
		// on a narration tail or on a bare tool call still holds an answer
		// somewhere in its own turns, and that answer is the model's own
		// deliverable — complete with the Candidate Matrix and the citations
		// the auditor checks. Shipping it beats a synthesis call, which
		// discards that structure and, once a failover cooldown has set in,
		// may not even reach a provider (it would be short-circuited with the
		// stale error that tripped the cooldown).
		// Both scans exclude the seeded input messages: the caller's chat
		// history is persisted into the session as input events before the
		// turn runs, so without the exclusion a labeled answer from an
		// EARLIER turn would be recovered as this turn's deliverable.
		// A continuation the gate REFUSED must not be recovered: it is still
		// in the session (its tool calls carried evidence, so it was not
		// rolled back), and a bare answer line satisfies hasAnswerLine — so
		// without this exclusion a rejected repair could ship as the final
		// answer and, carrying an answer line, suppress the synthesis that
		// would have produced a real one.
		notRejected := func(m *schema.Message) bool {
			_, wasRejected := gateRejected[contentFingerprint(m.Content)]
			return !wasRejected
		}
		carried := sess.explorer.lastAssistant(ctx, func(m *schema.Message) bool {
			return notRejected(m) && !inputMessageIDs[adk.GetMessageID(m)] &&
				strings.TrimSpace(m.Content) != "" && hasAnswerLine(m.Content)
		})
		switch {
		case hasFOSStructure(auditedFinal):
			// The deliverable the auditor last examined first: the gate has
			// been iterating on it, so it is the most refined FOS the run
			// produced, and it is the one the auditor's findings describe.
			final = auditedFinal
		case carried != "":
			final = carried
		default:
			final = sess.explorer.lastAssistant(ctx, func(m *schema.Message) bool {
				return notRejected(m) && !inputMessageIDs[adk.GetMessageID(m)] && strings.TrimSpace(m.Content) != ""
			})
		}
		if final != "" {
			common.WarnCtx(ctx, "agentic_rag: final carried no answer line, recovered an earlier deliverable",
				zap.Int("recovered_bytes", len(final)))
		}
	}
	// The delivery hard block is GONE (user decision, 2026-10-03). A gate FAIL
	// used to replace the answer line - and everything riding on it - with a
	// "not derivable from the corpus" declaration. Measured on smoke49 the run
	// had served the gold document, cited it and shipped the correct value in
	// its matrix, and still lost the answer line to ONE finding the auditor had
	// graded advisory; the official judge reads the answer TEXT, so the
	// declaration scored 0 where the candidate would have scored 1. The gate's
	// verdict is not lost: GateAudit.Passed and the verdict excerpts keep a
	// FAIL visible in the archived row and the log, and the answer ships as its
	// author wrote it.
	if runErr != nil && final != preGateFinal {
		// The gate adopted a fresh substantive continuation, so the main
		// loop's error no longer describes the answer being returned.
		runErr = nil
	}
	// Label governance is deliberately NOT applied here: it runs ONCE, at the
	// end, on whatever text actually ships. Governing before the synthesis let
	// a synthesized deliverable escape it entirely - w3 shipped `Final Answer`
	// from the synthesizer on a run whose gate never audited at all (4 pre-audit
	// rejections, no verdict), while the log showed a demotion on the
	// pre-synthesis draft the synthesizer then replaced.

	// Terminating action: the gate loop has spent its budget (or every
	// continuation failed) and the answer still carries no machine-parseable
	// FOS deliverable. Synthesize one deterministically from the question,
	// whatever partial output exists, and the evidence gathered so far — the
	// turn must never end on narration or a blank.
	// The FOS answer-line contract belongs to the plan producer's deliverable
	// format (## Candidate Matrix / Final Answer). A plain-explorer reply is
	// prose by design, so for every other shape a missing answer line is not a
	// defect and must not trigger a synthesis call - the model's own answer
	// ships as written. A BLANK or errored turn is still recovered for every
	// shape: the turn must never end on nothing.
	needsFOS := plannedRun && !hasAnswerLine(final)
	if final == "" || runErr != nil || needsFOS {
		// Detach from the run's cancellation entirely: the fallback fires most
		// often BECAUSE the shared wall-clock budget expired, and a deadline
		// expiry and a client hang-up are indistinguishable to the derived
		// context (both close the same Done channel) — keeping cancellation
		// would reintroduce the failure this call exists to survive. The
		// unbounded-work risk that WithoutCancel normally carries is covered
		// by the bounded budget attached here.
		synthCtx, synthCancel := context.WithTimeoutCause(context.WithoutCancel(ctx), finalizeTimeout, fmt.Errorf("final answer synthesis timed out after %s", finalizeTimeout))
		defer synthCancel()
		synthCtx = tokenizer.WithUsagePhase(synthCtx, "synth")
		synthMessages := in.Messages
		if tmpl.customContent {
			synthMessages = append([]*schema.Message{schema.SystemMessage(tmpl.Content)}, synthMessages...)
		}
		// structuredOnly is true only for a PLANNED run whose partial really is
		// a structured deliverable. On the plain-explorer path hasFOSStructure
		// is true even for an empty partial, and asking the model to render a
		// FOS deliverable there mislabels ordinary prose as an audited answer.
		structuredOnly := needsFOS || (plannedRun && hasFOSStructure(final))
		synth, synthErr := finalizeAnswer(synthCtx, synthModelFor(in), synthMessages, final, evidence, structuredOnly)
		if synthErr == nil && strings.TrimSpace(synth) == "" {
			synthErr = errors.New("empty answer")
		}
		if synthErr != nil {
			synthErr = fmt.Errorf("final answer synthesis failed: %w", withCancellationCause(synthCtx, synthErr))
			// Without this an empty final is completely silent: the run looks
			// healthy in every other log line and the user simply gets nothing.
			common.WarnCtx(ctx, "agentic_rag: finalizeAnswer failed", zap.Error(synthErr))
		}
		// The FOS answer-line acceptance only applies to the plan producer's
		// deliverable format (## Candidate Matrix / Final Answer). A plain-explorer
		// synthesis is prose by design, so it ships as written.
		strictFOS := plannedRun
		switch {
		case synthErr == nil && (!strictFOS || (synth != "" && answerLineCount(synth) == 1)):
			// Replace rather than append: appending leaves the reader with a
			// narration paragraph followed by the real answer. The single
			// answer emission at the end of Run carries it.
			final = synth
			runErr = nil
		case synthErr == nil && synth != "":
			// The synthesis DEGENERATED — zero or multiple FOS answer lines
			// (observed: the model re-rendered the whole deliverable inside a
			// `Final Answer: **## ...**` mega-value, shipping two Final
			// Answer lines). The pre-synthesis deliverable is the model's own
			// structured work and ships instead of the blob.
			common.WarnCtx(ctx, "agentic_rag: finalizeAnswer degenerated, keeping the standing deliverable",
				zap.Int("synth_bytes", len(synth)),
				zap.Int("answer_lines", answerLineCount(synth)))
			if final != "" {
				runErr = nil
			}
		case runErr == nil:
			// Normal termination but synthesis failed — surface the reason.
			runErr = synthErr
		default:
			runErr = fmt.Errorf("%w; %w", runErr, synthErr)
		}
	}

	// The label is the MODEL's claim and the AUDITOR's judgement, and the gate does not
	// touch it. `Final Answer` asserts that every discriminating constraint is
	// corpus-verified; the auditor's contract fails that assertion when the record does
	// not establish them (`Final Answer claims corpus-verification the record does not
	// establish`), and its repair directive tells the producer to state a label it can
	// support. The pipeline used to do that edit itself — `demoteFinalAnswerLabel`
	// spliced the word with a regex and appended a canned note — which is the pipeline
	// rewriting the deliverable rather than the deliverable's author correcting it. What
	// the gate keeps is the VERDICT, so a `Final Answer` that ships without a PASS is at
	// least visible in the log and in the archived row (GateAudit.Passed).
	observeUnverifiedFinalLabel(ctx, final, in.GateAudit)

	// Diagnosis, not enforcement: a locate tool the run called but the matrix
	// never credits leaves nothing in the archived row to show it was used. On
	// #71 the ONE call that surfaced the answer's own document was a
	// search_semantic_chunks query the matrix omitted, so only the server log
	// revealed that the pure-vector leg had done the work. Two of three runs
	// trip this (see unrecordedLocateTools), which is why it warns instead of
	// rejecting: the deliverable's job is the answer, and a bookkeeping
	// omission that the schema does not enforce is not worth a repair pass.
	if missing := unrecordedLocateTools(final, in.ToolCallCounts); len(missing) > 0 {
		common.WarnCtx(ctx, "agentic_rag: deliverable omits locate calls it used",
			zap.Strings("tools", missing))
	}

	// The answer channel carries exactly one thing: the deliverable this run
	// is shipping, emitted once, now that the gate and the fallbacks have
	// settled on it. Everything the loop said on the way there already went
	// out live as thinking, so the user watched the work without being handed
	// several competing "Final Answer" blocks.
	// Upper-layer check of turn-level errors — the ONE place model-caused
	// turn deaths are attributed: a model failure that killed a gate turn
	// (repair turn, audit round) surfaces into the run error, so the row's
	// ragflow_error names the provider instead of reading like a content
	// refusal (#44: 39 such rows). The gate certifying the answer clears it —
	// a recovered run is degraded, not failed.
	if gateModelErr != nil && (in.GateAudit == nil || !in.GateAudit.Passed) && runErr == nil {
		runErr = fmt.Errorf("agentic_rag: %w", gateModelErr)
		common.WarnCtx(ctx, "agentic_rag: model failure killed a gate turn — surfaced into the run error",
			zap.Int("model_killed_turns", in.GateAudit.ModelKilledTurns),
			zap.String("cause", in.GateAudit.LastModelKill), zap.Error(gateModelErr))
	}
	emit(ctx, in.OnDelta, final, "")
	return final, runErr
}

// Eino wraps ctx.Err(), which omits cancellation causes such as token exhaustion.
func withCancellationCause(ctx context.Context, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		if cause := context.Cause(ctx); cause != nil && !errors.Is(err, cause) {
			return fmt.Errorf("%w: %w", cause, err)
		}
	}
	return err
}

// adoptableContinuation reports whether a repair continuation may replace the
// standing deliverable: it must carry the FOS SECTIONS. An answer line alone may
// not - a bare "Guessed Answer: I do not have sufficient evidence" satisfies that
// while dropping every cited chunk, and adopting it threw away a structured
// deliverable to buy a re-audit that could only repeat the coverage defect
// (q1093 burned 16 passes that way).
//
// The answer VALUE is deliberately not required. A matrix without its answer line
// is auditable, and the auditor's `schema integrity: answer is missing` names the
// defect precisely; requiring the value here instead rejected the continuation
// three times and then gave up - q490, q784 and q872 all ended that way, never
// audited, on a deliverable that only needed its last line restored.
func adoptableContinuation(text string) bool {
	return hasFOSStructure(text)
}

var modelFailureRe = regexp.MustCompile(`exceeds max retries|failed on every model|short-circuited by failover cooldown|overloaded_error|status 529|insufficient balance`)

// modelFailure reports whether an error means the model chain, not the run's
// content, failed the call.
func modelFailure(err error) bool {
	return err != nil && modelFailureRe.MatchString(err.Error())
}

// ModelFailure reports whether an error means the model chain, not the run's
// content, failed the call. Exported for the chat pipeline's group failover:
// a turn that died on a model failure flips the gp- primary's current member
// inactive so the next run's chain picks the next member.
func ModelFailure(err error) bool {
	return modelFailure(err)
}

// modelCause extracts the provider-facing reason from a model-failure error,
// for the one-line record the question's failure carries. A non-model
// failure has no model cause.
func modelCause(err error) string {
	if err == nil || !modelFailure(err) {
		return ""
	}
	s := err.Error()
	if m := regexp.MustCompile(`status (\d+)`).FindStringSubmatch(s); m != nil {
		if strings.Contains(s, "overloaded_error") {
			return fmt.Sprintf("provider overloaded (HTTP %s)", m[1])
		}
		return fmt.Sprintf("provider error (HTTP %s)", m[1])
	}
	return "model chain failure"
}

// countModelKilledTurn records a gate-loop turn (repair turn, audit round)
// that a model-chain failure killed outright, with the provider-facing cause.
// The attribution happens ONCE at the run's end (see Run's model-failure
// record), never per turn.
func countModelKilledTurn(audit *GateAuditRecord, cause string) {
	if audit != nil {
		audit.ModelKilledTurns++
		audit.LastModelKill = cause
	}
}

// countGateAuditFailure records a deliverable refused for citing chunks the run
// never received. See GateAuditRecord.UnreadCitations.
func countUnreadCitation(audit *GateAuditRecord) {
	if audit != nil {
		audit.UnreadCitations++
	}
}

// countChainBreak records a deliverable refused for a Survivors chain-order
// violation. See GateAuditRecord.ChainBreaks.
func countChainBreak(audit *GateAuditRecord) {
	if audit != nil {
		audit.ChainBreaks++
	}
}

func countGateAuditFailure(audit *GateAuditRecord) {
	if audit != nil {
		audit.AuditFailures++
	}
}

// countGateRejection records a deliverable the gate refused before any audit
// ran, so "the gate never concluded PASS" survives into the label governance
// and the benchmark's accounting even when the auditor never got a turn.
// carryImageParts collects the image parts of the caller's messages so a
// text-only planned input can still carry the turn's attachments. Only image
// parts are taken: the text of those messages is superseded by the plan.
func carryImageParts(msgs []*schema.Message) []schema.MessageInputPart {
	var parts []schema.MessageInputPart
	for _, m := range msgs {
		for _, p := range m.UserInputMultiContent {
			if p.Type == schema.ChatMessagePartTypeImageURL {
				parts = append(parts, p)
			}
		}
	}
	return parts
}

// contentFingerprint identifies a piece of model output by a short hash, so a
// large draft can be remembered without holding its text. Used to keep the
// recovery ladder from resurrecting a continuation the gate refused.
func contentFingerprint(text string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.TrimSpace(text))))
}

func countGateRejection(audit *GateAuditRecord) {
	if audit != nil {
		audit.Rejections++
	}
}

// observeUnverifiedFinalLabel records a `Final Answer` that ships without a passing
// audit. It is an OBSERVATION, never a rewrite: the label is the deliverable author's
// claim and the auditor's judgement, so the pipeline does not splice it (it used to —
// see the removed `demoteFinalAnswerLabel`, a regex that rewrote the word and appended
// a canned English note). What the gate knows deterministically is its own verdict, and
// this line puts it in the record: a run that shipped a `Final Answer` the gate never
// PASSed is visible instead of silent.
func observeUnverifiedFinalLabel(ctx context.Context, final string, audit *GateAuditRecord) {
	if answerLabel(final) != "final" {
		return
	}
	if audit != nil && audit.Passed {
		return
	}
	rounds, rejections, failures := 0, 0, 0
	if audit != nil {
		rounds, rejections, failures = len(audit.Suspects), audit.Rejections, audit.AuditFailures
	}
	common.WarnCtx(ctx, "agentic_rag: shipping a Final Answer label the gate never PASSed",
		zap.Int("audit_rounds", rounds), zap.Int("rejections", rejections),
		zap.Int("audit_failures", failures))
}

// gateNoProgressLimit bounds CONSECUTIVE repair attempts that fail to advance
// the deliverable - a continuation that aborted on a tool error, went silent,
// or ended in narration without the FOS answer line. It is the gate's single
// "this is not working" budget, and such attempts are retried IN PLACE: they
// leave the deliverable unchanged, so re-auditing first would only reproduce
// the verdict already in hand (a re-audit is only worth running after the
// final has actually changed). Reaching the limit means the model cannot
// render or the tool layer is down; either way the gate gives up and falls
// through to finalizeAnswer.
//
// It is NOT redundant with the suspect-count stall check below, and removing
// it hangs the gate: `pass` counts only ADOPTED repairs, so a model that keeps
// emitting unusable continuations never advances the loop and the audit ceiling
// can never be reached; and since those attempts are retried in place they produce
// no verdict, so the suspect trend has nothing to observe. This counter is the
// only exit that does not wait for the run's deadline. Measured across the
// frames runs: 12 give-ups here against 5 stall cut-offs — disjoint paths.
const gateNoProgressLimit = 3

// gateStallWindow is how many consecutive audits are weighed to decide
// whether the suspect count is still falling (see the stall check in
// runDeliveryGate). With the last three counts f1, f2, f3 (oldest first), the
// repair is still working only while f3 is below f1 or f2 - the newest audit
// found fewer problems than at least one of the two before it. Once f3 sits at
// or above BOTH, the count has stopped falling (it may even be climbing back)
// and the loop stops.
//
// Three is the smallest window that can tell a trend from one noisier verdict.
// This replaces an earlier rule that fired only when three counts were exactly
// EQUAL and gave low counts 15 passes of slack: that slack was earned by q667
// (13 passes at 2 suspects, then it cleared and scored 4), but once the
// candidate-matrix audit was removed the failure shape changed - runs froze at
// 1 suspect instead (q371 and q629 burned 18 passes each, q646 17, q481 and
// q617 the full 20): 112 passes across 13 questions for five stalls of which
// only one ever broke. A flat trend test cuts those by pass 7 while a
// genuinely falling count still runs to the ceiling.
const gateStallWindow = 3

// deliveryGateInput carries the gate loop's dependencies.
type deliveryGateInput struct {
	// explorer is the research agent itself; every repair turn runs it inside
	// the managed session below, so the gate sends no history.
	explorer adk.Agent
	// auditor is the answer_auditor sub-agent, run BY THE GATE on every pass
	// inside the managed session below — the agent never sees it in its
	// toolset.
	auditor adk.Agent
	// sess owns the two managed conversations: the explorer's (research +
	// repair turns) and the auditor's continuous audit conversation. Each turn
	// hands over only its NEW messages; the Runner replays the rest.
	sess *runSession
	// toolCallCounts is the run's shared per-question tally (see
	// Input.ToolCallCounts). Repair turns and auditor passes append to the
	// SAME ledger so per-question usage accounting covers the whole turn.
	toolCallCounts map[string]int
	// toolCallErrors / toolErrorSamples are the run's failure tallies (see
	// Input.ToolCallErrors). Repair and audit turns pass them through so a
	// tool call that returned a canonical error result is counted as a failure
	// there too, not just as an invocation.
	toolCallErrors   map[string]int
	toolErrorSamples map[string]string
	// reads is the run's chunk-read ledger — the ids of every chunk the corpus
	// retrieval tools put in front of the model in this run (Input.ChunkReads).
	// The gate compares the deliverable's cited ids against it: the one citation
	// question that needs no reading of content (see unreadCitations).
	reads *chunkReadLedger
	// searches is the run's search ledger (Input.Searches) - every retrieval
	// action actually executed, append-only. Its size rides into the audit
	// payload as the nudge that tells the auditor a read_search_ledger call
	// has facts to verify coverage judgements against.
	searches *searchLedger
	// serves is the run's served ledger (Input.Serves) - every chunk the
	// retrieval tools put in front of the model, with its document. Diffed
	// against deepDocs at audit time: documents served but never deep-read.
	serves *servedLedger
	// deepDocs is the run's deep-read document set (Input.RetrievedDocIDs,
	// filled only by full-content tools) - the read half the served ledger is
	// diffed against.
	deepDocs *docIDLedger
	// planBody is the stage-one plan without its resolved-question line - the
	// structural fact (each block's Binds/From edges) the Survivors chain
	// check walks. It is already in scope at the gate's call site, so this
	// costs nothing to carry.
	planBody string
	// audit, when non-nil, receives the per-round suspect accounting of every
	// audit this gate runs (see GateAuditRecord).
	audit *GateAuditRecord
	// baseMessages is the caller's chat history (ends on the current user
	// question); every repair turn is built on top of a fresh copy.
	baseMessages []*schema.Message
	// seedMessages is what the FIRST explorer turn actually received for a
	// planned run — the `## Resolved question` / `## The question
	// decomposition` input. A repair turn seeds from this, not from
	// baseMessages: the resolved question and the verified plan are exactly
	// what the deliverable must render, and re-seeding the raw history would
	// hand the repair a different task than the turn it is repairing. Nil for
	// the plain-explorer shape, where baseMessages is the right seed.
	seedMessages []*schema.Message
	final        string
	// auditMaxPass is the template's audit budget (Template.AuditMaxPass):
	// how many audit passes the gate may spend. Zero or less ships the
	// deliverable unaudited — the gate is not entered at all.
	auditMaxPass int
}

// runDeliveryGate drives the audit loop and returns the adopted final answer
// plus the deliverable the auditor last examined (auditedFinal) — the
// caller's fallback candidate when the loop exits without the FOS answer
// line. The agent's job ended with its FINAL message; the gate OWNS
// verification: every pass audits the standing final with answer_auditor,
// ships on PASS, and on FAIL hands the audit findings back to the agent as
// one repair directive (full toolset, so repairs can list_chunks-read the
// flagged steps). Nothing depends on the model choosing to call the auditor
// — the audit never runs inside the agent, so compliance cannot be defied.
// runDeliveryGate drives the audit loop and returns the adopted final answer,
// the deliverable the auditor last examined (auditedFinal), and the error of
// the last gate turn a model-chain failure killed outright (nil when no turn
// died to the provider) — the upper layer checks that error and surfaces it
// into the run's own error, so a provider death is recorded AND
// self-describing instead of reading like a content refusal (#44: 39 rows).
func runDeliveryGate(ctx context.Context, in deliveryGateInput) (string, string, map[string]struct{}, error) {
	if in.auditMaxPass <= 0 {
		return in.final, "", nil, nil
	}
	final := in.final
	// auditedFinal: the deliverable the auditor last examined or was about to
	// examine (updated at the top of every pass). On any exit it is the
	// fallback the caller may ship verbatim when the standing final lacks the
	// FOS answer line — an audit-failed but structured deliverable beats blind
	// synthesis (see the finalizeAnswer gate in Run).
	auditedFinal := final
	// modelErr is the error of the last gate turn a model-chain failure killed
	// outright (repair turn, audit round) — nil when none died to the
	// provider. It rides back for the upper layer to check (#44 forensics).
	modelErr := error(nil)
	// rejected holds a fingerprint of every repair continuation this gate
	// REFUSED, and rides back to the caller so the recovery ladder cannot
	// resurrect one. A rejected repair stays in the explorer session (its tool
	// calls may carry useful evidence, so it is not rolled back), and a bare
	// answer line satisfies hasAnswerLine — so without this set a rejected
	// continuation could be recovered as the final answer, or suppress the
	// synthesis that would have produced a real one.
	rejected := make(map[string]struct{})
	// The auditor's OWN conversation, threaded across passes: user payload ->
	// (tool-call turns + verdict) -> next payload -> ... One continuous audit
	// session means the auditor remembers its earlier opinions and the chunks
	// it already deep-read — a re-audit focuses on what changed instead of
	// re-reading everything. The session (not the caller) owns that history.
	if in.sess == nil {
		in.sess = newRunSession()
	}
	// noProgress: consecutive repair attempts that failed to advance the
	// deliverable (see gateNoProgressLimit).
	noProgress := 0
	// suspectHist is the suspect count of EVERY audit this gate runs, oldest
	// first — the whole curve, not just the window that decides the stall. It
	// is bounded by the audit budget (auditMaxPass ints at most), so keeping it
	// costs nothing and the short-circuit log can show how the repair actually
	// behaved: a count that fell 15→11→6→5 before freezing reads very
	// differently from one that never moved. The stall check reads its tail.
	suspectHist := make([]int, 0, in.auditMaxPass)
	// reanchored bounds the RE-ANCHOR round to one per run. The stall check used
	// to end the loop outright; measured on the smoke47 re-run, that shipped a
	// wrong anchor after the count stopped falling (q1093: 10→12→13 in three
	// rounds, the correct series never named once while four of its documents sat
	// in the served set; q351: 13→10→6→7→10 in five). One round that forces the
	// search onto a different anchor — with the served-but-unread documents NAMED
	// and a new `Searched:` line REQUIRED — is the cheapest thing that can move
	// such a count; a second flat count after it is the same evidence re-bought.
	reanchored := false
	// repairFeedback: why the last attempt was discarded, appended to the
	// next attempt's directive — a silently discarded attempt leaves the
	// model repeating the same narration (observed on q716).
	repairFeedback := ""

	// pass counts loop ITERATIONS, and the loop is bounded by two separate
	// budgets: audits and repairs. They used to share one counter, which let a
	// repair spend the audit budget — with auditMaxPass=1 and a blank first
	// draft, one successful repair exhausted the budget and the gate shipped a
	// draft it had never audited. Counting them apart keeps the audit budget for
	// audits, while repairGateBudget still bounds the repair turns so repeated
	// precheck failures cannot spin.
	pass := 0
	audits := 0
	repairs := 0
	for audits < in.auditMaxPass && repairs < repairGateBudget {
		// Deadline awareness (q268 lesson): every gate step costs minutes and
		// the run shares ONE wall-clock budget with the main loop. When it is
		// gone, auditing or repairing on a dead context can only fail — and
		// every in-flight tool call dies mid-flight — so stop and ship the
		// best deliverable in hand. The caller's finalizeAnswer fallback still
		// runs, on a context of its own.
		if err := ctx.Err(); err != nil {
			common.WarnCtx(ctx, "agentic_rag: delivery gate out of time",
				zap.Int("pass", pass+1), zap.Error(err))
			return final, auditedFinal, rejected, modelErr
		}
		// A blank deliverable cannot produce anything but the auditor's
		// mechanical first opinion — `schema integrity: final_message is
		// missing` — so auditing it burns a call and delays the repair that is
		// obviously needed. Skip straight to the directive; the next pass
		// audits as soon as a deliverable exists.
		verdict := ""
		directive := ""
		// reanchorRound marks the single round the reanchored flag buys, so the
		// adoption rule below can ENFORCE it: the continuation must add a new
		// `Searched:` line, or a re-render of the same matrix would consume the
		// round without changing the search that produced it.
		reanchorRound := false
		reanchorDirective := ""
		// The gate reads the answer LABEL, not the answer: `hasAnswerLine` is what it
		// knows without guessing at content, and the auditor owns the rest (see
		// answerLabel). The label rides into the payload as evidence for the
		// label-consistency rules.
		answerLbl := answerLabel(final)
		// Citation grounding, before the audit: every id the deliverable cites must be
		// a chunk this run actually received. This is membership in the read ledger,
		// not a reading of the deliverable's prose, so it is safe to enforce - the
		// auditor cannot see what the producer read, and a fabricated id is the defect
		// its contract calls the most damaging one (a snippet beside an unread id can
		// never be verbatim). Refusing here buys the repair turn the finding needs.
		unreadDirective := ""
		if strings.TrimSpace(final) != "" {
			if unread := unreadCitations(final, in.reads); len(unread) > 0 {
				countUnreadCitation(in.audit)
				unreadDirective = unreadCitationDirective(unread)
				common.WarnCtx(ctx, "agentic_rag: delivery gate refused the deliverable — cited chunks the run never read",
					zap.Int("pass", pass+1), zap.Strings("unread_chunk_ids", unread))
			}
		}
		// Survivors chain integrity, beside the citation grounding: the plan's
		// From edges are structural facts, so a downstream set standing on an
		// empty upstream slot is mechanical, not prose - refusing here buys
		// the repair turn before the audit spends its budget on evidence the
		// producer must replace anyway (see survivors_chain.go).
		chainDirective := ""
		if strings.TrimSpace(final) != "" && in.planBody != "" {
			if blocks, _ := checkDecompositionBlocks(in.planBody); len(blocks) > 0 {
				if breaks := survivorsChainBreaks(final, blocks); len(breaks) > 0 {
					countChainBreak(in.audit)
					chainDirective = survivorsChainDirective(breaks)
					common.WarnCtx(ctx, "agentic_rag: delivery gate refused the deliverable — Survivors chain order broken",
						zap.Int("pass", pass+1), zap.Strings("breaks", breaks))
				}
			}
		}
		if strings.TrimSpace(final) != "" && unreadDirective == "" && chainDirective == "" {
			// The gate audits the deliverable it actually holds — audit-target
			// freshness is structural, not tracked. The question lives in the
			// auditor's system prompt, so the payload carries the deliverable
			// plus the gate's own reading of it.
			auditedFinal = final
			// The hold log: the full deliverable at DEBUG, plus ONE capped INFO line
			// carrying the gate's own label reading and the deliverable's last line.
			// The question it answers is "did the gate hold the text that was
			// archived?", and only the text itself can — the audit payload is
			// truncated at 2000 chars FROM THE START, so the answer line (always the
			// LAST line) never reaches the log, and a round whose archived delivery is
			// well formed was indistinguishable from a stale-read bug (measured on
			// q283: two such rounds, and the archived text does carry a well-formed
			// answer line). It used to fire only when the gate raised a precheck;
			// prechecks are gone, and this record is the part of them worth keeping.
			common.DebugCtx(ctx, "agentic_rag: delivery gate hold (full)",
				zap.Int("pass", pass+1), zap.String("answer_label", answerLbl),
				zap.String("deliverable", final))
			// The deliverable itself is DEBUG-only above: a rejected draft can
			// carry private KB excerpts, and at INFO this line reached general
			// service logs. INFO keeps the label and the size, which is what
			// triage needs, and nothing that quotes the corpus.
			common.InfoCtx(ctx, "agentic_rag: delivery gate hold",
				zap.Int("pass", pass+1),
				zap.String("gate_answer_label", answerLbl),
				zap.Int("deliverable_bytes", len(final)),
				zap.Int("deliverable_lines", strings.Count(final, "\n")+1))
			var err error
			unread := servedUnreadDocs(in.serves, in.deepDocs, servedUnreadAuditorMax)
			if len(unread) > 0 {
				// One greppable INFO line per audit round carrying the census with
				// its frequencies: the DEBUG payload below has them too, but the
				// question "was the answer's document served and unread, and how
				// hard did the retrieval insist?" is asked long after the run, when
				// only the INFO tier survives.
				common.InfoCtx(ctx, "agentic_rag: served-unread census",
					zap.Int("pass", pass+1), zap.Int("leads", len(unread)), zap.Strings("lead_docs", unread))
			}
			verdict, err = gateRunAudit(ctx, in.auditor, in.sess.auditor, final,
				in.toolCallCounts, in.toolCallErrors, in.toolErrorSamples, in.searches, unread)
			audits++ // charged to the audit budget, independent of repairs
			if err != nil {
				// The auditor itself failed (LLM timeout, tool outage).
				// Retrying inside this request rarely helps; fall through to
				// finalizeAnswer, which still catches answer-less finals.
				// The failure is RECORDED before returning: this is the one exit
				// that leaves no verdict behind, and an unrecorded exit silently
				// upgrades an unverified deliverable to an audited one.
				countGateAuditFailure(in.audit)
				common.WarnCtx(ctx, "agentic_rag: delivery gate audit failed",
					zap.Int("pass", pass+1), zap.Error(err))
				if modelFailure(err) {
					countModelKilledTurn(in.audit, modelCause(err))
					modelErr = fmt.Errorf("gate audit round: %w", err)
				}
				return final, auditedFinal, rejected, modelErr
			}
			// Per-round suspect accounting for benchmarks (Input.GateAudit):
			// every audit is recorded, PASS rounds included — a PASS verdict
			// reports 0 suspects, so the list doubles as the round count. The
			// verdict's own words ride along, so a curve can be explained from
			// the record instead of reverse-engineered from the deliverable.
			if in.audit != nil {
				in.audit.Suspects = append(in.audit.Suspects, auditSuspectCount(verdict))
				in.audit.Passed = auditPassed(verdict)
			}
			recordAuditVerdict(in.audit, verdict)
			if auditPassed(verdict) {
				break // audited and passed as a whole — ship
			}
			// Advisory-only verdict: every finding the auditor raised is one
			// the pipeline grades as completeness or form, so it is reported
			// and never billed as a blocking defect. #36: the one round the
			// auditor graded advisory handed q875 its first win, while the
			// same class left untagged refused q1005 on M=14 across six
			// rounds. The grading is the pipeline's from here on.
			if auditShipsOnAdvisory(verdict) {
				common.WarnCtx(ctx, "agentic_rag: delivery gate shipped on advisory-only findings",
					zap.Int("pass", pass+1),
					zap.Int("reported_suspects", auditSuspectCount(verdict)),
					zap.Int("advisory_findings", len(auditOpinions(verdict))))
				if in.audit != nil {
					in.audit.Passed = true
				}
				break
			}

			// The finding, not the verdict string: the auditor echoes the whole
			// deliverable back, so the raw verdict spends its first 300 chars on
			// the ## Candidate Matrix and cuts the reason off entirely.
			common.InfoCtx(ctx, "agentic_rag: delivery gate audit failed the deliverable",
				zap.Int("pass", pass+1),
				zap.Int("suspects", auditSuspectCount(verdict)),
				zap.String("verdict", auditFindings(verdict)))

			suspectHist = append(suspectHist, auditSuspectCount(verdict))
			// Stall check fires BEFORE the repair turn: once three observations
			// are in, act as soon as the newest count is no better than BOTH of
			// the two before it. Equal counts are only the special case - this
			// also catches a count climbing back (f1 < f2 < f3), where the
			// repair is making the deliverable worse and shipping what stands
			// strictly beats buying another pass. A count that is still falling
			// keeps the loop alive. The trend is read off the tail of the full
			// history.
			//
			// A stall no longer ends the loop on its own: it buys ONE re-anchor
			// round first (auditReanchorDirective), because every directive up to
			// this point asks for a repair of the RECORD while a stalled count
			// says the SEARCH never moved. Only a second stall - the count no
			// better after the anchor was forced to change - ships what stands.
			if n := len(suspectHist); n >= gateStallWindow {
				f1, f2, f3 := suspectHist[n-3], suspectHist[n-2], suspectHist[n-1]
				if f3 >= f1 && f3 >= f2 {
					if !reanchored {
						reanchored = true
						// The round gets the LONGER census: its job is to put the right
						// unread documents in front of the producer, and the ranking is
						// by serve count, not by relevance to the answer, so the extra
						// slots are the only way a quieter answer-bearing lead makes the
						// list (q1093's two, served twice each, ranked below ten louder
						// ones).
						leads := servedUnreadDocs(in.serves, in.deepDocs, servedUnreadReanchorMax)
						reanchorDirective = auditReanchorDirective(suspectHist, leads)
						common.InfoCtx(ctx, "agentic_rag: delivery gate re-anchor round — the suspect count stopped falling",
							zap.Int("pass", pass+1), zap.Ints("suspects", suspectHist),
							zap.Int("auditor_leads", len(unread)), zap.Int("reanchor_leads", len(leads)),
							zap.Strings("reanchor_lead_docs", leads))
					} else {
						common.InfoCtx(ctx, "agentic_rag: delivery gate short-circuit — suspect count stopped falling after the re-anchor round",
							zap.Int("pass", pass+1), zap.Ints("suspects", suspectHist))
						break
					}
				}
			}
		}

		if strings.TrimSpace(final) == "" {
			// No deliverable: this repair is not a correction, it is the FIRST
			// production of the answer.
			common.InfoCtx(ctx, "agentic_rag: delivery gate skipped the audit — no deliverable yet",
				zap.Int("pass", pass+1))
			countGateRejection(in.audit)
			directive = ("You have produced NO deliverable yet, so there is nothing to ship to the user. " +
				"Continue the investigation in this turn and render the COMPLETE FOS FINAL message: the " +
				"`## Candidate Matrix` blocks, then `## Reasoning Chain`, and the LAST line " +
				"`Final Answer: **<value>**` (or `Guessed Answer: **<value>** (assumption: ...)`). " +
				"If a retrieval call errored or came back empty, rephrase the query and try again BEFORE " +
				"answering: an answer drawn from memory instead of the corpus is not acceptable, and " +
				"ending the turn on narration or on another tool call leaves the user with nothing.")
		} else if unreadDirective != "" || chainDirective != "" {
			// No audit ran this pass: the gate's own reading already names the
			// defect, and re-auditing a deliverable it just refused would spend a
			// pass on a verdict it can predict. A draft can carry BOTH defects —
			// citations to unread chunks and a broken Survivors chain — so the
			// repair model gets both findings, not whichever check ran first.
			switch {
			case unreadDirective != "" && chainDirective != "":
				directive = unreadDirective + "\n\n" + chainDirective
			case unreadDirective != "":
				directive = unreadDirective
			default:
				directive = chainDirective
			}
		} else if reanchorDirective != "" {
			// The audit ran and the suspect count has stalled: this round moves the
			// SEARCH instead of repairing the same record again (see the stall check
			// above), and it is enforced below.
			directive = reanchorDirective
			reanchorDirective = ""
			reanchorRound = true
		} else {
			directive = auditRepairDirective(auditSuspectCount(verdict), verdict, suspectHist)
		}
		// A repair is only worth starting if an audit slot remains to CHECK its
		// result. Without this, the last audit pass could reject draft A, the
		// repair could produce draft B, and the loop would exit shipping B
		// unchecked while the recorded verdict still describes A — a draft the
		// gate never saw, presented as audited.
		if audits >= in.auditMaxPass {
			common.InfoCtx(ctx, "agentic_rag: delivery gate stopping — no audit budget left to check a repair",
				zap.Int("pass", pass+1), zap.Int("audits", audits))
			break
		}
		// Repair loop: retry IN PLACE until the deliverable ADVANCES. An
		// attempt that aborted, went silent, or ended in narration leaves the
		// deliverable unchanged, so re-auditing it would only reproduce the
		// verdict in hand — the retry is cheaper and carries the reason the
		// previous attempt was discarded.
		giveUp := false
		for {
			attempt := directive
			if repairFeedback != "" {
				attempt += "\n\n" + repairFeedback
			}
			if err := ctx.Err(); err != nil {
				// Budget gone: a repair turn would only abort mid-flight, and
				// the in-place retry budget would burn on instant failures.
				common.WarnCtx(ctx, "agentic_rag: delivery gate out of time before repair",
					zap.Int("pass", pass+1), zap.Error(err))
				giveUp = true
				break
			}
			// Charged per INVOCATION, not per adoption: the cap bounds the
			// number of repair CALLS (each is a model turn with its own cost),
			// so a rejected attempt must count too — otherwise a run could
			// make 2x repairGateBudget calls by interleaving rejections.
			repairs++
			gateFinal, gateErr := runRepairAttempt(ctx, in, in.sess.explorer, final, attempt)
			if gateErr == nil {
				// The turn stays in the session even when it will be
				// rejected: a narrating attempt may still have retrieved
				// useful evidence, and the session replays it — reordering
				// parallel tool results and nothing else — provider-valid.
				trimmed := strings.TrimSpace(gateFinal)
				// Adoption rule (format presence, not correctness): a
				// continuation must carry BOTH the FOS answer line and the
				// FOS sections. The answer line alone is not enough — a bare
				// "Guessed Answer: I do not have sufficient evidence" line
				// satisfies it while dropping every cited chunk, so adopting
				// it throws away a structured (if failing) deliverable and
				// buys a re-audit that can only repeat the coverage defect
				// (q1093 burned 16 audit passes that way).
				// The re-anchor round is ENFORCED, not merely requested: the
				// continuation must add at least one new `Searched:` line. Without
				// this the directive's demand is prose a re-render satisfies
				// (q1093 bought three audits with the anchor never moving, and
				// q351 five).
				newSearches := searchedLineCount(trimmed) > searchedLineCount(final)
				if adoptableContinuation(trimmed) && (!reanchorRound || newSearches) {
					final = trimmed // deliverable-shaped continuation → adopt
					noProgress = 0
					repairFeedback = ""
					pass++
					break
				}
				// Remember what was refused: the recovery ladder below must
				// not resurrect it (see runDeliveryGate's rejected set).
				rejected[contentFingerprint(trimmed)] = struct{}{}
				discardWhy := "did not render the COMPLETE deliverable (it was missing the FOS answer line, or the " +
					"`## Candidate Matrix` / `## Reasoning Chain` sections, or both)"
				if reanchorRound && adoptableContinuation(trimmed) {
					discardWhy = "rendered the deliverable but added NO new `Searched:` line - this round requires " +
						"retrieval on a DIFFERENT anchor, and a re-render of the same matrix is discarded"
				}
				repairFeedback = ("REJECTION NOTICE: your previous repair continuation was DISCARDED - it " +
					discardWhy + ". Your next reply must " +
					"render it in the SAME turn: the `## Candidate Matrix` blocks, then " +
					"`## Reasoning Chain`, and the LAST line `Final Answer: **<value>**` or " +
					"`Guessed Answer: **<value>** (assumption: ...)` - a bare answer line or any narration " +
					"instead of the deliverable is discarded again.")
				countGateRejection(in.audit)
				// The discarded draft is DEBUG-only: a repair turn quotes the
				// corpus, so its preview does not belong in INFO logs. The
				// boolean shape fields below are what distinguish a
				// quota-truncated repair from one that never rendered the
				// deliverable (q798-class diagnosis) without quoting it.
				common.DebugCtx(ctx, "agentic_rag: delivery gate rejected continuation (full)",
					zap.Int("pass", pass+1), zap.String("preview", trimmed))
				common.InfoCtx(ctx, "agentic_rag: delivery gate rejected answer-less continuation",
					zap.Int("pass", pass+1),
					zap.Int("discarded_bytes", len(trimmed)),
					zap.Bool("had_answer_line", hasAnswerLine(trimmed)),
					zap.Bool("had_fos_structure", hasFOSStructure(trimmed)))
			} else {
				repairFeedback = fmt.Sprintf(("TOOL FAILURE NOTICE: your previous repair turn aborted with " +
					"a tool error (%v) and produced nothing usable. The audit findings above still stand - " +
					"retry the same repair, but avoid the failing call: if a retrieval tool errored, use a " +
					"different tool or a different query."), gateErr)
				common.WarnCtx(ctx, "agentic_rag: delivery gate repair turn aborted",
					zap.Int("pass", pass+1), zap.Error(gateErr))
				// Silent accounting only: the model-caused death is recorded
				// here and attributed ONCE, at the run's end, so the cause
				// does not scatter across per-turn logs (#44 forensics).
				if modelFailure(gateErr) {
					countModelKilledTurn(in.audit, modelCause(gateErr))
					modelErr = fmt.Errorf("gate repair turn: %w", gateErr)
				}
			}

			// A repair attempt that aborted because the shared budget expired
			// will fail identically on every retry: stop burning the
			// no-progress allowance and ship.
			if err := ctx.Err(); err != nil {
				common.WarnCtx(ctx, "agentic_rag: delivery gate out of time after repair",
					zap.Int("pass", pass+1), zap.Error(err))
				giveUp = true
				break
			}

			noProgress++
			if noProgress >= gateNoProgressLimit {
				common.InfoCtx(ctx, "agentic_rag: delivery gate short-circuit — no progress across attempts",
					zap.Int("pass", pass+1), zap.Int("attempts", noProgress))
				giveUp = true
				break
			}
		}
		if giveUp {
			break
		}
		// Next pass re-audits whatever final now stands.
	}

	return final, auditedFinal, rejected, modelErr
}

// runRepairAttempt performs ONE explorer continuation for a repair directive,
// returning the continuation's FINAL message. Retry policy lives in the gate
// loop, which retries in place while the deliverable has not advanced.
//
// The turn carries NO history: the managed session replays it, so the only
// thing handed over is the directive — plus the caller's chat history when an
// earlier failure forced the conversation to restart (see conversation.turnMessages).
func runRepairAttempt(
	ctx context.Context,
	in deliveryGateInput,
	conv *conversation,
	final, directive string,
) (string, error) {
	// What the producer was actually TOLD, at full length, debug only: the repair
	// directive was previously logged nowhere, so "the model ignored the anchor
	// demand" and "the demand was never in front of it" were indistinguishable.
	common.DebugCtx(ctx, "agentic_rag: delivery gate repair directive (full)",
		zap.String("directive", directive))
	// turnMessages now takes the current turn as a Message (not a string), so the
	// directive is handed over as the user message it is.
	seed := in.baseMessages
	if len(in.seedMessages) > 0 {
		seed = in.seedMessages
	}
	gateMsgs := conv.turnMessages(seed, final, schema.UserMessage(directive))
	// Deltas stay off for gate passes: clients already hold the first
	// answer, and only the corrected final is returned upward.
	noStream := func(contentDelta, thinkingDelta string) {}
	head := conv.head(ctx)
	iter := conv.runner(ctx, in.explorer, false).Run(ctx, gateMsgs)
	// Tally repair-turn tool calls into the run's shared ledger: the audit
	// directive routinely asks for list_chunks re-reads, and dropping those
	// from the counts made tool_<name> disagree with tool_<name>_ms.
	out, _, err := consumeAgentEvents(ctx, iter, noStream, in.toolCallCounts, in.toolCallErrors, in.toolErrorSamples)
	if err != nil {
		// A failure must not leak a half-finished turn into the next one:
		// an unanswered tool call replays as a provider error.
		conv.discardFailedTurn(ctx, head)
	}
	return out, err
}

// anchorDemand returns the "change the anchor before re-rendering" clause of a
// repair directive. It binds only once a repair has ALREADY failed (two or more
// suspect observations): re-rendering the same matrix is the shape the gate
// keeps seeing when the answer is wrong - the audit count sits flat or climbs
// because the retrieval anchor never moved, so the repair buys audit rounds
// without buying evidence. From the second failed audit on, a repair turn must
// open with new retrieval, and a turn with no new `Searched` line is rejected.
// auditRepairDirective is the instruction the gate hands back to the producer after an
// audit FAIL: what to fix, in what order, and - since q221 - what NOT to touch.
//
// It is a function rather than an inline literal because the rules it carries are
// behavioural contracts (repair the record and not the conclusion; a stalled suspect
// count needs NEW evidence rather than a re-render) and are pinned by tests, the same
// way anchorDemand is.
func auditRepairDirective(suspects int, verdict string, hist []int) string {
	directive := fmt.Sprintf("Your FINAL message failed the pipeline's mandatory answer auditor "+
		"(%d suspect item(s)):\n%s\nFix every item whose audit opinion is not `pass` before shipping: "+
		"keep retrieving or list_chunks-read the flagged line, repair or drop it, re-derive the answer "+
		"from the supported steps, and re-render the COMPLETE FOS FINAL message (## Candidate Matrix, "+
		"## Reasoning Chain, and the Final/Guessed Answer line). If the audit reported schema integrity "+
		"(missing candidate_matrix / reasoning_chain / answer), render the full deliverable now. "+
		"If the audit reported `retrieval breadth insufficient`, do NOT re-render the same matrix: "+
		"decompose the question into its rarest distinctive terms (proper nouns, unique titles, exact "+
		"dates) and run one SHORT keyword query per term - long multi-clue paraphrases do not match "+
		"the corpus's wording. "+
		// Lever 3: a repair that only re-renders the matrix is the
		// observed failure shape - the suspect count sits flat or
		// climbs because the anchor never moved. Once a repair has
		// already failed, require NEW evidence before any re-render.
		anchorDemand(len(hist), hist)+

		// (b2) The two classes a failing item can belong to, because the
		// anti-swap rule below was read as a blanket ban: q283's producer quoted
		// it FOUR times across nine audits to justify keeping "The Hatchling"
		// while the auditor correctly said the value does not fill the asked
		// Z-name slot (the gold, `Zimri Elder`, is in two corpus docs). The rule
		// bans the swap that silences a complaint with no evidence behind it; it
		// was never a licence to re-render a value the auditor has already shown
		// to fail a constraint, and the cost of that misreading was eight extra
		// audits and a wrong answer.
		"TWO KINDS OF FAILING ITEM, TWO DIFFERENT REPAIRS. (i) A RECORD item - a missing or malformed field, an unsupported "+
		"step, an untested rival, a line that only NAMES the candidate - is repaired IN THE RECORD, and never by changing which "+
		"value ships. (ii) An item about the VALUE ITSELF - `the answer does not fill the asked slot`, `answer is not grounded "+
		"in the candidate matrix`, a cited chunk that contradicts it, a constraint the value FAILS - IS evidence against that "+
		"value, so the repair is to CHANGE THE VALUE: re-derive it from a candidate that satisfies the slot, or, when the corpus "+
		"holds no such candidate, ship the corpus's negative result under `Guessed Answer` with the searches that showed it. "+
		// REVERTED 2026-10-05: the value-slot clause that replaced the licence
		// above measured NET NEGATIVE over ten questions (smoke63 5/10 against
		// smoke61's 8/10 on the same build, three previously-stable questions
		// lost, tokens flat at 54M), and the single-pair win it was approved on
		// (smoke62 q11+q351) did not replicate: q11 went back to seven audits
		// and 10.3M tokens. The failure shape was uniform - every wrong
		// delivery shipped `Guessed Answer: **<candidate>** (assumption: ...)`
		// and q679 passed the gate in two rounds on a wrong candidate - i.e.
		// telling the producer that the value slot MUST hold a candidate turned
		// "keep verifying" into "name what you already have". The contradiction
		// with the auditor's `answer value is not a slot filler` remains real
		// and is left as it was: fixing it needs an evaluation that can
		// actually resolve it, which a ten-question single run cannot.
		"Class (ii) is not what the rule below forbids: what is forbidden is a swap with no evidence behind it, and what is "+
		"equally forbidden is re-rendering the same value a third time when the auditor has shown a constraint it fails. "+
		// (c) Repair the RECORD, not the conclusion. The cheapest repair
		// observed is a SWAP: q221 shipped "Opium: A Portrait of the
		// Heavenly Demon" under `Final Answer` right after the auditor pushed
		// the producer off a weakness-based elimination of the gold - a PASS
		// bought by weakening the answer, strictly worse than the FAIL it
		// replaced. A grounded rival that cannot be refuted is a declared
		// TIE, never a promotion by default.
		"REPAIR THE RECORD, NOT THE CONCLUSION: an audit opinion is a claim about the RECORD - a field, "+
		"a snippet, an unsupported step, an untested rival, a constraint that is not established - and it is "+
		"NEVER on its own a reason to change which candidate you retain or which value you ship. Do NOT "+
		"swap the retained candidate or the answer value to make a complaint disappear: a deliverable "+
		"re-decided against its own evidence can pass the same auditor and then ships the WEAKER answer "+
		"under the SAME label, which is worse than the FAIL it replaces. Change the value ONLY when an "+
		"opinion names evidence AGAINST the current candidate (a chunk that contradicts it, a constraint "+
		"it fails), and carry that evidence on the new line. When a GROUNDED rival cannot be refuted, the "+
		"lawful outcome is a DECLARED TIE (a `(tie: ...)` clause per rival on the answer line), never a "+
		"silent swap; a rival that is ungrounded is eliminated for ABSENCE with the named search that "+
		"showed it, never promoted. "+
		// (c2) The label is the producer's to write and the auditor's to judge; the
		// pipeline no longer relabels a deliverable itself (it used to splice the word
		// and append a note). Without this clause the anti-swap rule above reads as a
		// ban on touching the answer line at all, and a run whose record does not
		// establish its `Final Answer` would keep claiming it.
		"THE LABEL IS NOT THE VALUE: when a failing item says the record does not establish what `Final Answer` claims "+
		"(an untested constraint, a rival not refuted, a declared tie, an unverified step), the repair is your OWN answer "+
		"line - state `Guessed Answer` with the assumption note, keep the value you can support, and leave the rest of the "+
		"record intact. `REPAIR THE RECORD, NOT THE CONCLUSION` governs the VALUE, not this claim, and nothing downstream "+
		"will relabel the deliverable for you. "+
		// (d) Fix the EVIDENCE, not the wording. v2 on q221 spent three
		// passes re-arguing ONE naming-level inference ("the collection is
		// called the Opium Collection") and flatlined at 5,5,5 while the
		// deliverable kept "advancing" in prose: a rejected argument
		// restated is not a repair, and a flat count makes the gate ship
		// the weaker deliverable.
		"FIX THE EVIDENCE, NOT THE WORDING: when an opinion says a line only NAMES the candidate (a title, "+
		"a citation, a bibliography or listing entry, a collection name), restating the same evidence in "+
		"different words is not a repair and will be rejected again - the only admissible responses are to "+
		"cite a chunk that STATES the candidate's fitness for the asked slot, or to drop the line and "+
		"eliminate the candidate for ABSENCE with the named search that showed no content about it. The same "+
		"holds for any argument the auditor has already rejected (a weakness-based elimination, a "+
		"naming-level inference): change the EVIDENCE or drop the CLAIM, never re-wrap the claim. An attempt "+
		"that advances only the prose leaves the suspect count where it was. "+
		"Citation-field repairs: call list_chunks for the cited chunk_id and OVERWRITE the line's "+
		"fields with what the tool actually returns - never guess an identifier from memory; a value "+
		"you cannot look up must be dropped, not invented. Never leave an item whose opinion is not "+
		"`pass` in the deliverable.", suspects, verdict)
	return directive
}

func anchorDemand(failures int, hist []int) string {
	if failures < 2 {
		return ""
	}
	trend := ""
	if n := len(hist); n >= 2 {
		trend = fmt.Sprintf(" The suspect count moved %s over your failed repairs, which is the signature of re-rendering the same anchor.",
			joinInts(hist))
	}
	return ("ANCHOR CHANGE REQUIRED: this is failed audit #" + fmt.Sprint(failures) + "." + trend +
		" Your repair MUST open with at least one NEW retrieval call on a DIFFERENT anchor than the ones already in the matrix " +
		"(the rarest proper noun queried alone, or a grep_chunks co-occurrence regex over two clue terms) and log it as a new " +
		"`Searched` line. A repair turn that re-renders the matrix with no new `Searched` line is rejected outright. " +
		// The anchor that keeps a flat count flat is an anchor about the CANDIDATE
		// the run already holds. q283 is the measured shape: 123 retrievals over
		// nine audits, every one of them about Outer Wilds or the Z-name, and the
		// evidence set (nine docs, including the developer's own page wording the
		// black hole) was NEVER served - zero of nine. Another query about the
		// candidate cannot produce a value that fills the slot it fails.
		"When the failing items are about the VALUE (an asked slot it does not fill, a chunk that contradicts it, a constraint " +
		"it fails), the new anchor must be one that can surface a RIVAL candidate for the SAME constraint: run the broad " +
		"listing/overview query the constraint makes enumerable (the game/show/book list, the roster, the year list) and test " +
		"each hit against the slot. An anchor that can only return more about the candidate you already hold keeps this count " +
		"where it is, and the gate ships what stands.")
}

// auditReanchorDirective is the gate's LAST repair round: it is spent only when
// the suspect count has stopped falling, and it moves the SEARCH instead of
// repairing the record once more.
//
// Everything else in the loop asks the producer to fix a finding. A stalled
// count is different evidence: measured on the smoke47 re-run, the two failed
// questions were the two whose suspect count never fell (q1093 10→12→13 over
// three rounds - the right series was named ZERO times while four documents
// containing it sat in the served set and were never read; q351 13→10→6→7→10
// over five - the answer's own award category appeared in seven served documents
// and never on the answer line, while its two expected documents were never
// served at all). Repairing the record cannot fix that; only a different query
// can.
//
// Three parts, all mechanical: the anchor must change, the documents the run was
// SERVED and never deep-read are NAMED (the pipeline already computes that set
// for the auditor - naming it turns the demand into a to-do list), and the round
// is enforced in runRepairAttempt's adoption rule by requiring a new `Searched:`
// line. It is explicitly the last round: if the count does not fall, the gate
// ships what stands rather than buying more of the same.
func auditReanchorDirective(hist []int, servedUnread []string) string {
	docs := ""
	if len(servedUnread) > 0 {
		docs = " These documents were SERVED to you and never deep-read - most-served first, each with the count the retrieval served it: " +
			strings.Join(servedUnread, ", ") + ". Read each with `list_chunks` and profile it as a candidate before you re-render."
	}
	return "ANCHOR CHANGE REQUIRED — THIS IS THE GATE'S LAST REPAIR ROUND. Your suspect count moved " +
		joinInts(hist) + " across the failed audits and has stopped falling, which is the signature of " +
		"re-querying and re-rendering the SAME anchor." + docs +
		" OPEN this turn with retrieval on a DIFFERENT anchor than any already in your matrix: query the rarest " +
		"proper noun ALONE, or a `grep_chunks` co-occurrence regex over two clue terms, or the broad listing the " +
		"failing constraint makes enumerable (the roster, the year list, the episode list, the award table), then " +
		"test every hit against the slot. Your re-render MUST carry at least one NEW `Searched:` line for that " +
		"anchor - a turn that only re-renders the matrix is DISCARDED and this round is wasted. When the new anchor " +
		"still cannot move the count, ship the best-supported candidate and say on its line which constraint the " +
		"corpus does not establish: this round is the last one, and the gate ships what stands after it."
}

func joinInts(values []int) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, fmt.Sprint(v))
	}
	return strings.Join(parts, " -> ")
}

// gateRunAudit runs one audit pass of the answer auditor on the current
// deliverable, extending the auditor's continuous conversation: the managed
// session replays every earlier audit pass, so this turn hands over nothing
// but ONE new user message carrying the JSON payload, then produces the
// auditor's ReAct turns (list_chunks deep-reads + the echoed deliverable with
// audit opinions). It returns the verdict text.
//
// A failed audit leaves nothing behind: its events are rolled out of the
// session, so the next pass replays exactly what the last successful one saw.
func gateRunAudit(
	ctx context.Context,
	auditor adk.Agent,
	conv *conversation,
	final string,
	toolCallCounts map[string]int,
	toolCallErrors map[string]int,
	toolErrorSamples map[string]string,
	searches *searchLedger,
	unreadDocs []string,
) (string, error) {
	if auditor == nil {
		return "", fmt.Errorf("agentic_rag: auditor unavailable")
	}
	payload := buildAuditPayload(final, searches, unreadDocs)
	// Metadata only at WARN: the payload embeds the deliverable, which quotes
	// private KB excerpts. The full text stays DEBUG below.
	common.InfoCtx(ctx, "agentic_rag: gate-run audit start",
		zap.Int("payload_bytes", len(payload)),
		zap.Int("search_entries", searches.Count()),
		zap.Int("unread_docs", len(unreadDocs)))
	// The auditor's INPUT and OUTPUT at full length, debug only: the tail of both is
	// the answer-line region (the deliverable it echoed back plus its opinions), and
	// the capped lines above can never reach it. Log volume is the cost and it is
	// deliberate - these two are the only record of what the auditor was shown.
	common.DebugCtx(ctx, "agentic_rag: gate-run audit payload (full)", zap.String("payload", payload))
	head := conv.head(ctx)
	iter := conv.runner(ctx, auditor, false).Run(ctx, []adk.Message{schema.UserMessage(payload)})
	verdict, _, err := consumeAgentEvents(ctx, iter, func(string, string) {}, toolCallCounts, toolCallErrors, toolErrorSamples)
	if err != nil {
		conv.discardFailedTurn(ctx, head)
		return "", err
	}
	// The verdict echoes the deliverable's answer line, so only its shape and
	// outcome go to INFO; the text stays DEBUG.
	common.InfoCtx(ctx, "agentic_rag: gate-run audit verdict",
		zap.Bool("passed", auditPassed(verdict)),
		zap.Int("suspects", auditSuspectCount(verdict)),
		zap.Int("verdict_bytes", len(verdict)))
	common.DebugCtx(ctx, "agentic_rag: gate-run audit verdict (full)", zap.String("verdict", verdict))
	return verdict, nil
}

// consumeAgentEvents drains the agent event iterator, streaming assistant
// deltas through onDelta (falling back to runtime.EmitAgentMessage when nil)
// and returning the agent's FINAL message plus an evidence digest. A ReAct
// loop produces one assistant message per turn — tool-call turns interleaved
// with the last, tool-free answer turn — and the FOS deliverable is exactly
// that LAST message, so `final` REPLACES the previous content on every
// completed assistant message instead of concatenating across turns.
//
// The transcript this used to return is gone: the conversation is the session's
// business now (see run_session.go). `evidence` still accumulates tool output
// because the last-resort synthesis needs a digest, not a message list.
// When toolCallCounts is non-nil, each tool the model requested is tallied by
// name (one tally per requested call, which matches one actual tool
// invocation).
func consumeAgentEvents(
	ctx context.Context,
	iter *adk.AsyncIterator[*adk.AgentEvent],
	onDelta func(contentDelta, thinkingDelta string),
	toolCallCounts map[string]int,
	toolCallErrors map[string]int,
	toolErrorSamples map[string]string,
) (string, string, error) {
	var final string
	var evidence strings.Builder
	// toolNames maps tool_call_id -> tool name. Tool-result events carry only
	// the call id, so the name is captured here when the model issues the call
	// and joined back when the result arrives.
	toolNames := make(map[string]string)
	// Everything the agent says goes out LIVE, but on the THINKING channel.
	// A ReAct turn emits many messages and any of them can already be a
	// complete deliverable (Candidate Matrix + Reasoning Chain + Final Answer)
	// — the loop often renders one, keeps retrieving, and renders a better one.
	// Routing those to the answer channel is what put several competing "Final
	// Answer" blocks in front of the user. On the thinking channel they stay
	// readable as the work unfolds without competing with the answer, which is
	// emitted exactly once, at the end of the run, by Run itself (the only
	// place that knows which deliverable is actually being shipped — the gate
	// may adopt a different one).
	live := func(content, reasoning string) {
		emit(ctx, onDelta, "", content+reasoning)
	}
	// chargeSpent bills the run's budget for the tokens spent since the last
	// call, counted from the same RunUsage the final SSE reports — so the
	// ceiling and the number the client sees are one quantity.
	//
	// It lives here because the streaming path cannot charge itself:
	// HandleStreamingResponse takes no context, so only non-streaming calls
	// reach ChargeTokens. In a ReAct run almost every turn after the first
	// streams (only the tool-bound turn goes through Generate), which left a
	// budgeted run free to spend millions against a 500k ceiling — measured:
	// 1.82M prompt tokens on one question, budget never fired.
	chargeSpent := func() {
		_, _, total, _ := tokenizer.GetRunUsage(ctx).Snapshot()
		// Non-streaming turns have ALREADY charged the budget from the API's
		// own usage, so bill only the shortfall between that counter and this
		// one: whichever is ahead wins and no turn is charged twice.
		if delta := int64(total) - common.TokenBudgetUsed(ctx); delta > 0 {
			common.ChargeTokens(ctx, int(delta))
		}
	}
	for {
		// Bill the previous turn before waiting on the next one, so an
		// over-budget run stops at the turn boundary rather than after the
		// next full response.
		chargeSpent()
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			return final, evidence.String(), ev.Err
		}
		if ev.Output == nil || ev.Output.MessageOutput == nil {
			continue
		}
		mo := ev.Output.MessageOutput
		if mo.Role != schema.Assistant {
			// Tool-result events and non-assistant messages carry the tool's
			// returned content; log it and accumulate a truncated summary for
			// the last-resort synthesis.
			if mo.Message != nil {
				content := mo.Message.Content
				common.DebugCtx(ctx, "agentic_rag: tool result",
					zap.String("tool", toolNames[mo.Message.ToolCallID]),
					zap.String("tool_call_id", mo.Message.ToolCallID),
					zap.Int("content_bytes", len(content)),
					zap.String("content", content),
				)
				// Failure accounting: tools report failures as canonical
				// <tool_error> results so the loop keeps running; the
				// severity="error" ones are tallies here for the benchmark's
				// run stats (severity="warn" partials stay visible to the
				// model but don't count as outages). One sample per tool is
				// kept — the first root cause seen, truncated.
				if strings.Contains(content, toolErrorMarker) && strings.Contains(content, `severity="error"`) {
					if name := toolNames[mo.Message.ToolCallID]; name != "" && toolCallErrors != nil {
						toolCallErrors[name]++
						if toolErrorSamples != nil {
							if _, ok := toolErrorSamples[name]; !ok {
								toolErrorSamples[name] = content
							}
						}
					}
				}
				if trimmed := strings.TrimSpace(content); trimmed != "" {
					if evidence.Len() > 0 {
						evidence.WriteString("\n\n")
					}
					evidence.WriteString(truncateForLog(trimmed, 4000))
				}
			}
			continue
		}
		// Log the tool calls the model decided to make this round, and tally
		// them by name into the caller's counter (when provided) so per-question
		// tool invocation counts can be aggregated.
		if mo.Message != nil && len(mo.Message.ToolCalls) > 0 {
			for i := range mo.Message.ToolCalls {
				tc := &mo.Message.ToolCalls[i]
				name := tc.Function.Name
				if tc.ID != "" {
					toolNames[tc.ID] = name
				}
				common.DebugCtx(ctx, "agentic_rag: tool call",
					zap.String("tool", name),
					zap.String("tool_call_id", tc.ID),
					zap.String("args", tc.Function.Arguments),
				)
				if toolCallCounts != nil && name != "" {
					toolCallCounts[name]++
				}
			}
		}
		if mo.IsStreaming {
			if mo.MessageStream == nil {
				continue
			}
			// One MessageOutput event carries one assistant message; its
			// completion REPLACES the running final (last-message semantics).
			// Chunks are merged back into one message — content join plus
			// tool-call delta assembly by Index — so a parallel search round
			// keeps every call it issued.
			var chunks []*schema.Message
			for {
				chunk, recvErr := mo.MessageStream.Recv()
				if recvErr != nil {
					if !errors.Is(recvErr, io.EOF) {
						// A real stream error: close the reader and surface it
						// instead of returning silently-truncated content.
						mo.MessageStream.Close()
						return final, evidence.String(), recvErr
					}
					break // io.EOF marks normal end of the stream.
				}
				live(chunk.Content, chunk.ReasoningContent)
				chunks = append(chunks, chunk)
			}
			mo.MessageStream.Close()
			merged := mergeStreamedAssistant(chunks)
			// Streaming providers put tool-call deltas on the chunks rather
			// than on mo.Message. Count the assembled calls once, after the
			// deltas have been merged by index.
			for i := range merged.ToolCalls {
				tc := &merged.ToolCalls[i]
				name := tc.Function.Name
				if tc.ID != "" {
					toolNames[tc.ID] = name
				}
				if toolCallCounts != nil && name != "" {
					toolCallCounts[name]++
				}
			}
			final = merged.Content
			continue
		}
		if mo.Message != nil {
			live(mo.Message.Content, mo.Message.ReasoningContent)
			final = mo.Message.Content
		}
	}
	return final, evidence.String(), nil
}

// mergeStreamedAssistant assembles one assistant message from its stream
// chunks: content is joined verbatim, and tool-call deltas are merged by
// their Index (name and arguments stream incrementally). A nil chunk or an
// all-empty stream yields an empty assistant message.
func mergeStreamedAssistant(chunks []*schema.Message) *schema.Message {
	out := &schema.Message{Role: schema.Assistant}
	callPos := make(map[int]int)
	for _, c := range chunks {
		if c == nil {
			continue
		}
		out.Content += c.Content
		for _, tc := range c.ToolCalls {
			// Index is a pointer in this eino version; treat nil as 0.
			idx := 0
			if tc.Index != nil {
				idx = *tc.Index
			}
			pos, ok := callPos[idx]
			if !ok {
				clone := tc
				out.ToolCalls = append(out.ToolCalls, clone)
				callPos[idx] = len(out.ToolCalls) - 1
				continue
			}
			out.ToolCalls[pos].Function.Name += tc.Function.Name
			out.ToolCalls[pos].Function.Arguments += tc.Function.Arguments
			if tc.ID != "" {
				out.ToolCalls[pos].ID = tc.ID
			}
		}
	}
	return out
}

// emit streams a (content, reasoning) delta through onDelta, falling back to
// runtime.EmitAgentMessage when onDelta is nil.
func emit(
	ctx context.Context,
	onDelta func(contentDelta, thinkingDelta string),
	content, reasoning string,
) {
	if onDelta != nil {
		onDelta(content, reasoning)
		return
	}
	runtime.EmitAgentMessage(ctx, content, reasoning)
}

// finalizeTimeout is the synthesis fallback's own budget. It runs on a context
// detached from the run's (see context.WithoutCancel at the call site): the
// fallback exists precisely for the case where the run's wall-clock deadline
// expired mid-investigation, so inheriting that deadline would guarantee its
// failure — q268 shipped a 60-char narration sentence because the final
// synthesis call was itself killed by the budget that had just run out.
//
// A local LLM over a large evidence block can legitimately need more than the
// default to produce its first token (#21829), so operators can raise the
// budget with AGENTIC_FINALIZE_TIMEOUT (Go duration, e.g. "5m"); the default
// stays 90s.
var finalizeTimeout = durationFromEnv("AGENTIC_FINALIZE_TIMEOUT", 90*time.Second)

// finalizeEvidenceMaxRunes caps the evidence block handed to the synthesis
// call. Tool outputs accumulate 4KB each; without a cap a long investigation
// turns the prompt into a mega-message that makes thinking models deliberate
// for minutes and blow the client timeout (observed: exactly 300s on q55).
//
// It is deliberately larger than the truncation the run itself used: when the
// token ceiling fires, this block is the ONLY thing the synthesis call can
// answer from, so cutting it short is what makes a capped run answer worse than
// an uncapped one. 40k runes stays inside the 90s synthesis budget measured on
// this corpus.
const finalizeEvidenceMaxRunes = 40000

// claimAnswerTimeout bounds the claims path as a whole: an answer call over
// evidence that was already retrieved and, only if it produced an answer, its
// verification. Both are non-streaming and need a fraction of the loop's budget -
// and a hung provider must not eat the turn. AGENTIC_CLAIM_ANSWER_TIMEOUT
// raises it for a provider that legitimately needs longer than the 90s
// default to start answering (#21829).
var claimAnswerTimeout = durationFromEnv("AGENTIC_CLAIM_ANSWER_TIMEOUT", 90*time.Second)

// durationFromEnv parses one Go-duration environment variable, falling back to
// the default when unset or malformed so a typo cannot zero out a budget.
func durationFromEnv(name string, fallback time.Duration) time.Duration {
	if raw := strings.TrimSpace(os.Getenv(name)); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			return d
		}
		// Package-level vars evaluate before main installs the logger, so
		// WarnCtx would drop this warning on every startup path; stderr is
		// the only channel that exists that early.
		if common.Logger != nil {
			common.WarnCtx(context.Background(), "agentic_rag: ignoring malformed duration override",
				zap.String("env", name), zap.String("value", raw), zap.Duration("default", fallback))
		} else {
			fmt.Fprintf(os.Stderr, "agentic_rag: ignoring malformed duration override %s=%q, using default %s\n", name, raw, fallback)
		}
	}
	return fallback
}

// synthModelFor picks the model that answers without tools: the caller's
// dedicated synthesis instance when it supplied one, else the agent's own model.
func synthModelFor(in Input) *models.EinoChatModel {
	if in.SynthModel != nil {
		return in.SynthModel
	}
	return in.Model
}

// claimAnswerPrompt renders the user message of the call that produces a
// candidate answer from the recalled claims.
//
// One call produces it, and the sufficiency judgement is made afterwards, by
// claimVerifyPrompt, on the answer itself. Judging the page BEFORE answering was
// measured and rejected: with no answer to commit to, the model answered "is the
// page enough?" too harshly, refusing five questions whose facts were all on the
// page (gap_overlap 0.86-1.00), and a refusal is what feeds the loop - q142 cost
// 12.6k as an answer and 590,139 after a refusal, for the same 4 points. An
// answer is also the only thing that exposes the failure this corpus produces
// (the film's release year given as the year of the award), because a page can
// hold the wrong-looking claim without the answer using it.
//
// It carries the rules the loop would otherwise hold in its system prompt: every
// fact line must be traceable to the claim it came from via its [ID:n] handle
// (the pipeline turns those handles into the reference payload), and nothing may
// be extrapolated. A computation must name its inputs: "the last time they beat
// them" answered from the wrong edition of the same fact is a real error here,
// and naming the inputs is what makes it visible. It deliberately does NOT
// advertise tools: this call has none, so a model that looks for one would burn
// its only turn.
func claimAnswerPrompt(evidence string) string {
	var b strings.Builder
	b.WriteString("### Compiled evidence retrieved for this question\n")
	b.WriteString("Each line below is one compiled claim: a statement, the verbatim source quote it was compiled from, ")
	b.WriteString("the chunk that quote came from, and the passage handle `ref=n` it was shown as.\n\n")
	b.WriteString(evidence)
	b.WriteString("\n\n### Task\n")
	b.WriteString("Answer the question above from these claims.\n")
	b.WriteString("- End every line that states a fact with the handle of the claim it came from: `[ID:n]`, where n ")
	b.WriteString("is that claim's `ref=n` value. Never introduce an entity, date or number that no claim states. ")
	b.WriteString("Write the answer in the user's language.\n")
	b.WriteString("- Combining claims IS answering: a difference, a count, an age, or a who/which that no single ")
	b.WriteString("claim states outright is derived from the claims that give its inputs.\n")
	b.WriteString("- Use the fact the question actually asks for. When the page states several candidates - two ")
	b.WriteString("editions of an event, a release year and an award year, several people with the same role - pick ")
	b.WriteString("the one the question names (last, first, most, the award, the time of the battle) instead of the ")
	b.WriteString("first one you read.\n")
	b.WriteString("- A count must be taken from the complete list the page gives, not from the entries you happened ")
	b.WriteString("to read; a difference must name both operands.\n")
	b.WriteString("- If the answer is computed from claims, finish with one line naming every claim it used: ")
	b.WriteString("`derived_from: [ID:a], [ID:b]`. A derivation with one input is not a derivation.\n")
	b.WriteString("- If a fact the answer needs is absent from every claim, do NOT guess and do NOT use world ")
	b.WriteString("knowledge: report the question as not answerable and name that fact.\n")
	b.WriteString("- Do not call tools and do not narrate what you are about to do: this call is the whole ")
	b.WriteString("turn, so a plan or a promise to search further is not an answer.\n\n")
	b.WriteString("Reply with ONE JSON object and nothing else:\n")
	b.WriteString(`{"answerable": true, "answer": "<the final answer, with [ID:n] handles>"}`)
	b.WriteString(" or ")
	b.WriteString(`{"answerable": false, "missing": "<the fact that is absent>"}`)
	return b.String()
}

// claimVerifyPrompt renders the user message of the verification call: the one
// place the question "does this answer the question that was asked?" is decided.
//
// It is asked with the answer in hand, which is what makes it tractable, and it
// is asked about ONE failure mode: the near miss. Both wrong shortcuts measured
// on this corpus were near misses - the release year given for the year of the
// award, and an earlier World Series win given for "the last time they beat
// them" while the page also stated the later one. Neither is detectable by asking
// whether the page was sufficient, because the page WAS sufficient; the error is
// in which fact the answer used.
//
// It explicitly forbids rewriting the answer or answering the question, so this
// call stays a check rather than a second opinion that could itself be wrong in
// a new way.
func claimVerifyPrompt(evidence, answer string) string {
	var b strings.Builder
	b.WriteString("### Compiled evidence that was available\n")
	b.WriteString(evidence)
	b.WriteString("\n\n### Candidate answer\n")
	b.WriteString(answer)
	b.WriteString("\n\n### Task\n")
	b.WriteString("Check ONE thing: is this candidate answer the answer to the question that was asked?\n")
	b.WriteString("- Look for a near miss: a fact about the right subject that answers a DIFFERENT question. The ")
	b.WriteString("release year instead of the year of the award. An earlier edition of an event when the question ")
	b.WriteString("says last/latest. One candidate when the question asks for the first, the most, or the one that ")
	b.WriteString("holds a named office. A namesake instead of the person. If another claim on the page fits the ")
	b.WriteString("question better than the one the answer used, that is a near miss and the answer is wrong.\n")
	b.WriteString("- Every date, number, title and entity the answer states must be stated by the claim it cites, ")
	b.WriteString("or computed from the claims it names. A computation is fine when every input is on the page and ")
	b.WriteString("the operands named really are those inputs.\n")
	b.WriteString("- The page is the only source. \"It is well known that\" is not support, and a fact only the ")
	b.WriteString("answer knows is not evidence.\n")
	b.WriteString("- Do not rewrite the answer, do not improve it, and do not answer the question yourself. Judge ")
	b.WriteString("only whether this answer answers this question.\n\n")
	b.WriteString("Reply with ONE JSON object and nothing else:\n")
	b.WriteString(`{"supported": true}`)
	b.WriteString(" or ")
	b.WriteString(`{"supported": false, "reason": "<wrong attribute, ignored candidate, or unsupported fact>"}`)
	return b.String()
}

// claimAnswer is the answer call's reply. answerable=false is not an error: the
// page may genuinely not hold the asked-for fact, and the caller then runs the
// ordinary loop instead of shipping a non-answer.
type claimAnswer struct {
	answerable bool
	answer     string
	missing    string
	// derivedFrom is the handles the answer says it computed from, when it gave
	// them as a JSON field rather than as the `derived_from:` line.
	derivedFrom []string
}

// parseClaimAnswer reads the JSON answer. A reply with no answer field is
// reported as not answerable: shipping unparsed prose is how a turn ends up
// scored zero, and one extra loop is far cheaper.
func parseClaimAnswer(reply string) claimAnswer {
	obj, ok := decodeJSONObject(reply)
	if !ok {
		return claimAnswer{missing: "model did not return the requested JSON verdict"}
	}
	out := claimAnswer{missing: strings.TrimSpace(stringFromAny(obj["missing"]))}
	if s, ok := obj["answer"].(string); ok {
		out.answer = strings.TrimSpace(s)
	}
	if v, ok := obj["answerable"].(bool); ok {
		out.answerable = v
	} else {
		// A model that answers without the flag has still answered. Reading the
		// missing field as a refusal would hand a usable answer to the loop, which
		// is the expensive direction of this mistake.
		out.answerable = out.answer != ""
	}
	if list, ok := obj["derived_from"].([]any); ok {
		for _, item := range list {
			if h := strings.TrimSpace(stringFromAny(item)); h != "" {
				out.derivedFrom = append(out.derivedFrom, h)
			}
		}
	}
	if out.answerable && out.answer == "" {
		out.answerable = false
	}
	return out
}

// claimSupport is the verifier's finding on a candidate answer.
type claimSupport struct {
	supported bool
	reason    string
	// parsed records whether the reply was the requested object at all. A reply
	// that is not is treated as support (see parseClaimSupport) but must stay
	// visible in the logs, or a prompt regression would look like a clean run.
	parsed bool
}

// parseClaimSupport reads the verifier's JSON. A reply that is not the requested
// object counts as SUPPORTED. The two mistakes are not symmetric: rejecting a
// good answer hands the question to the loop for 300-600k tokens, while shipping
// a wrong one costs 2-4 points, so the expensive mistake is the rejection. The
// caller logs the unparsed case so this default cannot hide one.
func parseClaimSupport(reply string) claimSupport {
	obj, ok := decodeJSONObject(reply)
	if !ok {
		return claimSupport{supported: true}
	}
	v, ok := obj["supported"].(bool)
	if !ok {
		return claimSupport{supported: true}
	}
	return claimSupport{supported: v, reason: strings.TrimSpace(stringFromAny(obj["reason"])), parsed: true}
}

// claimHandleCount counts the distinct [ID:n] handles an answer cites.
func claimHandleCount(answer string) int {
	return claimDistinctHandles(claimHandleRe.FindAllString(answer, -1))
}

// claimDerivedInputs returns the handles an answer says it computed from: the
// JSON field when it gave one, else the `derived_from:` line the prompt asks for
// inside the answer text.
func claimDerivedInputs(a claimAnswer) []string {
	if len(a.derivedFrom) > 0 {
		return a.derivedFrom
	}
	if m := claimDerivedLineRe.FindStringSubmatch(a.answer); m != nil {
		return claimHandleRe.FindAllString(m[1], -1)
	}
	return nil
}

func claimDistinctHandles(handles []string) int {
	seen := map[string]struct{}{}
	for _, h := range handles {
		seen[h] = struct{}{}
	}
	return len(seen)
}

var (
	claimHandleRe      = regexp.MustCompile(`\[ID:\d+\]`)
	claimDerivedLineRe = regexp.MustCompile(`(?i)derived_from:\s*([^\n]*)`)
)

// answerFromClaims is the termination of a turn whose pre-loop compiled
// prefetch already found claims for the question: an answer call over them, then
// - only if it produced an answer - a verification call. No tools, so no raw
// chunk is read either way.
//
// The order is deliberate and measured. Verification runs AFTER the answer, not
// before it: a sufficiency call with no answer in hand refuses pages that hold
// every needed fact, and each of those refusals costs the loop 300-600k tokens
// for the same final score. With the answer in hand the check is narrower and
// cheaper per question (it only runs when there IS an answer), and it can look
// for the failure that actually happens here: the right subject, the wrong fact
// (a release year given for an award year, an earlier edition given for "the
// last time").
//
// It returns ("", nil) when no answer survives, which the caller treats as a
// reason to run the ordinary loop instead - the loop, not this path, decides
// what to search next.
func answerFromClaims(
	ctx context.Context,
	m *models.EinoChatModel,
	instruction string,
	messages []*schema.Message,
	pf compiledPrefetch,
) (string, error) {
	if m == nil || strings.TrimSpace(pf.evidence) == "" {
		return "", nil
	}
	// Its own usage phase: the share of turns that never entered the loop is
	// exactly what a cost comparison against the loop path needs.
	ctx, cancel := context.WithTimeout(tokenizer.WithUsagePhase(ctx, "claims"), claimAnswerTimeout)
	defer cancel()

	// The template's instruction goes in as the system message because this path
	// bypasses the agent that would otherwise carry it: the answer must keep the
	// same shape and language rules as every other answer this template ships.
	ask := func(prompt string) (string, error) {
		req := make([]*schema.Message, 0, len(messages)+2)
		req = append(req, schema.SystemMessage(instruction))
		req = append(req, messages...)
		req = append(req, schema.UserMessage(prompt))
		resp, err := m.Generate(ctx, req)
		if err != nil {
			return "", err
		}
		if resp == nil {
			return "", nil
		}
		return resp.Content, nil
	}

	answerReply, err := ask(claimAnswerPrompt(pf.evidence))
	if err != nil {
		return "", err
	}
	answer := parseClaimAnswer(answerReply)
	if !answer.answerable {
		// gap_overlap splits the two ways a refusal can happen, and it is the only
		// signal that tells them apart in the logs: a gap whose words ARE on the
		// page means the model would not use what it had (selection or prompt),
		// a gap whose words are not means the page never held the fact
		// (retrieval). Both need different work, and only this number says which.
		common.InfoCtx(ctx, "agentic_rag: claims judged unanswerable by the model",
			zap.Int("claims", len(pf.claims)),
			zap.Int("chunks", len(pf.chunks)),
			zap.Strings("docs", claimDocsInOrder(pf.claims)),
			zap.String("missing", truncateForLog(answer.missing, 200)),
			zap.Float64("gap_overlap", claimGapOverlap(pf.claims, answer.missing)),
			zap.Int("reply_bytes", len(answerReply)))
		return "", nil
	}
	// Deterministic floors, checked before the verifier because they are free.
	// An answer that cites nothing cannot be resolved into a reference payload,
	// and an answer that names a derivation but only one input has not derived
	// anything - both are the shape a near miss takes.
	handles := claimHandleCount(answer.answer)
	if handles == 0 {
		common.InfoCtx(ctx, "agentic_rag: claim answer cites no claim; falling back to the loop",
			zap.Int("claims", len(pf.claims)),
			zap.Int("answer_bytes", len(answer.answer)))
		return "", nil
	}
	if derived := claimDerivedInputs(answer); len(derived) > 0 {
		if n := claimDistinctHandles(derived); n < 2 {
			common.InfoCtx(ctx, "agentic_rag: claim answer derives from one claim; falling back to the loop",
				zap.Int("derived_inputs", n),
				zap.Int("claims", len(pf.claims)),
				zap.Int("answer_bytes", len(answer.answer)))
			return "", nil
		}
	}

	verifyReply, err := ask(claimVerifyPrompt(pf.evidence, answer.answer))
	if err != nil {
		return "", err
	}
	support := parseClaimSupport(verifyReply)
	if !support.parsed {
		common.WarnCtx(ctx, "agentic_rag: claim verification reply was not the requested JSON; answer accepted",
			zap.Int("reply_bytes", len(verifyReply)))
	}
	if !support.supported {
		common.InfoCtx(ctx, "agentic_rag: claim answer rejected by verification; falling back to the loop",
			zap.String("reason", truncateForLog(support.reason, 200)),
			zap.Int("handles", handles),
			zap.Int("claims", len(pf.claims)),
			zap.Int("answer_bytes", len(answer.answer)))
		return "", nil
	}
	return answer.answer, nil
}

// claimGapOverlap is the share of the refused gap's content words that the page's
// claims already state. It is a diagnostic, not a control signal: the refusal
// still falls back to the loop. See the log line it feeds.
func claimGapOverlap(claims []claimHit, gap string) float64 {
	gapTokens := claimTokens(gap)
	if len(gapTokens) == 0 {
		return 0
	}
	page := make(map[string]struct{}, len(claims)*8)
	for _, c := range claims {
		for t := range claimTokens(c.name + " " + c.description + " " + c.quote) {
			page[t] = struct{}{}
		}
	}
	hit := 0
	for t := range gapTokens {
		if _, ok := page[t]; ok {
			hit++
		}
	}
	return float64(hit) / float64(len(gapTokens))
}

// finalizeAnswer issues a single deterministic synthesis call so a turn that
// ended abnormally (empty final, or an error aborted the loop mid-investigation)
// still returns a grounded answer instead of a blank. It feeds the model the
// original user question, whatever partial assistant output was produced, and a
// truncated summary of the evidence retrieved so far, and instructs it to commit
// to the best answer it can — using the closest available data with an explicit
// assumption when exact figures are missing.
func finalizeAnswer(
	ctx context.Context,
	m *models.EinoChatModel,
	messages []*schema.Message,
	partial, evidence string,
	structuredOnly bool,
) (string, error) {
	question := lastUserQuestion(messages)
	body := new(strings.Builder)
	fmt.Fprintf(body, "You were asked:\n%s\n\n", question)
	structured := structuredOnly && hasFOSStructure(partial)
	if strings.TrimSpace(partial) != "" {
		fmt.Fprintf(body, "The research so far produced this partial output:\n%s\n\n", partial)
	}
	if ev := strings.TrimSpace(evidence); ev != "" {
		fmt.Fprintf(body, "Evidence gathered during the search:\n%s\n\n", truncateRunes(ev, finalizeEvidenceMaxRunes))
	}
	if structured {
		// The partial output IS the audited deliverable (Candidate Matrix /
		// Reasoning Chain) minus the FOS answer line. Re-render it whole:
		// collapsing it to a bare answer throws away the corpus-grounded
		// reasoning chain and chunk citations (q71 shipped a 3-char "EF5"
		// that way — correct, but with no trace of how it was derived).
		body.WriteString("The partial output above already contains the deliverable's structure " +
			"(`## Candidate Matrix` and/or `## Reasoning Chain`). Re-render the COMPLETE final " +
			"deliverable now: keep every section and chunk citation the evidence supports, " +
			"repair or drop the lines it does not, and end with the LAST line " +
			"`Final Answer: **<value>**` (or `Guessed Answer: **<value>** (assumption: ...)`). " +
			"Do not request any tools and do not hedge with \"I would need\" or \"I could not find\".")
	} else {
		body.WriteString(finalizeInstruction)
	}

	finalMessages := append([]*schema.Message(nil), messages...)
	finalMessages = append(finalMessages, &schema.Message{
		Role:    schema.User,
		Content: body.String(),
	})
	resp, err := m.Generate(ctx, finalMessages)
	if err != nil {
		return "", err
	}
	if resp == nil || resp.Content == "" {
		return "", errors.New("agentic_rag: finalizeAnswer produced empty output")
	}
	synth := strings.TrimSpace(resp.Content)
	if structured && !hasFOSStructure(synth) {
		// Safety net: the re-render dropped the structure. Graft the bare
		// answer onto the audited partial instead of shipping it alone, so
		// the reasoning chain survives even a non-compliant synthesis call.
		//
		// Two things this must NOT do. It must not wrap prose that carries no
		// answer line in a `Final Answer:` label — that label asserts the run
		// verified the value against the corpus, and the producer acceptance
		// check reads any such line as a deliverable. It must not restore
		// citations the gate refused, so the graft is only taken when the
		// synthesis actually produced an answer line (i.e. its own claim to
		// have answered); otherwise the shorter, honest text ships alone.
		if hasAnswerLine(synth) {
			return strings.TrimSpace(partial) + "\n\n" + synth + "\n", nil
		}
		return synth + "\n", nil
	}
	out := synth
	// A forced answer is still an answer: a bare "1" is not one.
	//
	// The ceiling ends a run mid-investigation, and a model told to reply with
	// "the final answer ONLY" will happily emit the number and nothing else —
	// which is indistinguishable, to the reader, from a broken run. One retry
	// asking for the full answer is cheaper than shipping the digit.
	if len([]rune(out)) < finalizeMinAnswerRunes {
		retry := append([]*schema.Message(nil), finalMessages...)
		retry = append(retry, &schema.Message{
			Role: schema.User,
			Content: fmt.Sprintf("Your reply (%q) is not an acceptable answer: it is too short to be useful. "+
				"Answer the question in full sentences, naming the entities and the figures the evidence "+
				"supports, and cite the chunk handles ([ID:n]) you relied on.", out),
		})
		if again, rerr := m.Generate(ctx, retry); rerr == nil && again != nil {
			if longer := strings.TrimSpace(again.Content); len([]rune(longer)) > len([]rune(out)) {
				out = longer
			}
		}
	}
	return out, nil
}

// finalizeInstruction is what the synthesis call is asked to do. It no longer
// says "final answer ONLY": that phrasing produced bare numbers on capped runs
// (measured: a 1-byte "1"), and the point of the fallback is a usable answer,
// not the shortest one.
const finalizeInstruction = "You have reached the end of your investigation. Based strictly on the evidence " +
	"above, produce your final answer to the original question now, as plain text. " +
	"Give a complete answer: state the result, then the facts from the evidence that support it, " +
	"with the [ID:n] handles you relied on. Do NOT reply with a bare number or a single word — " +
	"if the question asks how many, give the count AND what it counts. " +
	"If the exact data is not available, use the closest available figure, state the assumption " +
	"explicitly, and still provide the computed answer. Do not request any tools and do not hedge " +
	"with \"I would need\" or \"I could not find\"."

// finalizeMinAnswerRunes is the length below which a forced answer is treated as
// a non-answer and retried once.
//
// It is deliberately short: the case it exists for is a bare figure ("1"), and
// a threshold high enough to catch a real sentence would retry answers that are
// terse but complete, doubling the synthesis cost of every capped run.
const finalizeMinAnswerRunes = 12

// lastUserQuestion returns the content of the last user message, used as the
// question for the final-answer fallback and for per-question usage logs.
func lastUserQuestion(messages []*schema.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i] != nil && messages[i].Role == schema.User {
			return messages[i].Content
		}
	}
	return ""
}

// priorTurns returns the messages BEFORE the last user message — the earlier
// turns of a multi-turn conversation, which the plan stage resolves
// the pinned question's references against (a follow-up like "and his
// spouse?" names no entity of its own). Everything from the last user message
// on is excluded: the message itself is pinned in the planner's instructions,
// and a later assistant draft must not masquerade as settled context. Empty
// for a single-turn conversation.
func priorTurns(messages []*schema.Message) []*schema.Message {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i] != nil && messages[i].Role == schema.User {
			return messages[:i]
		}
	}
	return nil
}

// Every debug log carries its fields WHOLE - a truncated field is an unreadable
// field, and a cut tool result once hid the exact evidence a forensic needed
// (the difference between a one-look diagnosis and a wrong attribution). The
// log volume at debug level is the accepted cost; the model-facing evidence
// assembly is the only place a cap survives, and there it is context hygiene,
// not concealment.
func lastNonBlankLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(lines[i]); t != "" {
			return t
		}
	}
	return ""
}

func truncateForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// truncateRunes caps a string to max runes (not bytes) so CJK content is never
// cut mid-rune into invalid UTF-8.
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "..."
}

// unknownToolResult answers a call to a tool this run never registered.
//
// The capability set is built per conversation on purpose: advertising a tool
// the run cannot execute is worse than not advertising it at all. But the
// prompt is shared, and it names tools a deployment may not supply — web_search
// with internet off is the common case — so the model occasionally calls one.
// eino's default is to raise "tool X not found in toolsNode indexes" and abort
// the turn, which turns a recoverable detour into a zero-scored run. Answering
// with an ordinary tool result lets the model drop that leg and keep reading.
func unknownToolResult(_ context.Context, name, _ string) (string, error) {
	return fmt.Sprintf("Tool %q is not available in this conversation: it is not registered, "+
		"so calling it again fails the same way. Continue with the tools in your tool list.", name), nil
}
