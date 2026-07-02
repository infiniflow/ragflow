//go:build cgo && manual

package pdf

import (
	"bytes"
	"context"
	"encoding/base64"
	"image/png"
	"os"
	"path/filepath"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"testing"
)

func TestParse_CropSectionImages(t *testing.T) {
	pdfPath := filepath.Join("testdata", "pdfs", "01_english_simple.pdf")
	data, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Skipf("test PDF not found: %v", err)
	}

	eng, err := NewEngine(data)
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	defer eng.Close()

	cfg := pdf.DefaultParserConfig()
	p := NewParser(cfg)
	result, err := p.ParseRaw(context.Background(), eng, &MockDocAnalyzer{Healthy: true})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	withImage, withoutImage := 0, 0
	for _, s := range result.Sections {
		if s.Image == "" {
			withoutImage++
			t.Logf("no image: type=%s text=%q", s.LayoutType, s.Text[:min(30, len(s.Text))])
		} else {
			withImage++
			decoded, err := base64.StdEncoding.DecodeString(s.Image)
			if err != nil {
				t.Errorf("invalid base64 for section %q: %v", s.Text[:min(20, len(s.Text))], err)
				continue
			}
			img, err := png.Decode(bytes.NewReader(decoded))
			if err != nil {
				t.Errorf("invalid PNG for section %q: %v", s.Text[:min(20, len(s.Text))], err)
				continue
			}
			if img.Bounds().Dx() == 0 || img.Bounds().Dy() == 0 {
				t.Errorf("zero-size image for section %q", s.Text[:min(20, len(s.Text))])
			}
		}
	}

	t.Logf("%d sections: %d with image, %d without", len(result.Sections), withImage, withoutImage)

	if withImage == 0 {
		t.Error("no sections have images — crop pipeline not working")
	}
}

func TestCrop_Regression_SnapshotPDFs(t *testing.T) {
	for _, name := range []string{
		"01_english_simple", "02_chinese_simple", "03_multipage",
	} {
		t.Run(name, func(t *testing.T) {
			pdfPath := filepath.Join("testdata", "pdfs", name+".pdf")
			data, err := os.ReadFile(pdfPath)
			if err != nil {
				t.Skipf("PDF not found: %v", err)
			}
			eng, err := NewEngine(data)
			if err != nil {
				t.Fatalf("engine: %v", err)
			}
			defer eng.Close()

			p := NewParser(pdf.DefaultParserConfig())
			result, err := p.ParseRaw(context.Background(), eng, &MockDocAnalyzer{Healthy: true})
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			for i, s := range result.Sections {
				if s.Image == "" {
					t.Errorf("section[%d] has no image: type=%s text=%q",
						i, s.LayoutType, s.Text[:min(40, len(s.Text))])
				}
				if s.Image != "" {
					decoded, _ := base64.StdEncoding.DecodeString(s.Image)
					img, _ := png.Decode(bytes.NewReader(decoded))
					if img != nil && (img.Bounds().Dx() == 0 || img.Bounds().Dy() == 0) {
						t.Errorf("section[%d] zero-size image", i)
					}
				}
			}
			if len(result.Sections) == 0 {
				t.Error("no sections parsed")
			}
		})
	}
}
