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

import (
	"sync"
	"testing"
)

func TestSessionPoolSetMaxFree(t *testing.T) {
	p := newSessionPool[string, *fakePooledSession](0, 4)
	if p.maxFree.Load() != 4 {
		t.Fatalf("initial maxFree = %d, want 4", p.maxFree.Load())
	}
	p.SetMaxFree(8)
	if p.maxFree.Load() != 8 {
		t.Fatalf("after SetMaxFree(8): maxFree = %d, want 8", p.maxFree.Load())
	}
	p.SetMaxFree(2)
	if p.maxFree.Load() != 2 {
		t.Fatalf("after SetMaxFree(2): maxFree = %d, want 2", p.maxFree.Load())
	}
}

func TestApplySessionBudget(t *testing.T) {
	// Apply a calibrated budget to all pools; maxFree per model must match.
	budget := SessionBudget{
		DLA: 4, TSR: 2, DET: 8, REC: 4,
	}
	saved := snapshotPoolMaxFree()
	t.Cleanup(restorePoolMaxFree(saved))

	ApplySessionBudget(budget)

	got := snapshotPoolMaxFree()
	// DLA and TSR now have independent pools and independent caps.
	if got["dla"] != 4 || got["tsr"] != 2 || got["det"] != 8 || got["rec"] != 4 {
		t.Fatalf("applied budget %v, got %v", budget, got)
	}
}

func TestApplySessionBudgetZeroKeepsCurrent(t *testing.T) {
	// A zero entry means "not calibrated" — must not wipe the pool's setting.
	budget := SessionBudget{DLA: 0, TSR: 0, DET: 0, REC: 0}
	saved := snapshotPoolMaxFree()
	t.Cleanup(restorePoolMaxFree(saved))

	ApplySessionBudget(budget)

	got := snapshotPoolMaxFree()
	for k, v := range saved {
		if got[k] != v {
			t.Errorf("zero budget changed %s: %d -> %d", k, v, got[k])
		}
	}
}

// TestSetMaxFreeConcurrentSnapshot is a race-detector regression guard. The
// production release path reads the idle cap while SetMaxFree may update it;
// the cap must be atomic so concurrent read/write is race-free. Under -race
// this test fails against a plain int field and passes with an atomic one.
func TestSetMaxFreeConcurrentSnapshot(t *testing.T) {
	saved := snapshotPoolMaxFree()
	t.Cleanup(restorePoolMaxFree(saved))
	const iterations = 2000
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			detSessions.SetMaxFree(i%8 + 1)
		}
		close(stop)
	}()
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				snapshotPoolMaxFree()
			}
		}
	}()
	wg.Wait()
}

// TestApplySessionBudgetSeparateDLATSR asserts DLA and TSR get INDEPENDENT
// idle caps. They use separate pools (different model + shape); a single
// shared max(DLA,TSR) cap over-retains both and promotes TSR to DLA's quota.
//
// Adversarial fixture: DLA != TSR (4 vs 2). A shared-cap implementation gives
// both 4 and fails the TSR assertion.
func TestApplySessionBudgetSeparateDLATSR(t *testing.T) {
	saved := snapshotPoolMaxFree()
	t.Cleanup(restorePoolMaxFree(saved))

	ApplySessionBudget(SessionBudget{DLA: 4, TSR: 2, DET: 8, REC: 4})

	got := snapshotPoolMaxFree()
	if got["dla"] != 4 {
		t.Errorf("DLA cap = %d, want 4", got["dla"])
	}
	if got["tsr"] != 2 {
		t.Errorf("TSR cap = %d, want 2 (must not inherit DLA's 4)", got["tsr"])
	}
	if got["det"] != 8 || got["rec"] != 4 {
		t.Errorf("DET/REC caps = %d/%d, want 8/4", got["det"], got["rec"])
	}
}

// TestCollapsedKeyPoolDefaultCapacity asserts that after collapsing all shapes
// onto one key, the default idle cap is large enough to retain sessions for
// typical concurrent inference. The old per-shape caps (rec=1, det=4) were
// totals across 96/24 distinct key-pools; with a single key they cap the whole
// pool at 1/4, so concurrent Runs past that rebuild sessions on every Get.
//
// Adversarial fixture: assert the floor is a real concurrency value (>=16),
// which fails against the old per-shape defaults regardless of budget file.
func TestCollapsedKeyPoolDefaultCapacity(t *testing.T) {
	const minConcurrentIdle = 16
	if got := int(recSessions.maxFree.Load()); got < minConcurrentIdle {
		t.Errorf("recSessions default maxFree = %d, want >= %d (single key must retain concurrent sessions)", got, minConcurrentIdle)
	}
	if got := int(detSessions.maxFree.Load()); got < minConcurrentIdle {
		t.Errorf("detSessions default maxFree = %d, want >= %d (single key must retain concurrent sessions)", got, minConcurrentIdle)
	}
}
