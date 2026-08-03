package tool

import (
	"strings"
	"testing"
)

// TestSplitKeywords_BigramFallback asserts comma terms are used when >=3, and
// space-split bigrams otherwise.
func TestSplitKeywords_BigramFallback(t *testing.T) {
	got := splitKeywords("alpha, beta, gamma")
	if len(got) != 3 {
		t.Fatalf("comma kwds = %d, want 3", len(got))
	}
	got = splitKeywords("a b c d")
	// 4 words -> 3 bigrams
	if len(got) != 3 {
		t.Fatalf("bigram kwds = %d, want 3", len(got))
	}
	if got[0] != "a b" {
		t.Errorf("bigram[0] = %q, want \"a b\"", got[0])
	}
}

// TestNarrowContent_KeepsKeywordSentence asserts narrowing keeps the
// keyword-bearing sentence and its neighbours, and highlights the keyword.
func TestNarrowContent_KeepsKeywordSentence(t *testing.T) {
	content := "The introduction is boring. The key insight about rocket engines is here. The conclusion is short."
	nc, ok := narrowContent(content, []string{"rocket"})
	if !ok {
		t.Fatal("expected keyword match")
	}
	if !strings.Contains(nc, "key insight") {
		t.Errorf("narrowed content missing keyword sentence: %q", nc)
	}
	if !strings.Contains(nc, "<em>rocket</em>") {
		t.Errorf("narrowed content missing highlight: %q", nc)
	}
}

// TestNarrowContent_NoKeyword asserts narrowing returns false when no keyword
// occurs.
func TestNarrowContent_NoKeyword(t *testing.T) {
	if _, ok := narrowContent("no match here at all", []string{"zzz"}); ok {
		t.Fatal("expected no match")
	}
}

// TestSplitSentences_DigitPeriod asserts "3.14" is not split into two sentences.
func TestSplitSentences_DigitPeriod(t *testing.T) {
	sents := splitSentences("pi is 3.14 and e is 2.71. end.")
	found := false
	for _, s := range sents {
		if strings.Contains(s, "3.14") {
			found = true
		}
	}
	if !found {
		t.Fatalf("digit-period sentence lost: %v", sents)
	}
}

// TestNarrowByKeywords_DropsKeywordless asserts chunks without keywords are
// dropped and deduped by narrowed content hash.
func TestNarrowByKeywords_DropsKeywordless(t *testing.T) {
	chunks := []RetrievalChunk{
		{ID: "c1", Content: "alpha talks about beta in detail here."},
		{ID: "c2", Content: "unrelated content entirely."},
		{ID: "c3", Content: "alpha talks about beta in detail here."}, // dup of c1
	}
	out := narrowByKeywords(chunks, "beta")
	if len(out) != 1 {
		t.Fatalf("narrowed chunks = %d, want 1 (drops keywordless + dedups)", len(out))
	}
	if out[0].ID != "c1" {
		t.Errorf("kept chunk = %q, want c1", out[0].ID)
	}
}
