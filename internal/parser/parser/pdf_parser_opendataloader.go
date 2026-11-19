package parser

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"ragflow/internal/common"
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

// openDataLoaderBleedTolerance is how far past the page edge a box may reach,
// as a fraction of that page dimension. Printed content bleeds slightly; a box
// further out is not a bleed but a disagreement about the coordinate space.
const openDataLoaderBleedTolerance = 0.1

// openDataLoaderItemsWithGeometry flattens every typed element into a parse
// item. pageSizes maps a 0-based page index to the PDF's page size in points;
// when it is nil (nocgo, or a document pdfium cannot load) the items keep
// their text and the reported page number, but carry no positions. The second
// return value reports the pages whose boxes did not fit that page size.
//
// OpenDataLoader reports boxes as [left, bottom, right, top] in PDF points
// with the origin at the bottom-left. The point space is a service contract
// this adapter cannot read, so a box that cannot belong to its page is taken
// as evidence of a different space.
func openDataLoaderItemsWithGeometry(root any, pageSizes map[int][2]float64) ([]map[string]any, []string) {
	var blocks []ExternalBlock
	var walk func(any)
	walk = func(node any) {
		switch v := node.(type) {
		case map[string]any:
			if block, ok := openDataLoaderBlock(v); ok {
				blocks = append(blocks, block)
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
	return externalBlockMaps(blocks, ExternalBlockOptions{
		Provider:          "OpenDataLoader",
		PageSizes:         externalPageSizes(pageSizes),
		BleedTolerance:    openDataLoaderBleedTolerance,
		FigurePlaceholder: "[Image]",
	})
}

// openDataLoaderBlock reads one typed element. Nodes without a type are
// containers and yield no block.
func openDataLoaderBlock(el map[string]any) (ExternalBlock, bool) {
	rawType, _ := el["type"].(string)
	t := strings.ToLower(strings.TrimSpace(rawType))
	if t == "" {
		return ExternalBlock{}, false
	}
	block := ExternalBlock{Type: ExternalBlockText, Text: stringValue(el["content"])}
	if strings.TrimSpace(block.Text) == "" {
		block.Text = stringValue(el["text"])
	}
	switch t {
	case "table":
		block.Type = ExternalBlockTable
		block.HTML = stringValue(el["html"])
		if strings.TrimSpace(block.HTML) == "" {
			block.HTML = stringValue(el["html_content"])
		}
		block.TableCells = openDataLoaderCellRows(el["cells"])
	case "image", "picture", "figure":
		block.Type = ExternalBlockFigure
	case "formula", "equation":
		block.Type = ExternalBlockEquation
	case "title", "heading":
		block.Type = ExternalBlockTitle
	}
	if page, ok := openDataLoaderPageNumber(el); ok {
		block.Page = page
		if box, ok := openDataLoaderBox(el); ok {
			block.Regions = []ExternalRegion{{Page: page, Box: box, BottomLeft: true}}
		}
	}
	return block, true
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
