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
	"errors"
	"fmt"

	"gorm.io/gorm"

	"ragflow/internal/ingestion/component"
	"ragflow/internal/ingestion/component/globals"
	"ragflow/internal/parser/parser"
)

// chunkMediaSourceFactory resolves the source document for deferred media_refs.
// Tests override it with an in-memory source.
var chunkMediaSourceFactory = func(db *gorm.DB, inputs map[string]any) parser.MediaSource {
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
