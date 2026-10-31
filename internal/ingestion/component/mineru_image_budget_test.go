package component

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"ragflow/internal/parser/media"
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

func TestMinerUZipImageDataURIHonorsMediaBudget(t *testing.T) {
	images := mineruTestZipImages(t, map[string][]byte{
		"images/a.png": bytes.Repeat([]byte{1}, 100),
		"images/b.png": bytes.Repeat([]byte{2}, 100),
	})
	budget := media.NewBudgetWithLimits(media.Limits{
		ImageMaxBytes:   200,
		MaxImagesPerRun: 10,
		MaxBytesPerRun:  150,
	})
	first := mineruZipImageDataURI(images, "images/a.png", budget)
	if !strings.HasPrefix(first, "data:image/png;base64,") {
		t.Fatalf("first image not inlined: %q", first)
	}
	second := mineruZipImageDataURI(images, "images/b.png", budget)
	if second != "" {
		t.Fatalf("second image should exceed run byte budget, got %q", second)
	}
	warnings := budget.Warnings()
	if len(warnings) == 0 {
		t.Fatal("expected byte-budget warning")
	}
	if got := mineruZipImageDataURI(images, "images/b.png", nil); got == "" {
		t.Fatal("nil budget must not apply run limits")
	}
}

func TestMinerUImageItemKeptWhenZipMemberExists(t *testing.T) {
	images := mineruTestZipImages(t, map[string][]byte{"images/a.png": bytes.Repeat([]byte{1}, 8)})
	item := map[string]any{
		"type": "image", "img_path": "images/a.png",
		"page_idx": 0, "bbox": []any{1.0, 2.0, 3.0, 4.0},
	}
	got := mineruContentItemToJSONBudget(item, images, map[int][2]float64{0: {612, 792}}, media.NewBudget())
	if got == nil {
		t.Fatal("image item with a zip member must be kept without inlining")
	}
	if _, ok := got["image"]; ok {
		t.Fatalf("image must not be inlined on JSON path: %#v", got)
	}
	if got["doc_type_kwd"] != "image" {
		t.Fatalf("doc_type_kwd = %v, want image", got["doc_type_kwd"])
	}
	if _, ok := got["_pdf_positions"]; !ok {
		t.Fatalf("positions must be kept: %#v", got)
	}
	missing := map[string]any{"type": "image", "img_path": "images/none.png"}
	if mineruContentItemToJSONBudget(missing, images, nil, nil) != nil {
		t.Fatal("image with no text and no zip member must still be dropped")
	}
}

func TestMinerUStructuredDataURINotCopiedAndWarnsOnBudget(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 200))
	dataURI := "data:image/png;base64," + payload
	budget := media.NewBudgetWithLimits(media.Limits{
		ImageMaxBytes:   512,
		MaxImagesPerRun: 1,
		MaxBytesPerRun:  100,
	})
	item := map[string]any{
		"type": "image", "image_data_uri": dataURI,
		"image_caption": []any{"cap"},
		"page_idx":      0, "bbox": []any{1.0, 2.0, 3.0, 4.0},
	}
	got := mineruContentItemToJSONBudget(item, nil, map[int][2]float64{0: {612, 792}}, budget)
	if got == nil {
		t.Fatal("expected image item")
	}
	if _, ok := got["image"]; ok {
		t.Fatalf("structured data URI must not be copied: %#v", got)
	}
	if len(budget.Warnings()) == 0 {
		t.Fatalf("expected budget warnings, got none")
	}
}

func TestMinerUExtractJSONItemsManyImagesStaySmall(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	list := make([]map[string]any, 0, 50)
	for i := 0; i < 50; i++ {
		name := "images/fig" + strconv.Itoa(i) + ".png"
		list = append(list, map[string]any{
			"type": "image", "img_path": name,
			"image_caption": []any{"caption"},
			"page_idx":      0, "bbox": []any{10.0, 10.0, 20.0, 20.0},
		})
	}
	raw, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	cl, err := zw.Create("content_list.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = cl.Write(raw); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		name := "images/fig" + strconv.Itoa(i) + ".png"
		img, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = img.Write(bytes.Repeat([]byte{1, 2, 3}, 4000)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	items, warnings, err := mineruExtractJSONItems(buf.Bytes(), map[int][2]float64{0: {612, 792}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 50 {
		t.Fatalf("len(items)=%d, want 50", len(items))
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > 512*1024 {
		t.Fatalf("serialized items too large (%d bytes); zip images were likely inlined", len(encoded))
	}
	for _, item := range items {
		if _, ok := item["image"]; ok {
			t.Fatalf("unexpected inlined image on item %#v", item)
		}
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
}
