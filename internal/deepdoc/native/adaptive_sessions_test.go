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

//go:build cgo

package native

import "testing"

func testModels() []ModelBudget {
	return []ModelBudget{
		{Name: "dla", PeakRPS: 15.5, Saturation: 8, SessionMemMB: 675},
		{Name: "tsr", PeakRPS: 24.35, Saturation: 4, SessionMemMB: 823},
		{Name: "det", PeakRPS: 71.0, Saturation: 8, SessionMemMB: 320},
		{Name: "rec", PeakRPS: 80.0, Saturation: 4, SessionMemMB: 268},
	}
}

func TestAllocateSessionsKnapsack(t *testing.T) {
	// 3060 budget: 12288*0.85 - 347 ~= 10098 MB.
	// Greedy by e_i/V_i (rps per session / mem per session) with saturation caps:
	//   REC 80/4=20 /268 -> 0.0746 ; DET 71/8=8.875 /320 -> 0.0277
	//   TSR 24.35/4=6.09 /823 -> 0.0074 ; DLA 15.5/8=1.94 /675 -> 0.0029
	// Order REC > DET > TSR > DLA, each capped at saturation:
	//   REC=4 (1072MB), DET=8 (2560MB), TSR=4 (3292MB), DLA=4 (2700MB) = 9624MB
	got := allocateSessions(testModels(), 10098)
	want := map[string]int{"dla": 4, "tsr": 4, "det": 8, "rec": 4}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("model %s: got %d sessions, want %d (full map: %v)", k, got[k], v, got)
		}
	}
}

func TestAllocateSessionsMinGuarantee(t *testing.T) {
	// Adversarial: budget smaller than the sum of one session per model.
	// Every model must still get at least 1 session (otherwise it can never serve).
	got := allocateSessions(testModels(), 500)
	for _, m := range testModels() {
		if got[m.Name] < 1 {
			t.Errorf("model %s got %d sessions with tiny budget; want >= 1", m.Name, got[m.Name])
		}
	}
}

func TestAllocateSessionsSaturationCap(t *testing.T) {
	// Adversarial: huge budget. No model may exceed its saturation point
	// M_i* — beyond that, extra sessions waste VRAM and do not add throughput.
	got := allocateSessions(testModels(), 10_000_000)
	for _, m := range testModels() {
		if got[m.Name] > m.Saturation {
			t.Errorf("model %s got %d sessions, exceeds saturation %d", m.Name, got[m.Name], m.Saturation)
		}
	}
}

func TestAllocateSessionsRespectsBudget(t *testing.T) {
	// Adversarial: total allocated VRAM must never exceed the budget.
	budget := int64(5000)
	models := testModels()
	got := allocateSessions(models, budget)
	var used int64
	for _, m := range models {
		used += int64(got[m.Name]) * m.SessionMemMB
	}
	// Min-guarantee can slightly exceed a sub-minimum budget by design; for a
	// budget above the min-guarantee sum, it must be respected.
	minSum := int64(0)
	for _, m := range models {
		minSum += m.SessionMemMB
	}
	if budget >= minSum && used > budget {
		t.Errorf("allocated %d MB exceeds budget %d MB (map: %v)", used, budget, got)
	}
}
