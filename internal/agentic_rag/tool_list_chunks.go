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
	"sort"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
	"go.uber.org/zap"

	"ragflow/internal/agent/tool"
	"ragflow/internal/common"
)

// listChunksToolName is the constrained deep-read tool. It pages through ALL
// chunks of exactly ONE document in reading order (chunk_index) within an
// offset/limit range. The document is scoped by doc_id (authoritative and
// unique — it can only belong to one dataset), and each returned chunk carries
// its owning dataset_id in the output.
const listChunksToolName = "list_chunks"

const listChunksToolDescription = `Read the FULL original text of ONE dataset document in reading order (Deep Read).

Use this AFTER grep_chunks / search_chunks locate a document: pass its doc_id so you can read the complete chunk text — including surrounding context that grep snippets and graph triples omit.

## Input
- doc_id: REQUIRED — the document id to read (use the doc_id value from grep_chunks / search_chunks output). This uniquely identifies the document.
- limit / offset: pagination over the document's chunks (default limit 20, max 100; offset default 0). Chunks are returned in reading order (chunk_index). When a page fills up (fetched == limit) and more chunks may remain, call again with a larger offset to read the next page.

## Output (XML)
Returns an XML <chunks> document with doc_id, offset, limit and fetched attributes. Each chunk is a <chunk> element carrying chunk_id, doc_id, page_num, chunk_index, dataset_id, doc_title and a <content> element with the FULL original chunk text, ordered by reading order (doc_id, page_num, chunk_index) so you can read sequentially. A <pagination next_offset=.../> element signals that more pages remain. Graph relation/entity chunks are excluded.`

// listChunksArgs is the JSON the model sends into InvokableRun.
type listChunksArgs struct {
	DocID  string `json:"doc_id"`
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

// listChunksDefaults bound the deep-read page size.
const (
	listChunksDefaultLimit = 20
	listChunksMaxLimit     = 100
)

// deepReadService is the capability list_chunks needs on top of
// GrepService. *GrepAdapter implements it.
type deepReadService interface {
	tool.GrepService
	ListByDocIDs(ctx context.Context, req tool.GrepRequest) ([]tool.RetrievalChunk, error)
}

// ListChunksTool reads the full original chunks of one document.
type ListChunksTool struct{}

// NewListChunksTool returns a ListChunksTool implementing eino's
// tool.InvokableTool.
func NewListChunksTool() *ListChunksTool {
	return &ListChunksTool{}
}

// Info returns the tool's metadata for the chat model. The parameter schema is
// declared as a single JSON string so the model sees the required single-id
// contract and numeric ranges in one readable block, then parsed into
// *jsonschema.Schema.
func (l *ListChunksTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	schemaJSON := `{
  "type": "object",
  "properties": {
    "doc_id": {
      "type": "string",
      "description": "REQUIRED: the document id to read (use the doc_id value from grep_chunks / search_chunks output)."
    },
    "limit": {
      "type": "integer",
      "description": "Max number of chunks to return per call (default 20, max 100).",
      "default": 20,
      "minimum": 1,
      "maximum": 100
    },
    "offset": {
      "type": "integer",
      "description": "Number of chunks to skip (0-based) for pagination (default 0).",
      "default": 0,
      "minimum": 0
    }
  },
  "required": ["doc_id"]
}`
	s := &jsonschema.Schema{}
	if err := json.Unmarshal([]byte(schemaJSON), s); err != nil {
		return nil, fmt.Errorf("list_chunks: parse schema: %w", err)
	}
	return &schema.ToolInfo{
		Name:        listChunksToolName,
		Desc:        listChunksToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByJSONSchema(s),
	}, nil
}

// InvokableRun reads the full original chunks of one document within the
// offset/limit range, ordered by reading order (chunk_index).
func (l *ListChunksTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	var args listChunksArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("list_chunks: parse arguments: %w", err)
	}

	docID := strings.TrimSpace(args.DocID)
	if docID == "" {
		return "", fmt.Errorf("list_chunks: doc_id is required")
	}

	limit := args.Limit
	if limit <= 0 {
		limit = listChunksDefaultLimit
	}
	if limit > listChunksMaxLimit {
		limit = listChunksMaxLimit
	}
	offset := max(args.Offset, 0)

	svc := tool.GetGrepService()
	tenantID := tool.CanvasTenantID(ctx)
	dr, ok := svc.(deepReadService)
	if svc == nil || tenantID == "" {
		return chunksXMLEmpty(), nil
	}
	if !ok {
		return "", fmt.Errorf("list_chunks: configured grep service does not support deep read")
	}

	// Push the document's reading order down to the engine so ES applies
	// offset/limit over the deterministically-ordered chunks — no over-fetching
	// of offset+limit just to re-sort in memory. A stable in-memory re-sort by
	// reading order is kept as a defensive no-op (the engine already returns the
	// page in reading order) so the output order never depends on engine choice.
	chunks, err := dr.ListByDocIDs(ctx, tool.GrepRequest{
		DocScope:     []string{docID},
		DatasetIDs:   nil, // a doc_id uniquely identifies the document across all datasets
		Limit:        limit,
		Offset:       offset,
		Sort:         grepChunksSortFields, // doc_id, page_num_int, chunk_order_int
		SelectFields: grepChunksSelectFields,
		TenantID:     tenantID,
	})
	if err != nil {
		return "", fmt.Errorf("list_chunks: %w", err)
	}
	sort.SliceStable(chunks, func(i, j int) bool {
		return readingOrderLess(chunks[i], chunks[j])
	})

	common.Debug("agentic_rag: list_chunks result",
		zap.Int("chunks", len(chunks)),
		zap.String("doc_id", docID),
		zap.Int("limit", limit),
		zap.Int("offset", offset),
	)

	return formatChunksXML(docID, chunks, limit, offset), nil
}

// chunksXMLEmpty returns an empty deep-read result set in the same XML shape as
// formatChunksXML.
func chunksXMLEmpty() string {
	return `<chunks fetched="0">
</chunks>`
}
