// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package tableutil converts between table markup and the structured
// entity.TableData contract. It intentionally mirrors the legacy chunker
// extraction in internal/ingestion/component/chunker/html_rows.go so that the
// cells it produces are byte-for-byte what consumers used to read out of the
// HTML <table> text.
package tableutil

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"ragflow/internal/entity"
)

// ParseTableHTML extracts a structured TableData from table markup. It mirrors
// the legacy chunker behaviour (html_rows.go tableRowsWithHeader): each <tr>
// becomes one row, each direct <td>/<th> one cell, a row carrying a <th> cell
// is a header row, <br> becomes a newline, inert elements
// (script/style/noscript/template) are skipped, and a nested table's text is
// folded into the enclosing cell (the nested <tr> does NOT become a row).
func ParseTableHTML(s string) (*entity.TableData, error) {
	lower := strings.ToLower(s)
	if strings.Contains(lower, "<tr") && !strings.Contains(lower, "<table") {
		s = "<table>" + s + "</table>"
	}
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		return nil, err
	}
	var rows [][]string
	var headerFlags []bool
	var caption string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && isInertElement(n.Data) {
			return
		}
		if isHTMLElement(n, "caption") {
			caption = cellText(n)
			return
		}
		if isHTMLElement(n, "tr") {
			var cells []string
			anyTh := false
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if isHTMLElement(c, "td") || isHTMLElement(c, "th") {
					if isHTMLElement(c, "th") {
						anyTh = true
					}
					cells = append(cells, cellText(c))
				}
			}
			rows = append(rows, cells)
			headerFlags = append(headerFlags, anyTh)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	headerCount := 0
	for headerCount < len(headerFlags) && headerFlags[headerCount] {
		headerCount++
	}
	if len(rows) > 0 && headerCount == 0 {
		headerCount = 1
	}
	return &entity.TableData{Rows: rows, HeaderRows: headerCount, Caption: caption}, nil
}

// RenderTableHTML emits a deterministic <table> markup for a TableData. It is
// used only as a test oracle / round-trip helper and is NOT part of the
// persisted contract (producers emit TableData, not HTML). The emitted shape
// matches the legacy spreadsheet producer's renderSpreadsheetTable exactly
// (including the inter-row newlines) so that RenderTableText reproduces the
// legacy htmlTableRE-stripped text byte for byte.
func RenderTableHTML(td *entity.TableData) string {
	var b strings.Builder
	b.WriteString("<table>")
	if td.Caption != "" {
		b.WriteString("<caption>")
		b.WriteString(html.EscapeString(td.Caption))
		b.WriteString("</caption>")
	}
	b.WriteString("\n")
	for i, row := range td.Rows {
		b.WriteString("<tr>")
		isHeader := i < td.HeaderRows
		for _, cell := range row {
			tag := "td"
			if isHeader {
				tag = "th"
			}
			b.WriteString("<")
			b.WriteString(tag)
			b.WriteString(">")
			b.WriteString(html.EscapeString(cell))
			b.WriteString("</")
			b.WriteString(tag)
			b.WriteString(">")
		}
		b.WriteString("</tr>")
		b.WriteString("\n")
	}
	b.WriteString("</table>")
	b.WriteString("\n")
	return b.String()
}

// tableTagRE mirrors the legacy tokenizer stripping pattern
// (htmlTableRE in internal/ingestion/component/tokenizer.go) so that the
// plaintext a chunker writes for an embed-only path matches the legacy
// tag-stripped output exactly.
var tableTagRE = regexp.MustCompile(`</?(table|td|caption|tr|th)( [^<>]{0,12})?>`)

// RenderTableText renders a TableData to plain text with no markup tags, for
// the rare embed path that receives a table without a downstream TableChunker.
// It reproduces the legacy htmlTableRE stripping of RenderTableHTML(td).
func RenderTableText(td *entity.TableData) string {
	h := RenderTableHTML(td)
	h = tableTagRE.ReplaceAllString(h, " ")
	return strings.TrimSpace(h)
}

// cellText returns the visible text of a table cell: text nodes contribute
// verbatim, <br> becomes a newline, inert elements are skipped, and other
// elements are descended. The result is TrimSpace'd.
func cellText(cell *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch {
		case n.Type == html.TextNode:
			sb.WriteString(n.Data)
		case isHTMLElement(n, "br"):
			sb.WriteByte('\n')
		case n.Type == html.ElementNode && isInertElement(n.Data):
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(cell)
	return strings.TrimSpace(sb.String())
}

func isHTMLElement(n *html.Node, tag string) bool {
	return n.Type == html.ElementNode && n.Namespace == "" && n.Data == tag
}

func isInertElement(tag string) bool {
	switch tag {
	case "script", "style", "noscript", "template":
		return true
	}
	return false
}
