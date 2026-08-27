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

package task

import (
	"testing"
)

const (
	testDocID = "doc-1"
)

func TestChunkID_NotPanic(t *testing.T) {
	// Edge cases: empty content
	_ = ChunkID("", testDocID)
	_ = ChunkID("content", "")
	_ = ChunkID("", "")
	// All should not panic
}

func TestChunkID_PreservesLeadingZero(t *testing.T) {
	content := ""
	docID := "doc-1"
	got := ChunkID(content, docID)
	want := "037fe13bd80c56aa"
	if got != want {
		t.Fatalf("ChunkID(%q, %q) = %q, want %q", content, docID, got, want)
	}
	if len(got) != 16 {
		t.Fatalf("ChunkID length = %d, want 16", len(got))
	}
}
