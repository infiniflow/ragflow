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

	"go.uber.org/zap"

	"ragflow/internal/common"
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

// mergedProductReader is an optional extension used by entity-mode Wiki. It
// loads one stable dataset-level page by id; keeping it separate from Reader
// avoids forcing test/offline readers and non-Wiki variants to implement a
// dataset-row lookup they do not need.
type mergedProductReader interface {
	LoadMergedProduct(ctx context.Context, tenant, kb, id string) (kccommon.Product, error)
}

type mergedWikiPageReader interface {
	LoadMergedWikiPages(ctx context.Context, tenant, kb string) ([]kccommon.Product, error)
}

type documentWikiPageReader interface {
	LoadDocumentWikiPagesBySlugs(ctx context.Context, tenant, kb string, slugs []string) ([]kccommon.Product, error)
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
//
// wiki_incremental port: the list also selects `kc_kind` and
// `create_timestamp_flt` (+`create_time`) so the reader can round-trip the
// wiki product kind (page/section) and the original creation timestamp without
// re-deriving them (see productFromChunkMap). Without these in the SELECT list,
// the stored values are invisible to the reader and every merged row would fall
// back to the compile_kwd-derived kind / a fresh now() timestamp — which both
// breaks the page/section filter and re-stamps creation time on every rebuild.
var compiledSelectFields = []string{
	"id", "doc_id", "tenant_id", "compile_kwd",
	"available_int",
	"content_with_weight", "kc_payload",
	"source_chunk_ids", "source_doc_ids",
	"name_kwd", "entity_type_kwd", "from_entity_kwd", "to_entity_kwd",
	"slug_kwd", "type",
	"kc_kind", "create_timestamp_flt", "create_time",
	"compilation_template_kind_kwd", "compilation_template_ids",
	// The product kind discriminator for structure/tree: the component stores it
	// under knowledge_graph_kwd (structure: graph/entity/relation) / raptor_kwd
	// (tree: root/summary). Without these the reader cannot restore Meta["kind"]
	// and the dataset-nav dispatch would skip structure/tree products (B2).
	"knowledge_graph_kwd", "raptor_kwd",
	// mention_count_int round-trips the entity mention count for reprojection.
	// (relation type lives in the content_with_weight payload, matching Python —
	// there is NO relation_type_kwd column.)
	"mention_count_int",
}

// wikiSelectFields are the additional columns a wiki page carries (beyond
// compiledSelectFields) that must survive the doc→merge round-trip so the
// dataset-level merged rows keep the fields the artifact API and page renderers
// depend on (page_type_kwd/topic_kwd/title_kwd/...).
//
// "q_*_vec" selects the (dimension-agnostic) embedding column. Without it,
// LoadDocProducts reconstructs products with an empty Vector, so merged rows
// carry no embedding and the dataset-level KNN dedup (SearchSimilar on
// available_int=1 + q_<dim>_vec) can never match an existing wiki page — the
// graph keeps accumulating cross-run duplicates. ES accepts the wildcard in
// both _source includes and the fields parameter.
var wikiSelectFields = []string{
	"page_type_kwd", "topic_kwd", "plan_group_kwd", "generation_kwd", "title_kwd",
	"entity_names_kwd", "summary_with_weight",
	"related_kb_pages_kwd", "outlinks_kwd", "section_level_int",
	"q_*_vec",
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
			// Reverse-map the row's compile_kwd; reject dirty/unknown kinds
			// (KwdToVariant error) so a malformed row never loads as a product.
			rowVariant, verr := KwdToVariant(asString(c["compile_kwd"]))
			if verr != nil {
				continue
			}
			if p, ok := productFromChunkMap(c, tenant, rowVariant); ok {
				out = append(out, p)
			}
		}
		if len(res.Chunks) < loadDocProductsLimit {
			break
		}
		offset += loadDocProductsLimit
	}
	// Diagnostics: count how many loaded per-doc products carry an embedding
	// and report the (deduped) vector dimensions. If this shows 0/empty while
	// per-doc rows in ES do have q_<dim>_vec, VectorFromChunkMap is failing to
	// restore the vector and the merged rows will end up embedding-less.
	vecCount := 0
	dims := map[int]int{}
	for _, p := range out {
		if dim := len(p.Vector); dim > 0 {
			vecCount++
			dims[dim]++
		}
	}
	common.Info("knowledge_compile: LoadDocProducts vector audit",
		zap.String("kb_id", kb),
		zap.String("doc_id", docID),
		zap.Int("products", len(out)),
		zap.Int("with_vector", vecCount),
		zap.Any("vector_dims", dims))
	return out, nil
}

func (r engineReader) LoadMergedProduct(ctx context.Context, tenant, kb, id string) (kccommon.Product, error) {
	eng := r.eng
	if eng == nil {
		eng = engine.Get()
	}
	if eng == nil || id == "" {
		return kccommon.Product{}, nil
	}
	filter := map[string]interface{}{"id": id, "available_int": 1, "scope_kwd": "dataset"}
	res, err := eng.Search(ctx, &types.SearchRequest{
		IndexNames: []string{fmt.Sprintf("ragflow_%s", tenant)}, KbIDs: []string{kb}, Limit: 1,
		SelectFields: append(append([]string(nil), compiledSelectFields...), wikiSelectFields...),
		Filter:       filter,
	})
	if err != nil {
		return kccommon.Product{}, err
	}
	if res == nil || len(res.Chunks) == 0 {
		return kccommon.Product{}, nil
	}
	product, ok := productFromChunkMap(res.Chunks[0], tenant, kccommon.VariantWiki)
	if !ok {
		return kccommon.Product{}, nil
	}
	return product, nil
}

func (r engineReader) LoadMergedWikiPages(ctx context.Context, tenant, kb string) ([]kccommon.Product, error) {
	eng := r.eng
	if eng == nil {
		eng = engine.Get()
	}
	if eng == nil {
		return nil, nil
	}
	filter := map[string]interface{}{"available_int": 1, "scope_kwd": "dataset", "compile_kwd": compileKwdWikiPage}
	var pages []kccommon.Product
	for offset := 0; ; offset += loadDocProductsLimit {
		res, err := eng.Search(ctx, &types.SearchRequest{
			IndexNames: []string{fmt.Sprintf("ragflow_%s", tenant)}, KbIDs: []string{kb},
			Limit: loadDocProductsLimit, Offset: offset,
			OrderBy:      (&types.OrderByExpr{}).Asc("id"),
			SelectFields: append(append([]string(nil), compiledSelectFields...), wikiSelectFields...),
			Filter:       filter,
		})
		if err != nil {
			return nil, err
		}
		if res == nil || len(res.Chunks) == 0 {
			break
		}
		for _, row := range res.Chunks {
			if product, ok := productFromChunkMap(row, tenant, kccommon.VariantWiki); ok && metaString(product.Meta, "kind") == "page" {
				pages = append(pages, product)
			}
		}
		if len(res.Chunks) < loadDocProductsLimit {
			break
		}
	}
	return pages, nil
}

func (r engineReader) LoadDocumentWikiPagesBySlugs(ctx context.Context, tenant, kb string, slugs []string) ([]kccommon.Product, error) {
	eng := r.eng
	if eng == nil {
		eng = engine.Get()
	}
	if eng == nil || len(slugs) == 0 {
		return nil, nil
	}
	filter := map[string]interface{}{
		"available_int": 0,
		"compile_kwd":   compileKwdWikiPage,
		"slug_kwd":      slugs,
	}
	var pages []kccommon.Product
	for offset := 0; ; offset += loadDocProductsLimit {
		res, err := eng.Search(ctx, &types.SearchRequest{
			IndexNames: []string{fmt.Sprintf("ragflow_%s", tenant)}, KbIDs: []string{kb},
			Limit: loadDocProductsLimit, Offset: offset,
			OrderBy:      (&types.OrderByExpr{}).Asc("id"),
			SelectFields: append(append([]string(nil), compiledSelectFields...), wikiSelectFields...),
			Filter:       filter,
		})
		if err != nil {
			return nil, err
		}
		if res == nil || len(res.Chunks) == 0 {
			break
		}
		for _, row := range res.Chunks {
			if product, ok := productFromChunkMap(row, tenant, kccommon.VariantWiki); ok && metaString(product.Meta, "kind") == "page" {
				pages = append(pages, product)
			}
		}
		if len(res.Chunks) < loadDocProductsLimit {
			break
		}
	}
	return pages, nil
}

// productFromChunkMap reconstructs a kccommon.Product from a stored compiled
// chunk document. It reads the payload from kc_payload (falling back to
// content_with_weight) and the embedding from the q_<dim>_vec column.
//
// expect is the variant the caller is querying for. The stored compile_kwd is
// reverse-mapped via KwdToVariant and compared against expect; a mismatch (or
// an unknown/dirty compile_kwd that does not map to any known variant) causes
// the row to be skipped. This is the canonical dirty-row contract promised by
// the plan: we never rely on the raw string equality alone, so unknown kinds
// are rejected consistently rather than leaking into the wrong bucket.
func productFromChunkMap(c map[string]interface{}, tenant string, expect kccommon.Variant) (kccommon.Product, bool) {
	content, _ := c["kc_payload"].(string)
	if content == "" {
		content, _ = c["content_with_weight"].(string)
	}
	if content == "" {
		return kccommon.Product{}, false
	}
	id, _ := c["id"].(string)
	docID, _ := c["doc_id"].(string)
	// Normalize through the shared asString helper (matching LoadDocProducts) so
	// a non-string scalar or a list-wrapped keyword column from the engine is
	// reverse-mapped consistently instead of being rejected as a dirty row.
	variant := asString(c["compile_kwd"])
	// Dirty-row contract: the raw compile_kwd must reverse-map to the expected
	// variant. An unknown/dirty kwd (KwdToVariant error) or a mapped variant
	// that differs from expect is rejected.
	mapped, err := KwdToVariant(variant)
	if err != nil || mapped != expect {
		return kccommon.Product{}, false
	}
	// Round-trip the raw compile_kwd (the inferred compile type / autotype:
	// list/set/hypergraph/timeline/mindmap/…) so the dataset-level merge can
	// stamp the SAME value on the dataset row as the doc row (Python _do_build
	// carries the doc row's compile_kwd through verbatim). Without this, the
	// merge falls back to compileKwdForVariant ("structure"/"mindmap"), which
	// diverges from the doc row's autotype ("hypergraph"/"timeline").
	merged := isAvailable(c["available_int"])

	meta := map[string]any{}
	// Preserve the raw compile_kwd (autotype) for the dataset merge (see the
	// round-trip note above). A non-string scalar from the engine is normalized
	// via asString, matching the variant reverse-map at the top of this func.
	if v := asString(c["compile_kwd"]); v != "" {
		meta["compile_kwd"] = v
	}
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
	if v, ok := metaInt(c, "mention_count_int"); ok {
		meta["mention_count"] = v
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
	// Restore the structure/tree product kind. The component stores it under
	// knowledge_graph_kwd (structure: graph/entity/relation) / raptor_kwd (tree:
	// root/summary), and the dataset-nav dispatch (B2) keys off Meta["kind"] to
	// pick the root/graph summary. Prefer these over the generic entity default.
	// Use asString (not a bare type assertion): the engine may return a
	// list-wrapped keyword column (review Major).
	if v := asString(c["knowledge_graph_kwd"]); v != "" {
		meta["kind"] = v
	} else if v := asString(c["raptor_kwd"]); v != "" {
		meta["kind"] = v
	} else if _, ok := meta["kind"]; !ok {
		if _, hasName := meta["name"]; hasName {
			meta["kind"] = "entity"
		}
	}
	// wiki_incremental port: round-trip the wiki product kind so the
	// dataset-level merge can reliably distinguish pages from sections. The
	// merged writer stores kc_kind; when present it is authoritative. Without
	// it (legacy rows), derive from compile_kwd: wiki_page -> "page",
	// wiki_section -> "section". This fix is what stops the processBatch
	// "Meta.kind==page" filter from deleting every wiki page (previously kind
	// was empty for wiki pages that had no entity/relation endpoint).
	if v := asString(c["kc_kind"]); v != "" {
		meta["kind"] = v
	} else if variant == compileKwdWikiPage {
		meta["kind"] = "page"
	} else if variant == compileKwdWikiSection {
		meta["kind"] = "section"
	}
	if v := asString(c["plan_group_kwd"]); v != "" {
		meta["plan_group"] = v
	}
	if v := asString(c["generation_kwd"]); v != "" {
		meta["generation"] = v
	}
	// wiki_incremental port: restore the original creation timestamp so a
	// page merge preserves it instead of stamping
	// a fresh now(). existing rows carry create_timestamp_flt (and optionally
	// a human-readable create_time string).
	if v, ok := metaFloat(c, "create_timestamp_flt"); ok {
		meta["created_at_unix"] = v
	}
	if v, ok := c["create_time"].(string); ok && v != "" {
		meta["created_at"] = v
	}
	if v := metaStringSlice(c, "source_chunk_ids"); len(v) > 0 {
		meta["source_chunk_ids"] = v
	}
	if v := metaStringSlice(c, "source_doc_ids"); len(v) > 0 {
		meta["source_doc_ids"] = v
	}

	vec, _ := kccommon.VectorFromChunkMap(c, 0)
	// Restore the authoritative template kind (compilation_template_kind_kwd)
	// so callers (e.g. RebuildDataset's variant recovery, B1a) can map it back
	// via KindToVariant instead of re-deriving from the ambiguous compile_kwd.
	kind := asString(c["compilation_template_kind_kwd"])
	// Restore the compilation template id (from compilation_template_ids) so the
	// dataset merge can bucket structure rows per template and read/delete paths
	// can filter by it. A row should carry exactly one template id; if it carries
	// more than one the first is used (multi-template rows are a config error
	// surfaced elsewhere).
	templateID := ""
	if ids := metaStringSlice(c, "compilation_template_ids"); len(ids) > 0 {
		templateID = ids[0]
	}
	return kccommon.Product{
		ID:         id,
		DocID:      docID,
		TenantID:   tenant,
		Variant:    expect,
		Kind:       kind,
		TemplateID: templateID,
		Content:    content,
		Vector:     vec,
		Meta:       meta,
		Merged:     merged,
	}, true
}

// SearchSimilar runs a dense KNN over the existing merged rows (available_int=1,
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
			"type", "source_chunk_ids", "source_doc_ids", "available_int", "compile_kwd",
			"kc_kind", "create_timestamp_flt", "create_time"},
			wikiSelectFields...),
		Filter: map[string]interface{}{
			"available_int": 1,
			"compile_kwd":   compileKwdForVariant(variant),
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
	// The KNN Filter scopes compile_kwd, but a foreign/legacy row that slipped
	// past it must not poison the merge candidate. Reject by the raw compile_kwd
	// (wiki_page vs wiki_section are distinct keywords even though both map to
	// VariantWiki); the page/section distinction is resolved downstream by
	// Meta.kind (see productFromChunkMap).
	expectKwd := compileKwdForVariant(variant)
	for _, c := range res.Chunks {
		// The KNN Filter already scopes compile_kwd, but a foreign/legacy row that
		// slipped past it must not poison the merge candidate. Reject by the
		// reverse-mapped variant (the dirty-row contract): productFromChunkMap
		// validates KwdToVariant(c) == variant and drops dirty/unknown kinds. The
		// raw-keyword check below is a fast pre-filter before the full product
		// reconstruction.
		if asString(c["compile_kwd"]) != expectKwd {
			continue
		}
		p, ok := productFromChunkMap(c, tenant, variant)
		if !ok || !p.Merged {
			continue
		}
		// The DocEngine stores the dense similarity in the "_score" key (mirroring
		// ES's _score), not "score". Reading the wrong key yields 0 and misleads
		// downstream diagnostics (KNN groups logging). The value is informational
		// only — KNN eligibility is enforced by the engine's min_score filter.
		score := toFloat64(c["_score"])
		return p, score, nil
	}
	return kccommon.Product{}, 0, nil
}

// isAvailable normalizes the boxed available_int field returned by the DocEngine,
// which may be stored as a string ("1"/"0"/"true"), a bool, or a numeric,
// depending on backend and mapping. Returns true for any positive/true form.
func isAvailable(v interface{}) bool {
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

// asString normalizes a boxed chunk-map value into a string (used where the
// backend may return a typed string, a json.Number, or a single-element list for
// a keyword column). A list-wrapped keyword (e.g. []string{"wiki_page"} or
// []any{"wiki_page"}) is unwrapped to its first element so reverse-mapping and
// raw-keyword filters behave consistently across engine backends.
func asString(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case []string:
		if len(t) == 1 {
			return t[0]
		}
		return ""
	case []interface{}:
		if len(t) == 1 {
			if s, ok := t[0].(string); ok {
				return s
			}
			return fmt.Sprintf("%v", t[0])
		}
		return ""
	}
	return fmt.Sprintf("%v", v)
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
