//go:build cgo && manual

package pdf

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ragflow/internal/deepdoc/parser/pdf/tool"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestParse_BatchEquivalence verifies that batched processing produces
// the same output as processing all pages at once. Uses batchSize=1
// (every page is its own batch) on a multi-page fixture to maximize
// batch boundary stress.
func TestParse_BatchEquivalence(t *testing.T) {
	data, err := readTestPDF(t, "03_multipage.pdf")
	if err != nil {
		t.Fatal(err)
	}

	parse := func(batchSize int) *pdf.ParseResult {
		eng, err := NewEngine(data)
		if err != nil {
			t.Fatal(err)
		}
		defer eng.Close()
		cfg := pdf.DefaultParserConfig()
		cfg.BatchSize = batchSize
		p := NewParser(cfg)
		result, err := p.ParseRaw(context.Background(), eng, mockDLA)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}

	// No batching (all pages at once).
	full := parse(9999)
	// Aggressive batching (1 page per batch).
	batched := parse(1)

	// Compare section counts.
	if len(full.Sections) != len(batched.Sections) {
		t.Logf("section count: full=%d batched=%d (small diff acceptable at batch boundaries)",
			len(full.Sections), len(batched.Sections))
	}

	// Compare text content via CharSimilarity.
	fullText := sectionsText(full.Sections)
	batchedText := sectionsText(batched.Sections)
	charSim := tool.CharSimilarity(fullText, batchedText)
	t.Logf("CharSimilarity: %.1f%%", charSim)
	if charSim < 95 {
		t.Errorf("batch equivalence too low: CharSim=%.1f%% (want >= 95%%)", charSim)
	}

	// Compare metrics (should be identical or very close).
	t.Logf("Metrics: full=%+v batched=%+v", full.Metrics, batched.Metrics)
	if full.Metrics.BoxesInitial != batched.Metrics.BoxesInitial {
		t.Errorf("BoxesInitial: full=%d batched=%d",
			full.Metrics.BoxesInitial, batched.Metrics.BoxesInitial)
	}

	// Bug fix regression: PageImages must survive batched merge.
	if len(full.PageImages) == 0 {
		t.Error("full parse: PageImages should not be empty (3-page document)")
	}
	if len(batched.PageImages) == 0 {
		t.Error("batched parse: PageImages should be preserved across batches")
	}
}

func readTestPDF(t *testing.T, name string) ([]byte, error) {
	t.Helper()
	return os.ReadFile(filepath.Join("testdata", "pdfs", name))
}

func sectionsText(sections []pdf.Section) string {
	var sb strings.Builder
	for _, s := range sections {
		sb.WriteString(s.Text)
		sb.WriteByte('\n')
	}
	return sb.String()
}
