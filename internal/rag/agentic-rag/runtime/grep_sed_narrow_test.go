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

package runtime

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

// TestGrepWordsAreTheCallersOwnWords pins the split between the two readings of
// one query: the terms LOCATE (and may decompose an unbroken clause into CJK
// windows), while the words are what the caller actually wrote — the only ones a
// seat probe may spend a retrieval on.
//
// Measured (2026-09-15): a seat pass built from the locate reading probed 羽斩 /
// 杀的 / 的有 / 领名 / 单温, reported 14 of 20 probes "absent", and never reached a
// single name the question was missing.
func TestGrepWordsAreTheCallersOwnWords(t *testing.T) {
	// A caller-enumerated list is words in both readings — minus the single-rune
	// particles ("斩" is a predicate, not something to probe as a name; the phrase
	// search carries it).
	listed := "关羽 斩 颜良 文丑 车胄"
	if got := GrepWordsFromQuery(listed); strings.Join(got, "|") != "关羽|颜良|文丑|车胄" {
		t.Errorf("GrepWordsFromQuery(%q) = %v, want the caller's own words (single-rune tokens are particles)", listed, got)
	}

	// An unbroken clause names no individual: the locate reading decomposes it
	// into windows, the words reading yields nothing to probe.
	clause := "关羽斩杀敌将名单温酒斩华雄"
	if got := GrepWordsFromQuery(clause); len(got) != 0 {
		t.Errorf("GrepWordsFromQuery(%q) = %v, want none: a clause is not a word to probe", clause, got)
	}
	terms := GrepTermsFromQuery(clause)
	if len(terms) == 0 {
		t.Fatalf("GrepTermsFromQuery(%q) = none, want the two-rune windows grep locates with", clause)
	}
	for _, term := range terms {
		if term == clause {
			t.Errorf("GrepTermsFromQuery(%q) returned the whole clause; the point of the windows is that the clause itself locates nothing", clause)
		}
	}

	// The name boundary is the one this file already declares (cjkPhraseRunes):
	// up to four runes is still a name, past it is prose.
	for _, name := range []string{"孔秀", "夏侯存", "成吉思汗"} {
		if got := GrepWordsFromQuery(name); len(got) != 1 || got[0] != name {
			t.Errorf("GrepWordsFromQuery(%q) = %v, want the name itself", name, got)
		}
	}
	if got := GrepWordsFromQuery("成吉思汗东征"); len(got) != 0 {
		t.Errorf("GrepWordsFromQuery = %v, want none: past the name boundary a CJK token is prose", got)
	}
}

// TestGrepPatternIsAMatchNotATermFilter pins the grep half of grep_search: the
// pattern decides which candidates are evidence, and it keeps its own semantics
// instead of being degraded into "does the text carry one of these words".
//
// The ordering case is the one a term-locate pass can never express: both chunks
// carry 关公 and 斩, and only the written order distinguishes the clause that says
// he killed from the one that says he killed and then went back to camp.
func TestGrepPatternIsAMatchNotATermFilter(t *testing.T) {
	chunks := []map[string]any{
		{"chunk_id": "a", "content": "关公马快，早赶上文丑，脑后一刀，斩于马下。"},
		{"chunk_id": "b", "content": "斩将之后，关公回营，众将皆来称贺。"},
		{"chunk_id": "c", "content": "话说曹操引军而回，不在话下。"},
		{"chunk_id": "d", "content": "荀正 引军来战，被云长一刀斩于马下。"},
	}
	re := grepPatternOf("关公.*斩")
	if re == nil {
		t.Fatal("关公.*斩 carries pattern syntax and must be read as a pattern")
	}
	kept, matched := matchGrepPattern(chunks, re, contextCharBudget, GrepOutTotalChars)
	if matched != 1 || len(kept) != 1 {
		t.Fatalf("matched=%d kept=%d, want exactly the chunk whose text has 关公 BEFORE 斩", matched, len(kept))
	}
	if id := ChunkIDOf(kept[0]); id != "a" {
		t.Errorf("kept %q, want \"a\": the pattern's ORDER must be enforced, not just its words", id)
	}
	// The window is the clause around the match, not the whole chunk.
	if txt := ChunkTextOf(kept[0]); !strings.Contains(txt, "斩") {
		t.Errorf("window %q does not carry the match", txt)
	}

	// An alternation is a batch: it matches each alternative and reports per-term
	// reach over the same candidate set, which is how "which of these did the
	// corpus answer" becomes readable.
	alt := grepPatternOf("华雄|荀正|管亥")
	if alt == nil {
		t.Fatal("华雄|荀正|管亥 must be a pattern")
	}
	keptAlt, matchedAlt := matchGrepPattern(chunks, alt, contextCharBudget, GrepOutTotalChars)
	if matchedAlt != 1 || len(keptAlt) != 1 {
		t.Fatalf("alternation matched=%d kept=%d, want the one candidate carrying one of the three names", matchedAlt, len(keptAlt))
	}
	if id := ChunkIDOf(keptAlt[0]); id != "d" {
		t.Errorf("kept %q, want \"d\" (the only candidate carrying 荀正)", id)
	}
	located, counts, absent := termReach(chunks, []string{"华雄", "荀正", "管亥"})
	if strings.Join(located, ",") != "荀正" || counts[0] != 1 {
		t.Errorf("termReach located=%v counts=%v, want only 荀正 reached once", located, counts)
	}
	if strings.Join(absent, ",") != "华雄,管亥" {
		t.Errorf("absent=%v, want 华雄 and 管亥 reported as NOT reached by this query", absent)
	}
}

// TestPlainQueryStaysOnTheTermLocatePath guards the other direction: pattern
// syntax is the ONLY trigger, so every ordinary question keeps the behaviour it
// had (locate terms -> windows), and nothing about it changes.
func TestPlainQueryStaysOnTheTermLocatePath(t *testing.T) {
	for _, q := range []string{
		"关羽 斩颜良",
		"三国演义中关羽杀了多少有姓名的人物",
		"华为2023年营收",
		"partner of the 1984 Olympic keelboat competitor",
	} {
		if re := grepPatternOf(q); re != nil {
			t.Errorf("grepPatternOf(%q) compiled a pattern; a plain query must take the term-locate path", q)
		}
	}
}

// TestReachLineNamesWhatTheBatchMissed pins the line the MODEL reads: a batch of
// names comes back with per-term reach, so a member that was never reached is
// visible as such instead of looking exactly like a member nobody asked about.
//
// The capability was already there (the pattern engine, the per-term accounting,
// the log line); what was missing is that only the LOG could read it.
func TestReachLineNamesWhatTheBatchMissed(t *testing.T) {
	chunks := []map[string]any{
		{"chunk_id": "c1", "content": "云长手起一刀，斩华雄于马下"},
		{"chunk_id": "c2", "content": "华雄又斩了潘凤"},
	}
	q := "华雄|荀正|管亥"
	line := GrepReachLine(q, chunks, ReachTermsOf(q))
	for _, want := range []string{"[reach]", "华雄(2)", "NOT reached by this query"} {
		if !strings.Contains(line, want) {
			t.Errorf("line %q missing %q", line, want)
		}
	}
	// Both unanswered names are named, and the wording refuses to call them absent.
	for _, missed := range []string{"荀正", "管亥"} {
		if !strings.Contains(line, missed) {
			t.Errorf("line %q must name %q as not reached", line, missed)
		}
	}

	// A PATTERN reports on its OPERANDS: they are what the keyword leg searched
	// for, so "关公.*斩" is reported as 关公 and 斩, not as one clause.
	pq := "关公.*斩|云长.*斩"
	pline := GrepReachLine(pq, []map[string]any{{"chunk_id": "c3", "content": "关公勒马，一刀斩之"}}, ReachTermsOf(pq))
	for _, want := range []string{"关公(1)", "云长"} {
		if !strings.Contains(pline, want) {
			t.Errorf("pattern line %q missing %q", pline, want)
		}
	}

	// No terms -> no line at all (never an empty "[reach]" heading).
	if got := GrepReachLine("", chunks, nil); got != "" {
		t.Errorf("empty reach = %q, want no line", got)
	}
}
