//go:build cgo && integration

package pdf

import (
	"fmt"
	"os"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestAN0590FigureBounds exercises complete DLA figure bounds on the real PDF.
// AN0590_PDF points to an0590scv140.pdf; MODEL_DIR supplies DeepDOC models.
func TestAN0590FigureBounds(t *testing.T) {
	path := os.Getenv("AN0590_PDF")
	if path == "" {
		t.Skip("AN0590_PDF is required")
	}
	analyzer := mustConnectInProcessAnalyzer(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := pdf.DefaultParserConfig()
	result, err := NewParser(cfg).Parse(t.Context(), data, analyzer)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()
	for _, tc := range []struct {
		number, page        int
		minWidth, minHeight float64
	}{
		{2, 1, 240, 50}, {3, 3, 350, 100},
	} {
		t.Run(fmt.Sprintf("figure%d", tc.number), func(t *testing.T) {
			var position *pdf.Position
			for _, section := range result.Figures() {
				for i := range section.Positions {
					pos := &section.Positions[i]
					if len(pos.PageNumbers) == 1 && pos.PageNumbers[0] == tc.page &&
						pos.Right-pos.Left >= tc.minWidth && pos.Bottom-pos.Top >= tc.minHeight {
						position = pos
						break
					}
				}
			}
			if position == nil {
				t.Fatalf("figure %d missing", tc.number)
			}
		})
	}
}
