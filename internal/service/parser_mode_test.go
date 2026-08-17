package service

import (
	"testing"
)

func TestValidateParseTypeMode_Nil(t *testing.T) {
	_, _, err := ValidateParseTypeMode(nil, nil, nil)
	if err == nil || err.Error() != "parse_type is required" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateParseTypeMode_InvalidZero(t *testing.T) {
	v := 0
	_, _, err := ValidateParseTypeMode(&v, nil, nil)
	if err == nil || err.Error() != "invalid parse_type: 0 (must be 1 or 2)" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateParseTypeMode_InvalidThree(t *testing.T) {
	v := 3
	_, _, err := ValidateParseTypeMode(&v, nil, nil)
	if err == nil || err.Error() != "invalid parse_type: 3 (must be 1 or 2)" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateParseTypeMode_BuiltinMissingParserID(t *testing.T) {
	t.Run("nil parserID", func(t *testing.T) {
		pt := 1
		_, _, err := ValidateParseTypeMode(&pt, nil, nil)
		if err == nil || err.Error() != "parser_id is required when parse_type is BuiltIn" {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("empty parserID", func(t *testing.T) {
		pt := 1
		empty := ""
		_, _, err := ValidateParseTypeMode(&pt, &empty, nil)
		if err == nil || err.Error() != "parser_id is required when parse_type is BuiltIn" {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("whitespace parserID", func(t *testing.T) {
		pt := 1
		ws := "  "
		_, _, err := ValidateParseTypeMode(&pt, &ws, nil)
		if err == nil || err.Error() != "parser_id is required when parse_type is BuiltIn" {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestValidateParseTypeMode_BuiltinOK(t *testing.T) {
	pt := 1
	pid := "laws"
	builtin, pipeline, err := ValidateParseTypeMode(&pt, &pid, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !builtin || pipeline {
		t.Fatalf("expected builtin=true pipeline=false, got builtin=%v pipeline=%v", builtin, pipeline)
	}
}

func TestValidateParseTypeMode_PipelineMissingPipelineID(t *testing.T) {
	t.Run("nil pipelineID", func(t *testing.T) {
		pt := 2
		_, _, err := ValidateParseTypeMode(&pt, nil, nil)
		if err == nil || err.Error() != "pipeline_id is required when parse_type is Pipeline" {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("empty pipelineID", func(t *testing.T) {
		pt := 2
		empty := ""
		_, _, err := ValidateParseTypeMode(&pt, nil, &empty)
		if err == nil || err.Error() != "pipeline_id is required when parse_type is Pipeline" {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestValidateParseTypeMode_PipelineOK(t *testing.T) {
	pt := 2
	pipe := "abc123"
	builtin, pipeline, err := ValidateParseTypeMode(&pt, nil, &pipe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if builtin || !pipeline {
		t.Fatalf("expected builtin=false pipeline=true, got builtin=%v pipeline=%v", builtin, pipeline)
	}
}

func TestResolveParseMode_BuiltinIgnoresPipelineID(t *testing.T) {
	pt := 1
	parserID := "manual"
	pipelineID := "should-be-ignored"
	cur := ParseModeState{ParserID: "naive", PipelineID: strPtr("prior-canvas")}
	isPipeline, effParserID, effPipelineID := ResolveParseMode(&pt, &parserID, &pipelineID, cur)
	if isPipeline {
		t.Fatalf("isPipeline = true, want false (builtin mode)")
	}
	if effParserID != "manual" {
		t.Fatalf("effParserID = %q, want manual", effParserID)
	}
	if effPipelineID != nil {
		t.Fatalf("effPipelineID = %v, want nil (canvas cleared)", effPipelineID)
	}
}

// TestResolveParseMode_PipelineIgnoresParserID reproduces the comment-3 bug
// contract: parse_type=2 must ignore a dirty req.ParserID and keep isPipeline
// true so parser_config is cleaned against the canvas DSL, not the builtin DSL.
func TestResolveParseMode_PipelineIgnoresParserID(t *testing.T) {
	pt := 2
	parserID := "should-be-ignored"
	pipelineID := "1234567890abcdef1234567890abcdef"
	cur := ParseModeState{ParserID: "naive", PipelineID: nil}
	isPipeline, effParserID, effPipelineID := ResolveParseMode(&pt, &parserID, &pipelineID, cur)
	if !isPipeline {
		t.Fatalf("isPipeline = false, want true (pipeline mode)")
	}
	if effParserID != "naive" {
		t.Fatalf("effParserID = %q, want current naive (parser_id not applicable in pipeline mode)", effParserID)
	}
	if effPipelineID == nil || *effPipelineID != pipelineID {
		t.Fatalf("effPipelineID = %v, want %q", effPipelineID, pipelineID)
	}
}

func TestResolveParseMode_BuiltinFallsBackToCurrentParserID(t *testing.T) {
	pt := 1
	cur := ParseModeState{ParserID: "naive", PipelineID: strPtr("prior-canvas")}
	isPipeline, effParserID, effPipelineID := ResolveParseMode(&pt, nil, nil, cur)
	if isPipeline {
		t.Fatalf("isPipeline = true, want false")
	}
	if effParserID != "naive" {
		t.Fatalf("effParserID = %q, want naive", effParserID)
	}
	if effPipelineID != nil {
		t.Fatalf("effPipelineID = %v, want nil", effPipelineID)
	}
}

func TestResolveParseMode_PipelineFallsBackToCurrentPipelineID(t *testing.T) {
	pt := 2
	cur := ParseModeState{ParserID: "naive", PipelineID: strPtr("prior-canvas")}
	isPipeline, effParserID, effPipelineID := ResolveParseMode(&pt, nil, nil, cur)
	if !isPipeline {
		t.Fatalf("isPipeline = false, want true")
	}
	if effParserID != "naive" {
		t.Fatalf("effParserID = %q, want naive", effParserID)
	}
	if effPipelineID == nil || *effPipelineID != "prior-canvas" {
		t.Fatalf("effPipelineID = %v, want prior-canvas", effPipelineID)
	}
}

// TestResolveParseMode_NilParseTypeInheritsCurrent covers the "only
// parser_config changed" path: no mode switch, inherit current state.
func TestResolveParseMode_NilParseTypeInheritsCurrent(t *testing.T) {
	t.Run("current is pipeline", func(t *testing.T) {
		cur := ParseModeState{ParserID: "naive", PipelineID: strPtr("canvas-1")}
		isPipeline, effParserID, effPipelineID := ResolveParseMode(nil, nil, nil, cur)
		if !isPipeline {
			t.Fatalf("isPipeline = false, want true")
		}
		if effParserID != "naive" {
			t.Fatalf("effParserID = %q, want naive", effParserID)
		}
		if effPipelineID == nil || *effPipelineID != "canvas-1" {
			t.Fatalf("effPipelineID = %v, want canvas-1", effPipelineID)
		}
	})
	t.Run("current is builtin", func(t *testing.T) {
		cur := ParseModeState{ParserID: "manual", PipelineID: nil}
		isPipeline, effParserID, effPipelineID := ResolveParseMode(nil, nil, nil, cur)
		if isPipeline {
			t.Fatalf("isPipeline = true, want false")
		}
		if effParserID != "manual" {
			t.Fatalf("effParserID = %q, want manual", effParserID)
		}
		if effPipelineID != nil {
			t.Fatalf("effPipelineID = %v, want nil", effPipelineID)
		}
	})
	t.Run("incremental parser_id update applied", func(t *testing.T) {
		cur := ParseModeState{ParserID: "naive", PipelineID: nil}
		newParser := "laws"
		isPipeline, effParserID, effPipelineID := ResolveParseMode(nil, &newParser, nil, cur)
		if isPipeline {
			t.Fatalf("isPipeline = true, want false")
		}
		if effParserID != "laws" {
			t.Fatalf("effParserID = %q, want laws", effParserID)
		}
		if effPipelineID != nil {
			t.Fatalf("effPipelineID = %v, want nil", effPipelineID)
		}
	})
}
