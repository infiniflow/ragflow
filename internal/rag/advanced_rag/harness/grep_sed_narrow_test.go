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

package harness

import (
	"strings"
	"testing"
)

// TestGrepTermsFromQueryCJK pins the derivation a Chinese query needs.
//
// The alnum-only original yields NO term for Chinese, and GrepSearch's own guard
// (`if not chunks or not terms: return res`) then returns whole chunks: measured
// on a Chinese question, the "Keyword-first locate" line was logged, a narrowed
// line never was, and every hit was a ~1200-char chunk.
func TestGrepTermsFromQueryCJK(t *testing.T) {
	// An alternation is the caller's own term list (the count protocol batches its
	// probes as 名|名|名), so it is split as-is — predicates included.
	if got := GrepTermsFromQuery("颜良|文丑|荀正"); !cjkTermsEqual(got, []string{"颜良", "文丑", "荀正"}) {
		t.Errorf("alternation terms = %v, want 颜良 文丑 荀正", got)
	}
	if got := GrepTermsFromQuery("关羽|斩"); !cjkTermsEqual(got, []string{"关羽", "斩"}) {
		t.Errorf("alternation with a one-rune predicate = %v, want 关羽 斩", got)
	}
	if got := GrepTermsFromQuery("关羽|关羽|关羽"); !cjkTermsEqual(got, []string{"关羽"}) {
		t.Errorf("alternation terms = %v, want a single deduped term", got)
	}

	// Space-separated Chinese keeps its tokens; a lone character is a particle.
	if got := GrepTermsFromQuery("关羽 斩颜良 文丑"); !cjkTermsEqual(got, []string{"关羽", "斩颜良", "文丑"}) {
		t.Errorf("split terms = %v", got)
	}
	if got := GrepTermsFromQuery("关羽 斩 文丑"); !cjkTermsEqual(got, []string{"关羽", "文丑"}) {
		t.Errorf("split terms = %v, want the one-rune particle dropped outside an alternation", got)
	}

	// An unbroken clause has no token at all, so its two-rune windows stand in:
	// the names in the clause still locate, and windows that occur nowhere cost
	// one failed lookup inside the narrowing pass.
	got := GrepTermsFromQuery("关羽杀了多少有姓名的人物")
	if len(got) != GrepTermsMax {
		t.Fatalf("clause windows = %v (%d), want %d windows", got, len(got), GrepTermsMax)
	}
	for _, want := range []string{"关羽", "姓名"} {
		if !cjkTermsContain(got, want) {
			t.Errorf("clause windows = %v, want %q among them", got, want)
		}
	}
	for _, term := range got {
		if !hasCJK(term) {
			t.Errorf("clause windows = %v, want CJK only (got %q)", got, term)
		}
	}
	if got := GrepTermsFromQuery("   "); got != nil {
		t.Errorf("blank query = %v, want nil", got)
	}
}

// TestExecOnTextCJKNarrowsToTheSentenceNotTheWholeChunk: a Chinese chunk carries
// no newlines, so its "line" IS the chunk and the line window hands back all of
// it. The window has to be cut at sentence punctuation instead — bounded, and
// with the clause that carries the name intact (that clause is what the model
// reads a name off).
func TestExecOnTextCJKNarrowsToTheSentenceNotTheWholeChunk(t *testing.T) {
	filler := "曹操引军退去诸将皆惊张辽与徐晃双马齐出先射中头盔徐晃败走沿河追赶。"
	content := "前文提到青州兵甲之事与本案无关。" +
		strings.Repeat(filler, 14) +
		"关羽赶上荀正交马一合砍荀正于马下。" +
		strings.Repeat(filler, 3) +
		"后文又叙河北军大半落水之事。"
	if len(content) <= contextCharBudget*2 {
		t.Fatalf("test content is %d bytes; it must exceed the per-side budget to exercise the fallback", len(content))
	}

	got, matched := execOnText(content, TermsToPatterns([]string{"荀正"}), 1, 0, GrepOutCharsPerChunk)
	if !matched {
		t.Fatal("荀正 is in the content, the narrow must match")
	}
	if !strings.Contains(got, "荀正") {
		t.Fatalf("window lost the hit: %q", got)
	}
	if len(got) > GrepOutCharsPerChunk {
		t.Errorf("window = %d bytes, want at most %d", len(got), GrepOutCharsPerChunk)
	}
	if !strings.Contains(got, "砍荀正于马下") {
		t.Errorf("window cut the clause mid-sentence: %q", got)
	}
	if strings.Contains(got, "青州兵甲") {
		t.Errorf("window dragged in the far head of the chunk: %q", got)
	}
}

// TestExecOnTextKeepsLineWindowsWhenThereAreLines: the sentence fallback is for
// text with no line structure. Text that HAS lines keeps grep's line window
// (±1 line), which is the behaviour every Latin question already relies on.
func TestExecOnTextKeepsLineWindowsWhenThereAreLines(t *testing.T) {
	content := "line one about alpha\nline two about beta\nline three about gamma\nline four about delta"
	got, matched := execOnText(content, TermsToPatterns([]string{"beta"}), 1, 0, GrepOutCharsPerChunk)
	if !matched {
		t.Fatal("beta is in the content, the narrow must match")
	}
	if !strings.Contains(got, "line one about alpha") || !strings.Contains(got, "line two about beta") {
		t.Errorf("window = %q, want the hit line and its before-context line", got)
	}
	if strings.Contains(got, "line four about delta") {
		t.Errorf("window = %q, want the after-context line excluded (after=0)", got)
	}
}

// TestNarrowByTermsCJKNarrowsThroughTheRealEntry pins the same behaviour one
// level up, where grep actually calls it.
func TestNarrowByTermsCJKNarrowsThroughTheRealEntry(t *testing.T) {
	text := strings.Repeat("曹操引军退去诸将皆惊。", 12) +
		"关羽赶上荀正交马一合砍荀正于马下。" +
		strings.Repeat("河北军大半落水。", 12)
	chunks := []map[string]any{{"id": "c1", "content_with_weight": text}}

	res := NarrowByTerms(chunks, []string{"荀正"}, nil, "", NarrowContext{Before: 1, After: 0},
		GrepOutCharsPerChunk, GrepOutTotalChars)

	if !res.Stats.Matched {
		t.Fatalf("stats = %+v, want a match", res.Stats)
	}
	if len(res.Kept) != 1 {
		t.Fatalf("kept = %d chunk(s), want 1", len(res.Kept))
	}
	kept := ChunkTextOf(res.Kept[0])
	if !strings.Contains(kept, "荀正") {
		t.Errorf("kept passage lost the hit: %q", kept)
	}
	if len(kept) >= len(text) {
		t.Errorf("kept %d bytes of %d: the passage was not narrowed at all", len(kept), len(text))
	}
}

// cjkTermsEqual / cjkTermsContain are local helpers: this file needs exact-order
// and membership checks over the derived terms.
func cjkTermsEqual(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func cjkTermsContain(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
