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

// LLM-guided navigation-tree routing, the Go port of the tree-walk half of
// Python rag/advanced_rag/harness/tools/navigation.py:
//
//	_NAV_SELECT_SYSTEM / _ask_nav_select   (:396-467)
//	_collect_nav_leaves                    (:470-506)
//	_nav_cluster_names                     (:509-511)
//	_content_recall_docs                   (:514-565)
//	dataset_navigation_by_tree             (:568-659)
//
// Python routes through the compiled nav tree two ways: the tool
// `_navigate_tree_impl` runs the flat hybrid sweep (`_nav_search_titled`), and
// `dataset_navigation_by_tree` walks the tree with the chat model (select
// clusters → descend to leaves → select documents), falling back to chunk
// content recall at every miss. The Go NavigateTree historically had only the
// sweep; this file adds the LLM walk and the content-recall fallback behind
// optional seams so both Python routing semantics exist on one entry point.
package harness

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/agent/chat"
)

// Tree-walk router tunables (Python :382-385).
const (
	// navMaxClusters is _NAV_MAX_CLUSTERS: top-level clusters listed / rendered
	// to the LLM.
	navMaxClusters = 500
	// navChildrenPageSize is _NAV_CHILDREN_PAGE_SIZE: children fetched per node.
	navChildrenPageSize = 1000
	// navTreeMaxDepth is _NAV_TREE_MAX_DEPTH: BFS depth cap when descending
	// sub-clusters to leaves.
	navTreeMaxDepth = 6
	// navTreeMaxLeaves is _NAV_TREE_MAX_LEAVES: document leaves rendered to the
	// doc-select LLM.
	navTreeMaxLeaves = 300
)

// Chunk-level content recall (fallback) tunables (Python :387-394).
//
// The nav tree routes by *cluster summaries*; a question that matches a detail
// only present in a document's body (not its one-line summary) can fall through
// the tree. We therefore back the tree result with a plain chunk retrieval and
// fold the documents it hits back in as a recall fallback — reusing the existing
// chunk index, so no new compilation artifact is required.
const (
	// navRecallTopN is _NAV_RECALL_TOP_N: chunk candidates fetched before doc
	// aggregation.
	navRecallTopN = 40
	// navRecallMaxDocs is _NAV_RECALL_MAX_DOCS: extra docs the content recall
	// may add on top of the tree.
	navRecallMaxDocs = 4
	// navRecallMinScore is the retrieval similarity_threshold Python passes
	// (:548) — a literal 0.2, not the session's configured threshold.
	navRecallMinScore = 0.2
	// navRecallVectorWeight is the hybrid vector weight Python hardcodes when
	// an embedder exists (:539): `vector_weight = 0.3 if embd_mdl else 0`.
	navRecallVectorWeight = 0.3
)

// navWalkMaxDocs is Python _NAV_MAX_DOCS (:378): documents the nav tree routes
// a query to.
const navWalkMaxDocs = 8

// navSelectTemperature is the sampling temperature Python passes to async_chat
// (:442): `{"temperature": 0.2}`.
const navSelectTemperature = 0.2

// navSelectSystemTemplate mirrors Python _NAV_SELECT_SYSTEM (:396-408) with
// str.format's {noun} placeholder and doubled braces intact; navSelectSystem
// applies the format.
const navSelectSystemTemplate = `You are routing a question through a dataset's navigation tree.

You are given a QUESTION and a numbered list of {noun}, each with a name and a short description.
Choose the {noun} most likely to contain information relevant to answering the question.

Rules:
1. Judge only from the names and descriptions shown.
2. Be selective — include an item only if it is plausibly relevant. Include several when several are equally plausible.
3. If none are clearly relevant, return an empty list.
4. Return the bracketed index numbers of the chosen {noun}.

Output ONLY JSON, no prose, no code fences:
{{"relevant": [<index>, ...]}}`

// navSelectSystem mirrors `_NAV_SELECT_SYSTEM.format(noun=noun)`: substitute
// {noun}, then unescape the doubled braces of the literal JSON example.
func navSelectSystem(noun string) string {
	s := strings.ReplaceAll(navSelectSystemTemplate, "{noun}", noun)
	s = strings.ReplaceAll(s, "{{", "{")
	return strings.ReplaceAll(s, "}}", "}")
}

// navFenceRe mirrors Python :446 `re.sub(r"```(?:json)?\s*|\s*```", "", cleaned)`:
// every fence marker is removed, wherever it appears.
var navFenceRe = regexp.MustCompile("```(?:json)?\\s*|\\s*```")

// NavTreeBrowser browses a dataset's compiled navigation tree, mirroring the
// two dataset_api_service calls the Python walk makes:
//
//	list_nav_clusters(kb.id, kb.tenant_id, page=1, page_size=…)
//	list_nav_children(kb.id, kb.tenant_id, name, page=1, page_size=…)
//
// Items are rendered in Python's _nav_item shape (dataset_api_service.py:3416):
// name / description / keywords / entities / doc_count / type ("cluster" |
// "doc") / doc_id / has_children. The production adapter lives in
// internal/service/nav (the harness must not own the service singleton).
type NavTreeBrowser interface {
	ListNavClusters(ctx context.Context, tenantID, kbID string, pageSize int) ([]map[string]any, error)
	ListNavChildren(ctx context.Context, tenantID, kbID, name string, pageSize int) ([]map[string]any, error)
}

// navItemStr coerces a nav-item field to a string the way Python's str() does
// (None → "", non-strings through str()).
func navItemStr(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	default:
		return fmt.Sprint(x)
	}
}

// navItemInt coerces a nav-item field to an int the way Python's int() does in
// the paths the walk uses (doc_count truthiness, index parsing): numbers
// truncate, digit strings parse, everything else fails.
func navItemInt(v any) (int, bool) {
	switch x := v.(type) {
	case nil:
		return 0, false
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(x))
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

// pyCapitalize mirrors Python str.capitalize: first character upper, the REST
// lower ("CLUSTERS" → "Clusters").
func pyCapitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + strings.ToLower(string(r[1:]))
}

// AskNavSelect mirrors Python _ask_nav_select (:411-467): ask the chat model
// which of items are relevant to query.
//
// Items are rendered as a numbered list of name (+ optional doc_count) and
// description; the model returns the chosen indices. Index-based (not id-based)
// so the model never has to reproduce opaque doc ids or exact names. Returns
// the selected item dicts (a subset of items), or nil when the model declines
// or the call fails.
func AskNavSelect(ctx context.Context, model SessionModel, query string, items []map[string]any, noun string, maxItems int) []map[string]any {
	if len(items) == 0 || model == nil {
		return nil
	}
	capped := items
	if maxItems > 0 && maxItems < len(items) {
		capped = items[:maxItems]
	}
	lines := make([]string, 0, len(capped))
	for i, it := range capped {
		name := strings.TrimSpace(navItemStr(it["name"]))
		if name == "" {
			name = fmt.Sprintf("item-%d", i)
		}
		desc := strings.ReplaceAll(strings.TrimSpace(navItemStr(it["description"])), "\n", " ")
		extra := ""
		// Python :429 — `if it.get("doc_count")`: any truthy count renders.
		if dc, ok := navItemInt(it["doc_count"]); ok && dc != 0 {
			extra = fmt.Sprintf(" [%d docs]", dc)
		}
		head := ""
		// Python :430-435 — the first six keywords / entities, joined, render
		// as bracketed tags only when non-empty.
		if tags := pyJoinPrefix(it["keywords"], 6); tags != "" {
			head += " [tags: " + tags + "]"
		}
		if ents := pyJoinPrefix(it["entities"], 6); ents != "" {
			head += " [entities: " + ents + "]"
		}
		// Python :436 — the description is cut to 300 CHARACTERS (code points).
		lines = append(lines, fmt.Sprintf("[%d] %s%s%s: %s", i, name, extra, head, runeCut(desc, 300)))
	}

	system := navSelectSystem(noun)
	user := fmt.Sprintf("Question:\n%s\n\n%s (numbered):\n%s\n\nOutput JSON:", query, pyCapitalize(noun), strings.Join(lines, "\n"))

	// Python :441 — message_fit_in(form_message(system, user), max_length).
	budget := 0
	if cl, ok := model.(ContextLengthModel); ok {
		budget = cl.ContextLength()
	}
	if budget <= 0 {
		budget = chat.EffectiveContextLength(0)
	}
	fitted, fitErr := chat.FitMessages(system, []schema.Message{
		*schema.UserMessage(user),
	}, budget)
	if fitErr != "" {
		_LOG.Printf("[Dataset navigation] prompt fitting failed: %s", fitErr)
		return nil
	}

	// Python :442 — async_chat(..., {"temperature": 0.2}); the Go seam exposes
	// per-call temperature only on models implementing TemperatureModel.
	var (
		reply *ModelReply
		err   error
	)
	if tm, ok := model.(TemperatureModel); ok {
		reply, err = tm.CompleteWithTemperature(ctx, fitted, nil, navSelectTemperature)
	} else {
		reply, err = model.Complete(ctx, fitted, nil)
	}
	if err != nil {
		_LOG.Printf("[Dataset navigation] LLM %s selection failed: %v", noun, err)
		return nil
	}

	// Python :445-447 — strip the think preamble and EVERY fence marker, then
	// json_repair.loads (or {} on failure).
	cleaned := reThinkWrap.ReplaceAllString(reply.Content, "")
	cleaned = navFenceRe.ReplaceAllString(cleaned, "")
	cleaned = strings.TrimSpace(cleaned)
	verdict := repairJSONWhole(cleaned)
	data, ok := verdict.(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := data["relevant"].([]any)
	if !ok {
		return nil
	}

	out := make([]map[string]any, 0, len(raw))
	seenIdx := map[int]bool{}
	for _, r := range raw {
		idx, ok := navItemInt(r)
		if !ok {
			continue
		}
		if 0 <= idx && idx < len(capped) && !seenIdx[idx] {
			seenIdx[idx] = true
			out = append(out, capped[idx])
		}
	}
	return out
}

// pyJoinPrefix mirrors Python's `", ".join(str(k) for k in kwds[:6]).strip()`
// over a list-valued nav-item field; an absent / non-list field yields "".
func pyJoinPrefix(v any, n int) string {
	list, ok := v.([]any)
	if !ok {
		return ""
	}
	if n > 0 && n < len(list) {
		list = list[:n]
	}
	parts := make([]string, 0, len(list))
	for _, e := range list {
		parts = append(parts, navItemStr(e))
	}
	return strings.TrimSpace(strings.Join(parts, ", "))
}

// runeCut cuts s to at most n code points (Python's s[:n]).
func runeCut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// CollectNavLeaves mirrors Python _collect_nav_leaves (:470-506): BFS from the
// selected clusters down to their document leaves.
//
// Each cluster carries name + kb_id (Python tags {**item, "kb": kb} at listing
// time). A node's children are either document leaves (type == "doc" —
// collected, deduped by doc_id) or sub-clusters (descended, depth- and
// count-capped). docScope (when non-empty) restricts the collected leaves.
func CollectNavLeaves(ctx context.Context, source NavTreeBrowser, tenantID string, clusters []map[string]any, docScope []string) []map[string]any {
	leaves := []map[string]any{}
	seenDocs := map[string]bool{}
	type node struct {
		kbID  string
		name  string
		depth int
	}
	seenNodes := map[string]bool{}
	frontier := make([]node, 0, len(clusters))
	for _, c := range clusters {
		// Python :480 — clusters without a name are skipped.
		if name := strings.TrimSpace(navItemStr(c["name"])); name != "" {
			frontier = append(frontier, node{kbID: navItemStr(c["kb_id"]), name: name})
		}
	}
	allowed := map[string]bool{}
	for _, d := range docScope {
		if d != "" {
			allowed[d] = true
		}
	}

	for len(frontier) > 0 && len(leaves) < navTreeMaxLeaves {
		n := frontier[0]
		frontier = frontier[1:]
		nodeKey := n.kbID + "\x00" + n.name
		if seenNodes[nodeKey] {
			continue
		}
		seenNodes[nodeKey] = true
		items, err := source.ListNavChildren(ctx, tenantID, n.kbID, n.name, navChildrenPageSize)
		if err != nil {
			_LOG.Printf("[Dataset navigation] list_nav_children failed for kb=%s node=%s: %v", n.kbID, n.name, err)
			continue
		}
		for _, item := range items {
			switch navItemStr(item["type"]) {
			case "doc":
				did := strings.TrimSpace(navItemStr(item["doc_id"]))
				if did == "" {
					continue
				}
				// Python :499 — an empty scope admits every doc.
				if len(allowed) > 0 && !allowed[did] {
					continue
				}
				if seenDocs[did] {
					continue
				}
				seenDocs[did] = true
				leaves = append(leaves, item)
				if len(leaves) >= navTreeMaxLeaves {
					break
				}
			case "cluster":
				if name := navItemStr(item["name"]); name != "" && n.depth+1 < navTreeMaxDepth {
					frontier = append(frontier, node{kbID: n.kbID, name: name, depth: n.depth + 1})
				}
			}
		}
	}
	return leaves
}

// NavClusterNames mirrors Python _nav_cluster_names (:509-511): the selected
// clusters' names joined for log lines, "none" when there are none.
func NavClusterNames(clusters []map[string]any) string {
	names := make([]string, 0, len(clusters))
	for _, c := range clusters {
		if n := strings.TrimSpace(navItemStr(c["name"])); n != "" {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

// ContentRecallDocs mirrors Python _content_recall_docs (:514-565): fallback
// doc discovery by chunk CONTENT, aggregated to docs.
//
// Runs a plain hybrid retrieval over the bound KBs' chunk index and returns the
// doc_ids of the hit documents, most-hit-first. This is the safety net for the
// nav-tree router: a question that matches detail living only in a document's
// body (not its cluster summary) never appears in the tree, so this retrieval —
// which reads real chunk text — is what catches it.
//
// docScope (the already-effective routed doc scope) is forwarded as doc_ids to
// the retrieval so content recall stays restricted to the same documents the
// tree routed to, instead of re-opening the whole KB.
//
// Returns nil on failure or when nothing hits. hasEmbedder is Python's
// `embd_mdl` presence: it picks the hybrid blend (0.3) vs keyword-only.
func ContentRecallDocs(ctx context.Context, backend Retriever, query string, docScope []string, tenantID string, kbIDs []string, hasEmbedder bool) []string {
	// Python :529-535 — no query / no bound KBs: nothing to recall.
	if strings.TrimSpace(query) == "" || backend == nil || len(kbIDs) == 0 {
		return nil
	}
	// Python :548 — a literal 0.2 similarity threshold.
	threshold := navRecallMinScore
	req := RetrieveRequest{
		Query:               query,
		DatasetIDs:          kbIDs,
		DocScope:            docScope,
		TopN:                navRecallTopN, // Python :547 — _NAV_RECALL_TOP_N.
		SimilarityThreshold: &threshold,
		TenantID:            tenantID,
	}
	// Python :538-539 — 0.3 hybrid when an embedder exists, keyword-only
	// otherwise (DisableVectorLeg mirrors embd_mdl=None: no dense leg at all).
	if hasEmbedder {
		w := navRecallVectorWeight
		req.VectorSimilarityWeight = &w
	} else {
		req.DisableVectorLeg = true
	}
	chunks, err := backend.Retrieve(ctx, req)
	if err != nil {
		_LOG.Printf("[Dataset navigation] content-recall retrieval failed: %v", err)
		return nil
	}
	// Python :557-563 — the doc_aggs of the retrieval, read in order. The ES
	// aggregation orders by hit count descending; the Go Retriever returns
	// flat chunks, so the same order is derived here.
	order := make([]string, 0, 8)
	counts := map[string]int{}
	for _, c := range chunks {
		did := strings.TrimSpace(navItemStr(c["doc_id"]))
		if did == "" {
			continue
		}
		if _, ok := counts[did]; !ok {
			order = append(order, did)
		}
		counts[did]++
	}
	sort.SliceStable(order, func(i, j int) bool { return counts[order[i]] > counts[order[j]] })
	_LOG.Printf("[Dataset navigation] Content recall found %d candidate doc(s).", len(order))
	return order
}

// navWalkDeps carries the LLM walk's seams (Python reaches them through the
// RAGTools instance + dataset_api_service).
type navWalkDeps struct {
	Model       SessionModel
	Source      NavTreeBrowser
	Backend     Retriever // nil disables the content-recall fallback tiers
	HasEmbedder bool
	TenantID    string
	KbIDs       []string
	DocScope    []string
}

// recall caps the content-recall fallback the way Python does at every miss:
// `(await _content_recall_docs(...))[:_NAV_MAX_DOCS]`.
func (w navWalkDeps) recall(ctx context.Context, query string) [][2]string {
	ids := ContentRecallDocs(ctx, w.Backend, query, w.DocScope, w.TenantID, w.KbIDs, w.HasEmbedder)
	if len(ids) > navWalkMaxDocs {
		ids = ids[:navWalkMaxDocs]
	}
	out := make([][2]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, [2]string{id, ""})
	}
	return out
}

// RouteByTreeLLM mirrors Python dataset_navigation_by_tree (:568-659): return
// the doc_ids most relevant to the question by walking the dataset nav tree
// with the chat model.
//
// Two LLM passes narrow the corpus coarse-to-fine:
//
//  1. List the top-level clusters across the bound KBs.
//  2. Ask the model which clusters are relevant to the question.
//  3. Descend those clusters to their document leaves.
//  4. Ask the model which documents are worth reading.
//  5. Back the routed set with the content-recall fallback.
//
// Returns the routed (doc_id, summary) pairs — summaries come from the leaf's
// description when present, "" for recall-added docs. Every miss falls back to
// content recall exactly where Python's does.
func RouteByTreeLLM(ctx context.Context, w navWalkDeps, query string) [][2]string {
	_LOG.Printf(`[Dataset navigation] Walking the dataset tree for "%s"`, query)

	// 1. List every top-level cluster across the bound KBs (tagged with its KB;
	// Python :597-609).
	clusters := []map[string]any{}
	for _, kbID := range w.KbIDs {
		items, err := w.Source.ListNavClusters(ctx, w.TenantID, kbID, navMaxClusters)
		if err != nil {
			_LOG.Printf("[Dataset navigation] list_nav_clusters failed for kb=%s: %v", kbID, err)
			continue
		}
		for _, item := range items {
			if navItemStr(item["type"]) == "cluster" && navItemStr(item["name"]) != "" {
				item["kb_id"] = kbID // Python {**item, "kb": kb}
				clusters = append(clusters, item)
			}
		}
	}

	// 2. No cluster at all → content recall (Python :611-613).
	if len(clusters) == 0 {
		_LOG.Printf("[Dataset navigation] no cluster there — falling back to content recall.")
		return w.recall(ctx, query)
	}

	// 3. Ask the model which clusters are relevant (Python :615-620).
	selectedClusters := AskNavSelect(ctx, w.Model, query, clusters, "clusters", navMaxClusters)
	if len(selectedClusters) == 0 {
		_LOG.Printf("[Dataset navigation] no cluster found — falling back to content recall.")
		return w.recall(ctx, query)
	}
	_LOG.Printf("[Dataset navigation] %d/%d cluster(s) selected.", len(selectedClusters), len(clusters))

	// 4. Descend the selected clusters to their document leaves (:622-626).
	leaves := CollectNavLeaves(ctx, w.Source, w.TenantID, selectedClusters, w.DocScope)
	if len(leaves) == 0 {
		_LOG.Printf("[Dataset navigation] no leaf under selected cluster %s — falling back to content recall.", NavClusterNames(selectedClusters))
		return w.recall(ctx, query)
	}

	// 5. Ask the model which documents to look into (:628-632).
	selectedDocs := AskNavSelect(ctx, w.Model, query, leaves, "documents", navTreeMaxLeaves)
	if len(selectedDocs) == 0 {
		_LOG.Printf("[Dataset navigation] no doc selected under cluster %s — falling back to content recall.", NavClusterNames(selectedClusters))
		return w.recall(ctx, query)
	}

	// Dedup the routed doc ids, tree order preserved (Python :634-641).
	routed := make([][2]string, 0, len(selectedDocs))
	seenDocs := map[string]bool{}
	for _, d := range selectedDocs {
		did := strings.TrimSpace(navItemStr(d["doc_id"]))
		if did == "" || seenDocs[did] {
			continue
		}
		seenDocs[did] = true
		// The nav leaf carries the document's overall summary (description),
		// so the route is labelled for free.
		routed = append(routed, [2]string{did, strings.TrimSpace(navItemStr(d["description"]))})
	}
	_LOG.Printf("[Dataset navigation] Routed to %d document(s).", len(routed))

	// 6. Content-recall fallback (Python :643-658). The tree routes only by
	// cluster summaries, so a question matching a detail present only in a
	// document's body can fall through it. Back the routed set with a plain
	// chunk retrieval: docs that actually hit by content are folded back in
	// (deduped, tree docs first) so a missed document still reaches the caller.
	// Skip the extra retrieval when the tree already filled the whole cap.
	if len(routed) < navWalkMaxDocs && w.Backend != nil {
		fallback := ContentRecallDocs(ctx, w.Backend, query, w.DocScope, w.TenantID, w.KbIDs, w.HasEmbedder)
		added := make([]string, 0, len(fallback))
		for _, did := range fallback {
			if !seenDocs[did] {
				added = append(added, did)
			}
		}
		if len(added) > navRecallMaxDocs {
			added = added[:navRecallMaxDocs]
		}
		if len(added) > 0 {
			for _, did := range added {
				routed = append(routed, [2]string{did, ""})
			}
			_LOG.Printf("[Dataset navigation] Content recall added %d fallback doc(s) on top of the %d tree-routed one(s).",
				len(added), len(routed)-len(added))
		}
	}
	if len(routed) > navWalkMaxDocs {
		routed = routed[:navWalkMaxDocs]
	}
	return routed
}
