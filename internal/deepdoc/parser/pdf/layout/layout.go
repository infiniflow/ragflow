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

	pageGroups, sortedPages := groupBoxesByPage(boxes)

	result := make([]pdf.TextBox, len(boxes))
	copy(result, boxes)

	// Step A: per-page best k using silhouette score.
	pageCols := make(map[int]int)
	for _, pg := range sortedPages {
		indices := pageGroups[pg]
		determineBestKForPage(boxes, result, indices, pg, pageCols)
	}

	// Step B: assign col_id per page using per-page best k.
	// Labels are remapped by centroid x-order: leftmost column → 0.
	for _, pg := range sortedPages {
		indices := pageGroups[pg]
		assignColIDsForPage(boxes, result, indices, pg, pageCols)
	}

	return result
}

// determineBestKForPage finds the best number of clusters (k) for a page using silhouette score
func determineBestKForPage(boxes, result []pdf.TextBox, indices []int, pg int, pageCols map[int]int) {
	n := len(indices)
	if n < 2 {
		pageCols[pg] = 1
		for _, idx := range indices {
			result[idx].ColID = 0
		}
		return
	}

	x0s, minX0, maxX1 := extractX0Values(boxes, indices)
	pageWidth := maxX1 - minX0
	indentTol := pageWidth * 0.12
	applyIndentTolerance(x0s, minX0, indentTol)

	bestK, _ := findBestK(x0s, n)
	pageCols[pg] = bestK
}

// extractX0Values extracts x0 coordinates from boxes on a page and finds minX0 and maxX1
func extractX0Values(boxes []pdf.TextBox, indices []int) (x0s []float64, minX0 float64, maxX1 float64) {
	n := len(indices)
	x0s = make([]float64, n)
	minX0 = math.MaxFloat64
	maxX1 = 0.0
	for i, idx := range indices {
		x0s[i] = boxes[idx].X0
		if x0s[i] < minX0 {
			minX0 = x0s[i]
		}
		if boxes[idx].X1 > maxX1 {
			maxX1 = boxes[idx].X1
		}
	}
	return x0s, minX0, maxX1
}

// applyIndentTolerance adjusts x0 values that are close to minX0 to improve clustering
func applyIndentTolerance(x0s []float64, minX0, indentTol float64) {
	for i := range x0s {
		if math.Abs(x0s[i]-minX0) < indentTol {
			x0s[i] = minX0
		}
	}
}

// findBestK tries k from 1 to min(4, n) and returns the k with the best silhouette score
func findBestK(x0s []float64, n int) (bestK int, bestScore float64) {
	maxTry := min(4, n)
	if maxTry < 2 {
		maxTry = 1
	}
	bestK, bestScore = 1, -1.0

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
	return bestK, bestScore
}

// assignColIDsForPage assigns column IDs to boxes on a page using the best k
func assignColIDsForPage(boxes, result []pdf.TextBox, indices []int, pg int, pageCols map[int]int) {
	if len(indices) == 0 {
		return
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
	remap := remapLabelsByCentroidOrder(centroids)

	for i, idx := range indices {
		result[idx].ColID = remap[labels[i]]
	}
}

// remapLabelsByCentroidOrder remaps cluster labels so leftmost column = 0
func remapLabelsByCentroidOrder(centroids []float64) map[int]int {
	type clPair struct {
		center float64
		label  int
	}
	var pairs []clPair
	for lbl, c := range centroids {
		pairs = append(pairs, clPair{c, lbl})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].center < pairs[j].center })
	remap := make(map[int]int, len(centroids))
	for newL, p := range pairs {
		remap[p.label] = newL
	}
	return remap
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
		if mh <= 0 {
			mh = util.MedianHeight(bxs)
		}
		mw := medianWidths[pg]
		if mw <= 0 {
			mw = 8 // Python fallback: np.median([...]) if chars else 8 (pdf_parser.py:1465)
		}

		// Process boxes for this page
		processed := processPageBoxes(bxs, mh, mw, isEnglish)
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

// processPageBoxes processes all boxes for a single page
func processPageBoxes(boxes []pdf.TextBox, mh, mw float64, isEnglish bool) []pdf.TextBox {
	if len(boxes) == 0 {
		return boxes
	}

	// Sort by Top, X0
	sortedBoxes := make([]pdf.TextBox, len(boxes))
	copy(sortedBoxes, boxes)
	sort.Slice(sortedBoxes, func(i, j int) bool {
		if sortedBoxes[i].Top != sortedBoxes[j].Top {
			return sortedBoxes[i].Top < sortedBoxes[j].Top
		}
		return sortedBoxes[i].X0 < sortedBoxes[j].X0
	})

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
