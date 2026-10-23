package component

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"reflect"
	"strings"
	"testing"
)

func monkeyTestZip(t *testing.T, layouts string, images map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, _ := zw.Create("sample/sample.json")
	_, _ = f.Write([]byte(layouts))
	for name, data := range images {
		w, _ := zw.Create("sample/" + name)
		_, _ = w.Write(data)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestMonkeyOCRv2ExtractItemsEmitPositions(t *testing.T) {
	zipBytes := monkeyTestZip(t, `{"layouts":[
		{"label":"Title","content":"Heading","page_num":2,"bbox":[10,20,110,40]},
		{"label":"Text","content":"no page key","bbox":[50,60,5,10]},
		{"label":"Text","content":"no bbox"}
	]}`, nil)
	items, _, err := monkeyOCRv2ExtractItems(zipBytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("items=%v", items)
	}
	if got := items[0]["_pdf_positions"].([][]any); !reflect.DeepEqual(got, [][]any{{2, 10.0, 110.0, 20.0, 40.0}}) {
		t.Fatalf("first positions=%#v", got)
	}
	if got := items[1]["positions"].([][]any); !reflect.DeepEqual(got, [][]any{{1, 5.0, 50.0, 10.0, 60.0}}) {
		t.Fatalf("second positions (default page, normalized)=%#v", got)
	}
	if _, ok := items[2]["positions"]; ok {
		t.Fatalf("layout without bbox must not carry positions: %#v", items[2])
	}
}

func TestMonkeyOCRv2EmbeddedImageBudget(t *testing.T) {
	zipBytes := monkeyTestZip(t, `{"layouts":[
		{"label":"Picture","content":"![a](images/a.png)","page_num":1,"bbox":[1,2,3,4]},
		{"label":"Picture","content":"![b](images/b.png)","page_num":1,"bbox":[5,6,7,8]},
		{"label":"Picture","content":"![c](images/missing.png)"}
	]}`, map[string][]byte{
		"images/a.png": bytes.Repeat([]byte{1}, 100),
		"images/b.png": bytes.Repeat([]byte{2}, 100),
	})
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatal(err)
	}
	images := map[string]*zip.File{}
	for _, f := range zr.File {
		images[f.Name[len("sample/"):]] = f
	}
	images["a.png"], images["b.png"] = images["images/a.png"], images["images/b.png"]
	budget := &monkeyOCRv2ImageBudget{remaining: 150}
	pos := []any{1, 1.0, 3.0, 2.0, 4.0}
	first := monkeyOCRv2EmbeddedImage("![a](images/a.png)", images, pos, budget)
	if first == nil || first["image"] == nil || budget.remaining != 50 {
		t.Fatalf("first=%v budget=%+v", first, budget)
	}
	second := monkeyOCRv2EmbeddedImage("![b](images/b.png)", images, pos, budget)
	if second == nil {
		t.Fatal("over-budget image with a position must be kept as a positions-only item")
	}
	if _, ok := second["image"]; ok {
		t.Fatalf("over-budget image must not be inlined: %v", second)
	}
	if _, ok := second["_pdf_positions"]; !ok || budget.remaining != 50 {
		t.Fatalf("second=%v budget=%+v", second, budget)
	}
	if budget.docOverflow != 1 {
		t.Fatalf("over-budget omission must be counted for warnings: %+v", budget)
	}
	if got := monkeyOCRv2EmbeddedImage("![c](images/missing.png)", images, nil, budget); got != nil {
		t.Fatalf("missing image without a position must be dropped, got %v", got)
	}
	if got := monkeyOCRv2EmbeddedImage("![c](images/missing.png)", images, pos, budget); got == nil {
		t.Fatal("missing image with a position must keep a positions-only item")
	}
	if w := budget.warnings(); len(w) != 1 || !strings.Contains(w[0], "document image budget") {
		t.Fatalf("warnings=%v", w)
	}
}

func TestMonkeyOCRv2JSONParseResultPageCount(t *testing.T) {
	zipBytes := monkeyTestZip(t, `{"layouts":[
		{"label":"Text","content":"a","page_num":1,"bbox":[1,2,3,4]},
		{"label":"Text","content":"b","page_num":3,"bbox":[1,2,3,4]}
	]}`, nil)
	items, _, err := monkeyOCRv2ExtractItems(zipBytes)
	if err != nil {
		t.Fatal(err)
	}
	res := monkeyOCRv2JSONParseResult("sample.pdf", items, nil)
	if res.OutputFormat != "json" || res.File["page_count"] != 3 || res.File["parse_method"] != "monkeyocrv2" {
		t.Fatalf("result=%+v", res)
	}
}

func TestMonkeyOCRv2InlineDataURIChargesDocumentBudget(t *testing.T) {
	png := bytes.Repeat([]byte{7}, 100)
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	budget := &monkeyOCRv2ImageBudget{remaining: 150}
	raw := map[string]any{"layouts": []any{
		map[string]any{"label": "Text", "content": "![a](" + uri + ")", "page_num": float64(1), "bbox": []any{1.0, 2.0, 3.0, 4.0}},
		map[string]any{"label": "Text", "content": "![b](" + uri + ")", "page_num": float64(1), "bbox": []any{5.0, 6.0, 7.0, 8.0}},
	}}
	items := monkeyOCRv2LayoutItems(raw, nil, budget)
	if len(items) != 2 {
		t.Fatalf("items=%v", items)
	}
	if _, ok := items[0]["image"]; !ok {
		t.Fatalf("inline image within budget must be inlined: %#v", items[0])
	}
	if _, ok := items[1]["image"]; ok {
		t.Fatalf("inline image past the document budget must not be inlined: %#v", items[1])
	}
	if _, ok := items[1]["_pdf_positions"]; !ok {
		t.Fatalf("over-budget inline image must keep its position: %#v", items[1])
	}
	if budget.docOverflow != 1 {
		t.Fatalf("omission must be counted: %+v", budget)
	}
	if w := budget.warnings(); len(w) != 1 || !strings.Contains(w[0], "document image budget") {
		t.Fatalf("warnings=%v", w)
	}
}

func TestMonkeyOCRv2InlineDataURIPositionsOnlyWithoutPosition(t *testing.T) {
	budget := &monkeyOCRv2ImageBudget{remaining: 10}
	raw := map[string]any{"layouts": []any{
		map[string]any{"label": "Text", "content": "![x](data:image/png;base64," + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 64)) + ")"},
	}}
	items := monkeyOCRv2LayoutItems(raw, nil, budget)
	if len(items) != 0 {
		t.Fatalf("over-budget inline image without a position must be dropped: %#v", items)
	}
	if budget.docOverflow != 1 {
		t.Fatalf("omission must be counted: %+v", budget)
	}
}
