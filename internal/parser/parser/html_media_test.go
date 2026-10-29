package parser

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/parser/media"
)

const htmlMediaDataURI = "data:image/png;base64,aGVsbG8="

type recordingMediaCache struct {
	puts     []recordingMediaPut
	putErr   error
	getData  map[string][]byte
	putCalls int
}

type recordingMediaPut struct {
	format string
	mime   string
	data   []byte
}

func (c *recordingMediaCache) Put(_ context.Context, format, mime string, data []byte) (entity.MediaRef, error) {
	c.putCalls++
	c.puts = append(c.puts, recordingMediaPut{format: format, mime: mime, data: append([]byte(nil), data...)})
	if c.putErr != nil {
		return entity.MediaRef{}, c.putErr
	}
	sum := sha256.Sum256(data)
	ref := entity.MediaRef{
		Format: format,
		Kind:   entity.MediaKindKV,
		Scope:  "run-1",
		SHA256: hex.EncodeToString(sum[:]),
		MIME:   mime,
		Size:   int64(len(data)),
	}
	if c.getData == nil {
		c.getData = map[string][]byte{}
	}
	c.getData[ref.SHA256] = append([]byte(nil), data...)
	return ref, nil
}

func (c *recordingMediaCache) Get(_ context.Context, ref entity.MediaRef) ([]byte, error) {
	data, ok := c.getData[ref.SHA256]
	if !ok {
		return nil, media.ErrMediaNotFound
	}
	return append([]byte(nil), data...), nil
}

func recordingMediaContext() (context.Context, *recordingMediaCache) {
	cache := &recordingMediaCache{}
	return media.WithCache(context.Background(), cache), cache
}

func TestHTMLParser_WritesImageToRunScopedMediaCache(t *testing.T) {
	cache := &recordingMediaCache{}
	ctx := media.WithCache(context.Background(), cache)
	result := NewHTMLParser().ParseWithResult(ctx, "cached.html", []byte(`<img alt="figure" src="`+htmlMediaDataURI+`">`))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	if cache.putCalls != 1 || len(cache.puts) != 1 {
		t.Fatalf("cache Put calls = %d/%d, want 1/1", cache.putCalls, len(cache.puts))
	}
	if cache.puts[0].format != "html" || cache.puts[0].mime != "image/png" || string(cache.puts[0].data) != "hello" {
		t.Fatalf("cache Put = %+v", cache.puts[0])
	}
	item := result.JSON[0].ToMap()
	ref := entity.MediaRefFromAny(item["media_ref"])
	if ref == nil || ref.Kind != entity.MediaKindKV || ref.Scope != "run-1" {
		t.Fatalf("media_ref = %+v", ref)
	}
	if _, ok := item["image"]; ok {
		t.Fatalf("image bytes retained: %+v", item)
	}
	encoded, err := json.Marshal(result.JSON)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"scope":"run-1"`) {
		t.Fatalf("serialized media_ref lost scope: %s", encoded)
	}
}

func TestHTMLParser_FailsWhenMediaCacheWriteFails(t *testing.T) {
	writeErr := errors.New("kvrocks unavailable")
	cache := &recordingMediaCache{putErr: writeErr}
	ctx := media.WithCache(context.Background(), cache)
	input := `<img src="` + htmlMediaDataURI + `"><img src="` + htmlMediaDataURI + `">`
	result := NewHTMLParser().ParseWithResult(ctx, "failed.html", []byte(input))
	if !errors.Is(result.Err, writeErr) {
		t.Fatalf("ParseWithResult error = %v, want %v", result.Err, writeErr)
	}
	if cache.putCalls != 1 {
		t.Fatalf("cache Put calls = %d, want traversal to stop after the first failure", cache.putCalls)
	}
}

func TestHTMLParser_EmitsMediaRefsInDocumentOrder(t *testing.T) {
	input := `<p>before <img alt="figure alt" src="` + htmlMediaDataURI + `"> after</p>`
	ctx, _ := recordingMediaContext()
	result := NewHTMLParser().ParseWithResult(ctx, "inline.html", []byte(input))
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
	ctx, _ := recordingMediaContext()
	result := NewHTMLParser().ParseWithResult(ctx, "table.html", []byte(input))
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

func TestHTMLParser_SkipsOversizedInlineImagePayload(t *testing.T) {
	t.Setenv("RAGFLOW_HTML_IMAGE_MAX_BYTES", "4")
	for name, oversized := range map[string]string{
		"inline": `<p><img src="` + htmlMediaDataURI + `"></p>`,
		"table":  `<table><tr><td><img src="` + htmlMediaDataURI + `"></td></tr></table>`,
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cache := recordingMediaContext()
			input := oversized + `<p>after <img src="data:image/png;base64,YQ=="></p>`
			result := NewHTMLParser().ParseWithResult(ctx, "large.html", []byte(input))
			if result.Err != nil {
				t.Fatalf("ParseWithResult: %v", result.Err)
			}
			if cache.putCalls != 1 || string(cache.puts[0].data) != "a" {
				t.Fatalf("cached images = %+v, want only the smaller image", cache.puts)
			}
			if !strings.Contains(renderedText(result.JSON[len(result.JSON)-2].ToMap()), "after") {
				t.Fatalf("text after oversized image lost: %+v", result.JSON)
			}
			if !strings.Contains(strings.Join(result.Warnings, " "), "invalid or oversized inline image") {
				t.Fatalf("missing skipped-image warning: %v", result.Warnings)
			}
		})
	}
}

func TestHTMLParser_HasNoParserSideImageCountLimit(t *testing.T) {
	const count = maxEmbeddedMediaItems + 44
	input := "<p>" + strings.Repeat(`<img src="`+htmlMediaDataURI+`">`, count) + "</p>"
	ctx, _ := recordingMediaContext()
	result := NewHTMLParser().ParseWithResult(ctx, "many.html", []byte(input))
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
	ctx, _ := recordingMediaContext()
	result := p.ParseWithResult(ctx, "ref.html", []byte(`<p><img alt="figure" src="`+htmlMediaDataURI+`"></p>`))
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
	if ref.Format != "html" || ref.Kind != entity.MediaKindKV || ref.Scope != "run-1" || ref.MIME != "image/png" || ref.Size != int64(len("hello")) {
		t.Fatalf("media_ref = %+v", ref)
	}
	sum := sha256.Sum256([]byte("hello"))
	if want := hex.EncodeToString(sum[:]); ref.SHA256 != want {
		t.Fatalf("sha256 = %s, want %s", ref.SHA256, want)
	}
}

func TestParserMediaRefsResolveFromRunCache(t *testing.T) {
	input := `<p>before <img src="` + htmlMediaDataURI + `"></p>` +
		`<table><tr><td><img alt="cell" src="` + htmlMediaDataURI + `"></td></tr></table>`
	ctx, cache := recordingMediaContext()
	result := NewHTMLParser().ParseWithResult(ctx, "match.html", []byte(input))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
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
		data, err := cache.Get(ctx, *ref)
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
	ctx, _ := recordingMediaContext()
	result := p.ParseWithResult(ctx, "large.html", []byte(input))
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

// TestHTMLMediaCacheMatchesParserAttributeSemantics pins that the bytes sent to
// Kvrocks come from the parser's decoded attribute value: entities in the media
// type, an encoded comma, a semicolon-less numeric reference, and an unquoted
// value ending in "/>".
func TestHTMLMediaCacheMatchesParserAttributeSemantics(t *testing.T) {
	cases := map[string]string{
		"entity in header":        `<img src="data:image&#47;png;base64,aGVsbG8=">`,
		"entity comma":            `<img src="data:image/png;base64&#44;aGVsbG8=">`,
		"semicolonless entity":    `<img src="data:image/png;base64,aGVsbG8&#43">`,
		"unquoted trailing slash": `<img src=data:image/png;base64,aGVsbG8/>`,
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			p := NewHTMLParser()
			ctx, cache := recordingMediaContext()
			result := p.ParseWithResult(ctx, "case.html", []byte(input))
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
			data, err := cache.Get(ctx, *ref)
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

// TestHTMLMediaCacheIgnoresScriptDataURIs pins that image-like text inside a
// script is not written to the cache.
func TestHTMLMediaCacheIgnoresScriptDataURIs(t *testing.T) {
	img := `<img src="data:image/png;base64,aGVsbG8=">`
	input := "<script>" + strings.Repeat(img, 5000) + "</script>" + img

	p := NewHTMLParser()
	ctx, cache := recordingMediaContext()
	result := p.ParseWithResult(ctx, "script.html", []byte(input))
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
	if _, err := cache.Get(ctx, *ref); err != nil {
		t.Fatalf("resolve: %v", err)
	}
}

// mediaRefFromHTML returns the first image reference the HTML parser emits.
func mediaRefFromHTML(t *testing.T, input string, cache *recordingMediaCache) *entity.MediaRef {
	t.Helper()
	p := NewHTMLParser()
	result := p.ParseWithResult(media.WithCache(context.Background(), cache), "case.html", []byte(input))
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

func resolveMediaInput(t *testing.T, ref entity.MediaRef, cache *recordingMediaCache) []byte {
	t.Helper()
	data, err := cache.Get(context.Background(), ref)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return data
}

func TestMediaResolverHandlesSelfClosingImg(t *testing.T) {
	input := `<img src="data:image/png;base64,aGVsbG8=" />`
	cache := &recordingMediaCache{}
	ref := mediaRefFromHTML(t, input, cache)
	if data := resolveMediaInput(t, *ref, cache); string(data) != "hello" {
		t.Fatalf("data = %q, want hello", data)
	}
}

func TestHTMLMediaCacheUsesParserDOM(t *testing.T) {
	// In the HTML tree builder <svg><title> is foreign content, so the <img>
	// becomes an element. The parser writes the image from its own DOM walk;
	// there is no second tokenizer or fallback scan.
	input := `<svg><title><img src="data:image/png;base64,aGVsbG8="></title></svg>`
	cache := &recordingMediaCache{}
	ref := mediaRefFromHTML(t, input, cache)
	if data := resolveMediaInput(t, *ref, cache); string(data) != "hello" {
		t.Fatalf("data = %q, want hello", data)
	}
}
