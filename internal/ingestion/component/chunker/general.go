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

package chunker

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"gorm.io/gorm"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
)

const ComponentNameGeneralChunker = "GeneralChunker"

type generalChunkerParam struct {
	ChunkTokenSize     int
	Delimiters         []string
	OverlappedPercent  float64
	ChildrenDelimiters []string
	TableContextSize   int
	ImageContextSize   int
}

func (generalChunkerParam) Defaults() generalChunkerParam {
	return generalChunkerParam{
		ChunkTokenSize:     512,
		Delimiters:         []string{"\n"},
		ChildrenDelimiters: []string{},
	}
}

func (p *generalChunkerParam) Update(conf map[string]any) {
	if value, ok := schema.NumericFromAny(conf["chunk_token_size"]); ok {
		p.ChunkTokenSize = int(value)
	}
	if value, ok := conf["delimiters"].([]any); ok {
		p.Delimiters = stringListFromAny(value)
	} else if value, ok := conf["delimiters"].([]string); ok {
		p.Delimiters = append([]string(nil), value...)
	}
	if value, ok := conf["overlapped_percent"]; ok {
		p.OverlappedPercent = schema.NormalizeOverlappedPercent(value)
	}
	if value, ok := conf["children_delimiters"].([]any); ok {
		p.ChildrenDelimiters = stringListFromAny(value)
	} else if value, ok := conf["children_delimiters"].([]string); ok {
		p.ChildrenDelimiters = append([]string(nil), value...)
	}
	if value, ok := schema.NumericFromAny(conf["table_context_size"]); ok {
		p.TableContextSize = max(0, int(value))
	}
	if value, ok := schema.NumericFromAny(conf["image_context_size"]); ok {
		p.ImageContextSize = max(0, int(value))
	}
}

// GeneralChunkerComponent owns the general ingestion chunking policy and
// selects its format-specific strategy from the Parser's canonical file_type.
type GeneralChunkerComponent struct {
	param generalChunkerParam
}

func NewGeneralChunker(params map[string]any) (runtime.Component, error) {
	param := generalChunkerParam{}.Defaults()
	param.Update(params)
	return &GeneralChunkerComponent{param: param}, nil
}

func (c *GeneralChunkerComponent) Inputs() map[string]string { return ChunkerInputs }

func (c *GeneralChunkerComponent) Outputs() map[string]string { return ChunkerOutputs }

func (c *GeneralChunkerComponent) Invoke(ctx context.Context, db *gorm.DB, inputs map[string]any) (map[string]any, error) {
	upstream, err := decodeChunkerFromUpstream(inputs)
	if err != nil {
		return nil, fmt.Errorf("GeneralChunker: decode input: %w", err)
	}
	if strings.TrimSpace(upstream.FileType) == "" {
		return nil, fmt.Errorf("GeneralChunker: file_type is required")
	}

	switch generalStrategyForFileType(upstream.FileType) {
	case generalStrategyPDF:
		return c.chunkPDF(ctx, db, upstream)
	case generalStrategyDOCX:
		return c.chunkDOCX(ctx, upstream)
	case generalStrategyMarkdown:
		return c.chunkMarkdown(ctx, upstream)
	case generalStrategySpreadsheet:
		return c.chunkSpreadsheet(ctx, upstream)
	default:
		return c.chunkGeneral(ctx, upstream)
	}
}

type generalStrategy uint8

const (
	generalStrategyText generalStrategy = iota
	generalStrategyPDF
	generalStrategyDOCX
	generalStrategyMarkdown
	generalStrategySpreadsheet
)

func generalStrategyForFileType(fileType string) generalStrategy {
	switch fileType {
	case "pdf":
		return generalStrategyPDF
	case "docx":
		return generalStrategyDOCX
	case "md":
		return generalStrategyMarkdown
	case "xls", "xlsx", "csv":
		return generalStrategySpreadsheet
	default:
		return generalStrategyText
	}
}

func (c *GeneralChunkerComponent) chunkPDF(ctx context.Context, _ *gorm.DB, upstream schema.ChunkerFromUpstream) (map[string]any, error) {
	return c.chunkGeneral(ctx, upstream)
}

func (c *GeneralChunkerComponent) chunkDOCX(ctx context.Context, upstream schema.ChunkerFromUpstream) (map[string]any, error) {
	return c.chunkGeneral(ctx, upstream)
}

func (c *GeneralChunkerComponent) chunkMarkdown(ctx context.Context, upstream schema.ChunkerFromUpstream) (map[string]any, error) {
	return c.chunkGeneral(ctx, upstream)
}

func (c *GeneralChunkerComponent) chunkSpreadsheet(ctx context.Context, upstream schema.ChunkerFromUpstream) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("GeneralChunker: %w", err)
	}
	units := upstream.JSONResult
	if upstream.OutputFormat == schema.PayloadFormatChunks {
		units = upstream.Chunks
	}
	if len(units) == 0 {
		return emptyOutputs(), nil
	}
	chunks := make([]schema.ChunkDoc, 0, len(units))
	pendingRows := make([]schema.ChunkDoc, 0)
	flushRows := func() {
		if len(pendingRows) == 0 {
			return
		}
		chunks = append(chunks, mergeSpreadsheetRows(pendingRows, c.param.ChunkTokenSize)...)
		pendingRows = pendingRows[:0]
	}
	for _, unit := range units {
		if unit.CKType == "table_header" {
			flushRows()
			continue
		}
		if unit.CKType == "table_row" {
			pendingRows = append(pendingRows, unit)
			continue
		}
		flushRows()
		chunks = append(chunks, cloneChunkDoc(unit))
	}
	flushRows()
	childrenPattern := compileChildrenPattern(c.param.ChildrenDelimiters)
	chunks = finalizeGeneralChunks(chunks, childrenPattern)
	if len(chunks) == 0 {
		return emptyOutputs(), nil
	}
	return chunkOutputs(chunks), nil
}

func (c *GeneralChunkerComponent) chunkGeneral(ctx context.Context, upstream schema.ChunkerFromUpstream) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("GeneralChunker: %w", err)
	}
	units := upstream.JSONResult
	if upstream.OutputFormat == schema.PayloadFormatChunks {
		units = upstream.Chunks
	}
	if len(units) == 0 {
		return emptyOutputs(), nil
	}
	primaryPattern := compileDelimPattern(c.param.Delimiters)
	childrenPattern := compileChildrenPattern(c.param.ChildrenDelimiters)
	units = splitGeneralUnits(units, primaryPattern)
	if !hasCustomDelim(c.param.Delimiters) {
		units = mergeGeneralUnits(units, c.param.ChunkTokenSize, c.param.OverlappedPercent, "\n")
	}
	units = finalizeGeneralChunks(units, childrenPattern)
	if len(units) == 0 {
		return emptyOutputs(), nil
	}
	return chunkOutputs(units), nil
}

// mergeGeneralUnits implements the general OVER_CAP policy. A text unit is
// appended while the current chunk is at or below the overlap-scaled target;
// the append itself may cross the target. Oversized units remain indivisible.
func mergeGeneralUnits(units []schema.ChunkDoc, target int, overlapPct float64, joinSep string) []schema.ChunkDoc {
	overlapPct = max(0, min(100, overlapPct))
	threshold := float64(target) * (100 - overlapPct) / 100
	merged := make([]schema.ChunkDoc, 0, len(units))
	current := -1

	for _, unit := range units {
		if itemDocType(unit) != "text" {
			merged = append(merged, cloneChunkDoc(unit))
			current = -1
			continue
		}
		if strings.TrimSpace(unit.Text) == "" {
			continue
		}

		count := generalUnitTokens(unit)
		unit.DocType = "text"
		unit.CKType = "text"
		unit.TKNums = intPtr(count)
		if count > target {
			merged = append(merged, cloneChunkDoc(unit))
			current = -1
			continue
		}
		if current < 0 || float64(intValue(merged[current].TKNums)) > threshold {
			merged = append(merged, cloneChunkDoc(unit))
			current = len(merged) - 1
			continue
		}

		mergeGeneralChunk(&merged[current], unit, joinSep)
	}

	return applyGeneralOverlap(merged, overlapPct)
}

func mergeSpreadsheetRows(rows []schema.ChunkDoc, target int) []schema.ChunkDoc {
	merged := make([]schema.ChunkDoc, 0, len(rows))
	current := -1
	for _, row := range rows {
		if row.CKType != "table_row" || itemDocType(row) != "text" {
			merged = append(merged, cloneChunkDoc(row))
			current = -1
			continue
		}
		count := generalUnitTokens(row)
		if current < 0 || count > target || !sameSpreadsheetTable(merged[current], row) || intValue(merged[current].TKNums)+count > target {
			row.TKNums = intPtr(count)
			merged = append(merged, cloneChunkDoc(row))
			current = len(merged) - 1
			continue
		}
		mergeGeneralChunk(&merged[current], row, "\n")
		mergeSpreadsheetRowRange(&merged[current], row)
	}
	return merged
}

func sameSpreadsheetTable(first, second schema.ChunkDoc) bool {
	if first.TableID != "" || second.TableID != "" {
		return first.TableID == second.TableID
	}
	if first.SheetIndex != nil || second.SheetIndex != nil {
		return first.SheetIndex != nil && second.SheetIndex != nil && *first.SheetIndex == *second.SheetIndex
	}
	return first.Sheet == second.Sheet
}

func mergeSpreadsheetRowRange(dst *schema.ChunkDoc, src schema.ChunkDoc) {
	dst.RowStart = minSpreadsheetInt(dst.RowStart, src.RowStart, false)
	dst.RowEnd = minSpreadsheetInt(dst.RowEnd, src.RowEnd, true)
	dst.ColStart = minSpreadsheetInt(dst.ColStart, src.ColStart, false)
	dst.ColEnd = minSpreadsheetInt(dst.ColEnd, src.ColEnd, true)
}

func minSpreadsheetInt(first, second *int, maximum bool) *int {
	if first == nil {
		if second == nil {
			return nil
		}
		value := *second
		return &value
	}
	if second == nil {
		value := *first
		return &value
	}
	value := *first
	if (maximum && *second > value) || (!maximum && *second < value) {
		value = *second
	}
	return &value
}

func generalUnitTokens(unit schema.ChunkDoc) int {
	if unit.TKNums != nil && *unit.TKNums > 0 {
		return *unit.TKNums
	}
	return tokenizeStr(unit.Text)
}

func mergeGeneralChunk(dst *schema.ChunkDoc, src schema.ChunkDoc, joinSep string) {
	if dst.Text != "" && src.Text != "" {
		dst.Text += joinSep
	}
	dst.Text += src.Text
	dst.TKNums = intPtr(intValue(dst.TKNums) + generalUnitTokens(src))
	dst.PDFPositions = mergeGeneralPositions(dst.PDFPositions, src.PDFPositions)
	dst.Positions = mergeGeneralPositions(dst.Positions, src.Positions)
	mergeGeneralMetadata(dst, src)
}

func mergeGeneralMetadata(dst *schema.ChunkDoc, src schema.ChunkDoc) {
	if dst.Image == "" {
		dst.Image = src.Image
	}
	if dst.ImgID == "" {
		dst.ImgID = src.ImgID
	}
	if dst.Layout == "" {
		dst.Layout = src.Layout
	}
	if dst.LayoutType == "" {
		dst.LayoutType = src.LayoutType
	}
	if dst.LayoutNo == "" {
		dst.LayoutNo = src.LayoutNo
	}
	if dst.PageNumber == nil {
		dst.PageNumber = src.PageNumber
	}
	if dst.Extra == nil && len(src.Extra) > 0 {
		dst.Extra = make(map[string]json.RawMessage, len(src.Extra))
	}
	for key, value := range src.Extra {
		if _, exists := dst.Extra[key]; !exists {
			dst.Extra[key] = append(json.RawMessage(nil), value...)
		}
	}
}

func mergeGeneralPositions(first, second json.RawMessage) json.RawMessage {
	if positions := mergePositionMatrix(first, second); len(positions) > 0 {
		encoded, err := json.Marshal(positions)
		if err == nil {
			return encoded
		}
	}
	return extendRawJSONArray(first, second)
}

func applyGeneralOverlap(chunks []schema.ChunkDoc, overlapPct float64) []schema.ChunkDoc {
	if overlapPct <= 0 {
		return chunks
	}
	previousText := -1
	for i := range chunks {
		if itemDocType(chunks[i]) != "text" {
			previousText = -1
			continue
		}
		if previousText >= 0 {
			prefix, _ := computeOverlapPrefix(chunks[previousText].Text, overlapPct)
			if prefix != "" {
				current := cloneChunkDoc(chunks[i])
				current.Text = prefix + current.Text
				current.TKNums = intPtr(tokenizeStr(current.Text))
				current.PDFPositions = mergeGeneralPositions(chunks[previousText].PDFPositions, current.PDFPositions)
				current.Positions = mergeGeneralPositions(chunks[previousText].Positions, current.Positions)
				mergeGeneralMetadata(&current, chunks[previousText])
				chunks[i] = current
			}
		}
		previousText = i
	}
	return chunks
}

func splitGeneralUnits(units []schema.ChunkDoc, pattern *regexp.Regexp) []schema.ChunkDoc {
	result := make([]schema.ChunkDoc, 0, len(units))
	for _, unit := range units {
		unit = cloneChunkDoc(unit)
		unit.Text = normalizeGeneralNewlines(itemTextOrFallback(unit))
		unit.DocType = itemDocType(unit)
		unit.CKType = unit.DocType
		if unit.DocType != "text" || pattern == nil || !pattern.MatchString(unit.Text) {
			unit.TKNums = intPtr(tokenizeStr(unit.Text))
			result = append(result, unit)
			continue
		}
		for _, part := range splitDroppingDelim(unit.Text, pattern) {
			if strings.TrimSpace(part) == "" {
				continue
			}
			piece := cloneChunkDoc(unit)
			piece.Text = part
			piece.TKNums = intPtr(tokenizeStr(part))
			result = append(result, piece)
		}
	}
	return result
}

func normalizeGeneralNewlines(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

func finalizeGeneralChunks(chunks []schema.ChunkDoc, childrenPattern *regexp.Regexp) []schema.ChunkDoc {
	visible := make([]schema.ChunkDoc, 0, len(chunks))
	for _, chunk := range chunks {
		chunk.Text = removeTag(chunk.Text)
		if strings.TrimSpace(chunk.Text) == "" && chunk.Image == "" {
			continue
		}
		visible = append(visible, chunk)
	}
	visible = splitByChildren(visible, childrenPattern)
	for i := range visible {
		visible[i].TKNums = intPtr(tokenizeStr(visible[i].Text))
	}
	return visible
}

func init() {
	MustRegisterChunker(ComponentNameGeneralChunker)
}
