package parser

import (
	"testing"

	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

// TestIsPDFParseMethod_RecognizesRegisteredExternalAnalyzer covers the gap that
// the EE build surfaced: a parse method backed by a registered external
// analyzer factory (RegisterAnalyzerFactory) must be reported as a PDF parse
// method, consistent with validateParseMethod. Without this, routing code such
// as pdf_vision_dispatch.go and the ingestion config migration would
// misclassify a registered method (e.g. "deepdoc_ee") as a VLM model selector.
func TestIsPDFParseMethod_RecognizesRegisteredExternalAnalyzer(t *testing.T) {
	const method = "ext_is_pdf_parse_method_fix"
	factory := func(u string) (deepdoctype.DocAnalyzer, error) {
		return &stubDispatchAnalyzer{tag: method}, nil
	}
	// "ext_is_pdf_parse_method_fix" is a non-built-in custom method, so
	// RegisterAnalyzerFactory accepts it (it only panics for built-ins and the
	// empty method).
	RegisterAnalyzerFactory(method, factory)
	// The registry persists across tests in this package; remove our entry so
	// other tests (and re-runs) see a clean global state.
	t.Cleanup(func() { delete(analyzerFactories, normalizePDFParseMethod(method)) })

	// Registered method is recognized as a PDF parse method.
	if !IsPDFParseMethod(method) {
		t.Errorf("IsPDFParseMethod(%q) = false, want true for a registered external analyzer method", method)
	}
	// Recognition survives the same normalization IsPDFParseMethod applies
	// internally (case-fold + trim), because the lookup uses the normalized key.
	if !IsPDFParseMethod("  EXT_IS_PDF_PARSE_METHOD_FIX ") {
		t.Errorf("IsPDFParseMethod(<case/space variant of %q>) = false, want true", method)
	}
}

// TestIsPDFParseMethod_BuiltinAndSelectorBehavior documents the contract that
// must NOT change when external methods are recognized:
//   - built-in whitelist spellings still report true;
//   - unknown methods report false;
//   - "@"-suffixed layout_recognizer selectors report false (resolved separately).
func TestIsPDFParseMethod_BuiltinAndSelectorBehavior(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"deepdoc", true},
		{"plain_text", true},
		{"plaintext", true},
		{"mineru", true},
		{"paddleocr", true},
		{"somark", true},
		{"unknown_method_xyz", false},
		{"foo@mineru", false},
		{"bar @paddleocr", false},
	}
	for _, c := range cases {
		if got := IsPDFParseMethod(c.raw); got != c.want {
			t.Errorf("IsPDFParseMethod(%q) = %v, want %v", c.raw, got, c.want)
		}
	}
}
