package raptor

import (
	"math"
	"testing"
)

// cosEmbeddings places points on the unit circle so that the cosine similarity
// between consecutive points equals sims[i]. It lets tests assert exact
// watershed cut points from a known similarity profile.
func cosEmbeddings(sims []float64) [][]float64 {
	angles := make([]float64, len(sims)+1)
	for i, c := range sims {
		if c > 1 {
			c = 1
		}
		if c < -1 {
			c = -1
		}
		angles[i+1] = angles[i] + math.Acos(c)
	}
	emb := make([][]float64, len(angles))
	for i, a := range angles {
		emb[i] = []float64{math.Cos(a), math.Sin(a)}
	}
	return emb
}

func uniqueCount(labels []int) int {
	seen := map[int]struct{}{}
	for _, l := range labels {
		seen[l] = struct{}{}
	}
	return len(seen)
}

func TestWatershedEdgeCases(t *testing.T) {
	if _, err := watershed(nil, 4); err == nil {
		t.Fatalf("expected error for empty input")
	}
	labels, err := watershed([][]float64{{1, 0}}, 4)
	if err != nil || len(labels) != 1 || labels[0] != 0 {
		t.Fatalf("n=1: got %v err %v", labels, err)
	}
	// n=2: the Python reference always splits two points into two clusters.
	labels, err = watershed([][]float64{{1, 0}, {0, 1}}, 4)
	if err != nil || len(labels) != 2 || labels[0] != 0 || labels[1] != 1 {
		t.Fatalf("n=2: got %v err %v", labels, err)
	}
}

// TestWatershedTreeOrderControlsCount verifies that the tree_order branching
// factor drives the expected cluster count: for a monotone similarity drop the
// number of clusters is non-increasing as the order grows (larger order => fewer,
// bigger clusters). The expected count is approximately 1 + (N-1)/order.
func TestWatershedTreeOrderControlsCount(t *testing.T) {
	emb := cosEmbeddings([]float64{0.2, 0.4, 0.6, 0.8}) // N = 5 points
	cases := []struct {
		order int
		want  int
	}{
		{2, 3},
		{3, 2},
		{4, 2},
		{10, 2},
	}
	prev := 1 << 30
	for _, c := range cases {
		labels, err := watershed(emb, c.order)
		if err != nil {
			t.Fatalf("order %v: %v", c.order, err)
		}
		got := uniqueCount(labels)
		if got != c.want {
			t.Fatalf("order %v: got %d clusters, want %d", c.order, got, c.want)
		}
		if got > prev {
			t.Fatalf("order %v: clusters %d > previous %d (not non-increasing)", c.order, got, prev)
		}
		prev = got
	}
}

func TestWatershedDeterministic(t *testing.T) {
	emb := cosEmbeddings([]float64{0.1, 0.9, 0.2, 0.8, 0.3})
	a, err := watershed(emb, 4)
	if err != nil {
		t.Fatal(err)
	}
	b, err := watershed(emb, 4)
	if err != nil {
		t.Fatal(err)
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("non-deterministic at %d: %v vs %v", i, a, b)
		}
	}
}

func TestWatershedScaleInvariant(t *testing.T) {
	emb := cosEmbeddings([]float64{0.2, 0.4, 0.6, 0.8})
	scaled := make([][]float64, len(emb))
	for i, r := range emb {
		scaled[i] = []float64{r[0] * 5, r[1] * 5}
	}
	a, _ := watershed(emb, 2)
	b, _ := watershed(scaled, 2)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("scale changed labels at %d: %v vs %v", i, a, b)
		}
	}
}

// TestWatershedTreeOrderDefault verifies the exported default is honored when
// the parameter is absent.
func TestWatershedTreeOrderDefault(t *testing.T) {
	emb := cosEmbeddings([]float64{0.2, 0.4, 0.6, 0.8})
	a, _ := watershed(emb, DefaultTreeOrder)
	b, _ := watershed(emb, 4)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("default order != 4 at %d: %v vs %v", i, a, b)
		}
	}
}
