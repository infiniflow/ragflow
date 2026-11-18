package parser

import "testing"

// Large TOC-heavy input: O(k*n) append-delete in the old loop makes this
// quadratic as the number of dropped entries scales.
func tocHeavyItems() []map[string]any {
	items := []map[string]any{{"text": "前言"}}
	items = append(items, map[string]any{"text": "目录"})
	for i := 0; i < 600; i++ {
		items = append(items, map[string]any{"text": "第一章 小节 " + string(rune('A'+i%26))})
	}
	for i := 0; i < 200; i++ {
		items = append(items, map[string]any{"text": "正文段落 " + string(rune('a'+i%26))})
	}
	return items
}

func tocDegenerateItems() []map[string]any {
	items := []map[string]any{{"text": "目录"}}
	// eng mode: empty text -> empty prefix -> the `for prefix == ""` loop
	// append-deletes every one of these, O(k*n).
	for i := 0; i < 400; i++ {
		items = append(items, map[string]any{"text": "   "})
	}
	items = append(items, map[string]any{"text": "Body"})
	return items
}

func BenchmarkRemoveContentsTableDegenerate(b *testing.B) {
	base := tocDegenerateItems()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		items := append([]map[string]any(nil), base...)
		removeContentsTable(items, true, plainTOCRules)
	}
}

func BenchmarkRemoveContentsTableLarge(b *testing.B) {
	base := tocHeavyItems()
	b.ReportAllocs()
	// Clone the slice header each iteration (cheap) so the benchmark measures
	// removeContentsTable itself, not repeated construction of 800 maps.
	for i := 0; i < b.N; i++ {
		items := append([]map[string]any(nil), base...)
		removeContentsTable(items, false, plainTOCRules)
	}
}
