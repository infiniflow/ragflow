//go:build !race

package parser

import (
	"context"
	"testing"
)

// Baseline allocates 4582 per parse for a 20-table document; reusing the
// renderer removes roughly 15 per table. The budget is deliberately loose to
// absorb allocator drift across Go versions; BenchmarkMarkdownTableRender
// records the exact figure and is the precise regression guard. This test is
// excluded under -race, whose instrumentation inflates allocation counts.
const rendererAllocBudget = 4500.0

// TestMarkdownRendererReuse_Allocations guards the per-table render path: a
// renderer must be reused instead of constructed fresh for every table.
func TestMarkdownRendererReuse_Allocations(t *testing.T) {
	data := buildTablesMarkdown(20)
	ctx := context.Background()
	p, _ := NewMarkdownParser(GoMarkdown)
	allocs := testing.AllocsPerRun(20, func() {
		p.ParseWithResult(ctx, "tables.md", data)
	})
	if allocs > rendererAllocBudget {
		t.Fatalf("allocs per parse = %f, want <= %f (renderer reused across tables)", allocs, rendererAllocBudget)
	}
}
