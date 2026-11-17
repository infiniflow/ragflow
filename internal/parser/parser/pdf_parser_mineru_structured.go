package parser

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"math"
	"strings"
)

const minerUBoxScale = 1000.0

// minerUItemsFromZip reads MinerU content_list.json, or structured_content.json
// when the zip has no usable content list, into parser items. Page identity,
// block type, and caption text stay on the block that produced them.
// middle.json page sizes turn normalized boxes into PDF-point positions.
// A zip without those JSON members returns nil so the caller can use markdown.
func minerUItemsFromZip(zipBytes []byte) ([]map[string]any, int) {
	if len(zipBytes) == 0 {
		return nil, 0
	}
	reader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, 0
	}
	var contentList, structured, middle []byte
	for _, file := range reader.File {
		name := strings.ToLower(strings.ReplaceAll(file.Name, "\\", "/"))
		if strings.Contains(name, "__macosx") {
			continue
		}
		var dest *[]byte
		switch {
		case strings.HasSuffix(name, "content_list.json") && len(contentList) == 0:
			dest = &contentList
		case strings.HasSuffix(name, "structured_content.json") && len(structured) == 0:
			dest = &structured
		case strings.HasSuffix(name, "middle.json") && len(middle) == 0:
			dest = &middle
		default:
			continue
		}
		body, readErr := readMinerUZipFile(file)
		if readErr != nil {
			return nil, 0
		}
		*dest = body
	}
	sizes := minerUPageSizes(middle)
	items := minerUItemsFromContentList(contentList, sizes)
	if len(items) == 0 {
		items = minerUItemsFromStructured(structured, sizes)
	}
	if len(items) == 0 {
		return nil, 0
	}
	return items, minerUSourcePageCount(contentList, middle, structured, items)
}

func readMinerUZipFile(file *zip.File) ([]byte, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func minerUPageSizes(middle []byte) map[int][2]float64 {
	if len(middle) == 0 {
		return nil
	}
	var parsed struct {
		PdfInfo []struct {
			PageIdx  *int      `json:"page_idx"`
			PageSize []float64 `json:"page_size"`
		} `json:"pdf_info"`
		Extensions struct {
			DocVortexLayout struct {
				Pages []struct {
					PageIdx  *int    `json:"page_idx"`
					WidthPt  float64 `json:"width_pt"`
					HeightPt float64 `json:"height_pt"`
				} `json:"pages"`
			} `json:"docvortex_layout"`
		} `json:"extensions"`
	}
	if err := json.Unmarshal(middle, &parsed); err != nil {
		return nil
	}
	sizes := make(map[int][2]float64, len(parsed.PdfInfo))
	for i, page := range parsed.PdfInfo {
		if len(page.PageSize) < 2 || page.PageSize[0] <= 0 || page.PageSize[1] <= 0 {
			continue
		}
		idx := i
		if page.PageIdx != nil && *page.PageIdx >= 0 {
			idx = *page.PageIdx
		}
		sizes[idx] = [2]float64{page.PageSize[0], page.PageSize[1]}
	}
	if len(sizes) > 0 {
		return sizes
	}
	// MinerU 4 middle JSON omits pdf_info. Page size is already in PDF points.
	for i, page := range parsed.Extensions.DocVortexLayout.Pages {
		if page.WidthPt <= 0 || page.HeightPt <= 0 {
			continue
		}
		idx := i
		if page.PageIdx != nil && *page.PageIdx >= 0 {
			idx = *page.PageIdx
		}
		sizes[idx] = [2]float64{page.WidthPt, page.HeightPt}
	}
	if len(sizes) == 0 {
		return nil
	}
	return sizes
}

func minerUSourcePageCount(contentList, middle, structured []byte, items []map[string]any) int {
	count := minerUHighestItemPage(items)
	if extent := minerUContentListPageExtent(contentList); extent > count {
		count = extent
	}
	if extent := minerUJSONPageExtent(middle); extent > count {
		count = extent
	}
	if extent := minerUStructuredPageExtent(structured); extent > count {
		count = extent
	}
	return count
}

func minerUJSONPageExtent(raw []byte) int {
	if len(raw) == 0 {
		return 0
	}
	var parsed struct {
		PdfInfo []struct {
			PageIdx *int `json:"page_idx"`
		} `json:"pdf_info"`
		Extensions struct {
			DocVortexLayout struct {
				Pages []struct {
					PageIdx *int `json:"page_idx"`
				} `json:"pages"`
			} `json:"docvortex_layout"`
		} `json:"extensions"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return 0
	}
	extent := indexedPageExtent(len(parsed.PdfInfo), func(i int) (int, bool) {
		idx := parsed.PdfInfo[i].PageIdx
		if idx == nil || *idx < 0 {
			return 0, false
		}
		return *idx, true
	})
	docVortex := indexedPageExtent(len(parsed.Extensions.DocVortexLayout.Pages), func(i int) (int, bool) {
		idx := parsed.Extensions.DocVortexLayout.Pages[i].PageIdx
		if idx == nil || *idx < 0 {
			return 0, false
		}
		return *idx, true
	})
	return max(extent, docVortex)
}

func minerUStructuredPageExtent(raw []byte) int {
	if len(raw) == 0 {
		return 0
	}
	var doc struct {
		Pages []struct {
			PageIdx *int `json:"page_idx"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return 0
	}
	return indexedPageExtent(len(doc.Pages), func(i int) (int, bool) {
		idx := doc.Pages[i].PageIdx
		if idx == nil || *idx < 0 {
			return 0, false
		}
		return *idx, true
	})
}

func indexedPageExtent(length int, indexAt func(int) (int, bool)) int {
	extent := length
	for i := 0; i < length; i++ {
		idx, ok := indexAt(i)
		if ok && idx+1 > extent {
			extent = idx + 1
		}
	}
	return extent
}

func minerUContentListPageExtent(raw []byte) int {
	if len(raw) == 0 {
		return 0
	}
	var rawItems []struct {
		PageIdx *int `json:"page_idx"`
	}
	if err := json.Unmarshal(raw, &rawItems); err != nil {
		return 0
	}
	extent := 0
	for _, item := range rawItems {
		if item.PageIdx != nil && *item.PageIdx >= 0 && *item.PageIdx+1 > extent {
			extent = *item.PageIdx + 1
		}
	}
	return extent
}

func minerUHighestItemPage(items []map[string]any) int {
	highest := 0
	for _, item := range items {
		if page := int(numberValue(item["page_number"])); page > highest {
			highest = page
		}
	}
	return highest
}

func minerUItemsFromContentList(raw []byte, sizes map[int][2]float64) []map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var rawItems []map[string]any
	if err := json.Unmarshal(raw, &rawItems); err != nil {
		return nil
	}
	return minerUConvertContentItems(rawItems, sizes)
}

func minerUItemsFromStructured(raw []byte, sizes map[int][2]float64) []map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var doc struct {
		Pages []struct {
			PageIdx *int             `json:"page_idx"`
			Blocks  []map[string]any `json:"blocks"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	rawItems := make([]map[string]any, 0)
	for i, page := range doc.Pages {
		pageIdx := i
		if page.PageIdx != nil && *page.PageIdx >= 0 {
			pageIdx = *page.PageIdx
		}
		for _, block := range page.Blocks {
			if item := minerUStructuredBlock(pageIdx, block); item != nil {
				rawItems = append(rawItems, item)
			}
		}
	}
	return minerUConvertContentItems(rawItems, sizes)
}

func minerUStructuredBlock(pageIdx int, block map[string]any) map[string]any {
	typ := strings.ToLower(strings.TrimSpace(stringValue(block["type"])))
	if minerUSkippedBlock(typ) {
		return nil
	}
	content := stringValue(block["content"])
	captions := minerUAnnotationTexts(block["captions"])
	footnotes := minerUAnnotationTexts(block["footnotes"])
	item := map[string]any{"page_idx": pageIdx}
	if bbox := minerUScaleBox(block["bbox"]); len(bbox) > 0 {
		item["bbox"] = bbox
	}
	switch typ {
	case "doc_title", "paragraph_title":
		item["type"] = "title"
		item["text"] = content
	case "table":
		item["type"] = "table"
		item["table_body"] = content
		item["table_caption"] = captions
		item["table_footnote"] = footnotes
	case "image":
		item["type"] = "image"
		item["image_caption"] = captions
		item["image_footnote"] = footnotes
		if strings.TrimSpace(content) != "" {
			item["vlm_description"] = content
		}
	case "chart":
		item["type"] = "chart"
		item["chart_caption"] = captions
		item["chart_footnote"] = footnotes
		if strings.TrimSpace(content) != "" {
			item["vlm_description"] = content
		}
	case "list":
		item["type"] = "list"
		if content != "" {
			item["list_items"] = []any{content}
		}
	case "code":
		item["type"] = "code"
		item["code_body"] = content
		item["code_caption"] = captions
		item["code_footnote"] = footnotes
	default:
		item["type"] = "text"
		if typ != "" {
			item["type"] = typ
		}
		item["text"] = content
	}
	return item
}

func minerUConvertContentItems(rawItems []map[string]any, sizes map[int][2]float64) []map[string]any {
	out := make([]map[string]any, 0, len(rawItems))
	for _, raw := range rawItems {
		if item := minerUContentItem(raw, sizes); item != nil {
			out = append(out, item)
		}
	}
	return out
}

func minerUContentItem(item map[string]any, sizes map[int][2]float64) map[string]any {
	typ := strings.ToLower(strings.TrimSpace(stringValue(item["type"])))
	if minerUSkippedBlock(typ) {
		return nil
	}
	text := minerUItemText(item, typ)
	docType := "text"
	layout := "text"
	headingLevel := 0
	switch typ {
	case "title":
		layout = "title"
	case "text":
		if lvl, ok := minerUInt(item["text_level"]); ok && lvl > 0 {
			layout = "title"
			headingLevel = lvl
		}
	case "list":
		layout = "list"
	case "code":
		layout = "code"
	case "equation":
		layout = "equation"
	case "image", "chart":
		docType = "image"
		layout = "figure"
	case "table":
		docType = "table"
		layout = "table"
	}
	pageIdx, hasPage := minerUInt(item["page_idx"])
	positions := minerUItemPositions(pageIdx, hasPage, item["bbox"], sizes)
	if text == "" && len(positions) == 0 {
		return nil
	}
	if (typ == "table" || typ == "text" || typ == "title" || typ == "list" || typ == "code" || typ == "equation" || typ == "") && text == "" {
		return nil
	}
	jsonItem := map[string]any{
		"text":         text,
		"doc_type_kwd": docType,
		"layout":       layout,
	}
	if hasPage && pageIdx >= 0 {
		jsonItem["page_number"] = pageIdx + 1
	}
	if headingLevel > 0 {
		jsonItem["heading_level"] = headingLevel
	}
	if len(positions) > 0 {
		jsonItem["positions"] = positions
		jsonItem["_pdf_positions"] = positions
	}
	if docType == "table" {
		projectPDFTable(jsonItem)
	}
	return jsonItem
}

func minerUSkippedBlock(typ string) bool {
	switch typ {
	case "header", "footer", "page_number", "page-header", "page-footer", "page_header", "page_footer", "aside_text", "discarded":
		return true
	default:
		return false
	}
}

func minerUItemText(item map[string]any, typ string) string {
	var parts []string
	switch typ {
	case "table":
		parts = append(parts, stringValue(item["table_body"]))
		parts = append(parts, minerUStringList(item["table_caption"])...)
		parts = append(parts, minerUStringList(item["table_footnote"])...)
	case "image":
		parts = append(parts, minerUStringList(item["image_caption"])...)
		parts = append(parts, minerUStringList(item["image_footnote"])...)
		parts = append(parts, stringValue(item["vlm_description"]))
	case "chart":
		parts = append(parts, minerUStringList(item["chart_caption"])...)
		parts = append(parts, minerUStringList(item["chart_footnote"])...)
		parts = append(parts, stringValue(item["vlm_description"]))
	case "list":
		parts = append(parts, minerUStringList(item["list_items"])...)
	case "code":
		if body := stringValue(item["code_body"]); body != "" {
			parts = append(parts, body)
		} else {
			parts = append(parts, stringValue(item["text"]))
		}
		parts = append(parts, minerUStringList(item["code_caption"])...)
		parts = append(parts, minerUStringList(item["code_footnote"])...)
	default:
		parts = append(parts, stringValue(item["text"]))
	}
	return strings.TrimSpace(strings.Join(compactStrings(parts), "\n"))
}

func minerUItemPositions(pageIdx int, hasPage bool, bboxRaw any, sizes map[int][2]float64) [][]any {
	if !hasPage || pageIdx < 0 {
		return nil
	}
	bbox := minerUFloatSlice(bboxRaw)
	if len(bbox) < 4 {
		return nil
	}
	size, ok := sizes[pageIdx]
	if !ok || size[0] <= 0 || size[1] <= 0 {
		return nil
	}
	x0, top, x1, bottom := bbox[0], bbox[1], bbox[2], bbox[3]
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if top > bottom {
		top, bottom = bottom, top
	}
	left := minerUClamp(x0*size[0]/minerUBoxScale, 0, size[0])
	right := minerUClamp(x1*size[0]/minerUBoxScale, 0, size[0])
	topPt := minerUClamp(top*size[1]/minerUBoxScale, 0, size[1])
	bottomPt := minerUClamp(bottom*size[1]/minerUBoxScale, 0, size[1])
	if right <= left || bottomPt <= topPt {
		return nil
	}
	return [][]any{{pageIdx + 1, left, right, topPt, bottomPt}}
}

func minerUScaleBox(raw any) []any {
	bbox := minerUFloatSlice(raw)
	if len(bbox) < 4 {
		return nil
	}
	maxValue := bbox[0]
	for _, value := range bbox[1:] {
		if value > maxValue {
			maxValue = value
		}
	}
	if maxValue <= 1 {
		for i, value := range bbox {
			bbox[i] = value * minerUBoxScale
		}
	}
	out := make([]any, len(bbox))
	for i, value := range bbox {
		out[i] = value
	}
	return out
}

func minerUAnnotationTexts(raw any) []any {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]any, 0, len(items))
	for _, item := range items {
		switch value := item.(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				out = append(out, strings.TrimSpace(value))
			}
		case map[string]any:
			if text := strings.TrimSpace(stringValue(value["content"])); text != "" {
				out = append(out, text)
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func minerUStringList(raw any) []string {
	switch values := raw.(type) {
	case []string:
		return values
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if text := strings.TrimSpace(stringValue(value)); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func compactStrings(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			out = append(out, part)
		}
	}
	return out
}

func minerUInt(raw any) (int, bool) {
	switch value := raw.(type) {
	case int:
		return value, true
	case int64:
		return int(value), true
	case float64:
		return int(value), true
	case json.Number:
		n, err := value.Int64()
		return int(n), err == nil
	default:
		return 0, false
	}
}

func minerUFloatSlice(raw any) []float64 {
	switch values := raw.(type) {
	case []float64:
		return values
	case []any:
		out := make([]float64, 0, len(values))
		for _, value := range values {
			switch n := value.(type) {
			case float64:
				out = append(out, n)
			case int:
				out = append(out, float64(n))
			case int64:
				out = append(out, float64(n))
			case json.Number:
				f, err := n.Float64()
				if err != nil {
					return nil
				}
				out = append(out, f)
			default:
				return nil
			}
		}
		return out
	default:
		return nil
	}
}

func minerUClamp(value, low, high float64) float64 {
	return math.Max(low, math.Min(high, value))
}
