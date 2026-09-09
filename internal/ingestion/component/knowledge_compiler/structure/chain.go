package structure

import (
	"context"
	"fmt"
	"strings"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// Chain-shape validation for ``list`` / ``timeline`` kinds, mirroring the
// Python structure.py block (CHAIN_KINDS, _chain_detect_violations,
// validate_and_correct_chain). Both kinds model a strict linear chain of
// entities (one predecessor, one successor, no cycles). The per-chunk
// extractor is happy to emit branches / cycles when the source text supports
// multiple readings, so the relation set is validated post-extraction and the
// LLM is asked to pick the correct chain out of the offenders. Correction is
// best-effort: any failure returns the input untouched (fail-open).

// ChainKinds mirrors CHAIN_KINDS.
var ChainKinds = map[Type]bool{TypeList: true, Type("timeline"): true}

const (
	chainCorrectionMaxChunkChars = 8196
	chainCorrectionMaxChunks     = 12
	chainCorrectionMaxRelations  = 16
)

// chainCorrectionPrompt mirrors CHAIN_CORRECTION_PROMPT verbatim ({} placeholders
// are filled with {kind, bad_relations_json, source_chunks_text}).
const chainCorrectionPrompt = `You are correcting an extracted {kind}-kind structure.

Constraint: the relations must form a strict linear chain — every entity has
at most one predecessor and at most one successor, and there must be no
cycle. The relations below were flagged by an automated detector as
violating this constraint. Each one carries the issue that was detected.

Bad relations (review and keep only those supported by the source text):
{bad_relations_json}

Source chunks the relations were extracted from:
{source_chunks_text}

Your task: from the bad relations above, pick the subset that should be
kept. Drop the rest. Do not invent new relations. Use only ` + "``from`` and ``to``" + ` slugs that appear verbatim in the bad-relations list. The result
must satisfy the strict-chain constraint.

Return ONLY a JSON object with this exact shape (no markdown fences, no
commentary):
{
  "keep": [
    {"from": "<slug>", "to": "<slug>"},
    ...
  ]
}
`

// chainCorrectionSystem is the system prompt of the correction call (Python
// passes it inline to gen_json).
const chainCorrectionSystem = "You correct extracted graph relations to satisfy a strict-chain constraint."

// chainJudgeTemperature mirrors the correction call's gen_conf (temperature 0.0).
var chainJudgeTemperature = 0.0

type chainEdge struct{ From, To string }

// chainExtractEdge mirrors _chain_extract_edge: the relation row's endpoint
// pair, from the authoritative meta columns with a payload fallback.
func chainExtractEdge(row common.Product) (chainEdge, bool) {
	if kind, _ := row.Meta["kind"].(string); kind != "relation" {
		return chainEdge{}, false
	}
	from, _ := row.Meta["from"].(string)
	to, _ := row.Meta["to"].(string)
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if from != "" && to != "" {
		return chainEdge{from, to}, true
	}
	payload := parsePayload(row.Content)
	if payload == nil {
		return chainEdge{}, false
	}
	from = relationEndpoint(payload, "", "source", "src", "from")
	to = relationEndpoint(payload, "", "target", "tgt", "to")
	if from == "" || to == "" {
		return chainEdge{}, false
	}
	return chainEdge{from, to}, true
}

// chainDetectViolations mirrors _chain_detect_violations: it returns
// {edge: [issue strings]} for every edge involved in a self-loop, fan-out,
// fan-in, or a directed cycle (SCC size >= 2). Cycle detection uses iterative
// Kosaraju — no recursion, so the pathologically-deep case Python guards with
// RecursionError cannot occur here.
func chainDetectViolations(edges []chainEdge) map[chainEdge][]string {
	issues := map[chainEdge][]string{}
	add := func(e chainEdge, reason string) {
		issues[e] = append(issues[e], reason)
	}

	outGroups := map[string][]chainEdge{}
	inGroups := map[string][]chainEdge{}
	for _, e := range edges {
		if e.From == e.To {
			add(e, "self-loop")
		}
		outGroups[e.From] = append(outGroups[e.From], e)
		inGroups[e.To] = append(inGroups[e.To], e)
	}
	for node, group := range outGroups {
		if len(group) > 1 {
			siblings := sortedStrings(func() []string {
				var out []string
				for _, g := range group {
					out = append(out, g.To)
				}
				return out
			}())
			reason := fmt.Sprintf("fan-out from '%s' (also points to %s)", node, siblings)
			for _, e := range group {
				add(e, reason)
			}
		}
	}
	for node, group := range inGroups {
		if len(group) > 1 {
			siblings := sortedStrings(func() []string {
				var out []string
				for _, g := range group {
					out = append(out, g.From)
				}
				return out
			}())
			reason := fmt.Sprintf("fan-in to '%s' (also reached from %s)", node, siblings)
			for _, e := range group {
				add(e, reason)
			}
		}
	}

	for _, comp := range chainSCCs(edges) {
		for _, e := range edges {
			if comp[e.From] && comp[e.To] {
				add(e, fmt.Sprintf("cycle within %s", sortedKeys(comp)))
			}
		}
	}
	return issues
}

// chainSCCs returns the strongly connected components of size >= 2 (each a
// directed cycle), computed with iterative Kosaraju.
func chainSCCs(edges []chainEdge) []map[string]bool {
	adj := map[string][]string{}
	radj := map[string][]string{}
	nodes := map[string]bool{}
	for _, e := range edges {
		nodes[e.From] = true
		nodes[e.To] = true
		adj[e.From] = append(adj[e.From], e.To)
		radj[e.To] = append(radj[e.To], e.From)
	}

	// Pass 1: finishing order on the forward graph (iterative DFS).
	visited := map[string]bool{}
	var order []string
	for start := range nodes {
		if visited[start] {
			continue
		}
		type frame struct {
			node string
			next int
		}
		stack := []frame{{node: start}}
		visited[start] = true
		for len(stack) > 0 {
			top := &stack[len(stack)-1]
			if top.next < len(adj[top.node]) {
				w := adj[top.node][top.next]
				top.next++
				if !visited[w] {
					visited[w] = true
					stack = append(stack, frame{node: w})
				}
				continue
			}
			order = append(order, top.node)
			stack = stack[:len(stack)-1]
		}
	}

	// Pass 2: DFS on the transposed graph in reverse finishing order.
	assigned := map[string]bool{}
	var sccs []map[string]bool
	for i := len(order) - 1; i >= 0; i-- {
		root := order[i]
		if assigned[root] {
			continue
		}
		comp := map[string]bool{}
		stack := []string{root}
		assigned[root] = true
		for len(stack) > 0 {
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			comp[v] = true
			for _, w := range radj[v] {
				if !assigned[w] {
					assigned[w] = true
					stack = append(stack, w)
				}
			}
		}
		if len(comp) >= 2 {
			sccs = append(sccs, comp)
		}
	}
	return sccs
}

// chainGatherChunkText mirrors _chain_gather_chunk_text: deduplicated
// (chunk_id, text) pairs for the correction prompt, capped.
func chainGatherChunkText(badRows []common.Product, chunksByID map[string]string) [][2]string {
	seen := map[string]bool{}
	var out [][2]string
	for _, row := range badRows {
		for _, cid := range metaStrings(row.Meta, "source_chunk_ids") {
			if cid == "" || seen[cid] {
				continue
			}
			seen[cid] = true
			text := strings.TrimSpace(chunksByID[cid])
			if text == "" {
				continue
			}
			if len(text) > chainCorrectionMaxChunkChars {
				text = text[:chainCorrectionMaxChunkChars]
			}
			out = append(out, [2]string{cid, text})
			if len(out) >= chainCorrectionMaxChunks {
				return out
			}
		}
	}
	return out
}

// chainCorrectBatch mirrors correct_batch: one LLM correction over a batch of
// bad edges. It fails open — a failed or malformed call retains the batch's
// relations. The returned set holds the edges to keep.
func chainCorrectBatch(ctx context.Context, deps common.Deps, llmID string, kind Type, batchEdges []chainEdge, edgeToRows map[chainEdge][]common.Product, violations map[chainEdge][]string, chunksByID map[string]string) []chainEdge {
	batchKeep := append([]chainEdge{}, batchEdges...)

	var badRows []common.Product
	relations := make([]map[string]any, 0, len(batchEdges))
	for _, e := range batchEdges {
		issue := "cross-batch conflict"
		if iss := violations[e]; len(iss) > 0 {
			issue = strings.Join(iss, "; ")
		}
		relations = append(relations, map[string]any{"from": e.From, "to": e.To, "issue": issue})
		badRows = append(badRows, edgeToRows[e]...)
	}
	chunkPairs := chainGatherChunkText(badRows, chunksByID)
	var sourceText strings.Builder
	for _, pair := range chunkPairs {
		fmt.Fprintf(&sourceText, "[%s]\n%s\n\n", pair[0], pair[1])
	}
	if sourceText.Len() == 0 {
		sourceText.WriteString("(no source chunks available)")
	}

	user := chainCorrectionPrompt
	user = strings.ReplaceAll(user, "{kind}", string(kind))
	user = strings.ReplaceAll(user, "{bad_relations_json}", mustJSONList(relations))
	user = strings.ReplaceAll(user, "{source_chunks_text}", strings.TrimSpace(sourceText.String()))

	res, err := common.GenJSON(ctx, deps.Chat, common.ChatRequest{
		LLMID: llmID, SystemPrompt: chainCorrectionSystem, UserPrompt: user, Temperature: &chainJudgeTemperature,
	})
	if err != nil {
		return batchKeep // fail open
	}
	keepRaw, ok := res["keep"].([]any)
	if !ok {
		return batchKeep
	}
	batchSet := make(map[chainEdge]bool, len(batchEdges))
	for _, e := range batchEdges {
		batchSet[e] = true
	}
	var kept []chainEdge
	for _, item := range keepRaw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		from, _ := m["from"].(string)
		to, _ := m["to"].(string)
		e := chainEdge{strings.TrimSpace(from), strings.TrimSpace(to)}
		if batchSet[e] {
			kept = append(kept, e)
		}
	}
	return kept
}

// validateAndCorrectChain mirrors validate_and_correct_chain: relations of
// chain-kind rows must form a strict linear chain; offending relations the
// LLM does not keep are dropped from the returned rows. Any failure returns
// the input verbatim (fail-open), so a misbehaving model can never block the
// pipeline.
func validateAndCorrectChain(ctx context.Context, deps common.Deps, llmID string, rows []common.Product, chunksByID map[string]string, kind Type) []common.Product {
	if len(rows) == 0 || !ChainKinds[kind] {
		return rows
	}

	edgeToRows := map[chainEdge][]common.Product{}
	var allEdges []chainEdge
	for _, row := range rows {
		e, ok := chainExtractEdge(row)
		if !ok {
			continue
		}
		if _, seen := edgeToRows[e]; !seen {
			allEdges = append(allEdges, e)
		}
		edgeToRows[e] = append(edgeToRows[e], row)
	}
	violations := chainDetectViolations(allEdges)
	if len(violations) == 0 {
		return rows
	}
	badEdges := make([]chainEdge, 0, len(violations))
	for e := range violations {
		badEdges = append(badEdges, e)
	}

	// Correct in capped batches (sequential: deterministic, and in-run
	// violation counts are small — Python parallelises under a semaphore).
	keepSet := map[chainEdge]bool{}
	for i := 0; i < len(badEdges); i += chainCorrectionMaxRelations {
		end := i + chainCorrectionMaxRelations
		if end > len(badEdges) {
			end = len(badEdges)
		}
		for _, e := range chainCorrectBatch(ctx, deps, llmID, kind, badEdges[i:end], edgeToRows, violations, chunksByID) {
			keepSet[e] = true
		}
	}

	// Independent corrections can be valid inside each request but conflict
	// after their results are combined. Re-check the combined keep set and
	// give the model one final decision over the remaining conflicts.
	var keepList []chainEdge
	for e := range keepSet {
		keepList = append(keepList, e)
	}
	if combined := chainDetectViolations(keepList); len(combined) > 0 {
		conflictEdges := make([]chainEdge, 0, len(combined))
		for e := range combined {
			conflictEdges = append(conflictEdges, e)
		}
		finalKeep := chainCorrectBatch(ctx, deps, llmID, kind, conflictEdges, edgeToRows, combined, chunksByID)
		for _, e := range conflictEdges {
			delete(keepSet, e)
		}
		for _, e := range finalKeep {
			keepSet[e] = true
		}
	}

	// LLM kept everything → no correction applied.
	if len(keepSet) == len(badEdges) {
		allKept := true
		for _, e := range badEdges {
			if !keepSet[e] {
				allKept = false
				break
			}
		}
		if allKept {
			return rows
		}
	}

	droppedIDs := map[string]bool{}
	for _, e := range badEdges {
		if keepSet[e] {
			continue
		}
		for _, row := range edgeToRows[e] {
			if row.ID != "" {
				droppedIDs[row.ID] = true
			}
		}
	}
	if len(droppedIDs) == 0 {
		return rows
	}
	corrected := make([]common.Product, 0, len(rows)-len(droppedIDs))
	for _, row := range rows {
		if !droppedIDs[row.ID] {
			corrected = append(corrected, row)
		}
	}
	return corrected
}

// dropIsolatedTimelineEntities mirrors cleanup_timeline_isolated_entities:
// for the timeline kind, entity rows not referenced by any relation endpoint
// are dropped. The in-memory port runs it after all merges/rewrites (the
// Python ES version schedules it after every flush for the same reason: a
// later relation can still reference the entity).
func dropIsolatedTimelineEntities(rows []common.Product) []common.Product {
	connected := map[string]bool{}
	for _, row := range rows {
		if e, ok := chainExtractEdge(row); ok {
			connected[strings.ToLower(e.From)] = true
			connected[strings.ToLower(e.To)] = true
		}
	}
	out := make([]common.Product, 0, len(rows))
	for _, row := range rows {
		if kind, _ := row.Meta["kind"].(string); kind == "entity" {
			if !connected[strings.ToLower(entityNameValue(row))] {
				continue
			}
		}
		out = append(out, row)
	}
	return out
}

func sortedStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return sortedStrings(out)
}

func mustJSONList(items []map[string]any) string {
	var b strings.Builder
	b.WriteString("[")
	for i, item := range items {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(payloadJSON(item))
	}
	b.WriteString("]")
	return b.String()
}
