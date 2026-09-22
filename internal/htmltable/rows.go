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

package htmltable

import (
	"strings"

	"golang.org/x/net/html"
)

// TableRows walks the parsed HTML and returns the <td>/<th> text of every
// <tr>, in document order.
//
// The markup is parsed into a tree rather than matched with a regex because
// this input is not guaranteed to be well formed: seven parsers render table
// items (xlsx, csv, docx, html, pdf, …) and some of that markup originates
// from user-supplied documents. A tree also settles the cases a tag-level
// scan gets wrong: a nested <table> no longer terminates its enclosing row
// early — that row keeps its own cells, with the nested table's text folded
// into the cell holding it — and a row or cell missing its closing tag is
// recovered rather than dropped.
func TableRows(htmlStr string) [][]string {
	rows, _ := TableRowsWithHeader(htmlStr)
	return rows
}

// TableRowsWithHeader returns every row's cell text in document order, plus
// the number of leading header rows. A row counts as a header row when any of
// its cells is a <th>. When no row uses <th> at all the first row is treated
// as the header — the same convention SplitLargeHTMLTable applies when
// replicating headers into sub-tables.
func TableRowsWithHeader(htmlStr string) (rows [][]string, headerCount int) {
	// A <tr> outside a <table> is discarded by the HTML5 "in body" insertion
	// mode, so a bare row fragment would yield nothing. Give the parser the
	// table context it needs instead of dropping the rows silently.
	lower := strings.ToLower(htmlStr)
	if strings.Contains(lower, "<tr") && !strings.Contains(lower, "<table") {
		htmlStr = "<table>" + htmlStr + "</table>"
	}
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return nil, 0
	}
	var headerFlags []bool
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		// An inert subtree is parsed but never rendered. <template> puts its
		// content straight into the ordinary child list (the parser has no
		// separate template-contents field), so without this its rows would
		// be read as rows of the enclosing table.
		if n.Type == html.ElementNode && isInertElement(n.Data) {
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
					cells = append(cells, CellText(c))
				}
			}
			// Return without descending: the cells above already collected
			// the nested table's text, so its rows must not be reported a
			// second time as rows of the enclosing table.
			rows = append(rows, cells)
			headerFlags = append(headerFlags, anyTh)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	for len(headerFlags) > headerCount && headerFlags[headerCount] {
		headerCount++
	}
	if len(rows) > 0 && headerCount == 0 {
		headerCount = 1
	}
	return rows, headerCount
}

// CellText returns the visible text of a table cell. The parser hands text
// nodes over already unescaped, nested markup contributes its text without
// its tags (a nested table's cells are concatenated, not separated), and a
// <br> becomes a newline instead of silently gluing the two halves of the
// cell together.
func CellText(cell *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch {
		case n.Type == html.TextNode:
			sb.WriteString(n.Data)
		case isHTMLElement(n, "br"):
			sb.WriteByte('\n')
		case n.Type == html.ElementNode && isInertElement(n.Data):
			// Stop here rather than descending: the content is parsed but
			// never rendered, so it is not text a reader of the cell sees.
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(cell)
	return strings.TrimSpace(sb.String())
}

// isHTMLElement reports whether n is an element with the given tag name in
// the HTML namespace. Foreign content reuses HTML tag names for unrelated
// elements — an <svg><tr> is not a table row — so matching on the tag name
// alone would read markup that carries no table semantics.
func isHTMLElement(n *html.Node, tag string) bool {
	return n.Type == html.ElementNode && n.Namespace == "" && n.Data == tag
}

// isInertElement reports whether an element's content is inert — parsed, but
// never rendered as visible text. An inline <script> or <style> inside a
// table cell of a user-supplied document would otherwise be embedded into the
// Q&A pair as if it were part of the sentence, and a <template>'s placeholder
// rows would be read as real ones.
func isInertElement(tag string) bool {
	switch tag {
	case "script", "style", "noscript", "template":
		return true
	}
	return false
}
