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
	"strings"

	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

const maxMergedWikiMarkdownBytes = 256 * 1024

// wikiEntityMerge combines evidence for the same stable page without
// using a page replacement consumer. Existing and incoming Markdown blocks are
// retained deterministically; source provenance is unioned.
func wikiEntityMerge(existing, incoming kccommon.Product) kccommon.Product {
	if existing.ID == "" {
		incoming.Merged = true
		return incoming
	}
	merged := existing
	merged.Content = unionWikiMarkdown(existing.Content, incoming.Content)
	merged.Vector = incoming.Vector
	merged.Meta = unionWikiProvenance(existing.Meta, incoming.Meta)
	merged.ID = existing.ID
	merged.DocID = existing.DocID
	merged.Merged = true
	return merged
}

// selectMergedWikiTopicPath chooses the materialized path with the strongest
// source support when document pages with the same canonical slug disagree.
// No additional metadata is persisted: the selected path remains the page's
// single topic value.
func selectMergedWikiTopicPath(products []kccommon.Product) string {
	type support struct {
		topic    string
		docs     map[string]struct{}
		chunks   map[string]struct{}
		products int
	}
	byKey := make(map[string]*support)
	for _, product := range products {
		topic := productTopic(product)
		key := topicKey(topic)
		if key == "" {
			continue
		}
		current := byKey[key]
		if current == nil {
			current = &support{topic: topic, docs: make(map[string]struct{}), chunks: make(map[string]struct{})}
			byKey[key] = current
		}
		current.products++
		docIDs := metaStringSliceAny(product.Meta, "source_doc_ids")
		if len(docIDs) == 0 && product.DocID != "" {
			docIDs = []string{product.DocID}
		}
		for _, docID := range docIDs {
			if docID = strings.TrimSpace(docID); docID != "" {
				current.docs[docID] = struct{}{}
			}
		}
		for _, chunkID := range metaStringSliceAny(product.Meta, "source_chunk_ids") {
			if chunkID = strings.TrimSpace(chunkID); chunkID != "" {
				current.chunks[chunkID] = struct{}{}
			}
		}
	}

	var best *support
	for _, candidate := range byKey {
		if best == nil || len(candidate.docs) > len(best.docs) ||
			(len(candidate.docs) == len(best.docs) && len(candidate.chunks) > len(best.chunks)) ||
			(len(candidate.docs) == len(best.docs) && len(candidate.chunks) == len(best.chunks) && candidate.products > best.products) ||
			(len(candidate.docs) == len(best.docs) && len(candidate.chunks) == len(best.chunks) && candidate.products == best.products && topicKey(candidate.topic) < topicKey(best.topic)) {
			best = candidate
		}
	}
	if best == nil {
		return ""
	}
	return best.topic
}

func unionWikiMarkdown(left, right string) string {
	left, right = strings.TrimSpace(left), strings.TrimSpace(right)
	if left == "" {
		return right
	}
	if right == "" || left == right || strings.Contains(left, right) {
		return left
	}
	if strings.Contains(right, left) {
		return right
	}
	blocks := append(splitMarkdownBlocks(left), splitMarkdownBlocks(right)...)
	seen := map[string]struct{}{}
	unique := make([]string, 0, len(blocks))
	length := 0
	for _, block := range blocks {
		key := strings.TrimSpace(block)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		if length+len(block)+2 > maxMergedWikiMarkdownBytes {
			break
		}
		seen[key] = struct{}{}
		unique = append(unique, key)
		length += len(block) + 2
	}
	return strings.Join(unique, "\n\n")
}

func splitMarkdownBlocks(markdown string) []string {
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	blocks := make([]string, 0)
	var current strings.Builder
	inFence := false
	flush := func() {
		if block := strings.TrimSpace(current.String()); block != "" {
			blocks = append(blocks, block)
		}
		current.Reset()
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
		}
		if trimmed == "" && !inFence {
			flush()
			continue
		}
		if current.Len() > 0 {
			current.WriteByte('\n')
		}
		current.WriteString(line)
	}
	flush()
	return blocks
}

// unionWikiProvenance returns a new Meta map based on a (the existing row). The
// candidate b contributes content metadata, while the existing page identity
// (slug/title/page_type) remains authoritative because pages are merged only
// when their canonical slug is equal. This prevents a retry or another
// document's equivalent page from renaming the dataset-level page.
// The candidate b overwrites kind / summary /
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
	for _, key := range []string{"summary", "kind"} {
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

// isWikiGroup reports whether a merge group targets the wiki variant and needs
// the page-specific Markdown merge instead of the generic JSON decider.
func isWikiGroup(g MergeGroup) bool {
	return g.Existing.Variant == kccommon.VariantWiki
}

// wikiMergeBatch folds every wiki-group candidate into its existing row while
// retaining Markdown evidence and source provenance.
func wikiMergeBatch(_ context.Context, groups []MergeGroup) []MergeGroup {
	for gi := range groups {
		existing := groups[gi].Existing
		var distinct []kccommon.Product
		duplicated := false
		for _, cand := range groups[gi].Candidates {
			if isTopicPage(existing) && isTopicPage(cand) {
				existing = mergeTopicPage(existing, cand)
			} else {
				existing = wikiEntityMerge(existing, cand)
			}
			duplicated = true
		}
		groups[gi].Merged = existing
		groups[gi].Duplicate = duplicated
		groups[gi].Distinct = distinct
	}
	return groups
}
