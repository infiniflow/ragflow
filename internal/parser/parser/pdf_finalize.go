package parser

import (
	"strings"

	pdfLayout "ragflow/internal/deepdoc/parser/pdf/layout"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

var pdfHeaderFooterLayouts = map[string]struct{}{
	"header":      {},
	"footer":      {},
	"page_number": {},
	"page-header": {},
	"page-footer": {},
	"discarded":   {},
}

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
	filtered := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if onTOCPage(item, pages0) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func onTOCPage(item map[string]any, pages0 map[int]bool) bool {
	if page := int(numberValue(item["page_number"])); page > 0 && pages0[page-1] {
		return true
	}
	for page := range collectPDFPageNumbers(item) {
		if pages0[page-1] {
			return true
		}
	}
	return false
}
