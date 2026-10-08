package wiki

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSummarizeMarkdown(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "empty", content: " \n\n", want: ""},
		{name: "only headings", content: "# Title\n\n## Details", want: ""},
		{name: "plain text", content: "# Title\n\n齿轮是钟楼的零件。", want: "齿轮是钟楼的零件。"},
		{
			name:    "wiki link and emphasis",
			content: "# 齿轮\n\n**齿轮**是[海雾镇](artifact/b157a72656fa4d61b6d5ad49a002a3d2/entity/location/海雾镇)钟楼的*核心*零件。",
			want:    "齿轮是海雾镇钟楼的核心零件。",
		},
		{name: "nested link label", content: "See [**the _map_**](https://example.com/a_(b)) now.", want: "See the map now."},
		{name: "reference link", content: "See [the map][map].\n\n[map]: artifact/kb/entity/map", want: "See the map."},
		{name: "inline code", content: "Use `content_with_weight` and ~~old~~ new text.", want: "Use content_with_weight and old new text."},
		{name: "literal punctuation", content: `Keep a_b, [brackets], 2 * 3 and \*literal\*.`, want: "Keep a_b, [brackets], 2 * 3 and *literal*."},
		{name: "entities and escapes", content: `&amp; \&amp; &#38;amp;`, want: "& &amp; &amp;"},
		{name: "code stays literal", content: "Keep `<b> &amp; \\*literal\\*`.", want: "Keep <b> &amp; \\*literal\\*."},
		{name: "escaped HTML stays literal", content: `Keep \<b\> and &lt;b&gt;.`, want: "Keep <b> and <b>."},
		{name: "inline HTML", content: "<span>First</span><br>second.", want: "First second."},
		{name: "HTML block", content: "<div><b>First</b> paragraph.<br>More.</div>", want: "First paragraph. More."},
		{name: "HTML block boundaries", content: "<section>First</section><section>Second</section>", want: "First Second"},
		{name: "autolink labels", content: "Visit <https://example.com> or <reader@example.com>.", want: "Visit https://example.com or reader@example.com."},
		{name: "image alt text", content: "See ![the **map**](artifact/kb/map.png).", want: "See the map."},
		{name: "first paragraph", content: "# Title\n\nFirst line\nsecond line.\n\nOther paragraph.", want: "First line second line."},
		{name: "list", content: "# Title\n\n- **First** item\n- Second item", want: "First item"},
		{name: "quote", content: "# Title\n\n> **First** paragraph.\n\nOther paragraph.", want: "First paragraph."},
		{name: "code block", content: "# Title\n\n```go\nvalue := \"<b>\"\n```", want: `value := "<b>"`},
		{name: "table", content: "# Title\n\n| Name | Value |\n| --- | --- |\n| **Map** | [Town](artifact/kb/entity/town) |", want: "Name Value Map Town"},
		{name: "skip non-text blocks", content: "# Title\n\n---\n\n<!-- comment -->\n\n**Body**.", want: "Body."},
		{name: "skip script", content: "<script>ignored()</script>\n\n**Body**.", want: "Body."},
		{name: "skip style", content: "<style>.ignored { color: red; }</style>\n\n**Body**.", want: "Body."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := summarizeMarkdown(tt.content, ""); got != tt.want {
				t.Fatalf("summarizeMarkdown() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSummarizeMarkdownTruncatesAfterRemovingLinks(t *testing.T) {
	content := "**齿轮**是[海雾镇](artifact/" + strings.Repeat("a", 400) + "/entity/location/海雾镇)钟楼的零件。"
	if got := summarizeMarkdown(content, ""); got != "齿轮是海雾镇钟楼的零件。" {
		t.Fatalf("summary = %q, want the complete visible text", got)
	}

	for _, runeText := range []string{"中", "🔔"} {
		t.Run(runeText, func(t *testing.T) {
			prefix := strings.Repeat("a", 299)
			got := summarizeMarkdown(prefix+runeText+" tail", "")
			if got != prefix || !utf8.ValidString(got) {
				t.Fatalf("summary = %q, want a valid UTF-8 prefix at the 300-byte limit", got)
			}
		})
	}
	if got := summarizeMarkdown(strings.Repeat("中", 101), ""); got != strings.Repeat("中", 100) {
		t.Fatalf("Chinese summary length = %d bytes, want 300", len(got))
	}
	if got := summarizeMarkdown(strings.Repeat("a", 301), ""); len(got) != 300 {
		t.Fatalf("ASCII summary length = %d bytes, want 300", len(got))
	}
}

func TestSummarizeMarkdownTitleDoesNotRestoreHiddenContent(t *testing.T) {
	for _, title := range []string{
		`<img src="artifact/kb/entity/gear" alt="">`,
		"<script>ignored()</script>",
		"<style>.ignored { color: red; }</style>",
	} {
		t.Run(title, func(t *testing.T) {
			if got := summarizeMarkdown("", title); got != "" {
				t.Fatalf("summary = %q, want empty text for a title with no visible content", got)
			}
		})
	}
}

func TestBuildWikiPageProductsGeneratesPlainTextSummary(t *testing.T) {
	const body = "# 齿轮\n\n**齿轮**是[海雾镇](artifact/kb/entity/location/海雾镇)钟楼的零件。\n\n## Details\n\nSee `a_b`."
	for _, rawOnly := range []bool{false, true} {
		page := wikiPageResult{
			Slug:       "entity/equipment/齿轮",
			Title:      "齿轮",
			PageType:   "entity",
			Topic:      "Clock/Parts",
			ContentRaw: body,
			Outlinks:   []string{"entity/location/海雾镇"},
		}
		if !rawOnly {
			page.Content = body
		}
		products := buildWikiPageProducts("tenant", "doc", []wikiPageResult{page})
		if len(products) == 0 {
			t.Fatal("no page product")
		}
		product := products[0]
		if got := product.Meta["summary"]; got != "齿轮是海雾镇钟楼的零件。" {
			t.Fatalf("summary = %q, want plain text", got)
		}
		if product.Content != body || product.Meta["content_md_raw"] != body {
			t.Fatal("summary generation changed the Markdown body")
		}
		if product.Meta["topic"] != "Clock/Parts" {
			t.Fatalf("topic = %q, want unchanged topic", product.Meta["topic"])
		}
		links := product.Meta["outlinks"].([]string)
		if len(links) != 1 || links[0] != "entity/location/海雾镇" {
			t.Fatalf("outlinks = %#v, want unchanged outlinks", links)
		}
	}
}

func TestBuildWikiPageProductsSummaryFallsBackToTitle(t *testing.T) {
	for _, title := range []string{"**齿轮**", "# 齿轮", "---"} {
		t.Run(title, func(t *testing.T) {
			products := buildWikiPageProducts("tenant", "doc", []wikiPageResult{{
				Slug:    "concept/gear",
				Title:   title,
				Content: "# 齿轮\n\n<!-- no body -->",
			}})
			want := "齿轮"
			if title == "---" {
				want = title
			}
			if len(products) == 0 || products[0].Meta["summary"] != want {
				t.Fatalf("products = %#v, want plain-text title summary %q", products, want)
			}
		})
	}
}
