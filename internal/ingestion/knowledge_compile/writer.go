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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cespare/xxhash/v2"
	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/utility"
)

// Writer persists dataset-level merged products and removes them on document
// deletion (§11.7).
type Writer interface {
	// WriteMerged upserts the dataset-level merged products (available_int=1).
	WriteMerged(ctx context.Context, tenant, kb string, products []kccommon.Product) error
	// DeleteDocLevelForDocs drops every per-document (doc-level, available_int=0)
	// product of the deleted docs in a single DocEngine call. Dataset-level
	// merged rows are not targeted because their doc_id equals the kb, never a
	// deleted source doc id.
	DeleteDocLevelForDocs(ctx context.Context, tenant, kb string, deletedDocIDs []string) error
	// StripMergedSources removes deletedDocIDs from the source_doc_ids array of
	// every dataset-level (available_int=1) product for the dataset. It searches the
	// merged set once, rewrites the source array of every non-empty survivor in a
	// single update pass, and deletes (in one call) any product whose array
	// became empty.
	StripMergedSources(ctx context.Context, tenant, kb string, deletedDocIDs []string) error
	// ProjectWikiGraph reads every merged wiki_page product for the dataset and
	// re-materializes the wiki page graph (entities + relations) as
	// wiki_entity / wiki_relation compiled rows. It is a delete-then-insert
	// replacement: the DocEngine has no cross-delete/insert transaction boundary,
	// so the graph is rebuilt from a consistent read and the old graph is dropped
	// first. The graph is reconstructible, so the lack of atomicity is accepted.
	ProjectWikiGraph(ctx context.Context, tenant, kb string) error
	// DropWikiGraph deletes every wiki_entity / wiki_relation row for the dataset.
	DropWikiGraph(ctx context.Context, tenant, kb string) error
}

// engineWriter persists dataset-level merged products through the global
// DocEngine (§11.7). Like engineReader, it depends on the process-wide DocEngine
// obtained via engine.Get(); the storage schema lives behind the engine
// abstraction rather than in this package.
type engineWriter struct {
	eng engine.DocEngine
}

// writeMergedBatchSize bounds how many rows each parallel InsertChunks call
// carries, so the DocEngine write fan-out stays granular under the shared pool.
const writeMergedBatchSize = 200

func (w engineWriter) WriteMerged(ctx context.Context, tenant, kb string, products []kccommon.Product) error {
	if len(products) == 0 {
		return nil
	}
	// Dataset-level telemetry: break the merged set down by compile_kwd so a
	// missing wiki_page at query time can be traced to "WriteMerged never
	// received any wiki_page products" (generation/merge bug) rather than
	// "received then dropped" (downstream delete bug).
	byKwd := map[string]int{}
	for _, p := range products {
		byKwd[compileKwdForVariant(p.Variant)]++
	}
	common.Info("knowledge_compile: WriteMerged dataset-level products",
		zap.String("kb_id", kb),
		zap.String("tenant_id", tenant),
		zap.Int("total", len(products)),
		zap.Any("by_compile_kwd", byKwd),
	)
	eng := w.eng
	if eng == nil {
		eng = engine.Get()
	}
	if eng == nil {
		return nil
	}
	baseName := fmt.Sprintf("ragflow_%s", tenant)
	// Source-tracing fields (tasks/2026-08-07-wiki-duplicates-research.md §5.1):
	// one runID + inputHash + timestamp per WriteMerged call, shared by every
	// shard, so rows from the same execution share plan_kwd and reruns of the
	// same plan share input_hash_kwd.
	now := time.Now()
	runID := utility.GenerateUUID()
	inputHash := mergedInputHash(products)
	// Diagnostics: before building merged rows, confirm the incoming products
	// actually carry an embedding. mergedChunkMap only writes q_<dim>_vec when
	// len(p.Vector)>0; if this reports 0 vectors while LoadDocProducts/dedup
	// audits reported vectors, the vector is being dropped somewhere between
	// dedup and WriteMerged.
	{
		vecCount, dims := 0, map[int]int{}
		for _, p := range products {
			if dim := len(p.Vector); dim > 0 {
				vecCount++
				dims[dim]++
			}
		}
		common.Info("knowledge_compile: WriteMerged vector audit",
			zap.String("kb_id", kb),
			zap.Int("products", len(products)),
			zap.Int("with_vector", vecCount),
			zap.Any("vector_dims", dims))
	}
	// Shard the rows and drive the inserts through the shared global pool
	// (docengine-bounded) instead of one monolithic InsertChunks call.
	jobs := make([]CompilerJob, 0, (len(products)+writeMergedBatchSize-1)/writeMergedBatchSize)
	for start := 0; start < len(products); start += writeMergedBatchSize {
		end := start + writeMergedBatchSize
		if end > len(products) {
			end = len(products)
		}
		batch := products[start:end]
		jobs = append(jobs, func() error {
			chunks := make([]map[string]interface{}, 0, len(batch))
			for _, p := range batch {
				chunks = append(chunks, mergedChunkMap(tenant, kb, runID, inputHash, now, p))
			}
			// Telemetry: confirm the bytes actually handed to InsertChunks carry
			// compile_kwd=wiki_page (vs the WriteMerged stats that only reflect
			// the in-memory Variant). If this shows wiki_page but ES returns "",
			// the engine drops the field; if this shows "" too, the map is wrong.
			if len(chunks) > 0 {
				common.Info("knowledge_compile: WriteMerged chunk sample",
					zap.String("kb_id", kb),
					zap.String("compile_kwd", metaString(chunks[0], "compile_kwd")),
					zap.String("variant", string(batch[0].Variant)),
					zap.String("id", metaString(chunks[0], "id")),
				)
			}
			_, err := eng.InsertChunks(ctx, chunks, baseName, kb)
			return err
		})
	}
	return runCompilerJobs(ctx, jobs)
}

// mergedChunkMap builds the chunk-index document for a dataset-level merged
// product. It uses the dataset-level idempotency key (§11.6) as `id`, never the
// per-doc key, and is always available_int=1 (searchable).
//
// runID and inputHash carry the minimal source-tracing fields (see
// tasks/2026-08-07-wiki-duplicates-research.md §5.1): runID is the execution
// identifier shared by every row of one WriteMerged call (plan_kwd), inputHash
// is the canonical input fingerprint shared by reruns of the same plan
// (input_hash_kwd), and now stamps both wall-clock audit fields. We deliberately
// do not write the full PLAN JSON into the engine.
func mergedChunkMap(tenant, kb, runID, inputHash string, now time.Time, p kccommon.Product) map[string]interface{} {
	srcDocIDs := metaStringSlice(p.Meta, "source_doc_ids")
	srcChunkIDs := metaStringSlice(p.Meta, "source_chunk_ids")
	m := map[string]interface{}{
		"id":                   datasetLevelID(tenant, kb, p),
		"doc_id":               kb,
		"tenant_id":            tenant,
		"kb_id":                kb,
		"available_int":        1,
		"compile_kwd":          compileKwdForVariant(p.Variant),
		"content_with_weight":  p.Content,
		"kc_payload":           p.Content, // raw payload, for Reader reconstruction
		"source_doc_ids":       srcDocIDs,
		"source_chunk_ids":     srcChunkIDs,
		"plan_kwd":             runID,
		"input_hash_kwd":       inputHash,
		"create_time":          now.Format("2006-01-02 15:04:05"),
		"create_timestamp_flt": float64(now.Unix()),
	}
	// Carry the wiki page metadata onto the merged row so the dataset-level
	// products keep the fields the artifact API (ListArtifacts/ListWikiTopics)
	// and page renderers read. Without this the merged rows lose page_type_kwd /
	// topic_kwd / title_kwd and the compilation page would show no wiki pages
	// even though per-document products carry them.
	//
	// slug_kwd follows the Python writer contract (api/db/db_models.py): it is
	// stored as the full "<page_type>/<slug>" form so GetWikiPage's filter
	// (page_type + "/" + slug) matches directly.
	pageType := metaString(p.Meta, "page_type")
	if slug := metaString(p.Meta, "slug"); slug != "" {
		// Normalize to the full "<page_type>/<slug>" form (Python writer
		// contract). Idempotent: slugs that already carry the prefix are kept.
		fullSlug := slug
		if pageType != "" && !strings.Contains(slug, "/") {
			fullSlug = pageType + "/" + slug
		}
		m["slug_kwd"] = fullSlug
		m["artifact_slug_kwd"] = fullSlug
	}
	if v := metaString(p.Meta, "title"); v != "" {
		m["title_kwd"] = v
	}
	if pageType != "" {
		m["page_type_kwd"] = pageType
	}
	if v := metaString(p.Meta, "topic"); v != "" {
		m["topic_kwd"] = v
	}
	if v := metaString(p.Meta, "summary"); v != "" {
		m["summary_with_weight"] = v
	}
	if v := metaStringSlice(p.Meta, "entity_names"); len(v) > 0 {
		m["entity_names_kwd"] = v
	}
	if v := metaStringSlice(p.Meta, "related_kb_pages"); len(v) > 0 {
		m["related_kb_pages_kwd"] = v
	}
	if v := metaStringSlice(p.Meta, "outlinks"); len(v) > 0 {
		m["outlinks_kwd"] = v
	}
	// Persist the merged product's embedding under the dimension-suffixed column
	// used elsewhere in the index, so dataset-level rows remain vector-searchable
	// and the Reader can reconstruct them (otherwise the vector is silently
	// dropped and KNN search returns nothing for merged rows).
	if dim := len(p.Vector); dim > 0 {
		m[fmt.Sprintf("q_%d_vec", dim)] = p.Vector
	}
	return m
}

// DeleteDocLevelForDocs removes the per-document (doc-level) products of every
// deleted doc in a single DocEngine call. The table is scoped to the dataset
// (kb), and merged rows carry doc_id == kb, so filtering on doc_id IN
// deletedDocIDs can only match the per-document products of the deleted docs.
func (w engineWriter) DeleteDocLevelForDocs(ctx context.Context, tenant, kb string, deletedDocIDs []string) error {
	if len(deletedDocIDs) == 0 {
		return nil
	}
	eng := w.eng
	if eng == nil {
		eng = engine.Get()
	}
	if eng == nil {
		return nil
	}
	baseName := fmt.Sprintf("ragflow_%s", tenant)
	_, err := eng.DeleteChunks(ctx, map[string]interface{}{
		"doc_id": deletedDocIDs,
	}, baseName, kb)
	return err
}

// StripMergedSources removes deletedDocIDs from the source_doc_ids array of
// every dataset-level (available_int=1) product for the dataset. The query filters
// on source_doc_ids IN deletedDocIDs so the engine only returns rows that
// actually reference a deleted doc (intersection pushed down); the survivors'
// source arrays are rewritten in a single update pass driven by the shared
// pool, and any product whose array became empty is deleted in one call. The
// deleted docs' products themselves are never loaded into memory.
func (w engineWriter) StripMergedSources(ctx context.Context, tenant, kb string, deletedDocIDs []string) error {
	if len(deletedDocIDs) == 0 {
		return nil
	}
	eng := w.eng
	if eng == nil {
		eng = engine.Get()
	}
	if eng == nil {
		return nil
	}
	baseName := fmt.Sprintf("ragflow_%s", tenant)
	delSet := make(map[string]bool, len(deletedDocIDs))
	for _, d := range deletedDocIDs {
		delSet[d] = true
	}

	const batchSize = 2000
	var toDeleteIDs []string
	var jobs []CompilerJob
	offset := 0
	for {
		res, err := eng.Search(ctx, &types.SearchRequest{
			IndexNames: []string{baseName},
			KbIDs:      []string{kb},
			// available_int=1 isolates dataset-level rows; source_doc_ids IN
			// deletedDocIDs pushes the intersection test into the engine so only
			// rows that actually reference a deleted doc are returned (Infinity
			// array IN means "contains at least one of").
			Filter: map[string]interface{}{
				"available_int":  1,
				"source_doc_ids": deletedDocIDs,
			},
			SelectFields: []string{"id", "source_doc_ids"},
			Limit:        batchSize,
			Offset:       offset,
		})
		if err != nil {
			return err
		}
		if len(res.Chunks) == 0 {
			break
		}
		for _, c := range res.Chunks {
			id, _ := c["id"].(string)
			if id == "" {
				continue
			}
			src := metaStringSlice(c, "source_doc_ids")
			kept := make([]string, 0, len(src))
			changed := false
			for _, d := range src {
				if delSet[d] {
					changed = true
					continue
				}
				kept = append(kept, d)
			}
			if !changed {
				continue
			}
			if len(kept) == 0 {
				toDeleteIDs = append(toDeleteIDs, id)
				continue
			}
			keptCopy := append([]string(nil), kept...)
			idCopy := id
			jobs = append(jobs, func() error {
				return eng.UpdateChunks(ctx, map[string]interface{}{"id": idCopy},
					map[string]interface{}{"source_doc_ids": keptCopy}, baseName, kb)
			})
		}
		if len(res.Chunks) < batchSize {
			break
		}
		offset += batchSize
	}
	if err := runCompilerJobs(ctx, jobs); err != nil {
		return err
	}
	if len(toDeleteIDs) > 0 {
		if _, err := eng.DeleteChunks(ctx, map[string]interface{}{
			"id":    toDeleteIDs,
			"kb_id": kb,
		}, baseName, kb); err != nil {
			return err
		}
	}
	return nil
}

// canonicalKey derives a stable cluster key for a merged product.
func canonicalKey(p kccommon.Product) string {
	if slug, ok := p.Meta["slug"].(string); ok && slug != "" {
		return slug
	}
	if p.Meta["name"] != nil {
		name, _ := p.Meta["name"].(string)
		typ, _ := p.Meta["entity_type"].(string)
		if typ == "" {
			typ, _ = p.Meta["type"].(string)
		}
		if name != "" {
			return hashStr(name + "\x00" + typ)
		}
	}
	return hashStr(p.Content)
}

// datasetLevelID is the dataset-level idempotency key (§11.6): a stable hash of
// (tenant, kb, variant, canonical cluster key).
func datasetLevelID(tenant, kb string, p kccommon.Product) string {
	return hashStr(tenant + "\x00" + kb + "\x00" + string(p.Variant) + "\x00" + canonicalKey(p))
}

func hashStr(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// mergedInputHash derives a canonical SHA-256 fingerprint of a WriteMerged
// batch's input evidence: the union of source_doc_ids across all products,
// sorted and deduped. Reruns of the same plan (same input documents) share the
// fingerprint, so input_hash_kwd distinguishes "same plan content" across
// executions while plan_kwd separates individual runs.
func mergedInputHash(products []kccommon.Product) string {
	seen := make(map[string]struct{}, 16)
	docs := make([]string, 0, 16)
	for _, p := range products {
		for _, d := range metaStringSlice(p.Meta, "source_doc_ids") {
			if _, ok := seen[d]; ok {
				continue
			}
			seen[d] = struct{}{}
			docs = append(docs, d)
		}
	}
	sort.Strings(docs)
	return hashStr(strings.Join(docs, "\n"))
}

// metaString extracts a string from a map value, tolerating a missing or
// non-string entry.
func metaString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func metaStringSlice(m map[string]any, key string) []string {
	switch v := m[key].(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// metaInt extracts an integer from a map value that may be boxed as float64
// (JSON number), int64, string, or a typed int — the engine/JSON round-trip does
// not guarantee a single numeric type.
func metaInt(m map[string]any, key string) (int64, bool) {
	switch v := m[key].(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case float64:
		return int64(v), true
	case string:
		var n int64
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n, true
		}
	}
	return 0, false
}

// --- Wiki page graph materialization (wiki_entity / wiki_relation) ---

// compile_kwd values for the dataset-level products this package writes. The
// wiki variant compiles into "wiki_page" rows (per the Python writer contract
// that GetWikiAlteration / ListArtifacts / GetWikiGraph all filter on
// compile_kwd = "wiki_page"), while the page graph is materialized as the
// dedicated wiki_entity / wiki_relation buckets.
const (
	compileKwdWikiPage      = "wiki_page"
	compileKwdWikiEntity    = "wiki_entity"
	compileKwdWikiRelation  = "wiki_relation"
	compileKwdWikiPageGraph = "wiki_page_graph" // legacy Python blob, swept on drop
)

// wikiGraphBatchSize bounds how many graph rows each parallel InsertChunks call
// carries, mirroring writeMergedBatchSize.
const wikiGraphBatchSize = 200

// wikiGraphSourceDocCap bounds the source_doc_ids array stored on a graph row so
// a heavily-shared page does not accumulate an unbounded id list.
const wikiGraphSourceDocCap = 64

// compileKwdForVariant maps a compiler Variant to the compile_kwd stamped on the
// merged chunk document. The wiki variant is special-cased because all read
// paths filter on compile_kwd = "wiki_page", not the raw variant string "wiki".
func compileKwdForVariant(v kccommon.Variant) string {
	if v == kccommon.VariantWiki {
		return compileKwdWikiPage
	}
	return string(v)
}

// wikiGraphXXHash derives a stable 16-char hex id (matches Python
// xxh64().hexdigest()) for a graph node/edge namespaced under the kb.
func wikiGraphXXHash(namespace, kb, key string) string {
	return fmt.Sprintf("%016x", xxhash.Sum64String(namespace+":"+kb+":"+key))
}

// wikiGraphBareKey reduces a full "<page_type>/<slug>" identity (or a bare
// slug) to a canonical bare key used by the graph's reverse slug index. It
// strips any "<page_type>/" prefix and normalizes underscores to hyphens so a
// bare outlink ("dong-zhuo") matches the page whose slug_kwd is
// "entity/dong_zhuo" (or "entity/dong-zhuo") regardless of the LLM's
// underscore-vs-hyphen formatting. Empty strings yield "" (never a valid key).
func wikiGraphBareKey(slug string) string {
	s := strings.TrimSpace(slug)
	if s == "" {
		return ""
	}
	if idx := strings.LastIndex(s, "/"); idx >= 0 && idx < len(s)-1 {
		s = s[idx+1:]
	}
	s = strings.ReplaceAll(s, "_", "-")
	return strings.TrimSpace(s)
}

// wikiPageProjection is the subset of a merged wiki_page row that the graph
// projection needs. It is reconstructed from the stored display columns (the
// same fields GetWikiGraph reads back), not from the JSON payload.
type wikiPageProjection struct {
	Slug     string
	PageType string
	Title    string
	Aliases  []string
	Summary  string
	// Outlinks are the other wiki pages this page links to (by full
	// "<page_type>/<slug>" identity). These inter-page links are the SOLE source
	// of wiki_relation edges: a wiki_relation row exists iff some page lists
	// another page in its Outlinks (and that target page also exists). No edge
	// comes from entity extraction, co-occurrence, or semantic similarity.
	Outlinks []string
	// SourceDocIDs are the originating document ids that produced this page.
	// For a wiki_relation the stored source_doc_ids is the UNION of both
	// endpoints' SourceDocIDs, so the relation is dropped only once neither
	// endpoint traces to a surviving document.
	SourceDocIDs   []string
	SourceChunkIDs []string
}

// ProjectWikiGraph reads every merged wiki_page for the dataset and
// re-materializes the page graph. See the Writer interface doc for the
// delete-then-insert (non-atomic) contract.
func (w engineWriter) ProjectWikiGraph(ctx context.Context, tenant, kb string) error {
	pages, err := w.loadMergedWikiPages(ctx, tenant, kb)
	if err != nil {
		return err
	}
	// Telemetry: confirm how many merged wiki_page rows survived WriteMerged.
	// If this is 0 while WriteMerged reported wiki_page:N, the merged rows are
	// not queryable under (compile_kwd=wiki_page AND available_int=1) — point at the
	// stored field values, not a downstream delete.
	common.Info("knowledge_compile: ProjectWikiGraph load",
		zap.String("kb_id", kb),
		zap.Int("merged_wiki_pages_loaded", len(pages)))
	// Zero pages: the dataset has no wiki graph. Drop any stale graph rows and
	// return — a full reprojection of an empty set would only rewrite nothing.
	if len(pages) == 0 {
		return w.dropWikiGraph(ctx, tenant, kb)
	}
	rows, err := w.projectWikiGraphRows(ctx, tenant, kb, pages)
	if err != nil {
		return err
	}
	// delete-then-insert: drop the previous graph first, then write the new one.
	if err := w.dropWikiGraph(ctx, tenant, kb); err != nil {
		return err
	}
	return w.insertWikiGraphChunks(ctx, tenant, kb, rows)
}

// DropWikiGraph deletes every wiki_entity / wiki_relation row for the dataset.
func (w engineWriter) DropWikiGraph(ctx context.Context, tenant, kb string) error {
	return w.dropWikiGraph(ctx, tenant, kb)
}

// dropWikiGraph is the shared delete path for both ProjectWikiGraph (when the
// page set is empty, or before re-inserting) and DropWikiGraph. It deletes by
// kb_id + compile_kwd IN (the graph buckets), also sweeping any legacy
// wiki_page_graph blob left by an earlier Python writer so the index does not
// accumulate stale state. The dataset-wide full delete of merged rows is owned
// by DeleteDocLevelForKb elsewhere and is not repeated here.
func (w engineWriter) dropWikiGraph(ctx context.Context, tenant, kb string) error {
	eng := w.eng
	if eng == nil {
		eng = engine.Get()
	}
	if eng == nil {
		return nil
	}
	baseName := fmt.Sprintf("ragflow_%s", tenant)
	_, err := eng.DeleteChunks(ctx, map[string]interface{}{
		"kb_id": kb,
		"compile_kwd": []string{
			compileKwdWikiEntity,
			compileKwdWikiRelation,
			compileKwdWikiPageGraph,
		},
	}, baseName, kb)
	return err
}

// loadMergedWikiPages scrolls every merged wiki_page product for the dataset.
// It selects only the display columns the projection needs (slug / page_type /
// title / entity_names_kwd (aliases) / summary_with_weight / outlinks_kwd /
// source_doc_ids / source_chunk_ids), never the JSON payload.
func (w engineWriter) loadMergedWikiPages(ctx context.Context, tenant, kb string) ([]wikiPageProjection, error) {
	eng := w.eng
	if eng == nil {
		eng = engine.Get()
	}
	if eng == nil {
		return nil, nil
	}
	baseName := fmt.Sprintf("ragflow_%s", tenant)
	const batchSize = 2000
	var out []wikiPageProjection
	offset := 0
	kwdSeen := map[string]int{}
	for {
		res, err := eng.Search(ctx, &types.SearchRequest{
			IndexNames: []string{baseName},
			KbIDs:      []string{kb},
			Filter: map[string]interface{}{
				"compile_kwd":   compileKwdWikiPage,
				"available_int": 1,
				"kb_id":         kb,
			},
			SelectFields: []string{
				"slug_kwd", "page_type_kwd", "title_kwd",
				"entity_names_kwd", "summary_with_weight", "outlinks_kwd",
				"source_doc_ids", "source_chunk_ids",
				// compile_kwd is selected purely so the query-result telemetry
				// below can confirm the Search filter is honoured (without it the
				// "compile_kwd_seen" audit would always read as {"":n}).
				"compile_kwd",
			},
			Limit:  batchSize,
			Offset: offset,
			// Stable deterministic ordering so the deep-offset pages (beyond ES's
			// default result window) keep a consistent cursor and the engine can
			// switch to search_after instead of failing on deep offset.
			OrderBy: (&types.OrderByExpr{}).Asc("slug_kwd"),
		})
		if err != nil {
			return nil, err
		}
		// Telemetry: surface whether the Search filter is actually honoured. If
		// the returned rows carry compile_kwd other than wiki_page, the engine's
		// Filter map is being ignored and wiki_entity rows leak into the page
		// projection (which then has nothing under compile_kwd=wiki_page).
		for _, c := range res.Chunks {
			kwdSeen[metaString(c, "compile_kwd")]++
		}
		if len(res.Chunks) == 0 {
			break
		}
		for _, c := range res.Chunks {
			p := wikiPageProjection{
				Slug:           metaString(c, "slug_kwd"),
				PageType:       metaString(c, "page_type_kwd"),
				Title:          metaString(c, "title_kwd"),
				Aliases:        metaStringSlice(c, "entity_names_kwd"),
				Summary:        metaString(c, "summary_with_weight"),
				Outlinks:       metaStringSlice(c, "outlinks_kwd"),
				SourceDocIDs:   metaStringSlice(c, "source_doc_ids"),
				SourceChunkIDs: metaStringSlice(c, "source_chunk_ids"),
			}
			out = append(out, p)
		}
		if len(res.Chunks) < batchSize {
			break
		}
		offset += batchSize
	}
	common.Info("knowledge_compile: loadMergedWikiPages query result",
		zap.String("kb_id", kb),
		zap.Int("rows_returned", len(out)),
		zap.Any("compile_kwd_seen", kwdSeen))
	return out, nil
}

// projectWikiGraphRows builds the wiki_entity / wiki_relation chunk documents
// from a full projection of the dataset's merged wiki pages.
//
// Graph model (mirrors the Python writer in dataset_wiki_generator.py):
//   - One wiki_entity row per merged wiki page (node = page).
//   - One wiki_relation row per inter-page outlink: a page lists another page
//     in its Outlinks, and that target page also exists in the projection.
//     Thus the EDGES of this graph are exclusively the mutual links BETWEEN
//     wiki pages (page A outlinks to page B => edge A->B). No edge originates
//     from entity extraction, co-occurrence, or semantic similarity.
//
// Both entities and relations are keyed on the full "<page_type>/<slug>"
// identity (matching Go wiki input), so a bare slug collision across page types
// is impossible. A relation whose target page is absent from this projection (a
// dangling edge, e.g. the target was pruned/deleted) is skipped, as is a
// self-loop (src == tgt) — the graph is rebuilt from a consistent read each
// time, so cross-batch dangling edges cannot accumulate.
func (w engineWriter) projectWikiGraphRows(_ context.Context, tenant, kb string, pages []wikiPageProjection) ([]map[string]interface{}, error) {
	// Sort pages deterministically by full slug before building the indexes so
	// the outcome is reproducible across reprojections. The engine's row order
	// is otherwise scroll-order dependent, which made bare-slug first-match
	// resolution flip between runs.
	sort.SliceStable(pages, func(i, j int) bool {
		return pages[i].Slug < pages[j].Slug
	})
	bySlug := make(map[string]wikiPageProjection, len(pages))
	// Bare-slug index so inter-page outlinks match regardless of whether they
	// were emitted as a full "<page_type>/<slug>" identity or a bare slug. The
	// LLM writes wikitext links using bare slugs (and may mix "_" vs "-"), so
	// the wiki_page outlinks_kwd in ES is frequently bare; matching only the
	// full slug would silently drop every relation (graph has nodes, no edges).
	//
	// A bare key shared by two pages (e.g. "entity/foo" and "concept/foo" both
	// reduce to "foo") is ambiguous; such links resolve to nothing rather than
	// nondeterministically flipping between pages across runs.
	byBareSlug := make(map[string]wikiPageProjection, len(pages))
	bareAmbiguous := make(map[string]bool, len(pages))
	for _, p := range pages {
		if p.Slug == "" {
			continue
		}
		bySlug[p.Slug] = p
		if b := wikiGraphBareKey(p.Slug); b != "" {
			if prev, ok := byBareSlug[b]; ok {
				if prev.Slug != p.Slug {
					bareAmbiguous[b] = true
				}
			} else {
				byBareSlug[b] = p
			}
		}
	}

	rows := make([]map[string]interface{}, 0, len(bySlug))
	seen := make(map[string]bool, len(bySlug))
	for _, p := range bySlug {
		entityID := wikiGraphXXHash("wiki_entity", kb, p.Slug)
		if seen[entityID] {
			continue
		}
		seen[entityID] = true

		weight := len(p.Outlinks) // raw outlink count; 0 is allowed (Python parity)
		content, err := json.Marshal(map[string]any{
			"slug":      p.Slug,
			"page_type": p.PageType,
			"title":     p.Title,
			"aliases":   p.Aliases,
			"summary":   p.Summary,
			"weight":    weight,
		})
		if err != nil {
			return nil, err
		}
		rows = append(rows, map[string]interface{}{
			"id":                      entityID,
			"doc_id":                  kb,
			"tenant_id":               tenant,
			"kb_id":                   kb,
			"available_int":           1,
			"compile_kwd":             compileKwdWikiEntity,
			"type_kwd":                "wiki_" + p.PageType,
			"entity_type_kwd":         "wiki_" + p.PageType,
			"slug_kwd":                p.Slug,
			"title_kwd":               p.Title,
			"aliases_kwd":             p.Aliases,
			"description_with_weight": p.Summary,
			"weight_int":              weight,
			"source_chunk_ids":        p.SourceChunkIDs,
			"source_doc_ids":          capSourceDocs(p.SourceDocIDs),
			"content_with_weight":     string(content),
		})

		// Relations: one edge per outlink whose target page exists in this
		// projection. A target may be a full "<page_type>/<slug>" identity or a
		// bare slug (the latter is what the LLM emits in wikitext links); resolve
		// both so a bare outlink still produces an edge.
		for _, tgt := range p.Outlinks {
			if tgt == p.Slug {
				continue // self-loop: Python skips src == tgt
			}
			tp, ok := bySlug[tgt]
			if !ok {
				// Bare-slug outlink: normalize (strip prefix, "_"->"-") and look
				// up the reverse index. Keeps the canonical full slug for the
				// stored relation so from/to are consistent with slug_kwd. An
				// ambiguous bare key (mapped to more than one page) resolves to
				// nothing instead of nondeterministically choosing one.
				if bare := wikiGraphBareKey(tgt); bare != "" && !bareAmbiguous[bare] {
					tp, ok = byBareSlug[bare]
					if ok {
						tgt = tp.Slug
					}
				}
			}
			if !ok {
				continue // dangling edge: target page not in this projection
			}
			relID := wikiGraphXXHash("wiki_relation", kb, p.Slug+":"+tgt)
			if seen[relID] {
				continue
			}
			seen[relID] = true
			relContent, err := json.Marshal(map[string]any{
				"from": p.Slug,
				"to":   tgt,
			})
			if err != nil {
				return nil, err
			}
			srcDocs := unionCap(p.SourceDocIDs, tp.SourceDocIDs)
			rows = append(rows, map[string]interface{}{
				"id":                  relID,
				"doc_id":              kb,
				"tenant_id":           tenant,
				"kb_id":               kb,
				"available_int":       1,
				"compile_kwd":         compileKwdWikiRelation,
				"type_kwd":            compileKwdWikiRelation,
				"from_id":             entityID,
				"to_id":               wikiGraphXXHash("wiki_entity", kb, tgt),
				"from_kwd":            p.Slug,
				"to_kwd":              tgt,
				"source_doc_ids":      srcDocs,
				"content_with_weight": string(relContent),
			})
		}
	}
	var relCount, entCount int
	for _, r := range rows {
		switch r["compile_kwd"] {
		case compileKwdWikiRelation:
			relCount++
		case compileKwdWikiEntity:
			entCount++
		}
	}
	common.Info("knowledge_compile: projectWikiGraphRows result",
		zap.String("kb_id", kb),
		zap.Int("pages_projected", len(bySlug)),
		zap.Int("wiki_entity_rows", entCount),
		zap.Int("wiki_relation_rows", relCount))
	return rows, nil
}

// insertWikiGraphChunks shards the graph rows and drives the inserts through the
// shared global pool, mirroring WriteMerged.
func (w engineWriter) insertWikiGraphChunks(ctx context.Context, tenant, kb string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	eng := w.eng
	if eng == nil {
		eng = engine.Get()
	}
	if eng == nil {
		return nil
	}
	baseName := fmt.Sprintf("ragflow_%s", tenant)
	jobs := make([]CompilerJob, 0, (len(rows)+wikiGraphBatchSize-1)/wikiGraphBatchSize)
	for start := 0; start < len(rows); start += wikiGraphBatchSize {
		end := start + wikiGraphBatchSize
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[start:end]
		jobs = append(jobs, func() error {
			_, err := eng.InsertChunks(ctx, batch, baseName, kb)
			return err
		})
	}
	return runCompilerJobs(ctx, jobs)
}

// capSourceDocs returns up to wikiGraphSourceDocCap source doc ids.
func capSourceDocs(ids []string) []string {
	if len(ids) <= wikiGraphSourceDocCap {
		return ids
	}
	return ids[:wikiGraphSourceDocCap]
}

// unionCap returns the union of two doc-id lists, capped at wikiGraphSourceDocCap.
func unionCap(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, id := range a {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, id := range b {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return capSourceDocs(out)
}
