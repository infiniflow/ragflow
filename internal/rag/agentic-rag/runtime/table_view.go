//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package runtime

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// Render HTML tables into an LLM-friendly Markdown view.
//
// The compiled chunks of a Wikipedia-sourced KB carry machine-generated HTML tables.
// Handing them over raw is both expensive and hard to read: a person infobox is 1-2K
// chars of <table>/<td> markup, and a question like "how many children did all ten
// nominees have" needs ten of them at once. The 2026-09-16 FRAMES attribution showed
// the values ARE in the KB while the model still answers "cannot be determined" — the
// numbers were never surfaced in a form it could aggregate.
//
// Two shapes, following the established practice for tables in RAG:
//
//   - a two-column table (Wikipedia infobox) -> Markdown-KV lines: "Children: 3";
//   - any other table (ranked list, election results, timeline) -> a Markdown pipe
//     table carrying the header and EVERY row, because rank/order/completeness decide
//     those answers and row-window narrowing silently drops the answer row.
//
// Values are kept VERBATIM, entities/whitespace are collapsed, all-empty rows are
// dropped. The raw HTML is never destroyed by this file: callers keep the original
// chunk in the evidence pool (for citation) and use the view only for the
// model-visible text.

// tableViewMaxRows / tableViewMaxCharsPerTable are generous ceilings. A Wikipedia
// election or standings table can run to 50K+ raw chars across 100+ rows; capping at
// 4K chars / 60 rows silently dropped the answer row for exactly the
// "large / multi-column" tables this renderer exists to serve (e.g. a 14.7K-char
// standings table whose rank-19 row sat at ~62% of the table). The ceiling exists to
// stop ONE pathological table from eating the whole evidence budget, not to truncate
// normal tables — the caller still holds the raw chunk and can re-read it on demand.
const (
	tableViewMaxRows          = 400
	tableViewMaxCharsPerTable = 20000
)

// tableViewUnitTokens are the tokens of a Wikipedia table's "units" row — the row
// right under the header that reads e.g. "No. | % | No. | %". It carries no data and
// only pollutes a Markdown pipe table, so it is dropped before rendering.
var tableViewUnitTokens = map[string]struct{}{
	"": {}, "no.": {}, "no": {}, "n": {}, "%": {}, "#": {},
	"—": {}, "-": {}, "•": {}, "·": {},
}

// RenderTables replaces every HTML table in text with a Markdown view.
//
// ok is false when text holds no renderable table, so callers keep their previous
// behaviour untouched. A parse failure falls back to ok=false; this never panics and
// never returns a partial view.
func RenderTables(text string) (string, bool) {
	if text == "" || !strings.Contains(strings.ToLower(text), "<table") {
		return "", false
	}
	doc, err := html.Parse(strings.NewReader(text))
	if err != nil {
		return "", false
	}
	var views []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "table" {
			if view := renderOneTable(n); view != "" {
				views = append(views, view)
			}
		}
		// Nested tables are visited too: the selector is recursive.
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if len(views) == 0 {
		return "", false
	}
	return strings.Join(views, "\n\n"), true
}

// TableViewOrRaw is RenderTables when it produces something, else the text unchanged.
//
// Small convenience for the call sites that only want "the best available
// model-visible form of this chunk".
func TableViewOrRaw(text string) string {
	if view, ok := RenderTables(text); ok {
		return view
	}
	return text
}

// renderOneTable renders a single <table>, or "" when it holds no renderable row.
func renderOneTable(table *html.Node) string {
	rows := tableRows(table)
	if len(rows) == 0 {
		return ""
	}
	// Infobox: two columns, a real key in the first cell and a value in the second (the
	// photo-caption row has an empty second cell and is dropped here).
	pairs := make([][2]string, 0, len(rows))
	for _, r := range rows {
		if len(r) == 2 && r[0] != "" && r[1] != "" {
			pairs = append(pairs, [2]string{r[0], r[1]})
		}
	}
	if len(pairs) >= 3 {
		limit := min(len(pairs), tableViewMaxRows)
		lines := make([]string, 0, limit)
		for _, p := range pairs[:limit] {
			lines = append(lines, p[0]+": "+p[1])
		}
		return truncateRunes(strings.Join(lines, "\n"), tableViewMaxCharsPerTable)
	}

	width := 0
	for _, r := range rows {
		width = max(width, len(r))
	}
	header := rows[0]
	// Drop header-repeat / units rows so the pipe body starts at real data (otherwise a
	// wide election table leads with a "No. | % | No. | %" junk row).
	body := make([][]string, 0, len(rows))
	for _, r := range rows[1:min(len(rows), tableViewMaxRows+1)] {
		if !isUnitsRow(r) {
			body = append(body, r)
		}
	}
	if len(body) == 0 {
		return ""
	}
	line := func(cells []string) string {
		padded := make([]string, width)
		for i := 0; i < width; i++ {
			if i < len(cells) {
				padded[i] = mdCell(cells[i])
			}
		}
		return "| " + strings.Join(padded, " | ") + " |"
	}

	out := make([]string, 0, len(body)+3)
	if caption := tableCaption(table); caption != "" {
		out = append(out, "**Table: "+caption+"**")
	}
	out = append(out, line(header), "|"+strings.Repeat("---|", width))
	for _, r := range body {
		out = append(out, line(r))
	}
	if omitted := len(rows) - 1 - len(body); omitted > 0 {
		out = append(out, fmt.Sprintf("... (%d more row(s) omitted)", omitted))
	}
	return truncateRunes(strings.Join(out, "\n"), tableViewMaxCharsPerTable)
}

// tableRows returns the non-empty rows of table as lists of cell strings.
func tableRows(table *html.Node) [][]string {
	var rows [][]string
	for _, tr := range descendants(table, "tr") {
		cells := descendants(tr, "td", "th")
		if len(cells) == 0 {
			continue
		}
		vals := make([]string, 0, len(cells))
		nonEmpty := false
		for _, cell := range cells {
			v := cellText(cell)
			if v != "" {
				nonEmpty = true
			}
			vals = append(vals, v)
		}
		if nonEmpty {
			rows = append(rows, vals)
		}
	}
	return rows
}

// tableCaption returns the first <caption>'s text, "" when there is none.
func tableCaption(table *html.Node) string {
	captions := descendants(table, "caption")
	if len(captions) == 0 {
		return ""
	}
	return cellText(captions[0])
}

// cellText is the whitespace-collapsed cell text (tags stripped, entities resolved).
//
// Text pieces are joined with a single space and every whitespace run collapses to one
// space, so a multi-line cell reads as one value. Comments carry no text and are
// skipped.
func cellText(cell *html.Node) string {
	parts := make([]string, 0, 4)
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			if s := strings.TrimSpace(n.Data); s != "" {
				parts = append(parts, s)
			}
			return
		case html.CommentNode, html.DoctypeNode:
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for c := cell.FirstChild; c != nil; c = c.NextSibling {
		walk(c)
	}
	return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
}

// descendants lists the descendant elements named in names, in document order.
func descendants(n *html.Node, names ...string) []*html.Node {
	want := make(map[string]struct{}, len(names))
	for _, name := range names {
		want[name] = struct{}{}
	}
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode {
				if _, ok := want[c.Data]; ok {
					out = append(out, c)
				}
			}
			walk(c)
		}
	}
	walk(n)
	return out
}

// isUnitsRow reports whether every cell of a row is a unit token — a row that carries
// no data.
func isUnitsRow(vals []string) bool {
	if len(vals) == 0 {
		return false
	}
	for _, v := range vals {
		if _, ok := tableViewUnitTokens[strings.ToLower(strings.TrimSpace(v))]; !ok {
			return false
		}
	}
	return true
}

// mdCell escapes a value for a Markdown pipe cell (the delimiter must stay intact).
func mdCell(value string) string {
	return strings.NewReplacer(`\`, `\\`, "|", `\|`).Replace(value)
}
