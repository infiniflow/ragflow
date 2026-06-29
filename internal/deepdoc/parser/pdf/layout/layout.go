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

// AssignColumn groups boxes into columns on each page by KMeans x0 clustering
// with silhouette score selection, matching Python's _assign_column().
//
// Python: pdf_parser.py:739 _assign_column()
func AssignColumn(boxes []pdf.TextBox, zoom float64) []pdf.TextBox {
	if len(boxes) == 0 {
		return boxes
	}

	pageGroups := make(map[int][]int)
	for i, b := range boxes {
		pageGroups[b.PageNumber] = append(pageGroups[b.PageNumber], i)
	}

	result := make([]pdf.TextBox, len(boxes))
	copy(result, boxes)

	// Step A: per-page best k using silhouette score.
	pageCols := make(map[int]int)
	for pg, indices := range pageGroups {
		n := len(indices)
		if n < 2 {
			pageCols[pg] = 1
			for _, idx := range indices {
				result[idx].ColID = 0
			}
			continue
		}

		// Extract x0 values and apply indent tolerance (12% of page width).
		x0s := make([]float64, n)
		minX0 := math.MaxFloat64
		maxX1 := 0.0
		for i, idx := range indices {
			x0s[i] = boxes[idx].X0
			if x0s[i] < minX0 {
				minX0 = x0s[i]
			}
			if boxes[idx].X1 > maxX1 {
				maxX1 = boxes[idx].X1
			}
		}
		pageWidth := maxX1 - minX0
		indentTol := pageWidth * 0.12

		for i := range x0s {
			if math.Abs(x0s[i]-minX0) < indentTol {
				x0s[i] = minX0
			}
		}

		// Try k = 1 .. min(4, n), pick best by silhouette.
		maxTry := min(4, n)
		if maxTry < 2 {
			maxTry = 1
		}
		bestK, bestScore := 1, -1.0

		for k := 1; k <= maxTry; k++ {
			labels, _ := util.KMeans1D(x0s, k)
			var score float64
			if k > 1 {
				score = util.Silhouette1D(x0s, labels)
			}
			// score = 0 for k=1; score = -1 if silhouette undefined.
			if score > bestScore {
				bestScore = score
				bestK = k
			}
		}
		pageCols[pg] = bestK
	}

	// Step B: assign col_id per page using per-page best k.
	// Labels are remapped by centroid x-order: leftmost column → 0.
	for pg, indices := range pageGroups {
		if len(indices) == 0 {
			continue
		}
		k := pageCols[pg]
		if len(indices) < k {
			k = 1
		}

		x0s := make([]float64, len(indices))
		for i, idx := range indices {
			x0s[i] = boxes[idx].X0
		}

		labels, centroids := util.KMeans1D(x0s, k)

		// Sort centroids by x position, remap labels left→right.
		type clPair struct {
			center float64
			label  int
		}
		var pairs []clPair
		for lbl, c := range centroids {
			pairs = append(pairs, clPair{c, lbl})
		}
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].center < pairs[j].center })
		remap := make(map[int]int, k)
		for newL, p := range pairs {
			remap[p.label] = newL
		}

		for i, idx := range indices {
			result[idx].ColID = remap[labels[i]]
		}
	}

	return result
}

// ---- Text merge (horizontal) ----

// TextMerge horizontally merges adjacent boxes at similar vertical positions.
//
// Python: pdf_parser.py:888 _text_merge()
func TextMerge(boxes []pdf.TextBox, medianHeights map[int]float64, zoom float64) []pdf.TextBox {
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
func NaiveVerticalMerge(boxes []pdf.TextBox, medianHeights map[int]float64, medianWidths map[int]float64, isEnglish bool) []pdf.TextBox {
	if len(boxes) < 2 {
		return boxes
	}
	// Group by page only — matches Python's _naive_vertical_merge which
	// hardcodes col="x" (pdf_parser.py:868), ignoring column assignment.
	// Cross-column merges are prevented by the 30% horizontal overlap check.
	groups := make(map[int][]int)
	for i, b := range boxes {
		groups[b.PageNumber] = append(groups[b.PageNumber], i)
	}
	// Sort page keys for deterministic output order (Python dict preserves
	// insertion order since 3.7, Go map iteration is random).
	pageKeys := make([]int, 0, len(groups))
	for pg := range groups {
		pageKeys = append(pageKeys, pg)
	}
	sort.Ints(pageKeys)

	var result []pdf.TextBox
	for _, pg := range pageKeys {
		indices := groups[pg]
		sort.Slice(indices, func(i, j int) bool {
			bi, bj := boxes[indices[i]], boxes[indices[j]]
			if bi.Top != bj.Top {
				return bi.Top < bj.Top
			}
			return bi.X0 < bj.X0
		})
		bxs := make([]pdf.TextBox, len(indices))
		for i, idx := range indices {
			bxs[i] = boxes[idx]
		}

		mh := medianHeights[pg]
		if mh <= 0 {
			mh = util.MedianHeight(bxs)
		}
		mw := medianWidths[pg]
		if mw <= 0 {
			mw = 8 // Python fallback: np.median([...]) if chars else 8 (pdf_parser.py:1465)
		}

		// Collect pattern: build output slice, merging into last element when appropriate.
		out := make([]pdf.TextBox, 0, len(bxs))
		for i := 0; i < len(bxs); i++ {
			b := bxs[i]
			// Cross-page suffix (e.g. page number on previous page): skip.
			if i > 0 && bxs[i-1].PageNumber < b.PageNumber && pageNumSuffixPattern.MatchString(bxs[i-1].Text) {
				continue
			}
			if strings.TrimSpace(b.Text) == "" {
				// Whitespace gap bridge: absorb into prev box if gap/xov pass,
				// extending prev.Bottom.  This matches Python's while/pop which
				// keeps whitespace inline and lets it extend the previous box.
				if len(out) > 0 {
					prev := &out[len(out)-1]
					if b.Top-prev.Bottom <= mh*1.5 && util.OverlapX(prev, &b) >= 0.3 {
						// TODO: prev.Bottom = math.Max(prev.Bottom, b.Bottom) — direct assignment
						// can shrink a tall merged box when a short whitespace box overlaps.
						// Matches Python behavior (also direct assignment). Defer fix until
						// pipeline alignment is shipped. See TestNaiveVerticalMerge_BottomShrink.
						prev.Bottom = b.Bottom
					}
				}
				continue
			}
			if len(out) == 0 {
				out = append(out, b)
				continue
			}
			prev := &out[len(out)-1]
			if prev.LayoutNo != b.LayoutNo || strings.TrimSpace(b.Text) == "" {
				slog.Debug("vm reject", "reason", "layout_no", "prevLayout", prev.LayoutNo, "bLayout", b.LayoutNo)
				out = append(out, b)
				continue
			}
			gap := b.Top - prev.Bottom
			if gap > mh*1.5 {
				slog.Debug("vm reject", "reason", "gap", "gap", gap, "threshold", mh*1.5, "mh", mh)
				out = append(out, b)
				continue
			}
			ov := util.OverlapX(prev, &b)
			if ov < 0.3 {
				slog.Debug("vm reject", "reason", "ovX", "ov", ov, "threshold", 0.3)
				out = append(out, b)
				continue
			}

			// Strip text before checking first/last characters (matching Python's
			// b["text"].strip()[-1] / b_["text"].strip()[0]).
			prevText := strings.TrimSpace(prev.Text)
			bText := strings.TrimSpace(b.Text)

			concatting := []bool{
				endsWithOneOf(prevText, ",;:\"，、‘“；：-"),
				endsSecondLastOneOf(prevText, ",;:\"，、‘“；："),
				startsWithOneOf(bText, "。；？！”）),，、："),
			}
			anti := []bool{
				endsWithOneOf(prevText, "。？！?"),
				isEnglish && endsWithOneOf(prevText, ".!?"),
				prev.PageNumber == b.PageNumber && b.Top-prev.Bottom > mh*1.5,
				prev.PageNumber < b.PageNumber && math.Abs(prev.X0-b.X0) > mw*4,
			}
			detach := []bool{prev.X1 < b.X0, prev.X0 > b.X1}
			if (slices.Contains(anti, true) && !slices.Contains(concatting, true)) || slices.Contains(detach, true) {
				out = append(out, b)
				continue
			}

			slog.Debug("vm merge", "gap", gap, "ovX", ov, "mh", mh, "prev", prevText[:min(40, len(prevText))], "next", bText[:min(40, len(bText))])
			// Python: (b["text"].rstrip() + " " + b_["text"].lstrip()).strip()
			prev.Text = strings.TrimSpace(strings.TrimRight(prevText, " \t") + " " + strings.TrimLeft(bText, " \t"))
			// Preserve the taller bottom when merging (prev.Bottom may already
			// extend beyond b.Bottom from a previous merge step).
			prev.Bottom = math.Max(prev.Bottom, b.Bottom)
			prev.X0 = math.Min(prev.X0, b.X0)
			prev.X1 = math.Max(prev.X1, b.X1)
		}
		result = append(result, out...)
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
