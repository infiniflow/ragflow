package filesystem

import (
	"strings"
	"testing"
)

// TestFormatSizeCharacterization pins the behavior that must survive the
// convergence to common.FormatBytes: zero renders as a dash, sub-kilobyte
// values are exact byte counts, and larger values pick the expected unit.
// Decimal precision is intentionally not asserted here because the shared
// formatter uses one decimal place instead of two.
func TestFormatSizeCharacterization(t *testing.T) {
	if got := formatSize(0); got != "-" {
		t.Fatalf("zero: got %q want %q", got, "-")
	}
	if got := formatSize(1); got != "1 B" {
		t.Fatalf("bytes: got %q want %q", got, "1 B")
	}
	if got := formatSize(1023); got != "1023 B" {
		t.Fatalf("sub-kilobyte: got %q want %q", got, "1023 B")
	}
	cases := []struct {
		size int64
		unit string
	}{
		{1024, "KB"},
		{1024 * 1024, "MB"},
		{1024 * 1024 * 1024, "GB"},
		{1024 * 1024 * 1024 * 1024, "TB"},
	}
	for _, tc := range cases {
		if got := formatSize(tc.size); !strings.HasSuffix(got, " "+tc.unit) {
			t.Fatalf("size %d: got %q want suffix %q", tc.size, got, tc.unit)
		}
	}
}
