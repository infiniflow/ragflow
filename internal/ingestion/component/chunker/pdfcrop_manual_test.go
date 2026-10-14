//go:build cgo && manual

package chunker

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
	infnative "ragflow/internal/deepdoc/parser/pdf/inference/native_analyzer"
	"ragflow/internal/deepdoc/parser/pdf/util"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/ingestion/component"
	"ragflow/internal/ingestion/component/schema"
	parser "ragflow/internal/parser/parser"
	"ragflow/internal/storage"
)

// TestHongLouMengCropPreview exports previews for visual inspection of the
// Hong Lou Meng book fixture, using its Chinese chapter-heading pattern.
// PDF_CROP_INPUT and PDF_CROP_OUTPUT are required; MODEL_DIR selects DeepDoc models.
func TestHongLouMengCropPreview(t *testing.T) {
	input, output := os.Getenv("PDF_CROP_INPUT"), os.Getenv("PDF_CROP_OUTPUT")
	if input == "" || output == "" {
		t.Skip("PDF_CROP_INPUT and PDF_CROP_OUTPUT required")
	}
	data, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	originalAnalyzer := deepdoctype.NativeDocAnalyzerFactory
	t.Cleanup(func() { deepdoctype.SetNativeDocAnalyzerFactory(originalAnalyzer) })
	if err := infnative.Register(os.Getenv("MODEL_DIR"), infnative.DefaultDropScore); err != nil {
		t.Fatal(err)
	}
	p := parser.NewPDFParser()
	p.ConfigureFromSetup(map[string]any{"output_format": "json"})
	p.OnPageDone = func(done, total int) { t.Logf("parsed %d/%d pages", done, total) }
	result := p.ParseWithResult(t.Context(), filepath.Base(input), data)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	inputs := map[string]any{"name": filepath.Base(input), "file_type": "pdf", "output_format": result.OutputFormat, "json": result.JSON, "file": result.File, "bucket": "crop-test", "path": "source.pdf"}
	encoded, err := json.Marshal(inputs)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(output, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "parser.json"), encoded, 0644); err != nil {
		t.Fatal(err)
	}
	mem := storage.NewMemoryStorage()
	if err := mem.Put(t.Context(), "crop-test", "source.pdf", data); err != nil {
		t.Fatal(err)
	}
	prev := component.SetStorageFactoryOverride
	component.SetStorageFactoryOverride = func() storage.Storage { return mem }
	t.Cleanup(func() { component.SetStorageFactoryOverride = prev })
	for _, tc := range []struct {
		name string
		new  func(map[string]any) (runtime.Component, error)
	}{
		{"general", NewGeneralChunker}, {"token", NewTokenChunker},
		{"group", NewGroupTitleChunker}, {"hierarchy", NewHierarchyTitleChunker},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := tc.new(map[string]any{"chunk_token_size": 512, "levels": [][]string{{`^第.{1,8}回`}}, "hierarchy": 1})
			if err != nil {
				t.Fatal(err)
			}
			out, err := c.Invoke(t.Context(), nil, inputs)
			if err != nil {
				t.Fatal(err)
			}
			if msg, ok := out["_ERROR"]; ok {
				t.Fatal(msg)
			}
			docs, _, err := schema.ChunkDocsFromAny(out["chunks"])
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for i := range docs {
				if docs[i].Image == "" {
					continue
				}
				png, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(docs[i].Image, "data:image/png;base64,"))
				if err != nil {
					t.Fatal(err)
				}
				name := fmt.Sprintf("%s-%03d.png", tc.name, i)
				if err := os.WriteFile(filepath.Join(output, name), png, 0644); err != nil {
					t.Fatal(err)
				}
				docs[i].Image = name
				count++
			}
			if count == 0 {
				t.Fatalf("no previews in %d chunks", len(docs))
			}
			b, err := json.MarshalIndent(docs, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(output, tc.name+".json"), b, 0644); err != nil {
				t.Fatal(err)
			}
			t.Logf("%d chunks, %d previews", len(docs), count)
		})
	}
	engine, err := deepdocpdf.NewEngine(data)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	page, err := deepdocpdf.RenderPageToImage(engine, 0)
	if err != nil {
		t.Fatal(err)
	}
	png, err := util.EncodePNG(page)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "page-001.png"), png, 0644); err != nil {
		t.Fatal(err)
	}
}
