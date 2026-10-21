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

package component

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

// TestCleanEmbedText pins the table-structure strip that mirrors Python's
// cleaned_txt (rag/flow/tokenizer/tokenizer.py:86). The contract is a direct
// regex port, quirks included: case-sensitive tag names, and an attribute
// group bounded to 12 non-tag characters.
func TestCleanEmbedText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"whitespace only", "  \n\t ", ""},
		{"plain text unchanged", "just words", "just words"},
		{"text keeps inner whitespace", "a  b\nc", "a  b\nc"},

		{"empty table", "<table></table>", ""},
		{"empty table rows cells", "<table><tr><td></td></tr></table>", ""},
		{"whitespace-only table", "<table>   </table>", ""},
		{"empty caption", "<caption></caption>", ""},
		{"empty th", "<th></th>", ""},

		{"table with text", "<table><tr><td>hello</td></tr></table>", "hello"},
		// Each replaced tag contributes one space; Python's re.sub does not
		// collapse runs of spaces, and neither does this port.
		{"text around table", "before<table><tr><td>x</td></tr></table>after", "before   x   after"},
		{"nested tables", "<table><tr><td><table><tr><td>deep</td></tr></table></td></tr></table>", "deep"},
		{"all tag kinds", "<table><caption>cap</caption><tr><th>h</th><td>d</td></tr></table>", "cap   h  d"},

		{"closing tags alone", "</table></td>", ""},
		{"attributes within bound", `<td colspan="2">x`, "x"},
		{"attribute bound exactly twelve", `<td class="abc">x`, "x"},
		{"attribute over bound survives", `<td class="abcde">x`, `<td class="abcde">x`},
		{"long attribute opening survives closing does not", `<table class="a-longer-class-name">cell</table>`, `<table class="a-longer-class-name">cell`},

		{"uppercase tags are not stripped (case-sensitive parity)", "<TABLE><TR><TD>x</TD></TR></TABLE>", "<TABLE><TR><TD>x</TD></TR></TABLE>"},
		{"mixed case tag not stripped", "<Table>x</Table>", "<Table>x</Table>"},

		{"unclosed tag untouched", "<table", "<table"},
		{"angle bracket soup untouched", "a < b > c", "a < b > c"},
		{"unicode around tags", "中文<table><tr><td>内容</td></tr></table>尾", "中文   内容   尾"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := cleanEmbedText(c.in); got != c.want {
				t.Fatalf("cleanEmbedText(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestCleanEmbedTextStripsEveryMatch guards the global (not first-match)
// substitution: a long text with many table tags must have all of them
// replaced, not just the first.
func TestCleanEmbedTextStripsEveryMatch(t *testing.T) {
	in := "<td>a</td><td>b</td><td>c</td>"
	got := cleanEmbedText(in)
	if strings.Contains(got, "<td>") || strings.Contains(got, "</td>") {
		t.Fatalf("cleanEmbedText(%q) = %q, want every tag replaced", in, got)
	}
	if got != "a  b  c" {
		t.Fatalf("cleanEmbedText(%q) = %q, want %q", in, got, "a  b  c")
	}
}

// TestEmbedChunksAllSkippedWithCacheStoreSkipsTitleEncode: when every chunk's
// cleaned text is empty and no cache entry can serve them, nothing may be
// embedded — not even the title. Python returns before the title encode when
// the batch is empty (tokenizer.py:92-93); a configured cache store does not
// change that, it only matters when it actually serves a vector.
func TestEmbedChunksAllSkippedWithCacheStoreSkipsTitleEncode(t *testing.T) {
	comp, stub := withStubEmbedder(t, 4)
	comp.param.Fields = []string{"text"}
	chunks := []schema.ChunkDoc{
		chunkWithID("c1", "<table><tr><td></td></tr></table>"),
		chunkWithID("c2", "   "),
	}

	out, consumed, err := comp.embedChunks(context.Background(), "tenant", "kb", "doc.pdf", chunks, newMemCacheStore())
	if err != nil {
		t.Fatalf("embedChunks: %v", err)
	}
	if got := stub.calls.Load(); got != 0 {
		t.Fatalf("embedder calls = %d, want 0 (an all-skipped batch embeds nothing, title included)", got)
	}
	if consumed != 0 {
		t.Fatalf("embedding_token_consumption = %d, want 0", consumed)
	}
	for i, ck := range out {
		if hasEmbeddingVector(ck) {
			t.Errorf("chunk[%d] unexpectedly carries a vector", i)
		}
	}
}

// TestEmbedChunksCacheHitOnlyBatchStillAttachesVectors guards the other side
// of the same condition: when the cache serves every chunk, the flow must
// continue (attach the cached vectors) even though the batch itself is empty.
func TestEmbedChunksCacheHitOnlyBatchStillAttachesVectors(t *testing.T) {
	comp, stub := withStubEmbedder(t, 4)
	comp.param.Fields = []string{"text"}
	store := newMemCacheStore()

	warm := []schema.ChunkDoc{chunkWithID("c1", "alpha beta")}
	if _, _, err := comp.embedChunks(context.Background(), "tenant", "kb", "doc.pdf", warm, store); err != nil {
		t.Fatalf("warm-up embedChunks: %v", err)
	}
	callsAfterWarm := stub.calls.Load()
	if callsAfterWarm == 0 {
		t.Fatal("warm-up embedded nothing; the cache cannot be warm")
	}

	out, _, err := comp.embedChunks(context.Background(), "tenant", "kb", "doc.pdf", warm, store)
	if err != nil {
		t.Fatalf("cached embedChunks: %v", err)
	}
	// The title vector is not cached, so a fully cached pass re-encodes the
	// title (the weighted merge needs it) but must not re-embed content.
	if got, want := stub.calls.Load(), callsAfterWarm+1; got != want {
		t.Fatalf("embedder calls = %d after the cached pass, want %d (title only)", got, want)
	}
	last := stub.callInputs[len(stub.callInputs)-1]
	if len(last) != 1 || last[0] != "doc.pdf" {
		t.Fatalf("cached pass re-embedded content: last call inputs = %v, want the title only", last)
	}
	if !hasEmbeddingVector(out[0]) {
		t.Error("the cache-served chunk must still carry its vector")
	}
}
