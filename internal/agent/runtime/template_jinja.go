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

// template_jinja.go — gonja direct import.
//
// Gonja was already a declared dependency in go.mod (marked
// `// indirect`). This file promotes gonja to a direct import by
// adding a parallel template resolver that handles Jinja2-specific
// syntax ({% ... %}, {{ x | filter }}, {# comment #}).
//
// The regex-based ResolveTemplate in template.go is kept as the
// fast path for the common `{{cpn_id@key}}` form — that path
// compiles to a single regex match + lookup, no parser
// allocation. ResolveTemplateAuto dispatches to one or the other
// based on a one-byte scan for Jinja2 markers; for the common
// case the dispatch cost is just the scan.

package runtime

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/nikolalohinski/gonja"
)

// ContainsJinjaSyntax reports whether s contains any Jinja2-only
// construct that the regex fast path can't handle:
//
//   - {% ... %}  statements (if, for, set, ...)
//   - {# ... #}  comments
//   - |          filter pipe inside a {{...}} expression
//
// Pure `{{var}}` references (the common v1 case) return false —
// the fast path handles them. The check is a single byte scan, not
// a parse, so the cost is negligible.
func ContainsJinjaSyntax(s string) bool {
	if strings.Contains(s, "{%") {
		return true
	}
	if strings.Contains(s, "{#") {
		return true
	}
	// Filter pipe: a '|' inside a {{...}} block. We don't need to
	// track whether we're inside braces — the regex fast path
	// would not match `{{x|filter}}` correctly because the '|'
	// isn't part of the ref grammar, so any pipe anywhere is
	// grounds for falling back to gonja.
	if strings.ContainsRune(s, '|') {
		return true
	}
	return false
}

// ResolveTemplateJinja renders s through gonja. The execution
// context is built from the canvas state — every cpn_id@key pair
// GetVar knows about becomes a top-level variable in the template
// scope, alongside the special `sys.*` and `env.*` names.
//
// The conversion from CanvasState to gonja context is a best-effort
// flatten: nested maps become dotted identifiers (matching the v1
// `{{cpn_id@key.subkey}}` grammar that GetVar already understands),
// other types are passed through unchanged. The flattening is
// shallow — gonja templates that walk deeply nested structures
// may need a future revision that builds a proper nested map.
//
// Loud-fail on parse / execution errors: the regex fast path
// degrades gracefully (it returns the partial output plus an
// error); the Jinja2 path either parses cleanly or returns an
// error with the original string untouched. Callers that want the
// "always return a string" semantics can wrap the call themselves.
func ResolveTemplateJinja(s string, state *CanvasState) (string, error) {
	if state == nil {
		return "", fmt.Errorf("template: nil canvas state")
	}
	ctx := stateToGonjaContext(state)
	tpl, err := gonja.FromString(s)
	if err != nil {
		return "", fmt.Errorf("template: parse: %w", err)
	}
	out, err := tpl.Execute(ctx)
	if err != nil {
		return "", fmt.Errorf("template: execute: %w", err)
	}
	return out, nil
}

// stateToGonjaContext builds a map[string]any from a CanvasState
// that gonja can resolve. Top-level keys are the cpn ids
// (begin_0, llm_0, ...); values are the per-cpn output buckets
// passed through unchanged so gonja can walk nested structures
// with its native dot syntax ({{ agent_0.user.name }} resolves
// to the bucket's "user" sub-map's "name" key).
func stateToGonjaContext(state *CanvasState) map[string]any {
	ctx := make(map[string]any, len(state.Outputs))
	for cpnID, bucket := range state.Snapshot() {
		ctx[cpnID] = bucket
	}
	return ctx
}

// flattenMap converts a nested map[string]any into a flat map
// where nested keys are joined by '.'. Lists / scalars are
// passed through. Used by stateToGonjaContext so gonja can walk
// `{{ cpn_0.user.name }}` the same way GetVar walks
// `cpn_0@user.name`.
func flattenMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if nested, ok := v.(map[string]any); ok {
			for nk, nv := range flattenMap(nested) {
				out[k+"."+nk] = nv
			}
			continue
		}
		out[k] = v
	}
	return out
}

// ResolveTemplateAuto dispatches to the regex fast path
// (ResolveTemplate) or the Jinja2 path (ResolveTemplateJinja)
// based on whether the input contains Jinja2-only syntax. The
// fast path is preferred because:
//
//   - It compiles to a single regex match + state lookup — no
//     parse allocation, no map flatten.
//   - It already handles every `{{ cpn_id@key }}` form that
//     RAGFlow v1 fixtures use (per plan §2.11.6 entry for
//     Switch, Categorize, etc.).
//   - It fails loud on unresolvable refs — useful for catching
//     misconfigured canvases at runtime.
//
// Falls back to gonja only when the input has Jinja2 markers
// ({% %}, {# #}, or | inside a {{...}}). Callers that always
// want Jinja2 semantics can call ResolveTemplateJinja directly.
func ResolveTemplateAuto(s string, state *CanvasState) (string, error) {
	if !ContainsJinjaSyntax(s) {
		return ResolveTemplate(s, state)
	}
	return ResolveTemplateJinja(s, state)
}

// bytesBuffer is a no-op import-anchor kept for the future
// streaming-resolver path (chunked streaming; not implemented
// in this revision).
var _ = bytes.NewBuffer
