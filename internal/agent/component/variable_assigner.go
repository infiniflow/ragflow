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

// Package component — VariableAssigner (T3, plan §2.11.3 row 20).
//
// VariableAssigner applies an ordered list of (variable, operator,
// parameter) tuples to the shared *CanvasState. Each tuple's operator
// reads the current variable value, computes a new one, and (unless the
// operator returns an "ERROR:..." sentinel) writes the new value back to
// the state bucket the variable ref points at.
//
// The eleven operators mirror agent/component/variable_assigner.py:
//
//	overwrite, clear, set, append, extend, remove_first, remove_last,
//	"+= -= *= /="
//
// Variable refs may target cpn outputs ("cpn_0@x"), the sys namespace
// ("sys.x"), the env namespace ("env.x"), or iteration aliases
// ("item" / "index"). Cpn-typed refs are split on the first "@" into
// (cpnID, param) and written via SetVar; sys/env/item/index are written
// to their respective CanvasState maps directly.
//
// On "ERROR:..." returns the operator result is exposed at
// outputs["errors"] and the state bucket is left unchanged.
package component

import (
	"context"
	"fmt"
	"strings"

	"ragflow/internal/agent/runtime"
)

const componentNameVariableAssigner = "VariableAssigner"

// variableAssignerParam is the static configuration. variables is a
// list of {variable, operator, parameter} dicts.
type variableAssignerParam struct {
	Variables []map[string]any `json:"variables"`
}

// Update copies a fresh param map into the receiver. `variables` may
// arrive as []any (engine-decoded from JSON) or []map[string]any
// (test/direct construction); both shapes are accepted.
func (p *variableAssignerParam) Update(conf map[string]any) error {
	if conf == nil {
		p.Variables = nil
		return nil
	}
	raw, ok := conf["variables"]
	if !ok {
		p.Variables = nil
		return nil
	}
	var list []any
	switch x := raw.(type) {
	case []any:
		list = x
	case []map[string]any:
		list = make([]any, 0, len(x))
		for _, v := range x {
			list = append(list, v)
		}
	default:
		return &ParamError{Field: "variables", Reason: "must be a list"}
	}
	out := make([]map[string]any, 0, len(list))
	for i, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return &ParamError{Field: fmt.Sprintf("variables[%d]", i), Reason: "must be a map"}
		}
		out = append(out, m)
	}
	p.Variables = out
	return nil
}

// Check is a no-op for VariableAssigner; the Python base class also
// returns True unconditionally.
func (p *variableAssignerParam) Check() error { return nil }

// AsDict returns the params as a plain map.
func (p *variableAssignerParam) AsDict() map[string]any {
	out := map[string]any{"variables": make([]any, 0, len(p.Variables))}
	for _, v := range p.Variables {
		out["variables"] = append(out["variables"].([]any), v)
	}
	return out
}

// VariableAssignerComponent applies the configured (variable, operator,
// parameter) tuples to the canvas state.
type VariableAssignerComponent struct {
	name  string
	param variableAssignerParam
}

// NewVariableAssignerComponent constructs a VariableAssigner from the
// DSL param map.
func NewVariableAssignerComponent(params map[string]any) (Component, error) {
	p := &variableAssignerParam{}
	if err := p.Update(params); err != nil {
		return nil, fmt.Errorf("VariableAssigner: param update: %w", err)
	}
	if err := p.Check(); err != nil {
		return nil, fmt.Errorf("VariableAssigner: param check: %w", err)
	}
	return &VariableAssignerComponent{
		name:  componentNameVariableAssigner,
		param: *p,
	}, nil
}

// Name returns the registered component name.
func (v *VariableAssignerComponent) Name() string { return v.name }

// Invoke walks the param.variables list, evaluates each tuple against
// the canvas state, and writes the result back unless the operator
// returned an "ERROR:..." sentinel. The list of refs that were
// assigned is returned at outputs["assignments"]; per-item errors (if
// any) are returned at outputs["errors"].
func (v *VariableAssignerComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil {
		return nil, fmt.Errorf("VariableAssigner: %w", err)
	}
	if state == nil {
		return nil, fmt.Errorf("VariableAssigner: nil canvas state")
	}

	items := v.param.Variables
	// Allow runtime override via inputs["variables"] (a list of tuples
	// in the same shape as param.variables).
	if override, ok := inputs["variables"].([]any); ok && len(override) > 0 {
		items = items[:0]
		for _, raw := range override {
			if m, ok := raw.(map[string]any); ok {
				items = append(items, m)
			}
		}
	}

	assignments := make([]string, 0, len(items))
	var errors []string
	for i, item := range items {
		ref, _ := item["variable"].(string)
		op, _ := item["operator"].(string)
		param, _ := item["parameter"]
		if ref == "" || op == "" {
			return nil, &ParamError{
				Field:  fmt.Sprintf("variables[%d]", i),
				Reason: "variable and operator must be non-empty",
			}
		}
		oldVal, err := state.GetVar(ref)
		if err != nil {
			// bad ref shape — surface as an error rather than silently skip
			return nil, fmt.Errorf("VariableAssigner: variables[%d] get %q: %w", i, ref, err)
		}
		newVal, opErr := operate(state, oldVal, op, param)
		if opErr != "" {
			errors = append(errors, fmt.Sprintf("variables[%d] %s: %s", i, ref, opErr))
			continue
		}
		if err := writeVar(state, ref, newVal); err != nil {
			return nil, fmt.Errorf("VariableAssigner: variables[%d] write %q: %w", i, ref, err)
		}
		assignments = append(assignments, ref)
	}
	out := map[string]any{"assignments": assignments}
	if len(errors) > 0 {
		out["errors"] = errors
	}
	return out, nil
}

// Stream mirrors Invoke; VariableAssigner is a single-shot apply.
func (v *VariableAssignerComponent) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out, err := v.Invoke(ctx, inputs)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]any, 1)
	ch <- out
	close(ch)
	return ch, nil
}

// Inputs returns the public parameter surface.
func (v *VariableAssignerComponent) Inputs() map[string]string {
	return map[string]string{
		"variables": "Optional runtime override: a list of {variable, operator, parameter} dicts.",
	}
}

// Outputs returns the assigned refs and any per-item errors.
func (v *VariableAssignerComponent) Outputs() map[string]string {
	return map[string]string{
		"assignments": "List of refs that were successfully written back to state.",
		"errors":      "Per-item error messages; absent when all operators succeeded.",
	}
}

// operate applies the operator. Returns ("", "ERROR:...") on failure;
// the caller treats the empty string opErr as success and the new value
// as the value to write back.
func operate(state *runtime.CanvasState, oldVal any, op string, param any) (any, string) {
	switch op {
	case "overwrite":
		// overwrite: new = canvas.get_variable_value(parameter).
		// parameter is itself a ref string (possibly wrapped in
		// {{...}}); strip the wrapping before lookup so
		// "{{cpn_1@y}}" resolves to cpn_1's "y" output value.
		if s, ok := param.(string); ok {
			bare := stripVarBraces(s)
			v, err := state.GetVar(bare)
			if err != nil {
				return nil, "ERROR:PARAMETER_UNRESOLVED"
			}
			return v, ""
		}
		// If parameter is not a string, the Python code calls
		// get_variable_value which expects a string. Pass through.
		return param, ""

	case "clear":
		switch oldVal.(type) {
		case nil:
			return nil, ""
		case []any:
			return []any{}, ""
		case string:
			return "", ""
		case map[string]any:
			return map[string]any{}, ""
		case bool:
			return false, ""
		case int, int64, float64, float32:
			return 0, ""
		}
		return nil, ""

	case "set":
		switch oldVal.(type) {
		case nil, string:
			// Try to interpret parameter as a ref (or {{...}} template);
			// fall back to the raw value when it doesn't look like one.
			if s, ok := param.(string); ok && s != "" {
				if v, err := state.GetVar(s); err == nil && v != nil {
					return v, ""
				}
				// also try template resolution against state for {{...}}
				if strings.Contains(s, "{{") {
					if resolved, err := runtime.ResolveTemplate(s, state); err == nil {
						return resolved, ""
					}
				}
			}
			return param, ""
		default:
			return param, ""
		}

	case "append":
		p, _ := state.GetVar(asRefString(param))
		// when param is a non-ref literal, p is "" — fall back to raw
		_ = p
		p = resolveParamValue(state, param)
		if oldVal == nil {
			oldVal = []any{}
		}
		lst, ok := oldVal.([]any)
		if !ok {
			return nil, "ERROR:VARIABLE_NOT_LIST"
		}
		if len(lst) > 0 {
			if !compatibleElemType(lst[0], p) {
				return nil, "ERROR:PARAMETER_NOT_LIST_ELEMENT_TYPE"
			}
		}
		// append returns the original list mutated
		lst = append(lst, p)
		return lst, ""

	case "extend":
		p := resolveParamValue(state, param)
		if oldVal == nil {
			oldVal = []any{}
		}
		lst, ok := oldVal.([]any)
		if !ok {
			return nil, "ERROR:VARIABLE_NOT_LIST"
		}
		pl, ok := p.([]any)
		if !ok {
			return nil, "ERROR:PARAMETER_NOT_LIST"
		}
		if len(lst) > 0 && len(pl) > 0 {
			if !compatibleElemType(lst[0], pl[0]) {
				return nil, "ERROR:PARAMETER_NOT_LIST_ELEMENT_TYPE"
			}
		}
		return append(lst, pl...), ""

	case "remove_first":
		lst, ok := oldVal.([]any)
		if !ok {
			return nil, "ERROR:VARIABLE_NOT_LIST"
		}
		if len(lst) == 0 {
			return lst, ""
		}
		return lst[1:], ""

	case "remove_last":
		lst, ok := oldVal.([]any)
		if !ok {
			return nil, "ERROR:VARIABLE_NOT_LIST"
		}
		if len(lst) == 0 {
			return lst, ""
		}
		return lst[:len(lst)-1], ""

	case "+=":
		pv := resolveParamValue(state, param)
		if !isNumberish(oldVal) || !isNumberish(pv) {
			return nil, "ERROR:VARIABLE_NOT_NUMBER or PARAMETER_NOT_NUMBER"
		}
		return toFloat64(oldVal) + toFloat64(pv), ""

	case "-=":
		pv := resolveParamValue(state, param)
		if !isNumberish(oldVal) || !isNumberish(pv) {
			return nil, "ERROR:VARIABLE_NOT_NUMBER or PARAMETER_NOT_NUMBER"
		}
		return toFloat64(oldVal) - toFloat64(pv), ""

	case "*=":
		pv := resolveParamValue(state, param)
		if !isNumberish(oldVal) || !isNumberish(pv) {
			return nil, "ERROR:VARIABLE_NOT_NUMBER or PARAMETER_NOT_NUMBER"
		}
		return toFloat64(oldVal) * toFloat64(pv), ""

	case "/=":
		pv := resolveParamValue(state, param)
		if !isNumberish(oldVal) || !isNumberish(pv) {
			return nil, "ERROR:VARIABLE_NOT_NUMBER or PARAMETER_NOT_NUMBER"
		}
		if toFloat64(pv) == 0 {
			return nil, "ERROR:DIVIDE_BY_ZERO"
		}
		return toFloat64(oldVal) / toFloat64(pv), ""
	}
	return nil, "ERROR:UNKNOWN_OPERATOR"
}

// writeVar routes a ref to the correct CanvasState bucket.
//
//   - "cpn_id@param..." → SetVar(cpnID, param...)
//   - "sys.x"           → Sys["x"] = v
//   - "env.x"           → Env["x"] = v
//   - "item"            → Globals["__item__"] = v
//   - "index"           → Globals["__index__"] = v
func writeVar(state *runtime.CanvasState, ref string, v any) error {
	switch {
	case ref == "item":
		state.Globals["__item__"] = v
		return nil
	case ref == "index":
		state.Globals["__index__"] = v
		return nil
	case strings.HasPrefix(ref, "sys."):
		state.Sys[strings.TrimPrefix(ref, "sys.")] = v
		return nil
	case strings.HasPrefix(ref, "env."):
		state.Env[strings.TrimPrefix(ref, "env.")] = v
		return nil
	}
	idx := strings.Index(ref, "@")
	if idx <= 0 {
		return fmt.Errorf("invalid variable ref %q", ref)
	}
	cpnID, param := ref[:idx], ref[idx+1:]
	state.SetVar(cpnID, param, v)
	return nil
}

// asRefString returns param as a string ref if it is one, "" otherwise.
// Used to gate "should I look this up in state" decisions.
func asRefString(param any) string {
	if s, ok := param.(string); ok {
		return s
	}
	return ""
}

// stripVarBraces removes one or two layers of surrounding `{` `}` plus
// whitespace from s, matching the chained .strip("{").strip("}").strip(" ").
// strip("{").strip("}") at agent/canvas.py:196. The double-layer case
// handles "{{cpn_1@y}}" → "cpn_1@y" so canvas.GetVar can resolve it
// against the cpn output bucket.
func stripVarBraces(s string) string {
	s = strings.TrimSpace(s)
	for range 2 {
		if len(s) >= 2 && s[0] == '{' && s[len(s)-1] == '}' {
			s = strings.TrimSpace(s[1 : len(s)-1])
			continue
		}
		break
	}
	return s
}

// resolveParamValue returns the value of param. Strings are looked up
// in state first (treating them as refs); anything else is passed
// through unchanged. The Python _canvas.get_variable_value does the
// same after stripping the surrounding {{ }}; non-string params
// (numbers, lists, dicts) are passed verbatim.
func resolveParamValue(state *runtime.CanvasState, param any) any {
	if s, ok := param.(string); ok && s != "" {
		// Try the parameter as a bare ref first (matches Python's
		// canvas.get_variable_value which strips braces then splits on @).
		bare := stripVarBraces(s)
		if v, err := state.GetVar(bare); err == nil && v != nil {
			return v
		}
		// Fall back to the original string for cases where the param
		// contains template fragments that didn't fully resolve.
	}
	return param
}

// compatibleElemType mirrors the Python isinstance check used in
// _append / _extend. The Python code uses strict isinstance; the Go
// port relaxes this to "same Go kind" (int / float64 / string) so
// JSON-decoded numbers from LLM output compose correctly.
func compatibleElemType(a, b any) bool {
	return goKind(a) == goKind(b)
}

func goKind(v any) string {
	switch v.(type) {
	case int, int64, int32:
		return "int"
	case float64, float32:
		return "float"
	case string:
		return "string"
	case bool:
		return "bool"
	case map[string]any:
		return "map"
	case []any:
		return "list"
	}
	return "unknown"
}

// isNumberish returns true for numeric values (int, float, including
// JSON-decoded numbers). Booleans are explicitly excluded — Python's
// numbers.Number is a superclass of int/float/complex/Decimal but
// Python's isinstance(True, numbers.Number) is False; the spec matches
// that.
func isNumberish(v any) bool {
	if _, ok := v.(bool); ok {
		return false
	}
	switch v.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return true
	}
	return false
}

// toFloat64 converts any numeric value to float64. Callers must guard
// with isNumberish first.
func toFloat64(v any) float64 {
	switch x := v.(type) {
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case int32:
		return float64(x)
	case float64:
		return x
	case float32:
		return float64(x)
	}
	return 0
}

func init() {
	Register(componentNameVariableAssigner, NewVariableAssignerComponent)
}
