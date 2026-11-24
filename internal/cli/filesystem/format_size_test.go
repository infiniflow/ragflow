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
