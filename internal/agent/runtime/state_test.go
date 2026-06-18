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

package runtime

import (
	"encoding/json"
	"testing"
)

// TestCanvasState_MarshalUnmarshalJSON pins the JSON wire shape
// introduced by the MarshalJSON hook (unblocking the eino
// interrupt path's "failed to marshal state: unknown type:
// runtime.CanvasState" error). Every field on CanvasState must
// round-trip without losing the map values, the CancelFlag bool,
// and the RunID / TaskID strings.
func TestCanvasState_MarshalUnmarshalJSON(t *testing.T) {
	t.Parallel()
	src := NewCanvasState("run-1", "task-1")
	src.Sys["query"] = "hello"
	src.Env["counter"] = 0.0
	src.CancelFlag.Store(true)
	src.Outputs["message_0"] = map[string]any{"content": "hi world"}
	src.Path = []string{"begin_0", "message_0"}

	raw, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var dst CanvasState
	if err := json.Unmarshal(raw, &dst); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got, _ := dst.Sys["query"].(string); got != "hello" {
		t.Errorf("Sys[query] = %q, want %q", got, "hello")
	}
	if !dst.CancelFlag.Load() {
		t.Error("CancelFlag not preserved")
	}
	if got, _ := dst.Outputs["message_0"]["content"].(string); got != "hi world" {
		t.Errorf("Outputs[message_0][content] = %q, want %q", got, "hi world")
	}
	if dst.RunID != "run-1" {
		t.Errorf("RunID = %q, want %q", dst.RunID, "run-1")
	}
	if dst.TaskID != "task-1" {
		t.Errorf("TaskID = %q, want %q", dst.TaskID, "task-1")
	}
	if len(dst.Path) != 2 || dst.Path[0] != "begin_0" || dst.Path[1] != "message_0" {
		t.Errorf("Path = %v, want [begin_0 message_0]", dst.Path)
	}
}

// TestCanvasState_MarshalJSON_DoesNotLeakMutex pins the wire-shape
// invariant: the unexported `mu sync.RWMutex` field must not appear
// in the JSON output. If a future maintainer adds a `json:"mu"` tag
// or refactors the struct so the lock ends up serialised, this test
// catches the regression — serialised mutex state is a
// deterministic-breakage risk for any consumer that caches the
// payload and unmarshals it later.
func TestCanvasState_MarshalJSON_DoesNotLeakMutex(t *testing.T) {
	t.Parallel()
	s := NewCanvasState("r", "t")
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if got := string(raw); contains(got, `"mu"`) || contains(got, `"Mu"`) {
		t.Errorf("serialised state leaks the unexported mutex field: %s", got)
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
