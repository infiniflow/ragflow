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

package htmltable

import (
	"strings"
	"testing"
)

func TestIsTableStrictHTML(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"<table><tr><td>a</td></tr></table>", true},
		{"  \n<TABLE >", true},
		{"<table-x>", true}, // historical prefix semantics: "<table" prefix only
		{"<tr><td>a</td></tr>", false},
		{"<div>x</div>", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsTableStrictHTML(c.in); got != c.want {
			t.Errorf("IsTableStrictHTML(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestIsTableHTMLAtLeastAsPermissiveAsTableRows(t *testing.T) {
	// The routing predicate must never deny what TableRows can read: a bare
	// <tr> fragment yields rows via the parser rewrap, so it must route here.
	frag := "<tr><td>q</td><td>a</td></tr>"
	if !IsTableHTML(frag) {
		t.Errorf("bare <tr> fragment rejected by IsTableHTML")
	}
	if len(TableRows(frag)) != 1 {
		t.Fatalf("TableRows did not recover the bare fragment")
	}
	if IsTableHTML("plain prose") {
		t.Errorf("prose falsely routed as table")
	}
	if !IsTableHTML("<table></table>") {
		t.Errorf("outer table rejected")
	}
}

func TestRenderTableHTMLGoldenBytes(t *testing.T) {
	// Byte-for-byte the wire contract previously produced by
	// recordsToHTMLTableItem: unconditional caption, "\n" after
	// </caption> and after each </tr>, trailing newline.
	got := RenderTableHTML("S<1>", []string{" a ", "b"}, [][]string{{"1", "x&y"}, {"<z>", ""}})
	want := "<table><caption>S&lt;1&gt;</caption>\n" +
		"<tr><th>a</th><th>b</th></tr>\n" +
		"<tr><td>1</td><td>x&amp;y</td></tr>\n" +
		"<tr><td>&lt;z&gt;</td><td></td></tr>\n" +
		"</table>\n"
	if got != want {
		t.Errorf("RenderTableHTML mismatch:\ngot:  %q\nwant: %q", got, want)
	}

	headerOnly := RenderTableHTML("s", []string{"h1", "h2"}, nil)
	if !strings.HasPrefix(headerOnly, "<table><caption>s</caption>\n<tr><th>h1</th><th>h2</th></tr>\n") ||
		!strings.HasSuffix(headerOnly, "</table>\n") {
		t.Errorf("header-only table layout changed: %q", headerOnly)
	}
}

func TestTableRowsCoreShapes(t *testing.T) {
	rows := TableRows("<table><tr><th>h</th></tr><tr><td>a</td><td>b</td></tr></table>")
	if len(rows) != 2 || rows[0][0] != "h" || len(rows[1]) != 2 || rows[1][1] != "b" {
		t.Fatalf("basic table read failed: %v", rows)
	}

	// Nested table: inner rows must not be re-reported; the nested text
	// folds into the cell that holds it.
	nested := TableRows("<table><tr><td>out<table><tr><td>in</td></tr></table></td></tr></table>")
	if len(nested) != 1 || len(nested[0]) != 1 || nested[0][0] != "outin" {
		t.Fatalf("nested table handling changed: %v", nested)
	}

	// Missing close tags are recovered, not dropped.
	broken := TableRows("<table><tr><td>a<td>b</table>")
	if len(broken) != 1 || len(broken[0]) != 2 || broken[0][0] != "a" || broken[0][1] != "b" {
		t.Fatalf("recovery of unclosed cells changed: %v", broken)
	}

	// Inert subtrees contribute no rows or text.
	inert := TableRows("<table><tr><td>x</td><td><template><tr><td>t</td></tr></template></td></tr></table>")
	if len(inert) != 1 || len(inert[0]) != 2 || inert[0][1] != "" {
		t.Fatalf("inert template content leaked: %v", inert)
	}

	// <br> becomes a newline inside the cell text.
	br := TableRows("<table><tr><td>up<br>down</td></tr></table>")
	if len(br) != 1 || br[0][0] != "up\ndown" {
		t.Fatalf("<br> handling changed: %q", br)
	}
}
