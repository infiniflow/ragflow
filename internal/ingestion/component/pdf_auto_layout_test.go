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
	if !isAutoLayoutRecognize(got) {
		t.Fatalf("parse_method Auto must win over leftover layout_recognizer, got %q", got)
	}
	got = pdfLayoutRecognizerFromSetup(schema.ParserSetup{
		"layout_recognize":  "Auto",
		"layout_recognizer": "DeepDOC",
	})
	if !isAutoLayoutRecognize(got) {
		t.Fatalf("layout_recognize Auto must win over leftover layout_recognizer, got %q", got)
	}
	got = pdfLayoutRecognizerFromSetup(schema.ParserSetup{
		"layout_recognizer": "DeepDOC",
		"parse_method":      "Plain Text",
	})
	if got != "DeepDOC" {
		t.Fatalf("concrete layout_recognizer should win when nothing is Auto, got %q", got)
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

func TestAutoProbePageWindows(t *testing.T) {
	windows := autoProbePageWindows(schema.ParserSetup{})
	if len(windows) != 1 || windows[0] != [2]int{0, -1} {
		t.Fatalf("empty pages = %v, want [[0 -1]]", windows)
	}
	windows = autoProbePageWindows(schema.ParserSetup{
		"pages": []any{[]any{float64(2), float64(4)}},
	})
	if len(windows) != 1 || windows[0] != [2]int{1, 4} {
		t.Fatalf("pages [[2,4]] = %v, want [[1 4]]", windows)
	}
	windows = autoProbePageWindows(schema.ParserSetup{
		"pages": []any{[]any{float64(1), float64(1)}, []any{float64(10), float64(10)}},
	})
	if len(windows) != 2 || windows[0] != [2]int{0, 1} || windows[1] != [2]int{9, 10} {
		t.Fatalf("disjoint pages = %v, want [[0 1] [9 10]]", windows)
	}
}

func TestChooseAutoPDFLayout(t *testing.T) {
	if got := chooseAutoPDFLayout(100, 100, "DeepDOC", "MinerU"); got != "DeepDOC" {
		t.Fatalf("at threshold = %q, want DeepDOC", got)
	}
	if got := chooseAutoPDFLayout(99, 100, "DeepDOC", "MinerU"); got != "MinerU" {
		t.Fatalf("below threshold = %q, want MinerU", got)
	}
}

func TestApplyChosenAutoPDFLayoutAlwaysWritesParseMethod(t *testing.T) {
	updated := applyChosenAutoPDFLayout(schema.ParserSetup{
		"parse_method":     "Auto",
		"layout_recognize": "Auto",
		"pages":            []any{[]any{1, 2}},
	}, "Plain Text")
	if updated["parse_method"] != "Plain Text" {
		t.Fatalf("parse_method = %v, want Plain Text", updated["parse_method"])
	}
	if updated["layout_recognizer"] != "Plain Text" {
		t.Fatalf("layout_recognizer = %v, want Plain Text", updated["layout_recognizer"])
	}
	if updated["layout_recognize"] != "Plain Text" {
		t.Fatalf("layout_recognize = %v, want Plain Text", updated["layout_recognize"])
	}
}

func TestAutoMinCharsFromSetup(t *testing.T) {
	if n := autoMinCharsFromSetup(schema.ParserSetup{}); n != 100 {
		t.Fatalf("default = %d, want 100", n)
	}
	if n := autoMinCharsFromSetup(schema.ParserSetup{parserConfigAutoMinCharsKey: 40}); n != 40 {
		t.Fatalf("int = %d, want 40", n)
	}
	if n := autoMinCharsFromSetup(schema.ParserSetup{parserConfigAutoMinCharsKey: "25"}); n != 25 {
		t.Fatalf("string = %d, want 25", n)
	}
}
