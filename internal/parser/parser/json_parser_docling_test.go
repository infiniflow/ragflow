package parser

import (
	"reflect"
	"testing"

	"ragflow/internal/utility"
)

func TestJSONParser_ImportsDoclingReadingOrder(t *testing.T) {
	data := []byte(`{
		"schema_name": "DoclingDocument",
		"version": "1.5.0",
		"body": {"children": [{"$ref": "#/texts/2"}, {"$ref": "#/groups/0"}]},
		"furniture": {"children": [{"$ref": "#/texts/1"}]},
		"groups": [{"children": [
			{"$ref": "#/texts/0"}, {"$ref": "#/groups/0"},
			{"$ref": "#/texts/3"}, {"$ref": "#/texts/50"}
		]}],
		"texts": [
			{"text": "A repeated paragraph", "label": "text", "prov": [
				{"page_no": 1, "bbox": {"l": 10, "t": 50, "r": 60, "b": 40, "coord_origin": "BOTTOMLEFT"}}
			]},
			{"text": "Running footer", "label": "page_footer"},
			{"text": "Results", "label": "section_header", "level": 2, "prov": [
				{"page_no": 1, "bbox": {"l": 10, "t": 70, "r": 50, "b": 60, "coord_origin": "BOTTOMLEFT"}}
			]},
			{"text": "A repeated paragraph", "label": "text", "prov": [
				{"page_no": 2, "bbox": {"l": 10, "t": 20, "r": 60, "b": 30, "coord_origin": "TOPLEFT"}}
			]}
		],
		"pages": {"1": {"size": {"height": 100}}, "2": {"size": {"height": 100}}}
	}`)
	name := "report.json"
	p, format, err := GetParser(utility.GetFileType(name), data)
	if err != nil || format != utility.FileTypeJSON {
		t.Fatalf("selected parser format %v, error %v", format, err)
	}
	res := p.ParseWithResult(t.Context(), name, data)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	assertJSONTexts(t, res, "Results\n", "A repeated paragraph\n", "A repeated paragraph\n")
	if res.File["page_count"] != 2 || res.File["name"] != name || res.File["size"] != len(data) {
		t.Fatalf("unexpected file metadata: %#v", res.File)
	}

	items := ItemsToMaps(res.JSON)
	if items[0]["layout"] != "heading" || items[0]["heading_level"] != 2 {
		t.Errorf("expected a level-two heading, got %#v", items[0])
	}
	want := [][][]any{
		{{1, 10.0, 50.0, 30.0, 40.0}},
		{{1, 10.0, 60.0, 50.0, 60.0}},
		{{2, 10.0, 60.0, 20.0, 30.0}},
	}
	for i, item := range items {
		if item["page_number"] != want[i][0][0] ||
			!reflect.DeepEqual(item["positions"], want[i]) ||
			!reflect.DeepEqual(item["_pdf_positions"], want[i]) {
			t.Errorf("item %d has incorrect page positions: %#v", i, item)
		}
	}
}

func TestJSONParser_DoclingKeepsPageWhenGeometryIsInvalid(t *testing.T) {
	data := []byte(`{
		"schema_name": "DoclingDocument",
		"body": {"children": [{"$ref": "#/texts/0"}]},
		"texts": [{"text": "Text without a usable box", "prov": [
			{"page_no": 2, "bbox": {"l": "NaN", "t": 1, "r": 2, "b": 3}}
		]}]
	}`)
	res := NewJSONParser().ParseWithResult(t.Context(), "report.json", data)
	assertJSONTexts(t, res, "Text without a usable box\n")
	item := res.JSON[0].ToMap()
	if item["page_number"] != 2 || item["positions"] != nil || item["_pdf_positions"] != nil {
		t.Fatalf("unexpected page metadata: %#v", item)
	}
}

func TestJSONParser_DoclingDetectionKeepsOrdinaryRecords(t *testing.T) {
	p := NewJSONParser()
	ordinary := `{"body":{"children":[]},"schema_name":"OtherDocument","texts":[]}`
	docling := `{"body":{"children":[]},"schema_name":"DoclingDocument","texts":[]}`
	assertJSONTexts(t, p.ParseWithResult(t.Context(), "other.json", []byte(ordinary)), ordinary)
	assertJSONTexts(t, p.ParseWithResult(t.Context(), "records.jsonl", []byte(docling+"\n")), docling)
}
