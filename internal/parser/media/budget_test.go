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

func testBudget(maxSources int, maxImageBytes, maxTotalBytes int64) *Budget {
	return NewBudgetWithLimits(Limits{
		ImageMaxBytes:   maxImageBytes,
		MaxImagesPerRun: maxSources,
		MaxBytesPerRun:  maxTotalBytes,
	})
}

func reserveAndCommit(t *testing.T, budget *Budget, source string, size Size) bool {
	t.Helper()
	reservation, admission := budget.Reserve(source, size)
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

func TestBudgetRepeatedSourceConsumesOneItem(t *testing.T) {
	budget := testBudget(2, 16, 16)
	for attempt := 0; attempt < 5; attempt++ {
		reservation, admission := budget.Reserve("same.png", 4)
		if admission != Admitted {
			t.Fatalf("Reserve #%d = %v, want admitted", attempt+1, admission)
		}
		if !reservation.Commit(4) {
			t.Fatalf("Commit #%d rejected an admitted source", attempt+1)
		}
	}
	if budget.Count() != 1 || budget.totalBytes != 4 {
		t.Fatalf("sources=%d bytes=%d, want 1 source and 4 bytes", budget.Count(), budget.totalBytes)
	}
	if warnings := budget.Warnings(); len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
}

func TestBudgetSourceLimitExhaustion(t *testing.T) {
	budget := testBudget(2, 16, 16)
	for _, source := range []string{"a.png", "b.png"} {
		reservation, admission := budget.Reserve(source, UnknownSize)
		if admission != Admitted || !reservation.Commit(1) {
			t.Fatalf("source %q was not admitted", source)
		}
	}
	if _, admission := budget.Reserve("c.png", UnknownSize); admission != RejectedByCount {
		t.Fatalf("third source admission = %v, want count rejection", admission)
	}
	if _, admission := budget.Reserve("a.png", UnknownSize); admission != Admitted {
		t.Fatalf("already admitted source after exhaustion = %v, want admitted", admission)
	}
	if !strings.Contains(strings.Join(budget.Warnings(), "\n"), "2-item run limit") {
		t.Fatalf("warnings = %v, want the source-count warning", budget.Warnings())
	}
}

func TestBudgetDeclaredByteRejectionsDoNotConsumeSourceSlots(t *testing.T) {
	budget := testBudget(1, 8, 5)
	if _, admission := budget.Reserve("too-large-1.png", 9); admission != RejectedByBytes {
		t.Fatalf("per-image rejection = %v, want byte rejection", admission)
	}
	if _, admission := budget.Reserve("too-large-2.png", 6); admission != RejectedByBytes {
		t.Fatalf("total-byte rejection = %v, want byte rejection", admission)
	}
	if !reserveAndCommit(t, budget, "small.png", 1) {
		t.Fatal("byte-rejected sources exhausted the source budget")
	}
	if budget.Count() != 1 || budget.totalBytes != 1 {
		t.Fatalf("sources=%d bytes=%d, want 1 source and 1 byte", budget.Count(), budget.totalBytes)
	}
	if budget.oversizedImages != 1 || budget.totalOverflows != 1 {
		t.Fatalf("oversized=%d overflows=%d, want 1 and 1", budget.oversizedImages, budget.totalOverflows)
	}
}

func TestBudgetCommitRejectionsAreMemoized(t *testing.T) {
	budget := testBudget(4, 4, 8)
	reservation, admission := budget.Reserve("big.png", UnknownSize)
	if admission != Admitted {
		t.Fatalf("Reserve = %v, want admitted", admission)
	}
	for attempt := 0; attempt < 3; attempt++ {
		if reservation.Commit(5) {
			t.Fatal("Commit admitted an oversized payload")
		}
	}
	if _, admission := budget.Reserve("big.png", 5); admission != RejectedByBytes {
		t.Fatalf("repeated Reserve = %v, want byte rejection", admission)
	}
	if budget.oversizedImages != 1 || budget.totalBytes != 0 {
		t.Fatalf("oversized=%d bytes=%d, want one oversized skip and no bytes", budget.oversizedImages, budget.totalBytes)
	}
}

func TestBudgetTotalByteLimitDoesNotRecount(t *testing.T) {
	budget := testBudget(4, 8, 5)
	if !reserveAndCommit(t, budget, "a.png", 3) {
		t.Fatal("first payload should fit")
	}
	reservation, admission := budget.Reserve("b.png", UnknownSize)
	if admission != Admitted {
		t.Fatalf("second source admission = %v, want admitted", admission)
	}
	if reservation.Commit(3) {
		t.Fatal("Commit admitted a payload past the total byte limit")
	}
	if reservation.Commit(3) {
		t.Fatal("Commit recounted the rejected source")
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
	if !reserveAndCommit(t, budget, "exact.png", 4) {
		t.Fatal("exact-limit image should be admitted")
	}
	reservation, admission := budget.ReserveOccurrence(UnknownSize)
	if admission != Admitted || reservation.Commit(1) {
		t.Fatal("image after exact run limit should be omitted")
	}
}

func TestBudgetIncludedSourceIsNotRejectedByLaterDeclaration(t *testing.T) {
	budget := testBudget(2, 8, 5)
	if !reserveAndCommit(t, budget, "a.png", 3) {
		t.Fatal("source was not admitted")
	}
	reservation, admission := budget.Reserve("a.png", 9)
	if admission != Admitted || !reservation.Commit(3) {
		t.Fatal("later declaration changed an included source's state")
	}
	if budget.totalBytes != 3 || budget.Count() != 1 {
		t.Fatalf("sources=%d bytes=%d, want 1 source and 3 bytes", budget.Count(), budget.totalBytes)
	}
}

func TestBudgetInvalidSizes(t *testing.T) {
	budget := testBudget(2, 8, 8)
	if _, admission := budget.Reserve("invalid.png", -2); admission != RejectedInvalid {
		t.Fatalf("invalid declared size = %v, want invalid rejection", admission)
	}
	reservation, admission := budget.Reserve("valid.png", UnknownSize)
	if admission != Admitted {
		t.Fatalf("valid reservation = %v, want admitted", admission)
	}
	if reservation.Commit(-2) {
		t.Fatal("Commit admitted an invalid actual size")
	}
}

func TestBudgetWarningsAreStable(t *testing.T) {
	budget := testBudget(3, 4, 5)
	_, _ = budget.Reserve("too-large.png", 5)
	_, _ = budget.ReserveOccurrence(6)
	_, _ = budget.Reserve("a.png", UnknownSize)
	_, _ = budget.Reserve("b.png", UnknownSize)
	_, _ = budget.Reserve("c.png", UnknownSize)
	_, _ = budget.Reserve("d.png", UnknownSize)
	want := []string{
		"omitted 2 embedded image payload(s) larger than the 4-byte per-image limit",
		"stopped extracting embedded images after the 3-item run limit",
	}
	if got := budget.Warnings(); !reflect.DeepEqual(got, want) {
		t.Fatalf("warnings = %q, want %q", got, want)
	}
}
