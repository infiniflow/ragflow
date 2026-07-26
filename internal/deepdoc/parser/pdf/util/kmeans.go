package util

import (
	"math"
	"sort"
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

// Silhouette1D computes the silhouette score for 1D data.
// Returns a score in [-1, 1]. Higher is better.
// Returns -1 if the score cannot be computed (fewer than 2 unique labels).
// Samples alone in their cluster contribute 0, matching sklearn behavior.
//
// Python: sklearn.metrics.silhouette_score with Euclidean distance.
func Silhouette1D(data []float64, labels []int) float64 {
	n := len(data)
	if n <= 1 {
		return 0
	}

	clusterCounts := make(map[int]int)
	for _, l := range labels {
		clusterCounts[l]++
	}

	uniqueClusters := make([]int, 0, len(clusterCounts))
	for cl := range clusterCounts {
		uniqueClusters = append(uniqueClusters, cl)
	}

	// Need at least 2 distinct labels for silhouette.
	if len(uniqueClusters) < 2 {
		return -1
	}
	sort.Ints(uniqueClusters)

	var totalScore float64
	for i := 0; i < n; i++ {
		// sklearn convention: silhouette = 0 for samples alone in their cluster.
		if clusterCounts[labels[i]] <= 1 {
			continue
		}

		// a_i: mean distance to other points in same cluster
		var aSum float64
		aCount := 0
		for j := 0; j < n; j++ {
			if i != j && labels[j] == labels[i] {
				aSum += math.Abs(data[i] - data[j])
				aCount++
			}
		}
		a := 0.0
		if aCount > 0 {
			a = aSum / float64(aCount)
		}

		// b_i: min mean distance to points in other clusters
		b := math.MaxFloat64
		for _, cl := range uniqueClusters {
			if cl == labels[i] {
				continue
			}
			var bSum float64
			bCount := 0
			for j := 0; j < n; j++ {
				if labels[j] == cl {
					bSum += math.Abs(data[i] - data[j])
					bCount++
				}
			}
			if bCount > 0 {
				meanDist := bSum / float64(bCount)
				if meanDist < b {
					b = meanDist
				}
			}
		}
		if b == math.MaxFloat64 {
			b = 0
		}

		maxAB := math.Max(a, b)
		if maxAB > 0 {
			totalScore += (b - a) / maxAB
		}
	}

	return totalScore / float64(n)
}
