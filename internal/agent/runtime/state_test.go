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

func TestCanvasStateCheckpointPreservesCurrentUserMarker(t *testing.T) {
	t.Parallel()
	src := NewCanvasState("run-checkpoint", "task-checkpoint")
	src.AppendHistory("user", "previous question")
	src.AppendHistory("assistant", "previous answer")
	src.AppendCurrentUser("current question")

	raw, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var dst CanvasState
	if err := json.Unmarshal(raw, &dst); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	prior := dst.SnapshotPriorHistory()
	if len(prior) != 2 {
		t.Fatalf("prior history length after restore = %d, want 2", len(prior))
	}
	if got := prior[1]["content"]; got != "previous answer" {
		t.Fatalf("prior assistant content after restore = %v, want previous answer", got)
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

func TestCanvasStateConversationHistory(t *testing.T) {
	t.Parallel()
	state := NewCanvasState("run-history", "task-history")
	state.AppendHistory("user", "previous question")
	state.AppendHistory("assistant", map[string]any{"content": "previous answer"})
	state.AppendCurrentUser("current question")
	state.AppendSysHistory("user: previous question")
	state.AppendSysHistory("assistant: {'content': 'previous answer'}")
	state.AppendSysHistory("user: current question")

	prior := state.SnapshotPriorHistory()
	if len(prior) != 2 {
		t.Fatalf("prior history length = %d, want 2", len(prior))
	}
	if got := prior[1]["content"]; got != "previous answer" {
		t.Fatalf("prior assistant content = %v, want previous answer", got)
	}
	if got := state.SnapshotSysHistory(); len(got) != 3 || got[2] != "user: current question" {
		t.Fatalf("sys.history = %#v", got)
	}
}

func TestCanvasStatePriorHistoryPreservesPersistedUser(t *testing.T) {
	t.Parallel()
	state := NewCanvasState("run-history", "task-history")
	state.AppendHistory("user", "persisted unanswered question")

	prior := state.SnapshotPriorHistory()
	if len(prior) != 1 || prior[0]["content"] != "persisted unanswered question" {
		t.Fatalf("prior history = %#v, want persisted user turn", prior)
	}

	state.AppendCurrentUser("current question")
	prior = state.SnapshotPriorHistory()
	if len(prior) != 1 || prior[0]["content"] != "persisted unanswered question" {
		t.Fatalf("prior history = %#v, want only current user excluded", prior)
	}
}

func TestCanvasStateIncrementConversationTurns(t *testing.T) {
	t.Parallel()
	state := NewCanvasState("run-turns", "task-turns")

	state.IncrementConversationTurns()
	if got := state.Sys["conversation_turns"]; got != 1 {
		t.Fatalf("conversation_turns = %#v, want 1", got)
	}
	state.IncrementConversationTurns()
	if got := state.Sys["conversation_turns"]; got != 2 {
		t.Fatalf("conversation_turns = %#v, want 2", got)
	}

	state.Sys["conversation_turns"] = float64(4)
	state.IncrementConversationTurns()
	if got := state.Sys["conversation_turns"]; got != float64(5) {
		t.Fatalf("JSON conversation_turns = %#v, want float64(5)", got)
	}
}

func TestCanvasStateHistorySnapshotsDeepCopyPayload(t *testing.T) {
	t.Parallel()
	payload := map[string]any{
		"content":  "answer",
		"nil":      nil,
		"nil_map":  map[string]any(nil),
		"nil_list": []any(nil),
		"metadata": map[string]any{
			"tags": []any{"one", map[string]any{"name": "nested"}},
		},
	}
	state := NewCanvasState("run-copy", "task-copy")
	state.AppendHistory("assistant", payload)

	snapshot := state.SnapshotHistory()
	snapshotPayload := snapshot[0]["payload"].(map[string]any)
	metadata := snapshotPayload["metadata"].(map[string]any)
	tags := metadata["tags"].([]any)
	metadata["new"] = true
	tags[0] = "changed"
	tags[1].(map[string]any)["name"] = "changed"

	unchanged := state.SnapshotHistory()[0]["payload"].(map[string]any)
	unchangedMetadata := unchanged["metadata"].(map[string]any)
	unchangedTags := unchangedMetadata["tags"].([]any)
	if unchanged["nil"] != nil || unchanged["nil_map"].(map[string]any) != nil || unchanged["nil_list"].([]any) != nil {
		t.Fatalf("nil values were not preserved: %#v", unchanged)
	}
	if _, exists := unchangedMetadata["new"]; exists || unchangedTags[0] != "one" || unchangedTags[1].(map[string]any)["name"] != "nested" {
		t.Fatalf("snapshot mutation leaked into state: %#v", unchanged)
	}

	payload["metadata"].(map[string]any)["source"] = "caller mutation"
	unchanged = state.SnapshotHistory()[0]["payload"].(map[string]any)
	if _, exists := unchanged["metadata"].(map[string]any)["source"]; exists {
		t.Fatalf("caller mutation leaked into state: %#v", unchanged)
	}
}

func TestCanvasStateMemoryIsSeparateFromHistory(t *testing.T) {
	t.Parallel()
	state := NewCanvasState("run-memory", "task-memory")
	state.AppendMemory("question", "answer", "used search")

	if len(state.SnapshotHistory()) != 0 {
		t.Fatalf("memory polluted history: %#v", state.SnapshotHistory())
	}
	memory := state.SnapshotMemory()
	if len(memory) != 1 || memory[0]["summary"] != "used search" {
		t.Fatalf("memory = %#v", memory)
	}
}

func TestCanvasState_EnsureSysDate(t *testing.T) {
	t.Parallel()

	state := NewCanvasState("r", "t")
	if got, _ := state.Sys["date"].(string); got == "" {
		t.Fatal("NewCanvasState did not initialize sys.date")
	}

	state.Sys["date"] = ""
	state.EnsureSysDate()
	if got, _ := state.Sys["date"].(string); got == "" {
		t.Fatal("EnsureSysDate left blank sys.date unchanged")
	}

	state.Sys["date"] = "custom-date"
	state.EnsureSysDate()
	if got := state.Sys["date"]; got != "custom-date" {
		t.Fatalf("EnsureSysDate overwrote non-empty sys.date: %v", got)
	}
}

func TestCanvasState_SetRetrievalReferencesMergesCalls(t *testing.T) {
	t.Parallel()

	state := NewCanvasState("run-1", "task-1")
	state.SetRetrievalReferences(
		[]map[string]any{{"id": "chunk-1"}},
		[]map[string]any{{"doc_name": "doc-1", "count": 1}},
	)
	state.SetRetrievalReferences(
		[]map[string]any{
			{"id": "chunk-1", "content": "duplicate"},
			{"id": "chunk-2"},
			{"content": "chunk without an ID"},
		},
		[]map[string]any{
			{"doc_name": "doc-1", "count": 99},
			{"doc_name": "doc-2", "count": 2},
			{"doc_name": "", "count": 3},
		},
	)

	chunks := state.GetRetrievalChunks()
	if len(chunks) != 3 {
		t.Fatalf("chunks length = %d, want 3", len(chunks))
	}
	if chunks[0]["id"] != "chunk-1" || chunks[1]["id"] != "chunk-2" {
		t.Errorf("chunk IDs = [%v %v], want [chunk-1 chunk-2]", chunks[0]["id"], chunks[1]["id"])
	}
	if chunks[2]["content"] != "chunk without an ID" {
		t.Errorf("chunk without ID was not retained: %#v", chunks[2])
	}

	docAggs := state.GetRetrievalDocAggs()
	if len(docAggs) != 2 {
		t.Fatalf("doc_aggs length = %d, want 2", len(docAggs))
	}
	firstDoc := docAggs["doc-1"]
	if firstDoc["count"] != 1 {
		t.Errorf("doc_aggs[doc-1].count = %v, want first value 1", firstDoc["count"])
	}
	if _, ok := docAggs["doc-2"]; !ok {
		t.Error("doc_aggs missing doc-2 from the second call")
	}
	if _, ok := docAggs[""]; ok {
		t.Error("doc_aggs retained an empty document name")
	}
}

func TestCanvasState_GetRetrievalReferenceReturnsFrontendPayload(t *testing.T) {
	t.Parallel()

	state := NewCanvasState("run-1", "task-1")
	state.SetRetrievalReferences(
		[]map[string]any{{"id": "chunk-1", "document_name": "Doc 1"}},
		[]map[string]any{{"doc_name": "Doc 1", "doc_id": "doc-1", "count": 1}},
	)

	reference := state.GetRetrievalReference()
	chunks, _ := reference["chunks"].([]any)
	if len(chunks) != 1 {
		t.Fatalf("chunks length = %d, want 1", len(chunks))
	}
	docAggs, _ := reference["doc_aggs"].([]any)
	if len(docAggs) != 1 {
		t.Fatalf("doc_aggs length = %d, want 1", len(docAggs))
	}
	if reference["total"] != 1 {
		t.Fatalf("total = %v, want 1", reference["total"])
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
