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
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"ragflow/internal/engine"
	enginetypes "ragflow/internal/engine/types"
)

// Knowledge-graph walk backing the graph_explore tool. It is the agentic-RAG
// port of the harness walk (internal/rag/agentic-rag/runtime/tool_exploration.go):
// seed entity rows for the query, hop out over relation rows, and hand the
// subgraph plus the source passages behind it back to the model.
//
// Deliberate deviation from the harness original: there the walk asks the chat
// model whether the subgraph answers the question and short-circuits with a
// bare answer when it does. This port never calls a model. In this ReAct loop
// the model is already the judge of its own evidence, a tool holds no chat
// model, and an answer-only result would strip the passages the answer has to
// cite. The walk therefore always returns the subgraph and its passages.

// Error returned when no KgService was installed at boot.
var ErrKgServiceMissing = errors.New(
	"kg service not registered — call agentic_rag.SetKgService(...) at boot",
)

// Kg row scopes and walk limits, mirroring the harness constants.
const (
	kgScopeDataset = "dataset"
	kgScopeDoc     = "doc"

	kgSeeds        = 2   // top-N entities matched directly to the question
	kgSeedPool     = 64  // KNN candidate pool before the mention_count re-sort
	kgSeedSim      = 0.8 // dense seed similarity floor
	kgHops         = 2   // relation hops out from the seeds
	kgNeighbors    = 128 // cap on neighbour entity rows resolved per hop
	kgRelLimit     = 32  // relations fetched per endpoint filter
	kgMaxPassages  = 6   // source passages returned per walk
	kgPassageRunes = 1500
)

// KgEntity is one entity row of the compiled graph.
type KgEntity struct {
	Name           string
	Type           string
	Description    string
	Aliases        []string
	SourceChunkIDs []string
	DocID          string
}

// KgRelation is one relation (edge) row of the compiled graph.
type KgRelation struct {
	ID             string
	From           string
	To             string
	Type           string
	SourceChunkIDs []string
	DocID          string
}

// KgPassage is one source chunk behind a graph node or edge.
type KgPassage struct {
	ChunkID   string
	DocID     string
	DocName   string
	DatasetID string
	Content   string
}

// KgExploreRequest is one graph walk request. DatasetIDs is the conversation's
// server-bound scope; DocScope, when non-empty, pins the walk to those docs.
type KgExploreRequest struct {
	TenantID   string
	DatasetIDs []string
	DocScope   []string
	Query      string
}

// KgExploreResult is the walk output: the seeded + hopped subgraph and the
// passages behind it, in first-seen order.
type KgExploreResult struct {
	Entities  []KgEntity
	Relations []KgRelation
	Passages  []KgPassage
}

// KgService walks the compiled knowledge graph. The tools resolve it per call
// so the server can install the engine-backed adapter at boot.
type KgService interface {
	Explore(ctx context.Context, req KgExploreRequest) (KgExploreResult, error)
}

var (
	kgServiceMu   sync.RWMutex
	kgServiceImpl KgService = stubKgService{}
)

// SetKgService installs the graph-walk service. nil restores the stub.
func SetKgService(svc KgService) {
	kgServiceMu.Lock()
	defer kgServiceMu.Unlock()
	if svc == nil {
		kgServiceImpl = stubKgService{}
		return
	}
	kgServiceImpl = svc
}

// GetKgService returns the installed graph-walk service (a failing stub when
// none was installed).
func GetKgService() KgService {
	kgServiceMu.RLock()
	defer kgServiceMu.RUnlock()
	return kgServiceImpl
}

type stubKgService struct{}

func (stubKgService) Explore(context.Context, KgExploreRequest) (KgExploreResult, error) {
	return KgExploreResult{}, ErrKgServiceMissing
}

// KgAdapter walks the compiled graph rows through the document engine. It is
// stateless and safe to share across goroutines.
type KgAdapter struct {
	docEngine engine.DocEngine
	// seedEncoder embeds the query for the dense seed search. nil (or a nil
	// vector) degrades the seed search to a keyword match on the entity rows,
	// so the walk still works on a deployment without a tenant embedder.
	seedEncoder func(ctx context.Context, tenantID, text string) []float64
}

// NewKgAdapter wraps a doc engine behind the KgService interface.
func NewKgAdapter(docEngine engine.DocEngine) *KgAdapter {
	return &KgAdapter{docEngine: docEngine}
}

// SetSeedEncoder installs the query-side embedder used by the dense seed
// search. Pass nil to fall back to keyword seeding.
func (a *KgAdapter) SetSeedEncoder(fn func(ctx context.Context, tenantID, text string) []float64) {
	if a == nil {
		return
	}
	a.seedEncoder = fn
}

// Explore seeds entities for the query, hops kgHops out over their relations,
// and returns the subgraph with the passages behind it.
func (a *KgAdapter) Explore(ctx context.Context, req KgExploreRequest) (KgExploreResult, error) {
	empty := KgExploreResult{}
	if a == nil || a.docEngine == nil {
		return empty, ErrKgServiceMissing
	}
	query := strings.TrimSpace(req.Query)
	datasetIDs := nonEmptyStrings(req.DatasetIDs)
	if query == "" || len(datasetIDs) == 0 {
		return empty, nil
	}

	// Encode the seed text ONCE: one embedding request serves every dataset.
	seedVec := a.seedVector(ctx, req.TenantID, query)

	var entities []KgEntity
	var relations []KgRelation
	seenNames := map[string]bool{}

	addEntities := func(found []KgEntity, kbID string) []string {
		var added []string
		for _, e := range found {
			key := kbID + ":" + strings.ToLower(e.Name)
			if seenNames[key] {
				continue
			}
			seenNames[key] = true
			entities = append(entities, e)
			added = append(added, e.Name)
		}
		return added
	}

	docs := nonEmptyStrings(req.DocScope)
	// Whole-dataset walk: no scope filter. The compacted rows of this corpus
	// live at scope_kwd=doc, so filtering by scope_kwd=dataset would read the
	// graph as empty; the harness's dataset default is not carried over.
	scopeKwd := ""
	if len(docs) > 0 {
		scopeKwd = kgScopeDoc
	}

	for _, kbID := range datasetIDs {
		seedRows := a.seedSearch(ctx, req.TenantID, kbID, docs, query, scopeKwd, seedVec)
		var seeds []KgEntity
		for _, row := range seedRows {
			if e, ok := kgParseEntity(row); ok {
				seeds = append(seeds, e)
			}
		}
		frontier := addEntities(seeds, kbID)

		for hop := 0; hop < kgHops; hop++ {
			if len(frontier) == 0 {
				break
			}
			terms := kgEndpointTerms(frontier)
			relRows := a.kgSearch(ctx, req.TenantID, kbID, docs, "relation", "", kgRelLimit, scopeKwd,
				map[string]interface{}{"from_entity_kwd": terms}, "", 0)
			relRows = append(relRows, a.kgSearch(ctx, req.TenantID, kbID, docs, "relation", "", kgRelLimit, scopeKwd,
				map[string]interface{}{"to_entity_kwd": terms}, "", 0)...)

			// Dedup on the row id so the same endpoint pair from different rows
			// survives; fall back to the from|to|type triple when the engine
			// returned no id.
			seenRel := map[string]bool{}
			var hopRelations []KgRelation
			for _, row := range relRows {
				rel, ok := kgParseRelation(row)
				if !ok {
					continue
				}
				key := rel.ID
				if key == "" {
					key = rel.From + "|" + rel.To + "|" + rel.Type
				}
				if seenRel[key] {
					continue
				}
				seenRel[key] = true
				hopRelations = append(hopRelations, rel)
			}
			relations = append(relations, hopRelations...)

			seenLower := map[string]bool{}
			for key := range seenNames {
				if strings.HasPrefix(key, kbID+":") {
					seenLower[strings.TrimPrefix(key, kbID+":")] = true
				}
			}
			neighSet := map[string]string{}
			for _, rel := range hopRelations {
				for _, name := range []string{rel.From, rel.To} {
					name = strings.TrimSpace(name)
					if name == "" || seenLower[strings.ToLower(name)] {
						continue
					}
					neighSet[strings.ToLower(name)] = name
				}
			}
			if len(neighSet) == 0 {
				break
			}
			neighbours := make([]string, 0, len(neighSet))
			for _, name := range neighSet {
				neighbours = append(neighbours, name)
			}
			limit := kgNeighbors
			if len(neighbours) < limit {
				limit = len(neighbours)
			}
			neighRows := a.kgSearch(ctx, req.TenantID, kbID, docs, "entity", "", limit, scopeKwd,
				map[string]interface{}{"name_kwd": kgEndpointTerms(neighbours)}, "", 0)
			var found []KgEntity
			for _, row := range neighRows {
				if e, ok := kgParseEntity(row); ok {
					found = append(found, e)
				}
			}
			frontier = addEntities(found, kbID)
		}
	}

	if len(entities) == 0 && len(relations) == 0 {
		return empty, nil
	}
	return KgExploreResult{
		Entities:  entities,
		Relations: relations,
		Passages:  a.evidencePassages(ctx, req.TenantID, datasetIDs, entities, relations),
	}, nil
}

// seedVector encodes the query for the dense seed search. A nil result (no
// encoder, or an encoding failure) sends the caller to the keyword fallback.
func (a *KgAdapter) seedVector(ctx context.Context, tenantID, text string) []float64 {
	if a.seedEncoder == nil {
		return nil
	}
	defer func() {
		// A misconfigured embedding service must degrade to keyword seeding,
		// never take the turn down.
		_ = recover()
	}()
	vec := a.seedEncoder(ctx, tenantID, text)
	if len(vec) == 0 {
		return nil
	}
	return vec
}

// seedSearch resolves the seed entity rows: dense KNN over the entity rows
// re-ranked by mention_count desc, or a keyword match when no vector is
// available.
func (a *KgAdapter) seedSearch(ctx context.Context, tenantID, kbID string, docs []string, text, scopeKwd string, seedVec []float64) []map[string]interface{} {
	if len(seedVec) > 0 {
		dense := &enginetypes.MatchDenseExpr{
			VectorColumnName:  fmt.Sprintf("q_%d_vec", len(seedVec)),
			EmbeddingData:     seedVec,
			EmbeddingDataType: "float",
			DistanceType:      "cosine",
			TopN:              kgSeedPool,
			ExtraOptions:      map[string]interface{}{"similarity": kgSeedSim},
		}
		rows := a.kgSearchRaw(ctx, tenantID, kbID, docs, "entity", scopeKwd, nil, []interface{}{dense}, "mention_count_int", kgSeedPool)
		return kgTopMentionCount(rows, kgSeeds)
	}
	return a.kgSearch(ctx, tenantID, kbID, docs, "entity", text, kgSeeds, scopeKwd, nil, "mention_count_int", kgSeedPool)
}

// kgSearchRaw is the low-level KG row search over one dataset.
func (a *KgAdapter) kgSearchRaw(ctx context.Context, tenantID, kbID string, docs []string, kind, scopeKwd string, extra map[string]interface{}, matchExprs []interface{}, orderDesc string, limit int) []map[string]interface{} {
	condition := map[string]interface{}{"knowledge_graph_kwd": kind}
	if scopeKwd != "" {
		condition["scope_kwd"] = scopeKwd
	}
	if len(docs) > 0 {
		condition["doc_id"] = docs
	}
	for key, value := range extra {
		condition[key] = value
	}
	req := &enginetypes.SearchRequest{
		IndexNames: []string{kgIndexName(tenantID)},
		KbIDs:      []string{kbID},
		SelectFields: []string{
			"id", "content_with_weight", "source_chunk_ids", "doc_id", "docnm_kwd",
			"name_kwd", "mention_count_int", "from_entity_kwd", "to_entity_kwd",
		},
		Filter: condition,
		Limit:  limit,
		// KG rows are compiled products: an available_int=1 default would hide
		// every one of them on an engine with equality semantics.
		IncludeUnavailable: true,
		MatchExprs:         matchExprs,
	}
	if orderDesc != "" {
		req.OrderBy = &enginetypes.OrderByExpr{}
		req.OrderBy.Desc(orderDesc)
	}
	res, err := a.docEngine.Search(ctx, req)
	if err != nil {
		return nil
	}
	return res.Chunks
}

// kgSearch searches the KG rows of one dataset with a text match expression.
func (a *KgAdapter) kgSearch(ctx context.Context, tenantID, kbID string, docs []string, kind, text string, topN int, scopeKwd string, extra map[string]interface{}, orderDesc string, pool int) []map[string]interface{} {
	var matchExprs []interface{}
	if text != "" {
		candidates := topN
		if pool > candidates {
			candidates = pool
		}
		matchExprs = []interface{}{&enginetypes.MatchTextExpr{
			Fields:       []string{"content_ltks", "content_sm_ltks"},
			MatchingText: text,
			TopN:         candidates,
		}}
	}
	return a.kgSearchRaw(ctx, tenantID, kbID, docs, kind, scopeKwd, extra, matchExprs, orderDesc, topN)
}

// evidencePassages loads the source chunks behind the subgraph, in first-seen
// doc order, capped at kgMaxPassages.
func (a *KgAdapter) evidencePassages(ctx context.Context, tenantID string, datasetIDs []string, entities []KgEntity, relations []KgRelation) []KgPassage {
	type evidence struct {
		docID string
		ids   []string
	}
	var groups []evidence
	index := map[string]int{}
	seen := map[string]bool{}
	add := func(docID string, ids []string) {
		for _, chunkID := range ids {
			if chunkID == "" {
				continue
			}
			key := docID + "|" + chunkID
			if seen[key] {
				continue
			}
			seen[key] = true
			i, ok := index[docID]
			if !ok {
				i = len(groups)
				index[docID] = i
				groups = append(groups, evidence{docID: docID})
			}
			groups[i].ids = append(groups[i].ids, chunkID)
		}
	}
	for _, e := range entities {
		add(e.DocID, e.SourceChunkIDs)
	}
	for _, r := range relations {
		add(r.DocID, r.SourceChunkIDs)
	}

	out := make([]KgPassage, 0, kgMaxPassages)
	for _, group := range groups {
		if len(out) >= kgMaxPassages {
			break
		}
		remaining := kgMaxPassages - len(out)
		for _, p := range a.loadPassages(ctx, tenantID, datasetIDs, group.ids, remaining) {
			out = append(out, p)
		}
	}
	return out
}

// loadPassages reads the given chunk ids back from the tenant index.
func (a *KgAdapter) loadPassages(ctx context.Context, tenantID string, datasetIDs, chunkIDs []string, limit int) []KgPassage {
	chunkIDs = nonEmptyStrings(chunkIDs)
	if len(chunkIDs) == 0 || limit <= 0 {
		return nil
	}
	if len(chunkIDs) > limit {
		chunkIDs = chunkIDs[:limit]
	}
	res, err := a.docEngine.Search(ctx, &enginetypes.SearchRequest{
		IndexNames:   []string{kgIndexName(tenantID)},
		KbIDs:        datasetIDs,
		Filter:       map[string]interface{}{"id": chunkIDs},
		SelectFields: []string{"content_with_weight", "doc_id", "docnm_kwd", "kb_id"},
		Limit:        len(chunkIDs),
	})
	if err != nil || res == nil {
		return nil
	}
	out := make([]KgPassage, 0, len(res.Chunks))
	for _, row := range res.Chunks {
		content := kgStr(row["content_with_weight"])
		if strings.TrimSpace(content) == "" {
			continue
		}
		out = append(out, KgPassage{
			ChunkID:   kgStr(row["id"]),
			DocID:     kgStr(row["doc_id"]),
			DocName:   kgStr(row["docnm_kwd"]),
			DatasetID: kgStr(row["kb_id"]),
			Content:   truncateRunes(content, kgPassageRunes),
		})
	}
	return out
}

// kgIndexName is the chunk index of a tenant.
func kgIndexName(tenantID string) string {
	return "ragflow_" + tenantID
}

// kgTopMentionCount re-ranks rows by mention_count desc and returns the top N.
func kgTopMentionCount(rows []map[string]interface{}, topN int) []map[string]interface{} {
	sort.SliceStable(rows, func(i, j int) bool {
		return kgMentionCount(rows[i]) > kgMentionCount(rows[j])
	})
	if len(rows) > topN {
		rows = rows[:topN]
	}
	return rows
}

func kgMentionCount(row map[string]interface{}) int {
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

// kgParseEntity projects an entity row into a KgEntity. Rows without a usable
// name are dropped rather than rendered as an anonymous node.
func kgParseEntity(row map[string]interface{}) (KgEntity, bool) {
	payload := map[string]interface{}{}
	if s := kgStr(row["content_with_weight"]); s != "" {
		_ = json.Unmarshal([]byte(s), &payload)
	}
	name := ""
	for _, key := range []string{"name", "term", "title"} {
		if v := kgStr(payload[key]); v != "" {
			name = v
			break
		}
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return KgEntity{}, false
	}
	e := KgEntity{
		Name:           name,
		Type:           kgStrOr(payload["type"], "other"),
		Description:    kgStrOr(payload["description"], ""),
		SourceChunkIDs: kgStrSlice(row["source_chunk_ids"]),
		DocID:          kgStr(row["doc_id"]),
	}
	if aliases, ok := payload["aliases"].([]interface{}); ok {
		for _, alias := range aliases {
			if s := strings.TrimSpace(kgStr(alias)); s != "" {
				e.Aliases = append(e.Aliases, s)
			}
		}
	}
	return e, true
}

// kgParseRelation projects a relation row into a KgRelation. A row with an
// incomplete endpoint pair is dropped: an edge "A ->" carries no fact.
func kgParseRelation(row map[string]interface{}) (KgRelation, bool) {
	from := strings.TrimSpace(kgStr(row["from_entity_kwd"]))
	to := strings.TrimSpace(kgStr(row["to_entity_kwd"]))
	if from == "" || to == "" {
		return KgRelation{}, false
	}
	typ := "related"
	if payload := kgStr(row["content_with_weight"]); payload != "" {
		var p map[string]interface{}
		if json.Unmarshal([]byte(payload), &p) == nil {
			if t := kgStr(p["type"]); t != "" {
				typ = t
			} else if t := kgStr(p["relation"]); t != "" {
				typ = t
			}
		}
	}
	return KgRelation{
		ID:             kgStr(row["id"]),
		From:           from,
		To:             to,
		Type:           typ,
		SourceChunkIDs: kgStrSlice(row["source_chunk_ids"]),
		DocID:          kgStr(row["doc_id"]),
	}, true
}

// kgEndpointTerms returns the original and lowercased forms of every name, so a
// hop query matches both compacted (lowercased) and per-document endpoint
// fields.
func kgEndpointTerms(names []string) []string {
	set := map[string]bool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		set[name] = true
		set[strings.ToLower(name)] = true
	}
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func kgStr(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func kgStrOr(v interface{}, fallback string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return fallback
}

func kgStrSlice(v interface{}) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []interface{}:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
