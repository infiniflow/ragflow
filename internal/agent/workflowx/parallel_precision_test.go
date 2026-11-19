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

package workflowx

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/eino/compose"
)

func TestParallelResumePreservesCompletedAndDynamicInputNumbers(t *testing.T) {
	for _, concurrency := range []int{0, 2} {
		t.Run(fmt.Sprint(concurrency), func(t *testing.T) {
			const largeID = "11099848653006015581"
			var calls [2]atomic.Int32
			sub := compose.NewWorkflow[map[string]any, map[string]any]()
			op := sub.AddLambdaNode("op", compose.InvokableLambda(func(ctx context.Context, in map[string]any) (map[string]any, error) {
				index := int(in["index"].(float64))
				if calls[index].Add(1) == 1 && index == 1 {
					return nil, compose.StatefulInterrupt(ctx, "wait", index)
				}
				return in, nil
			}))
			op.AddInput(compose.START)
			sub.End().AddInput("op")
			outer := compose.NewWorkflow[[]map[string]any, []map[string]any]()
			par, err := AddParallelNode(t.Context(), outer, "par", sub, WithParallelMaxConcurrency(concurrency), WithParallelEnableSubCheckpoint(false))
			if err != nil {
				t.Fatal(err)
			}
			par.AddInput(compose.START)
			outer.End().AddInput("par")
			compiled, err := outer.Compile(t.Context(), compose.WithCheckPointStore(newInMemoryStore()))
			if err != nil {
				t.Fatal(err)
			}
			items := []map[string]any{
				{"index": float64(0), "nested": []any{map[string]any{"id": json.Number(largeID), "score": 1.5}}},
				{"index": float64(1), "nested": []any{map[string]any{"id": json.Number(largeID), "score": 1.5}}},
			}
			const checkpoint = "parallel-large-ids"
			_, err = compiled.Invoke(t.Context(), items, compose.WithCheckPointID(checkpoint))
			if _, ok := compose.ExtractInterruptInfo(err); !ok {
				t.Fatalf("first run: want interrupt, got %v", err)
			}
			resumed := compose.Resume(t.Context(), firstRootInterruptID(t, err))
			out, err := compiled.Invoke(resumed, nil, compose.WithCheckPointID(checkpoint))
			if err != nil {
				t.Fatal(err)
			}
			if len(out) != 2 {
				t.Fatalf("outputs = %#v", out)
			}
			for i, item := range out {
				nested := item["nested"].([]any)[0].(map[string]any)
				if nested["id"] != json.Number(largeID) {
					t.Errorf("item %d id = %v (%T), want exact %s", i, nested["id"], nested["id"], largeID)
				}
				if nested["score"] != float64(1.5) {
					t.Errorf("item %d score = %v (%T)", i, nested["score"], nested["score"])
				}
			}
			if calls[0].Load() != 1 || calls[1].Load() != 2 {
				t.Fatalf("calls = %d/%d, want 1/2", calls[0].Load(), calls[1].Load())
			}
		})
	}
}

func TestParallelResumePreservesTypedLargeIntegerResults(t *testing.T) {
	t.Run("signed", func(t *testing.T) { assertTypedParallelResume(t, []int64{9007199254740993, 9223372036854775807}) })
	t.Run("unsigned", func(t *testing.T) { assertTypedParallelResume(t, []uint64{11099848653006015581, 18446744073709551615}) })
}

func assertTypedParallelResume[T comparable](t *testing.T, inputs []T) {
	t.Helper()
	calls := map[T]int{}
	sub := compose.NewWorkflow[T, T]()
	op := sub.AddLambdaNode("op", compose.InvokableLambda(func(ctx context.Context, in T) (T, error) {
		calls[in]++
		if in == inputs[1] && calls[in] == 1 {
			var zero T
			return zero, compose.StatefulInterrupt(ctx, "wait", 1)
		}
		return in, nil
	}))
	op.AddInput(compose.START)
	sub.End().AddInput("op")
	outer := compose.NewWorkflow[[]T, []T]()
	par, err := AddParallelNode(t.Context(), outer, "par", sub, WithParallelEnableSubCheckpoint(false))
	if err != nil {
		t.Fatal(err)
	}
	par.AddInput(compose.START)
	outer.End().AddInput("par")
	compiled, err := outer.Compile(t.Context(), compose.WithCheckPointStore(newInMemoryStore()))
	if err != nil {
		t.Fatal(err)
	}
	const checkpoint = "typed-integers"
	_, err = compiled.Invoke(t.Context(), inputs, compose.WithCheckPointID(checkpoint))
	if _, ok := compose.ExtractInterruptInfo(err); !ok {
		t.Fatalf("want interrupt, got %v", err)
	}
	out, err := compiled.Invoke(compose.Resume(t.Context(), firstRootInterruptID(t, err)), nil, compose.WithCheckPointID(checkpoint))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0] != inputs[0] || out[1] != inputs[1] {
		t.Fatalf("result = %#v, want %#v", out, inputs)
	}
	if calls[inputs[0]] != 1 || calls[inputs[1]] != 2 {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestCoerceCompletedLargeIntegerRejectsOverflow(t *testing.T) {
	if _, ok := coerceAnyToO[int64](json.Number("11099848653006015581")); ok {
		t.Fatal("accepted out-of-range signed integer")
	}
	if _, ok := coerceAnyToO[uint64](json.Number("-9007199254740992")); ok {
		t.Fatal("accepted negative unsigned integer")
	}
}
