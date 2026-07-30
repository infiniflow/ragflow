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

	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// Writer persists dataset-level merged products and removes them on document
// deletion (§11.7).
type Writer interface {
	// WriteMerged upserts the dataset-level merged products (available_int=1).
	WriteMerged(ctx context.Context, tenant, kb string, products []kccommon.Product) error
	// DeleteMergedForDoc removes merged products that became fully orphaned by
	// the deletion of docID. Multi-doc merged products are recomputed by the
	// caller (via the Reader + Deduper), not here.
	DeleteMergedForDoc(ctx context.Context, tenant, kb, docID string) error
}

type infinityWriter struct{}

func (infinityWriter) WriteMerged(ctx context.Context, tenant, kb string, products []kccommon.Product) error {
	if len(products) == 0 {
		return nil
	}
	eng := engine.Get()
	if eng == nil {
		return nil
	}
	baseName := fmt.Sprintf("ragflow_%s", tenant)
	chunks := make([]map[string]interface{}, 0, len(products))
	for _, p := range products {
		chunks = append(chunks, mergedChunkMap(tenant, kb, p))
	}
	_, err := eng.InsertChunks(ctx, chunks, baseName, kb)
	return err
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
	// Persist the merged product's embedding under the dimension-suffixed column
	// used elsewhere in the index, so dataset-level rows remain vector-searchable
	// and Reader.LoadCompiledProducts can reconstruct them (otherwise the vector
	// is silently dropped and search returns nothing for merged rows).
	if dim := len(p.Vector); dim > 0 {
		m[fmt.Sprintf("q_%d_vec", dim)] = p.Vector
	}
	return m
}

func (infinityWriter) DeleteMergedForDoc(ctx context.Context, tenant, kb, docID string) error {
	eng := engine.Get()
	if eng == nil {
		return nil
	}
	baseName := fmt.Sprintf("ragflow_%s", tenant)
	const batchSize = 2000
	// Re-query from the start each iteration (deletions shift the result set),
	// deleting every fully-orphaned merged row we find, until a page returns
	// fewer than batchSize candidates. Without pagination, orphaned merged rows
	// beyond the 2000 cap would never be cleaned up.
	for {
		res, err := eng.Search(ctx, &types.SearchRequest{
			IndexNames:   []string{baseName},
			KbIDs:        []string{kb},
			Filter:       map[string]interface{}{"available_int": 1, "kc_merged": 1},
			SelectFields: []string{"id", "source_doc_ids"},
			Limit:        batchSize,
			Offset:       0,
		})
		if err != nil {
			return err
		}
		if len(res.Chunks) == 0 {
			break
		}
		for _, c := range res.Chunks {
			ids := metaStringSlice(c, "source_doc_ids")
			// Fully orphaned: the only contributor is the deleted document.
			if len(ids) == 1 && ids[0] == docID {
				if _, err := eng.DeleteChunks(ctx, map[string]interface{}{
					"id":            c["id"],
					"kb_id":         kb,
					"available_int": 1,
				}, baseName, kb); err != nil {
					return err
				}
			}
		}
		if len(res.Chunks) < batchSize {
			break
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
