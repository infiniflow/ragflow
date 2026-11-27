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

import "sort"

// ModelBudget describes one model's measured capacity profile, used by the
// adaptive session allocator.
type ModelBudget struct {
	Name         string
	PeakRPS      float64
	Saturation   int
	SessionMemMB int64
}

// allocateSessions computes per-model session counts that maximize total
// throughput within a VRAM budget (bounded knapsack by value density).
//
// Objective: maximize sum_i(n_i * e_i) where e_i = PeakRPS_i / Saturation_i
// (throughput contributed by one session), subject to sum_i(n_i * mem_i) <=
// budget and 1 <= n_i <= Saturation_i.
//
// Because e_i is linear up to Saturation_i (measured), the fractional-knapsack
// greedy is optimal: sort models by value density e_i/mem_i descending and
// allocate each up to its saturation cap while budget remains. Every model
// gets at least one session even when the budget is below the minimum sum.
func allocateSessions(models []ModelBudget, budgetMB int64) map[string]int {
	result := make(map[string]int, len(models))

	// Minimum guarantee: one session per model. This may exceed budget when
	// the budget is smaller than the sum of all session memories — that is
	// intentional: a model with zero sessions can never serve a request.
	remaining := budgetMB
	for _, m := range models {
		result[m.Name] = 1
		remaining -= m.SessionMemMB
	}

	// Rank by value density: rps-per-session / MB-per-session.
	type ranked struct {
		model   ModelBudget
		density float64
	}
	ranks := make([]ranked, 0, len(models))
	for _, m := range models {
		if m.Saturation < 1 {
			m.Saturation = 1
		}
		if m.SessionMemMB <= 0 {
			continue
		}
		e := m.PeakRPS / float64(m.Saturation)
		ranks = append(ranks, ranked{model: m, density: e / float64(m.SessionMemMB)})
	}
	sort.Slice(ranks, func(i, j int) bool { return ranks[i].density > ranks[j].density })

	// Greedy: add sessions to the highest-density model first, up to saturation.
	for _, r := range ranks {
		m := r.model
		extra := 0
		for remaining >= m.SessionMemMB && result[m.Name]+extra < m.Saturation {
			remaining -= m.SessionMemMB
			extra++
		}
		result[m.Name] += extra
	}
	return result
}
