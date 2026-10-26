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

package decisionhead

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"ragflow/internal/tokenizer"
)

// fakeEncoder is a MetaspaceEncoder that maps a rune to its own code point. The
// mapping is deliberately reversible: the assembly tests decode a Sequence back
// into text and assert on the text, instead of re-deriving the tokenizer's
// arithmetic and proving only that the test agrees with itself.
type fakeEncoder struct{}

func (fakeEncoder) IDs(text string) []int32 {
	ids := make([]int32, 0, len(text))
	for _, r := range text {
		ids = append(ids, int32(r))
	}
	return ids
}

func (fakeEncoder) Count(text string) int { return len([]rune(text)) }

var _ tokenizer.MetaspaceEncoder = fakeEncoder{}

func decode(ids []int32) string {
	var b strings.Builder
	for _, id := range ids {
		b.WriteRune(rune(id))
	}
	return b.String()
}

// fakeScorer records every forward pass and answers with logits the test
// controls. It is the seam that keeps the assembly, routing, threshold and
// budget logic testable without the ONNX graph.
type fakeScorer struct {
	mu    sync.Mutex
	seen  []Sequence
	err   error
	logit func(seq Sequence) []float32
}

func (f *fakeScorer) Logits(_ context.Context, seq Sequence) ([]float32, error) {
	f.mu.Lock()
	f.seen = append(f.seen, seq)
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	if f.logit != nil {
		return f.logit(seq), nil
	}
	return make([]float32, len(seq.Markers)), nil
}

func (f *fakeScorer) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.seen)
}

func (f *fakeScorer) last() Sequence {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.seen[len(f.seen)-1]
}

// testContract mirrors the shipped laya_config.json for the fields the
// assembly reads.
func testContract() *Contract {
	return &Contract{
		MaxLen:         512,
		HeadMaxLen:     256,
		OptionTokenCap: 56,
		QTypes:         map[string]int{"noul": 2},
		SpecialIDs:     map[string]int32{"cls": 2, "sep": 1, "mask": 4, "pad": 0},
		MaskToken:      "<mask>",
		InstructionTemplates: map[string]map[string][]string{
			"noul": {
				"en": {"Does the provided evidence sufficiently answer the question: {q}?"},
				"zh": {"给定的证据是否足以回答这个问题：{q}？"},
			},
		},
		LangRule:             LangRule{CJKThreshold: 0.2, Fallback: "en"},
		HeadFormat:           "{qtype} question: {instructions}",
		Temperature:          0.8,
		TemperatureByOptions: map[string]float64{"noul:2": 0.8},
		cjkRanges:            []cjkRange{{lo: 0x3400, hi: 0x9fff}},
	}
}

func testJudge(scorer scorer, cfg Config) *judge {
	if cfg.SegmentTokens == 0 {
		cfg.SegmentTokens = DefaultSegmentTokens
	}
	if cfg.MaxSegments == 0 {
		cfg.MaxSegments = DefaultMaxSegments
	}
	if cfg.MinProbability == 0 {
		cfg.MinProbability = DefaultMinProbability
	}
	if cfg.Budget == 0 {
		cfg.Budget = DefaultBudgetMS * time.Millisecond
	}
	return newJudge(cfg, testContract(), fakeEncoder{}, scorer)
}

func TestSplitShortTextStaysWhole(t *testing.T) {
	segments, overflow := SplitSegments(fakeEncoder{}, "hello world", 512, 12)
	if len(segments) != 1 || segments[0] != "hello world" {
		t.Fatalf("segments = %q, want the whole text as one segment", segments)
	}
	if overflow != "" {
		t.Fatalf("overflow = %q, want empty", overflow)
	}
}

func TestSplitCutsOversizedTextAtTokenBoundary(t *testing.T) {
	text := "abcdefghij" // 10 runes, 5-token budget -> 5 + 5
	segments, overflow := SplitSegments(fakeEncoder{}, text, 5, 12)
	if len(segments) != 2 || segments[0] != "abcde" || segments[1] != "fghij" {
		t.Fatalf("segments = %q, want [abcde fghij]", segments)
	}
	if overflow != "" {
		t.Fatalf("overflow = %q, want empty", overflow)
	}
}

func TestSplitEmptyTextYieldsNothing(t *testing.T) {
	segments, overflow := SplitSegments(fakeEncoder{}, "", 512, 12)
	if len(segments) != 0 || overflow != "" {
		t.Fatalf("segments = %q overflow = %q, want both empty", segments, overflow)
	}
}

// Text past maxSegments is returned as overflow and must be kept UNJUDGED by the
// caller: it is evidence nobody paid to score, not evidence the model rejected.
func TestSplitReturnsOverflowPastSegmentBound(t *testing.T) {
	segments, overflow := SplitSegments(fakeEncoder{}, "abcdefghij", 3, 2)
	if len(segments) != 2 {
		t.Fatalf("segments = %q, want 2", segments)
	}
	if overflow != "ghij" {
		t.Fatalf("overflow = %q, want %q", overflow, "ghij")
	}
}

func TestScrubTextRemovesEveryMarkerSpelling(t *testing.T) {
	got := scrubText("keep <mask> and [MASK] and <mask>", "<mask>")
	if strings.Contains(got, "mask") || strings.Contains(got, "MASK") {
		t.Fatalf("scrubbed text still carries a marker spelling: %q", got)
	}
	if !strings.HasPrefix(got, "keep ") {
		t.Fatalf("scrub removed content it should have kept: %q", got)
	}
}

// The id-level scrub is the authoritative half: it is the only way to prove a
// span carries no marker, and it closes separator injection too.
func TestScrubIDsDropsMarkersAndSeparators(t *testing.T) {
	special := testContract().SpecialIDs
	got := scrubIDs([]int32{10, special["mask"], 11, special["cls"], special["sep"], 12}, special)
	want := []int32{10, 11, 12}
	if len(got) != len(want) {
		t.Fatalf("scrubIDs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("scrubIDs = %v, want %v", got, want)
		}
	}
}

func TestBuildSequenceMarksEveryOption(t *testing.T) {
	c := testContract()
	seq, err := BuildSequence(fakeEncoder{}, c, qtypeNoul, "instructions", noulOptions, "state")
	if err != nil {
		t.Fatalf("BuildSequence: %v", err)
	}
	if len(seq.Markers) != len(noulOptions) {
		t.Fatalf("markers = %v, want %d", seq.Markers, len(noulOptions))
	}
	if seq.QType != int64(c.QTypes[qtypeNoul]) {
		t.Fatalf("qtype = %d, want %d", seq.QType, c.QTypes[qtypeNoul])
	}
	for i, marker := range seq.Markers {
		if seq.IDs[marker] != c.SpecialIDs["mask"] {
			t.Fatalf("marker %d points at id %d, want the mask id", i, seq.IDs[marker])
		}
		if i > 0 && marker <= seq.Markers[i-1] {
			t.Fatalf("markers are not increasing: %v", seq.Markers)
		}
	}
	if seq.IDs[0] != c.SpecialIDs["cls"] {
		t.Fatalf("sequence does not start with cls: %v", seq.IDs[:1])
	}
}

func TestTrueProbabilityDividesByTemperature(t *testing.T) {
	// logits [0 2] / T=0.8 = [0 2.5] -> sigmoid(2.5) = 0.9241.
	got, ok := trueProbability([]float32{0, 2}, 0.8)
	if !ok {
		t.Fatal("trueProbability reported a degraded run")
	}
	if want := 0.9241; got < want-0.001 || got > want+0.001 {
		t.Fatalf("probability = %.4f, want %.4f", got, want)
	}
}

// A model that did not emit one score per option is a degraded run, not a
// negative decision: without this the argmax would silently read the wrong slot.
func TestTrueProbabilityRejectsWrongOptionCount(t *testing.T) {
	if _, ok := trueProbability([]float32{1}, 0.8); ok {
		t.Fatal("trueProbability accepted a single-logit row")
	}
}

func TestDecideRoutesLanguageFromQuestion(t *testing.T) {
	zh := testJudge(&fakeScorer{}, Config{})
	zh.decide(context.Background(), "国家法定节假日共多少天？", []string{"evidence"})
	if head := decode(zh.model.(*fakeScorer).last().IDs); !strings.Contains(head, "给定的证据") {
		t.Fatalf("Chinese question routed to the English template: %q", head)
	}

	en := testJudge(&fakeScorer{}, Config{})
	en.decide(context.Background(), "How many statutory holidays are there?", []string{"evidence"})
	if head := decode(en.model.(*fakeScorer).last().IDs); !strings.Contains(head, "Does the provided evidence") {
		t.Fatalf("English question routed to the Chinese template: %q", head)
	}
}

func TestDecideAppliesCalibratedThreshold(t *testing.T) {
	// logits [0 1] / 0.8 -> sigmoid(1.25) = 0.777, above the 0.6 default.
	useful := testJudge(&fakeScorer{logit: func(Sequence) []float32 { return []float32{0, 1} }}, Config{})
	verdicts := useful.decide(context.Background(), "q", []string{"a"})
	if !verdicts[0].Judged || !verdicts[0].Useful {
		t.Fatalf("verdict = %+v, want judged and useful", verdicts[0])
	}
	if verdicts[0].Probability < 0.77 || verdicts[0].Probability > 0.78 {
		t.Fatalf("probability = %.4f, want ~0.777", verdicts[0].Probability)
	}

	// logits [1 0] / 0.8 -> 0.223, below the threshold.
	useless := testJudge(&fakeScorer{logit: func(Sequence) []float32 { return []float32{1, 0} }}, Config{})
	verdicts = useless.decide(context.Background(), "q", []string{"a"})
	if !verdicts[0].Judged || verdicts[0].Useful {
		t.Fatalf("verdict = %+v, want judged and not useful", verdicts[0])
	}
}

func TestDecideCountsSegmentTokens(t *testing.T) {
	j := testJudge(&fakeScorer{}, Config{})
	verdicts := j.decide(context.Background(), "q", []string{"abcd"})
	if verdicts[0].Tokens != 4 {
		t.Fatalf("tokens = %d, want 4", verdicts[0].Tokens)
	}
}

// When the per-chunk budget is gone the remaining segments are left unjudged —
// kept, never dropped — and the model is not called again.
func TestDecideSpendsBudget(t *testing.T) {
	scorer := &fakeScorer{}
	j := testJudge(scorer, Config{Budget: time.Nanosecond})
	verdicts := j.decide(context.Background(), "q", []string{"a", "b", "c"})
	if scorer.calls() != 0 {
		t.Fatalf("scorer ran %d times under an exhausted budget", scorer.calls())
	}
	for _, v := range verdicts {
		if v.Judged || !v.Useful {
			t.Fatalf("verdict = %+v, want unjudged and useful", v)
		}
	}
}

// A failing model must not lose evidence: every segment stays useful.
func TestDecideDegradesOnScorerError(t *testing.T) {
	scorer := &fakeScorer{err: errors.New("onnx exploded")}
	verdicts := testJudge(scorer, Config{}).decide(context.Background(), "q", []string{"a", "b"})
	for _, v := range verdicts {
		if v.Judged || !v.Useful {
			t.Fatalf("verdict = %+v, want unjudged and useful", v)
		}
	}
}

func TestDecideStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	scorer := &fakeScorer{}
	verdicts := testJudge(scorer, Config{}).decide(ctx, "q", []string{"a"})
	if scorer.calls() != 0 {
		t.Fatalf("scorer ran %d times on a cancelled context", scorer.calls())
	}
	if verdicts[0].Judged || !verdicts[0].Useful {
		t.Fatalf("verdict = %+v, want unjudged and useful", verdicts[0])
	}
}

// One judgement per chunk per round: the same segment must not be paid for
// twice, and the memo must not change the answer.
func TestDecideMemoisesRepeatedSegments(t *testing.T) {
	scorer := &fakeScorer{logit: func(Sequence) []float32 { return []float32{0, 1} }}
	j := testJudge(scorer, Config{})
	first := j.decide(context.Background(), "q", []string{"a", "b"})
	second := j.decide(context.Background(), "q", []string{"a", "b"})
	if scorer.calls() != 2 {
		t.Fatalf("scorer ran %d times, want 2 (one per distinct segment)", scorer.calls())
	}
	for i := range first {
		if first[i].Probability != second[i].Probability ||
			first[i].Useful != second[i].Useful || first[i].Judged != second[i].Judged {
			t.Fatalf("memo changed the verdict: %+v vs %+v", first[i], second[i])
		}
	}
	// A memo hit is served without a forward pass, so it carries no latency.
	if second[0].Latency != 0 {
		t.Fatalf("memo hit reported latency %s, want 0", second[0].Latency)
	}
}

// The template rotates by decision index, so a memo key that ignored the index
// would serve a verdict computed with a different template.
func TestDecideKeepsTemplateRotationDeterministic(t *testing.T) {
	j := testJudge(&fakeScorer{}, Config{})
	j.decide(context.Background(), "q", []string{"a"})
	head := decode(j.model.(*fakeScorer).last().IDs)
	if !strings.Contains(head, "Does the provided evidence") {
		t.Fatalf("head does not carry the rendered template: %q", head)
	}
	if strings.Contains(head, "{q}") {
		t.Fatalf("head carries an unrendered placeholder: %q", head)
	}
}

// Asking whether the evidence is RELEVANT is a different question from asking
// whether it answers, and the head must carry the one that was configured: a
// relevance run that silently asks the sufficiency phrasing would measure
// nothing.
func TestDecideAsksRelevanceWhenConfigured(t *testing.T) {
	j := testJudge(&fakeScorer{}, Config{QuestionStyle: StyleRelevance})
	j.decide(context.Background(), "which teams won", []string{"a"})
	head := decode(j.model.(*fakeScorer).last().IDs)
	if !strings.Contains(head, "relevant to the question") {
		t.Fatalf("head does not carry the relevance phrasing: %q", head)
	}
	if strings.Contains(head, "sufficiently answer") {
		t.Fatalf("head still asks the sufficiency question: %q", head)
	}
	if strings.Contains(head, "{q}") {
		t.Fatalf("head carries an unrendered placeholder: %q", head)
	}
}

// The two questions must never share a cached verdict: the same (question,
// segment) has a different answer under each.
func TestMemoKeySeparatesQuestionStyles(t *testing.T) {
	if memoKey(StyleSufficiency, "q", 0, "s") == memoKey(StyleRelevance, "q", 0, "s") {
		t.Fatal("the two question styles share a memo key")
	}
}

// With no deployment in the environment the feature is off and every entry
// point is a pass-through. This is the regression that keeps the head from changing
// behaviour on a host that never configured it.
func TestWithoutDeploymentEverythingIsKept(t *testing.T) {
	segments, overflow := Split("some chunk text")
	if len(segments) != 1 || segments[0] != "some chunk text" || overflow != "" {
		t.Fatalf("Split = %q / %q, want the whole text", segments, overflow)
	}
	verdicts := Judge(context.Background(), "q", segments)
	if len(verdicts) != 1 {
		t.Fatalf("Judge returned %d verdicts, want 1", len(verdicts))
	}
	if verdicts[0].Judged || !verdicts[0].Useful {
		t.Fatalf("verdict = %+v, want unjudged and useful", verdicts[0])
	}
}

func TestParseCodePointAcceptsEverySpelling(t *testing.T) {
	for _, spec := range []string{`\u4e00`, `\U4E00`, "u+4e00", "U+4E00", "0x4e00", "4e00", " 4E00 "} {
		got, err := parseCodePoint(spec)
		if err != nil {
			t.Fatalf("parseCodePoint(%q): %v", spec, err)
		}
		if got != 0x4e00 {
			t.Fatalf("parseCodePoint(%q) = %#x, want 0x4e00", spec, got)
		}
	}
	if _, err := parseCodePoint("not-hex"); err == nil {
		t.Fatal("parseCodePoint accepted a non-hex bound")
	}
}

// The shipped contract writes cjk_ranges exactly as its own regex does — with a
// literal backslash-u prefix — so a parser that reads bare hex fails on the only
// contract that exists, and the whole feature never loads. This is the
// regression: with the ranges unparsed, every question would route to the
// fallback language and the calibration would drift silently.
func TestLoadContractReadsDeployCJKRanges(t *testing.T) {
	raw := `{
  "max_len": 1024,
  "head_max_len": 256,
  "option_token_cap": 56,
  "special_ids": {"cls": 2, "sep": 1, "mask": 4, "pad": 0},
  "mask_token": "<mask>",
  "head_format": "{qtype} question: {instructions}",
  "instruction_lang_rule": {
    "cjk_threshold": 0.2,
    "fallback": "en",
    "cjk_ranges": ["\\u3400-\\u4dbf", "\\u4e00-\\u9fff", "\\uf900-\\ufaff"]
  }
}`
	path := filepath.Join(t.TempDir(), "laya_config.json")
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write contract: %v", err)
	}
	contract, err := LoadContract(path)
	if err != nil {
		t.Fatalf("LoadContract: %v", err)
	}
	if got := contract.CJKRatio("国家法定节假日共多少天？"); got < 0.9 {
		t.Fatalf("CJKRatio = %.3f, want the Chinese question to count as CJK", got)
	}
	if got := contract.RouteLang("国家法定节假日共多少天？"); got != "zh" {
		t.Fatalf("RouteLang(zh question) = %q, want zh", got)
	}
	if got := contract.RouteLang("How many statutory holidays are there?"); got != "en" {
		t.Fatalf("RouteLang(en question) = %q, want en", got)
	}
}

// The checkpoint is a per-deployment artifact: a sidecar deployment needs the
// contract and the tokenizer, never the .ort.
func TestAvailableWithoutCheckpointWhenSidecarIsConfigured(t *testing.T) {
	dir := t.TempDir()
	for _, rel := range []string{
		filepath.Join("onnx", "laya_config.json"),
		filepath.Join("onnx", "tokenizer", "tokenizer.json"),
	} {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	if (Config{Dir: dir}).Available() {
		t.Fatal("a deployment without the checkpoint reported available")
	}
	if !(Config{Dir: dir, URL: "http://127.0.0.1:8390"}).Available() {
		t.Fatal("a sidecar deployment was required to ship the checkpoint")
	}
	if (Config{Dir: dir + "-missing", URL: "http://127.0.0.1:8390"}).Available() {
		t.Fatal("a deployment with no model directory reported available")
	}
}

func TestConfigFromEnvDefaultsAndDisable(t *testing.T) {
	t.Setenv("RAGFLOW_DECISION_DIR", "")
	cfg := ConfigFromEnv()
	if cfg.Enabled() || cfg.Available() {
		t.Fatalf("cfg = %+v, want disabled", cfg)
	}
	if cfg.SegmentTokens != DefaultSegmentTokens || cfg.MaxSegments != DefaultMaxSegments {
		t.Fatalf("cfg = %+v, want the shipped defaults", cfg)
	}
	if cfg.MinProbability != DefaultMinProbability {
		t.Fatalf("min probability = %v, want %v", cfg.MinProbability, DefaultMinProbability)
	}
}
