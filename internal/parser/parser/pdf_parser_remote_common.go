package parser

import (
	"fmt"
	"strings"
)

func pdfFileMeta(filename string) map[string]any {
	return map[string]any{
		"name":       filename,
		"page_count": 0,
		"outline":    []map[string]any{},
	}
}

func pdfItemsToResult(filename string, items []map[string]any, outputFormat string) ParseResult {
	if len(items) == 0 {
		items = []map[string]any{{"text": "", "doc_type_kwd": "text"}}
	}
	switch strings.ToLower(strings.TrimSpace(outputFormat)) {
	case "", "json":
		return ParseResult{
			OutputFormat: "json",
			File:         pdfFileMeta(filename),
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
			File:         pdfFileMeta(filename),
			Markdown:     strings.TrimRight(b.String(), "\n"),
		}
	default:
		return ParseResult{Err: fmt.Errorf("parser: unsupported PDF output_format %q", outputFormat)}
	}
}
