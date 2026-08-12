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

// TestFlexibleTimeScan verifies scanning native time, byte strings, and nil.
func TestFlexibleTimeScan(t *testing.T) {
	var value FlexibleTime
	if err := value.Scan(time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("scan time: %v", err)
	}
	if !value.Time().Equal(time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("scanned time = %s", value.Time())
	}

	var parsed FlexibleTime
	// MySQL driver returns varchar column values as []byte with fractional seconds.
	if err := parsed.Scan([]byte("2026-08-12 15:32:12.99303409")); err != nil {
		t.Fatalf("scan bytes: %v", err)
	}
	if got := parsed.Time().Format("2006-01-02 15:04:05"); got != "2026-08-12 15:32:12" {
		t.Fatalf("parsed time = %s", got)
	}

	// Python ISO string with timezone.
	var tz FlexibleTime
	if err := tz.Scan("2026-08-12T15:32:12.993034+00:00"); err != nil {
		t.Fatalf("scan tz string: %v", err)
	}
	expected := time.Date(2026, 8, 12, 15, 32, 12, 993034000, time.UTC)
	if !tz.Time().Equal(expected) {
		t.Fatalf("tz time = %s, want %s", tz.Time(), expected)
	}

	var none FlexibleTime
	if err := none.Scan(nil); err != nil {
		t.Fatalf("scan nil: %v", err)
	}
	if !none.Time().IsZero() {
		t.Fatalf("nil scanned time = %s", none.Time())
	}

	if err := parsed.Scan("not-a-time"); err == nil {
		t.Fatalf("expected parse error")
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
