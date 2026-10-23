//go:build cgo && integration

package pdf

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"ragflow/internal/deepdoc/parser/pdf/util"
)

// TestAN0590Figures exercises real native text, DLA, OCR and rendered crops.
// AN0590_PDF points to an0590scv140.pdf; MODEL_DIR supplies DeepDOC models.
func TestAN0590Figures(t *testing.T) {
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
	dir := os.Getenv("DEEPDOC_DIAGNOSTIC_DIR")
	if dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		b, err := json.MarshalIndent(result.Sections, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "sections.json"), b, 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		number, page        int
		minWidth, minHeight float64
	}{
		{2, 1, 240, 50}, {3, 3, 350, 100},
	} {
		t.Run(fmt.Sprintf("figure%d", tc.number), func(t *testing.T) {
			var figure *pdf.Section
			for _, section := range result.Figures() {
				text := strings.Join(strings.Fields(section.Text), "")
				if strings.Contains(text, fmt.Sprintf("图%d.", tc.number)) {
					s := section
					figure = &s
					break
				}
			}
			if figure == nil {
				t.Fatalf("figure %d missing", tc.number)
			}
			pos := figure.Positions[0]
			if len(pos.PageNumbers) != 1 || pos.PageNumbers[0] != tc.page || pos.Right-pos.Left < tc.minWidth || pos.Bottom-pos.Top < tc.minHeight {
				t.Fatalf("incomplete figure bounds: %+v", pos)
			}
			if tc.number == 3 && (!strings.Contains(figure.Text, "SCL") || !strings.Contains(figure.Text, "SDA")) {
				t.Fatalf("native waveform labels missing: %q", figure.Text)
			}
			if dir != "" {
				img, err := RenderPageToImage(result.Engine, tc.page)
				if err != nil {
					t.Fatal(err)
				}
				crop := util.CropSectionPositionsRaster(figure.Positions, map[int]image.Image{tc.page: img}, pdf.DlaScale)
				if crop == nil {
					t.Fatal("figure crop missing")
				}
				f, err := os.Create(filepath.Join(dir, fmt.Sprintf("figure%d.png", tc.number)))
				if err != nil {
					t.Fatal(err)
				}
				if err := png.Encode(f, crop); err != nil {
					f.Close()
					t.Fatal(err)
				}
				if err := f.Close(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
