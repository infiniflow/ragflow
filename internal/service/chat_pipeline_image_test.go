//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package service

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/storage"
)

// seedChatImageStorage installs an in-memory storage holding one jpeg blob
// under user-1's downloads bucket, the layout the upload_info flow writes.
func seedChatImageStorage(t *testing.T) {
	t.Helper()
	memory := storage.NewMemoryStorage()
	if err := memory.Put(t.Context(), "user-1-downloads", "img-1", []byte("jpeg-bytes")); err != nil {
		t.Fatalf("put: %v", err)
	}
	factory := storage.GetStorageFactory()
	original := factory.GetStorage()
	factory.SetStorage(memory)
	t.Cleanup(func() { factory.SetStorage(original) })
}

var chatImageFileDict = map[string]interface{}{
	"id":         "img-1",
	"name":       "photo.jpeg",
	"mime_type":  "image/jpeg",
	"created_by": "user-1",
}

// Regression (no-KB chat path): the chat UI uploads attachments as file
// dicts; AsyncChatSolo must resolve them to base64 data URIs for
// vision-capable models. The previous implementation returned no images
// for file dicts, so the attached image never reached the model.
func TestSplitChatAttachments_FileDictModeReturnsDataURIs(t *testing.T) {
	seedChatImageStorage(t)

	svc := NewChatPipelineService()
	texts, images := svc.splitChatAttachments(t.Context(), "user-1", []interface{}{chatImageFileDict})
	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %v", images)
	}
	want := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString([]byte("jpeg-bytes"))
	if images[0] != want {
		t.Fatalf("image = %q, want data URI %q", images[0], want)
	}
	if texts != "" {
		t.Fatalf("expected no text attachments for an image-only upload, got %q", texts)
	}
}

// String-mode files keep only data:-prefixed entries as images; every
// other non-empty entry stays in the text attachments.
func TestSplitChatAttachments_StringModeKeepsDataURIsOnly(t *testing.T) {
	svc := NewChatPipelineService()
	texts, images := svc.splitChatAttachments(t.Context(), "user-1", []interface{}{
		"data:image/png;base64,AAA", "https://example.com/a.png", "plain text", "  ",
	})
	if len(images) != 1 || images[0] != "data:image/png;base64,AAA" {
		t.Fatalf("unexpected images: %v", images)
	}
	if texts != "https://example.com/a.png\n\nplain text" {
		t.Fatalf("unexpected text attachments: %q", texts)
	}
}

// countingStorage wraps a Storage and counts Get calls, pinning the
// single-fetch contract of splitChatAttachments in file-dict mode.
type countingStorage struct {
	storage.Storage
	gets int
}

func (c *countingStorage) Get(ctx context.Context, bucket, fnm string, tenantID ...string) ([]byte, error) {
	c.gets++
	return c.Storage.Get(ctx, bucket, fnm, tenantID...)
}

// Resolving file-dict attachments must read every blob from storage exactly
// once: the text/image split happens on a single fetch, so a vision-model
// chat does not double-read attachment blobs.
func TestSplitChatAttachments_FetchesStorageOnce(t *testing.T) {
	memory := storage.NewMemoryStorage()
	for _, id := range []string{"img-1", "img-2"} {
		if err := memory.Put(t.Context(), "user-1-downloads", id, []byte("jpeg-bytes")); err != nil {
			t.Fatalf("put %s: %v", id, err)
		}
	}
	counter := &countingStorage{Storage: memory}
	factory := storage.GetStorageFactory()
	original := factory.GetStorage()
	factory.SetStorage(counter)
	t.Cleanup(func() { factory.SetStorage(original) })

	svc := NewChatPipelineService()
	texts, images := svc.splitChatAttachments(t.Context(), "user-1", []interface{}{
		chatImageFileDict,
		map[string]interface{}{"id": "img-2", "name": "photo2.jpeg", "mime_type": "image/jpeg", "created_by": "user-1"},
	})
	if len(images) != 2 {
		t.Fatalf("expected 2 images, got %v", images)
	}
	if texts != "" {
		t.Fatalf("expected no text attachments, got %q", texts)
	}
	if counter.gets != 2 {
		t.Fatalf("storage Get called %d times for 2 files, want exactly 2", counter.gets)
	}
}

// Regression (KB chat path): splitFileAttachments(raw=true) images feed
// ConvertLastUserMsgToMultimodal. Raw binary blobs used to fail
// parseDataURIOrB64, so the conversion errored and the caller silently
// skipped it, dropping the image before the LLM call.
func TestSplitFileAttachmentsRawImagesFeedMultimodalConversion(t *testing.T) {
	seedChatImageStorage(t)

	_, images := splitFileAttachments(t.Context(), "user-1", []interface{}{chatImageFileDict}, true)
	if len(images) != 1 {
		t.Fatalf("expected 1 image from splitFileAttachments, got %v", images)
	}

	converted, err := common.ConvertLastUserMsgToMultimodal(
		map[string]interface{}{"role": "user", "content": "describe the image"},
		images,
		"tongyi-qianwen",
	)
	if err != nil {
		t.Fatalf("ConvertLastUserMsgToMultimodal failed: %v", err)
	}
	rendered, ok := converted["content"].([]map[string]interface{})
	if !ok {
		t.Fatalf("content not rendered as parts: %T", converted["content"])
	}
	hasImage := false
	for _, part := range rendered {
		if partType, _ := part["type"].(string); partType == "image_url" {
			hasImage = true
			url, ok := part["image_url"].(*common.ImageURL)
			if !ok {
				t.Fatalf("image_url part has unexpected shape: %T", part["image_url"])
			}
			if !strings.HasPrefix(url.URL, "data:image/jpeg;base64,") {
				t.Fatalf("image_url not a jpeg data URI: %q", url.URL)
			}
		}
	}
	if !hasImage {
		t.Fatalf("no image part in converted content: %v", rendered)
	}
}
