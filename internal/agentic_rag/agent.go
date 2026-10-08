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
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/entity/models"
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

var errNilModel = errors.New("agentic_rag: model is required")

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

	tmpl, errT := resolveTemplateFor(in.TemplateID)
	if errT != nil {
		common.ErrorCtx(ctx, "agentic_rag: resolve template", errT)
		return "", errT
	}

	// Shared per-run duration tally: every tool invocation is timed and
	// accumulated here for per-question usage accounting.
	if in.ToolCallDurations == nil {
		in.ToolCallDurations = NewDurationAccumulator()
	}

	tools := in.Tools
	if len(tools) == 0 {
		// No explicit tool set: build it from the requested template, which
		// lets operators change the tool list without recompiling. The config
		// is reloaded from disk when its mtime changes, so a quick edit +
		// re-run is enough to try a different tool subset or prompt variant.
		tools = toolsFor(tmpl, in.TenantID, in.DatasetIDs)
		// Web search is injected, never declared: the template's tool list
		// describes the corpus toolset, and a conversation whose context
		// carries a provider gets one extra tool at run time.
		tools = append(tools, webSearchTools(ctx)...)
	}

	maxIter := in.MaxIterations
	if maxIter <= 0 {
		maxIter = defaultMaxIterations
	}

	// Wrap every invokable tool so each call is timed and emitted as a debug
	// log, accumulated into in.ToolCallDurations. Tools that aren't
	// InvokableTool (e.g. streamable-only) are passed through unwrapped — all
	// agent tools here are InvokableTool.
	wrapped := make([]tool.BaseTool, len(tools))
	for i, t := range tools {
		if it, ok := t.(tool.InvokableTool); ok {
			wrapped[i] = &instrumentedTool{
				InvokableTool: it,
				acc:           in.ToolCallDurations,
			}
		} else {
			wrapped[i] = t
		}
	}
	tools = wrapped

	common.DebugCtx(ctx, "agentic_rag: run start",
		zap.Int("max_iterations", maxIter),
		zap.Int("messages", len(in.Messages)),
		zap.Int("tools", len(tools)),
	)

	cfg := &adk.ChatModelAgentConfig{
		Name:          "smart-reasoning",
		Instruction:   instructionFor(tmpl),
		Model:         in.Model,
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
				// parallel intent is not accidental. All six tools
				// (think/todo_write/grep_chunks/search_chunks/list_chunks/run_javascript)
				// are concurrency-safe: they keep no shared mutable state across
				// calls (run_javascript creates a fresh goja VM per invocation).
				ExecuteSequentially: false,
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
	question := lastUserQuestion(in.Messages)
	seed := in.Messages
	if len(seed) > 0 {
		// The final user message is the new turn and must be appended exactly
		// once by turnMessages.
		seed = seed[:len(seed)-1]
	}
	runMessages := sess.explorer.turnMessages(seed, "", question)
	iter := sess.explorer.runner(ctx, explorerAgent, in.Stream).Run(ctx, runMessages)
	final, evidence, runErr := consumeAgentEvents(ctx, iter, in.OnDelta, in.ToolCallCounts, in.ToolCallErrors, in.ToolErrorSamples)

	// Recovery, cheapest and most faithful first. A run that ended on a
	// narration tail or on a bare tool call still holds the model's own answer
	// somewhere in its turns — shipping it beats a synthesis call, which
	// discards that work and, once a failover cooldown has set in, may not
	// even reach a provider (it would be short-circuited with the stale error
	// that tripped the cooldown).
	if final == "" || runErr != nil {
		if carried := sess.explorer.lastAssistant(ctx, func(m *schema.Message) bool {
			return strings.TrimSpace(m.Content) != ""
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
		synthModel := in.Model
		if in.SynthModel != nil {
			synthModel = in.SynthModel
		}
		synth, synthErr := finalizeAnswer(synthCtx, synthModel, in.Messages, final, evidence)
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
	for {
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
const finalizeEvidenceMaxRunes = 16000

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
	body.WriteString("You have reached the end of your investigation. Based strictly on the evidence " +
		"above, produce your best final answer to the original question now, as plain text. " +
		"Reply with the final answer ONLY — no reasoning process, no plan, no narration. " +
		"If the exact data is not available, use the closest available figure, state the " +
		"assumption explicitly, and still provide the computed answer. Do not request any tools " +
		"and do not hedge with \"I would need\" or \"I could not find\".")

	resp, err := m.Generate(ctx, []*schema.Message{{
		Role:    schema.User,
		Content: body.String(),
	}})
	if err != nil {
		return "", err
	}
	if resp == nil || resp.Content == "" {
		return "", errors.New("agentic_rag: finalizeAnswer produced empty output")
	}
	return strings.TrimSpace(resp.Content), nil
}

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
