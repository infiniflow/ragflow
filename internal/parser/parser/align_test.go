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
// Warranties, INCLUDING THE WARRANTIES OF MERCHANTABILITY AND
// FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.
//

package parser

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// Normalizer transforms a single item's text before comparison. Normalizers
// are composed per parser type so the same comparison core is reused across
// every format (sessions A–E of the Go↔Python parser alignment).
type Normalizer func(string) string

// WithDelimiterStrip returns a Normalizer that replaces every rune present in
// delims with a single space. This normalizes the delimiter-split difference:
// Python splits the text at delimiters into separate items (so the delimiter
// becomes an item boundary, i.e. whitespace), while Go keeps the delimiter
// inline. Replacing with a space — rather than deleting — preserves the token
// separation on the Go side, so after CollapseWhitespace both sides yield the
// same space-joined text.
func WithDelimiterStrip(delims string) Normalizer {
	return func(s string) string {
		return strings.Map(func(r rune) rune {
			if strings.ContainsRune(delims, r) {
				return ' '
			}
			return r
		}, s)
	}
}

// CollapseWhitespace returns a Normalizer that trims and collapses runs of
// whitespace into a single space. Universal normalizer for tolerant compare.
func CollapseWhitespace() Normalizer {
	return func(s string) string {
		return strings.Join(strings.Fields(s), " ")
	}
}

// htmlTagRE matches an HTML tag so table/HTML markup can be ignored when
// comparing content across the two Markdown libraries (goldmark vs
// Python-Markdown serialize tables differently but the cell text is the same).
var htmlTagRE = regexp.MustCompile(`(?is)<[^>]+>`)

// StripHTMLTags returns a Normalizer that removes HTML tags, leaving the
// visible text. Tags are replaced with a single space (not deleted) so
// adjacent cell text does not fuse — e.g. "<td>A</td><td>B</td>" becomes
// "A B" rather than "AB". CollapseWhitespace then folds the extra space.
// Used so table-markup differences between Markdown libraries don't mask the
// underlying content equivalence.
func StripHTMLTags() Normalizer {
	return func(s string) string {
		return htmlTagRE.ReplaceAllString(s, " ")
	}
}

// Markdown-syntax regexes removed by StripMarkdownSyntax. The Python flow
// parser keeps raw Markdown in its section text ("# Title", "- item",
// ``` fenced ```); the Go parser emits clean per-block text. These are
// representation differences (PARSER_ALIGNMENT_HANDOFF.md §3.1), not content
// divergences, so the Markdown alignment strips them before comparing.
var (
	mdHeaderRE = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	mdListRE   = regexp.MustCompile(`(?m)^\s*[-*+]\s+`)
	mdFenceRE  = regexp.MustCompile("(?s)```[^\n]*\n(.*?)```")
)

// StripMarkdownSyntax returns a Normalizer that removes Markdown presentation
// characters (ATX headings, list bullets, fenced-code fences) from a section,
// leaving the bare text. It must run before CollapseWhitespace because the
// fence regex relies on the surrounding newlines.
func StripMarkdownSyntax() Normalizer {
	return func(s string) string {
		s = mdHeaderRE.ReplaceAllString(s, "")
		s = mdListRE.ReplaceAllString(s, "")
		s = mdFenceRE.ReplaceAllString(s, "$1")
		return s
	}
}

// FilterByDocType returns only the items whose doc_type_kwd equals kwd.
// Python emits duplicate table items (separate_tables=False still appends
// them) — excluding doc_type_kwd:"table" lets the comparison focus on the
// inlined textual content, which is what Go produces.
func FilterByDocType(items []map[string]any, kwd string) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		if v, _ := it["doc_type_kwd"].(string); v == kwd {
			out = append(out, it)
		}
	}
	return out
}

// AlignOptions configures NormalizeConcat / CompareAlignment.
type AlignOptions struct {
	// Normalizers applied (in order) to each item's text before concat.
	Normalizers []Normalizer
	// ItemKey is the field holding the compared text (default "text").
	ItemKey string
}

func alignItemText(item map[string]any, key string) string {
	if key == "" {
		key = "text"
	}
	if v, ok := item[key].(string); ok {
		return v
	}
	return ""
}

// NormalizeConcat extracts the text field from each item, applies the
// normalizers in order, and concatenates into one string. Format-agnostic.
//
// Items are joined with a single space (after whitespace is collapsed by the
// normalizers) rather than by newlines: Go emits one item per top-level block
// while Python splits the same text on delimiters into many smaller items, so
// the item *boundaries* legitimately differ. Joining on whitespace makes the
// comparison boundary-agnostic — only the concatenated content (order
// preserved) is compared, which is exactly the alignment guarantee we want.
// Empty items are skipped so Python's trailing/duplicate segments don't mask a
// real content difference.
func NormalizeConcat(items []map[string]any, opts AlignOptions) string {
	key := opts.ItemKey
	if key == "" {
		key = "text"
	}
	parts := make([]string, 0, len(items))
	for _, it := range items {
		t := alignItemText(it, key)
		for _, n := range opts.Normalizers {
			t = n(t)
		}
		if strings.TrimSpace(t) == "" {
			continue
		}
		// Trim so a delimiter turned into a trailing space (WithDelimiterStrip)
		// doesn't combine with the join space into a double gap.
		t = strings.TrimSpace(t)
		parts = append(parts, t)
	}
	return strings.Join(parts, " ")
}

// CompareAlignment reports whether two parser outputs are aligned after
// normalization. goItems come from Go's ParseResult.JSON; pyItems come from the
// Python golden JSON. Returns (equal, diffReport).
func CompareAlignment(goItems, pyItems []map[string]any, opts AlignOptions) (bool, string) {
	g := NormalizeConcat(goItems, opts)
	p := NormalizeConcat(pyItems, opts)
	if g == p {
		return true, ""
	}
	return false, diffReport(g, p)
}

func diffReport(g, p string) string {
	const max = 2000
	if len(g) > max {
		g = g[:max] + "...(truncated)"
	}
	if len(p) > max {
		p = p[:max] + "...(truncated)"
	}
	return "alignment mismatch after normalization:\n--- GO ---\n" + g + "\n--- PY ---\n" + p
}

// GoldenDoc is a {meta, items} Python golden baseline. Meta records how the
// baseline was produced (generator, sample, delimiter, accepted divergences)
// so it stays reproducible without a committed generator script; Items is the
// list of parsed output items compared against Go's parser.
type GoldenDoc struct {
	Meta  map[string]any
	Items []map[string]any
}

// parseGolden unmarshals a golden file that may be either a bare JSON array of
// items (legacy format) or a {meta, items} document (current format). It
// returns the full document either way. Tolerant parsing keeps older
// callers/tests working after the format gained a meta block.
func parseGolden(t *testing.T, data []byte) (*GoldenDoc, error) {
	t.Helper()
	var doc GoldenDoc
	if err := json.Unmarshal(data, &doc); err == nil && doc.Items != nil {
		return &doc, nil
	}
	// Legacy flat-array format: treat the whole file as the items list.
	var items []map[string]any
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("parse golden: %w", err)
	}
	return &GoldenDoc{Items: items}, nil
}

// LoadGolden reads a Python golden JSON file and returns its items. The file
// may be a bare array (legacy) or a {meta, items} document; either way only
// the items are returned, so existing callers keep working unchanged.
func LoadGolden(t *testing.T, path string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("load golden %s: %v", path, err)
	}
	doc, err := parseGolden(t, data)
	if err != nil {
		t.Fatalf("load golden %s: %v", path, err)
	}
	if len(doc.Items) == 0 {
		t.Fatalf("golden %s has no items", path)
	}
	return doc.Items
}

// LoadGoldenDoc reads a Python golden JSON file and returns the full
// {meta, items} document, including the meta block. Used by tests that drive
// behavior from the golden's metadata (e.g. accepted_divergences).
func LoadGoldenDoc(t *testing.T, path string) *GoldenDoc {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("load golden %s: %v", path, err)
	}
	doc, err := parseGolden(t, data)
	if err != nil {
		t.Fatalf("load golden %s: %v", path, err)
	}
	if len(doc.Items) == 0 {
		t.Fatalf("golden %s has no items", path)
	}
	return doc
}

// AcceptedDivergences returns the doc_type_kwd values the golden baseline
// declares as accepted representation differences (e.g. "table"/"image"), so
// the comparison can ignore them on both sides. Driven entirely by the
// golden's meta block — the test holds no hardcoded divergence list.
func AcceptedDivergences(meta map[string]any) []string {
	raw, ok := meta["accepted_divergences"]
	if !ok {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, e := range list {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// FilterOutDocTypes returns the items whose doc_type_kwd is NOT in drop. Used
// to exclude the meta-declared accepted divergences from the comparison.
func FilterOutDocTypes(items []map[string]any, drop []string) []map[string]any {
	if len(drop) == 0 {
		return items
	}
	banned := make(map[string]bool, len(drop))
	for _, d := range drop {
		banned[d] = true
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		if v, _ := it["doc_type_kwd"].(string); !banned[v] {
			out = append(out, it)
		}
	}
	return out
}

// filterTableDivergence applies the "table" accepted divergence on BOTH sides
// of the alignment comparison. Go emits a table as a single
// doc_type_kwd:"table" item (already dropped by FilterOutDocTypes), while
// Python keeps the <table> markup inlined as a doc_type_kwd:"text" item. The
// table content is dropped from the prose comparison so the test focuses on
// non-table prose, but the table is NOT silently removed from a combined prose
// item: only items that are ENTIRELY a <table> block are dropped (see
// isStandaloneTable). A prose item that merely *contains* a table (e.g.
// "intro <table>…</table> outro") is kept whole, so structural differences in
// surrounding prose are never masked. Table *equivalence* (column/cell content
// matching between Go and Python) is checked separately by tableSignatures in
// the golden tests.
func filterTableDivergence(items []map[string]any, drop []string) []map[string]any {
	items = FilterOutDocTypes(items, drop)
	if !slices.Contains(drop, "table") {
		return items
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		if text, _ := it["text"].(string); isStandaloneTable(text) {
			continue
		}
		out = append(out, it)
	}
	return out
}

// isStandaloneTable reports whether text is exactly a <table> element with no
// surrounding prose (ignoring surrounding whitespace) — i.e. the whole item IS
// the table. This recognizes Python's HTML golden inline table text item
// (doc_type_kwd:"text", text == "<table>…</table>") without accidentally
// treating a prose item that merely contains a table as droppable. A combined
// item such as "intro <table>…</table> outro" returns false and is kept in the
// prose comparison.
func isStandaloneTable(text string) bool {
	t := strings.TrimSpace(text)
	if !strings.HasPrefix(strings.ToLower(t), "<table") {
		return false
	}
	close := strings.ToLower(t)
	idx := strings.LastIndex(close, "</table>")
	return idx >= 0 && strings.TrimSpace(t[idx+len("</table>"):]) == ""
}

// tableCellSignature returns a normalized, order-preserving signature of a
// <table>'s cell text (th/td inner text, whitespace-collapsed, cells joined by
// "\n"). It is intentionally tolerant of serialization differences (attribute
// order, <tbody> insertion, self-closing vs paired tags) because only the
// visible cell text is used — which is exactly what guards against a table
// being collapsed or a column/cell being dropped. Returns "" if text is not a
// parseable table.
func tableCellSignature(text string) string {
	doc, err := html.Parse(strings.NewReader(text))
	if err != nil {
		return ""
	}
	var cells []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "th" || n.Data == "td") {
			var b strings.Builder
			var collect func(*html.Node)
			collect = func(c *html.Node) {
				if c.Type == html.TextNode {
					b.WriteString(c.Data)
				}
				for ch := c.FirstChild; ch != nil; ch = ch.NextSibling {
					collect(ch)
				}
			}
			collect(n)
			cells = append(cells, strings.Join(strings.Fields(b.String()), " "))
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(doc)
	return strings.Join(cells, "\n")
}

// tableSignatures extracts the distinct per-table cell signatures from items.
// A table item is either doc_type_kwd:"table", or a text item that is entirely
// a <table> block (Python's HTML golden keeps the table markup inline as a
// standalone text item). Duplicate signatures are collapsed to a set, because
// Python's Markdown golden keeps BOTH an inline copy and a structured
// doc_type_kwd:"table" item for the same table — we compare DISTINCT tables,
// not raw item counts. The returned set is the unit of Go↔Python table
// equivalence compared by the alignment golden tests.
func tableSignatures(items []map[string]any) map[string]bool {
	sigs := map[string]bool{}
	for _, it := range items {
		kd, _ := it["doc_type_kwd"].(string)
		text, _ := it["text"].(string)
		if kd != "table" && !isStandaloneTable(text) {
			continue
		}
		if sig := tableCellSignature(text); sig != "" {
			sigs[sig] = true
		}
	}
	return sigs
}

// assertTablesEquivalent fails the test if the set of tables (by cell-content
// signature) differs between the Go output and the Python golden. This restores
// the Go↔Python table-equivalence guard that filterTableDivergence intentionally
// removes from the prose comparison: it catches a table being collapsed or a
// column/cell dropped on either side, while tolerating serialization
// differences (attribute order, <tbody> insertion).
func assertTablesEquivalent(t *testing.T, goItems, pyItems []map[string]any) {
	t.Helper()
	goSigs := tableSignatures(goItems)
	pySigs := tableSignatures(pyItems)
	for sig := range goSigs {
		if !pySigs[sig] {
			t.Errorf("Go table not present in Python golden (possible collapse / dropped column):\n%s", sig)
		}
	}
	for sig := range pySigs {
		if !goSigs[sig] {
			t.Errorf("Python golden table not present in Go output (possible collapse / dropped column):\n%s", sig)
		}
	}
}

// MarkdownAlignOptions returns the normalizer preset for Markdown. The order
// matters:
//   - StripMarkdownSyntax first: drops "#"/"-"/fenced-code markup that Python
//     keeps inline but Go parses out (relies on the surrounding newlines, so it
//     must run before CollapseWhitespace).
//   - StripHTMLTags next: replace table/HTML tags with a space (not delete) so
//     adjacent cell text does not fuse, e.g. "<td>A</td><td>B</td>" → "A B".
//   - WithDelimiterStrip: replace the delimiter set Python consumes at split
//     points with a space while Go keeps it inline, so both sides keep the same
//     token separation. Runs before CollapseWhitespace so the introduced space
//     is folded normally.
//   - CollapseWhitespace last: folds all remaining internal whitespace (the
//     inter-tag gaps of an HTML table, the space from delimiter replacement,
//     the newlines inside a fenced code block) into single spaces.
//
// Reused by every Markdown alignment test; other formats define their own
// preset and share CompareAlignment.
func MarkdownAlignOptions(delimiter string) AlignOptions {
	return AlignOptions{
		Normalizers: []Normalizer{
			StripMarkdownSyntax(),
			StripHTMLTags(),
			WithDelimiterStrip(delimiter),
			CollapseWhitespace(),
		},
		ItemKey: "text",
	}
}

// TextCodeAlignOptions returns the normalizer preset for the text&code family.
// Unlike markdown it has no syntax or HTML markup to strip, so only the
// delimiter-set replacement and whitespace collapse run:
//   - WithDelimiterStrip: replace the delimiter runes Python consumes at split
//     points (kept inline on the Go side via keep_delimiters=True) with a space
//     so both sides keep the same token separation.
//   - CollapseWhitespace last: folds the introduced spaces and any inter-segment
//     gaps into single spaces.
//
// Reused by every text&code alignment test; shares CompareAlignment with the
// other format presets (MarkdownAlignOptions).
func TextCodeAlignOptions(delimiter string) AlignOptions {
	return AlignOptions{
		Normalizers: []Normalizer{
			WithDelimiterStrip(delimiter),
			CollapseWhitespace(),
		},
		ItemKey: "text",
	}
}

// htmlHeadingMarkerRE matches a leading ATX heading marker so Python's
// "# Title" (deepdoc merge_block_text prefixes h1–h6 with "# ") can be
// normalized to Go's clean heading text.
var htmlHeadingMarkerRE = regexp.MustCompile(`(?m)^#{1,6}\s+`)

// StripHTMLHeadingMarker returns a Normalizer that removes a leading ATX
// heading marker ("#"/"##"/…) from a line. Python's HTML flow parser
// (deepdoc parser.py merge_block_text) prefixes h1–h6 sections with "# ",
// while the Go HTML parser emits clean heading text. This is a representation
// difference, not a content divergence, so it is stripped before comparing.
// It must run before CollapseWhitespace because the marker relies on the line
// start.
func StripHTMLHeadingMarker() Normalizer {
	return func(s string) string {
		return htmlHeadingMarkerRE.ReplaceAllString(s, "")
	}
}

// HTMLAlignOptions returns the normalizer preset for HTML. Order matters:
//   - StripHTMLHeadingMarker first: drops the "# " Python prefixes from h1–h6
//     sections (relies on the line start, so before CollapseWhitespace).
//   - StripHTMLTags next: replace table/HTML tags with a space (not delete) so
//     adjacent cell text does not fuse, e.g. "<td>A</td><td>B</td>" → "A B".
//   - CollapseWhitespace last: folds all remaining internal whitespace (the
//     inter-tag gaps of a table, the space from any heading-marker removal)
//     into single spaces.
//
// No WithDelimiterStrip: HTML is emitted one item per block (no delimiter
// split), and the Python flow chunks HTML at 512 tokens — boundaries differ,
// but CompareAlignment concatenates normalized text boundary-agnostically, so
// only the concatenated content (order preserved) is compared.
//
// Reused by the HTML alignment test; shares CompareAlignment with the other
// format presets (MarkdownAlignOptions, TextCodeAlignOptions).
func HTMLAlignOptions() AlignOptions {
	return AlignOptions{
		Normalizers: []Normalizer{
			StripHTMLHeadingMarker(),
			StripHTMLTags(),
			CollapseWhitespace(),
		},
		ItemKey: "text",
	}
}

// TestParseGolden locks the two tolerated golden formats (legacy bare array
// and {meta, items}) plus the corrupt-input failure path, so future format
// evolution of the golden files cannot silently change parse behavior.
func TestParseGolden(t *testing.T) {
	// Legacy bare-array format: parsed as the items list, no meta.
	legacy := []byte(`[{"text":"hello"},{"text":"world"}]`)
	doc, err := parseGolden(t, legacy)
	if err != nil {
		t.Fatalf("legacy: parse golden: %v", err)
	}
	if doc.Meta != nil {
		t.Fatalf("legacy: want nil meta, got %v", doc.Meta)
	}
	if len(doc.Items) != 2 {
		t.Fatalf("legacy: want 2 items, got %d", len(doc.Items))
	}
	if got := doc.Items[0]["text"]; got != "hello" {
		t.Fatalf("legacy: item0 text = %v, want hello", got)
	}

	// {meta, items} format: meta preserved, items parsed.
	meta := `{"meta":{"generator":"py","sample":"in.txt","accepted_divergences":["table"]},"items":[{"text":"a"},{"text":"b"}]}`
	doc2, err := parseGolden(t, []byte(meta))
	if err != nil {
		t.Fatalf("{meta,items}: parse golden: %v", err)
	}
	if doc2.Meta == nil {
		t.Fatalf("{meta,items}: want non-nil meta")
	}
	if got := doc2.Meta["generator"]; got != "py" {
		t.Fatalf("{meta,items}: meta.generator = %v, want py", got)
	}
	if len(doc2.Items) != 2 {
		t.Fatalf("{meta,items}: want 2 items, got %d", len(doc2.Items))
	}
	if got := doc2.Items[1]["text"]; got != "b" {
		t.Fatalf("{meta,items}: item1 text = %v, want b", got)
	}

	// Corrupt input must fail loudly: a broken golden should error, not
	// silently fall through to an empty baseline.
	if _, err := parseGolden(t, []byte(`{not valid json`)); err == nil {
		t.Fatalf("corrupt input: expected parseGolden to fail, but it returned")
	}
}
