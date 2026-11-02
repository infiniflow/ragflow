//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package parser

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"

	"ragflow/internal/entity"
	"ragflow/internal/parser/media"
)

// epubTestChapter is one spine item of a test EPUB. A missing chapter is
// listed in the manifest and spine but has no ZIP entry; a damaged one is
// stored as a Deflate entry whose data is not a valid Deflate stream.
type epubTestChapter struct {
	name    string
	entry   string // ZIP entry under OEBPS/ when it differs from the manifest href
	content string
	missing bool
	damaged bool
}

// epubTestImage is one manifest image of a test EPUB.
type epubTestImage struct {
	name    string // manifest href, also the ZIP entry under OEBPS/ unless entry is set
	entry   string // ZIP entry under OEBPS/ when it differs from the manifest href
	mime    string // manifest media type; defaults to image/png
	content string
	missing bool // listed in the manifest but absent from the ZIP
	damaged bool // raw Deflate entry whose data is not a valid Deflate stream
}

func epubTestHTML(body string) string {
	return `<?xml version="1.0" encoding="utf-8"?><html xmlns="http://www.w3.org/1999/xhtml"><head><title>Test</title></head><body>` + body + `</body></html>`
}

func buildTestEPUB(t *testing.T, chapters []epubTestChapter) []byte {
	t.Helper()
	return buildTestEPUBWithImages(t, chapters, nil)
}

func buildTestEPUBWithImages(t *testing.T, chapters []epubTestChapter, images []epubTestImage) []byte {
	t.Helper()
	var manifest, spine strings.Builder
	for i, ch := range chapters {
		fmt.Fprintf(&manifest, `<item id="c%d" href="%s" media-type="application/xhtml+xml"/>`, i, ch.name)
		fmt.Fprintf(&spine, `<itemref idref="c%d"/>`, i)
	}
	for i, img := range images {
		href := img.name
		mime := img.mime
		if mime == "" {
			mime = "image/png"
		}
		fmt.Fprintf(&manifest, `<item id="m%d" href="%s" media-type="%s"/>`, i, href, mime)
	}
	containerXML := `<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`
	contentOPF := `<?xml version="1.0" encoding="utf-8"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0"><manifest>` + manifest.String() + `</manifest><spine>` + spine.String() + `</spine></package>`

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	write := func(name, content string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	writeRaw := func(name string) {
		w, err := zw.CreateRaw(&zip.FileHeader{Name: "OEBPS/" + name, Method: zip.Deflate, CompressedSize64: 4, UncompressedSize64: 64})
		if err != nil {
			t.Fatalf("create raw %s: %v", name, err)
		}
		if _, err := w.Write([]byte{0xff, 0xff, 0xff, 0xff}); err != nil {
			t.Fatalf("write raw %s: %v", name, err)
		}
	}
	write("META-INF/container.xml", containerXML)
	write("OEBPS/content.opf", contentOPF)
	for _, ch := range chapters {
		switch {
		case ch.missing:
		case ch.damaged:
			writeRaw(ch.name)
		case ch.entry != "":
			write("OEBPS/"+ch.entry, ch.content)
		default:
			write("OEBPS/"+ch.name, ch.content)
		}
	}
	for _, img := range images {
		entry := img.entry
		if entry == "" {
			entry = img.name
		}
		switch {
		case img.missing:
		case img.damaged:
			writeRaw(entry)
		default:
			write("OEBPS/"+entry, img.content)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func parseTestEPUB(t *testing.T, chapters []epubTestChapter) ParseResult {
	t.Helper()
	return parseTestEPUBContext(t, context.Background(), chapters, nil)
}

func parseTestEPUBContext(t *testing.T, ctx context.Context, chapters []epubTestChapter, images []epubTestImage) ParseResult {
	t.Helper()
	return NewEPUBParser().ParseWithResult(ctx, "book.epub", buildTestEPUBWithImages(t, chapters, images))
}

// The run media budget bounds how many distinct ZIP images one EPUB extracts.
// Distinct sources are required to reach the cap; repeated references share
// one source (see TestEPUBParserRepeatedZipImageReadsAndWritesOnce).
func TestEPUBParserCapsZipImageExtractionAcrossChapters(t *testing.T) {
	t.Setenv("RAGFLOW_MEDIA_MAX_IMAGES_PER_RUN", "256")
	const limit = 256
	var first, second strings.Builder
	images := make([]epubTestImage, 0, limit)
	for i := 0; i < limit; i++ {
		name := fmt.Sprintf("Images/plate-%d.png", i)
		if i < limit/2 {
			first.WriteString(`<img src="` + name + `">`)
		} else {
			second.WriteString(`<img src="` + name + `">`)
		}
		images = append(images, epubTestImage{name: name, content: fmt.Sprintf("plate-%d", i)})
	}
	// One distinct source past the cap, which must be omitted.
	const overflowName = "Images/overflow.png"
	second.WriteString(`<img src="` + overflowName + `">`)
	images = append(images, epubTestImage{name: overflowName, content: "overflow"})

	ctx, cache := recordingMediaContext()
	res := parseTestEPUBContext(t, ctx, []epubTestChapter{
		{name: "one.xhtml", content: epubTestHTML(first.String())},
		{name: "two.xhtml", content: epubTestHTML(second.String() + "<p>After images.</p>")},
	}, images)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	items := 0
	for _, item := range ItemsToMaps(res.JSON) {
		if item["doc_type_kwd"] == DocTypeImage {
			items++
		}
	}
	if items != limit || !strings.Contains(epubTestText(res), "After images.") {
		t.Fatalf("images=%d text=%q", items, epubTestText(res))
	}
	if cache.putCalls != limit {
		t.Fatalf("cache writes = %d, want %d", cache.putCalls, limit)
	}
	if !strings.Contains(strings.Join(res.Warnings, "\n"), "256-item run limit") {
		t.Fatalf("missing run image limit warning: %v", res.Warnings)
	}
}

func epubTestText(res ParseResult) string {
	var parts []string
	for _, item := range ItemsToMaps(res.JSON) {
		text, _ := item["text"].(string)
		parts = append(parts, text)
	}
	return strings.Join(parts, "\n")
}

func TestEPUBParser_IntactBookReadsEveryChapter(t *testing.T) {
	res := parseTestEPUB(t, []epubTestChapter{
		{name: "ch1.xhtml", content: epubTestHTML("<p>ALPHA chapter</p>")},
		{name: "ch2.xhtml", content: epubTestHTML("<p>BRAVO chapter</p>")},
	})
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	text := epubTestText(res)
	for _, want := range []string{"ALPHA", "BRAVO"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in %q", want, text)
		}
	}
}

func TestEPUBParser_OneUnreadableChapterKeepsTheRest(t *testing.T) {
	res := parseTestEPUB(t, []epubTestChapter{
		{name: "ch1.xhtml", content: epubTestHTML("<p>ALPHA chapter</p>")},
		{name: "ch2.xhtml", damaged: true},
		{name: "ch3.xhtml", missing: true},
	})
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if text := epubTestText(res); !strings.Contains(text, "ALPHA") {
		t.Errorf("readable chapter lost: %q", text)
	}
}

func TestEPUBParser_NoReadableChapterIsAnError(t *testing.T) {
	cases := []struct {
		name     string
		chapters []epubTestChapter
		want     string
	}{
		{
			name:     "every spine item missing",
			chapters: []epubTestChapter{{name: "ch1.xhtml", missing: true}, {name: "ch2.xhtml", missing: true}},
			want:     "epub: no readable content: 2 of 2 content items could not be read (ch1.xhtml: ",
		},
		{
			name:     "every spine item damaged",
			chapters: []epubTestChapter{{name: "ch1.xhtml", damaged: true}, {name: "ch2.xhtml", damaged: true}},
			want:     "epub: no readable content: 2 of 2 content items could not be read (ch1.xhtml: ",
		},
		{
			// An empty item is neither read nor failed, so nothing readable is left.
			name:     "empty chapter next to an unreadable one",
			chapters: []epubTestChapter{{name: "ch1.xhtml", content: ""}, {name: "ch2.xhtml", damaged: true}},
			want:     "epub: no readable content: 1 of 2 content items could not be read (ch2.xhtml: ",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := parseTestEPUB(t, tc.chapters)
			if res.Err == nil {
				t.Fatalf("want an error, got items %v", res.JSON)
			}
			if !strings.Contains(res.Err.Error(), tc.want) {
				t.Errorf("error = %q, want it to contain %q", res.Err, tc.want)
			}
		})
	}
}

func TestEPUBParser_ImageOnlyChapterCountsAsRead(t *testing.T) {
	// The image-only chapter was read and has no text; the book is not unreadable.
	// The chapter has no body text.
	imageOnly := `<?xml version="1.0" encoding="utf-8"?><html xmlns="http://www.w3.org/1999/xhtml"><body><img src="plate.png" alt=""/></body></html>`
	ctx, _ := recordingMediaContext()
	res := parseTestEPUBContext(t, ctx, []epubTestChapter{
		{name: "ch1.xhtml", content: imageOnly},
		{name: "ch2.xhtml", damaged: true},
	}, []epubTestImage{{name: "plate.png", content: "plate-bytes"}})
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if text := epubTestText(res); text != "" {
		t.Fatalf("image-only chapter produced text %q", text)
	}
}

func TestEPUBParser_OnlyEmptyChaptersIsNotAnError(t *testing.T) {
	res := parseTestEPUB(t, []epubTestChapter{{name: "ch1.xhtml", content: ""}})
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
}

// Manifest hrefs are URLs, so non-ASCII and space file names are
// percent-encoded and may carry a fragment.
func TestEPUBParser_ResolvesManifestHrefsAsURLs(t *testing.T) {
	res := parseTestEPUB(t, []epubTestChapter{
		{name: "Text/%E7%AC%AC%E4%B8%80%E7%AB%A0.xhtml", entry: "Text/第一章.xhtml", content: epubTestHTML("<p>ALPHA chapter</p>")},
		{name: "Text/chapter%202.xhtml#start", entry: "Text/chapter 2.xhtml", content: epubTestHTML("<p>BRAVO chapter</p>")},
	})
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	text := epubTestText(res)
	for _, want := range []string{"ALPHA", "BRAVO"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in %q", want, text)
		}
	}
}

// Chapter title text and block boundaries retain their text representation.
func TestEPUBParser_SeparatesBlockElements(t *testing.T) {
	res := parseTestEPUB(t, []epubTestChapter{
		{name: "ch1.xhtml", content: epubTestHTML(
			"<section>chapter text</section><dl><dt>Term</dt><dd>Definition</dd></dl>" +
				"<figure><figcaption>Figure 1</figcaption></figure><main>Afterword</main>Epilogue")},
	})
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if got, want := epubTestText(res), "Test chapter text Term Definition Figure 1 Afterword Epilogue"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

// A self-closing raw-text element is valid XHTML but an unclosed start tag
// under HTML5 rules, which would swallow the rest of the chapter.
func TestEPUBParser_SelfClosingRawTextKeepsFollowingContent(t *testing.T) {
	res := parseTestEPUB(t, []epubTestChapter{
		{name: "ch1.xhtml", content: epubTestHTML(
			`<script src="chapter.js"/><style type="text/css"/><p>After self-closing.</p>` +
				`<table><tr><td>Owner</td><td>RET</td></tr></table>`)},
	})
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if text := epubTestText(res); !strings.Contains(text, "After self-closing.") {
		t.Fatalf("body after self-closing raw-text element lost: %q", text)
	}
	tables := 0
	for _, item := range res.JSON {
		if item.ToMap()["doc_type_kwd"] == "table" {
			tables++
		}
	}
	if tables != 1 {
		t.Fatalf("table after self-closing raw-text element lost: %#v", ItemsToMaps(res.JSON))
	}
}

// XHTML has no &nbsp; and EPUB tools often write numeric character
// references; only a fixed list of named entities was decoded, so
// "don&#8217;t" reached the index as written.
func TestEPUBParser_DecodesCharacterReferences(t *testing.T) {
	res := parseTestEPUB(t, []epubTestChapter{
		{name: "ch1.xhtml", content: epubTestHTML(
			"<p>don&#8217;t&#160;stop &#x2014; caf&eacute; &frac12; cup &hellip; &amp; &#38; &bogus;</p>")},
	})
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if text, want := epubTestText(res), "don’t stop — café ½ cup … & & &bogus;"; !strings.Contains(text, want) {
		t.Errorf("text = %q, want it to contain %q", text, want)
	}
}

func TestEPUBParserPreservesChapterTextUnits(t *testing.T) {
	paragraph := strings.Repeat("chapter prose ", 600)
	res := parseTestEPUB(t, []epubTestChapter{
		{name: "one.xhtml", content: epubTestHTML("<p>" + paragraph + "</p><p>End one.</p>")},
		{name: "two.xhtml", content: epubTestHTML("<p>Second &ldquo;chapter&rdquo;.</p><p>\u3000End&#x00A0;two.</p>")},
	})
	if res.Err != nil || len(res.JSON) != 2 {
		t.Fatalf("expected one text item per chapter: items=%d err=%v", len(res.JSON), res.Err)
	}
	if got, want := res.JSON[0].ToMap()["text"], "Test "+paragraph+"End one."; got != want {
		t.Fatal("first chapter text changed")
	}
	if got, want := res.JSON[1].ToMap()["text"], "Test Second \"chapter\". \u3000End two."; got != want {
		t.Fatalf("second chapter text=%q, want %q", got, want)
	}
}

func TestEPUBParser_GB2312(t *testing.T) {
	// Create an in-memory EPUB zip with GB2312-encoded chapter XHTML
	chapterContent := `<?xml version="1.0" encoding="GB2312"?>
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.1//EN" "http://www.w3.org/TR/xhtml11/DTD/xhtml11.dtd">
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>第一章</title></head>
<body><h1>第一章 破晓</h1><p>这是一段包含中文的EPUB正文内容。</p></body>
</html>`
	chapterGBK, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(chapterContent))
	if err != nil {
		t.Fatalf("GBK encode: %v", err)
	}

	containerXML := `<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`
	contentOPF := `<?xml version="1.0" encoding="utf-8"?><package xmlns="http://www.idpf.org/2007/opf" unique-identifier="BookID" version="2.0"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>测试</dc:title></metadata><manifest><item id="c1" href="c1.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="c1"/></spine></package>`

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	w, _ := zw.Create("META-INF/container.xml")
	w.Write([]byte(containerXML))
	w, _ = zw.Create("OEBPS/content.opf")
	w.Write([]byte(contentOPF))
	w, _ = zw.Create("OEBPS/c1.xhtml")
	w.Write(chapterGBK)
	zw.Close()

	p := NewEPUBParser()
	res := p.ParseWithResult(context.Background(), "book.epub", buf.Bytes())
	if res.Err != nil {
		t.Fatalf("EPUBParser.ParseWithResult failed: %v", res.Err)
	}
	if len(res.JSON) == 0 {
		t.Fatal("want non-empty JSON items from EPUB")
	}
	text := epubTestText(res)
	if strings.ContainsRune(text, '\ufffd') {
		t.Errorf("EPUB text contains replacement runes: %s", text)
	}
	for _, want := range []string{"第一章 破晓", "这是一段包含中文的EPUB正文内容。"} {
		if !strings.Contains(text, want) {
			t.Errorf("EPUB text missing %q; got: %s", want, text)
		}
	}
	if enc, _ := res.File["encoding"].(string); enc != "gb2312" && enc != "gbk" && enc != "gb18030" {
		t.Errorf("EPUB File.encoding = %q, want gb2312/gbk/gb18030", enc)
	}
}

func epubTestImageItems(t *testing.T, res ParseResult) []map[string]any {
	t.Helper()
	var images []map[string]any
	for _, item := range ItemsToMaps(res.JSON) {
		if item["doc_type_kwd"] == DocTypeImage {
			images = append(images, item)
		}
	}
	return images
}

func epubTestWarningText(res ParseResult) string {
	return strings.Join(res.Warnings, "\n")
}

func TestHTMLWalkStateResolveMediaPrecedesResolveImageAndDataURI(t *testing.T) {
	zipRef := &entity.MediaRef{Format: "epub", Kind: entity.MediaKindKV, SHA256: strings.Repeat("a", sha256.Size*2)}
	ctx, cache := recordingMediaContext()
	state := &htmlWalkState{
		refOnly:     true,
		ctx:         ctx,
		mediaCache:  cache,
		budget:      media.NewBudget(),
		mediaFormat: "epub",
		resolveMedia: func(src string) (*entity.MediaRef, bool) {
			if src == "zip.png" || src == htmlMediaDataURI {
				return zipRef, true
			}
			return nil, false
		},
		resolveImage: func(src string) (string, bool) { return "inline:" + src, true },
	}

	if src, ref, ok := state.imageSource("zip.png"); !ok || ref != zipRef || src != "" {
		t.Fatalf("resolveMedia did not win over resolveImage: src=%q ref=%+v ok=%v", src, ref, ok)
	}
	if src, ref, ok := state.imageSource(htmlMediaDataURI); !ok || ref != zipRef || src != "" {
		t.Fatalf("resolveMedia did not win over the data URI branch: src=%q ref=%+v ok=%v", src, ref, ok)
	}
	if cache.putCalls != 0 {
		t.Fatalf("cache was written although resolveMedia handled the source: %+v", cache.puts)
	}
	if src, ref, ok := state.imageSource("remote.png"); !ok || ref != nil || src != "inline:remote.png" {
		t.Fatalf("declined source did not fall through to resolveImage: src=%q ref=%+v ok=%v", src, ref, ok)
	}
}

func TestHTMLWalkStateDataURIWriteUsesMediaFormat(t *testing.T) {
	for _, tc := range []struct {
		name   string
		format string
		want   string
	}{
		{name: "default html", format: "", want: "html"},
		{name: "explicit epub", format: "epub", want: "epub"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cache := recordingMediaContext()
			state := &htmlWalkState{refOnly: true, ctx: ctx, mediaCache: cache, budget: media.NewBudget(), mediaFormat: tc.format}
			_, ref, ok := state.imageSource(htmlMediaDataURI)
			if !ok || ref == nil {
				t.Fatalf("data URI image was not cached: ok=%v ref=%+v", ok, ref)
			}
			if len(cache.puts) != 1 || cache.puts[0].format != tc.want || ref.Format != tc.want {
				t.Fatalf("cache puts = %+v, ref = %+v, want format %q", cache.puts, ref, tc.want)
			}
		})
	}
}

// HTML does not install a media resolver: a non-data image source stays
// unresolved and is neither cached nor reported as an EPUB-style skip.
func TestHTMLParserIgnoresNonDataImageSourcesWithoutResolveMedia(t *testing.T) {
	ctx, cache := recordingMediaContext()
	result := NewHTMLParser().ParseWithResult(ctx, "plain.html", []byte(`<p>before</p><img src="figure.png" alt="figure"><p>after</p>`))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	if cache.putCalls != 0 {
		t.Fatalf("cache writes = %+v, want none", cache.puts)
	}
	if images := epubTestImageItems(t, result); len(images) != 0 {
		t.Fatalf("image items = %#v, want none", images)
	}
	if strings.Contains(epubTestWarningText(result), "unresolved") {
		t.Fatalf("HTML reported an unresolved image warning: %v", result.Warnings)
	}
}

func TestEPUBParserCachesZipImageBytes(t *testing.T) {
	ctx, cache := recordingMediaContext()
	res := parseTestEPUBContext(t, ctx, []epubTestChapter{{
		name:    "Text/ch1.xhtml",
		content: epubTestHTML(`<p>Before.</p><img src="../Images/cover.png" alt="cover art"><p>After.</p>`),
	}}, []epubTestImage{{name: "Images/cover.png", content: "cover-bytes"}})
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", res.Warnings)
	}
	if len(cache.puts) != 1 {
		t.Fatalf("cache puts = %+v, want one", cache.puts)
	}
	put := cache.puts[0]
	if put.format != "epub" || put.mime != "image/png" || string(put.data) != "cover-bytes" {
		t.Fatalf("cache put = %+v", put)
	}
	items := ItemsToMaps(res.JSON)
	if len(items) != 3 {
		t.Fatalf("items = %#v, want text/image/text", items)
	}
	if items[0]["text"] != "Test Before." || items[0]["doc_type_kwd"] != DocTypeText {
		t.Fatalf("before item = %#v", items[0])
	}
	image := items[1]
	if image["doc_type_kwd"] != DocTypeImage || image["text"] != "cover art" || image["media_order"] != 1 {
		t.Fatalf("image item = %#v", image)
	}
	if _, ok := image["image"]; ok {
		t.Fatalf("inline image retained: %#v", image)
	}
	ref := entity.MediaRefFromAny(image["media_ref"])
	if ref == nil || ref.Format != "epub" || ref.MIME != "image/png" || ref.Size != int64(len("cover-bytes")) {
		t.Fatalf("media_ref = %+v", ref)
	}
	sum := sha256.Sum256([]byte("cover-bytes"))
	if want := hex.EncodeToString(sum[:]); ref.SHA256 != want {
		t.Fatalf("sha256 = %s, want %s", ref.SHA256, want)
	}
	if items[2]["text"] != "After." || items[2]["doc_type_kwd"] != DocTypeText {
		t.Fatalf("after item = %#v", items[2])
	}
}

func TestEPUBParserResolvesPercentEncodedAndFragmentImageHrefs(t *testing.T) {
	ctx, cache := recordingMediaContext()
	res := parseTestEPUBContext(t, ctx, []epubTestChapter{{
		name:    "Text/ch1.xhtml",
		content: epubTestHTML(`<img src="../Images/cover%20art.png#frag" alt="cover">`),
	}}, []epubTestImage{{name: "Images/cover%20art.png", entry: "Images/cover art.png", content: "art-bytes"}})
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(cache.puts) != 1 || string(cache.puts[0].data) != "art-bytes" {
		t.Fatalf("cache puts = %+v, want the decoded ZIP entry", cache.puts)
	}
	if images := epubTestImageItems(t, res); len(images) != 1 {
		t.Fatalf("image items = %#v, want one", images)
	}
}

func TestEPUBParserSkipsImagesOutsideTheZipRoot(t *testing.T) {
	sources := map[string]string{
		"root escape":       "../../../escape.png",
		"absolute path":     "/Images/cover.png",
		"remote URL":        "https://example.com/cover.png",
		"protocol relative": "//example.com/cover.png",
	}
	for name, src := range sources {
		t.Run(name, func(t *testing.T) {
			ctx, cache := recordingMediaContext()
			res := parseTestEPUBContext(t, ctx, []epubTestChapter{{
				name:    "Text/ch1.xhtml",
				content: epubTestHTML(`<img src="` + src + `" alt="">`),
			}}, []epubTestImage{{name: "Images/cover.png", content: "cover-bytes"}})
			if res.Err != nil {
				t.Fatalf("ParseWithResult: %v", res.Err)
			}
			if cache.putCalls != 0 {
				t.Fatalf("cache writes = %+v, want none", cache.puts)
			}
			if images := epubTestImageItems(t, res); len(images) != 0 {
				t.Fatalf("image items = %#v, want none", images)
			}
			if !strings.Contains(epubTestWarningText(res), "unresolved image reference") {
				t.Fatalf("missing unresolved-image warning: %v", res.Warnings)
			}
		})
	}
}

func TestEPUBParserSkipsUnreadableImageEntries(t *testing.T) {
	cases := map[string]epubTestImage{
		"missing entry": {name: "Images/cover.png", missing: true},
		"damaged entry": {name: "Images/cover.png", damaged: true},
		"empty entry":   {name: "Images/cover.png"},
	}
	for name, image := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, cache := recordingMediaContext()
			res := parseTestEPUBContext(t, ctx, []epubTestChapter{{
				name:    "ch1.xhtml",
				content: epubTestHTML(`<img src="Images/cover.png" alt="cover">`),
			}}, []epubTestImage{image})
			if res.Err != nil {
				t.Fatalf("ParseWithResult: %v", res.Err)
			}
			if cache.putCalls != 0 {
				t.Fatalf("cache writes = %+v, want none", cache.puts)
			}
			if images := epubTestImageItems(t, res); len(images) != 0 {
				t.Fatalf("image items = %#v, want none", images)
			}
			if !strings.Contains(epubTestWarningText(res), "unresolved image reference") {
				t.Fatalf("missing unresolved-image warning: %v", res.Warnings)
			}
		})
	}
}

// A manifest image missing from the ZIP is skipped without reading bytes, so
// it must not consume an image attempt and hide a later readable image. The
// missing sources are distinct so the count cap would be reachable if the
// resolver charged them.
func TestEPUBParserMissingZipEntryDoesNotConsumeImageBudget(t *testing.T) {
	const missing = 256
	var content strings.Builder
	images := make([]epubTestImage, 0, missing+1)
	for i := 0; i < missing; i++ {
		name := fmt.Sprintf("Images/gone-%d.png", i)
		content.WriteString(`<img src="` + name + `">`)
		images = append(images, epubTestImage{name: name, missing: true})
	}
	content.WriteString(`<img src="Images/keep.png" alt="keep">`)
	images = append(images, epubTestImage{name: "Images/keep.png", content: "keep-bytes"})

	ctx, cache := recordingMediaContext()
	res := parseTestEPUBContext(t, ctx, []epubTestChapter{{
		name:    "ch1.xhtml",
		content: epubTestHTML(content.String()),
	}}, images)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if cache.putCalls != 1 || len(cache.puts) != 1 || string(cache.puts[0].data) != "keep-bytes" {
		t.Fatalf("cache puts = %+v, want only the readable image", cache.puts)
	}
	items := epubTestImageItems(t, res)
	if len(items) != 1 || items[0]["text"] != "keep" {
		t.Fatalf("image items = %#v, want the readable image", items)
	}
	if !strings.Contains(epubTestWarningText(res), "unresolved image reference") {
		t.Fatalf("missing unresolved-image warning: %v", res.Warnings)
	}
}

// An image above the per-image limit is reported through the budget warning
// without reading bytes, so it must not consume an image attempt either.
func TestEPUBParserOversizedZipImageDoesNotConsumeImageBudget(t *testing.T) {
	t.Setenv("RAGFLOW_MEDIA_IMAGE_MAX_BYTES", "8")
	t.Setenv("RAGFLOW_MEDIA_MAX_IMAGES_PER_RUN", "256")
	ctx, cache := recordingMediaContext()
	content := epubTestHTML(strings.Repeat(`<img src="Images/huge.png">`, 256) +
		`<img src="Images/ok.png" alt="ok">`)
	res := parseTestEPUBContext(t, ctx, []epubTestChapter{{
		name:    "ch1.xhtml",
		content: content,
	}}, []epubTestImage{
		{name: "Images/huge.png", content: strings.Repeat("x", 9)},
		{name: "Images/ok.png", content: "y"},
	})
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if cache.putCalls != 1 || len(cache.puts) != 1 || string(cache.puts[0].data) != "y" {
		t.Fatalf("cache puts = %+v, want only the small image", cache.puts)
	}
	if !strings.Contains(epubTestWarningText(res), "larger than the 8-byte per-image limit") {
		t.Fatalf("missing oversized-image warning: %v", res.Warnings)
	}
}

func TestEPUBParserByteRejectedEntriesDoNotConsumeImageSlots(t *testing.T) {
	t.Setenv("RAGFLOW_MEDIA_IMAGE_MAX_BYTES", "8")
	t.Setenv("RAGFLOW_MEDIA_MAX_IMAGES_PER_RUN", "2")
	t.Setenv("RAGFLOW_MEDIA_MAX_BYTES_PER_RUN", "5")
	ctx, cache := recordingMediaContext()
	res := parseTestEPUBContext(t, ctx, []epubTestChapter{{
		name: "ch1.xhtml",
		content: epubTestHTML(
			`<img src="Images/large-1.png"><img src="Images/large-2.png"><img src="Images/small.png" alt="small">`),
	}}, []epubTestImage{
		{name: "Images/large-1.png", content: strings.Repeat("a", 6)},
		{name: "Images/large-2.png", content: strings.Repeat("b", 6)},
		{name: "Images/small.png", content: "c"},
	})
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if cache.putCalls != 1 || len(cache.puts) != 1 || string(cache.puts[0].data) != "c" {
		t.Fatalf("cache puts = %+v, want only the small image", cache.puts)
	}
	if !strings.Contains(epubTestWarningText(res), "5-byte run image budget") {
		t.Fatalf("missing total-byte warning: %v", res.Warnings)
	}
}

// A read failure is still a fetch attempt, so the item budget keeps bounding
// the reads a document full of unreadable entries can force. Distinct sources
// are required: repeating one damaged entry shares one admission.
func TestEPUBParserUnreadableImageAttemptsCountAgainstBudget(t *testing.T) {
	t.Setenv("RAGFLOW_MEDIA_MAX_IMAGES_PER_RUN", "256")
	const attempts = 256
	var content strings.Builder
	images := make([]epubTestImage, 0, attempts+1)
	for i := 0; i < attempts; i++ {
		name := fmt.Sprintf("Images/bad-%d.png", i)
		content.WriteString(`<img src="` + name + `">`)
		images = append(images, epubTestImage{name: name, damaged: true})
	}
	content.WriteString(`<img src="Images/keep.png" alt="keep">`)
	images = append(images, epubTestImage{name: "Images/keep.png", content: "keep-bytes"})

	ctx, cache := recordingMediaContext()
	res := parseTestEPUBContext(t, ctx, []epubTestChapter{{
		name:    "ch1.xhtml",
		content: epubTestHTML(content.String()),
	}}, images)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if cache.putCalls != 0 {
		t.Fatalf("cache puts = %+v, want the readable image dropped after the attempt cap", cache.puts)
	}
	if !strings.Contains(epubTestWarningText(res), "256-item run limit") {
		t.Fatalf("missing run image limit warning: %v", res.Warnings)
	}
}

func TestEPUBParserMediaCacheWriteFailureFailsTask(t *testing.T) {
	writeErr := errors.New("kvrocks unavailable")
	cache := &recordingMediaCache{putErr: writeErr}
	ctx := media.WithCache(context.Background(), cache)
	res := parseTestEPUBContext(t, ctx, []epubTestChapter{{
		name:    "ch1.xhtml",
		content: epubTestHTML(`<img src="Images/a.png"><img src="Images/b.png">`),
	}}, []epubTestImage{{name: "Images/a.png", content: "a"}, {name: "Images/b.png", content: "b"}})
	if !errors.Is(res.Err, writeErr) {
		t.Fatalf("ParseWithResult error = %v, want %v", res.Err, writeErr)
	}
	if cache.putCalls != 1 {
		t.Fatalf("cache writes = %d, want traversal to stop after the first failure", cache.putCalls)
	}
}

func TestEPUBParserImageWithoutCacheFails(t *testing.T) {
	cases := map[string]string{
		"zip image": `<img src="Images/cover.png">`,
		"data URI":  `<img src="` + htmlMediaDataURI + `">`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			res := parseTestEPUBContext(t, context.Background(), []epubTestChapter{{
				name:    "ch1.xhtml",
				content: epubTestHTML(body),
			}}, []epubTestImage{{name: "Images/cover.png", content: "cover-bytes"}})
			if res.Err == nil || !strings.Contains(res.Err.Error(), "epub media cache is not configured") {
				t.Fatalf("error = %v, want a missing media cache error", res.Err)
			}
		})
	}
}

func TestEPUBTextParserSkipsImagesWithoutCache(t *testing.T) {
	res := NewEPUBTextParser().ParseWithResult(context.Background(), "book.epub", buildTestEPUBWithImages(t, []epubTestChapter{{
		name:    "ch1.xhtml",
		content: epubTestHTML(`<p>Before.</p><img src="Images/cover.png" alt="cover"><img src="` + htmlMediaDataURI + `" alt="inline"><p>After.</p>`),
	}}, []epubTestImage{{name: "Images/cover.png", content: "cover-bytes"}}))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if images := epubTestImageItems(t, res); len(images) != 0 {
		t.Fatalf("image items = %#v, want none", images)
	}
	if text := epubTestText(res); !strings.Contains(text, "Before.") || !strings.Contains(text, "After.") {
		t.Fatalf("text = %q, want surrounding prose", text)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none for a text-only parse", res.Warnings)
	}
}

// Data URIs share the run budget with ZIP entries. Distinct sources reach the
// count cap; repeated data URIs share one admission.
func TestEPUBParserDataURIImagesShareRunBudget(t *testing.T) {
	t.Setenv("RAGFLOW_MEDIA_MAX_IMAGES_PER_RUN", "256")
	const limit = 256
	var content strings.Builder
	for i := 0; i <= limit; i++ {
		content.WriteString(`<img src="data:image/png;base64,`)
		content.WriteString(base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("image-%d", i))))
		content.WriteString(`">`)
	}
	content.WriteString("<p>After images.</p>")
	ctx, cache := recordingMediaContext()
	res := parseTestEPUBContext(t, ctx, []epubTestChapter{{
		name:    "ch1.xhtml",
		content: epubTestHTML(content.String()),
	}}, nil)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if cache.putCalls != limit {
		t.Fatalf("cache writes = %d, want %d", cache.putCalls, limit)
	}
	images := 0
	for _, item := range ItemsToMaps(res.JSON) {
		if item["doc_type_kwd"] == DocTypeImage {
			images++
		}
	}
	if images != limit || !strings.Contains(epubTestText(res), "After images.") {
		t.Fatalf("images=%d text=%q", images, epubTestText(res))
	}
	if !strings.Contains(strings.Join(res.Warnings, "\n"), "256-item run limit") {
		t.Fatalf("missing run image limit warning: %v", res.Warnings)
	}
}

// TestEPUBParserRepeatedZipImageReadsAndWritesOnce pins source dedupe on the
// resolver: one ZIP entry referenced repeatedly is read and cached once, while
// every reference and its alt text stay in the result.
func TestEPUBParserRepeatedZipImageReadsAndWritesOnce(t *testing.T) {
	ctx, cache := recordingMediaContext()
	res := parseTestEPUBContext(t, ctx, []epubTestChapter{{
		name: "ch1.xhtml",
		content: epubTestHTML(
			`<p>one</p><img src="Images/icon.png" alt="first"><p>two</p>` +
				`<img src="Images/icon.png" alt="second"><img src="Images/icon.png" alt="third">`),
	}}, []epubTestImage{{name: "Images/icon.png", content: "icon-bytes"}})
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if cache.putCalls != 1 || len(cache.puts) != 1 || string(cache.puts[0].data) != "icon-bytes" {
		t.Fatalf("cache puts = %+v, want one write for the repeated entry", cache.puts)
	}
	items := epubTestImageItems(t, res)
	if len(items) != 3 {
		t.Fatalf("image items = %#v, want all three references", items)
	}
	first := entity.MediaRefFromAny(items[0]["media_ref"])
	for i, wantAlt := range []string{"first", "second", "third"} {
		if items[i]["text"] != wantAlt {
			t.Fatalf("image %d alt = %v, want %q", i, items[i]["text"], wantAlt)
		}
		ref := entity.MediaRefFromAny(items[i]["media_ref"])
		if first == nil || ref == nil || ref.SHA256 != first.SHA256 {
			t.Fatalf("image %d ref = %+v, want the shared reference %+v", i, ref, first)
		}
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", res.Warnings)
	}
}

// TestEPUBParserRepeatedZipImageAcrossChaptersReadsAndWritesOnce pins that the
// source memo is shared by the whole parse, not recreated for each chapter.
func TestEPUBParserRepeatedZipImageAcrossChaptersReadsAndWritesOnce(t *testing.T) {
	ctx, cache := recordingMediaContext()
	res := parseTestEPUBContext(t, ctx, []epubTestChapter{
		{name: "ch1.xhtml", content: epubTestHTML(`<img src="Images/icon.png" alt="first">`)},
		{name: "ch2.xhtml", content: epubTestHTML(`<img src="Images/icon.png" alt="second">`)},
	}, []epubTestImage{{name: "Images/icon.png", content: "icon-bytes"}})
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if cache.putCalls != 1 || len(cache.puts) != 1 || string(cache.puts[0].data) != "icon-bytes" {
		t.Fatalf("cache puts = %+v, want one write across chapters", cache.puts)
	}
	items := epubTestImageItems(t, res)
	if len(items) != 2 {
		t.Fatalf("image items = %#v, want one per chapter", items)
	}
	first := entity.MediaRefFromAny(items[0]["media_ref"])
	second := entity.MediaRefFromAny(items[1]["media_ref"])
	if first == nil || second == nil || first.SHA256 != second.SHA256 {
		t.Fatalf("refs = %+v / %+v, want the shared reference", first, second)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", res.Warnings)
	}
}

func TestEPUBParserRepeatedRejectedZipImageAcrossChaptersWarnsOnce(t *testing.T) {
	tests := []struct {
		name     string
		maxImage string
		maxTotal string
		payload  string
		wantWarn string
	}{
		{
			name:     "oversized",
			maxImage: "8",
			maxTotal: "8",
			payload:  strings.Repeat("x", 9),
			wantWarn: "larger than the 8-byte per-image limit",
		},
		{
			name:     "total bytes",
			maxImage: "8",
			maxTotal: "5",
			payload:  strings.Repeat("x", 6),
			wantWarn: "5-byte run image budget",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("RAGFLOW_MEDIA_IMAGE_MAX_BYTES", tt.maxImage)
			t.Setenv("RAGFLOW_MEDIA_MAX_BYTES_PER_RUN", tt.maxTotal)
			ctx, cache := recordingMediaContext()
			res := parseTestEPUBContext(t, ctx, []epubTestChapter{
				{name: "ch1.xhtml", content: epubTestHTML(`<img src="Images/rejected.png">`)},
				{name: "ch2.xhtml", content: epubTestHTML(`<img src="Images/rejected.png">`)},
			}, []epubTestImage{{name: "Images/rejected.png", content: tt.payload}})
			if res.Err != nil {
				t.Fatalf("ParseWithResult: %v", res.Err)
			}
			if cache.putCalls != 0 {
				t.Fatalf("cache writes = %+v, want none", cache.puts)
			}
			warnings := epubTestWarningText(res)
			if strings.Count(warnings, tt.wantWarn) != 1 {
				t.Fatalf("warnings = %q, want one %q entry", warnings, tt.wantWarn)
			}
		})
	}
}

// TestEPUBParserSkipsImagesAfterRunCacheLimit pins that a cache limit error is
// a skip with a warning, not a task failure.
func TestEPUBParserSkipsImagesAfterRunCacheLimit(t *testing.T) {
	cache := &recordingMediaCache{putErr: fmt.Errorf("mediacache: run image count limit 1 exceeded: %w", media.ErrMediaLimitExceeded)}
	ctx := media.WithCache(context.Background(), cache)
	res := parseTestEPUBContext(t, ctx, []epubTestChapter{{
		name:    "ch1.xhtml",
		content: epubTestHTML(`<p>before</p><img src="Images/a.png"><img src="Images/b.png"><p>after</p>`),
	}}, []epubTestImage{{name: "Images/a.png", content: "a"}, {name: "Images/b.png", content: "b"}})
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if cache.putCalls != 2 {
		t.Fatalf("cache writes = %d, want both references attempted", cache.putCalls)
	}
	if images := epubTestImageItems(t, res); len(images) != 0 {
		t.Fatalf("image items = %#v, want none", images)
	}
	if !strings.Contains(epubTestWarningText(res), "run media cache limit") {
		t.Fatalf("missing run-limit warning: %v", res.Warnings)
	}
	if !strings.Contains(epubTestText(res), "after") {
		t.Fatalf("text after skipped images lost: %q", epubTestText(res))
	}
}

func TestEPUBParserEmitsTableCellImages(t *testing.T) {
	ctx, cache := recordingMediaContext()
	res := parseTestEPUBContext(t, ctx, []epubTestChapter{{
		name:    "ch1.xhtml",
		content: epubTestHTML(`<table><tr><td>Owner</td><td><img src="Images/cell.png" alt="cell"></td></tr></table>`),
	}}, []epubTestImage{{name: "Images/cell.png", content: "cell-bytes"}})
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(cache.puts) != 1 || string(cache.puts[0].data) != "cell-bytes" {
		t.Fatalf("cache puts = %+v, want the table image", cache.puts)
	}
	items := ItemsToMaps(res.JSON)
	if len(items) != 3 {
		t.Fatalf("items = %#v, want text/table/image", items)
	}
	if items[1]["doc_type_kwd"] != DocTypeTable || items[1]["source_table_id"] != "html-table-1" {
		t.Fatalf("table item = %#v", items[1])
	}
	image := items[2]
	if image["doc_type_kwd"] != DocTypeImage || image["text"] != "cell" ||
		image["parent_table_id"] != "html-table-1" || image["row_index"] != 1 || image["column_index"] != 2 {
		t.Fatalf("table image item = %#v", image)
	}
	if _, ok := image["image"]; ok {
		t.Fatalf("inline image retained: %#v", image)
	}
	if entity.MediaRefFromAny(image["media_ref"]) == nil {
		t.Fatalf("table image without media_ref: %#v", image)
	}
}

func TestEPUBParserDataURIImagesUseEPUBFormat(t *testing.T) {
	ctx, cache := recordingMediaContext()
	res := parseTestEPUBContext(t, ctx, []epubTestChapter{{
		name:    "ch1.xhtml",
		content: epubTestHTML(`<img src="` + htmlMediaDataURI + `" alt="inline">`),
	}}, nil)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(cache.puts) != 1 || cache.puts[0].format != "epub" || cache.puts[0].mime != "image/png" || string(cache.puts[0].data) != "hello" {
		t.Fatalf("cache puts = %+v", cache.puts)
	}
	images := epubTestImageItems(t, res)
	if len(images) != 1 {
		t.Fatalf("image items = %#v, want one", images)
	}
	if _, ok := images[0]["image"]; ok {
		t.Fatalf("inline image retained: %#v", images[0])
	}
	ref := entity.MediaRefFromAny(images[0]["media_ref"])
	if ref == nil || ref.Format != "epub" {
		t.Fatalf("media_ref = %+v, want an epub reference", ref)
	}
}

func TestEPUBParserPreservesImageOrderAndAlt(t *testing.T) {
	ctx, _ := recordingMediaContext()
	res := parseTestEPUBContext(t, ctx, []epubTestChapter{{
		name:    "ch1.xhtml",
		content: epubTestHTML(`<p>one</p><img src="Images/a.png" alt="A"><p>two</p><img src="Images/b.png" alt="B">`),
	}}, []epubTestImage{{name: "Images/a.png", content: "a"}, {name: "Images/b.png", content: "b"}})
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	items := ItemsToMaps(res.JSON)
	if len(items) != 4 {
		t.Fatalf("items = %#v, want text/image/text/image", items)
	}
	if items[0]["text"] != "Test one" || items[0]["doc_type_kwd"] != DocTypeText {
		t.Fatalf("first item = %#v", items[0])
	}
	if items[1]["doc_type_kwd"] != DocTypeImage || items[1]["text"] != "A" || items[1]["media_order"] != 1 {
		t.Fatalf("first image = %#v", items[1])
	}
	if items[2]["text"] != "two" || items[2]["doc_type_kwd"] != DocTypeText {
		t.Fatalf("middle item = %#v", items[2])
	}
	if items[3]["doc_type_kwd"] != DocTypeImage || items[3]["text"] != "B" || items[3]["media_order"] != 2 {
		t.Fatalf("second image = %#v", items[3])
	}
}
