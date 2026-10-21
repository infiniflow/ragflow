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
	"regexp"
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
	// (e.g. "smart-reasoning", "smart-grep"), carried through from the
	// request's agent_mode. It is mandatory: an empty or unknown id fails the
	// run with a clear error — there is no default/env/first-template fallback.
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
// every ChatModelAgent this package builds (the explorer): provider hiccups —
// MiniMax 529 overload, 429 rate limits, 5xx, connection resets — are retried
// with seconds-scale backoff, while client errors (400/401) and exhausted
// deadlines fail fast.
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

// instrumentedTool decorates a tool.InvokableTool, accumulating the total
// wall-clock duration of every InvokableRun into a shared accumulator keyed by
// the wrapped tool's name. It embeds tool.InvokableTool (which itself embeds
// tool.BaseTool), promoting Info, then overrides InvokableRun to time the call.
type instrumentedTool struct {
	tool.InvokableTool
	acc *durationAccumulator
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
	fields := []zap.Field{
		zap.String("tool", name),
		zap.Float64("cost_ms", float64(cost.Milliseconds())),
		// Args are logged IN FULL: they are the run's single variable state - a
		// truncated copy makes before/after comparisons impossible after the
		// fact. Tool args are bounded by construction (queries, ids), so the
		// volume cost is a few kilobytes per call at most.
		zap.String("args", args),
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

	// Identity the judge is chosen for: the decision model is configured per chat
	// and per tenant in the UI, so the filter has to know which run it is
	// serving. The generated key above is a per-turn placeholder and must not be
	// used here — a run with no real chat id falls back to the tenant default,
	// exactly as a chat that never overrode it does.
	ctx = withDecisionScope(ctx, in.TenantID, in.ConversationKey)
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

	// Shared per-run duration tally: every tool invocation is timed and
	// accumulated here for per-question usage accounting.
	if in.ToolCallDurations == nil {
		in.ToolCallDurations = &durationAccumulator{dur: make(map[string]time.Duration)}
	}

	// With no explicit tools, use the requested template. The config reloads
	// when its mtime changes so operators can change the tool list and prompt.
	tools := toolsForRun(ctx, tmpl, in, engine.Get())
	agentModel, err := modelForTools(ctx, in.Model, tools, in.DatasetIDs)
	if err != nil {
		return "", err
	}

	maxIter := agenticMaxIterations(in.MaxIterations)

	// ONE claim recall per turn, BEFORE the loop, consumed as a control signal
	// rather than as prompt text.
	//
	// A recall that covers the question needs no document and no chunk: the
	// claims carry their own verbatim evidence, so the turn terminates on a
	// single model call over them. Descending the structure again, or reading
	// the raw chunks they were compiled from, could only re-derive what the
	// claims already state - and every additional step is another chance to
	// prune away the branch that holds the answer.
	//
	// A recall that does not cover the question is discarded here. The loop then
	// runs exactly as it would have without the recall, and navigate_structure
	// stays available for the model to call on demand.
	if len(in.Tools) == 0 && !tmpl.strictTools {
		pf := recallCompiledClaims(ctx, in.TenantID, in.DatasetIDs, in.Messages)
		if pf.sufficient {
			answer, aerr := answerFromClaims(ctx, synthModelFor(in), tmpl.Content, in.Messages, pf)
			switch {
			case aerr != nil:
				// Falling through keeps the turn answerable: the claims are
				// dropped from the registry's numbering (nothing was shown) and
				// the loop retrieves what it needs itself.
				common.WarnCtx(ctx, "agentic_rag: claim answer failed; falling back to the loop", zap.Error(aerr))
			case strings.TrimSpace(answer) != "":
				// The loop is skipped entirely, so this line is the only record
				// that the turn never read a chunk: a review of tool usage must
				// be able to tell it from a turn the model gave up on.
				common.InfoCtx(ctx, "agentic_rag: answered from compiled claims",
					zap.Int("claims", len(pf.claims)),
					zap.Int("docs", len(pf.docs)),
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

	// Wrap every invokable tool so each call is timed and emitted as a debug
	// log, accumulated into in.ToolCallDurations. Tools that aren't
	// InvokableTool (e.g. streamable-only) are passed through unwrapped — all
	// agent tools here are InvokableTool.
	wrapped := make([]tool.BaseTool, len(tools))
	for i, t := range tools {
		if it, ok := t.(tool.InvokableTool); ok {
			timed := &instrumentedTool{
				InvokableTool: it,
				acc:           in.ToolCallDurations,
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

	// The explorer is driven as a MANAGED SESSION. Seed the request-scoped
	// session with the compacted conversation history, then hand over the new
	// user turn; subsequent turns in the same session can rely on the event log
	// replay (see run_session.go).
	sess := newRunSession()

	// EnableStreaming lives on RunnerConfig, not ChatModelAgentConfig.
	explorerHead := sess.explorer.head(ctx)
	question := schema.UserMessage(lastUserQuestion(in.Messages))
	seed := in.Messages
	if len(seed) > 0 {
		question = seed[len(seed)-1]
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

	// Recovery, cheapest and most faithful first. A run that ended on a
	// narration tail or on a bare tool call still holds the model's own answer
	// somewhere in its turns — shipping it beats a synthesis call, which
	// discards that work and, once a failover cooldown has set in, may not
	// even reach a provider (it would be short-circuited with the stale error
	// that tripped the cooldown).
	if final == "" || runErr != nil {
		if carried := sess.explorer.lastAssistant(ctx, func(m *schema.Message) bool {
			return !inputMessageIDs[adk.GetMessageID(m)] && strings.TrimSpace(m.Content) != ""
		}); carried != "" {
			final = carried
			runErr = nil
		}
	}
	if runErr != nil {
		// Read the recovery candidate before rollback removes the failed turn.
		sess.explorer.discardFailedTurn(ctx, explorerHead)
	} else {
		sess.explorer.markSeeded()
	}

	// Terminating action: the turn must never end on narration or a blank.
	// Synthesize deterministically from the question, whatever partial output
	// exists, and the evidence gathered so far.
	if final == "" || runErr != nil {
		// Detach from the run's cancellation entirely: the fallback fires most
		// often BECAUSE the shared wall-clock budget expired, and a deadline
		// expiry and a client hang-up are indistinguishable to the derived
		// context (both close the same Done channel) — keeping cancellation
		// would reintroduce the failure this call exists to survive. The
		// unbounded-work risk that WithoutCancel normally carries is covered
		// by the bounded budget attached here.
		synthCtx, synthCancel := context.WithTimeout(context.WithoutCancel(ctx), finalizeTimeout)
		defer synthCancel()
		synthCtx = tokenizer.WithUsagePhase(synthCtx, "synth")
		synthMessages := in.Messages
		if tmpl.customContent {
			synthMessages = append([]*schema.Message{schema.SystemMessage(tmpl.Content)}, synthMessages...)
		}
		synth, synthErr := finalizeAnswer(synthCtx, synthModelFor(in), synthMessages, final, evidence)
		if synthErr != nil {
			// Without this an empty final is completely silent: the run looks
			// healthy in every other log line and the user simply gets nothing.
			common.WarnCtx(ctx, "agentic_rag: finalizeAnswer failed", zap.Error(synthErr))
		} else if strings.TrimSpace(synth) == "" {
			common.WarnCtx(ctx, "agentic_rag: finalizeAnswer returned an empty answer")
		}
		switch {
		case synthErr == nil && strings.TrimSpace(synth) != "":
			// Replace rather than append: appending leaves the reader with a
			// narration paragraph followed by the real answer. The single
			// answer emission at the end of Run carries it.
			final = synth
			runErr = nil
		case runErr == nil:
			// Normal termination but synthesis failed — surface the reason.
			runErr = synthErr
		}
	}

	// The answer channel carries exactly one thing: the deliverable this run is
	// shipping, emitted once. Everything the loop said on the way there already
	// went out live as thinking, so the user watched the work without being
	// handed several competing answers.
	emit(ctx, in.OnDelta, final, "")
	return final, runErr
}

var modelFailureRe = regexp.MustCompile(`exceeds max retries|failed on every model|short-circuited by failover cooldown|overloaded_error|status 529|insufficient balance`)

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
	// A ReAct turn emits many messages, any of which can already look like an
	// answer — the loop often renders one, keeps retrieving, and renders a
	// better one. Routing those to the answer channel is what put several
	// competing answers in front of the user. On the thinking channel they stay
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
const finalizeTimeout = 90 * time.Second

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

// claimAnswerTimeout bounds the sufficient-claims answer. It is a single
// non-streaming call over evidence that was already retrieved, so it needs a
// fraction of the loop's budget - and a hung provider must not eat the turn.
const claimAnswerTimeout = 90 * time.Second

// synthModelFor picks the model that answers without tools: the caller's
// dedicated synthesis instance when it supplied one, else the agent's own model.
func synthModelFor(in Input) *models.EinoChatModel {
	if in.SynthModel != nil {
		return in.SynthModel
	}
	return in.Model
}

// claimAnswerPrompt renders the user message of the sufficient-claims answer.
//
// It states the rules the loop would otherwise carry in its system prompt, in
// the one place they apply: the answer must be built from these claims, each
// fact line must carry the [ID:n] handle of the passage it came from (the
// pipeline turns those handles into the reference payload), and nothing may be
// extrapolated. It deliberately does NOT advertise tools: this call has none, so
// a model that looks for one would burn its only turn.
//
// The reply is a JSON verdict, not prose, because the caller has to act on it:
// "these claims do not answer the question" is a routing decision (fall back to
// the loop), and reading it reliably out of free text is not possible. A model
// that cannot answer from the claims says so here instead of shipping a
// non-answer, which is exactly what a zero-score benchmark run looked like.
func claimAnswerPrompt(evidence string) string {
	var b strings.Builder
	b.WriteString("### Compiled evidence retrieved for this question\n")
	b.WriteString("Each line below is one compiled claim: a statement, the verbatim source quote it was compiled from, ")
	b.WriteString("the chunk that quote came from, and the passage handle `ref=n` it was shown as.\n\n")
	b.WriteString(evidence)
	b.WriteString("\n\n### Task\n")
	b.WriteString("Decide whether this evidence answers the question above, then answer from it.\n")
	b.WriteString("- If it does: combine claims where the question needs it, and end every line that states a fact with ")
	b.WriteString("the handle of the claim it came from: `[ID:n]`, where n is that claim's `ref=n` value. Never ")
	b.WriteString("introduce an entity, date or number that no claim states. Write the answer in the user's language.\n")
	b.WriteString("- If it does not (wrong topic, or the specific fact the question asks for is absent): do NOT ")
	b.WriteString("guess and do NOT answer from world knowledge. Report it as not answerable instead.\n")
	b.WriteString("- Do not call tools and do not narrate what you are about to do: this call is the whole ")
	b.WriteString("turn, so a plan or a promise to search further is not an answer.\n\n")
	b.WriteString("Reply with ONE JSON object and nothing else:\n")
	b.WriteString(`{"answerable": true, "answer": "<the final answer, with [ID:n] handles>"}`)
	b.WriteString(" or ")
	b.WriteString(`{"answerable": false, "missing": "<the fact that is absent>"}`)
	return b.String()
}

// claimAnswer is the model's verdict on a sufficient-claims recall. answerable
// false is a real outcome, not an error: it means the gate's relevance floor let
// through claims that do not actually answer the question, so the caller must
// fall back to the ordinary loop instead of shipping a non-answer.
type claimAnswer struct {
	answerable bool
	answer     string
	missing    string
}

// parseClaimAnswer reads the JSON verdict. A reply that is not the requested
// object is reported as not answerable: shipping unparsed prose is how a turn
// with no retrieval ends up scored zero, and one extra loop is far cheaper.
func parseClaimAnswer(reply string) claimAnswer {
	obj, ok := decodeJSONObject(reply)
	if !ok {
		return claimAnswer{missing: "model did not return the requested JSON verdict"}
	}
	out := claimAnswer{missing: stringFromAny(obj["missing"])}
	if v, ok := obj["answerable"].(bool); ok {
		out.answerable = v
	}
	if s, ok := obj["answer"].(string); ok {
		out.answer = strings.TrimSpace(s)
	}
	if out.answerable && out.answer == "" {
		out.answerable = false
	}
	return out
}

// answerFromClaims is the termination of a turn whose pre-loop claim recall
// already covers the question: ONE model call over the claims, with no tools and
// therefore no raw-chunk read. It returns ("", nil) when the model judged the
// claims insufficient, which the caller treats as a reason to run the ordinary
// loop instead.
func answerFromClaims(
	ctx context.Context,
	m *models.EinoChatModel,
	instruction string,
	messages []*schema.Message,
	pf claimPrefetch,
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
	req := make([]*schema.Message, 0, len(messages)+2)
	req = append(req, schema.SystemMessage(instruction))
	req = append(req, messages...)
	req = append(req, schema.UserMessage(claimAnswerPrompt(pf.evidence)))

	resp, err := m.Generate(ctx, req)
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", nil
	}
	verdict := parseClaimAnswer(resp.Content)
	if !verdict.answerable {
		common.InfoCtx(ctx, "agentic_rag: claims judged insufficient by the model",
			zap.Int("claims", len(pf.claims)),
			zap.Strings("docs", pf.docs),
			zap.String("missing", truncateForLog(verdict.missing, 200)))
		return "", nil
	}
	return verdict.answer, nil
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
