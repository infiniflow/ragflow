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
	"encoding/json"
	"fmt"
	"strconv"

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

// wikiSelectFields are the additional columns a wiki page carries (beyond
// compiledSelectFields) that must survive the doc→merge round-trip so the
// dataset-level merged rows keep the fields the artifact API and page renderers
// depend on (page_type_kwd/topic_kwd/title_kwd/...).
var wikiSelectFields = []string{
	"page_type_kwd", "topic_kwd", "title_kwd",
	"entity_names_kwd", "summary_with_weight",
	"related_kb_pages_kwd", "outlinks_kwd", "section_level_int",
}

// loadDocProductsLimit is the per-page size used when scrolling a single
// document's compiled rows. A document can compile more than this many rows, so
// LoadDocProducts pages until the engine returns fewer than a full page.
const loadDocProductsLimit = 5000

// LoadDocProducts returns the per-document compiled rows for a single document.
// It is bounded to one document, so the consumer never loads the whole KB. The
// results are paged so a document with more than loadDocProductsLimit rows is
// not silently truncated.
func (r engineReader) LoadDocProducts(ctx context.Context, tenant, kb, docID string) ([]kccommon.Product, error) {
	eng := r.eng
	if eng == nil {
		eng = engine.Get()
	}
	if eng == nil {
		return nil, nil
	}
	var out []kccommon.Product
	offset := 0
	for {
		res, err := eng.Search(ctx, &types.SearchRequest{
			IndexNames:   []string{fmt.Sprintf("ragflow_%s", tenant)},
			KbIDs:        []string{kb},
			Filter:       map[string]interface{}{"doc_id": docID},
			SelectFields: append(append([]string(nil), compiledSelectFields...), wikiSelectFields...),
			Limit:        loadDocProductsLimit,
			Offset:       offset,
		})
		if err != nil {
			return nil, err
		}
		for _, c := range res.Chunks {
			// Only compiled products carry compile_kwd; skip ordinary source chunks.
			if _, ok := c["compile_kwd"]; !ok {
				continue
			}
			if p, ok := productFromChunkMap(c, tenant); ok {
				out = append(out, p)
			}
		}
		if len(res.Chunks) < loadDocProductsLimit {
			break
		}
		offset += loadDocProductsLimit
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
	merged := isMerged(c["kc_merged"])

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
		// slug_kwd is the full "<page_type>/<slug>" form (Python writer
		// contract); reconstruct it verbatim so the round-trip stays full-form.
		meta["slug"] = v
	}
	// Restore wiki page fields so the merged product (and hence the dataset-level
	// merged row) retains the metadata the artifact API and page renderers read.
	if v, ok := c["page_type_kwd"].(string); ok && v != "" {
		meta["page_type"] = v
	}
	if v, ok := c["topic_kwd"].(string); ok && v != "" {
		meta["topic"] = v
	}
	if v, ok := c["title_kwd"].(string); ok && v != "" {
		meta["title"] = v
	}
	if v, ok := c["summary_with_weight"].(string); ok && v != "" {
		meta["summary"] = v
	}
	if v := metaStringSlice(c, "entity_names_kwd"); len(v) > 0 {
		meta["entity_names"] = v
	}
	if v := metaStringSlice(c, "related_kb_pages_kwd"); len(v) > 0 {
		meta["related_kb_pages"] = v
	}
	if v := metaStringSlice(c, "outlinks_kwd"); len(v) > 0 {
		meta["outlinks"] = v
	}
	if v, ok := metaInt(c, "section_level_int"); ok {
		meta["section_level"] = v
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
		SelectFields: append([]string{"id", "doc_id", "kb_id", "content_with_weight", "kc_payload",
			"name_kwd", "entity_type_kwd", "from_entity_kwd", "to_entity_kwd", "slug_kwd",
			"type", "source_chunk_ids", "source_doc_ids", "kc_merged", "compile_kwd"},
			wikiSelectFields...),
		Filter: map[string]interface{}{
			"kc_merged":   1,
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
		score := toFloat64(c["score"])
		return p, score, nil
	}
	return kccommon.Product{}, 0, nil
}

// isMerged normalizes the boxed kc_merged field returned by the DocEngine,
// which may be stored as a string ("1"/"0"/"true"), a bool, or a numeric,
// depending on backend and mapping. Returns true for any positive/true form.
func isMerged(v interface{}) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		switch t {
		case "1", "true", "True", "TRUE":
			return true
		}
		if f, err := strconv.ParseFloat(t, 64); err == nil {
			return f > 0
		}
		return false
	case int:
		return t > 0
	case int64:
		return t > 0
	case float64:
		return t > 0
	case float32:
		return t > 0
	case json.Number:
		if f, err := t.Float64(); err == nil {
			return f > 0
		}
	}
	return false
}

// toFloat64 normalizes the boxed score field returned by the DocEngine into a
// float64, accepting float32, float64, numeric strings, and json.Number. It
// returns 0 when the value is missing or not numeric.
func toFloat64(v interface{}) float64 {
	switch t := v.(type) {
	case nil:
		return 0
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case string:
		if f, err := strconv.ParseFloat(t, 64); err == nil {
			return f
		}
	case json.Number:
		if f, err := t.Float64(); err == nil {
			return f
		}
	}
	return 0
}
