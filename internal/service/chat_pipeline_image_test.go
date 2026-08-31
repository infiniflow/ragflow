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

// Regression (KB chat path, "chat"-typed model config): a text-only chat
// model must not receive image content parts — providers reject them (e.g.
// Zhipu GLM error 1210: messages.content.type only allows 'text'). The
// vision gate drops the images after the split, so the multimodal
// conversion never runs and the user gets a graceful text-only answer
// instead of a provider-side rejection.
func TestGateImageAttachmentsDropsImagesForTextOnlyChatModel(t *testing.T) {
	seedChatImageStorage(t)

	texts, images := splitFileAttachments(t.Context(), "user-1", []interface{}{chatImageFileDict}, false)
	if len(images) != 1 {
		t.Fatalf("split still resolves file-dict images with raw=false, got %v", images)
	}
	kept, attached := gateImageAttachments("glm-4-flash@Zhipu", "chat", images)
	if len(kept) != 0 {
		t.Fatalf("text-only chat model must not keep images, got %v", kept)
	}
	if !attached {
		t.Fatal("attachment flag must report the pre-drop image so the empty-response fallback stays out of the way")
	}
	if len(texts) != 0 {
		t.Fatalf("expected no text attachments for an image-only upload, got %v", texts)
	}
}

// Vision-capable (image2text) models keep every split image, and both
// split modes resolve file-dict attachments to jpeg data URIs that feed
// ConvertLastUserMsgToMultimodal.
func TestGateImageAttachmentsKeepsImagesForVisionModel(t *testing.T) {
	seedChatImageStorage(t)

	for _, raw := range []bool{false, true} {
		texts, images := splitFileAttachments(t.Context(), "user-1", []interface{}{chatImageFileDict}, raw)
		kept, attached := gateImageAttachments("qwen3-vl-plus@Tongyi-Qianwen", "image2text", images)
		if !attached || len(kept) != 1 {
			t.Fatalf("raw=%v: vision model must keep the image (kept=%v attached=%v)", raw, kept, attached)
		}
		if !strings.HasPrefix(kept[0], "data:image/jpeg;base64,") {
			t.Fatalf("raw=%v: image not a jpeg data URI: %q", raw, kept[0])
		}
		if len(texts) != 0 {
			t.Fatalf("raw=%v: expected no text attachments, got %v", raw, texts)
		}
	}
}

// The empty-response fallback must not short-circuit image-only requests:
// the model is the only consumer of the image, so the canned response
// would swallow it before the multimodal conversion.
func TestEmptyResponseAppliesRequiresNoAttachments(t *testing.T) {
	if !emptyResponseApplies(0, "", false) {
		t.Fatal("no knowledge and no attachments: the fallback applies")
	}
	for _, tc := range []struct {
		name        string
		knowledges  int
		attachments string
		images      bool
	}{
		{"retrieval found something", 1, "", false},
		{"text attachments present", 0, "doc text", false},
		{"image-only request", 0, "", true},
	} {
		if emptyResponseApplies(tc.knowledges, tc.attachments, tc.images) {
			t.Fatalf("%s: fallback must not apply", tc.name)
		}
	}
}
