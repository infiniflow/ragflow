package component

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func mineruTestZipImages(t *testing.T, files map[string][]byte) map[string]*zip.File {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	images := map[string]*zip.File{}
	for _, f := range zr.File {
		images[f.Name] = f
	}
	return images
}

func TestMinerUZipImageDataURIHonorsBudget(t *testing.T) {
	images := mineruTestZipImages(t, map[string][]byte{
		"images/a.png": bytes.Repeat([]byte{1}, 100),
		"images/b.png": bytes.Repeat([]byte{2}, 100),
	})
	budget := 150
	first := mineruZipImageDataURI(images, "images/a.png", &budget)
	if !strings.HasPrefix(first, "data:image/png;base64,") {
		t.Fatalf("first image not inlined: %q", first)
	}
	if budget != 50 {
		t.Fatalf("budget = %d, want 50 after inlining 100 bytes", budget)
	}
	if second := mineruZipImageDataURI(images, "images/b.png", &budget); second != "" {
		t.Fatalf("second image should exceed remaining budget, got %q", second)
	}
	if budget != 50 {
		t.Fatalf("skipped image must not change budget, got %d", budget)
	}
	if got := mineruZipImageDataURI(images, "images/b.png", nil); got == "" {
		t.Fatal("nil budget must not limit inlining")
	}
}

func TestMinerUImageItemKeptWhenBudgetExhausted(t *testing.T) {
	images := mineruTestZipImages(t, map[string][]byte{"images/a.png": bytes.Repeat([]byte{1}, 100)})
	item := map[string]any{
		"type": "image", "img_path": "images/a.png",
		"page_idx": 0, "bbox": []any{1.0, 2.0, 3.0, 4.0},
	}
	budget := 10
	got := mineruContentItemToJSONBudget(item, images, &budget)
	if got == nil {
		t.Fatal("image item with a zip member must be kept when it cannot be inlined")
	}
	if _, ok := got["image"]; ok {
		t.Fatalf("image over budget must not be inlined: %#v", got)
	}
	if got["doc_type_kwd"] != "image" {
		t.Fatalf("doc_type_kwd = %v, want image", got["doc_type_kwd"])
	}
	if _, ok := got["_pdf_positions"]; !ok {
		t.Fatalf("positions must be kept: %#v", got)
	}
	missing := map[string]any{"type": "image", "img_path": "images/none.png"}
	if mineruContentItemToJSONBudget(missing, images, &budget) != nil {
		t.Fatal("image with no text and no zip member must still be dropped")
	}
}
