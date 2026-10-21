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

//go:build cgo

package chunker

import (
	"testing"

	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

// TestCropTitleChunksNeverEmitsChunkStructure pins that the Title-family crop
// path keeps the output shape identical whether or not a PDF engine is
// available: retired ck_type and structural layout never leave the component.
func TestCropTitleChunksNeverEmitsChunkStructure(t *testing.T) {
	for _, engine := range []deepdoctype.PDFEngine{nil, mockCropEngine{}} {
		in := []map[string]any{{
			"text":         "body",
			"doc_type_kwd": "text",
			"ck_type":      "heading",
			"layout":       "heading",
		}}
		got := cropTitleChunks(t.Context(), engine, in)
		for i, ck := range got {
			if kind, ok := ck["ck_type"]; ok {
				t.Errorf("engine=%T chunk[%d] emits retired ck_type %v: %#v", engine, i, kind, ck)
			}
			if layout, ok := ck["layout"]; ok {
				t.Errorf("engine=%T chunk[%d] leaks structural layout %v: %#v", engine, i, layout, ck)
			}
		}
	}
}
