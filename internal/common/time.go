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

package common

import (
	"log/slog"
	"strings"
	"time"
)

// ParseISO8601 parses a date string trying multiple ISO 8601 / RFC3339
// variants, mirroring the flexibility of Python's dateutil.isoparse.
//
// Supported formats (tried in order):
//   - RFC3339Nano:  "2006-01-02T15:04:05.999999999Z07:00"
//   - RFC3339:      "2006-01-02T15:04:05Z07:00"
//   - No timezone:  "2006-01-02T15:04:05"
//   - Date only:    "2006-01-02"
//
// Z suffix is automatically normalised to +00:00 before parsing (PR #16483).
func ParseISO8601(dateString string) (time.Time, error) {
	// Normalise Z → +00:00 for compatibility with time.RFC3339.
	normalized := dateString
	if strings.HasSuffix(dateString, "Z") {
		normalized = dateString[:len(dateString)-1] + "+00:00"
	}

	layouts := []string{
		time.RFC3339Nano,      // "2006-01-02T15:04:05.999999999Z07:00"
		time.RFC3339,          // "2006-01-02T15:04:05Z07:00"
		"2006-01-02T15:04:05", // no timezone
		"2006-01-02",          // date only
	}
	for _, layout := range layouts {
		var t time.Time
		var err error
		if strings.Contains(layout, "Z07:00") || strings.Contains(layout, "MST") {
			t, err = time.Parse(layout, normalized)
		} else {
			t, err = time.ParseInLocation(layout, normalized, time.Local)
		}
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, &time.ParseError{
		Layout:     "ISO 8601",
		Value:      dateString,
		LayoutElem: "",
		ValueElem:  dateString,
		Message:    "failed to parse as any supported ISO 8601 variant",
	}
}

// FormatISO8601ToYMDHMS parses an ISO 8601 / RFC3339 date string and
// returns it formatted as "YYYY-MM-DD HH:MM:SS". If parsing fails the
// original string is returned unchanged.
//
// Mirrors Python's format_iso_8601_to_ymd_hms in common/time_utils.py,
// with the fix from PR #16483 (single-parse using dateutil.isoparse
// instead of a broken double-parse).
func FormatISO8601ToYMDHMS(timeStr string) string {
	dt, err := ParseISO8601(timeStr)
	if err != nil {
		slog.Error("FormatISO8601ToYMDHMS parse error", "input", timeStr, "error", err)
		return timeStr
	}
	return dt.Format("2006-01-02 15:04:05")
}

// DeltaSeconds calculates seconds elapsed from a given date string to now.
//
// Supports multiple time formats:
//   - "YYYY-MM-DD HH:MM:SS" (e.g., "2024-01-01 12:00:00")
//   - ISO 8601 / RFC3339 (e.g., "2026-04-09T18:55:46+08:00")
//
// Args:
//
//	dateString: Date string in supported format
//
// Returns:
//
//	float64: Number of seconds between the given date and current time
//
// Example:
//
//	DeltaSeconds("2024-01-01 12:00:00")
//	DeltaSeconds("2026-04-09T18:55:46+08:00")
func DeltaSeconds(dateString string) (float64, error) {
	// Try ISO 8601 / RFC3339 with flexible parsing (PR #16483).
	dt, err := ParseISO8601(dateString)
	if err == nil {
		return time.Since(dt).Seconds(), nil
	}

	// Try custom format without timezone (e.g., "2024-01-01 12:00:00")
	const layout = "2006-01-02 15:04:05"
	dt, err = time.ParseInLocation(layout, dateString, time.Local)
	if err != nil {
		return 0, err
	}
	return time.Since(dt).Seconds(), nil
}
