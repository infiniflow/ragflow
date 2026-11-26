//go:build cgo

package native

import "testing"

func TestCPUResourceDetection(t *testing.T) {
	r := CPUResource{}
	if r.Name() != "cpu-memory" {
		t.Fatalf("name = %s", r.Name())
	}
	// Total should be detectable on a normal Linux host (cgroup or /proc).
	total := r.TotalMB()
	if total <= 0 {
		t.Fatalf("CPU total detection returned %d (expected > 0)", total)
	}
	t.Logf("CPU total detected: %d MB (cgroup path=%s)", total, cgroupV2Path())
	// Used is best-effort; >= 0 is fine.
	if used := r.UsedMB(); used < 0 {
		t.Fatalf("CPU used = %d", used)
	}
}

func TestAdaptiveKSharedFormula(t *testing.T) {
	// 12GB card equivalent, ~1.1GB weights: K should be ~16, larger than a
	// naive fixed 8, and scale with total.
	k8 := computeAdaptiveK(8192, 1150, 0.90, 560, 2, 64)
	k12 := computeAdaptiveK(12288, 1150, 0.90, 560, 2, 64)
	k24 := computeAdaptiveK(24576, 1150, 0.90, 560, 2, 64)
	if k12 < 12 || k12 > 20 {
		t.Fatalf("12GB K = %d, expected ~12-20", k12)
	}
	if !(k8 < k12 && k12 < k24) {
		t.Fatalf("K should scale with total: 8GB=%d 12GB=%d 24GB=%d", k8, k12, k24)
	}
	// A tiny resource (1 GiB) cannot cover the 1150 MB weights, so usable<=0
	// and the formula must clamp to kMin.
	if got := computeAdaptiveK(1<<10, 1150, 0.90, 560, 2, 64); got != 2 {
		t.Fatalf("tiny resource K=%d expected min 2", got)
	}
	t.Logf("adaptive K: 8GB=%d 12GB=%d 24GB=%d", k8, k12, k24)
}
