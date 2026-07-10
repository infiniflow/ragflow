//go:build cgo && integration

package pdf

import (
	"context"
	"reflect"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestParser_PageParallel_DeterministicOrder verifies the plan's real-fixture
// acceptance criteria: pool size 1 and pool size 4 must produce stable
// Sections, Tables, Metrics, and per-page PNG-equivalent images on the named
// PDF fixtures. With Config.Parallelism removed, page concurrency is governed
// by the process-wide worker pool.
func TestParser_PageParallel_DeterministicOrder(t *testing.T) {
	client := mustConnectInferenceClient(t)

	fixtures := []string{
		"03_multipage.pdf",
		"06_table_content.pdf",
		"07_mixed_content.pdf",
		"19_multipage_chunk.pdf",
	}

	runParse := func(t *testing.T, name string, poolSize int) *pdf.ParseResult {
		t.Helper()
		setPoolSize(t, poolSize)
		p := NewParser(pdf.DefaultParserConfig())
		result, err := p.Parse(context.Background(), mustReadPDF(t, name), client)
		if err != nil {
			t.Fatalf("fixture=%s poolSize=%d: Parse: %v", name, poolSize, err)
		}
		t.Cleanup(func() { result.Close() })
		return result
	}

	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			baseline := runParse(t, name, 1)
			parallel := runParse(t, name, 4)

			if !reflect.DeepEqual(stableSections(baseline.Sections), stableSections(parallel.Sections)) {
				t.Fatalf("fixture=%s: Sections diverged between poolSize=1 and poolSize=4", name)
			}
			if !reflect.DeepEqual(stableTables(baseline.Tables), stableTables(parallel.Tables)) {
				t.Fatalf("fixture=%s: Tables diverged between poolSize=1 and poolSize=4", name)
			}
			if !reflect.DeepEqual(baseline.Metrics, parallel.Metrics) {
				t.Fatalf("fixture=%s: Metrics diverged: %#v vs %#v", name, baseline.Metrics, parallel.Metrics)
			}
			if !reflect.DeepEqual(baseline.PageHeight, parallel.PageHeight) {
				t.Fatalf("fixture=%s: PageHeight diverged between poolSize=1 and poolSize=4", name)
			}
		})
	}
}
