package deepdoc

import (
	"testing"

	doctype "ragflow/internal/deepdoc/parser/type"
)

// Adversarial (core regression for the explicit-zero fix): a user who sets
// inference_cpu_cores=0 means "all cores" per the documented contract. The
// old inline logic replaced an explicit zero with localCap, collapsing
// coresPerInference from totalCores/K to 1 and gutting local throughput.
// This table pins the distinguishing condition: explicit zero (rows 1-2)
// must keep resolvedN=0 so ValidateInferenceConfig resolves all cores,
// while an unset N (row 3) is still derived from K.
func TestResolveInferenceBudgetExplicitZeroMeansAllCores(t *testing.T) {
	cases := []struct {
		name            string
		k, rawN, cores  int
		explicitN       bool
		wantResolvedN   int
		wantLocalCap    int
		wantCoresPerInf int
	}{
		{"explicit zero K=1 -> all cores", 1, 0, 16, true, 0, 1, 16},
		{"explicit zero K=4 -> all cores", 4, 0, 16, true, 0, 4, 4},
		{"unset N -> derived from K", 4, 0, 16, false, 4, 4, 1},
		{"explicit positive N honoured", 4, 8, 16, true, 8, 4, 2},
		{"explicit N smaller than K caps local", 8, 2, 16, true, 2, 2, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := resolveInferenceBudget(tc.k, tc.rawN, tc.explicitN, tc.cores)
			if err != nil {
				t.Fatalf("resolveInferenceBudget(%d,%d,%v,%d) error: %v", tc.k, tc.rawN, tc.explicitN, tc.cores, err)
			}
			if b.resolvedN != tc.wantResolvedN {
				t.Fatalf("resolvedN = %d, want %d", b.resolvedN, tc.wantResolvedN)
			}
			if b.localCap != tc.wantLocalCap {
				t.Fatalf("localCap = %d, want %d", b.localCap, tc.wantLocalCap)
			}
			if b.coresPerInference != tc.wantCoresPerInf {
				t.Fatalf("coresPerInference = %d, want %d", b.coresPerInference, tc.wantCoresPerInf)
			}
			if b.K != tc.k {
				t.Fatalf("K = %d, want %d", b.K, tc.k)
			}
		})
	}
}

// Adversarial (error path): an explicitly configured N larger than the core
// budget, or K above cores, must still fail fast through the shared validator.
func TestResolveInferenceBudgetRejectsInvalidCombinations(t *testing.T) {
	// K above cores is legal by design: the outer gate uses the full K for
	// remote clients while the inner/local gate clamps to the core count.
	if b, err := resolveInferenceBudget(32, 0, false, 16); err != nil || b.localCap != 16 {
		t.Fatalf("K above cores should clamp localCap to cores: budget=%+v err=%v", b, err)
	}
	if _, err := resolveInferenceBudget(4, 64, true, 16); err == nil {
		t.Fatal("explicit N above cores must fail fast")
	}
}

// Adversarial (reusable-seam requirement): the registration entry point must
// accept explicit inputs and register the process gate WITHOUT depending on
// the in-process/native backend being available. This is the seam a remote
// inference deployment calls to bound client-side concurrency.
func TestRegisterInferenceBudgetIsBackendIndependent(t *testing.T) {
	if _, err := RegisterInferenceBudget(3, 0, false, 16); err != nil {
		t.Fatalf("RegisterInferenceBudget error: %v", err)
	}
	if got := doctype.InferenceLimit(); got != 3 {
		t.Fatalf("registered outer gate capacity = %d, want 3", got)
	}
}
