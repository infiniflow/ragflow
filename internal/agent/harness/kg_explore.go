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
	"sort"
	"strings"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/agent/chat"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/service"
)

// graph_explore (mirrors Python harness/tools/navigation.py graph_explore): walk
// the compiled knowledge graph. Unlike ontology_navigate/mindmap_navigate (which
// read the single merged "graph" JSON), the KG store keeps one searchable row per
// entity/relation, so we *search* our way to a small subgraph: dense-seed
// entities by the question, hop _KG_HOPS out over relations, then ask the chat
// model whether that subgraph answers the question.
//
// Seeds use the tenant embedding model via service.NavEmbedder (dense KNN,
// similarity>=kgSeedSim, re-ranked by mention_count_int desc); when the embedding
// model is unavailable it degrades to keyword match (mirrors Python _kg_search's
// `embed_mdl is None` path).

const (
	kgScopeDataset = "dataset"
	kgScopeDoc     = "doc"

	kgSeeds     = 2   // top-N entities matched directly to the question
	kgSeedPool  = 64  // KNN candidate pool before the mention_count_int re-sort
	kgSeedSim   = 0.8 // dense seed similarity floor (Python _KG_SEED_SIM)
	kgHops      = 2   // relation hops out from the seeds
	kgNeighbors = 128 // cap on neighbour entity rows resolved per hop
	kgRelLimit  = 32  // relations fetched per endpoint filter
)

type kgEntity struct {
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	Description    string   `json:"description"`
	Aliases        []string `json:"aliases"`
	SourceChunkIDs []string `json:"source_chunk_ids"`
	DocID          string   `json:"doc_id"`
}

type kgRelation struct {
	From           string   `json:"from"`
	To             string   `json:"to"`
	Type           string   `json:"type"`
	SourceChunkIDs []string `json:"source_chunk_ids"`
	DocID          string   `json:"doc_id"`
}

// ExploreResult is the graph_explore output: exactly one of Answer / Chunks is
// populated.
type ExploreResult struct {
	Answer string
	Chunks []map[string]interface{}
}

// ExploreGraph implements graph_explore.
func ExploreGraph(ctx context.Context, tenantID string, datasetIDs []string, query, keywords string, docScope []string) (ExploreResult, error) {
	empty := ExploreResult{}
	text := strings.TrimSpace(query + " " + keywords)
	if text == "" || len(datasetIDs) == 0 {
		return empty, nil
	}
	de := engine.Get()
	if de == nil {
		return empty, fmt.Errorf("graph_explore: engine not configured")
	}

	scopeKwd := kgScopeDataset
	if len(docScope) > 0 {
		scopeKwd = kgScopeDoc
	}

	var entities []kgEntity
	var relations []kgRelation
	seenNames := map[string]bool{}

	addEntities := func(new []kgEntity, scopeKey string) []string {
		var added []string
		for _, e := range new {
			key := scopeKey + ":" + strings.ToLower(e.Name)
			if seenNames[key] {
				continue
			}
			seenNames[key] = true
			entities = append(entities, e)
			added = append(added, e.Name)
		}
		return added
	}

	// Encode the seed text ONCE (not per dataset): a single embedding request
	// serves every KB. A nil vector means the embedding model is unavailable and
	// the seed search falls back to keyword match.
	seedVec := encodeSeedVector(ctx, tenantID, text)

	for _, kbID := range datasetIDs {
		// (1) Seeds: dense KNN (similarity>=_KG_SEED_SIM) over the scoped entity
		// rows, re-ranked by mention_count_int desc; falls back to keyword match
		// when the embedding model is unavailable.
		seedRows := kgSeedSearch(ctx, de, tenantID, kbID, docScope, text, scopeKwd, seedVec)
		var seeds []kgEntity
		for _, r := range seedRows {
			if e, ok := kgParseEntity(r); ok {
				seeds = append(seeds, e)
			}
		}
		frontier := addEntities(seeds, kbID)

		// (2) Expand kgHops out, collecting relations and neighbour entities.
		for hop := 0; hop < kgHops; hop++ {
			if len(frontier) == 0 {
				break
			}
			terms := endpointTerms(frontier)
			relRows := kgSearch(ctx, de, tenantID, kbID, docScope, "relation", "", kgRelLimit, scopeKwd,
				map[string]interface{}{"from_entity_kwd": terms}, "", 0)
			relRows = append(relRows, kgSearch(ctx, de, tenantID, kbID, docScope, "relation", "", kgRelLimit, scopeKwd,
				map[string]interface{}{"to_entity_kwd": terms}, "", 0)...)
			seenRel := map[string]bool{}
			var hopRelations []kgRelation
			for _, r := range relRows {
				rel, ok := kgParseRelation(r)
				if !ok {
					continue
				}
				k := rel.From + "|" + rel.To + "|" + rel.Type
				if seenRel[k] {
					continue
				}
				seenRel[k] = true
				hopRelations = append(hopRelations, rel)
			}
			relations = append(relations, hopRelations...)

			// Neighbour entity names not yet visited.
			seenLower := map[string]bool{}
			for k := range seenNames {
				if strings.HasPrefix(k, kbID+":") {
					seenLower[strings.TrimPrefix(k, kbID+":")] = true
				}
			}
			neighSet := map[string]string{}
			for _, r := range hopRelations {
				for _, n := range []string{r.From, r.To} {
					n = strings.TrimSpace(n)
					if n == "" || seenLower[strings.ToLower(n)] {
						continue
					}
					neighSet[strings.ToLower(n)] = n
				}
			}
			if len(neighSet) == 0 {
				break
			}
			neighFiltered := make([]string, 0, len(neighSet))
			for _, n := range neighSet {
				neighFiltered = append(neighFiltered, n)
			}
			limit := kgNeighbors
			if len(neighFiltered) < limit {
				limit = len(neighFiltered)
			}
			neighRows := kgSearch(ctx, de, tenantID, kbID, docScope, "entity", "", limit, scopeKwd,
				map[string]interface{}{"name_kwd": endpointTerms(neighFiltered)}, "", 0)
			var neighbours []kgEntity
			for _, r := range neighRows {
				if e, ok := kgParseEntity(r); ok {
					neighbours = append(neighbours, e)
				}
			}
			frontier = addEntities(neighbours, kbID)
		}
	}

	if len(entities) == 0 && len(relations) == 0 {
		return empty, nil
	}

	// (3) Does the subgraph answer the question?
	answer, relevant := askStructureAnswer(ctx, query, entities, relations)
	// (4a) Sufficient — return the answer, no chunks.
	if answer != "" {
		return ExploreResult{Answer: answer}, nil
	}

	// (4b) Insufficient — return source passages behind the relevant nodes.
	evidence := collectEvidenceIDs(entities, relations, relevant)
	var chunks []map[string]interface{}
	for docID, ids := range evidence {
		if docID != "" && len(ids) > 0 {
			chunks = append(chunks, loadChunksByIDs(ctx, tenantID, ids)...)
		}
	}
	return ExploreResult{Chunks: chunks}, nil
}

// encodeSeedVector encodes the seed text once for the whole ExploreGraph call.
// Returns nil when the tenant embedding model is unavailable (or encoding fails).
func encodeSeedVector(ctx context.Context, tenantID, text string) []float64 {
	embedder := service.NewNavEmbedder(service.NewModelProviderService(), "")
	vecs, err := embedder.Encode(ctx, tenantID, []string{text})
	if err != nil || len(vecs) == 0 || len(vecs[0]) == 0 {
		return nil
	}
	vec := make([]float64, len(vecs[0]))
	for i, v := range vecs[0] {
		vec[i] = float64(v)
	}
	return vec
}

// kgSeedSearch searches the compiled KG entity rows for seeds (mirrors Python
// _kg_search dense branch): dense KNN over name_kwd with similarity>=0.8,
// re-ranked by mention_count_int desc, top kgSeeds. Falls back to keyword match
// when seedVec is nil (embedding model unavailable).
func kgSeedSearch(ctx context.Context, de engine.DocEngine, tenantID, kbID string, docIDs []string, text, scopeKwd string, seedVec []float64) []map[string]interface{} {
	if seedVec != nil {
		dense := &types.MatchDenseExpr{
			VectorColumnName:  fmt.Sprintf("q_%d_vec", len(seedVec)),
			EmbeddingData:     seedVec,
			EmbeddingDataType: "float",
			DistanceType:      "cosine",
			TopN:              kgSeedPool,
			ExtraOptions:      map[string]interface{}{"similarity": kgSeedSim},
		}
		rows := kgSearchRaw(ctx, de, tenantID, kbID, docIDs, "entity", scopeKwd, nil, []interface{}{dense}, "mention_count_int", kgSeedPool)
		return topMentionCount(rows, kgSeeds)
	}
	// Text fallback (mirrors Python _kg_search `embed_mdl is None` path).
	return kgSearch(ctx, de, tenantID, kbID, docIDs, "entity", text, kgSeeds, scopeKwd, nil, "mention_count_int", kgSeedPool)
}

// topMentionCount re-ranks rows by mention_count_int desc and returns topN.
func topMentionCount(rows []map[string]interface{}, topN int) []map[string]interface{} {
	sort.SliceStable(rows, func(i, j int) bool {
		return mentionCount(rows[i]) > mentionCount(rows[j])
	})
	if len(rows) > topN {
		rows = rows[:topN]
	}
	return rows
}

func mentionCount(row map[string]interface{}) int {
	switch v := row["mention_count_int"].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	}
	return 0
}

// kgSearchRaw is the low-level KG row search with explicit match exprs and any
// extra filter keys (e.g. from_entity_kwd/to_entity_kwd/name_kwd).
func kgSearchRaw(ctx context.Context, de engine.DocEngine, tenantID, kbID string, docIDs []string, kind, scopeKwd string, extra map[string]interface{}, matchExprs []interface{}, orderDesc string, limit int) []map[string]interface{} {
	idx := fmt.Sprintf("ragflow_%s", tenantID)
	condition := map[string]interface{}{"knowledge_graph_kwd": kind}
	if scopeKwd != "" {
		condition["scope_kwd"] = scopeKwd
	}
	if len(docIDs) > 0 {
		condition["doc_id"] = docIDs
	}
	for k, v := range extra {
		condition[k] = v
	}
	fields := []string{"content_with_weight", "source_chunk_ids", "doc_id", "docnm_kwd", "name_kwd", "mention_count_int", "from_entity_kwd", "to_entity_kwd"}
	req := &types.SearchRequest{
		IndexNames:   []string{idx},
		KbIDs:        []string{kbID},
		SelectFields: fields,
		Filter:       condition,
		Limit:        limit,
		MatchExprs:   matchExprs,
	}
	if orderDesc != "" {
		req.OrderBy = &types.OrderByExpr{}
		req.OrderBy.Desc(orderDesc)
	}
	res, err := de.Search(ctx, req)
	if err != nil {
		return nil
	}
	return res.Chunks
}

// kgSearch searches the compiled KG rows of one KB (mirrors Python _kg_search),
// using keyword match. It only builds the MatchTextExpr (with the pool-based
// TopN) and delegates the request construction to kgSearchRaw.
func kgSearch(ctx context.Context, de engine.DocEngine, tenantID, kbID string, docIDs []string, kind, text string, topN int, scopeKwd string, extra map[string]interface{}, orderDesc string, pool int) []map[string]interface{} {
	var matchExprs []interface{}
	if text != "" {
		knnTopN := topN
		if pool > knnTopN {
			knnTopN = pool
		}
		matchExprs = []interface{}{&types.MatchTextExpr{
			Fields:       []string{"content_ltks", "content_sm_ltks"},
			MatchingText: text,
			TopN:         knnTopN,
		}}
	}
	return kgSearchRaw(ctx, de, tenantID, kbID, docIDs, kind, scopeKwd, extra, matchExprs, orderDesc, topN)
}

func kgParseEntity(row map[string]interface{}) (kgEntity, bool) {
	name := ""
	payload := map[string]interface{}{}
	if s, ok := row["content_with_weight"].(string); ok {
		_ = json.Unmarshal([]byte(s), &payload)
	}
	if v, ok := payload["name"].(string); ok && v != "" {
		name = v
	} else if v, ok := payload["term"].(string); ok && v != "" {
		name = v
	} else if v, ok := payload["title"].(string); ok && v != "" {
		name = v
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return kgEntity{}, false
	}
	e := kgEntity{
		Name:           name,
		Type:           strOr(payload["type"], "other"),
		Description:    strOr(payload["description"], ""),
		SourceChunkIDs: strSliceField(row["source_chunk_ids"]),
		DocID:          strOr(row["doc_id"], ""),
	}
	if aliases, ok := payload["aliases"].([]interface{}); ok {
		for _, a := range aliases {
			if s, ok := a.(string); ok && strings.TrimSpace(s) != "" {
				e.Aliases = append(e.Aliases, strings.TrimSpace(s))
			}
		}
	}
	return e, true
}

func kgParseRelation(row map[string]interface{}) (kgRelation, bool) {
	src := strings.TrimSpace(strOr(row["from_entity_kwd"], ""))
	tgt := strings.TrimSpace(strOr(row["to_entity_kwd"], ""))
	if src == "" || tgt == "" {
		return kgRelation{}, false
	}
	typ := "related"
	if payload, ok := row["content_with_weight"].(string); ok {
		var p map[string]interface{}
		if json.Unmarshal([]byte(payload), &p) == nil {
			if t, ok := p["type"].(string); ok && t != "" {
				typ = t
			} else if t, ok := p["relation"].(string); ok && t != "" {
				typ = t
			}
		}
	}
	return kgRelation{
		From: src, To: tgt, Type: typ,
		SourceChunkIDs: strSliceField(row["source_chunk_ids"]),
		DocID:          strOr(row["doc_id"], ""),
	}, true
}

// endpointTerms mirrors Python _endpoint_terms: original + lowercased forms, so
// hop queries match both merged (lowercased) and per-doc (original-case)
// endpoint fields.
func endpointTerms(names []string) []string {
	set := map[string]bool{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		set[n] = true
		set[strings.ToLower(n)] = true
	}
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// collectEvidenceIDs mirrors Python _collect_evidence_ids: group source chunk ids
// of relevant entities AND relations by doc.
func collectEvidenceIDs(entities []kgEntity, relations []kgRelation, relevantNames []string) map[string][]string {
	wanted := map[string]bool{}
	for _, n := range relevantNames {
		if s := strings.TrimSpace(n); s != "" {
			wanted[strings.ToLower(s)] = true
		}
	}
	byDoc := map[string][]string{}
	seen := map[string]bool{}
	add := func(docID string, ids []string) {
		for _, cid := range ids {
			if cid == "" {
				continue
			}
			key := docID + "|" + cid
			if seen[key] {
				continue
			}
			seen[key] = true
			byDoc[docID] = append(byDoc[docID], cid)
		}
	}
	for _, e := range entities {
		names := map[string]bool{strings.ToLower(e.Name): true}
		for _, a := range e.Aliases {
			names[strings.ToLower(a)] = true
		}
		if intersects(names, wanted) {
			add(e.DocID, e.SourceChunkIDs)
		}
	}
	for _, r := range relations {
		if wanted[strings.ToLower(r.From)] || wanted[strings.ToLower(r.To)] {
			add(r.DocID, r.SourceChunkIDs)
		}
	}
	return byDoc
}

func intersects(a, b map[string]bool) bool {
	for k := range a {
		if b[k] {
			return true
		}
	}
	return false
}

// askStructureAnswer asks the chat model whether the subgraph answers the query
// (mirrors Python _ask_structure), returning (answer, relevant_names).
func askStructureAnswer(ctx context.Context, query string, entities []kgEntity, relations []kgRelation) (string, []string) {
	inv := chat.GetDefaultInvoker()
	if inv == nil {
		return "", nil
	}
	rendered := renderKGSubgraph(entities, relations)
	resp, err := inv.Invoke(ctx, nil, chat.Request{
		Messages: []schema.Message{
			{Role: schema.System, Content: strings.ReplaceAll(navSystemPrompt, "{noun}", "knowledge graph")},
			{Role: schema.User, Content: fmt.Sprintf("Question:\n%s\n\nKnowledge graph:\n%s\n\nOutput JSON:", query, rendered)},
		},
	})
	if err != nil {
		return "", nil
	}
	var v structureNavVerdict
	if err := unmarshalModelJSON(resp.Content, &v); err != nil {
		return "", nil
	}
	answer := ""
	if v.IsSufficient {
		answer = strings.TrimSpace(v.Answer)
	}
	return answer, v.RelevantEntities
}

func renderKGSubgraph(entities []kgEntity, relations []kgRelation) string {
	var b strings.Builder
	b.WriteString("Entities:")
	for i, e := range entities {
		if i >= maxStructureEntities {
			break
		}
		b.WriteString("\n- " + e.Name + " (" + orStr(e.Type, "other") + ")")
		if d := strings.Join(strings.Fields(e.Description), " "); d != "" {
			b.WriteString(": " + d)
		}
	}
	b.WriteString("\n\nRelations:")
	for i, r := range relations {
		if i >= maxStructureRelations {
			break
		}
		b.WriteString("\n- " + r.From + " -[" + r.Type + "]-> " + r.To)
	}
	return b.String()
}

func strOr(v interface{}, def string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return def
}

func strSliceField(v interface{}) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []interface{}:
		var out []string
		for _, item := range x {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
