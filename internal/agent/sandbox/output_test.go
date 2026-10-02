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

package sandbox

import (
	"strings"
	"testing"
)

func TestOutputCapture_DrainsBeyondLimit(t *testing.T) {
	var output outputCapture
	output.limit = 32
	for range 100 {
		if n, err := output.Write([]byte(strings.Repeat("x", 1024))); err != nil || n != 1024 {
			t.Fatalf("Write = (%d, %v), want (1024, nil)", n, err)
		}
	}
	if output.buffer.Len() > 32 {
		t.Errorf("captured %d bytes, want at most 32", output.buffer.Len())
	}
	if output.total != 102400 {
		t.Errorf("total = %d, want 102400", output.total)
	}
}

// TestOutputCapture_NoLimitRetainsEverything pins the zero-limit case:
// a provider configured without max_output_bytes must keep buffering,
// so the bound cannot silently start truncating for those deployments.
func TestOutputCapture_NoLimitRetainsEverything(t *testing.T) {
	var output outputCapture
	chunk := []byte(strings.Repeat("y", 4096))
	for range 8 {
		if _, err := output.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if got := output.buffer.Len(); got != 8*4096 {
		t.Errorf("captured %d bytes, want %d", got, 8*4096)
	}
	if output.total != 8*4096 {
		t.Errorf("total = %d, want %d", output.total, 8*4096)
	}
}
