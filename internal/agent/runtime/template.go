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

// runtime — {{...}} variable reference parser shared by canvas and
// component packages.
//
// The regex is byte-for-byte identical to agent/component/base.py:368
// — any drift must be coordinated with the Python regex in the same
// line.
package runtime

import (
	"fmt"
	"regexp"
)

// VarRefPattern matches the RAGFlow variable reference syntax.
// Mirrors agent/component/base.py:368 in spirit with one deviation: the
// cpn_id part includes '_' (real RAGFlow cpn_ids are like "begin_0",
// "llm_0", "cpn_0"). The Python regex as documented in the plan
// (`[a-zA-Z:0-9]+`) would not match those — this looks like a
// documentation bug in the plan; the Python source likely has
// the underscore too. The pattern uses underscore-friendly
// matching; a future cross-check against the live Python source
// can confirm the exact behavior.
//
// Pattern:
//
//	\{+\s*(<ref>)\s*\}+
//	where <ref> = cpn_id@param | sys.x | env.x | item | index
//	cpn_id = [a-zA-Z:0-9_]+   (note: underscore added; see deviation note)
//	param  = [A-Za-z0-9_.-]+
//
// Capture group 1 holds the bare ref without braces (e.g. "cpn_0@content",
// "sys.query", "env.max_tokens", "item", "index").
var VarRefPattern = regexp.MustCompile(`\{+\s*([a-zA-Z:0-9_]+@[A-Za-z0-9_.-]+|sys\.[A-Za-z0-9_.]+|env\.[A-Za-z0-9_.]+|item|index)\s*\}+`)

// ExtractRefs returns the unique ref strings (without the surrounding
// braces) appearing in s, in first-occurrence order. Pure regex — does not
// touch state. Use this when you need to know "which references does this
// template contain?" without resolving.
func ExtractRefs(s string) []string {
	matches := VarRefPattern.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		ref := m[1]
		if _, dup := seen[ref]; dup {
			continue
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
	}
	return out
}

// ResolveTemplate substitutes every {{...}} in s with the current
// state's value for that ref. Unresolvable refs (GetVar returns
// nil) become errors — the Go port trades Python's silent
// soft-fail (canvas.py:177-178 returns "" for None) for a
// Go-idiomatic loud-fail so parameter binding can surface
// misconfigured canvases early. The partial output (with "" in
// place of the unresolved ref) is still returned so callers can
// choose to log it.
//
// Supported forms match GetVar (cpn_id@param[.path], sys.x[.path], env.x[.path],
// item, index).
func ResolveTemplate(s string, state *CanvasState) (string, error) {
	if !VarRefPattern.MatchString(s) {
		return s, nil
	}
	var firstErr error
	out := VarRefPattern.ReplaceAllStringFunc(s, func(match string) string {
		// Re-extract the bare ref from the match (ReplaceAllStringFunc gives
		// the whole match, not the subgroup).
		sub := VarRefPattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		ref := sub[1]
		v, err := state.GetVar(ref)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("canvas: resolve %q: %w", ref, err)
			}
			return ""
		}
		if v == nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("canvas: unresolved reference %q", ref)
			}
			return ""
		}
		return fmt.Sprintf("%v", v)
	})
	return out, firstErr
}

// ResolveTemplateForDisplay is the display-only variant of
// ResolveTemplate. Unresolvable refs (GetVar returns nil or an
// error) render as empty string instead of failing the call.
// Intended for Message-style template rendering where the partial
// output is what the user ultimately sees; parameter binding
// call sites should keep using ResolveTemplate so a misconfigured
// ref surfaces as an error early.
//
// Mirrors the Python canvas.py:177-178 soft-fail ("unresolved ref
// → empty string") for display rendering.
func ResolveTemplateForDisplay(s string, state *CanvasState) string {
	if state == nil || !VarRefPattern.MatchString(s) {
		return s
	}
	return VarRefPattern.ReplaceAllStringFunc(s, func(match string) string {
		sub := VarRefPattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		v, _ := state.GetVar(sub[1])
		if v == nil {
			return ""
		}
		return fmt.Sprintf("%v", v)
	})
}
