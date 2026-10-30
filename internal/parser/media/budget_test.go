// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package media

import (
	"reflect"
	"strings"
	"testing"
)

func testBudget(maxItems int, maxImageBytes, maxTotalBytes int64) *Budget {
	return NewBudgetWithLimits(Limits{
		ImageMaxBytes:   maxImageBytes,
		MaxImagesPerRun: maxItems,
		MaxBytesPerRun:  maxTotalBytes,
	})
}

func reserveAndCommit(t *testing.T, budget *Budget, size Size) bool {
	t.Helper()
	reservation, admission := budget.Reserve(size)
	if admission != Admitted {
		return false
	}
	return reservation.Commit(size)
}

func TestNewBudgetUsesRunCaps(t *testing.T) {
	t.Setenv("RAGFLOW_MEDIA_IMAGE_MAX_BYTES", "2048")
	t.Setenv("RAGFLOW_MEDIA_MAX_IMAGES_PER_RUN", "7")
	t.Setenv("RAGFLOW_MEDIA_MAX_BYTES_PER_RUN", "1234")
	budget := NewBudget()
	want := Limits{ImageMaxBytes: 2048, MaxImagesPerRun: 7, MaxBytesPerRun: 1234}
	if budget.limits != want {
		t.Fatalf("budget limits = %+v, want %+v", budget.limits, want)
	}
}

func TestBudgetItemLimitExhaustion(t *testing.T) {
	budget := testBudget(2, 16, 16)
	for attempt := 0; attempt < 2; attempt++ {
		reservation, admission := budget.Reserve(UnknownSize)
		if admission != Admitted || !reservation.Commit(1) {
			t.Fatalf("item #%d was not admitted", attempt+1)
		}
	}
	if _, admission := budget.Reserve(UnknownSize); admission != RejectedByCount {
		t.Fatalf("third item admission = %v, want count rejection", admission)
	}
	if !strings.Contains(strings.Join(budget.Warnings(), "\n"), "2-item run limit") {
		t.Fatalf("warnings = %v, want the item-count warning", budget.Warnings())
	}
}

func TestBudgetDeclaredByteRejectionsDoNotConsumeItemSlots(t *testing.T) {
	budget := testBudget(1, 8, 5)
	if _, admission := budget.Reserve(9); admission != RejectedByBytes {
		t.Fatalf("per-image rejection = %v, want byte rejection", admission)
	}
	if _, admission := budget.Reserve(6); admission != RejectedByBytes {
		t.Fatalf("total-byte rejection = %v, want byte rejection", admission)
	}
	if !reserveAndCommit(t, budget, 1) {
		t.Fatal("byte-rejected items exhausted the item budget")
	}
	if budget.Count() != 1 || budget.totalBytes != 1 {
		t.Fatalf("items=%d bytes=%d, want 1 item and 1 byte", budget.Count(), budget.totalBytes)
	}
	if budget.oversizedImages != 1 || budget.totalOverflows != 1 {
		t.Fatalf("oversized=%d overflows=%d, want 1 and 1", budget.oversizedImages, budget.totalOverflows)
	}
}

func TestBudgetCommitRejectionsAreStable(t *testing.T) {
	budget := testBudget(4, 4, 8)
	reservation, admission := budget.Reserve(UnknownSize)
	if admission != Admitted {
		t.Fatalf("Reserve = %v, want admitted", admission)
	}
	for attempt := 0; attempt < 3; attempt++ {
		if reservation.Commit(5) {
			t.Fatal("Commit admitted an oversized payload")
		}
	}
	if budget.oversizedImages != 1 || budget.totalBytes != 0 {
		t.Fatalf("oversized=%d bytes=%d, want one oversized skip and no bytes", budget.oversizedImages, budget.totalBytes)
	}
}

func TestBudgetTotalByteLimitDoesNotRecount(t *testing.T) {
	budget := testBudget(4, 8, 5)
	if !reserveAndCommit(t, budget, 3) {
		t.Fatal("first payload should fit")
	}
	reservation, admission := budget.Reserve(UnknownSize)
	if admission != Admitted {
		t.Fatalf("second item admission = %v, want admitted", admission)
	}
	if reservation.Commit(3) {
		t.Fatal("Commit admitted a payload past the total byte limit")
	}
	if reservation.Commit(3) {
		t.Fatal("Commit recounted the rejected item")
	}
	if budget.totalOverflows != 1 || budget.totalBytes != 3 {
		t.Fatalf("overflows=%d bytes=%d, want one overflow and 3 bytes", budget.totalOverflows, budget.totalBytes)
	}
}

func TestBudgetOccurrenceConsumesCountBeforeByteAdmission(t *testing.T) {
	budget := testBudget(1, 8, 5)
	if _, admission := budget.ReserveOccurrence(6); admission != RejectedByBytes {
		t.Fatalf("byte rejection = %v, want rejected by bytes", admission)
	}
	if budget.Count() != 1 {
		t.Fatalf("count = %d, want the rejected occurrence to consume one slot", budget.Count())
	}
	if _, admission := budget.ReserveOccurrence(UnknownSize); admission != RejectedByCount {
		t.Fatalf("next occurrence = %v, want rejected by count", admission)
	}
}

func TestBudgetOccurrenceKeepsWalkingAfterByteRejection(t *testing.T) {
	budget := testBudget(3, 4, 5)
	reservation, admission := budget.ReserveOccurrence(UnknownSize)
	if admission != Admitted || !reservation.Commit(3) {
		t.Fatal("first occurrence should fit")
	}
	reservation, admission = budget.ReserveOccurrence(UnknownSize)
	if admission != Admitted {
		t.Fatalf("second occurrence admission = %v, want admitted", admission)
	}
	if reservation.Commit(3) {
		t.Fatal("second occurrence exceeded the byte limit")
	}
	reservation, admission = budget.ReserveOccurrence(UnknownSize)
	if admission != Admitted {
		t.Fatalf("third occurrence admission = %v, want admitted", admission)
	}
	if !reservation.Commit(2) {
		t.Fatal("third occurrence should fit after the byte rejection")
	}
	if budget.Count() != 3 || budget.totalBytes != 5 {
		t.Fatalf("count=%d bytes=%d, want 3 and 5", budget.Count(), budget.totalBytes)
	}
}

func TestBudgetExactByteLimits(t *testing.T) {
	budget := testBudget(2, 4, 4)
	if !reserveAndCommit(t, budget, 4) {
		t.Fatal("exact-limit image should be admitted")
	}
	reservation, admission := budget.ReserveOccurrence(UnknownSize)
	if admission != Admitted || reservation.Commit(1) {
		t.Fatal("image after exact run limit should be omitted")
	}
}

func TestBudgetInvalidSizes(t *testing.T) {
	budget := testBudget(2, 8, 8)
	if _, admission := budget.Reserve(-2); admission != RejectedInvalid {
		t.Fatalf("invalid declared size = %v, want invalid rejection", admission)
	}
	reservation, admission := budget.Reserve(UnknownSize)
	if admission != Admitted {
		t.Fatalf("valid reservation = %v, want admitted", admission)
	}
	if reservation.Commit(-2) {
		t.Fatal("Commit admitted an invalid actual size")
	}
}

func TestBudgetWarningsAreStable(t *testing.T) {
	budget := testBudget(3, 4, 5)
	_, _ = budget.Reserve(5)
	_, _ = budget.ReserveOccurrence(6)
	_, _ = budget.Reserve(UnknownSize)
	_, _ = budget.Reserve(UnknownSize)
	_, _ = budget.Reserve(UnknownSize)
	_, _ = budget.Reserve(UnknownSize)
	want := []string{
		"omitted 2 embedded image payload(s) larger than the 4-byte per-image limit",
		"stopped extracting embedded images after the 3-item run limit",
	}
	if got := budget.Warnings(); !reflect.DeepEqual(got, want) {
		t.Fatalf("warnings = %q, want %q", got, want)
	}
}
