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

package service

import (
	"math"
	"sync"
)

// runProgress accumulates one pipeline run's component progress in memory.
// Percent is (completed components + sum of in-flight fractions) / total,
// guarded to never decrease: the flusher persists it as document.progress,
// and a regressing progress bar reads as a restart even though the run is
// advancing. Components are keyed by canvas component id, the identity both
// lifecycle events and fraction reports carry.
//
// The tracker is born empty on every run: the pipeline has no partial-resume
// entry point, so an MQ redelivery replays the whole graph and there is no
// prior state worth seeding from.
type runProgress struct {
	mu        sync.Mutex
	total     int
	done      map[string]struct{}
	frac      map[string]float64
	highWater float64
}

func newRunProgress() *runProgress {
	return &runProgress{
		done: make(map[string]struct{}),
		frac: make(map[string]float64),
	}
}

// SetTotal records the component-count denominator from OnComponentTotal.
func (r *runProgress) SetTotal(total int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.total = total
}

// MarkDone counts a component as complete on its lifecycle exit event and
// drops its fraction so the two never double-count.
func (r *runProgress) MarkDone(component string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.done[component] = struct{}{}
	delete(r.frac, component)
}

// SetFrac records an in-flight component's 0..1 completion fraction. Reports
// for already-completed components are ignored: a late fraction (e.g. a
// trailing page callback racing the exit event) must not pull percent down.
func (r *runProgress) SetFrac(component string, frac float64) {
	// NaN passes both clamps below and would then poison the sticky
	// high-water mark forever; drop it instead of storing it.
	if math.IsNaN(frac) {
		return
	}
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.done[component]; ok {
		return
	}
	r.frac[component] = frac
}

// Percent returns the run's 0..1 progress, clamped and monotonic. It returns
// 0 until OnComponentTotal supplies the denominator.
func (r *runProgress) Percent() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.total <= 0 {
		return 0
	}
	sum := float64(len(r.done))
	for _, f := range r.frac {
		sum += f
	}
	p := sum / float64(r.total)
	if p > 1 {
		p = 1
	}
	if p < r.highWater {
		return r.highWater
	}
	r.highWater = p
	return p
}
