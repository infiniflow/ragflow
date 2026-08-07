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

package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"

	"gorm.io/gorm"
	"ragflow/internal/agent/tool"
	"ragflow/internal/common"
	"ragflow/internal/service/nav"
	"ragflow/internal/service/wikisearch"
)

// ProductionRunner wires the real agentic-search tools (hybrid_search,
// dataset_navigation_by_tree, wiki_query) into the RunAgenticRAG flow, so the
// tools are actually invoked rather than merely registered. This is the
// production counterpart to the unit-testable SearchFn seam.
type ProductionRunner struct {
	db         *gorm.DB
	tenantID   string
	datasetIDs []string
	searchTool einotool.InvokableTool
	navSvc     nav.NavService // defaults to nav.GetNavService() when nil
	wikiSvc    wikisearch.Service
	// webTool is an optional, already-configured web search tool. When nil the
	// runner never exposes web fallback (P8: no web provider configured => the
	// agent does not attempt web search and no failing tool call is made).
	webTool einotool.InvokableTool
}

// NewProductionRunner builds a ProductionRunner backed by the real tools. The
// dataset-nav router (harness.NavigateDatasetByTree) resolves its NavService
// lazily via nav.GetNavService(). When a web provider is configured (a Tavily
// API key is present), the runner also wires the web fallback tool so
// high/ultra modes can fill an empty KB result from the web; otherwise no web
// tool is attached and no web call is ever attempted (P8/R2).
func NewProductionRunner(db *gorm.DB, tenantID string, datasetIDs []string) (*ProductionRunner, error) {
	searchBase, err := tool.BuildByName("hybrid_search", nil)
	if err != nil {
		return nil, err
	}
	search, ok := searchBase.(einotool.InvokableTool)
	if !ok {
		return nil, fmt.Errorf("hybrid_search is not invokable")
	}
	r := &ProductionRunner{db: db, tenantID: tenantID, datasetIDs: datasetIDs, searchTool: search}
	if common.GetEnv(common.EnvTavilyApiKey) != "" {
		r.webTool = tool.NewTavilyTool()
	}
	return r, nil
}

// newProductionRunnerWithTools builds a ProductionRunner with an injected
// search tool and nav service, for unit/E2E tests that want to fake the
// invocation surface without real services.
func newProductionRunnerWithTools(db *gorm.DB, tenantID string, datasetIDs []string, searchTool einotool.InvokableTool, navSvc nav.NavService) *ProductionRunner {
	return &ProductionRunner{db: db, tenantID: tenantID, datasetIDs: datasetIDs, searchTool: searchTool, navSvc: navSvc}
}

// Run executes the agentic-search graph with the real tools. It computes the
// route once and uses it to pick a search strategy: when the route suggests a
// wiki compilation and the bound KBs actually carry wiki artifacts, the runner
// tries wiki_query first and falls back to general hybrid search on an empty
// result; otherwise it uses hybrid search. Web fallback is only reachable when a
// web provider is configured (P8). Returns the final answer.
func (r *ProductionRunner) Run(ctx context.Context, question, keywords, modeLabel string) AnswerResult {
	if r.searchTool == nil {
		log.Printf("agentic_rag: production runner not fully wired (search tool missing)")
		return AnswerResult{FinalAnswer: emptyResultMessage, Empty: true}
	}
	route := RouteNode(ctx, r.db, question, modeLabel)

	// Base hybrid search, optionally scoped by the nav router for decomposition
	// modes.
	searchFn := r.hybridSearchFn(ctx, question, keywords, modeLabel)

	// P8/R4: web fallback is phase-gated — only wired for modes whose
	// AvailableTools actually include web_search (high/ultra), AND only when a
	// web provider is configured. Low/medium never trigger external web requests
	// from an empty KB result. Unconfigured => no web tool call is ever attempted.
	if modeAllowsWeb(modeLabel) {
		searchFn = r.webFallbackFn(searchFn)
	}

	// P5: prefer wiki when the route suggests it AND the bound KBs carry the
	// artifact; fall back to hybrid on empty/absent wiki results.
	if route.SuggestsCompilation == "wiki" && r.wikiAvailable(ctx) {
		searchFn = r.wikiPreferredSearchFn(searchFn)
	}
	return RunAgenticRAGWithRoute(ctx, r.db, question, keywords, modeLabel, route, searchFn)
}

// modeAllowsWeb reports whether the mode's AvailableTools include web_search, so
// web fallback is only reachable in the modes that are supposed to have it
// (high/ultra). Unknown modes are treated as not allowing web.
func modeAllowsWeb(modeLabel string) bool {
	mode, ok := GetMode(modeLabel)
	if !ok {
		return false
	}
	for _, name := range mode.AvailableTools {
		if name == "web_search" {
			return true
		}
	}
	return false
}

// hybridSearchFn builds the base hybrid search closure (optionally doc-scoped
// for decomposition modes).
func (r *ProductionRunner) hybridSearchFn(ctx context.Context, question, keywords, modeLabel string) SearchFn {
	searchFn := func(ctx context.Context, query, kws string) ([]map[string]interface{}, []map[string]interface{}) {
		return r.search(ctx, query, kws, nil)
	}
	if mode, _ := GetMode(modeLabel); mode.RequiresDecomposition {
		docs := r.routeDocs(ctx, question, keywords)
		if len(docs) > 0 {
			searchFn = func(ctx context.Context, query, kws string) ([]map[string]interface{}, []map[string]interface{}) {
				return r.search(ctx, query, kws, docs)
			}
		}
	}
	return searchFn
}

// wikiPreferredSearchFn wraps the hybrid searchFn so that each search first asks
// the compiled wiki for the query and only falls back to hybrid when the wiki
// returns nothing (or the wiki backend is unavailable). This is the P5 route
// consumption: a wiki suggestion selects the wiki path without discarding the
// hybrid fallback.
func (r *ProductionRunner) wikiPreferredSearchFn(hybrid SearchFn) SearchFn {
	return func(ctx context.Context, query, kws string) ([]map[string]interface{}, []map[string]interface{}) {
		chunks, aggs := r.wikiSearch(ctx, query, kws)
		if len(chunks) > 0 {
			return chunks, aggs
		}
		return hybrid(ctx, query, kws)
	}
}

// wikiAvailable reports whether the bound datasets carry searchable wiki
// artifacts, so the runner only selects the wiki path when it can actually serve.
func (r *ProductionRunner) wikiAvailable(ctx context.Context) bool {
	ws := r.wikiSvc
	if ws == nil {
		ws = wikisearch.GetService()
	}
	if ws == nil {
		return false
	}
	return ws.AvailableFor(ctx, r.tenantID, r.datasetIDs)
}

// wikiSearch invokes the wiki_query tool against the compiled wiki. It returns
// empty chunks (never a hard error) when the service is unavailable or yields
// nothing, so the caller falls back to hybrid search.
func (r *ProductionRunner) wikiSearch(ctx context.Context, query, keywords string) ([]map[string]interface{}, []map[string]interface{}) {
	ws := r.wikiSvc
	if ws == nil {
		ws = wikisearch.GetService()
	}
	if ws == nil || !ws.AvailableFor(ctx, r.tenantID, r.datasetIDs) {
		return nil, nil
	}
	res, err := ws.QueryPages(ctx, r.tenantID, r.datasetIDs, query, keywords, 12)
	if err != nil || len(res.Chunks) == 0 {
		return nil, nil
	}
	// P7: backfill the original source chunks referenced by the compiled page
	// hits, deduped and bounded, so the answer can cite raw evidence.
	return r.expandCompiledEvidence(ctx, res.Chunks, res.DocAggs)
}

// search invokes the hybrid_search tool and normalizes its chunk output.
func (r *ProductionRunner) search(ctx context.Context, query, keywords string, docScope []string) ([]map[string]interface{}, []map[string]interface{}) {
	args := map[string]interface{}{"query": query, "keywords": keywords, "kb_ids": r.datasetIDs, "top_n": 12}
	if len(docScope) > 0 {
		args["doc_scope"] = docScope
	}
	raw, err := r.searchTool.InvokableRun(ctx, mustJSON(args))
	if err != nil {
		log.Printf("agentic_rag: hybrid_search failed: %v", err)
		return nil, nil
	}
	var res struct {
		Chunks []map[string]interface{} `json:"chunks"`
	}
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		return nil, nil
	}
	return res.Chunks, nil
}

// expandCompiledEvidence backfills the ORIGINAL source chunks a compiled-page
// hit was built from (P7/R3). It collects the page hits' source_chunk_ids
// (bounded per page and in total), then asks the concrete wiki service to fetch
// them BY ID — scoped to the tenant + datasets — so the answer can cite raw
// evidence. When the page hits carry no source ids, the service is unavailable,
// or none of the ids resolve, the page results are kept as-is (safe degradation;
// nothing is fabricated).
func (r *ProductionRunner) expandCompiledEvidence(ctx context.Context, chunks, aggs []map[string]interface{}) ([]map[string]interface{}, []map[string]interface{}) {
	if len(chunks) == 0 {
		return chunks, aggs
	}
	ws := r.wikiSvc
	if ws == nil {
		ws = wikisearch.GetService()
	}
	if ws == nil {
		return chunks, aggs
	}
	const maxEvidencePerPage = 4
	const maxEvidenceTotal = 12

	// Collect bounded source-chunk ids from the page hits (deduped, in page
	// order), grouped by dataset so the backfill stays within each KB's scope.
	var sourceIDs []string
	seen := map[string]bool{}
	datasets := map[string]bool{}
	for _, c := range chunks {
		if len(sourceIDs) >= maxEvidenceTotal {
			break
		}
		ids := stringSlice(c["source_chunk_ids"])
		count := 0
		for _, id := range ids {
			if count >= maxEvidencePerPage {
				break
			}
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			count++
			sourceIDs = append(sourceIDs, id)
			if ds := stringValue(c["dataset_id"]); ds != "" {
				datasets[ds] = true
			}
			if len(sourceIDs) >= maxEvidenceTotal {
				break
			}
		}
	}
	if len(sourceIDs) == 0 {
		return chunks, aggs
	}
	// Scope the backfill to the page hits' datasets (fall back to all bound
	// datasets when the page hits carry none). Build a fresh slice: never mutate
	// r.datasetIDs.
	scope := make([]string, 0, len(r.datasetIDs))
	if len(datasets) == 0 {
		scope = append(scope, r.datasetIDs...)
	} else {
		for ds := range datasets {
			scope = append(scope, ds)
		}
	}
	evidence, err := ws.BackfillChunks(ctx, r.tenantID, scope, sourceIDs)
	if err != nil || len(evidence) == 0 {
		return chunks, aggs
	}

	// Stable merge: page results first (in retrieval order), then the backfilled
	// evidence, deduped by chunk key.
	merged := append([]map[string]interface{}(nil), chunks...)
	keys := map[string]bool{}
	for _, c := range chunks {
		if k := chunkKey(c); k != "" {
			keys[k] = true
		}
	}
	for _, e := range evidence {
		k := chunkKey(e)
		if k != "" && !keys[k] {
			keys[k] = true
			merged = append(merged, e)
		}
	}
	// Doc aggs: union the page doc aggs with the evidence docs.
	dseen := map[string]bool{}
	for _, d := range aggs {
		if id, _ := d["doc_id"].(string); id != "" {
			dseen[id] = true
		}
	}
	for _, e := range evidence {
		id := stringValue(e["doc_id"])
		if id == "" {
			continue
		}
		if !dseen[id] {
			dseen[id] = true
			aggs = append(aggs, map[string]interface{}{"doc_id": id, "doc_name": stringValue(e["docnm_kwd"])})
		}
	}
	return merged, aggs
}

// webFallbackFn wraps a SearchFn so that, when the KB search returns nothing, a
// configured web provider is invoked to fill the gap (P8). It is only used when
// webTool is non-nil; otherwise it returns the hybrid path unchanged and no web
// tool call is ever attempted (no failing call when unconfigured).
func (r *ProductionRunner) webFallbackFn(hybrid SearchFn) SearchFn {
	if r.webTool == nil {
		return hybrid
	}
	return func(ctx context.Context, query, kws string) ([]map[string]interface{}, []map[string]interface{}) {
		chunks, aggs := hybrid(ctx, query, kws)
		if len(chunks) > 0 {
			return chunks, aggs
		}
		raw, err := r.webTool.InvokableRun(ctx, mustJSON(map[string]interface{}{"query": query, "keywords": kws}))
		if err != nil {
			return nil, nil
		}
		var res struct {
			Chunks  []map[string]interface{} `json:"chunks"`
			Results []map[string]interface{} `json:"results"`
		}
		if err := json.Unmarshal([]byte(raw), &res); err != nil {
			return nil, nil
		}
		// Normalize web evidence into the same agentic evidence shape as KB
		// chunks. Accept both the agent "chunks" envelope and the Tavily
		// "results" envelope (tavily.go returns {"results":[...]}); each result
		// contributes content + a doc_id reference so the answer can retain the
		// source URL.
		src := res.Chunks
		if len(src) == 0 {
			src = res.Results
		}
		out := make([]map[string]interface{}, 0, len(src))
		for _, c := range src {
			url := firstNonEmpty(stringValue(c["url"]), stringValue(c["link"]), stringValue(c["source"]))
			if url == "" {
				continue
			}
			content := firstNonEmpty(stringValue(c["content"]), stringValue(c["raw_content"]), stringValue(c["text"]))
			if content == "" {
				continue
			}
			docID := stringValue(c["doc_id"])
			if docID == "" {
				docID = url + "|" + stringValue(c["source"])
			}
			out = append(out, map[string]interface{}{
				"chunk_id": docID, "content_with_weight": content,
				"doc_id": docID, "docnm_kwd": firstNonEmpty(stringValue(c["title"]), stringValue(c["source"])),
				"dataset_id": stringValue(c["dataset_id"]), "url": url, "source": "web",
			})
		}
		if len(out) == 0 {
			return nil, nil
		}
		return out, nil
	}
}

func stringValue(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func stringSlice(v interface{}) []string {
	if raw, ok := v.([]string); ok {
		return raw
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// routeDocs derives the doc scope via the canonical dataset-nav router
// (harness.NavigateDatasetByTree — the full LLM two-round selection). It routes
// across ALL bound datasets and merges the doc ids, so every KB contributes its
// own relevant docs to the shared scope (a multi-KB session must not collapse to
// the first KB only).
func (r *ProductionRunner) routeDocs(ctx context.Context, topic, keywords string) []string {
	ns := r.navSvc
	if ns == nil {
		ns = nav.GetNavService()
	}
	if ns == nil {
		log.Printf("agentic_rag: dataset nav service not initialized; skipping doc routing")
		return nil
	}
	// Combine topic + keywords into the routing query so the nav router actually
	// uses the full user signal (keywords must not be dropped).
	query := strings.TrimSpace(topic + " " + keywords)
	seen := map[string]bool{}
	var docs []string
	for _, kbID := range r.datasetIDs {
		for _, id := range NavigateDatasetByTree(ctx, r.db, ns, r.tenantID, kbID, query) {
			if id != "" && !seen[id] {
				seen[id] = true
				docs = append(docs, id)
			}
		}
	}
	return docs
}

func mustJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
