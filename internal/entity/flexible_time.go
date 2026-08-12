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
	"strings"
	"time"
)

// flexibleTimeLayouts covers the formats persisted by the Python backend
// (ISO strings in varchar columns) and the Go backend (native time.Time).
var flexibleTimeLayouts = []string{
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
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
		return f.parse(string(typed))
	case string:
		return f.parse(typed)
	}
	return fmt.Errorf("cannot scan %T into FlexibleTime", value)
}

// parse parses a persisted timestamp string.
func (f *FlexibleTime) parse(value string) error {
	value = strings.TrimSpace(value)
	for _, layout := range flexibleTimeLayouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			*f = FlexibleTime(parsed)
			return nil
		}
	}
	return fmt.Errorf("cannot parse %q as time", value)
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
