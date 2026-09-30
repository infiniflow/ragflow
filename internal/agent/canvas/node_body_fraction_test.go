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

	"ragflow/internal/agent/runtime"

	"gorm.io/gorm"
)

// fractionComponent reports a fixed fraction from inside Invoke, the way a
// real component reports pages parsed or chunks embedded.
type fractionComponent struct {
	fraction float64
}

func (f *fractionComponent) Name() string { return "fraction" }

func (f *fractionComponent) Invoke(ctx context.Context, _ *gorm.DB, in map[string]any) (map[string]any, error) {
	runtime.ReportComponentFraction(ctx, f.fraction)
	return in, nil
}

func (f *fractionComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

func (f *fractionComponent) Inputs() map[string]string  { return nil }
func (f *fractionComponent) Outputs() map[string]string { return nil }

// TestRealComponentBody_BindsFractionToNodeID verifies realComponentBody is
// the single injection point that attributes fraction reports to the running
// node: the component reports a bare fraction and the run-level callback
// receives it under the node's cpnID. Two nodes of the same class must not
// cross-attribute.
func TestRealComponentBody_BindsFractionToNodeID(t *testing.T) {
	var got [][2]interface{}
	ctx := runtime.WithProgressFractionCallback(t.Context(), func(component string, fraction float64) {
		got = append(got, [2]interface{}{component, fraction})
	})

	bodyA := realComponentBody("Parser:aaa", "TestFraction", &fractionComponent{fraction: 0.25})
	bodyB := realComponentBody("Parser:bbb", "TestFraction", &fractionComponent{fraction: 0.75})
	if _, err := bodyA(ctx, map[string]any{}); err != nil {
		t.Fatalf("bodyA: %v", err)
	}
	if _, err := bodyB(ctx, map[string]any{}); err != nil {
		t.Fatalf("bodyB: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("fraction reports = %d, want 2", len(got))
	}
	if got[0][0] != "Parser:aaa" || got[0][1] != 0.25 {
		t.Fatalf("report 0 = %v, want [Parser:aaa 0.25]", got[0])
	}
	if got[1][0] != "Parser:bbb" || got[1][1] != 0.75 {
		t.Fatalf("report 1 = %v, want [Parser:bbb 0.75]", got[1])
	}
}

// TestRealComponentBody_NoFractionSinkIsNoop verifies a headless run (no
// run-level callback) pays nothing and components still report freely.
func TestRealComponentBody_NoFractionSinkIsNoop(t *testing.T) {
	body := realComponentBody("test-cpn", "TestFraction", &fractionComponent{fraction: 0.5})
	if _, err := body(t.Context(), map[string]any{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
