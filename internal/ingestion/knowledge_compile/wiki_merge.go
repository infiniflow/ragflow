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

	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// wikiReplace folds a wiki page candidate into its existing dataset-level merged
// row using a REPLACE-ONLY strategy. Wiki pages are Markdown, so the generic
// structure.LLMMergeDecider JSON merge (which concatenates/merges arbitrary JSON
// payloads) would mangle the page body — it must never run on wiki content.
//
// The existing row keeps its identity (id / doc_id / creation time) and provenance
// is unioned (source_doc_ids + source_chunk_ids of existing ∪ candidate). The
// page body is replaced by the candidate's Markdown verbatim. This mirrors
// Python's wiki incremental mode where a page re-compiled from a newer document
// simply supersedes the stored page of the same slug, rather than being merged.
func wikiReplace(existing, candidate kccommon.Product) kccommon.Product {
	merged := existing

	// Body: the newer (incoming) Markdown wins verbatim.
	merged.Content = candidate.Content
	// The embedding must match the replacement Markdown. WriteMerged persists
	// p.Vector without re-embedding, so keeping existing.Vector would leave a
	// stale embedding for the old page body and break KNN similarity searches.
	merged.Vector = candidate.Vector

	// Identity preserved: keep existing id, doc_id (== kb), and the original
	// creation timestamp (carried via Meta.created_at_unix by the Reader).
	merged.ID = existing.ID
	merged.DocID = existing.DocID

	// Provenance union (deduped) so the merged page references every source doc
	// and chunk that contributed to it across runs.
	merged.Meta = unionWikiProvenance(existing.Meta, candidate.Meta)

	// Re-stamp the resolver/run id from the incoming candidate so the row is
	// attributed to the latest batch, but the created_at_unix stays with existing.
	if v, ok := candidate.Meta["run_id"]; ok {
		merged.Meta["run_id"] = v
	}
	return merged
}

// unionWikiProvenance returns a new Meta map based on a (the existing row). The
// candidate b overwrites kind / slug / page_type / title / summary /
// entity_names / related_kb_pages / outlinks. Identity and creation time
// (created_at_unix, created_at) stay from a. Source provenance arrays
// (source_doc_ids, source_chunk_ids) are unioned and deduped.
func unionWikiProvenance(a, b map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range a {
		out[k] = v
	}
	// The incoming page replaces current page metadata. Identity and creation
	// time remain from the existing row.
	for _, key := range []string{"slug", "page_type", "title", "summary", "kind"} {
		if v, ok := b[key]; ok {
			out[key] = v
		}
	}
	out["source_doc_ids"] = unionStrs(metaStringSliceAny(a, "source_doc_ids"), metaStringSliceAny(b, "source_doc_ids"))
	out["source_chunk_ids"] = unionStrs(metaStringSliceAny(a, "source_chunk_ids"), metaStringSliceAny(b, "source_chunk_ids"))
	if v, ok := b["entity_names"]; ok {
		out["entity_names"] = v
	}
	if v, ok := b["related_kb_pages"]; ok {
		out["related_kb_pages"] = v
	}
	if v, ok := b["outlinks"]; ok {
		out["outlinks"] = v
	}
	return out
}

func unionStrs(a, b []string) []string {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, s := range append(append([]string(nil), a...), b...) {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// metaStringSliceAny reads a []string from a meta map that may box the value as
// []string or []any (engine/JSON round-trip does not guarantee a single type).
func metaStringSliceAny(m map[string]any, key string) []string {
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

// isWikiGroup reports whether a merge group targets the wiki variant and must use
// replace-only (never the JSON-merge decider).
func isWikiGroup(g MergeGroup) bool {
	return g.Existing.Variant == kccommon.VariantWiki
}

// wikiDecideBatch folds every wiki-group candidate into its existing row using
// replace-only semantics, WITHOUT any LLM call. It mutates the passed groups in
// place and returns them.
func wikiDecideBatch(_ context.Context, groups []MergeGroup) []MergeGroup {
	for gi := range groups {
		existing := groups[gi].Existing
		var distinct []kccommon.Product
		duplicated := false
		for _, cand := range groups[gi].Candidates {
			// A wiki page candidate is always a replacement of the existing row
			// (same slug, newer content). It is never kept as an additional distinct
			// row — distinctness for wiki is decided by KNN (different slug -> different
			// existing row -> different group).
			existing = wikiReplace(existing, cand)
			duplicated = true
		}
		groups[gi].Merged = existing
		groups[gi].Duplicate = duplicated
		groups[gi].Distinct = distinct
	}
	return groups
}
