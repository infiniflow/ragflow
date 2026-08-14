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
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"log"
	"net/mail"
	"regexp"
	"strings"
	"time"
)

// compactOffsetPattern expands the legacy "+0000" style offsets to "+00:00".
var compactOffsetPattern = regexp.MustCompile(`([+-]\d{2})(\d{2})$`)

// flexibleTimeLayouts covers every combination of the timestamp spellings
// persisted by the Python backend (ISO strings in varchar columns) and the Go
// backend (native time.Time and the UTC ISO strings written by Value): T or
// space separator, optional fractional seconds, and optional zone.
var flexibleTimeLayouts = []string{
	"2006-01-02T15:04:05.999999999Z07:00",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05Z07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	time.RFC3339Nano,
	time.RFC3339,
}

// FlexibleTime scans time values from both native time.Time columns and the
// varchar timestamp strings the Python backend writes, while still
// serializing to JSON like time.Time.
type FlexibleTime time.Time

// Scan implements sql.Scanner.
func (f *FlexibleTime) Scan(value any) error {
	if f == nil {
		return fmt.Errorf("cannot scan into nil FlexibleTime")
	}
	if value == nil {
		*f = FlexibleTime{}
		return nil
	}
	switch typed := value.(type) {
	case time.Time:
		*f = FlexibleTime(typed)
		return nil
	case []byte:
		f.parse(string(typed))
		return nil
	case string:
		f.parse(typed)
		return nil
	}
	return fmt.Errorf("cannot scan %T into FlexibleTime", value)
}

// parse parses a persisted timestamp string. A value that matches no known
// format falls back to the zero time instead of an error: the sync_logs poll
// waterline is a varchar shared with the Python backend, and one
// legacy/unexpected timestamp must not wedge the syncer by making ClaimTask
// or ListDueTasks fail forever. The next successful sync rewrites the
// waterline.
func (f *FlexibleTime) parse(value string) {
	original := value
	value = strings.TrimSpace(value)
	// RFC 5322 headers (for example raw Gmail Date values) keep the compact
	// "+0000" zone spelling, so try the standard library parser first.
	if value != "" {
		if parsed, err := mail.ParseDate(value); err == nil {
			*f = FlexibleTime(parsed)
			return
		}
	}
	if value != "" {
		last := value[len(value)-1]
		if last == 'Z' || last == 'z' {
			value = value[:len(value)-1] + "+00:00"
		}
	}
	value = compactOffsetPattern.ReplaceAllString(value, "$1:$2")
	for _, layout := range flexibleTimeLayouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			*f = FlexibleTime(parsed)
			return
		}
	}
	if strings.TrimSpace(original) != "" {
		log.Printf("flexible_time: cannot parse %q as time, falling back to zero time", original)
	}
	*f = FlexibleTime{}
}

// Value implements driver.Valuer. The shared sync_logs columns are varchar
// and the metadata DSN renders time.Time in the local zone, so an explicit
// UTC ISO string is written to keep the stored instant unambiguous. The
// layout mirrors Python's DateTimeTzField.db_value (datetime.isoformat on a
// UTC value): 6-digit microseconds and a "+00:00" offset.
func (f FlexibleTime) Value() (driver.Value, error) {
	value := time.Time(f)
	if value.IsZero() {
		return nil, nil
	}
	return value.UTC().Format("2006-01-02T15:04:05.999999-07:00"), nil
}

// Time returns the underlying time value.
func (f FlexibleTime) Time() time.Time {
	return time.Time(f)
}

// NewFlexibleTime wraps a time.Time pointer, returning nil for nil input.
func NewFlexibleTime(value *time.Time) *FlexibleTime {
	if value == nil {
		return nil
	}
	converted := FlexibleTime(*value)
	return &converted
}

// MarshalJSON serializes like time.Time (RFC3339).
func (f FlexibleTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Time(f))
}

// UnmarshalJSON parses an RFC3339 timestamp.
func (f *FlexibleTime) UnmarshalJSON(data []byte) error {
	var value *time.Time
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if value == nil {
		*f = FlexibleTime{}
		return nil
	}
	*f = FlexibleTime(*value)
	return nil
}
