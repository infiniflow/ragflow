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
	"sync"

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

// Execute implements ToolExecutor for the wired tools. Tools whose port has not
// landed are classified, not errored: they report MISS (the tool is valid, this
// call reached nothing) so the model falls back to a different tool instead of
// stalling.
func (e *searchExecutor) Execute(ctx context.Context, name string, args map[string]any) (ToolOutcome, error) {
	// Mirror Python rag/llm/tool_decorator.py:tool "[Function tool] Running the
	// {name} tool with: {args}", one of the three namespaces Python's
	// _SCOPED_PREFIXES forwarded into the think block. Python emitted it from a
	// root-logging handler; Go has no root logger, so it is logged through the
	// wrapped deps.Logger, which thinkLogger forwards to the think block.
	if logger := e.deps.Logger; logger != nil && name != "" {
		logger.Printf("[Function tool] Running the %s tool with: %s", name, renderToolArgs(args))
	}
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
		Reason:  ReasonNoDoc,
		Metrics: map[string]any{},
	}, nil
}

// renderToolArgs renders a tool's arguments for the "[Function tool]" think-log
// line. Python interpolated the raw args dict; Go marshals to JSON so nested
// values (scopes, tag maps) stay readable. Empty args render as "{}" to match
// Python's look rather than an empty string.
func renderToolArgs(args map[string]any) string {
	if len(args) == 0 {
		return "{}"
	}
	b, err := json.Marshal(args)
	if err != nil {
		return "{}"
	}
	return string(b)
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
	router := e.deps.NavRouter
	if router == nil {
		// Default navigation-tree router mirrors Python _NAV_TREE_ROUTER = "chunk_agg":
		// route documents by aggregating raw-chunk retrieval rather than by a nav-row
		// KNN descent (nav.NewNavServiceRouter). A nil retrieval backend degrades to the
		// nav-row router, which needs no backend.
		if e.deps.Backend != nil {
			router = nav.NewChunkAggRouter(
				chunkAggRetrieveFrom(e.deps.Backend),
				nav.ChunkAggSummarize(),
			)
		} else {
			router = nav.NewNavServiceRouter()
		}
	}
	// The tool argument is NOT threaded (Python _exec_navigate_tree,
	// action_session.py:_exec_navigate_tree, calls _navigate_tree_impl(query, keywords=...) with
	// no doc_scope), but the SESSION scope still applies: the impl routes through
	// _nav_search_titled, which ceilings its scope with tools.scoped_doc_ids(None)
	// — the session doc_scope (navigation.py:_nav_search_titled). The router must therefore
	// receive the session ceiling, never the tool argument.
	res := NavigateTree(ctx, router, NavTreeInput{
		Query:    query,
		Keywords: e.req.Keywords,
		DocScope: e.deps.DocScope,
		TenantID: e.deps.TenantID,
		KbIDs:    e.deps.KbIDs,
	})

	switch res.EmptyReason {
	case ReasonNoStructure:
		return ToolOutcome{
			Payload: []any{map[string]any{
				"kind":    "navigate_tree",
				"note":    "This dataset has no compiled document-navigation tree. Use search_chunks / retrieve instead.",
				"query":   query,
				"doc_ids": []string{},
			}},
			Status:  StatusEmpty,
			Reason:  ReasonNoStructure,
			Metrics: map[string]any{"docs": 0, "routed_docs": [][2]string{}},
		}, nil
	case ReasonNoDoc:
		// Structure exists, this query reached nothing — a MISS, not an EMPTY.
		return ToolOutcome{
			Payload: []any{map[string]any{
				"kind":    "navigate_tree",
				"content": res.Text,
				"doc_ids": []string{},
				"note":    "The navigation tree exists but this query routed to no document. Try a different topic/entity phrasing, or use search_chunks.",
				"query":   query,
			}},
			Status:  StatusMiss,
			Reason:  ReasonNoDoc,
			Metrics: map[string]any{"docs": 0},
		}, nil
	case ReasonInfra, ReasonBadArgs:
		return ToolOutcome{
			Payload: []any{},
			Status:  ReasonStatus(res.EmptyReason),
			Reason:  res.EmptyReason,
		}, nil
	}

	// Routed: expose the summary-bearing payload the ladder consumes.
	payload := []any{map[string]any{
		"kind":    "navigate_tree",
		"content": res.Text,
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
			return ToolOutcome{Payload: []any{}, Status: ReasonStatus(res.EmptyReason), Reason: res.EmptyReason}, nil
		}
		docIDs = res.DocIDs
		if len(docIDs) > navTreeMaxDocs {
			docIDs = docIDs[:navTreeMaxDocs]
		}
		if len(docIDs) == 0 {
			// Routing reached no document (Python: empty_reason="no_doc").
			return ToolOutcome{
				Payload: []any{map[string]any{
					"kind":    "navigate_structure",
					"note":    "The query routed to no document in this dataset. Try a different topic/entity phrasing, pass doc_id, or use search_chunks.",
					"query":   query,
					"doc_ids": []string{},
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
	}
	if res.EmptyReason != "" {
		return ToolOutcome{
			Payload: []any{map[string]any{
				"kind":    "navigate_structure",
				"note":    fmt.Sprintf("No compiled structure of kind=%q reachable for the located document(s). Try another kind, or use search_chunks / retrieve / list_chunks.", kind),
				"doc_ids": docIDs,
			}},
			Status:  ReasonStatus(res.EmptyReason),
			Reason:  res.EmptyReason,
			Metrics: metrics,
		}, nil
	}
	// Reached structures but drilled to nothing usable: not an error, just weak —
	// the orchestrator falls back to retrieval on status == poor.
	status := StatusOK
	if drill.chunkPtrs == 0 {
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
// share (mirrors Python's _NAV_TREE_ROUTER = "chunk_agg"; a nil retrieval backend
// degrades to the nav-row router, which needs no backend).
func (e *searchExecutor) navRouter() NavTreeRouter {
	if e.deps.NavRouter != nil {
		return e.deps.NavRouter
	}
	if e.deps.Backend != nil {
		return nav.NewChunkAggRouter(
			chunkAggRetrieveFrom(e.deps.Backend),
			nav.ChunkAggSummarize(),
		)
	}
	return nav.NewNavServiceRouter()
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
// (kbinfos["chunks"]). Mirrors Python _EVIDENCE_POOL_CAP (=60, the same ceiling
// as _MAX_SNIPPET_POOL / _SCA_VIEW_CAP): once the pool is saturated, further
// admits cannot reach the SCA view or improve the answer, so the action session
// stops admitting chunks (observed pools otherwise grew to ~106).
const evidencePoolCap = 60

// evidencePoolFullLogged mirrors Python _EVIDENCE_POOL_STATE["full_logged"], which
// Python documents as a PER-PROCESS flag (action_session.py:62-64: "Per-process
// flag so the 'pool FULL' log line is emitted once per fill, not once per
// rejected chunk"). So the line is emitted once per PROCESS — not once per
// rejected chunk, and NOT once per session: a second session in the same process
// stays silent, exactly as in Python. sync.Once because concurrent tool calls
// run the check. The pool never shrinks mid-session, so no reset is needed.
var evidencePoolFullLogged sync.Once

// evidencePoolFull mirrors the early-stop at the top of Python _admit_evidence:
// once the shared pool reaches evidencePoolCap, admit no further chunk and
// return true so the caller skips it.
func evidencePoolFull(pool *Kbinfos) bool {
	if pool == nil || len(pool.Chunks) < evidencePoolCap {
		return false
	}
	evidencePoolFullLogged.Do(func() {
		_LOG.Printf("[Action Session] evidence pool FULL (%d chunks >= cap %d); early-stopping admit of further chunks.", len(pool.Chunks), evidencePoolCap)
	})
	return true
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

	// Max queries per tool call mirrors Python action_session.execute_tool
	// (_arg_query_list): retrieve=3, search_chunks=2. grep_search/grep_chunks
	// are Go-internal tools (Python exposes no such session tool), so they are
	// left uncapped.
	maxQ := len(queries)
	switch name {
	case "retrieve":
		maxQ = 3
	case "search_chunks":
		maxQ = 2
	}
	if len(queries) > maxQ {
		queries = queries[:maxQ]
	}

	var payload []any
	var evidenceIDs []string
	newChunks := 0
	// Per-call admittance state, mirroring Python _run_search/_admit_evidence
	// (action_session.py:_run_search): `seen` dedups chunks ACROSS the queries of
	// this one call; kbSeen holds the shared pool's existing identities
	// (Python `kb_seen`).
	seen := map[string]bool{}
	kbSeen := make(map[string]bool, len(e.deps.KB.Chunks))
	for _, c := range e.deps.KB.Chunks {
		kbSeen[chunkKey(c)] = true
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
		// Only the first snippetsPerQuery (4) hits of each query are considered
		// (Python cands[:_SNIPPETS_PER_QUERY]).
		if len(chunks) > snippetsPerQuery {
			chunks = chunks[:snippetsPerQuery]
		}
		// Admittance mirrors _admit_evidence exactly: per-call dedup by chunk
		// id, the chunk ID as the evidence reference (Python's `ids` holds ids,
		// not pool positions), and only chunks NEW to the shared pool appended
		// to it — so REDUNDANT means "nothing new", not "nothing returned".
		for _, c := range chunks {
			// Python _admit_evidence early-stops at the top once the shared pool
			// reaches the cap, BEFORE the per-call dedup.
			if evidencePoolFull(e.deps.KB) {
				continue
			}
			cid := ChunkIDOf(c)
			if seen[cid] {
				continue
			}
			seen[cid] = true
			evidenceIDs = append(evidenceIDs, cid)
			payload = append(payload, passageFromChunk(c))
			// Pool identity uses chunkKey (Go's stable key): Python's _chunk_key
			// falls back to id(ck) — the dict's address — so an equivalent
			// re-retrieved chunk never matches and is appended again.
			key := chunkKey(c)
			if !kbSeen[key] {
				kbSeen[key] = true
				e.deps.KB.Chunks = append(e.deps.KB.Chunks, c)
				newChunks++
			}
		}
		// Pool this search's doc_aggs (skipped when it returned no chunks, like
		// _merge_kbinfos).
		e.deps.KB.MergeDocAggs(aggs)
	}

	if len(payload) == 0 {
		return ToolOutcome{
			Payload:     []any{},
			EvidenceIDs: nil,
			Status:      StatusMiss,
			Reason:      ReasonNoDoc,
			Metrics:     map[string]any{"hits": 0, "new_evidence": 0},
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
		}, nil
	}
	return ToolOutcome{
		Payload:     payload,
		EvidenceIDs: evidenceIDs,
		Status:      StatusOK,
		Reason:      ReasonNone,
		Metrics:     map[string]any{"hits": len(payload), "new_evidence": newChunks},
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
		SimilarityThreshold:      req.SimilarityThreshold,
		KeywordsSimilarityWeight: req.KeywordsSimilarityWeight,
		TenantID:                 req.TenantID,
		RankFeature:              req.RankFeature,
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
			"chunk_id":          c.ID,
			"content":           c.Content,
			"doc_id":            c.DocumentID,
			"docnm_kwd":         c.DocumentName,
			"dataset_id":        c.DatasetID,
			"kb_id":             c.DatasetID,
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
