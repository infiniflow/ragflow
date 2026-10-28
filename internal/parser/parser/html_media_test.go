package parser

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"ragflow/internal/entity"
)

const htmlMediaDataURI = "data:image/png;base64,aGVsbG8="

func TestHTMLParser_EmitsMediaRefsInDocumentOrder(t *testing.T) {
	input := `<p>before <img alt="figure alt" src="` + htmlMediaDataURI + `"> after</p>`
	result := NewHTMLParser().ParseWithResult(context.Background(), "inline.html", []byte(input))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none for a valid inline image", result.Warnings)
	}
	if len(result.JSON) != 3 {
		t.Fatalf("items = %+v, want text/image/text", result.JSON)
	}
	if result.JSON[0].ToMap()["text"] != "before" || result.JSON[0].ToMap()["doc_type_kwd"] != "text" {
		t.Errorf("before item = %+v", result.JSON[0])
	}
	imageItem := result.JSON[1].ToMap()
	if imageItem["doc_type_kwd"] != "image" || imageItem["text"] != "figure alt" || imageItem["media_order"] != 1 {
		t.Errorf("image item = %+v", imageItem)
	}
	if _, ok := imageItem["image"]; ok {
		t.Errorf("image bytes retained: %+v", imageItem)
	}
	if entity.MediaRefFromAny(imageItem["media_ref"]) == nil {
		t.Errorf("image item without media_ref: %+v", imageItem)
	}
	if result.JSON[2].ToMap()["text"] != "after" || result.JSON[2].ToMap()["doc_type_kwd"] != "text" {
		t.Errorf("after item = %+v", result.JSON[2])
	}
}

func TestHTMLParser_EmitsTableCellImagesWithParentAndCellOrder(t *testing.T) {
	input := `<table><tr><td>A<img alt="first" src="` + htmlMediaDataURI + `"></td><td><img src="https://example.com/second.png"></td></tr></table>`
	result := NewHTMLParser().ParseWithResult(context.Background(), "table.html", []byte(input))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	if len(result.JSON) != 2 {
		t.Fatalf("items = %+v, want table and one inline image item", result.JSON)
	}
	if result.JSON[0].ToMap()["doc_type_kwd"] != "table" || result.JSON[0].ToMap()["source_table_id"] != "html-table-1" {
		t.Errorf("table item = %+v", result.JSON[0])
	}
	tableText := renderedText(result.JSON[0].ToMap())
	if strings.Contains(tableText, "data:image/") {
		t.Error("table text duplicates the separately emitted inline image payload")
	}
	if !strings.Contains(tableText, "A") {
		t.Errorf("table text lost the cell text: %q", tableText)
	}
	item := result.JSON[1].ToMap()
	if _, ok := item["image"]; ok {
		t.Errorf("image bytes retained: %+v", item)
	}
	if entity.MediaRefFromAny(item["media_ref"]) == nil {
		t.Errorf("image item without media_ref: %+v", item)
	}
	if item["doc_type_kwd"] != "image" || item["parent_table_id"] != "html-table-1" {
		t.Errorf("inline image item = %+v", item)
	}
	if item["row_index"] != 1 || item["column_index"] != 1 || item["media_order"] != 1 {
		t.Errorf("inline image position = %+v", item)
	}
}

func TestHTMLParser_RejectsOversizedInlineImagePayload(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString(make([]byte, maxEmbeddedImageBytes+1))
	input := "<p><img alt=\"large\" src=\"data:image/png;base64," + payload + "\"></p>"
	result := NewHTMLParser().ParseWithResult(context.Background(), "large.html", []byte(input))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	for _, item := range ItemsToMaps(result.JSON) {
		if item["doc_type_kwd"] == "image" {
			t.Fatal("oversized data URI emitted as an image item")
		}
	}
	if len(result.Warnings) == 0 {
		t.Fatal("oversized inline image should produce a parser warning")
	}
}

func TestHTMLParser_HasNoParserSideImageCountLimit(t *testing.T) {
	const count = maxEmbeddedMediaItems + 44
	input := "<p>" + strings.Repeat(`<img src="`+htmlMediaDataURI+`">`, count) + "</p>"
	result := NewHTMLParser().ParseWithResult(context.Background(), "many.html", []byte(input))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	images := 0
	for _, item := range result.JSON {
		if item.ToMap()["doc_type_kwd"] == "image" {
			images++
		}
	}
	if images != count {
		t.Fatalf("images = %d, want %d (no parser-side cap, matching PDF)", images, count)
	}
	if strings.Contains(strings.Join(result.Warnings, "\n"), "256-item") {
		t.Fatalf("unexpected parser-side image cap warning: %v", result.Warnings)
	}
}

func TestHTMLParser_EmitsMediaRef(t *testing.T) {
	p := NewHTMLParser()
	result := p.ParseWithResult(context.Background(), "ref.html", []byte(`<p><img alt="figure" src="`+htmlMediaDataURI+`"></p>`))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	item := result.JSON[0].ToMap()
	if _, ok := item["image"]; ok {
		t.Fatalf("image bytes retained: %+v", item)
	}
	ref := entity.MediaRefFromAny(item["media_ref"])
	if ref == nil {
		t.Fatalf("missing media_ref: %+v", item)
	}
	if ref.Format != "html" || ref.Kind != entity.MediaKindDataURI || ref.MIME != "image/png" || ref.Size != int64(len("hello")) {
		t.Fatalf("media_ref = %+v", ref)
	}
	sum := sha256.Sum256([]byte("hello"))
	if want := hex.EncodeToString(sum[:]); ref.SHA256 != want {
		t.Fatalf("sha256 = %s, want %s", ref.SHA256, want)
	}
}

func TestHTMLParser_OmitsImageBytes(t *testing.T) {
	p := NewHTMLParser()
	result := p.ParseWithResult(context.Background(), "ref-only.html", []byte(`<p><img src="`+htmlMediaDataURI+`"></p>`))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	item := result.JSON[0].ToMap()
	if _, ok := item["image"]; ok {
		t.Fatalf("parser retained image bytes: %+v", item)
	}
	if entity.MediaRefFromAny(item["media_ref"]) == nil {
		t.Fatalf("parser dropped media_ref: %+v", item)
	}
}

func TestScanHTMLMediaSpansMatchesParserRefs(t *testing.T) {
	input := `<p>before <img src="` + htmlMediaDataURI + `"></p>` +
		`<table><tr><td><img alt="cell" src="` + htmlMediaDataURI + `"></td></tr></table>`
	p := NewHTMLParser()
	result := p.ParseWithResult(context.Background(), "match.html", []byte(input))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	var want []string
	for _, item := range result.JSON {
		m := item.ToMap()
		if m["doc_type_kwd"] != "image" {
			continue
		}
		ref := entity.MediaRefFromAny(m["media_ref"])
		if ref == nil {
			t.Fatalf("image item without ref: %+v", m)
		}
		want = append(want, ref.SHA256)
	}
	spans, _ := scanHTMLMediaSpans(context.Background(), []byte(input))
	if len(spans) != len(want) {
		t.Fatalf("scanned %d images, parser emitted %d", len(spans), len(want))
	}
	for i := range want {
		if spans[i].Ref.SHA256 != want[i] {
			t.Fatalf("image %d digest = %s, want %s", i, spans[i].Ref.SHA256, want[i])
		}
		data, sum, err := decodeHTMLImagePayload([]byte(input)[spans[i].TagStart:spans[i].TagEnd])
		if err != nil {
			t.Fatalf("image %d decode: %v", i, err)
		}
		if string(data) != "hello" {
			t.Fatalf("image %d = %q, want hello", i, data)
		}
		if hex.EncodeToString(sum[:]) != want[i] {
			t.Fatalf("image %d decoded digest = %x", i, sum)
		}
	}
}

func TestScanHTMLMediaSpansHandlesCaseQuotesAndPadding(t *testing.T) {
	input := `<img src='DATA:IMAGE/PNG;BASE64,aGVsbG8'>`
	spans, _ := scanHTMLMediaSpans(context.Background(), []byte(input))
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	if spans[0].Ref.MIME != "image/png" {
		t.Errorf("mime = %q, want image/png", spans[0].Ref.MIME)
	}
	data, sum, err := decodeHTMLImagePayload([]byte(input)[spans[0].TagStart:spans[0].TagEnd])
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("data = %q, want hello", data)
	}
	if hex.EncodeToString(sum[:]) != spans[0].Ref.SHA256 {
		t.Errorf("scan digest and decode digest differ")
	}
}

// TestHTMLParser_RefModeKeepsResultIndependentOfImageBytes pins the core
// goal: the parse result no longer scales with image payloads.
func TestHTMLParser_ResultIndependentOfImageBytes(t *testing.T) {
	const imageCount = 64
	payload := base64.StdEncoding.EncodeToString(make([]byte, 64<<10))
	input := strings.Repeat(`<img src="data:image/png;base64,`+payload+`">`, imageCount)

	p := NewHTMLParser()
	result := p.ParseWithResult(context.Background(), "large.html", []byte(input))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	items := 0
	for _, item := range result.JSON {
		if item.ToMap()["doc_type_kwd"] == "image" {
			items++
		}
	}
	if items != imageCount {
		t.Fatalf("image items = %d, want %d", items, imageCount)
	}
	encoded, err := json.Marshal(result.JSON)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	// 64 refs are a few KB; the inline form would be over 4 MB.
	if len(encoded) > 256<<10 {
		t.Fatalf("ref-mode result is %d bytes; it must not scale with image payloads", len(encoded))
	}
}

func TestScanHTMLMediaSpansDecodesCharacterReferences(t *testing.T) {
	// aGVsbG8+ decodes to "hello>"; &#43; is the HTML-escaped '+'.
	input := `<img src="data:image/png;base64,aGVsbG8&#43;">`
	spans, _ := scanHTMLMediaSpans(context.Background(), []byte(input))
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	data, sum, err := decodeHTMLImagePayload([]byte(input)[spans[0].TagStart:spans[0].TagEnd])
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(data) != "hello>" {
		t.Fatalf("data = %q, want hello>", data)
	}
	if hex.EncodeToString(sum[:]) != spans[0].Ref.SHA256 {
		t.Fatal("scan and decode digests differ")
	}
}

func TestScanHTMLMediaSpansSkipsHeadersBeyondTheParserLimit(t *testing.T) {
	// The parser rejects headers longer than 128 bytes; the scan must not
	// look past that window for a comma, and must still find the next image.
	input := `<img src="data:image/png;` + strings.Repeat("x", 200) + `">` +
		`<img src="data:image/png;base64,aGVsbG8=">`
	spans, _ := scanHTMLMediaSpans(context.Background(), []byte(input))
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want the valid image only", len(spans))
	}
}
