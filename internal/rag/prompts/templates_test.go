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
		"action_set",
		"action_initialize_state",
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

// TestEnumerationProtocolIsNotInTheSystemPrompt pins the split: the enumeration
// protocol lives in its own template, which the harness appends to the turn of a
// session that has SHOWN it is enumerating (see appendBatchProtocol) — never to the
// system prompt, and never to the seed.
//
// It was briefly a section of action_run.md, and it did its job there (the answer
// went from ten-to-thirteen members to sixteen), but a system prompt is paid for
// by EVERY question: a strategy that only helps an enumeration is a tax on the
// single-value questions it cannot help. It then rode the seed, gated on the shape
// of the planner's table, and that gate misfired too — measured (2026-09-15) 11 of
// 20 FRAMES sessions were handed it for questions whose answer is one number.
//
// The FORM of the batch matters as much as the instruction to write one, which is
// why the alternation is pinned here. Measured (2026-09-14, the recall run): every
// name the answer needed that later runs lost — 程远志、荀正、杨龄、夏侯存 — entered
// the corpus as the MODEL'S OWN query term, in an alternation of the names it
// doubted (`关羽斩杨龄|荀正|夏侯存`, `关公斩庞德|成何`, `关羽斩黄巾|程远志|管亥|张宝|张梁`),
// with the runtime then searching each name on its own. Then the template was
// rewritten to say "batches of four to six, space-separated" and the same question
// came back with thirteen members: the batches became `关羽 斩 颜良 文丑` — four
// fields of which two are the subject and the verb — and the doubtful names were
// never asked about at all (grep of that run for 荀正/杨龄/夏侯存: zero).
//
// The alternation instruction ALONE did not restore them, which is why the demand
// for unseen names is pinned too. Measured (2026-09-16): a run whose seed carried
// this template (3416 chars — the version with items 2 and 3 written as the
// alternation and the doubtful batch, but without item 1's "at least one you have
// not") wrote the alternation ZERO times and answered the same thirteen members.
// The template that had produced seventeen members (2509ccb96) differed in exactly
// two places, and both demanded NAMES rather than a FORM: it asked for a long list
// ("fifteen to twenty-five candidates") and it said what to leave out — "Do NOT
// stop at the names you have already seen in a passage: those are the ones
// retrieval reaches by itself, and they are exactly the ones that are not
// missing". A number is a corpus-independent way of saying "long", but the ratio
// in item 1 says the same thing without pretending the answer's size is known, and
// it keeps the second, load-bearing half: the names you have already read are not
// the missing ones.
func TestEnumerationProtocolIsNotInTheSystemPrompt(t *testing.T) {
	run, err := (EmbeddedPromptLoader{}).Load("action_run")
	if err != nil {
		t.Fatalf("Load(action_run): %v", err)
	}
	if strings.Contains(run, "SET / COUNT directions") {
		t.Error("the enumeration protocol must not be part of the system prompt")
	}

	set, err := (EmbeddedPromptLoader{}).Load("action_set")
	if err != nil {
		t.Fatalf("Load(action_set): %v", err)
	}
	for _, want := range []string{"SET / COUNT directions", "name1|name2|name3", "at least one you have not", "names you are LEAST sure of", "two consecutive batches"} {
		if !strings.Contains(set, want) {
			t.Errorf("action_set is missing %q", want)
		}
	}
	// The budget exception is GONE, and this is the assertion that keeps it gone.
	// "the 1-2-call budget does NOT apply" turned a method into a licence: the run
	// that carried it in every set-shaped seed went 60 → 92 rounds and 1 → 5
	// zero-score questions. A batch is a new fact ON a set direction; the global call
	// budget is not suspended by this template.
	if strings.Contains(set, "does NOT apply") {
		t.Error("action_set must not suspend the global call budget")
	}
}
