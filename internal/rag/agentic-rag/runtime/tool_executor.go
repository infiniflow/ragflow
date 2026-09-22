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

// Package runtime holds the low-level agentic-RAG capability library: the
// retrieval backend, the action-session runtime, the tool executor, and the
// compiled-structure / knowledge-graph navigation. These are the leaf building
// blocks the RAGTools methods in the parent `agent` package (Run, ComposeAnswer,
// ComposeNaiveAnswer, Formalize, ...) orchestrate.
//
// tool_executor.go is the tool-executor adapter: its searchExecutor type and the Execute
// dispatch are the search / navigation / calculation / web tools the action session calls.
// They are pulled out into this file so the runtime package need not import the
// agentic_rag package.
package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/service/nav"
)

// RunRequest is one runtime run. It lives in the runtime package (not the agent
// package) because searchExecutor needs it to carry the per-run query, and the
// agentic_rag package must not be imported from here (import-cycle rule). RAGTools.Run
// in the agentic_rag package uses it via the runtime package.
type RunRequest struct {
	// Question is the user's question.
	Question string
	// ThinkingMode selects the ModeSpec ("low"/"medium"/"high"/"ultra").
	// An unrecognised value degrades to NAIVE.
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
	// MaxLength is the chat model's context window. It bounds evidence and document-level
	// reads.
	// <=0 selects the file-level defaults.
	MaxLength int
	// SessionID identifies the conversation this call belongs to. It is carried
	// for plumbing (e.g. future session-scoped state) but is not read by the
	// near-duplicate answer cache: that cache is per-request, i.e. per turn.
	SessionID string
	// Images are vision-gated base64 data URIs. The
	// outer react loop turns them into multimodal content blocks on the last
	// user message so a vision model sees them (agentic_rag.Rag assembles the
	// message from these).
	Images []string
	// TextAttachments is the joined text-file content, appended to the question so the
	// model reads attached documents in the reasoning path.
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
// Exported so the agentic_rag package (which cannot be imported from here) can reuse
// the same evidence handling instead of duplicating it.
func NewSearchExecutor(deps SearchDeps, req RunRequest) ToolExecutor {
	return &searchExecutor{deps: deps, req: req}
}

// CoverageRunner is the tool layer's enumeration seam: the runtime asks the EXECUTOR to
// run the direction's own enumeration over the corpus, so the step belongs to the graph
// rather than to a prompt.
//
// It is a runner rather than a caller-side helper for one reason: the runtime has to be
// able to ask the corpus itself. Queries handed to the model as a list are advice and may
// simply not be run, so the enumeration is a step of the graph, and the only thing it needs
// from the tool layer is a search.
type CoverageRunner interface {
	EnumerateCoverage(ctx context.Context, cov Coverage, kb *Kbinfos) CoverageSet
}

// EnumerateCoverage runs the direction's OWN enumeration over the corpus: one recall per
// operand (the actor's declared forms and the act words), then the windows where the deed
// is stated (see EnumerateCoverage in coverage_enumerate.go).
func (e *searchExecutor) EnumerateCoverage(ctx context.Context, cov Coverage, kb *Kbinfos) CoverageSet {
	deps := e.deps
	if len(deps.KbIDs) == 0 && len(e.req.DatasetIDs) > 0 {
		deps.KbIDs = e.req.DatasetIDs
	}
	return EnumerateCoverage(ctx, deps, cov, kb)
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
	// "[Function tool] Running the {name} tool with: {args}" — one of the namespaces the
	// thinking block shows. The log line and the step below are the same call site on
	// purpose: a tool step is a step because this method says so, not because a tagged
	// line matched a pattern.
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
	case "metadata_search":
		return e.metadataSearch(ctx, args)
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
// line and the event's Args field. The args dict is marshalled to JSON so nested
// values (scopes, tag maps) stay readable. Empty args render as "{}" rather than an
// empty string.
//
// String values are capped (capForThink): this field is rendered for display, and
// one uncapped slot-evidence query turns a single call into a paragraph repeated
// on every line of its round.
//
// Exported because the outer react loop (agentic_rag.outerReactSession) logs the
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
// Shared with the outer react loop's narration (agentic_rag.outerReactSession)
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

// summarizeDocument loads the document and hands the model the freshly rendered evidence
// blocks.
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
	router := e.navRouter(ctx)
	// The tool argument is NOT threaded (navigate_tree takes query + keywords only), but
	// the SESSION scope still applies: the router ceilings its scope with the session
	// doc_scope. The router must therefore receive the session ceiling, never the tool
	// argument.
	res := NavigateTree(ctx, router, NavTreeInput{
		Query: query,
		// threads ONLY the tool argument's keywords (usually absent, so "") — passing the
		// run-level request keywords re-biased every tree routing toward the original
		// question keywords.
		Keywords: argString(args, "keywords"),
		DocScope: e.deps.DocScope,
		TenantID: e.deps.TenantID,
		KbIDs:    e.deps.KbIDs,
	})

	switch res.EmptyReason {
	case ReasonNoStructure:
		return ToolOutcome{
			Payload: []any{map[string]any{
				// The dataset_has_compilation gate: the payload carries kind+note only.
				"kind": "navigate_tree",
				"note": "This dataset has no compiled document-navigation structure; use search_chunks / retrieve instead.",
			}},
			Status: StatusEmpty,
			Reason: ReasonNoStructure,
		}, nil
	case ReasonNoDoc:
		// Structure exists, this query reached nothing — a MISS, not an EMPTY.
		// note verbatim; the payload carries kind+note only.
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

	// Routed: expose the summary-bearing payload the ladder consumes, capped at 8000
	// chars.
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

// chunkAggRetrieveFrom adapts a runtime Retriever into the chunk_agg retrieve
// leg. It scopes the search to the dataset (kbID) and the caller's document
// scope and pulls the wide pool chunk_agg needs; the backend is expected to
// exclude compiled rows (the production retrieval filters available_int=1).
//
// It stays in the runtime package because it depends on the agentic Retriever /
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
// reads only the compact graph-blob rows, whereas BOTH row shapes (graph blob AND
// per-entity/relation rows) should be merged. ParseCompiledStructure in navtools.go
// already implements the merged parsing; wiring it into the reader is the remaining
// step.
func (e *searchExecutor) navigateStructure(ctx context.Context, args map[string]any) (ToolOutcome, error) {
	query := argString(args, "query")
	docID := argString(args, "doc_id")
	if query == "" {
		query = e.req.Question
	}
	if query == "" {
		return ToolOutcome{Payload: []any{}, Status: StatusError, Reason: ReasonBadArgs}, nil
	}
	// No scope-as-doc-pin: only args["doc_id"] is read — args["doc_scope"] is never
	// consulted.
	kind := argString(args, "kind")
	if kind == "" {
		kind = "catalog"
	}

	// Resolve the document set. When the caller omits doc_id, route to the documents by
	// descending the compiled navigation tree (the exact seam navigate_tree uses), not by
	// refusing the call. The routed set is capped at navTreeMaxDocs.
	var docIDs []string
	if docID != "" {
		docIDs = []string{docID}
	} else {
		// DocScope is the session ceiling, not the tool argument (see the note in
		// navigateTree): args["doc_scope"] is never threaded, but the routing done when
		// doc_id is absent applies the session document scope.
		res := NavigateTree(ctx, e.navRouter(ctx), NavTreeInput{
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
			// Routing reached no document (empty_reason="no_doc"). The SAME note is emitted
			// for every empty_reason.
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

	// Zero-LLM vector-beam drill-down: read each document's compiled structure of the
	// requested kind, drill toward the query, and hand the model the merged
	// <structure_navigation> outline with chunk-pointer anchors.
	res, drill := navigateStructures(ctx, e.deps.TenantID, query, docIDs, kind, e.navRouter(ctx), e.deps)
	metrics := map[string]any{
		"entities":    drill.nodes,
		"chunk_ptrs":  drill.chunkPtrs,
		"top_score":   drill.topScore,
		"chunk_paths": drill.chunkPaths,
		"claim_hits":  drill.claimHits,
	}
	if res.EmptyReason != "" {
		// one note for every empty_reason; kind renders quoted.
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
// share, decided in this order:
//
//  1. The dataset has a COMPILATION. Navigation routes over compiled structure, so a
//     dataset carrying no compiled rows has nothing to route on and gets
//     noCompilationRouter — the tool then reports no_structure and the session is told to
//     use search_chunks / retrieve, i.e. plain search. Without this gate the route stood
//     behind an uncompiled dataset too, because the chunk leg is a plain similarity
//     ranking that needs no compilation: raw retrieval came back dressed as a structure
//     descent, and the rounds went to "navigation" results that name no structure.
//  2. It follows the agentic router choice: the navigation-tree route asks for
//     router="claim_agg" — the claim leg runs first and decides the ranking when it hits,
//     and raw-chunk aggregation (chunk_agg) is the fallback. A nil retrieval backend
//     degrades to the nav-row router, which needs no backend.
//
// A caller-installed NavRouter is an explicit override and skips both.
func (e *searchExecutor) navRouter(ctx context.Context) NavTreeRouter {
	if e.deps.NavRouter != nil {
		return e.deps.NavRouter
	}
	if !DatasetHasCompilation(ctx, e.deps) {
		return noCompilationRouter{}
	}
	return defaultNavRouter(e.deps)
}

// noCompilationRouter is what an UNCOMPILED dataset gets: Route answers (nil, nil), the
// NavTreeRouter contract's "this dataset has no compiled tree", so NavigateTree reports
// no_structure (dataset-level, and its tool note says to use search_chunks / retrieve)
// instead of routing the query through raw-chunk aggregation.
type noCompilationRouter struct{}

// Route implements NavTreeRouter: no compiled structure, nothing to route, not an error.
func (noCompilationRouter) Route(context.Context, string, string, string, []string, int) ([][2]string, error) {
	return nil, nil
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

// evidencePoolCap is the hard cap on the shared evidence pool. It is deliberately LARGER
// than the SCA view cap (60) so storage and review stay DECOUPLED — the pool accumulates
// while the SCA reads a ranked top-60 view. Coupling them at 60 starved the raw-evidence
// channel in 42% of rounds (every admit rejected -> status REDUNDANT -> the model
// re-searched for nothing).
//
// Claim pseudo-chunks BYPASS this cap: they are appended directly to the pool, and the cap
// only guards regular chunk admission.
//
// The cap is 200 (the old 120 was sized for a consumer that no longer exists) —
// the round-level sweep that rendered the WHOLE pool into one prompt, where the cap
// and that prompt's budget were the same number. Nothing renders the pool whole any
// more (the SCA reads a ranked 60-chunk view, the session seed injects a bounded
// digest, the draft is bounded), so the cap is a storage-discipline number again —
// and on an enumeration it is the NEXT ceiling rather than a prompt limit: what it
// refuses is the passage that would have reached the last members, which are named
// late in the pool's order.
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
// the model may never write — a `|`-triggered mechanism would fire never.
//
// The cap (GrepTermsMax) bounds the seat pass, which runs one cheap keyword
// search per unreached term; the caller logs how many named terms were dropped
// so the ceiling is visible in the run.
//
// The terms are the caller's OWN WORDS (GrepWordsFromQuery), not the CJK windows
// the locate step derives from them: a window is our guess at where a name can be
// found, and probing one spends a retrieval on a fragment nobody asked about.
func namedTermsOf(queries []string) []string {
	var out []string
	seen := make(map[string]bool, len(queries)*2)
	for _, q := range queries {
		for _, t := range GrepWordsFromQuery(q) {
			// A PIECE OF A PATTERN is a phrase, not a name: an alternation carrying
			// operators asks how a deed is written, and probing its pieces as names
			// spends a retrieval on a word nobody proposed as a member. An alternation
			// of PLAIN words is exactly what the seat exists for, so the test is pattern
			// OPERATORS, not "|".
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
// The two retrieval tools share this body, differing only in whether compiled expansion
// runs (search_chunks expands, retrieve does not) and in the accepted query count.
func (e *searchExecutor) search(ctx context.Context, name string, args map[string]any) (ToolOutcome, error) {
	queries := toolQueries(args)
	if len(queries) == 0 {
		// A missing/blank query yields no queries and simply admits nothing: the outcome is
		// MISS/no_doc, NOT a bad-args error.
		return ToolOutcome{
			Payload:     []any{},
			EvidenceIDs: nil,
			Status:      StatusMiss,
			Reason:      ReasonNoDoc,
			Metrics:     map[string]any{"hits": 0, "new_evidence": 0},
		}, nil
	}
	// The pool is CREATED when absent, so every search has a (possibly empty) pool to admit
	// into: the search runs and its outcome is decided by what it actually admitted.
	if e.deps.KB == nil {
		e.deps.KB = &Kbinfos{}
	}
	logger := searchLogger(e.deps)

	// Max queries per tool call: retrieve=3, search_chunks=2. grep_search/grep_chunks are
	// Go-internal tools, so they are
	// left uncapped.
	//
	// A direction assembling a SET/COUNT raises the ceiling. There the call's
	// queries are facets of one list rather than rephrasings of one question, so
	// a dropped query is not a spared repeat — it is a member nobody searched.
	// The flag is set by the same gate that hands the model the set method (see
	// Kbinfos.MarkSetDirection), so a VALUE direction keeps the small cap it had.
	// Calls ask several queries apiece, and a name the answer later turns out to be
	// missing was often named only in a dropped one.
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
	// before any retrieval happens, and those individuals can be the only mentions of
	// the very names the answer is missing.
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
	// Per-call admittance state: `seen` dedups chunks ACROSS the queries of this one call.
	// The pool-side dedup is Kbinfos.Admit's job, against the LIVE pool — a per-call
	// snapshot of it is exact only while nothing can interleave, and another session
	// appending makes it stale.
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
	// Claim-first, MUTUALLY EXCLUSIVE: when claim rows hit, their verbatim evidence IS the
	// answer material — chunk snippets on top would echo the same passages and burn tokens.
	// Claims carry chunk pointers, so deep-reading stays one list_chunks away. No hits →
	// the chunk search runs exactly as before. Applies to the whole retrieve family +
	// search_chunks. Best effort: any failure falls through without claims.
	if name == "retrieve" || name == "search_chunks" || strings.HasPrefix(name, "grep") {
		if len(queries) > 0 {
			if claimPayload, _, claimPseudo, ok := ClaimPrefetch(ctx, e.deps, queries[0], seen); ok {
				newEvidence := 0
				e.deps.KB.Admit(func(p *PoolAdmitter) {
					for _, pc := range claimPseudo {
						// Claim pseudo-chunks BYPASS the pool cap: they are appended directly to the
						// pool and the cap only guards regular chunk admits — the verbatim
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
		// Per-tool top_n: retrieve=10, search_chunks=20. Collapsing both onto e.req.TopN
		// made search_chunks return far fewer candidates.
		topN := 10
		if name == "search_chunks" {
			topN = 20
		}
		// These tools dispatch to genuinely different search functions: retrieve/grep_* →
		// grep_search (keyword-only), search_chunks → hybrid_search (vector leg when an
		// embedder is configured). Each is its own function so a single global switch can no
		// longer disable the semantic leg.
		//
		// search_chunks ALWAYS enables compiled-structure expansion, in ALL modes, not just
		// high; retrieve/grep_search never do. So compiled is on for search_chunks and off for
		// every other retrieve-family tool, independent of e.req.UseCompiled (which gates the
		// L1 direct retrieve, not the action-session tool loop).
		var searchFn func(context.Context, SearchDeps, SearchParams) ([]map[string]any, []map[string]any)
		useCompiled := name == "search_chunks"
		if useCompiled {
			searchFn = HybridSearch
		} else {
			searchFn = GrepSearch
		}
		// Keywords differ per tool:
		//   - retrieve → grep_search(keywords=nav_hint or None). The nav hint is an explicit
		//     PARAMETER, passed ONLY by the navigation ladder; the model's own retrieve
		//     dispatch passes none, in which case GrepSearch falls back to the query's own
		//     extracted terms and turns them into the BM25 hint. The session run keywords
		//     (req.Keywords) are never forwarded.
		//   - search_chunks takes no keywords at all, so hybrid_search's narrowing is a
		//     no-op.
		var kws string
		if !useCompiled {
			kws = argString(args, "nav_hint")
		}
		// doc_scope is honoured by the retrieve family only: retrieve reads
		// args["doc_scope"], while search_chunks takes no doc_scope at all and its
		// hybrid_search call is unscoped. Gating on useCompiled keeps that split.
		var docScope []string
		if !useCompiled {
			docScope = toolDocScope(args)
		}
		// The aggregations the search body would otherwise drop are what the answer's
		// document/reference list is built from — this tool is their only writer.
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
		// — EXCEPT for a query that NAMES
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
		// id, the chunk ID as the evidence reference (ids, not pool positions), and only
		// chunks NEW to the shared pool appended to it — so REDUNDANT means "nothing new",
		// not "nothing returned".
		//
		// ONE query's batch is one critical section: the per-query loop has no await, so
		// two sessions' batches cannot interleave. Locking per chunk would let them
		// interleave into pool orders that should not happen.
		e.deps.KB.Admit(func(p *PoolAdmitter) {
			// The claim-covered set is computed from the LIVE pool once per batch, under the
			// same critical section.
			covered := p.ClaimCoveredIDs()
			// The cap exemption for this batch, derived from the probe's own terms
			// (see probeTerms / PoolAdmitter.Novelty): a full pool still takes the
			// window that answers a name nothing in the pool has reached yet,
			// because that window is the batch's result rather than one more
			// passage.
			novel := p.Novelty(probeTerms(q))
			for _, c := range chunks {
				// Admission early-stops once the shared pool reaches the cap, BEFORE the
				// per-call dedup — except for the probe window above, which IS the answer the
				// caller asked for.
				if p.Full() && !novel.Admits(c) {
					continue
				}
				cid := ChunkIDOf(c)
				if seen[cid] {
					continue
				}
				// already quoted verbatim by a pooled claim →
				// skip the full passage (table chunks exempt: their answer rows
				// survive only in full text). Not pooled, not passed to the model.
				if p.CoveredByClaim(cid, covered, IsTableChunk(c)) {
					continue
				}
				seen[cid] = true
				evidenceIDs = append(evidenceIDs, cid)
				payload = append(payload, passageFromChunk(c))
				// Pool identity uses chunkKey (a stable key): an address-based key would never
				// match an equivalent re-retrieved chunk, and it would be appended again.
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
	// OK. The FULL payload is still returned here — the model sees the passages AND a
	// redundant status, so it knows the ground is already covered. The session tool node
	// appends the "ALREADY in your evidence" note on StatusRedundant, which is what stops
	// the re-issue; dropping the payload (as an earlier version did) hid the evidence the
	// model needs.
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

// metadataKeysHintMax caps the available-keys list echoed back on a bad key.
const metadataKeysHintMax = 30

// metadataSearch is the metadata_search tool: a document-metadata SELECTOR. It resolves the
// documents a filter matches and returns their doc_ids — it runs NO retrieval of its own.
//
// The ids are the whole output on purpose: a document handle can be spent on several
// follow-up calls (list_chunks, navigate_structure, retrieve's doc_scope, and in ultra
// mode graph_explore's doc_scope), whereas the passages an in-tool retrieval used to return
// could only be read once. Keeping the selector free of retrieval is also what keeps its
// answer "which documents match" independent of what the model meant to search for.
//
// The statuses are set explicitly rather than derived (see ReasonStatus): a missing filter
// or a metadata key the dataset does not carry is MISS/bad_args — the model should change
// the filter or switch tools, not read it as an infrastructure failure — a filter matching
// no document is MISS/no_doc, and only a metadata-index read failure is ERROR/infra.
// (Python folds a keys-query exception into "this dataset has no metadata", which tells the
// model the wrong thing about the corpus.)
func (e *searchExecutor) metadataSearch(ctx context.Context, args map[string]any) (ToolOutcome, error) {
	// searchLogger falls back to the package logger: Logger is optional on SearchDeps,
	// and a nil *log.Logger panics on the first Printf.
	logger := searchLogger(e.deps)
	badArgs := func(note string) (ToolOutcome, error) {
		return ToolOutcome{
			Payload: []any{map[string]any{"kind": "metadata_search", "note": note}},
			Status:  StatusMiss,
			Reason:  ReasonBadArgs,
			Metrics: map[string]any{"docs": 0},
		}, nil
	}
	filters := metadataFiltersOf(args)
	if len(filters) == 0 {
		return badArgs("No metadata conditions given. metadata_search needs at least one {key, value, op} filter — otherwise use search_chunks / retrieve.")
	}
	if e.deps.MetadataResolver == nil {
		return ToolOutcome{
			Payload: []any{map[string]any{
				"kind": "metadata_search",
				"note": "metadata_search is unavailable in this deployment (no metadata resolver is wired). Use search_chunks / retrieve.",
			}},
			Status:  StatusError,
			Reason:  ReasonInfra,
			Metrics: map[string]any{"docs": 0},
		}, nil
	}
	targetIDs := metadataTargetIDs(e.deps)
	if len(targetIDs) == 0 {
		return ToolOutcome{
			Payload: []any{},
			Status:  StatusError,
			Reason:  ReasonInfra,
			Metrics: map[string]any{"docs": 0},
		}, nil
	}

	// 1) Key validation against the dataset's OFFERED metadata fields — the same union of
	//    declared and indexed fields the tool schema advertises (see resolveMetadataFields).
	//    Validating against anything else would let the schema offer a key the tool then
	//    rejects; a dataset without the requested key must degrade to a hint, never to an
	//    empty retrieval the model retries forever.
	known, _, declared, err := resolveMetadataFields(ctx, e.deps)
	if err != nil {
		logger.Printf("[Metadata search] metadata index read failed: %v", err)
		return ToolOutcome{
			Payload: []any{map[string]any{
				"kind": "metadata_search",
				"note": "The document-metadata index could not be read (infrastructure failure — NOT a statement about the dataset). Fall back to search_chunks / retrieve.",
			}},
			Status:  StatusError,
			Reason:  ReasonInfra,
			Metrics: map[string]any{"docs": 0},
		}, nil
	}
	knownSet := make(map[string]bool, len(known))
	for _, k := range known {
		knownSet[k] = true
	}
	var bad []string
	for _, f := range filters {
		if key := asString(f["key"]); !knownSet[key] {
			bad = append(bad, key)
		}
	}
	if len(bad) > 0 {
		available := strings.Join(capStrings(known, metadataKeysHintMax), ", ")
		if available == "" {
			available = "NONE — this dataset has no metadata; use search_chunks / retrieve"
		}
		logger.Printf("[Metadata search] bad key(s) %v — not in the dataset's metadata (available: %s)", bad, available)
		return badArgs(fmt.Sprintf("Metadata key(s) %v do not exist in this dataset. Available: %s.", bad, available))
	}

	// 2) Normalize the filter values BEFORE they reach the push-down / in-memory filter, so
	//    a model that emits a list where one keyword is expected can never silently produce
	//    a no-match.
	normalized := make([]map[string]any, 0, len(filters))
	for _, f := range filters {
		nf := make(map[string]any, len(f)+1)
		for k, v := range f {
			nf[k] = v
		}
		nf["value"] = NormalizeMetadataValue(f["value"], asString(f["op"]))
		normalized = append(normalized, nf)
	}
	logic := argString(args, "logic")
	if logic != "or" {
		logic = "and"
	}

	// 3) Resolve the document set. This IS the tool's output: the ids are returned to the
	//    model so it can spend them on the search tools that take a document handle, and the
	//    filter itself is query-independent (it never needed a query).
	docIDs, ok := MetadataDocIDs(ctx, e.deps, normalized, logic)
	if !ok {
		logger.Printf("[Metadata search] no documents matched filters=%v logic=%s", normalized, logic)
		return ToolOutcome{
			Payload: []any{map[string]any{
				"kind": "metadata_search",
				"note": "No documents match the given metadata conditions. Loosen or change the filters (try a shorter substring, or the space form instead of underscores), or use search_chunks / retrieve for unfiltered search.",
			}},
			Status:  StatusMiss,
			Reason:  ReasonNoDoc,
			Metrics: map[string]any{"docs": 0},
		}, nil
	}

	// 4) Context block: the filter that was actually applied plus each matched document's
	//    own metadata values.
	//
	//    WHY the values travel with the ids. The ids alone are a handle, not an answer, and
	//    the values are exactly what the filter matched ON: a model that must call
	//    list_chunks on every selected document just to learn its title, file name or
	//    timestamp spends a whole round re-reading what the selection already knew. Carrying
	//    them here is what makes the selection readable in one step — "these 3 documents were
	//    updated on 2026-09-20, and here are their titles and file names" — instead of
	//    N round-trips that each cost context.
	//
	//    WHY it is context and NOT an interface. No other tool consumes these values: the
	//    machine-readable part of the result stays doc_ids (the argument retrieve's doc_scope,
	//    list_chunks and navigate_structure take). Keeping the values read-only matters
	//    because they are free-form text — a title or a file name spliced into a later
	//    search argument would let a value the model merely READ silently re-scope the
	//    search, while the document selection is the one thing the filter actually
	//    guarantees. The values therefore explain the selection; they never become input to
	//    the next one.
	//
	//    The two halves are also reported separately for exactly this reason: `doc_ids` is
	//    complete (every match), while `documents` is capped (see metadataContextDocsMax) —
	//    truncating context must never truncate the result the model acts with.
	item := map[string]any{
		"kind":    "metadata_search",
		"doc_ids": docIDs,
		"filters": normalized,
	}
	perDoc, ctxErr := e.deps.MetadataResolver.MetadataForDocIDs(ctx, targetIDs, docIDs)
	if ctxErr != nil {
		logger.Printf("[Metadata search] metadata context read degraded: %v", ctxErr)
	}
	if docs := metadataContextDocs(docIDs, perDoc, metadataContextKeys(normalized, declared)); len(docs) > 0 {
		item["documents"] = docs
	}

	logger.Printf("[Metadata search] %d doc(s) selected via filters=%v logic=%s", len(docIDs), normalized, logic)
	return ToolOutcome{
		Payload: []any{item},
		// No passages are retrieved here, so there is no evidence to admit: the ids reach
		// the answer only through the tool the model spends them on.
		EvidenceIDs: nil,
		Status:      StatusOK,
		Reason:      ReasonNone,
		Metrics:     map[string]any{"docs": len(docIDs)},
	}, nil
}

// Metadata-search context caps: how many matched documents carry their metadata values in
// the result, and how long one value may be.
const (
	metadataContextDocsMax    = 20
	metadataContextValueRunes = 200
)

// metadataContextKeys picks the fields the context block carries per document: first the
// dataset's DECLARED fields (its curated schema — a title, a file name, a timestamp), then
// any field the filter itself named, so the model can see WHY each document matched.
//
// Observed-only keys are deliberately left out: the metadata index also carries extraction
// and annotation noise (a judge's rationale, a verdict) that the dataset never declared as
// metadata, and dumping it into every result would spend context on fields no one can act on.
func metadataContextKeys(filters []map[string]any, declared map[string]common.MetadataFieldDef) []string {
	out := make([]string, 0, len(declared)+len(filters))
	seen := map[string]bool{}
	keys := make([]string, 0, len(declared))
	for k := range declared {
		keys = append(keys, k)
	}
	sort.Strings(keys) // map iteration is randomised; the payload must not be
	for _, k := range keys {
		if k != "" && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	for _, f := range filters {
		k := asString(f["key"])
		if k != "" && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

// metadataContextDocs renders the per-document context block: one {doc_id, metadata} entry
// per matched document, in the selection's own order, capped at metadataContextDocsMax.
// Missing and blank values are skipped, so the block never advertises a field as present
// when the document does not carry it. A nil perDoc (the context read failed or no
// resolver answered) yields nil — the ids alone are still a complete result.
func metadataContextDocs(docIDs []string, perDoc map[string]map[string]any, keys []string) []any {
	if len(perDoc) == 0 || len(keys) == 0 {
		return nil
	}
	out := make([]any, 0, min(len(docIDs), metadataContextDocsMax))
	for _, docID := range docIDs {
		if len(out) >= metadataContextDocsMax {
			break
		}
		fields, ok := perDoc[docID]
		if !ok {
			continue
		}
		values := make(map[string]any, len(keys))
		for _, k := range keys {
			if metadataCatalogExcluded(k) {
				continue
			}
			v, exists := fields[k]
			if !exists || v == nil {
				continue
			}
			rendered := strings.TrimSpace(metadataValueString(v))
			if rendered == "" {
				continue
			}
			values[k] = Snippet(rendered, metadataContextValueRunes)
		}
		if len(values) == 0 {
			continue
		}
		out = append(out, map[string]any{"doc_id": docID, "metadata": values})
	}
	return out
}

// metadataValueString renders one metadata value as text. The doc-metadata index merges a
// document's per-chunk values into a list, so a list joins with ", " rather than reaching
// the model as Go's "[a b]" rendering; a scalar renders as-is.
func metadataValueString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []string:
		return strings.Join(t, ", ")
	case []any:
		parts := make([]string, 0, len(t))
		for _, item := range t {
			if s := strings.TrimSpace(fmt.Sprint(item)); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprint(t)
	}
}

// metadataFiltersOf reads the filters argument: a list of {key, value, op} objects.
// Entries without a key or an op are dropped, so a malformed entry cannot ride along into
// the push-down as a condition that matches everything.
func metadataFiltersOf(args map[string]any) []map[string]any {
	raw, ok := args["filters"]
	if !ok || raw == nil {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		key := strings.TrimSpace(asString(m["key"]))
		op := strings.TrimSpace(asString(m["op"]))
		if key == "" || op == "" {
			continue
		}
		out = append(out, map[string]any{"key": key, "value": m["value"], "op": op})
	}
	return out
}

// NormalizeMetadataValue coerces a model-supplied filter value to the single value a
// metadata filter expects. The string ops (contains / = / start with / end with /
// not contains) take ONE keyword: a list that sneaks in collapses to its first non-empty
// element (one condition = one keyword) and is logged, so the caller can re-issue per
// keyword. 'in' / 'not in' keep their list (the value SET); empty / not empty take no value.
//
// Exported because the two callers that accept model-written metadata conditions — the
// metadata_search tool and the pre-search fan-out channel — must coerce them identically.
func NormalizeMetadataValue(value any, op string) any {
	if op == "in" || op == "not in" {
		return value
	}
	items, ok := value.([]any)
	if !ok {
		return value
	}
	flat := make([]string, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		if s := strings.TrimSpace(fmt.Sprint(item)); s != "" {
			flat = append(flat, s)
		}
	}
	if len(flat) == 0 {
		return nil
	}
	if len(flat) > 1 {
		_LOG.Printf("[Metadata search] value list collapsed to the single keyword %q (ignored: %v); to match all, issue one condition per keyword", flat[0], flat[1:])
	}
	return flat[0]
}

// capStrings returns the first n elements of list (n <= 0 means "no cap").
func capStrings(list []string, n int) []string {
	if n <= 0 || len(list) <= n {
		return list
	}
	return list[:n]
}

// calculate derives a number the evidence does not state outright, by having the model
// write ONE
// expression and evaluating it against the AST whitelist (see arithmetic.go).
//
// The computed value is returned as evidence, so a later answer step can cite it
// without re-deriving. Nothing derivable is POOR/no_doc, not an error — the model
// then answers from the facts it already has.
func (e *searchExecutor) calculate(ctx context.Context, args map[string]any) (ToolOutcome, error) {
	// NOTHING is validated here: an absent question or fact list flows into
	// compute_from_facts, whose own `if not question or not facts` guard returns None — a
	// POOR/no_doc "nothing derivable", never a bad-args error. There is no fallback to the
	// run question either.
	question := argString(args, "question")
	facts := make([]string, 0, 8)
	for _, f := range toolStringList(args, "facts") {
		if s := strings.TrimSpace(f); s != "" {
			facts = append(facts, s)
		}
	}
	if e.deps.Model == nil {
		// no chat model is an INFRA failure, checked BEFORE any derivation.
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
		// Nothing derivable is POOR/no_doc.
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
	// the success payload is exactly {"kind","expression","result"};
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

// toolDocScope reads the optional doc_scope restriction — parsed in exactly two
// dispatchers: retrieve and graph_explore, as
//
//	[str(d) for d in (args.get("doc_scope") or []) if str(d).strip()]
//
// so ONLY the "doc_scope" key is honoured: there is deliberately NO "doc_ids" alias and
// NO bare-string coercion (a string would be iterated char-wise; no tool schema advertises
// a doc_ids alias or a bare string, so both cases are unreachable). The other tools that
// used to call this — search_chunks, navigate_tree, navigate_structure — must NOT read a
// scope: none of them threads args["doc_scope"] into its impl.
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

// passageFromChunk renders one chunk as the passage dict the model sees: {"id",
// "content", "doc_id"} — no title and no query. The id must stay under "id": the drill
// merge reads it as entry["id"],
// so a "chunk_id" key silently disables the drill's structure_path attachment.
func passageFromChunk(c map[string]any) map[string]any {
	// Table chunks are shown to the model as a Markdown view (key-value for infoboxes,
	// a full-row pipe table for ranked/result tables) instead of raw <table> markup:
	// same rows, a fraction of the tokens, and a form the model can aggregate. They
	// also pass through un-truncated: the 1200-char cap would hide rows mid/late in a
	// long standings table. The shared pool keeps the RAW chunk for citation.
	var content string
	if IsTableChunk(c) {
		content = TableViewOrRaw(ChunkTextOf(c))
	} else {
		// Content is a plain slice at 1200 chars (no trim, no ellipsis).
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
// referenceDocAggsFromRetrieval (which dual-write both field-name styles), so no
// translation is needed by the consumer. Exported because the
// caller sits in the parent agentic_rag package (agentic_rag.go), not
// inside retrieval.
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

// RuntimeRetriever adapts runtime.GetRetrievalService() to the runtime Retriever
// interface. The service is read on every call (not captured at construction)
// because the server installs it during boot, which may happen after a runtime
// component was built.
//
// This is the only place in the runtime that knows about internal/agent/runtime;
// it lives in the runtime root package (not a sub-package) so the runtime
// dependency does not leak into the runtime's testable core.
// RuntimeRetriever is the production runtime retriever: it runs the backend search and
// returns the normalised chunks. Child-fragment promotion (retrieval_by_children) is NOT
// done here — it is entry-point specific (only the hybrid and retrieve legs do it), so it
// lives in runSearch, gated by searchOpts.promoteChildren, and this Backend stays
// caller-agnostic.
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
		SimilarityThreshold:      req.SimilarityThreshold,
		KeywordsSimilarityWeight: req.KeywordsSimilarityWeight,
		TenantID:                 req.TenantID,
		RankFeature:              &req.RankFeature,
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

// chunksToMaps normalises runtime.RetrievalChunk values into the runtime chunk
// shape (the keys the rest of the runtime expects: chunk_id / content / doc_id
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
// empty. The runtime stays independent of the tool registry, so it reads only
// what CanvasState.GetVar exposes. Exported so the agentic_rag package's Run can call
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
