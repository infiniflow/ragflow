package service

import (
	"testing"
)

func pi(v int) *int       { return &v }
func ps(v string) *string { return &v }

// All fields absent -> selection block omitted, no error, nil selection.
func TestFromRequest_AllAbsent(t *testing.T) {
	sel, err := FromRequest(nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sel != nil {
		t.Fatalf("expected nil selection, got %#v", sel)
	}
}

// Whitespace-only ids with no parse_type are treated as absent.
func TestFromRequest_BlankIDsNoType(t *testing.T) {
	sel, err := FromRequest(nil, ps("   "), ps("\t"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sel != nil {
		t.Fatalf("expected nil selection, got %#v", sel)
	}
}

// An id without parse_type must be rejected (explicit discriminator required).
func TestFromRequest_ParserIDWithoutType(t *testing.T) {
	sel, err := FromRequest(nil, ps("general"), nil)
	if err == nil || err.Error() != "parse_type is required" {
		t.Fatalf("expected 'parse_type is required', got %v", err)
	}
	if sel != nil {
		t.Fatalf("expected nil selection, got %#v", sel)
	}
}

func TestFromRequest_PipelineIDWithoutType(t *testing.T) {
	sel, err := FromRequest(nil, nil, ps("canvas-1"))
	if err == nil || err.Error() != "parse_type is required" {
		t.Fatalf("expected 'parse_type is required', got %v", err)
	}
	if sel != nil {
		t.Fatalf("expected nil selection, got %#v", sel)
	}
}

// Both ids without parse_type must be rejected.
func TestFromRequest_BothIDsWithoutType(t *testing.T) {
	sel, err := FromRequest(nil, ps("general"), ps("canvas-1"))
	if err == nil || err.Error() != "parse_type is required" {
		t.Fatalf("expected 'parse_type is required', got %v", err)
	}
	if sel != nil {
		t.Fatalf("expected nil selection, got %#v", sel)
	}
}

func TestFromRequest_InvalidZero(t *testing.T) {
	sel, err := FromRequest(pi(0), nil, nil)
	if err == nil || err.Error() != "invalid parse_type: 0 (must be 1 or 2)" {
		t.Fatalf("unexpected error: %v", err)
	}
	if sel != nil {
		t.Fatalf("expected nil selection, got %#v", sel)
	}
}

func TestFromRequest_InvalidThree(t *testing.T) {
	sel, err := FromRequest(pi(3), nil, nil)
	if err == nil || err.Error() != "invalid parse_type: 3 (must be 1 or 2)" {
		t.Fatalf("unexpected error: %v", err)
	}
	if sel != nil {
		t.Fatalf("expected nil selection, got %#v", sel)
	}
}

func TestFromRequest_BuiltinMissingParserID(t *testing.T) {
	t.Run("nil parserID", func(t *testing.T) {
		sel, err := FromRequest(pi(1), nil, nil)
		if err == nil || err.Error() != "parser_id is required when parse_type is BuiltIn" {
			t.Fatalf("unexpected error: %v", err)
		}
		if sel != nil {
			t.Fatalf("expected nil selection, got %#v", sel)
		}
	})
	t.Run("empty parserID", func(t *testing.T) {
		sel, err := FromRequest(pi(1), ps(""), nil)
		if err == nil || err.Error() != "parser_id is required when parse_type is BuiltIn" {
			t.Fatalf("unexpected error: %v", err)
		}
		if sel != nil {
			t.Fatalf("expected nil selection, got %#v", sel)
		}
	})
	t.Run("whitespace parserID", func(t *testing.T) {
		sel, err := FromRequest(pi(1), ps("  "), nil)
		if err == nil || err.Error() != "parser_id is required when parse_type is BuiltIn" {
			t.Fatalf("unexpected error: %v", err)
		}
		if sel != nil {
			t.Fatalf("expected nil selection, got %#v", sel)
		}
	})
}

// BuiltIn mode must not carry a pipeline_id (mode/id mismatch).
func TestFromRequest_BuiltinWithPipelineID(t *testing.T) {
	sel, err := FromRequest(pi(1), ps("general"), ps("canvas-1"))
	if err == nil || err.Error() != "pipeline_id must not be set when parse_type is BuiltIn" {
		t.Fatalf("unexpected error: %v", err)
	}
	if sel != nil {
		t.Fatalf("expected nil selection, got %#v", sel)
	}
}

func TestFromRequest_BuiltinOK(t *testing.T) {
	sel, err := FromRequest(pi(1), ps("  laws  "), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sel == nil || !sel.IsBuiltIn() || sel.IsPipeline() {
		t.Fatalf("expected builtin selection, got %#v", sel)
	}
	if sel.ParserID != "laws" {
		t.Fatalf("expected trimmed parser_id 'laws', got %q", sel.ParserID)
	}
	if sel.PipelineID != "" {
		t.Fatalf("expected empty pipeline_id, got %q", sel.PipelineID)
	}
}

func TestFromRequest_PipelineMissingPipelineID(t *testing.T) {
	t.Run("nil pipelineID", func(t *testing.T) {
		sel, err := FromRequest(pi(2), nil, nil)
		if err == nil || err.Error() != "pipeline_id is required when parse_type is Pipeline" {
			t.Fatalf("unexpected error: %v", err)
		}
		if sel != nil {
			t.Fatalf("expected nil selection, got %#v", sel)
		}
	})
	t.Run("empty pipelineID", func(t *testing.T) {
		sel, err := FromRequest(pi(2), nil, ps(""))
		if err == nil || err.Error() != "pipeline_id is required when parse_type is Pipeline" {
			t.Fatalf("unexpected error: %v", err)
		}
		if sel != nil {
			t.Fatalf("expected nil selection, got %#v", sel)
		}
	})
}

// Pipeline mode must not carry a parser_id (mode/id mismatch).
func TestFromRequest_PipelineWithParserID(t *testing.T) {
	sel, err := FromRequest(pi(2), ps("general"), ps("canvas-1"))
	if err == nil || err.Error() != "parser_id must not be set when parse_type is Pipeline" {
		t.Fatalf("unexpected error: %v", err)
	}
	if sel != nil {
		t.Fatalf("expected nil selection, got %#v", sel)
	}
}

func TestFromRequest_PipelineOK(t *testing.T) {
	sel, err := FromRequest(pi(2), nil, ps("  canvas-1  "))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sel == nil || !sel.IsPipeline() || sel.IsBuiltIn() {
		t.Fatalf("expected pipeline selection, got %#v", sel)
	}
	if sel.PipelineID != "canvas-1" {
		t.Fatalf("expected trimmed pipeline_id 'canvas-1', got %q", sel.PipelineID)
	}
	if sel.ParserID != "" {
		t.Fatalf("expected empty parser_id, got %q", sel.ParserID)
	}
}

// Effective() for BuiltIn returns no pipeline_id.
func TestParserSelection_EffectiveBuiltIn(t *testing.T) {
	sel := &ParserSelection{Mode: ParseModeBuiltIn, ParserID: "general"}
	isPipeline, parserID, pipelineID := sel.Effective()
	if isPipeline {
		t.Fatalf("isPipeline = true, want false")
	}
	if parserID != "general" {
		t.Fatalf("parserID = %q, want general", parserID)
	}
	if pipelineID != nil {
		t.Fatalf("pipelineID = %v, want nil", pipelineID)
	}
}

// Effective() for Pipeline returns the canvas id; the builtin parser_id is
// ignored by the canvas DSL loader but is preserved for completeness.
func TestParserSelection_EffectivePipeline(t *testing.T) {
	sel := &ParserSelection{Mode: ParseModePipeline, ParserID: "general", PipelineID: "canvas-1"}
	isPipeline, parserID, pipelineID := sel.Effective()
	if !isPipeline {
		t.Fatalf("isPipeline = false, want true")
	}
	if parserID != "general" {
		t.Fatalf("parserID = %q, want general", parserID)
	}
	if pipelineID == nil || *pipelineID != "canvas-1" {
		t.Fatalf("pipelineID = %v, want canvas-1", pipelineID)
	}
}

// CurrentSelection derives pipeline mode from a non-empty persisted pipeline_id.
func TestCurrentSelection_Pipeline(t *testing.T) {
	pid := "canvas-1"
	sel := CurrentSelection("general", &pid)
	if !sel.IsPipeline() {
		t.Fatalf("expected pipeline selection, got %#v", sel)
	}
	if sel.ParserID != "general" || sel.PipelineID != "canvas-1" {
		t.Fatalf("unexpected selection: %#v", sel)
	}
}

func TestCurrentSelection_BuiltIn(t *testing.T) {
	sel := CurrentSelection("general", nil)
	if !sel.IsBuiltIn() {
		t.Fatalf("expected builtin selection, got %#v", sel)
	}
	if sel.PipelineID != "" {
		t.Fatalf("expected empty pipeline_id, got %q", sel.PipelineID)
	}
}

func TestCurrentSelection_BuiltInEmptyPipelineID(t *testing.T) {
	empty := ""
	sel := CurrentSelection("general", &empty)
	if !sel.IsBuiltIn() {
		t.Fatalf("expected builtin selection, got %#v", sel)
	}
}

// Resolve(create): current == nil requires a non-nil selection.
func TestResolve_CreateRequiresSelection(t *testing.T) {
	_, err := Resolve(nil, nil)
	if err == nil || err.Error() != "parse_type is required" {
		t.Fatalf("expected 'parse_type is required', got %v", err)
	}
}

// Resolve(create): current == nil, sel present -> returns sel.
func TestResolve_CreateWithSelection(t *testing.T) {
	sel := &ParserSelection{Mode: ParseModeBuiltIn, ParserID: "general"}
	eff, err := Resolve(nil, sel)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eff != sel {
		t.Fatalf("expected sel returned, got %#v", eff)
	}
}

// Resolve(update): sel == nil keeps the current selection (PATCH omit).
func TestResolve_UpdateOmitKeepsCurrent(t *testing.T) {
	current := CurrentSelection("general", nil)
	eff, err := Resolve(current, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eff != current {
		t.Fatalf("expected current kept, got %#v", eff)
	}
}

// Resolve(update): sel overrides current (mode switch).
func TestResolve_UpdateSwitchesMode(t *testing.T) {
	current := CurrentSelection("general", nil)
	pid := "canvas-1"
	sel := &ParserSelection{Mode: ParseModePipeline, ParserID: "general", PipelineID: pid}
	eff, err := Resolve(current, sel)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eff != sel || !eff.IsPipeline() {
		t.Fatalf("expected pipeline sel returned, got %#v", eff)
	}
}

// Resolve(update): sel overrides current (same-mode id change).
func TestResolve_UpdateSameModeNewID(t *testing.T) {
	current := CurrentSelection("general", nil)
	sel := &ParserSelection{Mode: ParseModeBuiltIn, ParserID: "laws"}
	eff, err := Resolve(current, sel)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eff != sel || eff.ParserID != "laws" {
		t.Fatalf("expected builtin laws, got %#v", eff)
	}
}
