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

// Package component — DataOperations (T3, plan §2.11.3 row 16).
//
// DataOperations applies one of seven dict/list transforms to a list
// of dicts pulled from the canvas state. It is pure: no state writes;
// the transformed payload is returned at outputs["result"].
//
// Operations:
//   - select_keys     : keep only the listed keys per dict
//   - literal_eval    : walk input_objects; try to parse JSON-like
//     string leaves (the Go port uses json.Unmarshal
//     as a stand-in for Python's ast.literal_eval —
//     tuples/sets are NOT supported, matching the
//     JSON-shaped LLM output the canvas typically
//     consumes).
//   - combine         : merge all input dicts into one
//   - filter_values   : keep dicts matching all rules
//   - append_or_update: apply updates [{key, value}] per dict
//   - remove_keys     : drop the listed keys per dict
//   - rename_keys     : rename per [{old_key, new_key}] per dict
//
// Mirrors agent/component/data_operations.py.
package component

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ragflow/internal/agent/runtime"
)

const componentNameDataOperations = "DataOperations"

// dataOperationsParam is the static configuration.
type dataOperationsParam struct {
	Query        []string         `json:"query"`
	Operations   string           `json:"operations"`
	SelectKeys   []string         `json:"select_keys"`
	FilterValues []map[string]any `json:"filter_values"`
	Updates      []map[string]any `json:"updates"`
	RemoveKeys   []string         `json:"remove_keys"`
	RenameKeys   []map[string]any `json:"rename_keys"`
}

// Update copies a fresh param map into the receiver.
func (p *dataOperationsParam) Update(conf map[string]any) error {
	if conf == nil {
		conf = map[string]any{}
	}
	p.Query = toStringSlice(conf["query"])
	p.Operations, _ = conf["operations"].(string)
	if p.Operations == "" {
		p.Operations = "literal_eval"
	}
	p.SelectKeys = toStringSlice(conf["select_keys"])
	p.FilterValues = toMapSlice(conf["filter_values"])
	p.Updates = toMapSlice(conf["updates"])
	p.RemoveKeys = toStringSlice(conf["remove_keys"])
	p.RenameKeys = toMapSlice(conf["rename_keys"])
	return nil
}

// Check validates the param.
func (p *dataOperationsParam) Check() error {
	switch p.Operations {
	case "select_keys", "literal_eval", "combine", "filter_values",
		"append_or_update", "remove_keys", "rename_keys":
		// ok
	default:
		return &ParamError{
			Field:  "operations",
			Reason: "must be one of: select_keys, literal_eval, combine, filter_values, append_or_update, remove_keys, rename_keys",
		}
	}
	return nil
}

// AsDict returns the params as a plain map.
func (p *dataOperationsParam) AsDict() map[string]any {
	return map[string]any{
		"query":         p.Query,
		"operations":    p.Operations,
		"select_keys":   p.SelectKeys,
		"filter_values": p.FilterValues,
		"updates":       p.Updates,
		"remove_keys":   p.RemoveKeys,
		"rename_keys":   p.RenameKeys,
	}
}

// toStringSlice normalizes a value to []string. Strings (CSV) and
// []any are accepted; nil returns nil.
func toStringSlice(v any) []string {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		// CSV fallback: "a,b,c" → ["a","b","c"]
		parts := strings.Split(x, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			s := strings.TrimSpace(p)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return append([]string{}, x...)
	}
	return nil
}

// toMapSlice normalizes a value to []map[string]any.
func toMapSlice(v any) []map[string]any {
	switch x := v.(type) {
	case nil:
		return nil
	case []any:
		out := make([]map[string]any, 0, len(x))
		for _, item := range x {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	case []map[string]any:
		return append([]map[string]any{}, x...)
	}
	return nil
}

// DataOperationsComponent implements the 7 dict transforms.
type DataOperationsComponent struct {
	name  string
	param dataOperationsParam
}

// NewDataOperationsComponent constructs a DataOperations from the
// DSL param map.
func NewDataOperationsComponent(params map[string]any) (Component, error) {
	p := &dataOperationsParam{}
	if err := p.Update(params); err != nil {
		return nil, fmt.Errorf("DataOperations: param update: %w", err)
	}
	if err := p.Check(); err != nil {
		return nil, fmt.Errorf("DataOperations: param check: %w", err)
	}
	return &DataOperationsComponent{
		name:  componentNameDataOperations,
		param: *p,
	}, nil
}

// Name returns the registered component name.
func (d *DataOperationsComponent) Name() string { return d.name }

// Invoke loads input_objects from the configured query refs, then
// dispatches to the operation-specific helper.
func (d *DataOperationsComponent) Invoke(ctx context.Context, _ map[string]any) (map[string]any, error) {
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil {
		return nil, fmt.Errorf("DataOperations: %w", err)
	}
	if state == nil {
		return nil, fmt.Errorf("DataOperations: nil canvas state")
	}

	// Coerce query to a list: param.query may arrive as a single
	// string in the JSON DSL, which the Python code wraps in [x].
	queries := d.param.Query
	if len(queries) == 0 {
		// fall back to single ref parsed from a string param — when
		// the engine loads the DSL it may pass a single ref; tolerate.
		queries = []string{}
	}

	var inputObjects []map[string]any
	for _, ref := range queries {
		if ref == "" {
			continue
		}
		v, err := state.GetVar(ref)
		if err != nil {
			return nil, fmt.Errorf("DataOperations: query %q: %w", ref, err)
		}
		if v == nil {
			continue
		}
		switch x := v.(type) {
		case map[string]any:
			inputObjects = append(inputObjects, x)
		case []any:
			for _, item := range x {
				if m, ok := item.(map[string]any); ok {
					inputObjects = append(inputObjects, m)
				}
			}
		}
	}

	var result any
	switch d.param.Operations {
	case "select_keys":
		result = d.opSelectKeys(inputObjects)
	case "literal_eval":
		result = d.opLiteralEval(inputObjects)
	case "combine":
		result = d.opCombine(inputObjects)
	case "filter_values":
		result = d.opFilterValues(state, inputObjects)
	case "append_or_update":
		result = d.opAppendOrUpdate(state, inputObjects)
	case "remove_keys":
		result = d.opRemoveKeys(inputObjects)
	case "rename_keys":
		result = d.opRenameKeys(inputObjects)
	}
	return map[string]any{"result": result}, nil
}

// Stream mirrors Invoke; DataOperations is a single-shot transform.
func (d *DataOperationsComponent) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out, err := d.Invoke(ctx, inputs)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]any, 1)
	ch <- out
	close(ch)
	return ch, nil
}

// Inputs returns an empty surface — all config is in the param.
func (d *DataOperationsComponent) Inputs() map[string]string {
	return map[string]string{}
}

// Outputs returns the transformed payload.
func (d *DataOperationsComponent) Outputs() map[string]string {
	return map[string]string{
		"result": "Transformed payload: a list of dicts for most ops, or a single dict for combine.",
	}
}

// opSelectKeys keeps only the listed keys per dict. Result is []any
// of dicts.
func (d *DataOperationsComponent) opSelectKeys(items []map[string]any) []any {
	keep := make(map[string]struct{}, len(d.param.SelectKeys))
	for _, k := range d.param.SelectKeys {
		keep[k] = struct{}{}
	}
	out := make([]any, 0, len(items))
	for _, item := range items {
		cp := make(map[string]any, len(keep))
		for k := range item {
			if _, ok := keep[k]; ok {
				cp[k] = item[k]
			}
		}
		out = append(out, cp)
	}
	return out
}

// opLiteralEval walks the input list and tries to JSON-decode any
// string leaf that looks like a JSON literal. Returns a list of
// (possibly-mutated) dicts.
func (d *DataOperationsComponent) opLiteralEval(items []map[string]any) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, recursiveEval(item))
	}
	return out
}

// recursiveEval mirrors the Python _recursive_eval helper: any string
// that starts with a JSON delimiter or known literal is unmarshaled.
// On failure, the original string is returned.
func recursiveEval(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = recursiveEval(val)
		}
		return out
	case []any:
		out := make([]any, 0, len(x))
		for _, item := range x {
			out = append(out, recursiveEval(item))
		}
		return out
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return x
		}
		// Detect likely JSON literal: starts with one of { [ ( " '
		// digit, or is a known scalar literal (true/false/null).
		first := s[0]
		lower := strings.ToLower(s)
		isLiteral := false
		switch first {
		case '{', '[', '(', '"', '\'':
			isLiteral = true
		}
		if !isLiteral {
			// digit
			if first >= '0' && first <= '9' {
				isLiteral = true
			}
		}
		if !isLiteral && (lower == "true" || lower == "false" || lower == "null" || lower == "none") {
			isLiteral = true
		}
		if !isLiteral {
			return x
		}
		var parsed any
		// Try JSON. If it fails, return the original string.
		if err := json.Unmarshal([]byte(s), &parsed); err == nil {
			return parsed
		}
		return x
	}
	return v
}

// opCombine merges all input dicts into one. Key conflicts:
//   - existing is a list → extend (or append if new is scalar)
//   - existing is scalar, new is list → wrap as [old, *new]
//   - existing is scalar, new is scalar → wrap as [old, new]
func (d *DataOperationsComponent) opCombine(items []map[string]any) map[string]any {
	out := map[string]any{}
	for _, obj := range items {
		for k, v := range obj {
			existing, ok := out[k]
			if !ok {
				out[k] = v
				continue
			}
			switch ex := existing.(type) {
			case []any:
				if vl, ok := v.([]any); ok {
					out[k] = append(ex, vl...)
				} else {
					out[k] = append(ex, v)
				}
			default:
				if vl, ok := v.([]any); ok {
					out[k] = []any{ex, vl}
				} else {
					out[k] = []any{ex, v}
				}
			}
		}
	}
	return out
}

// opFilterValues keeps dicts where every rule matches.
func (d *DataOperationsComponent) opFilterValues(state *runtime.CanvasState, items []map[string]any) []any {
	rules := d.param.FilterValues
	out := make([]any, 0, len(items))
	for _, obj := range items {
		if len(rules) == 0 {
			out = append(out, obj)
			continue
		}
		all := true
		for _, rule := range rules {
			if !matchRule(state, obj, rule) {
				all = false
				break
			}
		}
		if all {
			out = append(out, obj)
		}
	}
	return out
}

// matchRule evaluates one filter rule against obj. Mirrors the
// Python match_rule helper.
func matchRule(state *runtime.CanvasState, obj map[string]any, rule map[string]any) bool {
	key, _ := rule["key"].(string)
	if _, ok := obj[key]; !ok {
		return false
	}
	op := strings.ToLower(asString(rule["operator"]))
	if op == "" {
		op = "equals"
	}
	target := normValue(rule["value"])
	// Try to resolve {{...}} in target via state.
	if s, ok := rule["value"].(string); ok && strings.Contains(s, "{{") {
		if resolved, err := runtime.ResolveTemplate(s, state); err == nil {
			target = resolved
		}
	}
	v := normValue(obj[key])
	switch op {
	case "=", "equals":
		return v == target
	case "≠", "!=":
		return v != target
	case "contains":
		return strings.Contains(v, target)
	case "start with":
		return strings.HasPrefix(v, target)
	case "end with":
		return strings.HasSuffix(v, target)
	}
	return false
}

// asString is a forgiving cast for params that may arrive as int/str.
func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// opAppendOrUpdate copies each dict and applies updates. Values that
// look like {{ref}} are resolved via state; otherwise used as-is.
func (d *DataOperationsComponent) opAppendOrUpdate(state *runtime.CanvasState, items []map[string]any) []any {
	out := make([]any, 0, len(items))
	for _, obj := range items {
		cp := make(map[string]any, len(obj))
		for k, v := range obj {
			cp[k] = v
		}
		for _, upd := range d.param.Updates {
			k := strings.TrimSpace(asString(upd["key"]))
			if k == "" {
				continue
			}
			raw := upd["value"]
			// Resolve {{...}} templates first; fall back to plain
			// state-ref resolution (matches the Python
			// get_value_with_variable behavior — strings are looked
			// up in state when they look like refs).
			if s, ok := raw.(string); ok {
				if strings.Contains(s, "{{") {
					if resolved, err := runtime.ResolveTemplate(s, state); err == nil && resolved != "" {
						cp[k] = resolved
						continue
					}
				}
				if v, err := state.GetVar(s); err == nil && v != nil {
					cp[k] = v
					continue
				}
			}
			cp[k] = raw
		}
		out = append(out, cp)
	}
	return out
}

// opRemoveKeys copies each dict and drops the listed keys.
func (d *DataOperationsComponent) opRemoveKeys(items []map[string]any) []any {
	out := make([]any, 0, len(items))
	for _, obj := range items {
		cp := make(map[string]any, len(obj))
		for k, v := range obj {
			cp[k] = v
		}
		for _, k := range d.param.RemoveKeys {
			if _, ok := cp[k]; ok {
				delete(cp, k)
			}
		}
		out = append(out, cp)
	}
	return out
}

// opRenameKeys copies each dict and renames per the configured pairs.
func (d *DataOperationsComponent) opRenameKeys(items []map[string]any) []any {
	out := make([]any, 0, len(items))
	for _, obj := range items {
		cp := make(map[string]any, len(obj))
		for k, v := range obj {
			cp[k] = v
		}
		for _, pair := range d.param.RenameKeys {
			old := strings.TrimSpace(asString(pair["old_key"]))
			new := strings.TrimSpace(asString(pair["new_key"]))
			if old == "" || new == "" || old == new {
				continue
			}
			if v, ok := cp[old]; ok {
				cp[new] = v
				delete(cp, old)
			}
		}
		out = append(out, cp)
	}
	return out
}

func init() {
	Register(componentNameDataOperations, NewDataOperationsComponent)
}
