//go:build cgo

package parser

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	officeOxide "github.com/yfedoseev/office_oxide/go"
)

// TestPPTXParser_FormatField verifies the format field wiring:
// NewPPTXParser() defaults to "pptx", and an explicit "ppt" can be set.
func TestPPTXParser_FormatField(t *testing.T) {
	p := NewPPTXParser()
	if p.format != "pptx" {
		t.Errorf("NewPPTXParser().format = %q, want %q", p.format, "pptx")
	}
	p2 := &PPTXParser{format: "ppt"}
	if p2.format != "ppt" {
		t.Errorf("explicit PPTXParser{format: \"ppt\"}.format = %q, want %q", p2.format, "ppt")
	}
}

// TestPPTXParser_ParseWithResult_CGO verifies that PPTXParser can
// parse a programmatically generated PPTX document into per-slide
// JSON items. Uses office_oxide's own PptxWriter to produce the
// test data so no external file is needed.
func TestPPTXParser_ParseWithResult_CGO(t *testing.T) {
	ctx := t.Context()
	p := NewPPTXParser()
	data := buildPPTX(t, "Hello World")
	res := p.ParseWithResult(ctx, "test.pptx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if res.OutputFormat != "json" {
		t.Errorf("OutputFormat = %q, want %q", res.OutputFormat, "json")
	}
	if got := res.File["format"]; got != "pptx" {
		t.Errorf("File[format] = %v, want %q", got, "pptx")
	}
	if len(res.JSON) == 0 {
		t.Fatal("JSON items is empty; expected at least one slide")
	}
}

// TestPPTXParser_ParseWithResult_MultiSlide_CGO is the regression test
// for the one-chunk-per-slide contract: a multi-slide deck must emit
// one JSON item per slide, each carrying that slide's text only.
// office_oxide's PlainText concatenates slides without a delimiter, so
// splitting must come from the structured IR sections.
func TestPPTXParser_ParseWithResult_MultiSlide_CGO(t *testing.T) {
	ctx := t.Context()
	p := NewPPTXParser()

	w := officeOxide.NewPptxWriter()
	s1 := w.AddSlide()
	w.SetSlideTitle(s1, "Slide One Title")
	w.AddSlideText(s1, "First slide body about the Alpha topic.")
	s2 := w.AddSlide()
	w.SetSlideTitle(s2, "Slide Two Title")
	w.AddSlideText(s2, "Second slide body about the Beta topic.")
	data, err := w.ToBytes()
	if err != nil {
		t.Fatalf("PptxWriter.ToBytes: %v", err)
	}

	res := p.ParseWithResult(ctx, "deck.pptx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) != 2 {
		t.Fatalf("JSON items = %d, want 2 (one per slide): %#v", len(res.JSON), res.JSON)
	}
	for i, want := range []struct {
		text     string
		slideNum int
	}{
		{"Slide One Title\nFirst slide body about the Alpha topic.", 1},
		{"Slide Two Title\nSecond slide body about the Beta topic.", 2},
	} {
		it := res.JSON[i]
		if got := it["text"]; got != want.text {
			t.Errorf("item %d text = %q, want %q", i, got, want.text)
		}
		if got := it["slide_number"]; got != want.slideNum {
			t.Errorf("item %d slide_number = %v, want %d", i, got, want.slideNum)
		}
		if got := it["doc_type_kwd"]; got != "text" {
			t.Errorf("item %d doc_type_kwd = %v, want text", i, got)
		}
	}
}

// TestPPTXParser_ParseWithResult_EmptySlide_CGO verifies that a slide
// without extractable text still yields its own JSON item with an
// empty text field, so slide numbering stays aligned with the deck.
func TestPPTXParser_ParseWithResult_EmptySlide_CGO(t *testing.T) {
	ctx := t.Context()
	p := NewPPTXParser()

	w := officeOxide.NewPptxWriter()
	s1 := w.AddSlide()
	w.SetSlideTitle(s1, "Only Title")
	w.AddSlide() // empty slide
	s3 := w.AddSlide()
	w.SetSlideTitle(s3, "Third Title")
	w.AddSlideText(s3, "Third body.")
	data, err := w.ToBytes()
	if err != nil {
		t.Fatalf("PptxWriter.ToBytes: %v", err)
	}

	res := p.ParseWithResult(ctx, "deck.pptx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) != 3 {
		t.Fatalf("JSON items = %d, want 3 (one per slide): %#v", len(res.JSON), res.JSON)
	}
	if got := res.JSON[1]["text"]; got != "" {
		t.Errorf("empty slide text = %q, want empty", got)
	}
	if got := res.JSON[1]["slide_number"]; got != 2 {
		t.Errorf("empty slide slide_number = %v, want 2", got)
	}
}

// TestPPTParser_ParseWithResult_CGO verifies that PPTParser
// delegates correctly to PPTXParser{format:"ppt"} and, via the
// bidirectional magic-byte fallback, correctly opens OOXML content
// through the PPTX path. A .pptx payload uploaded as .ppt should
// succeed and report File["format"] == "pptx" (real container).
func TestPPTParser_ParseWithResult_CGO(t *testing.T) {
	ctx := t.Context()
	p := NewPPTParser()
	data := buildPPTX(t, "Hello")
	res := p.ParseWithResult(ctx, "test.ppt", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if got := res.File["format"]; got != "pptx" {
		t.Errorf("File[format] = %v, want %q (OOXML fallback ppt->pptx)", got, "pptx")
	}
}

// TestPPTParser_TCADPFileType verifies that when a .ppt file is routed
// through PPTParser with parse_method="tcadp", the underlying
// PPTXParser must pass "PPT" as the file_type to TCADP, not "PPTX".
// This test verifies format propagation from PPTParser through
// ConfigureFromSetup to the embedded PPTXParser.
func TestPPTParser_TCADPFileType(t *testing.T) {
	// PPTParser delegates to PPTXParser{format:"ppt"}.
	p := NewPPTParser()
	setup := map[string]any{
		"parse_method":  "tcadp",
		"output_format": "json",
	}
	p.ConfigureFromSetup(setup)

	// Verify the embedded PPTXParser received the config and keeps
	// format="ppt" (which maps to "PPT" in TCADP fileType).
	if p.pptx.format != "ppt" {
		t.Errorf("PPTParser.pptx.format = %q, want %q", p.pptx.format, "ppt")
	}
	if p.pptx.ParseMethod != "tcadp" {
		t.Errorf("PPTParser.pptx.ParseMethod = %q, want %q", p.pptx.ParseMethod, "tcadp")
	}
	if p.pptx.OutputFormat != "json" {
		t.Errorf("PPTParser.pptx.OutputFormat = %q, want %q", p.pptx.OutputFormat, "json")
	}
}

// TestPPTXParser_TCADPFileType verifies that a PPTXParser
// with format="pptx" must derive fileType "PPTX" for TCADP calls.
func TestPPTXParser_TCADPFileType(t *testing.T) {
	p := NewPPTXParser()
	if p.format != "pptx" {
		t.Fatalf("NewPPTXParser().format = %q, want pptx", p.format)
	}
	setup := map[string]any{
		"parse_method":  "tcadp",
		"output_format": "json",
	}
	p.ConfigureFromSetup(setup)
	if p.ParseMethod != "tcadp" {
		t.Errorf("ParseMethod = %q, want tcadp", p.ParseMethod)
	}
}

// TestPPTParser_TCADPIntegration drives the end-to-end TCADP path for a
// .ppt file: PPTParser must POST file_type="PPT" to the reconstruct
// endpoint and then process the returned ZIP artifact into JSON items.
func TestPPTParser_TCADPIntegration(t *testing.T) {
	testPresentationTCADPIntegration(t, NewPPTParser(), "PPT", "presentation.ppt")
}

// TestPPTXParser_TCADPIntegration drives the end-to-end TCADP path for a
// .pptx file: PPTXParser must POST file_type="PPTX" to the reconstruct
// endpoint and then process the returned ZIP artifact into JSON items.
func TestPPTXParser_TCADPIntegration(t *testing.T) {
	testPresentationTCADPIntegration(t, NewPPTXParser(), "PPTX", "presentation.pptx")
}

// tcadpPresentationParser is the shared contract of PPTParser and
// PPTXParser used by the TCADP integration helper below.
type tcadpPresentationParser interface {
	ConfigureFromSetup(setup map[string]any)
	ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult
}

func testPresentationTCADPIntegration(t *testing.T, p tcadpPresentationParser, wantFileType, filename string) {
	withSSRFBypass(t)
	t.Helper()
	zipPayload := tcadpZipFixture(t)
	var gotFileType string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/reconstruct_document":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read reconstruct request: %v", err)
			} else {
				var req struct {
					FileType string `json:"file_type"`
				}
				if err := json.Unmarshal(body, &req); err != nil {
					t.Errorf("decode reconstruct request: %v", err)
				}
				gotFileType = req.FileType
			}
			_, _ = w.Write([]byte(`{"DocumentRecognizeResultUrl":"` + server.URL + `/download.zip"}`))
		case "/download.zip":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zipPayload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	p.ConfigureFromSetup(map[string]any{
		"parse_method":    "tcadp",
		"output_format":   "json",
		"tcadp_apiserver": server.URL,
	})
	ctx := t.Context()
	res := p.ParseWithResult(ctx, filename, []byte("dummy-presentation-bytes"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if gotFileType != wantFileType {
		t.Errorf("TCADP request file_type = %q, want %q", gotFileType, wantFileType)
	}
	if res.OutputFormat != "json" {
		t.Errorf("OutputFormat = %q, want json", res.OutputFormat)
	}
	if len(res.JSON) == 0 {
		t.Fatalf("JSON items = 0, want the fixture's parsed content")
	}
}

// TestPPTXParser_TCADPDownloadHTTPError verifies that a non-2xx response
// from the TCADP download endpoint is surfaced as an explicit error rather
// than parsed as a (malformed) ZIP artifact.
func TestPPTXParser_TCADPDownloadHTTPError(t *testing.T) {
	withSSRFBypass(t)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/reconstruct_document":
			_, _ = w.Write([]byte(`{"DocumentRecognizeResultUrl":"` + server.URL + `/download.zip"}`))
		case "/download.zip":
			http.Error(w, "upstream failure", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	p := NewPPTXParser()
	p.ConfigureFromSetup(map[string]any{
		"parse_method":    "tcadp",
		"output_format":   "json",
		"tcadp_apiserver": server.URL,
	})
	ctx := t.Context()
	res := p.ParseWithResult(ctx, "a.pptx", []byte("dummy"))
	if res.Err == nil {
		t.Fatal("ParseWithResult: want error for non-2xx download, got nil")
	}
	if !strings.Contains(res.Err.Error(), "download HTTP") {
		t.Errorf("err = %v, want error mentioning 'download HTTP'", res.Err)
	}
}

// buildPPTX creates a minimal valid PPTX document with one slide
// containing the given text, using office_oxide's PptxWriter.
func buildPPTX(t *testing.T, text string) []byte {
	t.Helper()
	w := officeOxide.NewPptxWriter()
	slide := w.AddSlide()
	w.SetSlideTitle(slide, text)
	data, err := w.ToBytes()
	if err != nil {
		t.Fatalf("PptxWriter.ToBytes: %v", err)
	}
	return data
}
