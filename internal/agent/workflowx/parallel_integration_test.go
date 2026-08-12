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

// parallel_integration_test.go — full eino integration tests
// for the parallel extension. These tests use a real
// compose.Workflow. The unit tests in parallel_test.go cover
// the helpers and state machine; the integration tests here
// cover the end-to-end contract.
package workflowx

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/compose"
)

// TestIntegration_Stream_OuterUnsupported asserts the v1
// outer-stream contract end-to-end through the compiled
// workflow. The Stream() call must return the documented
// ErrParallelOuterStreamUnsupported.
func TestIntegration_Stream_OuterUnsupported(t *testing.T) {
	outer := compose.NewWorkflow[[]int, []int]()
	pNode, err := AddParallelNode(context.Background(), outer, "par",
		buildParallelIncSub(t),
	)
	if err != nil {
		t.Fatalf("AddParallelNode: %v", err)
	}
	pNode.AddInput(compose.START)
	outer.End().AddInput("par")
	compiled, err := outer.Compile(context.Background())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	_, err = compiled.Stream(context.Background(), []int{1, 2, 3})
	if err == nil {
		t.Fatal("expected stream-unsupported error, got nil")
	}
	if !errors.Is(err, ErrParallelOuterStreamUnsupported) {
		t.Errorf("errors.Is(err, ErrParallelOuterStreamUnsupported) = false; err = %v", err)
	}
	if !strings.Contains(err.Error(), "v1") {
		t.Errorf("error %q must mention v1", err.Error())
	}
}

// TestIntegration_WithForceNewRun_ResetsState asserts that
// when the parallel extension sees a fresh ctx (no prior
// parallel state), the next run is treated as a fresh run —
// the same semantics as eino's WithForceNewRun. We exercise
// the contract at the runParallelInvoke level.
