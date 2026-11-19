package parser

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"maps"
	"math"
	"ragflow/internal/common"
	"sort"
	"strconv"
	"strings"

	"github.com/ledongthuc/pdf"
	"go.uber.org/zap"

	models "ragflow/internal/entity/models"
)

type doclingChunk struct {
	Text  string `json:"text"`
	Chunk *struct {
		Text string `json:"text"`
	} `json:"chunk"`
}

type doclingDocument struct {
	MDContent   string         `json:"md_content"`
	TextContent string         `json:"text_content"`
	JSONContent map[string]any `json:"json_content"`
}

type doclingResult struct {
	Document *doclingDocument `json:"document"`
	Result   *doclingDocument `json:"result"`
}

type doclingResponse struct {
	Document  *doclingDocument  `json:"document"`
	Documents []doclingDocument `json:"documents"`
	Results   []doclingResult   `json:"results"`
}

func parsePDFWithDocling(ctx context.Context, filename string, data []byte, parser *PDFParser) ParseResult {
	if len(data) == 0 {
		return emptyPDFResult(filename)
	}
	serverURL := strings.TrimSpace(parser.DoclingServerURL)
	if serverURL == "" {
		serverURL = strings.TrimSpace(common.GetEnv(common.EnvDoclingServerURL))
	}
	if serverURL == "" {
		return ParseResult{Err: fmt.Errorf("parser: Docling requires docling_server_url or DOCLING_SERVER_URL")}
	}
	apiKey := strings.TrimSpace(parser.DoclingAPIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(common.GetEnv(common.EnvDoclingAPIKey))
	}

	baseURL := strings.TrimRight(serverURL, "/")
	auth := ""
	if apiKey != "" {
		auth = "Bearer " + apiKey
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	convertOptions := doclingConvertOptions(parser)
	payloads := []struct {
		endpoint string
		body     func() map[string]any
		chunked  bool
	}{
		// Standard responses carry per-page provenance (json_content.prov), so
		// they are tried first: positions are the point of this path. Chunked
		// responses have no provenance and stay as the fallback for servers or
		// documents the standard conversion rejects.
		{
			endpoint: "/v1/convert/source",
			chunked:  false,
			body:     func() map[string]any { return doclingStandardPayload(filename, encoded, false, convertOptions) },
		},
		{
			endpoint: "/v1alpha/convert/source",
			chunked:  false,
			body:     func() map[string]any { return doclingStandardPayload(filename, encoded, true, convertOptions) },
		},
		{
			endpoint: "/v1/convert/source",
			chunked:  true,
			body:     func() map[string]any { return doclingChunkedPayload(filename, encoded, false, convertOptions) },
		},
		{
			endpoint: "/v1alpha/convert/source",
			chunked:  true,
			body:     func() map[string]any { return doclingChunkedPayload(filename, encoded, true, convertOptions) },
		},
	}

	sourcePages := doclingSourcePageCount(data)

	var lastErr error
	for _, candidate := range payloads {
		// Each candidate marshals the whole base64 PDF, so stop before building
		// another request once the run is cancelled.
		if err := ctx.Err(); err != nil {
			lastErr = err
			break
		}
		url := baseURL + candidate.endpoint
		resp, err := models.PostJSONRequest(ctx, common.GetSSRFHTTPClient(), url, auth, candidate.body())
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", candidate.endpoint, err)
			continue
		}
		body, readErr := func() ([]byte, error) {
			defer resp.Body.Close()
			return io.ReadAll(resp.Body)
		}()
		if readErr != nil {
			lastErr = fmt.Errorf("%s: read response: %w", candidate.endpoint, readErr)
			continue
		}
		if resp.StatusCode >= 300 {
			if candidate.chunked {
				lastErr = fmt.Errorf("%s: HTTP %d", candidate.endpoint, resp.StatusCode)
				continue
			}
			lastErr = fmt.Errorf("%s: HTTP %d %s", candidate.endpoint, resp.StatusCode, string(body))
			continue
		}
		if candidate.chunked {
			if res, ok := parseDoclingChunkedResult(filename, body, sourcePages); ok {
				common.Warn("Docling chunked response carries no page provenance; positions unavailable",
					zap.String("filename", filename))
				res.Warnings = append(res.Warnings, "Docling chunked responses carry no page provenance; positions unavailable")
				return res
			}
			lastErr = fmt.Errorf("%s: chunked response contained no usable text", candidate.endpoint)
			continue
		}
		if res, ok := parseDoclingStandardResult(ctx, filename, body, sourcePages); ok {
			return res
		}
		lastErr = fmt.Errorf("%s: standard response contained no parsed document", candidate.endpoint)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("Docling remote convert failed")
	}
	return ParseResult{Err: fmt.Errorf("parser: Docling convert: %w", lastErr)}
}

// doclingConvertOptions collects the Docling Serve conversion options set in the
// parser setup, falling back to the environment. Unset options are left out so
// Docling Serve keeps its own defaults.
func doclingConvertOptions(parser *PDFParser) map[string]any {
	options := map[string]any{}
	if parser.DoclingDoOCR != nil {
		options["do_ocr"] = *parser.DoclingDoOCR
	} else if v, err := strconv.ParseBool(strings.TrimSpace(common.GetEnv(common.EnvDoclingDoOCR))); err == nil {
		options["do_ocr"] = v
	}
	backend := strings.TrimSpace(parser.DoclingPDFBackend)
	if backend == "" {
		backend = strings.TrimSpace(common.GetEnv(common.EnvDoclingPDFBackend))
	}
	if backend != "" {
		options["pdf_backend"] = backend
	}
	return options
}

func doclingStandardPayload(filename string, encoded string, alpha bool, convertOptions map[string]any) map[string]any {
	source := map[string]any{"filename": filename, "base64_string": encoded}
	options := map[string]any{"from_formats": []string{"pdf"}, "to_formats": []string{"json", "md", "text"}}
	maps.Copy(options, convertOptions)
	if alpha {
		return map[string]any{
			"options":      options,
			"file_sources": []map[string]any{source},
		}
	}
	source["kind"] = "file"
	return map[string]any{
		"options": options,
		"sources": []map[string]any{source},
	}
}

func doclingChunkedPayload(filename string, encoded string, alpha bool, convertOptions map[string]any) map[string]any {
	payload := doclingStandardPayload(filename, encoded, alpha, convertOptions)
	options := map[string]any{
		"from_formats": []string{"pdf"},
		"to_formats":   []string{"json", "md", "text"},
		"do_chunking":  true,
		"chunking_options": map[string]any{
			"max_tokens": 512,
			"overlap":    50,
			"tokenizer":  "sentencepiece",
		},
	}
	maps.Copy(options, convertOptions)
	payload["options"] = options
	return payload
}

func parseDoclingChunkedResult(filename string, body []byte, sourcePages int) (ParseResult, bool) {
	var chunks []doclingChunk
	if err := json.Unmarshal(body, &chunks); err != nil {
		var wrapped struct {
			Results []doclingChunk `json:"results"`
		}
		if err := json.Unmarshal(body, &wrapped); err != nil {
			return ParseResult{}, false
		}
		chunks = wrapped.Results
	}
	texts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		text := strings.TrimSpace(chunk.Text)
		if text == "" && chunk.Chunk != nil {
			text = strings.TrimSpace(chunk.Chunk.Text)
		}
		if text != "" {
			texts = append(texts, text)
		}
	}
	if len(texts) == 0 {
		return ParseResult{}, false
	}
	// A chunked response carries no page metadata: report the source
	// PDF's real page count when known, not the number of text chunks.
	pageCount := len(texts)
	if sourcePages > 0 {
		pageCount = sourcePages
	}
	return doclingTextsToResult(filename, texts, pageCount), true
}

// doclingNoProvenanceWarning is attached when a standard response has to be
// read from its Markdown or text export.
const doclingNoProvenanceWarning = "Docling response carries no usable page provenance; read from its Markdown or text export and positions are unavailable"

// parseDoclingStandardResult builds the result from json_content, the typed
// element stream that carries each element's label, page and box. The
// Markdown and text exports of the same document are a low-fidelity fallback
// for a response whose elements cannot be placed on a page.
func parseDoclingStandardResult(ctx context.Context, filename string, body []byte, sourcePages int) (ParseResult, bool) {
	var payload doclingResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return ParseResult{}, false
	}
	docs := make([]doclingDocument, 0, 1+len(payload.Documents)+len(payload.Results))
	if payload.Document != nil {
		docs = append(docs, *payload.Document)
	}
	docs = append(docs, payload.Documents...)
	for _, result := range payload.Results {
		switch {
		case result.Document != nil:
			docs = append(docs, *result.Document)
		case result.Result != nil:
			docs = append(docs, *result.Result)
		}
	}
	pageCount := doclingPageCount(docs, sourcePages)
	for _, doc := range docs {
		items, warnings := ExternalBlocksToItems(doclingBlocks(doc.JSONContent), ExternalBlockOptions{
			Provider:  "Docling",
			PageSizes: doclingPageSizes(doc.JSONContent),
			// Docling does not fix which vertical edge is "t": it depends on
			// the box's coord_origin.
			ReorderEdges: true,
		})
		structured := ParseResult{File: pdfFileMeta(filename, pageCount), JSON: items, Warnings: warnings}
		if doclingHasPositions(items) {
			return structured, true
		}
		fallback, ok := ParseResult{}, false
		if md := strings.TrimSpace(doc.MDContent); md != "" {
			fallback, ok = pdfMarkdownToResult(ctx, filename, md, pageCount), true
		} else if txt := strings.TrimSpace(doc.TextContent); txt != "" {
			fallback, ok = doclingTextsToResult(filename, []string{txt}, pageCount), true
		} else if md, _ := doc.JSONContent["md_content"].(string); strings.TrimSpace(md) != "" {
			fallback, ok = pdfMarkdownToResult(ctx, filename, md, pageCount), true
		}
		if ok {
			common.Warn(doclingNoProvenanceWarning, zap.String("filename", filename))
			fallback.Warnings = append(fallback.Warnings, doclingNoProvenanceWarning)
			return fallback, true
		}
		if len(items) > 0 {
			return structured, true
		}
	}
	return ParseResult{}, false
}

func doclingHasPositions(items []DocItem) bool {
	for _, item := range items {
		if _, ok := ExtractPDFPositions(item.ToMap()); ok {
			return true
		}
	}
	return false
}

// doclingBlocks reads json_content into blocks in reading order. The body
// tree gives that order and nests each caption under its table or picture; a
// document without one is read as its flat element lists.
func doclingBlocks(content map[string]any) []ExternalBlock {
	nodes := map[string]map[string]any{}
	var listed []string
	for _, key := range []string{"texts", "tables", "pictures", "groups"} {
		entries, _ := content[key].([]any)
		for index, raw := range entries {
			node, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			ref := fmt.Sprintf("#/%s/%d", key, index)
			nodes[ref] = node
			if key != "groups" {
				listed = append(listed, ref)
			}
		}
	}
	walker := doclingWalker{nodes: nodes, seen: map[string]bool{}}
	if body, _ := content["body"].(map[string]any); len(doclingRefs(body["children"])) > 0 {
		walker.children(body)
	} else {
		for _, ref := range listed {
			walker.visit(ref)
		}
	}
	return walker.blocks
}

// doclingWalker visits each element once, so a caption attached to a table or
// picture is not emitted again as body text.
type doclingWalker struct {
	nodes  map[string]map[string]any
	seen   map[string]bool
	blocks []ExternalBlock
}

func (w *doclingWalker) children(node map[string]any) {
	for _, ref := range doclingRefs(node["children"]) {
		w.visit(ref)
	}
}

func (w *doclingWalker) visit(ref string) {
	node := w.nodes[ref]
	if node == nil || w.seen[ref] {
		return
	}
	w.seen[ref] = true
	// Page furniture and background layers are not document content; the
	// Markdown export leaves them out as well.
	if layer := stringValue(node["content_layer"]); layer != "" && layer != "body" {
		return
	}
	switch {
	case strings.HasPrefix(ref, "#/groups/"):
		if stringValue(node["label"]) != "inline" {
			w.children(node)
			return
		}
		// An inline group is one paragraph that Docling split by formatting.
		block := ExternalBlock{Type: ExternalBlockText}
		for _, child := range doclingRefs(node["children"]) {
			part := w.nodes[child]
			if part == nil || w.seen[child] {
				continue
			}
			w.seen[child] = true
			block.Text = strings.TrimSpace(block.Text + " " + doclingText(part))
			block.Regions = append(block.Regions, doclingRegions(part)...)
			if block.Page == 0 {
				block.Page = doclingPage(part)
			}
		}
		w.blocks = append(w.blocks, block)
	case strings.HasPrefix(ref, "#/tables/"):
		data, _ := node["data"].(map[string]any)
		w.blocks = append(w.blocks, ExternalBlock{
			Type:      ExternalBlockTable,
			HTML:      doclingTableHTML(data),
			Captions:  w.attached(node["captions"]),
			Footnotes: w.attached(node["footnotes"]),
			Page:      doclingPage(node),
			Regions:   doclingRegions(node),
		})
		// A table's children are its captions and the contents of its cells,
		// both already in the block.
		for _, child := range doclingRefs(node["children"]) {
			w.seen[child] = true
		}
	case strings.HasPrefix(ref, "#/pictures/"):
		w.blocks = append(w.blocks, ExternalBlock{
			Type:      ExternalBlockFigure,
			Captions:  w.attached(node["captions"]),
			Footnotes: w.attached(node["footnotes"]),
			Page:      doclingPage(node),
			Regions:   doclingRegions(node),
		})
		// Text recognized inside the picture follows it as body text.
		w.children(node)
	default:
		if block, ok := doclingTextBlock(node); ok {
			w.blocks = append(w.blocks, block)
		}
		w.children(node)
	}
}

// attached resolves caption or footnote references to their text and claims
// the referenced elements for the table or picture.
func (w *doclingWalker) attached(raw any) []string {
	var texts []string
	for _, ref := range doclingRefs(raw) {
		node := w.nodes[ref]
		if node == nil || w.seen[ref] {
			continue
		}
		w.seen[ref] = true
		if text := doclingText(node); text != "" {
			texts = append(texts, text)
		}
	}
	return texts
}

// doclingTextBlock maps a text element's label onto a block type. Labels that
// carry no structure of their own (text, paragraph, caption, footnote,
// reference, ...) are body text.
func doclingTextBlock(node map[string]any) (ExternalBlock, bool) {
	block := ExternalBlock{Type: ExternalBlockText, Text: doclingText(node), Page: doclingPage(node), Regions: doclingRegions(node)}
	switch strings.ToLower(strings.TrimSpace(stringValue(node["label"]))) {
	case "page_header", "page_footer":
		return ExternalBlock{}, false
	case "title":
		block.Type, block.HeadingLevel = ExternalBlockHeading, 1
	case "section_header":
		// The document title is level 1, so a section header of Docling
		// level n is written one level below it, as in the Markdown export.
		level := 1
		if n, ok := numericAny(node["level"]); ok && n >= 1 {
			level = int(n)
		}
		block.Type, block.HeadingLevel = ExternalBlockHeading, min(level+1, 6)
	case "list_item":
		marker := "-"
		if enumerated, _ := node["enumerated"].(bool); enumerated {
			if m := strings.TrimSpace(stringValue(node["marker"])); m != "" {
				marker = m
			}
		}
		block.Type = ExternalBlockList
		if block.Text != "" {
			block.Text = marker + " " + block.Text
		}
	case "code":
		block.Type = ExternalBlockCode
	case "formula":
		block.Type = ExternalBlockEquation
	}
	return block, true
}

// doclingText returns an element's text, falling back to the text as it was
// read from the page when Docling produced no cleaned form.
func doclingText(node map[string]any) string {
	if text := strings.TrimSpace(stringValue(node["text"])); text != "" {
		return text
	}
	return strings.TrimSpace(stringValue(node["orig"]))
}

func doclingRefs(raw any) []string {
	list, _ := raw.([]any)
	refs := make([]string, 0, len(list))
	for _, entry := range list {
		obj, _ := entry.(map[string]any)
		if ref := stringValue(obj["$ref"]); ref != "" {
			refs = append(refs, ref)
		}
	}
	return refs
}

// doclingProvPage returns the 1-based page of a provenance entry.
func doclingProvPage(prov map[string]any) (int, bool) {
	page, ok := numericAny(prov["page_no"])
	if !ok || page < 1 || page != math.Trunc(page) {
		return 0, false
	}
	return int(page), true
}

// doclingPage returns the page an element starts on, or zero when its
// provenance names none. The page is known even when the box is not.
func doclingPage(node map[string]any) int {
	for _, prov := range doclingProvs(node) {
		if page, ok := doclingProvPage(prov); ok {
			return page
		}
	}
	return 0
}

// doclingRegions converts an element's provenance entries (1-based page_no
// and bbox {l, t, r, b}) into regions. A bbox with coord_origin BOTTOMLEFT
// measures y from the bottom of the page.
func doclingRegions(node map[string]any) []ExternalRegion {
	var regions []ExternalRegion
	for _, prov := range doclingProvs(node) {
		page, ok := doclingProvPage(prov)
		if !ok {
			continue
		}
		bbox, _ := prov["bbox"].(map[string]any)
		region := ExternalRegion{Page: page}
		complete := true
		for i, key := range []string{"l", "t", "r", "b"} {
			if region.Box[i], ok = numericAny(bbox[key]); !ok {
				complete = false
			}
		}
		if !complete {
			continue
		}
		origin, _ := bbox["coord_origin"].(string)
		region.BottomLeft = strings.EqualFold(origin, "BOTTOMLEFT")
		regions = append(regions, region)
	}
	return regions
}

// doclingTableRows returns the cells of each table row, each merged cell once.
// The grid repeats a merged cell in every slot it covers, so only the slot
// where the cell starts counts. A table without a grid is read from its cell
// list, where every cell names the row and column it starts at.
func doclingTableRows(data map[string]any) [][]map[string]any {
	start := func(cell map[string]any, key string) (int, bool) {
		n, ok := numericAny(cell[key])
		return int(n), ok && n >= 0
	}
	var rows [][]map[string]any
	if grid, _ := data["grid"].([]any); len(grid) > 0 {
		for rowIndex, rawRow := range grid {
			var row []map[string]any
			rawCells, _ := rawRow.([]any)
			for colIndex, rawCell := range rawCells {
				cell, ok := rawCell.(map[string]any)
				if !ok {
					continue
				}
				if at, ok := start(cell, "start_row_offset_idx"); ok && at != rowIndex {
					continue
				}
				if at, ok := start(cell, "start_col_offset_idx"); ok && at != colIndex {
					continue
				}
				row = append(row, cell)
			}
			rows = append(rows, row)
		}
		return rows
	}
	cells, _ := data["table_cells"].([]any)
	for _, rawCell := range cells {
		cell, ok := rawCell.(map[string]any)
		if !ok {
			continue
		}
		rowIndex, ok := start(cell, "start_row_offset_idx")
		if !ok || rowIndex >= doclingMaxTableRows {
			continue
		}
		for len(rows) <= rowIndex {
			rows = append(rows, nil)
		}
		rows[rowIndex] = append(rows[rowIndex], cell)
	}
	for _, row := range rows {
		sort.SliceStable(row, func(i, j int) bool {
			left, _ := start(row[i], "start_col_offset_idx")
			right, _ := start(row[j], "start_col_offset_idx")
			return left < right
		})
	}
	return rows
}

// doclingMaxTableRows bounds the row index a cell list may claim, so a
// malformed offset cannot allocate rows without limit.
const doclingMaxTableRows = 100_000

// doclingTableHTML renders a table as markup once, keeping merged cells and
// header rows. It returns "" for a table with no text in any cell.
func doclingTableHTML(data map[string]any) string {
	var b strings.Builder
	hasText := false
	b.WriteString("<table>")
	for _, row := range doclingTableRows(data) {
		b.WriteString("<tr>")
		for _, cell := range row {
			tag := "td"
			if header, _ := cell["column_header"].(bool); header {
				tag = "th"
			}
			b.WriteString("<" + tag)
			for _, span := range [][2]string{{"row_span", "rowspan"}, {"col_span", "colspan"}} {
				if n, ok := numericAny(cell[span[0]]); ok && n > 1 {
					fmt.Fprintf(&b, ` %s="%d"`, span[1], int(n))
				}
			}
			text := strings.TrimSpace(stringValue(cell["text"]))
			hasText = hasText || text != ""
			b.WriteString(">" + html.EscapeString(text) + "</" + tag + ">")
		}
		b.WriteString("</tr>")
	}
	b.WriteString("</table>")
	if !hasText {
		return ""
	}
	return b.String()
}

// doclingPageCount returns the number of PDF pages described by the Docling
// documents: the size of json_content.pages when present, otherwise the
// highest page_no found in any provenance entry. A single source converts to
// one logical document, so payload shapes that repeat it (document,
// documents, results) are duplicates: the maximum per-document count wins,
// never the sum. It falls back to the number of documents only when the
// response carries no page information.
func doclingPageCount(docs []doclingDocument, sourcePages int) int {
	total := 0
	for _, doc := range docs {
		count := 0
		switch pages := doc.JSONContent["pages"].(type) {
		case map[string]any:
			count = len(pages)
		case []any:
			count = len(pages)
		}
		for _, entry := range doclingTextEntries(doc.JSONContent) {
			for _, prov := range doclingProvs(entry) {
				if page, ok := numericAny(prov["page_no"]); ok && int(page) > count {
					count = int(page)
				}
			}
		}
		if count > total {
			total = count
		}
	}
	// Provenance can end before the final page when the backend extracts a
	// partial document, so the source PDF page count is a lower bound.
	if sourcePages > total {
		return sourcePages
	}
	if total == 0 {
		// No page metadata at all: report the number of returned documents.
		return len(docs)
	}
	return total
}

// doclingMaxSourcePageCount bounds a catalog /Count read from the source PDF;
// a corrupt or malicious value far beyond any real document is ignored.
const doclingMaxSourcePageCount = 1_000_000

// doclingSourcePageCount counts the pages of the source PDF so a Docling
// response without page metadata still reports the real page count. It
// returns 0 when the bytes cannot be opened or the count is implausible.
func doclingSourcePageCount(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return 0
	}
	if pages := reader.NumPage(); pages > 0 && pages <= doclingMaxSourcePageCount {
		return pages
	}
	return 0
}

func doclingTextEntries(content map[string]any) []map[string]any {
	var entries []map[string]any
	texts, _ := content["texts"].([]any)
	for _, raw := range texts {
		if entry, ok := raw.(map[string]any); ok {
			entries = append(entries, entry)
		}
	}
	return entries
}

func doclingProvs(entry map[string]any) []map[string]any {
	var provs []map[string]any
	list, _ := entry["prov"].([]any)
	for _, raw := range list {
		if prov, ok := raw.(map[string]any); ok {
			provs = append(provs, prov)
		}
	}
	return provs
}

// doclingPageSizes maps 1-based page numbers to [width, height] from the
// json_content pages metadata. The height converts BOTTOMLEFT boxes and the
// size keeps a box inside its page.
func doclingPageSizes(content map[string]any) map[int][2]float64 {
	sizes := map[int][2]float64{}
	collect := func(page map[string]any, keyNo int) {
		no, ok := numericAny(page["page_no"])
		if !ok && keyNo > 0 {
			no, ok = float64(keyNo), true
		}
		if !ok || no < 1 {
			return
		}
		size, _ := page["size"].(map[string]any)
		width, _ := numericAny(size["width"])
		height, _ := numericAny(size["height"])
		sizes[int(no)] = [2]float64{width, height}
	}
	switch pages := content["pages"].(type) {
	case map[string]any:
		for key, raw := range pages {
			page, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			keyNo, _ := strconv.Atoi(key)
			collect(page, keyNo)
		}
	case []any:
		for _, raw := range pages {
			if page, ok := raw.(map[string]any); ok {
				collect(page, 0)
			}
		}
	}
	return sizes
}

func doclingTextsToResult(filename string, texts []string, pageCount int) ParseResult {
	items := make([]map[string]any, 0, len(texts))
	for _, text := range texts {
		items = append(items, map[string]any{
			"text":         text,
			"doc_type_kwd": "text",
		})
	}
	return ParseResult{
		File: pdfFileMeta(filename, pageCount),
		JSON: MapsToItems(items),
	}
}
