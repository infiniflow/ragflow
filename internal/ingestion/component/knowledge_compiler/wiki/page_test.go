package wiki

import (
	"strings"
	"testing"
)

func TestParseOutline_Basic(t *testing.T) {
	md := `# Main Title

Preamble paragraph that introduces the page.

## Background

This is the background section.

## Methods

This is the methods section.

### Subsection

Deeper detail.

## Conclusion

Final thoughts.`
	sections := ParseOutline(md)
	if len(sections) != 5 {
		t.Fatalf("ParseOutline produced %d sections, want 5", len(sections))
	}
	titles := []string{"Main Title", "Background", "Methods", "Subsection", "Conclusion"}
	levels := []int{1, 2, 2, 3, 2}
	for i, sec := range sections {
		if sec.Title != titles[i] {
			t.Errorf("section %d title = %q, want %q", i, sec.Title, titles[i])
		}
		if sec.Level != levels[i] {
			t.Errorf("section %d level = %d, want %d", i, sec.Level, levels[i])
		}
		if strings.TrimSpace(sec.Content) == "" {
			t.Errorf("section %d content is empty", i)
		}
	}
}

func TestParseOutline_Empty(t *testing.T) {
	if got := ParseOutline(""); len(got) != 0 {
		t.Fatalf("ParseOutline(\"\") = %d sections, want 0", len(got))
	}
	if got := ParseOutline("   \n\n  \n"); len(got) != 0 {
		t.Fatalf("ParseOutline(whitespace) = %d sections, want 0", len(got))
	}
}

func TestParseOutline_PreambleOnly(t *testing.T) {
	// Markdown with no headings at all: the prose still becomes a synthetic
	// level-0 section so the page body is not silently dropped.
	sections := ParseOutline("Just some prose, no headings here.")
	if len(sections) != 1 {
		t.Fatalf("preamble-only produced %d sections, want 1", len(sections))
	}
	if sections[0].Level != 0 || sections[0].Title != "" {
		t.Errorf("synthetic preamble = level=%d title=%q, want level=0 title=\"\"", sections[0].Level, sections[0].Title)
	}
	if !strings.Contains(sections[0].Content, "Just some prose") {
		t.Errorf("synthetic preamble content missing prose: %q", sections[0].Content)
	}
}

func TestParseOutline_StripsEmphasis(t *testing.T) {
	md := "## **Bold heading**"
	sections := ParseOutline(md)
	if len(sections) != 1 || sections[0].Title != "Bold heading" {
		t.Fatalf("emphasis strip: got %#v, want title \"Bold heading\"", sections)
	}
}

func TestBuildPageProducts_PageAndSections(t *testing.T) {
	md := `# The Page Title

Lead paragraph.

## Section A

A body.

## Section B

B body.

### Subsection B1

Deeper.`
	products := buildPageProducts("t1", "d1", md, []string{"c1", "c2"})
	if len(products) < 2 {
		t.Fatalf("buildPageProducts = %d products, want page + 3 sections (>= 4)", len(products))
	}
	// First must be the page itself.
	page := products[0]
	if kind, _ := page.Meta["kind"].(string); kind != "page" {
		t.Fatalf("products[0].kind = %q, want \"page\"", page.Meta["kind"])
	}
	if title, _ := page.Meta["title"].(string); title != "The Page Title" {
		t.Errorf("page title = %q, want \"The Page Title\"", title)
	}
	if slug, _ := page.Meta["slug"].(string); slug != "the-page-title" {
		t.Errorf("page slug = %q, want \"the-page-title\"", slug)
	}
	if ids, _ := page.Meta["source_chunk_ids"].([]string); len(ids) != 2 {
		t.Errorf("page source_chunk_ids = %#v, want 2 ids", page.Meta["source_chunk_ids"])
	}
	// Sections are linked via parent_id = page.ID.
	for i, p := range products[1:] {
		if p.ParentID != page.ID {
			t.Errorf("section[%d].parent_id = %q, want %q", i, p.ParentID, page.ID)
		}
		if kind, _ := p.Meta["kind"].(string); kind != "section" {
			t.Errorf("section[%d].kind = %q, want \"section\"", i, p.Meta["kind"])
		}
		if _, has := p.Meta["section_level"]; !has {
			t.Errorf("section[%d] missing section_level meta", i)
		}
		if ids, _ := p.Meta["source_chunk_ids"].([]string); len(ids) != 2 {
			t.Errorf("section[%d] source_chunk_ids = %#v, want 2 ids", i, p.Meta["source_chunk_ids"])
		}
		if strings.TrimSpace(p.Content) == "" {
			t.Errorf("section[%d] content is empty", i)
		}
	}
}

func TestBuildPageProducts_NoSections(t *testing.T) {
	// A page that contains only preamble: still emits a page product, with
	// zero section children (the preamble is rolled into the page summary).
	products := buildPageProducts("t1", "d1", "Just prose, no headings.", []string{"c1"})
	if len(products) != 1 {
		t.Fatalf("preamble-only page = %d products, want 1", len(products))
	}
	if kind, _ := products[0].Meta["kind"].(string); kind != "page" {
		t.Errorf("kind = %q, want page", products[0].Meta["kind"])
	}
}

func TestBuildPageProducts_StableIDs(t *testing.T) {
	md := "# Title\n\n## A\n\nbody\n"
	first := buildPageProducts("t1", "d1", md, []string{"c1"})
	second := buildPageProducts("t1", "d1", md, []string{"c1"})
	if len(first) != len(second) {
		t.Fatalf("length mismatch: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Errorf("product[%d] id not stable: %q vs %q", i, first[i].ID, second[i].ID)
		}
	}
}

func TestBuildWikiPageProducts_MultiPage(t *testing.T) {
	pages := []wikiPageResult{
		{
			Slug:           "entity/alpha",
			Title:          "Alpha",
			PageType:       "entity",
			ContentRaw:     "# Alpha\n\nSee [[concept/beta]].\n\n## Details\n\nAlpha body.",
			Content:        "# Alpha\n\nSee [Beta](artifact/kb/concept/beta).\n\n## Details\n\nAlpha body.",
			SourceChunkIDs: []string{"c1"},
			SourceDocIDs:   []string{"d1"},
			Outlinks:       []string{"concept/beta"},
		},
		{
			Slug:           "concept/beta",
			Title:          "Beta",
			PageType:       "concept",
			ContentRaw:     "# Beta\n\nBody.",
			Content:        "# Beta\n\nBody.",
			SourceChunkIDs: []string{"c2"},
			SourceDocIDs:   []string{"d1"},
		},
	}
	products := buildWikiPageProducts("t1", "d1", pages)
	if len(products) != 5 {
		t.Fatalf("buildWikiPageProducts = %d products, want 5", len(products))
	}
	page := products[0]
	if kind, _ := page.Meta["kind"].(string); kind != "page" {
		t.Fatalf("page kind = %q, want page", page.Meta["kind"])
	}
	if outlinks, _ := page.Meta["outlinks"].([]string); len(outlinks) != 1 || outlinks[0] != "concept/beta" {
		t.Fatalf("page outlinks = %#v, want concept/beta", page.Meta["outlinks"])
	}
	if ids, _ := page.Meta["source_chunk_ids"].([]string); len(ids) != 1 || ids[0] != "c1" {
		t.Fatalf("page source_chunk_ids = %#v, want c1", page.Meta["source_chunk_ids"])
	}
	if ids, _ := page.Meta["source_doc_ids"].([]string); len(ids) != 1 || ids[0] != "d1" {
		t.Fatalf("page source_doc_ids = %#v, want d1", page.Meta["source_doc_ids"])
	}
}

func TestTransformWikiLinks(t *testing.T) {
	rendered, outlinks := transformWikiLinks(
		"See [[concept/beta|Beta]] and [[entity/alpha]]. Also [Beta](artifact/kb/concept/beta).",
		"kb",
		map[string]string{"entity/alpha": "Alpha", "concept/beta": "Beta"},
	)
	if !strings.Contains(rendered, "(artifact/kb/concept/beta)") || !strings.Contains(rendered, "(artifact/kb/entity/alpha)") {
		t.Fatalf("rendered links not rewritten: %q", rendered)
	}
	if len(outlinks) != 2 {
		t.Fatalf("outlinks = %#v, want 2 unique slugs", outlinks)
	}
}

func TestSlugify(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Hello World", "hello-world"},
		{"  Trim Me  ", "trim-me"},
		{"Punctuation!@#Stripped", "punctuationstripped"},
		{"", "page"},
		// Non-ASCII (CJK) letters are preserved (M17), so the slug keeps the
		// Chinese characters and the trailing ASCII word.
		{"中文 title", "中文-title"},
		{"a-b-c", "a-b-c"},
		// Punctuation-only title yields a hash fallback, not the bare "page".
		{"!@#", "page-8b024aa5"},
	}
	for _, c := range cases {
		if got := slugify(c.in); got != c.want {
			t.Errorf("slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
