//
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
//

package entity

import (
	"encoding/json"
	"testing"
	"time"
)

func TestFlexibleTimeScanFormats(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// Formats already persisted by Python and the Go syncer.
		{"2026-08-12T15:32:12.993034+00:00", "2026-08-12T15:32:12.993034Z"},
		{"2026-08-12T15:32:12Z", "2026-08-12T15:32:12Z"},
		{"2026-08-12 15:32:12", "2026-08-12T15:32:12Z"},
		{"2026-08-12T15:32:12", "2026-08-12T15:32:12Z"},
		{"2026-08-12 15:32:12.99303409", "2026-08-12T15:32:12.99303409Z"},
		// Legacy compact offsets without a colon.
		{"2026-08-12T15:32:12+0000", "2026-08-12T15:32:12Z"},
		{"2026-08-12 15:32:12+0000", "2026-08-12T15:32:12Z"},
		{"2026-08-12T15:32:12.993034+0800", "2026-08-12T07:32:12.993034Z"},
		{"2026-08-12T15:32:12-0800", "2026-08-12T23:32:12Z"},
		// Lowercase z and zone-less fractional timestamps.
		{"2026-08-12T15:32:12z", "2026-08-12T15:32:12Z"},
		{"2026-08-12T15:32:12.123456", "2026-08-12T15:32:12.123456Z"},
		// RFC 5322 headers, e.g. raw Gmail Date values.
		{"Wed, 13 Aug 2026 10:00:00 +0000", "2026-08-13T10:00:00Z"},
	}
	for _, tc := range cases {
		var value FlexibleTime
		if err := value.Scan(tc.input); err != nil {
			t.Fatalf("Scan(%q): %v", tc.input, err)
		}
		if got := value.Time().UTC().Format(time.RFC3339Nano); got != tc.want {
			t.Fatalf("Scan(%q) = %s, want %s", tc.input, got, tc.want)
		}
	}
}

// TestFlexibleTimeScanLenient verifies that a legacy or unexpected timestamp
// falls back to the zero time instead of erroring, so one bad poll_range value
// cannot wedge the syncer scheduler at schedule status forever.
func TestFlexibleTimeScanLenient(t *testing.T) {
	for _, input := range []any{"not-a-time", "", nil} {
		var value FlexibleTime
		if err := value.Scan(input); err != nil {
			t.Fatalf("Scan(%v) = %v, want nil", input, err)
		}
		if !value.Time().IsZero() {
			t.Fatalf("Scan(%v) = %s, want zero time", input, value.Time())
		}
	}

	var native FlexibleTime
	now := time.Date(2026, 8, 12, 15, 32, 12, 0, time.UTC)
	if err := native.Scan(now); err != nil {
		t.Fatalf("Scan(time.Time) = %v", err)
	}
	if !native.Time().Equal(now) {
		t.Fatalf("Scan(time.Time) = %s, want %s", native.Time(), now)
	}
}

// TestFlexibleTimeValue verifies UTC ISO string serialization for varchar writes.
func TestFlexibleTimeValue(t *testing.T) {
	value := FlexibleTime(time.Date(2026, 8, 12, 10, 2, 1, 795728000, time.UTC))
	got, err := value.Value()
	if err != nil {
		t.Fatalf("value: %v", err)
	}
	if got != "2026-08-12T10:02:01.795728+00:00" {
		t.Fatalf("value = %q", got)
	}
	var zero FlexibleTime
	got, err = zero.Value()
	if err != nil {
		t.Fatalf("zero value: %v", err)
	}
	if got != nil {
		t.Fatalf("zero value = %v, want nil", got)
	}
}

// TestFlexibleTimeJSON verifies RFC3339 JSON round-trip.
func TestFlexibleTimeJSON(t *testing.T) {
	original := FlexibleTime(time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC))
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(data) != `"2026-07-01T08:00:00Z"` {
		t.Fatalf("marshal = %s", data)
	}
	var decoded FlexibleTime
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !decoded.Time().Equal(original.Time()) {
		t.Fatalf("round-trip = %s", decoded.Time())
	}
}
