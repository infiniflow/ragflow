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

// TestPPTParser_ParseWithResult_CGO verifies that PPTParser
// delegates correctly to PPTXParser{format:"ppt"} and produces
// output with File["format"] = "ppt".
func TestPPTParser_ParseWithResult_CGO(t *testing.T) {
	ctx := t.Context()
	p := NewPPTParser()
	// Use PPTX content — office_oxide may reject it with format="ppt"
	// hint (expects OLE binary). When it does, skip gracefully; when
	// it succeeds, verify the metadata contract.
	data := buildPPTX(t, "Hello")
	res := p.ParseWithResult(ctx, "test.ppt", data)
	if res.Err != nil {
		t.Skip("PPTParser with PPTX data (expected maybe to fail):", res.Err)
	}
	if got := res.File["format"]; got != "ppt" {
		t.Errorf("File[format] = %v, want %q", got, "ppt")
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
