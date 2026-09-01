package table

import (
	"html"
	"sort"
	"strings"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// captionText is a caption box's text plus its top edge, used to order
// multiple captions of one table in READING order (top→bottom) before
// concatenation. Section order is not guaranteed to match the PDF layout
// (e.g. 06's lower caption box precedes the upper one in sections), so the
// top coordinate is carried explicitly.
type captionText struct {
	top  float64
	text string
}

// captionSep returns the separator inserted before a caption whose text is
// being appended to an existing caption string. Python's __html_table
// (deepdoc/vision/table_structure_recognizer.py construct_table) adds a space
// between captions only for ENGLISH documents; for non-English (e.g. CJK) it
// concatenates directly. MergeCaptions is not threaded the document language,
// so we approximate per caption: a caption containing ASCII letters is treated
// as English and gets a space, matching Python for the dominant cases without
// changing the function signature.
func captionSep(text string) string {
	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return " "
		}
	}
	return ""
}

func MergeCaptions(sections []pdf.Section, figures []pdf.Section) []pdf.Section {
	captions := make([]int, 0, 4)
	// Group caption texts by the target section index they attach to, so
	// multiple caption boxes for the SAME table/figure collapse into a single
	// <caption> element. HTML allows only one <caption> per <table>; injecting
	// one per box would emit invalid HTML whose extra <caption>s consumers
	// (browsers, HTML→Markdown converters) silently drop — a real content loss.
	byTarget := make(map[int][]captionText)
	for i, s := range sections {
		captionType := CaptionKind(s)
		if captionType == "" {
			continue
		}
		target := findNearestParent(i, s, sections, figures, captionType)
		if target >= 0 {
			// Emit the caption inside the target table's HTML as a <caption>
			// element (matching Python's __html_table) and drop the standalone
			// caption section. Retaining the caption text closes the previous
			// content-loss go_bug table-html-emission-format; it is NOT a
			// table-assembly change (cell content/structure are untouched).
			top := 1e9
			if len(s.Positions) > 0 {
				top = s.Positions[0].Top
			}
			byTarget[target] = append(byTarget[target], captionText{top: top, text: s.Text})
			captions = append(captions, i)
			continue
		}
		// No merge target. A FIGURE caption is kept as its own section: a pure
		// image figure has no text section (BoxesToSections skips empty figure
		// boxes), so removing it would drop caption text that Python keeps
		// (07_mixed_content 'Figure 1/2'). A TABLE caption without a table
		// section is a DLA mislabel (e.g. rotate_270's rotated text labeled
		// "table") — keep the historical removal so rotated-page text is not
		// duplicated.
		if captionType != pdf.LayoutTypeFigure {
			captions = append(captions, i)
		}
	}
	// Inject one combined <caption> per target. Captions of the same table are
	// ordered by top edge (reading order, top→bottom) before concatenation.
	for idx, entries := range byTarget {
		sort.SliceStable(entries, func(i, j int) bool { return entries[i].top < entries[j].top })
		texts := make([]string, len(entries))
		for i, e := range entries {
			texts[i] = e.text
		}
		if sections[idx].LayoutType == pdf.LayoutTypeTable {
			injectCaption(&sections[idx], texts)
			continue
		}
		// Non-table target (figure): keep the historical raw-text
		// concatenation. The <caption> element is table-specific; wrapping a
		// figure section's text in it would emit meaningless HTML in a figure
		// section (which carries an image, not a table). Figure captions are
		// out of this PR's scope, so preserve their pre-existing behavior.
		appendRawCaptions(&sections[idx], texts)
	}
	// Remove caption sections in reverse order.
	n := len(sections)
	out := make([]pdf.Section, 0, n-len(captions))
	capSet := make(map[int]bool, len(captions))
	for _, idx := range captions {
		capSet[idx] = true
	}
	for i, s := range sections {
		if !capSet[i] {
			out = append(out, s)
		}
	}
	return out
}

// findNearestParent finds the nearest figure (for figure caption) or
// table (for table caption) section by position proximity.
// captionType is "table" or "figure" (from captionKind).
// Returns the index in `sections` (for tables) or a virtual index mapping
// to `figures` (negative offset for figures).
func findNearestParent(captionIdx int, caption pdf.Section, sections []pdf.Section, figures []pdf.Section, captionType string) int {
	find := func(targets []pdf.Section, skipIdx int) (int, float64) {
		bestIdx := -1
		bestDist := 1e9
		for i, t := range targets {
			if i == skipIdx {
				continue // don't match caption to itself
			}
			if len(t.Positions) == 0 || len(caption.Positions) == 0 {
				continue
			}
			tp := t.Positions[0]
			cp := caption.Positions[0]
			// Squared Euclidean distance (Python _extract_table_figure:1196).
			// Caption is typically below. Use center-point distance.
			cx := (tp.Left + tp.Right) / 2
			cy := (tp.Top + tp.Bottom) / 2
			ccx := (cp.Left + cp.Right) / 2
			ccy := (cp.Top + cp.Bottom) / 2
			dist := (cx-ccx)*(cx-ccx) + (cy-ccy)*(cy-ccy)
			if dist < bestDist {
				bestDist = dist
				bestIdx = i
			}
		}
		return bestIdx, bestDist
	}

	const maxCaptionGap = 40000.0 // PDF points (~7cm) — beyond this, don't attach.
	// maxCaptionVGap is the vertical band within which a caption attaches to a
	// table regardless of its horizontal offset. A narrow caption (e.g. a short
	// Chinese label) sitting directly above a much wider table has a large dx to
	// the table's center; dx² alone can exceed maxCaptionGap and wrongly reject
	// the match, dropping a legitimate caption. Vertical adjacency (small gapY)
	// is the primary signal that the caption belongs to that table; dx only
	// discriminates between candidate tables, which findTables already resolves
	// via min-distance. Keep this in line with the vertical tolerance implied by
	// maxCaptionGap when dx≈0 (~200pt).
	const maxCaptionVGap = 200.0
	if captionType == pdf.LayoutTypeFigure && len(figures) > 0 {
		idx, dist := find(figures, -1) // figures don't contain the caption itself
		if idx >= 0 && dist < maxCaptionGap {
			// Match by position coordinates, not PositionTag strings.
			f := figures[idx]
			for i, s := range sections {
				if s.LayoutType != pdf.LayoutTypeFigure || len(s.Positions) == 0 || len(f.Positions) == 0 {
					continue
				}
				sp, fp := s.Positions[0], f.Positions[0]
				if sp.Left == fp.Left && sp.Right == fp.Right &&
					sp.Top == fp.Top && sp.Bottom == fp.Bottom {
					return i
				}
			}
		}
	}
	if captionType == pdf.LayoutTypeTable {
		idx, dist, gapY := findTables(sections, caption)
		// Attach a vertically-adjacent caption even when it is horizontally
		// offset from the (often much wider) table's center. See
		// maxCaptionVGap for the rationale (narrow captions above wide
		// tables, e.g. icbccs '请求参数').
		if idx >= 0 && (dist < maxCaptionGap || gapY <= maxCaptionVGap) {
			return idx
		}
	}
	return -1
}

// findTables returns the nearest section whose LayoutType is table, by
// distance from the caption to the table's NEAREST EDGE (top/bottom) plus
// horizontal center offset. Restricting to table sections (not all sections)
// prevents a caption sitting in the left margin — far from the table's
// horizontal center — from matching a nearer non-table section (e.g. another
// caption) and being wrongly dropped.
//
// The distance is edge-based, NOT center-based: a cross-page table is one tall
// merged section, so its CENTER is far from a caption sitting just above/below
// it — center-distance exceeded maxCaptionGap and the caption was dropped
// (real content loss, e.g. 13's 'Extended Financial Report' / 14's 'Table 1:
// Revenue'). A caption just above the table (gap to its top) or just below
// (gap to its bottom) has a small edge distance and attaches; one that
// vertically overlaps the table has gap 0.
func findTables(sections []pdf.Section, caption pdf.Section) (int, float64, float64) {
	bestIdx := -1
	bestDist := 1e9
	bestGapY := 0.0
	if len(caption.Positions) == 0 {
		return bestIdx, bestDist, bestGapY
	}
	cp := caption.Positions[0]
	ccx := (cp.Left + cp.Right) / 2
	// Page-scope guard: a caption attaches to a table only on its OWN page (a
	// table whose page set includes the caption's page — a genuine cross-page
	// merged table carries multiple Position entries, one per spanned page, so
	// a caption on any of those pages is on-page). A caption on a DIFFERENT
	// page may attach ONLY to such a cross-page merged table when it vertically
	// overlaps the table's Y band (gapY==0) — the cross-page continuation case
	// (13's later-page caption 'Table: Monthly financial summary FY2024' lands
	// inside the merged table's Y band in page-local coordinates). A caption on
	// a different page that merely repeats a single-page table's page-local Y
	// (page-local Y ranges repeat every page) is a FALSE attachment and must be
	// rejected: this was the icbccs bug where a page-3 caption wrongly attached
	// to a page-5 table and was concatenated into the <caption>, duplicating it
	// ("请求参数 请求参数").
	capKnown := len(cp.PageNumbers) > 0
	capPage := 0
	if capKnown {
		capPage = cp.PageNumbers[0]
	}
	for i, t := range sections {
		if t.LayoutType != pdf.LayoutTypeTable || len(t.Positions) == 0 {
			continue
		}
		tp := t.Positions[0]
		// Vertical gap to the table's nearest edge. A caption fully above the
		// table measures the gap to the top; fully below, to the bottom;
		// vertically overlapping the table, the gap is 0.
		gapY := 0.0
		if cp.Bottom <= tp.Top {
			gapY = tp.Top - cp.Bottom
		} else if cp.Top >= tp.Bottom {
			gapY = cp.Top - tp.Bottom
		}
		if capKnown {
			// Page-scope guard: a caption attaches to a table only on a page
			// the table actually occupies. A genuine cross-page MERGED table
			// carries every spanned page in its Position.PageNumbers (set by
			// tableRegionBox/createTableBoxFromItem from the merged TableItem's
			// Positions), so a caption on any of those pages is on-page and
			// attaches. A caption on a DIFFERENT page that merely repeats a
			// single-page table's page-local Y (page-local Y ranges repeat
			// every page) is a FALSE attachment and is rejected — this was the
			// icbccs bug where a page-3 caption wrongly attached to a page-5
			// table and was concatenated into the <caption>, duplicating it
			// ("请求参数 请求参数").
			onPage := false
			for _, pp := range t.Positions {
				for _, pn := range pp.PageNumbers {
					if pn == capPage {
						onPage = true
						break
					}
				}
				if onPage {
					break
				}
			}
			if !onPage {
				continue
			}
		}
		// Horizontal center offset: a caption far to the side ranks worse.
		dx := (tp.Left+tp.Right)/2 - ccx
		if dx < 0 {
			dx = -dx
		}
		dist := gapY*gapY + dx*dx
		if dist < bestDist {
			bestDist = dist
			bestGapY = gapY
			bestIdx = i
		}
	}
	return bestIdx, bestDist, bestGapY
}

// appendRawCaptions concatenates caption texts onto a non-table section
// (figure) as raw text, preserving the historical behavior before the
// <caption> injection existed. The <caption> element is table-specific and
// would be meaningless in a figure section's text.
func appendRawCaptions(target *pdf.Section, captions []string) {
	var b strings.Builder
	for _, c := range captions {
		t := strings.TrimSpace(c)
		if t == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString(captionSep(c))
		}
		b.WriteString(t)
	}
	if b.Len() == 0 {
		return
	}
	if target.Text != "" {
		target.Text += " " + b.String()
	} else {
		target.Text = b.String()
	}
}

// injectCaption concatenates the given caption texts (already grouped per
// target by MergeCaptions) into a SINGLE <caption> element inserted
// immediately after the table's opening <table> tag (matching Python's
// __html_table). This keeps the caption text inside the table HTML instead of
// losing it as a dropped standalone section, and emits valid HTML (one
// <caption> per table) rather than one <caption> per caption box. If the
// target has no <table> tag the <caption> is prepended so the text is at least
// preserved.
func injectCaption(table *pdf.Section, captions []string) {
	var b strings.Builder
	for _, c := range captions {
		t := strings.TrimSpace(c)
		if t == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString(captionSep(c))
		}
		b.WriteString(html.EscapeString(t))
	}
	if b.Len() == 0 {
		return
	}
	escaped := "<caption>" + b.String() + "</caption>"
	if table.Text == "" {
		table.Text = escaped
		return
	}
	const open = "<table>"
	if idx := strings.Index(table.Text, open); idx >= 0 {
		at := idx + len(open)
		table.Text = table.Text[:at] + escaped + table.Text[at:]
		return
	}
	table.Text = escaped + table.Text
}
