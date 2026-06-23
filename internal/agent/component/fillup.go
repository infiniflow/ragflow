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

// Package component — Fillup component (T3).
//
// Fillup is the lighter sibling of UserFillUp: it does NOT render a
// `tips` template. It only passes the form's input map through to
// its outputs. The Python codebase has no separate Fillup class;
// this component is the Go port's normalized, tips-less variant
// of UserFillUp so the DSL can spawn it without paying for the
// unused template path.
package component

import (
	"context"
	"fmt"

	"ragflow/internal/agent/runtime"
)

const componentNameFillup = "Fillup"

// fillupParam is the per-instance configuration for Fillup. It is
// strictly a subset of userFillUpParam — `enable_tips` and `tips` are
// intentionally absent because Fillup never renders tips.
type fillupParam struct {
	LayoutRecognize string `json:"layout_recognize"`
}

// Update copies a fresh params map into the receiver. Layout_recognize
// is the only field; unknown keys are silently ignored to keep the
// Update contract forgiving (mirrors the existing P0/P1 components).
func (p *fillupParam) Update(conf map[string]any) error {
	if conf == nil {
		conf = map[string]any{}
	}
	if v, ok := stringFrom(conf, "layout_recognize"); ok {
		p.LayoutRecognize = v
	}
	return nil
}

// Check performs parameter validation. Fillup has no required fields.
func (p *fillupParam) Check() error { return nil }

// AsDict returns the param as a plain map for serialization / debug.
func (p *fillupParam) AsDict() map[string]any {
	return map[string]any{
		"layout_recognize": p.LayoutRecognize,
	}
}

// FillupComponent is the canvas tips-less form-filling node.
type FillupComponent struct {
	name  string
	param fillupParam
}

// NewFillupComponent builds a FillupComponent from a DSL params map.
func NewFillupComponent(p fillupParam) *FillupComponent {
	return &FillupComponent{name: componentNameFillup, param: p}
}

// Name returns the registered component name.
func (f *FillupComponent) Name() string { return f.name }

// Invoke emits one output per form field, with file-typed fields
// stubbed as "<file:key>". No "tips" key is added — that is the
// defining difference from UserFillUp.
func (f *FillupComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	// State is required by the engine contract; we don't read from it
	// here, but we still extract it to fail loudly if the engine forgot
	// to wire it (consistent with UserFillUp's behavior).
	if _, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx); err != nil {
		return nil, fmt.Errorf("Fillup: %w", err)
	}

	fields, _ := formFields(inputs)
	out := make(map[string]any, len(fields))
	for k, v := range fields {
		out[k] = resolveFieldValue(k, v)
	}
	return out, nil
}

// Stream is the synchronous facade over Invoke: a single payload, then
// close. Mirrors the pattern used by UserFillUp and the P0 components.
func (f *FillupComponent) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out, err := f.Invoke(ctx, inputs)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]any, 1)
	ch <- out
	close(ch)
	return ch, nil
}

// Inputs returns parameter metadata. There is no "tips" surface — the
// DSL editor should not show one for Fillup nodes.
func (f *FillupComponent) Inputs() map[string]string {
	return map[string]string{
		"inputs":           "Map of form-field name → {value, type, optional?}.",
		"layout_recognize": "Layout recognizer hint used for file inputs.",
	}
}

// Outputs returns one entry per form field. The "*" wildcard mirrors
// the UserFillUp contract; no "tips" key is ever emitted.
func (f *FillupComponent) Outputs() map[string]string {
	return map[string]string{
		"*": "One output per form-field key in inputs (file inputs are stubbed as \"<file:key>\").",
	}
}

// init registers Fillup with the orchestrator-owned registry.
func init() {
	Register(componentNameFillup, func(params map[string]any) (Component, error) {
		var p fillupParam
		if err := p.Update(params); err != nil {
			return nil, err
		}
		return NewFillupComponent(p), nil
	})
}
