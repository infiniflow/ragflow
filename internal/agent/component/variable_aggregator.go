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

// Package component — VariableAggregator (T3, plan §2.11.3 row 19).
//
// For each "group" in its param, VariableAggregator walks a list of
// variable selectors and picks the first one whose resolved value is
// truthy. The picked value is exposed at outputs[<group_name>].
//
// Mirrors agent/component/variable_aggregator.py. The Python implementation
// also records each group's variables list under the synthetic input key
// "<group_name>.variables" for engine bookkeeping; the Go port skips that
// side-effect because the canvas engine consumes param data directly via
// the component factory, not via a re-emitted inputs map.
package component

import (
	"context"
	"fmt"

	"ragflow/internal/agent/runtime"
)

const componentNameVariableAggregator = "VariableAggregator"

// variableAggregatorParam is the static configuration loaded from the DSL.
// It mirrors the Python VariableAggregatorParam surface.
type variableAggregatorParam struct {
	// Groups is a list of {group_name, variables} dicts. Each
	// group.variables entry is itself a {value: <ref-string>} dict.
	Groups []map[string]any `json:"groups"`
}

// Update copies a fresh param map into the receiver. Mirrors the Python
// ComponentParamBase contract.
//
// `groups` may arrive as either []any (engine-decoded from JSON) or
// []map[string]any (test/direct construction); both shapes are accepted
// so callers don't have to coerce.
func (p *variableAggregatorParam) Update(conf map[string]any) error {
	if conf == nil {
		p.Groups = nil
		return nil
	}
	rawGroups, ok := conf["groups"]
	if !ok {
		p.Groups = nil
		return nil
	}
	var groupsList []any
	switch x := rawGroups.(type) {
	case []any:
		groupsList = x
	case []map[string]any:
		groupsList = make([]any, 0, len(x))
		for _, g := range x {
			groupsList = append(groupsList, g)
		}
	default:
		return &ParamError{Field: "groups", Reason: "must be a list"}
	}
	out := make([]map[string]any, 0, len(groupsList))
	for i, raw := range groupsList {
		g, ok := raw.(map[string]any)
		if !ok {
			return &ParamError{Field: fmt.Sprintf("groups[%d]", i), Reason: "must be a map"}
		}
		out = append(out, g)
	}
	p.Groups = out
	return nil
}

// Check performs shallow validation. Mirrors VariableAggregatorParam.check.
func (p *variableAggregatorParam) Check() error {
	if len(p.Groups) == 0 {
		return &ParamError{Field: "groups", Reason: "must not be empty"}
	}
	for i, g := range p.Groups {
		name, _ := g["group_name"].(string)
		if name == "" {
			return &ParamError{Field: fmt.Sprintf("groups[%d].group_name", i), Reason: "must not be empty"}
		}
		vars, ok := g["variables"]
		if !ok {
			return &ParamError{Field: fmt.Sprintf("groups[%d].variables", i), Reason: "must be a list"}
		}
		switch vars.(type) {
		case []any, []map[string]any:
			// accept both shapes
		default:
			return &ParamError{Field: fmt.Sprintf("groups[%d].variables", i), Reason: "must be a list"}
		}
	}
	return nil
}

// AsDict returns the params as a plain map.
func (p *variableAggregatorParam) AsDict() map[string]any {
	out := map[string]any{"groups": make([]any, 0, len(p.Groups))}
	for _, g := range p.Groups {
		out["groups"] = append(out["groups"].([]any), g)
	}
	return out
}

// VariableAggregatorComponent walks each group's selectors and emits
// outputs[group_name] = first non-empty resolved value.
type VariableAggregatorComponent struct {
	name  string
	param variableAggregatorParam
}

// NewVariableAggregatorComponent constructs a VariableAggregator from
// the DSL param map. The param is validated via Check(); a check failure
// is returned to the caller so the engine can surface a clean error.
func NewVariableAggregatorComponent(params map[string]any) (Component, error) {
	p := &variableAggregatorParam{}
	if err := p.Update(params); err != nil {
		return nil, fmt.Errorf("VariableAggregator: param update: %w", err)
	}
	if err := p.Check(); err != nil {
		return nil, fmt.Errorf("VariableAggregator: param check: %w", err)
	}
	return &VariableAggregatorComponent{
		name:  componentNameVariableAggregator,
		param: *p,
	}, nil
}

// Name returns the registered component name.
func (v *VariableAggregatorComponent) Name() string { return v.name }

// Invoke iterates the configured groups and resolves each selector's
// value against the canvas state. The first truthy value in a group
// wins; outputs[group_name] is set to that value. Groups with no truthy
// selector produce no output key.
//
// Variable references may be passed in two ways:
//   - static via param.groups[i].variables[j].value
//   - runtime via inputs["variables"] (a list of selector dicts that
//     REPLACES the static config for the duration of this call).
//
// The runtime override matches the Python component's get_input_form
// contract: the engine is allowed to pass the resolved variable list
// per-invocation. When inputs["variables"] is absent the static param
// config is used unchanged.
func (v *VariableAggregatorComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil {
		return nil, fmt.Errorf("VariableAggregator: %w", err)
	}
	if state == nil {
		return nil, fmt.Errorf("VariableAggregator: nil canvas state")
	}

	groups := v.param.Groups
	// Optional runtime override: the engine can pass a fresh
	// "variables" list (a list of group dicts) that replaces the
	// static param. We accept either shape — a bare list of selectors
	// replaces the FIRST group's variables, or a list of group dicts
	// replaces all groups entirely. The latter is the common case
	// because the engine passes one item per group.
	if override, ok := inputs["variables"].([]any); ok && len(override) > 0 {
		if first, ok := override[0].(map[string]any); ok {
			if _, hasGroups := first["groups"]; hasGroups {
				// shape: [{groups: [...]}] — flatten outer wrapper
				groups = make([]map[string]any, 0, len(override))
				for _, raw := range override {
					if m, ok := raw.(map[string]any); ok {
						if gs, ok := m["groups"].([]any); ok {
							for _, g := range gs {
								if gm, ok := g.(map[string]any); ok {
									groups = append(groups, gm)
								}
							}
						}
					}
				}
			} else {
				// treat override as a list of group dicts
				groups = make([]map[string]any, 0, len(override))
				for _, g := range override {
					if gm, ok := g.(map[string]any); ok {
						groups = append(groups, gm)
					}
				}
			}
		}
	}

	out := make(map[string]any, len(groups))
	for _, g := range groups {
		gname, _ := g["group_name"].(string)
		if gname == "" {
			continue
		}
		selectors, _ := g["variables"].([]any)
		for _, raw := range selectors {
			sel, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			ref, _ := sel["value"].(string)
			if ref == "" {
				continue
			}
			val, err := state.GetVar(ref)
			if err != nil || !isTruthy(val) {
				continue
			}
			out[gname] = val
			break
		}
	}
	return out, nil
}

// Stream mirrors Invoke; VariableAggregator is a single-shot reduce.
func (v *VariableAggregatorComponent) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out, err := v.Invoke(ctx, inputs)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]any, 1)
	ch <- out
	close(ch)
	return ch, nil
}

// Inputs returns the public parameter surface. The "variables" key
// accepts a runtime override of the per-group variable list (matching
// the Python get_input_form contract).
func (v *VariableAggregatorComponent) Inputs() map[string]string {
	return map[string]string{
		"variables": "Optional runtime override of the per-group variable selector list.",
	}
}

// Outputs returns one key per configured group: <group_name> = first
// non-empty resolved value for that group.
func (v *VariableAggregatorComponent) Outputs() map[string]string {
	out := make(map[string]string, len(v.param.Groups))
	for _, g := range v.param.Groups {
		if name, _ := g["group_name"].(string); name != "" {
			out[name] = "First non-empty resolved value among the group's selectors."
		}
	}
	return out
}

// isTruthy mirrors Python's bool() coercion: nil is false, empty
// strings/slices/maps are false, zero numbers are false, false is
// false, everything else is true.
func isTruthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	}
	return true
}

func init() {
	Register(componentNameVariableAggregator, NewVariableAggregatorComponent)
}
