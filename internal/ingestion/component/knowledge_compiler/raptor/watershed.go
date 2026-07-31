// watershed is the sole clustering method used by the RAPTOR tree.
//
// It is the algorithm historically named "AHC" in the Python raptor
// implementation (raptor.py::_get_clusters_ahc, logged as "RAPTOR seq-clus");
// the name "watershed" reflects what the code actually does, which is not
// agglomerative hierarchical clustering.
//
// See PORT_PLAN.md §3.3 and the M6 validation gate (缺口 C).
package raptor

import (
	"errors"
	"math"
	"sort"
)

// DefaultTreeOrder is the default branching factor (the B+ tree "order") used by
// watershed clustering. It is the average chunk count per level-0 cluster: the
// expected number of level-0 clusters is close to len(embeddings)/DefaultTreeOrder,
// so the recursion depth is roughly log_DefaultTreeOrder(len(embeddings)). 4
// (ratio 25) is close to the Python raptor "cluster_percentile" default of 30.
const DefaultTreeOrder = 4

// MinTreeOrder and MaxTreeOrder bound the integer tree_order parameter.
const (
	MinTreeOrder = 2
	MaxTreeOrder = 16
)

// watershed performs a 1D watershed over the sequence of embeddings: it scans
// adjacent pairs only and starts a new cluster whenever the cosine similarity
// between two consecutive embeddings falls below a percentile threshold.
//
// The cut points are contiguous segments of the input in its given order.
// Callers pass document-ordered embeddings, so the boundaries correspond to
// topic shifts in the text. This is a cheap, deterministic text-segmentation
// heuristic (not a semantic clustering), and is O(N) in the number of chunks.
//
// treeOrder is the branching factor of the RAPTOR tree (the B+ tree "order"):
// the average chunk count per level-0 cluster. It maps to the percentile
// threshold as 100/treeOrder, so the expected level-0 cluster count is
// approximately len(embeddings)/treeOrder. Larger treeOrder => fewer, bigger
// clusters and a shallower tree; smaller treeOrder => more, smaller clusters
// and a deeper tree. treeOrder is clamped to [MinTreeOrder, MaxTreeOrder].
func watershed(embeddings [][]float64, treeOrder int) ([]int, error) {
	n := len(embeddings)
	switch {
	case n == 0:
		return nil, errors.New("raptor: watershed requires at least one embedding")
	case n == 1:
		return []int{0}, nil
	case n == 2:
		// One adjacent pair: the Python reference always splits two points into
		// two clusters, so the recursion terminates with one-point leaves.
		return []int{0, 1}, nil
	}

	if treeOrder < MinTreeOrder {
		treeOrder = MinTreeOrder
	} else if treeOrder > MaxTreeOrder {
		treeOrder = MaxTreeOrder
	}

	// L2-normalize so the dot product equals cosine similarity.
	normalized := normalizeRows(embeddings)

	// Adjacent cosine similarities (n-1 pairs).
	adj := make([]float64, n-1)
	for i := 0; i < n-1; i++ {
		adj[i] = dot(normalized[i], normalized[i+1])
	}

	// The percentile threshold is 100/treeOrder: a larger treeOrder lowers the
	// threshold percentile, producing fewer cuts (fewer, bigger clusters).
	clusterRatio := 100.0 / float64(treeOrder)
	threshold := percentile(adj, clusterRatio)
	labels := make([]int, n)
	clusterID := 0
	for i := 1; i < n; i++ {
		if adj[i-1] >= threshold {
			labels[i] = clusterID
		} else {
			clusterID++
			labels[i] = clusterID
		}
	}
	return labels, nil
}

// normalizeRows returns a copy of rows with each row L2-normalized. Zero rows
// are left unchanged (their dot products are zero) so they cannot produce NaN.
func normalizeRows(rows [][]float64) [][]float64 {
	out := make([][]float64, len(rows))
	for i, r := range rows {
		var norm float64
		for _, v := range r {
			norm += v * v
		}
		norm = math.Sqrt(norm)
		if norm == 0 {
			norm = 1
		}
		row := make([]float64, len(r))
		for j, v := range r {
			row[j] = v / norm
		}
		out[i] = row
	}
	return out
}

func dot(a, b []float64) float64 {
	var s float64
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

// percentile returns the p-th percentile of values using linear interpolation,
// matching numpy's default. p is clamped to [0,100].
func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	cp := append([]float64(nil), values...)
	sort.Float64s(cp)
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	rank := (p / 100.0) * float64(len(cp)-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return cp[lo]
	}
	frac := rank - float64(lo)
	return cp[lo]*(1-frac) + cp[hi]*frac
}
