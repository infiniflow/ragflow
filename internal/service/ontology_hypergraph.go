package service

import (
	"context"
	"sort"
	"strings"

	"ragflow/internal/agentic_rag"
	"ragflow/internal/common"
	"ragflow/internal/service/nlp"

	"go.uber.org/zap"
)

// ---- The hypergraph recall layer ----
//
// The compiler already materializes two derived row families from the entity /
// relation rows (see internal/ingestion/.../structure/hypergraph.go and
// hypergraph_retrieval.md §1): one `hyperedge` row per flattened factual block,
// and two `hypernode` rows per (path, value) slot — one embedding the key, one
// embedding the value.
//
// This file is the read side. It adds three things to a knowledge lookup and
// changes nothing about the existing assertion path:
//
//	S3  two hypernode recall legs (key side, value side)
//	S5  greedy coverage selection of the blocks those slots belong to
//	S7  assembly: fact blocks + the original chunks they point at
//	S8  exact coverage over the selected blocks, as a zero-LLM sufficiency gate
//
// Every step degrades to "no hypergraph section" rather than to an error: a
// corpus that has never compiled the layer, or a scope whose slots belong to no
// block, must still answer from the assertion rows.
//
// The whole section is behind OntologyABoxRetrievalRequest.HypergraphRecall
// (default off) so the layer can be measured against the assertion-only path on
// the same questions — see hypergraph_retrieval.md §4.

const (
	// ontologyHypergraphNodeTopK bounds each recall leg. A slot set larger than
	// this is one the coverage selection cannot explain anyway, and the legs are
	// paid for on every turn.
	ontologyHypergraphNodeTopK = 20
	// ontologyHypergraphEdgeTopK bounds how many blocks are assembled. It is the
	// cost ceiling of the fact section: four blocks of a handful of slots is
	// what a reader can act on in one turn.
	ontologyHypergraphEdgeTopK = 4
	// ontologyHypergraphChunkTopK bounds the original-chunk block.
	ontologyHypergraphChunkTopK = 6
	// ontologyHypergraphKeySimilarity is the key leg's floor, unchanged from the
	// assertion path: a key is a natural-language attribute path and reads
	// exactly like the wording a question uses.
	ontologyHypergraphKeySimilarity = 0.2
	// ontologyHypergraphValueSimilarity is the value leg's floor. A value is
	// short ("winter wheat", "north plain") and a short string sits close to
	// everything in a dense space, so the leg needs a higher floor than the key
	// leg to keep the slot set meaningful.
	ontologyHypergraphValueSimilarity = 0.25
	// ontologyHypergraphSufficientCoverage is the node-coverage needed before the
	// assembled blocks are called sufficient. Below it the caller should keep
	// searching rather than answer from a partial block set.
	ontologyHypergraphSufficientCoverage = 0.6
	// ontologyHypergraphMaxFactsPerEdge bounds one block's rendered slots. A
	// block is generated from a document and can be long; the section is a
	// pointer to the original chunk, not a replacement for it.
	ontologyHypergraphMaxFactsPerEdge = 12
	// ontologyHypergraphMaxContentRunes bounds one original chunk carried in the
	// digest. The full text is one tool call away; what the section needs is
	// enough prose to recognise the passage.
	ontologyHypergraphMaxContentRunes = 800
	// ontologyHypergraphPromotionFields bounds the entity-to-block promotion
	// (S6): a name per hit entity, capped so one ambitious question cannot ask
	// the engine for a filter of unbounded width.
	ontologyHypergraphPromotionLimit = 20
)

// ontologyHypernodeRowFields is the hypernode projection. node_value_kwd is
// requested because it is the only exact spelling of a slot's value — without it
// a key-side row cannot be joined to its value-side twin, and the entity
// promotion stage could only match by full-text.
var ontologyHypernodeRowFields = []string{
	"id", "content_with_weight", "path_kwd", "prop_kwd", "node_role_kwd",
	"node_value_kwd", "hyperedge_ids", "depth_int", "source_chunk_ids", "doc_id",
}

// ontologyHyperedgeRowFields is the hyperedge projection: the payload (the
// block's slots), the root it was flattened from, and the chunks it cites.
var ontologyHyperedgeRowFields = []string{
	"id", "content_with_weight", "name_kwd", "entity_type_kwd", "depth_int",
	"mention_count_int", "source_chunk_ids", "doc_id", "doc_ids_kwd", "source_doc_ids",
}

// OntologyHypernodeKindFilter pins one recall leg to one half of the layer.
// An empty role is both halves, which no leg asks for: the two halves score
// against different text and mixing them would rank a value against a key.
func OntologyHypernodeKindFilter(role string) map[string]interface{} {
	filter := map[string]interface{}{"knowledge_graph_kwd": []string{"hypernode"}}
	if role != "" {
		filter["node_role_kwd"] = []string{role}
	}
	return filter
}

// OntologyHyperedgeKindFilter pins a read to factual blocks.
func OntologyHyperedgeKindFilter() map[string]interface{} {
	return map[string]interface{}{"knowledge_graph_kwd": []string{"hyperedge"}}
}

// ontologyRowReader reads raw rows by filter. It exists as a seam because the
// assembly stages need rows the retrieval service does not return (a block by
// id, the blocks a name belongs to), and a unit test must be able to answer
// those reads without a document engine.
type ontologyRowReader func(ctx context.Context, tenantID string, datasetIDs []string, filter map[string]interface{}, fields []string, limit int) ([]map[string]interface{}, error)

// ontologyChunkReader loads original chunks by id.
type ontologyChunkReader func(ctx context.Context, tenantID string, datasetIDs, ids []string) []map[string]interface{}

// readOntologyRows is the production reader: one filtered search per dataset,
// merged. It goes through graphRowSearch so it scopes by dataset the same way
// every other compiled-row reader does.
func readOntologyRows(ctx context.Context, tenantID string, datasetIDs []string, filter map[string]interface{}, fields []string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 1
	}
	out := make([]map[string]interface{}, 0, limit)
	for _, datasetID := range datasetIDs {
		rows, _, err := graphRowSearch(ctx, tenantID, datasetID, fields, filter, nil, 0, limit, nil)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			out = append(out, row)
		}
	}
	return out, nil
}

// ontologyHypernodeHit is one recalled slot: the (path, value) pair, the blocks
// it belongs to, and the score the leg gave it.
type ontologyHypernodeHit struct {
	// nodeKey identifies the slot. It is path+value, which is the row id's own
	// identity (both halves of a hypernode carry both columns), so the two legs
	// dedupe onto one slot instead of counting it twice.
	nodeKey string
	// path is the mechanical attribute path (path_kwd) and prop its leaf
	// attribute name. slot is the (class, attribute) pair derived from path.
	path string
	prop string
	slot string
	// value is the slot's value exactly as the source states it; role is the leg
	// that recalled it.
	value string
	role  string
	// score is the fused similarity the retrieval service computed for this row.
	score float64
	// depth is the block depth the row was written at.
	depth int
	// edgeIDs are the blocks this slot belongs to — the membership relation the
	// coverage selection runs on.
	edgeIDs []string
	// chunkIDs / docID place the slot back in the corpus.
	chunkIDs []string
	docID    string
	// rowID is the engine row id, kept for diagnostics.
	rowID string
	// promoted reports that the hit came from S6 (an entity's name node) rather
	// than a recall leg; those carry no similarity of their own.
	promoted bool
}

// buildOntologyHypergraph runs the hypergraph section of one knowledge lookup.
//
// It never returns an error: every failure is a reason the section is empty or
// partial, and the assertion rows the caller already has must still answer. The
// returned value is nil only when the request did not ask for the section.
func (s *OntologyABoxRetrievalService) buildOntologyHypergraph(
	ctx context.Context,
	scope ontologyABoxScope,
	req *OntologyABoxRetrievalRequest,
	assertionRows []map[string]interface{},
) *agentic_rag.OntologyHypergraph {
	if !req.HypergraphRecall {
		return nil
	}
	partial := false

	// S3 — the two recall legs. A failed leg leaves the other one standing and
	// marks the coverage as partial: the slot set the numbers are computed over
	// is then smaller than it should be, and saying so is what stops "no block
	// covered this" from being read as "the corpus lacks it".
	keyHits, err := s.recallOntologyHypernodes(ctx, scope, req, "key", ontologyHypergraphNodeTopK, ontologyHypergraphKeySimilarity)
	if err != nil {
		partial = true
		common.WarnCtx(ctx, "ontology hypergraph: key leg failed", zap.Error(err))
	}
	valueHits, err := s.recallOntologyHypernodes(ctx, scope, req, "value", ontologyHypergraphNodeTopK, ontologyHypergraphValueSimilarity)
	if err != nil {
		partial = true
		common.WarnCtx(ctx, "ontology hypergraph: value leg failed", zap.Error(err))
	}

	// S4 — one slot counted once, at its best score.
	hits := mergeOntologyHypernodeHits(keyHits, valueHits)

	// S6 — an entity that was hit as an assertion but is not a root has no
	// hyperedge of its own; its name node is the only way in. This runs before
	// selection because those blocks are chosen by the same coverage, not
	// appended afterwards.
	if promoted, promoteErr := s.promoteOntologyEntityNames(ctx, scope, req, assertionRows); promoteErr != nil {
		partial = true
		common.WarnCtx(ctx, "ontology hypergraph: entity promotion failed", zap.Error(promoteErr))
	} else {
		hits = mergeOntologyHypernodeHits(hits, promoted)
	}

	if len(hits) == 0 {
		common.InfoCtx(ctx, "ontology hypergraph absent: no slot recalled",
			zap.Strings("datasets", req.DatasetIDs))
		return &agentic_rag.OntologyHypergraph{
			Edges:  []agentic_rag.OntologyHyperedge{},
			Chunks: []agentic_rag.OntologyHypergraphChunk{},
			Coverage: agentic_rag.OntologyHypergraphCoverage{
				Partial: partial,
			},
			Degrade: &agentic_rag.OntologyDegrade{
				Code: agentic_rag.OntologyDegradeHypergraphAbsent,
				Reason: "the knowledge scope holds no compiled hypernode slot for this question, " +
					"so no factual block could be assembled",
			},
		}
	}

	// S5 — which blocks explain the most of what was recalled.
	selection := selectOntologyHyperedgeBlocks(hits, ontologyHypergraphEdgeTopK)
	if len(selection.edgeIDs) == 0 {
		common.InfoCtx(ctx, "ontology hypergraph absent: recalled slots belong to no block",
			zap.Int("hits", len(hits)), zap.Int("orphans", selection.orphanHits))
		return &agentic_rag.OntologyHypergraph{
			Edges:  []agentic_rag.OntologyHyperedge{},
			Chunks: []agentic_rag.OntologyHypergraphChunk{},
			Coverage: agentic_rag.OntologyHypergraphCoverage{
				HitNodes:     len(hits),
				OrphanHits:   selection.orphanHits,
				NodeCoverage: 0,
				SlotCoverage: 0,
				Partial:      partial,
			},
			Degrade: &agentic_rag.OntologyDegrade{
				Code: agentic_rag.OntologyDegradeHypergraphAbsent,
				Reason: "the recalled slots carry no hyperedge membership, so the compiled blocks " +
					"are missing or were written by an older compiler",
			},
		}
	}

	// S7 — the blocks themselves, then the original text they point at.
	edges := s.loadOntologyHyperedgeBlocks(ctx, scope, req, selection)
	chunks := s.loadOntologyHyperedgeChunks(ctx, scope, req, edges)

	// S8 — coverage, exactly over what was recalled and selected.
	coverage := ontologyHypergraphCoverage(hits, selection, partial)
	common.InfoCtx(ctx, "ontology hypergraph assembled",
		zap.Int("hits", coverage.HitNodes),
		zap.Int("covered", coverage.CoveredNodes),
		zap.Int("edges", len(edges)),
		zap.Int("chunks", len(chunks)),
		zap.Float64("nodeCoverage", coverage.NodeCoverage),
		zap.Bool("sufficient", coverage.Sufficient),
		zap.Bool("partial", coverage.Partial))

	return &agentic_rag.OntologyHypergraph{
		Edges:    edges,
		Chunks:   chunks,
		Coverage: coverage,
	}
}

// recallOntologyHypernodes runs one recall leg and parses its rows into slots.
func (s *OntologyABoxRetrievalService) recallOntologyHypernodes(
	ctx context.Context,
	scope ontologyABoxScope,
	req *OntologyABoxRetrievalRequest,
	role string,
	topK int,
	similarity float64,
) ([]ontologyHypernodeHit, error) {
	filter := OntologyHypernodeKindFilter(role)
	page, err := s.searcher.Retrieval(ctx, &nlp.RetrievalRequest{
		TenantIDs:             scope.TenantIDs,
		Question:              req.Question,
		KbIDs:                 req.DatasetIDs,
		DocIDs:                req.DocumentIDs,
		Page:                  1,
		PageSize:              topK,
		RerankCandidatesCount: &topK,
		SimilarityThreshold:   &similarity,
		EmbeddingModel:        scope.EmbeddingModel,
		Filter:                filter,
		SelectFields:          ontologyHypernodeRowFields,
		Aggs:                  ontologyBoolValue(false),
		Highlight:             ontologyBoolValue(false),
	})
	if err != nil {
		return nil, err
	}
	if page == nil {
		return nil, nil
	}
	return ontologyHypernodeHitsFromRows(page.Chunks, role), nil
}

// ontologyBoolValue returns a pointer to v, for the request's tri-state fields.
func ontologyBoolValue(v bool) *bool {
	return &v
}

// ontologyHypernodeHitsFromRows parses one leg's rows.
func ontologyHypernodeHitsFromRows(rows []map[string]interface{}, role string) []ontologyHypernodeHit {
	hits := make([]ontologyHypernodeHit, 0, len(rows))
	for _, row := range rows {
		hit, ok := ontologyHypernodeHitFromRow(row, role)
		if ok {
			hits = append(hits, hit)
		}
	}
	return hits
}

// ontologyHypernodeHitFromRow converts one engine row into a slot.
//
// A row without a path is dropped: the path is what the block selection groups
// on, and a slot whose path is unknown cannot be placed in any block.
func ontologyHypernodeHitFromRow(row map[string]interface{}, role string) (ontologyHypernodeHit, bool) {
	path := strings.TrimSpace(firstStringValue(row["path_kwd"]))
	if path == "" {
		return ontologyHypernodeHit{}, false
	}
	rowID := firstStringValue(row["chunk_id"])
	if rowID == "" {
		rowID = firstStringValue(row["id"])
	}
	value := strings.TrimSpace(firstStringValue(row["node_value_kwd"]))
	if value == "" {
		// Rows written before node_value_kwd existed carry the value only in
		// their payload, and only on the value half. Reading it back keeps the
		// layer usable until a recompile, at the cost of not being able to join
		// the key half to its value half.
		value = ontologyHypernodePayloadHalf(row, role)
	}
	nodeKey := ontologyHypernodeKey(path, value, rowID)
	return ontologyHypernodeHit{
		nodeKey:  nodeKey,
		path:     path,
		prop:     strings.TrimSpace(firstStringValue(row["prop_kwd"])),
		slot:     ontologyHypergraphSlot(path),
		value:    value,
		role:     role,
		score:    ontologyFloatValue(row["similarity"]),
		depth:    ontologyIntValue(row["depth_int"]),
		edgeIDs:  stringSliceValue(row["hyperedge_ids"]),
		chunkIDs: stringSliceValue(row["source_chunk_ids"]),
		docID:    strings.TrimSpace(firstStringValue(row["doc_id"])),
		rowID:    rowID,
	}, true
}

// ontologyHypernodePayloadHalf reads the half a hypernode row carries.
func ontologyHypernodePayloadHalf(row map[string]interface{}, role string) string {
	payload := graphLoadPayload(row)
	if payload == nil {
		return ""
	}
	if role == "key" {
		return strings.TrimSpace(toStringValue(payload["key"]))
	}
	return strings.TrimSpace(toStringValue(payload["value"]))
}

// ontologyHypernodeKey is the slot identity used for dedupe and coverage. When
// a row's value could not be read, the row id is the only identity it has — a
// key half without a value is a slot that cannot be joined, not a slot equal to
// every other key half of the same path.
func ontologyHypernodeKey(path, value, rowID string) string {
	if value == "" {
		return "row:" + rowID
	}
	return path + "\x00" + value
}

// ontologyHypergraphSlot is the (class, attribute) pair a path names: the last
// dotted segment of `<class>:<prop>` chains. It is what slot_coverage counts, so
// it is read off the path's structure rather than off the prop column alone —
// the same attribute name on two classes is two slots.
func ontologyHypergraphSlot(path string) string {
	last := path
	if idx := strings.LastIndex(path, "."); idx >= 0 {
		last = path[idx+1:]
	}
	if strings.TrimSpace(last) == "" {
		return path
	}
	return last
}

// mergeOntologyHypernodeHits folds several legs onto one slot set, keeping the
// best score and the union of every membership they reported. The union matters:
// a slot recalled by the key leg and again by the value leg may name the same
// blocks twice, but a slot recalled once as a hit node and once as an entity's
// name node can name different blocks only if the compiler disagreed with
// itself — keeping both is the honest reading of two rows.
func mergeOntologyHypernodeHits(groups ...[]ontologyHypernodeHit) []ontologyHypernodeHit {
	byKey := map[string]*ontologyHypernodeHit{}
	order := make([]string, 0)
	for _, group := range groups {
		for _, hit := range group {
			existing, ok := byKey[hit.nodeKey]
			if !ok {
				copied := hit
				byKey[hit.nodeKey] = &copied
				order = append(order, hit.nodeKey)
				continue
			}
			if hit.score > existing.score {
				existing.score = hit.score
			}
			if existing.value == "" && hit.value != "" {
				existing.value = hit.value
			}
			if existing.slot == "" && hit.slot != "" {
				existing.slot = hit.slot
			}
			existing.edgeIDs = dedupeStrings(append(existing.edgeIDs, hit.edgeIDs...))
			existing.chunkIDs = dedupeStrings(append(existing.chunkIDs, hit.chunkIDs...))
			if existing.docID == "" {
				existing.docID = hit.docID
			}
			if hit.promoted {
				existing.promoted = true
			}
		}
	}
	out := make([]ontologyHypernodeHit, 0, len(order))
	for _, key := range order {
		out = append(out, *byKey[key])
	}
	// Best first, then by slot key so the ordering — and therefore which blocks
	// a tied selection can reach — does not depend on map iteration.
	sortOntologyHypernodeHits(out)
	return out
}

// promoteOntologyEntityNames is S6: an entity that was hit as an assertion but
// is not a root of the object-property graph has no hyperedge of its own (the
// compiler only writes a block per visited entity, and the retrieval-side join
// from an entity row to a block is the name node). Asking for the name nodes of
// the hit entities recovers those blocks.
//
// It reads the name-type slots by their exact value column — one batched read
// for every hit entity — because a full-text match would silently miss a renamed
// entity and there would be nothing to notice.
func (s *OntologyABoxRetrievalService) promoteOntologyEntityNames(
	ctx context.Context,
	scope ontologyABoxScope,
	req *OntologyABoxRetrievalRequest,
	assertionRows []map[string]interface{},
) ([]ontologyHypernodeHit, error) {
	if s.rowReader == nil {
		// Without a row reader there is no way to look up a name node; the
		// recall legs still work, so this is a missing promotion rather than a
		// missing section.
		return nil, nil
	}
	names := make([]string, 0, len(assertionRows))
	classOf := make(map[string]string, len(assertionRows))
	for _, row := range assertionRows {
		if ontologyRowKind(row) != "entity" {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(firstStringValue(row["name_kwd"])))
		if name == "" {
			continue
		}
		if _, seen := classOf[name]; seen {
			continue
		}
		classOf[name] = strings.TrimSpace(firstStringValue(row["entity_type_kwd"]))
		names = append(names, name)
		if len(names) >= ontologyHypergraphPromotionLimit {
			break
		}
	}
	if len(names) == 0 {
		return nil, nil
	}
	filter := map[string]interface{}{
		"knowledge_graph_kwd": []string{"hypernode"},
		"node_role_kwd":       []string{"value"},
		"node_value_kwd":      names,
	}
	rows, err := s.rowReader(ctx, scope.TenantIDs[0], req.DatasetIDs, filter,
		ontologyHypernodeRowFields, len(names)*ontologyHypergraphEdgeTopK*2)
	if err != nil {
		return nil, err
	}
	hits := make([]ontologyHypernodeHit, 0, len(rows))
	for _, row := range rows {
		path := strings.TrimSpace(firstStringValue(row["path_kwd"]))
		if !strings.HasSuffix(path, ":name") {
			// Only a name slot is an entity's identity; an attribute slot that
			// happens to hold the same value is a different assertion.
			continue
		}
		value := strings.ToLower(strings.TrimSpace(firstStringValue(row["node_value_kwd"])))
		class, ok := classOf[value]
		if !ok {
			continue
		}
		if class != "" && ontologyHypergraphSlot(path) != class+":name" {
			// The same name can be typed by another class in the same block;
			// only the class the hit entity actually carries is this entity's
			// identity.
			continue
		}
		rowID := firstStringValue(row["id"])
		hits = append(hits, ontologyHypernodeHit{
			nodeKey:  ontologyHypernodeKey(path, value, rowID),
			path:     path,
			prop:     "name",
			slot:     ontologyHypergraphSlot(path),
			value:    value,
			role:     "name",
			depth:    ontologyIntValue(row["depth_int"]),
			edgeIDs:  stringSliceValue(row["hyperedge_ids"]),
			chunkIDs: stringSliceValue(row["source_chunk_ids"]),
			docID:    strings.TrimSpace(firstStringValue(row["doc_id"])),
			rowID:    rowID,
			promoted: true,
		})
	}
	return hits, nil
}

// ontologyHyperedgeSelection is the outcome of the greedy coverage pass.
type ontologyHyperedgeSelection struct {
	// edgeIDs are the selected blocks, in the order they were chosen.
	edgeIDs []string
	// covered are the slot keys the selection explains.
	covered map[string]bool
	// contributions is how many uncovered slots each selected block added.
	contributions map[string]int
	// orphanHits counts recalled slots that belong to no block at all.
	orphanHits int
}

// selectOntologyHyperedgeBlocks is S5. The blocks are chosen to explain as many
// of the recalled slots as possible, one at a time, with a deterministic
// tie-break: more newly covered slots, then higher score, then shallower, then
// smaller row id. Determinism is the requirement — the same question over the
// same rows has to assemble the same blocks, or two runs cannot be compared.
//
// No composite "similarity × coverage" score is used on purpose: the selection
// has to be explainable ("this block covered three of the four slots you
// recalled") before it is optimized.
func selectOntologyHyperedgeBlocks(hits []ontologyHypernodeHit, edgeTopK int) ontologyHyperedgeSelection {
	selection := ontologyHyperedgeSelection{
		covered:       map[string]bool{},
		contributions: map[string]int{},
	}
	members := map[string][]string{}  // edge id -> slot keys among hits
	edgeScore := map[string]float64{} // best hit score of a member
	edgeDepth := map[string]int{}     // shallowest member depth
	for _, hit := range hits {
		if len(hit.edgeIDs) == 0 {
			selection.orphanHits++
			continue
		}
		for _, edgeID := range hit.edgeIDs {
			members[edgeID] = append(members[edgeID], hit.nodeKey)
			if score, ok := edgeScore[edgeID]; !ok || hit.score > score {
				edgeScore[edgeID] = hit.score
			}
			if depth, ok := edgeDepth[edgeID]; !ok || hit.depth < depth {
				edgeDepth[edgeID] = hit.depth
			}
		}
	}
	if len(members) == 0 || edgeTopK <= 0 {
		return selection
	}

	chosen := map[string]bool{}
	for len(selection.edgeIDs) < edgeTopK {
		bestID := ""
		bestGain := 0
		for edgeID, slots := range members {
			if chosen[edgeID] {
				continue
			}
			gain := 0
			for _, slot := range slots {
				if !selection.covered[slot] {
					gain++
				}
			}
			if gain == 0 {
				continue
			}
			if bestID == "" || ontologyHyperedgeBetterChoice(edgeID, gain, bestID, bestGain, edgeScore, edgeDepth) {
				bestID, bestGain = edgeID, gain
			}
		}
		if bestID == "" {
			break
		}
		chosen[bestID] = true
		selection.edgeIDs = append(selection.edgeIDs, bestID)
		selection.contributions[bestID] = bestGain
		for _, slot := range members[bestID] {
			selection.covered[slot] = true
		}
	}
	return selection
}

// ontologyHyperedgeBetterChoice applies the tie-break rule: more newly covered
// slots, then higher score, then shallower depth, then smaller id.
func ontologyHyperedgeBetterChoice(edgeID string, gain int, bestID string, bestGain int, score map[string]float64, depth map[string]int) bool {
	if gain != bestGain {
		return gain > bestGain
	}
	if score[edgeID] != score[bestID] {
		return score[edgeID] > score[bestID]
	}
	if depth[edgeID] != depth[bestID] {
		return depth[edgeID] < depth[bestID]
	}
	return edgeID < bestID
}

// loadOntologyHyperedgeBlocks fetches the selected blocks and renders them for
// the digest. A block whose row cannot be read is dropped rather than rendered
// from the selection alone: the section's whole value is the facts, and an empty
// block would be a citation with nothing behind it.
func (s *OntologyABoxRetrievalService) loadOntologyHyperedgeBlocks(
	ctx context.Context,
	scope ontologyABoxScope,
	req *OntologyABoxRetrievalRequest,
	selection ontologyHyperedgeSelection,
) []agentic_rag.OntologyHyperedge {
	if s.rowReader == nil {
		return []agentic_rag.OntologyHyperedge{}
	}
	filter := OntologyHyperedgeKindFilter()
	filter["id"] = selection.edgeIDs
	rows, err := s.rowReader(ctx, scope.TenantIDs[0], req.DatasetIDs, filter,
		ontologyHyperedgeRowFields, len(selection.edgeIDs))
	if err != nil {
		common.WarnCtx(ctx, "ontology hypergraph: block rows unreadable", zap.Error(err))
		return []agentic_rag.OntologyHyperedge{}
	}
	byID := make(map[string]map[string]interface{}, len(rows))
	for _, row := range rows {
		if id := firstStringValue(row["id"]); id != "" {
			byID[id] = row
		}
	}
	out := make([]agentic_rag.OntologyHyperedge, 0, len(selection.edgeIDs))
	for _, edgeID := range selection.edgeIDs {
		row, ok := byID[edgeID]
		if !ok {
			continue
		}
		out = append(out, ontologyHyperedgeFromRow(edgeID, row, selection.contributions[edgeID]))
	}
	return out
}

// ontologyHyperedgeFromRow renders one block: its root, its depth, its facts in
// path order, and the chunks it came from.
func ontologyHyperedgeFromRow(edgeID string, row map[string]interface{}, covered int) agentic_rag.OntologyHyperedge {
	payload := graphLoadPayload(row)
	facts := make([]agentic_rag.OntologyHypergraphFact, 0, len(payload))
	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if len(facts) >= ontologyHypergraphMaxFactsPerEdge {
			break
		}
		value := strings.TrimSpace(toStringValue(payload[key]))
		if value == "" {
			continue
		}
		facts = append(facts, agentic_rag.OntologyHypergraphFact{Path: key, Value: value})
	}
	return agentic_rag.OntologyHyperedge{
		RowID:        edgeID,
		Root:         strings.TrimSpace(firstStringValue(row["name_kwd"])),
		RootType:     strings.TrimSpace(firstStringValue(row["entity_type_kwd"])),
		Depth:        ontologyIntValue(row["depth_int"]),
		Facts:        facts,
		CoveredNodes: covered,
		ChunkIDs:     stringSliceValue(row["source_chunk_ids"]),
		DocIDs:       ontologyRowDocIDs(row),
	}
}

// loadOntologyHyperedgeChunks is the original-text block: the chunks the
// selected blocks point at, ordered by how much of the question's slot set the
// block that pulled them in explained.
func (s *OntologyABoxRetrievalService) loadOntologyHyperedgeChunks(
	ctx context.Context,
	scope ontologyABoxScope,
	req *OntologyABoxRetrievalRequest,
	edges []agentic_rag.OntologyHyperedge,
) []agentic_rag.OntologyHypergraphChunk {
	type chunkRef struct {
		contribution int
	}
	order := make([]string, 0)
	refs := map[string]chunkRef{}
	for _, edge := range edges {
		for _, chunkID := range edge.ChunkIDs {
			if chunkID == "" {
				continue
			}
			existing, ok := refs[chunkID]
			if !ok {
				order = append(order, chunkID)
				refs[chunkID] = chunkRef{contribution: edge.CoveredNodes}
				continue
			}
			if edge.CoveredNodes > existing.contribution {
				refs[chunkID] = chunkRef{contribution: edge.CoveredNodes}
			}
		}
	}
	if len(order) == 0 {
		return []agentic_rag.OntologyHypergraphChunk{}
	}
	// Highest contribution first, then the order the blocks named them: the
	// chunk cap must drop the least explanatory passage, not an arbitrary one.
	sort.SliceStable(order, func(i, j int) bool {
		return refs[order[i]].contribution > refs[order[j]].contribution
	})
	if len(order) > ontologyHypergraphChunkTopK {
		order = order[:ontologyHypergraphChunkTopK]
	}
	if s.chunkReader == nil {
		return []agentic_rag.OntologyHypergraphChunk{}
	}
	rows := s.chunkReader(ctx, scope.TenantIDs[0], req.DatasetIDs, order)
	byID := make(map[string]map[string]interface{}, len(rows))
	for _, row := range rows {
		id := firstStringValue(row["chunk_id"])
		if id == "" {
			id = firstStringValue(row["id"])
		}
		if id != "" {
			byID[id] = row
		}
	}
	out := make([]agentic_rag.OntologyHypergraphChunk, 0, len(order))
	for _, chunkID := range order {
		row, ok := byID[chunkID]
		if !ok {
			continue
		}
		out = append(out, agentic_rag.OntologyHypergraphChunk{
			ChunkID:      chunkID,
			DocID:        strings.TrimSpace(firstStringValue(row["doc_id"])),
			DocName:      strings.TrimSpace(firstStringValue(row["docnm_kwd"])),
			Content:      truncateOntologyContent(firstStringValue(row["content_with_weight"])),
			CoveredNodes: refs[chunkID].contribution,
		})
	}
	return out
}

// ontologyHypergraphCoverage is S8, computed exactly over the recalled slots and
// the selected blocks. It is the zero-LLM sufficiency gate: nothing here asks a
// model whether the blocks are enough, because a judgement that cannot be
// reproduced cannot be argued with.
func ontologyHypergraphCoverage(hits []ontologyHypernodeHit, selection ontologyHyperedgeSelection, partial bool) agentic_rag.OntologyHypergraphCoverage {
	out := agentic_rag.OntologyHypergraphCoverage{
		HitNodes:   len(hits),
		OrphanHits: selection.orphanHits,
		Partial:    partial,
	}
	slots := map[string]bool{}
	coveredSlots := map[string]bool{}
	for _, hit := range hits {
		if hit.slot != "" {
			slots[hit.slot] = true
		}
		if selection.covered[hit.nodeKey] {
			out.CoveredNodes++
			if hit.slot != "" {
				coveredSlots[hit.slot] = true
			}
		}
	}
	if out.HitNodes > 0 {
		out.NodeCoverage = float64(out.CoveredNodes) / float64(out.HitNodes)
	}
	if len(slots) > 0 {
		out.SlotCoverage = float64(len(coveredSlots)) / float64(len(slots))
	}
	out.Sufficient = out.NodeCoverage >= ontologyHypergraphSufficientCoverage && out.OrphanHits == 0
	return out
}

// sortOntologyHypernodeHits orders hits by descending score, then by slot key,
// so the selection and every log line that prints them are reproducible.
func sortOntologyHypernodeHits(hits []ontologyHypernodeHit) {
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].nodeKey < hits[j].nodeKey
	})
}

// truncateOntologyContent cuts on a rune boundary so a multi-byte character is
// never split into an invalid one.
func truncateOntologyContent(content string) string {
	trimmed := strings.TrimSpace(content)
	runes := []rune(trimmed)
	if len(runes) <= ontologyHypergraphMaxContentRunes {
		return trimmed
	}
	return strings.TrimSpace(string(runes[:ontologyHypergraphMaxContentRunes])) + "…"
}

// ontologyIntValue reads an engine numeric column.
func ontologyIntValue(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float32:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

// ontologyFloatValue reads an engine float column.
func ontologyFloatValue(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}
