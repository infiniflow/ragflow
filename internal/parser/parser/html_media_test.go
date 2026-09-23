package parser

import (
	"context"
	"testing"
)

const htmlMediaDataURI = "data:image/png;base64,aGVsbG8="

func TestHTMLParser_EmitsInlineImagesInDocumentOrder(t *testing.T) {
	input := `<p>before <img alt="figure alt" src="` + htmlMediaDataURI + `"> after</p>`
	result := NewHTMLParser().ParseWithResult(context.Background(), "inline.html", []byte(input))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	if len(result.JSON) != 3 {
		t.Fatalf("items = %+v, want text/image/text", result.JSON)
	}
	if result.JSON[0]["text"] != "before" || result.JSON[0]["doc_type_kwd"] != "text" {
		t.Errorf("before item = %+v", result.JSON[0])
	}
	imageItem := result.JSON[1]
	if imageItem["doc_type_kwd"] != "image" || imageItem["image"] != htmlMediaDataURI || imageItem["text"] != "figure alt" || imageItem["media_order"] != 1 {
		t.Errorf("image item = %+v", imageItem)
	}
	if result.JSON[2]["text"] != "after" || result.JSON[2]["doc_type_kwd"] != "text" {
		t.Errorf("after item = %+v", result.JSON[2])
	}
}

func TestHTMLParser_EmitsTableCellImagesWithParentAndCellOrder(t *testing.T) {
	input := `<table><tr><td>A<img alt="first" src="` + htmlMediaDataURI + `"></td><td><img src="https://example.com/second.png"></td></tr></table>`
	result := NewHTMLParser().ParseWithResult(context.Background(), "table.html", []byte(input))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	if len(result.JSON) != 3 {
		t.Fatalf("items = %+v, want table and two cell image items", result.JSON)
	}
	if result.JSON[0]["doc_type_kwd"] != "table" || result.JSON[0]["source_table_id"] != "html-table-1" {
		t.Errorf("table item = %+v", result.JSON[0])
	}
	wantPayloads := []string{htmlMediaDataURI, "https://example.com/second.png"}
	for i, payload := range wantPayloads {
		item := result.JSON[i+1]
		if item["doc_type_kwd"] != "image" || item["image"] != payload || item["parent_table_id"] != "html-table-1" {
			t.Errorf("image item %d = %+v", i, item)
		}
		if item["row_index"] != 1 || item["column_index"] != i+1 || item["media_order"] != i+1 {
			t.Errorf("image item %d position = %+v", i, item)
		}
	}
}

func TestHTMLParser_EmitsRelativeImageSourcesAsResources(t *testing.T) {
	input := `<p>caption <img alt="missing" src="images/photo.png"></p>`
	result := NewHTMLParser().ParseWithResult(context.Background(), "relative.html", []byte(input))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	if len(result.JSON) != 2 {
		t.Fatalf("items = %+v, want text and image", result.JSON)
	}
	imageItem := result.JSON[1]
	if imageItem["doc_type_kwd"] != "image" || imageItem["image_src"] != "images/photo.png" || imageItem["text"] != "missing" {
		t.Fatalf("relative image item = %+v", imageItem)
	}
	for _, item := range result.JSON {
		if item["doc_type_kwd"] == "image" && item["image"] != nil {
			t.Fatalf("relative source should be kept distinct from image payload: %+v", item)
		}
	}
}

func TestHTMLParser_EmitsRelativeTableCellImagesWithParentOrder(t *testing.T) {
	input := `<table><tr><td><img alt="chart" src="assets/chart.png"></td></tr></table>`
	result := NewHTMLParser().ParseWithResult(context.Background(), "report.html", []byte(input))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	if len(result.JSON) != 2 {
		t.Fatalf("items = %+v, want table and image", result.JSON)
	}
	tableID := result.JSON[0]["source_table_id"]
	imageItem := result.JSON[1]
	if imageItem["doc_type_kwd"] != "image" || imageItem["image_src"] != "assets/chart.png" {
		t.Fatalf("relative table image item = %+v", imageItem)
	}
	if imageItem["parent_table_id"] != tableID || imageItem["row_index"] != 1 || imageItem["column_index"] != 1 || imageItem["media_order"] != 1 {
		t.Errorf("relative table image metadata = %+v", imageItem)
	}
}
