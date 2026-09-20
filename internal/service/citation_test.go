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

package service

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestSentenceSplit_PlainText(t *testing.T) {
	result := sentenceSplit("Hello world. This is a test.")
	if len(result) < 2 {
		t.Errorf("expected at least 2 sentences, got %d: %q", len(result), result)
	}
}

func TestSentenceSplit_Chinese(t *testing.T) {
	result := sentenceSplit("你好世界。这是一个测试。")
	if len(result) < 2 {
		t.Errorf("expected at least 2 sentences, got %d: %q", len(result), result)
	}
}

func TestSentenceSplit_SingleSentence(t *testing.T) {
	result := sentenceSplit("hello world")
	if len(result) != 1 {
		t.Errorf("expected 1 sentence, got %d: %q", len(result), result)
	}
}

func TestSplitAnswer_Basic(t *testing.T) {
	sentences, idx := splitAnswer("Hello world. This is a test.")
	if len(sentences) < 2 {
		t.Errorf("expected >=2, got %d", len(sentences))
	}
	if len(idx) != len(sentences) {
		t.Errorf("idx len %d != sentences len %d", len(idx), len(sentences))
	}
}

func TestSplitAnswer_Empty(t *testing.T) {
	s, i := splitAnswer("")
	if len(s) != 0 || len(i) != 0 {
		t.Errorf("expected empty, got %d, %d", len(s), len(i))
	}
}

func TestSplitAnswer_CodeBlock(t *testing.T) {
	sentences, _ := splitAnswer("Hello. ```code``` World.")
	if len(sentences) < 2 {
		t.Errorf("expected >=2 sentences, got %d", len(sentences))
	}
}

func TestSplitAnswer_ShortSentencesFiltered(t *testing.T) {
	// "Hi" is too short (< 5 chars), should be filtered.
	sentences, _ := splitAnswer("Hi. Hello world and more text here.")
	for _, s := range sentences {
		if len(strings.TrimSpace(s)) < minSentenceLen {
			t.Errorf("short sentence not filtered: %q", s)
		}
	}
}

func TestVecNorm(t *testing.T) {
	if vecNorm([]float64{3, 4}) != 5 {
		t.Error("norm of [3,4] should be 5")
	}
	if vecNorm([]float64{0, 0}) != 0 {
		t.Error("norm of zero vector should be 0")
	}
}

func TestSplitAnswer_Chinese(t *testing.T) {
	sentences, idx := splitAnswer("你好世界。这是一个测试。")
	if len(sentences) < 2 {
		t.Errorf("expected >=2 sentences for Chinese, got %d: %q", len(sentences), sentences)
	}
	if len(idx) != len(sentences) {
		t.Errorf("idx len mismatch: %d vs %d", len(idx), len(sentences))
	}
	for i, s := range sentences {
		if len(s) < minSentenceLen {
			t.Errorf("sentence %d too short: %q", i, s)
		}
		// Each sentence must be valid UTF-8 and end with 。or contain meaningful text.
		if !strings.ContainsAny(s, "。世界测试") && len(s) < 10 {
			t.Errorf("unexpected short sentence without context: %q", s)
		}
	}
}

func TestSplitAnswer_Arabic(t *testing.T) {
	// Arabic: "Hello world. This is a test." in Arabic script
	sentences, _ := splitAnswer("مرحبا بالعالم. هذا اختبار.")
	if len(sentences) == 0 {
		t.Fatal("expected at least 1 sentence for Arabic")
	}
	for _, s := range sentences {
		// Must be valid UTF-8 — no replacement characters or garbled bytes
		if strings.ContainsRune(s, '�') {
			t.Errorf("garbled UTF-8 in Arabic sentence: %q", s)
		}
	}
}

func TestSplitAnswer_Japanese(t *testing.T) {
	sentences, _ := splitAnswer("こんにちは世界。これはテストです。")
	if len(sentences) < 2 {
		t.Errorf("expected >=2 sentences for Japanese, got %d: %q", len(sentences), sentences)
	}
	for _, s := range sentences {
		if strings.ContainsRune(s, '�') {
			t.Errorf("garbled UTF-8 in Japanese sentence: %q", s)
		}
	}
}

func TestSplitAnswer_Korean(t *testing.T) {
	sentences, _ := splitAnswer("안녕하세요 세계. 이것은 테스트입니다.")
	if len(sentences) < 2 {
		t.Errorf("expected >=2 sentences for Korean, got %d: %q", len(sentences), sentences)
	}
	for _, s := range sentences {
		if strings.ContainsRune(s, '�') {
			t.Errorf("garbled UTF-8 in Korean sentence: %q", s)
		}
	}
}

func TestDot(t *testing.T) {
	if dot([]float64{1, 2}, []float64{3, 4}) != 11 {
		t.Error("1*3+2*4 = 11")
	}
}

func TestCosineSimMatrix(t *testing.T) {
	a := [][]float64{{1, 0}, {0, 1}}
	b := [][]float64{{1, 0}, {0.707, 0.707}}
	m := cosineSimMatrix(a, b)
	if math.Abs(m[0][0]-1.0) > 1e-9 {
		t.Errorf("m[0][0] = %f, want 1.0", m[0][0])
	}
	if math.Abs(m[0][1]-0.707) > 1e-3 {
		t.Errorf("m[0][1] = %f", m[0][1])
	}
}

func TestFindCitations(t *testing.T) {
	// Sentence 0 is very similar to chunk 0.
	// Sentence 1 is similar to chunk 1.
	sim := [][]float64{
		{0.9, 0.1},
		{0.1, 0.85},
	}
	cites := findCitations(sim)
	if len(cites) == 0 {
		t.Fatal("expected citations found")
	}
	if c, ok := cites[0]; !ok || len(c) == 0 || c[0] != 0 {
		t.Errorf("sentence 0 should cite chunk 0: %v", cites[0])
	}
	if c, ok := cites[1]; !ok || len(c) == 0 || c[0] != 1 {
		t.Errorf("sentence 1 should cite chunk 1: %v", cites[1])
	}
}

func TestFindCitations_ThresholdDescent(t *testing.T) {
	// All similarities are moderate (0.5) — none above 0.63*0.99=0.62
	// After 0.63*0.8=0.504, still below
	// After 0.504*0.8=0.403, 0.5 > 0.403*0.99=0.399 → found!
	sim := [][]float64{{0.5}}
	cites := findCitations(sim)
	if len(cites) == 0 {
		t.Fatal("expected citations after threshold descent")
	}
}

func TestFindCitations_NoMatch(t *testing.T) {
	sim := [][]float64{{0.1}}
	cites := findCitations(sim)
	if len(cites) != 0 {
		t.Errorf("expected no citations for low similarity")
	}
}

func TestInsertCitationsWithVectors_Happy(t *testing.T) {
	chunks := []SourcedChunk{{ID: "abc123"}, {ID: "def456"}}
	sentences, sIdx := splitAnswer("First sentence is interesting. Second one too.")
	sentenceVecs := [][]float64{{1, 0, 0}, {0, 1, 0}}
	chunkVecs := [][]float64{{1, 0, 0}, {0, 1, 0}}
	answer, cited := InsertCitationsWithVectors(
		"First sentence is interesting. Second one too.",
		chunks, sentenceVecs, chunkVecs, sentences, sIdx)
	if len(cited) == 0 {
		t.Fatal("expected citations")
	}
	// Markers carry the chunk's position in `chunks` (the list the caller returns
	// as the reference), not its chunk id.
	if !strings.Contains(answer, "[ID:0]") {
		t.Errorf("answer should contain [ID:0]: %q", answer)
	}
	if !strings.Contains(answer, "[ID:1]") {
		t.Errorf("answer should contain [ID:1]: %q", answer)
	}
}

func TestInsertCitationsWithVectors_Empty(t *testing.T) {
	answer, cited := InsertCitationsWithVectors("", nil, nil, nil, nil, nil)
	if answer != "" || len(cited) != 0 {
		t.Error("empty input should give empty output")
	}
}

func TestApplyCitations(t *testing.T) {
	chunks := []SourcedChunk{{ID: "c1"}}
	cites := map[int][]int{0: {0}}
	answer, cited := applyCitations("Hello world.", []string{"Hello world."}, []int{0}, cites, chunks)
	if answer != "Hello world. [ID:0]" {
		t.Errorf("got %q", answer)
	}
	if len(cited) != 1 || cited[0] != 0 {
		t.Errorf("cited = %v", cited)
	}
}

// fakeEmbedder implements Embedder with pre-computed vectors.
type fakeEmbedder struct {
	vecs [][]float64
	err  error
}

func (f *fakeEmbedder) Encode(ctx context.Context, texts []string) ([][]float64, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.vecs, nil
}

func TestInsertCitations_Happy(t *testing.T) {
	ctx := t.Context()
	chunks := []SourcedChunk{{ID: "abc123"}, {ID: "def456"}}
	chunkVectors := [][]float64{{1, 0, 0}, {0, 1, 0}}
	embedder := &fakeEmbedder{vecs: [][]float64{{1, 0, 0}, {0, 1, 0}}}
	answer, cited := InsertCitations(ctx, "First sentence. Second sentence here.", chunks, embedder, chunkVectors)
	if len(cited) == 0 {
		t.Fatalf("expected citations, got none. answer=%q", answer)
	}
	if !strings.Contains(answer, "[ID:0]") || !strings.Contains(answer, "[ID:1]") {
		t.Errorf("missing positional [ID:*] markers: %q", answer)
	}
}

func TestInsertCitations_EmptyAnswer(t *testing.T) {
	ctx := t.Context()
	c, _ := InsertCitations(ctx, "", nil, &fakeEmbedder{}, nil)
	if c != "" {
		t.Errorf("empty answer: %q", c)
	}
}

func TestInsertCitations_NoChunks(t *testing.T) {
	ctx := t.Context()
	c, _ := InsertCitations(ctx, "Hello world.", nil, &fakeEmbedder{}, [][]float64{})
	if c != "Hello world." {
		t.Errorf("no chunks should return original: %q", c)
	}
}

func TestInsertCitations_EncodeError(t *testing.T) {
	ctx := t.Context()
	c, _ := InsertCitations(ctx, "Hello world.", []SourcedChunk{{ID: "c1"}}, &fakeEmbedder{err: fmt.Errorf("offline")}, [][]float64{{1, 0}})
	if c != "Hello world." {
		t.Errorf("encode error should return original: %q", c)
	}
}

func TestInsertCitations_EncodeEmpty(t *testing.T) {
	ctx := t.Context()
	c, _ := InsertCitations(ctx, "Hello world.", []SourcedChunk{{ID: "c1"}}, &fakeEmbedder{vecs: [][]float64{}}, [][]float64{{1, 0}})
	if c != "Hello world." {
		t.Errorf("empty encode result should return original: %q", c)
	}
}

func TestCosineSimMatrix_ZeroVectors(t *testing.T) {
	m := cosineSimMatrix([][]float64{{0, 0}}, [][]float64{{1, 2}})
	if m[0][0] != 0 {
		t.Errorf("zero vector sim should be 0: %f", m[0][0])
	}
}

func TestMaxRow(t *testing.T) {
	if maxRow([]float64{1, 3, 2}) != 3 {
		t.Error("max should be 3")
	}
	if maxRow(nil) != 0 {
		t.Error("max of nil should be 0")
	}
}

// TestRepairBadCitationFormats covers the malformed-citation shapes
// RepairBadCitationFormats rewrites to "[ID:N]", including the
// markdown-asterisk-wrapped parenthetical form (**ID:5**) that mirrors Python
// agentic_rag.py:885's re.sub(r"\(\**(ID:\d+)\**\)", r"[\1]", ...).
func TestRepairBadCitationFormats(t *testing.T) {
	cases := []struct{ in, want string }{
		{"(**ID:5**)", "[ID:5]"}, // Python deep-research asterisk form
		{"(*ID: 5*)", "[ID:5]"},  // single star, spaced
		{"(ID:12)", "[ID:12]"},   // plain parenthetical (existing pattern)
		{"[ID: 12]", "[ID:12]"},  // already canonical-ish but spaced
		{"Built in 1865 (**ID:1**).", "Built in 1865 [ID:1]."},
		{"plain text no cites", "plain text no cites"},
	}
	for _, c := range cases {
		if got := RepairBadCitationFormats(c.in); got != c.want {
			t.Errorf("RepairBadCitationFormats(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// -----------------------------------------------------------------------
// RepairSlotCitations — leaked "[ID:Slot N]" markers (Bug: slot-table
// citations in the final answer, unlocatable by the user).
// -----------------------------------------------------------------------

func TestRepairSlotCitations_MapsSlotToEvidenceChunk(t *testing.T) {
	chunks := []map[string]interface{}{
		{"chunk_id": "c1", "content": "one"},
		{"chunk_id": "c2", "content": "two"},
		{"id": "c9", "content": "slot evidence"},
	}
	answer := "Gustave Eiffel designed it [ID:Slot 0]. It opened in 1889."
	got := RepairSlotCitations(answer, map[string][]string{"0": {"c9", "missing"}}, chunks)
	if strings.Contains(got, "Slot") {
		t.Fatalf("slot marker leaked: %q", got)
	}
	if !strings.Contains(got, "[ID:2]") {
		t.Fatalf("slot marker not mapped to the evidence chunk position: %q", got)
	}
	if got != "Gustave Eiffel designed it [ID:2]. It opened in 1889." {
		t.Fatalf("unexpected rewrite: %q", got)
	}
}

func TestRepairSlotCitations_DropsUnresolvable(t *testing.T) {
	chunks := []map[string]interface{}{{"chunk_id": "c1", "content": "one"}}
	answer := "Fact one [ID:Slot 0]. Fact two [Slot 1]."
	got := RepairSlotCitations(answer, map[string][]string{"0": {"not-in-pool"}, "1": {"also-missing"}}, chunks)
	if strings.Contains(strings.ToLower(got), "slot") {
		t.Fatalf("unresolvable slot markers must be dropped, got %q", got)
	}
	if got != "Fact one . Fact two ." {
		t.Fatalf("unexpected rewrite: %q", got)
	}
}

func TestRepairSlotCitations_MarkerVariants(t *testing.T) {
	chunks := []map[string]interface{}{{"chunk_id": "a"}, {"id": "b"}}
	cases := []struct {
		marker string
		want   string
	}{
		{"[ID:Slot 1]", "[ID:1]"},
		{"[ID: slot 1]", "[ID:1]"},
		{"[Slot 1]", "[ID:1]"},
		{"[slot 1]", "[ID:1]"},
	}
	for _, c := range cases {
		got := RepairSlotCitations("x "+c.marker+" y", map[string][]string{"1": {"b"}}, chunks)
		if !strings.Contains(got, c.want) {
			t.Errorf("marker %q: got %q, want it rewritten to %q", c.marker, got, c.want)
		}
	}
}

func TestRepairSlotCitations_NoopWithoutSlotMarkersOrMap(t *testing.T) {
	answer := "Plain answer [ID:0]."
	if got := RepairSlotCitations(answer, nil, nil); got != answer {
		t.Fatalf("nil map must be a no-op, got %q", got)
	}
	if got := RepairSlotCitations(answer, map[string][]string{"0": {"c"}}, nil); got != answer {
		t.Fatalf("no slot markers must be a no-op, got %q", got)
	}
}

// -----------------------------------------------------------------------
// ExpandRangeCitations — range-merged citations ("[ID:1-3]") the model
// produces on its own (no code path emits them).
// -----------------------------------------------------------------------

func TestExpandRangeCitations(t *testing.T) {
	cases := []struct {
		name     string
		answer   string
		poolSize int
		want     string
	}{
		{
			"plain range",
			"Fact [ID:1-3].",
			5,
			"Fact [ID:1][ID:2][ID:3].",
		},
		{
			"spaced dash variant",
			"Fact [ID: 1 - 3].",
			5,
			"Fact [ID:1][ID:2][ID:3].",
		},
		{
			"tilde variant",
			"Fact [ID:1~3].",
			5,
			"Fact [ID:1][ID:2][ID:3].",
		},
		{
			"reversed bounds swap",
			"Fact [ID:3-1].",
			5,
			"Fact [ID:1][ID:2][ID:3].",
		},
		{
			"single-bound range",
			"Fact [ID:2-2].",
			5,
			"Fact [ID:2].",
		},
		{
			"out of bounds dropped",
			"Fact [ID:99-102].",
			5,
			"Fact .",
		},
		{
			"partially out of bounds dropped",
			"Fact [ID:2-99].",
			5,
			"Fact .",
		},
		{
			"empty pool dropped",
			"Fact [ID:1-3].",
			0,
			"Fact .",
		},
		{
			"no ranges untouched",
			"Fact [ID:1] and [ID:2].",
			5,
			"Fact [ID:1] and [ID:2].",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ExpandRangeCitations(c.answer, c.poolSize); got != c.want {
				t.Fatalf("ExpandRangeCitations(%q, %d) = %q, want %q", c.answer, c.poolSize, got, c.want)
			}
		})
	}
}

// TestDecorateHarnessAnswerExpandsRangeCitations pins the end-to-end fix for
// the [ID:1-3] report: the final answer carries only individual citations,
// each resolving to a pooled chunk, and only the cited doc reaches the
// reference.
func TestDecorateHarnessAnswerExpandsRangeCitations(t *testing.T) {
	kbinfos := map[string]interface{}{
		"chunks": []map[string]interface{}{
			{"chunk_id": "c0", "content_with_weight": "a", "doc_id": "d1", "docnm_kwd": "Doc One"},
			{"chunk_id": "c1", "content_with_weight": "b", "doc_id": "d1", "docnm_kwd": "Doc One"},
			{"chunk_id": "c2", "content_with_weight": "c", "doc_id": "d1", "docnm_kwd": "Doc One"},
			{"chunk_id": "c3", "content_with_weight": "d", "doc_id": "d1", "docnm_kwd": "Doc One"},
		},
		"doc_aggs": []interface{}{
			map[string]interface{}{"doc_id": "d1", "doc_name": "Doc One"},
			map[string]interface{}{"doc_id": "d2", "doc_name": "Doc Two"},
		},
	}
	s := &ChatPipelineService{}
	res := s.decorateHarnessAnswer("The range claim holds [ID:1-3].", kbinfos, nil, nil, true)

	if strings.Contains(res.Answer, "1-3") {
		t.Fatalf("final answer still carries the range citation: %q", res.Answer)
	}
	// Expanded markers keep the 0-based indexes the client resolves against
	// reference.chunks.
	for _, want := range []string{"[ID:1]", "[ID:2]", "[ID:3]"} {
		if !strings.Contains(res.Answer, want) {
			t.Fatalf("final answer missing expanded citation %s: %q", want, res.Answer)
		}
	}
	// Every cited chunk belongs to Doc One, so the uncited Doc Two must be
	// filtered out of the reference (recall_docs = cited docs).
	if aggs, _ := res.Reference["doc_aggs"].([]interface{}); len(aggs) != 1 {
		t.Fatalf("reference doc_aggs = %#v, want the single cited doc", res.Reference["doc_aggs"])
	}
}

func TestCitationStreamFilter(t *testing.T) {
	var filter citationStreamFilter
	want := "Answer\n## Heading\n[2024][guide](https://example.com) end"
	got := ""
	for _, delta := range []string{"Answer[", "ID:0][ID:1-", "3][ID:Slot 0](ID: 2)#", "#0", "$", "$\n#", "# Heading\n[2024][guide](https://example.com) end"} {
		got += filter.write(delta)
		if !strings.HasPrefix(want, got) {
			t.Fatalf("stream leaked citation text: %q", got)
		}
	}
	got += filter.flush()
	if got != want {
		t.Fatalf("filtered answer=%q, want %q", got, want)
	}
}
