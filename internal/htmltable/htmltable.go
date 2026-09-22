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

// Package htmltable owns the wire shape of self-describing HTML tables
// shared between producers (parsers rendering spreadsheets as <table>) and
// consumers (chunkers detecting and extracting rows from that markup).
//
// The package intentionally depends on nothing above it: no parser, chunker,
// or canvas types. Token counting is injected where needed so this package
// stays free of NLP dependencies.
package htmltable

import (
	"html"
	"strings"
)

// IsTableStrictHTML reports whether block text is an outer <table> element
// (the inlined GFM/HTML table). Only such blocks may be emitted as structured
// table items; other raw HTML (e.g. <div>, <style>) is plain text. This is
// the historical markdown-parser predicate and must not be widened: callers
// that classify whole blocks rely on the exact "<table" prefix semantics.
func IsTableStrictHTML(s string) bool {
	return strings.HasPrefix(strings.TrimSpace(strings.ToLower(s)), "<table")
}

// IsTableHTML is the payload-level routing predicate: true when the text is a
// table by content fact, i.e. an outer <table> element (strict) or a bare-row
// fragment containing <tr>. It is deliberately at least as permissive as
// TableRows, which rewraps "<tr"-bearing fragments that lack "<table". Using
// this instead of type metadata is what keeps a real table from being
// mis-routed as prose.
func IsTableHTML(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(lower, "<table") || strings.Contains(lower, "<tr")
}

// RenderTableHTML renders records as one self-describing <table> payload:
// a caption naming the sheet, the header row as <th> cells, then each data
// row as <td> cells. Cells are trimmed and HTML-escaped. The exact byte
// layout is the wire contract shared with the chunkers — change it only
// together with every consumer.
func RenderTableHTML(sheet string, header []string, rows [][]string) string {
	var builder strings.Builder
	builder.WriteString("<table><caption>")
	builder.WriteString(html.EscapeString(sheet))
	builder.WriteString("</caption>\n<tr>")
	for _, cell := range header {
		builder.WriteString("<th>")
		builder.WriteString(html.EscapeString(strings.TrimSpace(cell)))
		builder.WriteString("</th>")
	}
	builder.WriteString("</tr>\n")
	for _, row := range rows {
		builder.WriteString("<tr>")
		for _, cell := range row {
			builder.WriteString("<td>")
			builder.WriteString(html.EscapeString(strings.TrimSpace(cell)))
			builder.WriteString("</td>")
		}
		builder.WriteString("</tr>\n")
	}
	builder.WriteString("</table>\n")
	return builder.String()
}
