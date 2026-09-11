package chunker

import (
	"fmt"
	"strings"
	"testing"
)

// Extraction must follow the table's structure rather than a tag pattern.
// The markup is rendered by several parsers and some of it comes from
// user-supplied documents, so it is not guaranteed to be well formed —
// a tag-level scan mishandles most cases below.
func TestExtractQATableFollowsStructure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		html   string
		strict bool
		want   [][2]string
	}{
		{
			name: "newline inside a cell",
			html: "<table><tr><th>问题\n跨行</th><th>答案</th></tr></table>",
			want: [][2]string{{"问题\n跨行", "答案"}},
		},
		{
			name: "colspan",
			html: `<table><tr><td colspan="2">q</td><td>a</td></tr></table>`,
			want: [][2]string{{"q", "a"}},
		},
		{
			name: "missing </td>",
			html: "<table><tr><td>q<td>a</tr></table>",
			want: [][2]string{{"q", "a"}},
		},
		{
			name: "missing </tr>",
			html: "<table><tr><td>q</td><td>a</td><tr><td>q2</td><td>a2</td></tr></table>",
			want: [][2]string{{"q", "a"}, {"q2", "a2"}},
		},
		{
			name: "attribute containing '>'",
			html: `<table><tr><td data-x="a>b">q</td><td>a</td></tr></table>`,
			want: [][2]string{{"q", "a"}},
		},
		{
			name: "<br> inside a cell",
			html: "<table><tr><td>q<br>L2</td><td>a</td></tr></table>",
			want: [][2]string{{"q\nL2", "a"}},
		},
		{
			// Content that is parsed but never rendered must not join the
			// sentence: an inline <script>/<style> is not text a reader sees.
			name: "inert elements contribute no text",
			html: "<table><tr><td>q<script>var x=1;</script></td>" +
				"<td>a<style>.c{color:red}</style></td></tr></table>",
			want: [][2]string{{"q", "a"}},
		},
		{
			name: "noscript and template contribute no text",
			html: "<table><tr><td>q<noscript>enable js</noscript></td>" +
				"<td>a<template>tpl</template></td></tr></table>",
			want: [][2]string{{"q", "a"}},
		},
		{
			// The counterpart of the two cases above: elements whose content
			// *is* rendered keep their text, so the skip list stays narrow.
			name: "rendered descendants keep their text",
			html: "<table><tr><td>q<textarea>notes</textarea></td>" +
				"<td>a<span>bold</span></td></tr></table>",
			want: [][2]string{{"qnotes", "abold"}},
		},
		{
			// A template's content goes into the ordinary child list, so its
			// placeholder rows would otherwise be read as rows of the table.
			name: "template rows are not table rows",
			html: "<table><tr><td>q</td><td>a</td></tr>" +
				"<template><tr><td>TQ</td><td>TA</td></tr></template></table>",
			want: [][2]string{{"q", "a"}},
		},
		{
			// Foreign content reuses HTML tag names: an <svg><tr> carries no
			// table semantics.
			name: "svg rows are not table rows",
			html: "<table><tr><td>q</td><td>a</td></tr>" +
				"<svg><tr><td>VQ</td><td>VA</td></tr></svg></table>",
			want: [][2]string{{"q", "a"}},
		},
		{
			// A template's rows are markup, not rows: the script's content is
			// raw text to the parser, so only the real row becomes a pair.
			// Found on a real page, where the unrendered template would
			// otherwise contribute placeholder pairs.
			name: "markup inside a script is not a row",
			html: "<table><script type=\"text/tpl\"><tr><td>tpl</td><td>tpl2</td></tr></script>" +
				"<tr><td>q</td><td>a</td></tr></table>",
			want: [][2]string{{"q", "a"}},
		},
		{
			name: "commented-out markup is not a pair",
			html: "<table><!-- <tr><td>old</td><td>value</td></tr> --><tr><td>q</td><td>a</td></tr></table>",
			want: [][2]string{{"q", "a"}},
		},
		{
			name: "commented-out cell is not a cell",
			html: "<table><tr><!-- <td>dead</td> --><td>q</td><td>a</td></tr></table>",
			want: [][2]string{{"q", "a"}},
		},
		{
			// The nested table's text is folded into the cell holding it, and
			// its own rows are not reported as rows of the enclosing table.
			name: "nested table",
			html: "<table><tr><td><table><tr><td>in</td><td>x</td></tr></table></td><td>a</td></tr></table>",
			want: [][2]string{{"inx", "a"}},
		},
		{
			// A bare row fragment: the HTML5 "in body" mode would discard the
			// <tr>, so it has to be parsed in a table context.
			name: "row without a table wrapper",
			html: "<tr><td>q</td><td>a</td></tr>",
			want: [][2]string{{"q", "a"}},
		},
		{
			name: "rows without a table wrapper",
			html: "<tr><td>q</td><td>a</td></tr><tr><td>q2</td><td>a2</td></tr>",
			want: [][2]string{{"q", "a"}, {"q2", "a2"}},
		},
		{
			name: "rows wrapped in tbody only",
			html: "<tbody><tr><td>q</td><td>a</td></tr></tbody>",
			want: [][2]string{{"q", "a"}},
		},
		{
			// Deliberately narrow: cells without a row are not a pair, matching
			// the previous behaviour instead of inventing a row for them.
			name: "cells without a row yield nothing",
			html: "<td>q</td><td>a</td>",
			want: nil,
		},
		{
			name: "empty cell is skipped when picking the pair",
			html: "<table><tr><td>q</td><td></td><td>a</td></tr></table>",
			want: [][2]string{{"q", "a"}},
		},
		{
			name: "unbalanced row yields nothing",
			html: "<table><tr><td>q only</td></tr></table>",
			want: nil,
		},
		{
			// The CSV contract (Python qa.py:365) is exactly two fields.
			name:   "three columns are rejected in strict mode",
			html:   "<table><tr><td>q</td><td>a</td><td>extra</td></tr></table>",
			strict: true,
			want:   nil,
		},
		{
			name:   "three columns keep the first two in lax mode",
			html:   "<table><tr><td>q</td><td>a</td><td>extra</td></tr></table>",
			strict: false,
			want:   [][2]string{{"q", "a"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pairs := extractQATable(tc.html, tc.strict)
			got := make([][2]string, 0, len(pairs))
			for _, p := range pairs {
				got = append(got, [2]string{p.Question, p.Answer})
			}
			if len(got) != len(tc.want) {
				t.Fatalf("pairs = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("pair %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// BenchmarkExtractQATable makes the cost of the extraction reproducible:
//
//	go test -run '^$' -bench BenchmarkExtractQATable -benchmem ./internal/ingestion/component/chunker/
//
// The cells carry the two shapes that the previous tag-level scan handled
// worst — an embedded newline and an entity — so the numbers reflect the
// markup table items actually carry.
func BenchmarkExtractQATable(b *testing.B) {
	for _, rows := range []int{10, 256, 1000} {
		markup := benchmarkTable(rows)
		b.Run(fmt.Sprintf("%d_rows", rows), func(b *testing.B) {
			b.SetBytes(int64(len(markup)))
			for i := 0; i < b.N; i++ {
				extractQATable(markup, false)
			}
		})
	}
}

// benchmarkTable renders a two-column table of the given size, one row per
// Q&A pair, with newlines and entities inside the cells.
func benchmarkTable(rows int) string {
	var sb strings.Builder
	sb.WriteString("<table><caption>Sheet1</caption>" +
		"<tr><th>question</th><th>answer</th></tr>")
	for i := 0; i < rows; i++ {
		fmt.Fprintf(&sb,
			"<tr><td>question %d, which spans\na second line</td>"+
				"<td>answer %d, with an entity &amp; and more text</td></tr>", i, i)
	}
	sb.WriteString("</table>")
	return sb.String()
}
