package parser

import (
	"context"
	"fmt"
	"image"
	"reflect"
	"testing"

	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
	deepdoctype "ragflow/internal/deepdoc/parser/pdf/type"
)

// noopDocAnalyzer is a DocAnalyzer that reports unhealthy and returns empty
// results, forcing the parser onto the charsToBoxes path. It lets the parser
// package exercise the deepdoc parser without the real DeepDoc models.
type noopDocAnalyzer struct{}

func (noopDocAnalyzer) DLA(context.Context, image.Image) ([]deepdoctype.DLARegion, error) {
	return nil, nil
}
func (noopDocAnalyzer) TSR(context.Context, image.Image) ([]deepdoctype.TSRCell, error) {
	return nil, nil
}
func (noopDocAnalyzer) OCRDetect(context.Context, image.Image) ([]deepdoctype.OCRBox, error) {
	return nil, nil
}
func (noopDocAnalyzer) OCRRecognize(context.Context, image.Image) ([]deepdoctype.OCRText, error) {
	return nil, nil
}
func (noopDocAnalyzer) Health() bool { return false }

// TestPDFParser_PagesEndToEnd_FromConfigureFromSetup verifies the full path
// from a filetype setup map (as delivered by the pipeline override_params)
// through ConfigureFromSetup -> PDFParser.Pages -> deepdoc ParserConfig.Pages
// -> resolvePagesToProcess, asserting only the configured page ranges are
// parsed.
//
// This ties together step 2 (ConfigureFromSetup + cfg.Pages plumbing) and
// step 1 (deepdoc page filtering) at the PDFParser level.
func TestPDFParser_PagesEndToEnd_FromConfigureFromSetup(t *testing.T) {
	// Build a 10-page mock engine where page N carries the text "pN".
	chars := make(map[int][]deepdoctype.TextChar, 10)
	for i := 0; i < 10; i++ {
		chars[i] = []deepdoctype.TextChar{
			{Text: fmt.Sprintf("p%d", i), X0: 10, X1: 50, Top: 10, Bottom: 30, PageNumber: i},
		}
	}
	eng := &deepdocpdf.MockEngine{NumPages: 10, Chars: chars, RenderW: 100, RenderH: 100}

	// 1. ConfigureFromSetup reads "pages" exactly as the pipeline would pass
	//    it (JSON-decoded []any of []any of float64).
	p := &PDFParser{}
	p.ConfigureFromSetup(map[string]any{
		"pages": []any{
			[]any{float64(1), float64(3)},
			[]any{float64(8), float64(10)},
		},
	})
	wantPages := [][]int{{1, 3}, {8, 10}}
	if !reflect.DeepEqual(p.Pages, wantPages) {
		t.Fatalf("PDFParser.Pages = %v, want %v", p.Pages, wantPages)
	}

	// 2. Build the deepdoc config the same way ParseWithResult does
	//    (cfg.Pages = p.Pages) and run the parser.
	cfg := deepdoctype.DefaultParserConfig()
	cfg.Pages = p.Pages
	docParser := deepdocpdf.NewParser(cfg)

	result, err := docParser.ParseRaw(context.Background(), eng, noopDocAnalyzer{})
	if err != nil {
		t.Fatalf("ParseRaw: %v", err)
	}

	// 3. [1,3] -> 0-based 0..2 ; [8,10] -> 7..9
	gotPages := make(map[int]struct{}, len(result.PageHeight))
	for k := range result.PageHeight {
		gotPages[k] = struct{}{}
	}
	want := map[int]struct{}{0: {}, 1: {}, 2: {}, 7: {}, 8: {}, 9: {}}
	if !reflect.DeepEqual(gotPages, want) {
		t.Errorf("PageHeight keys = %v, want %v", gotPages, want)
	}
}
