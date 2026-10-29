package parser

import (
	"reflect"
	"strconv"
	"testing"
)

func newTestEmbeddedMediaBudget(t *testing.T, maxImageBytes int, maxTotalBytes int64, maxItems int) *embeddedMediaBudget {
	t.Helper()
	t.Setenv("RAGFLOW_MEDIA_IMAGE_MAX_BYTES", strconv.Itoa(maxImageBytes))
	t.Setenv("RAGFLOW_MEDIA_MAX_BYTES_PER_RUN", strconv.FormatInt(maxTotalBytes, 10))
	t.Setenv("RAGFLOW_MEDIA_MAX_IMAGES_PER_RUN", strconv.Itoa(maxItems))
	return newEmbeddedMediaBudget()
}

func TestNewEmbeddedMediaBudgetUsesSharedRunCaps(t *testing.T) {
	countBudget := newTestEmbeddedMediaBudget(t, 2048, 4096, 7)
	for i := 0; i < 7; i++ {
		if !countBudget.reserveItem() || !countBudget.includeReservedSize(1) {
			t.Fatalf("image %d should fit the shared 7-item cap", i)
		}
	}
	if countBudget.reserveItem() {
		t.Fatal("embedded budget ignored the shared 7-item cap")
	}
	if got := countBudget.warnings(); !reflect.DeepEqual(got, []string{"stopped extracting embedded images after the 7-item run limit"}) {
		t.Fatalf("count warnings = %q", got)
	}

	byteBudget := newTestEmbeddedMediaBudget(t, 2048, 1234, 7)
	if included, keepWalking := byteBudget.include(make([]byte, 1235)); included || !keepWalking {
		t.Fatalf("byte admission = (%v, %v), want (false, true)", included, keepWalking)
	}
	if got := byteBudget.warnings(); !reflect.DeepEqual(got, []string{"omitted 1 embedded image payload(s) after reaching the 1234-byte run image budget"}) {
		t.Fatalf("byte warnings = %q", got)
	}
}

func TestEmbeddedMediaBudgetReservedItemsAndWarnings(t *testing.T) {
	budget := newTestEmbeddedMediaBudget(t, 4, 5, 3)

	if !budget.reserveItem() || !budget.includeReservedSize(3) {
		t.Fatal("first image should fit")
	}
	if !budget.reserveItem() || budget.includeReservedSize(3) {
		t.Fatal("second image should exceed the run byte limit")
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
	if budget.items != 3 {
		t.Fatalf("budget items = %d, want 3", budget.items)
	}
	want := []string{
		"omitted 1 embedded image payload(s) larger than the 4-byte per-image limit",
		"omitted 1 embedded image payload(s) after reaching the 5-byte run image budget",
		"stopped extracting embedded images after the 3-item run limit",
	}
	if got := budget.warnings(); !reflect.DeepEqual(got, want) {
		t.Fatalf("warnings = %q, want %q", got, want)
	}
}

func TestEmbeddedMediaBudgetIncludesExactByteLimits(t *testing.T) {
	budget := newTestEmbeddedMediaBudget(t, 4, 4, 2)
	if included, keepWalking := budget.include([]byte("abcd")); !included || !keepWalking {
		t.Fatalf("exact-limit image = (%v, %v), want (true, true)", included, keepWalking)
	}
	if included, keepWalking := budget.include([]byte("e")); included || !keepWalking {
		t.Fatalf("image after exact run limit = (%v, %v), want (false, true)", included, keepWalking)
	}
}
