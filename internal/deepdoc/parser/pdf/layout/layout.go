package layout

import (
	"log/slog"
	"math"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ---- Column assignment ----
//
// AssignColumn is implemented in combined_column.go (gap + KMeans hybrid).

// ---- Text merge (horizontal) ----

// DedupIdenticalText collapses boxes whose text is byte-identical on the SAME
// page AND whose Y bands are pairwise DISJOINT, keeping the first
// (reading-order) occurrence. OCR repeatedly detects the same text at disjoint
// Y positions (09_crosspage_paragraph detects each paragraph 14-18x per page);
// Python's downstream merge collapses these, so Go must too or the replay
// output duplicates every paragraph. Boxes that OVERLAP in Y are kept: multiple
// columns / adjacent lines on one page legitimately share text (e.g.
// eval_three_wide has 3 columns at the same Y). Different pages keep their own
// copies (a cross-page paragraph appears once per page).
//
// A same-text group is treated as a pseudo-duplicate only when it forms a
// REGULAR rolling-stride CHAIN of at least pseudoDupChainMin boxes (same X,
// pairwise disjoint Y, gaps > 4x height). Shorter groups (2-4 copies, e.g. the
// identical template rows of eval_two_wide_gutter / eval_two_indented_first_para)
// are real document content — distinct physical lines that happen to share
// text — and are kept verbatim.
const pseudoDupChainMin = 5

func DedupIdenticalText(boxes []pdf.TextBox) []pdf.TextBox {
	type key struct {
		page int
		text string
	}
	groups := make(map[key][]int, len(boxes))
	for i, b := range boxes {
		// Only OCR boxes are de-duplicated: char-path digital-PDF boxes may
		// legitimately repeat the same text (clauses, headings) and must be
		// kept verbatim — dropping them would silently lose content.
		if !b.IsOCR {
			continue
		}
		t := strings.TrimSpace(b.Text)
		if t == "" {
			continue
		}
		groups[key{b.PageNumber, t}] = append(groups[key{b.PageNumber, t}], i)
	}

	drop := make(map[int]bool, len(boxes))
	for _, idxs := range groups {
		// A pseudo-duplicate needs a rolling-stride CHAIN of at least
		// pseudoDupChainMin detections. Isolated pairs / short groups of
		// identical text are real repeated lines and must be kept (e.g. the
		// eval_two_* template rows detected 2-4x, where dropping any copy
		// loses a real line).
		if len(idxs) < pseudoDupChainMin {
			continue
		}
		// Short identical texts (e.g. the repeated keyword 'Transformer' in
		// 16_dense_cjk) are real document content, not OCR paragraph
		// duplicates — only paragraphs (>= 20 runes) are collapsed.
		if utf8.RuneCountInString(strings.TrimSpace(boxes[idxs[0]].Text)) < 20 {
			continue
		}
		// OCR pseudo-duplicate: the same text detected repeatedly at disjoint
		// Y positions of the SAME X location, separated by MORE than 4x the box
		// height (a rolling-stride re-detection). Adjacent identical rows
		// (~1x height apart, e.g. eval_two_narrow_gutter) and different columns
		// are real content and are kept.
		gapThreshold := 4 * (boxes[idxs[0]].Bottom - boxes[idxs[0]].Top)
		if gapThreshold <= 0 {
			continue
		}
		hasOverlapY := false
		allOverlapX := true
		allGapOK := true
		for a := 0; a < len(idxs) && !hasOverlapY; a++ {
			for c := a + 1; c < len(idxs); c++ {
				ba, bc := boxes[idxs[a]], boxes[idxs[c]]
				if ba.Bottom > bc.Top && bc.Bottom > ba.Top {
					hasOverlapY = true
				}
				if bc.X1 <= ba.X0 || ba.X1 <= bc.X0 {
					allOverlapX = false
				}
				gap := ba.Top - bc.Top
				if gap < 0 {
					gap = -gap
				}
				if gap <= gapThreshold {
					allGapOK = false
				}
			}
		}
		if !hasOverlapY && allOverlapX && allGapOK {
			for _, idx := range idxs[1:] {
				drop[idx] = true
			}
		}
	}

	out := boxes[:0]
	for i, b := range boxes {
		if drop[i] {
			continue
		}
		out = append(out, b)
	}
	return out
}

// DedupSubstringOverlaps collapses a box whose text is a CONTIGUOUS SUBSTRING
// of another same-page box AND is geometrically contained in that box (X AND Y
// inside), keeping the longer box. OCR detects both a full paragraph and its
// middle fragment at the same location (e.g. 01_english_simple: full paragraph
// y=(105,166) plus fragment "language models. When a user asks..." y=(119,132));
// Python drops the fragment, Go must too or the merged paragraph repeats it. A
// substring box at a disjoint Y OR X (different column, e.g.
// eval_two_wide_gutter) is kept — a real repeated heading or another column's
// text is legal. The containment test is bound to the substring box (not just
// the shorter-height box) so a physically taller box whose short text is a
// substring of a neighbour is never silently dropped.
//
// The substring comparison is whitespace-insensitive: ocrMergeChars fills line
// fragments with char-layer text that preserves the PDF's original spaces
// ("- name: SSL_CERT_FILE", "⽂章 中 提到") while the containing paragraph OCR
// box carries the recognizer's joined text ("-name: SSL_CERT_FILE",
// "⽂章中提到"). Byte-wise matching would miss the fragment relation and the
// inner box survives to NaiveVerticalMerge, which concatenates it into the
// paragraph — duplicating text (the ocr_real text gaps: plugin-daemon,
// RAG分词, 三国人物). Geometry (boxInsideTolerant) remains the actual
// containment proof; whitespace normalization only makes the fragment check
// robust to the char-vs-OCR space divergence.
func DedupSubstringOverlaps(boxes []pdf.TextBox) []pdf.TextBox {
	drop := make([]bool, len(boxes))
	// Precompute the whitespace-normalized text once per box. The inner loop
	// compares every OCR pair, so recomputing norm there would make the O(n²)
	// loop O(n²·len) in the worst case (a large scan document hits ~3k boxes).
	norm := make([]string, len(boxes))
	for i, b := range boxes {
		if b.IsOCR {
			norm[i] = dedupNormText(strings.TrimSpace(b.Text))
		}
	}
	for i := range boxes {
		if drop[i] {
			continue
		}
		// Only OCR-vs-OCR pairs are collapsed (see DedupIdenticalText): a
		// char-path box is never treated as a fragment of another.
		if !boxes[i].IsOCR {
			continue
		}
		ai := strings.TrimSpace(boxes[i].Text)
		if ai == "" {
			continue
		}
		for j := range boxes {
			if i == j || drop[j] || boxes[i].PageNumber != boxes[j].PageNumber || !boxes[j].IsOCR {
				continue
			}
			aj := strings.TrimSpace(boxes[j].Text)
			if aj == "" || ai == aj {
				continue
			}
			ni, nj := norm[i], norm[j]
			// Never collapse a substring across columns. OCR double-detection
			// fragments always share the SAME column as their container; a
			// substring in a DIFFERENT column (e.g. the opposite column of a
			// two-column page whose wide OCR box spans the gutter, or a left
			// column short line whose text happens to be a substring of a right
			// column paragraph) is independent document text and must be kept.
			// ColID is assigned by AssignColumn, which now runs before dedup;
			// boxes without column info (ColID==0, the single-column default, or
			// unset) keep the original geometry-only behaviour so legacy tests
			// and single-column docs are unaffected.
			if boxes[i].ColID != boxes[j].ColID {
				continue
			}
			// Collapse only when the SUBSTRING-text box is geometrically CONTAINED
			// in the text-containing box. Binding the geometry to the actual
			// substring (not merely the shorter-height box) avoids silently
			// dropping a physically taller box whose short text happens to be a
			// substring of a neighbour — OCR double-detection fragments are always
			// the smaller, contained box, so the taller container is kept.
			if len(ni) >= len(nj) && strings.Contains(ni, nj) {
				// j's text is a substring of i's -> drop j only if j sits inside i.
				if boxInsideTolerant(boxes[j], boxes[i]) {
					drop[j] = true
				}
			} else if len(nj) > len(ni) && strings.Contains(nj, ni) {
				// i's text is a substring of j's -> drop i only if i sits inside j.
				if boxInsideTolerant(boxes[i], boxes[j]) {
					drop[i] = true
					break
				}
			}
		}
	}
	out := boxes[:0]
	for i, b := range boxes {
		if drop[i] {
			continue
		}
		out = append(out, b)
	}
	return out
}

// dedupNormText strips all whitespace for substring comparison. Spaces are the
// only divergence between the char-derived fragment text and the OCR box text,
// and whitespace carries no content identity here — the containment geometry is
// the real proof (see DedupSubstringOverlaps).
func dedupNormText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if !unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// FilterWatermarkBoxes drops single-character ASCII boxes whose text repeats
// verbatim many times on the SAME page. This targets the "tiled watermark on a
// rotated glyph string" pattern reported in #18145:
//
//   - Resume / template PDFs plant a watermark string (e.g. "ce1a60…ZH1c56f")
//     repeated diagonally across the page. The string is rotated so each
//     glyph fails the layout line-overlap check and ends up in its own
//     single-character box.
//   - Naive chunker then interleaves those garbage boxes into every chunk.
//
// This runs at the POST-PROCESS level (after char→box and OCR→box have both
// converged into the same `[]pdf.TextBox` slice), so it covers both the
// direct-text and OCR-merge code paths that the issue reproduces. An earlier
// attempt to filter at the char-stage in `CharsToBoxes` was a dead branch
// for the OCR-merge path and operated at the wrong granularity (multi-char
// tokens), per the review on PR #18308.
//
// Scope is intentionally tight so legitimate content is never dropped:
//
//   - Only boxes whose Text is exactly one ASCII letter or digit, classified
//     against raw Text (no normalization). This excludes multi-token
//     repetition (e.g. repeated SKU "ABC123"), CJK glyphs (always ≥3 UTF-8
//     bytes → len>1), punctuation, and whitespace.
//   - Promoted only if the box's text appears ≥ watermarkBoxesMinOccurrences
//     times on the page. A real "Q" or "1" appears at most once per page
//     outside a watermark tiling.
//   - Page-local, so a watermark on page 3 cannot affect page 1. This also
//     keeps the O(n) cost bounded by page size.
//
// CJK is unconstrained: a single box's bytes may be ≥1 even when the rune
// count is 1, so `len(box.Text) == 1` already excludes CJK fonts (which are
// typically 3 bytes per glyph). A box that 姓名 — the canonical Chinese name
// field — survives verbatim even when the rest of the document is contaminated.
func FilterWatermarkBoxes(boxes []pdf.TextBox) []pdf.TextBox {
	if len(boxes) == 0 {
		return boxes
	}
	// Pass 1: per-page count of single-char ASCII boxes. Use the raw
	// box.Text for both classification and the promotion key so no
	// normalization collapses distinct bytes.
	type key struct {
		page int
		text string
	}
	counts := make(map[key]int, len(boxes))
	for _, b := range boxes {
		if !isSingleAsciiAlnum(b.Text) {
			continue
		}
		counts[key{b.PageNumber, b.Text}]++
	}
	// Promote any (page, text) that meets the watermark threshold.
	promoted := make(map[key]struct{}, len(counts))
	for k, n := range counts {
		if n >= watermarkBoxesMinOccurrences {
			promoted[k] = struct{}{}
		}
	}
	if len(promoted) == 0 {
		return boxes
	}
	// Pass 2: drop boxes whose (page, text) was promoted.
	out := make([]pdf.TextBox, 0, len(boxes))
	for _, b := range boxes {
		if isSingleAsciiAlnum(b.Text) {
			if _, drop := promoted[key{b.PageNumber, b.Text}]; drop {
				continue
			}
		}
		out = append(out, b)
	}
	return out
}

// watermarkBoxesMinOccurrences is the per-page repetition threshold for a
// single-char ASCII box to be considered a watermark glyph. Legitimate short
// content ("1.", "Q.", "A.") rarely appears more than once per page outside
// of dense content (table rows, list items), and even there the same label
// rarely reaches this threshold. A watermark tile places the same glyph at
// 10+ x-intercepts across the page, so the threshold is set conservatively
// above that floor to keep false-positive rate negligible.
const watermarkBoxesMinOccurrences = 4

// isSingleAsciiAlnum reports whether s is exactly one ASCII letter or digit
// (no whitespace, no CJK, no punctuation, no multibyte UTF-8). Used by
// FilterWatermarkBoxes to limit the watermark signal to the rotated-glyph
// shape rather than the broader "any short token" case, which would
// collide with repeated SKUs and part numbers.
func isSingleAsciiAlnum(s string) bool {
	if len(s) != 1 {
		return false
	}
	c := s[0]
	switch {
	case c >= 'A' && c <= 'Z':
		return true
	case c >= 'a' && c <= 'z':
		return true
	case c >= '0' && c <= '9':
		return true
	}
	return false
}

// dedupYTolerancePt tolerates a small Y-boundary overshoot of an OCR
// double-detection fragment's BOTTOM edge (and the height-match slack for the
// top-overshoot case, see boxInsideTolerant). Such fragments sit on the SAME
// text line as their container but their detected Y bounds jitter by ~0.5-2pt
// of detection noise (observed on Rag Flow Usage / 三国人物); strict Y
// containment then misses them and the duplicate text leaks into the output
// after TextMerge. We only relax Y (never X) and only inside
// DedupSubstringOverlaps, where the text-substring precondition already proves
// the box is an OCR duplicate — so a few points of Y noise must not keep it.
const dedupYTolerancePt = 3.0

// boxInsideTolerant reports whether inner is contained within outer: X is
// strict, inner's bottom may extend up to dedupYTolerancePt past outer's
// bottom, and inner's top may overshoot only when the two boxes are the SAME
// text line (their heights match within dedupYTolerancePt). It confirms an OCR
// substring fragment sits inside the box whose text contains it — not merely
// sharing a Y band at a different column, nor poking out horizontally beyond
// the container. Requiring horizontal containment stops a whitespace-
// normalized substring match from dropping a box that extends past the
// container's X range (e.g. an adjacent-column line whose text happens to be a
// substring of the paragraph's). The top-overshoot height-match requirement is
// what tells a same-line double-detection fragment (Rag Flow Usage's
// "toseeyou again", top 0.5pt above its line) apart from an ADJACENT line
// whose top rises past the container but whose height differs (eval_one_
// indented_block's indented line, top 1pt above a two-line container — kept).
func boxInsideTolerant(inner, outer pdf.TextBox) bool {
	if inner.X0 < outer.X0 || inner.X1 > outer.X1 {
		return false
	}
	if inner.Top < outer.Top-dedupYTolerancePt {
		return false
	}
	if inner.Top < outer.Top {
		// Top overshoot: only tolerable when the fragment is the SAME line as
		// the container — i.e. the two heights match within the tolerance. An
		// adjacent line that pokes above the container is taller/shorter in
		// height, so it is kept.
		if math.Abs((inner.Bottom-inner.Top)-(outer.Bottom-outer.Top)) > dedupYTolerancePt {
			return false
		}
	}
	if inner.Bottom > outer.Bottom+dedupYTolerancePt {
		return false
	}
	return true
}

// TextMerge horizontally merges adjacent boxes at similar vertical positions.
//
// Python: pdf_parser.py:888 _text_merge()
func TextMerge(boxes []pdf.TextBox, medianHeights map[int]float64) []pdf.TextBox {
	if len(boxes) < 2 {
		return boxes
	}
	// Build output via collect: O(n) instead of O(n²) slice-element removal.
	out := make([]pdf.TextBox, 0, len(boxes))
	i := 0
	for i < len(boxes) {
		cur := boxes[i]
		i++
		for i < len(boxes) {
			nxt := boxes[i]
			if cur.PageNumber != nxt.PageNumber || cur.ColID != nxt.ColID {
				break
			}
			// Python: b.get("layoutno", "0") != b_.get("layoutno", "1") —
			// asymmetric defaults mean empty/missing layoutno never merge horizontally.
			if cur.LayoutNo != nxt.LayoutNo || cur.LayoutNo == "" || nxt.LayoutNo == "" ||
				cur.LayoutType == pdf.LayoutTypeTable || cur.LayoutType == pdf.LayoutTypeFigure || cur.LayoutType == pdf.LayoutTypeEquation {
				break
			}
			mh := medianHeights[cur.PageNumber]
			if mh <= 0 {
				mh = 10
			}
			if math.Abs(util.BoxYDis(cur, nxt)) < mh/3 {
				cur.X1 = nxt.X1
				cur.Top = (cur.Top + nxt.Top) / 2
				cur.Bottom = (cur.Bottom + nxt.Bottom) / 2
				cur.Text += nxt.Text
				i++
			} else {
				break
			}
		}
		out = append(out, cur)
	}
	return out
}

// ---- Naive vertical merge ----

// NaiveVerticalMerge vertically merges boxes on the same page/column.
//
// Python: pdf_parser.py:926 _naive_vertical_merge()
func NaiveVerticalMerge(boxes []pdf.TextBox, medianHeights map[int]float64, medianWidths map[int]float64, pageEnglish map[int]bool) []pdf.TextBox {
	if len(boxes) < 2 {
		return boxes
	}

	// Group boxes by page
	pageGroups, sortedPages := groupBoxesByPage(boxes)

	var result []pdf.TextBox
	for _, pg := range sortedPages {
		// Collect all boxes for this page
		indices := pageGroups[pg]
		bxs := make([]pdf.TextBox, len(indices))
		for i, idx := range indices {
			bxs[i] = boxes[idx]
		}

		mh := medianHeights[pg]
		if pageEnglish[pg] {
			// Python: for is_english documents chars are cleared so
			// mean_height becomes 0 and _naive_vertical_merge skips every
			// pair (gap > 0). Mirror that for English pages DIRECTLY — do not
			// fall back to the (positive) char-derived median height, or real
			// English pages would still merge and 'linexxx' rows (eval_*)
			// concatenate into one giant line. The old guard `if mh <= 0`
			// never fired for real pages because their char-derived median
			// height is always positive.
			mh = 0
		} else if mh <= 0 {
			mh = util.MedianHeight(bxs)
		}
		mw := medianWidths[pg]
		if mw <= 0 {
			mw = 8 // Python fallback: np.median([...]) if chars else 8 (pdf_parser.py:1465)
		}

		// Process boxes for this page
		processed := processPageBoxes(bxs, mh, mw, pageEnglish[pg])
		result = append(result, processed...)
	}
	slog.Debug("vm result", "in", len(boxes), "out", len(result))
	return result
}

// ---- Reading order ----

// FinalReadingOrderMerge sorts boxes by page → column → top → x0.
//
// Python: pdf_parser.py:1007 _final_reading_order_merge()
func FinalReadingOrderMerge(boxes []pdf.TextBox) []pdf.TextBox {
	if len(boxes) == 0 {
		return boxes
	}
	sort.Slice(boxes, func(i, j int) bool {
		bi, bj := boxes[i], boxes[j]
		if bi.PageNumber != bj.PageNumber {
			return bi.PageNumber < bj.PageNumber
		}
		if bi.ColID != bj.ColID {
			return bi.ColID < bj.ColID
		}
		if bi.Top != bj.Top {
			return bi.Top < bj.Top
		}
		return bi.X0 < bj.X0
	})
	return boxes
}

var pageNumSuffixPattern = regexp.MustCompile(`[0-9  •一—-]+$`)

// groupBoxesByPage groups text boxes by page, returning a map from page number to index list and sorted page number list
func groupBoxesByPage(boxes []pdf.TextBox) (map[int][]int, []int) {
	if len(boxes) == 0 {
		return map[int][]int{}, []int{}
	}

	pageGroups := make(map[int][]int)
	for i, b := range boxes {
		pageGroups[b.PageNumber] = append(pageGroups[b.PageNumber], i)
	}

	// Sort page numbers
	pageKeys := make([]int, 0, len(pageGroups))
	for pg := range pageGroups {
		pageKeys = append(pageKeys, pg)
	}
	sort.Ints(pageKeys)

	return pageGroups, pageKeys
}

// shouldMergeBoxes determines whether two boxes should be merged
func shouldMergeBoxes(prev, curr *pdf.TextBox, mh, mw float64, isEnglish bool) bool {
	// Check layout number
	if prev.LayoutNo != curr.LayoutNo {
		slog.Debug("vm reject", "reason", "layoutNo", "prevLayout", prev.LayoutNo, "currLayout", curr.LayoutNo)
		return false
	}

	// Check vertical gap
	gap := curr.Top - prev.Bottom
	if gap > mh*1.5 {
		slog.Debug("vm reject", "reason", "gap", "gap", gap, "threshold", mh*1.5, "mh", mh)
		return false
	}

	// Check horizontal overlap
	ov := util.OverlapX(prev, curr)
	if ov < 0.3 {
		slog.Debug("vm reject", "reason", "ovX", "ov", ov, "threshold", 0.3)
		return false
	}

	// Check merge/block conditions
	prevText := strings.TrimSpace(prev.Text)
	currText := strings.TrimSpace(curr.Text)

	concatting := []bool{
		endsWithOneOf(prevText, ",;:\"，、‘“；：-"),
		endsSecondLastOneOf(prevText, ",;:\"，、‘“；："),
		startsWithOneOf(currText, "。；？！?\"）),，、："),
	}
	anti := []bool{
		endsWithOneOf(prevText, "。？！?"),
		isEnglish && endsWithOneOf(prevText, ".!?"),
		prev.PageNumber < curr.PageNumber && math.Abs(prev.X0-curr.X0) > mw*4,
	}
	detach := []bool{prev.X1 < curr.X0, prev.X0 > curr.X1}

	if (slices.Contains(anti, true) && !slices.Contains(concatting, true)) || slices.Contains(detach, true) {
		return false
	}

	return true
}

// mergeTwoBoxes merges two text boxes
func mergeTwoBoxes(prev, curr pdf.TextBox) pdf.TextBox {
	prevText := strings.TrimSpace(prev.Text)
	currText := strings.TrimSpace(curr.Text)

	prev.Text = strings.TrimSpace(strings.TrimRight(prevText, " \t") + " " + strings.TrimLeft(currText, " \t"))
	prev.Bottom = math.Max(prev.Bottom, curr.Bottom)
	prev.X0 = math.Min(prev.X0, curr.X0)
	prev.X1 = math.Max(prev.X1, curr.X1)

	prevTrunc, currTrunc := prevText, currText
	if r := []rune(prevTrunc); len(r) > 40 {
		prevTrunc = string(r[:40])
	}
	if r := []rune(currTrunc); len(r) > 40 {
		currTrunc = string(r[:40])
	}
	slog.Debug("vm merge", "prev", prevTrunc, "curr", currTrunc)

	return prev
}

// processPageBoxes vertically merges the boxes of a single page. Boxes are
// bucketed by column first so merges never cross columns. Titles that precede
// all non-title content and occupy their own column are moved ahead of the
// column groups.
func processPageBoxes(boxes []pdf.TextBox, mh, mw float64, isEnglish bool) []pdf.TextBox {
	if len(boxes) == 0 {
		return boxes
	}

	colGroups, sortedCols := groupBoxesByCol(boxes)

	out := make([]pdf.TextBox, 0, len(boxes))
	for _, col := range sortedCols {
		indices := colGroups[col]
		bxs := make([]pdf.TextBox, len(indices))
		for i, idx := range indices {
			bxs[i] = boxes[idx]
		}
		// Sort within the column by Top, X0.
		sort.Slice(bxs, func(i, j int) bool {
			if bxs[i].Top != bxs[j].Top {
				return bxs[i].Top < bxs[j].Top
			}
			return bxs[i].X0 < bxs[j].X0
		})
		out = append(out, mergeColumnBoxes(bxs, mh, mw, isEnglish)...)
	}
	return moveLeadingTitlesFirst(out)
}

// groupBoxesByCol groups boxes by column id and returns the groups plus the
// column ids in ascending order (leftmost column first).
func groupBoxesByCol(boxes []pdf.TextBox) (map[int][]int, []int) {
	colGroups := make(map[int][]int)
	for i, b := range boxes {
		colGroups[b.ColID] = append(colGroups[b.ColID], i)
	}
	colKeys := make([]int, 0, len(colGroups))
	for c := range colGroups {
		colKeys = append(colKeys, c)
	}
	sort.Ints(colKeys)
	return colGroups, colKeys
}

// mergeColumnBoxes vertically merges boxes that already belong to one column
// and are sorted top→bottom. It skips cross-page number suffixes and merges
// vertically adjacent text.
func mergeColumnBoxes(sortedBoxes []pdf.TextBox, mh, mw float64, isEnglish bool) []pdf.TextBox {
	out := make([]pdf.TextBox, 0, len(sortedBoxes))
	for i := 0; i < len(sortedBoxes); i++ {
		curr := sortedBoxes[i]

		// Skip cross-page suffixes (like previous page number)
		if i > 0 && sortedBoxes[i-1].PageNumber < curr.PageNumber && pageNumSuffixPattern.MatchString(sortedBoxes[i-1].Text) {
			continue
		}

		// Handle empty boxes
		if strings.TrimSpace(curr.Text) == "" {
			if len(out) > 0 {
				prev := &out[len(out)-1]
				if curr.Top-prev.Bottom <= mh*1.5 && util.OverlapX(prev, &curr) >= 0.3 {
					// TODO: prev.Bottom = math.Max(prev.Bottom, curr.Bottom) — direct assignment might shrink tall merged boxes
					// Matches Python behavior (also direct assignment). Defer fix until pipeline alignment release.
					prev.Bottom = curr.Bottom
				}
			}
			continue
		}

		if len(out) == 0 {
			out = append(out, curr)
			continue
		}

		prev := &out[len(out)-1]
		if shouldMergeBoxes(prev, &curr, mh, mw, isEnglish) {
			out[len(out)-1] = mergeTwoBoxes(*prev, curr)
		} else {
			out = append(out, curr)
		}
	}

	return out
}

func moveLeadingTitlesFirst(boxes []pdf.TextBox) []pdf.TextBox {
	firstNonTitleTop := math.Inf(1)
	colsWithNonTitle := make(map[int]struct{})
	for _, box := range boxes {
		if box.LayoutType != pdf.LayoutTypeTitle {
			firstNonTitleTop = math.Min(firstNonTitleTop, box.Top)
			colsWithNonTitle[box.ColID] = struct{}{}
		}
	}

	titles := make([]pdf.TextBox, 0)
	rest := make([]pdf.TextBox, 0, len(boxes))
	for _, box := range boxes {
		_, sharesColumnWithContent := colsWithNonTitle[box.ColID]
		if box.LayoutType == pdf.LayoutTypeTitle && !sharesColumnWithContent && box.Bottom <= firstNonTitleTop {
			titles = append(titles, box)
			continue
		}
		rest = append(rest, box)
	}
	sort.SliceStable(titles, func(i, j int) bool {
		if titles[i].Top != titles[j].Top {
			return titles[i].Top < titles[j].Top
		}
		return titles[i].X0 < titles[j].X0
	})
	return append(titles, rest...)
}

// ---- rune-based text helpers (CJK-safe) ----

func lastRune(s string) rune {
	r, _ := utf8.DecodeLastRuneInString(s)
	return r
}

func firstRune(s string) rune {
	r, _ := utf8.DecodeRuneInString(s)
	return r
}

func secondLastRune(s string) rune {
	r, size := utf8.DecodeLastRuneInString(s)
	if r == utf8.RuneError && size == 0 {
		return 0
	}
	r2, _ := utf8.DecodeLastRuneInString(s[:len(s)-size])
	return r2
}

func endsWithOneOf(s, set string) bool {
	r := lastRune(s)
	if r == 0 {
		return false
	}
	return strings.ContainsRune(set, r)
}

func endsSecondLastOneOf(s, set string) bool {
	r := secondLastRune(s)
	if r == 0 {
		return false
	}
	return strings.ContainsRune(set, r)
}

func startsWithOneOf(s, set string) bool {
	r := firstRune(s)
	if r == 0 {
		return false
	}
	return strings.ContainsRune(set, r)
}

// MergeSameBullet merges adjacent boxes that start with the same bullet/number
// character, combining their text with a newline separator.
func MergeSameBullet(boxes []pdf.TextBox, tok pdf.Tokenizer) []pdf.TextBox {
	if len(boxes) < 2 {
		return boxes
	}
	out := make([]pdf.TextBox, 0, len(boxes))
	i := 0
	for i < len(boxes) {
		if strings.TrimSpace(boxes[i].Text) == "" {
			i++
			continue
		}
		cur := boxes[i]
		i++
		for i < len(boxes) {
			if strings.TrimSpace(boxes[i].Text) == "" {
				i++
				continue
			}
			nxt := boxes[i]
			firstCur := firstRuneString(cur.Text)
			firstNxt := firstRuneString(nxt.Text)
			if firstCur != firstNxt ||
				unicode.Is(unicode.Latin, firstCur) ||
				isChinese(firstCur, tok) ||
				cur.Top > nxt.Bottom {
				break
			}
			cur.Text = cur.Text + "\n" + nxt.Text
			cur.X0 = min(cur.X0, nxt.X0)
			cur.X1 = max(cur.X1, nxt.X1)
			cur.Bottom = nxt.Bottom
			i++
		}
		out = append(out, cur)
	}
	return out
}

func firstRuneString(s string) rune {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	return []rune(s)[0]
}

// isChinese checks if a rune is a Chinese character (CJK Unified Ideograph).
func isChinese(r rune, tok pdf.Tokenizer) bool {
	if tok != nil {
		return strings.Contains(tok.Tag(string(r)), "n")
	}
	return (r >= 0x4E00 && r <= 0x9FFF) ||
		(r >= 0x3400 && r <= 0x4DBF) ||
		(r >= 0x20000 && r <= 0x2A6DF)
}
