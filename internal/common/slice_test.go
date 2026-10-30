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

package common

import "testing"

// TestSlicePage_NoOverflowPanic guards the review-Critical integer overflow:
// an unbounded positive page/page_size must never overflow (page-1)*pageSize to
// a negative start and panic; out-of-range pages return nil.
func TestSlicePage_NoOverflowPanic(t *testing.T) {
	s := []int{1, 2, 3, 4, 5}
	// A huge page must return nil, not panic on an overflowing offset.
	if got := SlicePage(s, 1<<40, 1<<40); got != nil {
		t.Fatalf("huge page/pageSize = %v, want nil", got)
	}
	if got := SlicePage(s, 2, 3); len(got) != 2 || got[0] != 4 || got[1] != 5 {
		t.Fatalf("page2/size3 = %v, want [4 5]", got)
	}
	if got := SlicePage(s, 3, 2); len(got) != 1 || got[0] != 5 {
		t.Fatalf("page3/size2 (start 4) = %v, want [5]", got)
	}
	if got := SlicePage(s, 4, 2); got != nil {
		t.Fatalf("page4/size2 (start 6 >= len 5) = %v, want nil", got)
	}
	// Empty/no-paging semantics preserved (empty input returns the empty slice,
	// never a panic).
	if got := SlicePage([]int{}, 1, 10); len(got) != 0 {
		t.Fatalf("empty slice = %v, want empty", got)
	}
}

// TestSlicePage_NeverExposesElementsPastLen pins the straddling page: slicing a
// page whose high bound crosses len(s) must stop at the data, not read into the
// backing array's spare capacity.
func TestSlicePage_NeverExposesElementsPastLen(t *testing.T) {
	full := make([]int, 5, 20)
	for i := range full {
		full[i] = i + 1
	}
	got := SlicePage(full, 2, 3)
	if len(got) != 2 || got[0] != 4 || got[1] != 5 {
		t.Fatalf("page2/size3 on len 5 cap 20 = %v, want [4 5]", got)
	}
	// A capacity-tight slice must not panic on the same request.
	tight := append([]int(nil), full...)
	if got := SlicePage(tight, 2, 3); len(got) != 2 {
		t.Fatalf("capacity-tight page2/size3 = %v, want 2 items", got)
	}
}
