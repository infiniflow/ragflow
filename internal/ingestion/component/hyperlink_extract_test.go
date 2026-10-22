//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package component

import (
	"archive/zip"
	"bytes"
	"reflect"
	"sort"
	"testing"
)

func buildDocxWithRels(t *testing.T, relsXML string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range map[string]string{
		"[Content_Types].xml": `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"></Types>`,
		"word/document.xml":   `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body/></w:document>`,
	} {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if relsXML != "" {
		f, err := w.Create("word/_rels/document.xml.rels")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(relsXML)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractLinksFromDocx(t *testing.T) {
	rels := `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="https://example.com/a" TargetMode="External"/>` +
		`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="https://example.com/b"/>` +
		`<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/image1.png"/>` +
		`</Relationships>`
	got, err := extractLinksFromDocx(buildDocxWithRels(t, rels))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	want := []string{"https://example.com/a", "https://example.com/b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestExtractLinksFromDocxNoRels(t *testing.T) {
	got, err := extractLinksFromDocx(buildDocxWithRels(t, ""))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
	if _, err := extractLinksFromDocx([]byte("not a zip")); err == nil {
		t.Fatal("want error for invalid zip")
	}
}

func TestExtractLinksFromPDF(t *testing.T) {
	pdf := []byte(`%PDF-1.7
1 0 obj <</Type/Annot/Subtype/Link/A <</S/URI/URI (https://example.com/literal)>>>>
2 0 obj <</Type/Annot/Subtype/Link/A <</S/URI/URI <68747470733a2f2f6578616d706c652e636f6d2f686578>>>>>
3 0 obj <</Type/Annot/Subtype/Link/A <</S/URI/URI (https://example.com/esc\)paren)>>>>
4 0 obj <</Type/Page/Annots [1 0 R]>>`)
	got := extractLinksFromPDF(pdf)
	want := []string{"https://example.com/esc)paren", "https://example.com/hex", "https://example.com/literal"}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if out := extractLinksFromPDF([]byte("%PDF-1.7 no links here")); len(out) != 0 {
		t.Fatalf("got %v, want empty", out)
	}
}

func TestExtractLinksFromMarkdownText(t *testing.T) {
	text := "see [docs](https://example.com/docs \"Title\") and ![img](https://example.com/i.png)\n" +
		`<a href="https://example.com/anchor">x</a> [dup](https://example.com/docs)`
	got := extractLinksFromMarkdownText(text)
	// image embeds (![...](...)) are skipped; dupes collapse
	wantExact := []string{"https://example.com/docs", "https://example.com/anchor"}
	if !reflect.DeepEqual(got, wantExact) {
		t.Fatalf("got %v, want %v", got, wantExact)
	}
}

func TestNormalizeAndDedupeLinks(t *testing.T) {
	in := []string{
		"  https://example.com/b ",
		"https://example.com/a",
		"https://example.com/a",
		"mailto:x@y.z",
		"javascript:void(0)",
		"#frag",
		"ftp://example.com/f",
		"",
	}
	got, truncated := normalizeAndDedupeLinks(in, 10)
	want := []string{"https://example.com/a", "https://example.com/b"}
	if truncated || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v truncated=%v, want %v truncated=false", got, truncated, want)
	}
	if _, truncated := normalizeAndDedupeLinks(in, 1); !truncated {
		t.Fatal("want truncated=true when capped")
	}
	// determinism across input orders
	a, _ := normalizeAndDedupeLinks([]string{"https://x/2", "https://x/1"}, 10)
	b, _ := normalizeAndDedupeLinks([]string{"https://x/1", "https://x/2"}, 10)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("order-dependent: %v vs %v", a, b)
	}
}
