package parser

import (
	"testing"

	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

// mustPanic runs f and fails the test unless f panics. Used to assert that
// RegisterAnalyzerFactory rejects mis-registrations (fail-loud) instead of
// silently ignoring them.
func mustPanic(t *testing.T, name string, f func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("%s: expected RegisterAnalyzerFactory to panic, but it did not", name)
		}
	}()
	f()
}

// Built-in methods that are dispatched by an internal engine (every switch case
// in ParseWithResult except the fall-through to DeepDoc) cannot be overridden by
// an external factory — the switch intercepts them before resolution runs, so a
// factory registered for them would be silently ignored. RegisterAnalyzerFactory
// must reject such registrations instead of letting them fail silently.
func TestRegisterAnalyzerFactory_RejectsBuiltinEngineMethods(t *testing.T) {
	for _, m := range []string{"plain_text", "mineru", "paddleocr", "docling", "opendataloader", "somark", "tcadp"} {
		mm := m
		mustPanic(t, mm, func() {
			RegisterAnalyzerFactory(mm, newStubFactory("reject"))
		})
	}
}

// Registering for the empty (default) method would hijack every caller that
// omits parse_method and route the default in-process path to an external
// service. That override is not allowed and must be rejected.
func TestRegisterAnalyzerFactory_RejectsEmptyMethod(t *testing.T) {
	mustPanic(t, "empty", func() {
		RegisterAnalyzerFactory("", newStubFactory("reject"))
	})
}

// The "deepdoc" method is the one built-in that falls through to the DeepDoc
// path, so it remains a valid external-override target (the DEEPDOC_URL
// convention). RegisterAnalyzerFactory must NOT reject it.
func TestRegisterAnalyzerFactory_AllowsDeepdoc(t *testing.T) {
	// Must not panic.
	RegisterAnalyzerFactory("deepdoc", newStubFactory("allow"))
	// And it must actually be registered so resolution can pick it up.
	if _, ok := analyzerFactories[normalizePDFParseMethod("deepdoc")]; !ok {
		t.Fatal("deepdoc should be a valid external-override target")
	}
	// Clean up: other tests rely on "deepdoc" being unregistered to exercise
	// the in-process fallback, and the global registry persists across tests.
	delete(analyzerFactories, normalizePDFParseMethod("deepdoc"))
}

// A non-built-in custom method name is the intended use case and must be
// accepted (not panicking).
func TestRegisterAnalyzerFactory_AllowsCustomMethod(t *testing.T) {
	RegisterAnalyzerFactory("ext_register_allowed", func(u string) (deepdoctype.DocAnalyzer, error) {
		return &stubDispatchAnalyzer{tag: "ext"}, nil
	})
	if _, ok := analyzerFactories[normalizePDFParseMethod("ext_register_allowed")]; !ok {
		t.Fatal("custom method should be registered")
	}
}
