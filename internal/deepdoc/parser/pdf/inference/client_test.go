package inference

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mustNewDeepDocClient wraps NewClient for test convenience.
// Fails the test if the URL is empty.
func mustNewDeepDocClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	client, err := NewClient(baseURL)
	if err != nil {
		t.Fatalf("NewDeepDocClient(%q): %v", baseURL, err)
	}
	return client
}

// testImage creates a small 10x10 red image for HTTP client tests.
func testImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	return img
}

// ── Happy-path tests ──────────────────────────────────────────────────

func TestDeepDocHTTP_DLA(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request format.
		if r.URL.Path != "/predict/dla" {
			t.Errorf("path = %q, want /predict/dla", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Error("expected multipart/form-data content type")
		}
		// Verify multipart field name is "request".
		file, header, err := r.FormFile("request")
		if err != nil {
			t.Fatalf("missing 'request' multipart field: %v", err)
		}
		defer file.Close()
		if !strings.HasSuffix(header.Filename, ".jpeg") {
			t.Errorf("filename = %q, want *.jpeg", header.Filename)
		}

		// Return canned DLA response: one table region (classId=5).
		// Format: bboxes = [[x0, y0, x1, y1, confidence, classId], ...]
		json.NewEncoder(w).Encode(map[string]any{
			"bboxes": [][]float64{
				{50, 100, 500, 300, 0.95, 5}, // classId 5 = "table"
				{50, 10, 500, 50, 0.90, 0},   // classId 0 = "title"
			},
		})
	}))
	defer srv.Close()

	client := mustNewDeepDocClient(t, srv.URL)
	regions, err := client.DLA(context.Background(), testImage())
	if err != nil {
		t.Fatal(err)
	}
	if len(regions) != 2 {
		t.Fatalf("got %d regions, want 2", len(regions))
	}
	if regions[0].Label != "table" {
		t.Errorf("region[0].Label = %q, want 'table'", regions[0].Label)
	}
	if regions[0].Confidence != 0.95 {
		t.Errorf("region[0].Confidence = %f, want 0.95", regions[0].Confidence)
	}
	if regions[1].Label != "title" {
		t.Errorf("region[1].Label = %q, want 'title'", regions[1].Label)
	}
}

func TestDeepDocHTTP_TSR(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/predict/tsr" {
			t.Errorf("path = %q, want /predict/tsr", r.URL.Path)
		}
		// Return canned TSR response: 2 cells.
		json.NewEncoder(w).Encode(map[string]any{
			"bboxes": [][]float64{
				{10, 20, 200, 50, 0.99},
				{210, 20, 400, 50, 0.98},
			},
		})
	}))
	defer srv.Close()

	client := mustNewDeepDocClient(t, srv.URL)
	cells, err := client.TSR(context.Background(), testImage())
	if err != nil {
		t.Fatal(err)
	}
	if len(cells) != 2 {
		t.Fatalf("got %d cells, want 2", len(cells))
	}
	if cells[0].X0 != 10 || cells[0].Y1 != 50 {
		t.Errorf("cell[0] coords wrong: %+v", cells[0])
	}
}

func TestDeepDocHTTP_OCRDetect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/predict/ocr" {
			t.Errorf("path = %q, want /predict/ocr", r.URL.Path)
		}
		// Verify operator=det form field.
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatal(err)
		}
		if op := r.FormValue("operator"); op != "det" {
			t.Errorf("operator = %q, want 'det'", op)
		}
		// Verify image is JPEG (not PNG).
		file, header, _ := r.FormFile("request")
		defer file.Close()
		if !strings.HasSuffix(header.Filename, ".jpeg") {
			t.Errorf("filename = %q, want *.jpeg", header.Filename)
		}

		// Return canned OCR detect response: 1 quad box.
		// Format: {"output": [[[[[x0,y0],[x1,y1],[x2,y2],[x3,y3]], ...]]]}
		json.NewEncoder(w).Encode(map[string]any{
			"output": [][][][][]float64{
				{
					{
						{{10, 20}, {100, 20}, {100, 40}, {10, 40}},
					},
				},
			},
		})
	}))
	defer srv.Close()

	client := mustNewDeepDocClient(t, srv.URL)
	boxes, err := client.OCRDetect(context.Background(), testImage())
	if err != nil {
		t.Fatal(err)
	}
	if len(boxes) != 1 {
		t.Fatalf("got %d boxes, want 1", len(boxes))
	}
	if boxes[0].X0 != 10 || boxes[0].Y0 != 20 || boxes[0].X1 != 100 {
		t.Errorf("box coords wrong: %+v", boxes[0])
	}
}

func TestDeepDocHTTP_OCRRecognize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/predict/ocr" {
			t.Errorf("path = %q, want /predict/ocr", r.URL.Path)
		}
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatal(err)
		}
		if op := r.FormValue("operator"); op != "rec" {
			t.Errorf("operator = %q, want 'rec'", op)
		}

		// Return canned OCR recognize response.
		// Format: {"output": [[[["text", confidence], ...]]]}
		json.NewEncoder(w).Encode(map[string]any{
			"output": [][][][]any{
				{
					{
						{"Hello World", 0.98},
						{"你好世界", 0.95},
					},
				},
			},
		})
	}))
	defer srv.Close()

	client := mustNewDeepDocClient(t, srv.URL)
	texts, err := client.OCRRecognize(context.Background(), testImage())
	if err != nil {
		t.Fatal(err)
	}
	if len(texts) != 2 {
		t.Fatalf("got %d texts, want 2", len(texts))
	}
	if texts[0].Text != "Hello World" || texts[0].Confidence != 0.98 {
		t.Errorf("text[0] = %+v, want {Hello World, 0.98}", texts[0])
	}
	if texts[1].Text != "你好世界" {
		t.Errorf("text[1].Text = %q, want '你好世界'", texts[1].Text)
	}
}

func TestDeepDocHTTP_Health(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("path = %q, want /health", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("method = %q, want GET", r.Method)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	client := mustNewDeepDocClient(t, srv.URL)
	if !client.Health() {
		t.Error("Health() = false, want true")
	}
}

// ── Error-path tests ──────────────────────────────────────────────────

func TestDeepDocHTTP_HealthDown(t *testing.T) {
	// Connection refused — no server running.
	client := mustNewDeepDocClient(t, "http://127.0.0.1:1")
	if client.Health() {
		t.Error("Health() = true for unreachable server, want false")
	}
}

func TestDeepDocHTTP_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal server error"))
	}))
	defer srv.Close()

	client := mustNewDeepDocClient(t, srv.URL)

	_, err := client.DLA(context.Background(), testImage())
	if err == nil {
		t.Error("DLA: expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("DLA error should mention 500: %v", err)
	}

	_, err = client.TSR(context.Background(), testImage())
	if err == nil {
		t.Error("TSR: expected error for 500 response")
	}
}

func TestDeepDocHTTP_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{not valid json"))
	}))
	defer srv.Close()

	client := mustNewDeepDocClient(t, srv.URL)

	_, err := client.DLA(context.Background(), testImage())
	if err == nil {
		t.Error("DLA: expected error for malformed JSON")
	}

	_, err = client.TSR(context.Background(), testImage())
	if err == nil {
		t.Error("TSR: expected error for malformed JSON")
	}
}

func TestDeepDocHTTP_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"bboxes": []any{}})
	}))
	defer srv.Close()

	client := mustNewDeepDocClient(t, srv.URL)

	regions, err := client.DLA(context.Background(), testImage())
	if err != nil {
		t.Fatalf("DLA: unexpected error: %v", err)
	}
	if len(regions) != 0 {
		t.Errorf("DLA: got %d regions, want 0", len(regions))
	}

	cells, err := client.TSR(context.Background(), testImage())
	if err != nil {
		t.Fatalf("TSR: unexpected error: %v", err)
	}
	if len(cells) != 0 {
		t.Errorf("TSR: got %d cells, want 0", len(cells))
	}
}

func TestDeepDocHTTP_ShortBBox(t *testing.T) {
	// BBox with fewer than required fields should be skipped.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"bboxes": [][]float64{
				{10, 20, 100},              // too short for DLA (needs 6) and TSR (needs 5)
				{10, 20, 100, 200, 0.9, 5}, // valid DLA
			},
		})
	}))
	defer srv.Close()

	client := mustNewDeepDocClient(t, srv.URL)
	regions, err := client.DLA(context.Background(), testImage())
	if err != nil {
		t.Fatal(err)
	}
	// Only the valid bbox should be returned.
	if len(regions) != 1 {
		t.Errorf("got %d regions, want 1 (short bbox should be skipped)", len(regions))
	}
}
