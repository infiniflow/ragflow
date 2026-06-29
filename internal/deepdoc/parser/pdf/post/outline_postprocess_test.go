package post

import (
	"context"
	"testing"

	pdftype "ragflow/internal/deepdoc/parser/pdf/type"
)

// ── Tests for remove_toc config flag ────────────────────────────────────────

// TestPostProcess_RemoveTOC_DisabledByConfig verifies that when
// remove_toc=false, outlines are NOT used to remove TOC pages even
// when outlines are present.
func TestPostProcess_RemoveTOC_DisabledByConfig(t *testing.T) {
	result := newTestResult(
		makePosSection("目录内容 page1", 1, 100, 500, 100, 200),
		makePosSection("更多目录 page2", 2, 100, 500, 100, 200),
		makePosSection("第一章 正文", 3, 100, 500, 100, 200),
		makePosSection("第二章 正文", 5, 100, 500, 100, 200),
	)
	outlines := []pdftype.Outline{
		{Title: "目录", Level: 0, PageNumber: 1},
		{Title: "第一章", Level: 0, PageNumber: 3},
		{Title: "第二章", Level: 0, PageNumber: 5},
	}

	config := PipelineConfig{
		ConfigKeyRemoveTOC: false,
		ConfigKeyOutlines:  outlines,
	}
	err := PostProcess(context.Background(), result, config)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Sections) != 4 {
		t.Errorf("remove_toc=false should keep all sections, got %d", len(result.Sections))
	}
}

// TestPostProcess_RemoveTOC_EnabledByConfig verifies that when
// remove_toc=true and outlines are present, TOC pages are removed.
func TestPostProcess_RemoveTOC_EnabledByConfig(t *testing.T) {
	result := newTestResult(
		makePosSection("目录内容 page1", 1, 100, 500, 100, 200),
		makePosSection("更多目录 page2", 2, 100, 500, 100, 200),
		makePosSection("第一章 正文", 3, 100, 500, 100, 200),
		makePosSection("第二章 正文", 5, 100, 500, 100, 200),
	)
	outlines := []pdftype.Outline{
		{Title: "目录", Level: 0, PageNumber: 1},
		{Title: "第一章", Level: 0, PageNumber: 3},
		{Title: "第二章", Level: 0, PageNumber: 5},
	}

	config := PipelineConfig{
		ConfigKeyRemoveTOC: true,
		ConfigKeyOutlines:  outlines,
	}
	err := PostProcess(context.Background(), result, config)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Sections) != 2 {
		t.Errorf("remove_toc=true should remove TOC pages, got %d sections", len(result.Sections))
	}
	for _, s := range result.Sections {
		for _, p := range s.Positions {
			for _, pn := range p.PageNumbers {
				if pn < 3 {
					t.Errorf("TOC page %d should have been removed: section %q", pn, s.Text)
				}
			}
		}
	}
}

// TestPostProcess_RemoveTOC_NoOutlines verifies that when no outlines
// are passed, no TOC removal happens.
func TestPostProcess_RemoveTOC_NoOutlines(t *testing.T) {
	result := newTestResult(
		makePosSection("目录内容", 1, 100, 500, 100, 200),
		makePosSection("第一章 正文", 3, 100, 500, 100, 200),
	)
	config := PipelineConfig{
		ConfigKeyRemoveTOC: true,
	}
	err := PostProcess(context.Background(), result, config)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Sections) != 2 {
		t.Errorf("no outlines → all sections kept, got %d", len(result.Sections))
	}
}

// TestPostProcess_RemoveTOC_EmptyOutlines verifies empty outlines array is no-op.
func TestPostProcess_RemoveTOC_EmptyOutlines(t *testing.T) {
	result := newTestResult(
		makePosSection("目录", 1, 100, 500, 100, 200),
		makePosSection("正文", 2, 100, 500, 100, 200),
	)
	config := PipelineConfig{
		ConfigKeyRemoveTOC: true,
		ConfigKeyOutlines:  []pdftype.Outline{},
	}
	err := PostProcess(context.Background(), result, config)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Sections) != 2 {
		t.Errorf("empty outlines → all sections kept, got %d", len(result.Sections))
	}
}
