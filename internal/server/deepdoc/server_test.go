//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package deepdoc

import (
	"testing"

	goruntime "runtime"
)

// TestInferenceTotalCoresIsCgroupAware pins that the DeepDoc inference core
// budget passed to native.ValidateInferenceConfig is derived from
// runtime.GOMAXPROCS(0) (cgroup-quota aware in Go 1.25+) rather than
// runtime.NumCPU() (host affinity mask, which ignores a container's CPU limit).
// This keeps the fail-fast oversubscription guard effective under container CPU
// quotas: with NumCPU(), a 2-CPU-limit pod on a 64-core host would "resolve"
// inference_cpu_cores: 0 to 64 and let inference_concurrency: 16 pass validation,
// oversubscribing the box the check exists to prevent.
func TestInferenceTotalCoresIsCgroupAware(t *testing.T) {
	got := inferenceTotalCores()
	want := goruntime.GOMAXPROCS(0)
	if got != want {
		t.Fatalf("inferenceTotalCores() = %d, want runtime.GOMAXPROCS(0) = %d (cgroup-aware budget)", got, want)
	}
	if got < 1 {
		t.Fatalf("inferenceTotalCores() = %d, want >= 1", got)
	}
}

// Tests for local inference budget derivation. The single user-facing K drives
// a remote gate at K and a local gate at deriveLocalBudget(...), which must
// never exceed the machine's core count (no oversubscription) even when K is
// much larger (e.g. K=32 on 16 cores).
func TestDeriveLocalBudget(t *testing.T) {
	cases := []struct {
		name       string
		k          int
		totalCores int
		rawN       int
		explicitN  bool
		want       int
	}{
		{"K exceeds cores, N unset -> clamp to cores", 32, 16, 0, false, 16},
		{"K below cores, N unset -> K", 8, 16, 0, false, 8},
		{"K exceeds cores, explicit smaller N -> N", 32, 16, 4, true, 4},
		{"K exceeds cores, explicit N=0 (all cores) -> cores", 32, 16, 0, true, 16},
		{"K below cores, explicit larger N -> clamp to cores? no, N capped by K", 8, 16, 32, true, 8},
		{"zero cores floor to one", 32, 0, 0, false, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := deriveLocalBudget(tc.k, tc.totalCores, tc.rawN, tc.explicitN); got != tc.want {
				t.Fatalf("deriveLocalBudget(%d,%d,%d,%v) = %d, want %d",
					tc.k, tc.totalCores, tc.rawN, tc.explicitN, got, tc.want)
			}
		})
	}
}
