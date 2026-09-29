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

// Follow-up losslessness tests for the two whitespace-handling defects found
// during the #20276 review:
//   - applyChildrenDelimText must fold a whitespace-only delimiter piece only
//     within its OWN parent, never into a different parent whose text happens
//     to match (the old code compared out[n-1].Mom, which is parent TEXT, not
//     parent identity).
//   - chunkFromItem must not discard whitespace-only delimiter pieces; it must
//     fold them into an adjacent real piece, mirroring the text/children paths.

package chunker

import (
	"reflect"
	"regexp"
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

// TestApplyChildrenDelimText_NoCrossParentFold pins the cross-parent folding
// bug: two distinct parents whose text (minus one leading "\n") is identical
// must each keep their own leading delimiter. The buggy Mom comparison folds
// the second parent's whitespace into the first parent's child, corrupting the
// first and dropping the second parent's delimiter.
func TestApplyChildrenDelimText_NoCrossParentFold(t *testing.T) {
	pattern := regexp.MustCompile("\n")
	docs := []schema.ChunkDoc{
		{Text: "\nA", CKType: "text"},
		{Text: "\nA", CKType: "text"},
	}
	out := applyChildrenDelimText(docs, pattern, true)
	if len(out) != 2 {
		t.Fatalf("expected 2 children (one per parent), got %d: %+v", len(out), out)
	}
	want := []string{"\nA", "\nA"}
	for i, w := range want {
		if out[i].Text != w {
			t.Errorf("child %d: got %q want %q", i, out[i].Text, w)
		}
	}
}

// TestChunkFromItem_RetainsWhitespacePieces pins the whitespace-drop bug in
// chunkFromItem: a bare retained delimiter that produces a whitespace-only
// piece (blank line between two delimiters, or a leading delimiter) must be
// preserved by folding it into an adjacent real piece, not discarded.
func TestChunkFromItem_RetainsWhitespacePieces(t *testing.T) {
	pattern := regexp.MustCompile("\n")
	cases := []struct {
		name string
		text string
		want []string
	}{
		{"interior blank line", "alpha\n\nbeta", []string{"alpha\n", "\nbeta"}},
		{"leading delimiter", "\nsecond line", []string{"\nsecond line"}},
		{"trailing delimiter", "alpha\n\n", []string{"alpha\n\n"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			it := schema.ChunkDoc{Text: c.text, CKType: "text"}
			out := chunkFromItem(it, pattern, true)
			got := make([]string, 0, len(out))
			for _, d := range out {
				got = append(got, d.Text)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %#v want %#v", got, c.want)
			}
		})
	}
}

// TestChunkFromItem_LosslessWhitespaceReconstruct asserts the end-to-end
// lossless contract for an item containing repeated bare delimiters: the
// concatenation of the emitted chunk texts reproduces the source exactly.
func TestChunkFromItem_LosslessWhitespaceReconstruct(t *testing.T) {
	pattern := regexp.MustCompile("\n")
	src := "alpha\n\nbeta\n\ngamma"
	it := schema.ChunkDoc{Text: src, CKType: "text"}
	out := chunkFromItem(it, pattern, true)
	var recon string
	for _, d := range out {
		recon += d.Text
	}
	if recon != src {
		t.Fatalf("lossless reconstruction failed: got %q want %q", recon, src)
	}
}
