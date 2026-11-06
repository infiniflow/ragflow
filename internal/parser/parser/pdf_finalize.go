package parser

import (
	"strings"
	"unicode/utf8"

	pdfLayout "ragflow/internal/deepdoc/parser/pdf/layout"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

var pdfHeaderFooterLayouts = map[string]struct{}{
	"header":      {},
	"footer":      {},
	"page_number": {},
	"page-header": {},
	"page-footer": {},
	"page_header": {},
	"page_footer": {},
	"discarded":   {},
}

// Mirror DeepDoc TOC guards (layout/toc_header_footer.go) at JSON item granularity.
const (
	pdfTOCMaxProseRunes = 60
	pdfTOCMaxLongBoxes  = 2
	pdfTOCBBodyRunes    = 20
	pdfTOCMaxBodyBoxes  = 2
)

// FinalizePDFResult attaches bookmarks from the source PDF and applies the
// remove_toc / remove_header_footer parser options to JSON items. DeepDoc applies
// both at box level before JSON conversion; remote engines and plain_text do not,
// so this step aligns behavior across parse methods. Idempotent for DeepDoc:
// TOC pages and header/footer regions are already gone from the item stream.
func (p *PDFParser) FinalizePDFResult(data []byte, result ParseResult) ParseResult {
	if result.Err != nil {
		return result
	}
	result = AttachPDFOutlines(result, data)
	if !p.RemoveHeaderFooter && !p.RemoveTOC {
		return result
	}
	if p.RemoveHeaderFooter {
		result.JSON = filterPDFHeaderFooterItems(result.JSON)
	}
	if p.RemoveTOC {
		items := ItemsToMaps(result.JSON)
		outlines := outlinesFromFileMeta(result.File)
		items = removePDFTOCItems(items, outlines)
		if len(items) == 0 {
			items = []map[string]any{{"text": "", "doc_type_kwd": "text"}}
		}
		result.JSON = MapsToItems(items)
	}
	return result
}

func filterPDFHeaderFooterItems(items []DocItem) []DocItem {
	if len(items) == 0 {
		return items
	}
	filtered := make([]DocItem, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		fields := item.ToMap()
		layout, _ := fields["layout"].(string)
		if layout == "" {
			layout, _ = fields["layout_type"].(string)
		}
		layout = strings.ToLower(strings.TrimSpace(layout))
		if _, drop := pdfHeaderFooterLayouts[layout]; drop {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func outlinesFromFileMeta(file map[string]any) []deepdoctype.Outline {
	if file == nil {
		return nil
	}
	raw, ok := file["outline"].([]map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make([]deepdoctype.Outline, 0, len(raw))
	for _, entry := range raw {
		title, _ := entry["title"].(string)
		level := int(numberValue(entry["level"]))
		page := int(numberValue(entry["page_number"]))
		if title == "" && page <= 0 {
			continue
		}
		out = append(out, deepdoctype.Outline{
			Title:      title,
			Level:      level,
			PageNumber: page,
		})
	}
	return out
}

func removePDFTOCItems(items []map[string]any, outlines []deepdoctype.Outline) []map[string]any {
	if len(outlines) > 0 {
		if pages := pdfLayout.TOCPageRangeFromOutlines(outlines); len(pages) > 0 {
			items = dropPDFItemsOnTOCOutlinePages(items, pages)
		}
	}
	docxOutlines := make([]docxOutline, len(outlines))
	for i, o := range outlines {
		docxOutlines[i] = docxOutline{Title: o.Title, Level: o.Level}
	}
	return removeTOCWord(items, docxOutlines, isEnglishItems(items))
}

func dropPDFItemsOnTOCOutlinePages(items []map[string]any, pages0 map[int]bool) []map[string]any {
	if len(pages0) == 0 {
		return items
	}
	perPage := make(map[int][]map[string]any)
	docPages := make(map[int]struct{})
	for _, item := range items {
		pages := collectPDFPageNumbers(item)
		if len(pages) == 0 {
			if page := int(numberValue(item["page_number"])); page > 0 {
				pages = map[int]struct{}{page: {}}
			}
		}
		for page := range pages {
			perPage[page] = append(perPage[page], item)
			docPages[page] = struct{}{}
		}
	}
	droppable0 := make(map[int]bool)
	droppedPages := 0
	for page1 := range docPages {
		page0 := page1 - 1
		if !pages0[page0] {
			continue
		}
		if pdfPageItemsCarryProse(perPage[page1]) {
			continue
		}
		droppable0[page0] = true
		droppedPages++
	}
	if droppedPages == 0 || droppedPages == len(docPages) {
		return items
	}
	filtered := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if onTOCPage(item, droppable0) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func pdfPageItemsCarryProse(pageItems []map[string]any) bool {
	var prose, body int
	for _, item := range pageItems {
		text := itemText(item)
		if text == "" {
			continue
		}
		runes := utf8.RuneCountInString(text)
		if runes > pdfTOCMaxProseRunes {
			prose++
			continue
		}
		if runes > pdfTOCBBodyRunes {
			body++
		}
	}
	return prose > pdfTOCMaxLongBoxes || body > pdfTOCMaxBodyBoxes
}

func onTOCPage(item map[string]any, pages0 map[int]bool) bool {
	pages := collectPDFPageNumbers(item)
	if len(pages) == 0 {
		if page := int(numberValue(item["page_number"])); page > 0 {
			pages = map[int]struct{}{page: {}}
		}
	}
	if len(pages) == 0 {
		return false
	}
	for page := range pages {
		if !pages0[page-1] {
			return false
		}
	}
	return true
}
