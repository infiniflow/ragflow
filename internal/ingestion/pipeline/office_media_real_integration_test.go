//go:build cgo && integration

package pipeline

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
	officeOxide "github.com/yfedoseev/office_oxide/go"
)

// TestOfficeMediaRealServices uses the production native/Excelize parsers and
// the ingestion canvas, never a parser or storage double.
func TestOfficeMediaRealServices(t *testing.T) {
	if os.Getenv("RAGFLOW_REAL_OFFICE_MEDIA_TEST") != "1" {
		t.Skip("set RAGFLOW_REAL_OFFICE_MEDIA_TEST=1 with configured MySQL, Kvrocks and MinIO")
	}
	docx, err := os.ReadFile(filepath.Join(repoRootFromPipelineTest(t), "internal/parser/parser/testdata/docx_table_image.docx"))
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		t.Fatal(err)
	}
	var png []byte
	for _, entry := range archive.File {
		if !strings.HasPrefix(entry.Name, "word/media/") || !strings.HasSuffix(entry.Name, ".png") {
			continue
		}
		f, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		png, err = io.ReadAll(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		break
	}
	if len(png) == 0 {
		t.Fatal("DOCX fixture has no PNG")
	}
	t.Run("docx_table_image", func(t *testing.T) { runMediaRealServices(t, "docx", docx, png) })
	t.Run("pptx_slide_image", func(t *testing.T) {
		w := officeOxide.NewPptxWriter()
		defer w.Close()
		slide := w.AddSlide()
		w.AddSlideText(slide, "Office media fixture")
		w.AddSlideImage(slide, png, "png", 914400, 914400, 914400, 457200)
		data, err := w.ToBytes()
		if err != nil {
			t.Fatal(err)
		}
		runMediaRealServices(t, "pptx", data, png)
	})
	t.Run("xlsx_cell_image", func(t *testing.T) {
		f := excelize.NewFile()
		defer f.Close()
		if err := f.SetCellValue("Sheet1", "A1", "Office media fixture"); err != nil {
			t.Fatal(err)
		}
		if err := f.AddPictureFromBytes("Sheet1", "C3", &excelize.Picture{Extension: ".png", File: png}); err != nil {
			t.Fatal(err)
		}
		buf, err := f.WriteToBuffer()
		if err != nil {
			t.Fatal(err)
		}
		runMediaRealServices(t, "xlsx", buf.Bytes(), png)
	})
}
