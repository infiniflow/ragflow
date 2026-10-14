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
	"unicode/utf8"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
	"go.uber.org/zap"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
)

const searchBm25ChunksToolName = "search_bm25_chunks"

const searchBm25ChunksToolDescription = `Lexical full-text search tool: pure BM25 keyword ranking over tokenized chunk fields — no vector/semantic component.

Use it to recall chunks containing your keywords with relevance ranking (term frequency × rarity), which complements grep_chunks' exact regex matching (grep requires the whole pattern to literally match; BM25 also recalls partial/fuzzy keyword co-occurrence) while staying grounded in literal terms unlike semantic search.

## Required Input Behavior
"queries" must contain 1-5 short KEYWORD queries — entities, names, phrases, or term combinations (e.g. "马元义 斩", "Ralph Bronner MOCA"). Do NOT send full sentences; split into focused term queries instead.

## Output (XML)
Returns an XML <search_results count="N" query="..."> document (unified with grep_chunks / search_chunks). Each hit is a <chunk> element with rank, chunk_id, doc_id, page_num, chunk_index, dataset_id, doc_name and score attributes, plus a <match_snippet> element — a single-line window spanning from snippetContextRunes runes BEFORE the earliest keyword hit to that many runes AFTER the latest one. Snippets are for relevance triage only — call list_chunks with anchor_chunk_ids for the authoritative full-text deep read.`

type searchBm25ChunksArgs struct {
	Queries    []string `json:"queries"`
	DatasetIDs []string `json:"dataset_ids,omitempty"`
	DocScope   []string `json:"doc_scope,omitempty"`
	TopN       int      `json:"top_n,omitempty"`
}

const searchBm25DefaultTopN = 12

// SearchBm25ChunksTool performs lexical BM25 retrieval over 1-5 keyword
// queries, merging results by chunk id. Backs onto runtime.GetBm25Service().
type SearchBm25ChunksTool struct {
	tenantID   string
	datasetIDs []string
}

// NewSearchBm25ChunksTool returns a SearchBm25ChunksTool scoped to the given
// tenant and datasets, implementing eino's InvokableTool.
func NewSearchBm25ChunksTool(tenantID string, datasetIDs []string) *SearchBm25ChunksTool {
	return &SearchBm25ChunksTool{tenantID: tenantID, datasetIDs: datasetIDs}
}

// Info returns the tool metadata with a JSON-schema parameter block, mirroring
// search_chunks' conventions.
func (k *SearchBm25ChunksTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	schemaJSON := `{
  "type": "object",
  "properties": {
    "queries": {
      "type": "array",
      "description": "REQUIRED: 1-5 short KEYWORD queries (entities, names, phrases). Each is scored independently with BM25; results are merged. Example: [\"马元义\", \"马元义 斩\"] — NOT full sentences.",
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
      "description": "Optional document ids to restrict retrieval to (at most 10, normally taken from grep_chunks results).",
      "items": { "type": "string" },
      "maxItems": 10
    },
    "top_n": {
      "type": "integer",
      "description": "Number of chunks to return per query, 1-50 (default 12).",
      "minimum": 0,
      "maximum": 50
    }
  },
  "required": ["queries"]
}`
	s := &jsonschema.Schema{}
	if err := json.Unmarshal([]byte(schemaJSON), s); err != nil {
		return nil, fmt.Errorf("search_bm25_chunks: parse schema: %w", err)
	}
	return &schema.ToolInfo{
		Name:        searchBm25ChunksToolName,
		Desc:        searchBm25ChunksToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByJSONSchema(s),
	}, nil
}

// InvokableRun validates arguments and delegates to the registered Bm25Service.
func (k *SearchBm25ChunksTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	return guardedToolRun(ctx, searchBm25ChunksToolName, k.invokableRun, argumentsInJSON)
}

func (k *SearchBm25ChunksTool) invokableRun(ctx context.Context, argumentsInJSON string) (string, error) {
	var args searchBm25ChunksArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("search_bm25_chunks: parse arguments: %w", err)
	}

	queries := make([]string, 0, len(args.Queries))
	for _, q := range args.Queries {
		if q = strings.TrimSpace(q); q != "" {
			queries = append(queries, q)
		}
	}
	if len(queries) == 0 {
		return "", fmt.Errorf("search_bm25_chunks: queries must contain 1-5 non-empty keyword queries")
	}
	if len(queries) > 5 {
		return "", fmt.Errorf("search_bm25_chunks: queries must contain at most 5 keyword queries, got %d", len(queries))
	}

	topN := args.TopN
	if topN <= 0 {
		topN = searchBm25DefaultTopN
	}
	if topN > 50 {
		topN = 50
	}
	datasetIDs, err := resolveDatasetScope(k.datasetIDs, args.DatasetIDs)
	if err != nil {
		return "", fmt.Errorf("search_bm25_chunks: %w", err)
	}
	if len(datasetIDs) > 10 {
		datasetIDs = datasetIDs[:10]
	}
	if len(args.DocScope) > 10 {
		args.DocScope = args.DocScope[:10]
	}

	svc := runtime.GetBm25Service()
	chunks, err := svc.SearchBm25(ctx, runtime.Bm25Request{
		Queries:    queries,
		DatasetIDs: datasetIDs,
		DocScope:   args.DocScope,
		TopN:       topN,
		TenantID:   k.tenantID,
	})
	if err != nil {
		return "", fmt.Errorf("search_bm25_chunks: %w", err)
	}

	common.DebugCtx(ctx, "agentic_rag: search_bm25_chunks result",
		zap.Int("chunks", len(chunks)),
		zap.Strings("queries", queries),
	)
	hits := snippetHitsFor(chunks, queries)
	return formatLocateResultsXML(ctx, searchBm25ChunksToolName, strings.Join(queries, " | "), hits), nil
}

// bm25TermTokens derives lowercase literal search terms from the query strings:
// whitespace-split words with surrounding punctuation trimmed. Purely for
// snippet anchoring — scoring itself happens inside the engine's BM25 index.
func bm25TermTokens(queries []string) []string {
	var terms []string
	for _, q := range queries {
		for _, w := range strings.Fields(q) {
			w = strings.ToLower(strings.Trim(w, ".,;:!?()\"'“”‘’、。，；：！？"))
			if utf8.RuneCountInString(w) >= 2 || w != "" && hasCJK(w) {
				terms = append(terms, w)
			}
		}
	}
	return terms
}

func hasCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

// snippetHitsFor renders pre-computed match_snippets for a located chunk set,
// centred on the whole span of term hits (earliest start − N .. latest end + N,
// shared snippetContextRunes half-window).
func snippetHitsFor(chunks []runtime.RetrievalChunk, queries []string) []snippetHit {
	terms := bm25TermTokens(queries)
	hits := make([]snippetHit, 0, len(chunks))
	for _, c := range chunks {
		snippet, truncated, preview := "", false, false
		if first, last, ok := termMatchSpan(terms, c.Content); ok {
			snippet, truncated = snippetForMatches(c.Content, first, last)
		} else {
			// Lexical scoring matched a stemmed form the surface query does not
			// contain: ship the chunk's opening and label it a preview rather
			// than handing the model a chunk_id with no text at all.
			snippet, truncated = previewForChunk(c.Content)
			preview = snippet != ""
		}
		hits = append(hits, snippetHit{chunk: c, snippet: snippet, truncated: truncated, preview: preview})
	}
	return hits
}
