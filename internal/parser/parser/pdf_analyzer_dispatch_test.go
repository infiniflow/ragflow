package parser

import (
	"context"
	"errors"
	"fmt"
	"image"
	"strings"
	"sync"
	"testing"

	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

// stubDispatchAnalyzer is a minimal DocAnalyzer used to prove the external
// factory dispatch routes to the registered factory without touching ONNX
// Runtime or the native backend.
type stubDispatchAnalyzer struct{ tag string }

func (s *stubDispatchAnalyzer) DLA(context.Context, image.Image) ([]deepdoctype.DLARegion, error) {
	return nil, nil
}
func (s *stubDispatchAnalyzer) TSR(context.Context, image.Image) ([]deepdoctype.TSRCell, error) {
	return nil, nil
}
func (s *stubDispatchAnalyzer) OCRDetect(context.Context, image.Image) ([]deepdoctype.OCRBox, error) {
	return nil, nil
}
func (s *stubDispatchAnalyzer) OCRRecognize(context.Context, image.Image) ([]deepdoctype.OCRText, error) {
	return nil, nil
}
func (s *stubDispatchAnalyzer) Health() bool { return true }

// --- Happy path ---------------------------------------------------------

// A registered external factory is selected for its method, and receives the
// endpoint from the generic <METHOD>_URL environment convention.
func TestResolveDocAnalyzerForMethod_RegisteredFactoryGetsEnvURL(t *testing.T) {
	var gotURL string
	RegisterAnalyzerFactory("ext_svc_happy", func(u string) (deepdoctype.DocAnalyzer, error) {
		gotURL = u
		return &stubDispatchAnalyzer{tag: "ext"}, nil
	})
	t.Setenv("EXT_SVC_HAPPY_URL", "http://ee.example:9380")

	a, err := resolveDocAnalyzerForMethod("ext_svc_happy", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a == nil {
		t.Fatal("expected analyzer from the registered factory, got nil")
	}
	if gotURL != "http://ee.example:9380" {
		t.Errorf("factory received URL %q, want the EXT_SVC_HAPPY_URL value", gotURL)
	}
}

// --- Error paths --------------------------------------------------------

// A registered method with no configured URL must fail loudly (never silently
// degrade to empty results) and must not return a half-built analyzer.
func TestResolveDocAnalyzerForMethod_RegisteredFactoryNoURLFailsLoud(t *testing.T) {
	RegisterAnalyzerFactory("ext_svc_nourl", func(u string) (deepdoctype.DocAnalyzer, error) {
		t.Fatalf("factory must not be invoked when %s_URL is unset", "EXT_SVC_NOURL")
		return nil, nil
	})
	// Ensure the env var is genuinely unset for this method.
	t.Setenv("EXT_SVC_NOURL_URL", "")

	a, err := resolveDocAnalyzerForMethod("ext_svc_nourl", nil)
	if err == nil {
		t.Fatal("expected an error when the external URL is unset, got nil")
	}
	if a != nil {
		t.Errorf("expected nil analyzer on error, got %#v", a)
	}
	if !strings.Contains(err.Error(), "EXT_SVC_NOURL_URL") {
		t.Errorf("error %q should name the missing env var for operator diagnosability", err)
	}
}

// An error from the registered factory is propagated verbatim to the caller.
func TestResolveDocAnalyzerForMethod_RegisteredFactoryErrorPropagates(t *testing.T) {
	sentinel := errors.New("boom from factory")
	RegisterAnalyzerFactory("ext_svc_err", func(u string) (deepdoctype.DocAnalyzer, error) {
		return nil, sentinel
	})
	t.Setenv("EXT_SVC_ERR_URL", "http://ee.example:9380")

	a, err := resolveDocAnalyzerForMethod("ext_svc_err", nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected the factory error to propagate, got %v", err)
	}
	if a != nil {
		t.Errorf("expected nil analyzer when the factory errors, got %#v", a)
	}
}

// --- Fallback to in-process (behavior preserved) ------------------------

// A method with no registered factory falls back to the in-process native
// factory — this is the historical, unchanged OSS production path.
func TestResolveDocAnalyzerForMethod_UnregisteredMethodFallsBackToInProcess(t *testing.T) {
	native := func() (deepdoctype.DocAnalyzer, bool) {
		return &stubDispatchAnalyzer{tag: "native"}, true
	}
	a, err := resolveDocAnalyzerForMethod("deepdoc", native)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := a.(*stubDispatchAnalyzer)
	if !ok || got.tag != "native" {
		t.Fatalf("expected the in-process native analyzer, got %#v", a)
	}
}

// No registered factory + unavailable native backend => error (fail loud).
func TestResolveDocAnalyzerForMethod_UnregisteredMethodNoNativeErrors(t *testing.T) {
	native := func() (deepdoctype.DocAnalyzer, bool) { return nil, false }
	a, err := resolveDocAnalyzerForMethod("deepdoc", native)
	if err == nil {
		t.Fatal("expected an error when no backend is available, got nil")
	}
	if a != nil {
		t.Errorf("expected nil analyzer, got %#v", a)
	}
}

// The empty method (callers that do not thread a parse method) must resolve to
// the in-process backend, preserving existing behavior.
func TestResolveDocAnalyzerForMethod_EmptyMethodFallsBackToInProcess(t *testing.T) {
	native := func() (deepdoctype.DocAnalyzer, bool) {
		return &stubDispatchAnalyzer{tag: "native"}, true
	}
	a, err := resolveDocAnalyzerForMethod("", native)
	if err != nil {
		t.Fatalf("unexpected error for empty method: %v", err)
	}
	if a == nil {
		t.Fatal("expected the in-process analyzer for the empty method, got nil")
	}
}

// --- Corner cases -------------------------------------------------------

// Registration and resolution agree after normalization: a canonical method
// registered once is found regardless of the spelling/case used to resolve.
func TestResolveDocAnalyzerForMethod_NormalizesMethodAndCase(t *testing.T) {
	RegisterAnalyzerFactory("ext_svc_norm", func(u string) (deepdoctype.DocAnalyzer, error) {
		return &stubDispatchAnalyzer{tag: "ext"}, nil
	})
	t.Setenv("EXT_SVC_NORM_URL", "http://ee.example:9380")

	for _, q := range []string{"ext_svc_norm", "EXT_SVC_NORM", "  ext_svc_norm  "} {
		a, err := resolveDocAnalyzerForMethod(q, nil)
		if err != nil {
			t.Fatalf("query %q: unexpected error: %v", q, err)
		}
		if a == nil {
			t.Fatalf("query %q: expected the registered analyzer, got nil", q)
		}
	}
}

// Surrounding whitespace in the URL is trimmed before it reaches the factory.
func TestResolveDocAnalyzerForMethod_TrimsURLWhitespace(t *testing.T) {
	var gotURL string
	RegisterAnalyzerFactory("ext_svc_trim", func(u string) (deepdoctype.DocAnalyzer, error) {
		gotURL = u
		return &stubDispatchAnalyzer{}, nil
	})
	t.Setenv("EXT_SVC_TRIM_URL", "  http://ee.example:9380\n")

	if _, err := resolveDocAnalyzerForMethod("ext_svc_trim", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotURL != "http://ee.example:9380" {
		t.Errorf("factory received %q, want the trimmed URL", gotURL)
	}
}

// Re-registering the same method replaces the previous factory (last write
// wins), so a redeploy can swap the backend without a restart of the map.
func TestResolveDocAnalyzerForMethod_LatestRegistrationWins(t *testing.T) {
	RegisterAnalyzerFactory("ext_svc_swap", func(u string) (deepdoctype.DocAnalyzer, error) {
		return &stubDispatchAnalyzer{tag: "first"}, nil
	})
	RegisterAnalyzerFactory("ext_svc_swap", func(u string) (deepdoctype.DocAnalyzer, error) {
		return &stubDispatchAnalyzer{tag: "second"}, nil
	})
	t.Setenv("EXT_SVC_SWAP_URL", "http://ee.example:9380")

	a, err := resolveDocAnalyzerForMethod("ext_svc_swap", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, _ := a.(*stubDispatchAnalyzer); got == nil || got.tag != "second" {
		t.Fatalf("expected the latest registration to win, got %#v", a)
	}
}

// --- Security -----------------------------------------------------------

// The URL is an opaque string handed to the factory; the dispatcher must not
// interpret it as a filesystem path or otherwise transform it beyond trimming.
// A traversal-looking value must reach the factory verbatim.
func TestResolveDocAnalyzerForMethod_URLNotInterpretedAsPath(t *testing.T) {
	const raw = "../../etc/passwd"
	var gotURL string
	RegisterAnalyzerFactory("ext_svc_path", func(u string) (deepdoctype.DocAnalyzer, error) {
		gotURL = u
		return &stubDispatchAnalyzer{}, nil
	})
	t.Setenv("EXT_SVC_PATH_URL", raw)

	if _, err := resolveDocAnalyzerForMethod("ext_svc_path", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotURL != raw {
		t.Errorf("dispatcher altered the URL: got %q, want verbatim %q", gotURL, raw)
	}
}

// The method name is used only as a literal (upper-cased) env-var key; a name
// carrying separators or odd characters must not panic or inject — it simply
// yields a lookup and the documented fail-loud error.
func TestResolveDocAnalyzerForMethod_MethodCannotInjectEnvKey(t *testing.T) {
	RegisterAnalyzerFactory("weird method/../x", func(u string) (deepdoctype.DocAnalyzer, error) {
		return &stubDispatchAnalyzer{}, nil
	})
	// No matching env var for the upper-cased key; must fail loud, not panic.
	_, err := resolveDocAnalyzerForMethod("weird method/../x", nil)
	if err == nil {
		t.Fatal("expected fail-loud error for a registered method with no URL, got nil")
	}
}

// --- Wiring through the real entry point --------------------------------

// The parse method threaded through pdfPostProcessOptions must reach the
// dispatcher at the real parse entry point: a registered external method routes
// parsePDFWithDeepDocOptions to the factory's analyzer (handed to parseFn),
// proving the opts.parseMethod wiring is live end-to-end without ONNX Runtime.
func TestParsePDFWithDeepDocOptions_RoutesRegisteredMethodToFactory(t *testing.T) {
	want := &stubDispatchAnalyzer{tag: "ext-wire"}
	RegisterAnalyzerFactory("ext_wire", func(u string) (deepdoctype.DocAnalyzer, error) {
		return want, nil
	})
	t.Setenv("EXT_WIRE_URL", "http://ee.example:9380")

	var got deepdoctype.DocAnalyzer
	parseFn := func(_ context.Context, _ []byte, a deepdoctype.DocAnalyzer) (*deepdoctype.ParseResult, error) {
		got = a
		return &deepdoctype.ParseResult{}, nil
	}
	res := parsePDFWithDeepDocOptions(context.Background(), "f.pdf", []byte("dummy"),
		pdfPostProcessOptions{parseMethod: "ext_wire"}, parseFn)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if got != want {
		t.Fatalf("parseFn received %#v, want the registered factory analyzer %#v", got, want)
	}
}

// The default (empty) parse method keeps the historical in-process path: with
// no registered factory, the native factory is consulted at the entry point.
func TestParsePDFWithDeepDocOptions_EmptyMethodUsesInProcess(t *testing.T) {
	nativeCalled := false
	native := func() (deepdoctype.DocAnalyzer, bool) {
		nativeCalled = true
		return &stubDispatchAnalyzer{tag: "native"}, true
	}
	// Temporarily point the package-level native factory at our stub so the
	// entry point resolves in-process without ONNX Runtime.
	prev := deepdoctype.NativeDocAnalyzerFactory
	deepdoctype.NativeDocAnalyzerFactory = native
	defer func() { deepdoctype.NativeDocAnalyzerFactory = prev }()

	parseFn := func(_ context.Context, _ []byte, a deepdoctype.DocAnalyzer) (*deepdoctype.ParseResult, error) {
		return &deepdoctype.ParseResult{}, nil
	}
	res := parsePDFWithDeepDocOptions(context.Background(), "f.pdf", []byte("dummy"),
		pdfPostProcessOptions{}, parseFn)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if !nativeCalled {
		t.Fatal("expected the in-process native factory to be consulted for the empty method")
	}
}

// --- Concurrency (read-safety) ------------------------------------------
// Concurrent resolution against a pre-registered factory must be safe: the
// registry is populated at init/startup and only read during parsing.
func TestResolveDocAnalyzerForMethod_ConcurrentResolvesSafe(t *testing.T) {
	RegisterAnalyzerFactory("ext_svc_conc", func(u string) (deepdoctype.DocAnalyzer, error) {
		return &stubDispatchAnalyzer{tag: "ext"}, nil
	})
	t.Setenv("EXT_SVC_CONC_URL", "http://ee.example:9380")

	const n = 64
	var wg sync.WaitGroup
	errs := make([]error, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			a, err := resolveDocAnalyzerForMethod("ext_svc_conc", nil)
			if err == nil && a == nil {
				errs[i] = fmt.Errorf("nil analyzer without error")
			} else {
				errs[i] = err
			}
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}
}
