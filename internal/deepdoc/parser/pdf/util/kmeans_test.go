package util

import (
	"math"
	"testing"
)

func TestKMeans1D(t *testing.T) {
	t.Run("single cluster", func(t *testing.T) {
		data := []float64{10, 12, 11, 9, 13}
		labels, centroids := KMeans1D(data, 1)
		if len(centroids) != 1 {
			t.Fatalf("expected 1 centroid, got %d", len(centroids))
		}
		if len(labels) != len(data) {
			t.Fatalf("expected %d labels, got %d", len(data), len(labels))
		}
		for _, l := range labels {
			if l != 0 {
				t.Errorf("all labels should be 0, got %d", l)
			}
		}
	})

	t.Run("two well-separated clusters", func(t *testing.T) {
		data := []float64{10, 12, 11, 90, 92, 91}
		labels, centroids := KMeans1D(data, 2)
		if len(centroids) != 2 {
			t.Fatalf("expected 2 centroids, got %d", len(centroids))
		}
		if len(labels) != len(data) {
			t.Fatalf("expected %d labels, got %d", len(data), len(labels))
		}
		// First 3 points should be in one cluster, last 3 in the other
		if labels[0] == labels[3] {
			t.Error("far-apart points should be in different clusters")
		}
	})

	t.Run("k equals data points", func(t *testing.T) {
		data := []float64{10, 50, 90}
		_, centroids := KMeans1D(data, 3)
		if len(centroids) != 3 {
			t.Errorf("n=k: expected 3 centroids, got %d", len(centroids))
		}
		for i, c := range centroids {
			if math.Abs(c-data[i]) > 1e-6 {
				t.Errorf("centroid[%d]=%v, want %v", i, c, data[i])
			}
		}
	})

	t.Run("k greater than data points", func(t *testing.T) {
		data := []float64{10, 50}
		labels, centroids := KMeans1D(data, 5)
		if len(centroids) != 2 {
			t.Errorf("k>n: expected 2 centroids, got %d", len(centroids))
		}
		if labels[0] == labels[1] {
			t.Error("two distinct points should be in different clusters")
		}
	})
}

func TestSilhouette1D(t *testing.T) {
	t.Run("well-separated clusters", func(t *testing.T) {
		data := []float64{0, 1, 2, 100, 101, 102}
		labels := []int{0, 0, 0, 1, 1, 1}
		score := Silhouette1D(data, labels)
		if score < 0.8 {
			t.Errorf("well-separated score should be high, got %.3f", score)
		}
	})

	t.Run("overlapping clusters", func(t *testing.T) {
		data := []float64{0, 1, 0, 1, 0, 1}
		labels := []int{0, 0, 0, 1, 1, 1}
		score := Silhouette1D(data, labels)
		if score > 0.5 {
			t.Errorf("overlapping score should be low, got %.3f", score)
		}
	})

	t.Run("single cluster returns -1", func(t *testing.T) {
		data := []float64{1, 2, 3}
		labels := []int{0, 0, 0}
		if score := Silhouette1D(data, labels); score != -1 {
			t.Errorf("single cluster should return -1, got %.3f", score)
		}
	})
}
