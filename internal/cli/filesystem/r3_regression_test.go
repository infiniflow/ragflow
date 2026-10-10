package filesystem

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseTimeRFC3339WithOffset(t *testing.T) {
	got := parseTime("2026-09-30T15:04:05+05:30")
	if got.IsZero() {
		t.Fatal("RFC3339 timestamp with zone offset must parse, got zero time")
	}
	want, _ := time.Parse(time.RFC3339, "2026-09-30T15:04:05+05:30")
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseTimeMillisecondsRemainder(t *testing.T) {
	var record map[string]interface{}
	if err := json.Unmarshal([]byte(`{"created_at":1727700000123}`), &record); err != nil {
		t.Fatal(err)
	}
	for _, input := range []interface{}{int64(1727700000123), record["created_at"]} {
		got := parseTime(input)
		if got.UnixMilli() != 1727700000123 {
			t.Fatalf("millisecond remainder lost for %T: got %d", input, got.UnixMilli())
		}
	}
}
