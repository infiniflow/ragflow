package layout

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// referenceDedupSubstringOverlaps is a faithful copy of the ORIGINAL O(n²)
// double-loop implementation of DedupSubstringOverlaps. It is the brute-force
// oracle for the equivalence tests below: the indexed rewrite must produce a
// byte-identical kept-set, so any divergence (wrong bucket key, dropped j==i
// guard, broken break semantics) is caught by TestDedupSubstringOverlapsEquivalence.
func referenceDedupSubstringOverlaps(boxes []pdf.TextBox) []pdf.TextBox {
	drop := make([]bool, len(boxes))
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
			if boxes[i].ColID != boxes[j].ColID {
				continue
			}
			if len(ni) >= len(nj) && strings.Contains(ni, nj) {
				if boxInsideTolerant(boxes[j], boxes[i]) {
					drop[j] = true
				}
			} else if len(nj) > len(ni) && strings.Contains(nj, ni) {
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

func mkBox(text string, page, col int, x0, x1, top, bottom float64, isOCR bool) pdf.TextBox {
	return pdf.TextBox{Text: text, PageNumber: page, ColID: col, X0: x0, X1: x1, Top: top, Bottom: bottom, IsOCR: isOCR}
}

// substringVocab has deliberate substring relationships so the random stream
// exercises the containment branches (e.g. "ab" ⊂ "abc", "abc" ⊂ "abcd").
var substringVocab = []string{
	"a", "ab", "abc", "abcd", "abcde",
	"hello", "hello world", "world",
	"", "x", "xy", "xyz", "wxyz",
	"RAG分词", "三国人物", "文章 中 提到", "文章中提到",
}

// randBoxes builds a synthetic document. PageNumber may be 0 (~20%) and ColID
// may be 0, so the bucket key's zero-value paths are exercised. IsOCR is ~90%
// true so OCR-vs-char-path and OCR-vs-OCR paths both appear.
func randBoxes(rng *rand.Rand, pages, perPage, cols int) []pdf.TextBox {
	boxes := make([]pdf.TextBox, 0, pages*perPage)
	for p := 0; p < pages; p++ {
		for k := 0; k < perPage; k++ {
			text := substringVocab[rng.Intn(len(substringVocab))]
			isOCR := rng.Intn(10) != 0
			col := rng.Intn(cols + 1)
			page := p
			if rng.Intn(5) == 0 {
				page = 0
			}
			top := float64(rng.Intn(800))
			h := float64(10 + rng.Intn(30))
			x0 := float64(rng.Intn(500))
			x1 := x0 + float64(50+rng.Intn(400))
			boxes = append(boxes, pdf.TextBox{
				Text:       text,
				PageNumber: page,
				ColID:      col,
				Top:        top,
				Bottom:     top + h,
				X0:         x0,
				X1:         x1,
				IsOCR:      isOCR,
			})
		}
	}
	return boxes
}

// TestDedupSubstringOverlapsEquivalence pins the indexed rewrite to the
// brute-force oracle across many random documents (varied page counts, boxes per
// page, columns, PageNumber==0, ColID==0, mixed OCR). Any bucket-key or
// control-flow divergence shows up as a kept-set mismatch.
func TestDedupSubstringOverlapsEquivalence(t *testing.T) {
	for seed := int64(1); seed <= 50; seed++ {
		rng := rand.New(rand.NewSource(seed))
		boxes := randBoxes(rng, 10+rng.Intn(15), 25+rng.Intn(35), 5)
		// DedupSubstringOverlaps reuses the input slice's backing array
		// (out := boxes[:0]), so each call must get its own copy.
		inA := append([]pdf.TextBox(nil), boxes...)
		inB := append([]pdf.TextBox(nil), boxes...)
		got := DedupSubstringOverlaps(inA)
		want := referenceDedupSubstringOverlaps(inB)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("seed %d: indexed result differs from brute-force oracle (got %d boxes, want %d)", seed, len(got), len(want))
		}
	}
	// A realistic large document (100 pages x 100 boxes) stresses the bucket
	// path the way a real scan doc would.
	for _, seed := range []int64{7, 23} {
		rng := rand.New(rand.NewSource(seed))
		boxes := randBoxes(rng, 100, 100, 5)
		inA := append([]pdf.TextBox(nil), boxes...)
		inB := append([]pdf.TextBox(nil), boxes...)
		got := DedupSubstringOverlaps(inA)
		want := referenceDedupSubstringOverlaps(inB)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("large-doc seed %d: indexed result differs from brute-force oracle (got %d boxes, want %d)", seed, len(got), len(want))
		}
	}
}

func TestDedupSubstringOverlaps_EmptyAndSingle(t *testing.T) {
	if got := DedupSubstringOverlaps(nil); got != nil {
		t.Fatalf("nil input must return nil, got %d", len(got))
	}
	one := []pdf.TextBox{mkBox("hello world", 0, 0, 50, 500, 100, 130, true)}
	if got := DedupSubstringOverlaps(one); len(got) != 1 {
		t.Fatalf("single box must be kept, got %d", len(got))
	}
}

// TestDedupSubstringOverlaps_CharPathContainerKeepsOCR: a char-path box (IsOCR
// false) whose text contains an OCR box's text and geometrically contains it
// must NOT cause the OCR box to be dropped — char-path boxes are never treated
// as fragments.
func TestDedupSubstringOverlaps_CharPathContainerKeepsOCR(t *testing.T) {
	container := mkBox("hello world", 0, 0, 50, 500, 100, 130, false)
	frag := mkBox("world", 0, 0, 100, 400, 105, 120, true)
	got := DedupSubstringOverlaps([]pdf.TextBox{container, frag})
	if len(got) != 2 {
		t.Fatalf("OCR fragment under char-path container must be kept, got %d: %+v", len(got), got)
	}
}

// TestDedupSubstringOverlaps_CrossPageKept: a substring box on a different page
// is independent document text and must be kept.
func TestDedupSubstringOverlaps_CrossPageKept(t *testing.T) {
	a := mkBox("hello world", 5, 0, 50, 500, 100, 130, true)
	b := mkBox("world", 7, 0, 100, 400, 105, 120, true)
	got := DedupSubstringOverlaps([]pdf.TextBox{a, b})
	if len(got) != 2 {
		t.Fatalf("cross-page substring must be kept, got %d", len(got))
	}
}

// TestDedupSubstringOverlaps_CrossColumnKept: a substring box in a DIFFERENT
// column, even when geometrically inside the container, must be kept by the
// ColID guard.
func TestDedupSubstringOverlaps_CrossColumnGeomInsideKept(t *testing.T) {
	a := mkBox("hello world", 5, 0, 50, 500, 100, 130, true)
	b := mkBox("world", 5, 1, 100, 400, 105, 120, true) // inside A geometry, different column
	got := DedupSubstringOverlaps([]pdf.TextBox{a, b})
	if len(got) != 2 {
		t.Fatalf("cross-column substring (geom inside) must be kept by ColID guard, got %d: %+v", len(got), got)
	}
}

// TestDedupSubstringOverlaps_PageZeroSemantics: PageNumber 0 boxes compare with
// other PageNumber 0 boxes (0==0) but not with PageNumber>0 boxes. So an
// in-geometry fragment on page 0 is dropped, while an identical in-geometry
// fragment on page 5 is kept.
func TestDedupSubstringOverlaps_PageZeroSemantics(t *testing.T) {
	c := mkBox("hello world", 0, 0, 50, 500, 100, 130, true)
	f := mkBox("world", 0, 0, 100, 400, 105, 120, true)  // page 0 -> dropped
	f2 := mkBox("world", 5, 0, 100, 400, 105, 120, true) // page 5 -> kept
	got := DedupSubstringOverlaps([]pdf.TextBox{c, f, f2})
	if len(got) != 2 {
		t.Fatalf("want 2 kept (C and F2), got %d: %+v", len(got), got)
	}
	for _, b := range got {
		if b.PageNumber == 0 && b.Text == "world" {
			t.Fatalf("page-0 fragment should have been dropped")
		}
	}
}

// TestDedupSubstringOverlaps_ColIDZeroChain: with the default ColID 0, a
// container and two nested fragments on the same page collapse to the container.
func TestDedupSubstringOverlaps_ColIDZeroChain(t *testing.T) {
	c := mkBox("abcde", 3, 0, 50, 500, 100, 130, true)
	f1 := mkBox("abc", 3, 0, 100, 400, 105, 120, true)
	f2 := mkBox("cde", 3, 0, 100, 400, 105, 120, true)
	got := DedupSubstringOverlaps([]pdf.TextBox{c, f1, f2})
	if len(got) != 1 || got[0].Text != "abcde" {
		t.Fatalf("want only the container 'abcde', got %d: %+v", len(got), got)
	}
}

// TestDedupSubstringOverlaps_TallerFragmentKept: a box whose text is a
// substring but which is geometrically NOT contained (taller, poking out on
// both axes beyond the 3pt tolerance) must be kept.
func TestDedupSubstringOverlaps_TallerFragmentKeptOpt(t *testing.T) {
	c := mkBox("hello world", 0, 0, 50, 500, 100, 130, true) // height 30
	f := mkBox("world", 0, 0, 100, 400, 95, 135, true)       // top/bottom exceed tolerance
	got := DedupSubstringOverlaps([]pdf.TextBox{c, f})
	if len(got) != 2 {
		t.Fatalf("geometrically-disjoint substring must be kept, got %d", len(got))
	}
}

// TestDedupSubstringOverlaps_FragmentDropped: the happy-path case — an OCR
// fragment contained in its container is dropped.
func TestDedupSubstringOverlaps_FragmentDropped(t *testing.T) {
	c := mkBox("hello world", 0, 0, 50, 500, 100, 130, true)
	f := mkBox("world", 0, 0, 100, 400, 105, 120, true)
	got := DedupSubstringOverlaps([]pdf.TextBox{c, f})
	if len(got) != 1 || got[0].Text != "hello world" {
		t.Fatalf("fragment must be dropped, got %d: %+v", len(got), got)
	}
}

func buildSyntheticBoxes(pages, perPage int) []pdf.TextBox {
	rng := rand.New(rand.NewSource(99))
	return randBoxes(rng, pages, perPage, 4)
}

// Both benchmarks run on the SAME synthetic document (fixed seed) so the per-op
// numbers are directly comparable: the indexed path is O(Σ bucket²) while the
// brute oracle is O(n²) over the whole slice.
func BenchmarkDedupSubstringOverlapsIndexed(b *testing.B) {
	boxes := buildSyntheticBoxes(120, 100) // 12000 boxes
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = DedupSubstringOverlaps(boxes)
	}
}

func BenchmarkDedupSubstringOverlapsBrute(b *testing.B) {
	boxes := buildSyntheticBoxes(120, 100) // same input as Indexed
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = referenceDedupSubstringOverlaps(boxes)
	}
}
