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

import "ragflow/internal/ingestion/component/schema"

// structuralLayouts are the non-media block kinds a parser asserts on the
// layout channel. They gate heading detection and line splitting; they never
// describe content type (doc_type_kwd owns that) and are cleared before the
// chunker output boundary.
//
// "title" is deliberately absent: it is the PDF models' heuristic label and
// goes through the Title chunker's notTitle guard, while "heading" is an
// asserted title that bypasses that guard.
var structuralLayouts = map[string]bool{
	"heading":   true,
	"paragraph": true,
	"list":      true,
	"code":      true,
	"quote":     true,
}

func isStructuralLayout(layout string) bool {
	return structuralLayouts[layout]
}

// clearStructuralLayout drops the block-kind marker after a chunker has
// merged or split the block into plain body text. PDF model layouts
// (title/figure/equation/...) pass through unchanged.
func clearStructuralLayout(ck *schema.ChunkDoc) {
	if isStructuralLayout(ck.Layout) {
		ck.Layout = ""
	}
}

func isHeadingLayout(layout string) bool {
	return layout == "heading"
}
