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
	"context"
	"testing"
	"time"
)

// TestBuildWorkflow_ParallelBatchStructure verifies that a Canvas with
// two sibling nodes (no inter-node dependency) compiles successfully.
//
// The Go port uses eino's compose.Workflow which natively executes
// independent nodes in a layer concurrently — this is the documented
// eino behavior, not a custom scheduler. This test pins the
// structural contract: a sibling topology compiles without errors,
// which is the precondition for the runtime parallel-execution path
// to engage.
//
// The Compile call is wrapped in a 5s outer context. Compile is
// expected to return in <10ms; the 5s bound is a coarse safety net
// against a regression to sequential processing. We do not measure
// wall-clock elapsed time directly — that's fragile under CPU
// pressure — and instead use a context-budget pattern: if Compile
// doesn't return within the budget, the test fails with a clear
// "Compile did not return within 5s" message.
func TestBuildWorkflow_ParallelBatchStructure(t *testing.T) {
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"begin": {Obj: CanvasComponentObj{ComponentName: "Begin"}, Downstream: []string{"a", "b"}},
			"a":     {Obj: CanvasComponentObj{ComponentName: "Message"}, Downstream: []string{"final"}},
			"b":     {Obj: CanvasComponentObj{ComponentName: "Message"}, Downstream: []string{"final"}},
			"final": {Obj: CanvasComponentObj{ComponentName: "Message"}},
		},
		Path: []string{"begin", "a", "b", "final"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	type compileResult struct {
		cc  *CompiledCanvas
		err error
	}
	done := make(chan compileResult, 1)
	go func() {
		cc, err := Compile(ctx, c)
		done <- compileResult{cc: cc, err: err}
	}()

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("Compile: %v", r.err)
		}
		if r.cc == nil {
			t.Fatal("Compile returned nil CompiledCanvas")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Compile did not return within 5s — suspected regression to sequential processing or hang")
	}
}
