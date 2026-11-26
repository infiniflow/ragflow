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

package service

import "testing"

// copyFilter feeds call sites that write into the returned map immediately,
// so a nil input must still produce a writable (non-nil) map.
func TestCopyFilterNilMapIsWritable(t *testing.T) {
	got := copyFilter(nil)
	if got == nil {
		t.Fatal("copyFilter(nil) returned nil; callers that write to the result would panic")
	}
	got["knowledge_graph_kwd"] = []string{"entity"}
	if len(got) != 1 {
		t.Fatalf("copyFilter(nil) map has %d entries, want 1", len(got))
	}
}

func TestMapStringAnyNilMapIsWritable(t *testing.T) {
	got := mapStringAny(nil)
	if got == nil {
		t.Fatal("mapStringAny(nil) returned nil; callers need a writable map")
	}
	got["k"] = "v"
	if got["k"] != "v" {
		t.Fatalf("mapStringAny(nil) map write lost, got %v", got)
	}
}
