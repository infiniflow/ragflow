// Package canvas — variable resolver unit tests.
//
// Scope: tests the 3 reference forms documented in
// docs/develop/agent-go-port-design.md appendix D:
//   - cpn_id@param     (e.g. "llm_0@content", "begin_0@query")
//   - sys.<name>       (e.g. "sys.query", "sys.user_id")
//   - env.<name>       (e.g. "env.max_tokens")
//
// Additional supported aliases:
//   - {{item}} / {{index}} iteration aliases
//
// Out of scope:
//   - nested dot paths (cpn_0@result.answer) — base.py:400-410 does this
//     in canvas.get_value_with_variable AFTER the regex match succeeds.
//   - list indexing (xs.0) — same nested-path machinery.
//
// The VarRefPattern regex (internal/agent/runtime/template.go) is kept
// in sync with Python's agent/component/base.py variable_ref_patt.
// PR #16792 (Jun 2026) widened cpn_id from [a-zA-Z:0-9]+ to
// [a-zA-Z0-9_:]+ to accept underscored frontend ids and colon-bearing
// legacy DSL ids. This file pins both behaviors to prevent silent
// regression.
package canvas

import (
	"reflect"
	"testing"

	"ragflow/internal/agent/runtime"
)

func TestVariableResolver(t *testing.T) {
	mkState := func() *CanvasState {
		s := NewCanvasState("run-1", "task-1")
		s.SetVar("llm_0", "content", "hello world")
		s.SetVar("begin_0", "query", "ragflow go port")
		s.Sys["query"] = "what is ragflow"
		s.Sys["user_id"] = "tenant-1"
		s.Env["max_tokens"] = 1024
		return s
	}

	type tcase struct {
		name     string
		template string
		setup    func(s *CanvasState)
		want     string
		wantErr  bool
	}

	cases := []tcase{
		{
			name:     "single cpn ref",
			template: "{{llm_0@content}}",
			setup:    func(s *CanvasState) {},
			want:     "hello world",
		},
		{
			name:     "triple-brace (Python allows extra braces)",
			template: "{{{llm_0@content}}}",
			setup:    func(s *CanvasState) {},
			want:     "hello world",
		},
		{
			name:     "single brace (Python allows)",
			template: "{llm_0@content}",
			setup:    func(s *CanvasState) {},
			want:     "hello world",
		},
		{
			name:     "embedded in text",
			template: "Refined: {{llm_0@content}} done",
			setup:    func(s *CanvasState) {},
			want:     "Refined: hello world done",
		},
		{
			name:     "sys ref",
			template: "Q: {{sys.query}}",
			setup:    func(s *CanvasState) {},
			want:     "Q: what is ragflow",
		},
		{
			name:     "env ref",
			template: "limit {{env.max_tokens}}",
			setup:    func(s *CanvasState) {},
			want:     "limit 1024",
		},
		{
			name:     "multiple refs in one template",
			template: "{{sys.query}} :: {{llm_0@content}} :: {{env.max_tokens}}",
			setup:    func(s *CanvasState) {},
			want:     "what is ragflow :: hello world :: 1024",
		},
		{
			name:     "no ref returns input as-is",
			template: "plain text only",
			setup:    func(s *CanvasState) {},
			want:     "plain text only",
		},
		{
			// Go behavior: ResolveTemplate returns an error on
			// unresolved refs (loud-fail; see variable.go
			// ResolveTemplate doc). Python's canvas.py:177-178
			// silently returns "" — the Go port trades Python's
			// silent soft-fail for a Go-idiomatic error return so
			// parameter binding can surface misconfigured canvases
			// early.
			name:     "unresolved cpn ref returns error (loud-fail, Go port deviation)",
			template: "x={{missing@thing}}y",
			setup:    func(s *CanvasState) {},
			wantErr:  true,
		},
		{
			name:     "sys ref missing key returns error",
			template: "[{{sys.nope}}]",
			setup:    func(s *CanvasState) {},
			wantErr:  true,
		},
		{
			name:     "iteration item alias resolves from globals",
			template: "{{item}}",
			setup: func(s *CanvasState) {
				s.Globals["__item__"] = "alpha"
			},
			want: "alpha",
		},
		{
			name:     "iteration index alias resolves from globals",
			template: "i={{index}}",
			setup: func(s *CanvasState) {
				s.Globals["__index__"] = 3
			},
			want: "i=3",
		},
		{
			name:     "garbage ref (no @ or sys/env prefix) passes through unchanged",
			template: "{{garbage}}",
			setup:    func(s *CanvasState) {},
			want:     "{{garbage}}",
		},
		{
			name:     "empty template",
			template: "",
			setup:    func(s *CanvasState) {},
			want:     "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := mkState()
			c.setup(s)
			got, err := ResolveTemplate(c.template, s)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (val=%q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

// TestVarRefPattern_MatchesPythonDrift guards against accidental regex
// changes. If someone edits VarRefPattern, this test demands they also
// update the Python source (or document the deviation) — preventing
// silent divergence between Go and Python regex behavior.
func TestVarRefPattern_MatchesPythonDrift(t *testing.T) {
	positive := []string{
		"{{llm_0@content}}",
		"{{{llm_0@content}}}",
		"{llm_0@content}",
		"{{sys.query}}",
		"{{sys.user_id}}",
		"{{env.max_tokens}}",
		"{{begin_0@query}}",
		"prefix {{llm_0@x}} suffix",
		"{{agent:ThreePathsDecide@content}}", // colon-prefixed cpn id
	}
	for _, s := range positive {
		if !VarRefPattern.MatchString(s) {
			t.Errorf("expected match for %q", s)
		}
	}
	negative := []string{
		"plain text",
		"",
	}
	for _, s := range negative {
		if VarRefPattern.MatchString(s) {
			t.Errorf("expected NO match for %q", s)
		}
	}
}

// TestExtractRefs covers the pure-regex extraction helper.
func TestExtractRefs(t *testing.T) {
	got := ExtractRefs("{{a@x}} {{b@y}} {{a@x}} {{sys.q}}")
	want := []string{"a@x", "b@y", "sys.q"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtractRefs: got %v want %v", got, want)
	}
}

// TestVarRefPattern_MatchesUnderscoredComponentIDs pins that frontend-
// emitted ids like "userfillup_abc@line" match VarRefPattern.
// Regression test for the original #16758 underscore fix
// (Python equivalent: test_variable_ref_patt_matches_underscored_component_ids).
func TestVarRefPattern_MatchesUnderscoredComponentIDs(t *testing.T) {
	cases := []struct {
		text     string
		expected string // group-1 capture (bare ref)
	}{
		{"{userfillup_abc@line}", "userfillup_abc@line"},
		{"{retrieval_xyz@chunks}", "retrieval_xyz@chunks"},
		{"{llm_0@content}", "llm_0@content"},
		{"{message_0@answer}", "message_0@answer"},
	}

	for _, c := range cases {
		t.Run(c.text, func(t *testing.T) {
			matches := VarRefPattern.FindAllStringSubmatch(c.text, 1)
			if len(matches) == 0 || len(matches[0]) < 2 {
				t.Fatalf("Expected %q to match VarRefPattern", c.text)
			}
			got := matches[0][1]
			if got != c.expected {
				t.Fatalf("%q: wrong capture — got %q, expected %q", c.text, got, c.expected)
			}
		})
	}
}

// TestVarRefPattern_MatchesColonBearingComponentIDs pins that legacy
// DSL ids like "UserFillUp:CateInput@text" match VarRefPattern.
// Regression test from PR #16792 review note — colon-bearing cpn_ids
// are real and used in test fixtures/templates
// (Python equivalent: test_variable_ref_patt_matches_colon_bearing_component_ids).
func TestVarRefPattern_MatchesColonBearingComponentIDs(t *testing.T) {
	cases := []struct {
		text     string
		expected string
	}{
		{"{UserFillUp:CateInput@text}", "UserFillUp:CateInput@text"},
		{"{UserFillUp:CodeInput@x}", "UserFillUp:CodeInput@x"},
		{"{UserFillUp:LoopInput@value}", "UserFillUp:LoopInput@value"},
		{"{Retrieval:KBSearch@formalized_content}", "Retrieval:KBSearch@formalized_content"},
		{"{CodeExec:Double@result}", "CodeExec:Double@result"},
		{"{Browser:BusyHatsSink@content}", "Browser:BusyHatsSink@content"},
	}

	for _, c := range cases {
		t.Run(c.text, func(t *testing.T) {
			matches := VarRefPattern.FindAllStringSubmatch(c.text, 1)
			if len(matches) == 0 || len(matches[0]) < 2 {
				t.Fatalf("Expected %q to match VarRefPattern — colon-bearing cpn_id lost its support.", c.text)
			}
			got := matches[0][1]
			if got != c.expected {
				t.Fatalf("%q: wrong capture — got %q, expected %q", c.text, got, c.expected)
			}
		})
	}
}

// TestVarRefPattern_StillMatchesLegacyIDs verifies backward-compat:
// legacy ids without underscores/colons must still resolve.
func TestVarRefPattern_StillMatchesLegacyIDs(t *testing.T) {
	cases := []struct {
		text     string
		expected string
	}{
		{"{begin@line}", "begin@line"},
		{"{retrieval@chunks}", "retrieval@chunks"},
		{"{sys.query}", "sys.query"},
		{"{sys.user_id}", "sys.user_id"},
		{"{env.HOME}", "env.HOME"},
	}

	for _, c := range cases {
		t.Run(c.text, func(t *testing.T) {
			matches := VarRefPattern.FindAllStringSubmatch(c.text, 1)
			if len(matches) == 0 || len(matches[0]) < 2 {
				t.Fatalf("Expected %q to match VarRefPattern", c.text)
			}
			if matches[0][1] != c.expected {
				t.Fatalf("%q: wrong capture — got %q, expected %q", c.text, matches[0][1], c.expected)
			}
		})
	}
}

// TestVarRefPattern_DoesNotMatchBareVarName verifies that
// `{line}` without a cpn_id prefix is intentionally not a template
// ref — it must remain literal so the user sees the literal text.
func TestVarRefPattern_DoesNotMatchBareVarName(t *testing.T) {
	if VarRefPattern.MatchString("{line}") {
		t.Fatal("Bare `{line}` should not match — only `cpn_id@var` / `sys.*` / `env.*` are valid template refs.")
	}
}

// TestVarRefPattern_ConsistencyPin verifies that the runtime.VarRefPattern
// pattern string and its factory-compiled regex agree. This guards against
// accidental edits that change the string without updating the compile.
func TestVarRefPattern_ConsistencyPin(t *testing.T) {
	// The Go regexp package does not expose a way to reconstruct a
	// regexp from its string representation at test time while
	// preserving the same object identity, so we instead pin that the
	// source string matches the expected pattern character-by-character.
	want := `\{+\s*([a-zA-Z:0-9_]+@[A-Za-z0-9_.-]+|sys\.[A-Za-z0-9_.]+|env\.[A-Za-z0-9_.]+|item|index)\s*\}+`
	if runtime.VarRefPattern.String() != want {
		t.Errorf(
			"VarRefPattern string has drifted.\n got  %s\n want %s\n"+
				"Did you edit the regex? If so, update this test AND the Python source at agent/component/base.py.",
			runtime.VarRefPattern.String(), want,
		)
	}
}

// TestResolveTemplate_SubstitutesUnderscoredRef ensures that
// an underscored ref is substituted end-to-end through ResolveTemplate.
func TestResolveTemplate_SubstitutesUnderscoredRef(t *testing.T) {
	s := NewCanvasState("run-1", "task-1")
	s.SetVar("userfillup_abc", "line", "hello world")

	rendered, err := ResolveTemplate("Repeat: {{userfillup_abc@line}}", s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rendered != "Repeat: hello world" {
		t.Fatalf("got %q want %q", rendered, "Repeat: hello world")
	}
}

// TestExtractRefs_ColonBearing verifies ExtractRefs returns colon-bearing refs.
func TestExtractRefs_ColonBearing(t *testing.T) {
	got := ExtractRefs(
		"{{UserFillUp:CateInput@text}} {{Retrieval:KBSearch@f}} {{sys.query}}",
	)
	want := []string{"UserFillUp:CateInput@text", "Retrieval:KBSearch@f", "sys.query"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtractRefs colon-bearing: got %v want %v", got, want)
	}
}
