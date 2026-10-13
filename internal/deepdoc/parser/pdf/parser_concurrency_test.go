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

// TestDeepDocConcurrencyDefaultOne pins the process inference budget default:
// when nothing configures it, DeepDocConcurrency() returns 1.
func TestDeepDocConcurrencyDefaultOne(t *testing.T) {
	if got := DeepDocConcurrency(); got != 1 {
		t.Fatalf("initial DeepDocConcurrency() = %d, want 1", got)
	}

	orig := DeepDocConcurrency()
	t.Cleanup(func() { SetDeepDocConcurrency(orig) })

	SetDeepDocConcurrency(7)
	if got := DeepDocConcurrency(); got != 7 {
		t.Fatalf("DeepDocConcurrency() after SetDeepDocConcurrency(7) = %d, want 7", got)
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

// TestPageConcurrencyDefaultsToTwo pins the per-document page concurrency
// default: when nothing configures it, PageConcurrency() returns 2.
func TestPageConcurrencyDefaultsToTwo(t *testing.T) {
	orig := PageConcurrency()
	t.Cleanup(func() { SetPageConcurrency(orig) })

	SetPageConcurrency(2)
	if got := PageConcurrency(); got != 2 {
		t.Fatalf("PageConcurrency() = %d, want 2", got)
	}
}

// TestSetPageConcurrency pins the setter's clamp: a non-positive value is
// floored to 1, a value above the max is clamped to MaxPageConcurrency, and a
// value inside the range is honoured exactly.
func TestSetPageConcurrency(t *testing.T) {
	orig := PageConcurrency()
	t.Cleanup(func() { SetPageConcurrency(orig) })

	SetPageConcurrency(0)
	if got := PageConcurrency(); got != 1 {
		t.Fatalf("PageConcurrency() after SetPageConcurrency(0) = %d, want 1", got)
	}
	SetPageConcurrency(-5)
	if got := PageConcurrency(); got != 1 {
		t.Fatalf("PageConcurrency() after SetPageConcurrency(-5) = %d, want 1", got)
	}
	SetPageConcurrency(maxPageConcurrency + 10)
	if got := PageConcurrency(); got != maxPageConcurrency {
		t.Fatalf("PageConcurrency() after SetPageConcurrency(max+10) = %d, want %d", got, maxPageConcurrency)
	}
	SetPageConcurrency(7)
	if got := PageConcurrency(); got != 7 {
		t.Fatalf("PageConcurrency() after SetPageConcurrency(7) = %d, want 7", got)
	}
}

// TestPageConcurrencyIndependentOfInferenceBudget pins that the shared page
// worker pool is sized from the per-document page concurrency (N), not the
// process inference budget: N may exceed the inference budget, and the pool is
// still created at N (extra workers merely queue rendered bitmaps).
func TestPageConcurrencyIndependentOfInferenceBudget(t *testing.T) {
	origBudget := DeepDocConcurrency()
	origPage := PageConcurrency()
	t.Cleanup(func() {
		SetDeepDocConcurrency(origBudget)
		SetPageConcurrency(origPage)
	})

	SetDeepDocConcurrency(1)
	SetPageConcurrency(8)
	if got := defaultPageWorkerCount(); got != 8 {
		t.Fatalf("defaultPageWorkerCount() = %d, want 8 (page concurrency independent of inference budget)", got)
	}
	if got := PageConcurrency(); got != 8 {
		t.Fatalf("PageConcurrency() = %d, want 8", got)
	}
}
