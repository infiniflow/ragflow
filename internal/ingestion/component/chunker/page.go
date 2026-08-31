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

// PageChunker emits one chunk per upstream slide or page.
// It is the faithful Go port of the Python `presentation` chunk method
// (rag/app/presentation.py), whose docstring states: "Every page will
// be treated as a chunk."
//
// Unlike TokenChunker (which merges slides into a single chunk) or
// OneChunker (which collapses many slides into one), PageChunker
// keeps each slide as the unit of chunking. The upstream parser produces
// one record per slide with text and slide_number; this chunker passes
// each through unchanged. Note that the PPTX/PPT path does not emit image
// or position information (unlike the PDF path), so slide chunks carry no
// image and no bbox-based preview positioning.
package chunker

import (
	"context"
	"fmt"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"

	"gorm.io/gorm"
)

const ComponentNamePageChunker = "PageChunker"

type pageChunkerParam struct{}

func (p *pageChunkerParam) Update(conf map[string]any) {}

func (pageChunkerParam) Defaults() pageChunkerParam {
	return pageChunkerParam{}
}

func (pageChunkerParam) Validate() error { return nil }

type PageChunkerComponent struct {
	name  string
	param pageChunkerParam
}

func NewPageChunker(params map[string]any) (runtime.Component, error) {
	p := pageChunkerParam{}.Defaults()
	(&p).Update(params)
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &PageChunkerComponent{
		name:  ComponentNamePageChunker,
		param: p,
	}, nil
}
func (c *PageChunkerComponent) Inputs() map[string]string { return ChunkerInputs }

func (c *PageChunkerComponent) Outputs() map[string]string { return ChunkerOutputs }

func (c *PageChunkerComponent) Invoke(ctx context.Context, db *gorm.DB, inputs map[string]any) (map[string]any, error) {
	return c.invoke(ctx, inputs)
}

func (c *PageChunkerComponent) invoke(_ context.Context, inputs map[string]any) (map[string]any, error) {
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

	// The presentation template only configures pdf and slides
	// parser setups, matching Python's restriction to .pptx/.ppt/.pdf.
	// The upstream parser therefore always emits per-slide JSON, never
	// flat text/markdown/html, so every payload keeps one-chunk-per-slide.
	items := slideItems(upstream.JSONResult, upstream.Chunks)
	if len(items) == 0 {
		return emptyOutputs(), nil
	}
	return chunkOutputs(items), nil
}

// slideItems returns the per-slide records, preferring JSONResult and
// falling back to Chunks. Each record (slide/page) becomes one chunk.
func slideItems(items, chunks []schema.ChunkDoc) []schema.ChunkDoc {
	if len(items) > 0 {
		return items
	}
	return chunks
}

func init() {
	MustRegisterChunker(ComponentNamePageChunker)
}
