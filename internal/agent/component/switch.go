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

// Package component — Switch component (T2).
//
// Switch is a multi-condition router implemented in pure Go (no eino
// Lambda dependency). It walks a list of AND/OR-combined condition
// groups against the current *CanvasState, picks the first matching
// group's downstream cpn_id, and returns it as outputs["_next"]. The
// downstream cpn_id for a matching group is taken from the optional
// "to" field; if absent, the fallback is the index-based
// "matched_<i>" naming used in the spec's example assertions.
//
// Switch's runtime choice is consumed by the canvas scheduler's
// MultiBranch wiring (see canvas/multibranch.go). The eino
// `compose.NewGraphMultiBranch` integration is a thin pass-through:
// the routing decision lives here, MultiBranch just wires the edges.
//
// Mirrors the Python agent/component/switch.py behavior.
package component

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"ragflow/internal/agent/runtime"
)

const componentNameSwitch = "Switch"

// SwitchComponent implements the Switch routing node. It is stateless
// across invocations: the inputs map carries everything it needs.
type SwitchComponent struct {
	name   string
	params map[string]any
}

// NewSwitchComponent constructs a Switch component from the DSL params.
// Invoke merges these static params with any dynamic input overrides.
func NewSwitchComponent(params map[string]any) (Component, error) {
	cp := make(map[string]any, len(params))
	for k, v := range params {
		cp[k] = v
	}
	return &SwitchComponent{name: componentNameSwitch, params: cp}, nil
}

// Name returns the registered component name.
func (s *SwitchComponent) Name() string { return s.name }

// Invoke evaluates the conditions list in order, returns the first
// matching group's downstream cpn_ids at outputs["_next"]. If no
// group matches, outputs["_next"] = inputs["default"] (a
// free-form string — resolved to a real cpn_id by the canvas
// scheduler's MultiBranch wiring in canvas/multibranch.go).
// Unknown / empty inputs are tolerated: an absent "conditions"
// list yields outputs["_next"] = inputs["default"].
//
// The "to" field in each condition group can be a single string
// or a list of strings — Python's Switch routes to ALL targets
// in the "to" list simultaneously. The Go port mirrors this by
// returning "_next" as a []any (list of cpn_ids). The canvas
// scheduler's MultiBranch condition consumes this list via
// NewGraphMultiBranch so every declared target fires.
func (s *SwitchComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil {
		return nil, fmt.Errorf("Switch: %w", err)
	}
	if state == nil {
		return nil, fmt.Errorf("Switch: nil canvas state")
	}

	merged := make(map[string]any, len(s.params)+len(inputs))
	for k, v := range s.params {
		merged[k] = v
	}
	for k, v := range inputs {
		merged[k] = v
	}

	defaultNext, _ := merged["default"].(string)
	if defaultNext == "" {
		defaultNext = legacySwitchDefaultTarget(merged)
	}
	if raw, ok := merged["conditions"].([]any); ok {
		for i, item := range raw {
			group, ok := item.(map[string]any)
			if !ok {
				continue
			}
			matched, evalErr := evaluateGroup(group, state)
			if evalErr != nil {
				return nil, fmt.Errorf("Switch: condition[%d]: %w", i, evalErr)
			}
			if !matched {
				continue
			}
			targets := switchGroupTargets(group)
			if len(targets) == 0 {
				targets = []string{"matched_" + strconv.Itoa(i)}
			}
			// Return all targets as a list. Python's Switch routes
			// to every cpn_id in the "to" list; the canvas
			// scheduler's MultiBranch condition uses
			// NewGraphMultiBranch to fire all of them.
			nextAny := make([]any, len(targets))
			for j, t := range targets {
				nextAny[j] = t
			}
			return map[string]any{"_next": nextAny}, nil
		}
	}
	// Default: single target. Wrap as a one-element list so the
	// MultiBranch condition can still consume it uniformly.
	return map[string]any{"_next": []any{defaultNext}}, nil
}

// Stream is a synchronous facade over Invoke for P0. Switch is a
// routing decision, not a stream of partial results; the channel
// receives one payload and closes.
func (s *SwitchComponent) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out, err := s.Invoke(ctx, inputs)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]any, 1)
	ch <- out
	close(ch)
	return ch, nil
}

// Inputs returns the public parameter surface.
func (s *SwitchComponent) Inputs() map[string]string {
	return map[string]string{
		"conditions": "Ordered list of condition groups; each is {op: \"and\"|\"or\", to?: cpn_id, clauses: [{left, op, right?}]}.",
		"default":    "Downstream cpn_id used when no condition matches.",
	}
}

// Outputs returns the chosen cpn_id.
func (s *SwitchComponent) Outputs() map[string]string {
	return map[string]string{
		"_next": "The cpn_id of the downstream node to route to.",
	}
}

// evaluateGroup applies the group's op (and/or) to its clauses and
// returns true if the group matches. It is the lock-free inner of
// Switch.Invoke; caller must not hold state.mu.
func evaluateGroup(group map[string]any, state *runtime.CanvasState) (bool, error) {
	op, clauses := normalizeLegacyGroup(group)
	if op == "" {
		op = "and"
	}
	if len(clauses) == 0 {
		// An empty group must NOT match — otherwise a Switch with
		// no clauses (or all skipped `cpn_id`s) routes to the empty
		// group's `to` target before reaching the else / end_cpn_ids
		// branch. Mirrors PR #15644: the Python `if all(res):` form
		// was buggy for the same reason (all([]) is True) and was
		// tightened to `if res and all(res):`. The Go fix is the
		// empty-clauses short-circuit: an un-matchable group falls
		// through to the next condition or the end targets.
		return false, nil
	}
	for i, raw := range clauses {
		c, ok := raw.(map[string]any)
		if !ok {
			return false, fmt.Errorf("clause[%d] not a map", i)
		}
		matched, err := evaluateClause(c, state)
		if err != nil {
			return false, fmt.Errorf("clause[%d]: %w", i, err)
		}
		if op == "or" && matched {
			return true, nil
		}
		if op == "and" && !matched {
			return false, nil
		}
	}
	// For "and" with no early false: matched. For "or" with no early true: not matched.
	return op == "and", nil
}

func normalizeLegacyGroup(group map[string]any) (string, []any) {
	if group == nil {
		return "", nil
	}
	if clauses, ok := group["clauses"].([]any); ok {
		op, _ := group["op"].(string)
		return op, clauses
	}

	op, _ := group["logical_operator"].(string)
	rawItems, _ := group["items"].([]any)
	if len(rawItems) == 0 {
		return op, nil
	}
	clauses := make([]any, 0, len(rawItems))
	for _, raw := range rawItems {
		item, ok := raw.(map[string]any)
		if !ok || item == nil {
			continue
		}
		left := legacySwitchLeft(item)
		operator, _ := item["operator"].(string)
		clause := map[string]any{
			"left": left,
			"op":   normalizeLegacySwitchOperator(operator),
		}
		if v, ok := item["value"]; ok {
			clause["right"] = v
		}
		clauses = append(clauses, clause)
	}
	return op, clauses
}

func legacySwitchLeft(item map[string]any) string {
	left, _ := item["left"].(string)
	if left != "" {
		return left
	}
	cpnRef, _ := item["cpn_id"].(string)
	if cpnRef == "" {
		return ""
	}
	if runtime.VarRefPattern.MatchString(cpnRef) {
		return "{{" + cpnRef + "}}"
	}
	if strings.Contains(cpnRef, "@") || strings.HasPrefix(cpnRef, "sys.") || strings.HasPrefix(cpnRef, "env.") {
		return "{{" + cpnRef + "}}"
	}
	return cpnRef
}

func normalizeLegacySwitchOperator(op string) string {
	switch op {
	case "", "=":
		return "=="
	case "<>":
		return "!="
	default:
		return op
	}
}

// switchGroupTargets returns all cpn_ids from the group's "to" field.
// The "to" field can be a single string or a list of strings —
// Python's Switch routes to ALL targets simultaneously, and the
// Go port mirrors this via NewGraphMultiBranch in the canvas
// scheduler. Returns nil when "to" is absent or empty.
func switchGroupTargets(group map[string]any) []string {
	if group == nil {
		return nil
	}
	// Single string: "to": "DataOperations:UpdateSample"
	if next, ok := group["to"].(string); ok && next != "" {
		return []string{next}
	}
	// List: "to": ["DataOperations:UpdateSample", "ListOperations:Top2"]
	if raw, ok := group["to"].([]any); ok {
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			if next, ok := item.(string); ok && next != "" {
				out = append(out, next)
			}
		}
		return out
	}
	if raw, ok := group["to"].([]string); ok {
		out := make([]string, 0, len(raw))
		for _, next := range raw {
			if next != "" {
				out = append(out, next)
			}
		}
		return out
	}
	return nil
}

// switchGroupTarget returns the first cpn_id from the group's "to"
// field. Kept for backward compat with single-target callers; the
// main Invoke path now uses switchGroupTargets.
func switchGroupTarget(group map[string]any) (string, bool) {
	targets := switchGroupTargets(group)
	if len(targets) > 0 {
		return targets[0], true
	}
	return "", false
}

func legacySwitchDefaultTarget(merged map[string]any) string {
	if merged == nil {
		return ""
	}
	if raw, ok := merged["end_cpn_ids"].([]any); ok {
		for _, item := range raw {
			if next, ok := item.(string); ok && next != "" {
				return next
			}
		}
	}
	if raw, ok := merged["end_cpn_ids"].([]string); ok {
		for _, next := range raw {
			if next != "" {
				return next
			}
		}
	}
	return ""
}

// evaluateClause resolves a single clause. left is a {{...}} reference
// (passed through runtime.ResolveTemplate); op is one of
// "==", "!=", ">", "<", ">=", "<=", "contains", "not contains",
// "start with", "end with", "empty", "not empty". The "empty" /
// "not empty" operators ignore right.
//
// String operators (==, !=, contains, not contains, start with,
// end with) are case-insensitive — they lowercase both sides
// before comparing. The Python agent's Categorize / Switch
// nodes do the same; v1 ported case-insensitive matching for
// user-facing labels where "Hello" and "hello" should be
// treated as the same value.
func evaluateClause(clause map[string]any, state *runtime.CanvasState) (bool, error) {
	left, _ := clause["left"].(string)
	op, _ := clause["op"].(string)
	if op == "" {
		op = "=="
	}

	// "empty" / "not empty" don't read `right`.
	if op == "empty" {
		return isEmptyValue(leftValue(left, state)), nil
	}
	if op == "not empty" {
		return !isEmptyValue(leftValue(left, state)), nil
	}

	right := clause["right"]
	lv := leftValue(left, state)

	// Port of python PR #16320: for the four string operators,
	// coerce nil on either side to "". In Python this avoids
	// AttributeError on `.lower()`; in Go there's no crash (fmt
	// renders nil as "<nil>"), but the Python post-fix semantic —
	// where "foo" contains None is True — diverges from Go's
	// "<nil>" rendering. Coercing to "" aligns the Go port with
	// the Python workflow.
	if op == "contains" || op == "not contains" || op == "start with" || op == "end with" {
		if lv == nil {
			lv = ""
		}
		if right == nil {
			right = ""
		}
	}

	switch op {
	case "==":
		return equalFoldValues(lv, right), nil
	case "!=":
		return !equalFoldValues(lv, right), nil
	case "contains":
		ls, rs := fmt.Sprintf("%v", lv), fmt.Sprintf("%v", right)
		return containsString(strings.ToLower(ls), strings.ToLower(rs)), nil
	case "not contains":
		ls, rs := fmt.Sprintf("%v", lv), fmt.Sprintf("%v", right)
		return !containsString(strings.ToLower(ls), strings.ToLower(rs)), nil
	case "start with":
		ls, rs := fmt.Sprintf("%v", lv), fmt.Sprintf("%v", right)
		return strings.HasPrefix(strings.ToLower(ls), strings.ToLower(rs)), nil
	case "end with":
		ls, rs := fmt.Sprintf("%v", lv), fmt.Sprintf("%v", right)
		return strings.HasSuffix(strings.ToLower(ls), strings.ToLower(rs)), nil
	case ">", "<", ">=", "<=":
		ln, lok := numericize(lv)
		rn, rok := numericize(right)
		if !lok || !rok {
			return false, fmt.Errorf("operator %q requires numeric operands (left=%T, right=%T)", op, lv, right)
		}
		switch op {
		case ">":
			return ln > rn, nil
		case "<":
			return ln < rn, nil
		case ">=":
			return ln >= rn, nil
		case "<=":
			return ln <= rn, nil
		}
	}
	return false, fmt.Errorf("unknown operator %q", op)
}

// leftValue resolves a {{...}} reference against state. References
// without braces are returned as a literal (matches ResolveTemplate's
// pre-check behavior).
func leftValue(left string, state *runtime.CanvasState) any {
	if left == "" {
		return ""
	}
	// If the caller supplied an un-resolved literal (no {{...}}), pass
	// it through unchanged so operators like "==" can compare against
	// the raw value (e.g. left="raw string" → returned as "raw string").
	if !runtime.VarRefPattern.MatchString(left) {
		return left
	}
	resolved, err := runtime.ResolveTemplate(left, state)
	if err != nil {
		// On resolution failure, return the raw string so == can still
		// operate; we don't want a misconfigured ref to crash the run.
		return left
	}
	return resolved
}

// equalValues compares two any values with a forgiving type coercion
// (string ↔ fmt-rendered, int ↔ float64). Returns false on type
// mismatches that don't coerce cleanly.
func equalValues(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	// Stringify then compare — covers most canvas-DSL comparisons.
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

// equalFoldValues is the case-insensitive variant of equalValues.
// Used by the "==" / "!=" operators in evaluateClause so user-facing
// labels like "Hello" and "hello" compare equal — matches Python's
// Categorize / Switch node semantics.
func equalFoldValues(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	as := fmt.Sprintf("%v", a)
	bs := fmt.Sprintf("%v", b)
	// Numeric strings should still numeric-compare (so "1" == 1 holds
	// at the DSL level), but otherwise fall back to case-folded string
	// compare.
	if an, ok := numericize(as); ok {
		if bn, ok2 := numericize(bs); ok2 {
			return an == bn
		}
	}
	return strings.EqualFold(as, bs)
}

func containsString(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	return indexOf(haystack, needle) >= 0
}

// indexOf is a tiny wrapper around strings.Index to keep the operator
// table readable. strings import is hidden behind this helper to
// minimize the import list surface.
func indexOf(s, sub string) int {
	// use stdlib to avoid hand-rolling
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// numericize attempts to convert v to float64. Returns ok=false if v
// is a string that doesn't parse as a number (e.g. an LLM response);
// numeric operators will then error out with a clear message.
func numericize(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case string:
		f, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

// isEmptyValue reports whether a value is "empty" by the canvas DSL
// definition: nil, empty string, empty slice, empty map.
func isEmptyValue(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	}
	return false
}

// mapsCopyDup is a no-op duplicate alias kept for symmetry with the
// begin.go / message.go helpers in the package; here Switch doesn't
// need to copy maps but the alias documents the convention.
var _ = mapsCopyDup

func mapsCopyDup(dst, src map[string]any) { maps.Copy(dst, src) }

func init() {
	Register(componentNameSwitch, NewSwitchComponent)
}
