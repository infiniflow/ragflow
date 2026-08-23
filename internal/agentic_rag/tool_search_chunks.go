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
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
	"go.uber.org/zap"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/dao"
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

## Output (XML)
Returns an XML <search_results> document. Each hit is a <chunk> element with attributes rank, chunk_id, doc_id (owning document id), page_num, chunk_index, dataset_id (owning dataset id), doc_title and score, and a <content> element carrying the FULL chunk text — rely on it for deep reading; there is no separate deep-read runtime. Pass the returned doc_id to list_chunks to page through a document's full text.`

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
	searchChunksDefaultKeywordsSimilarityWeight = 0.3
)

// SearchChunksTool performs semantic retrieval over 1–5 queries, merging and
// deduplicating the results. Backs onto GetRetrievalService() with hybrid
// weighting (vector + keyword).
type SearchChunksTool struct{}

// NewSearchChunksTool returns a SearchChunksTool implementing eino's runtime.InvokableTool.
func NewSearchChunksTool() *SearchChunksTool {
	return &SearchChunksTool{}
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
      "description": "Weight (0.0-1.0) balancing keyword (lexical) vs vector (semantic) similarity: 0.0 = pure vector, 1.0 = pure keyword. Default 0.3.",
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
	var args searchChunksArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("search_chunks: parse arguments: %w", err)
	}

	// Validate query count.
	queries := make([]string, 0, len(args.Queries))
	for _, q := range args.Queries {
		if q = strings.TrimSpace(q); q != "" {
			queries = append(queries, q)
		}
	}
	if len(queries) == 0 {
		return "", fmt.Errorf("search_chunks: queries must contain 1-5 non-empty semantic questions")
	}
	if len(queries) > 5 {
		return "", fmt.Errorf("search_chunks: queries must contain at most 5 questions, got %d", len(queries))
	}

	// Enforce the declared input bounds before touching production retrieval: a
	// hostile or confused model could otherwise ask for top_n in the millions and
	// forward an unbounded TopK downstream.
	topN := args.TopN
	if topN <= 0 {
		topN = searchChunksDefaultTopN
	}
	if topN > 50 {
		topN = 50
	}
	similarityThreshold := searchChunksDefaultSimilarityThreshold
	if args.SimilarityThreshold != nil {
		similarityThreshold = clampFloat01(*args.SimilarityThreshold)
	}
	weight := searchChunksDefaultKeywordsSimilarityWeight
	if args.KeywordsSimilarityWeight != nil {
		weight = clampFloat01(*args.KeywordsSimilarityWeight)
	}
	if len(args.DatasetIDs) > 10 {
		args.DatasetIDs = args.DatasetIDs[:10]
	}
	if len(args.DocScope) > 10 {
		args.DocScope = args.DocScope[:10]
	}

	svc := runtime.GetRetrievalService()
	tenantID := runtime.TenantID(ctx)
	datasetIDs := runtime.DatasetIDs(ctx, args.DatasetIDs)
	if svc == nil || tenantID == "" || len(datasetIDs) == 0 {
		return searchResultsXMLEmpty(), nil
	}

	// Bounded engine context: eino's streaming ReAct may hand the tool an
	// already-canceled context right after the model emits tool_calls; a canceled
	// parent must not kill the retrieval. Fall back to a fresh bounded context in
	// that case; the agent's smartReasoningTimeout still caps the whole run.
	ectx, ecancel := engineCallContext(ctx)
	defer ecancel()

	seen := map[string]struct{}{}
	var merged []runtime.RetrievalChunk
	for _, q := range queries {
		chunks, err := svc.Search(ectx, dao.DB, runtime.RetrievalRequest{
			Query:                    q,
			DatasetIDs:               datasetIDs,
			TopN:                     topN,
			TopK:                     topN * 4,
			SimilarityThreshold:      &similarityThreshold,
			KeywordsSimilarityWeight: &weight,
			DocScope:                 args.DocScope,
			TenantID:                 tenantID,
			SelectFields:             grepChunksSelectFields,
			// Same "only ordinary prose" filter as grep_chunks: exclude
			// compiled products so the model reads original document text, not
			// derived graph content.
			OnlyOriginalText: true,
		})
		if err != nil {
			continue // fall back on failure, keep other queries' results
		}
		for _, c := range chunks {
			if _, dup := seen[c.ID]; dup {
				continue
			}
			seen[c.ID] = struct{}{}
			// Skip graph relation/entity/location chunks so the model reads
			// actual document prose rather than extracted graph triples
			// (which are sparse and miss events like "何进斩马元义").
			if isGraphChunkContent(c.Content) {
				continue
			}
			merged = append(merged, c)
		}
	}

	common.Debug("agentic_rag: search_chunks result",
		zap.Int("chunks", len(merged)),
		zap.Strings("queries", queries),
	)
	return formatSearchResultsXML(merged), nil
}
