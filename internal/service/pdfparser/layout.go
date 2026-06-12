package pdfparser

import (
	"math"
	"regexp"
	"sort"
	"strings"
)

// ---- Column assignment ----

// AssignColumn groups boxes into columns on each page by KMeans x0 clustering
// with silhouette score selection, matching Python's _assign_column().
//
// Python: pdf_parser.py:739 _assign_column()
func AssignColumn(boxes []TextBox, zoom float64) []TextBox {
	if len(boxes) == 0 {
		return boxes
	}

	pageGroups := make(map[int][]int)
	for i, b := range boxes {
		pageGroups[b.PageNumber] = append(pageGroups[b.PageNumber], i)
	}

	result := make([]TextBox, len(boxes))
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
			labels, _ := kmeans1D(x0s, k)
			var score float64
			if k > 1 {
				score = silhouette1D(x0s, labels)
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

		labels, centroids := kmeans1D(x0s, k)

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
func TextMerge(boxes []TextBox, meanHeights map[int]float64, zoom float64) []TextBox {
	if len(boxes) < 2 {
		return boxes
	}
	result := make([]TextBox, len(boxes))
	copy(result, boxes)

	i := 0
	for i < len(result)-1 {
		b, bNext := result[i], result[i+1]
		if b.PageNumber != bNext.PageNumber || b.ColID != bNext.ColID {
			i++; continue
		}
		// Python: b.get("layoutno", "0") != b_.get("layoutno", "1") —
		// asymmetric defaults mean empty/missing layoutno never merge horizontally.
		if b.LayoutNo != bNext.LayoutNo || b.LayoutNo == "" || bNext.LayoutNo == "" ||
			b.LayoutType == "table" || b.LayoutType == "figure" || b.LayoutType == "equation" {
			i++; continue
		}
		mh := meanHeights[b.PageNumber]
		if mh <= 0 { mh = 10 }
		if math.Abs(BoxYDis(b, bNext)) < mh/3 {
			result[i].X1 = bNext.X1
			result[i].Top = (b.Top + bNext.Top) / 2
			result[i].Bottom = (b.Bottom + bNext.Bottom) / 2
			result[i].Text += bNext.Text
			result = append(result[:i+1], result[i+2:]...)
			continue
		}
		i++
	}
	return result
}

// ---- Naive vertical merge ----

// NaiveVerticalMerge vertically merges boxes on the same page/column.
//
// Python: pdf_parser.py:926 _naive_vertical_merge()
func NaiveVerticalMerge(boxes []TextBox, meanHeights map[int]float64, meanWidths map[int]float64, isEnglish bool) []TextBox {
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

	var result []TextBox
	for _, pg := range pageKeys {
		indices := groups[pg]
		sort.Slice(indices, func(i, j int) bool {
			bi, bj := boxes[indices[i]], boxes[indices[j]]
			if bi.Top != bj.Top { return bi.Top < bj.Top }
			return bi.X0 < bj.X0
		})
		bxs := make([]TextBox, len(indices))
		for i, idx := range indices {
			bxs[i] = boxes[idx]
		}

		mh := meanHeights[pg]
		if mh <= 0 { mh = MedianHeight(bxs) }
		mw := meanWidths[pg]
		if mw <= 0 { mw = 5 }

		i := 0
		for i+1 < len(bxs) {
			b, bNext := bxs[i], bxs[i+1]
			if b.PageNumber < bNext.PageNumber && pageNumSuffixPattern.MatchString(b.Text) {
				bxs = append(bxs[:i], bxs[i+1:]...); continue
			}
			if strings.TrimSpace(b.Text) == "" {
				bxs = append(bxs[:i], bxs[i+1:]...); continue
			}
			if b.LayoutNo != bNext.LayoutNo || strings.TrimSpace(bNext.Text) == "" {
				i++; continue
			}
			if bNext.Top-b.Bottom > mh*1.5 { i++; continue }
			if TextBoxOverlapX(b, bNext) < 0.3 { i++; continue }

			// Strip text before checking first/last characters (matching Python's
			// b["text"].strip()[-1] / b_["text"].strip()[0]).
			bText := strings.TrimSpace(b.Text)
			bNextText := strings.TrimSpace(bNext.Text)

			concatting := []bool{
				endsWithOneOf(bText, ",;:'\",‘、“；：-"),
				endsSecondLastOneOf(bText, ",;:'\",‘、“；："),
				startsWithOneOf(bNextText, "。；？！?”)）、，、："),
			}
			anti := []bool{
				b.LayoutNo != bNext.LayoutNo,
				endsWithOneOf(bText, "。？！?"),
				isEnglish && endsWithOneOf(bText, ".!?"),
				b.PageNumber == bNext.PageNumber && bNext.Top-b.Bottom > mh*1.5,
				b.PageNumber < bNext.PageNumber && math.Abs(b.X0-bNext.X0) > mw*4,
			}
			detach := []bool{b.X1 < bNext.X0, b.X0 > bNext.X1}
			if (any(anti) && !any(concatting)) || any(detach) { i++; continue }

			// Python: (b["text"].rstrip() + " " + b_["text"].lstrip()).strip()
			bxs[i].Text = strings.TrimSpace(strings.TrimRight(bText, " \t\n\r") + " " + strings.TrimLeft(bNextText, " \t\n\r"))
			bxs[i].Bottom = bNext.Bottom
			bxs[i].X0 = math.Min(b.X0, bNext.X0)
			bxs[i].X1 = math.Max(b.X1, bNext.X1)
			bxs = append(bxs[:i+1], bxs[i+2:]...)
		}
		result = append(result, bxs...)
	}
	return result
}

// ---- Reading order ----

// FinalReadingOrderMerge sorts boxes by page → column → top → x0.
//
// Python: pdf_parser.py:1007 _final_reading_order_merge()
func FinalReadingOrderMerge(boxes []TextBox) []TextBox {
	if len(boxes) == 0 {
		return boxes
	}
	sort.Slice(boxes, func(i, j int) bool {
		bi, bj := boxes[i], boxes[j]
		if bi.PageNumber != bj.PageNumber { return bi.PageNumber < bj.PageNumber }
		if bi.ColID != bj.ColID { return bi.ColID < bj.ColID }
		if bi.Top != bj.Top { return bi.Top < bj.Top }
		return bi.X0 < bj.X0
	})
	return boxes
}

// ---- Proj (heading detection) ----

var projPatterns = []*regexp.Regexp{
	regexp.MustCompile(`第[零一二三四五六七八九十百]+章`),
	regexp.MustCompile(`第[零一二三四五六七八九十百]+[条节]`),
	regexp.MustCompile(`[零一二三四五六七八九十百]+[、是 　]`),
	regexp.MustCompile(`[\(（][零一二三四五六七八九十百]+[）\)]`),
	regexp.MustCompile(`[\(（][0-9]+[）\)]`),
	regexp.MustCompile(`[0-9]+(、|\.[　 ]|）|\.[^0-9./a-zA-Z_%><-]{4,})`),
	regexp.MustCompile(`[0-9]+\.[0-9.]+(、|\.[ 　])`),
	regexp.MustCompile(`[⚫•➢①②]`),
}

var pageNumSuffixPattern = regexp.MustCompile(`[0-9  •一—-]+$`)

// MatchProj checks if a text box represents a heading.
//
// Python: pdf_parser.py:119 _match_proj()
func MatchProj(b TextBox) bool {
	text := strings.TrimSpace(b.Text)
	for _, p := range projPatterns {
		if p.MatchString(text) { return true }
	}
	return false
}

// ---- rune-based text helpers (CJK-safe) ----

func lastRune(s string) rune {
	runes := []rune(s)
	if len(runes) == 0 { return 0 }
	return runes[len(runes)-1]
}

func firstRune(s string) rune {
	runes := []rune(s)
	if len(runes) == 0 { return 0 }
	return runes[0]
}

func secondLastRune(s string) rune {
	runes := []rune(s)
	if len(runes) < 2 { return 0 }
	return runes[len(runes)-2]
}

func endsWithOneOf(s, set string) bool {
	r := lastRune(s)
	if r == 0 { return false }
	return containsRune(set, r)
}

func endsSecondLastOneOf(s, set string) bool {
	r := secondLastRune(s)
	if r == 0 { return false }
	return containsRune(set, r)
}

func startsWithOneOf(s, set string) bool {
	r := firstRune(s)
	if r == 0 { return false }
	return containsRune(set, r)
}

func containsRune(set string, r rune) bool {
	for _, rr := range set {
		if rr == r { return true }
	}
	return false
}

// -- utility --

func any(flags []bool) bool {
	for _, f := range flags {
		if f {
			return true
		}
	}
	return false
}
