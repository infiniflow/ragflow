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

package main

import (
	"testing"
)

// TestSanitizeProcessed_StripsVectorKeys locks that the golden-compare tool
// drops the SAME vector keys the production debug payload strips, via the
// shared task.IsVectorKey. The previous inline check
// (strings.HasPrefix(k,"q_") && strings.HasSuffix(k,"_vec")) only caught the
// dimension-scoped q_<dim>_vec pattern and let the fixed legacy keys
// (vector/embedding/feature/q_vec) leak — causing spurious diffs against the
// expected output that already excludes them. This test pins the full set.
func TestSanitizeProcessed_StripsVectorKeys(t *testing.T) {
	in := []map[string]any{
		{
			"text":                 "hello",
			"vector":               []float64{0.1},
			"embedding":            []float64{0.2},
			"q_1024_vec":           []float64{0.3},
			"q_vec":                []float64{0.4},
			"create_time":          "2026",
			"create_timestamp_flt": 1.0,
		},
	}

	got := sanitizeProcessed(in)
	if len(got) != 1 {
		t.Fatalf("sanitizeProcessed returned %d chunks, want 1", len(got))
	}
	cp := got[0]

	// All vector keys (fixed legacy + dimension-scoped) must be removed.
	for _, k := range []string{"vector", "embedding", "q_1024_vec", "q_vec"} {
		if _, ok := cp[k]; ok {
			t.Errorf("vector key %q must be stripped, but present: %#v", k, cp)
		}
	}
	// create_time / create_timestamp_flt are unrelated bookkeeping keys the
	// tool always drops; assert they stay dropped.
	for _, k := range []string{"create_time", "create_timestamp_flt"} {
		if _, ok := cp[k]; ok {
			t.Errorf("bookkeeping key %q must be stripped, but present: %#v", k, cp)
		}
	}
	// Non-vector payload must survive.
	if cp["text"] != "hello" {
		t.Errorf("text=%v want hello", cp["text"])
	}
}
