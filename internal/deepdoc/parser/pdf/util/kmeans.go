package util

import (
	"math"
)

// KMeans1D performs 1-dimensional KMeans clustering.
// Returns per-point labels and final centroid values.
//
// Initialization: evenly spaced centroids (deterministic, equivalent to
// sklearn KMeans with fixed seed in practice for 1D data).
func KMeans1D(data []float64, k int) (labels []int, centroids []float64) {
	n := len(data)
	labels = make([]int, n)

	if k <= 1 {
		var sum float64
		for _, v := range data {
			sum += v
		}
		return labels, []float64{sum / float64(n)}
	}
	if n <= k {
		// Each point gets its own centroid. When n < k we return n
		// centroids (you cannot have more clusters than data points).
		centroids = make([]float64, n)
		for i, v := range data {
			centroids[i] = v
			labels[i] = i
		}
		return labels, centroids
	}

	// Linear scan for min/max: O(n) instead of O(n log n) sort.
	minV, maxV := data[0], data[0]
	for _, v := range data {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}

	centroids = make([]float64, k)
	for c := 0; c < k; c++ {
		// Evenly space between min and max
		if k == 1 {
			centroids[c] = minV
		} else {
			centroids[c] = minV + float64(c)*(maxV-minV)/float64(k-1)
		}
	}

	// Lloyd's algorithm
	for iter := 0; iter < 100; iter++ {
		changed := false
		// Assign each point to nearest centroid
		for i, v := range data {
			bestC, bestD := 0, math.Abs(v-centroids[0])
			for c := 1; c < k; c++ {
				d := math.Abs(v - centroids[c])
				if d < bestD {
					bestC, bestD = c, d
				}
			}
			if labels[i] != bestC {
				changed = true
			}
			labels[i] = bestC
		}
		if !changed {
			break
		}
		// Update centroids
		counts := make([]int, k)
		sums := make([]float64, k)
		for i, v := range data {
			counts[labels[i]]++
			sums[labels[i]] += v
		}
		for c := 0; c < k; c++ {
			if counts[c] > 0 {
				centroids[c] = sums[c] / float64(counts[c])
			}
		}
	}

	return
}
