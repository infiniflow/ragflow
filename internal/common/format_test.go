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

import "testing"

func TestPtrString_Nil(t *testing.T) {
	if got := PtrString[int](nil); got != "<nil>" {
		t.Errorf("PtrString(nil) = %q, want <nil>", got)
	}
}

func TestPtrString_Value(t *testing.T) {
	val := 42
	if got := PtrString(&val); got != "42" {
		t.Errorf("PtrString(&42) = %q, want 42", got)
	}
}

func TestPtrString_Bool(t *testing.T) {
	val := true
	if got := PtrString(&val); got != "true" {
		t.Errorf("PtrString(&true) = %q, want true", got)
	}
}

func TestChunkID_NotPanic(t *testing.T) {
	_ = ChunkID("", "")
	_ = ChunkID("doc-1", "content")
}

func TestChunkID_PreservesLeadingZero(t *testing.T) {
	got := ChunkID("doc-1", "")
	want := "037fe13bd80c56aa"
	if got != want {
		t.Fatalf("ChunkID(%q, %q) = %q, want %q", "doc-1", "", got, want)
	}
	if len(got) != 16 {
		t.Fatalf("ChunkID length = %d, want 16", len(got))
	}
}
