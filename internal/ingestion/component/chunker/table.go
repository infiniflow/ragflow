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

// TableChunker emits one chunk per upstream table row. It is the faithful
// Go port of the Python `table` chunk method (rag/app/table.py), whose
// docstring states: "Every row in table will be treated as a chunk."
//
// Unlike TokenChunker (which token-shreds a row's text into multiple
// pieces) or OneChunker (which merges many rows into a single chunk),
// TableChunker keeps the row as the unit of chunking. The table parser is
// responsible for producing one structured record (ChunkDoc) per row with
// the formatted "- field: value" content and any column-role / field_map
// metadata; this chunker passes each record through unchanged, one chunk
// per row.
package chunker

import (
	"context"
	"fmt"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
)

const ComponentNameTableChunker = "TableChunker"

type tableChunkerParam struct{}

func (p *tableChunkerParam) Update(conf map[string]any) {}

func (tableChunkerParam) Defaults() tableChunkerParam { return tableChunkerParam{} }

func (tableChunkerParam) Validate() error { return nil }

type TableChunkerComponent struct {
	name  string
	param tableChunkerParam
}

func NewTableChunker(params map[string]any) (runtime.Component, error) {
	p := tableChunkerParam{}.Defaults()
	(&p).Update(params)
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &TableChunkerComponent{
		name:  ComponentNameTableChunker,
		param: p,
	}, nil
}
func (c *TableChunkerComponent) Inputs() map[string]string { return ChunkerInputs }

func (c *TableChunkerComponent) Outputs() map[string]string { return ChunkerOutputs }

func (c *TableChunkerComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	return c.invoke(ctx, inputs)
}

func (c *TableChunkerComponent) invoke(_ context.Context, inputs map[string]any) (map[string]any, error) {
	if inputs == nil {
		return emptyOutputs(), nil
	}
	upstream, err := decodeChunkerFromUpstream(inputs)
	if err != nil {
		return map[string]any{
			"output_format": "chunks",
			"chunks":        []map[string]any{},
			"_ERROR":        fmt.Sprintf("Input error: %v", err),
		}, nil
	}

	switch upstream.OutputFormat {
	case schema.PayloadFormatMarkdown:
		if upstream.MarkdownResult == nil {
			return emptyOutputs(), nil
		}
		return emitOne(*upstream.MarkdownResult, "text"), nil
	case schema.PayloadFormatText:
		if upstream.TextResult == nil {
			return emptyOutputs(), nil
		}
		return emitOne(*upstream.TextResult, "text"), nil
	case schema.PayloadFormatHTML:
		if upstream.HTMLResult == nil {
			return emptyOutputs(), nil
		}
		return emitOne(*upstream.HTMLResult, "text"), nil
	default:
		// Row-structured payload: one chunk per upstream record.
		items := tableItems(upstream.JSONResult, upstream.Chunks)
		if len(items) == 0 {
			return emptyOutputs(), nil
		}
		return chunkOutputs(items), nil
	}
}

// tableItems returns the per-row records, preferring JSONResult and
// falling back to Chunks. Each record becomes exactly one chunk.
func tableItems(items, chunks []schema.ChunkDoc) []schema.ChunkDoc {
	if len(items) > 0 {
		return items
	}
	return chunks
}

func init() {
	MustRegisterChunker(ComponentNameTableChunker)
}
