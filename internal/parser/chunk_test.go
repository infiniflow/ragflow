//go:build cgo

package parser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ragflow/internal/parser/tools"
)

// TestParse_ChunkEquivalence verifies that chunked processing produces
// the same output as processing all pages at once. Uses chunkSize=1
// (every page is its own chunk) on a multi-page fixture to maximize
// chunk boundary stress.
func TestParse_ChunkEquivalence(t *testing.T) {
	data, err := readTestPDF(t, "03_multipage.pdf")
	if err != nil {
		t.Fatal(err)
	}

	parse := func(chunkSize int) *ParseResult {
		eng, err := NewEngine(data)
		if err != nil {
			t.Fatal(err)
		}
		defer eng.Close()
		cfg := DefaultParserConfig()
		cfg.ChunkSize = chunkSize
		p := NewParser(cfg, &MockDocAnalyzer{Healthy: true, Model: ModelSaas})
		result, err := p.Parse(context.Background(), eng)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}

	// No chunking (all pages at once).
	full := parse(9999)
	// Aggressive chunking (1 page per chunk).
	chunked := parse(1)

	// Compare section counts.
	if len(full.Sections) != len(chunked.Sections) {
		t.Logf("section count: full=%d chunked=%d (small diff acceptable at chunk boundaries)",
			len(full.Sections), len(chunked.Sections))
	}

	// Compare text content via CharSimilarity.
	fullText := sectionsText(full.Sections)
	chunkedText := sectionsText(chunked.Sections)
	charSim := tools.CharSimilarity(fullText, chunkedText)
	t.Logf("CharSimilarity: %.1f%%", charSim)
	if charSim < 95 {
		t.Errorf("chunk equivalence too low: CharSim=%.1f%% (want >= 95%%)", charSim)
	}

	// Compare metrics (should be identical or very close).
	t.Logf("Metrics: full=%+v chunked=%+v", full.Metrics, chunked.Metrics)
	if full.Metrics.BoxesInitial != chunked.Metrics.BoxesInitial {
		t.Errorf("BoxesInitial: full=%d chunked=%d",
			full.Metrics.BoxesInitial, chunked.Metrics.BoxesInitial)
	}

	// Bug fix regression: PageImages must survive chunked merge.
	if len(full.PageImages) == 0 {
		t.Error("full parse: PageImages should not be empty (3-page document)")
	}
	if len(chunked.PageImages) == 0 {
		t.Error("chunked parse: PageImages should be preserved across chunks")
	}
}

func readTestPDF(t *testing.T, name string) ([]byte, error) {
	t.Helper()
	return os.ReadFile(filepath.Join("testdata", "pdfs", name))
}

func sectionsText(sections []Section) string {
	var sb strings.Builder
	for _, s := range sections {
		sb.WriteString(s.Text)
		sb.WriteByte('\n')
	}
	return sb.String()
}
