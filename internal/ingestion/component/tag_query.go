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
//

package component

import (
	"context"
	"math"
	"sort"
	"strings"

	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/tokenizer"
)

const (
	// tagQueryTopLabels caps how many tag source records contribute to the
	// question's tag distribution.
	//
	// Python's tag_query (rag/nlp/search.py) issues the match with a
	// MatchTextExpr whose topn is 100, so the engine returns at most 100
	// records and the tag aggregation is computed over exactly that window:
	//
	//   - Infinity (default engine): limit=0 is rewritten to 10000 but
	//     builder.match_text(..., topn=100) caps the result set; get_aggregation
	//     counts over the returned DataFrame.
	//   - OceanBase: LIMIT offset, fulltext_topn (= 100).
	//   - Elasticsearch/OpenSearch: aggregation comes from the engine's own
	//     terms agg over ALL matching documents, so there is no window at all.
	//
	// 100 matches the non-ES engines; for ES the window is an approximation.
	tagQueryTopLabels = 100

	// tagQueryProportionSmoothing is the S constant in Python's
	// all_tags_in_portion / tag_query formulas.
	tagQueryProportionSmoothing = 1000.0

	// tagQueryLang is the analyzer language for the retrieval-side tag source
	// index. "" selects the tokenizer's default (English). Documents and the
	// question are tokenized with the same analyzer, so both sides stay
	// consistent.
	tagQueryLang = ""
)

// tagQueryIndexCache caches the merged retrieval-side index per set of tag
// source files. Separate from tagSourceFileIndexCache (the per-file index,
// which both consumers feed) because the merged index is keyed by the whole
// file set instead of one file.
var tagQueryIndexCache = newBoundedTagCache(tagSourceCacheMax)

// TagSourceRef names a tag source file (parser_config tags.tag_file_id) and the
// tenant that owns it. The tenant is required so the file can be resolved
// inside the dataset's ownership scope before it is read (IDOR, CWE-639).
type TagSourceRef struct {
	FileID   string
	TenantID string
}

// TagFileIDFromParserConfig returns the tag source file id declared in a
// dataset's parser_config. Two persisted shapes are supported:
//
//   - Legacy / flat form: parser_config["tags"]["tag_file_id"].
//   - Go pipeline scoped form: parser_config["Extractor:<name>"]["tags"]
//     ["tag_file_id"], mirroring how NewExtractorComponent reads its params.
//
// Returns "" when unset.
func TagFileIDFromParserConfig(parserConfig map[string]any) string {
	if parserConfig == nil {
		return ""
	}
	if v := tagFileIDFromTagsMap(parserConfig["tags"]); v != "" {
		return v
	}
	// Several Extractor nodes can each declare their own tags block, so the
	// winner has to be picked in a stable order: Go randomizes map iteration,
	// and an unstable pick would let the dataset's tag source change between
	// process restarts (different chunks tagged, different retrieval
	// features). Sorting makes it deterministic; "Extractor" sorts before
	// "Extractor:<name>" because it is a prefix, so the unscoped node wins.
	keys := make([]string, 0, len(parserConfig))
	for key := range parserConfig {
		if key == "Extractor" || strings.HasPrefix(key, "Extractor:") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		extractorMap, ok := asStringAnyMap(parserConfig[key])
		if !ok {
			continue
		}
		// The tags config sits at the top level of the Extractor node value,
		// i.e. parser_config["Extractor:<name>"]["tags"].
		if v := tagFileIDFromTagsMap(extractorMap["tags"]); v != "" {
			return v
		}
	}
	return ""
}

// tagFileIDFromTagsMap pulls tag_file_id out of a raw "tags" config value,
// accepting either a map[string]any or an entity.JSONMap.
func tagFileIDFromTagsMap(raw any) string {
	tags, ok := asStringAnyMap(raw)
	if !ok {
		return ""
	}
	if v, ok := tags["tag_file_id"].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// asStringAnyMap accepts both plain and entity.JSONMap values, which is how
// parser_config is persisted depending on the write path.
func asStringAnyMap(raw any) (map[string]any, bool) {
	switch v := raw.(type) {
	case map[string]any:
		return v, true
	case entity.JSONMap:
		return map[string]any(v), true
	default:
		return nil, false
	}
}

// resolvedTagSource pairs a reference with the file row it resolved to.
type resolvedTagSource struct {
	ref  TagSourceRef
	file *entity.File
}

// LoadTagQueryIndex loads the referenced tag source files and merges their
// records into a single retrieval-time matching index, cached by the file set
// (including each file's size/update time, so an edited tag file invalidates
// the merged index). A single tag source file needs no merging: its own cached
// index is returned as-is.
//
// It is the Go equivalent of Python's tag-dataset corpus used by
// all_tags_in_portion / tag_query: Go has no tag dataset, so the records come
// from the tag source file. Returns (nil, nil) when refs is empty or yields no
// usable records.
func LoadTagQueryIndex(ctx context.Context, refs []TagSourceRef) (*MemoryTagIndex, error) {
	resolved := make([]resolvedTagSource, 0, len(refs))
	tokens := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.FileID == "" {
			continue
		}
		f, err := resolveTagSourceFile(ctx, ref.FileID, ref.TenantID)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, resolvedTagSource{ref: ref, file: f})
		tokens = append(tokens, tagSourceFileCacheKey(f, tagQueryLang))
	}
	if len(resolved) == 0 {
		return nil, nil
	}
	// Single-file fast path: the per-file index was built from exactly the
	// labels and the tokenizer the merged index would use, so it already is the
	// merged index. Reusing it skips the rebuild and keeps only one copy cached.
	// One tag source file per dataset is the common case - refs are deduplicated
	// by (tenant, file).
	if len(resolved) == 1 {
		r := resolved[0]
		idx, _, err := loadOrBuildTagFileIndex(ctx, r.file, r.ref.TenantID, tagQueryLang)
		return idx, err
	}
	sort.Strings(tokens)
	key := "tag-query\x00" + strings.Join(tokens, "\x01")
	if cached, hit := tagQueryIndexCache.load(key); hit {
		return cached, nil
	}

	var labels []schema.TagLabel
	for _, r := range resolved {
		idx, _, err := loadOrBuildTagFileIndex(ctx, r.file, r.ref.TenantID, tagQueryLang)
		if err != nil {
			return nil, err
		}
		if idx != nil {
			labels = append(labels, idx.examples...)
		}
	}
	if len(labels) == 0 {
		return nil, nil
	}
	merged := buildMemoryTagIndex(labels, tokenizer.New(tagQueryLang))
	if merged == nil {
		return nil, nil
	}
	return tagQueryIndexCache.store(key, merged), nil
}

// TagProportions returns the per-tag background proportions used by the
// tag_query formula, mirroring Python's all_tags_in_portion: a tag's count is
// the number of records that mention it, and its proportion is
// (count+1)/(total+smoothing).
// The counts are precomputed on the index: only the smoothing denominator
// differs per call, so this is O(distinct tags) rather than a walk over every
// tag occurrence of every record.
func (idx *MemoryTagIndex) TagProportions(smoothing float64) map[string]float64 {
	out := make(map[string]float64)
	if idx == nil || idx.tagTotal == 0 {
		return out
	}
	denom := float64(idx.tagTotal) + smoothing
	for t, c := range idx.tagCounts {
		out[t] = (float64(c) + 1) / denom
	}
	return out
}

// MatchTagQuery mirrors rag/nlp/search.py::tag_query over a Go tag source file:
// it matches the question against the tag source records, aggregates the tags of
// the best-matching records, and converts that distribution into rank-feature
// weights with Python's formula
//
//	round(0.1 * (count+1) / (total + S) / max(1e-6, all_tags[tag]))
//
// returning the top-n tags (dots normalized to underscores, floored at 1).
//
// The question is tokenized with the analyzer the index itself was built with
// (idx.tok), so records and question can never be analyzed differently: a
// mismatched analyzer does not fail, it just silently matches nothing.
//
// The record-matching step approximates Python's engine query with an
// IDF-weighted token overlap over the tag source records; unlike ingestion-time
// tagging it applies no coverage threshold, because the query is short and
// Python's tag_query runs with min_match=0.0.
func (idx *MemoryTagIndex) MatchTagQuery(question string, topnTags int) map[string]float64 {
	out := make(map[string]float64)
	if idx == nil || len(idx.examples) == 0 || topnTags <= 0 || strings.TrimSpace(question) == "" {
		return out
	}
	tokens, err := idx.tok.Tokenize(question)
	if err != nil {
		return out
	}

	scores := make(map[int]float64)
	seen := make(map[string]struct{})
	for _, w := range strings.Fields(tokens) {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		if _, dup := seen[w]; dup {
			continue
		}
		seen[w] = struct{}{}
		idf, ok := idx.idfs[w]
		if !ok || idf <= 1e-6 {
			continue
		}
		for _, docID := range idx.postings[w] {
			scores[docID] += idf
		}
	}
	if len(scores) == 0 {
		return out
	}

	matched := make([]int, 0, len(scores))
	for docID := range scores {
		matched = append(matched, docID)
	}
	sort.Slice(matched, func(i, j int) bool {
		if scores[matched[i]] != scores[matched[j]] {
			return scores[matched[i]] > scores[matched[j]]
		}
		return matched[i] < matched[j]
	})
	if len(matched) > tagQueryTopLabels {
		matched = matched[:tagQueryTopLabels]
	}

	counts := make(map[string]int)
	for _, docID := range matched {
		for _, t := range idx.examples[docID].Tags {
			counts[t]++
		}
	}
	if len(counts) == 0 {
		return out
	}
	total := 0
	for _, c := range counts {
		total += c
	}
	allTags := idx.TagProportions(tagQueryProportionSmoothing)

	type tagScore struct {
		tag   string
		score float64
	}
	scored := make([]tagScore, 0, len(counts))
	for t, c := range counts {
		background := allTags[t]
		if background <= 0 {
			background = 0.0001
		}
		raw := 0.1 * float64(c+1) / (float64(total) + tagQueryProportionSmoothing) / math.Max(1e-6, background)
		// Python rounds the raw score with round() before ranking it and only
		// then floors it at 1 (search.py: tag_query), so the weight is an
		// integer. round() is round-half-to-even, i.e. math.RoundToEven, not
		// math.Round: round(2.5) == 2 while math.Round(2.5) == 3. Ranking on
		// the rounded value keeps the top-n cut the same as Python's. Ties are
		// broken by tag name here, where Python's are left in the engine's
		// aggregation order.
		scored = append(scored, tagScore{tag: t, score: math.RoundToEven(raw)})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].tag < scored[j].tag
	})
	for i := 0; i < topnTags && i < len(scored); i++ {
		name := strings.ReplaceAll(scored[i].tag, ".", "_")
		value := math.Max(1.0, scored[i].score)
		if existing, ok := out[name]; !ok || value > existing {
			out[name] = value
		}
	}
	return out
}
