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

// parallel_helpers_test.go — shared helpers used by the
// parallel extension's test files. Kept in a separate file so
// production code does not have to compile them.
package workflowx

import (
	"context"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// injectResumeState wires a test-only backdoor payload into
// ctx. The parallel snapshot loader checks
// parallelResumeBackdoorKey first; this lets unit tests drive
// the resume path without a real eino checkpoint store.
func injectResumeState(ctx context.Context, payload []byte) context.Context {
	return context.WithValue(ctx, parallelResumeBackdoorKey{}, payload)
}

// testCountingRunnable is a hand-rolled compose.Runnable used
// by the parallel extension's unit tests. It records the
// number of Invoke calls and optionally blocks on a release
// channel so the test can observe in-flight concurrency.
//
// This is a stand-in for the eino Workflow which serialises
// concurrent Invoke calls internally; for unit-testing the
// fan-out helper itself we need a Runnable that is safe to
// invoke from multiple goroutines simultaneously.
type testCountingRunnable struct {
	fn func(ctx context.Context, in int, opts ...compose.Option) (int, error)
}

func (r testCountingRunnable) Invoke(ctx context.Context, in int, opts ...compose.Option) (int, error) {
	return r.fn(ctx, in, opts...)
}

func (r testCountingRunnable) Stream(ctx context.Context, in int, opts ...compose.Option) (*schema.StreamReader[int], error) {
	out, err := r.fn(ctx, in, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]int{out}), nil
}

func (r testCountingRunnable) Collect(ctx context.Context, in *schema.StreamReader[int], opts ...compose.Option) (int, error) {
	v, err := in.Recv()
	if err != nil {
		return 0, err
	}
	return r.fn(ctx, v, opts...)
}

func (r testCountingRunnable) Transform(ctx context.Context, in *schema.StreamReader[int], opts ...compose.Option) (*schema.StreamReader[int], error) {
	v, err := in.Recv()
	if err != nil {
		return nil, err
	}
	out, err := r.fn(ctx, v, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]int{out}), nil
}
