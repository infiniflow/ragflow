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
