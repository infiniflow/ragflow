// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package chunker

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"ragflow/internal/common"
	"ragflow/internal/ingestion/component/schema"
)

// TestFinalizeGeneralChunks_KeepsUploadedTagOnlyChunk is the regression test
// for A1: a media chunk whose text is ONLY a parser position tag (so
// removeTag yields "") but which already has a non-empty ImgID (set by the
// streaming crop upload) must survive finalizeGeneralChunks. The streaming
// upload clears out[i].Image immediately, so the old keep condition
// `Text=="" && Image==""` would drop it — discarding an already-uploaded
// image and orphaning the MinIO object. The keep condition must also consult
// ImgID.
func TestFinalizeGeneralChunks_KeepsUploadedTagOnlyChunk(t *testing.T) {
	in := []schema.ChunkDoc{
		{
			DocType: "image",
			Text:    "@@1\t2##",     // pure position tag; removeTag -> ""
			Image:   "",             // cleared by streaming upload
			ImgID:   "kb1-deadbeef", // uploaded, non-empty
		},
	}
	got := finalizeGeneralChunks(in, nil)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1 (uploaded tag-only chunk must survive finalize)", len(got))
	}
	if got[0].ImgID != "kb1-deadbeef" {
		t.Errorf("ImgID = %q, want preserved kb1-deadbeef", got[0].ImgID)
	}

	// Control: a genuinely empty chunk (no ImgID) must still be dropped.
	empty := []schema.ChunkDoc{{DocType: "image", Text: "", Image: "", ImgID: ""}}
	if got2 := finalizeGeneralChunks(empty, nil); len(got2) != 0 {
		t.Errorf("len(empty finalize) = %d, want 0 (true empty chunk dropped)", len(got2))
	}
}

// TestCropImageChunks_UploadFailureRedactsChunkText is the regression test for
// A2: when a streaming preview upload fails, cropImageChunks logs a warning,
// but that warning must NOT carry the document's chunk text (CWE-532 —
// sensitive data into logs). The text field must be redacted from the log.
func TestCropImageChunks_UploadFailureRedactsChunkText(t *testing.T) {
	core, observedLogs := observer.New(zapcore.WarnLevel)
	oldLogger := common.Logger
	common.Logger = zap.New(core)
	t.Cleanup(func() { common.Logger = oldLogger })

	ctx := withIngestionGlobals(t, "kb1", "doc1")

	orig := ChunkImageUploader
	ChunkImageUploader = func(_ context.Context, _, _ string, _ []byte) (string, error) {
		return "", fmt.Errorf("boom")
	}
	t.Cleanup(func() { ChunkImageUploader = orig })

	eng := mockCropEngine{}
	pos := jsonPositions(t, []float64{1, 10, 100, 10, 100})
	secret := "SECRET_DOC_TEXT_9f3a"
	chunks := []schema.ChunkDoc{{CKType: "image", Text: secret, PDFPositions: pos}}
	cropImageChunks(ctx, eng, chunks)

	for _, e := range observedLogs.All() {
		for _, f := range e.Context {
			if f.Key == "chunk" {
				t.Errorf("upload-failure log still carries a 'chunk' field: %v", f)
			}
			if sv, ok := f.Interface.(string); ok && strings.Contains(sv, secret) {
				t.Errorf("upload-failure log leaks document text %q via field %q", secret, f.Key)
			}
		}
	}
}
