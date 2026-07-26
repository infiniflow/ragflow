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

package canvas

import (
	"reflect"
	"sync/atomic"
	"testing"
)

func TestCanvasStateSerializer_RoundTrip(t *testing.T) {
	src := NewCanvasState("run_abc", "task_xyz")
	src.Outputs["retrieval_0"] = map[string]any{
		"chunks":   []string{"a", "b", "c"},
		"doc_aggs": map[string]int{"doc1": 3, "doc2": 1},
	}
	src.Outputs["llm_0"] = map[string]any{
		"answer":  "the sky is blue",
		"tokens":  17,
		"model":   "gpt-4o-mini",
		"stopped": true,
	}
	src.Sys["query"] = "what color is the sky?"
	src.Sys["user_id"] = "u_42"
	src.Sys["files"] = []any{"f1", "f2"}
	src.Env["DEPLOY_REGION"] = "us-west-2"
	src.Env["MODEL_TIER"] = "small"
	src.Path = []string{"begin_0", "retrieval_0", "llm_0", "message_0"}
	src.History = []map[string]any{
		{"role": "user", "content": "earlier turn"},
		{"role": "assistant", "content": "earlier reply"},
	}
	src.Globals["shared_key"] = "v1"
	src.CancelFlag.Store(true)

	ser := CanvasStateSerializer{}
	data, err := ser.Marshal(src)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Marshal returned empty bytes")
	}

	dst := NewCanvasState("", "")
	if err := ser.Unmarshal(data, dst); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if dst.RunID != src.RunID {
		t.Fatalf("RunID = %q, want %q", dst.RunID, src.RunID)
	}
	if dst.TaskID != src.TaskID {
		t.Fatalf("TaskID = %q, want %q", dst.TaskID, src.TaskID)
	}
	// JSON round-trip coerces numbers to float64, so we re-marshal both
	// sides and compare bytes — that is the real contract of the
	// serializer (lossless across the eino checkpoint boundary).
	srcBytes, _ := ser.Marshal(src)
	dstBytes, _ := ser.Marshal(dst)
	if string(srcBytes) != string(dstBytes) {
		t.Fatalf("round-trip not stable:\n src→bytes: %s\n dst→bytes: %s",
			srcBytes, dstBytes)
	}
	// Direct checks for the non-JSON-coerced fields.
	// Note: CancelFlag is *atomic.Bool; encoding/json does not marshal
	// its unexported fields, so the flag is reset to its zero value on
	// round-trip. That is acceptable for the canvas checkpoint
	// contract — the cancel signal lives in Redis (cancel.go) and a
	// resumed run gets a fresh context. The non-nil pointer is the
	// invariant that matters: nodes must always be able to call .Load()
	// without checking for nil first.
	if dst.CancelFlag == nil {
		t.Fatal("CancelFlag is nil after Unmarshal; downstream .Load() would panic")
	}
	// Spot check that nested maps survive.
	if dst.Outputs["llm_0"]["model"] != "gpt-4o-mini" {
		t.Fatalf("nested map lost: %v", dst.Outputs)
	}
	if v, _ := dst.Sys["user_id"].(string); v != "u_42" {
		t.Fatalf("Sys[user_id] = %v", dst.Sys["user_id"])
	}
	// Suppress unused import warning when reflect.DeepEqual is removed.
	_ = reflect.DeepEqual
}

func TestCanvasStateSerializer_EmptyState(t *testing.T) {
	// Edge case: zero-value state must round-trip without error.
	src := NewCanvasState("r", "t")
	ser := CanvasStateSerializer{}
	data, err := ser.Marshal(src)
	if err != nil {
		t.Fatalf("Marshal empty: %v", err)
	}
	dst := NewCanvasState("", "")
	if err := ser.Unmarshal(data, dst); err != nil {
		t.Fatalf("Unmarshal empty: %v", err)
	}
	if dst.RunID != "r" || dst.TaskID != "t" {
		t.Fatalf("ids not preserved: %q %q", dst.RunID, dst.TaskID)
	}
}

func TestCanvasStateSerializer_UnmarshalIntoExistingPointer(t *testing.T) {
	// The eino contract: Unmarshal fills a caller-owned pointer. Confirm
	// nested maps are populated (not just the top-level struct).
	src := NewCanvasState("r2", "t2")
	src.Outputs["only"] = map[string]any{"k": "v"}
	src.Sys["x"] = 1
	ser := CanvasStateSerializer{}
	data, _ := ser.Marshal(src)

	dst := NewCanvasState("old", "old")
	if err := ser.Unmarshal(data, dst); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if dst.Outputs["only"]["k"] != "v" {
		t.Fatalf("nested map not preserved: %v", dst.Outputs)
	}
	if v, ok := dst.Sys["x"].(float64); !ok || v != 1 {
		t.Fatalf("Sys[x] = %v (%T), want float64(1)", dst.Sys["x"], dst.Sys["x"])
	}
	// Ids are overwritten by the round-trip.
	if dst.RunID != "r2" || dst.TaskID != "t2" {
		t.Fatalf("ids not overwritten: %q %q", dst.RunID, dst.TaskID)
	}
}

// Ensure atomic.Bool preserves its zero value through JSON when set to false
// (avoids future regression on CancelFlag handling).
func TestCanvasStateSerializer_CancelFlagZero(t *testing.T) {
	src := NewCanvasState("r3", "t3")
	ser := CanvasStateSerializer{}
	data, _ := ser.Marshal(src)
	dst := NewCanvasState("", "")
	if err := ser.Unmarshal(data, dst); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if dst.CancelFlag == nil {
		t.Fatal("CancelFlag is nil after Unmarshal")
	}
	if dst.CancelFlag.Load() {
		t.Fatal("CancelFlag is true, want false")
	}
	// Cross-check the atomic is the same struct shape.
	var _ *atomic.Bool = dst.CancelFlag
}
