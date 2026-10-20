package parser

import (
	"context"
	"strings"
	"testing"

	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

// These tests cover the PUBLIC entry path (validateParseMethod + ParseWithResult)
// that the lower-level dispatch tests in pdf_analyzer_dispatch_test.go bypass.
// They pin the previously-failing behavior: a parse method registered via
// RegisterAnalyzerFactory must be accepted by validation and must flow through
// ParseWithResult all the way to the DeepDoc dispatch — not be rejected by the
// supported-methods whitelist before dispatch ever runs.

func newStubFactory(tag string) func(baseURL string) (deepdoctype.DocAnalyzer, error) {
	return func(u string) (deepdoctype.DocAnalyzer, error) {
		return &stubDispatchAnalyzer{tag: tag}, nil
	}
}

// A method registered through the external analyzer factory must pass
// validation, instead of being rejected as "unsupported". This is the Blocker:
// before the fix, validateParseMethod only consulted supportedPDFParseMethods.
func TestValidateParseMethod_RegisteredExternalMethodAccepted(t *testing.T) {
	RegisterAnalyzerFactory("ext_entry_happy", newStubFactory("ext"))
	if err := (&PDFParser{ParseMethod: "ext_entry_happy"}).validateParseMethod(); err != nil {
		t.Fatalf("registered method should pass validation, got err: %v", err)
	}
}

// Registration and resolution both normalize the method token, so a differently
// cased registration must still be accepted for the canonical (lower-cased)
// method name.
func TestValidateParseMethod_RegisteredMethodNormalization(t *testing.T) {
	RegisterAnalyzerFactory("Ext_Entry_Case", newStubFactory("ext"))
	if err := (&PDFParser{ParseMethod: "ext_entry_case"}).validateParseMethod(); err != nil {
		t.Fatalf("mixed-case registered method should pass validation, got err: %v", err)
	}
}

// Leading/trailing whitespace in the configured method must not defeat the
// registered-method lookup, since both sides normalize with TrimSpace.
func TestValidateParseMethod_RegisteredMethodTrimsWhitespace(t *testing.T) {
	RegisterAnalyzerFactory("ext_entry_ws", newStubFactory("ext"))
	if err := (&PDFParser{ParseMethod: "  EXT_ENTRY_WS  "}).validateParseMethod(); err != nil {
		t.Fatalf("whitespace-padded registered method should pass validation, got err: %v", err)
	}
}

// Regression guard: a method that is neither built-in nor registered must still
// be rejected. The fix must not weaken the whitelist for unknown methods.
func TestValidateParseMethod_UnregisteredUnknownMethodStillRejected(t *testing.T) {
	err := (&PDFParser{ParseMethod: "totally_unknown_method_xyz"}).validateParseMethod()
	if err == nil {
		t.Fatal("unknown unregistered method should be rejected")
	}
	if !strings.Contains(err.Error(), "unsupported PDF parse_method") {
		t.Fatalf("expected unsupported-method error, got: %v", err)
	}
}

// Sanity guard: built-in methods continue to validate as before.
func TestValidateParseMethod_KnownBuiltinMethodAccepted(t *testing.T) {
	for _, m := range []string{"", "deepdoc", "plain_text", "mineru", "paddleocr", "docling", "opendataloader", "somark", "tcadp"} {
		if err := (&PDFParser{ParseMethod: m}).validateParseMethod(); err != nil {
			t.Fatalf("built-in method %q should pass validation, got err: %v", m, err)
		}
	}
}

// End-to-end through the public parser facade: a registered method must reach
// the DeepDoc dispatch path (not be bounced by validateParseMethod). The deepdoc
// engine is not exercised for real here (no models); what we assert is that the
// error, if any, is NOT the "unsupported PDF parse_method" validation error —
// that would mean the registered method never reached dispatch.
func TestParseWithResult_RegisteredExternalMethodReachesDispatch(t *testing.T) {
	const method = "ext_entry_e2e"
	RegisterAnalyzerFactory(method, newStubFactory("ext"))
	t.Setenv("EXT_ENTRY_E2E_URL", "http://ee.example:9380")

	res := (&PDFParser{ParseMethod: method}).ParseWithResult(
		context.Background(), "doc.pdf", []byte("%PDF-1.4 dummy content"))

	if res.Err == nil {
		// Under !cgo the engine reports ErrPDFEngineUnavailable; either way the
		// method must have cleared validation and entered the deepdoc path.
		return
	}
	if strings.Contains(res.Err.Error(), "unsupported PDF parse_method") {
		t.Fatalf("registered method was wrongly rejected at validation: %v", res.Err)
	}
}

// Regression guard at the facade level: an unknown, unregistered method must
// still be rejected by ParseWithResult with the unsupported-method error.
func TestParseWithResult_UnregisteredUnknownMethodRejected(t *testing.T) {
	res := (&PDFParser{ParseMethod: "bogus_method_zzz"}).ParseWithResult(
		context.Background(), "doc.pdf", []byte("%PDF-1.4 dummy content"))
	if res.Err == nil {
		t.Fatal("unknown unregistered method should be rejected by ParseWithResult")
	}
	if !strings.Contains(res.Err.Error(), "unsupported PDF parse_method") {
		t.Fatalf("expected unsupported-method error, got: %v", res.Err)
	}
}
