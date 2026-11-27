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

func TestSessionPoolSetMaxFree(t *testing.T) {
	p := newSessionPool[string, *fakePooledSession](0, 4)
	if p.maxFree != 4 {
		t.Fatalf("initial maxFree = %d, want 4", p.maxFree)
	}
	p.SetMaxFree(8)
	if p.maxFree != 8 {
		t.Fatalf("after SetMaxFree(8): maxFree = %d, want 8", p.maxFree)
	}
	p.SetMaxFree(2)
	if p.maxFree != 2 {
		t.Fatalf("after SetMaxFree(2): maxFree = %d, want 2", p.maxFree)
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
	// DLA and TSR share the fixed-shape pool; its cap is max(dla, tsr).
	if got["dla"] != 4 || got["tsr"] != 4 || got["det"] != 8 || got["rec"] != 4 {
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
