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
	"strings"
	"testing"
)

func testBudget(maxSources, maxImageBytes int, maxTotalBytes int64) *Budget {
	return &Budget{
		maxSources:    maxSources,
		maxImageBytes: maxImageBytes,
		maxTotalBytes: maxTotalBytes,
		sources:       make(map[string]*budgetSource),
	}
}

func TestNewBudgetUsesRunCaps(t *testing.T) {
	t.Setenv("RAGFLOW_MEDIA_MAX_IMAGES_PER_RUN", "7")
	t.Setenv("RAGFLOW_MEDIA_MAX_BYTES_PER_RUN", "1234")
	budget := NewBudget()
	if budget.maxSources != 7 || budget.maxTotalBytes != 1234 {
		t.Fatalf("budget limits = %d sources / %d bytes, want 7 / 1234", budget.maxSources, budget.maxTotalBytes)
	}
}

func TestBudgetRepeatedSourceConsumesOneItem(t *testing.T) {
	budget := testBudget(2, 16, 16)
	for attempt := 0; attempt < 5; attempt++ {
		if !budget.Reserve("same.png") {
			t.Fatalf("Reserve #%d rejected an admitted source", attempt+1)
		}
		if !budget.Include("same.png", 4) {
			t.Fatalf("Include #%d rejected an admitted source", attempt+1)
		}
	}
	if budget.sourcesUsed != 1 || budget.totalBytes != 4 {
		t.Fatalf("sources=%d bytes=%d, want 1 source and 4 bytes", budget.sourcesUsed, budget.totalBytes)
	}
	if warnings := budget.Warnings(); len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
}

func TestBudgetSourceLimitExhaustion(t *testing.T) {
	budget := testBudget(2, 16, 16)
	if !budget.Reserve("a.png") || !budget.Reserve("b.png") {
		t.Fatal("Reserve rejected a source under the limit")
	}
	if budget.Reserve("c.png") {
		t.Fatal("Reserve admitted a source past the count limit")
	}
	if budget.Reserve("c.png") {
		t.Fatal("Reserve admitted an exhausted source a second time")
	}
	if !budget.Reserve("a.png") {
		t.Fatal("Reserve rejected an already admitted source after exhaustion")
	}
	if !strings.Contains(strings.Join(budget.Warnings(), "\n"), "2-item run limit") {
		t.Fatalf("warnings = %v, want the source-count warning", budget.Warnings())
	}
}

func TestBudgetPerImageLimitDoesNotRecount(t *testing.T) {
	budget := testBudget(4, 4, 16)
	if !budget.Reserve("big.png") {
		t.Fatal("Reserve rejected the source")
	}
	if budget.Include("big.png", 5) {
		t.Fatal("Include admitted an oversized payload")
	}
	for attempt := 0; attempt < 3; attempt++ {
		if budget.Include("big.png", 5) {
			t.Fatal("Include admitted the rejected source again")
		}
	}
	if budget.oversizedImages != 1 || budget.totalBytes != 0 {
		t.Fatalf("oversized=%d bytes=%d, want one oversized skip and no bytes", budget.oversizedImages, budget.totalBytes)
	}
	if !strings.Contains(strings.Join(budget.Warnings(), "\n"), "larger than the 4-byte per-image limit") {
		t.Fatalf("warnings = %v", budget.Warnings())
	}
}

func TestBudgetTotalByteLimitDoesNotRecount(t *testing.T) {
	budget := testBudget(4, 8, 5)
	if !budget.Reserve("a.png") || !budget.Reserve("b.png") {
		t.Fatal("Reserve rejected a source")
	}
	if !budget.Include("a.png", 3) {
		t.Fatal("Include rejected a payload under the total limit")
	}
	if budget.Include("b.png", 3) {
		t.Fatal("Include admitted a payload past the total byte limit")
	}
	if budget.Include("b.png", 3) {
		t.Fatal("Include recounted the rejected source")
	}
	if budget.totalOverflows != 1 || budget.totalBytes != 3 {
		t.Fatalf("overflows=%d bytes=%d, want one overflow and 3 bytes", budget.totalOverflows, budget.totalBytes)
	}
	if !strings.Contains(strings.Join(budget.Warnings(), "\n"), "5-byte run image budget") {
		t.Fatalf("warnings = %v", budget.Warnings())
	}
}

func TestBudgetReserveBytesChecksWithoutCharging(t *testing.T) {
	budget := testBudget(4, 8, 5)
	if !budget.Reserve("a.png") {
		t.Fatal("Reserve rejected the source")
	}
	if !budget.ReserveBytes("a.png", 5) {
		t.Fatal("ReserveBytes rejected a payload at the total limit")
	}
	if budget.totalBytes != 0 {
		t.Fatalf("ReserveBytes charged %d bytes before Include", budget.totalBytes)
	}
	if !budget.Include("a.png", 5) {
		t.Fatal("Include rejected the reserved payload")
	}
	if !budget.Include("a.png", 5) {
		t.Fatal("Include rejected an already included source")
	}
	if budget.totalBytes != 5 {
		t.Fatalf("total bytes = %d, want 5", budget.totalBytes)
	}
}

func TestBudgetReserveBytesRejectsBeforeRead(t *testing.T) {
	budget := testBudget(4, 8, 5)
	if !budget.Reserve("big.png") {
		t.Fatal("Reserve rejected the source")
	}
	if budget.ReserveBytes("big.png", 6) {
		t.Fatal("ReserveBytes admitted an entry past the total limit")
	}
	if budget.ReserveBytes("big.png", 6) {
		t.Fatal("ReserveBytes recounted a rejected entry")
	}
	if budget.totalOverflows != 1 || budget.totalBytes != 0 {
		t.Fatalf("overflows=%d bytes=%d, want one overflow and no bytes", budget.totalOverflows, budget.totalBytes)
	}
	if !strings.Contains(strings.Join(budget.Warnings(), "\n"), "5-byte run image budget") {
		t.Fatalf("warnings = %v", budget.Warnings())
	}
}

func TestBudgetRejectedBytesDoNotConsumeSourceSlots(t *testing.T) {
	budget := testBudget(2, 8, 5)
	if budget.ReserveBytes("too-large-1.png", 6) {
		t.Fatal("first oversized source was admitted")
	}
	if budget.ReserveBytes("too-large-2.png", 6) {
		t.Fatal("second oversized source was admitted")
	}
	if !budget.ReserveBytes("small.png", 1) || !budget.Reserve("small.png") || !budget.Include("small.png", 1) {
		t.Fatal("byte-rejected sources exhausted the source budget")
	}
	if budget.sourcesUsed != 1 || budget.totalBytes != 1 {
		t.Fatalf("sources=%d bytes=%d, want 1 source and 1 byte", budget.sourcesUsed, budget.totalBytes)
	}
	if budget.totalOverflows != 2 {
		t.Fatalf("total overflows = %d, want 2", budget.totalOverflows)
	}
}

func TestBudgetReservePreservesExistingAdmissionState(t *testing.T) {
	budget := testBudget(2, 8, 5)
	if !budget.ReserveBytes("a.png", 3) {
		t.Fatal("ReserveBytes rejected a valid source")
	}
	if !budget.Include("a.png", 3) {
		t.Fatal("source was not admitted")
	}
	if !budget.Reserve("a.png") {
		t.Fatal("second Reserve rejected an included source")
	}
	if !budget.Include("a.png", 3) {
		t.Fatal("Reserve overwrote the included state")
	}
	if budget.totalBytes != 3 || budget.sourcesUsed != 1 {
		t.Fatalf("sources=%d bytes=%d, want 1 source and 3 bytes", budget.sourcesUsed, budget.totalBytes)
	}
}

func TestBudgetNoteOversizedDoesNotConsumeItem(t *testing.T) {
	budget := testBudget(2, 4, 16)
	if !budget.Reserve("a.png") {
		t.Fatal("Reserve rejected the first source")
	}
	budget.NoteOversized()
	budget.NoteOversized()
	if !budget.Reserve("b.png") {
		t.Fatal("NoteOversized consumed the item budget")
	}
	if budget.Reserve("c.png") {
		t.Fatal("Reserve admitted a third source past the two-item limit")
	}
	if budget.oversizedImages != 2 {
		t.Fatalf("oversized = %d, want 2", budget.oversizedImages)
	}
}

func TestBudgetNilReceiver(t *testing.T) {
	var budget *Budget
	if !budget.Reserve("a.png") || !budget.Include("a.png", 1) {
		t.Fatal("nil budget rejected a source")
	}
	budget.NoteOversized()
	if warnings := budget.Warnings(); warnings != nil {
		t.Fatalf("warnings = %v, want nil", warnings)
	}
}
