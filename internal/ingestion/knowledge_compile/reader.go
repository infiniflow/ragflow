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
	"errors"
	"fmt"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"

	"gorm.io/gorm"
)

// Reader finds the compiled products needed for incremental dedup without
// loading the whole KB into memory (§11.6 step 1, §11.7 incremental re-dedup).
//
// The dedup between an incoming per-document product and the already-merged
// rows lives in external storage (DocEngine): the consumer only keeps the
// in-flight batch in memory and asks the engine for nearest matches via KNN.
// It never scans every compiled chunk of the KB, which would OOM on a large
// knowledge base — this mirrors _struct_doc_storage_dedup_batch, which
// takes only the just-compiled docs and KNN-queries the store per doc.
type Reader interface {
	// LoadDocProducts returns the per-document compiled rows for a single
	// document (doc_id == source_doc). Bounded by one document, never the whole
	// KB.
	LoadDocProducts(ctx context.Context, tenant, kb, docID string) ([]kccommon.Product, error)
	// LoadDocProductsByDocs returns the per-document compiled rows for many
	// documents in one query, grouped by doc_id. It is the batch form of
	// LoadDocProducts used to avoid one query per document.
	LoadDocProductsByDocs(ctx context.Context, tenant, kb string, docIDs []string) (map[string][]kccommon.Product, error)

	// SearchSimilar runs a dense (KNN) search over the existing merged rows of
	// the given variant and returns the single most-similar row whose score is
	// at least minScore, plus that score. It returns a zero Product when nothing
	// clears the threshold. This mirrors _struct_doc_storage_knn_candidate
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

// enabledDocumentIDs returns the documents that may contribute to dataset-level
// Wiki products. Wiki staging rows intentionally keep available_int=0 even when
// their document is disabled, so availability alone cannot identify active
// contributions.
//
// The bool reports whether the database is configured. Offline/unit readers do
// not have document status available and retain the previous engine-only behavior.
func enabledDocumentIDs(ctx context.Context, db *gorm.DB, datasetID string) ([]string, bool, error) {
	if db == nil {
		return nil, false, nil
	}
	var ids []string
	err := db.WithContext(ctx).
		Model(&entity.Document{}).
		Where("kb_id = ? AND (status IS NULL OR status <> ?)", datasetID, "0").
		Pluck("id", &ids).Error
	return ids, true, err
}

// engineReader loads the per-document compiled products through the injected
// DocEngine (§11.6 step 1, §11.7 incremental re-dedup). The engine is wired by
// the server assembly point (see WithDocEngine); the engine abstraction owns
// the storage schema, so this reader is not backend-specific. A reader without
// an engine fails fast instead of silently reporting "no products".
type engineReader struct {
	eng engine.DocEngine
	// db is the injected MySQL handle backing enabledDocumentIDs. It is wired by
	// WithStorage; a zero-value reader has a nil db and enabledDocumentIDs
	// reports "status unavailable" (the previous engine-only behavior).
	db *gorm.DB
}

// compiledSelectFields are the columns needed to reconstruct a Product. Every
// name must exist in the target engine schema (Infinity rejects a SELECT of an
// unknown column); payload = content_with_weight, kind = compile_kwd.
var compiledSelectFields = []string{
	"id", "doc_id", "compile_kwd",
	"available_int",
	"content_with_weight",
	"source_chunk_ids", "source_doc_ids",
	"name_kwd", "entity_type_kwd", "from_entity_kwd", "to_entity_kwd",
	"slug_kwd",
	"create_timestamp_flt", "create_time",
	"compilation_template_kind_kwd", "compilation_template_ids",
	// The product kind discriminator for structure/tree: the component stores it
	// under type_kwd (structure: graph/entity/relation) / raptor_kwd
	// (tree: root/summary). Without these the reader cannot restore Meta["kind"]
	// and the dataset-nav dispatch would skip structure/tree products (B2).
	"type_kwd", "knowledge_graph_kwd", "raptor_kwd",
	// mention_count_int round-trips the entity mention count for reprojection.
	// (relation type lives in the content_with_weight payload, matching
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
// graph keeps accumulating cross-run duplicates. ES takes the wildcard verbatim
// in _source; the Infinity engine expands it against the table's real columns
// because Infinity's SQL binder rejects a partial wildcard (3013).
var wikiSelectFields = []string{
	"page_type_kwd", "entity_type_kwd", "type_kwd", "topic_kwd", "plan_group_kwd", "generation_kwd", "title_kwd",
	"entity_names_kwd", "summary_with_weight",
	"related_kb_pages_kwd", "outlinks_kwd", "depth_int",
	"q_*_vec",
}

// loadDocProductsLimit is the per-page size used when scrolling a single
// document's compiled rows. A document can compile more than this many rows, so
// LoadDocProducts pages until the engine returns fewer than a full page.
const loadDocProductsLimit = 5000

// pageThrough issues req against eng, paging with plain from/size, and invokes
// visit for every page's chunks. It stops after the final (short) page so a
// result set larger than one page is read completely without silent
// truncation or looping. req.Offset is advanced on each iteration; callers must
// not set it beforehand. req.Limit is left to the caller (typically
// loadDocProductsLimit).
func pageThrough(ctx context.Context, eng engine.DocEngine, req *types.SearchRequest, limit int, visit func(chunks []map[string]interface{}) error) error {
	for offset := 0; ; offset += limit {
		req.Offset = offset
		res, err := eng.Search(ctx, req)
		if err != nil {
			return err
		}
		if res != nil {
			if err := visit(res.Chunks); err != nil {
				return err
			}
		}
		if len(res.Chunks) < limit {
			return nil
		}
	}
}

// LoadDocProducts returns the per-document compiled rows for a single document.
// It is bounded to one document, so the consumer never loads the whole KB. The
// results are paged so a document with more than loadDocProductsLimit rows is
// not silently truncated.
func (r engineReader) LoadDocProducts(ctx context.Context, tenant, kb, docID string) ([]kccommon.Product, error) {
	if r.eng == nil {
		return nil, errEngineNotConfigured
	}
	eng := r.eng
	var out []kccommon.Product
	req := &types.SearchRequest{
		IndexNames:   []string{fmt.Sprintf("ragflow_%s", tenant)},
		KbIDs:        []string{kb},
		Filter:       map[string]interface{}{"doc_id": docID},
		SelectFields: append(append([]string(nil), compiledSelectFields...), wikiSelectFields...),
		Limit:        loadDocProductsLimit,
	}
	if err := pageThrough(ctx, eng, req, loadDocProductsLimit, func(chunks []map[string]interface{}) error {
		for _, c := range chunks {
			// Only compiled products carry compile_kwd; skip ordinary source chunks.
			if _, ok := c["compile_kwd"]; !ok || types.IsNavigationRow(c) {
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
		return nil
	}); err != nil {
		return nil, err
	}
	// Diagnostics: count how many loaded per-doc products carry an embedding
	// and report the (deduped) vector dimensions. If this shows 0/empty while
	// per-doc rows in ES do have q_<dim>_vec, VectorFromChunkMap is failing to
	// restore the vector and the merged rows will end up embedding-less. Gated to
	// debug: the map construction below allocates even in quiet production.
	if common.IsDebugEnabled() {
		vecCount, dims := computeDocProductsVectorAudit(out)
		common.Info("knowledge_compile: LoadDocProducts vector audit",
			zap.String("kb_id", kb),
			zap.String("doc_id", docID),
			zap.Int("products", len(out)),
			zap.Int("with_vector", vecCount),
			zap.Any("vector_dims", dims))
	}
	return out, nil
}

// docProductsBatchSize bounds how many doc_ids are sent in a single IN filter so a
// batch read over a large KB does not build an unbounded query.
const docProductsBatchSize = 500

// LoadDocProductsByDocs returns the per-document compiled products for many
// documents in one query (doc_id IN [...]) instead of one query per document.
// Results are grouped by doc_id; documents with no compiled rows map to an empty
// (non-nil) slice so callers can distinguish "no products" from "missing".
//
// Each doc_id IN sub-batch is read with a single bounded query. Because plain
// from/size cannot page past the engine result window (MAX_RESULT_WINDOW), a
// sub-batch whose aggregated compiled rows would force a page beyond the window
// falls back to one bounded per-document read per document (the pre-PR behavior)
// instead of erroring — a product-dense large KB must not lose or mis-read
// merged state during a rebuild.
func (r engineReader) LoadDocProductsByDocs(ctx context.Context, tenant, kb string, docIDs []string) (map[string][]kccommon.Product, error) {
	if r.eng == nil {
		return nil, errEngineNotConfigured
	}
	if len(docIDs) == 0 {
		return map[string][]kccommon.Product{}, nil
	}
	out := make(map[string][]kccommon.Product, len(docIDs))
	for _, docID := range docIDs {
		out[docID] = nil // ensure every requested doc has an entry
	}
	fields := append(append([]string(nil), compiledSelectFields...), wikiSelectFields...)
	for start := 0; start < len(docIDs); start += docProductsBatchSize {
		end := start + docProductsBatchSize
		if end > len(docIDs) {
			end = len(docIDs)
		}
		batch := docIDs[start:end]
		local, overflow, err := r.loadDocProductsBatch(ctx, tenant, kb, batch, fields)
		if err != nil {
			return nil, err
		}
		if overflow {
			// The sub-batch's compiled rows exceed the engine result window, so a
			// single doc_id IN read cannot page past it without erroring. Fall
			// back to one bounded per-document read per document: each document
			// is bounded to itself and never aggregates to the window, so the
			// read stays correct for product-dense batches.
			for _, docID := range batch {
				ps, perr := r.LoadDocProducts(ctx, tenant, kb, docID)
				if perr != nil {
					return nil, perr
				}
				out[docID] = ps
			}
			continue
		}
		for docID, ps := range local {
			out[docID] = ps
		}
	}
	return out, nil
}

// errBatchOverflow is returned by loadDocProductsBatch's paging visitor to signal
// that the next page would exceed the engine result window; the caller falls
// back to per-document reads instead of letting the engine reject the query.
var errBatchOverflow = errors.New("knowledge_compile: batch read reached result window")

// loadDocProductsBatch reads the compiled products for a slice of doc_ids with a
// single doc_id IN query, paging with plain from/size. It returns the products
// grouped by doc_id. If the aggregate result set is large enough that the next
// page would require offset+limit > MAX_RESULT_WINDOW (which the engine rejects
// for plain from/size), it stops paging early and reports overflow=true so the
// caller can fall back to per-document reads rather than erroring.
func (r engineReader) loadDocProductsBatch(ctx context.Context, tenant, kb string, batch []string, fields []string) (map[string][]kccommon.Product, bool, error) {
	local := make(map[string][]kccommon.Product, len(batch))
	req := &types.SearchRequest{
		IndexNames:   []string{fmt.Sprintf("ragflow_%s", tenant)},
		KbIDs:        []string{kb},
		Filter:       map[string]interface{}{"doc_id": docIDInterfaceSlice(batch), "exists": "compile_kwd"},
		SelectFields: fields,
		Limit:        loadDocProductsLimit,
	}
	overflow := false
	err := pageThrough(ctx, r.eng, req, loadDocProductsLimit, func(chunks []map[string]interface{}) error {
		// The next page would land at offset+limit, and that fetch needs
		// (offset+limit)+limit <= MAX_RESULT_WINDOW. If it would not, stop now
		// and let the caller fall back to per-document reads for this batch.
		// A next page (offset+limit) is only fetched when this page is full, and
		// only then could a deep page exceed the engine result window. Signal
		// overflow only in that case, so a short final page never triggers the
		// per-document fallback.
		if len(chunks) == loadDocProductsLimit && req.Offset+2*loadDocProductsLimit > common.MAX_RESULT_WINDOW {
			overflow = true
			return errBatchOverflow
		}
		for _, c := range chunks {
			// Only compiled products carry compile_kwd; skip ordinary source chunks.
			if _, ok := c["compile_kwd"]; !ok || types.IsNavigationRow(c) {
				continue
			}
			rowVariant, verr := KwdToVariant(asString(c["compile_kwd"]))
			if verr != nil {
				continue
			}
			if p, ok := productFromChunkMap(c, tenant, rowVariant); ok {
				if docID, _ := c["doc_id"].(string); docID != "" {
					local[docID] = append(local[docID], p)
				}
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errBatchOverflow) {
			return local, true, nil
		}
		return nil, false, err
	}
	return local, overflow, nil
}

// docIDInterfaceSlice converts a string doc_id list into the []interface{} form
// the engine's IN filter expects.
func docIDInterfaceSlice(docIDs []string) []interface{} {
	out := make([]interface{}, len(docIDs))
	for i, d := range docIDs {
		out[i] = d
	}
	return out
}

func (r engineReader) LoadMergedProduct(ctx context.Context, tenant, kb, id string) (kccommon.Product, error) {
	if r.eng == nil {
		return kccommon.Product{}, errEngineNotConfigured
	}
	eng := r.eng
	if id == "" {
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
	if r.eng == nil {
		return nil, errEngineNotConfigured
	}
	eng := r.eng
	var pages []kccommon.Product
	req := &types.SearchRequest{
		IndexNames: []string{fmt.Sprintf("ragflow_%s", tenant)}, KbIDs: []string{kb},
		Limit:        loadDocProductsLimit,
		SelectFields: append(append([]string(nil), compiledSelectFields...), wikiSelectFields...),
		Filter:       map[string]interface{}{"available_int": 1, "scope_kwd": "dataset", "type_kwd": compileKwdWikiPage},
	}
	if err := pageThrough(ctx, eng, req, loadDocProductsLimit, func(chunks []map[string]interface{}) error {
		for _, row := range chunks {
			if product, ok := productFromChunkMap(row, tenant, kccommon.VariantWiki); ok && kccommon.MetaString(product.Meta, "kind") == "page" {
				pages = append(pages, product)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return pages, nil
}

func (r engineReader) LoadDocumentWikiPagesBySlugs(ctx context.Context, tenant, kb string, slugs []string) ([]kccommon.Product, error) {
	if r.eng == nil {
		return nil, errEngineNotConfigured
	}
	eng := r.eng
	if len(slugs) == 0 {
		return nil, nil
	}
	enabledIDs, statusAvailable, err := enabledDocumentIDs(ctx, r.db, kb)
	if err != nil {
		return nil, fmt.Errorf("load enabled Wiki documents: %w", err)
	}
	if statusAvailable && len(enabledIDs) == 0 {
		return nil, nil
	}
	filter := map[string]interface{}{
		"available_int": 0,
		"type_kwd":      compileKwdWikiPage,
		"slug_kwd":      slugs,
	}
	if statusAvailable {
		filter["doc_id"] = enabledIDs
	}
	var pages []kccommon.Product
	req := &types.SearchRequest{
		IndexNames: []string{fmt.Sprintf("ragflow_%s", tenant)}, KbIDs: []string{kb},
		Limit:        loadDocProductsLimit,
		SelectFields: append(append([]string(nil), compiledSelectFields...), wikiSelectFields...),
		Filter:       filter,
	}
	if err := pageThrough(ctx, eng, req, loadDocProductsLimit, func(chunks []map[string]interface{}) error {
		for _, row := range chunks {
			if product, ok := productFromChunkMap(row, tenant, kccommon.VariantWiki); ok && kccommon.MetaString(product.Meta, "kind") == "page" {
				pages = append(pages, product)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return pages, nil
}

// productFromChunkMap reconstructs a kccommon.Product from a stored compiled
// chunk document. It reads the payload from content_with_weight and the
// embedding from the q_<dim>_vec column.
//
// expect is the variant the caller is querying for. The stored compile_kwd is
// reverse-mapped via KwdToVariant and compared against expect; a mismatch (or
// an unknown/dirty compile_kwd that does not map to any known variant) causes
// the row to be skipped. This is the canonical dirty-row contract promised by
// the plan: we never rely on the raw string equality alone, so unknown kinds
// are rejected consistently rather than leaking into the wrong bucket.
func productFromChunkMap(c map[string]interface{}, tenant string, expect kccommon.Variant) (kccommon.Product, bool) {
	content, _ := c["content_with_weight"].(string)
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
	// Preserve compilation metadata for the dataset merge.
	merged := isAvailable(c["available_int"])

	meta := map[string]any{}
	var payload map[string]any
	if expect == kccommon.VariantStructure || expect == kccommon.VariantMindmap {
		if err := json.Unmarshal([]byte(content), &payload); err != nil {
			if strings.HasPrefix(strings.TrimSpace(content), "{") {
				common.Warn("knowledge_compile: failed to decode structure payload",
					zap.String("row_id", id), zap.String("compile_kwd", variant), zap.Error(err))
			}
			payload = nil
		}
	}
	if v := asString(c["compile_kwd"]); v != "" {
		meta["compile_kwd"] = v
	}
	if v, ok := c["name_kwd"].(string); ok && v != "" {
		name := v
		for _, key := range []string{"name", "text", "term", "title"} {
			if extracted, ok := payload[key].(string); ok && extracted != "" && normalizedStructureEntityName(extracted) == normalizedStructureEntityName(v) {
				name = strings.TrimSpace(extracted)
				break
			}
		}
		meta["name"] = name
	}
	if v, ok := c["entity_type_kwd"].(string); ok && v != "" {
		if expect == kccommon.VariantMindmap && v == "mindmap" {
			v = "mind_map"
		}
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
	if kccommon.MetaString(meta, "kind") == "relation" {
		if v, ok := payload["type"].(string); ok && v != "" {
			meta["relation_type"] = v
		}
	}
	if v, ok := kccommon.MetaInt(c, "mention_count_int"); ok {
		meta["mention_count"] = v
	}
	if v, ok := c["slug_kwd"].(string); ok && v != "" {
		// slug_kwd is the full "<page_type>/<slug>" form ( writer
		// contract); reconstruct it verbatim so the round-trip stays full-form.
		meta["slug"] = v
	}
	// Restore wiki page fields so the merged product (and hence the dataset-level
	// merged row) retains the metadata the artifact API and page renderers read.
	if v := types.WikiPageCategory(c); v != "" {
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
	if v := kccommon.MetaStringSlice(c, "entity_names_kwd"); len(v) > 0 {
		meta["entity_names"] = v
	}
	if v := kccommon.MetaStringSlice(c, "related_kb_pages_kwd"); len(v) > 0 {
		meta["related_kb_pages"] = v
	}
	if v := kccommon.MetaStringSlice(c, "outlinks_kwd"); len(v) > 0 {
		meta["outlinks"] = v
	}
	// Section depth comes from depth_int. Only the wiki variant consumes
	// Meta["section_level"] (applyVariantColumns); tree rows also stamp depth_int,
	// but they keep their depth in raptor_layer_int.
	if expect == kccommon.VariantWiki {
		if v, ok := kccommon.MetaInt(c, "depth_int"); ok {
			meta["section_level"] = v
		}
	}
	// Restore the structure/tree product kind. The component stores it under
	// type_kwd (structure: graph/entity/relation) / raptor_kwd (tree:
	// root/summary), and the dataset-nav dispatch (B2) keys off Meta["kind"] to
	// pick the root/graph summary. Prefer these over the generic entity default.
	// Use asString (not a bare type assertion): the engine may return a
	// list-wrapped keyword column (review Major).
	if v := asString(c["type_kwd"]); v != "" {
		meta["kind"] = v
	} else if v := asString(c["knowledge_graph_kwd"]); v != "" {
		meta["kind"] = v
	} else if v := asString(c["raptor_kwd"]); v != "" {
		meta["kind"] = v
	} else if _, ok := meta["kind"]; !ok {
		if _, hasName := meta["name"]; hasName {
			meta["kind"] = "entity"
		}
	}
	// Page and section roles share the wiki compilation kind.
	if types.CompilationRowType(c) == compileKwdWikiPage {
		meta["kind"] = "page"
	} else if types.CompilationRowType(c) == compileKwdWikiSection {
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
	if v := kccommon.MetaStringSlice(c, "source_chunk_ids"); len(v) > 0 {
		meta["source_chunk_ids"] = v
	}
	if v := kccommon.MetaStringSlice(c, "source_doc_ids"); len(v) > 0 {
		meta["source_doc_ids"] = v
	}

	vec, _ := kccommon.VectorFromChunkMap(c, 0)
	// Restore the producing compilation kind for routing and merging.
	kind := types.CompilationKind(c)
	// Restore the compilation template id (from compilation_template_ids) so the
	// dataset merge can bucket structure rows per template and read/delete paths
	// can filter by it. A row should carry exactly one template id; if it carries
	// more than one the first is used (multi-template rows are a config error
	// surfaced elsewhere).
	templateID := ""
	if ids := kccommon.MetaStringSlice(c, "compilation_template_ids"); len(ids) > 0 {
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
	if r.eng == nil {
		return kccommon.Product{}, 0, errEngineNotConfigured
	}
	eng := r.eng
	if topN <= 0 {
		topN = 1
	}
	dim := len(vector)
	req := &types.SearchRequest{
		IndexNames: []string{fmt.Sprintf("ragflow_%s", tenant)},
		KbIDs:      []string{kb},
		Limit:      topN,
		SelectFields: append([]string{"id", "doc_id", "kb_id", "content_with_weight",
			"name_kwd", "entity_type_kwd", "from_entity_kwd", "to_entity_kwd", "slug_kwd",
			"source_chunk_ids", "source_doc_ids", "available_int", "compile_kwd", "type_kwd",
			"create_timestamp_flt", "create_time"},
			wikiSelectFields...),
		Filter: map[string]interface{}{
			"available_int": 1,
			"compile_kwd":   compileKwdForVariant(variant),
		},
		MatchExprs: []interface{}{
			&types.MatchDenseExpr{
				VectorColumnName: common.VectorColumnName(dim),
				EmbeddingData:    vector,
				DistanceType:     "cosine",
				TopN:             topN,
				ExtraOptions:     map[string]interface{}{"min_score": minScore},
			},
		},
	}
	if variant == kccommon.VariantWiki {
		req.Filter["type_kwd"] = compileKwdWikiPage
	}
	res, err := eng.Search(ctx, req)
	if err != nil {
		return kccommon.Product{}, 0, err
	}
	// Verify the row family and role even when a backend returns foreign rows.
	expectKwd := compileKwdForVariant(variant)
	for _, c := range res.Chunks {
		if types.CompilationKind(c) != expectKwd || (variant == kccommon.VariantWiki && types.CompilationRowType(c) != compileKwdWikiPage) {
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
