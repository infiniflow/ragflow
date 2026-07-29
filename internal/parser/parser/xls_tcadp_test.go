package parser

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestXLSXParser_ParseWithResult_TCADPJSONIntegration(t *testing.T) {
	ctx := t.Context()
	zipPayload := tcadpZipFixture(t)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/reconstruct_document":
			_, _ = w.Write([]byte(`{"DocumentRecognizeResultUrl":"` + server.URL + `/download.zip"}`))
		case "/download.zip":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zipPayload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	p, err := NewXLSXParser("")
	if err != nil {
		t.Fatalf("NewXLSXParser: %v", err)
	}
	p.ConfigureFromSetup(map[string]any{
		"parse_method":                 "TCADP parser",
		"output_format":                "json",
		"tcadp_apiserver":              server.URL,
		"tcadp_api_key":                "tcadp-secret",
		"table_result_type":            "1",
		"markdown_image_response_type": "1",
	})
	res := p.ParseWithResult(ctx, "sample.xlsx", []byte("mock xlsx content"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) < 2 {
		t.Fatalf("JSON len = %d, want >=2", len(res.JSON))
	}
}

func TestXLSParser_ParseWithResult_TCADPJSONIntegration(t *testing.T) {
	ctx := t.Context()
	zipPayload := tcadpZipFixture(t)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/reconstruct_document":
			_, _ = w.Write([]byte(`{"DocumentRecognizeResultUrl":"` + server.URL + `/download.zip"}`))
		case "/download.zip":
			_, _ = w.Write(zipPayload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	p, err := NewXLSParser("")
	if err != nil {
		t.Fatalf("NewXLSParser: %v", err)
	}
	p.ConfigureFromSetup(map[string]any{
		"parse_method":    "TCADP parser",
		"output_format":   "json",
		"tcadp_apiserver": server.URL,
		"tcadp_api_key":   "tcadp-secret",
	})
	res := p.ParseWithResult(ctx, "sample.xls", []byte("mock xls content"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) < 2 {
		t.Fatalf("JSON len = %d, want >=2", len(res.JSON))
	}
}

func TestCSVParser_ParseWithResult_TCADPJSONIntegration(t *testing.T) {
	ctx := t.Context()
	zipPayload := tcadpZipFixture(t)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/reconstruct_document":
			_, _ = w.Write([]byte(`{"DocumentRecognizeResultUrl":"` + server.URL + `/download.zip"}`))
		case "/download.zip":
			_, _ = w.Write(zipPayload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	p := NewCSVParser()
	p.ConfigureFromSetup(map[string]any{
		"parse_method":    "TCADP parser",
		"output_format":   "json",
		"tcadp_apiserver": server.URL,
		"tcadp_api_key":   "tcadp-secret",
	})
	res := p.ParseWithResult(ctx, "sample.csv", []byte("a,b,c\n1,2,3"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) < 2 {
		t.Fatalf("JSON len = %d, want >=2", len(res.JSON))
	}
}

func TestXLSXParser_ParseWithResult_TCADPMarkdownIntegration(t *testing.T) {
	ctx := t.Context()
	zipPayload := tcadpZipFixture(t)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/reconstruct_document":
			_, _ = w.Write([]byte(`{"DocumentRecognizeResultUrl":"` + server.URL + `/download.zip"}`))
		case "/download.zip":
			_, _ = w.Write(zipPayload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	p, err := NewXLSXParser("")
	if err != nil {
		t.Fatalf("NewXLSXParser: %v", err)
	}
	p.ConfigureFromSetup(map[string]any{
		"parse_method":    "TCADP parser",
		"output_format":   "markdown",
		"tcadp_apiserver": server.URL,
	})
	res := p.ParseWithResult(ctx, "sample.xlsx", []byte("mock xlsx content"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if res.Markdown == "" {
		t.Fatal("Markdown is empty; want rendered content")
	}
}

func TestXLSXParser_ParseWithResult_TCADPRequiresAPIServer(t *testing.T) {
	ctx := t.Context()
	p, err := NewXLSXParser("")
	if err != nil {
		t.Fatalf("NewXLSXParser: %v", err)
	}
	p.ConfigureFromSetup(map[string]any{"parse_method": "TCADP parser"})
	res := p.ParseWithResult(ctx, "sample.xlsx", []byte("mock xlsx content"))
	if res.Err == nil {
		t.Fatal("expected error about tcadp_apiserver, got nil")
	}
}

func TestXLSParser_ParseWithResult_TCADPRequiresAPIServer(t *testing.T) {
	ctx := t.Context()
	p, err := NewXLSParser("")
	if err != nil {
		t.Fatalf("NewXLSParser: %v", err)
	}
	p.ConfigureFromSetup(map[string]any{"parse_method": "TCADP parser"})
	res := p.ParseWithResult(ctx, "sample.xls", []byte("mock xls content"))
	if res.Err == nil {
		t.Fatal("expected error about tcadp_apiserver, got nil")
	}
}

func TestCSVParser_ParseWithResult_TCADPRequiresAPIServer(t *testing.T) {
	ctx := t.Context()
	p := NewCSVParser()
	p.ConfigureFromSetup(map[string]any{"parse_method": "TCADP parser"})
	res := p.ParseWithResult(ctx, "sample.csv", []byte("mock csv content"))
	if res.Err == nil {
		t.Fatal("expected error about tcadp_apiserver, got nil")
	}
}

func TestXLSXParser_ParseWithResult_InvalidXLSXHandled(t *testing.T) {
	ctx := t.Context()
	p, err := NewXLSXParser("")
	if err != nil {
		t.Fatalf("NewXLSXParser: %v", err)
	}
	res := p.ParseWithResult(ctx, "sample.xlsx", []byte("not a valid xlsx"))
	if res.Err == nil {
		t.Fatal("expected error for invalid xlsx, got nil")
	}
}

func TestXLSParser_ParseWithResult_InvalidXLSHandled(t *testing.T) {
	ctx := t.Context()
	p, err := NewXLSParser("")
	if err != nil {
		t.Fatalf("NewXLSParser: %v", err)
	}
	res := p.ParseWithResult(ctx, "sample.xls", []byte("not a valid xls"))
	if res.Err == nil {
		t.Fatal("expected error for invalid xls, got nil")
	}
}

func TestCSVParser_ParseWithResult_DefaultCSVBehavior(t *testing.T) {
	ctx := t.Context()
	p := NewCSVParser()
	res := p.ParseWithResult(ctx, "sample.csv", []byte("a,b\n1,2"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if got, want := res.OutputFormat, "html"; got != want {
		t.Fatalf("OutputFormat = %q, want %q", got, want)
	}
	if res.HTML == "" {
		t.Fatal("HTML is empty; want rendered table")
	}
}

func TestXLSXParser_ConfigureFromSetup_TCADP(t *testing.T) {
	p, err := NewXLSXParser("")
	if err != nil {
		t.Fatalf("NewXLSXParser: %v", err)
	}
	p.ConfigureFromSetup(map[string]any{
		"parse_method":                 "TCADP parser",
		"output_format":                "json",
		"tcadp_apiserver":              "https://tcadp.example.com",
		"tcadp_api_key":                "secret",
		"table_result_type":            "2",
		"markdown_image_response_type": "2",
	})
	if got, want := p.ParseMethod, "TCADP parser"; got != want {
		t.Fatalf("ParseMethod = %q, want %q", got, want)
	}
	if got, want := p.OutputFormat, "json"; got != want {
		t.Fatalf("OutputFormat = %q, want %q", got, want)
	}
	if got, want := p.TCADPAPIServer, "https://tcadp.example.com"; got != want {
		t.Fatalf("TCADPAPIServer = %q, want %q", got, want)
	}
	if got, want := p.TCADPTableResultType, "2"; got != want {
		t.Fatalf("TCADPTableResultType = %q, want %q", got, want)
	}
	if got, want := p.TCADPMarkdownImageResponseType, "2"; got != want {
		t.Fatalf("TCADPMarkdownImageResponseType = %q, want %q", got, want)
	}
}
