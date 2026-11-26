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
	"ragflow/internal/utility"
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
	// Serves, when non-nil, records every chunk the retrieval tools put in
	// front of the model, keyed by chunk id with the document it belongs to.
	// The served-unread watchdog reads it, and the final result reports it as
	// ServedDocIDs so a benchmark can tell a document that was surfaced from
	// one that was actually opened.
	Serves *servedLedger
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

// servedLedger records the documents the retrieval tools served to the model,
// counting - per document - the DISTINCT QUERIES that returned it. The
// served-unread watchdog diffs it against the deep-read document set
// (RetrievedDocIDs, filled only by full-content tools) and names the documents
// that kept surfacing and were never opened. A document served once in a
// passing sweep is noise; one several different searches returned is a
// candidate.
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

// AddDeepIDs records the identifiers of deep-read chunks.
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
		// Args are logged IN FULL: a truncated copy makes a call impossible to
		// reproduce after the fact. Tool args are bounded by construction
		// (queries, ids), so the volume cost is a few kilobytes per call at most.
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

	// Shared per-run tool ledgers.
	if in.ToolCallDurations == nil {
		in.ToolCallDurations = &durationAccumulator{dur: make(map[string]time.Duration)}
	}
	if in.RetrievedDocIDs == nil {
		in.RetrievedDocIDs = NewDocIDLedger()
	}
	if in.ChunkReads == nil {
		in.ChunkReads = NewChunkReadLedger()
	}
	if in.Serves == nil {
		in.Serves = NewServedLedger()
	}
	// The serve ledger rides the context: every retrieval tool's serve point
	// records what it put in front of the model, so the served-unread watchdog
	// can name the documents the retrieval kept insisting on.
	ctx = withServedLedger(ctx, in.Serves)

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
	// The turn hands over only the current question and lets the managed
	// session replay the caller's history.
	question := schema.UserMessage(lastUserQuestion(in.Messages))
	seed := in.Messages
	if len(seed) > 0 {
		// The final user message is the new turn and must be appended exactly
		// once by turnMessages.
		seed = seed[:len(seed)-1]
	}
	runMessages := sess.explorer.turnMessages(seed, "", question)
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

	// Terminating action: the loop ended on narration, on a bare tool call, or
	// on an error. Synthesize an answer deterministically from the question,
	// whatever partial output exists, and the evidence gathered so far — the
	// turn must never end on narration or a blank. A plain-explorer reply is
	// prose by design, so a missing answer line is not a defect and the
	// model's own answer ships as written; only a BLANK or errored turn is
	// recovered here.
	if final == "" || runErr != nil {
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
		synth, synthErr := finalizeAnswer(synthCtx, synthModelFor(in), synthMessages, final, evidence)
		if synthErr == nil && strings.TrimSpace(synth) == "" {
			synthErr = errors.New("empty answer")
		}
		if synthErr != nil {
			synthErr = fmt.Errorf("final answer synthesis failed: %w", withCancellationCause(synthCtx, synthErr))
			// Without this an empty final is completely silent: the run looks
			// healthy in every other log line and the user simply gets nothing.
			common.WarnCtx(ctx, "agentic_rag: finalizeAnswer failed", zap.Error(synthErr))
		}
		switch {
		case synthErr == nil:
			// Replace rather than append: appending leaves the reader with a
			// narration paragraph followed by the real answer. The single
			// answer emission at the end of Run carries it.
			final = synth
			runErr = nil
		case runErr == nil:
			// Normal termination but synthesis failed — surface the reason.
			runErr = synthErr
		default:
			runErr = fmt.Errorf("%w; %w", runErr, synthErr)
		}
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
) (string, error) {
	question := lastUserQuestion(messages)
	body := new(strings.Builder)
	fmt.Fprintf(body, "You were asked:\n%s\n\n", question)
	if strings.TrimSpace(partial) != "" {
		fmt.Fprintf(body, "The research so far produced this partial output:\n%s\n\n", partial)
	}
	if ev := strings.TrimSpace(evidence); ev != "" {
		fmt.Fprintf(body, "Evidence gathered during the search:\n%s\n\n", truncateRunes(ev, finalizeEvidenceMaxRunes))
	}
	body.WriteString(finalizeInstruction)

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
	out := strings.TrimSpace(resp.Content)
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

func truncateForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// truncateRunes caps a string to max runes (not bytes) so CJK content is never
// cut mid-rune into invalid UTF-8. The ellipsis is caller-visible and only
// appears when truncation actually happened.
func truncateRunes(s string, max int) string {
	if truncated := utility.TruncateRunes(s, max); truncated != s {
		return truncated + "..."
	}
	return s
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
