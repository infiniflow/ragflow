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

package filesystem

import (
	"strconv"
	"testing"
	"time"
)

// TestParseTimeRFC3339WithZoneOffset checks supported string formats.
func TestParseTimeRFC3339WithZoneOffset(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		wantNonZero bool
		wantYear    int
		wantInstant time.Time
	}{
		{
			name:        "RFC3339 with positive zone offset",
			input:       "2026-09-30T15:04:05+05:30",
			wantInstant: time.Date(2026, 9, 30, 9, 34, 5, 0, time.UTC),
			wantNonZero: true,
			wantYear:    2026,
		},
		{
			name:        "RFC3339 with negative zone offset",
			input:       "2026-09-30T15:04:05-08:00",
			wantInstant: time.Date(2026, 9, 30, 23, 4, 5, 0, time.UTC),
			wantNonZero: true,
			wantYear:    2026,
		},
		{
			name:        "RFC3339 with Z (UTC)",
			input:       "2026-09-30T15:04:05Z",
			wantNonZero: true,
			wantYear:    2026,
		},
		{
			name:        "RFC3339Nano with fractional seconds and zone",
			input:       "2026-09-30T15:04:05.123456789+05:30",
			wantInstant: time.Date(2026, 9, 30, 9, 34, 5, 123456789, time.UTC),
			wantNonZero: true,
			wantYear:    2026,
		},
		{
			name:        "RFC3339Nano with fractional seconds and Z",
			input:       "2026-09-30T15:04:05.999999999Z",
			wantInstant: time.Date(2026, 9, 30, 15, 4, 5, 999999999, time.UTC),
			wantNonZero: true,
			wantYear:    2026,
		},
		{
			name:        "space-separated with Z",
			input:       "2026-09-30 15:04:05Z",
			wantNonZero: true,
			wantYear:    2026,
		},
		{
			name:        "no zone, space separator (existing format)",
			input:       "2026-09-30 15:04:05",
			wantNonZero: true,
			wantYear:    2026,
		},
		{
			name:        "date only (existing format)",
			input:       "2026-09-30",
			wantNonZero: true,
			wantYear:    2026,
		},
		{
			name:        "quoted RFC3339 (the strings.Trim path)",
			input:       `"2026-09-30T15:04:05+05:30"`,
			wantNonZero: true,
			wantYear:    2026,
		},
		{
			name:        "unparseable string returns zero time",
			input:       "not a timestamp",
			wantNonZero: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseTime(tc.input)
			if !tc.wantInstant.IsZero() && !got.Equal(tc.wantInstant) {
				t.Fatalf("parseTime(%q) = %v, want %v", tc.input, got, tc.wantInstant)
			}
			if tc.wantNonZero {
				if got.IsZero() {
					t.Fatalf("parseTime(%q) = zero time, want non-zero", tc.input)
				}
				if got.Year() != tc.wantYear {
					t.Errorf("parseTime(%q) year = %d, want %d", tc.input, got.Year(), tc.wantYear)
				}
			} else if !got.IsZero() {
				t.Fatalf("parseTime(%q) = %v, want zero time", tc.input, got)
			}
		})
	}
}

// TestParseTimePreservesMillisecondPrecision checks integer epoch inputs.
func TestParseTimePreservesMillisecondPrecision(t *testing.T) {
	// 2024-09-30T14:40:00.123 UTC.
	have := int64(1727700000123)
	want := time.Unix(1727700000, 123000000)

	cases := []struct {
		name  string
		input interface{}
	}{
		{"int64 milliseconds", have},
		{"int milliseconds", int(have)},
		{"float64 milliseconds (whole number)", float64(have)},
		{"string-encoded milliseconds", strconv.FormatInt(have, 10)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseTime(tc.input)
			if !got.Equal(want) {
				t.Fatalf("parseTime(%v) = %v, want %v", tc.input, got, want)
			}
			if got.Nanosecond() != 123000000 {
				t.Errorf("parseTime(%v) nanosecond = %d, want 123000000 (sub-second precision lost)",
					tc.input, got.Nanosecond())
			}
		})
	}
}

func TestParseTimeSecondsPrecisionUnchanged(t *testing.T) {
	// 2024-09-30T14:40:00 UTC — 10 digits, seconds precision.
	have := int64(1727700000)
	want := time.Unix(1727700000, 0)

	cases := []struct {
		name  string
		input interface{}
	}{
		{"int64 seconds", have},
		{"int seconds", int(have)},
		{"string-encoded seconds", strconv.FormatInt(have, 10)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseTime(tc.input)
			if !got.Equal(want) {
				t.Fatalf("parseTime(%v) = %v, want %v", tc.input, got, want)
			}
			if got.Nanosecond() != 0 {
				t.Errorf("parseTime(%v) nanosecond = %d, want 0", tc.input, got.Nanosecond())
			}
		})
	}
}

func TestParseTimeNil(t *testing.T) {
	if got := parseTime(nil); !got.IsZero() {
		t.Errorf("parseTime(nil) = %v, want zero time", got)
	}
}
