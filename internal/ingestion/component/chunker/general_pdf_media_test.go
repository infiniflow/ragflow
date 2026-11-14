//go:build cgo

package chunker

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"sync/atomic"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/ingestion/component"
	"ragflow/internal/storage"
)

func TestGeneralPDFContextOnlyMedia(t *testing.T) {
	data := newGeneralPDFChildPreviewFixture(t)
	for _, kind := range []string{"image", "table"} {
		for _, scenario := range []string{"crop", "retry upload", "no context", "no source", "missing source", "invalid pdf", "missing page"} {
			t.Run(kind+"/"+scenario, func(t *testing.T) {
				ctx := withIngestionGlobals(t, "context-test", "doc-1")
				store := storage.NewMemoryStorage()
				previousStorage, previousUploader := component.SetStorageFactoryOverride, ChunkImageUploader
				component.SetStorageFactoryOverride = func() storage.Storage { return store }
				ChunkImageUploader = component.DefaultImageUploader
				t.Cleanup(func() {
					component.SetStorageFactoryOverride = previousStorage
					ChunkImageUploader = previousUploader
				})
				if scenario == "retry upload" {
					var failed atomic.Bool
					ChunkImageUploader = func(ctx context.Context, kbID, chunkID string, data []byte) (string, error) {
						if chunkID != common.ChunkID("doc-1", "before.") && failed.CompareAndSwap(false, true) {
							return "", errors.New("temporary upload failure")
						}
						return component.DefaultImageUploader(ctx, kbID, chunkID, data)
					}
				}
				page := 1
				if scenario == "missing page" {
					page = 99
				}
				inputs := map[string]any{
					"name": "source.pdf", "file_type": "pdf", "output_format": "json",
					"json": []map[string]any{
						{"text": "before.", "doc_type_kwd": "text", "positions": [][]int{{1, 90, 400, 100, 120}}},
						{"text": "", "doc_type_kwd": kind, "positions": [][]int{{page, 90, 400, 200, 300}}},
					},
				}
				if scenario != "no source" {
					inputs["bucket"], inputs["path"] = "context-test", "source.pdf"
					if scenario != "missing source" {
						pdf := data
						if scenario == "invalid pdf" {
							pdf = []byte("%PDF-invalid")
						}
						if err := store.Put(ctx, "context-test", "source.pdf", pdf); err != nil {
							t.Fatal(err)
						}
					}
				}
				contextSize := 64
				if scenario == "no context" {
					contextSize = 0
				}
				inner, err := NewGeneralChunker(map[string]any{"image_context_size": contextSize, "table_context_size": contextSize})
				if err != nil {
					t.Fatal(err)
				}
				out, err := (&imageUploadDecorator{inner: inner}).Invoke(ctx, nil, inputs)
				if err != nil {
					t.Fatal(err)
				}
				chunks := outputChunks(t, out)
				want := 1
				if scenario == "crop" || scenario == "retry upload" {
					want = 2
				}
				if len(chunks) != want {
					t.Fatalf("chunks = %d, want %d: %#v", len(chunks), want, chunks)
				}
				if want == 1 {
					if chunks[0]["text"] != "before." || chunks[0]["doc_type_kwd"] != "text" {
						t.Fatalf("unexpected remaining text: %#v", chunks)
					}
					return
				}
				media := chunks[1]
				if media["doc_type_kwd"] != kind || media["text"] != "before." {
					t.Errorf("media lost its type or context: %#v", media)
				}
				id, _ := media["id"].(string)
				if media["img_id"] != "context-test-"+id || media["image"] != nil {
					t.Errorf("media was not uploaded correctly: %#v", media)
				}
				preview, err := store.Get(ctx, "context-test", id)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := png.DecodeConfig(bytes.NewReader(preview)); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
