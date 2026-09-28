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

package pdf

import "testing"

// TestDeepDocConcurrencyDefaultsToFour pins the process inference budget
// default: when nothing configures it, DeepDocConcurrency() returns 4.
func TestDeepDocConcurrencyDefaultsToFour(t *testing.T) {
	orig := DeepDocConcurrency()
	t.Cleanup(func() { SetDeepDocConcurrency(orig) })

	SetDeepDocConcurrency(4)
	if got := DeepDocConcurrency(); got != 4 {
		t.Fatalf("DeepDocConcurrency() = %d, want 4", got)
	}
}

// TestSetDeepDocConcurrencyClampsToAtLeastOne pins the setter's floor: a
// non-positive value can never shrink the budget below 1, and a positive
// value is honoured exactly.
func TestSetDeepDocConcurrencyClampsToAtLeastOne(t *testing.T) {
	orig := DeepDocConcurrency()
	t.Cleanup(func() { SetDeepDocConcurrency(orig) })

	SetDeepDocConcurrency(0)
	if got := DeepDocConcurrency(); got != 1 {
		t.Fatalf("DeepDocConcurrency() after SetDeepDocConcurrency(0) = %d, want 1", got)
	}
	SetDeepDocConcurrency(-5)
	if got := DeepDocConcurrency(); got != 1 {
		t.Fatalf("DeepDocConcurrency() after SetDeepDocConcurrency(-5) = %d, want 1", got)
	}
	SetDeepDocConcurrency(9)
	if got := DeepDocConcurrency(); got != 9 {
		t.Fatalf("DeepDocConcurrency() after SetDeepDocConcurrency(9) = %d, want 9", got)
	}
}

// TestPageWorkerPoolTracksInferenceBudget pins the invariant that the shared
// page worker pool never exceeds the process inference budget.
func TestPageWorkerPoolTracksInferenceBudget(t *testing.T) {
	orig := DeepDocConcurrency()
	t.Cleanup(func() { SetDeepDocConcurrency(orig) })

	SetDeepDocConcurrency(7)
	if got, budget := defaultPageWorkerCount(), DeepDocConcurrency(); got > budget {
		t.Fatalf("defaultPageWorkerCount() = %d > DeepDocConcurrency() = %d", got, budget)
	}
}
