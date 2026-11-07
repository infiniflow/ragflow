package parser

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// TestNormalizePresentationParseMethod verifies parse_method normalization
// for slides (PPTX and PPT) files.
func TestNormalizePresentationParseMethod(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"deepdoc", ""},
		{"DeepDOC", ""},
		{"DEEPDOC", ""},
		{"  deepdoc  ", ""},
		{"deepdoc parser", ""},
		{"DeepDOC Parser", ""},
		{"tcadp", "tcadp"},
		{"TCADP", "tcadp"},
		{"tcadp parser", "tcadp"},
		{"TCADP Parser", "tcadp"},
		{"TCADP parser", "tcadp"},
		{"unknown", "unknown"},
	}
	for _, c := range cases {
		if got := normalizePresentationParseMethod(c.in); got != c.want {
			t.Errorf("normalizePresentationParseMethod(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestPPTXParser_TCADPDefaults verifies NewPPTXParser and NewPPTParser initialize
// TCADPTableResultType and TCADPMarkdownImageResponseType to "1", consistent with
// NewPDFParser, NewXLSXParser, NewXLSParser, and NewCSVParser.
func TestPPTXParser_TCADPDefaults(t *testing.T) {
	pptx := NewPPTXParser()
	if got, want := pptx.TCADPTableResultType, "1"; got != want {
		t.Errorf("NewPPTXParser().TCADPTableResultType = %q, want %q", got, want)
	}
	if got, want := pptx.TCADPMarkdownImageResponseType, "1"; got != want {
		t.Errorf("NewPPTXParser().TCADPMarkdownImageResponseType = %q, want %q", got, want)
	}

	ppt := NewPPTParser()
	if got, want := ppt.pptx.TCADPTableResultType, "1"; got != want {
		t.Errorf("NewPPTParser().pptx.TCADPTableResultType = %q, want %q", got, want)
	}
	if got, want := ppt.pptx.TCADPMarkdownImageResponseType, "1"; got != want {
		t.Errorf("NewPPTParser().pptx.TCADPMarkdownImageResponseType = %q, want %q", got, want)
	}
}

// TestPPTXParser_ParseWithResult_TCADPParserSpelling asserts that PPTX and PPT
// parsers accept the canonical "TCADP Parser" UI parse_method and dispatch
// to the TCADP cloud API instead of falling through to local office_oxide extraction.
func TestPPTXParser_ParseWithResult_TCADPParserSpelling(t *testing.T) {
	withSSRFBypass(t)
	zipPayload := tcadpZipFixture(t)

	var mu sync.Mutex
	var lastReq map[string]any
	var serverCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/reconstruct_document":
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			serverCalled = true
			lastReq = req
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"DocumentRecognizeResultUrl": "http://" + r.Host + "/result.zip",
			})
		case "/result.zip":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zipPayload)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	testCases := []struct {
		name   string
		parser interface {
			ConfigureFromSetup(map[string]any)
			ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult
		}
		filename     string
		parseMethod  string
		wantFileType string
	}{
		{
			name:         "PPTXParser with TCADP Parser",
			parser:       NewPPTXParser(),
			filename:     "presentation.pptx",
			parseMethod:  "TCADP Parser",
			wantFileType: "PPTX",
		},
		{
			name:         "PPTParser with TCADP Parser",
			parser:       NewPPTParser(),
			filename:     "presentation.ppt",
			parseMethod:  "TCADP Parser",
			wantFileType: "PPT",
		},
		{
			name:         "PPTXParser with lowercase tcadp parser",
			parser:       NewPPTXParser(),
			filename:     "presentation.pptx",
			parseMethod:  "tcadp parser",
			wantFileType: "PPTX",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mu.Lock()
			serverCalled = false
			lastReq = nil
			mu.Unlock()

			tc.parser.ConfigureFromSetup(map[string]any{
				"parse_method":    tc.parseMethod,
				"tcadp_apiserver": server.URL,
			})

			res := tc.parser.ParseWithResult(t.Context(), tc.filename, []byte("fake presentation data"))

			mu.Lock()
			called := serverCalled
			capturedReq := lastReq
			mu.Unlock()

			if !called {
				t.Fatalf("TCADP API server was never called for parse_method %q; fell through to local extractor", tc.parseMethod)
			}
			if res.Err != nil {
				t.Fatalf("ParseWithResult error: %v", res.Err)
			}
			if !res.Produced() {
				t.Fatalf("ParseWithResult produced nothing")
			}
			if got := capturedReq["file_type"]; got != tc.wantFileType {
				t.Errorf("file_type = %v, want %s", got, tc.wantFileType)
			}
			cfg, _ := capturedReq["config"].(map[string]any)
			if got, want := cfg["TableResultType"], "1"; got != want {
				t.Errorf("config.TableResultType = %v, want default %q", got, want)
			}
			if got, want := cfg["MarkdownImageResponseType"], "1"; got != want {
				t.Errorf("config.MarkdownImageResponseType = %v, want default %q", got, want)
			}
		})
	}
}
