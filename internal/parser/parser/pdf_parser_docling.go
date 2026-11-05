package parser

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"ragflow/internal/common"
	"regexp"
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
		if md := strings.TrimSpace(doc.MDContent); md != "" {
			return doclingAttachPositions(pdfMarkdownToResult(ctx, filename, md, pageCount), doc.JSONContent), true
		}
		if txt := strings.TrimSpace(doc.TextContent); txt != "" {
			return doclingAttachPositions(doclingTextsToResult(filename, []string{txt}, pageCount), doc.JSONContent), true
		}
		if md, _ := doc.JSONContent["md_content"].(string); strings.TrimSpace(md) != "" {
			return doclingAttachPositions(pdfMarkdownToResult(ctx, filename, md, pageCount), doc.JSONContent), true
		}
	}
	return ParseResult{}, false
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

// doclingProvPosition converts a Docling provenance entry (1-based page_no and
// bbox {l, t, r, b}) into a [page, left, right, top, bottom] row with top
// above bottom. Docling permits bbox coord_origin BOTTOMLEFT (y measured from
// the page bottom); those boxes are converted to the top-left crop origin
// when the page height is known, otherwise the sorted values are kept.
func doclingProvPosition(prov map[string]any, heights map[int]float64) []any {
	page, ok := numericAny(prov["page_no"])
	if !ok || page < 1 {
		return nil
	}
	bbox, _ := prov["bbox"].(map[string]any)
	var v [4]float64
	for i, key := range []string{"l", "t", "r", "b"} {
		n, ok := numericAny(bbox[key])
		if !ok {
			return nil
		}
		v[i] = n
	}
	left, top, right, bottom := v[0], v[1], v[2], v[3]
	if origin, _ := bbox["coord_origin"].(string); strings.EqualFold(origin, "BOTTOMLEFT") {
		h := heights[int(page)]
		if h <= 0 {
			// Without the page height the bottom-origin Y cannot be converted
			// to the top-origin contract downstream; a missing position is
			// safer than a silently wrong crop.
			return nil
		}
		top, bottom = h-top, h-bottom
	}
	if left > right {
		left, right = right, left
	}
	if top > bottom {
		top, bottom = bottom, top
	}
	return []any{int(page), left, right, top, bottom}
}

// doclingPageHeights maps 1-based page numbers to page heights from the
// json_content pages metadata, used to convert BOTTOMLEFT bounding boxes.
func doclingPageHeights(content map[string]any) map[int]float64 {
	heights := map[int]float64{}
	collect := func(page map[string]any, keyNo int) {
		no, ok := numericAny(page["page_no"])
		if !ok && keyNo > 0 {
			no, ok = float64(keyNo), true
		}
		if !ok || no < 1 {
			return
		}
		size, _ := page["size"].(map[string]any)
		if h, ok := numericAny(size["height"]); ok && h > 0 {
			heights[int(no)] = h
		}
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
	return heights
}

func doclingNormalizeText(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

var doclingMarkdownPrefix = regexp.MustCompile(`^(#{1,6}\s+|[-*]\s+|\d+\.\s+)`)

// doclingStripMarkdownPrefix removes one recognized Markdown heading or list
// marker. Leading digits elsewhere are content ("2026 Outlook") and stay.
func doclingStripMarkdownPrefix(text string) (string, bool) {
	stripped := doclingMarkdownPrefix.ReplaceAllString(text, "")
	return stripped, stripped != text
}

// doclingAttachPositions copies page positions from Docling's structured
// json_content onto JSON items whose text matches a Docling text element. Items
// without a match are left unchanged.
func doclingAttachPositions(res ParseResult, content map[string]any) ParseResult {
	if res.Err != nil || len(res.JSON) == 0 || len(content) == 0 {
		return res
	}
	// Group positions per Docling text entry so repeated text keeps its own
	// provenance: the i-th parsed occurrence takes the i-th entry's
	// positions instead of every position of every identical entry.
	byText := map[string][][][]any{}
	heights := doclingPageHeights(content)
	for _, entry := range doclingTextEntries(content) {
		text := doclingNormalizeText(stringValue(entry["text"]))
		if text == "" {
			continue
		}
		var positions [][]any
		for _, prov := range doclingProvs(entry) {
			if pos := doclingProvPosition(prov, heights); pos != nil {
				positions = append(positions, pos)
			}
		}
		if len(positions) > 0 {
			byText[text] = append(byText[text], positions)
		}
	}
	if len(byText) == 0 {
		return res
	}
	itemMaps := make([]map[string]any, len(res.JSON))
	matched := false
	used := map[string]int{}
	for i, item := range res.JSON {
		m := item.ToMap()
		text := doclingNormalizeText(stringValue(m["text"]))
		matches, ok := byText[text]
		if !ok {
			// List and heading markers are not content, but leading digits
			// can be; only retry without a recognized Markdown prefix.
			if stripped, changed := doclingStripMarkdownPrefix(text); changed {
				if matches, ok = byText[stripped]; ok {
					text = stripped
				}
			}
		}
		if ok && text != "" {
			if idx := used[text]; idx < len(matches) {
				m["positions"] = matches[idx]
				m["_pdf_positions"] = matches[idx]
				used[text]++
				matched = true
			}
		}
		itemMaps[i] = m
	}
	if !matched {
		return res
	}
	res.JSON = MapsToItems(itemMaps)
	return res
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
