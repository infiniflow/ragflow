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

// Package canvas — public re-export of withState for cross-package tests.
//
// The package-internal withState attaches *CanvasState to a context so
// GetStateFromContext can retrieve it. It is unexported because the
// production call site is exactly one: the orchestrator's compile entry
// (compile.go). External callers should never need to inject state
// themselves.
//
// Cross-package unit tests (e.g. internal/agent/component/*_test.go) do
// need a way to set up a state for component Invoke() calls. This file
// exposes a single thin re-export — WithState — that the test code in
// other packages can call. Production code paths are not affected:
// nothing in the production binary calls WithState; the orchestrator
// keeps using the unexported withState directly.
package canvas

import (
	"context"

	"ragflow/internal/agent/runtime"
)

// WithState attaches *CanvasState to ctx for retrieval by
// GetStateFromContext. Intended ONLY for cross-package test setup
// (production code uses the unexported withState via compile.go).
// Both entry points delegate to runtime.WithState.
func WithState(ctx context.Context, s *CanvasState) context.Context {
	return runtime.WithState(ctx, s)
}
