package table

import (
	"html"
	"sort"
	"strings"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// captionText is one caption box: its text plus the box itself.
type captionText struct {
	text string
	// pos is the caption box. Two things read it: the reading-order sort below,
	// and the target's highlight geometry — merging a caption moves its TEXT
	// into the target's chunk, so its boxes have to move with it, or the UI
	// highlights the figure region only and never the caption line beside it.
	pos []pdf.Position
}

// top is the caption's top edge, used to order multiple captions of one target
// in READING order (top→bottom) before concatenation. Section order is not
// guaranteed to match the PDF layout (e.g. 06's lower caption box precedes the
// upper one in sections), so it is read from the box. A caption with no
// positions sorts last.
func (c captionText) top() float64 {
	if len(c.pos) == 0 {
		return 1e9
	}
	return c.pos[0].Top
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

func MergeCaptions(sections []pdf.Section) []pdf.Section {
	captions := make([]int, 0, 4)
	// Group caption texts by the target section index they attach to, so
	// multiple caption boxes for the SAME table/figure collapse into a single
	// <caption> element. HTML allows only one <caption> per <table>; injecting
	// one per box would emit invalid HTML whose extra <caption>s consumers
	// (browsers, HTML→Markdown converters) silently drop — a real content loss.
	byTarget := make(map[int][]captionText)
	for i, s := range sections {
		// An extracted figure/table box is already a parent candidate, never a
		// caption: Python pops table/figure boxes out of self.boxes BEFORE its
		// caption scan (pdf_parser.py _extract_table_figure), so they are never
		// classified there. Without this skip, CaptionKind's text patterns would
		// classify a figure whose embedded text starts with a caption marker
		// (图1 …, Figure 1: …) as its own caption, and findNearestParent would
		// match it to ITSELF (CollectFigures includes it — center distance 0) —
		// the figure would become its own merge target and be removed with the
		// caption, deleting the image from the output (go_bug
		// figure-self-caption-deleted).
		if s.LayoutType == pdf.LayoutTypeFigure || s.LayoutType == pdf.LayoutTypeTable {
			continue
		}
		captionType := CaptionKind(s)
		if captionType == "" {
			continue
		}
		target := findNearestParent(s, sections, captionType)
		if target >= 0 {
			// Emit the caption inside the target table's HTML as a <caption>
			// element (matching Python's __html_table) and drop the standalone
			// caption section. Retaining the caption text closes the previous
			// content-loss go_bug table-html-emission-format; it is NOT a
			// table-assembly change (cell content/structure are untouched).
			byTarget[target] = append(byTarget[target], captionText{text: s.Text, pos: s.Positions})
			captions = append(captions, i)
			continue
		}
		// No merge target. A FIGURE caption is kept as its own section: a pure
		// image figure has no text section (BoxesToSections skips empty figure
		// boxes), so removing it would drop caption text that Python keeps
		// (07_mixed_content 'Figure 1/2'). A TABLE caption with neither a table
		// nor a figure parent — findNearestParent already fell back to figures,
		// mirroring Python's nearest(tables)/nearest(figures) double search — is
		// a DLA mislabel (e.g. rotate_270's rotated text labeled "table"); keep
		// the historical removal so rotated-page text is not duplicated. Python
		// likewise drops a caption only when BOTH searches fail, so the removal
		// branch now fires only when no parent exists at all.
		if captionType != pdf.LayoutTypeFigure {
			captions = append(captions, i)
		}
	}
	// Inject one combined <caption> per target. Captions of the same table are
	// ordered by top edge (reading order, top→bottom) before concatenation.
	for idx, entries := range byTarget {
		sort.SliceStable(entries, func(i, j int) bool { return entries[i].top() < entries[j].top() })
		texts := make([]string, len(entries))
		for i, e := range entries {
			texts[i] = e.text
		}
		if sections[idx].LayoutType == pdf.LayoutTypeTable {
			injectCaption(&sections[idx], texts)
		} else {
			// Non-table target (figure): keep the historical raw-text
			// concatenation. The <caption> element is table-specific; wrapping a
			// figure section's text in it would emit meaningless HTML in a figure
			// section (which carries an image, not a table). Figure captions are
			// out of this PR's scope, so preserve their pre-existing behavior.
			appendRawCaptions(&sections[idx], texts)
		}
		// Either way the caption's text now lives in this section's chunk, so its
		// boxes have to move with it (see extendCaptionPositions).
		extendCaptionPositions(&sections[idx], entries)
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

// maxCaptionGap is the squared centre distance (PDF points²) beyond which a
// caption is not attached to a candidate parent: 40000 — about 200pt (~7cm).
const maxCaptionGap = 40000.0

// occupiesPage reports whether a section's recorded positions place it on
// `page`, and whether the section carries page metadata at all. Page-local
// coordinates repeat on every page (every page has a caption band at roughly
// the same y), so proximity alone cannot decide which page a caption belongs
// to — both parent searches must consult this.
//
// hasPages==false means the section has NO page metadata (a section built by a
// caller that never set PageNumbers). The two callers treat that differently on
// purpose: findTables rejects it (its pre-existing behavior), while the figure
// search keeps it, because that search is also the fallback target for
// orphaned table captions — rejecting on missing metadata there would delete
// their text.
func occupiesPage(s pdf.Section, page int) (onPage, hasPages bool) {
	for _, p := range s.Positions {
		for _, pn := range p.PageNumbers {
			hasPages = true
			if pn == page {
				onPage = true
			}
		}
	}
	return onPage, hasPages
}

// nearestFigureIn returns the index in `sections` of the figure closest to
// caption by page-local centre distance, or -1 when none is within
// maxCaptionGap.
//
// Candidates are read straight off `sections` and the section index is tracked
// directly. A position box is NOT a figure's identity: page-local coordinates
// repeat on every page, so two figures on different pages can carry the same
// box — the very repetition the page filter exists to resolve — and resolving a
// hit back by box would land on the wrong page's figure.
//
// requirePage restricts candidates to figures that occupy the caption's page.
// Without it the search is page-blind, and the nearest figure by page-local
// centre is usually on a DIFFERENT page. A figure with no page metadata stays a
// candidate in both modes, so a caller that must not lose text (the
// table-caption fallback) cannot be defeated by missing metadata — see
// occupiesPage.
func nearestFigureIn(sections []pdf.Section, caption pdf.Section, requirePage bool) int {
	if len(caption.Positions) == 0 {
		return -1
	}
	capKnown, capPage := false, 0
	if requirePage {
		if pn := caption.Positions[0].PageNumbers; len(pn) > 0 {
			capKnown, capPage = true, pn[0]
		}
	}
	cp := caption.Positions[0]
	ccx := (cp.Left + cp.Right) / 2
	ccy := (cp.Top + cp.Bottom) / 2

	// Candidates are ranked in two tiers so a figure recorded on the caption's
	// own page always beats a page-less one: tier 0 is occupied-by-capPage,
	// tier 1 is "no page metadata". A figure known to sit on a DIFFERENT page is
	// skipped outright. Keeping tier 1 means a figure whose caller never set
	// PageNumbers can still take a caption the table fallback must not drop;
	// ordering the tiers means it cannot outbid a figure we can actually place.
	//
	bestIdx, bestDist, bestTier := -1, 1e9, 2
	for i, t := range sections {
		if t.LayoutType != pdf.LayoutTypeFigure || len(t.Positions) == 0 {
			continue
		}
		tier := 0
		if capKnown {
			switch onPage, hasPages := occupiesPage(t, capPage); {
			case !hasPages:
				tier = 1
			case onPage:
				tier = 0
			default:
				continue
			}
		}
		tp := t.Positions[0]
		cx := (tp.Left + tp.Right) / 2
		cy := (tp.Top + tp.Bottom) / 2
		dist := (cx-ccx)*(cx-ccx) + (cy-ccy)*(cy-ccy)
		if tier < bestTier || (tier == bestTier && dist < bestDist) {
			bestTier, bestDist, bestIdx = tier, dist, i
		}
	}
	if bestIdx < 0 || bestDist >= maxCaptionGap {
		return -1
	}
	return bestIdx
}

// findNearestParent finds the nearest figure (for figure captions) or table
// (for table captions) section by position proximity. captionType is "table"
// or "figure" (from CaptionKind). A table caption first searches tables and,
// when none is in range, falls through to the figure search — mirroring
// Python's nearest(tables)/nearest(figures) double search and its `elif fk`
// fallback (pdf_parser.py _extract_table_figure), so a caption whose table is
// unreachable (wrong page, out of range, or absent) lands on the nearest
// figure instead of being dropped. Returns the index in `sections`, or -1
// when no parent is in range.
func findNearestParent(caption pdf.Section, sections []pdf.Section, captionType string) int {
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
	if captionType == pdf.LayoutTypeTable {
		idx, dist, gapY := findTables(sections, caption)
		// Attach a vertically-adjacent caption even when it is horizontally
		// offset from the (often much wider) table's center. See
		// maxCaptionVGap for the rationale (narrow captions above wide
		// tables, e.g. icbccs '请求参数').
		if idx >= 0 && (dist < maxCaptionGap || gapY <= maxCaptionVGap) {
			return idx
		}
		// No table in range: fall through to the figure search below —
		// Python's `elif fk` — rather than leaving the caller to delete a
		// caption whose text Python preserves on a nearby figure.
	}
	// Two-pass figure search (see nearestFigureIn). Page-local coordinates
	// repeat on every page, so proximity alone picks a figure on the wrong
	// page: Figure 1's caption on page 0, page-local centre (415,474), matched
	// a formula figure on page 13 at (417,480) — dist²=45, ~7pt.
	//
	// A genuine FIGURE caption is restricted to its own page: when no figure
	// shares that page it stays a standalone section, which MergeCaptions keeps
	// for figure captions — no text is lost and the caption is never glued to
	// an unrelated figure. The TABLE-caption fallback must not lose text
	// either, so it retries without the page filter and takes the nearest
	// figure anywhere, mirroring Python's nearest(figures), which has no page
	// scope at all: there, placement is best-effort but preservation is not.
	if idx := nearestFigureIn(sections, caption, true); idx >= 0 {
		return idx
	}
	if captionType == pdf.LayoutTypeTable {
		if idx := nearestFigureIn(sections, caption, false); idx >= 0 {
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
			// A table whose positions carry no page metadata is rejected,
			// matching the pre-existing behavior (see occupiesPage).
			if onPage, _ := occupiesPage(t, capPage); !onPage {
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

// extendCaptionPositions appends the attached captions' boxes to the target's
// own, so the target chunk's highlight geometry covers the caption as well as
// the table or figure region it was merged into. The chunk TEXT already
// contains the caption (appendRawCaptions / injectCaption); without this the
// caption is never highlighted with the parent it belongs to. A table's
// caption box routinely sits a couple of points ABOVE the table box rather
// than overlapping it, so this bites tables too.
//
// Only the pages the target itself occupies are merged. A cross-page
// attachment (the table-caption fallback can land on a figure pages away), or
// a caption box that itself spans pages, must not make the target claim a page
// it does not occupy: Position pages drive the section's page set, its
// render/eviction window and its crop plan. So a caption position is trimmed
// to the pages it shares with the target, and dropped when it shares none. A
// caption with no page metadata at all carries no page claim and is merged
// as-is.
//
// Eligibility is anchored to the target's ORIGINAL pages, so a caption merged
// earlier in the loop cannot widen what a later one is allowed to claim.
//
// Appending (not prepending) keeps Positions[0] as the primary box, which
// reading-order sorting and the proximity searches elsewhere depend on.
func extendCaptionPositions(target *pdf.Section, entries []captionText) {
	base := make([]pdf.Position, len(target.Positions))
	copy(base, target.Positions)
	for _, e := range entries {
		for _, p := range e.pos {
			shared, constrained := sharedPageNumbers(p, base)
			if constrained {
				if len(shared) == 0 {
					continue
				}
				p.PageNumbers = shared
			}
			target.Positions = append(target.Positions, p)
		}
	}
}

// sharedPageNumbers returns the page numbers p and have have in common.
// constrained is false when p carries no page numbers at all — there is then
// nothing to intersect and the position is safe to merge unchanged.
func sharedPageNumbers(p pdf.Position, have []pdf.Position) (shared []int, constrained bool) {
	if len(p.PageNumbers) == 0 {
		return nil, false
	}
	seen := make(map[int]struct{}, len(p.PageNumbers))
	for _, b := range p.PageNumbers {
		for _, h := range have {
			for _, a := range h.PageNumbers {
				if a == b {
					if _, dup := seen[b]; !dup {
						seen[b] = struct{}{}
						shared = append(shared, b)
					}
				}
			}
		}
	}
	return shared, true
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
