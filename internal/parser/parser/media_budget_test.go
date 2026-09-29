package parser

import (
	"reflect"
	"testing"
)

func TestEmbeddedMediaBudgetReservedItemsAndWarnings(t *testing.T) {
	budget := &embeddedMediaBudget{maxImageBytes: 4, maxTotalBytes: 5, maxItems: 3}

	if !budget.reserveItem() || !budget.includeReservedSize(3) {
		t.Fatal("first image should fit")
	}
	if !budget.reserveItem() || budget.includeReservedSize(3) {
		t.Fatal("second image should exceed the document byte limit")
	}
	if !budget.reserveItem() || budget.includeReservedSize(5) {
		t.Fatal("third image should exceed the per-image byte limit")
	}
	if budget.reserveItem() {
		t.Fatal("fourth image should be rejected before its source is read")
	}
	if included, keepWalking := budget.include([]byte("x")); included || keepWalking {
		t.Fatalf("include after count exhaustion = (%v, %v), want (false, false)", included, keepWalking)
	}
	if budget.items != 3 || budget.totalBytes != 3 {
		t.Fatalf("budget counters = (%d items, %d bytes), want (3, 3)", budget.items, budget.totalBytes)
	}
	want := []string{
		"omitted 1 embedded image payload(s) larger than the 4-byte per-image limit",
		"omitted 1 embedded image payload(s) after reaching the 5-byte document image budget",
		"stopped extracting embedded images after the 3-item document limit",
	}
	if got := budget.warnings(); !reflect.DeepEqual(got, want) {
		t.Fatalf("warnings = %q, want %q", got, want)
	}
}

func TestEmbeddedMediaBudgetIncludesExactByteLimits(t *testing.T) {
	budget := &embeddedMediaBudget{maxImageBytes: 4, maxTotalBytes: 4, maxItems: 2}
	if included, keepWalking := budget.include([]byte("abcd")); !included || !keepWalking {
		t.Fatalf("exact-limit image = (%v, %v), want (true, true)", included, keepWalking)
	}
	if included, keepWalking := budget.include([]byte("e")); included || !keepWalking {
		t.Fatalf("image after exact document limit = (%v, %v), want (false, true)", included, keepWalking)
	}
	if budget.totalBytes != 4 {
		t.Fatalf("totalBytes = %d, want 4", budget.totalBytes)
	}
}
