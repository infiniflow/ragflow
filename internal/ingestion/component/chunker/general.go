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
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/chunk"
	"ragflow/internal/parser/tableutil"
)

const ComponentNameGeneralChunker = "GeneralChunker"

var contextSentencePattern = regexp.MustCompile(`([。!?？；！\n]|\. )`)

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
	if err := param.Validate(); err != nil {
		return nil, err
	}
	return &GeneralChunkerComponent{param: param}, nil
}

func (p generalChunkerParam) Validate() error {
	if p.ChunkTokenSize < 0 {
		return fmt.Errorf("chunk_token_size must be non-negative")
	}
	return nil
}

func (c *GeneralChunkerComponent) Inputs() map[string]string { return ChunkerInputs }

func (c *GeneralChunkerComponent) Outputs() map[string]string { return ChunkerOutputs }

func (c *GeneralChunkerComponent) Invoke(ctx context.Context, db *gorm.DB, inputs map[string]any) (map[string]any, error) {
	upstream, err := decodeChunkerFromUpstream(inputs)
	if err != nil {
		return nil, fmt.Errorf("GeneralChunker: decode input: %w", err)
	}
	fileType := strings.ToLower(strings.TrimSpace(upstream.FileType))
	if fileType == "" {
		fileType = inferGeneralFileType(upstream)
		if fileType == "" {
			return nil, fmt.Errorf("GeneralChunker: file_type is required when source name has no extension")
		}
		common.Warn("GeneralChunker: missing parser file_type; inferred from source name",
			zap.String("name", upstream.Name), zap.String("file_type", fileType))
	}

	strategy := generalStrategyForFileType(fileType)
	if strategy == generalStrategyText && !isKnownGeneralFileType(fileType) {
		common.Debug("GeneralChunker: unknown file_type; using text fallback", zap.String("file_type", fileType))
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
	switch strings.ToLower(strings.TrimSpace(fileType)) {
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

func inferGeneralFileType(upstream schema.ChunkerFromUpstream) string {
	name := strings.TrimSpace(upstream.Name)
	if name == "" && upstream.File != nil {
		name = strings.TrimSpace(upstream.File.Name)
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	return ext
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
	childrenPattern := compileDelimPattern(c.param.ChildrenDelimiters)
	customDelimiter := hasCustomDelim(c.param.Delimiters)
	if customDelimiter {
		units = splitGeneralUnits(units, primaryPattern, false)
	}
	units = sortPDFUnits(units)
	if hasPDFPositions(units) && hasUnpositionedPDFMedia(units) {
		common.Warn("GeneralChunker: PDF media is missing position metadata; using degraded context/order fallback")
	}
	attachGeneralMediaContext(units, c.param.TableContextSize, c.param.ImageContextSize, primaryPattern)

	chunks := make([]schema.ChunkDoc, 0, len(units))
	body := make([]schema.ChunkDoc, 0, len(units))
	flushBody := func() {
		if len(body) == 0 {
			return
		}
		joinSep := "\n"
		if !customDelimiter {
			body = separateGeneralTextBlocks(body)
			body = splitGeneralUnits(body, primaryPattern, true)
			joinSep = ""
		}
		chunks = append(chunks, mergeGeneralUnits(body, c.param.ChunkTokenSize, c.param.OverlappedPercent, joinSep)...)
		body = body[:0]
	}
	for _, unit := range units {
		if itemDocType(unit) == "text" {
			body = append(body, unit)
		} else {
			flushBody()
			media := cloneChunkDoc(unit)
			media.Text = normalizeGeneralNewlines(media.Text)
			media.DocType = itemDocType(media)
			media.CKType = media.DocType
			chunks = append(chunks, media)
		}
	}
	flushBody()

	engine, err := newPDFEngineFromUpstream(ctx, db, upstream)
	if err != nil {
		common.Warn("GeneralChunker: could not open PDF for on-demand cropping", zap.Error(err))
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

// sortPDFUnits orders each contiguous positioned run by physical reading
// order. Coordinate-free units are barriers: they stay at their parser
// positions and preserve their relative order instead of being moved to the
// end of the document.
func sortPDFUnits(units []schema.ChunkDoc) []schema.ChunkDoc {
	type positionedUnit struct {
		unit       schema.ChunkDoc
		row        []float64
		positioned bool
	}
	ordered := make([]positionedUnit, 0, len(units))
	for _, unit := range units {
		row, ok := firstPositionRow(lineRecord{pdfPositions: unit.PDFPositions, positions: unit.Positions})
		ordered = append(ordered, positionedUnit{unit: cloneChunkDoc(unit), row: row, positioned: ok})
	}
	runStart := -1
	flush := func(end int) {
		if runStart < 0 {
			return
		}
		sort.SliceStable(ordered[runStart:end], func(i, j int) bool {
			return pdfPosRowLess(ordered[runStart+i].row, ordered[runStart+j].row)
		})
		runStart = -1
	}
	for i := 0; i <= len(ordered); i++ {
		positioned := i < len(ordered) && ordered[i].positioned
		if positioned {
			if runStart < 0 {
				runStart = i
			}
			continue
		}
		flush(i)
	}
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
	childrenPattern := compileDelimPattern(c.param.ChildrenDelimiters)
	customDelimiter := hasCustomDelim(c.param.Delimiters)
	recordPrefixes := chunk.CustomDelimiterPrefixes(c.param.Delimiters)
	recordGrouped := false
	if len(recordPrefixes) > 0 {
		units, recordGrouped = groupDOCXUnitsByRecordBoundary(units, recordPrefixes)
	}
	splitPattern := primaryPattern
	if recordGrouped {
		// Grouped records join paragraphs with "\n"; bare "\n" in the default
		// delimiter list must not re-split them (#20496).
		splitPattern = chunk.CompileDelimiterListPattern(c.param.Delimiters)
	}
	if !customDelimiter && !recordGrouped {
		attachGeneralMediaContext(units, c.param.TableContextSize, c.param.ImageContextSize, primaryPattern)
		units = separateGeneralTextBlocks(units)
	}
	if recordGrouped {
		units = splitGeneralUnitsPreservingRecordPrefix(units, splitPattern, recordPrefixes)
	} else {
		units = splitGeneralUnits(units, splitPattern, !customDelimiter)
	}
	if customDelimiter || recordGrouped {
		attachGeneralMediaContext(units, c.param.TableContextSize, c.param.ImageContextSize, primaryPattern)
	} else {
		units = mergeDOCXUnits(units, c.param.ChunkTokenSize)
	}
	overlapSep := ""
	if recordGrouped {
		overlapSep = "\n"
	}
	units = applyGeneralOverlap(units, c.param.OverlappedPercent, overlapSep)
	units = finalizeGeneralChunks(units, childrenPattern)
	if len(units) == 0 {
		return emptyOutputs(), nil
	}
	return chunkOutputs(units), nil
}

// separateGeneralTextBlocks retains parser block boundaries in the text before
// delimiter splitting. Sentence fragments can then be concatenated directly.
func separateGeneralTextBlocks(units []schema.ChunkDoc) []schema.ChunkDoc {
	units = append([]schema.ChunkDoc(nil), units...)
	previousText := -1
	for i := range units {
		if itemDocType(units[i]) != "text" {
			continue
		}
		if strings.TrimSpace(units[i].Text) == "" {
			continue
		}
		if previousText >= 0 {
			previous := units[previousText].Text
			current := units[i].Text
			// Splitting normalizes both CR and LF boundaries to LF.
			if !strings.HasSuffix(previous, "\n") && !strings.HasSuffix(previous, "\r") &&
				!strings.HasPrefix(current, "\n") && !strings.HasPrefix(current, "\r") {
				units[previousText].Text += "\n"
			}
		}
		previousText = i
	}
	return units
}

// groupDOCXUnitsByRecordBoundary merges consecutive text paragraphs into one
// unit until a paragraph begins with a custom (backtick) delimiter prefix.
// DOCX upstream emits one paragraph per unit, so a custom delimiter that only
// appears at the start of each logical record must group paragraphs rather
// than split within a single paragraph (#20496).
func groupDOCXUnitsByRecordBoundary(units []schema.ChunkDoc, recordPrefixes []string) ([]schema.ChunkDoc, bool) {
	if len(recordPrefixes) == 0 {
		return units, false
	}
	grouped := make([]schema.ChunkDoc, 0, len(units))
	var current *schema.ChunkDoc
	sawBoundary := false
	flush := func() {
		if current == nil {
			return
		}
		grouped = append(grouped, *current)
		current = nil
	}
	for _, unit := range units {
		if itemDocType(unit) != "text" {
			flush()
			media := cloneChunkDoc(unit)
			media.DocType = itemDocType(media)
			media.CKType = media.DocType
			grouped = append(grouped, media)
			continue
		}
		if strings.TrimSpace(unit.Text) == "" {
			continue
		}
		text := cloneChunkDoc(unit)
		text.DocType = "text"
		text.CKType = "text"
		if docxTextStartsRecordBoundary(text.Text, recordPrefixes) {
			sawBoundary = true
			flush()
			current = &text
			continue
		}
		if !sawBoundary {
			flush()
			grouped = append(grouped, text)
			continue
		}
		if current == nil {
			current = &text
			continue
		}
		mergeGeneralChunk(current, text, "\n")
	}
	flush()
	if !sawBoundary {
		return units, false
	}
	return grouped, true
}

func docxTextStartsRecordBoundary(text string, recordPrefixes []string) bool {
	trimmed := strings.TrimLeftFunc(text, unicode.IsSpace)
	for _, prefix := range recordPrefixes {
		if prefix == "" {
			continue
		}
		if strings.HasPrefix(text, prefix) || strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

// mergeDOCXUnits keeps the previous text merge target across intervening
// table/image units. Paragraph separators and sentence delimiters are already
// carried by the text units.
func mergeDOCXUnits(units []schema.ChunkDoc, target int) []schema.ChunkDoc {
	merged := make([]schema.ChunkDoc, 0, len(units))
	previousText := -1
	for _, unit := range units {
		if itemDocType(unit) != "text" {
			merged = append(merged, cloneChunkDoc(unit))
			continue
		}

		if previousText < 0 || intValue(merged[previousText].TKNums) >= target {
			merged = append(merged, cloneChunkDoc(unit))
			previousText = len(merged) - 1
			continue
		}
		mergeGeneralChunk(&merged[previousText], unit, "")
	}
	return merged
}

// attachGeneralMediaContext selects complete configured delimiter units or
// sentences from the neighboring source blocks.
func attachGeneralMediaContext(units []schema.ChunkDoc, tableTokens, imageTokens int, primaryPattern *regexp.Regexp) {
	if tableTokens <= 0 && imageTokens <= 0 {
		return
	}
	pattern := contextSentencePattern
	if primaryPattern != nil {
		pattern = regexp.MustCompile("(?:" + primaryPattern.String() + ")|(?:" + pattern.String() + ")")
	}
	for i := range units {
		docType := itemDocType(units[i])
		if docType != "table" && docType != "image" {
			continue
		}
		budget := imageTokens
		if docType == "table" {
			budget = tableTokens
		}
		if budget <= 0 {
			continue
		}
		units[i].ContextAbove = collectGeneralMediaContext(units, i, budget, true, pattern)
		units[i].ContextBelow = collectGeneralMediaContext(units, i, budget, false, pattern)
	}
}

func collectGeneralMediaContext(units []schema.ChunkDoc, index, budget int, above bool, pattern *regexp.Regexp) string {
	var parts []string
	remaining := budget
	step := -1
	if !above {
		step = 1
	}
	for cursor := index + step; cursor >= 0 && cursor < len(units) && remaining > 0; cursor += step {
		if generalMediaContextCrossesSpreadsheetBoundary(units[index], units[cursor]) {
			break
		}
		text, ok := generalContextSourceText(units[cursor])
		if !ok {
			continue
		}
		text = normalizeGeneralNewlines(text)
		tokens := tokenizeStr(text)
		if tokens >= remaining {
			piece := takeContextSentences(text, remaining, above, pattern)
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
	return strings.Join(parts, "\n")
}

// generalContextSourceText returns the readable text a unit contributes as
// media context. Text units contribute their body; spreadsheet table
// segments contribute their data rows' cell text — the row IR's loose text
// rows were the image context before the HTML wire unification, and the
// repeating header stays out of the context exactly as it did then. Only
// spreadsheet segments qualify: attachGeneralMediaContext is shared with the
// PDF and DOCX paths, where a table item carries no identity and must keep
// its previous behavior (no context contribution).
func generalContextSourceText(doc schema.ChunkDoc) (string, bool) {
	switch itemDocType(doc) {
	case "text":
		return doc.Text, true
	case "table":
		if !hasSpreadsheetIdentity(doc) {
			return "", false
		}
		td := tableDataOf(doc)
		if td == nil {
			return "", false
		}
		rows, headerCount := td.CellTexts(), td.HeaderRows
		lines := make([]string, 0, len(rows)-headerCount)
		for _, row := range rows[headerCount:] {
			cells := make([]string, 0, len(row))
			for _, cell := range row {
				if cell != "" {
					cells = append(cells, cell)
				}
			}
			if len(cells) > 0 {
				lines = append(lines, strings.Join(cells, " "))
			}
		}
		return strings.Join(lines, "\n"), true
	default:
		return "", false
	}
}

func generalMediaContextCrossesSpreadsheetBoundary(media, candidate schema.ChunkDoc) bool {
	if !hasSpreadsheetIdentity(media) && !hasSpreadsheetIdentity(candidate) {
		return false
	}
	return !sameSpreadsheetTable(media, candidate)
}

func hasSpreadsheetIdentity(doc schema.ChunkDoc) bool {
	return doc.Sheet != "" || doc.SheetIndex != nil
}

// takeContextSentences returns the whole sentences at one end of text whose
// combined token count reaches the budget, taking the last sentence even when
// it overshoots: a sentence is the smallest unit a context window may cut at,
// so a text without a sentence boundary comes back whole. Shared by the media
// context collectors of both chunkers use the supplied boundary pattern.
func takeContextSentences(text string, budget int, fromEnd bool, pattern *regexp.Regexp) string {
	sentences := splitByDelim(text, pattern, true)
	// The sentence pieces cover contiguous source segments. Track byte offsets
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
	childrenPattern := compileDelimPattern(c.param.ChildrenDelimiters)
	customDelimiter := hasCustomDelim(c.param.Delimiters)
	if !customDelimiter {
		attachGeneralMediaContext(units, c.param.TableContextSize, c.param.ImageContextSize, primaryPattern)
	}
	parts := splitMarkdownUnits(units, primaryPattern, !customDelimiter)
	if customDelimiter {
		attachGeneralMediaContext(parts.chunks, c.param.TableContextSize, c.param.ImageContextSize, primaryPattern)
	}
	units = mergeMarkdownUnits(parts, c.param.ChunkTokenSize, c.param.OverlappedPercent)
	units = finalizeGeneralChunks(units, childrenPattern)
	if len(units) == 0 {
		return emptyOutputs(), nil
	}
	return chunkOutputs(units), nil
}

// markdownSplit keeps each fragment's parser block boundary beside its chunk.
type markdownSplit struct {
	chunks        []schema.ChunkDoc
	blockStarts   []bool
	shortHeadings []bool // Classified from the complete source heading.
}

// splitMarkdownUnits expands only text units while retaining the semantic
// parser type on every piece. Markdown heading detection depends on ck_type,
// so the generic splitter's type normalization cannot be used here.
func splitMarkdownUnits(units []schema.ChunkDoc, pattern *regexp.Regexp, keepDelimiter bool) markdownSplit {
	result := markdownSplit{
		chunks:        make([]schema.ChunkDoc, 0, len(units)),
		blockStarts:   make([]bool, 0, len(units)),
		shortHeadings: make([]bool, 0, len(units)),
	}
	for _, unit := range units {
		unit = cloneChunkDoc(unit)
		unit.Text = normalizeGeneralNewlines(itemTextOrFallback(unit))
		if !keepDelimiter || unit.CKType == "heading" {
			unit.Text = strings.TrimSpace(unit.Text)
		}
		unit.DocType = itemDocType(unit)
		if unit.DocType == "table" || unit.DocType == "image" {
			unit.CKType = unit.DocType
		}
		shortHeading := isShortMarkdownHeading(unit)
		if unit.DocType != "text" || pattern == nil || !pattern.MatchString(unit.Text) {
			unit.TKNums = intPtr(tokenizeStr(unit.Text))
			result.chunks = append(result.chunks, unit)
			result.blockStarts = append(result.blockStarts, true)
			result.shortHeadings = append(result.shortHeadings, shortHeading)
			continue
		}
		parts := splitByDelim(unit.Text, pattern, keepDelimiter)
		if keepDelimiter {
			parts = foldWhitespaceSegments(parts)
		}
		startsBlock := true
		for _, part := range parts {
			if !keepDelimiter {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
			}
			piece := cloneChunkDoc(unit)
			piece.Text = part
			piece.TKNums = intPtr(tokenizeStr(part))
			result.chunks = append(result.chunks, piece)
			result.blockStarts = append(result.blockStarts, startsBlock)
			result.shortHeadings = append(result.shortHeadings, shortHeading)
			startsBlock = !keepDelimiter
		}
	}
	return result
}

// stripTextLinesFromMediaContextAbove removes folded unit text from collected
// media context, including a suffix selected by a small context budget.
func stripTextLinesFromMediaContextAbove(contextAbove, folded string) string {
	folded = strings.TrimSpace(folded)
	if folded == "" || contextAbove == "" {
		return contextAbove
	}
	if strings.HasSuffix(folded, strings.TrimSpace(contextAbove)) {
		return ""
	}
	parts := strings.Split(contextAbove, "\n")
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == folded {
			continue
		}
		filtered = append(filtered, part)
	}
	return strings.Join(filtered, "\n")
}

// markdownImageUnitHasMediaContext reports whether attachGeneralMediaContext
// populated context on a markdown image unit. Such units stay out of the text
// merge so ContextAbove and ContextBelow survive until materialization.
func markdownImageUnitHasMediaContext(unit schema.ChunkDoc) bool {
	return unit.ContextAbove != "" || unit.ContextBelow != ""
}

// mergeMarkdownUnits uses a projected token cap, while a short heading is always
// kept with the following unit. Markdown images are block attachments rather
// than standalone media chunks, so image-bearing units participate in the
// text merge and retain their image payload. Images with configured media
// context are emitted as standalone chunks instead.
func mergeMarkdownUnits(parts markdownSplit, target int, overlapPct float64) []schema.ChunkDoc {
	overlapPct = max(0, min(100, overlapPct))
	merged := make([]schema.ChunkDoc, 0, len(parts.chunks))
	current := -1
	currentTokens := 0
	for index, unit := range parts.chunks {
		startsBlock := parts.blockStarts[index]
		previousShortHeading := index > 0 && parts.shortHeadings[index-1]
		joinSep := ""
		if startsBlock && current >= 0 && !strings.HasSuffix(merged[current].Text, "\n") && !strings.HasPrefix(unit.Text, "\n") {
			joinSep = "\n"
		}
		if itemDocType(unit) == "table" {
			if current >= 0 && merged[current].CKType == "heading" && previousShortHeading {
				mergedImage, imagesOK := mergeMarkdownImagesChecked(merged[current].Image, unit.Image)
				if !imagesOK {
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
				table.Image = mergedImage
				table.DocType = "table"
				table.CKType = "table"
				table.ContextAbove = stripTextLinesFromMediaContextAbove(table.ContextAbove, heading.Text)
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

		if itemDocType(unit) == "image" && markdownImageUnitHasMediaContext(unit) {
			if current >= 0 {
				current = -1
				currentTokens = 0
			}
			standalone := cloneChunkDoc(unit)
			standalone.DocType = "image"
			if standalone.CKType == "" {
				standalone.CKType = "image"
			}
			standalone.TKNums = intPtr(generalUnitTokens(standalone))
			merged = append(merged, standalone)
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
		if startsBlock && unit.CKType == "heading" && previous.CKType != "heading" {
			merged = append(merged, unit)
			current = len(merged) - 1
			currentTokens = unitTokens
			continue
		}
		forceMerge := previous.CKType == "heading" && previousShortHeading
		mergedImage, imagesOK := mergeMarkdownImagesChecked(previous.Image, unit.Image)
		if !imagesOK {
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
		mergeGeneralChunk(previous, unit, joinSep)
		previous.Image = mergedImage
		previous.DocType = "text"
		if startsBlock || unit.CKType != "heading" {
			previous.CKType = "text"
		}
		currentTokens = projected
		previous.TKNums = intPtr(currentTokens)
	}
	return merged
}

func isShortMarkdownHeading(unit schema.ChunkDoc) bool {
	if unit.CKType != "heading" {
		return false
	}
	return tokenizeStr(markdownHeadingContent(unit.Text)) < 50
}

func markdownHeadingContent(text string) string {
	text = strings.TrimSpace(text)
	if len(text) == 0 || text[0] != '#' {
		return text
	}
	level := 0
	for level < len(text) && text[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || level == len(text) || text[level] != ' ' {
		return text
	}
	return strings.TrimSpace(text[level:])
}

const (
	maxMarkdownImageDimension = 8192
	maxMarkdownImagePixels    = 32 * 1024 * 1024
)

func mergeMarkdownImagesChecked(first, second string) (string, bool) {
	if first == "" {
		return second, true
	}
	if second == "" || first == second {
		return first, true
	}
	firstImage, firstOK := decodeMarkdownImage(first)
	secondImage, secondOK := decodeMarkdownImage(second)
	if !firstOK || !secondOK {
		return first, false
	}
	width := firstImage.Bounds().Dx()
	if secondImage.Bounds().Dx() > width {
		width = secondImage.Bounds().Dx()
	}
	height := firstImage.Bounds().Dy() + secondImage.Bounds().Dy()
	if !markdownImageWithinLimits(width, height) {
		return first, false
	}
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, image.Rect(0, 0, firstImage.Bounds().Dx(), firstImage.Bounds().Dy()), firstImage, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(0, firstImage.Bounds().Dy(), secondImage.Bounds().Dx(), canvas.Bounds().Dy()), secondImage, image.Point{}, draw.Src)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		return first, false
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes()), true
}

func markdownImageWithinLimits(width, height int) bool {
	if width <= 0 || height <= 0 || width > maxMarkdownImageDimension || height > maxMarkdownImageDimension {
		return false
	}
	return width <= maxMarkdownImagePixels/height
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
	config, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || !markdownImageWithinLimits(config.Width, config.Height) {
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
	attachGeneralMediaContext(units, c.param.TableContextSize, c.param.ImageContextSize, nil)
	chunks := make([]schema.ChunkDoc, 0, len(units))
	for _, unit := range units {
		if itemDocType(unit) == "table" && tableDataOf(unit) != nil {
			chunks = append(chunks, c.splitSpreadsheetTable(unit, upstream.DocID)...)
			continue
		}
		chunks = append(chunks, cloneChunkDoc(unit))
	}
	childrenPattern := compileDelimPattern(c.param.ChildrenDelimiters)
	chunks = finalizeGeneralChunks(chunks, childrenPattern)
	if len(chunks) == 0 {
		return emptyOutputs(), nil
	}
	return chunkOutputs(chunks), nil
}

// splitSpreadsheetTable enforces the ChunkTokenSize budget on a table
// segment: sub-tables replicate the caption and header rows, and the
// row-aligned positions matrix is sliced per sub-table (R11) so each chunk
// carries exactly its own rows' tuples. A segment that fits is emitted
// whole; a split whose matrix cannot be aligned to the markup rows drops
// positions rather than pointing every sub-table row at the full matrix (R1).
//
// The budget covers data rows only — the replicated header is fixed
// boilerplate and does not consume the per-part allowance (see
// splitLargeTable). docID attributes the oversized-header warning.
//
// The legacy path merged adjacent rows by token budget into multi-row chunks
// and deferred the header; a segment now passes through whole unless the
// budget forces a cut, and the header repeats in every part.
func (c *GeneralChunkerComponent) splitSpreadsheetTable(unit schema.ChunkDoc, docID string) []schema.ChunkDoc {
	td := tableDataOf(unit)
	if td == nil {
		return []schema.ChunkDoc{cloneChunkDoc(unit)}
	}
	parts, ranges, headerRows := splitLargeTable(td, c.param.ChunkTokenSize, tokenizeStr, docID)
	if ranges == nil {
		// The whole table fits one chunk: render it as HTML on Text so the
		// persisted content and the id (hash of Text) stay byte-consistent and
		// the UI can display a real table. TableData is dropped, never indexed.
		out := cloneChunkDoc(unit)
		out.Text = tableutil.RenderTableHTML(td)
		out.TableData = nil
		return []schema.ChunkDoc{out}
	}
	var matrix [][]float64
	// Spreadsheet items only: PDF items write layout boxes into the same
	// field, and those must never be sliced into sub-tables as row tuples.
	if unit.SheetIndex != nil && len(unit.Positions) > 0 {
		if err := json.Unmarshal(unit.Positions, &matrix); err != nil {
			matrix = nil
		}
	}
	// The matrix carries one five-field tuple per row; the splitter cut those
	// same rows, so its last range end is the total data-row count.
	aligned := false
	if len(ranges) > 0 {
		if expected := headerRows + ranges[len(ranges)-1][1]; expected > 0 && len(matrix) == expected {
			aligned = true
			for _, tuple := range matrix {
				if len(tuple) != 5 {
					aligned = false
					break
				}
			}
		}
	}
	out := make([]schema.ChunkDoc, 0, len(parts))
	for i, part := range parts {
		piece := cloneChunkDoc(unit)
		// Text carries the sub-table rendered as HTML so the persisted content
		// and the chunk id (hash of Text) stay byte-consistent and the UI can
		// display a real table. The structured TableData is dropped here and is
		// never serialized to the index.
		piece.Text = tableutil.RenderTableHTML(&part)
		piece.TableData = nil
		piece.Positions = nil
		lo := headerRows + ranges[i][0]
		hi := headerRows + ranges[i][1]
		if aligned && hi <= len(matrix) {
			sub := make([][]float64, 0, headerRows+hi-lo)
			sub = append(sub, matrix[:headerRows]...)
			sub = append(sub, matrix[lo:hi]...)
			if encoded, err := json.Marshal(sub); err == nil {
				piece.Positions = encoded
			}
		}
		out = append(out, piece)
	}
	return out
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
	childrenPattern := compileDelimPattern(c.param.ChildrenDelimiters)
	customDelimiter := hasCustomDelim(c.param.Delimiters)
	units = splitGeneralUnits(units, primaryPattern, !customDelimiter)
	attachGeneralMediaContext(units, c.param.TableContextSize, c.param.ImageContextSize, primaryPattern)
	if !customDelimiter {
		units = mergeGeneralUnits(units, c.param.ChunkTokenSize, c.param.OverlappedPercent, "")
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

func sameSpreadsheetTable(first, second schema.ChunkDoc) bool {
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

// splitGeneralUnitsPreservingRecordPrefix is splitGeneralUnits, except a
// leading custom-delimiter record prefix (for example "问：") is kept on the
// first fragment. Inline delimiter matches are still dropped in splitByDelim
// drop mode.
func splitGeneralUnitsPreservingRecordPrefix(units []schema.ChunkDoc, pattern *regexp.Regexp, recordPrefixes []string) []schema.ChunkDoc {
	result := make([]schema.ChunkDoc, 0, len(units))
	for _, unit := range units {
		unit = cloneChunkDoc(unit)
		unit.Text = normalizeGeneralNewlines(itemTextOrFallback(unit))
		unit.DocType = itemDocType(unit)
		unit.CKType = unit.DocType
		prefix, rest := peelLeadingRecordPrefix(unit.Text, recordPrefixes)
		if unit.DocType != "text" || pattern == nil || !pattern.MatchString(rest) {
			unit.Text = prefix + rest
			unit.TKNums = intPtr(tokenizeStr(unit.Text))
			result = append(result, unit)
			continue
		}
		parts := splitByDelim(rest, pattern, false)
		filtered := make([]string, 0, len(parts))
		for _, part := range parts {
			if strings.TrimSpace(part) == "" {
				continue
			}
			filtered = append(filtered, part)
		}
		if len(filtered) > 0 {
			filtered[0] = prefix + filtered[0]
		}
		for _, piece := range sliceGeneralSplitPieces(unit, filtered) {
			result = append(result, piece)
		}
	}
	return result
}

func peelLeadingRecordPrefix(text string, prefixes []string) (prefix, rest string) {
	if len(prefixes) == 0 {
		return "", text
	}
	trimmed := strings.TrimLeftFunc(text, unicode.IsSpace)
	leadingWS := text[:len(text)-len(trimmed)]
	for _, p := range prefixes {
		if p == "" {
			continue
		}
		if strings.HasPrefix(text, p) {
			return p, text[len(p):]
		}
		if strings.HasPrefix(trimmed, p) {
			return leadingWS + p, trimmed[len(p):]
		}
	}
	return "", text
}

func splitGeneralUnits(units []schema.ChunkDoc, pattern *regexp.Regexp, keepDelimiter bool) []schema.ChunkDoc {
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
		parts := splitByDelim(unit.Text, pattern, keepDelimiter)
		if keepDelimiter {
			parts = foldWhitespaceSegments(parts)
		}
		filtered := make([]string, 0, len(parts))
		for _, part := range parts {
			if !keepDelimiter && strings.TrimSpace(part) == "" {
				continue
			}
			filtered = append(filtered, part)
		}
		parts = filtered
		for _, piece := range sliceGeneralSplitPieces(unit, parts) {
			result = append(result, piece)
		}
	}
	return result
}

func normalizeGeneralNewlines(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

// sliceGeneralSplitPieces builds one output piece per text part, slicing the
// source paragraph bbox via slicePositionsByCounts so adjacent pieces abut
// instead of each carrying the full paragraph box. Parser position tags
// (@@...##) carry no visual height and are excluded from the ratio, mirroring
// the Token path (chunkFromItem/splitOversizedText).
func sliceGeneralSplitPieces(unit schema.ChunkDoc, parts []string) []schema.ChunkDoc {
	out := make([]schema.ChunkDoc, 0, len(parts))
	if len(parts) == 0 {
		return out
	}
	counts := make([]int, len(parts))
	for i, part := range parts {
		counts[i] = utf8.RuneCountInString(removeTag(part))
	}
	pdfSliced, posSliced := slicePositionsByCounts(unit.PDFPositions, unit.Positions, counts)
	for i, part := range parts {
		piece := cloneChunkDoc(unit)
		piece.Text = part
		piece.TKNums = intPtr(tokenizeStr(part))
		piece.PDFPositions = pdfSliced[i]
		piece.Positions = posSliced[i]
		out = append(out, piece)
	}
	return out
}

func finalizeGeneralChunks(chunks []schema.ChunkDoc, childrenPattern *regexp.Regexp) []schema.ChunkDoc {
	visible := make([]schema.ChunkDoc, 0, len(chunks))
	for _, chunk := range chunks {
		chunk.Text = removeTag(chunk.Text)
		// Keep a chunk whenever it still carries retrievable content. A
		// streamed crop upload sets ImgID and clears Image, so an uploaded
		// media chunk (text may be only a position tag, or empty) must be
		// retained via its ImgID — otherwise the already-uploaded image is
		// dropped and its MinIO object orphaned.
		if strings.TrimSpace(chunk.Text) == "" && chunk.Image == "" && chunk.ImgID == "" {
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
		raw := splitKeepingGeneralDelimiter(chunk.Text, pattern)
		parts := make([]string, 0, len(raw))
		for _, part := range raw {
			if strings.TrimSpace(part) == "" {
				continue
			}
			parts = append(parts, part)
		}
		if len(parts) == 0 {
			result = append(result, chunk)
			continue
		}
		for _, piece := range sliceGeneralSplitPieces(chunk, parts) {
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
