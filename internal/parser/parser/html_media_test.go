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
	"ragflow/internal/parser/media"
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

func TestParserMediaRefsResolveFromSource(t *testing.T) {
	input := `<p>before <img src="` + htmlMediaDataURI + `"></p>` +
		`<table><tr><td><img alt="cell" src="` + htmlMediaDataURI + `"></td></tr></table>`
	result := NewHTMLParser().ParseWithResult(context.Background(), "match.html", []byte(input))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	resolver := media.NewResolver(media.MediaSourceFunc(func(context.Context) ([]byte, error) {
		return []byte(input), nil
	}))
	images := 0
	for _, item := range result.JSON {
		m := item.ToMap()
		if m["doc_type_kwd"] != DocTypeImage {
			continue
		}
		images++
		ref := entity.MediaRefFromAny(m["media_ref"])
		if ref == nil {
			t.Fatalf("image item without ref: %+v", m)
		}
		data, err := resolver.Resolve(context.Background(), *ref)
		if err != nil {
			t.Fatalf("resolve image %d: %v", images, err)
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != ref.SHA256 {
			t.Fatalf("image %d digest = %s, want %s", images, got, ref.SHA256)
		}
	}
	if images != 2 {
		t.Fatalf("image items = %d, want 2", images)
	}
}

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

// TestMediaResolverMatchesParserAttributeSemantics pins the cases where a
// byte-level scanner used to disagree with the HTML parser: entities in the
// media type, an encoded comma, a semicolon-less numeric reference, and an
// unquoted value ending in "/>".
func TestMediaResolverMatchesParserAttributeSemantics(t *testing.T) {
	cases := map[string]string{
		"entity in header":        `<img src="data:image&#47;png;base64,aGVsbG8=">`,
		"entity comma":            `<img src="data:image/png;base64&#44;aGVsbG8=">`,
		"semicolonless entity":    `<img src="data:image/png;base64,aGVsbG8&#43">`,
		"unquoted trailing slash": `<img src=data:image/png;base64,aGVsbG8/>`,
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			p := NewHTMLParser()
			result := p.ParseWithResult(context.Background(), "case.html", []byte(input))
			if result.Err != nil {
				t.Fatal(result.Err)
			}
			var ref *entity.MediaRef
			for _, item := range result.JSON {
				m := item.ToMap()
				if m["doc_type_kwd"] != DocTypeImage {
					continue
				}
				ref = entity.MediaRefFromAny(m["media_ref"])
			}
			if ref == nil {
				t.Fatalf("parser emitted no media_ref: %+v", result.JSON)
			}
			data, err := media.NewResolver(media.MediaSourceFunc(func(context.Context) ([]byte, error) {
				return []byte(input), nil
			})).Resolve(context.Background(), *ref)
			if err != nil {
				t.Fatalf("resolver lost the parser's image: %v", err)
			}
			sum := sha256.Sum256(data)
			if got := hex.EncodeToString(sum[:]); got != ref.SHA256 {
				t.Fatalf("digest = %s, want %s", got, ref.SHA256)
			}
		})
	}
}

// TestMediaResolverIgnoresScriptDataURIs pins that image-like text inside a
// script cannot exhaust the span index and hide a real image.
func TestMediaResolverIgnoresScriptDataURIs(t *testing.T) {
	img := `<img src="data:image/png;base64,aGVsbG8=">`
	input := "<script>" + strings.Repeat(img, 5000) + "</script>" + img

	p := NewHTMLParser()
	result := p.ParseWithResult(context.Background(), "script.html", []byte(input))
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	var ref *entity.MediaRef
	for _, item := range result.JSON {
		m := item.ToMap()
		if m["doc_type_kwd"] == DocTypeImage {
			ref = entity.MediaRefFromAny(m["media_ref"])
		}
	}
	if ref == nil {
		t.Fatal("parser emitted no media_ref")
	}
	if _, err := media.NewResolver(media.MediaSourceFunc(func(context.Context) ([]byte, error) {
		return []byte(input), nil
	})).Resolve(context.Background(), *ref); err != nil {
		t.Fatalf("resolve: %v", err)
	}
}

// mediaRefFromHTML returns the first image reference the HTML parser emits.
func mediaRefFromHTML(t *testing.T, input string) *entity.MediaRef {
	t.Helper()
	p := NewHTMLParser()
	result := p.ParseWithResult(context.Background(), "case.html", []byte(input))
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	var ref *entity.MediaRef
	for _, item := range result.JSON {
		m := item.ToMap()
		if m["doc_type_kwd"] == DocTypeImage {
			ref = entity.MediaRefFromAny(m["media_ref"])
		}
	}
	if ref == nil {
		t.Fatalf("parser emitted no media_ref: %+v", result.JSON)
	}
	return ref
}

func resolveMediaInput(t *testing.T, input string, ref entity.MediaRef) []byte {
	t.Helper()
	data, err := media.NewResolver(media.MediaSourceFunc(func(context.Context) ([]byte, error) {
		return []byte(input), nil
	})).Resolve(context.Background(), ref)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return data
}

func TestMediaResolverHandlesSelfClosingImg(t *testing.T) {
	input := `<img src="data:image/png;base64,aGVsbG8=" />`
	ref := mediaRefFromHTML(t, input)
	if data := resolveMediaInput(t, input, *ref); string(data) != "hello" {
		t.Fatalf("data = %q, want hello", data)
	}
}

func TestMediaResolverFallsBackToTheParserDOM(t *testing.T) {
	// In the HTML tree builder <svg><title> is foreign content, so the <img>
	// becomes an element. The streaming tokenizer treats <title> as RCDATA and
	// misses it; the DOM fallback must recover the image.
	input := `<svg><title><img src="data:image/png;base64,aGVsbG8="></title></svg>`
	ref := mediaRefFromHTML(t, input)
	if data := resolveMediaInput(t, input, *ref); string(data) != "hello" {
		t.Fatalf("data = %q, want hello", data)
	}
}
