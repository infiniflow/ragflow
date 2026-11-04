// Command mdprobe runs the ingest Markdown parser over a corpus file and reports, item by item,
// what the parser produced - the fastest way to tell a parser-side loss from a downstream one.
//
// The coverage sweep flagged documents whose indexed chunks do not contain passages of the source
// file; on this corpus the missing passages are tables. The parser is the first suspect, so this
// probe answers one question: does the table text survive the parser?
//
// Usage:
//
//	go run ./tools/mdprobe <file.md> [needle ...]
//
// With needles it also prints, per needle, which item contains it (or that none does).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ragflow/internal/parser/parser"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: mdprobe <file.md> [needle ...]")
		os.Exit(2)
	}
	path := os.Args[1]
	needles := os.Args[2:]
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	p, err := parser.NewMarkdownParser(parser.GoMarkdown)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// The pipeline's setups for the markdown family: output_format=json.
	p.ConfigureFromSetup(map[string]any{"output_format": "json"})
	res := p.ParseWithResult(context.Background(), filepath.Base(path), data)

	fmt.Printf("file=%s bytes=%d output_format=%s items=%d\n", filepath.Base(path), len(data), res.OutputFormat, len(res.JSON))
	var total int
	for i, item := range res.JSON {
		text, _ := item["text"].(string)
		total += len(text)
		ck := fmt.Sprint(item["ck_type"])
		docType := fmt.Sprint(item["doc_type_kwd"])
		fmt.Printf("  [%3d] %-7s %-6s %6d chars  %q\n", i, ck, docType, len(text), truncate(text, 90))
	}
	fmt.Printf("parser emitted %d chars total (file %d)\n", total, len(data))

	for _, needle := range needles {
		lower := strings.ToLower(needle)
		found := -1
		for i, item := range res.JSON {
			if text, _ := item["text"].(string); strings.Contains(strings.ToLower(text), lower) {
				found = i
				break
			}
		}
		inFile := strings.Contains(strings.ToLower(string(data)), lower)
		fmt.Printf("needle %-28q in file=%v in parser output=%v (item %d)\n", needle, inFile, found >= 0, found)
	}

	if os.Getenv("MDPROBE_JSON") != "" {
		out, _ := json.MarshalIndent(res.JSON, "", " ")
		fmt.Println(string(out))
	}
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
