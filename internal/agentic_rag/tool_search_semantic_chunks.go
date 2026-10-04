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

// searchSemanticChunksToolName is the pure-vector member of the locate trio.
// The other two legs both consult the query's WORDS somewhere — grep_chunks
// matches them literally, search_chunks fuses a keyword leg with the vector leg
// (default 70% keyword) — so neither can cross a wording gap that the caller
// cannot guess. This one never looks at the words as literals at all.
const searchSemanticChunksToolName = "search_semantic_chunks"

const searchSemanticChunksToolDescription = `Pure-semantic search: ranks chunks by MEANING alone, with no keyword leg at all.

Use it when you can describe what you need but do NOT know how the corpus words it — and trigger it on a COUNT rather than on a feeling: once three lexical queries (grep_chunks / search_bm25_chunks) have gone at the same clue without leaving you a candidate you can test against another clue, run one probe here. This holds especially when the question asks for a NAME you have not guessed yet: guessing more names inside the same vocabulary is exactly what this tool exists to interrupt. Unlike search_chunks, this tool does not fuse your words into the ranking, and — decisively — it does NOT restrict the result set to chunks containing your terms: a hit may share no literal word with your query. That is what makes it the bridge across a wording gap, and it is also why it cannot help you find an exact literal: for an ID, a code, a quoted phrase, a spelling variant or a proper noun you already know, use grep_chunks or search_bm25_chunks instead.

Write each query as a DESCRIPTION of the thing you are looking for, in the corpus's own subject matter — as if explaining it to a person who knows the domain. Describe what it IS and what is true about it, not the question's abstract wording: for a question phrased "a family business ... a hallmark award from a global brand in the 1990s", query "an Italian family restaurant in Lombardy awarded three Michelin stars in 1996" — and if that comes back off-target, change the CATEGORY you describe (restaurant -> hotel -> winery -> food producer), because the wording gap may be an ontology gap rather than a synonym gap.

Accepts 1-5 queries. Output is the same <search_results> XML as the other locate tools; because a semantic hit usually shares no literal term with the query, its <match_snippet> is normally the chunk's own opening, marked match="none". Judge relevance from that text and deep-read with list_chunks before relying on it.`

// searchSemanticChunksArgs is the JSON the model sends into InvokableRun. It
// deliberately has NO keywords_similarity_weight: the whole point of this tool
// is that the keyword leg cannot be dialled back in, so the field would only
// invite the caller to rebuild search_chunks by hand.
type searchSemanticChunksArgs struct {
	Queries             []string `json:"queries"`
	DatasetIDs          []string `json:"dataset_ids,omitempty"`
	DocScope            []string `json:"doc_scope,omitempty"`
	TopN                int      `json:"top_n,omitempty"`
	SimilarityThreshold *float64 `json:"similarity_threshold,omitempty"`
}

// SearchSemanticChunksTool is the vector-only locate tool. The tenant and
// dataset scope are injected at construction from the session.
type SearchSemanticChunksTool struct {
	tenantID   string
	datasetIDs []string
}

// NewSearchSemanticChunksTool returns a SearchSemanticChunksTool scoped to the
// given tenant and datasets, implementing eino's runtime.InvokableTool.
func NewSearchSemanticChunksTool(tenantID string, datasetIDs []string) *SearchSemanticChunksTool {
	return &SearchSemanticChunksTool{tenantID: tenantID, datasetIDs: datasetIDs}
}

// Info returns the tool's metadata for the chat model. The schema mirrors
// search_chunks minus the fusion weight, so both legs stay comparable for the
// model: same query shape, same bounds, same output XML.
func (k *SearchSemanticChunksTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	schemaJSON := `{
  "type": "object",
  "properties": {
    "queries": {
      "type": "array",
      "description": "REQUIRED: 1-5 natural-language DESCRIPTIONS of what you are looking for (the thing and its properties), each embedded and matched by meaning. Do NOT pass the question's abstract wording verbatim, and do not expect literal matching: this tool ignores your words as tokens. When a description comes back off-target, describe a DIFFERENT category of thing (restaurant / hotel / winery / manufacturer) rather than rephrasing the same one.",
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
      "description": "Optional document ids to restrict retrieval to (at most 10). When set, only chunks of those documents are searched.",
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
      "description": "Minimum vector similarity (0.0-1.0) for a chunk to be returned; higher is stricter (fewer, more relevant results). Default 0.2.",
      "minimum": 0,
      "maximum": 1
    }
  },
  "required": ["queries"]
}`
	s := &jsonschema.Schema{}
	if err := json.Unmarshal([]byte(schemaJSON), s); err != nil {
		return nil, fmt.Errorf("%s: parse schema: %w", searchSemanticChunksToolName, err)
	}
	return &schema.ToolInfo{
		Name:        searchSemanticChunksToolName,
		Desc:        searchSemanticChunksToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByJSONSchema(s),
	}, nil
}

// InvokableRun performs the dense-only search and renders the shared locate
// payload.
func (k *SearchSemanticChunksTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	return guardedToolRun(ctx, searchSemanticChunksToolName, k.invokableRun, argumentsInJSON)
}

func (k *SearchSemanticChunksTool) invokableRun(ctx context.Context, argumentsInJSON string) (string, error) {
	var args searchSemanticChunksArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("%s: parse arguments: %w", searchSemanticChunksToolName, err)
	}
	return runLocateSearch(ctx, locateSearchSpec{
		tool:                searchSemanticChunksToolName,
		queries:             args.Queries,
		datasetIDs:          args.DatasetIDs,
		boundDatasetIDs:     k.datasetIDs,
		docScope:            args.DocScope,
		topN:                args.TopN,
		similarityThreshold: args.SimilarityThreshold,
		// The unclamped keyword share is 0 BY CONSTRUCTION: the caller cannot
		// raise it, which is the difference between this tool and
		// search_chunks(weight=0).
		weight:     0,
		vectorOnly: true,
		tenantID:   k.tenantID,
	})
}
