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

// Package component — ListOperations (T3, plan §2.11.3 row 17).
//
// ListOperations applies one of six transforms to a list pulled from
// the canvas state. It is pure: it does not write back to state; the
// transformed list is returned at outputs["result"], with the head
// and tail exposed at outputs["first"] / outputs["last"] for the
// convenience of downstream nodes that only need a scalar.
//
// Supported operations:
//   - nth          : 1-indexed (positive n) or -N (from end) element pick
//   - head         : first n items
//   - tail         : last n items
//   - filter       : keep items whose _norm(v) matches a filter rule
//   - sort         : stable sort; reverse for desc
//   - drop_duplicates : keep first occurrence by hashable key
//
// Mirrors agent/component/list_operations.py.
package component

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"ragflow/internal/agent/runtime"
)

const componentNameListOperations = "ListOperations"

// listOperationsParam is the static configuration.
type listOperationsParam struct {
	Query      string         `json:"query"`
	Operations string         `json:"operations"`
	N          int            `json:"n"`
	Strict     bool           `json:"strict"`
	SortMethod string         `json:"sort_method"`
	Filter     map[string]any `json:"filter"`
}

// Update copies a fresh param map into the receiver.
func (p *listOperationsParam) Update(conf map[string]any) error {
	if conf == nil {
		conf = map[string]any{}
	}
	p.Query, _ = conf["query"].(string)
	p.Operations, _ = conf["operations"].(string)
	if p.Operations == "" {
		p.Operations = "nth"
	}
	p.N = toInt(conf["n"])
	if s, ok := conf["strict"].(bool); ok {
		p.Strict = s
	}
	p.SortMethod, _ = conf["sort_method"].(string)
	if p.SortMethod == "" {
		p.SortMethod = "asc"
	}
	if f, ok := conf["filter"].(map[string]any); ok {
		p.Filter = f
	} else {
		p.Filter = map[string]any{}
	}
	return nil
}

// Check validates the param.
func (p *listOperationsParam) Check() error {
	if p.Query == "" {
		return &ParamError{Field: "query", Reason: "must not be empty"}
	}
	switch p.Operations {
	case "nth", "head", "tail", "filter", "sort", "drop_duplicates":
		// ok
	default:
		return &ParamError{
			Field:  "operations",
			Reason: "must be one of: nth, head, tail, filter, sort, drop_duplicates",
		}
	}
	return nil
}

// AsDict returns the params as a plain map.
func (p *listOperationsParam) AsDict() map[string]any {
	return map[string]any{
		"query":       p.Query,
		"operations":  p.Operations,
		"n":           p.N,
		"strict":      p.Strict,
		"sort_method": p.SortMethod,
		"filter":      p.Filter,
	}
}

// toInt coerces a value to int. Floats are truncated; strings parsed
// via Atoi; everything else falls back to 0.
func toInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		var n int
		fmt.Sscanf(x, "%d", &n)
		return n
	}
	return 0
}

// ListOperationsComponent implements the 6 list transforms.
type ListOperationsComponent struct {
	name  string
	param listOperationsParam
}

// NewListOperationsComponent constructs a ListOperations from the
// DSL param map.
func NewListOperationsComponent(params map[string]any) (Component, error) {
	p := &listOperationsParam{}
	if err := p.Update(params); err != nil {
		return nil, fmt.Errorf("ListOperations: param update: %w", err)
	}
	if err := p.Check(); err != nil {
		return nil, fmt.Errorf("ListOperations: param check: %w", err)
	}
	return &ListOperationsComponent{
		name:  componentNameListOperations,
		param: *p,
	}, nil
}

// Name returns the registered component name.
func (l *ListOperationsComponent) Name() string { return l.name }

// Invoke resolves the param.query against the canvas state and applies
// the configured operation. The transformed list is returned at
// outputs["result"], with outputs["first"] / outputs["last"] set to
// the first / last element of the result (or nil for an empty result).
func (l *ListOperationsComponent) Invoke(ctx context.Context, _ map[string]any) (map[string]any, error) {
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil {
		return nil, fmt.Errorf("ListOperations: %w", err)
	}
	if state == nil {
		return nil, fmt.Errorf("ListOperations: nil canvas state")
	}

	raw, err := state.GetVar(l.param.Query)
	if err != nil {
		return nil, fmt.Errorf("ListOperations: query %q: %w", l.param.Query, err)
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("ListOperations: input is not a list (got %T)", raw)
	}

	var out []any
	switch l.param.Operations {
	case "nth":
		out = l.opNth(items)
	case "head":
		out = l.opHead(items)
	case "tail":
		out = l.opTail(items)
	case "filter":
		out = l.opFilter(items)
	case "sort":
		out = l.opSort(items)
	case "drop_duplicates":
		out = l.opDropDuplicates(items)
	}

	first, last := any(nil), any(nil)
	if len(out) > 0 {
		first = out[0]
		last = out[len(out)-1]
	}
	return map[string]any{
		"result": out,
		"first":  first,
		"last":   last,
	}, nil
}

// Stream mirrors Invoke; ListOperations is a single-shot transform.
func (l *ListOperationsComponent) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out, err := l.Invoke(ctx, inputs)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]any, 1)
	ch <- out
	close(ch)
	return ch, nil
}

// Inputs returns an empty surface — all config is in the param.
func (l *ListOperationsComponent) Inputs() map[string]string {
	return map[string]string{}
}

// Outputs returns the transformed list plus head/tail scalars.
func (l *ListOperationsComponent) Outputs() map[string]string {
	return map[string]string{
		"result": "Transformed list (per the configured operation).",
		"first":  "First element of the result (nil for empty result).",
		"last":   "Last element of the result (nil for empty result).",
	}
}

// opNth: 1-indexed for positive n, -N (from end) for negative n.
// n=0 → empty (or error in strict mode).
func (l *ListOperationsComponent) opNth(items []any) []any {
	n := l.param.N
	if n == 0 {
		if l.param.Strict {
			panic(fmt.Sprintf("ListOperations: nth requires n to be within the valid range in strict mode, got %d", n))
		}
		return []any{}
	}
	if n > 0 {
		if n <= len(items) {
			return []any{items[n-1]}
		}
		if l.param.Strict {
			panic(fmt.Sprintf("ListOperations: nth requires n to be within the valid range in strict mode, got %d", n))
		}
		return []any{}
	}
	absN := -n
	if absN <= len(items) {
		return []any{items[n]}
	}
	if l.param.Strict {
		panic(fmt.Sprintf("ListOperations: nth requires n to be within the valid range in strict mode, got %d", n))
	}
	return []any{}
}

// opHead: first n items. n < 1 → empty. Strict: 1 ≤ n ≤ len(items).
func (l *ListOperationsComponent) opHead(items []any) []any {
	n := l.param.N
	if l.param.Strict {
		if n < 1 || n > len(items) {
			panic(fmt.Sprintf("ListOperations: head requires n to be within the valid range in strict mode, got %d", n))
		}
		return append([]any{}, items[:n]...)
	}
	if n < 1 {
		return []any{}
	}
	if n > len(items) {
		n = len(items)
	}
	return append([]any{}, items[:n]...)
}

// opTail: last n items. n < 1 → empty. Strict: 1 ≤ n ≤ len(items).
func (l *ListOperationsComponent) opTail(items []any) []any {
	n := l.param.N
	if l.param.Strict {
		if n < 1 || n > len(items) {
			panic(fmt.Sprintf("ListOperations: tail requires n to be within the valid range in strict mode, got %d", n))
		}
		return append([]any{}, items[len(items)-n:]...)
	}
	if n < 1 {
		return []any{}
	}
	if n > len(items) {
		n = len(items)
	}
	return append([]any{}, items[len(items)-n:]...)
}

// opFilter: keep items whose _norm(v) matches the filter rule.
func (l *ListOperationsComponent) opFilter(items []any) []any {
	op, _ := l.param.Filter["operator"].(string)
	val, _ := l.param.Filter["value"].(string)
	out := make([]any, 0, len(items))
	for _, item := range items {
		if evalFilter(normValue(item), op, val) {
			out = append(out, item)
		}
	}
	return out
}

// opSort: stable sort; for dict items, use hashable key. Reverse on
// sort_method == "desc". The Python implementation uses sorted() which
// is stable; Go's sort.SliceStable preserves that.
func (l *ListOperationsComponent) opSort(items []any) []any {
	if len(items) == 0 {
		return []any{}
	}
	reverse := strings.EqualFold(l.param.SortMethod, "desc")
	cp := append([]any{}, items...)
	if _, isMap := cp[0].(map[string]any); isMap {
		sort.SliceStable(cp, func(i, j int) bool {
			ki, kj := hashableKey(cp[i]), hashableKey(cp[j])
			if reverse {
				return lessKey(kj, ki)
			}
			return lessKey(ki, kj)
		})
	} else {
		sort.SliceStable(cp, func(i, j int) bool {
			if reverse {
				return lessScalar(cp[j], cp[i])
			}
			return lessScalar(cp[i], cp[j])
		})
	}
	return cp
}

// opDropDuplicates: keep first occurrence by hashable key. The
// hashable key is JSON-encoded for use as a Go map key — Go maps do
// not accept []any directly, so we serialize the canonical form to a
// string. Two items that JSON-encode to the same string are equal for
// dedup purposes.
func (l *ListOperationsComponent) opDropDuplicates(items []any) []any {
	seen := make(map[string]struct{}, len(items))
	out := make([]any, 0, len(items))
	for _, item := range items {
		k := dedupKey(item)
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, item)
	}
	return out
}

// dedupKey JSON-encodes v to a canonical string. Maps are encoded via
// a stable intermediate (hashableKey → JSON) so two dicts with the
// same content but different Go map iteration orders hash to the same
// key.
func dedupKey(v any) string {
	b, err := json.Marshal(hashableKey(v))
	if err != nil {
		// Fall back to %v rendering if JSON fails (shouldn't happen
		// for our supported types).
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// normValue is the Python _norm helper: "" for nil, else str(v).
func normValue(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

// evalFilter evaluates a single filter rule.
func evalFilter(v, op, target string) bool {
	switch op {
	case "=":
		return v == target
	case "≠":
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

// hashableKey produces a comparable key for items used by sort and
// drop_duplicates. Dicts are flattened to a tuple of (key, hashable
// value) pairs (sorted by key for determinism). Lists and slices
// recurse element-wise.
func hashableKey(v any) any {
	switch x := v.(type) {
	case map[string]any:
		// Stable ordering matters: sort by key string.
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		pairs := make([]any, 0, 2*len(keys))
		for _, k := range keys {
			pairs = append(pairs, k, hashableKey(x[k]))
		}
		return pairs
	case []any:
		out := make([]any, 0, len(x))
		for _, item := range x {
			out = append(out, hashableKey(item))
		}
		return out
	}
	return v
}

// lessKey compares two hashableKey results (both are themselves []any
// of either string-key/value or scalar tuples). Returns true when a<b.
func lessKey(a, b any) bool {
	as, aok := a.([]any)
	bs, bok := b.([]any)
	if !aok || !bok {
		return lessScalar(a, b)
	}
	// Element-wise compare
	for i := 0; i < len(as) && i < len(bs); i++ {
		if lessScalar(as[i], bs[i]) {
			return true
		}
		if lessScalar(bs[i], as[i]) {
			return false
		}
	}
	return len(as) < len(bs)
}

// lessScalar compares two scalar (or non-tuple) values. Numbers are
// compared numerically; everything else via fmt.Sprintf.
func lessScalar(a, b any) bool {
	af, aok := toFloat64OK(a)
	bf, bok := toFloat64OK(b)
	if aok && bok {
		return af < bf
	}
	return fmt.Sprintf("%v", a) < fmt.Sprintf("%v", b)
}

// toFloat64OK is a number-without-bool helper.
func toFloat64OK(v any) (float64, bool) {
	if _, isBool := v.(bool); isBool {
		return 0, false
	}
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}

func init() {
	Register(componentNameListOperations, NewListOperationsComponent)
}
