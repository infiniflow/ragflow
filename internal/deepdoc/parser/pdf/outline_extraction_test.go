package pdf

import (
	"context"
	"errors"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// ── outline-tracking mock engines ──────────────────────────────────────────

// outlineTrackingEngine wraps MockEngine and records whether Outlines()
// was called.
type outlineTrackingEngine struct {
	*MockEngine
	outlines       []pdf.Outline
	outlinesCalled bool
}

func (e *outlineTrackingEngine) Outlines() ([]pdf.Outline, error) {
	e.outlinesCalled = true
	return e.outlines, nil
}

// outlineErrorEngine returns an error from Outlines().
type outlineErrorEngine struct {
	*MockEngine
}

func (e *outlineErrorEngine) Outlines() ([]pdf.Outline, error) {
	return nil, errors.New("pdfium outline extraction failed")
}

// ── tests for outline extraction in Parse() ─────────────────────────────────

// TestParse_ExtractsOutlinesFromEngine verifies that Parse() calls
// engine.Outlines() and the result carries the outlines.
//
// This test currently FAILS because:
//  1. Parse() never calls engine.Outlines() → outlinesCalled stays false
//  2. ParseResult has no Outlines field → compilation error if we try to read it
func TestParse_ExtractsOutlinesFromEngine(t *testing.T) {
	expectedOutlines := []pdf.Outline{
		{Title: "Chapter 1", Level: 0, PageNumber: 1},
		{Title: "Section 1.1", Level: 1, PageNumber: 2},
	}
	eng := &outlineTrackingEngine{
		MockEngine: &MockEngine{NumPages: 3},
		outlines:   expectedOutlines,
	}
	mockDLA := &MockDocAnalyzer{Healthy: true}
	p := NewParser(pdf.DefaultParserConfig())

	result, err := p.ParseRaw(context.Background(), eng, mockDLA)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if result == nil {
		t.Fatal("Parse returned nil result")
	}

	// Check 1: engine.Outlines() was actually called
	if !eng.outlinesCalled {
		t.Error("BUG: Parse() never called engine.Outlines() — outlines are extracted by pdfium but ignored")
	}

	// Check 2: outlines are present in ParseResult
	if len(result.Outlines) == 0 {
		t.Error("BUG: ParseResult.Outlines is empty — outlines extracted but not stored")
	}
	if len(result.Outlines) != len(expectedOutlines) {
		t.Errorf("result.Outlines: got %d, want %d", len(result.Outlines), len(expectedOutlines))
	}
}

// TestParse_OutlinesErrorDoesNotBlockParsing verifies that when
// engine.Outlines() fails, the parse still completes successfully
// and produces sections (outlines are best-effort).
func TestParse_OutlinesErrorDoesNotBlockParsing(t *testing.T) {
	eng := &outlineErrorEngine{
		MockEngine: &MockEngine{
			NumPages: 2,
			Chars: map[int][]pdf.TextChar{
				0: {{Text: "Hello world", X0: 100, X1: 200, Top: 100, Bottom: 120}},
				1: {{Text: "Page two", X0: 100, X1: 200, Top: 100, Bottom: 120}},
			},
		},
	}
	mockDLA := &MockDocAnalyzer{Healthy: true}
	p := NewParser(pdf.DefaultParserConfig())

	result, err := p.ParseRaw(context.Background(), eng, mockDLA)
	if err != nil {
		t.Fatalf("Parse should not fail when Outlines() errors: %v", err)
	}
	if result == nil {
		t.Fatal("Parse returned nil result")
	}
	if len(result.Sections) == 0 {
		t.Error("Parse should still produce sections even if Outlines() fails")
	}
	if len(result.Outlines) != 0 {
		t.Errorf("outlines should be empty on error, got %d", len(result.Outlines))
	}
}
