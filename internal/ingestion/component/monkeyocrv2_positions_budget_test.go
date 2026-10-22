package component

import (
	"archive/zip"
	"bytes"
	"reflect"
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
	items, err := monkeyOCRv2ExtractItems(zipBytes)
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
	budget := 150
	pos := []any{1, 1.0, 3.0, 2.0, 4.0}
	first := monkeyOCRv2EmbeddedImage("![a](images/a.png)", images, pos, &budget)
	if first == nil || first["image"] == nil || budget != 50 {
		t.Fatalf("first=%v budget=%d", first, budget)
	}
	second := monkeyOCRv2EmbeddedImage("![b](images/b.png)", images, pos, &budget)
	if second == nil {
		t.Fatal("over-budget image with a position must be kept as a positions-only item")
	}
	if _, ok := second["image"]; ok {
		t.Fatalf("over-budget image must not be inlined: %v", second)
	}
	if _, ok := second["_pdf_positions"]; !ok || budget != 50 {
		t.Fatalf("second=%v budget=%d", second, budget)
	}
	if got := monkeyOCRv2EmbeddedImage("![c](images/missing.png)", images, nil, &budget); got != nil {
		t.Fatalf("missing image without a position must be dropped, got %v", got)
	}
	if got := monkeyOCRv2EmbeddedImage("![c](images/missing.png)", images, pos, &budget); got == nil {
		t.Fatal("missing image with a position must keep a positions-only item")
	}
}

func TestMonkeyOCRv2JSONParseResultPageCount(t *testing.T) {
	zipBytes := monkeyTestZip(t, `{"layouts":[
		{"label":"Text","content":"a","page_num":1,"bbox":[1,2,3,4]},
		{"label":"Text","content":"b","page_num":3,"bbox":[1,2,3,4]}
	]}`, nil)
	items, err := monkeyOCRv2ExtractItems(zipBytes)
	if err != nil {
		t.Fatal(err)
	}
	res := monkeyOCRv2JSONParseResult("sample.pdf", items)
	if res.OutputFormat != "json" || res.File["page_count"] != 3 || res.File["parse_method"] != "monkeyocrv2" {
		t.Fatalf("result=%+v", res)
	}
}
