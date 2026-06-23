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

// Package component — StringTransform (T3, plan §2.11.3 row 18).
//
// StringTransform has two modes:
//
//	split — break a string on one or more literal delimiters
//	merge — substitute {{name}} placeholders in a script with values
//	        pulled from the inputs map or the canvas state
//
// Mirrors agent/component/string_transform.py. The P1 port supports the
// common {{name}} placeholder shape only; the full Jinja2 surface
// (`{% if %}`, `{% for %}`) is deferred to a later phase per the plan.
package component

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"ragflow/internal/agent/runtime"
)

const componentNameStringTransform = "StringTransform"

// stringTransformParam is the static configuration.
type stringTransformParam struct {
	Method     string   `json:"method"`     // "split" or "merge"
	Script     string   `json:"script"`     // merge mode: template
	SplitRef   string   `json:"split_ref"`  // split mode: state ref to read
	Delimiters []string `json:"delimiters"` // split mode: literal delimiters
}

// Update copies a fresh param map into the receiver.
func (p *stringTransformParam) Update(conf map[string]any) error {
	if conf == nil {
		conf = map[string]any{}
	}
	p.Method, _ = conf["method"].(string)
	if p.Method == "" {
		p.Method = "split"
	}
	p.Script, _ = conf["script"].(string)
	p.SplitRef, _ = conf["split_ref"].(string)

	switch v := conf["delimiters"].(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		p.Delimiters = out
	case []string:
		// already correct shape
		p.Delimiters = append(p.Delimiters[:0], v...)
	case nil:
		// leave unchanged
	default:
		// unknown shape — treat as empty; Check() will reject
		p.Delimiters = nil
	}
	return nil
}

// Check validates the param.
func (p *stringTransformParam) Check() error {
	switch p.Method {
	case "split", "merge":
		// ok
	default:
		return &ParamError{Field: "method", Reason: "must be one of: split, merge"}
	}
	if len(p.Delimiters) == 0 {
		return &ParamError{Field: "delimiters", Reason: "must not be empty"}
	}
	return nil
}

// AsDict returns the params as a plain map.
func (p *stringTransformParam) AsDict() map[string]any {
	return map[string]any{
		"method":     p.Method,
		"script":     p.Script,
		"split_ref":  p.SplitRef,
		"delimiters": p.Delimiters,
	}
}

// placeholderPattern matches {{name}} where name is an identifier-like
// sequence. Intentionally narrower than the canvas var-ref pattern
// (which also handles sys.x / env.x) because merge placeholders are
// looked up in the inputs map and/or canvas state by simple key, not
// the full cpn_id@param / sys.x / env.x grammar.
var placeholderPattern = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

// StringTransformComponent implements the split/merge component.
type StringTransformComponent struct {
	name  string
	param stringTransformParam
}

// NewStringTransformComponent constructs a StringTransform from the
// DSL param map.
func NewStringTransformComponent(params map[string]any) (Component, error) {
	p := &stringTransformParam{}
	if err := p.Update(params); err != nil {
		return nil, fmt.Errorf("StringTransform: param update: %w", err)
	}
	if err := p.Check(); err != nil {
		return nil, fmt.Errorf("StringTransform: param check: %w", err)
	}
	return &StringTransformComponent{
		name:  componentNameStringTransform,
		param: *p,
	}, nil
}

// Name returns the registered component name.
func (s *StringTransformComponent) Name() string { return s.name }

// Invoke runs the configured method (split or merge) and returns
// outputs["result"] with the transformed payload.
func (s *StringTransformComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil {
		return nil, fmt.Errorf("StringTransform: %w", err)
	}
	if state == nil {
		return nil, fmt.Errorf("StringTransform: nil canvas state")
	}

	if s.param.Method == "split" {
		return s.doSplit(ctx, state, inputs)
	}
	return s.doMerge(ctx, state, inputs), nil
}

// Stream mirrors Invoke; StringTransform is a single-shot transform.
func (s *StringTransformComponent) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out, err := s.Invoke(ctx, inputs)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]any, 1)
	ch <- out
	close(ch)
	return ch, nil
}

// Inputs returns the parameter surface. The shape depends on the
// configured method.
func (s *StringTransformComponent) Inputs() map[string]string {
	if s.param.Method == "split" {
		return map[string]string{
			"line": "Optional direct string to split; if absent, the component reads state[split_ref].",
		}
	}
	// merge: placeholders derived from the script
	names := extractPlaceholders(s.param.Script)
	out := make(map[string]string, len(names))
	for _, n := range names {
		out[n] = "Value to substitute for {{" + n + "}} (drawn from inputs or state)."
	}
	return out
}

// Outputs returns the transformed payload.
func (s *StringTransformComponent) Outputs() map[string]string {
	return map[string]string{
		"result": "Split: a []string of kept tokens. Merge: a single string with placeholders resolved.",
	}
}

// doSplit runs the split method. Mirrors the Python _split helper
// (string_transform.py:76-91): build a regex of the literal
// delimiters, split with capture groups, keep the even-indexed
// (non-delimiter) tokens.
func (s *StringTransformComponent) doSplit(_ context.Context, state *runtime.CanvasState, inputs map[string]any) (map[string]any, error) {
	var varValue string
	if line, ok := inputs["line"].(string); ok && line != "" {
		varValue = line
	} else if s.param.SplitRef != "" {
		v, err := state.GetVar(s.param.SplitRef)
		if err != nil {
			return nil, fmt.Errorf("StringTransform: split_ref %q: %w", s.param.SplitRef, err)
		}
		if v == nil {
			varValue = ""
		} else if s, ok := v.(string); ok {
			varValue = s
		} else {
			return nil, fmt.Errorf("StringTransform: split input is not a string: %T", v)
		}
	}

	// Build the regex: |.join([regexp.QuoteMeta(d) for d in delimiters])
	parts := make([]string, 0, len(s.param.Delimiters))
	for _, d := range s.param.Delimiters {
		parts = append(parts, regexp.QuoteMeta(d))
	}
	pattern := "(?s)(" + strings.Join(parts, "|") + ")"
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("StringTransform: bad delimiter pattern: %w", err)
	}
	matches := re.FindAllStringIndex(varValue, -1)

	// Walk the input string, collecting the content between delimiter
	// matches. This mirrors Python's re.split with a capture group
	// (which interleaves content and delimiter tokens) followed by
	// dropping the odd-indexed (delimiter) tokens. When there are no
	// matches, the whole input is a single content token.
	kept := make([]string, 0, len(matches)+1)
	prevEnd := 0
	for _, m := range matches {
		kept = append(kept, varValue[prevEnd:m[0]])
		prevEnd = m[1]
	}
	kept = append(kept, varValue[prevEnd:])
	return map[string]any{"result": kept}, nil
}

// doMerge runs the merge method. Mirrors the Python _merge helper
// (string_transform.py:93-112): collect {{name}} placeholders, resolve
// each from inputs (preferred) or canvas state, substitute, and emit
// the resolved script.
func (s *StringTransformComponent) doMerge(_ context.Context, state *runtime.CanvasState, inputs map[string]any) map[string]any {
	script := s.param.Script

	// First pass: state-level template resolution for any {{ref}} that
	// is a valid cpn_id@param / sys.x / env.x reference. The Python
	// _is_jinjia2 + template.render path is more general; for P1 we
	// only support the simple state-resolvable form.
	if strings.Contains(script, "{{") {
		if resolved, err := runtime.ResolveTemplate(script, state); err == nil {
			script = resolved
		}
	}

	// Second pass: {{name}} placeholders → values from inputs, then state.
	names := extractPlaceholders(script)
	if len(names) == 0 {
		return map[string]any{"result": script}
	}
	for _, n := range names {
		placeholder := "{{" + n + "}}"
		var value any
		if v, ok := inputs[n]; ok {
			value = v
		} else if v, err := state.GetVar(n); err == nil && v != nil {
			value = v
		} else {
			value = ""
		}
		script = strings.ReplaceAll(script, placeholder, fmt.Sprintf("%v", value))
	}
	return map[string]any{"result": script}
}

// extractPlaceholders returns the unique placeholder names appearing
// in s, in first-occurrence order.
func extractPlaceholders(s string) []string {
	if s == "" {
		return nil
	}
	matches := placeholderPattern.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		name := m[1]
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func init() {
	Register(componentNameStringTransform, NewStringTransformComponent)
}
