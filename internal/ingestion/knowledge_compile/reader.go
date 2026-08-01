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

package knowledge_compile

import (
	"context"
	"fmt"

	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// Reader finds the compiled products needed for incremental dedup without
// loading the whole KB into memory (§11.6 step 1, §11.7 incremental re-dedup).
//
// The dedup between an incoming per-document product and the already-merged
// rows lives in external storage (DocEngine): the consumer only keeps the
// in-flight batch in memory and asks the engine for nearest matches via KNN.
// It never scans every compiled chunk of the KB, which would OOM on a large
// knowledge base — this mirrors Python's _struct_doc_storage_dedup_batch, which
// takes only the just-compiled docs and KNN-queries the store per doc.
type Reader interface {
	// LoadDocProducts returns the per-document compiled rows for a single
	// document (doc_id == source_doc). Bounded by one document, never the whole
	// KB.
	LoadDocProducts(ctx context.Context, tenant, kb, docID string) ([]kccommon.Product, error)

	// SearchSimilar runs a dense (KNN) search over the existing merged rows of
	// the given variant and returns the single most-similar row whose score is
	// at least minScore, plus that score. It returns a zero Product when nothing
	// clears the threshold. This mirrors Python's _struct_doc_storage_knn_candidate
	// (topn=1, similarity_threshold): find the dot product above the threshold
	// and maximum, then decide duplication with the LLM.
	SearchSimilar(ctx context.Context, tenant, kb string, variant kccommon.Variant, vector []float64, topN int, minScore float64) (kccommon.Product, float64, error)
}

// engineReader loads the per-document compiled products through the global
// DocEngine (§11.6 step 1, §11.7 incremental re-dedup). It depends on the
// process-wide DocEngine obtained via engine.Get(); the engine abstraction owns
// the storage schema, so this reader is not backend-specific.
type engineReader struct {
	eng engine.DocEngine
}

// compiledSelectFields are the columns needed to reconstruct a Product from a
// stored compiled chunk document.
var compiledSelectFields = []string{
	"id", "doc_id", "tenant_id", "compile_kwd",
	"content_with_weight", "kc_payload",
	"source_chunk_ids", "source_doc_ids",
	"name_kwd", "entity_type_kwd", "from_entity_kwd", "to_entity_kwd",
	"slug_kwd", "type",
}

// LoadDocProducts returns the per-document compiled rows for a single document.
// It is bounded to one document, so the consumer never loads the whole KB.
func (r engineReader) LoadDocProducts(ctx context.Context, tenant, kb, docID string) ([]kccommon.Product, error) {
	eng := r.eng
	if eng == nil {
		eng = engine.Get()
	}
	if eng == nil {
		return nil, nil
	}
	res, err := eng.Search(ctx, &types.SearchRequest{
		IndexNames:   []string{fmt.Sprintf("ragflow_%s", tenant)},
		KbIDs:        []string{kb},
		Filter:       map[string]interface{}{"doc_id": docID},
		SelectFields: compiledSelectFields,
		Limit:        5000,
	})
	if err != nil {
		return nil, err
	}
	var out []kccommon.Product
	for _, c := range res.Chunks {
		// Only compiled products carry compile_kwd; skip ordinary source chunks.
		if _, ok := c["compile_kwd"]; !ok {
			continue
		}
		if p, ok := productFromChunkMap(c, tenant); ok {
			out = append(out, p)
		}
	}
	return out, nil
}

// productFromChunkMap reconstructs a kccommon.Product from a stored compiled
// chunk document. It reads the payload from kc_payload (falling back to
// content_with_weight) and the embedding from the q_<dim>_vec column.
func productFromChunkMap(c map[string]interface{}, tenant string) (kccommon.Product, bool) {
	content, _ := c["kc_payload"].(string)
	if content == "" {
		content, _ = c["content_with_weight"].(string)
	}
	if content == "" {
		return kccommon.Product{}, false
	}
	id, _ := c["id"].(string)
	docID, _ := c["doc_id"].(string)
	variant, _ := c["compile_kwd"].(string)
	merged := c["kc_merged"] == "1"

	meta := map[string]any{}
	if v, ok := c["name_kwd"].(string); ok && v != "" {
		meta["name"] = v
	}
	if v, ok := c["entity_type_kwd"].(string); ok && v != "" {
		meta["entity_type"] = v
	}
	if v, ok := c["from_entity_kwd"].(string); ok && v != "" {
		meta["from"] = v
		meta["kind"] = "relation"
	}
	if v, ok := c["to_entity_kwd"].(string); ok && v != "" {
		meta["to"] = v
		meta["kind"] = "relation"
	}
	if v, ok := c["slug_kwd"].(string); ok && v != "" {
		meta["slug"] = v
	}
	if v, ok := c["type"].(string); ok && v != "" {
		meta["type"] = v
	}
	if _, ok := meta["kind"]; !ok {
		if _, hasName := meta["name"]; hasName {
			meta["kind"] = "entity"
		}
	}
	if v := metaStringSlice(c, "source_chunk_ids"); len(v) > 0 {
		meta["source_chunk_ids"] = v
	}
	if v := metaStringSlice(c, "source_doc_ids"); len(v) > 0 {
		meta["source_doc_ids"] = v
	}

	vec, _ := kccommon.VectorFromChunkMap(c, 0)
	return kccommon.Product{
		ID:       id,
		DocID:    docID,
		TenantID: tenant,
		Variant:  kccommon.Variant(variant),
		Content:  content,
		Vector:   vec,
		Meta:     meta,
		Merged:   merged,
	}, true
}

// SearchSimilar runs a dense KNN over the existing merged rows (kc_merged=1,
// compile_kwd=variant) of the KB and returns the closest hit above minScore.
func (r engineReader) SearchSimilar(ctx context.Context, tenant, kb string, variant kccommon.Variant, vector []float64, topN int, minScore float64) (kccommon.Product, float64, error) {
	eng := r.eng
	if eng == nil {
		eng = engine.Get()
	}
	if eng == nil {
		return kccommon.Product{}, 0, nil
	}
	if topN <= 0 {
		topN = 1
	}
	dim := len(vector)
	req := &types.SearchRequest{
		IndexNames: []string{fmt.Sprintf("ragflow_%s", tenant)},
		KbIDs:      []string{kb},
		Limit:      topN,
		SelectFields: []string{"id", "doc_id", "kb_id", "content_with_weight", "kc_payload",
			"name_kwd", "entity_type_kwd", "from_entity_kwd", "to_entity_kwd", "slug_kwd",
			"type", "source_chunk_ids", "source_doc_ids", "kc_merged", "compile_kwd"},
		Filter: map[string]interface{}{
			"kc_merged":   "1",
			"compile_kwd": string(variant),
		},
		MatchExprs: []interface{}{
			&types.MatchDenseExpr{
				VectorColumnName: fmt.Sprintf("q_%d_vec", dim),
				EmbeddingData:    vector,
				DistanceType:     "cosine",
				TopN:             topN,
				ExtraOptions:     map[string]interface{}{"min_score": minScore},
			},
		},
	}
	res, err := eng.Search(ctx, req)
	if err != nil {
		return kccommon.Product{}, 0, err
	}
	for _, c := range res.Chunks {
		p, ok := productFromChunkMap(c, tenant)
		if !ok || !p.Merged {
			continue
		}
		score, _ := c["score"].(float64)
		return p, score, nil
	}
	return kccommon.Product{}, 0, nil
}
