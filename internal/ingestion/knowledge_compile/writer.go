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
	"fmt"
	"strings"

	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// Writer persists dataset-level merged products and removes them on document
// deletion (§11.7).
type Writer interface {
	// WriteMerged upserts the dataset-level merged products (available_int=1).
	WriteMerged(ctx context.Context, tenant, kb string, products []kccommon.Product) error
	// DeleteDocLevelForDocs drops every per-document (doc-level, kc_merged != 1)
	// product of the deleted docs in a single DocEngine call. Dataset-level
	// merged rows are not targeted because their doc_id equals the kb, never a
	// deleted source doc id.
	DeleteDocLevelForDocs(ctx context.Context, tenant, kb string, deletedDocIDs []string) error
	// StripMergedSources removes deletedDocIDs from the source_doc_ids array of
	// every dataset-level (kc_merged=1) product for the dataset. It searches the
	// merged set once, rewrites the source array of every non-empty survivor in a
	// single update pass, and deletes (in one call) any product whose array
	// became empty.
	StripMergedSources(ctx context.Context, tenant, kb string, deletedDocIDs []string) error
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
	eng := w.eng
	if eng == nil {
		eng = engine.Get()
	}
	if eng == nil {
		return nil
	}
	baseName := fmt.Sprintf("ragflow_%s", tenant)
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
				chunks = append(chunks, mergedChunkMap(tenant, kb, p))
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
func mergedChunkMap(tenant, kb string, p kccommon.Product) map[string]interface{} {
	srcDocIDs := metaStringSlice(p.Meta, "source_doc_ids")
	srcChunkIDs := metaStringSlice(p.Meta, "source_chunk_ids")
	m := map[string]interface{}{
		"id":                  datasetLevelID(tenant, kb, p),
		"doc_id":              kb,
		"tenant_id":           tenant,
		"kb_id":               kb,
		"available_int":       1,
		"kc_merged":           1,
		"compile_kwd":         string(p.Variant),
		"content_with_weight": p.Content,
		"kc_payload":          p.Content, // raw payload, for Reader reconstruction
		"source_doc_ids":      srcDocIDs,
		"source_chunk_ids":    srcChunkIDs,
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
// every dataset-level (kc_merged=1) product for the dataset. The query filters
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
			// kc_merged=1 isolates dataset-level rows; source_doc_ids IN
			// deletedDocIDs pushes the intersection test into the engine so only
			// rows that actually reference a deleted doc are returned (Infinity
			// array IN means "contains at least one of").
			Filter: map[string]interface{}{
				"kc_merged":      1,
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
