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

package prompts

import (
	"strings"
	"testing"
)

// TestEmbeddedLoaderCanonicalTemplates makes sure every template the Go column
// relies on is actually embedded and stripped the way Python's load_prompt
// strips the canonical rag/prompts/*.md files.
func TestEmbeddedLoaderCanonicalTemplates(t *testing.T) {
	names := []string{
		"action_run",
		"action_initialize_state",
		"sca_select",
		"sca_query_rewrite",
	}
	for _, name := range names {
		got, err := (EmbeddedPromptLoader{}).Load(name)
		if err != nil {
			t.Fatalf("Load(%q): %v", name, err)
		}
		if got == "" {
			t.Errorf("Load(%q) returned an empty template", name)
		}
		if strings.TrimSpace(got) != got {
			t.Errorf("Load(%q) returned surrounding whitespace (load_prompt strips both ends)", name)
		}
	}
}

// TestEmbeddedLoaderMissingName reports an error for templates we do not embed,
// so the caller's fallback engages exactly like load_prompt's FileNotFoundError.
func TestEmbeddedLoaderMissingName(t *testing.T) {
	if _, err := (EmbeddedPromptLoader{}).Load("no_such_template"); err == nil {
		t.Fatal("Load(no_such_template) succeeded, want error")
	}
}

// TestRenderPromptNilLoaderUsesEmbeddedCanonical locks in that an unset loader
// now resolves the canonical embedded .md (1:1 with rag/prompts/sca_select.md)
// rather than the condensed constant — the whole point of the orchestration
// parity fix.
func TestRenderPromptNilLoaderUsesEmbeddedCanonical(t *testing.T) {
	vars := map[string]string{"question": "Q?", "overall_draft": "D", "claims_context": "C"}
	got := Render(nil, "sca_select", "", vars)

	for _, frag := range []string{
		`Google-style "Sufficient Context Agent"`, // canonical sca_select.md only
		"Q?",
		"D",
		"C",
	} {
		if !strings.Contains(got, frag) {
			t.Errorf("canonical sca_select render missing %q", frag)
		}
	}
	if strings.Contains(got, "{{") {
		t.Errorf("render left an unresolved placeholder: %q", got)
	}
}

// TestRenderPromptNilLoaderQueryRewriteCanonical is the rewrite-side twin:
// nil loader must resolve the full embedded sca_query_rewrite.md, not the
// condensed constant.

// TestRenderPromptNilLoaderQueryRewriteCanonical is the rewrite-side twin:
// nil loader must resolve the full embedded sca_query_rewrite.md, not the
// condensed constant.
func TestRenderPromptNilLoaderQueryRewriteCanonical(t *testing.T) {
	vars := map[string]string{
		"question":         "Q?",
		"gaps":             "G",
		"bridge_values":    "B",
		"research_context": "R",
	}
	got := Render(nil, "sca_query_rewrite", "", vars)

	// Rule 1 wording exists only in the canonical template.
	if !strings.Contains(got, "The query must name the specific missing entity") {
		t.Errorf("canonical sca_query_rewrite render not used; got:\n%s", got)
	}
	for _, frag := range []string{"Q?", "G", "B", "R"} {
		if !strings.Contains(got, frag) {
			t.Errorf("canonical sca_query_rewrite render missing %q", frag)
		}
	}
	if strings.Contains(got, "{{") {
		t.Errorf("render left an unresolved placeholder: %q", got)
	}
}

// TestRenderUnknownNameWithEmptyFallbackReturnsEmpty mirrors Python's
// fail-fast loading: Go has no condensed fallback constant (unlike the earlier
// prototype), so a loader that cannot resolve the name with fallback="" yields
// the empty string rather than a degraded prompt.

// TestRenderUnknownNameWithEmptyFallbackReturnsEmpty mirrors Python's
// fail-fast loading: Go has no condensed fallback constant (unlike the earlier
// prototype), so a loader that cannot resolve the name with fallback="" yields
// the empty string rather than a degraded prompt.
func TestRenderUnknownNameWithEmptyFallbackReturnsEmpty(t *testing.T) {
	got := Render(stubLoader{}, "no_such_template", "",
		map[string]string{"question": "Q?", "gaps": "G"})
	if got != "" {
		t.Errorf("expected empty string for unknown template with empty fallback, got:\n%s", got)
	}
}

// TestRenderPromptMissingVarInterpolatesEmpty mirrors Python's default Jinja
// Undefined: an unprovided variable renders as the empty string, not as a
// literal {{ var }}.

// TestRenderPromptMissingVarInterpolatesEmpty mirrors Python's default Jinja
// Undefined: an unprovided variable renders as the empty string, not as a
// literal {{ var }}.
func TestRenderPromptMissingVarInterpolatesEmpty(t *testing.T) {
	loader := stubLoader{text: "A={{present}} B={{ absent }} C={{absent}}"}
	got := Render(loader, "t", "fallback", map[string]string{"present": "x"})
	if want := "A=x B= C="; got != want {
		t.Errorf("render = %q, want %q", got, want)
	}
}

// TestRenderPromptSubstitutesBothPlaceholderSpellings: Jinja tolerates the
// spaced and unspaced forms; the renderer must handle both.

// TestRenderPromptSubstitutesBothPlaceholderSpellings: Jinja tolerates the
// spaced and unspaced forms; the renderer must handle both.
func TestRenderPromptSubstitutesBothPlaceholderSpellings(t *testing.T) {
	loader := stubLoader{text: "{{v}}|{{ v }}"}
	got := Render(loader, "t", "fallback", map[string]string{"v": "x"})
	if want := "x|x"; got != want {
		t.Errorf("render = %q, want %q", got, want)
	}
}

// stubLoader serves a canned template, mirroring how tests wire
// harness.StringPromptLoader.
type stubLoader struct{ text string }

func (s stubLoader) Load(string) (string, error) { return s.text, nil }

// TestRenderPromptNilLoaderUsesEmbeddedCanonical locks in that an unset loader
// now resolves the canonical embedded .md (1:1 with rag/prompts/sca_select.md)
// rather than the condensed constant — the whole point of the orchestration
// parity fix.
