package parser

import (
	"encoding/json"
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/parser/tableutil"
)

func TestTableRegressionMarkdownMixedBackticksKeepTrailingCells(t *testing.T) {
	for _, cell := range []string{"a\\|b", "`a|b`", "``a|`b``", "`a``|b``", "`a\\`|b`", "a\\\\|b", "``a`|b``", "`a``|b`", "[a|b](u)", ""} {
		for _, prefix := range []string{"", " ", "  ", "   ", "    ", "- "} {
			p, _ := NewMarkdownParser(GoMarkdown)
			res := p.ParseWithResult(t.Context(), "table.md", []byte(prefix+"| H1 | H2 |\n"+prefix+"|---|---|\n"+prefix+"| Owner | "+cell+" | TAIL_SENTINEL |\n"))
			if res.Err != nil {
				t.Fatal(res.Err)
			}
			wire, _ := json.Marshal(res.JSON)
			if !strings.Contains(string(wire), "TAIL_SENTINEL") {
				t.Fatalf("trailing cell dropped for cell=%q prefix=%q: %s", cell, prefix, wire)
			}
		}
	}
}

func TestTableRegressionDOCXCaptionAfterNestedTable(t *testing.T) {
	ir := `{"sections":[{"elements":[{"type":"text_box","content":[{"type":"table","rows":[{"cells":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"Nested"}]}]}]}]}]},{"type":"paragraph","content":[{"type":"text","text":"Outer Caption"}]},{"type":"table","rows":[{"cells":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"Outer"}]}]}]}]}]}]}`

	after := buildDOCXJSONSectionsWithCaptions(ir, nil, []string{"Outer Caption"})
	if len(after) != 2 {
		t.Fatalf("expected two tables without duplicate caption text: %#v", after)
	}
	for _, item := range after {
		if td, ok := item["table"].(*entity.TableData); ok {
			if tableRowsContain(td, "Nested") && td.Caption != "" {
				t.Error("nested table stole the outer caption")
			}
			if tableRowsContain(td, "Outer") && td.Caption != "Outer Caption" {
				t.Error("outer table lost its caption")
			}
		}
	}
}

func TestTableRegressionMarkdownFlattenBesideTable(t *testing.T) {
	source := "![logo](data:image/png;base64,aGVsbG8=) Before <table><tr><td>A</td><td>B</td></tr></table> After"
	p, _ := NewMarkdownParser(GoMarkdown)
	p.FlattenMediaToText = true
	after := p.ParseWithResult(t.Context(), "x.md", []byte(source))
	if after.Err != nil || len(after.JSON) == 0 {
		t.Fatalf("result=%+v", after)
	}
	payloads := 0
	for _, item := range after.JSON {
		if item.ToMap()["image"] != nil {
			payloads++
		}
		if item.ToMap()["doc_type_kwd"] == "image" {
			t.Error("flatten_media_to_text ignored")
		}
	}
	if payloads != 1 {
		t.Fatalf("image payloads=%d, want 1", payloads)
	}
}

func TestTableRegressionPPTXNestedImageRun(t *testing.T) {
	ir := `{"sections":[{"elements":[{"type":"paragraph","content":[{"type":"text","text":"before"},{"type":"image","content":[{"type":"text_box","content":[{"type":"image","data":"aGVsbG8="}]}]},{"type":"text","text":"after"}]}]}]}`

	after, err := buildPPTXJSONSections(ir, nil)
	if err != nil || len(after) != 3 || after[0]["text"] != "before" || after[2]["text"] != "after" {
		t.Fatalf("text/image order lost: %#v err=%v", after, err)
	}
	images := 0
	for _, item := range after {
		if item["doc_type_kwd"] == "image" {
			images++
		}
	}
	if images != 1 {
		t.Error("image inside a nested run was dropped")
	}
}

func TestTableRegressionSoMarkTableImageIDs(t *testing.T) {
	markup := "<table><tr><td>Logo</td><td><img src=\"data:image/png;base64,aGVsbG8=\"></td></tr></table>"
	items, _, _ := soMarkItems(t.Context(), map[string]any{"outputs": map[string]any{"json": map[string]any{"pages": []any{map[string]any{"blocks": []any{
		map[string]any{"type": "table", "content": markup}, map[string]any{"type": "table", "content": markup},
	}}}}}}, false, nil)

	seen := map[string]bool{}
	for _, item := range items {
		if id, ok := item["source_table_id"].(string); ok {
			if seen[id] {
				t.Errorf("duplicate table image association id: %q", id)
			}
			seen[id] = true
		}
	}
	if len(items) != 4 || len(seen) != 2 {
		t.Fatalf("items=%#v", items)
	}
}

func TestTableRegressionSoMarkDocumentImageBudget(t *testing.T) {
	markup := "| Field | Value |\n|---|---|\n| Logo | ![logo](data:image/png;base64,aGVsbG8=) |"
	blocks := make([]any, maxEmbeddedMediaItems+1)
	for i := range blocks {
		blocks[i] = map[string]any{"type": "table", "content": markup}
	}
	items, _, warnings := soMarkItems(t.Context(), map[string]any{"outputs": map[string]any{"json": map[string]any{"pages": []any{map[string]any{"blocks": blocks}}}}}, false, nil)
	images := 0
	for _, item := range items {
		if item["doc_type_kwd"] == "image" && item["image"] != nil {
			images++
		}
	}
	t.Logf("retained images=%d document item limit=%d", images, maxEmbeddedMediaItems)
	if images != maxEmbeddedMediaItems || !strings.Contains(strings.Join(warnings, "\n"), "256-item") {
		t.Errorf("retained=%d warnings=%v, want document-wide item limit", images, warnings)
	}
}

func TestTableRegressionHTMLTheadWithTDRetainsDefaultHeader(t *testing.T) {
	td, err := tableutil.ParseTableHTML("<table><thead><tr><td>Field</td><td>Value</td></tr><tr><td>Subfield</td><td>Subvalue</td></tr></thead><tbody><tr><td>Owner</td><td>RET</td></tr></tbody></table>")
	if err != nil {
		t.Fatal(err)
	}

	if td.HeaderRows != 1 {
		t.Errorf("header_rows=%d, want the default 1", td.HeaderRows)
	}
}

func TestTableRegressionMarkdownRawHTMLImageBudget(t *testing.T) {
	source := "<table><tr><td>" + strings.Repeat("<img src=\"data:image/png;base64,aGVsbG8=\">", maxEmbeddedMediaItems+1) + "</td></tr></table>\n\n"
	doc := markdownNew().Parse([]byte(source))
	budget := newEmbeddedMediaBudget()
	var items []map[string]any
	walkMarkdownBlocksWithImages(t.Context(), doc, &items, false, true, &htmlWalkState{mediaBudget: budget})
	images := 0
	for _, item := range items {
		if item["doc_type_kwd"] == "image" && item["image"] != nil {
			images++
		}
	}
	t.Logf("retained images=%d budget attempts=%d document item limit=%d", images, budget.items, maxEmbeddedMediaItems)
	if images != maxEmbeddedMediaItems || budget.items != maxEmbeddedMediaItems || !budget.countExhausted {
		t.Errorf("raw HTML budget not enforced: retained=%d budget=%+v", images, budget)
	}
}

func TestTableRegressionMarkdownStateResetsForNewDocument(t *testing.T) {
	p, _ := NewMarkdownParser(GoMarkdown)
	source := []byte("| Field | Value |\n|---|---|\n| Logo | ![logo](data:image/png;base64,aGVsbG8=) |")
	for range 2 {
		result := p.ParseWithResult(t.Context(), "doc.md", source)
		if result.Err != nil || len(result.JSON) != 2 {
			t.Fatalf("result=%+v", result)
		}
		table, img := result.JSON[0].ToMap(), result.JSON[1].ToMap()
		if table["source_table_id"] != "html-table-1" || img["parent_table_id"] != table["source_table_id"] {
			t.Fatalf("document state leaked: %#v", ItemsToMaps(result.JSON))
		}
	}
}

func TestMarkdownTableAfterContainerKeepsBlockBoundary(t *testing.T) {
	for _, prefix := range []string{"- Item\n", "> Quote\n"} {
		t.Run(strings.TrimSpace(prefix), func(t *testing.T) {
			p, _ := NewMarkdownParser(GoMarkdown)
			result := p.ParseWithResult(t.Context(), "table.md", []byte(prefix+"| H | V |\n|---|---|\n| OWNER | TAIL |\n\nAfter."))
			if result.Err != nil || len(result.JSON) != 3 {
				t.Fatalf("result=%+v", result)
			}
			wantPrefix := strings.TrimSpace(prefix)
			if strings.HasPrefix(wantPrefix, "> ") {
				wantPrefix = strings.TrimPrefix(wantPrefix, "> ")
			}
			if result.JSON[0].ToMap()["text"] != wantPrefix || result.JSON[2].ToMap()["text"] != "After." {
				t.Fatalf("surrounding text changed: %#v", ItemsToMaps(result.JSON))
			}
			requireTableRows(t, result.JSON[1].ToMap(), [][]string{{"H", "V"}, {"OWNER", "TAIL"}})
		})
	}
}

func TestMarkdownTableHookPreservesCodeAndHTMLBlocks(t *testing.T) {
	for _, source := range []string{
		"    intro\n    | H | V |\n    |---|---|\n    | x | y | extra |\n",
		"\tintro\n\t| H | V |\n\t|---|---|\n\t| x | y | extra |\n",
		"```\nintro\n| H | V |\n|---|---|\n| x | y | extra |\n```\n",
		"<pre>\nintro\n| H | V |\n|---|---|\n| x | y | extra |\n</pre>\n",
		"<div>\nintro\n| H | V |\n|---|---|\n| x | y | extra |\n</div>\n",
		"<div>| H |\n|---|---|\n| x | y | extra |\n</div>\n",
		"<div>\n<div>inner</div>\n| H | V |\n|---|---|\n| x | y | extra |\n</div>\n",
		"<!--\nintro\n| H | V |\n|---|---|\n| x | y | extra |\n-->\n",
		"- Item\n\n    ```\n    | H | V |\n    |---|---|\n    | x | y | extra |\n    ```\n",
		"- Item\n\n      | H | V |\n      |---|---|\n      | x | y | extra |\n",
		"1. Item\n\n       | H | V |\n       |---|---|\n       | x | y | extra |\n",
		"| H | V |\n    |---|---|\n    | x | y | extra |\n",
	} {
		p, _ := NewMarkdownParser(GoMarkdown)
		result := p.ParseWithResult(t.Context(), "blocks.md", []byte(source))
		if result.Err != nil {
			t.Fatal(result.Err)
		}
		for _, item := range result.JSON {
			if item.ToMap()["doc_type_kwd"] == "table" {
				t.Fatalf("code or HTML content became a table: %q", source)
			}
		}
		wire, _ := json.Marshal(result.JSON)
		if !strings.HasPrefix(source, "<!--") && !strings.Contains(string(wire), "| x | y | extra |") {
			t.Fatalf("protected content lost: %s", wire)
		}
		if strings.HasPrefix(source, "<") && !strings.HasPrefix(source, "<!--") && (len(result.JSON) != 1 || result.JSON[0].ToMap()["text"] != strings.TrimSpace(source)) {
			t.Fatalf("raw HTML changed: %s", wire)
		}
	}
}

func TestMarkdownTableHookBalancesIndentedListTable(t *testing.T) {
	for _, tc := range []struct{ marker, indent string }{
		{"- ", "  "}, {"1. ", "   "}, {"10. ", "    "}, {"- ", "    "}, {"- ", "\t"},
	} {
		t.Run(tc.marker+tc.indent, func(t *testing.T) {
			for _, wide := range []bool{false, true} {
				row := "| x | y |"
				want := [][]string{{"H", "V"}, {"x", "y"}}
				if wide {
					row = "| x | y | TAIL |"
					want = [][]string{{"H", "V", ""}, {"x", "y", "TAIL"}}
				}
				source := tc.marker + "Item\n\n" + tc.indent + "| H | V |\n" + tc.indent + "|---|---|\n" + tc.indent + row + "\n"
				p, _ := NewMarkdownParser(GoMarkdown)
				result := p.ParseWithResult(t.Context(), "list.md", []byte(source))
				if result.Err != nil || len(result.JSON) != 2 || result.JSON[0].ToMap()["text"] != tc.marker+"Item" {
					t.Fatalf("list content lost: %#v err=%v", ItemsToMaps(result.JSON), result.Err)
				}
				if result.JSON[0].ToMap()["layout"] != "list" {
					t.Fatalf("list layout lost: %#v", result.JSON[0].ToMap())
				}
				requireTableRows(t, result.JSON[1].ToMap(), want)
			}
		})
	}
}

func TestMarkdownWideTableAtQuoteDepthLimit(t *testing.T) {
	prefix := strings.Repeat("> ", 63)
	source := prefix + "| H | V |\n" + prefix + "|---|---|\n" + prefix + "| x | y | TAIL |\n"
	p, _ := NewMarkdownParser(GoMarkdown)
	result := p.ParseWithResult(t.Context(), "nested.md", []byte(source))
	if result.Err != nil || len(result.JSON) != 1 {
		t.Fatalf("result=%+v", result)
	}
	requireTableRows(t, result.JSON[0].ToMap(), [][]string{{"H", "V", ""}, {"x", "y", "TAIL"}})
}

func TestMarkdownTableInNestedListPreservesReferences(t *testing.T) {
	source := "- Outer\n\n    - Inner\n\n      | A | B |\n      |---|---|\n      | [team][ref] | y | TAIL |\n\n[ref]: https://example.org\n"
	p, _ := NewMarkdownParser(GoMarkdown)
	result := p.ParseWithResult(t.Context(), "nested-list.md", []byte(source))
	if result.Err != nil || len(result.JSON) != 3 {
		t.Fatalf("result=%#v err=%v", ItemsToMaps(result.JSON), result.Err)
	}
	if result.JSON[0].ToMap()["text"] != "- Outer" || result.JSON[1].ToMap()["text"] != "- Inner" {
		t.Fatalf("list text lost: %#v", ItemsToMaps(result.JSON))
	}
	requireTableRows(t, result.JSON[2].ToMap(), [][]string{{"A", "B", ""}, {"team", "y", "TAIL"}})
}

func TestMarkdownTableHookBalancesMarkdownInsideHTMLContainer(t *testing.T) {
	source := "<div>\n\n| H | V |\n|---|---|\n| x | y | TAIL |\n\n</div>\n"
	p, _ := NewMarkdownParser(GoMarkdown)
	result := p.ParseWithResult(t.Context(), "table.md", []byte(source))
	for _, item := range result.JSON {
		if item.ToMap()["doc_type_kwd"] == "table" {
			requireTableRows(t, item.ToMap(), [][]string{{"H", "V", ""}, {"x", "y", "TAIL"}})
			return
		}
	}
	t.Fatalf("missing table in Markdown-enabled HTML container: %+v", result)
}

func TestMarkdownTableHookResumesAfterProtectedBlock(t *testing.T) {
	source := "    code\n    | H | V |\n    |---|---|\n    | x | y |\n\n| H | V |\n|---|---|\n| x | y | TAIL |\n"
	p, _ := NewMarkdownParser(GoMarkdown)
	result := p.ParseWithResult(t.Context(), "table.md", []byte(source))
	if result.Err != nil || len(result.JSON) != 2 || result.JSON[0].ToMap()["text"] != "code\n| H | V |\n|---|---|\n| x | y |" {
		t.Fatalf("code block changed: %+v", result)
	}
	requireTableRows(t, result.JSON[1].ToMap(), [][]string{{"H", "V", ""}, {"x", "y", "TAIL"}})
}

func TestMarkdownTableHookResumesAfterHTMLComments(t *testing.T) {
	source := "<div>\n<!-- <div> is an example -->\n</div>\n\n| H | V |\n|---|---|\n| x | y | TAIL |\n"
	p, _ := NewMarkdownParser(GoMarkdown)
	result := p.ParseWithResult(t.Context(), "table.md", []byte(source))
	for _, item := range result.JSON {
		if item.ToMap()["doc_type_kwd"] == "table" {
			requireTableRows(t, item.ToMap(), [][]string{{"H", "V", ""}, {"x", "y", "TAIL"}})
			return
		}
	}
	t.Fatalf("table after HTML block lost: %+v", result)
}

func TestDOCXContainerHeadingDoesNotReplaceBodyHeading(t *testing.T) {
	table := `{"type":"table","rows":[{"cells":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"value"}]}]}]}]}`
	for _, container := range []string{"text_box", "list"} {
		t.Run(container, func(t *testing.T) {
			content := `{"type":"heading","level":1,"content":[{"type":"text","text":"Sidebar"}]},` + table
			wrapped := `{"type":"text_box","content":[` + content + `]}`
			if container == "list" {
				wrapped = `{"type":"list","items":[{"content":[` + content + `]}]}`
			}
			ir := `{"sections":[{"elements":[{"type":"heading","level":1,"content":[{"type":"text","text":"Chapter"}]},` + wrapped + `,` + table + `]}]}`
			items := buildDOCXJSONSections(ir, nil)
			var captions []string
			for _, item := range items {
				if td, ok := item["table"].(*entity.TableData); ok {
					captions = append(captions, td.Caption)
				}
			}
			if len(captions) != 2 || captions[0] != "Table Location: Sidebar" || captions[1] != "Table Location: Chapter" {
				t.Fatalf("table captions=%q", captions)
			}
		})
	}
}

func TestSoMarkPreservesImageResolutionWarnings(t *testing.T) {
	source := `<table><tr><td><img alt="logo" src="data:image/png;base64,!!!"></td></tr></table>`
	items, _, warnings := soMarkItems(t.Context(), map[string]any{"outputs": map[string]any{"json": map[string]any{"pages": []any{map[string]any{"blocks": []any{
		map[string]any{"type": "table", "content": source},
		map[string]any{"type": "table", "content": "| A | B |\n|---|---|\n| x | y |"},
	}}}}}}, false, nil)
	if len(items) != 2 {
		t.Fatalf("items=%#v", items)
	}
	requireTableRows(t, items[0], [][]string{{"logo"}})
	if !strings.Contains(strings.Join(warnings, "\n"), "could not resolve 1 image source") {
		t.Fatalf("missing image resolution warning: %v", warnings)
	}
}
