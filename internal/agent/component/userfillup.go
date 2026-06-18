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

// Package component — UserFillUp component (T3).
//
// UserFillUp is the user-interaction / form-filling node. It
// renders an optional `tips` template (with `{{key}}` placeholders
// resolved against the form's input map) and passes each form
// field through to its downstream outputs. File-type inputs
// (value.type starts with "file") emit a stable "<file:key>" stub
// so the run keeps flowing while the FileService integration
// surfaces the actual bytes via the storage layer.
//
// Mirrors agent/component/fillup.py.
package component

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"ragflow/internal/agent/runtime"
)

const componentNameUserFillUp = "UserFillUp"

// defaultUserFillUpTips is used when the operator omits the `tips` param.
// Matches the Python default in fillup.py:28.
const defaultUserFillUpTips = "Please fill up the form"

// fileStubPrefix is prepended to the form-field key when an input is
// classified as a file-type input. The FileService.get_files payload
// surfaces the actual bytes via the storage layer.
const fileStubPrefix = "<file:"

// tipsPlaceholderPattern matches `{{key}}` placeholders in the tips
// template. The pattern intentionally only accepts simple identifiers
// (matching Python's `re.sub(r"\{%s\}"%k, ...)` in fillup.py:62) — the
// placeholder key is looked up in the form's input map, not the full
// canvas-state ref grammar (cpn_id@param / sys.x / env.x).
var tipsPlaceholderPattern = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

// userFillUpParam is the per-instance configuration for UserFillUp.
//
// Mirrors UserFillUpParam in fillup.py:24-33 (Python).
type userFillUpParam struct {
	EnableTips      bool   `json:"enable_tips"`
	Tips            string `json:"tips"`
	LayoutRecognize string `json:"layout_recognize"`
}

// Update copies a fresh params map into the receiver, applying defaults
// for any omitted keys. Returns nil on success (param validation is
// performed by Check, not here — mirrors Python's two-phase pattern).
func (p *userFillUpParam) Update(conf map[string]any) error {
	if conf == nil {
		conf = map[string]any{}
	}
	if v, ok := boolFrom(conf, "enable_tips"); ok {
		p.EnableTips = v
	} else {
		// Default to true when the DSL omits the key entirely.
		p.EnableTips = true
	}
	if v, ok := stringFrom(conf, "tips"); ok {
		p.Tips = v
	} else if p.Tips == "" {
		p.Tips = defaultUserFillUpTips
	}
	if v, ok := stringFrom(conf, "layout_recognize"); ok {
		p.LayoutRecognize = v
	}
	return nil
}

// Check performs parameter validation. UserFillUp has no required
// fields — both the Python and Go implementations accept any config and
// degrade gracefully on missing template data. The method is kept to
// satisfy the ParamBase contract.
func (p *userFillUpParam) Check() error { return nil }

// AsDict returns the param as a plain map for serialization / debug.
func (p *userFillUpParam) AsDict() map[string]any {
	return map[string]any{
		"enable_tips":      p.EnableTips,
		"tips":             p.Tips,
		"layout_recognize": p.LayoutRecognize,
	}
}

// UserFillUpComponent is the canvas form-filling node.
type UserFillUpComponent struct {
	name  string
	param userFillUpParam
}

// NewUserFillUpComponent builds a UserFillUpComponent from a DSL params
// map. The map is shallow-copied into the embedded param; pass-through
// is keyed off the param's exported fields, not the original conf.
func NewUserFillUpComponent(p userFillUpParam) *UserFillUpComponent {
	return &UserFillUpComponent{name: componentNameUserFillUp, param: p}
}

// Name returns the registered component name.
func (u *UserFillUpComponent) Name() string { return u.name }

// Invoke renders the tips template (when enable_tips) and emits one
// output per form field. Inputs are expected under the top-level
// "inputs" key, mirroring the Python `kwargs.get("inputs", {})`
// contract in fillup.py:66.
func (u *UserFillUpComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	// State is required for the canvas ref grammar, but UserFillUp's
	// tips substitution uses simple {{key}} placeholders resolved
	// against the form input map. We still extract state so a
	// nil-state error surfaces early.
	if _, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx); err != nil {
		return nil, fmt.Errorf("UserFillUp: %w", err)
	}

	fields, _ := formFields(inputs)
	out := make(map[string]any, len(fields)+1)

	if u.param.EnableTips {
		rendered := renderTips(u.param.Tips, fields)
		out["tips"] = rendered
	}

	for k, v := range fields {
		out[k] = resolveFieldValue(k, v)
	}
	return out, nil
}

// Stream is the synchronous facade over Invoke: a single payload, then
// close. SSE streaming of the rendered tips is not meaningful for the
// P3 port (form-fill is a one-shot interaction in the DSL).
func (u *UserFillUpComponent) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out, err := u.Invoke(ctx, inputs)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]any, 1)
	ch <- out
	close(ch)
	return ch, nil
}

// Inputs returns parameter metadata for tooling. The "inputs" key is
// the form-field map; the rest are per-instance config.
func (u *UserFillUpComponent) Inputs() map[string]string {
	return map[string]string{
		"inputs":           "Map of form-field name → {value, type, optional?}.",
		"enable_tips":      "Render the `tips` template (default true).",
		"tips":             "Template string with {{key}} placeholders, resolved against the form fields.",
		"layout_recognize": "Layout recognizer hint used for file inputs.",
	}
}

// Outputs returns the rendered tips plus one entry per form field.
func (u *UserFillUpComponent) Outputs() map[string]string {
	return map[string]string{
		"tips": "Rendered tips string (only when enable_tips=true).",
		"*":    "One output per form-field key in inputs.",
	}
}

// formFields extracts the per-field map from the component's input
// payload. Returns an empty map (not an error) when the key is absent
// or malformed — mirrors the Python `kwargs.get("inputs", {})` shape.
func formFields(inputs map[string]any) (map[string]any, bool) {
	raw, ok := inputs["inputs"]
	if !ok {
		return map[string]any{}, false
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return map[string]any{}, false
	}
	return m, true
}

// renderTips substitutes every {{key}} placeholder in template with
// the corresponding field's value. File-type fields render as
// "<file:key>" stubs. Plain string fields use their value
// verbatim; non-string values are coerced via fmt.Sprintf("%v", ...).
func renderTips(template string, fields map[string]any) string {
	if template == "" {
		return ""
	}
	return tipsPlaceholderPattern.ReplaceAllStringFunc(template, func(match string) string {
		sub := tipsPlaceholderPattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		key := sub[1]
		raw, ok := fields[key]
		if !ok {
			return ""
		}
		return fieldValueToString(key, raw)
	})
}

// resolveFieldValue converts one form-field payload into the value
// that should appear in the component's output map.
//
// Rules (mirroring fillup.py:69-79):
//   - dict with type starting with "file" → "<file:key>" stub
//   - dict with optional=true and value==nil → nil
//   - dict with a `value` field → the inner value
//   - anything else → pass through unchanged
func resolveFieldValue(key string, raw any) any {
	m, ok := raw.(map[string]any)
	if !ok {
		return raw
	}
	if isFileType(m) {
		return fileStubPrefix + key + ">"
	}
	if opt, _ := m["optional"].(bool); opt {
		if v, present := m["value"]; !present || v == nil {
			return nil
		}
	}
	if v, present := m["value"]; present {
		return v
	}
	return m
}

// isFileType reports whether the form-field payload's `type` field
// starts with "file" (case-insensitive). Matches fillup.py:69's
// `v.get("type", "").lower().find("file") >= 0` test.
func isFileType(m map[string]any) bool {
	t, _ := m["type"].(string)
	return strings.HasPrefix(strings.ToLower(t), "file")
}

// fieldValueToString is the tips-substitution variant of
// resolveFieldValue: it returns a string suitable for direct insertion
// into the rendered template. File stubs are emitted here too so the
// tips template can reference a file field without crashing.
func fieldValueToString(key string, raw any) string {
	if m, ok := raw.(map[string]any); ok {
		if isFileType(m) {
			return fileStubPrefix + key + ">"
		}
		if v, present := m["value"]; present {
			return stringifyField(v)
		}
		return ""
	}
	return stringifyField(raw)
}

// stringifyField renders a single form-field value as a string for
// template substitution. nil → "", strings stay verbatim, everything
// else uses %v.
func stringifyField(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// init registers UserFillUp with the orchestrator-owned registry.
func init() {
	Register(componentNameUserFillUp, func(params map[string]any) (Component, error) {
		var p userFillUpParam
		if err := p.Update(params); err != nil {
			return nil, err
		}
		return NewUserFillUpComponent(p), nil
	})
}
