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
// faithful Go port of the `one` chunk method and also covers the `picture`
// / `audio` methods, whose chunk() functions return exactly one chunk per
// file — the latter additionally carrying the raw image / media context,
// which this chunker preserves.
//
// Unlike TokenChunker in "one" mode (which drops per-item media
// context), OneChunker carries the image attachment and doc_type of a
// single upstream item through unchanged, so picture/audio pipelines
// keep their media payload.
package chunker

import (
	"context"
	"encoding/json"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/schema"
)

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

func (c *OneChunkerComponent) Invoke(ctx context.Context, db *gorm.DB, inputs map[string]any) (map[string]any, error) {
	return c.invoke(ctx, db, inputs)
}

// invoke collapses the decoded upstream payload into a single chunk and crops
// a PDF preview image when valid PDF coordinates and a source PDF are present.
func (c *OneChunkerComponent) invoke(ctx context.Context, db *gorm.DB, inputs map[string]any) (map[string]any, error) {
	if inputs == nil {
		return emptyOutputs(), nil
	}
	upstream, err := decodeChunkerFromUpstream(inputs)
	if err != nil {
		return inputErrorOutput(err), nil
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
		out, fallbackImg, ok := buildOneChunkFromItems(upstream.JSONResult, upstream.Chunks, isOneChunkPDFCandidate(upstream))
		if !ok {
			return emptyOutputs(), nil
		}
		chunks := []schema.ChunkDoc{out}
		if isOneChunkPDFCandidate(upstream) && out.Image == "" && needsCrop(out) {
			engine, engErr := newPDFEngineFromUpstream(ctx, db, upstream)
			if engErr != nil {
				common.Warn("OneChunker: could not open PDF for on-demand cropping", zap.Error(engErr))
			}
			if engine != nil {
				defer engine.Close()
				chunks = cropImageChunks(ctx, engine, chunks)
			}
		}
		if chunks[0].Image == "" && chunks[0].ImgID == "" && fallbackImg != "" {
			chunks[0].Image = fallbackImg
		}
		return chunkOutputs(chunks), nil
	}
}

// emitOne wraps a single text payload as one chunk.
func emitOne(text, docType string) map[string]any {
	if strings.TrimSpace(text) == "" {
		return emptyOutputs()
	}
	return chunkOutputs([]schema.ChunkDoc{{
		// Strip parser coordinate tags so they never reach embedding/index,
		// matching the flow chunker's tag-free "one" output
		// ( delimiter_mode one). The legacy
		// predates tag stripping and is intentionally out of
		// scope here.
		Text:    removeTag(text),
		DocType: docType,
	}})
}

// emitOneFromItems collapses a structured upstream payload into a single
// chunk. When the payload is a single item, its media context (image),
// coordinates, and doc_type are preserved. When it is many items, their text is
// concatenated, PDF coordinates are merged, and the first available image
// attachment is carried over as a fallback when no PDF preview crop is produced.
func emitOneFromItems(items, chunks []schema.ChunkDoc) map[string]any {
	out, fallbackImg, ok := buildOneChunkFromItems(items, chunks, true)
	if !ok {
		return emptyOutputs()
	}
	if out.Image == "" && fallbackImg != "" {
		out.Image = fallbackImg
	}
	return chunkOutputs([]schema.ChunkDoc{out})
}

func buildOneChunkFromItems(items, chunks []schema.ChunkDoc, pdfCandidate bool) (schema.ChunkDoc, string, bool) {
	src := items
	if len(src) == 0 {
		src = chunks
	}
	if len(src) == 0 {
		return schema.ChunkDoc{}, "", false
	}
	if len(src) == 1 {
		it := src[0]
		docType := itemDocType(it)
		text := itemTextOrFallback(it)
		if strings.TrimSpace(text) == "" && !hasMediaPayload(it) {
			return schema.ChunkDoc{}, "", false
		}
		out := schema.ChunkDoc{
			// Strip parser coordinate tags (see emitOne).
			Text:         removeTag(text),
			DocType:      docType,
			Image:        it.Image,
			ImgID:        it.ImgID,
			MediaRef:     it.MediaRef,
			Positions:    it.Positions,
			PDFPositions: it.PDFPositions,
			Sheet:        it.Sheet,
			SheetIndex:   it.SheetIndex,
		}
		return out, "", true
	}

	var parts []string
	var img string
	var mediaRef *entity.MediaRef
	mergePositions := pdfCandidate && !hasOneChunkSpreadsheetItems(src)
	pdfSources := make([]json.RawMessage, 0, len(src))
	posSources := make([]json.RawMessage, 0, len(src))
	for _, it := range src {
		if t := itemTextOrFallback(it); t != "" {
			// Strip parser coordinate tags before joining (see emitOne).
			parts = append(parts, removeTag(t))
		}
		if img == "" && it.Image != "" {
			img = it.Image
		}
		if mediaRef == nil && it.MediaRef != nil {
			mediaRef = it.MediaRef
		}
		if mergePositions {
			if len(it.PDFPositions) > 0 {
				pdfSources = append(pdfSources, it.PDFPositions)
			}
			if len(it.Positions) > 0 {
				posSources = append(posSources, it.Positions)
			}
		}
	}
	merged := strings.Join(parts, "\n")
	if strings.TrimSpace(merged) == "" && img == "" && mediaRef == nil {
		return schema.ChunkDoc{}, "", false
	}
	out := schema.ChunkDoc{
		Text:     merged,
		DocType:  "text",
		MediaRef: mediaRef,
	}
	if mergePositions {
		out.PDFPositions = mergeRawPositionMatrix(pdfSources...)
		out.Positions = mergeRawPositionMatrix(posSources...)
	}
	if !pdfCandidate || !hasCropRegion(out) {
		if img != "" {
			out.Image = img
		}
		return out, "", true
	}
	// Defer inline sub-item image fallback until PDF preview cropping has run,
	// so a single embedded figure does not shadow the full-document PDF crop.
	return out, img, true
}

func mergeRawPositionMatrix(sources ...json.RawMessage) json.RawMessage {
	rows := mergePositionRows(sources, func(row []any) bool {
		return len(row) == 5
	})
	if len(rows) == 0 {
		return nil
	}
	encoded, err := json.Marshal(rows)
	if err != nil {
		return nil
	}
	return encoded
}

func hasOneChunkSpreadsheetItems(items []schema.ChunkDoc) bool {
	for _, it := range items {
		if it.SheetIndex != nil || it.Sheet != "" {
			return true
		}
	}
	return false
}

func isOneChunkPDFCandidate(upstream schema.ChunkerFromUpstream) bool {
	ft := strings.ToLower(strings.TrimSpace(upstream.FileType))
	if ft == "" {
		ft = inferGeneralFileType(upstream)
	}
	return ft == "" || ft == "pdf"
}

func init() {
	MustRegisterChunker(ComponentNameOneChunker)
}
