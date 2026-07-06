package parser

import (
	"fmt"
	"strings"
)

func pdfFileMeta(filename string, pageCount int) map[string]any {
	if pageCount < 0 {
		pageCount = 0
	}
	return map[string]any{
		"name":       filename,
		"page_count": pageCount,
		"outline":    []map[string]any{},
	}
}

func pdfItemsToResult(filename string, items []map[string]any, outputFormat string, pageCount int) ParseResult {
	if len(items) == 0 {
		items = []map[string]any{{"text": "", "doc_type_kwd": "text"}}
	}
	pageCount = normalizePDFPageCount(pageCount, items)
	switch strings.ToLower(strings.TrimSpace(outputFormat)) {
	case "", "json":
		return ParseResult{
			OutputFormat: "json",
			File:         pdfFileMeta(filename, pageCount),
			JSON:         items,
		}
	case "markdown":
		var b strings.Builder
		for _, item := range items {
			text, _ := item["text"].(string)
			layout, _ := item["layout"].(string)
			if strings.TrimSpace(text) == "" {
				continue
			}
			if layout == "title" && !strings.HasPrefix(strings.TrimSpace(text), "#") {
				b.WriteString("## ")
			}
			b.WriteString(text)
			b.WriteString("\n\n")
		}
		return ParseResult{
			OutputFormat: "markdown",
			File:         pdfFileMeta(filename, pageCount),
			Markdown:     strings.TrimRight(b.String(), "\n"),
		}
	default:
		return ParseResult{Err: fmt.Errorf("parser: unsupported PDF output_format %q", outputFormat)}
	}
}

func normalizePDFPageCount(fallback int, items []map[string]any) int {
	if inferred := inferPDFPageCountFromItems(items); inferred > fallback {
		return inferred
	}
	return fallback
}

func inferPDFPageCountFromItems(items []map[string]any) int {
	pages := map[int]struct{}{}
	for _, item := range items {
		for page := range collectPDFPageNumbers(item) {
			pages[page] = struct{}{}
		}
	}
	return len(pages)
}

func collectPDFPageNumbers(raw any) map[int]struct{} {
	pages := map[int]struct{}{}
	var walk func(any)
	walk = func(node any) {
		switch v := node.(type) {
		case map[string]any:
			for _, key := range []string{"page_number", "page_num", "page_no", "page_index", "page_idx", "page"} {
				if page := int(numberValue(v[key])); page > 0 {
					pages[page] = struct{}{}
				}
			}
			for _, key := range []string{"_pdf_positions", "positions"} {
				if positions, ok := v[key].([][]any); ok {
					for _, pos := range positions {
						if len(pos) > 0 {
							if page := int(numberValue(pos[0])); page > 0 {
								pages[page] = struct{}{}
							}
						}
					}
				}
			}
			for _, child := range v {
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		case []map[string]any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(raw)
	return pages
}
