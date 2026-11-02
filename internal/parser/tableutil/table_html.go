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
	"strconv"
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
//
// Unlike the legacy extractor, ParseTableHTML now honours colspan/rowspan:
// a merged cell keeps its text on the origin slot (ColSpan/RowSpan > 1) and
// the engulfed positions become Covered markers. This recovers merge geometry
// that the legacy code discarded. For tables without merges the resulting grid
// is identical to the legacy Rows, so CellTexts (the consumer view) is
// unchanged.
func ParseTableHTML(s string) (*entity.TableData, error) {
	lower := strings.ToLower(s)
	if strings.Contains(lower, "<tr") && !strings.Contains(lower, "<table") {
		s = "<table>" + s + "</table>"
	}
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		return nil, err
	}
	var rows [][]CellInput
	var headerFlags []bool
	var caption string
	// A <tfoot> shows below the body wherever the markup puts it, and HTML 4
	// required it before <tbody>, so its rows are collected apart and placed
	// last; read in source order, a "Total" row came before the data it sums.
	var footRows [][]CellInput
	var footFlags []bool
	inFoot := false
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && isInertElement(n.Data) {
			return
		}
		if isHTMLElement(n, "caption") {
			caption = cellText(n)
			return
		}
		// A table's footer goes below that table's own body, not below every table
		// the fragment holds
		if isHTMLElement(n, "table") && !inFoot {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
			rows = append(rows, footRows...)
			headerFlags = append(headerFlags, footFlags...)
			footRows, footFlags = nil, nil
			return
		}
		if isHTMLElement(n, "tfoot") && !inFoot {
			inFoot = true
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
			inFoot = false
			return
		}
		if isHTMLElement(n, "tr") {
			var cells []CellInput
			anyTh := false
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if isHTMLElement(c, "td") || isHTMLElement(c, "th") {
					if isHTMLElement(c, "th") {
						anyTh = true
					}
					cells = append(cells, CellInput{
						Text:    cellText(c),
						ColSpan: attrInt(c, "colspan", maxColSpan),
						RowSpan: attrInt(c, "rowspan", maxRowSpan),
					})
				}
			}
			if inFoot {
				footRows = append(footRows, cells)
				footFlags = append(footFlags, anyTh)
				return
			}
			rows = append(rows, cells)
			headerFlags = append(headerFlags, anyTh)
			// Do not recurse into the row's cells: a nested <table> inside a
			// cell is folded into that cell's text by cellText, not parsed as
			// a separate row.
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	rows = append(rows, footRows...)
	headerFlags = append(headerFlags, footFlags...)
	return BuildTableData(rows, headerFlags, caption), nil
}

// BuildTableData assembles the canonical TableData from collected cell rows,
// per-row header flags and a caption. It owns the header-row count (leading
// consecutive run, defaulting to 1 when the table has any rows) and the shared
// grid normalization (buildGrid). Both ParseTableHTML and the deepdoc direct
// converter (ToTableData) route through it, so the two produce identical grids
// by construction rather than by a runtime comparison.
//
// ColSpan/RowSpan inputs are clamped to [1, max] to match ParseTableHTML's
// attribute parser and to keep a hostile or malformed span from driving a
// multi-gigabyte allocation inside buildGrid's width pre-pass.
func BuildTableData(rows [][]CellInput, headerFlags []bool, caption string) *entity.TableData {
	headerCount := 0
	for headerCount < len(headerFlags) && headerFlags[headerCount] {
		headerCount++
	}
	if len(rows) > 0 && headerCount == 0 {
		headerCount = 1
	}
	for r := range rows {
		for i := range rows[r] {
			rows[r][i].ColSpan = clampSpan(rows[r][i].ColSpan, maxColSpan)
			rows[r][i].RowSpan = clampSpan(rows[r][i].RowSpan, maxRowSpan)
		}
	}
	return &entity.TableData{
		Grid:       buildGrid(rows),
		HeaderRows: headerCount,
		Caption:    caption,
	}
}

// clampSpan bounds a span extent to [1, max].
func clampSpan(v, max int) int {
	if v < 1 {
		return 1
	}
	if v > max {
		return max
	}
	return v
}

// CellInput is one cell as collected from a <tr> before grid normalization.
type CellInput struct {
	Text    string
	ColSpan int
	RowSpan int
}

// origin names the slot whose span produced a Covered marker.
type origin struct {
	row, col int
}

// buildGrid normalizes ragged cell rows into a dense grid where every logical
// position appears exactly once. A merged cell (ColSpan/RowSpan > 1) occupies
// its origin slot and the engulfed positions become Covered markers pointing
// back at the origin; overlapping spans clamp rather than double-count. A row
// that is genuinely shorter than the widest row is NOT padded with empty cells
// (a real gap stays a shorter row), but a row can be extended by Covered
// markers produced by an ancestor's rowspan so the grid stays dense. Trailing
// rows that carry no cell at all (neither a real cell nor a Covered marker)
// are trimmed. Spans are clamped to the surviving grid so a stored
// ColSpan/RowSpan never exceeds the positions it actually owns (mirrors
// AnyDoc's GridBuilder).
//
// The grid width is computed by a pre-pass that walks the same placement rules
// the main grid build uses, so it counts columns an ancestor rowspan still
// occupies in a later row. If the width only summed each row's own colspan
// (the legacy computation), an ancestor rowspan would push a later row's cells
// past the width and its trailing real cells would be silently dropped by the
// column guard — both losing data and regressing the legacy Rows text view.
func buildGrid(rows [][]CellInput) [][]entity.TableCell {
	// Pre-pass: discover the columns that ancestor rowspans occupy in each
	// later row (recorded in `covered`) and the true grid width. The placement
	// here mirrors the main loop below exactly so the width matches what the
	// main loop will actually reach.
	covered := make([]map[int]origin, len(rows))
	width := 0
	for r, row := range rows {
		col := 0
		ci := 0
		for {
			if _, ok := covered[r][col]; ok {
				// Ancestor rowspan owns this column here; advance past it.
				col++
			} else if ci < len(row) {
				c := row[ci]
				ci++
				cs := spanOf(c.ColSpan)
				rs := spanOf(c.RowSpan)
				col += cs
				if rs > 1 {
					for dr := 1; dr < rs; dr++ {
						rr := r + dr
						if rr >= len(covered) {
							break
						}
						if covered[rr] == nil {
							covered[rr] = map[int]origin{}
						}
						for dc := 0; dc < cs; dc++ {
							covered[rr][col-1-dc] = origin{r, col - cs}
						}
					}
				}
			} else {
				// No real cell at this column. An ancestor may still own
				// columns further right, so jump to the next covered column
				// instead of stopping; otherwise the row is exhausted.
				next := nextCoveredCol(covered[r], col)
				if next < 0 {
					break
				}
				col = next
			}
			if col > width {
				width = col
			}
		}
	}
	if width == 0 {
		return nil
	}
	grid := make([][]entity.TableCell, len(rows))
	for r, row := range rows {
		// Size this row to its OWN column count, not the whole-table width: a
		// wide row elsewhere must not make every short row allocate megabytes
		// of unused backing capacity. The exact cell count for the row is the
		// sum of each real cell's colspan plus the ancestor rowspan markers
		// already recorded in covered[r] — distinct positions, so the total is
		// exact and append never over-grows the slice.
		realCols := 0
		for _, c := range row {
			realCols += spanOf(c.ColSpan)
		}
		grid[r] = make([]entity.TableCell, 0, realCols+len(covered[r]))
		col := 0
		ci := 0
		for col < width {
			if o, ok := covered[r][col]; ok {
				grid[r] = append(grid[r], entity.TableCell{
					Covered:   true,
					OriginRow: o.row,
					OriginCol: o.col,
				})
				col++
				continue
			}
			if ci >= len(row) {
				// No more real cells in this source row, but an ancestor's
				// rowspan may still own columns further right: keep advancing
				// so those Covered markers are emitted instead of stopping
				// early (which would drop a rowspan continuation and break the
				// dense-grid invariant).
				col++
				continue
			}
			c := row[ci]
			ci++
			cs := spanOf(c.ColSpan)
			rs := spanOf(c.RowSpan)
			grid[r] = append(grid[r], entity.TableCell{
				Text:    c.Text,
				ColSpan: cs,
				RowSpan: rs,
			})
			// Mark the same-row positions this origin engulfs as Covered
			// markers so the grid stays dense and exactly-once (mirrors
			// AnyDoc's GridBuilder, which fills every covered slot). The
			// back-pointer names the origin slot (col), not the engulfed
			// position (col+dc). Ancestor rowspan continuation markers for
			// later rows were already registered in the pre-pass above.
			for dc := 1; dc < cs; dc++ {
				grid[r] = append(grid[r], entity.TableCell{
					Covered:   true,
					OriginRow: r,
					OriginCol: col,
				})
			}
			col += cs
		}
		// Rows are NOT padded to a uniform width: a row that genuinely has
		// fewer cells than the widest row keeps its actual (ragged) length so
		// CellTexts() reproduces the legacy row-major Rows exactly. Only
		// merged cells (colspan/rowspan) introduce Covered markers; a real
		// gap is simply a shorter row, which both the consumer text view and
		// HTML render handle without padding artifacts.
	}
	// Clamp stored spans to the grid so every claimed position is backed.
	for r := range grid {
		w := len(grid[r])
		for c := range grid[r] {
			cell := &grid[r][c]
			if cell.Covered {
				continue
			}
			if cell.RowSpan > len(grid)-r {
				cell.RowSpan = len(grid) - r
			}
			if cell.ColSpan > w-c {
				cell.ColSpan = w - c
			}
		}
	}
	// Trim trailing rows that are completely empty: a row with no real cell
	// and no Covered marker carries no information. A row that only carries
	// Covered markers is a genuine rowspan continuation and MUST stay so the
	// grid stays dense and every origin's RowSpan remains backed by a real
	// row; dropping it would make the claimed span exceed the available rows.
	for len(grid) > 0 && len(grid[len(grid)-1]) == 0 {
		grid = grid[:len(grid)-1]
	}
	return grid
}

// spanOf clamps a span extent to a minimum of 1.
func spanOf(v int) int {
	if v < 1 {
		return 1
	}
	return v
}

// nextCoveredCol returns the smallest column key in m strictly greater than col,
// or -1 if no such key exists. It is used by buildGrid's width pre-pass to jump
// past empty columns to a column still owned by an ancestor rowspan.
func nextCoveredCol(m map[int]origin, col int) int {
	next := -1
	for k := range m {
		if k > col && (next < 0 || k < next) {
			next = k
		}
	}
	return next
}

// maxColSpan and maxRowSpan are the HTML standard's own limits: browsers
// clamp a colspan above 1000 and a rowspan above 65534, so the grid must
// apply the same limits. Without an upper bound a malformed or hostile
// attribute (e.g. colspan="100000000") would reach buildGrid and drive a
// multi-gigabyte allocation (row capacity, covered-slot insertion) inside
// the ingestion worker.
const (
	maxColSpan = 1000
	maxRowSpan = 65534
)

// attrInt reads a non-negative integer HTML attribute, clamping 0, negative,
// non-numeric, and above-max values (to 1 and max respectively) so a
// malformed colspan/rowspan never produces a huge or negative grid.
func attrInt(n *html.Node, name string, max int) int {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, name) {
			v, err := strconv.Atoi(strings.TrimSpace(a.Val))
			if err != nil || v < 1 {
				return 1
			}
			if v > max {
				return max
			}
			return v
		}
	}
	return 1
}

// RenderTableHTML emits a deterministic <table> markup for a TableData. It is
// the persisted display form of a table chunk: the chunker renders each table
// chunk's Text with it, and the chunk id is a hash of that rendered text. For
// merged tables it emits colspan/rowspan on the origin slot and skips Covered
// slots.
func RenderTableHTML(td *entity.TableData) string {
	if td == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("<table>")
	if td.Caption != "" {
		b.WriteString("<caption>")
		b.WriteString(html.EscapeString(td.Caption))
		b.WriteString("</caption>")
	}
	b.WriteString("\n")
	for i, row := range td.Grid {
		isHeader := i < td.HeaderRows
		b.WriteString("<tr>")
		for _, cell := range row {
			if cell.Covered {
				continue
			}
			tag := "td"
			if isHeader {
				tag = "th"
			}
			b.WriteString("<")
			b.WriteString(tag)
			if cell.ColSpan > 1 {
				b.WriteString(" colspan=\"")
				b.WriteString(strconv.Itoa(cell.ColSpan))
				b.WriteString("\"")
			}
			if cell.RowSpan > 1 {
				b.WriteString(" rowspan=\"")
				b.WriteString(strconv.Itoa(cell.RowSpan))
				b.WriteString("\"")
			}
			b.WriteString(">")
			b.WriteString(html.EscapeString(cell.Text))
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
