// Package canvas — variable resolver unit tests.
//
// Scope: tests the 3 reference forms documented in
// docs/develop/agent-go-port-design.md appendix D:
//   - cpn_id@param     (e.g. "llm_0@content", "begin_0@query")
//   - sys.<name>       (e.g. "sys.query", "sys.user_id")
//   - env.<name>       (e.g. "env.max_tokens")
//
// Out of scope (handled by iteration components):
//   - {{item}} / {{index}} aliases — base.py:369 has a separate
//     iteration_alias_patt consulted only by iteration components.
//   - nested dot paths (cpn_0@result.answer) — base.py:400-410 does this
//     in canvas.get_value_with_variable AFTER the regex match succeeds.
//   - list indexing (xs.0) — same nested-path machinery.
//
// Cpn IDs in tests use underscores (e.g. "llm_0") which is the real
// RAGFlow naming convention; the original documented regex
// `[a-zA-Z:0-9]+` did not allow underscores — see variable.go
// VarRefPattern comment.
package canvas

import (
	"reflect"
	"testing"
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
			name:     "iteration alias NOT in v1 regex (matches base.py:368)",
			template: "{{item}}",
			setup:    func(s *CanvasState) {},
			want:     "{{item}}",
		},
		{
			name:     "iteration index alias passes through unchanged",
			template: "i={{index}}",
			setup:    func(s *CanvasState) {},
			want:     "i={{index}}",
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
		"{{item}}",            // iteration alias — not in v1 regex
		"{{index}}",           // iteration alias — not in v1 regex
		"{{ cpn_0@content }}", // inner spaces around cpn_id — regex does not allow
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
