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

package agentic_rag

import "testing"

// TestNodeClockKeepsItsFloorOnASpentBudget pins the property that decides whether a run
// whose research budget is already gone still retrieves anything: the clock is
// min(budget, max(floor, room)), with NO "room gone -> zero" branch.
//
// Measured 2026-09-20 (glm-4.6, a single-question MES ransomware run): the fan-out plus the
// slot-table decomposition spent 185.4s of the 180s budget, so the prefetch arrived with
// room == RemainingS() - MinRoundHeadroomS == -50. Returning zero there built a
// context.WithTimeout(ctx, 0); FanoutSearch's first act is a ctx.Done() check, so it
// admitted nothing and logged "Added 0 new passages" without running a single search. The
// run then reached the answer with an empty pool — the draft had nothing to summarise, the
// SCA had nothing to judge, and the reply was the no-evidence hedge — while the searches it
// never ran took 25ms each on the same corpus.
func TestNodeClockKeepsItsFloorOnASpentBudget(t *testing.T) {
	cases := []struct {
		name                   string
		budgetS, floorS, roomS float64
		want                   float64
	}{
		// The prefetch: its own 90s cap, floored at 10s.
		{"prefetch with the budget spent", PrefetchTimeoutS, 10.0, -50.0, 10.0},
		{"prefetch with no room at all", PrefetchTimeoutS, 10.0, 0, 10.0},
		{"prefetch with less room than its floor", PrefetchTimeoutS, 10.0, 4.0, 10.0},
		{"prefetch with room left", PrefetchTimeoutS, 10.0, 40.0, 40.0},
		{"prefetch capped by its own budget", PrefetchTimeoutS, 10.0, 600.0, 90.0},
		// The draft and the SCA review: 60s each, floored at 15s.
		{"draft with the budget spent", DraftTimeoutS, 15.0, -10.0, 15.0},
		{"sca with the budget spent", SCATimeoutS, 15.0, -10.0, 15.0},
		// The gap -> query rewrite call: 45s, floored at 10s.
		{"rewrite with the budget spent", RewriteTimeoutS, 10.0, -10.0, 10.0},
		// The slot research pass has no floor of its own — its caller floors it at 20s, and
		// only reaches it with MinRoundHeadroomS still on the clock.
		{"research pass with the budget spent", PassTimeoutS, 0, -25.0, 0},
	}
	for _, tc := range cases {
		if got := nodeClock(tc.budgetS, tc.floorS, tc.roomS); got != tc.want {
			t.Errorf("%s: nodeClock(%g, %g, %g) = %g, want %g",
				tc.name, tc.budgetS, tc.floorS, tc.roomS, got, tc.want)
		}
	}
}
