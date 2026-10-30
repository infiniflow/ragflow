//go:build cgo

package chunker

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"os"
	"sync/atomic"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/ingestion/component"
	"ragflow/internal/storage"
)

func TestGeneralPDFChildPreviews(t *testing.T) {
	data, err := os.ReadFile("../../../deepdoc/parser/pdf/pdfium/testdata/page_sizes.pdf")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		children   []string
		want       []string
		failUpload bool
	}{
		{"children", []string{"#"}, []string{"alpha#", "beta#", "gamma"}, false},
		{"no children", nil, []string{"alpha#beta#gamma"}, false},
		{"retry upload", []string{"#"}, []string{"alpha#", "beta#", "gamma"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := withIngestionGlobals(t, "preview-test", "doc-1")
			store := storage.NewMemoryStorage()
			if err := store.Put(ctx, "preview-test", "source.pdf", data); err != nil {
				t.Fatal(err)
			}
			previousStorage, previousUploader := component.SetStorageFactoryOverride, ChunkImageUploader
			component.SetStorageFactoryOverride = func() storage.Storage { return store }
			ChunkImageUploader = component.DefaultImageUploader
			if tc.failUpload {
				var failed atomic.Bool
				ChunkImageUploader = func(ctx context.Context, kbID, chunkID string, data []byte) (string, error) {
					if failed.CompareAndSwap(false, true) {
						return "", errors.New("temporary upload failure")
					}
					return component.DefaultImageUploader(ctx, kbID, chunkID, data)
				}
			}
			t.Cleanup(func() {
				component.SetStorageFactoryOverride = previousStorage
				ChunkImageUploader = previousUploader
			})
			inputs := map[string]any{
				"name": "source.pdf", "file_type": "pdf", "output_format": "json",
				"bucket": "preview-test", "path": "source.pdf",
				"json": []map[string]any{{
					"text": "alpha#beta#gamma", "doc_type_kwd": "text",
					"positions": [][]float64{{1, 90, 400, 200, 500}},
				}},
			}
			run := func(children []string) []map[string]any {
				t.Helper()
				inner, err := NewGeneralChunker(map[string]any{
					"chunk_token_size":    512,
					"delimiters":          []string{"#"},
					"children_delimiters": children,
				})
				if err != nil {
					t.Fatal(err)
				}
				chunker := &imageUploadDecorator{inner: inner}
				out, err := chunker.Invoke(ctx, nil, inputs)
				if err != nil {
					t.Fatal(err)
				}
				return outputChunks(t, out)
			}
			chunks := run(tc.children)
			if len(chunks) != len(tc.want) {
				t.Fatalf("got %d chunks, want %d: %#v", len(chunks), len(tc.want), chunks)
			}
			heights := make([]int, len(chunks))
			for i, chunk := range chunks {
				text, _ := chunk["text"].(string)
				if text != tc.want[i] {
					t.Fatalf("chunk %d text = %q, want %q", i, text, tc.want[i])
				}
				id, _ := chunk["id"].(string)
				if id != common.ChunkID("doc-1", text) {
					t.Errorf("chunk %d has stale id %q for %q", i, id, text)
				}
				if chunk["img_id"] != "preview-test-"+id {
					t.Errorf("chunk %d image reference = %v", i, chunk["img_id"])
				}
				if chunk["image"] != nil {
					t.Errorf("chunk %d retains uploaded image bytes", i)
				}
				preview, err := store.Get(ctx, "preview-test", id)
				if err != nil {
					t.Fatal(err)
				}
				img, err := png.DecodeConfig(bytes.NewReader(preview))
				if err != nil {
					t.Fatal(err)
				}
				heights[i] = img.Height
			}
			if objects := store.(*storage.MemoryStorage).Inspect(); len(objects) != len(tc.want)+1 {
				t.Errorf("stored %d objects, want source PDF plus %d child previews", len(objects), len(tc.want))
			}
			if len(chunks) > 1 {
				parent := run(nil)
				if len(parent) != 1 {
					t.Fatalf("unsplit chunks = %d, want 1", len(parent))
				}
				preview, err := store.Get(ctx, "preview-test", parent[0]["id"].(string))
				if err != nil {
					t.Fatal(err)
				}
				img, err := png.DecodeConfig(bytes.NewReader(preview))
				if err != nil {
					t.Fatal(err)
				}
				for i, height := range heights {
					if height >= img.Height {
						t.Errorf("chunk %d preview height = %d, want smaller than unsplit preview %d", i, height, img.Height)
					}
				}
			}
		})
	}
}
