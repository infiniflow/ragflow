package tool

import (
	"sort"
	"strings"
	"unicode"
)

func StripMeta(s string) string {
	if idx := strings.LastIndex(s, "\n#@meta"); idx >= 0 {
		return s[:idx]
	}
	return s
}

func CharSimilarity(a, b string) float64 {
	a = StripMeta(a)
	b = StripMeta(b)
	extract := func(s string) map[rune]int {
		m := make(map[rune]int)
		for _, r := range s {
			if !unicode.IsSpace(r) {
				m[r]++
			}
		}
		return m
	}
	ca, cb := extract(a), extract(b)
	if len(ca) == 0 && len(cb) == 0 {
		return 100
	}
	common, totalA, totalB := 0, 0, 0
	for r, n := range ca {
		totalA += n
		if n2, ok := cb[r]; ok {
			common += min(n, n2)
		}
	}
	for _, n := range cb {
		totalB += n
	}
	if totalA+totalB == 0 {
		return 100
	}
	return float64(common*2) / float64(totalA+totalB) * 100
}

func lcsRunes(a, b []rune) int {
	if len(a) < len(b) {
		a, b = b, a
	}
	m, n := len(b), len(a)
	prev := make([]int, m+1)
	cur := make([]int, m+1)
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			if a[i-1] == b[j-1] {
				cur[j] = prev[j-1] + 1
			} else {
				cur[j] = max(cur[j-1], prev[j])
			}
		}
		prev, cur = cur, prev
	}
	return prev[m]
}

func LcsSimilarity(a, b string) float64 {
	a = StripMeta(a)
	b = StripMeta(b)
	ra := make([]rune, 0)
	for _, r := range a {
		if !unicode.IsSpace(r) {
			ra = append(ra, r)
		}
	}
	rb := make([]rune, 0)
	for _, r := range b {
		if !unicode.IsSpace(r) {
			rb = append(rb, r)
		}
	}
	if len(ra) == 0 && len(rb) == 0 {
		return 100
	}
	if len(ra) == 0 || len(rb) == 0 {
		return 0
	}
	return float64(lcsRunes(ra, rb)) / float64(max(len(ra), len(rb))) * 100
}

// RawCharSimilarity is CharSimilarity without space stripping — spaces
// count as characters.  Still strips #@meta lines.
func RawCharSimilarity(a, b string) float64 {
	a = StripMeta(a)
	b = StripMeta(b)
	ca := make(map[rune]int)
	for _, r := range a {
		ca[r]++
	}
	cb := make(map[rune]int)
	for _, r := range b {
		cb[r]++
	}
	if len(ca) == 0 && len(cb) == 0 {
		return 100
	}
	common, totalA, totalB := 0, 0, 0
	for r, n := range ca {
		totalA += n
		if n2, ok := cb[r]; ok {
			common += min(n, n2)
		}
	}
	for _, n := range cb {
		totalB += n
	}
	if totalA+totalB == 0 {
		return 100
	}
	return float64(common*2) / float64(totalA+totalB) * 100
}

// RawLcsSimilarity is LcsSimilarity without space stripping — whitespace
// is kept in the LCS comparison.  Still strips #@meta lines.
func RawLcsSimilarity(a, b string) float64 {
	a = StripMeta(a)
	b = StripMeta(b)
	ra := []rune(a)
	rb := []rune(b)
	if len(ra) == 0 && len(rb) == 0 {
		return 100
	}
	if len(ra) == 0 || len(rb) == 0 {
		return 0
	}
	return float64(lcsRunes(ra, rb)) / float64(max(len(ra), len(rb))) * 100
}

// SectionAlignedScore computes a two-phase LCS similarity:
//
// Phase 1: One-to-one section matching — pair Go and Python sections by
// CharSimilarity (greedy, highest first). For matched pairs, compute
// per-section LCS ratio.
//
// Phase 2: Residual — concatenate all unmatched sections from both sides
// into one string each, compute LCS ratio once. This handles cases where
// one side merges sections that the other side keeps separate.
//
// Final score is a char-weighted average of matched and residual scores.
func SectionAlignedScore(goText, pyText string) float64 {
	split := func(s string) []string {
		s = StripMeta(s)
		return strings.Split(strings.TrimSpace(s), "\n")
	}
	gs := split(goText)
	ps := split(pyText)
	if len(gs) == 0 && len(ps) == 0 {
		return 100
	}
	if len(gs) == 0 || len(ps) == 0 {
		return 0
	}

	// Phase 1: Position-window greedy matching.
	// Sections are ordered top-to-bottom by page position, so a global
	// match beyond a small positional offset is extremely unlikely.
	// Constrain candidates to ±window to avoid O(n×m) blow-up on large docs.
	const alignWindow = 5
	type candidate struct {
		gi, pi int
		sim    float64
	}
	// Precompute rune lengths for length-ratio gating.
	glens := make([]int, len(gs))
	plens := make([]int, len(ps))
	for i, s := range gs {
		glens[i] = len([]rune(s))
	}
	for i, s := range ps {
		plens[i] = len([]rune(s))
	}

	candidates := make([]candidate, 0, len(gs)*(alignWindow*2+1))
	for i, g := range gs {
		lo := max(0, i-alignWindow)
		hi := min(len(ps)-1, i+alignWindow)
		for j := lo; j <= hi; j++ {
			// Skip pairs with >2x length difference — a 500-char section
			// matching a 30-char section produces near-zero LCS.
			if glens[i] > plens[j]*2 || plens[j] > glens[i]*2 {
				continue
			}
			if sim := CharSimilarity(g, ps[j]); sim > 30 {
				candidates = append(candidates, candidate{i, j, sim})
			}
		}
	}
	// Sort descending by similarity — best matches first.
	sort.Slice(candidates, func(a, b int) bool {
		return candidates[a].sim > candidates[b].sim
	})

	goUsed := make([]bool, len(gs))
	pyUsed := make([]bool, len(ps))
	matchedScore := 0.0
	matchedChars := 0

	for _, c := range candidates {
		if goUsed[c.gi] || pyUsed[c.pi] {
			continue
		}
		goUsed[c.gi] = true
		pyUsed[c.pi] = true

		// Compute LCS ratio for matched pair.
		ra := nonSpaceRunes(gs[c.gi])
		rb := nonSpaceRunes(ps[c.pi])
		lcsScore := 0.0
		if len(ra) > 0 && len(rb) > 0 {
			lcsScore = float64(lcsRunes(ra, rb)) / float64(max(len(ra), len(rb))) * 100
		} else if len(ra) == 0 && len(rb) == 0 {
			lcsScore = 100
		}
		chars := max(len(ra), len(rb))
		matchedScore += lcsScore * float64(chars)
		matchedChars += chars
	}

	// Phase 2: Residual — concat unmatched sections, compute LCS once.
	var goRes, pyRes strings.Builder
	for i, g := range gs {
		if !goUsed[i] {
			goRes.WriteString(g)
			goRes.WriteByte(' ')
		}
	}
	for j, p := range ps {
		if !pyUsed[j] {
			pyRes.WriteString(p)
			pyRes.WriteByte(' ')
		}
	}

	residualScore := 0.0
	residualChars := 0
	goResRunes := nonSpaceRunes(goRes.String())
	pyResRunes := nonSpaceRunes(pyRes.String())
	residualChars = max(len(goResRunes), len(pyResRunes))
	if residualChars > 0 {
		if len(goResRunes) > 5000 || len(pyResRunes) > 5000 {
			// Residual too large for O(n²) LCS — fall back to CharSimilarity.
			residualScore = CharSimilarity(goRes.String(), pyRes.String())
		} else {
			residualScore = float64(lcsRunes(goResRunes, pyResRunes)) / float64(residualChars) * 100
		}
	} else if len(goResRunes) == 0 && len(pyResRunes) == 0 {
		residualScore = 100
	}

	// Weighted average.
	totalChars := matchedChars + residualChars
	if totalChars == 0 {
		return 100
	}
	return (matchedScore + residualScore*float64(residualChars)) / float64(totalChars)
}

func nonSpaceRunes(s string) []rune {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if !unicode.IsSpace(r) {
			out = append(out, r)
		}
	}
	return out
}
