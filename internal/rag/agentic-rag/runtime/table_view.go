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
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// Render HTML tables into an LLM-friendly view: ONE JSON OBJECT PER ROW.
//
// The compiled chunks of a Wikipedia-sourced KB carry machine-generated HTML tables.
// Handing them over raw is both expensive and hard to read: a person infobox is 1-2K
// chars of <table>/<td> markup, and a question like "how many children did all ten
// nominees have" needs ten of them at once. The 2026-09-16 FRAMES attribution showed
// the values ARE in the KB while the model still answers "cannot be determined" — the
// numbers were never surfaced in a form it could aggregate.
//
// ONE shape, whatever the table, and the shape carries FIELD NAMES:
//
//	{"columns": ["Rank", "Rider", "Points"]}           — a table's header, carried once
//	{"Children": "3"}                                 — an infobox row
//	{"Rank": "19", "Rider": "Danilo", "Points": "62"}  — a ranked table's row
//
// A row is a set of field/value pairs, so it is rendered as one; a "key: value" line would have
// put the FIRST CELL in the key slot and demoted the real column names into value text. Because
// one row is still one line, one line-level cutter serves every chunk (see deliverable) while
// still being able to match a question against a FIELD rather than against a string of words.
//
// Values are kept VERBATIM, entities/whitespace are collapsed, units rows are dropped, a
// single-cell row stays plain text (it is not a field), and the text OUTSIDE the tables is kept in
// document order — so a chunk's metadata lines (Title/Source) survive rendering and stay citable.
//
// The raw HTML is never destroyed by this file: callers keep the original chunk in the
// evidence pool (for citation) and use the view only for the model-visible text.

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

// renderTables replaces every HTML table in text with the FIELD view — one JSON object per row
// (`{"Children": "3"}`; the object's keys are the table's columns) — KEEPING the text that lives
// outside the tables in document order.
//
// ok is false when text holds no table at all, so callers keep their previous behaviour
// untouched. A parse failure falls back to ok=false; this never panics and never returns a
// partial view. A table that renders to nothing (no readable row) contributes nothing and
// its surrounding text still comes back — a chunk whose metadata lines precede its table
// keeps those lines, which is what makes the Source URL still citable after rendering.
func renderTables(text string) (string, bool) {
	if text == "" || !strings.Contains(strings.ToLower(text), "<table") {
		return "", false
	}
	doc, err := html.Parse(strings.NewReader(text))
	if err != nil {
		return "", false
	}
	var parts []string
	var buf strings.Builder
	// flush pushes the text read since the last table as its own lines: line structure is
	// what the table view also uses, so the two read as one document.
	flush := func() {
		for _, line := range strings.Split(buf.String(), "\n") {
			if line = strings.TrimSpace(FlattenLine(line)); line != "" {
				parts = append(parts, line)
			}
		}
		buf.Reset()
	}
	// The views come FIRST and the surrounding text after them: a chunk that carries a table
	// carries its FACTS there (an infobox's rows, a standings table's rows), while the text
	// around it is the header (Title/Source/Prepared-Format) and the section that follows. A
	// line-level cutter spends a bounded budget (see deliverable), and with the header in
	// front the budget went to "Title: …" and "Source: …" while the field the question asked
	// for was cut — measured 2026-09-22 on a ten-member question, every member's preview led
	// with its metadata lines and carried no fact at all.
	var views []string
	var tail []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "table" {
			flush()
			tail = append(tail, parts...)
			parts = parts[:0]
			if view := renderOneTable(n); view != "" {
				if len(views) > 0 {
					views = append(views, "")
				}
				views = append(views, view)
			}
			// The table's markup is consumed by its view; its descendants are not walked
			// again, so a nested table is rendered once, inside its parent's view.
			return
		}
		if n.Type == html.TextNode {
			buf.WriteString(n.Data)
			buf.WriteString("\n")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for c := doc.FirstChild; c != nil; c = c.NextSibling {
		walk(c)
	}
	flush()
	tail = append(tail, parts...)
	parts = append(views, tail...)
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, "\n"), true
}

// tableViewOrRaw is renderTables when it produces something, else the text unchanged.
//
// Small convenience for the call sites that only want "the best available
// model-visible form of this chunk". A view that comes back blank (a table with no readable
// row, and no text around it) falls back to the raw text rather than handing the caller an
// empty string.
func tableViewOrRaw(text string) string {
	if view, ok := renderTables(text); ok {
		if strings.TrimSpace(view) != "" {
			return view
		}
	}
	return text
}

// renderOneTable renders a single <table> as one JSON object per ROW, or "" when it holds no
// readable row.
//
// Why objects and not "key: value" lines: a table row IS a set of field/value pairs, and the FIELD
// NAME is what a reader — and the line-level cutter (see deliverable) — matches a question against.
// Flattening a row into "Rank: Rider | Points" put the FIRST CELL in the key slot and demoted the
// real column names into value text, so a question about "Points" had no field to match.
//
// Shapes:
//
//	{"caption": "Final standings"}                      — when the table carries one
//	{"columns": ["Rank", "Rider", "Points"]}            — the header row, carried ONCE
//	{"Rank": "19", "Rider": "Danilo", "Points": "62"}   — a data row, keyed by its column names
//	{"Children": "3"}                                   — two cells, no header: the first cell names
//	                                                      the field (an infobox row)
//	{"key": "Alice", "value": "90 | A"}                 — no header, not two cells: the first cell
//	                                                      names the row, the rest are its value
//	Brendan Fraser                                      — a single-cell row is text, not a field
//
// Order is preserved (rank order IS the data of a ranked table) and every object keeps its keys in
// column order (see jsonObject), never sorted.
//
// A table with fewer than two rows renders to nothing: one line carries no relation, and the
// caller then keeps its raw text (see renderTables).
func renderOneTable(table *html.Node) string {
	rows := tableRowNodes(table)
	if len(rows) < 2 {
		return ""
	}
	// The header, when the table has one, is carried once as the column names the data rows are
	// keyed by. Two conditions, both needed: every cell of the row is a <th>, AND the row has at
	// least two cells. A one-cell all-<th> row is a Wikipedia infobox's NAME (e.g. a colspan=2
	// "Brendan Fraser" heading), not a list of columns — taking it for a header renamed the
	// member to a column and dropped the name from the view (measured 2026-09-22, the preview led
	// with {"columns": ["Brendan Fraser"]} and the row that belongs to it was gone).
	var columns []string
	if rows[0].isHeader {
		if first := nonEmptyCells(rows[0].cells); len(first) >= 2 {
			columns = first
		}
	}
	headerCarried := len(columns) > 0
	lines := make([]string, 0, min(len(rows), tableViewMaxRows))
	omitted := 0
	for i, r := range rows {
		if i == 0 && headerCarried {
			continue
		}
		// A units/header-repeat row carries no data (see tableViewUnitTokens).
		if isUnitsRow(r.cells) {
			continue
		}
		if len(lines) >= tableViewMaxRows {
			omitted++
			continue
		}
		if line := tableRowObject(r.cells, columns); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	out := make([]string, 0, len(lines)+2)
	if caption := tableCaption(table); caption != "" {
		out = append(out, jsonObject([][2]string{{"caption", caption}}))
	}
	if len(columns) > 0 {
		out = append(out, jsonStringArrayObject("columns", columns))
	}
	out = append(out, lines...)
	if omitted > 0 {
		out = append(out, fmt.Sprintf("... (%d more row(s) omitted)", omitted))
	}
	return TruncateRunes(strings.Join(out, "\n"), tableViewMaxCharsPerTable)
}

// tableRowObject renders one row as its JSON object — or as plain text when the row is a single
// cell, because a title/caption row is not a field. See renderOneTable for the shapes.
func tableRowObject(cells []string, columns []string) string {
	clean := nonEmptyCells(cells)
	switch {
	case len(clean) == 0:
		return ""
	case len(clean) == 1:
		return clean[0]
	case len(columns) > 0 && len(clean) == len(columns):
		pairs := make([][2]string, 0, len(clean))
		for i, c := range clean {
			pairs = append(pairs, [2]string{columns[i], c})
		}
		return jsonObject(pairs)
	case len(clean) == 2:
		// No header, two cells: the first cell names the field (an infobox row).
		return jsonObject([][2]string{{clean[0], clean[1]}})
	default:
		// No header to key by: the first cell names the row, the rest are its value.
		return jsonObject([][2]string{{clean[0], strings.Join(clean[1:], " | ")}})
	}
}

// nonEmptyCells trims cells and drops the empty ones, keeping order.
func nonEmptyCells(cells []string) []string {
	out := make([]string, 0, len(cells))
	for _, c := range cells {
		if c = strings.TrimSpace(c); c != "" {
			out = append(out, c)
		}
	}
	return out
}

// jsonObject renders {key: value, …} with the pairs in the order given and every string escaped by
// encoding/json.
//
// Written by hand rather than through a map because a Go map marshals its keys SORTED, which would
// reorder a table's columns ("Points" before "Rank") and break the one property the objects must
// keep: the column order of the row they came from.
func jsonObject(pairs [][2]string) string {
	var b strings.Builder
	b.WriteByte('{')
	for i, p := range pairs {
		if i > 0 {
			b.WriteString(", ")
		}
		key, _ := json.Marshal(p[0])
		value, _ := json.Marshal(p[1])
		b.Write(key)
		b.WriteString(": ")
		b.Write(value)
	}
	b.WriteByte('}')
	return b.String()
}

// jsonStringArrayObject renders {"<key>": ["a", "b"]}.
func jsonStringArrayObject(key string, values []string) string {
	k, _ := json.Marshal(key)
	v, _ := json.Marshal(values)
	return "{" + string(k) + ": " + string(v) + "}"
}

// tableRowNode is one <tr>: its cells, and whether EVERY cell is a <th> — the signal that tells a
// header row from a data row. A Wikipedia infobox row is <th>Born</th><td>1968</td>, i.e. MIXED,
// so it is data whose first cell names the field, never a header.
type tableRowNode struct {
	cells    []string
	isHeader bool
}

// tableRowNodes returns the non-empty rows of table, in document order.
func tableRowNodes(table *html.Node) []tableRowNode {
	var rows []tableRowNode
	for _, tr := range descendants(table, "tr") {
		cells := descendants(tr, "td", "th")
		if len(cells) == 0 {
			continue
		}
		node := tableRowNode{isHeader: true}
		hasText := false
		for _, cell := range cells {
			v := cellText(cell)
			if v != "" {
				hasText = true
			}
			if cell.Data != "th" {
				node.isHeader = false
			}
			node.cells = append(node.cells, v)
		}
		if !hasText {
			continue
		}
		rows = append(rows, node)
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
	return FlattenLine(strings.Join(parts, " "))
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

// mdCell escaped a value for a Markdown pipe cell; it went with the pipe shape (see
// renderOneTable) — a JSON object escapes its strings through encoding/json instead.
