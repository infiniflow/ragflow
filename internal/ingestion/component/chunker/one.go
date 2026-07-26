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

// OneChunker emits a single chunk per upstream document. It is the
// faithful Go port of the Python `one` chunk method
// (rag/app/one.py) and also covers the `picture` / `audio` methods,
// whose Python chunk() functions return exactly one chunk per file
// (rag/app/picture.py, rag/app/audio.py) — the latter additionally
// carrying the raw image / media context, which this chunker preserves.
//
// Unlike TokenChunker in "one" mode (which drops per-item media
// context), OneChunker carries the image attachment and doc_type of a
// single upstream item through unchanged, so picture/audio pipelines
// keep their media payload.
package chunker

import (
	"context"
	"fmt"
	"strings"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
)

const ComponentNameOneChunker = "OneChunker"

type oneChunkerParam struct{}

func (p *oneChunkerParam) Update(conf map[string]any) {}

func (oneChunkerParam) Defaults() oneChunkerParam { return oneChunkerParam{} }

func (oneChunkerParam) Validate() error { return nil }

type OneChunkerComponent struct {
	name  string
	param oneChunkerParam
}

func NewOneChunker(params map[string]any) (runtime.Component, error) {
	p := oneChunkerParam{}.Defaults()
	(&p).Update(params)
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &OneChunkerComponent{
		name:  ComponentNameOneChunker,
		param: p,
	}, nil
}
func (c *OneChunkerComponent) Inputs() map[string]string { return ChunkerInputs }

func (c *OneChunkerComponent) Outputs() map[string]string { return ChunkerOutputs }

func (c *OneChunkerComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	return c.invoke(ctx, inputs)
}

func (c *OneChunkerComponent) invoke(_ context.Context, inputs map[string]any) (map[string]any, error) {
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
		return emitOneFromItems(upstream.JSONResult, upstream.Chunks), nil
	}
}

// emitOne wraps a single text payload as one chunk.
func emitOne(text, docType string) map[string]any {
	if strings.TrimSpace(text) == "" {
		return emptyOutputs()
	}
	return chunkOutputs([]schema.ChunkDoc{{
		Text:    text,
		DocType: docType,
		CKType:  docType,
	}})
}

// emitOneFromItems collapses a structured upstream payload into a single
// chunk. When the payload is a single item, its media context (image)
// and doc_type are preserved. When it is many items, their text is
// concatenated and the first available image attachment is carried over —
// mirroring the Python "one chunk per file" behavior for picture/audio,
// where each upstream item is one page/slide/transcript segment of the
// same source file.
func emitOneFromItems(items, chunks []schema.ChunkDoc) map[string]any {
	src := items
	if len(src) == 0 {
		src = chunks
	}
	if len(src) == 0 {
		return emptyOutputs()
	}
	if len(src) == 1 {
		it := src[0]
		docType := itemDocType(it)
		text := itemTextOrFallback(it)
		if strings.TrimSpace(text) == "" && it.Image == "" {
			return emptyOutputs()
		}
		out := schema.ChunkDoc{
			Text:    text,
			DocType: docType,
			CKType:  docType,
			Image:   it.Image,
		}
		return chunkOutputs([]schema.ChunkDoc{out})
	}

	var parts []string
	var img string
	for _, it := range src {
		if t := itemTextOrFallback(it); t != "" {
			parts = append(parts, t)
		}
		if img == "" && it.Image != "" {
			img = it.Image
		}
	}
	merged := strings.Join(parts, "\n")
	if strings.TrimSpace(merged) == "" && img == "" {
		return emptyOutputs()
	}
	out := schema.ChunkDoc{Text: merged, DocType: "text", CKType: "text"}
	if img != "" {
		out.Image = img
	}
	return chunkOutputs([]schema.ChunkDoc{out})
}

func init() {
	MustRegisterChunker(ComponentNameOneChunker)
}
