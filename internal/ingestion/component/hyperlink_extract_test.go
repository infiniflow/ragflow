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

package component

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
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

// The two-sided error of the byte scan is part of its contract, documented on
// extractLinksFromPDF: an occurrence that is not an annotation action still
// matches, and a destination inside a compressed object stream does not.
func TestExtractLinksFromPDFScanBoundaries(t *testing.T) {
	// A content stream that merely contains the token is collected: a false
	// positive, bounded by the fetch budgets and the SSRF guard.
	inStream := []byte("%PDF-1.7\n4 0 obj <</Length 44>>\nstream\nBT (/URI(https://example.com/in-content) Tj) ET\nendstream\nendobj")
	if got := extractLinksFromPDF(inStream); len(got) != 1 || got[0] != "https://example.com/in-content" {
		t.Fatalf("got %v, want the documented content-stream match", got)
	}

	// A /URI that is not followed by a string is not a destination.
	if got := extractLinksFromPDF([]byte("%PDF-1.7\n<</S/URI/Type/Pagetree>>")); len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}

	// Object streams are never inflated, so links living in one are missed.
	var buf bytes.Buffer
	w, _ := flate.NewWriter(&buf, flate.BestCompression)
	_, _ = w.Write([]byte("0 40 <</A <</S/URI/URI (https://example.com/hidden)>>>"))
	_ = w.Close()
	compressed := append([]byte("%PDF-1.7\n5 0 obj <</Type/ObjStm/N 1/First 9/Filter/FlateDecode/Length "),
		strconv.Itoa(buf.Len())...)
	compressed = append(compressed, ">>\nstream\n"...)
	compressed = append(compressed, buf.Bytes()...)
	compressed = append(compressed, "\nendstream\nendobj"...)
	if got := extractLinksFromPDF(compressed); len(got) != 0 {
		t.Fatalf("got %v, want no links from a compressed object stream", got)
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

// The parser resolves every link form a renderer would: badge links wrapping an
// image, reference-style links through their definitions, angle autolinks and
// inline anchors — while a definition nothing references stays prose, and a bare
// URL is not a link in CommonMark.
func TestExtractLinksFromMarkdownTextLinkForms(t *testing.T) {
	text := "[![badge](badge.svg)](https://example.com/badge)\n" +
		"see [the docs][docs-ref], [empty][] and bare https://example.com/not-a-link\n" +
		"<https://example.com/auto>\n\n" +
		"[docs-ref]: https://example.com/docs\n" +
		"[empty]: https://example.com/empty\n" +
		"[orphan]: https://example.com/orphan\n"
	got := extractLinksFromMarkdownText(text)
	want := []string{
		"https://example.com/badge",
		"https://example.com/docs",
		"https://example.com/empty",
		"https://example.com/auto",
	}
	sorted := append([]string(nil), got...)
	sort.Strings(sorted)
	sortedWant := append([]string(nil), want...)
	sort.Strings(sortedWant)
	if !reflect.DeepEqual(sorted, sortedWant) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for _, url := range got {
		if url == "https://example.com/orphan" {
			t.Fatalf("unreferenced definition was collected: %v", got)
		}
		if url == "https://example.com/not-a-link" {
			t.Fatalf("bare URL is not a link in CommonMark: %v", got)
		}
	}
}

// A destination wrapped in angle brackets is a link, and its brackets are not
// part of the URL; a link written inside code is prose, not a link.
func TestExtractLinksFromMarkdownTextCodeAndAngleBrackets(t *testing.T) {
	text := "real [doc](<https://example.com/wrapped>) and [q](https://example.com/title \"why\")\n\n" +
		"[ref]: <https://example.com/def-angle>\n" +
		"see [ref][]\n\n" +
		"```\n[fenced](https://example.com/fenced)\n<a href=\"https://example.com/fenced-anchor\">a</a>\n```\n\n" +
		"~~~python\n[fenced2](https://example.com/fenced2)\n~~~\n\n" +
		"inline `[span](https://example.com/span)` here\n\n" +
		"    [indented](https://example.com/indented)\n"
	got := extractLinksFromMarkdownText(text)
	for _, url := range got {
		for _, bad := range []string{
			"https://example.com/fenced", "https://example.com/fenced-anchor",
			"https://example.com/fenced2", "https://example.com/span",
			"https://example.com/indented",
		} {
			if url == bad {
				t.Fatalf("a link inside code was collected as %q: %v", bad, got)
			}
		}
	}
	want := []string{
		"https://example.com/def-angle",
		"https://example.com/title",
		"https://example.com/wrapped",
	}
	sorted := append([]string(nil), got...)
	sort.Strings(sorted)
	if !reflect.DeepEqual(sorted, want) {
		t.Fatalf("got %v, want %v", got, want)
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
	got, distinct, truncated := normalizeAndDedupeLinks(in, 10)
	want := []string{"https://example.com/a", "https://example.com/b"}
	if truncated || distinct != 2 || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v distinct=%d truncated=%v, want %v distinct=2 truncated=false", got, distinct, truncated, want)
	}
	if _, _, truncated := normalizeAndDedupeLinks(in, 1); !truncated {
		t.Fatal("want truncated=true when capped")
	}
	// determinism across input orders
	a, _, _ := normalizeAndDedupeLinks([]string{"https://x/2", "https://x/1"}, 10)
	b, _, _ := normalizeAndDedupeLinks([]string{"https://x/1", "https://x/2"}, 10)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("order-dependent: %v vs %v", a, b)
	}
	// anchors collapse to one fetchable URL
	anchors, _, _ := normalizeAndDedupeLinks([]string{"https://x/a#s1", "https://x/a#s2"}, 10)
	if !reflect.DeepEqual(anchors, []string{"https://x/a"}) {
		t.Fatalf("anchors = %v, want one stripped URL", anchors)
	}
	// scheme and host are case-insensitive: one page must be fetched once
	cases, distinct, _ := normalizeAndDedupeLinks([]string{"HTTPS://X.A.com/p", "https://x.a.com/p", "https://x.a.com/P"}, 10)
	if distinct != 2 || !reflect.DeepEqual(cases, []string{"HTTPS://X.A.com/p", "https://x.a.com/P"}) {
		t.Fatalf("cases = %v distinct=%d, want two (same page, differing path case is a different page)", cases, distinct)
	}
}

func TestExtractLinksFromDocxRejectsOversizedRels(t *testing.T) {
	seg := `<Relationship Id="r" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="https://example.com"/>`
	huge := `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		strings.Repeat(seg, hyperlinkMaxRelsBytes/len(seg)+2) +
		`</Relationships>`
	if _, err := extractLinksFromDocx(buildDocxWithRels(t, huge)); err == nil {
		t.Fatal("want error for oversized rels part")
	}
}

func TestExtractLinksFromPDFSkipsOversizedURI(t *testing.T) {
	long := strings.Repeat("a", hyperlinkMaxURIBytes+10)
	pdf := []byte(`%PDF-1.7
1 0 obj <</Type/Annot/Subtype/Link/A <</S/URI/URI (` + long + `)>>>>
2 0 obj <</Type/Annot/Subtype/Link/A <</S/URI/URI (https://example.com/ok)>>>>>`)
	got := extractLinksFromPDF(pdf)
	if !reflect.DeepEqual(got, []string{"https://example.com/ok"}) {
		t.Fatalf("got %d links, want only the normal one", len(got))
	}
}

// An unterminated literal must not consume the rest of the file: the scan
// window is bounded, so a later valid /URI marker is still found. Before the
// bound existed, the first reader ran to the file's end and swallowed the
// valid marker's closing paren.
func TestExtractLinksFromPDFBoundsUnterminatedLiteral(t *testing.T) {
	pdf := []byte("/URI (" +
		strings.Repeat("a", hyperlinkMaxPDFScanBytes+10) +
		" /URI (https://example.com/ok)")
	got := extractLinksFromPDF(pdf)
	if !reflect.DeepEqual(got, []string{"https://example.com/ok"}) {
		t.Fatalf("got %v, want only the URI after the unterminated literal", got)
	}
}

// Same bound for the hex form: an unterminated <...> must not search the rest
// of the file and swallow a later valid marker.
func TestExtractLinksFromPDFBoundsUnterminatedHex(t *testing.T) {
	pdf := []byte("/URI <" +
		strings.Repeat("41", hyperlinkMaxPDFScanBytes) +
		" /URI <68747470733a2f2f6578616d706c652e636f6d2f6f6b>")
	got := extractLinksFromPDF(pdf)
	if !reflect.DeepEqual(got, []string{"https://example.com/ok"}) {
		t.Fatalf("got %v, want only the URI after the unterminated hex string", got)
	}
}

// Repeated malformed markers keep the scan bounded and still reach the valid
// marker that follows them.
func TestExtractLinksFromPDFRepeatedUnterminatedMarkers(t *testing.T) {
	pdf := []byte(strings.Repeat("/URI (", 200) +
		strings.Repeat("a", hyperlinkMaxPDFScanBytes+10) +
		" /URI (https://example.com/ok)")
	got := extractLinksFromPDF(pdf)
	if !reflect.DeepEqual(got, []string{"https://example.com/ok"}) {
		t.Fatalf("got %v, want the trailing URI", got)
	}
}

// The scan window includes both delimiters, so a URI at the decoded cap is
// still parsed. This pins the off-by-one where the closing '>' fell one byte
// outside the window.
func TestReadPDFHexStringKeepsMaxSizedURI(t *testing.T) {
	body := strings.Repeat("61", hyperlinkMaxURIBytes)
	uri, rest, ok := readPDFHexString([]byte("<" + body + ">tail"))
	if !ok || len(uri) != hyperlinkMaxURIBytes || string(rest) != "tail" {
		t.Fatalf("ok=%v len=%d rest=%q, want a %d-byte URI", ok, len(uri), rest, hyperlinkMaxURIBytes)
	}
}

func TestReadPDFHexStringRejectsOverCapURI(t *testing.T) {
	body := strings.Repeat("61", hyperlinkMaxURIBytes+1)
	if _, _, ok := readPDFHexString([]byte("<" + body + ">")); ok {
		t.Fatal("want over-cap hex URI rejected")
	}
}

// Escape-heavy literals can reach the decoded cap with two raw bytes per
// character; the window must still include the closing paren.
func TestReadPDFLiteralStringKeepsMaxSizedEscapedURI(t *testing.T) {
	body := strings.Repeat(`\a`, hyperlinkMaxURIBytes)
	uri, rest, ok := readPDFLiteralString([]byte("(" + body + ")tail"))
	if !ok || len(uri) != hyperlinkMaxURIBytes || string(rest) != "tail" {
		t.Fatalf("ok=%v len=%d rest=%q, want a %d-byte URI", ok, len(uri), rest, hyperlinkMaxURIBytes)
	}
}

func TestExtractLinksFromPDFCapsCandidates(t *testing.T) {
	var b strings.Builder
	for i := 0; i < hyperlinkMaxURLCandidates+10; i++ {
		fmt.Fprintf(&b, "/URI (https://example.com/%d)", i)
	}
	got := extractLinksFromPDF([]byte(b.String()))
	if len(got) != hyperlinkMaxURLCandidates {
		t.Fatalf("got %d links, want %d", len(got), hyperlinkMaxURLCandidates)
	}
}

func TestExtractLinksFromMarkdownTextCapsCandidates(t *testing.T) {
	var b strings.Builder
	for i := 0; i < hyperlinkMaxURLCandidates+10; i++ {
		fmt.Fprintf(&b, "[x](https://example.com/%d)", i)
	}
	got := extractLinksFromMarkdownText(b.String())
	if len(got) != hyperlinkMaxURLCandidates {
		t.Fatalf("got %d links, want %d", len(got), hyperlinkMaxURLCandidates)
	}
}

func TestHyperlinkDocxCapsCandidates(t *testing.T) {
	seg := `<Relationship Id="r%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="https://example.com/%d"/>`
	var b strings.Builder
	b.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for i := 0; i < hyperlinkMaxURLCandidates+5; i++ {
		fmt.Fprintf(&b, seg, i, i)
	}
	b.WriteString(`</Relationships>`)

	links, err := extractLinksFromDocx(buildDocxWithRels(t, b.String()))
	if err != nil {
		t.Fatalf("extractLinksFromDocx: %v", err)
	}
	if len(links) != hyperlinkMaxURLCandidates {
		t.Fatalf("got %d links, want the %d candidate cap", len(links), hyperlinkMaxURLCandidates)
	}
}
