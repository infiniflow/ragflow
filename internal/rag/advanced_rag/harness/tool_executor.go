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

// Package harness holds the low-level agentic-RAG capability library: the
// retrieval backend, the action-session runtime, the tool executor, and the
// compiled-structure / knowledge-graph navigation. These are the leaf building
// blocks the RAGTools methods in the parent `agent` package (Run, ComposeAnswer,
// ComposeNaiveAnswer, Formalize, ...) orchestrate. The split mirrors Python's
// layout, where agentic_rag.py's RAGTools imports the helpers under
// harness/ rather than inlining them.
//
// tool_executor.go is the Go-side tool-executor adapter: its
// searchExecutor type and the Execute dispatch correspond to the _exec_* tool
// methods in Python's harness/action_session.py (e.g. _exec_retrieve,
// _exec_navigate_tree, _exec_navigate_structure, _exec_list_chunks,
// _exec_calculate, _exec_web_search, _exec_wiki_query, _exec_graph_explore).
// In Python those live inline inside action_session.py; in Go they are pulled
// out into this file so the harness package need not import the advanced_rag package.
package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/service/nav"
)

// RunRequest is one harness run. It lives in the harness package (not the agent
// package) because searchExecutor needs it to carry the per-run query, and the
// advanced_rag package must not be imported from here (import-cycle rule). RAGTools.Run
// in the advanced_rag package uses it via the harness package.
type RunRequest struct {
	// Question is the user's question.
	Question string
	// ThinkingMode selects the ModeSpec ("low"/"medium"/"high"/"ultra").
	// An unrecognised value degrades to NAIVE, as in Python.
	ThinkingMode string
	// Keywords narrow retrieved chunks to the sentences mentioning them.
	Keywords string
	// DatasetIDs restricts retrieval. Empty falls back to the canvas context.
	DatasetIDs []string
	// TenantID scopes retrieval. Empty falls back to the canvas context.
	TenantID string
	// UseCompiled enables compiled-structure expansion during retrieval.
	UseCompiled bool
	// TopN is the per-search result count. <=0 selects the default.
	TopN int
	// DeadlineLeft is the wall-clock budget in seconds. <=0 selects the default.
	DeadlineLeft float64
	// MaxLength is the chat model's context window (Python
	// tools.chat_mdl.max_length). It bounds evidence and document-level reads.
	// <=0 selects the file-level defaults.
	MaxLength int
	// SessionID identifies the conversation this call belongs to. It is carried
	// for plumbing (e.g. future session-scoped state) but is not read by the
	// near-duplicate answer cache: that cache is per-request, mirroring Python's
	// per-turn RAGTools._rag_cache.
	SessionID string
	// Images are vision-gated base64 data URIs (Python image_attachments). The
	// outer react loop turns them into multimodal content blocks on the last
	// user message so a vision model sees them (advanced_rag.Rag assembles the
	// message from these).
	Images []string
	// TextAttachments is the joined text-file content (Python
	// text_attachments_content); appended to the question so the model reads
	// attached documents in the reasoning path.
	TextAttachments string
}

// searchExecutor adapts SearchDeps to ToolExecutor, so the retriever and
// compiled expander are what the session's retrieval tools call.
type searchExecutor struct {
	deps SearchDeps
	req  RunRequest
}

// NewSearchExecutor builds the retrieval/navigation ToolExecutor used by both
// the single-session path and the agentic loop's programmatic fan-out fetches.
//
// Exported so the advanced_rag package (which cannot be imported from here) can reuse
// the same evidence handling instead of duplicating it.
func NewSearchExecutor(deps SearchDeps, req RunRequest) ToolExecutor {
	return &searchExecutor{deps: deps, req: req}
}

// RunPattern executes ONE completeness pattern and returns the windows it matched, already
// narrowed (see ScanPatterns / RunCompletenessPass).
//
// It goes through the same grep path a retrieve call uses, so a pattern is treated as a
// pattern: its operands are recalled on their own (patternRecallTopN — a pattern's recall
// must not stop at a ranking's head, see that constant's measurement), the pattern decides
// which candidates are windows (matchGrepPattern), and the windows come back narrowed to
// the match (GrepOutCharsPerChunk / GrepOutTotalChars). The runtime runs this itself
// because the same queries handed to the model as a list went unrun (see
// RunCompletenessPass).
func (e *searchExecutor) RunPattern(ctx context.Context, pattern string) []map[string]any {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil
	}
	chunks, _ := GrepSearch(ctx, e.deps, SearchParams{
		Question: pattern,
		// A pattern is an expression for the MATCHER, not for the engine (see
		// GrepSearch): recall gets the operands, the pattern stays here and decides.
		Keywords: strings.Join(GrepPatternOperands(pattern), " "),
		TopN:     patternRecallTopN,
		KbIDs:    e.req.DatasetIDs,
		// The pass asks about the ACTOR and the ACT WORDS, never about candidate names
		// (see SearchParams.SkipReachLedger).
		SkipReachLedger: true,
	})
	return chunks
}

// Execute implements ToolExecutor for the wired tools. Tools whose port has not
// landed are classified, not errored: they report MISS (the tool is valid, this
// call reached nothing) so the model falls back to a different tool instead of
// stalling.
//
// One tool step is reported as its two halves: the invocation before dispatch
// and the outcome after it, each to the developer log AND as a step (think text +
// structured event). Python logs only the invocation (tool_decorator.py:311) and
// hands the raw payload to the model, which leaves the visible trace one-sided —
// a call that reached nothing reads exactly like one that found the answer, and
// the reader cannot tell which evidence the answer was built from.
func (e *searchExecutor) Execute(ctx context.Context, name string, args map[string]any) (ToolOutcome, error) {
	logger := e.deps.Logger
	renderedArgs := RenderToolArgs(args)
	// Mirror Python rag/llm/tool_decorator.py:tool "[Function tool] Running the
	// {name} tool with: {args}". The log line and the step below are the same
	// call site on purpose: a tool step is a step because this method says so,
	// not because a tagged line matched a pattern.
	//
	// The two projections then part: the log and the event's Args keep the argument
	// object Python logs and a developer greps, while the step reports the query
	// (ToolCallLine) — see there for why.
	if logger != nil && name != "" {
		logger.Printf("[Function tool] Running the %s tool with: %s", name, renderedArgs)
	}
	if name != "" {
		StepsFrom(ctx).Emit(ThinkEvent{
			Kind:    ThinkKindToolCall,
			Stage:   thinkToolStage,
			Tool:    name,
			Args:    renderedArgs,
			Summary: fmt.Sprintf("[%s] %s", thinkToolStage, ToolCallLine(name, args)),
		})
	}

	started := time.Now()
	// The tool's own work reports one level deeper: the search legs it runs (and
	// anything they run) belong to the call line above, not to the caller's level.
	// The result line below stays at the OUTER depth — it closes the call line, so
	// it is a sibling of it, not a child.
	outcome, err := e.dispatch(Nested(ctx), name, args)
	// One sentence template, two labels: the query names the call for both
	// audiences, and the fallback for a document-scoped call (no query argument)
	// exists only on the developer side — the think copy says nothing rather than
	// print an id a reader cannot use.
	label := ArgsLabel(args)
	human := renderToolOutcome(name, label, outcome, err)
	dev := human
	if label == "" {
		if devLabel := docIDLabel(args); devLabel != "" {
			dev = renderToolOutcome(name, devLabel, outcome, err)
		}
	}
	if logger != nil && name != "" {
		logger.Printf("[Function tool] %s", dev)
	}
	if name != "" {
		status := outcome.Status
		if err != nil && status == "" {
			// A transport failure is an error even when the executor returned a
			// zero ToolOutcome (its contract is to set one; this keeps a client
			// from seeing an unset status).
			status = StatusError
		}
		cause := outcome.Diagnostic
		if cause == "" && err != nil {
			cause = err.Error()
		}
		StepsFrom(ctx).Emit(ThinkEvent{
			Kind:       ThinkKindToolResult,
			Stage:      thinkToolStage,
			Tool:       name,
			Args:       renderedArgs,
			Status:     status,
			Reason:     outcome.Reason,
			Cause:      cause,
			Results:    len(outcome.Payload),
			Documents:  len(payloadDocIDs(outcome.Payload)),
			Sources:    eventSources(outcome, ThinkMaxSources),
			DurationMS: time.Since(started).Milliseconds(),
			Summary:    fmt.Sprintf("[%s] %s", thinkToolStage, human),
		})
	}
	return outcome, err
}

// thinkToolStage is the stage every tool step is reported under. The two
// sentences below are logged AND reported as steps here, so no other producer
// synthesizes a stage event for them.
const thinkToolStage = "Function tool"

// dispatch routes one tool call to its implementation.
func (e *searchExecutor) dispatch(ctx context.Context, name string, args map[string]any) (ToolOutcome, error) {
	switch name {
	case "retrieve", "search_chunks", "grep_search", "grep_chunks":
		return e.search(ctx, name, args)
	case "navigate_tree":
		return e.navigateTree(ctx, args)
	case "navigate_structure":
		return e.navigateStructure(ctx, args)
	case "list_chunks":
		return e.listChunks(ctx, args)
	case "fetch_full_document":
		return e.fetchFullDocument(ctx, args)
	case "summarize_document":
		return e.summarizeDocument(ctx, args)
	case "calculate":
		return e.calculate(ctx, args)
	case "graph_explore":
		return e.graphExplore(ctx, args)
	case "web_search":
		return WebSearchTool(ctx, e.deps, args)
	case "wiki_query":
		return e.wikiQuery(ctx, args)
	}
	return ToolOutcome{
		Payload: []any{map[string]any{
			"kind": name,
			"note": fmt.Sprintf("%s is not wired in this deployment yet. Use retrieve, search_chunks, navigate_tree or navigate_structure.", name),
		}},
		Status:  StatusMiss,
		Reason:  ReasonUnwired,
		Metrics: map[string]any{},
	}, nil
}

// RenderToolArgs renders a tool's arguments for the "[Function tool]" think-log
// line and the event's Args field. Python interpolated the raw args dict; Go
// marshals to JSON so nested values (scopes, tag maps) stay readable. Empty args
// render as "{}" to match Python's look rather than an empty string.
//
// String values are capped (capForThink): this field is rendered for display, and
// one uncapped slot-evidence query turns a single call into a paragraph repeated
// on every line of its round.
//
// Exported because the outer react loop (advanced_rag.outerReactSession) logs the
// same line for the tools it dispatches itself (rag / summarize_document).
func RenderToolArgs(args map[string]any) string {
	if len(args) == 0 {
		return "{}"
	}
	b, err := json.Marshal(capArgs(args))
	if err != nil {
		return "{}"
	}
	return string(b)
}

// QueryLabel renders a tool call's query argument(s) as bare quoted text
// ("\"曹操是谁\"" or "\"曹操的逝世日期\", \"曹操是谁\""), or "" when the call carries
// none. It is the ONE place the query-ish keys are listed, so a tool that renames
// its question field cannot make the think block and the result labels disagree
// about which calls have a query.
func QueryLabel(args map[string]any) string {
	for _, key := range []string{"query", "queries", "question"} {
		if label := quoteQueries(args[key]); label != "" {
			return label
		}
	}
	return ""
}

// ToolCallLine renders the think block's call step for a tool invocation:
// "Running the retrieve tool with \"曹操是谁\"." — a sentence, not a label.
//
// It carries the query and NOTHING else. The arguments the model emitted are a
// developer's record — document ids, nav hints, kind switches, scope lists — and
// printing them put raw JSON in front of a reader who only needs to know what was
// asked. A call with no query names only the tool ("Running the summarize_document
// tool.") rather than trailing an empty "with" or a document id, and the log keeps
// the full argument object for the developer who needs to correlate it.
//
// No colon after "with": the log line's colon (`with: {json}`, Python
// tool_decorator.py:311) introduces a dump, and this introduces a name — the
// sentence reads "running the tool with X". The log keeps its own punctuation.
//
// Exported because the outer react loop dispatches its own tools (rag /
// summarize_document) and reports the same line.
func ToolCallLine(name string, args map[string]any) string {
	if q := QueryLabel(args); q != "" {
		return fmt.Sprintf("Running the %s tool with %s.", name, q)
	}
	return fmt.Sprintf("Running the %s tool.", name)
}

// capArgs returns a copy of args with every string capped, walking the shapes a
// tool argument actually takes: a string, a list of them (a query list), or a
// nested object.
func capArgs(args map[string]any) map[string]any {
	out := make(map[string]any, len(args))
	for k, v := range args {
		out[k] = capArgValue(v)
	}
	return out
}

// capArgValue caps one argument value, recursing through the container shapes.
func capArgValue(v any) any {
	switch val := v.(type) {
	case string:
		return capForThink(val)
	case []string:
		out := make([]string, len(val))
		for i, s := range val {
			out[i] = capForThink(s)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = capArgValue(item)
		}
		return out
	case map[string]any:
		return capArgs(val)
	}
	return v
}

// renderToolOutcome renders one tool call's result as a sentence for the think
// block: the outcome status, how many results came back and — for passage-shaped
// results — how many documents they came from. It closes the "[Function tool]"
// narration opened by Execute, so one tool step reads as a whole.
//
// The sentence names the call's OWN arguments (the caller passes ArgsLabel, or
// docIDLabel for the developer copy) because a round runs several tool calls
// concurrently: with fan-out goals in flight, "Running navigate_tree with …" and
// "The navigate_tree tool returned …" lines interleave, and a result that does not
// say WHICH query it answers cannot be paired back to the call that produced it —
// two goals that both returned 1 result from 1 document read identically, which is
// exactly what a reader must be able to tell apart.
//
// The label is the ONLY thing that varies between the two audiences, so the
// wording above is written once: the same template renders the think sentence and
// the developer line, and they can never drift.
//
// The machine-readable reason is NOT spelled into the sentence. It stays in the
// tool_result event (Reason/Cause): "no_structure" is a routing token for the
// session's strike logic, while the sentence says what happened in words and, for
// failures, carries the producer's own diagnostic via causeSuffix.
func renderToolOutcome(name, label string, oc ToolOutcome, err error) string {
	if err != nil {
		return fmt.Sprintf("The %s tool failed%s: %v.", name, label, err)
	}
	results := CountOf(len(oc.Payload), "result")
	switch oc.Status {
	case StatusOK:
		return fmt.Sprintf("The %s tool returned %s%s%s.", name, results, documentSuffix(oc.Payload), label)
	case StatusRedundant:
		return fmt.Sprintf("The %s tool returned %s%s, already in the evidence pool.", name, results, label)
	case StatusMiss:
		if oc.Reason == ReasonUnwired {
			return fmt.Sprintf("The %s tool is not wired in this deployment, so nothing ran%s.", name, label)
		}
		return fmt.Sprintf("The %s tool matched nothing%s.", name, orForThisQuery(label))
	case StatusEmpty:
		if oc.Reason == ReasonNoStructure {
			return fmt.Sprintf("The %s tool has no compiled structure to read%s%s.", name, label, causeSuffix(oc.Diagnostic))
		}
		return fmt.Sprintf("The %s tool found nothing available%s%s.", name, label, causeSuffix(oc.Diagnostic))
	case StatusPoor:
		return fmt.Sprintf("The %s tool produced a result too weak to use%s%s.", name, label, causeSuffix(oc.Diagnostic))
	case StatusError:
		return fmt.Sprintf("The %s tool could not run%s%s.", name, label, causeSuffix(oc.Diagnostic))
	}
	return fmt.Sprintf("The %s tool returned %s%s.", name, results, label)
}

// ArgsLabel renders a tool call's identifying arguments as a trailing clause
// (" for \"曹操是谁\""), or "" when the call carries nothing that names it. It is
// what pairs a result line with its call line: the query (or question) is what
// distinguishes concurrent calls, and it is also the half a reader understands.
// Exact pairing stays possible in every case because the tool_result event
// carries the FULL arguments in Args.
//
// A document id is deliberately NOT a fallback here: it names nothing a reader
// knows. Where a document-scoped call has no query, the human sentence simply
// goes unlabelled and the developer copy uses docIDLabel.
func ArgsLabel(args map[string]any) string {
	if q := QueryLabel(args); q != "" {
		return " for " + q
	}
	return ""
}

// docIDLabel is the developer-log fallback ArgsLabel refuses: " for document
// 95a7aee3…" for a call that named a document and nothing else. It rides the log
// copy of a sentence only (never the think block), so a developer can pair a
// document-scoped call with the file it touched.
func docIDLabel(args map[string]any) string {
	if id, ok := args["doc_id"].(string); ok {
		if id = strings.TrimSpace(id); id != "" {
			return " for document " + id
		}
	}
	return ""
}

// quoteQueries renders a query argument as a quoted label: a single string, or a
// list of them rendered as a list. Each entry is capped (capForThink) because the
// slot-research driver searches a paragraph of evidence as its "query". It returns
// "" when the argument holds no text, so the caller falls back to the next
// identifying field.
func quoteQueries(raw any) string {
	var items []string
	switch v := raw.(type) {
	case string:
		items = []string{v}
	case []string:
		items = v
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				items = append(items, s)
			}
		}
	default:
		return ""
	}
	quoted := make([]string, 0, len(items))
	for _, s := range items {
		if strings.TrimSpace(s) != "" {
			quoted = append(quoted, fmt.Sprintf("%q", capForThink(s)))
		}
	}
	if len(quoted) == 0 {
		return ""
	}
	return strings.Join(quoted, ", ")
}

// orForThisQuery keeps a sentence from ending on a dangling preposition when the
// call named no query: "matched nothing for \"q\"" once labelled, "matched nothing
// for this query" otherwise.
func orForThisQuery(label string) string {
	if label == "" {
		return " for this query"
	}
	return label
}

// causeSuffix renders a tool's own failure explanation as a trailing clause.
// "reason: infra" answers WHICH bucket the failure fell in; this answers what
// actually happened, which is the part a reader can act on.
func causeSuffix(diagnostic string) string {
	if strings.TrimSpace(diagnostic) == "" {
		return ""
	}
	return ": " + diagnostic
}

// CountOf renders a count with a singular/plural noun ("1 result", "3 results").
// Shared with the outer react loop's narration (advanced_rag.outerReactSession)
// so both layers word their "[Function tool]" result lines identically.
func CountOf(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %s", n, pluralOf(noun))
}

// pluralOf pluralizes the LAST word of noun ("passage" → "passages"), including
// the consonant+y rule: appending a bare "s" is what printed "3 first-hop querys"
// and "3 targeted querys" in the trace.
func pluralOf(noun string) string {
	if len(noun) >= 2 && noun[len(noun)-1] == 'y' {
		switch noun[len(noun)-2] {
		case 'a', 'e', 'i', 'o', 'u':
		default:
			return noun[:len(noun)-1] + "ies"
		}
	}
	return noun + "s"
}

// ThinkLabelMaxRunes caps the piece of ONE argument a step's sentence and a tool
// call's rendering may carry.
//
// A tool's "query" is not always a question: the slot-research driver searches a
// paragraph of slot evidence, and an uncapped label put the same 200 characters
// on six lines of a single round (running / matched nothing / locate / searching
// / returned …) until the round was unreadable. The cap is per VALUE, so the
// sentence stays a sentence.
const ThinkLabelMaxRunes = 60

// capForThink caps one string for the think block: rune-safe (it never cuts a
// character in half) and marked with an ellipsis when it was cut.
func capForThink(s string) string {
	if utf8.RuneCountInString(s) <= ThinkLabelMaxRunes {
		return s
	}
	return truncateRunes(s, ThinkLabelMaxRunes) + "…"
}

// documentSuffix renders " from N document(s)" for the distinct doc ids a
// payload carries, or "" when it names none (a computed number or a note has no
// documents to name).
func documentSuffix(payload []any) string {
	n := len(payloadDocIDs(payload))
	if n == 0 {
		return ""
	}
	return " from " + CountOf(n, "document")
}

// payloadDocIDs lists the distinct documents a payload names, in first-seen
// order. Retrieval payloads carry one doc id per passage ("doc_id");
// navigate_tree's routed payload names the documents it routed to as a list
// ("doc_ids"), which is the only thing that outcome reports about the corpus.
// The structured event and the human sentence are both built from this, so the
// count a client shows always matches the count in the sentence.
func payloadDocIDs(payload []any) []string {
	seen := make(map[string]struct{}, len(payload))
	out := make([]string, 0, len(payload))
	add := func(id string) {
		if id == "" {
			return
		}
		if _, dup := seen[id]; dup {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, item := range payload {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if id, _ := m["doc_id"].(string); id != "" {
			add(id)
		}
		for _, id := range payloadStringList(m["doc_ids"]) {
			add(id)
		}
	}
	return out
}

// eventSources lists the evidence anchors a tool outcome produced: the chunk ids
// it admitted, or — for a tool that returns no passages (navigate_tree routes
// documents, it does not quote them) — the documents it reached. Capped at
// limit.
func eventSources(oc ToolOutcome, limit int) []string {
	seen := make(map[string]struct{}, len(oc.EvidenceIDs))
	sources := make([]string, 0, len(oc.EvidenceIDs))
	for _, s := range oc.EvidenceIDs {
		if s == "" {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		sources = append(sources, s)
	}
	if len(sources) == 0 {
		sources = payloadDocIDs(oc.Payload)
	}
	if limit > 0 && len(sources) > limit {
		sources = sources[:limit]
	}
	return sources
}

// payloadStringList reads a string-list FIELD of a tool payload: the executor
// builds it as []string, but a payload that made a round trip through a decoded
// message carries []any. Unlike toolStringList (an ARG reader that also accepts a
// lone string and coerces scalars), this is a payload read: only real strings
// count, so a malformed entry is skipped instead of being stringified.
func payloadStringList(raw any) []string {
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func (e *searchExecutor) fetchFullDocument(ctx context.Context, args map[string]any) (ToolOutcome, error) {
	docID := argString(args, "doc_id")
	if docID == "" {
		return ToolOutcome{Payload: []any{}, Status: StatusError, Reason: ReasonBadArgs}, nil
	}
	if e.deps.DocChunks == nil {
		return ToolOutcome{
			Payload: []any{map[string]any{
				"kind":   "fetch_full_document",
				"doc_id": docID,
				"note":   "Whole-document reading is not available for this session.",
			}},
			Status: StatusEmpty,
			Reason: ReasonNoDoc,
		}, nil
	}
	chunks, aggs := fetchFullDocument(ctx, e.deps, docID, e.req.MaxLength)
	if len(chunks) == 0 {
		return ToolOutcome{Payload: []any{}, Status: StatusEmpty, Reason: ReasonNoDoc}, nil
	}
	return ToolOutcome{
		Payload: []any{map[string]any{
			"kind":      "fetch_full_document",
			"doc_id":    docID,
			"count":     len(chunks),
			"chunks":    chunks,
			"doc_aggs":  aggs,
			"doc_names": []string{docID},
		}},
		Status:  StatusOK,
		Metrics: map[string]any{"chunks": len(chunks)},
	}, nil
}

// summarizeDocument mirrors Python's summarize_document tool: load the document
// and hand the model the freshly rendered evidence blocks.
func (e *searchExecutor) summarizeDocument(ctx context.Context, args map[string]any) (ToolOutcome, error) {
	docID := argString(args, "doc_id")
	if docID == "" {
		return ToolOutcome{Payload: []any{}, Status: StatusError, Reason: ReasonBadArgs}, nil
	}
	if e.deps.DocChunks == nil {
		return ToolOutcome{
			Payload: []any{map[string]any{
				"kind":   "summarize_document",
				"doc_id": docID,
				"note":   "Whole-document reading is not available for this session.",
			}},
			Status: StatusEmpty,
			Reason: ReasonNoDoc,
		}, nil
	}
	blocks := summarizeDocument(ctx, e.deps, docID, e.req.MaxLength)
	if len(blocks) == 0 {
		return ToolOutcome{Payload: []any{}, Status: StatusEmpty, Reason: ReasonNoDoc}, nil
	}
	content := ""
	for _, b := range blocks {
		content += b + "\n\n"
	}
	return ToolOutcome{
		Payload: []any{map[string]any{
			"kind":    "summarize_document",
			"doc_id":  docID,
			"content": content,
		}},
		Status:  StatusOK,
		Metrics: map[string]any{"blocks": len(blocks)},
	}, nil
}

// navigateTree routes to the top-n documents by descending the dataset's compiled
// navigation tree, reporting three outcomes for the ladder to act on:
//   - no router configured → EMPTY/no_structure (disable the tool, dataset-level);
//   - tree exists but this query routed to nothing → MISS (fall through to
//     `global`, do NOT disable);
//   - routed → OK with the doc ids and their summaries.
func (e *searchExecutor) navigateTree(ctx context.Context, args map[string]any) (ToolOutcome, error) {
	query := argString(args, "query")
	if query == "" {
		return ToolOutcome{Payload: []any{}, Status: StatusError, Reason: ReasonBadArgs}, nil
	}
	router := e.navRouter()
	// The tool argument is NOT threaded (Python _exec_navigate_tree,
	// action_session.py:_exec_navigate_tree, calls _navigate_tree_impl(query, keywords=...) with
	// no doc_scope), but the SESSION scope still applies: the impl routes through
	// _nav_search_titled, which ceilings its scope with tools.scoped_doc_ids(None)
	// — the session doc_scope (navigation.py:_nav_search_titled). The router must therefore
	// receive the session ceiling, never the tool argument.
	res := NavigateTree(ctx, router, NavTreeInput{
		Query: query,
		// Python :985 threads ONLY the tool argument's keywords (usually absent,
		// so "") — the run-level request keywords are a Go-only invention that
		// re-biased every tree routing toward the original question keywords.
		Keywords: argString(args, "keywords"),
		DocScope: e.deps.DocScope,
		TenantID: e.deps.TenantID,
		KbIDs:    e.deps.KbIDs,
	})

	switch res.EmptyReason {
	case ReasonNoStructure:
		return ToolOutcome{
			Payload: []any{map[string]any{
				// Python action_session.py:_exec_navigate_tree (dataset_has_compilation
				// gate) — note text verbatim; the payload carries kind+note only.
				"kind": "navigate_tree",
				"note": "This dataset has no compiled document-navigation structure; use search_chunks / retrieve instead.",
			}},
			Status: StatusEmpty,
			Reason: ReasonNoStructure,
		}, nil
	case ReasonNoDoc:
		// Structure exists, this query reached nothing — a MISS, not an EMPTY.
		// Python :990 note verbatim; the payload carries kind+note only.
		return ToolOutcome{
			Payload: []any{map[string]any{
				"kind": "navigate_tree",
				"note": "navigate_tree routed to no document for this query. Rephrase the query, or use search_chunks / retrieve instead.",
			}},
			Status: StatusMiss,
			Reason: ReasonNoDoc,
		}, nil
	case ReasonInfra, ReasonBadArgs:
		return ToolOutcome{
			Payload: []any{},
			Status:  ReasonStatus(res.EmptyReason),
			Reason:  res.EmptyReason,
			// Carry WHY the descent failed: the reason bucket alone ("infra")
			// leaves a reader with nothing to act on.
			Diagnostic: res.Diagnostic,
		}, nil
	}

	// Routed: expose the summary-bearing payload the ladder consumes. Python
	// :995 caps the content at 8000 chars.
	content := res.Text
	if len(content) > 8000 {
		content = content[:8000]
	}
	payload := []any{map[string]any{
		"kind":    "navigate_tree",
		"content": content,
		"doc_ids": res.DocIDs,
	}}
	return ToolOutcome{
		Payload:     payload,
		EvidenceIDs: nil, // routing only — no passages retrieved
		Status:      StatusOK,
		Reason:      ReasonNone,
		Metrics: map[string]any{
			"docs":        len(res.DocIDs),
			"routed_docs": res.RoutedDocs,
		},
	}, nil
}

// chunkAggRetrieveFrom adapts a harness Retriever into the chunk_agg retrieve
// leg. It scopes the search to the dataset (kbID) and the caller's document
// scope and pulls the wide pool chunk_agg needs; the backend is expected to
// exclude compiled rows (the production retrieval filters available_int=1),
// mirroring Python settings.retriever.retrieval under _search_layers_nav_chunk_agg.
//
// It stays in the harness package because it depends on the agentic Retriever /
// RetrieveRequest; the routing algorithm itself lives in internal/service/nav.
func chunkAggRetrieveFrom(r Retriever) nav.ChunkRetriever {
	return func(ctx context.Context, tenantID, kbID, query string, docScope []string, topN int, _ float64) ([]map[string]any, error) {
		chunks, err := r.Retrieve(ctx, RetrieveRequest{
			Query:      query,
			DatasetIDs: []string{kbID},
			TenantID:   tenantID,
			DocScope:   docScope,
			TopN:       topN,
			// page_size here IS the pool (Python passes `pool` positionally as
			// page_size, dataset_api_service.py:4106-4129), and the retrieval
			// service refuses `page * page_size > rerank_candidates_count`
			// (nlp/retrieval.go:126, mirroring rag/nlp/search.py:745). Python
			// therefore raises the candidate count to the same pool for this
			// leg (`rerank_candidates_count=pool`); omitting it left the default
			// 64 against a 256-wide pool, so EVERY nav-tree descent failed with
			// "rerank_candidates_count(64) must be greater than or equal to
			// page(1) * page_size(256)" and reported infra.
			RerankCandidatesCount: topN,
			// `must_not={"exists": "compile_kwd"}` (_NAV_CHUNK_AGG_EXCLUDE_COMPILED
			// = True): this leg aggregates ORIGINAL chunks per document, and a
			// compiled product row would otherwise be attributed to a document
			// as if it were a passage.
			ExcludeCompiled: true,
		})
		if err != nil {
			// Surface the failure instead of folding it into an empty result:
			// the router reports it as an error so the orchestrator takes its
			// fallback rather than telling the model the tree routed nothing.
			return nil, err
		}
		return chunks, nil
	}
}

// navigateStructure pinpoints passages inside ONE document using its compiled
// structure (heading tree / concept mindmap).
//
// It delegates to the navigation package's NavigateStructure, which loads the
// document's compiled rows, asks the model which entities answer the query, and
// returns the underlying source chunks.
//
// NOTE on shapes: the Go reader (navigation.loadStructureEntities) currently
// reads only the compact graph-blob rows, whereas Python merges BOTH row shapes
// (graph blob AND per-entity/relation rows). ParseCompiledStructure in
// navtools.go already implements the merged parsing; wiring it into the reader
// is the remaining step for full parity.
func (e *searchExecutor) navigateStructure(ctx context.Context, args map[string]any) (ToolOutcome, error) {
	query := argString(args, "query")
	docID := argString(args, "doc_id")
	if query == "" {
		query = e.req.Question
	}
	if query == "" {
		return ToolOutcome{Payload: []any{}, Status: StatusError, Reason: ReasonBadArgs}, nil
	}
	// No scope-as-doc-pin: Python _exec_navigate_structure
	// reads only args["doc_id"] — args["doc_scope"] is never consulted.
	kind := argString(args, "kind")
	if kind == "" {
		kind = "catalog"
	}

	// Resolve the document set. When the caller omits doc_id, mirror Python
	// _navigate_structure_impl: route to the documents by descending the compiled
	// navigation tree (the exact seam navigate_tree uses), not by refusing the
	// call. Python's _nav_search_titled caps the routed set at _NAV_TREE_MAX_DOCS.
	var docIDs []string
	if docID != "" {
		docIDs = []string{docID}
	} else {
		// DocScope is the session ceiling, not the tool argument (see the note in
		// navigateTree): Python's _exec_navigate_structure never threads
		// args["doc_scope"], but the routing it does when doc_id is absent goes
		// through _nav_search_titled, which applies tools.scoped_doc_ids(None)
		// (navigation.py:_nav_search_titled / :1012).
		res := NavigateTree(ctx, e.navRouter(), NavTreeInput{
			Query:    query,
			Keywords: e.req.Keywords,
			DocScope: e.deps.DocScope,
			TenantID: e.deps.TenantID,
			KbIDs:    e.deps.KbIDs,
		})
		if res.EmptyReason == ReasonInfra {
			return ToolOutcome{Payload: []any{}, Status: ReasonStatus(res.EmptyReason), Reason: res.EmptyReason, Diagnostic: res.Diagnostic}, nil
		}
		docIDs = res.DocIDs
		if len(docIDs) > navTreeMaxDocs {
			docIDs = docIDs[:navTreeMaxDocs]
		}
		if len(docIDs) == 0 {
			// Routing reached no document (Python: empty_reason="no_doc"). Python
			// :1037 emits the SAME note for every empty_reason — text verbatim.
			return ToolOutcome{
				Payload: []any{map[string]any{
					"kind":   "navigate_structure",
					"doc_id": docID,
					"note":   fmt.Sprintf("No compiled structure of kind='%s' reachable for this document. Try another doc_id or kind, or use search_chunks / retrieve / list_chunks.", kind),
				}},
				Status: StatusMiss,
				Reason: ReasonNoDoc,
			}, nil
		}
	}

	// Zero-LLM vector-beam drill-down (mirrors Python _read_structures +
	// _navigate_structure_impl): read each document's compiled structure of the
	// requested kind, drill toward the query, and hand the model the merged
	// <structure_navigation> outline with chunk-pointer anchors.
	res, drill := navigateStructures(ctx, e.deps.TenantID, query, docIDs, kind, e.navRouter(), e.deps)
	metrics := map[string]any{
		"entities":    drill.nodes,
		"chunk_ptrs":  drill.chunkPtrs,
		"top_score":   drill.topScore,
		"chunk_paths": drill.chunkPaths,
		"claim_hits":  drill.claimHits,
	}
	if res.EmptyReason != "" {
		// Python :1037 — one note for every empty_reason; kind renders with
		// Python repr() quoting (kind!r).
		return ToolOutcome{
			Payload: []any{map[string]any{
				"kind":   "navigate_structure",
				"doc_id": docID,
				"note":   fmt.Sprintf("No compiled structure of kind='%s' reachable for this document. Try another doc_id or kind, or use search_chunks / retrieve / list_chunks.", kind),
			}},
			Status:  ReasonStatus(res.EmptyReason),
			Reason:  res.EmptyReason,
			Metrics: metrics,
		}, nil
	}
	// Reached structures but drilled to nothing usable: not an error, just weak —
	// the orchestrator falls back to retrieval on status == poor. Claim-first
	// hits are the exception: the claims carry the answer material and load no
	// chunk snippets, so a zero chunk_ptr count with matched claims is a success.
	status := StatusOK
	if drill.chunkPtrs == 0 && drill.claimHits == 0 {
		status = StatusPoor
	}
	content := res.Text
	if len(content) > 8000 {
		content = content[:8000]
	}
	evidenceIDs := res.DocIDs
	if len(evidenceIDs) == 0 && len(docIDs) > 0 {
		evidenceIDs = docIDs
	}
	return ToolOutcome{
		Payload:     []any{map[string]any{"kind": "navigate_structure", "doc_id": firstOr(docIDs, ""), "content": content}},
		EvidenceIDs: evidenceIDs,
		Status:      status,
		Reason:      ReasonNone,
		Metrics:     metrics,
	}, nil
}

// navRouter returns the navigation-tree router navigateTree and navigateStructure
// share. It mirrors Python's agentic router choice: the navigation-tree route
// asks for router="claim_agg" (dataset_api_service.search_dataset_layers) — the
// claim leg runs first and decides the ranking when it hits, and raw-chunk
// aggregation (chunk_agg) is the fallback. A nil retrieval backend degrades to
// the nav-row router, which needs no backend.
func (e *searchExecutor) navRouter() NavTreeRouter {
	if e.deps.NavRouter != nil {
		return e.deps.NavRouter
	}
	return defaultNavRouter(e.deps)
}

// defaultNavRouter builds the claim_agg router over the chunk_agg fallback.
// See ClaimAggRouter and nav.NewChunkAggRouter.
func defaultNavRouter(deps SearchDeps) NavTreeRouter {
	if deps.Backend == nil {
		return nav.NewNavServiceRouter()
	}
	return &ClaimAggRouter{
		Deps:      deps,
		Fallback:  nav.NewChunkAggRouter(chunkAggRetrieveFrom(deps.Backend), nav.ChunkAggSummarize()),
		Summarize: nav.ChunkAggSummarize(),
	}
}

func firstOr(xs []string, def string) string {
	if len(xs) > 0 {
		return xs[0]
	}
	return def
}

// argString reads a string arg, treating an absent key or an explicit JSON null
// as empty. fmt.Sprint(nil) yields "<nil>", so the presence check must come
// first — otherwise a missing arg silently becomes the literal "<nil>".
func argString(args map[string]any, key string) string {
	raw, ok := args[key]
	if !ok || raw == nil {
		return ""
	}
	s := strings.TrimSpace(fmt.Sprint(raw))
	if s == "<nil>" {
		return ""
	}
	return s
}

// evidencePoolCap is the hard cap on the shared evidence pool
// (kbinfos["chunks"]). Mirrors Python _EVIDENCE_POOL_CAP (=120,
// action_session.py:62): deliberately LARGER than _SCA_VIEW_CAP (=60) so
// storage and review stay DECOUPLED — the pool accumulates while the SCA reads
// a ranked top-60 view. Coupling them at 60 starved the raw-evidence channel in
// 42% of rounds (every admit rejected -> status REDUNDANT -> the model
// re-searched for nothing).
//
// Claim pseudo-chunks BYPASS this cap: Python's _claim_prefetch appends them
// directly to kbinfos["chunks"] (:755), and the cap check (:657) only guards
// _admit_evidence's regular chunks.
//
// 200, not Python's 120. The 120 was sized for a consumer that no longer exists —
// the round-level sweep that rendered the WHOLE pool into one prompt, where the cap
// and that prompt's budget were the same number. Nothing renders the pool whole any
// more (the SCA reads a ranked 60-chunk view, the session seed injects a bounded
// digest, the draft is bounded), so the cap is a storage-discipline number again —
// and on an enumeration it is the NEXT ceiling rather than a prompt limit. Measured
// (2026-09-14, fixrecall): a round of batch name probing ended at 117 chunks, three
// below the cap, with the question's members still arriving; at 200 the same shape
// reached seventeen. Measured here (2026-09-16, 三国/关羽): the run's own line
// `pool at the cap 120 but the batch asked about "管亥"; admitting the passage that
// reaches it (pool 121, novelty slack 1/40)` — the tail members' passages are what
// the cap refuses.
const evidencePoolCap = 200

// The cap check and its "pool FULL" line now live on PoolAdmitter.Full, where
// the pool lock is held (see kbinfos.go): the check must not read len(Chunks)
// while another session appends.

// probeTerms returns the terms of a PROBE query — the caller's own alternation,
// "荀正|管亥|车胄" — and nil for every other query shape.
//
// An alternation is the one place the model states explicitly WHICH individuals
// it is asking about, which makes its per-term result the batch's answer and not
// just another search: a name that comes back empty is a name this corpus does
// not carry, and a name that comes back with a window is a member. Both facts are
// destroyed by a cap that drops the window (see PoolAdmitter.Novelty), so a probe
// is exempt while a topic query is not — a topic query names no individuals, so
// its hits compete for the cap like everything else.
func probeTerms(q string) []string {
	if !strings.Contains(q, "|") {
		return nil
	}
	return GrepTermsFromQuery(q)
}

// namedTermsOf is the call's own statement of WHICH individuals it asked about:
// the terms of the queries it carried, deduped and capped.
//
// Every shape the model uses lands here — an alternation ("A|B|C"), a
// space-separated list inside one string, or a list of query strings — because
// GrepTermsFromQuery already reads all three. That is the point: the seat
// mechanism is attached to the FACT that the call named terms, not to a syntax
// the model may never write. Measured (2026-09-15): fifteen calls, every one of
// them naming people, zero of them using `|` — a `|`-triggered mechanism fires
// never.
//
// The cap (GrepTermsMax) bounds the seat pass, which runs one cheap keyword
// search per unreached term; the caller logs how many named terms were dropped
// so the ceiling is visible in the run.
//
// The terms are the caller's OWN WORDS (GrepWordsFromQuery), not the CJK windows
// the locate step derives from them: a window is our guess at where a name can be
// found, and probing one spends a retrieval on a fragment nobody asked about
// (measured 2026-09-15: 羽斩 / 杀的 / 的有 / 领名, fourteen of twenty probes
// "absent").
func namedTermsOf(queries []string) []string {
	var out []string
	seen := make(map[string]bool, len(queries)*2)
	for _, q := range queries {
		for _, t := range GrepWordsFromQuery(q) {
			// A PIECE OF A PATTERN is a phrase, not a name: "关公.*斩" asks how a
			// deed is written, and probing 关公 or 斩 as a name spends a retrieval
			// on a word nobody proposed as a member (measured: a pass built from
			// such fragments probed 羽斩 / 杀的 / 的有 and reported fourteen
			// "absent"). An alternation of PLAIN words ("华雄|颜良|蔡阳") is exactly
			// what the seat exists for, so the test is pattern OPERATORS, not "|".
			if strings.ContainsAny(t, ".*+?()[]{}^$\\") {
				continue
			}
			low := strings.ToLower(strings.TrimSpace(t))
			if low == "" || seen[low] {
				continue
			}
			seen[low] = true
			out = append(out, t)
			if len(out) >= GrepTermsMax {
				return out
			}
		}
	}
	return out
}

// search runs one retrieval call for a tool invocation.
//
// Python's retrieve/search_chunks both funnel into tools/search.py, differing
// only in whether compiled expansion runs (search_chunks expands, retrieve does
// not) and in the accepted query count.
func (e *searchExecutor) search(ctx context.Context, name string, args map[string]any) (ToolOutcome, error) {
	queries := toolQueries(args)
	if len(queries) == 0 {
		// Python _arg_query_list yields [] for a missing/blank query and
		// _run_search simply admits nothing: _search_outcome([], ...) is
		// MISS/no_doc (action_session.py:_search_outcome), NOT a bad-args error.
		return ToolOutcome{
			Payload:     []any{},
			EvidenceIDs: nil,
			Status:      StatusMiss,
			Reason:      ReasonNoDoc,
			Metrics:     map[string]any{"hits": 0, "new_evidence": 0},
		}, nil
	}
	// Python's retrieval tools run against tools.kbinfos, and _seed_evidence
	// (action_session.py:_admit_evidence) CREATES it when absent — so every search has a
	// (possibly empty) pool to admit into. Mirror that instead of bailing out:
	// the search runs and its outcome is decided by what it actually admitted.
	if e.deps.KB == nil {
		e.deps.KB = &Kbinfos{}
	}
	logger := searchLogger(e.deps)

	// Max queries per tool call mirrors Python action_session.execute_tool
	// (_arg_query_list): retrieve=3, search_chunks=2. grep_search/grep_chunks
	// are Go-internal tools (Python exposes no such session tool), so they are
	// left uncapped.
	//
	// A direction assembling a SET/COUNT raises the ceiling. There the call's
	// queries are facets of one list rather than rephrasings of one question, so
	// a dropped query is not a spared repeat — it is a member nobody searched.
	// The flag is set by the same gate that hands the model the set method (see
	// Kbinfos.MarkSetDirection), so a VALUE direction keeps the small cap it had.
	// Measured (2026-09-16, medium mode): every call asked 3-5 queries against
	// these caps, nine calls were cut, and the names the run then failed to
	// record had been named only in the dropped ones.
	maxQ := len(queries)
	switch name {
	case "retrieve":
		maxQ = 3
	case "search_chunks":
		maxQ = 2
	}
	if e.deps.KB.IsSetDirection() {
		switch name {
		case "retrieve":
			maxQ = 6
		case "search_chunks":
			maxQ = 4
		}
	}
	// The full list is kept: maxQ bounds how many EXPENSIVE queries run, but the
	// terms of the dropped ones still get their own cheap seat below. Dropping a
	// list item silently (as this cut used to) drops the individuals it named
	// before any retrieval happens — measured (2026-09-15): a run named 29
	// queries, 8 never executed, and those 8 included the only mentions of the
	// names it was missing.
	allQueries := queries
	dropped := 0
	if len(queries) > maxQ {
		queries = queries[:maxQ]
		dropped = len(allQueries) - maxQ
		logger.Printf("[Action Session] %s asked %d quer(ies); running %d and taking named-term seats for the rest.", name, len(allQueries), maxQ)
	}

	var payload []any
	var evidenceIDs []string
	newChunks := 0
	// Per-call admittance state, mirroring Python _run_search/_admit_evidence
	// (action_session.py:_run_search): `seen` dedups chunks ACROSS the queries of
	// this one call. The pool-side dedup is Kbinfos.Admit's job, against the LIVE
	// pool — a per-call snapshot of it (Python's `kb_seen`) is exact only while
	// nothing can interleave, and another session appending makes it stale.
	seen := map[string]bool{}
	// reached accumulates every candidate the call's own searches returned. It is
	// what the seat pass below asks "which of the named terms did this retrieval
	// NOT reach?" — the question whose answer is a search of its own.
	var reached []map[string]any
	// Per-call reach notes: what each query reached term by term, and which of its
	// terms nothing reached (see ToolOutcome.Note / GrepReachLine). A batch of
	// names is exactly where this matters — the passages alone cannot say whether
	// a member was missing or simply never asked about.
	var reachNotes []string
	// Claim-first, MUTUALLY EXCLUSIVE (Python _run_search: _claim_prefetch +
	// _CLAIM_PREFETCH_EXCLUSIVE): when claim rows hit, their verbatim evidence
	// IS the answer material — chunk snippets on top would echo the same
	// passages and burn tokens. Claims carry chunk pointers, so deep-reading
	// stays one list_chunks away. No hits → the chunk search runs exactly as
	// before. Applies to the whole retrieve family + search_chunks, matching
	// Python's _exec_retrieve/_exec_search_chunks (both route through
	// _run_search). Best effort: any failure falls through without claims.
	if name == "retrieve" || name == "search_chunks" || strings.HasPrefix(name, "grep") {
		if len(queries) > 0 {
			if claimPayload, _, claimPseudo, ok := ClaimPrefetch(ctx, e.deps, queries[0], seen); ok {
				newEvidence := 0
				e.deps.KB.Admit(func(p *PoolAdmitter) {
					for _, pc := range claimPseudo {
						// Claim pseudo-chunks BYPASS the pool cap: Python
						// _claim_prefetch appends them directly to
						// kbinfos["chunks"] (:755) and the cap check (:657)
						// only guards regular chunk admits — the verbatim
						// evidence must land in the pool even when it is FULL.
						cid := ChunkIDOf(pc)
						if seen[cid] {
							continue
						}
						seen[cid] = true
						evidenceIDs = append(evidenceIDs, cid)
						payload = append(payload, passageFromChunk(pc))
						if p.Add(pc) {
							newEvidence++
						}
					}
				})
				status := StatusOK
				if newEvidence == 0 {
					status = StatusRedundant
				}
				return ToolOutcome{
					Payload:     payload,
					EvidenceIDs: evidenceIDs,
					Status:      status,
					Reason:      ReasonNone,
					Metrics:     map[string]any{"hits": len(claimPayload), "new_evidence": newEvidence, "claims": len(claimPayload)},
				}, nil
			}
		}
	}
	for _, q := range queries {
		// Per-tool top_n (mirrors Python action_session: retrieve=10,
		// search_chunks=20). They were previously collapsed onto e.req.TopN,
		// so search_chunks returned far fewer candidates than Python.
		topN := 10
		if name == "search_chunks" {
			topN = 20
		}
		// Python dispatches these tools to genuinely different search functions
		// (action_session.py:_exec_retrieve :751): retrieve/grep_* → grep_search
		// (keyword-only), search_chunks → hybrid_search (vector leg when an
		// embedder is configured). Each is now its own Go function so a single
		// global switch can no longer disable the semantic leg.
		//
		// Python's search_chunks tool ALWAYS enables compiled-structure expansion
		// in ALL modes, not just high (action_session.py:execute_tool passes
		// use_compiled=True); retrieve/grep_search never do. So Go mirrors that:
		// compiled is on for search_chunks and off for every other retrieve-family
		// tool, independent of e.req.UseCompiled (which gates the L1 direct
		// retrieve, not the action_session tool loop).
		var searchFn func(context.Context, SearchDeps, SearchParams) ([]map[string]any, []map[string]any)
		useCompiled := name == "search_chunks"
		if useCompiled {
			searchFn = HybridSearch
		} else {
			searchFn = GrepSearch
		}
		// Keywords mirror the two Python executors exactly:
		//   - retrieve → grep_search(keywords=nav_hint or None). In Python the
		//     nav hint is an explicit PARAMETER of _exec_retrieve and is passed
		//   ONLY by the navigation ladder (action_session.py:_run_drill_merge); the model's
		//     own retrieve dispatch (:1022) passes none, in which case grep_search
		//   falls back to the query's own extracted terms (search.py:grep_search) —
		//     that fallback lives in GrepSearch, which turns them into the BM25
		//     hint. The session run keywords (req.Keywords) are never forwarded.
		//   - search_chunks → _exec_search_chunks (:751) takes no keywords at all,
		//     so hybrid_search's _narrow_or_keep is a no-op.
		var kws string
		if !useCompiled {
			kws = argString(args, "nav_hint")
		}
		// doc_scope is honoured by the retrieve family only: Python's
		// _exec_retrieve reads args["doc_scope"], while
		// _exec_search_chunks (:751) takes no doc_scope at all and its
		// hybrid_search call is unscoped. Gating on useCompiled keeps that split.
		var docScope []string
		if !useCompiled {
			docScope = toolDocScope(args)
		}
		// Python _run_search reads only res["chunks"], but the aggregations it
		// drops are what the answer's document/reference list is built from —
		// this tool is their only writer.
		chunks, aggs := searchFn(ctx, e.deps, SearchParams{
			Question:    q,
			Keywords:    kws,
			UseCompiled: useCompiled,
			TopN:        topN,
			KbIDs:       e.req.DatasetIDs,
			DocScope:    docScope,
		})
		if len(chunks) == 0 {
			continue
		}
		// Computed on the LEG's candidates, not on the payload below: the payload is
		// truncated to the per-query snippet cap, and a term whose window was cut is
		// still a term this query reached.
		if line := GrepReachLine(q, chunks, ReachTermsOf(q)); line != "" {
			reachNotes = append(reachNotes, line)
		}
		// Only the first snippetsPerQueryFor(mode) hits of each query are considered
		// (Python cands[:_SNIPPETS_PER_QUERY]) — EXCEPT for a query that NAMES
		// terms, which keeps one candidate per named term first. The locate step
		// hands back one window per term, and a flat cut is what turns a six-name
		// call into "the names that matched most": the rarest lose their seat to the
		// ones the ranking already preferred.
		limit := snippetsPerQueryFor(e.req.ThinkingMode)
		if n := len(namedTermsOf([]string{q})); n > limit {
			limit = n
		}
		if len(chunks) > limit {
			chunks = chunks[:limit]
		}
		reached = append(reached, chunks...)
		// Admittance mirrors _admit_evidence exactly: per-call dedup by chunk
		// id, the chunk ID as the evidence reference (Python's `ids` holds ids,
		// not pool positions), and only chunks NEW to the shared pool appended
		// to it — so REDUNDANT means "nothing new", not "nothing returned".
		//
		// ONE query's batch is one critical section: Python's per-query loop has
		// no await (the awaits sit in the outer query loop, :691-700), so asyncio
		// cannot interleave two sessions' batches. Locking per chunk would let
		// them interleave into pool orders Python can never produce.
		e.deps.KB.Admit(func(p *PoolAdmitter) {
			// Python _admit_evidence computes _claim_covered_ids(kbinfos) from the
			// LIVE pool once per call; compute it once per batch under the same
			// critical section.
			covered := p.ClaimCoveredIDs()
			// The cap exemption for this batch, derived from the probe's own terms
			// (see probeTerms / PoolAdmitter.Novelty): a full pool still takes the
			// window that answers a name nothing in the pool has reached yet,
			// because that window is the batch's result rather than one more
			// passage.
			novel := p.Novelty(probeTerms(q))
			for _, c := range chunks {
				// Python _admit_evidence early-stops at the top once the shared pool
				// reaches the cap, BEFORE the per-call dedup — except for the probe
				// window above, which IS the answer the caller asked for.
				if p.Full() && !novel.Admits(c) {
					continue
				}
				cid := ChunkIDOf(c)
				if seen[cid] {
					continue
				}
				// Python :672-676 — already quoted verbatim by a pooled claim →
				// skip the full passage (table chunks exempt: their answer rows
				// survive only in full text). Not pooled, not passed to the model.
				if p.CoveredByClaim(cid, covered, IsTableChunk(c)) {
					continue
				}
				seen[cid] = true
				evidenceIDs = append(evidenceIDs, cid)
				payload = append(payload, passageFromChunk(c))
				// Pool identity uses chunkKey (Go's stable key): Python's _chunk_key
				// falls back to id(ck) — the dict's address — so an equivalent
				// re-retrieved chunk never matches and is appended again.
				if p.Add(c) {
					newChunks++
				}
			}
		})
		// Pool this search's doc_aggs (skipped when it returned no chunks, like
		// _merge_kbinfos).
		e.deps.KB.MergeDocAggs(aggs)
	}

	// ── Named-term seats ─────────────────────────────────────────────────────
	// The individuals this call named are honored IN FULL, independent of the
	// expensive leg's own limits: one cheap keyword search per term the call's
	// retrievals did not reach, each keeping the window that carries it (see
	// TermSeat). The terms of list items maxQ dropped are included — "the model
	// already said the name" should mean the name was looked for, whatever
	// syntax carried it and whichever leg ran.
	//
	// A term that reaches nothing is RECORDED, not retried (Kbinfos.RecordProbedAbsent):
	// "this corpus has no 荀正" is the answer to a probe, and it is what the
	// rewrite reads before choosing its next angle.
	if named := namedTermsOf(allQueries); len(named) > 0 {
		seatScope := []string(nil)
		if name != "search_chunks" {
			// doc_scope is a retrieve-family argument; search_chunks takes none
			// (see the query loop above).
			seatScope = toolDocScope(args)
		}
		unreached := termsNotCarried(reached, named)
		// Every named term is PROBED (recall: its own window enters the pool), but
		// only the ones the call proposed as ITEMS are RECORDED — the ledger is read
		// back as the session's to-do list, and a question's words are not members
		// it could record (see probeItemsOf).
		proposed := probeItemsOf(allQueries)
		seats, absent := 0, 0
		for _, term := range unreached {
			record := proposed[strings.ToLower(term)]
			seat, found := TermSeat(ctx, e.deps, SearchParams{
				KbIDs:    e.req.DatasetIDs,
				DocScope: seatScope,
			}, term)
			if !found {
				absent++
				if record {
					e.deps.KB.RecordProbedAbsent(term)
				}
				continue
			}
			// The seat's ids are collected here and recorded AFTER the batch: the
			// ledger has its own lock, so writing it inside the critical section
			// would be safe, but keeping the pool lock to pool work costs nothing
			// and leaves the locking order (pool → ledger) exercised in one place
			// only.
			var seatedIDs []string
			e.deps.KB.Admit(func(p *PoolAdmitter) {
				// A seat is a probe's answer, so it is exempt from the pool cap
				// on the same grounds as the weave above (see Novelty).
				novel := p.Novelty([]string{term})
				for _, c := range seat {
					if p.Full() && !novel.Admits(c) {
						continue
					}
					cid := ChunkIDOf(c)
					if cid != "" && seen[cid] {
						continue
					}
					if cid != "" {
						seen[cid] = true
					}
					evidenceIDs = append(evidenceIDs, cid)
					payload = append(payload, passageFromChunk(c))
					if p.Add(c) {
						newChunks++
					}
					if cid != "" {
						seatedIDs = append(seatedIDs, cid)
					}
					seats++
				}
			})
			// A seat IS the proof that this name is a member: record the pair
			// (term, passage) so the round's record holds the members with their
			// evidence, not just the count — again only for a proposed item.
			if record {
				for _, cid := range seatedIDs {
					e.deps.KB.RecordReachedTerm(term, cid)
				}
			}
		}
		if len(unreached) > 0 {
			logger.Printf("[Action Session] named-term seats: %d named, %d unreached, %d seat(s) admitted, %d absent.",
				len(named), len(unreached), seats, absent)
		}
	}

	// A cut is a fact about the SEARCH, not about the corpus, and the model cannot
	// read it off the passages: every dropped query's terms were seated above, so
	// a name it asked about still came back, while the query's own wording never
	// ran. Say so, so a dropped facet is re-asked instead of forgotten.
	if dropped > 0 {
		reachNotes = append(reachNotes, fmt.Sprintf(
			"only %d of this call's %d queries were searched — %d were dropped, and a dropped query's own wording was never run (the terms it named were each probed on their own, so a term that came back is proven to occur). Re-ask a dropped one directly if you need its wording.",
			maxQ, len(allQueries), dropped))
	}
	if len(payload) == 0 {
		return ToolOutcome{
			Payload:     []any{},
			EvidenceIDs: nil,
			Status:      StatusMiss,
			Reason:      ReasonNoDoc,
			Metrics:     map[string]any{"hits": 0, "new_evidence": 0},
			Note:        strings.Join(reachNotes, "\n"),
		}, nil
	}
	// Zero new evidence (every hit was already in the pool) is REDUNDANT, not
	// OK. Python _search_outcome still returns the
	// FULL payload here — the model sees the passages AND a redundant status, so
	// it knows the ground is already covered. The session tool node appends the
	// "ALREADY in your evidence" note on StatusRedundant,
	// which is what stops the re-issue; dropping the payload (as an earlier port
	// did) hid the evidence the model needs and is the divergence from Python.
	if newChunks == 0 {
		return ToolOutcome{
			Payload:     payload,
			EvidenceIDs: evidenceIDs,
			Status:      StatusRedundant,
			Reason:      ReasonNone,
			Metrics:     map[string]any{"hits": len(payload), "new_evidence": 0},
			Note:        strings.Join(reachNotes, "\n"),
		}, nil
	}
	return ToolOutcome{
		Payload:     payload,
		EvidenceIDs: evidenceIDs,
		Status:      StatusOK,
		Reason:      ReasonNone,
		Metrics:     map[string]any{"hits": len(payload), "new_evidence": newChunks},
		Note:        strings.Join(reachNotes, "\n"),
	}, nil
}

// calculate mirrors Python's calculate tool (action_session.py:_exec_calculate): derive a
// number the evidence does not state outright, by having the model write ONE
// expression and evaluating it against the AST whitelist (see arithmetic.go).
//
// The computed value is returned as evidence, so a later answer step can cite it
// without re-deriving. Nothing derivable is POOR/no_doc, not an error — the model
// then answers from the facts it already has.
func (e *searchExecutor) calculate(ctx context.Context, args map[string]any) (ToolOutcome, error) {
	// Python validates NOTHING here: an absent question or fact list flows into
	// compute_from_facts, whose own `if not question or not facts` guard returns
	// None — a POOR/no_doc "nothing derivable", never a bad-args error. There is
	// no fallback to the run question either.
	question := argString(args, "question")
	facts := make([]string, 0, 8)
	for _, f := range toolStringList(args, "facts") {
		if s := strings.TrimSpace(f); s != "" {
			facts = append(facts, s)
		}
	}
	if e.deps.Model == nil {
		// Python :946-948 — no chat model is an INFRA failure, checked BEFORE any
		// derivation, with the payload Python emits verbatim.
		return ToolOutcome{
			Payload: []any{map[string]any{
				"kind":  "calculate",
				"error": "no model",
			}},
			Status:  StatusError,
			Reason:  ReasonInfra,
			Metrics: map[string]any{},
		}, nil
	}
	cf := ComputeFromFacts(ctx, e.deps.Model, question, facts, 0)
	if cf == nil {
		// Nothing derivable is POOR/no_doc (Python :954-960), with Python's note.
		return ToolOutcome{
			Payload: []any{map[string]any{
				"kind":       "calculate",
				"expression": nil,
				"note":       "no numeric answer derivable from given facts; answer directly or retrieve more numbers.",
			}},
			Status:  StatusPoor,
			Reason:  ReasonNoDoc,
			Metrics: map[string]any{},
		}, nil
	}
	// Python :961 — the success payload is exactly {"kind","expression","result"};
	// the label/uses the model returned are deliberately NOT echoed back.
	return ToolOutcome{
		Payload: []any{map[string]any{
			"kind":       "calculate",
			"expression": cf.Expression,
			"result":     cf.Value,
		}},
		Status:  StatusOK,
		Reason:  ReasonNone,
		Metrics: map[string]any{},
	}, nil
}

// toolStringList reads a string-list arg, tolerating []any, []string, and a
// single string (models emit all three).
func toolStringList(args map[string]any, key string) []string {
	raw, ok := args[key]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if item == nil {
				continue
			}
			out = append(out, fmt.Sprint(item))
		}
		return out
	case string:
		return []string{v}
	}
	return nil
}
func toolQueries(args map[string]any) []string {
	raw, ok := args["query"]
	if !ok || raw == nil {
		// Absent (or an explicit null, which JSON models emit) falls back to the
		// `q` alias before giving up.
		if q, ok := args["q"].(string); ok && strings.TrimSpace(q) != "" {
			return []string{strings.TrimSpace(q)}
		}
		return nil
	}
	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s := strings.TrimSpace(fmt.Sprint(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(v))
		for _, s := range v {
			if strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// toolDocScope reads the optional doc_scope restriction — the tool argument
// Python parses in exactly two dispatchers: retrieve (action_session.py:execute_tool)
// and graph_explore (:977). Both do
//
//	[str(d) for d in (args.get("doc_scope") or []) if str(d).strip()]
//
// so ONLY the "doc_scope" key is honoured: there is deliberately NO "doc_ids"
// alias and NO bare-string coercion (Python would iterate a string char-wise;
// no tool schema advertises either key, so both cases are unreachable). The
// other tools that used to call this — search_chunks, navigate_tree,
// navigate_structure — must NOT read a scope: Python's _exec_search_chunks,
// _exec_navigate_tree and _exec_navigate_structure never thread args["doc_scope"]
// into their impls.
func toolDocScope(args map[string]any) []string {
	raw, ok := args["doc_scope"]
	if !ok {
		return nil
	}
	var items []string
	switch v := raw.(type) {
	case []string:
		items = v
	case []any:
		items = make([]string, 0, len(v))
		for _, item := range v {
			items = append(items, fmt.Sprint(item))
		}
	default:
		return nil
	}
	out := make([]string, 0, len(items))
	for _, s := range items {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// passageFromChunk renders one chunk as the passage dict the model sees: the keys
// of Python _admit_evidence (:667-673) — {"id", "content", "doc_id"}, no title and
// no query. The id must stay under "id": the drill merge reads it as entry["id"],
// so a "chunk_id" key silently disables the drill's structure_path attachment.
func passageFromChunk(c map[string]any) map[string]any {
	// Table chunks pass through un-truncated: the 1200-char cap would hide rows
	// mid/late in a long standings table.
	var content string
	if IsTableChunk(c) {
		content = ChunkTextOf(c)
	} else {
		// Python _admit_evidence: content = _ct[:1200], a plain slice (no trim,
		// no ellipsis).
		content = truncateRunes(ChunkTextOf(c), 1200)
	}
	return map[string]any{
		"id":      ChunkIDOf(c),
		"content": content,
		"doc_id":  DocIDOf(c),
	}
}

// PublishReferences writes the accumulated evidence into the canvas state so the
// agent's post-stream citation grounding can read it.
//
// Chunk and aggregation shapes match runtime's referenceChunksFromRetrieval /
// referenceDocAggsFromRetrieval (which dual-write both Python and Go field
// names), so no translation is needed by the consumer. Exported because the
// caller sits in the parent advanced_rag package (agentic_rag.go), not
// inside harness.
func PublishReferences(ctx context.Context, kb *Kbinfos) {
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil || state == nil {
		return
	}
	chunks := make([]map[string]any, 0, len(kb.Chunks))
	for idx, c := range kb.Chunks {
		chunkID := ChunkIDOf(c)
		docID := DocIDOf(c)
		name := DocTitleOf(c)
		content := ChunkTextOf(c)
		datasetID := DatasetIDOf(c)
		chunks = append(chunks, map[string]any{
			"id":                  fmt.Sprint(idx),
			"chunk_id":            chunkID,
			"content":             content,
			"content_with_weight": content,
			"document_id":         docID,
			"doc_id":              docID,
			"document_name":       name,
			"docnm_kwd":           name,
			"dataset_id":          datasetID,
			"kb_id":               datasetID,
		})
	}
	state.SetRetrievalReferences(chunks, kb.DocAggs)
}

// RuntimeRetriever adapts runtime.GetRetrievalService() to the harness Retriever
// interface. The service is read on every call (not captured at construction)
// because the server installs it during boot, which may happen after a harness
// component was built.
//
// This is the only place in the harness that knows about internal/agent/runtime;
// it lives in the harness root package (not a sub-package) so the runtime
// dependency does not leak into the harness's testable core.
// RuntimeRetriever is the production harness retriever. It mirrors Python's
// settings.retriever: it runs the backend search and returns the normalised
// chunks. Child-fragment promotion (retrieval_by_children) is NOT done here —
// it is entry-point specific (only hybrid_search / RAGTools.retrieve do it in
// Python, search.py:_normalize / agentic_rag.py:RAGTools.retrieve), so it lives in runSearch, gated
// by searchOpts.promoteChildren, and this Backend stays caller-agnostic.
type RuntimeRetriever struct{}

func (r *RuntimeRetriever) Retrieve(ctx context.Context, req RetrieveRequest) ([]map[string]any, error) {
	svc := runtime.GetRetrievalService()
	chunks, err := svc.Search(ctx, nil, runtime.RetrievalRequest{
		Query:                 req.Query,
		DatasetIDs:            req.DatasetIDs,
		DocScope:              req.DocScope,
		TopN:                  req.TopN,
		TopK:                  req.TopK,
		RerankCandidatesCount: req.RerankCandidatesCount,
		// Passed through as pointers: nil (the caller did not supply one) must
		// stay nil so the retrieval service keeps its own default, while a zero
		// is a real override. Taking the address of a zero value here forced
		// "threshold 0 / full vector weight" onto every caller that omitted them.
		SimilarityThreshold:    req.SimilarityThreshold,
		VectorSimilarityWeight: req.VectorSimilarityWeight,
		DisableVectorLeg:       req.DisableVectorLeg,
		TenantID:               req.TenantID,
		RankFeature:            req.RankFeature,
		// ExcludeCompiled maps Python hybrid_search's
		// must_not={"exists":"compile_kwd"} onto the runtime request's
		// OnlyOriginalText (the "no compile_kwd" exclusion).
		OnlyOriginalText: req.ExcludeCompiled,
	})
	if err != nil {
		return nil, err
	}
	return chunksToMaps(chunks), nil
}

// chunksToMaps normalises runtime.RetrievalChunk values into the harness chunk
// shape (the keys the rest of the harness expects: chunk_id / content / doc_id
// / docnm_kwd / ...).
func chunksToMaps(chunks []runtime.RetrievalChunk) []map[string]any {
	out := make([]map[string]any, 0, len(chunks))
	for _, c := range chunks {
		out = append(out, map[string]any{
			"chunk_id": c.ID,
			"content":  c.Content,
			// content_with_weight is the engine/storage spelling of the body.
			// Carried as an alias because the shared reference builder
			// (chunksFormat → getValue("content_with_weight", "content")) and
			// the citation prompt read the storage name; without it those
			// readers get an empty body from an agentic chunk.
			"content_with_weight": c.Content,
			"doc_id":              c.DocumentID,
			"docnm_kwd":           c.DocumentName,
			"dataset_id":          c.DatasetID,
			"kb_id":               c.DatasetID,
			// doc_type_kwd is what marks a chunk as an image (or table); the
			// reference card renders it and drops the marker without it.
			"doc_type_kwd":      c.DocType,
			"mom_id":            c.MomID,
			"similarity":        c.Score,
			"image_id":          c.ImageID,
			"url":               c.URL,
			"positions":         c.Positions,
			"chunk_order_int":   c.ChunkIndex,
			"page_num_int":      c.PageNum,
			"score":             c.Score,
			"term_similarity":   c.TermSimilarity,
			"vector_similarity": c.VectorSimilarity,
		})
	}
	return out
}

// TenantIDFromContext returns the tenant id bound on the canvas context, used as
// a fallback for SearchDeps.TenantID when the caller's SearchRequest leaves it
// empty. The harness stays independent of the tool registry, so it reads only
// what CanvasState.GetVar exposes. Exported so the advanced_rag package's Run can call
// it.
func TenantIDFromContext(ctx context.Context) string {
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil || state == nil {
		return ""
	}
	if tid, err := state.GetVar("tenant_id"); err == nil {
		if s, _ := tid.(string); s != "" {
			return s
		}
	}
	return ""
}
