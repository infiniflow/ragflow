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
