//go:build cgo

package native

import "testing"

func TestValidateInferenceConfig(t *testing.T) {
	cases := []struct {
		name                  string
		totalCores            int
		rawCPUCores           int
		concurrency           int
		wantCoresPerInference int
		wantTotalCPUCores     int
		wantErr               bool
	}{
		// N=0 (all cores) resolution.
		{"n_zero_k1", 8, 0, 1, 8, 8, false},
		{"n_zero_k2", 8, 0, 2, 4, 8, false},
		// Explicit N, K=1 -> all N cores per Run.
		{"n4_k1", 8, 4, 1, 4, 4, false},
		// Explicit N, K=2 -> N/2 cores per Run.
		{"n4_k2", 8, 4, 2, 2, 4, false},
		// When K exceeds N, each Run floors at 1 core (total = K, not N).
		{"n4_k8_floor", 8, 4, 8, 1, 4, false},
		{"n4_k4_floor", 8, 4, 4, 1, 4, false},
		// N == M.
		{"n_eq_m_k1", 4, 4, 1, 4, 4, false},
		// Rejections.
		{"k_zero_err", 8, 4, 0, 0, 0, true},
		{"k_negative_err", 8, 4, -1, 0, 0, true},
		{"k_gt_m_err", 8, 4, 9, 0, 0, true},
		{"n_gt_m_err", 8, 16, 1, 0, 0, true},
		{"n_negative_err", 8, -1, 1, 0, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotCores, gotTotal, err := ValidateInferenceConfig(c.totalCores, c.rawCPUCores, c.concurrency)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (cores=%d total=%d)", gotCores, gotTotal)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotCores != c.wantCoresPerInference {
				t.Errorf("coresPerInference = %d, want %d", gotCores, c.wantCoresPerInference)
			}
			if gotTotal != c.wantTotalCPUCores {
				t.Errorf("totalCPUCores = %d, want %d", gotTotal, c.wantTotalCPUCores)
			}
		})
	}
}

func TestSetIntraOpThreads(t *testing.T) {
	SetIntraOpThreads(4)
	if got := intraOpThreadCount(); got != 4 {
		t.Fatalf("intraOpThreadCount = %d, want 4", got)
	}
	// Non-positive is clamped to 1.
	SetIntraOpThreads(0)
	if got := intraOpThreadCount(); got != 1 {
		t.Fatalf("intraOpThreadCount = %d, want 1 after clamp", got)
	}
	// Restore the process default so other tests are unaffected.
	SetIntraOpThreads(1)
}

// TestValidateInferenceConfigKNExceedsBudget pins that the CPU-core budget N is
// a hard ceiling: when K > N the per-Run intra-op thread count (max(1, N/K))
// floors at 1, so total occupancy would be K, exceeding N. ValidateInferenceConfig
// must reject this rather than silently oversubscribing the box.
func TestValidateInferenceConfigKNExceedsBudget(t *testing.T) {
	if _, _, err := ValidateInferenceConfig(8, 2, 8); err == nil {
		t.Fatal("expected error when concurrency K=8 exceeds cpu-core budget N=2")
	}
	// Sanity: K <= N is accepted and yields N/K intra-op threads.
	if c, total, err := ValidateInferenceConfig(8, 4, 2); err != nil {
		t.Fatalf("unexpected error for K<=N: %v", err)
	} else if c != 2 || total != 4 {
		t.Fatalf("coresPerInference=%d totalCPUCores=%d, want 2/4", c, total)
	}
}
