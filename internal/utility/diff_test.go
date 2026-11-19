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

package utility

import (
	"strings"
	"testing"
)

func TestUnifiedDiffEmptyWhenIdentical(t *testing.T) {
	if diff := UnifiedDiff("same\nbody", "same\nbody"); diff != "" {
		t.Fatalf("UnifiedDiff() = %q, want empty", diff)
	}
}

func TestUnifiedDiffRendersHunkHeaders(t *testing.T) {
	diff := UnifiedDiff("line one\nline two\nline three", "line one\nline 2\nline three")
	if !strings.HasPrefix(diff, "--- a\n+++ b\n@@ ") {
		t.Fatalf("UnifiedDiff() = %q, want unified headers", diff)
	}
	if !strings.Contains(diff, "-line two\n") || !strings.Contains(diff, "+line 2\n") {
		t.Fatalf("UnifiedDiff() = %q, want the changed lines", diff)
	}
	if !strings.Contains(diff, " line one\n") {
		t.Fatalf("UnifiedDiff() = %q, want context lines", diff)
	}
}

// TestUnifiedDiffDegradesOnHugeInputs pins the guard that keeps the LCS table
// from allocating quadratically: past the cell budget every old line is a
// deletion and every new line an insertion, still a parseable unified diff.
func TestUnifiedDiffDegradesOnHugeInputs(t *testing.T) {
	oldLines := make([]string, 2500)
	newLines := make([]string, 2500)
	for i := range oldLines {
		oldLines[i] = "old"
		newLines[i] = "new"
	}
	diff := UnifiedDiff(strings.Join(oldLines, "\n"), strings.Join(newLines, "\n"))
	if !strings.HasPrefix(diff, "--- a\n+++ b\n@@ ") {
		t.Fatalf("UnifiedDiff() = %q, want unified headers", diff)
	}
	if got := strings.Count(diff, "-old\n"); got != len(oldLines) {
		t.Fatalf("deleted lines = %d, want %d", got, len(oldLines))
	}
	if got := strings.Count(diff, "+new\n"); got != len(newLines) {
		t.Fatalf("inserted lines = %d, want %d", got, len(newLines))
	}
}
