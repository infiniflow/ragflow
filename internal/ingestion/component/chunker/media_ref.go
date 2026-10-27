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
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component"
	"ragflow/internal/ingestion/component/globals"
	"ragflow/internal/parser/parser"
)

// EnvMediaRefShadow enables the both-mode shadow check. With the flag on, the
// chunker resolves every media_ref and verifies it against the inline payload
// that is actually uploaded. It changes no output; it measures whether the
// resolver reproduces the parser's bytes before a deployment flips to ref
// mode.
const EnvMediaRefShadow = "RAGFLOW_MEDIA_REF_SHADOW"

func mediaRefShadowEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvMediaRefShadow))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// ChunkMediaSource resolves the source document for deferred media_refs.
// Tests override it with an in-memory source.
var ChunkMediaSource = func(db *gorm.DB, inputs map[string]any) parser.MediaSource {
	return chunkMediaSource(db, inputs)
}

// chunkMediaSource re-acquires the source document for a deferred media_ref.
// It prefers the explicit bucket/path pair the Parser forwards and falls back
// to resolving the same reference from doc_id.
func chunkMediaSource(db *gorm.DB, inputs map[string]any) parser.MediaSource {
	return parser.MediaSourceFunc(func(ctx context.Context) ([]byte, error) {
		bucket := globals.GlobalOrInput(ctx, inputs, "bucket", "")
		path := globals.GlobalOrInput(ctx, inputs, "path", "")
		if bucket != "" && path != "" {
			return component.FetchBinary(ctx, bucket, path)
		}
		docID := globals.GlobalOrInput(ctx, inputs, "doc_id", "")
		if docID == "" || db == nil {
			return nil, errors.New("chunker: no source document reference for media_ref")
		}
		ref, err := component.ResolveDocumentStorage(ctx, db, docID)
		if err != nil {
			return nil, err
		}
		if ref == nil || ref.Bucket == "" || ref.Path == "" {
			return nil, fmt.Errorf("chunker: document %s has no storage reference", docID)
		}
		return component.FetchBinary(ctx, ref.Bucket, ref.Path)
	})
}

// verifyChunkMedia resolves each reference and compares the result with the
// bytes the parser inlined. Mismatches mean the source changed after parsing
// or the reference locator is wrong; both are worth a warning, neither
// changes the chunk.
func verifyChunkMedia(ctx context.Context, chunks []map[string]any, resolver parser.MediaResolver, src parser.MediaSource) {
	for _, ck := range chunks {
		ref := entity.MediaRefFromAny(ck["media_ref"])
		if ref == nil {
			continue
		}
		img, _ := ck["image"].(string)
		if img == "" {
			continue
		}
		inline, err := decodeChunkImage(img)
		if err != nil {
			common.Warn("media_ref shadow check: decode inline image failed", zap.String("sha256", ref.SHA256), zap.Error(err))
			continue
		}
		resolved, err := resolver.Resolve(ctx, *ref, src)
		if err != nil {
			common.Warn("media_ref shadow check: resolve failed", zap.String("sha256", ref.SHA256), zap.Error(err))
			continue
		}
		if !bytes.Equal(inline, resolved) {
			common.Warn("media_ref shadow check: resolved image differs from the inline payload",
				zap.String("sha256", ref.SHA256))
		}
	}
}
