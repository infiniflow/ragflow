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

// Reasoning-step reporting for the agentic RAG run.
//
// A step is a thing a PRODUCER decides to report — there is no allowlist of log
// tags and nothing intercepts the logger. Each stage declares its own step where
// it happens (StepReporter.Stage), and a tool declares both halves of its call
// (StepReporter.Emit). The developer log and the user-visible trace are separate
// channels fed from that one call, so they cannot drift, and nothing becomes
// visible (or invisible) because a log line was reworded.
package runtime

import (
	"context"
	"fmt"
	"log"
	"strings"

	"ragflow/internal/service"
)

// ThinkLineBreak separates two steps inside the think block.
//
// The block is delivered to the client as inline HTML (the chat UI wraps it in
// <details class="think"> and renders the message as markdown), where a newline
// is just whitespace. Mirrors Python think_log.py:69 ("<br>").
const ThinkLineBreak = "<br>"

// StepIndentUnit and StepMaxDepth shape the think block's nesting.
//
// A step is INDENTED when it is reported beneath the step that launched it: a
// prefetch's search legs under the "[Prefetch] … up front." line that opened them,
// a tool's internal searches under the tool call, an action session's tools under
// the research round, and (when an outer react loop is wired) a whole research graph
// under the "Running the rag tool …" line that started it. The depth is carried on
// the context (Nested) because those layers are entered by ordinary calls, so the
// indent costs each producer nothing.
//
// The unit is &nbsp; entities, not spaces: the block is delivered as inline HTML
// (the chat UI wraps it in <details> and renders it as markdown), where a run of
// leading spaces collapses away.
//
// The cap sits above the deepest nesting that actually occurs, so it clips nothing
// real. Counted from the outermost line a run can start at, with an outer react loop
// wired:
//
//	level 0  "Running the rag tool with …"            (outer react loop only)
//	level 1  the graph's own steps — Starting research / Planner / Prefetch /
//	         RAGAgent Round N / Draft / SCA / Finalize
//	level 2  a tool call the round launches, and a prefetch's search legs
//	level 3  the search leg a tool runs (grep / bm25 / hybrid under `retrieve`)
//
// A leg that DELEGATES adds no level of its own: GrepSearch hands the pool build to
// BM25Search with the same context, so "[BM25 search]" prints beside the
// "[Grep search]" line that called it rather than under it. Without an outer loop —
// the direct-graph path — every level shifts up by one: the graph's own steps sit at
// 0, a tool call or a prefetch leg at 1, and the deepest line (a tool's search leg)
// at 2. The cap exists so the indent (and the stored text) cannot grow without bound
// if a future layer nests deeper or a path recurses.
const (
	StepIndentUnit = "&nbsp;&nbsp;&nbsp;&nbsp;"
	StepMaxDepth   = 5
)

// stepsDepthKey carries the current step nesting depth.
type stepsDepthKey struct{}

// Nested returns a ctx whose steps are reported one level deeper. A layer calls it
// around the work it launches after reporting its own step, so everything that
// layer produces reads as belonging to it.
func Nested(ctx context.Context) context.Context {
	return context.WithValue(ctx, stepsDepthKey{}, stepDepth(ctx)+1)
}

// stepDepth reports the nesting depth bound to ctx, 0 at the top level.
func stepDepth(ctx context.Context) int {
	d, _ := ctx.Value(stepsDepthKey{}).(int)
	return d
}

// indentStep prefixes line with depth indentation units, capped at StepMaxDepth.
// Only the TEXT projection is indented: the log stays flush (it is grepped, and
// its lines double as the Python-parity record) and the event Summary stays clean
// (a client renders structure, not entities).
func indentStep(line string, depth int) string {
	if depth <= 0 {
		return line
	}
	if depth > StepMaxDepth {
		depth = StepMaxDepth
	}
	return strings.Repeat(StepIndentUnit, depth) + line
}

// ThinkEvent is ONE reasoning step, structured. The definition lives in
// internal/service, which owns the wire shape it travels in
// (AsyncChatResult.think_event); this is an ALIAS of it.
//
// The definition cannot live here: service carries the step over SSE and cannot
// import this package, while this one already imports service for its
// exploration providers (tool_exploration.go). Two structs, one on each side,
// bridged field by field in cmd, compiled for as long as one stayed a subset —
// and silently dropped any field added on the other. An alias has no second copy
// to fall out of step.
type ThinkEvent = service.ThinkEvent

// ThinkEvent kinds.
const (
	// ThinkKindStage is one narrative pipeline stage.
	ThinkKindStage = "stage"
	// ThinkKindToolCall is a tool about to run, with its arguments.
	ThinkKindToolCall = "tool_call"
	// ThinkKindToolResult is a tool's outcome, with its result metadata.
	ThinkKindToolResult = "tool_result"
)

// ThinkMaxSources caps the evidence anchors a tool_result event carries: a step
// is a pointer into the evidence, not a dump of it.
const ThinkMaxSources = 12

// ThinkSink receives one ThinkEvent per reasoning step. The sink is per request,
// so concurrent requests stay isolated. Nil means no client asked for structured
// steps: emission is a no-op and only the human text is produced.
type ThinkSink func(ThinkEvent)

// StepReporter delivers one reasoning step to its two projections.
//
// The caller fills one in once per run (RAGTools.Steps); Rag binds it on ctx and
// every stage and tool reports through StepsFrom(ctx) — the same per-request
// shape Python's ContextVar had, which the async task tree inherited. A
// projection the caller left nil is skipped, so a run without a think block
// still logs, and a run without a logger still narrates.
type StepReporter struct {
	// Text is the human-sentence projection (the <think> block). Nil = no block.
	Text func(line string)
	// Events is the structured projection. Nil = no structured client.
	Events ThinkSink
	// depth is the nesting level this copy reports at. It is stamped by StepsFrom
	// from the caller's context (Nested) and never set by the caller, so the two
	// projections stay independent: a run that indents its text does not indent its
	// structured steps.
	depth int
}

// Enabled reports whether any projection is wired.
func (r StepReporter) Enabled() bool { return r.Text != nil || r.Events != nil }

// Stage reports one narrative stage: "[stage] msg" is written to the developer
// log, the same sentence goes to the think block, and a stage event goes to
// structured clients.
//
// The log write lives here (rather than at each call site) because a stage line
// has no other developer-facing home: it IS the record.
func (r StepReporter) Stage(log *log.Logger, stage, format string, args ...any) {
	r.StageLine(log, stage, fmt.Sprintf(format, args...))
}

// StageLine is Stage for an already-rendered sentence, so a caller that built
// its message elsewhere (a helper that formats the whole line) does not have to
// route it through a "%s" format verb.
func (r StepReporter) StageLine(log *log.Logger, stage, message string) {
	r.StageLineDetail(log, stage, message, message)
}

// StageLineDetail is StageLine for the steps whose two audiences need different
// words: `summary` is what the run REPORTS (the think block and the stage event)
// and `detail` is what the developer log RECORDS.
//
// It exists for identifiers that are precise for a developer and noise for a
// reader: a 32-hex document id, or the per-document chunk/char breakdown a search
// leg computes. The log keeps them — it is the greppable record, and Python's
// line-for-line parity lives there — while the think block names the same fact in
// words ("3 passages from 2 documents" instead of "d1:2chunk(14chars)").
//
// Both strings describe ONE call: the two halves are rendered at the same call
// site from the same chunks, so a reader and a developer can never be told
// different stories about what a step saw. A step whose two audiences want the
// same sentence uses StageLine, which is this function with one message.
func (r StepReporter) StageLineDetail(log *log.Logger, stage, summary, detail string) {
	if log != nil {
		log.Printf("[%s] %s", stage, detail)
	}
	r.Emit(ThinkEvent{Kind: ThinkKindStage, Stage: stage, Summary: "[" + stage + "] " + summary})
}

// Emit reports a fully-formed step. The developer log is deliberately NOT
// written here: the tool executor already logs its own two sentences (call and
// result, Python tool_decorator parity), and writing the summary again would
// duplicate every tool call in the log.
//
// The text projection is indented by this copy's depth (see StepIndentUnit); the
// event keeps the sentence unindented, because a structured client renders the
// nesting from its own step tree rather than from entities it would have to strip.
func (r StepReporter) Emit(ev ThinkEvent) {
	if ev.Summary != "" && r.Text != nil {
		text := r.Text
		line := indentStep(ev.Summary, r.depth)
		func() {
			// A broken sink must never break the run (Python wrapped its sink
			// call in a bare try/except for the same reason).
			defer func() { _ = recover() }()
			text(line + ThinkLineBreak)
		}()
	}
	EmitThink(r.Events, ev)
}

// EmitThink delivers one event to sink. A nil sink and a panicking sink are both
// tolerated: think narration must never break the request or the run.
func EmitThink(sink ThinkSink, event ThinkEvent) {
	if sink == nil {
		return
	}
	defer func() { _ = recover() }()
	sink(event)
}

// stepsCtxKey carries the per-request StepReporter.
type stepsCtxKey struct{}

// WithSteps binds the run's step reporter to ctx. Every stage and tool beneath
// reads it with StepsFrom, so a step is reported where it happens without
// threading a sink through every node signature — the same shape as Python's
// per-request ContextVar (rag/advanced_rag/think_log.py), which the async task
// tree inherited. A sink bound here is per request, so concurrent requests stay
// isolated.
func WithSteps(ctx context.Context, steps StepReporter) context.Context {
	if !steps.Enabled() {
		return ctx
	}
	return context.WithValue(ctx, stepsCtxKey{}, steps)
}

// StepsFrom returns the step reporter bound to ctx, or the zero StepReporter
// when none is bound — in which case every report is a no-op and the run simply
// has no user-visible trace (a caller that wired neither projection).
//
// The returned copy carries ctx's nesting depth (Nested), which is what indents the
// text projection of a step reported beneath another one. The bound reporter itself
// stays depth-free, so the same run can report at different depths from different
// contexts — which is exactly how a nested layer works.
func StepsFrom(ctx context.Context) StepReporter {
	depth := stepDepth(ctx)
	if r, ok := ctx.Value(stepsCtxKey{}).(StepReporter); ok {
		r.depth = depth
		return r
	}
	return StepReporter{depth: depth}
}
