package parser

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMarkdownParser_ParseWithResult_Basic(t *testing.T) {
	p, err := NewMarkdownParser(GoMarkdown)
	if err != nil {
		t.Fatalf("NewMarkdownParser: %v", err)
	}
	md := "# Hello\n\nThis is a paragraph.\n\n* List item 1\n* List item 2\n\n```go\nfunc main() {}\n```\n"
	res := p.ParseWithResult("test.md", []byte(md))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if got, want := res.OutputFormat, "json"; got != want {
		t.Fatalf("OutputFormat = %q, want %q", got, want)
	}
	if len(res.JSON) == 0 {
		t.Fatal("JSON is empty; want at least one item")
	}
	// Verify heading
	if got, _ := res.JSON[0]["text"].(string); got != "Hello" {
		t.Fatalf("first item text = %q, want %q", got, "Hello")
	}
	if got, _ := res.JSON[0]["ck_type"].(string); got != "heading" {
		t.Fatalf("first item ck_type = %q, want %q", got, "heading")
	}
}

func TestMarkdownParser_ParseWithResult_EmptyInput(t *testing.T) {
	p, _ := NewMarkdownParser(GoMarkdown)
	res := p.ParseWithResult("empty.md", []byte(""))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("len(JSON) = %d, want 1", len(res.JSON))
	}
}

func TestMarkdownParser_ParseWithResult_ImageDataURI(t *testing.T) {
	p, _ := NewMarkdownParser(GoMarkdown)
	// 1×1 pixel transparent PNG encoded as data URI
	pixelPNG := make([]byte, 68) // minimal 1x1 PNG header
	pixelB64 := base64.StdEncoding.EncodeToString([]byte("fake-png-data"))
	md := "Some text with an image\n![test](data:image/png;base64," + pixelB64 + ")\n"
	_ = pixelPNG
	res := p.ParseWithResult("test.md", []byte(md))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	// Find the image item
	found := false
	for _, item := range res.JSON {
		if kd, _ := item["doc_type_kwd"].(string); kd == "image" {
			found = true
			if img, ok := item["image"].(string); !ok || img != pixelB64 {
				t.Fatalf("image data = %q, want %q", img, pixelB64)
			}
			break
		}
	}
	if !found {
		t.Fatal("no item with doc_type_kwd == 'image' found")
	}
}

func TestMarkdownParser_ParseWithResult_NoImage(t *testing.T) {
	p, _ := NewMarkdownParser(GoMarkdown)
	md := "# Title\n\nJust some text, no images here.\n\nMore text."
	res := p.ParseWithResult("test.md", []byte(md))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	for _, item := range res.JSON {
		if kd, _ := item["doc_type_kwd"].(string); kd == "image" {
			t.Fatal("unexpected image item in text-only markdown")
		}
	}
}

func TestMarkdownParser_ConfigureFromSetup(t *testing.T) {
	p, _ := NewMarkdownParser(GoMarkdown)
	p.ConfigureFromSetup(map[string]any{
		"parse_method":          "deepdoc",
		"output_format":         "json",
		"vlm":                   map[string]any{"llm_id": "gpt-4-vision"},
		"flatten_media_to_text": false,
	})
	if p.ParseMethod != "deepdoc" {
		t.Fatalf("ParseMethod = %q, want %q", p.ParseMethod, "deepdoc")
	}
	if p.OutputFormat != "json" {
		t.Fatalf("OutputFormat = %q, want %q", p.OutputFormat, "json")
	}
	if p.VLM == nil {
		t.Fatal("VLM is nil; want map")
	}
	if id, _ := p.VLM["llm_id"].(string); id != "gpt-4-vision" {
		t.Fatalf("VLM[llm_id] = %q, want %q", id, "gpt-4-vision")
	}
}

func TestMarkdownParser_ConfigureFromSetup_NilSafe(t *testing.T) {
	p, _ := NewMarkdownParser(GoMarkdown)
	p.ConfigureFromSetup(nil) // should not panic
	if p.ParseMethod != "" {
		t.Fatalf("ParseMethod should be empty after nil setup, got %q", p.ParseMethod)
	}
}

func TestResolveMarkdownImage_DataURI(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte("fakeimage"))
	md := "![alt](data:image/png;base64," + b64 + ")"
	result, found := resolveMarkdownImage("", md)
	if !found {
		t.Fatal("expected image found for data URI")
	}
	if result != b64 {
		t.Fatalf("got %q, want %q", result, b64)
	}
}

func TestResolveMarkdownImage_NoImage(t *testing.T) {
	_, found := resolveMarkdownImage("", "# Hello\nNo image here")
	if found {
		t.Fatal("expected no image found")
	}
}

func TestResolveMarkdownImage_HTTPImage(t *testing.T) {
	// httptest servers bind loopback, which the SSRF guard rejects by
	// default. Allow loopback for this test so the HTTP fetch path is
	// exercised (production keeps ssrfAllowLoopback == false).
	prev := ssrfAllowLoopback
	ssrfAllowLoopback = true
	defer func() { ssrfAllowLoopback = prev }()

	// Start a test HTTP server serving a fake PNG
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("fake-png-bytes"))
	}))
	defer ts.Close()

	md := "![alt](" + ts.URL + "/image.png)"
	result, found := resolveMarkdownImage("", md)
	if !found {
		t.Fatal("expected image found for HTTP URL")
	}
	expectedB64 := base64.StdEncoding.EncodeToString([]byte("fake-png-bytes"))
	if result != expectedB64 {
		t.Fatalf("got %q, want %q", result, expectedB64)
	}
}

func TestFetchImageAsBase64_RejectsCredentials(t *testing.T) {
	_, err := fetchImageAsBase64("https://user:pass@example.com/img.png")
	if err == nil {
		t.Fatal("expected error for URL with credentials")
	}
}

func TestFetchImageAsBase64_InvalidURL(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	_, err := fetchImageAsBase64(ts.URL + "/nonexistent.png")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}
