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

package component

import "testing"

// TestBeOutput_MirrorsPythonContract guards the Go-side
// component.BeOutput helper added for parity with
// agent/component/base.py:ComponentBase.be_output (PR #16363).
// Downstream consumers (Message, VariableAggregator) read
// `out["content"]`; the helper must produce that key for every
// value type so error/empty paths can return a uniform frame.
func TestBeOutput_MirrorsPythonContract(t *testing.T) {
	t.Parallel()
	if got := BeOutput("hello"); got["content"] != "hello" {
		t.Errorf("BeOutput(hello)[content] = %v, want hello", got["content"])
	}
	gotNil := BeOutput(nil)
	if _, ok := gotNil["content"]; !ok {
		t.Errorf("BeOutput(nil) should still produce content key, got %v", gotNil)
	}
	if got := BeOutput(42); got["content"] != 42 {
		t.Errorf("BeOutput(42)[content] = %v, want 42", got["content"])
	}
}
