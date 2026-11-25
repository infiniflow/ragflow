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

// Package canvas — state engine re-exports.
//
// The actual CanvasState type and its GetVar / SetVar / ReadVars
// methods live in internal/agent/runtime/state.go so the component
// package can depend on them without importing canvas. This file
// keeps the package-internal withState helper used by canvas_test.go
// and the cross-package GetStateFromContext re-export.
package canvas

import (
	"context"

	"ragflow/internal/agent/runtime"
)

// withState attaches *CanvasState to ctx. Production code uses this
// once per run from compile.go; cross-package tests use the exported
// WithState (state_export.go) which delegates to the same runtime
// helper.
func withState(ctx context.Context, s *CanvasState) context.Context {
	return runtime.WithState(ctx, s)
}

// GetStateFromContext re-exports runtime.GetStateFromContext so
// canvas-side callers (and tests that already import canvas) keep
// compiling without an extra import.
func GetStateFromContext(ctx context.Context) (*CanvasState, error) {
	return runtime.GetStateFromContext(ctx)
}
