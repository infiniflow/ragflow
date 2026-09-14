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

// Tests for the Phase 4.4 V2 canvas DSL decoder.
//
// Each test pins one branch of decodeCanvasFromDSL:
//   - empty DSL → ErrAgentStorageError (no JSON-round-trip happens)
//   - non-empty DSL with one component → clean *canvas.Canvas
//   - non-empty DSL with multiple components → all preserved
//
// All failures MUST carry ErrAgentStorageError so the handler-side
// mapAgentError classifies them as CodeServerError (500) with the
// sanitized message (no raw decoder-string leak). This is the
// regression pin that closes the v3.5.2 review's "raw decode error
// leak" concern at the Compile/Invoke boundary.
package service

import (
	"errors"
	"testing"

	agenttool "ragflow/internal/agent/tool"
)

// TestDecodeCanvasFromDSL_EmptyDSL pins the "empty DSL" branch:
// decodeCanvasFromDSL(nil) must return an error wrapping
// ErrAgentStorageError without attempting json.Marshal (the empty
// map serialises to "{}" which would decode to an empty Canvas,
// bypassing the "no components" check).
func TestDecodeCanvasFromDSL_EmptyDSL(t *testing.T) {
	t.Parallel()
	_, err := decodeCanvasFromDSL(nil)
	if err == nil {
		t.Fatal("expected error for empty DSL")
	}
	if !errors.Is(err, ErrAgentStorageError) {
		t.Errorf("expected ErrAgentStorageError in chain, got %v", err)
	}
}

// TestDecodeCanvasFromDSL_EmptyMapSameAsNil pins the equivalent
// branch for an explicitly-empty (but non-nil) map. Same outcome
// as the nil case — the function must reject before marshalling.
func TestDecodeCanvasFromDSL_EmptyMapSameAsNil(t *testing.T) {
	t.Parallel()
	_, err := decodeCanvasFromDSL(map[string]any{})
	if err == nil {
		t.Fatal("expected error for empty DSL map")
	}
	if !errors.Is(err, ErrAgentStorageError) {
		t.Errorf("expected ErrAgentStorageError in chain, got %v", err)
	}
}

// TestDecodeCanvasFromDSL_SingleComponent pins the happy-path:
// a DSL with one Begin component decodes to a *canvas.Canvas with
// that component under its id key.
func TestDecodeCanvasFromDSL_SingleComponent(t *testing.T) {
	t.Parallel()
	dsl := map[string]any{
		"components": map[string]any{
			"begin_0": map[string]any{
				"obj": map[string]any{
					"component_name": "Begin",
					"params":         map[string]any{},
				},
				"downstream": []any{"message_0"},
				"upstream":   []any{},
			},
		},
		"path": []any{"begin_0"},
	}
	c, err := decodeCanvasFromDSL(dsl)
	if err != nil {
		t.Fatalf("decodeCanvasFromDSL: %v", err)
	}
	if c == nil {
		t.Fatal("decoded Canvas is nil")
	}
	if len(c.Components) != 1 {
		t.Errorf("Components length = %d, want 1", len(c.Components))
	}
	if _, ok := c.Components["begin_0"]; !ok {
		t.Errorf("Components missing begin_0 key")
	}
	if len(c.Path) != 1 || c.Path[0] != "begin_0" {
		t.Errorf("Path = %v, want [begin_0]", c.Path)
	}
}

// TestDecodeCanvasFromDSL_MultiComponent pins the multi-node
// happy-path: Begin → Retrieval → Message → Answer decodes into
// 4 components with the correct Downstream / Upstream wiring.
// This is the shape real v1 DSLs use.
func TestDecodeCanvasFromDSL_MultiComponent(t *testing.T) {
	t.Parallel()
	dsl := map[string]any{
		"components": map[string]any{
			"begin_0": map[string]any{
				"obj": map[string]any{
					"component_name": "Begin",
					"params":         map[string]any{},
				},
				"downstream": []any{"retrieval_0"},
			},
			"retrieval_0": map[string]any{
				"obj": map[string]any{
					"component_name": "Retrieval",
					"params":         map[string]any{"kb_ids": []any{"kb-1"}},
				},
				"downstream": []any{"message_0"},
				"upstream":   []any{"begin_0"},
			},
			"message_0": map[string]any{
				"obj": map[string]any{
					"component_name": "Message",
					"params":         map[string]any{"text": "hi {{begin_0@result}}"},
				},
				"upstream": []any{"retrieval_0"},
			},
		},
		"path": []any{"begin_0", "retrieval_0", "message_0"},
	}
	c, err := decodeCanvasFromDSL(dsl)
	if err != nil {
		t.Fatalf("decodeCanvasFromDSL: %v", err)
	}
	if len(c.Components) != 3 {
		t.Errorf("Components length = %d, want 3", len(c.Components))
	}
	for _, id := range []string{"begin_0", "retrieval_0", "message_0"} {
		if _, ok := c.Components[id]; !ok {
			t.Errorf("Components missing %s", id)
		}
	}
	if got := c.Components["retrieval_0"].Obj.ComponentName; got != "Retrieval" {
		t.Errorf("retrieval_0 component_name = %q, want Retrieval", got)
	}
}

// TestDecodeCanvasFromDSL_NoComponents pins the "non-empty map
// with no components" branch: a DSL whose top-level keys are
// non-component (e.g. only "globals") must still fail because the
// resulting Canvas would have an empty Components map. The error
// must chain ErrAgentStorageError so mapAgentError -> 500.
func TestDecodeCanvasFromDSL_NoComponents(t *testing.T) {
	t.Parallel()
	dsl := map[string]any{
		"globals": map[string]any{
			"sys.query": "",
		},
	}
	_, err := decodeCanvasFromDSL(dsl)
	if err == nil {
		t.Fatal("expected error when DSL has no components")
	}
	if !errors.Is(err, ErrAgentStorageError) {
		t.Errorf("expected ErrAgentStorageError in chain, got %v", err)
	}
}

// TestDecodeCanvasFromDSL_GlobalsPreserved pins that round-tripping
// the DSL through JSON does not lose top-level metadata keys
// (globals, history, retrieval). These are needed at compile time
// for state-pre / state-post handlers to resolve references.
func TestDecodeCanvasFromDSL_GlobalsPreserved(t *testing.T) {
	t.Parallel()
	dsl := map[string]any{
		"components": map[string]any{
			"begin_0": map[string]any{
				"obj": map[string]any{
					"component_name": "Begin",
					"params":         map[string]any{},
				},
				"downstream": []any{},
			},
		},
		"globals": map[string]any{
			"sys.query":       "hello",
			"sys.user_id":     "user-1",
			"env.counter":     0.0,
			"env.loop_done":   false,
			"env.sample_rows": []any{},
		},
	}
	c, err := decodeCanvasFromDSL(dsl)
	if err != nil {
		t.Fatalf("decodeCanvasFromDSL: %v", err)
	}
	if c.Globals == nil {
		t.Fatal("Globals is nil after round-trip")
	}
	if got, _ := c.Globals["sys.query"].(string); got != "hello" {
		t.Errorf("Globals[sys.query] = %v, want \"hello\"", c.Globals["sys.query"])
	}
	if got, _ := c.Globals["sys.user_id"].(string); got != "user-1" {
		t.Errorf("Globals[sys.user_id] = %v, want \"user-1\"", c.Globals["sys.user_id"])
	}
}

func TestDecodeCanvasFromDSL_PreservesNodeParents(t *testing.T) {
	t.Parallel()
	dsl := map[string]any{
		"graph": map[string]any{
			"nodes": []any{
				map[string]any{"id": "Iteration:IterateList"},
				map[string]any{"id": "IterationItem:IterStart", "parentId": "Iteration:IterateList"},
				map[string]any{"id": "StringTransform:FmtItem", "parentId": "Iteration:IterateList"},
				map[string]any{"id": "Message:IterDone"},
			},
		},
		"components": map[string]any{
			"Iteration:IterateList": map[string]any{
				"obj": map[string]any{
					"component_name": "Parallel",
					"params":         map[string]any{"items_ref": "sys.items"},
				},
				"downstream": []any{"Message:IterDone"},
			},
			"IterationItem:IterStart": map[string]any{
				"obj": map[string]any{
					"component_name": "IterationItem",
					"params":         map[string]any{},
				},
				"downstream": []any{"StringTransform:FmtItem"},
			},
			"StringTransform:FmtItem": map[string]any{
				"obj": map[string]any{
					"component_name": "StringTransform",
					"params":         map[string]any{"method": "merge", "script": "{item}", "delimiters": []any{"|"}},
				},
			},
			"Message:IterDone": map[string]any{
				"obj": map[string]any{
					"component_name": "Message",
					"params":         map[string]any{"content": []any{"done"}},
				},
			},
		},
	}

	c, err := decodeCanvasFromDSL(dsl)
	if err != nil {
		t.Fatalf("decodeCanvasFromDSL: %v", err)
	}
	if c.NodeParents["IterationItem:IterStart"] != "Iteration:IterateList" {
		t.Fatalf("IterationItem parent = %q, want Iteration:IterateList", c.NodeParents["IterationItem:IterStart"])
	}
	if c.NodeParents["StringTransform:FmtItem"] != "Iteration:IterateList" {
		t.Fatalf("FmtItem parent = %q, want Iteration:IterateList", c.NodeParents["StringTransform:FmtItem"])
	}
	if _, ok := c.NodeParents["Message:IterDone"]; ok {
		t.Fatalf("outer follower Message:IterDone should not be marked as a grouped child")
	}
}

// TestNewAgentServiceWithOptions_NilOptions pins that the new
// constructor accepts all-nil options and produces a usable
// service (no panic on field init, no nil-deref when running
// without Redis). Existing test sites rely on this — the
// zero-arg NewAgentService() now just delegates here.
func TestNewAgentServiceWithOptions_NilOptions(t *testing.T) {
	t.Parallel()
	svc := NewAgentServiceWithOptions(nil, nil, nil)
	if svc == nil {
		t.Fatal("NewAgentServiceWithOptions returned nil")
	}
	if svc.runner == nil {
		t.Error("runner is nil after construction")
	}
	if svc.checkpointStore != nil {
		t.Error("checkpointStore should be nil when constructed with nil")
	}
	if svc.stateSerializer != nil {
		t.Error("stateSerializer should be nil when constructed with nil")
	}
	if svc.runTracker != nil {
		t.Error("runTracker should be nil when constructed with nil")
	}
	if svc.canvasDAO == nil || svc.versionDAO == nil {
		t.Error("DAOs should be non-nil after construction")
	}
}

// TestNewAgentService_DefaultsToNilOptions pins that the legacy
// zero-arg NewAgentService() is functionally equivalent to
// NewAgentServiceWithOptions(nil, nil, nil) — the field values
// must match exactly so existing call sites don't observe a
// behavioural change.
func TestNewAgentService_DefaultsToNilOptions(t *testing.T) {
	t.Parallel()
	a := NewAgentService()
	b := NewAgentServiceWithOptions(nil, nil, nil)
	if a.checkpointStore != b.checkpointStore {
		t.Errorf("checkpointStore mismatch: %v vs %v", a.checkpointStore, b.checkpointStore)
	}
	if a.stateSerializer != b.stateSerializer {
		t.Errorf("stateSerializer mismatch: %v vs %v", a.stateSerializer, b.stateSerializer)
	}
	if a.runTracker != b.runTracker {
		t.Errorf("runTracker mismatch: %v vs %v", a.runTracker, b.runTracker)
	}
}

func TestNewAgentService_RegistersSandboxClient(t *testing.T) {
	t.Parallel()
	agenttool.SetSandboxClient(nil)
	t.Cleanup(func() { agenttool.SetSandboxClient(nil) })

	_ = NewAgentService()

	if stub, ok := agenttool.GetSandboxClient().(interface{ IsStubSandboxClient() bool }); ok && stub.IsStubSandboxClient() {
		t.Fatal("sandbox client remained stub after NewAgentService boot wiring")
	}
}
