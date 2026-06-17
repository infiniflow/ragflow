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

// listOpPanic is the sentinel value that opNth/opHead/opTail panic with
// in strict-mode range errors. Invoke()'s defer/recover converts these
// into a typed error; any other panic is re-raised unchanged so a real
// bug in operator code is not masked as a "ListOperations: ..." error.
//
// Mirrors agent/component/list_operations.py:_raise_strict_range_error
// (raises ValueError, caught by the canvas framework).
type listOpPanic struct{ msg string }

func (p *listOpPanic) Error() string { return p.msg }

// strictRangePanic builds the panic value raised by opNth/opHead/opTail
// in strict mode. The result is recoverable by Invoke()'s defer/recover;
// callers use it as `panic(strictRangePanic("head", n))`.
func strictRangePanic(op string, n int) *listOpPanic {
	return &listOpPanic{
		msg: fmt.Sprintf("ListOperations: %s requires n to be within the valid range in strict mode, got %d", op, n),
	}
}

// coerceBool mirrors Python's _is_strict accept-list. Bools pass through;
// the strings "1", "true", "yes", "on" (any case) are accepted; everything
// else is false. Mirrors agent/component/list_operations.py:79-82.
func coerceBool(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	if s, ok := v.(string); ok {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "1", "true", "yes", "on":
			return true
		}
	}
	return false
}

// listOperationsParam is the static configuration.
type listOperationsParam struct {
	Query      string         `json:"query"`
	Operations string         `json:"operations"`
	N          int            `json:"n"`
	Strict     bool           `json:"strict"`
	SortMethod string         `json:"sort_method"`
	SortBy     []string       `json:"sort_by"`
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
	p.Operations = normalizeListOperationName(p.Operations)
	p.N = toInt(conf["n"])
	if v, ok := conf["strict"]; ok {
		p.Strict = coerceBool(v)
	}
	p.SortMethod, _ = conf["sort_method"].(string)
	if p.SortMethod == "" {
		p.SortMethod = "asc"
	}
	p.SortBy = parseSortByFieldList(conf["sort_by"])
	if f, ok := conf["filter"].(map[string]any); ok {
		p.Filter = f
	} else {
		p.Filter = map[string]any{}
	}
	return nil
}

func normalizeListOperationName(op string) string {
	switch strings.ToLower(strings.TrimSpace(op)) {
	case "topn":
		return "head"
	default:
		return op
	}
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
		"sort_by":     p.SortBy,
		"filter":      p.Filter,
	}
}

// parseSortByFieldList normalises the DSL `sort_by` field. The DSL
// takes a comma-separated string ("score" or "score,title") and the
// Python param layer accepts the same shape (see
// agent/component/list_operations.py:_sort). A nil/empty/whitespace
// input collapses to nil so opSort can fall back to the legacy
// hashableKey behaviour (sort by the lexicographically first field).
func parseSortByFieldList(v any) []string {
	if v == nil {
		return nil
	}
	var raw string
	switch x := v.(type) {
	case string:
		raw = x
	case []any:
		// Tolerate the JSON-array form some editors emit: ["score"].
		parts := make([]string, 0, len(x))
		for _, item := range x {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				parts = append(parts, strings.TrimSpace(s))
			}
		}
		return parts
	case []string:
		parts := make([]string, 0, len(x))
		for _, s := range x {
			if s = strings.TrimSpace(s); s != "" {
				parts = append(parts, s)
			}
		}
		return parts
	default:
		return nil
	}
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// toInt coerces a value to int. Floats are truncated; strings parsed
// via Atoi; bools follow Python's int() semantics (true → 1, false → 0);
// everything else falls back to 0. Mirrors Python's _coerce_n in
// agent/component/list_operations.py:73-77.
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
	case bool:
		if x {
			return 1
		}
		return 0
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
//
// A defer/recover at the top of this function converts any
// *listOpPanic raised by opNth/opHead/opTail (strict-mode range
// errors) into a returned error. Any other panic is re-raised so a
// real bug in the operator code is not masked as a "ListOperations:
// ..." error.
func (l *ListOperationsComponent) Invoke(ctx context.Context, _ map[string]any) (result map[string]any, err error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if lp, ok := r.(*listOpPanic); ok {
			err = lp
			return
		}
		panic(r)
	}()

	state, _, serr := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if serr != nil {
		return nil, fmt.Errorf("ListOperations: %w", serr)
	}
	if state == nil {
		return nil, fmt.Errorf("ListOperations: nil canvas state")
	}

	raw, serr := state.GetVar(l.param.Query)
	if serr != nil {
		return nil, fmt.Errorf("ListOperations: query %q: %w", l.param.Query, serr)
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
	default:
		return nil, fmt.Errorf("ListOperations: unsupported operation %q", l.param.Operations)
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

// Inputs returns the public parameter surface (declared so the editor
// can render per-input hints). All values map to the static DSL param
// unless overridden via the orchestrator's input map.
func (l *ListOperationsComponent) Inputs() map[string]string {
	return map[string]string{
		"query":       "List reference or data to operate on. Overrides the static DSL param.",
		"operations":  "Operation: nth, head, tail, filter, sort, drop_duplicates. Overrides DSL param.",
		"n":           "N value for nth/head/tail operations. Overrides DSL param.",
		"strict":      "When true, index-out-of-range is fatal. Accepts bool or '1'/'true'/'yes'/'on' (case-insensitive). Default false.",
		"sort_method": "Sort direction: 'asc' or 'desc'. Overrides DSL param.",
		"sort_by":     "Comma-separated list of map keys to sort by (primary, tiebreak, ...). Empty/missing falls back to lexicographically first key. Overrides DSL param.",
		"filter":      "Filter spec: {operator, value}. Operator is one of: =, ≠, contains, start with, end with. Overrides DSL param.",
	}
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
			panic(strictRangePanic("nth", n))
		}
		return []any{}
	}
	if n > 0 {
		if n <= len(items) {
			return []any{items[n-1]}
		}
		if l.param.Strict {
			panic(strictRangePanic("nth", n))
		}
		return []any{}
	}
	absN := -n
	if absN <= len(items) {
		return []any{items[n]}
	}
	if l.param.Strict {
		panic(strictRangePanic("nth", n))
	}
	return []any{}
}

// opHead: first n items. n < 1 → empty. Strict: 1 ≤ n ≤ len(items).
func (l *ListOperationsComponent) opHead(items []any) []any {
	n := l.param.N
	if l.param.Strict {
		if n < 1 || n > len(items) {
			panic(strictRangePanic("head", n))
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
			panic(strictRangePanic("tail", n))
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
//
// When SortBy is set, the sort key is the tuple (item[k1], item[k2],
// ...) for each map element (primary = first listed field, tiebreak =
// subsequent fields). When SortBy is empty, the comparator falls back
// to the full hashableKey — equivalent to the lexicographically first
// field, matching the pre-sort_by behaviour.
func (l *ListOperationsComponent) opSort(items []any) []any {
	if len(items) == 0 {
		return []any{}
	}
	reverse := strings.EqualFold(l.param.SortMethod, "desc")
	cp := append([]any{}, items...)
	if _, isMap := cp[0].(map[string]any); isMap {
		var keyFn func(any) any
		if len(l.param.SortBy) > 0 {
			fields := l.param.SortBy
			keyFn = func(x any) any {
				m, _ := x.(map[string]any)
				out := make([]any, 0, len(fields))
				for _, k := range fields {
					out = append(out, m[k])
				}
				return out
			}
		} else {
			keyFn = hashableKey
		}
		sort.SliceStable(cp, func(i, j int) bool {
			ki, kj := keyFn(cp[i]), keyFn(cp[j])
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
// Bools are rendered as "True" / "False" (Python's str() output) so
// filter `=` comparisons match the Python DSL contract. Mirrors
// agent/component/list_operations.py:151-153.
func normValue(v any) string {
	if v == nil {
		return ""
	}
	if b, ok := v.(bool); ok {
		if b {
			return "True"
		}
		return "False"
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
