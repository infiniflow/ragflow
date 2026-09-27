//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except under the License.
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package chunker

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"ragflow/internal/ingestion/component/schema"
)

func testCtx() context.Context { return context.Background() }

// charTokenizer installs a deterministic "1 token == 1 rune" stub (mirroring
// the Python test_token_cap suite, which fakes num_tokens_from_string as
// len(text)). This makes the cap a hard rune ceiling so assertions are fully
// reproducible. Callers must defer restoreTokenizer().
func charTokenizer() {
	numTokens = func(s string) int {
		if s == "" {
			return 0
		}
		return utf8.RuneCountInString(s)
	}
	trimToTokenLimit = func(s string, limit int) string {
		if limit < 0 {
			limit = 0
		}
		if utf8.RuneCountInString(s) <= limit {
			return s
		}
		return runePrefix(s, limit)
	}
}

// restoreTokenizer resets the tokenizer seam to the real implementation.
func restoreTokenizer() {
	numTokens = realNumTokens
	trimToTokenLimit = realTrimToTokenLimit
}

// assertCapInvariants checks the post-_enforce_token_cap guarantees for a
// slice of built chunks: every text piece is <= cap (re-tokenized), non-text
// chunks are untouched, and the concatenated text reproduces the source
// (lossless). The optional trailing newline that build_chunks appends is
// stripped before comparison, matching Python's rstrip("\n").
func assertCapInvariants(t *testing.T, chunks []map[string]any, cap int, source string) {
	t.Helper()
	var got strings.Builder
	for _, ck := range chunks {
		text := toString(ck["text"])
		dt := toStringOrDefault(ck["doc_type_kwd"], "text")
		if dt != "text" {
			got.WriteString(text)
			continue
		}
		if n := titleTokenCount(text); n > cap {
			t.Errorf("chunk exceeds cap: tokens=%d (cap=%d) text=%q", n, cap, text)
		}
		got.WriteString(text)
	}
	if strings.TrimRight(got.String(), "\n") != strings.TrimRight(source, "\n") {
		t.Errorf("lossless check failed:\n got=%q\nwant=%q", got.String(), source)
	}
}

func TestTitleTokenCount_CharStub(t *testing.T) {
	charTokenizer()
	defer restoreTokenizer()
	if got := titleTokenCount("hello"); got != 5 {
		t.Errorf("titleTokenCount(char) = %d, want 5", got)
	}
}

func TestTitleTokenCount_OfflineFallback(t *testing.T) {
	// Simulate an unavailable tokenizer (num_tokens_from_string returns 0).
	saved := numTokens
	numTokens = func(string) int { return 0 }
	defer func() { numTokens = saved }()
	// With the offline fallback, non-empty text counts as its rune length.
	if got := titleTokenCount("hello世界"); got != 7 {
		t.Errorf("offline fallback token count = %d, want 7", got)
	}
	if got := titleTokenCount(""); got != 0 {
		t.Errorf("empty text token count = %d, want 0", got)
	}
}

func TestTitleSentenceSplit_Boundaries(t *testing.T) {
	// Chinese boundaries. The final fragment without a trailing delimiter is
	// kept as its own sentence (matches Python re.split reassembly).
	zh := "第一句。第二句！第三句？第四句；尾"
	got := titleSentenceSplit(zh)
	if len(got) != 5 {
		t.Fatalf("zh split = %d sentences, want 5: %v", len(got), got)
	}
	wantEnds := []string{"。", "！", "？", "；"}
	for i := 0; i < 4; i++ {
		if !strings.HasSuffix(got[i], wantEnds[i]) {
			t.Errorf("sentence %d = %q, want suffix %q", i, got[i], wantEnds[i])
		}
	}
	if got[4] != "尾" {
		t.Errorf("trailing sentence = %q, want \"尾\"", got[4])
	}
	// English ". " boundary (the Python #18455 regex includes `\. `).
	en := "Hello. World. Foo"
	eg := titleSentenceSplit(en)
	if len(eg) != 3 {
		t.Fatalf("en split = %d, want 3: %v", len(eg), eg)
	}
	if eg[0] != "Hello. " || eg[1] != "World. " || eg[2] != "Foo" {
		t.Errorf("en split = %v, want [Hello.  World.  Foo]", eg)
	}
}

func TestTitleSentenceSplit_Lossless(t *testing.T) {
	zh := "第一句。第二句！第三句？第四句；尾"
	if got := strings.Join(titleSentenceSplit(zh), ""); got != zh {
		t.Errorf("sentence split not lossless: %q", got)
	}
	en := "Hello. World. Foo"
	if got := strings.Join(titleSentenceSplit(en), ""); got != en {
		t.Errorf("en sentence split not lossless: %q", got)
	}
}

func TestEnforceTitleTokenCap_CapZeroNoop(t *testing.T) {
	charTokenizer()
	defer restoreTokenizer()
	chunks := []map[string]any{
		{"text": strings.Repeat("x", 100)},
	}
	got := enforceTitleTokenCap(chunks, 0)
	if len(got) != 1 {
		t.Fatalf("cap=0 must keep 1 chunk, got %d", len(got))
	}
	if toString(got[0]["text"]) != strings.Repeat("x", 100) {
		t.Errorf("cap=0 altered text: %q", toString(got[0]["text"]))
	}
}

func TestEnforceTitleTokenCap_WithinCapUnchanged(t *testing.T) {
	charTokenizer()
	defer restoreTokenizer()
	chunks := []map[string]any{{"text": "S00。S01。"}}
	got := enforceTitleTokenCap(chunks, 512)
	if len(got) != 1 {
		t.Fatalf("within-cap chunk split unexpectedly: %d chunks", len(got))
	}
	if toString(got[0]["text"]) != "S00。S01。" {
		t.Errorf("within-cap text altered: %q", toString(got[0]["text"]))
	}
}

func TestEnforceTitleTokenCap_OverCapResplits(t *testing.T) {
	charTokenizer()
	defer restoreTokenizer()
	body := strings.Join(joinSentences(12), "")
	chunks := []map[string]any{{"text": body}}
	got := enforceTitleTokenCap(chunks, 20)
	if len(got) <= 1 {
		t.Fatalf("expected oversized chunk to be split, got %d", len(got))
	}
	assertCapInvariants(t, got, 20, body)
	for _, ck := range got {
		if !strings.HasSuffix(strings.TrimRight(toString(ck["text"]), "\n"), "。") {
			t.Errorf("chunk cut mid-sentence: %q", toString(ck["text"]))
		}
	}
}

func TestEnforceTitleTokenCap_NonTextAtomic(t *testing.T) {
	charTokenizer()
	defer restoreTokenizer()
	big := strings.Repeat("x", 200)
	chunks := []map[string]any{{"text": big, "doc_type_kwd": "table"}}
	got := enforceTitleTokenCap(chunks, 20)
	if len(got) != 1 {
		t.Fatalf("table chunk must stay atomic, got %d", len(got))
	}
	if toString(got[0]["text"]) != big {
		t.Errorf("table text altered: len=%d", len(toString(got[0]["text"])))
	}
	// image too
	img := []map[string]any{{"text": big, "doc_type_kwd": "image"}}
	if got := enforceTitleTokenCap(img, 20); len(got) != 1 {
		t.Errorf("image chunk must stay atomic, got %d", len(got))
	}
}

func TestEnforceTitleTokenCap_BoundarylessHardSplit(t *testing.T) {
	charTokenizer()
	defer restoreTokenizer()
	body := strings.Repeat("x", 100)
	chunks := []map[string]any{{"text": body}}
	got := enforceTitleTokenCap(chunks, 20)
	if len(got) <= 1 {
		t.Fatalf("boundary-less run must be hard-split, got %d", len(got))
	}
	assertCapInvariants(t, got, 20, body)
}

func TestEnforceTitleTokenCap_TokenizerZeroFallback(t *testing.T) {
	// numTokens reports 0 everywhere -> offline char fallback must still cap.
	saved := numTokens
	numTokens = func(string) int { return 0 }
	defer func() { numTokens = saved }()
	body := strings.Repeat("x", 100)
	chunks := []map[string]any{{"text": body}}
	got := enforceTitleTokenCap(chunks, 20)
	if len(got) <= 1 {
		t.Fatalf("cap must apply even when tokenizer reports 0, got %d", len(got))
	}
	assertCapInvariants(t, got, 20, body)
}

// TestHardSplitByTokens_EnglishRemainderStaysWhole pins the over-fragmentation
// fix: a remainder that already satisfies the TOKEN cap must be kept whole even
// when its RUNE count exceeds the cap (English text is ~4 runes/token). The
// char stub (1 rune == 1 token) makes the two counts identical and masks this,
// so this test uses a 3-runes-per-token stub.
func TestHardSplitByTokens_EnglishRemainderStaysWhole(t *testing.T) {
	savedNum, savedTrim := numTokens, trimToTokenLimit
	// 3 runes == 1 token.
	numTokens = func(s string) int {
		if s == "" {
			return 0
		}
		return (utf8.RuneCountInString(s) + 2) / 3
	}
	trimToTokenLimit = func(s string, limit int) string {
		maxRunes := limit * 3
		if utf8.RuneCountInString(s) <= maxRunes {
			return s
		}
		return runePrefix(s, maxRunes)
	}
	defer func() { numTokens, trimToTokenLimit = savedNum, savedTrim }()

	const cap = 100
	// 600 runes == 200 tokens == exactly 2 cap units. The second unit's
	// remainder (300 runes == 100 tokens) is within the cap and must NOT be
	// re-cut on runes.
	body := strings.Repeat("ab", 300)
	got := hardSplitByTokens(body, cap)
	if len(got) != 2 {
		t.Fatalf("hardSplitByTokens produced %d pieces, want 2 (in-cap remainder must stay whole)", len(got))
	}
	if strings.Join(got, "") != body {
		t.Errorf("hard-split not lossless: %d runes vs %d", utf8.RuneCountInString(strings.Join(got, "")), utf8.RuneCountInString(body))
	}
	for i, p := range got {
		if n := numTokens(p); n > cap {
			t.Errorf("piece %d exceeds cap: tokens=%d (cap=%d)", i, n, cap)
		}
	}
}

func TestEnforceTitleTokenCap_SubChunksHaveSlicedPositions(t *testing.T) {
	charTokenizer()
	defer restoreTokenizer()
	// Single 5-tuple row [page,left,right,top,bottom] with a height of 30.
	pos := [][]float64{{1, 10, 200, 50, 80}}
	body := strings.Join(joinSentences(12), "") // 48 runes
	chunks := []map[string]any{
		{"text": body, "positions": pos},
	}
	got := enforceTitleTokenCap(chunks, 20)
	if len(got) != 3 { // 20 + 20 + 8 runes
		t.Fatalf("expected split into 3 chunks, got %d", len(got))
	}
	// Each sub-chunk's positions must be vertically sliced by its rune share
	// of the whole body (heights 12.5 / 12.5 / 5 on the 30-high box).
	wantBounds := [][2]float64{{50, 62.5}, {62.5, 75}, {75, 80}}
	for i := range got {
		v, ok := got[i]["positions"].([][]float64)
		if !ok || len(v) != 1 {
			t.Fatalf("sub-chunk %d positions = %#v, want one sliced row", i, got[i]["positions"])
		}
		row := v[0]
		if row[0] != 1 || row[1] != 10 || row[2] != 200 {
			t.Errorf("sub-chunk %d kept columns wrong: %v", i, row)
		}
		if math.Abs(row[3]-wantBounds[i][0]) > 1e-9 || math.Abs(row[4]-wantBounds[i][1]) > 1e-9 {
			t.Errorf("sub-chunk %d bounds = [%v,%v], want [%v,%v]",
				i, row[3], row[4], wantBounds[i][0], wantBounds[i][1])
		}
	}
}

// TestEnforceTitleTokenCap_MalformedPositionsFallbackKeepsOriginal pins the
// fallback contract: a position matrix that cannot be sliced (no valid
// [page,left,right,top,bottom] rows) is carried to every sub-chunk verbatim,
// so the preview is degraded to the shared coarse bbox instead of being lost.
func TestEnforceTitleTokenCap_MalformedPositionsFallbackKeepsOriginal(t *testing.T) {
	charTokenizer()
	defer restoreTokenizer()
	body := strings.Join(joinSentences(12), "")
	src := []any{map[string]any{"page": float64(1)}}
	chunks := []map[string]any{
		{"text": body, "_pdf_positions": src},
	}
	got := enforceTitleTokenCap(chunks, 20)
	if len(got) <= 1 {
		t.Fatalf("expected split, got %d", len(got))
	}
	for i := range got {
		v, ok := got[i]["_pdf_positions"].([]any)
		if !ok || len(v) == 0 {
			t.Fatalf("sub-chunk %d _pdf_positions = %#v, want the original matrix", i, got[i]["_pdf_positions"])
		}
		if !reflect.DeepEqual(v[0], src[0]) {
			t.Errorf("sub-chunk %d _pdf_positions = %#v, want the source value verbatim", i, v[0])
		}
	}
}

func TestEnforceTitleTokenCap_UnknownPositionsTypeCarriedAsIs(t *testing.T) {
	charTokenizer()
	defer restoreTokenizer()
	body := strings.Join(joinSentences(12), "")
	// An unknown position value type (not [][]float64 / json.RawMessage) is
	// shallow-copied to every sub-chunk verbatim; the cap split never inspects
	// or normalizes position payloads.
	chunks := []map[string]any{
		{"text": body, "positions": "not-a-matrix"},
	}
	got := enforceTitleTokenCap(chunks, 20)
	if len(got) <= 1 {
		t.Fatalf("expected split, got %d", len(got))
	}
	for i := range got {
		v, ok := got[i]["positions"].(string)
		if !ok || v != "not-a-matrix" {
			t.Errorf("sub-chunk %d positions = %#v, want the source value carried as-is", i, got[i]["positions"])
		}
	}
}

func TestEnforceTitleTokenCap_GreedyGrouping(t *testing.T) {
	charTokenizer()
	defer restoreTokenizer()
	// 12 sentences, 4 runes each, cap 20 -> 5 sentences/chunk (20), not 1 each.
	body := strings.Join(joinSentences(12), "")
	chunks := []map[string]any{{"text": body}}
	got := enforceTitleTokenCap(chunks, 20)
	if len(got) != 3 { // 5+5+2
		t.Fatalf("greedy grouping produced %d chunks, want 3", len(got))
	}
	assertCapInvariants(t, got, 20, body)
}

// joinSentences builds 12 sentences "S00。".."S11。" mirroring the Python
// test_hierarchy_oversized_chunk_respects_cap body.
func joinSentences(n int) []string {
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, sprintfSentence(i))
	}
	return out
}

func sprintfSentence(i int) string {
	return fmt.Sprintf("S%02d。", i)
}

// TestSprintfSentence_ThreeDigits pins the %02d formatting for 3-digit
// indexes: the old hand-rolled twoDigit derived each digit from a single rune
// addition and broke at i >= 100 (produced ":0").
func TestSprintfSentence_ThreeDigits(t *testing.T) {
	if got := sprintfSentence(100); got != "S100。" {
		t.Errorf("sprintfSentence(100) = %q, want \"S100。\"", got)
	}
}

func TestJoinSentences_Formatting(t *testing.T) {
	got := joinSentences(15)
	if len(got) != 15 {
		t.Fatalf("joinSentences(15) = %d, want 15", len(got))
	}
	if got[14] != "S14。" {
		t.Errorf("joinSentences(15)[14] = %q, want \"S14。\"", got[14])
	}
	if got[9] != "S09。" {
		t.Errorf("joinSentences(15)[9] = %q, want \"S09。\"", got[9])
	}
}

// ---------------------------------------------------------------------------
// Pipeline (integration) tests: cap applied through invokeGroup/invokeHierarchy
// ---------------------------------------------------------------------------

func newTitleParam(t *testing.T, method string, cap int, levels [][]string) titleChunkerParam {
	t.Helper()
	p := defaultsTitle()
	conf := map[string]any{"method": method, "chunk_token_cap": cap}
	if levels != nil {
		lv := make([]any, 0, len(levels))
		for _, g := range levels {
			inner := make([]any, 0, len(g))
			for _, s := range g {
				inner = append(inner, s)
			}
			lv = append(lv, inner)
		}
		conf["levels"] = lv
	}
	if method == "hierarchy" {
		conf["hierarchy"] = 1
	}
	p.Update(conf)
	// NOTE: validation is intentionally skipped here — the char-stub tests use
	// sub-128 caps (e.g. 20) that the production Validate() rejects, mirroring
	// the Python suite which stubs out check(). Range validation is covered
	// separately by TestTitleChunkerParam_ChunkTokenCapValidate.
	return p
}

func TestTitleCap_GroupPipeline_RespectsCap(t *testing.T) {
	charTokenizer()
	defer restoreTokenizer()
	body := strings.Join(joinSentences(12), "")
	p := newTitleParam(t, "group", 20, [][]string{{`^# `}})
	inputs := map[string]any{"output_format": "text", "text": body}
	got, err := invokeGroup(testCtx(), nil, inputs, &p)
	if err != nil {
		t.Fatalf("invokeGroup: %v", err)
	}
	chunks := got["chunks"].([]map[string]any)
	if len(chunks) <= 1 {
		t.Fatalf("expected split, got %d", len(chunks))
	}
	assertCapInvariants(t, chunks, 20, body)
}

// TestTitleCap_GroupPipeline_ValidatedInRangeCap exercises the wired path with
// a cap inside the production-validated range (128..8000). The char-stub suite
// otherwise only uses sub-128 caps that production Validate() rejects, so this
// is the only end-to-end coverage of an accepted configuration.
func TestTitleCap_GroupPipeline_ValidatedInRangeCap(t *testing.T) {
	charTokenizer()
	defer restoreTokenizer()
	body := strings.Repeat("ab", 200) // 400 stub tokens > cap 128
	p := newTitleParam(t, "group", 128, [][]string{{`^# `}})
	if err := p.TitleChunkerParam.Validate(); err != nil {
		t.Fatalf("cap=128 must pass production Validate: %v", err)
	}
	inputs := map[string]any{"output_format": "text", "text": body}
	got, err := invokeGroup(testCtx(), nil, inputs, &p)
	if err != nil {
		t.Fatalf("invokeGroup: %v", err)
	}
	chunks := got["chunks"].([]map[string]any)
	if len(chunks) <= 1 {
		t.Fatalf("expected cap=128 to re-split the oversized body, got %d", len(chunks))
	}
	assertCapInvariants(t, chunks, 128, body)
}

func TestTitleCap_HierarchyPipeline_RespectsCap(t *testing.T) {
	charTokenizer()
	defer restoreTokenizer()
	body := strings.Join(joinSentences(12), "")
	p := newTitleParam(t, "hierarchy", 20, [][]string{{`^# `}})
	inputs := map[string]any{"output_format": "text", "text": body}
	got, err := invokeHierarchy(testCtx(), nil, inputs, &p)
	if err != nil {
		t.Fatalf("invokeHierarchy: %v", err)
	}
	chunks := got["chunks"].([]map[string]any)
	if len(chunks) <= 1 {
		t.Fatalf("expected split, got %d", len(chunks))
	}
	assertCapInvariants(t, chunks, 20, body)
}

func TestTitleCap_GroupPipeline_CapZeroNoop(t *testing.T) {
	charTokenizer()
	defer restoreTokenizer()
	body := strings.Join(joinSentences(12), "")
	p := newTitleParam(t, "group", 0, [][]string{{`^# `}})
	inputs := map[string]any{"output_format": "text", "text": body}
	got, err := invokeGroup(testCtx(), nil, inputs, &p)
	if err != nil {
		t.Fatalf("invokeGroup: %v", err)
	}
	chunks := got["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("cap=0 must keep 1 chunk, got %d", len(chunks))
	}
}

func TestTitleCap_GroupPipeline_MultiRecordMerged(t *testing.T) {
	charTokenizer()
	defer restoreTokenizer()
	// 6 records "S00。\n...\nS05。" -> built text is each on its own line.
	records := make([]string, 6)
	for i := 0; i < 6; i++ {
		records[i] = sprintfSentence(i)
	}
	body := strings.Join(records, "\n")
	p := newTitleParam(t, "group", 10, [][]string{{`^# `}})
	inputs := map[string]any{"output_format": "text", "text": body}
	got, err := invokeGroup(testCtx(), nil, inputs, &p)
	if err != nil {
		t.Fatalf("invokeGroup: %v", err)
	}
	chunks := got["chunks"].([]map[string]any)
	if len(chunks) <= 1 {
		t.Fatalf("expected merged split, got %d", len(chunks))
	}
	// The built text joins each record with "\n" and appends a trailing "\n".
	var want strings.Builder
	for _, r := range records {
		want.WriteString(r)
		want.WriteString("\n")
	}
	assertCapInvariants(t, chunks, 10, want.String())
}

func TestTitleCap_HierarchyPipeline_SubChunksKeepPositions(t *testing.T) {
	charTokenizer()
	defer restoreTokenizer()
	body := strings.Join(joinSentences(12), "")
	p := newTitleParam(t, "hierarchy", 20, [][]string{{`^# `}})
	inputs := map[string]any{
		"output_format": "chunks",
		"chunks": []schema.ChunkDoc{
			{Text: body, DocType: "text", Positions: json.RawMessage(`[[1,10,200,50,80]]`)},
		},
	}
	got, err := invokeHierarchy(testCtx(), nil, inputs, &p)
	if err != nil {
		t.Fatalf("invokeHierarchy: %v", err)
	}
	chunks := got["chunks"].([]map[string]any)
	if len(chunks) <= 1 {
		t.Fatalf("expected split, got %d", len(chunks))
	}
	// Every sub-chunk carries a sliced position row: the on-demand crop pass
	// must attach a preview of its own vertical region, not the whole box.
	for i := range chunks {
		v, ok := chunks[i]["positions"]
		if !ok {
			t.Errorf("sub-chunk %d missing positions", i)
			continue
		}
		vm, ok := v.([][]float64)
		if !ok || len(vm) == 0 || vm[0][0] != 1 {
			t.Errorf("sub-chunk %d positions = %#v, want a sliced [[1 ...]] matrix", i, v)
			continue
		}
		if vm[0][4] > 80+1e-9 || vm[0][3] < 50-1e-9 {
			t.Errorf("sub-chunk %d bounds out of source span: %v", i, vm[0])
		}
	}
}

// ---------------------------------------------------------------------------
// Schema validation
// ---------------------------------------------------------------------------

func TestTitleChunkerParam_ChunkTokenCapDefaults(t *testing.T) {
	if got := (schema.TitleChunkerParam{}).Defaults().ChunkTokenCap; got != 512 {
		t.Errorf("default ChunkTokenCap = %d, want 512", got)
	}
}

func TestTitleChunkerParam_ChunkTokenCapValidate(t *testing.T) {
	cases := []struct {
		cap int
		ok  bool
	}{
		{0, true},     // disabled
		{50, false},   // below 128
		{127, false},  // below 128
		{128, true},   // lower bound
		{512, true},   // default
		{8000, true},  // upper bound
		{8001, false}, // above 8000
		{9000, false}, // above 8000
	}
	for _, c := range cases {
		p := schema.TitleChunkerParam{Method: "group", Levels: [][]string{{"^#"}}, ChunkTokenCap: c.cap}
		err := p.Validate()
		if c.ok && err != nil {
			t.Errorf("cap=%d: unexpected error %v", c.cap, err)
		}
		if !c.ok && err == nil {
			t.Errorf("cap=%d: expected error, got nil", c.cap)
		}
	}
}

// TestTitleChunkerParam_ChunkTokenCapValidate_EmptyMethodBypass pins the
// validation-order fix: the cap range check must run even when Method is ""
// (which otherwise early-returns nil), so an out-of-range cap cannot slip
// through as an active ceiling.
func TestTitleChunkerParam_ChunkTokenCapValidate_EmptyMethodBypass(t *testing.T) {
	p := schema.TitleChunkerParam{Method: "", Levels: [][]string{{"^#"}}, ChunkTokenCap: 1}
	if err := p.Validate(); err == nil {
		t.Error(`method="" with cap=1 must be rejected (out of 128..8000)`)
	}
	pOK := schema.TitleChunkerParam{Method: "", Levels: [][]string{{"^#"}}, ChunkTokenCap: 512}
	if err := pOK.Validate(); err != nil {
		t.Errorf(`method="" with cap=512 must pass: %v`, err)
	}
}
