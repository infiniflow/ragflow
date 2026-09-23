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
)

// epubTestChapter is one spine item of a test EPUB. A missing chapter is
// listed in the manifest and spine but has no ZIP entry; a damaged one is
// stored as a Deflate entry whose data is not a valid Deflate stream.
type epubTestChapter struct {
	name    string
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

func epubTestText(res ParseResult) string {
	var parts []string
	for _, item := range res.JSON {
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
	res := parseTestEPUB(t, []epubTestChapter{
		{name: "ch1.xhtml", content: epubTestHTML(`<img src="plate.png" alt=""/>`)},
		{name: "ch2.xhtml", damaged: true},
	})
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
}

func TestEPUBParser_OnlyEmptyChaptersIsNotAnError(t *testing.T) {
	res := parseTestEPUB(t, []epubTestChapter{{name: "ch1.xhtml", content: ""}})
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
}
