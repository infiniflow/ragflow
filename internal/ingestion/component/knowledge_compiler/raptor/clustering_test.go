package raptor

import (
	"math"
	"math/rand"
	"testing"
)

// TestGMMCluster_TwoBlobs verifies the diagonal-GMM backend recovers two
// well-separated isotropic Gaussian blobs, produces valid labels/centroids, and
// is deterministic (the golden gate and production behaviour depend on this).
func TestGMMCluster_TwoBlobs(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	blobs := make([][]float64, 0, 20)
	for i := 0; i < 10; i++ {
		blobs = append(blobs, []float64{1 + rng.NormFloat64()*0.1, 1 + rng.NormFloat64()*0.1})
	}
	for i := 0; i < 10; i++ {
		blobs = append(blobs, []float64{-1 + rng.NormFloat64()*0.1, -1 + rng.NormFloat64()*0.1})
	}

	spec := ClusterSpec{Method: ClusteringGMM, MinClusters: 2, MaxClusters: 2}
	labels, centroids := gmmCluster(blobs, spec)
	if len(labels) != len(blobs) {
		t.Fatalf("labels len = %d, want %d", len(labels), len(blobs))
	}
	if len(centroids) == 0 {
		t.Fatal("gmmCluster returned no centroids")
	}
	for _, c := range centroids {
		if len(c) != 2 {
			t.Fatalf("centroid dim = %d, want 2", len(c))
		}
	}

	// Distinct cluster count must be exactly 2.
	counts := map[int]int{}
	for _, l := range labels {
		counts[l]++
	}
	if len(counts) != 2 {
		t.Fatalf("recovered %d clusters, want 2 (labels=%v)", len(counts), labels)
	}

	// Each recovered centroid must sit near one of the two blob centers.
	sum := map[int][]float64{}
	cnt := map[int]int{}
	for i, l := range labels {
		if sum[l] == nil {
			sum[l] = make([]float64, 2)
		}
		sum[l][0] += blobs[i][0]
		sum[l][1] += blobs[i][1]
		cnt[l]++
	}
	for l, s := range sum {
		mean := []float64{s[0] / float64(cnt[l]), s[1] / float64(cnt[l])}
		d0 := math.Hypot(mean[0]-1, mean[1]-1)
		d1 := math.Hypot(mean[0]+1, mean[1]+1)
		if math.Min(d0, d1) > 0.3 {
			t.Errorf("cluster %d centroid %v is far from both blob centers", l, mean)
		}
	}

	// Determinism: identical inputs must yield identical labels.
	labels2, _ := gmmCluster(blobs, spec)
	for i := range labels {
		if labels[i] != labels2[i] {
			t.Fatalf("non-deterministic GMM labels at %d: %v vs %v", i, labels, labels2)
		}
	}
}

// TestGMMCluster_SinglePoint ensures the degenerate n=1 case returns a valid
// single-cluster partition instead of panicking.
func TestGMMCluster_SinglePoint(t *testing.T) {
	labels, centroids := gmmCluster([][]float64{{0.5, -0.3}}, ClusterSpec{Method: ClusteringGMM, MinClusters: 2, MaxClusters: 4})
	if len(labels) != 1 || labels[0] != 0 {
		t.Fatalf("single-point labels = %v, want [0]", labels)
	}
	if len(centroids) != 1 {
		t.Fatalf("single-point centroids = %d, want 1", len(centroids))
	}
}
