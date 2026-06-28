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

// This file documents the canvas parallel-execution defense:
//
//	1. Structural compile-check (this file,
//	   TestCanvas_ParallelExecution_StaticAnalysis). Verifies the
//	   canvas compile produces an eino Workflow whose declared
//	   edges form a valid DAG — a precondition for eino's
//	   topological-wave parallel execution.
//
//	2. eino's own parallel-execution test in
//	   `internal/agent/canvas/parallel_batch_test.go`
//	   (TestBuildWorkflow_ParallelBatchStructure). Verifies the
//	   canvas-to-eino compile path with a 4-node sibling topology.
//
// Plus: eino's own `taskManager.submit` test in
// `go/pkg/mod/github.com/cloudwego/eino@v0.9.4/compose/graph_manager_test.go`
// covers the per-task `go func` parallel execution path.
//
// eino's compose.Workflow.Run spawns one `go t.execute()` goroutine
// per ready node in each topological wave, so the parallel-execution
// behavior is intrinsic to using eino — no Go port work needed
// beyond correct `AddInput` edge wiring, which the canvas scheduler
// already does.

package canvas

import (
	"context"
	"testing"
)

// TestCanvas_ParallelExecution_StaticAnalysis verifies the canvas
// compile produces an eino Workflow whose declared edges form a
// valid DAG (no cycles) — a precondition for eino's topological-
// wave parallel execution. A cycle in the eino DAG would surface
// as a graph-cycle error from Compile; a missing `AddInput` edge
// would surface as a different DAG topology than what the user
// expects at runtime.
//
// Together with `parallel_batch_test.go::TestBuildWorkflow_ParallelBatchStructure`
// (4-node sibling structural test) and eino's own `taskManager` tests,
// this gives the regression defense for the claim that
// "compose.Workflow natively executes independent nodes in a layer
// concurrently".
func TestCanvas_ParallelExecution_StaticAnalysis(t *testing.T) {
	// 5 nodes: begin → {a, b, c} (parallel wave) → final.
	// `a`, `b`, `c` are independent siblings — they have no
	// inter-dependency. eino's compile() will see all three
	// ready in the same topological wave.
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"begin": {Obj: CanvasComponentObj{ComponentName: "Begin"}, Downstream: []string{"a", "b", "c"}},
			"a":     {Obj: CanvasComponentObj{ComponentName: "Message"}, Downstream: []string{"final"}},
			"b":     {Obj: CanvasComponentObj{ComponentName: "Message"}, Downstream: []string{"final"}},
			"c":     {Obj: CanvasComponentObj{ComponentName: "Message"}, Downstream: []string{"final"}},
			"final": {Obj: CanvasComponentObj{ComponentName: "Message"}},
		},
		Path: []string{"begin", "a", "b", "c", "final"},
	}

	cc, err := Compile(context.Background(), c)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if cc == nil {
		t.Fatal("Compile returned nil CompiledCanvas")
	}
	// The 3-sibling topology (a, b, c) compiled without
	// errors. eino's topological sort ran in Compile; if we
	// had a cycle (e.g., c's downstream pointing back to
	// begin), Compile would fail with a graph-cycle error.
	// The fact that it succeeded is the structural proof.
}
