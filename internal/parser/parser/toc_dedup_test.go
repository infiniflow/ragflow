package parser

import "testing"

// Large TOC-heavy input: multiple TOC sections, each with non-matching gap
// entries between the first prefix-bearing entry and a later match, so the
// range deletion [i, j) is non-empty and repeated — the O(k*n) case the old
// append-delete loop paid for.
func tocHeavyItems() []map[string]any {
	items := []map[string]any{{"text": "前言"}}
	for section := 0; section < 40; section++ {
		items = append(items, map[string]any{"text": "目录"})
		prefix := "第一章 小节 " + string(rune('A'+section%26))
		items = append(items, map[string]any{"text": prefix + " 首条"})
		// Non-matching gap entries: the range [i, j) between the first entry
		// and a later match is deleted in one append-delete in the old code.
		for g := 0; g < 12; g++ {
			items = append(items, map[string]any{"text": "杂项 " + string(rune('a'+g%26))})
		}
		items = append(items, map[string]any{"text": prefix + " 末条"})
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
	work := make([]map[string]any, len(base))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Restore the work slice from base outside the timed region so the
		// benchmark measures removeContentsTable, not reset allocation.
		b.StopTimer()
		copy(work, base)
		b.StartTimer()
		removeContentsTable(work, true)
	}
}

func BenchmarkRemoveContentsTableLarge(b *testing.B) {
	base := tocHeavyItems()
	work := make([]map[string]any, len(base))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		copy(work, base)
		b.StartTimer()
		removeContentsTable(work, false)
	}
}

// F4: after compaction, removed item maps must not stay reachable through the
// backing array beyond the returned slice length (GC retention).
func TestRemoveContentsTableClearsTail(t *testing.T) {
	items := []map[string]any{
		{"text": "前言"},
		{"text": "目录"},
		{"text": "第一章 概述"},
		{"text": "第一章 方法"},
		{"text": "正文"},
	}
	out := removeContentsTable(items, false)
	w := len(out)
	// `items` shares the backing array; positions [w:] were removed and must
	// be cleared so the dropped maps are not retained by the backing array.
	for i := w; i < len(items); i++ {
		if items[i] != nil {
			t.Errorf("backing array index %d still retains a removed item: %v", i, items[i])
		}
	}
}

// F3: a document with no TOC heading should not pay for a deletion bitmap or
// a full-slice self-copy. Benchmark guards the allocation cost.
func tocNoHeadingItems() []map[string]any {
	items := make([]map[string]any, 400)
	for i := range items {
		items[i] = map[string]any{"text": "正文段落 " + string(rune('a'+i%26))}
	}
	return items
}

func BenchmarkRemoveContentsTableNoHeading(b *testing.B) {
	base := tocNoHeadingItems()
	work := make([]map[string]any, len(base))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		copy(work, base)
		b.StartTimer()
		removeContentsTable(work, false)
	}
}
