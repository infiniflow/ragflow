//go:build cgo

package component

import (
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

func TestIsAutoLayoutRecognize(t *testing.T) {
	if !isAutoLayoutRecognize("Auto") || !isAutoLayoutRecognize(" auto ") {
		t.Fatal("expected Auto to match")
	}
	if isAutoLayoutRecognize("DeepDOC") {
		t.Fatal("DeepDOC must not match Auto")
	}
}

func TestPdfLayoutRecognizerFromSetupReadsParseMethod(t *testing.T) {
	got := pdfLayoutRecognizerFromSetup(schema.ParserSetup{"parse_method": "Auto"})
	if !isAutoLayoutRecognize(got) {
		t.Fatalf("parse_method Auto = %q, want Auto", got)
	}
	got = pdfLayoutRecognizerFromSetup(schema.ParserSetup{
		"layout_recognizer": "DeepDOC",
		"parse_method":      "Auto",
	})
	if got != "DeepDOC" {
		t.Fatalf("layout_recognizer should win, got %q", got)
	}
}

func TestParsePositiveInt(t *testing.T) {
	n, err := parsePositiveInt("100")
	if err != nil || n != 100 {
		t.Fatalf("parsePositiveInt(100) = %d, %v", n, err)
	}
	if _, err := parsePositiveInt("0"); err == nil {
		t.Fatal("expected error for zero")
	}
}

func TestAutoProbePageWindow(t *testing.T) {
	from, to := autoProbePageWindow(schema.ParserSetup{})
	if from != 0 || to != -1 {
		t.Fatalf("empty pages = (%d,%d), want (0,-1)", from, to)
	}
	from, to = autoProbePageWindow(schema.ParserSetup{
		"pages": []any{[]any{float64(2), float64(4)}},
	})
	if from != 1 || to != 4 {
		t.Fatalf("pages [[2,4]] = (%d,%d), want (1,4)", from, to)
	}
}
