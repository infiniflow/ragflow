// Copyright 2025 Infiniflow, Inc. All rights reserved.
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
	"testing"

	"ragflow/internal/ingestion/component/schema"

	"gorm.io/gorm"
)

// typedChunksInner is a minimal runtime.Component fake that returns a fixed
// output map carrying a typed []ChunkDoc under "chunks".
type typedChunksInner struct {
	out map[string]any
}

func (t *typedChunksInner) Invoke(ctx context.Context, db *gorm.DB, inputs map[string]any) (map[string]any, error) {
	return t.out, nil
}

// TestImageUploadDecorator_ProcessesTypedChunks locks the R2 regression: the
// imageUploadDecorator is the single choke point every chunker variant flows
// through. When the upstream carrier is the typed []ChunkDoc form, the
// decorator must normalize it (not silently skip), stamp chunk ids, and
// re-emit the []map[string]any wire form. Before migration the bare
// .([]map[string]any) assertion failed on a typed carrier, so the whole
// id-stamp/upload pass was skipped.
//
// kb_id is intentionally absent, so this exercises the dry-run branch (drop
// image bytes, no MinIO upload).
func TestImageUploadDecorator_ProcessesTypedChunks(t *testing.T) {
	inner := &typedChunksInner{out: map[string]any{
		"chunks": []schema.ChunkDoc{
			{Text: "hello", Image: "img/1.png"}, // no id yet
		},
	}}
	dec := &imageUploadDecorator{inner: inner}

	out, err := dec.Invoke(context.Background(), nil, map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, ok := schema.ChunkCarrierToMaps(out["chunks"])
	if !ok {
		t.Fatalf("out[chunks] type = %T, want []map[string]any (typed carrier must be normalized)", out["chunks"])
	}
	if len(chunks) != 1 {
		t.Fatalf("len = %d, want 1", len(chunks))
	}
	if id, _ := chunks[0]["id"].(string); id == "" {
		t.Fatalf("chunk id not stamped: decorator skipped the typed carrier? chunk=%+v", chunks[0])
	}
}
