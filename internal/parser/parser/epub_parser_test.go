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
	"fmt"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
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

func epubTestHTML(body string) string {
	return `<?xml version="1.0" encoding="utf-8"?><html xmlns="http://www.w3.org/1999/xhtml"><head><title>Test</title></head><body>` + body + `</body></html>`
}

func buildTestEPUB(t *testing.T, chapters []epubTestChapter) []byte {
	t.Helper()
	var manifest, spine strings.Builder
	for i, ch := range chapters {
		fmt.Fprintf(&manifest, `<item id="c%d" href="%s" media-type="application/xhtml+xml"/>`, i, ch.name)
		fmt.Fprintf(&spine, `<itemref idref="c%d"/>`, i)
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
	write("META-INF/container.xml", containerXML)
	write("OEBPS/content.opf", contentOPF)
	for _, ch := range chapters {
		switch {
		case ch.missing:
		case ch.damaged:
			w, err := zw.CreateRaw(&zip.FileHeader{Name: "OEBPS/" + ch.name, Method: zip.Deflate, CompressedSize64: 4, UncompressedSize64: 64})
			if err != nil {
				t.Fatalf("create raw %s: %v", ch.name, err)
			}
			if _, err := w.Write([]byte{0xff, 0xff, 0xff, 0xff}); err != nil {
				t.Fatalf("write raw %s: %v", ch.name, err)
			}
		case ch.entry != "":
			write("OEBPS/"+ch.entry, ch.content)
		default:
			write("OEBPS/"+ch.name, ch.content)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func parseTestEPUB(t *testing.T, chapters []epubTestChapter) ParseResult {
	t.Helper()
	return NewEPUBParser().ParseWithResult(context.Background(), "book.epub", buildTestEPUB(t, chapters))
}

func TestEPUBParserReportsDocumentImageLimitAcrossChapters(t *testing.T) {
	image := `<img src="data:image/png;base64,aGVsbG8=">`
	res := parseTestEPUB(t, []epubTestChapter{
		{name: "one.xhtml", content: epubTestHTML(strings.Repeat(image, 128))},
		{name: "two.xhtml", content: epubTestHTML(strings.Repeat(image, 129) + "<p>After images.</p>")},
	})
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	images := 0
	for _, item := range res.JSON {
		if item.ToMap()["doc_type_kwd"] == "image" {
			images++
		}
	}
	if images != 256 || !strings.Contains(epubTestText(res), "After images.") {
		t.Fatalf("images=%d text=%q", images, epubTestText(res))
	}
	if !strings.Contains(strings.Join(res.Warnings, "\n"), "256-item document limit") {
		t.Fatalf("missing document image limit warning: %v", res.Warnings)
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
	res := parseTestEPUB(t, []epubTestChapter{
		{name: "ch1.xhtml", content: imageOnly},
		{name: "ch2.xhtml", damaged: true},
	})
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
