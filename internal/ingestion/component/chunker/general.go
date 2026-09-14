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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"log/slog"
	"regexp"
	"sort"
	"strings"

	"gorm.io/gorm"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/chunk"
)

const ComponentNameGeneralChunker = "GeneralChunker"

var generalSentencePattern = regexp.MustCompile(`([。!?？；！\n]|\. )`)

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
	if value, ok := conf["delimiters"]; ok {
		if delimiters, recognized := normalizeGeneralDelimiterValue(value); recognized {
			p.Delimiters = delimiters
		}
	} else if value, ok := conf["delimiter"]; ok {
		if delimiters, recognized := normalizeGeneralDelimiterValue(value); recognized {
			p.Delimiters = delimiters
		}
	}
	if value, ok := conf["overlapped_percent"]; ok {
		p.OverlappedPercent = schema.NormalizeOverlappedPercent(value)
	}
	if value, ok := conf["children_delimiters"]; ok {
		if delimiters, recognized := normalizeGeneralDelimiterValue(value); recognized {
			p.ChildrenDelimiters = delimiters
		}
	}
	if value, ok := schema.NumericFromAny(conf["table_context_size"]); ok {
		p.TableContextSize = max(0, int(value))
	}
	if value, ok := schema.NumericFromAny(conf["image_context_size"]); ok {
		p.ImageContextSize = max(0, int(value))
	}
}

// normalizeGeneralDelimiterValue accepts both the canonical list form and
// the legacy Python single-string form. A legacy string is split into Unicode
// rune delimiters, except backtick-wrapped tokens which remain one delimiter.
func normalizeGeneralDelimiterValue(value any) ([]string, bool) {
	switch value := value.(type) {
	case string:
		return splitGeneralDelimiterString(value), true
	case []string:
		return append([]string(nil), value...), true
	case []any:
		return stringListFromAny(value), true
	default:
		return nil, false
	}
}

func splitGeneralDelimiterString(value string) []string {
	return chunk.ParseDelimiterField(value)
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

	strategy := generalStrategyForFileType(upstream.FileType)
	if strategy == generalStrategyText && !isKnownGeneralFileType(upstream.FileType) {
		slog.Debug("GeneralChunker: unknown file_type; using text fallback", "file_type", upstream.FileType)
	}

	switch strategy {
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

func isKnownGeneralFileType(fileType string) bool {
	switch strings.ToLower(strings.TrimSpace(fileType)) {
	case "pdf", "doc", "docx", "ppt", "pptx", "xls", "xlsx", "csv",
		"html", "md", "txt", "epub", "json", "video", "email", "visual",
		"aural", "folder", "other":
		return true
	default:
		return false
	}
}

func (c *GeneralChunkerComponent) chunkPDF(ctx context.Context, db *gorm.DB, upstream schema.ChunkerFromUpstream) (map[string]any, error) {
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
	units = sortPDFUnits(units)
	if hasPDFPositions(units) && hasUnpositionedPDFMedia(units) {
		slog.Warn("GeneralChunker: PDF media is missing position metadata; using degraded context/order fallback")
	}
	attachGeneralMediaContext(units, c.param.TableContextSize, c.param.ImageContextSize)

	media := make([]schema.ChunkDoc, 0)
	body := make([]schema.ChunkDoc, 0, len(units))
	for _, unit := range units {
		if itemDocType(unit) == "text" {
			body = append(body, unit)
		} else {
			media = append(media, unit)
		}
	}
	body = mergeGeneralUnits(body, c.param.ChunkTokenSize, c.param.OverlappedPercent, "\n")
	chunks := append(media, body...)

	engine, err := newPDFEngineFromUpstream(ctx, db, upstream)
	if err != nil {
		slog.Warn("GeneralChunker: could not open PDF for on-demand cropping", "err", err)
	}
	if engine != nil {
		defer engine.Close()
		chunks = cropImageChunks(ctx, engine, chunks)
	}
	chunks = finalizeGeneralChunks(chunks, childrenPattern)
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("GeneralChunker: %w", err)
	}
	if len(chunks) == 0 {
		return emptyOutputs(), nil
	}
	attachPDFOutline(chunks, upstream.File)
	return chunkOutputs(chunks), nil
}

// sortPDFUnits orders positioned units by physical reading order and leaves
// coordinate-free units after them in their original stable order. When no
// positions are present at all, parser order is preserved unchanged.
func sortPDFUnits(units []schema.ChunkDoc) []schema.ChunkDoc {
	type positionedUnit struct {
		unit       schema.ChunkDoc
		row        []float64
		positioned bool
	}
	ordered := make([]positionedUnit, 0, len(units))
	hasPosition := false
	for _, unit := range units {
		row, ok := firstPositionRow(lineRecord{pdfPositions: unit.PDFPositions, positions: unit.Positions})
		if ok {
			hasPosition = true
		}
		ordered = append(ordered, positionedUnit{unit: cloneChunkDoc(unit), row: row, positioned: ok})
	}
	if !hasPosition {
		result := make([]schema.ChunkDoc, 0, len(ordered))
		for _, item := range ordered {
			result = append(result, item.unit)
		}
		return result
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].positioned != ordered[j].positioned {
			return ordered[i].positioned
		}
		if !ordered[i].positioned {
			return false
		}
		return pdfPosRowLess(ordered[i].row, ordered[j].row)
	})
	result := make([]schema.ChunkDoc, 0, len(ordered))
	for _, item := range ordered {
		result = append(result, item.unit)
	}
	return result
}

func hasPDFPositions(units []schema.ChunkDoc) bool {
	for _, unit := range units {
		if _, ok := firstPositionRow(lineRecord{pdfPositions: unit.PDFPositions, positions: unit.Positions}); ok {
			return true
		}
	}
	return false
}

func hasUnpositionedPDFMedia(units []schema.ChunkDoc) bool {
	for _, unit := range units {
		if itemDocType(unit) == "text" {
			continue
		}
		if _, ok := firstPositionRow(lineRecord{pdfPositions: unit.PDFPositions, positions: unit.Positions}); !ok {
			return true
		}
	}
	return false
}

func attachPDFOutline(chunks []schema.ChunkDoc, file *schema.ChunkerFileMeta) {
	if len(chunks) == 0 || file == nil || len(file.Extra) == 0 {
		return
	}
	raw, ok := file.Extra["outline"]
	if !ok || len(raw) == 0 {
		return
	}
	var input []map[string]any
	if err := json.Unmarshal(raw, &input); err != nil || len(input) == 0 {
		return
	}
	outline := make([]map[string]any, 0, len(input))
	for _, entry := range input {
		title, _ := entry["title"].(string)
		if title == "" {
			continue
		}
		depth := entry["level"]
		if depth == nil {
			depth = entry["depth"]
		}
		outline = append(outline, map[string]any{"title": title, "depth": depth})
	}
	if len(outline) == 0 {
		return
	}
	encoded, err := json.Marshal(outline)
	if err != nil {
		return
	}
	if chunks[0].Extra == nil {
		chunks[0].Extra = make(map[string]json.RawMessage)
	}
	chunks[0].Extra["__outline__"] = encoded
}

func (c *GeneralChunkerComponent) chunkDOCX(ctx context.Context, upstream schema.ChunkerFromUpstream) (map[string]any, error) {
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
	attachGeneralMediaContext(units, c.param.TableContextSize, c.param.ImageContextSize)
	units = mergeDOCXUnits(units, c.param.ChunkTokenSize, hasCustomDelim(c.param.Delimiters), "")
	units = finalizeGeneralChunks(units, childrenPattern)
	if len(units) == 0 {
		return emptyOutputs(), nil
	}
	return chunkOutputs(units), nil
}

// mergeDOCXUnits mirrors Python naive_merge_docx: text units keep the
// previous text merge target across intervening table/image units. This means
// a later paragraph can extend an earlier text chunk while the media item
// remains in its original output position.
func mergeDOCXUnits(units []schema.ChunkDoc, target int, customDelimiter bool, joinSep string) []schema.ChunkDoc {
	merged := make([]schema.ChunkDoc, 0, len(units))
	previousText := -1
	for _, unit := range units {
		if itemDocType(unit) != "text" {
			media := cloneChunkDoc(unit)
			media.DocType = itemDocType(media)
			media.CKType = media.DocType
			merged = append(merged, media)
			continue
		}

		text := cloneChunkDoc(unit)
		text.DocType = "text"
		text.CKType = "text"
		text.TKNums = intPtr(generalUnitTokens(text))
		if previousText < 0 || customDelimiter || intValue(merged[previousText].TKNums) >= target {
			merged = append(merged, text)
			previousText = len(merged) - 1
			continue
		}
		mergeGeneralChunk(&merged[previousText], text, joinSep)
		merged[previousText].DocType = "text"
		merged[previousText].CKType = "text"
	}
	return merged
}

// attachGeneralMediaContext follows Python's sentence-aware media context
// extraction. The shared TokenChunker helper intentionally returns the
// smallest token-fitting rune suffix/prefix; DOCX/PDF general strategies use
// complete sentence units instead.
func attachGeneralMediaContext(units []schema.ChunkDoc, tableTokens, imageTokens int) {
	for i := range units {
		if units[i].CKType != "table" && units[i].CKType != "image" {
			continue
		}
		budget := imageTokens
		if units[i].CKType == "table" {
			budget = tableTokens
		}
		if budget <= 0 {
			continue
		}
		units[i].ContextAbove = collectGeneralMediaContext(units, i, budget, true)
		units[i].ContextBelow = collectGeneralMediaContext(units, i, budget, false)
	}
}

func collectGeneralMediaContext(units []schema.ChunkDoc, index, budget int, above bool) string {
	var parts []string
	remaining := budget
	step := -1
	if !above {
		step = 1
	}
	for cursor := index + step; cursor >= 0 && cursor < len(units) && remaining > 0; cursor += step {
		if units[cursor].CKType != "text" {
			continue
		}
		text := units[cursor].Text
		tokens := generalUnitTokens(units[cursor])
		if tokens >= remaining {
			piece := takeGeneralContextSentence(text, remaining, above)
			if above {
				parts = append([]string{piece}, parts...)
			} else {
				parts = append(parts, piece)
			}
			break
		}
		if above {
			parts = append([]string{text}, parts...)
		} else {
			parts = append(parts, text)
		}
		remaining -= tokens
	}
	return strings.Join(parts, "")
}

func takeGeneralContextSentence(text string, budget int, fromEnd bool) string {
	sentences := splitGeneralContextSentences(text)
	if len(sentences) == 0 {
		return text
	}
	// The sentence pieces are contiguous slices of text. Track byte offsets
	// instead of repeatedly prepending slices and joining the selected text;
	// each candidate is still tokenized independently because BPE token counts
	// are not additive across sentence boundaries.
	if fromEnd {
		start := len(text)
		for i := len(sentences) - 1; i >= 0; i-- {
			start -= len(sentences[i])
			if tokenizeStr(text[start:]) >= budget {
				break
			}
		}
		return text[start:]
	}

	end := 0
	for _, sentence := range sentences {
		end += len(sentence)
		if tokenizeStr(text[:end]) >= budget {
			break
		}
	}
	return text[:end]
}

func splitGeneralContextSentences(text string) []string {
	indices := generalSentencePattern.FindAllStringIndex(text, -1)
	if len(indices) == 0 {
		if text == "" {
			return nil
		}
		return []string{text}
	}
	parts := make([]string, 0, len(indices)+1)
	start := 0
	for _, index := range indices {
		parts = append(parts, text[start:index[1]])
		start = index[1]
	}
	if start < len(text) {
		parts = append(parts, text[start:])
	}
	return parts
}

func (c *GeneralChunkerComponent) chunkMarkdown(ctx context.Context, upstream schema.ChunkerFromUpstream) (map[string]any, error) {
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
	units = splitMarkdownUnits(units, primaryPattern)
	units = mergeMarkdownUnits(units, c.param.ChunkTokenSize, c.param.OverlappedPercent, "\n")
	units = finalizeGeneralChunks(units, childrenPattern)
	if len(units) == 0 {
		return emptyOutputs(), nil
	}
	return chunkOutputs(units), nil
}

// splitMarkdownUnits expands only text units while retaining the semantic
// parser type on every piece. Markdown heading detection depends on ck_type,
// so the generic splitter's type normalization cannot be used here.
func splitMarkdownUnits(units []schema.ChunkDoc, pattern *regexp.Regexp) []schema.ChunkDoc {
	result := make([]schema.ChunkDoc, 0, len(units))
	for _, unit := range units {
		unit = cloneChunkDoc(unit)
		unit.Text = strings.TrimSpace(normalizeGeneralNewlines(itemTextOrFallback(unit)))
		unit.DocType = itemDocType(unit)
		if unit.DocType != "text" || pattern == nil || !pattern.MatchString(unit.Text) {
			unit.TKNums = intPtr(tokenizeStr(unit.Text))
			result = append(result, unit)
			continue
		}
		for _, part := range splitDroppingDelim(unit.Text, pattern) {
			part = strings.TrimSpace(part)
			if part == "" {
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

// mergeMarkdownUnits mirrors the Markdown branch of Python naive.chunk:
// ordinary units use a projected token cap, while a short heading is always
// kept with the following unit. Markdown images are block attachments rather
// than standalone media chunks, so image-bearing units participate in the
// text merge and retain their image payload.
func mergeMarkdownUnits(units []schema.ChunkDoc, target int, overlapPct float64, joinSep string) []schema.ChunkDoc {
	overlapPct = max(0, min(100, overlapPct))
	merged := make([]schema.ChunkDoc, 0, len(units))
	current := -1
	currentTokens := 0
	for _, unit := range units {
		if itemDocType(unit) == "table" {
			if current >= 0 && isShortMarkdownHeading(merged[current]) {
				if !markdownImagesMergeable(merged[current].Image, unit.Image) {
					merged = append(merged, cloneChunkDoc(unit))
					current = -1
					currentTokens = 0
					continue
				}
				table := cloneChunkDoc(unit)
				heading := merged[current]
				if heading.Text != "" && table.Text != "" {
					table.Text = heading.Text + joinSep + table.Text
				} else {
					table.Text = heading.Text + table.Text
				}
				table.TKNums = intPtr(currentTokens + generalUnitTokens(unit))
				table.PDFPositions = mergeGeneralPositions(heading.PDFPositions, table.PDFPositions)
				table.Positions = mergeGeneralPositions(heading.Positions, table.Positions)
				mergeGeneralMetadata(&table, heading)
				table.Image = mergeMarkdownImages(heading.Image, table.Image)
				table.DocType = "table"
				table.CKType = "table"
				merged[current] = table
				current = -1
				currentTokens = 0
				continue
			}
			merged = append(merged, cloneChunkDoc(unit))
			current = -1
			currentTokens = 0
			continue
		}

		unit = cloneChunkDoc(unit)
		unit.DocType = "text"
		if unit.CKType == "" {
			unit.CKType = "text"
		}
		unit.TKNums = intPtr(generalUnitTokens(unit))
		if current < 0 {
			merged = append(merged, unit)
			current = len(merged) - 1
			currentTokens = generalUnitTokens(unit)
			continue
		}

		previous := &merged[current]
		unitTokens := generalUnitTokens(unit)
		if unit.CKType == "heading" && previous.CKType != "heading" {
			merged = append(merged, unit)
			current = len(merged) - 1
			currentTokens = unitTokens
			continue
		}
		forceMerge := isShortMarkdownHeading(*previous)
		if !markdownImagesMergeable(previous.Image, unit.Image) {
			merged = append(merged, unit)
			current = len(merged) - 1
			currentTokens = unitTokens
			continue
		}
		projected := currentTokens + unitTokens
		if !forceMerge && projected > target {
			overlap, _ := computeOverlapPrefix(previous.Text, overlapPct)
			if overlap != "" {
				// The emitted chunk belongs to the current unit. Clone it
				// first so media and other metadata from the previous chunk
				// cannot leak through the overlap prefix.
				next := cloneChunkDoc(unit)
				next.Text = joinGeneralOverlapText(overlap, unit.Text, joinSep)
				currentTokens = tokenizeStr(overlap) + unitTokens
				next.TKNums = intPtr(currentTokens)
				next.PDFPositions = mergeGeneralPositions(previous.PDFPositions, next.PDFPositions)
				next.Positions = mergeGeneralPositions(previous.Positions, next.Positions)
				merged = append(merged, next)
			} else {
				merged = append(merged, unit)
				currentTokens = unitTokens
			}
			current = len(merged) - 1
			continue
		}
		mergeMarkdownChunk(previous, unit, joinSep)
		previous.DocType = "text"
		previous.CKType = "text"
		currentTokens = projected
		previous.TKNums = intPtr(currentTokens)
	}
	return merged
}

func isShortMarkdownHeading(unit schema.ChunkDoc) bool {
	return unit.CKType == "heading" && tokenizeStr(strings.TrimSpace(unit.Text)) < 50
}

func mergeMarkdownChunk(dst *schema.ChunkDoc, src schema.ChunkDoc, joinSep string) {
	mergeGeneralChunk(dst, src, joinSep)
	dst.Image = mergeMarkdownImages(dst.Image, src.Image)
}

// mergeMarkdownImages preserves the single image field consumed by downstream
// components while matching Python's vertical image aggregation when both
// payloads are decodable raster data. Callers must check
// markdownImagesMergeable before merging two non-empty payloads; a single
// string cannot represent two opaque object-storage references safely.
func mergeMarkdownImages(first, second string) string {
	if first == "" {
		return second
	}
	if second == "" || first == second {
		return first
	}
	firstImage, firstOK := decodeMarkdownImage(first)
	secondImage, secondOK := decodeMarkdownImage(second)
	if !firstOK || !secondOK {
		return first
	}
	width := firstImage.Bounds().Dx()
	if secondImage.Bounds().Dx() > width {
		width = secondImage.Bounds().Dx()
	}
	canvas := image.NewRGBA(image.Rect(0, 0, width, firstImage.Bounds().Dy()+secondImage.Bounds().Dy()))
	draw.Draw(canvas, image.Rect(0, 0, firstImage.Bounds().Dx(), firstImage.Bounds().Dy()), firstImage, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(0, firstImage.Bounds().Dy(), secondImage.Bounds().Dx(), canvas.Bounds().Dy()), secondImage, image.Point{}, draw.Src)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		return first
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())
}

func markdownImagesMergeable(first, second string) bool {
	if first == "" || second == "" || first == second {
		return true
	}
	_, firstOK := decodeMarkdownImage(first)
	_, secondOK := decodeMarkdownImage(second)
	return firstOK && secondOK
}

func decodeMarkdownImage(value string) (image.Image, bool) {
	payload := value
	if marker := strings.Index(value, "base64,"); marker >= 0 {
		payload = value[marker+len("base64,"):]
	} else if strings.HasPrefix(value, "data:") {
		marker := strings.IndexByte(value, ',')
		if marker < 0 {
			return nil, false
		}
		payload = value[marker+1:]
	}
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, false
	}
	decoded, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, false
	}
	return decoded, true
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
		// Python naive_merge prefixes each delimiter atom with a newline before
		// counting it. The prefix is a budgeting detail, not emitted content;
		// without it BPE counts can shift General's soft-cap boundaries.
		for i := range units {
			if itemDocType(units[i]) == "text" {
				units[i].TKNums = intPtr(tokenizeStr("\n" + units[i].Text))
			}
		}
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

	return applyGeneralOverlap(merged, overlapPct, joinSep)
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
	if firstSheet, firstOK := spreadsheetPositionSheet(first); firstOK {
		secondSheet, secondOK := spreadsheetPositionSheet(second)
		return secondOK && firstSheet == secondSheet
	}
	if first.Sheet != "" || second.Sheet != "" {
		return first.Sheet != "" && second.Sheet != "" && first.Sheet == second.Sheet
	}
	return false
}

func spreadsheetPositionSheet(doc schema.ChunkDoc) (float64, bool) {
	row, ok := firstPositionRow(lineRecord{positions: doc.Positions})
	if !ok {
		return 0, false
	}
	return row[0], true
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

func applyGeneralOverlap(chunks []schema.ChunkDoc, overlapPct float64, joinSep string) []schema.ChunkDoc {
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
				current.Text = joinGeneralOverlapText(prefix, current.Text, joinSep)
				current.TKNums = intPtr(tokenizeStr(current.Text))
				current.PDFPositions = mergeGeneralPositions(chunks[previousText].PDFPositions, current.PDFPositions)
				current.Positions = mergeGeneralPositions(chunks[previousText].Positions, current.Positions)
				chunks[i] = current
			}
		}
		previousText = i
	}
	return chunks
}

func joinGeneralOverlapText(prefix, text, separator string) string {
	if prefix == "" {
		return text
	}
	if text == "" || separator == "" {
		return prefix + text
	}
	if strings.HasSuffix(prefix, separator) || strings.HasPrefix(text, separator) {
		return prefix + text
	}
	return prefix + separator + text
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
	visible = splitGeneralChildren(visible, childrenPattern)
	for i := range visible {
		visible[i].TKNums = intPtr(tokenizeStr(visible[i].Text))
	}
	return visible
}

// splitGeneralChildren keeps the matched child delimiter attached to the
// preceding child. This matches the legacy General/naive path; the shared
// TokenChunker splitByChildren helper intentionally drops delimiters and is
// therefore not reused here.
func splitGeneralChildren(chunks []schema.ChunkDoc, pattern *regexp.Regexp) []schema.ChunkDoc {
	if pattern == nil {
		return chunks
	}
	result := make([]schema.ChunkDoc, 0, len(chunks))
	for _, chunk := range chunks {
		if itemDocType(chunk) != "text" {
			result = append(result, chunk)
			continue
		}
		mom := strings.TrimPrefix(chunk.Text, "\n")
		for _, part := range splitKeepingGeneralDelimiter(chunk.Text, pattern) {
			if strings.TrimSpace(part) == "" {
				continue
			}
			piece := cloneChunkDoc(chunk)
			piece.Text = part
			piece.Mom = mom
			result = append(result, piece)
		}
	}
	return result
}

func splitKeepingGeneralDelimiter(text string, pattern *regexp.Regexp) []string {
	matches := pattern.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return []string{text}
	}
	result := make([]string, 0, len(matches)+1)
	cursor := 0
	for _, match := range matches {
		if match[0] > cursor {
			result = append(result, text[cursor:match[1]])
		}
		cursor = match[1]
	}
	if cursor < len(text) {
		result = append(result, text[cursor:])
	}
	return result
}

func init() {
	MustRegisterChunker(ComponentNameGeneralChunker)
}
