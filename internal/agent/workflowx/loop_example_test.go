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

// loop_example_test.go — the canonical "look here first" usage of
// AddLoopNode. The single test in this file mirrors what a reader
// would copy out of godoc, but as a real Test* with an explicit
// assertion on the loop output. It is kept in its own file (rather
// than mixed in with loop_integration_test.go) so newcomers can find
// the smallest working example without scrolling past dozens of
// interrupt / resume / stream-mode tests.
//
// If you change this test, also revisit the package doc comment in
// loop.go and the .claude/plans/eino-workflow-loop.md plan, since
// this file is the de-facto runnable documentation.
package workflowx

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/compose"
)

// TestExample_AddLoopNode is the canonical end-to-end
// "happy path" usage of AddLoopNode. It was migrated here from a
// standalone Example function so the assertion on the loop output
// is explicit (an Example only compares stdout to a comment, which
// is fragile and silently passes when Println is dropped).
//
// The nested workflow increments its input by 1. The outer loop
// uses the do-while contract via shouldQuit(next >= 3), so iterations
// run as: in=0 -> 1, in=1 -> 2, in=2 -> 3 (quit). Final output: 3.
func TestExample_AddLoopNode(t *testing.T) {
	ctx := context.Background()

	sub := compose.NewWorkflow[int, int]()
	inc := compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		return in + 1, nil
	})
	subNode := sub.AddLambdaNode("inc", inc)
	subNode.AddInput(compose.START)
	sub.End().AddInput("inc")

	outer := compose.NewWorkflow[int, int]()
	shouldQuit := func(_ context.Context, _, _, next int) (bool, error) {
		return next >= 3, nil
	}

	loopNode, err := AddLoopNode(ctx, outer, "loop", sub, shouldQuit,
		WithLoopMaxIterations(10),
		WithLoopCheckpointIDBuilder(func(_ string, iter int) string {
			return "example-loop:" + itoa(iter)
		}),
	)
	if err != nil {
		t.Fatalf("AddLoopNode: %v", err)
	}
	loopNode.AddInput(compose.START)
	outer.End().AddInput("loop")

	runner, err := outer.Compile(ctx)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	out, err := runner.Invoke(ctx, 0)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if out != 3 {
		t.Fatalf("output: got %d, want 3", out)
	}
}
