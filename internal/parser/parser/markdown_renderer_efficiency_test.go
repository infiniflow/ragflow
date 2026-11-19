package parser

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"ragflow/internal/entity"
)

// baselineTableSummary is the exact parse output captured before the fix.
const baselineTableSummary = "[<nil>|[[h0 v0] [a0 b0]]][<nil>|[[h1 v1] [a1 b1]]][<nil>|[[h2 v2] [a2 b2]]][<nil>|[[h3 v3] [a3 b3]]][<nil>|[[h4 v4] [a4 b4]]][<nil>|[[h5 v5] [a5 b5]]][<nil>|[[h6 v6] [a6 b6]]][<nil>|[[h7 v7] [a7 b7]]][<nil>|[[h8 v8] [a8 b8]]][<nil>|[[h9 v9] [a9 b9]]][<nil>|[[h10 v10] [a10 b10]]][<nil>|[[h11 v11] [a11 b11]]][<nil>|[[h12 v12] [a12 b12]]][<nil>|[[h13 v13] [a13 b13]]][<nil>|[[h14 v14] [a14 b14]]][<nil>|[[h15 v15] [a15 b15]]][<nil>|[[h16 v16] [a16 b16]]][<nil>|[[h17 v17] [a17 b17]]][<nil>|[[h18 v18] [a18 b18]]][<nil>|[[h19 v19] [a19 b19]]]"

func buildTablesMarkdown(n int) []byte {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "| h%d | v%d |\n| --- | --- |\n| a%d | b%d |\n\n", i, i, i, i)
	}
	return []byte(b.String())
}

// TestMarkdownRendererReuse_Concurrent verifies the renderer pool never shares
// one instance across concurrent parses. Run under -race to catch data races.
func TestMarkdownRendererReuse_Concurrent(t *testing.T) {
	data := buildTablesMarkdown(8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, _ := NewMarkdownParser(GoMarkdown)
			for j := 0; j < 20; j++ {
				res := p.ParseWithResult(context.Background(), "tables.md", data)
				if res.Err != nil {
					t.Errorf("concurrent parse error: %v", res.Err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

// BenchmarkMarkdownTableRender guards the per-table render allocation count.
func BenchmarkMarkdownTableRender(b *testing.B) {
	data := buildTablesMarkdown(20)
	p, _ := NewMarkdownParser(GoMarkdown)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.ParseWithResult(ctx, "tables.md", data)
	}
}

// TestMarkdownRendererReuse_OutputPreserved pins the parse output for a
// multi-table document so the renderer reuse cannot change the result shape.
func TestMarkdownRendererReuse_OutputPreserved(t *testing.T) {
	p, _ := NewMarkdownParser(GoMarkdown)
	res := p.ParseWithResult(context.Background(), "tables.md", buildTablesMarkdown(20))
	if res.Err != nil {
		t.Fatalf("parse error: %v", res.Err)
	}
	maps := ItemsToMaps(res.JSON)
	if len(maps) != 20 {
		t.Fatalf("len(JSON) = %d, want 20 (one item per table)", len(maps))
	}
	var b strings.Builder
	for i, item := range maps {
		if item["doc_type_kwd"] != "table" {
			t.Fatalf("item %d doc_type_kwd = %v, want table", i, item["doc_type_kwd"])
		}
		td, ok := item["table"].(*entity.TableData)
		if !ok || td == nil {
			t.Fatalf("item %d missing *entity.TableData", i)
		}
		fmt.Fprintf(&b, "[%v|%v]", item["text"], td.CellTexts())
	}
	if got := b.String(); got != baselineTableSummary {
		t.Fatalf("parse output changed:\n got: %s\nwant: %s", got, baselineTableSummary)
	}
}
