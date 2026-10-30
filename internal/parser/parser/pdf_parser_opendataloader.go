package parser

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"ragflow/internal/common"
	"ragflow/internal/deepdoc/parser/pdf/table"
	"sort"
	"strings"
	"time"
)

// openDataLoaderTimeoutSeconds bounds the single submit call. OpenDataLoader
// parses synchronously, so a stalled server otherwise holds an ingestion worker
// open until the transport's own 20-minute header timeout.
const openDataLoaderTimeoutSeconds = 600

// parsePDFWithOpenDataLoader normalizes the selected server's JSON or Markdown
// output. Explicit tenant configuration never borrows a global endpoint or key.
func parsePDFWithOpenDataLoader(ctx context.Context, filename string, data []byte, parser *PDFParser) ParseResult {
	if len(data) == 0 {
		return emptyPDFResult(filename)
	}
	baseURL := strings.TrimSpace(parser.OpenDataLoaderAPIServer)
	if baseURL == "" && !parser.UseTenantModelConfig {
		baseURL = strings.TrimSpace(common.GetEnv(common.EnvOpenDataLoaderAPIServer))
	}
	if baseURL == "" {
		return ParseResult{Err: fmt.Errorf("parser: OpenDataLoader requires opendataloader_apiserver or OPENDATALOADER_APISERVER")}
	}
	apiKey := strings.TrimSpace(parser.OpenDataLoaderAPIKey)
	if apiKey == "" && !parser.UseTenantModelConfig {
		apiKey = strings.TrimSpace(common.GetEnv(common.EnvOpenDataLoaderAPIKey))
	}

	bodyReader, contentType, err := openDataLoaderMultipart(filename, data, parser)
	if err != nil {
		return ParseResult{Err: err}
	}
	timeout := time.Duration(parser.OpenDataLoaderTimeout) * time.Second
	if timeout <= 0 {
		timeout = openDataLoaderTimeoutSeconds * time.Second
	}
	// Derive from the pipeline context so a cancelled or expiring run stops
	// waiting on the server, and cap it by the configured timeout.
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/file_parse", bodyReader)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("parser: OpenDataLoader request: %w", err)}
	}
	req.Header.Set("Content-Type", contentType)
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := common.GetSSRFHTTPClient().Do(req)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("parser: OpenDataLoader submit: %w", err)}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("parser: OpenDataLoader read: %w", err)}
	}
	if resp.StatusCode >= 300 {
		return ParseResult{Err: fmt.Errorf("parser: OpenDataLoader HTTP %d: %s", resp.StatusCode, string(raw))}
	}
	var payload struct {
		JSONDoc any    `json:"json_doc"`
		MDText  string `json:"md_text"`
	}
	if err = json.Unmarshal(raw, &payload); err != nil {
		return ParseResult{Err: fmt.Errorf("parser: OpenDataLoader decode: %w", err)}
	}
	if payload.JSONDoc != nil {
		sizes, pageCount, geometryErr := openDataLoaderPageGeometry(ctx, data, openDataLoaderPageIndexes(payload.JSONDoc))
		items, boxWarnings := openDataLoaderItemsWithGeometry(payload.JSONDoc, sizes)
		if len(items) > 0 {
			if pageCount <= 0 {
				// No pdfium geometry (nocgo, or a document it cannot load):
				// fall back to the pages the service reported.
				pageCount = openDataLoaderReportedPageCount(payload.JSONDoc)
			}
			res := pdfItemsToResult(filename, items, pageCount)
			if geometryErr != nil {
				res.Warnings = append(res.Warnings, "OpenDataLoader PDF geometry unavailable: "+geometryErr.Error())
			}
			res.Warnings = append(res.Warnings, boxWarnings...)
			return res
		}
	}
	if strings.TrimSpace(payload.MDText) != "" {
		return pdfMarkdownToResult(ctx, filename, payload.MDText, 1)
	}
	return ParseResult{Err: fmt.Errorf("parser: OpenDataLoader returned no parsed content")}
}

func openDataLoaderMultipart(filename string, data []byte, parser *PDFParser) (io.Reader, string, error) {
	var body strings.Builder
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, "", fmt.Errorf("parser: OpenDataLoader create form file: %w", err)
	}
	if _, err = part.Write(data); err != nil {
		return nil, "", fmt.Errorf("parser: OpenDataLoader write PDF: %w", err)
	}
	if parser.OpenDataLoaderHybrid != "" {
		_ = writer.WriteField("hybrid", parser.OpenDataLoaderHybrid)
	}
	if parser.OpenDataLoaderImageOutput != "" {
		_ = writer.WriteField("image_output", parser.OpenDataLoaderImageOutput)
	}
	if parser.OpenDataLoaderSanitize != nil {
		if *parser.OpenDataLoaderSanitize {
			_ = writer.WriteField("sanitize", "true")
		} else {
			_ = writer.WriteField("sanitize", "false")
		}
	}
	if err = writer.Close(); err != nil {
		return nil, "", fmt.Errorf("parser: OpenDataLoader finalize form: %w", err)
	}
	return strings.NewReader(body.String()), writer.FormDataContentType(), nil
}

// openDataLoaderItemsWithGeometry flattens every typed element into a parse
// item. pageSizes maps a 0-based page index to the PDF's page size in points;
// when it is nil (nocgo, or a document pdfium cannot load) the items keep
// their text and the reported page number, but carry no positions. The second
// return value reports the pages whose boxes did not fit that page size.
func openDataLoaderItemsWithGeometry(root any, pageSizes map[int][2]float64) ([]map[string]any, []string) {
	items := []map[string]any{}
	// One warning per page: a page whose boxes disagree with the point space
	// is a coordinate-space mismatch worth surfacing, but repeating it per
	// element would bury the rest of the parse result in noise.
	reported := map[int]string{}
	var walk func(any)
	walk = func(node any) {
		switch v := node.(type) {
		case map[string]any:
			item, page, warning := openDataLoaderNodeToItem(v, pageSizes)
			if item != nil {
				items = append(items, item)
			}
			if warning != "" && reported[page] == "" {
				reported[page] = warning
			}
			for _, child := range v {
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(root)
	pages := make([]int, 0, len(reported))
	for page := range reported {
		pages = append(pages, page)
	}
	sort.Ints(pages)
	warnings := make([]string, 0, len(pages))
	for _, page := range pages {
		warnings = append(warnings, reported[page])
	}
	return items, warnings
}

func openDataLoaderNodeToItem(el map[string]any, pageSizes map[int][2]float64) (map[string]any, int, string) {
	rawType, _ := el["type"].(string)
	t := strings.ToLower(strings.TrimSpace(rawType))
	if t == "" {
		return nil, 0, ""
	}
	text := strings.TrimSpace(stringValue(el["content"]))
	if text == "" {
		text = strings.TrimSpace(stringValue(el["text"]))
	}
	switch t {
	case "table":
		html := strings.TrimSpace(stringValue(el["html"]))
		if html == "" {
			html = strings.TrimSpace(stringValue(el["html_content"]))
		}
		if html == "" {
			if rows := openDataLoaderCellRows(el["cells"]); rowsHaveText(rows) {
				html = table.SimpleRowsToHTML(rows)
			}
		}
		if html != "" {
			text = html
		}
		if text == "" {
			return nil, 0, ""
		}
		item := map[string]any{"text": text, "doc_type_kwd": pdfTableDocType(text), "layout": "table"}
		projectPDFTable(item)
		return openDataLoaderWithPositions(item, el, pageSizes)
	case "image", "picture", "figure":
		if text == "" {
			text = "[Image]"
		}
		return openDataLoaderWithPositions(map[string]any{"text": text, "doc_type_kwd": "image", "layout": "figure"}, el, pageSizes)
	case "formula", "equation":
		if text == "" {
			return nil, 0, ""
		}
		return openDataLoaderWithPositions(map[string]any{"text": text, "doc_type_kwd": "text", "layout": "equation"}, el, pageSizes)
	default:
		if text == "" {
			return nil, 0, ""
		}
		layout := "text"
		if t == "title" || t == "heading" {
			layout = "title"
		}
		return openDataLoaderWithPositions(map[string]any{"text": text, "doc_type_kwd": "text", "layout": layout}, el, pageSizes)
	}
}

func openDataLoaderWithPositions(item map[string]any, el map[string]any, pageSizes map[int][2]float64) (map[string]any, int, string) {
	page, ok := openDataLoaderPageNumber(el)
	if !ok {
		return item, 0, ""
	}
	// The page identity survives without geometry (nocgo, or a missing box),
	// matching the PaddleOCR adapter: downstream chunking and croppers can
	// still attribute the item to its page.
	item["page_number"] = page
	positions, warning := openDataLoaderPositions(page, el, pageSizes)
	if len(positions) > 0 {
		item["positions"], item["_pdf_positions"] = positions, positions
	}
	return item, page, warning
}

// openDataLoaderBleedTolerance is how far past the page edge a box may reach,
// as a fraction of that page dimension. Printed content bleeds slightly; a box
// further out than this is not a bleed but a disagreement about the coordinate
// space, so its positions are dropped and reported rather than clamped.
const openDataLoaderBleedTolerance = 0.1

// openDataLoaderPositions converts one element's geometry into RAGFlow's
// canonical [page, left, right, top, bottom] row. OpenDataLoader reports
// [left, bottom, right, top] in PDF points with the origin at the bottom-left,
// while positions are stored in screen space with the origin at the top-left,
// so the vertical edges are flipped against the page height. Without the page
// size the flip cannot be computed, and a wrong-y highlight is worse than none.
// The flip assumes both sides describe the same (post-rotation) page box, which
// holds for the page sizes pdfium reports; the point space itself is a service
// contract this adapter cannot read, so a box that cannot belong to the page is
// treated as evidence of a different space and reported through the second
// return value.
func openDataLoaderPositions(page int, el map[string]any, pageSizes map[int][2]float64) ([][]any, string) {
	size, ok := pageSizes[page-1]
	if !ok || size[0] <= 0 || size[1] <= 0 {
		return nil, ""
	}
	box, ok := openDataLoaderBox(el)
	if !ok {
		return nil, ""
	}
	left, bottom, right, top := box[0], box[1], box[2], box[3]
	if right <= left || top <= bottom {
		return nil, ""
	}
	overflowRight, overflowBottom := math.Max(0, math.Max(-left, right-size[0])), math.Max(0, math.Max(-bottom, top-size[1]))
	if overflowRight > size[0]*openDataLoaderBleedTolerance || overflowBottom > size[1]*openDataLoaderBleedTolerance {
		return nil, fmt.Sprintf("OpenDataLoader page %d: element box [%g %g %g %g] does not fit page size %gx%g; positions omitted because the service coordinates do not match the PDF point space", page, left, bottom, right, top, size[0], size[1])
	}
	left = math.Max(0, math.Min(size[0], left))
	right = math.Max(0, math.Min(size[0], right))
	screenTop := math.Max(0, math.Min(size[1], size[1]-top))
	screenBottom := math.Max(0, math.Min(size[1], size[1]-bottom))
	if right <= left || screenBottom <= screenTop {
		return nil, ""
	}
	return [][]any{{float64(page), left, right, screenTop, screenBottom}}, ""
}

// openDataLoaderReportedPageCount is the page-count fallback when pdfium is
// unavailable: prefer the root's page total, then the highest page number any
// element reports (a sparse document must not be counted by distinct pages),
// and finally one, because items exist.
func openDataLoaderReportedPageCount(root any) int {
	if top, ok := root.(map[string]any); ok {
		for _, key := range []string{"number of pages", "number_of_pages", "page count", "page_count"} {
			if count := int(numberValue(top[key])); count > 0 {
				return count
			}
		}
	}
	count := 0
	for page := range collectPDFPageNumbers(root) {
		if page > count {
			count = page
		}
	}
	if count == 0 {
		count = 1
	}
	return count
}

// openDataLoaderPageIndexes lists the zero-based pages the service reported
// content for, so the geometry lookup only opens those pages.
func openDataLoaderPageIndexes(root any) []int {
	pages := collectPDFPageNumbers(root)
	indexes := make([]int, 0, len(pages))
	for page := range pages {
		indexes = append(indexes, page-1)
	}
	sort.Ints(indexes)
	return indexes
}

func openDataLoaderPageNumber(el map[string]any) (int, bool) {
	for _, key := range []string{"page number", "page_number", "page"} {
		if raw, ok := el[key]; ok {
			n := numberValue(raw)
			if page := int(n); n > 0 && float64(page) == n {
				return page, true
			}
		}
	}
	return 0, false
}

func openDataLoaderBox(el map[string]any) ([4]float64, bool) {
	var box [4]float64
	for _, key := range []string{"bounding box", "bounding_box", "bbox"} {
		coords, ok := el[key].([]any)
		if !ok || len(coords) < 4 {
			continue
		}
		for i := 0; i < 4; i++ {
			v, ok := numericAny(coords[i])
			if !ok {
				return box, false
			}
			box[i] = v
		}
		return box, true
	}
	return box, false
}

func openDataLoaderCellRows(raw any) [][]string {
	cells, ok := raw.([]any)
	if !ok {
		return nil
	}
	rows := make(map[int][]string)
	maxRow := -1
	for _, cellRaw := range cells {
		cell, ok := cellRaw.(map[string]any)
		if !ok {
			continue
		}
		row := int(numberValue(cell["row"]))
		if row == 0 {
			row = int(numberValue(cell["row_index"]))
		}
		rows[row] = append(rows[row], stringValue(cell["content"]))
		if row > maxRow {
			maxRow = row
		}
	}
	if len(rows) == 0 || maxRow < 0 {
		return nil
	}
	ordered := make([][]string, 0, len(rows))
	for i := 0; i <= maxRow; i++ {
		if cols, ok := rows[i]; ok {
			ordered = append(ordered, cols)
		}
	}
	return ordered
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}

func numberValue(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	}
	return 0
}
