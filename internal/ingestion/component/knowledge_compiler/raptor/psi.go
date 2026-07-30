package raptor

import (
	"math"
)

// psiCluster builds the RAPTOR Psi tree: it computes the full pairwise cosine
// similarity matrix and links each node to its most-similar neighbor(s) via
// union-find, forming a forest of connected clusters. No external clustering
// backend is needed (raptor.py Psi path is clustering-free). The returned labels
// group points into connected components; centroids are the mean of each
// component's members. simThreshold controls the edge cutoff (0 = link to the
// single best neighbor only).
func psiCluster(embeddings [][]float64, simThreshold float64) ([]int, [][]float64) {
	n := len(embeddings)
	if n == 0 {
		return nil, nil
	}
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[rb] = ra
		}
	}

	norms := make([]float64, n)
	for i, v := range embeddings {
		norms[i] = math.Sqrt(dotSelf(v))
		if norms[i] == 0 {
			norms[i] = 1
		}
	}

	for i := 0; i < n; i++ {
		bestJ := -1
		bestScore := simThreshold
		for j := 0; j < n; j++ {
			if i == j {
				continue
			}
			score := cosine(embeddings[i], embeddings[j], norms[i], norms[j])
			if score > bestScore {
				bestScore = score
				bestJ = j
			}
		}
		if bestJ >= 0 {
			union(i, bestJ)
		}
	}

	labels := make([]int, n)
	rootID := map[int]int{}
	idx := 0
	for i := 0; i < n; i++ {
		r := find(i)
		if _, ok := rootID[r]; !ok {
			rootID[r] = idx
			idx++
		}
		labels[i] = rootID[r]
	}

	// Centroids per component.
	sums := make([][]float64, len(rootID))
	counts := make([]int, len(rootID))
	d := 0
	if n > 0 {
		d = len(embeddings[0])
	}
	for i := 0; i < len(sums); i++ {
		sums[i] = make([]float64, d)
	}
	for i := 0; i < n; i++ {
		c := labels[i]
		counts[c]++
		for j := 0; j < d; j++ {
			sums[c][j] += embeddings[i][j]
		}
	}
	centroids := make([][]float64, len(sums))
	for c := range sums {
		centroids[c] = make([]float64, d)
		if counts[c] > 0 {
			for j := 0; j < d; j++ {
				centroids[c][j] = sums[c][j] / float64(counts[c])
			}
		}
	}
	return labels, centroids
}

func cosine(a, b []float64, na, nb float64) float64 {
	var s float64
	for i := range a {
		s += a[i] * b[i]
	}
	return s / (na * nb)
}

func dotSelf(a []float64) float64 {
	var s float64
	for _, x := range a {
		s += x * x
	}
	return s
}
