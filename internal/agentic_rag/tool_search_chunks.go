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

package agentic_rag

import (
	"context"
	"encoding/json"
	"fmt"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
)

// searchChunksToolName is a thin wrapper over hybrid retrieval that accepts 1–5
// semantic queries and returns full chunk content (the deep-read carrier).
const searchChunksToolName = "search_chunks"

const searchChunksToolDescription = `Semantic/vector search tool for retrieving knowledge by meaning, intent, and conceptual relevance.

This tool uses embeddings to understand the query and find semantically similar content across dataset chunks. It searches by MEANING rather than exact text.

## What the Tool Does NOT Do
- Does NOT perform exact keyword matching (use grep_chunks for that)
- Should NOT receive long raw text or user messages as queries
- Should NOT be used to locate specific strings or error codes

## Required Input Behavior
"queries" must contain 1–5 short, well-formed semantic questions or conceptual statements.

## Hybrid weighting (affects what comes back)
Every call fuses a keyword leg with a vector leg; keywords_similarity_weight sets the split (default 0.7 = 70% keyword, 30% vector). Choose it deliberately: raise it (0.7-0.9) when the question carries distinctive terms, proper nouns, IDs or dates you expect to appear literally; lower it (0.2-0.4) when the question is conceptual or paraphrased and the corpus likely words it differently. EXTREMES are permitted and sometimes necessary: when several keyword-flavored queries return empty or off-target yet the corpus must contain the answer, push keywords_similarity_weight toward 0.0 (pure vector) — semantic similarity can bridge a total wording gap that no keyword query can cross (this is how you escape a paraphrase dead-end like an unknown book title); when you need the corpus's exact literal token (an ID, a code, a quoted phrase) and vector noise drowns it, push toward 1.0 (pure keyword). An extreme disables the other leg — that is the point — but it must be a DELIBERATE probe: before concluding the corpus lacks the answer, attempt the default, then each extreme on the most promising query.

## Output (XML)
Returns an XML <search_results count="N" query="..."> document (unified with grep_chunks / search_bm25_chunks). Each hit is a <chunk> element with attributes rank, chunk_id, doc_id (owning document id), page_num, chunk_index, dataset_id (owning dataset id), doc_name and score, plus a <match_snippet> element — a single-line window spanning from snippetContextRunes runes BEFORE the earliest keyword-bearing region to that many runes AFTER the latest one. Snippets are for relevance triage only: pass the doc_id together with anchor_chunk_ids to list_chunks for the authoritative full-text deep read.`

// searchChunksArgs is the JSON the model sends into InvokableRun.
type searchChunksArgs struct {
	Queries                  []string `json:"queries"`
	DatasetIDs               []string `json:"dataset_ids,omitempty"`
	DocScope                 []string `json:"doc_scope,omitempty"`
	TopN                     int      `json:"top_n,omitempty"`
	SimilarityThreshold      *float64 `json:"similarity_threshold,omitempty"`
	KeywordsSimilarityWeight *float64 `json:"keywords_similarity_weight,omitempty"`
}

// searchChunksDefaultTopN is the per-query result count.
const searchChunksDefaultTopN = 12

// Defaults for the retrieval knobs, exposed so the model can tighten/loosen the
// search.
const (
	searchChunksDefaultSimilarityThreshold      = 0.2
	searchChunksDefaultKeywordsSimilarityWeight = 0.7
)

// SearchChunksTool performs semantic retrieval over 1–5 queries, merging and
// deduplicating the results. Backs onto GetRetrievalService() with hybrid
// weighting (vector + keyword). The tenant and dataset scope are injected at
// construction from the session.
type SearchChunksTool struct {
	tenantID   string
	datasetIDs []string
}

// NewSearchChunksTool returns a SearchChunksTool scoped to the given tenant and
// datasets, implementing eino's runtime.InvokableTool.
func NewSearchChunksTool(tenantID string, datasetIDs []string) *SearchChunksTool {
	return &SearchChunksTool{tenantID: tenantID, datasetIDs: datasetIDs}
}

// Info returns the tool's metadata for the chat model. The parameter schema is
// declared as a single JSON string so the model sees array length bounds
// (minItems/maxItems), numeric ranges (minimum/maximum) and item types in one
// readable block, then parsed into *jsonschema.Schema.
func (k *SearchChunksTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	schemaJSON := `{
  "type": "object",
  "properties": {
    "queries": {
      "type": "array",
      "description": "REQUIRED: 1-5 short, well-formed semantic questions or conceptual statements. Each query is embedded to find meaningfully similar chunks. Break a broad question into multiple focused queries (e.g. one per entity or sub-question). Do NOT pass raw long text.",
      "items": { "type": "string" },
      "minItems": 1,
      "maxItems": 5
    },
    "dataset_ids": {
      "type": "array",
      "description": "Optional dataset ids to restrict retrieval to (at most 10). When omitted, the current conversation's bound datasets are used.",
      "items": { "type": "string" },
      "maxItems": 10
    },
    "doc_scope": {
      "type": "array",
      "description": "Optional document ids to restrict retrieval to (at most 10, normally taken from grep_chunks results). When set, only chunks of those documents are searched.",
      "items": { "type": "string" },
      "maxItems": 10
    },
    "top_n": {
      "type": "integer",
      "description": "Number of chunks to return per query, 1-50 (default 12). Larger values give more coverage at higher token cost.",
      "minimum": 0,
      "maximum": 50
    },
    "similarity_threshold": {
      "type": "number",
      "description": "Minimum similarity score (0.0-1.0) for a chunk to be returned; higher is stricter (fewer, more relevant results). Default 0.2.",
      "minimum": 0,
      "maximum": 1
    },
    "keywords_similarity_weight": {
      "type": "number",
      "description": "Weight (0.0-1.0) balancing keyword (lexical) vs vector (semantic) similarity in the hybrid fusion: 0.0 = pure vector/meaning, 1.0 = pure keyword. Default 0.7 (70% keyword, 30% vector). THIS WEIGHT DECIDES WHAT COMES BACK, so set it deliberately instead of leaving it or pushing it to an extreme: raise it (0.7-0.9) when the question carries distinctive terms, proper nouns, IDs, dates or exact phrasings you expect to appear literally - the lexical leg anchors those; lower it (0.2-0.4) when the question is conceptual or paraphrased and the corpus likely words it differently - the vector leg bridges the wording gap. Values near 0.0 or 1.0 disable one leg almost completely and usually lose recall; if a query returns nothing useful, retry with the weight moved toward the OTHER leg before concluding the corpus lacks the answer.",
      "minimum": 0,
      "maximum": 1
    }
  },
  "required": ["queries"]
}`
	s := &jsonschema.Schema{}
	if err := json.Unmarshal([]byte(schemaJSON), s); err != nil {
		return nil, fmt.Errorf("search_chunks: parse schema: %w", err)
	}
	return &schema.ToolInfo{
		Name:        searchChunksToolName,
		Desc:        searchChunksToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByJSONSchema(s),
	}, nil
}

// InvokableRun performs semantic retrieval over the given queries, merging and
// deduplicating results. Returns JSON with full chunk content.
func (k *SearchChunksTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	return guardedToolRun(ctx, searchChunksToolName, k.invokableRun, argumentsInJSON)
}

func (k *SearchChunksTool) invokableRun(ctx context.Context, argumentsInJSON string) (string, error) {
	var args searchChunksArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("search_chunks: parse arguments: %w", err)
	}

	// The keyword share is the model's to choose here (default 0.7): this tool
	// IS the fusion leg. Weight 0 does not turn it into a pure vector search —
	// use search_semantic_chunks for that (see locateSearchSpec.vectorOnly).
	weight := searchChunksDefaultKeywordsSimilarityWeight
	if args.KeywordsSimilarityWeight != nil {
		weight = clampFloat01(*args.KeywordsSimilarityWeight)
	}
	return runLocateSearch(ctx, locateSearchSpec{
		tool:                searchChunksToolName,
		queries:             args.Queries,
		datasetIDs:          args.DatasetIDs,
		boundDatasetIDs:     k.datasetIDs,
		docScope:            args.DocScope,
		topN:                args.TopN,
		similarityThreshold: args.SimilarityThreshold,
		weight:              weight,
		tenantID:            k.tenantID,
	})
}
