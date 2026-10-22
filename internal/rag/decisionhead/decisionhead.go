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

// decisionhead.go is the package's entry point: it owns the one loaded checkpoint and
// answers the only question RAGFlow asks of it — "is this chunk sufficient to
// answer the question?" — as a calibrated probability per segment.
//
// The API is three package-level functions because the checkpoint is a process
// singleton: the fp32 graph is ~1.7 GB resident and the measured peak RSS of one
// forward pass is over 2 GB, whether it sits in this process or in the sidecar,
// so a second copy is not something a caller should be able to create by
// accident. Split and Judge therefore share the one loaded judge, and both
// degrade to "keep everything" when it is not there.
//
// Degradation is the whole safety story: RAGFlow must never drop evidence
// because the model is missing, slow or broken. An unjudged segment is always
// Useful; only a completed decision can drop one.

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"ragflow/internal/tokenizer"
)

// qtypeNoul is the sufficiency decision. It is the only question type this
// package issues, and the only one the shipped checkpoint was trained on
// (DEPLOY_CONTRACT.md §三).
const qtypeNoul = "noul"

// Verdict is the outcome for one segment of one chunk. Judged distinguishes a
// segment the model actually ruled on from one the caller must keep untouchied:
// a spent budget, a cancelled context or a failed run leaves Judged false and
// Useful true, so a caller that keeps `Useful` segments keeps its evidence.
type Verdict struct {
	// Segment is the exact text that was judged (and, when Useful, the text the
	// caller should keep).
	Segment string
	// Tokens is the segment's length in the model's tokenizer, 0 when unknown.
	Tokens int
	// Probability is the calibrated P(sufficient) of the "true" option. It is
	// only meaningful when Judged.
	Probability float64
	// Useful is whether the segment should be kept. It is true for every
	// unjudged segment.
	Useful bool
	// Judged is whether the model produced this verdict.
	Judged bool
	// Latency is the forward pass time, 0 when served without one.
	Latency time.Duration
}

// scorer is the inference half. It is an interface so the assembly (sequence,
// language routing, temperature) can be tested without the ONNX graph; the
// production implementation is *Session.
type scorer interface {
	Logits(ctx context.Context, seq Sequence) ([]float32, error)
}

// judge is the loaded checkpoint and everything needed to feed it.
type judge struct {
	cfg      Config
	contract *Contract
	enc      tokenizer.MetaspaceEncoder
	model    scorer
	memo     memo
}

func newJudge(cfg Config, contract *Contract, enc tokenizer.MetaspaceEncoder, model scorer) *judge {
	return &judge{cfg: cfg, contract: contract, enc: enc, model: model}
}

// open loads the checkpoint described by cfg. cfg.Available must already be
// true; the caller decides what a missing deployment means.
func open(cfg Config) (*judge, error) {
	modelPath, contractPath, tokenizerPath := cfg.Paths()
	contract, err := LoadContract(contractPath)
	if err != nil {
		return nil, err
	}
	enc, err := tokenizer.LoadMetaspaceBPE(tokenizerPath)
	if err != nil {
		return nil, fmt.Errorf("decisionhead: loading tokenizer %s: %w", tokenizerPath, err)
	}
	model, err := openScorer(cfg, modelPath)
	if err != nil {
		return nil, err
	}
	return newJudge(cfg, contract, enc, model), nil
}

// openScorer decides where the graph runs. A configured sidecar wins, because it
// is the only configuration that works with the ONNX Runtime linked into this
// binary; the in-process path stays for the day that library covers the graph.
//
// The sidecar is health-checked here on purpose: pointing at a dead or
// model-less sidecar should be one line in the startup log, not a silent
// degradation on every chunk.
func openScorer(cfg Config, modelPath string) (scorer, error) {
	if cfg.URL == "" {
		return OpenSession(modelPath)
	}
	remote := newRemoteScorer(cfg.URL)
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	if err := probeSidecar(ctx, remote.client, cfg.URL); err != nil {
		return nil, fmt.Errorf("decisionhead: sidecar %s is not ready: %w", cfg.URL, err)
	}
	return remote, nil
}

// Close releases the checkpoint's ONNX session. It is what tests and a
// deliberate teardown use; a server holds the shared judge for its lifetime.
func (j *judge) Close() {
	if session, ok := j.model.(*Session); ok {
		session.Close()
	}
}

var (
	sharedOnce sync.Once
	shared     *judge
)

// shared returns the process-wide judge, loading it on the first call. A
// deployment that is not configured, or whose files do not load, logs once and
// leaves shared nil forever: retrying a broken model directory on every request
// would turn a static misconfiguration into a per-request latency tax.
func sharedJudge() *judge {
	sharedOnce.Do(func() {
		cfg := ConfigFromEnv()
		if !cfg.Available() {
			return
		}
		j, err := open(cfg)
		if err != nil {
			log.Printf("decisionhead: disabled, %v", err)
			return
		}
		log.Printf("decisionhead: loaded %s", cfg)
		shared = j
	})
	return shared
}

// Available reports whether RAGFLOW_DECISION_DIR names a usable model directory. It
// is the caller's cheap gate: false means Split and Judge are both pass-through
// and nothing else about them needs to be called.
func Available() bool { return ConfigFromEnv().Available() }

// Split cuts text into segments the model can judge one at a time, returning the
// tail past MaxSegments as overflow. The caller keeps overflow UNJUDGED: text
// beyond the segment bound is evidence nobody paid to score, which is not the
// same as evidence the model rejected.
//
// When the head is not available the whole text is one segment with no overflow, so
// a caller that feeds Split's output straight to Judge keeps every byte.
func Split(text string) (segments []string, overflow string) {
	j := sharedJudge()
	if j == nil {
		if text == "" {
			return nil, ""
		}
		return []string{text}, ""
	}
	return SplitSegments(j.enc, text, j.cfg.SegmentTokens, j.cfg.MaxSegments)
}

// SplitWhole hands the passage to the model as ONE segment, truncated to
// maxTokens, and returns no overflow.
//
// It is the alternative window policy for the sufficiency filter. Split cuts a
// passage into judged sub-segments so that a half-relevant chunk reaches the
// model as its relevant half; SplitWhole instead asks about the passage once, in
// full, which is what a model with a large context wants — at the cost that a
// passage taller than maxTokens is judged on its opening alone.
//
// The truncated tail is deliberately NOT returned as overflow. The policy being
// tested is truncation, and a caller that kept the tail unjudged would reinstate
// exactly the behaviour this mode exists to change.
func SplitWhole(text string, maxTokens int) (segments []string, overflow string) {
	if text == "" {
		return nil, ""
	}
	j := sharedJudge()
	if j == nil || maxTokens <= 0 {
		return []string{text}, ""
	}
	return []string{TruncateToTokens(j.enc, text, maxTokens)}, ""
}

// CountTokens reports what text costs under the model's encoder, or 0 when no
// deployment is configured.
//
// It exists for the passages the filter exempted: those are never judged, so
// their size cannot be read off a verdict, and a caller accounting for how much
// text a run served would otherwise silently omit exactly the passages that were
// kept for free.
func CountTokens(text string) int {
	if text == "" {
		return 0
	}
	j := sharedJudge()
	if j == nil {
		return 0
	}
	return j.enc.Count(text)
}

// Judge scores every segment for the noul decision, in order. It never fails:
// an unavailable model, a spent budget or a cancelled context leave the
// remaining segments unjudged, and unjudged always means Useful.
func Judge(ctx context.Context, question string, segments []string) []Verdict {
	if j := sharedJudge(); j != nil {
		return j.decide(ctx, question, segments)
	}
	return unjudged(segments)
}

// unjudged is the keep-everything verdict list, and the shape every degraded
// path returns.
func unjudged(segments []string) []Verdict {
	out := make([]Verdict, len(segments))
	for i, segment := range segments {
		out[i] = Verdict{Segment: segment, Useful: true}
	}
	return out
}

// decide judges one chunk's segments under the per-chunk time budget. The
// budget is what bounds the cost of a chunk that split into many segments: when
// it runs out the rest are left unjudged, which keeps them, rather than
// dropping evidence the model never saw.
//
// The language is routed from the QUESTION, once per chunk, not per segment and
// not from the state: the same evidence with a Chinese and an English question
// lands in two different distributions and the calibration drifts
// (DEPLOY_CONTRACT.md §3.4).
func (j *judge) decide(ctx context.Context, question string, segments []string) []Verdict {
	out := unjudged(segments)
	lang := j.contract.RouteLang(question)
	deadline := time.Now().Add(j.cfg.Budget)
	for i := range out {
		if ctx.Err() != nil || !time.Now().Before(deadline) {
			break
		}
		j.score(ctx, question, lang, i, &out[i])
	}
	return out
}

// score judges one segment in place. Any failure leaves the verdict unjudged:
// the caller keeps the segment and the reason is visible from Judged.
func (j *judge) score(ctx context.Context, question, lang string, index int, v *Verdict) {
	v.Tokens = j.enc.Count(v.Segment)
	key := memoKey(j.cfg.QuestionStyle, question, index, v.Segment)
	if cached, ok := j.memo.get(key); ok {
		v.Probability, v.Useful, v.Judged = cached.probability, cached.useful, cached.judged
		return
	}
	var (
		instructions string
		err          error
	)
	if j.cfg.AsksRelevance() {
		instructions, err = j.contract.RelevanceTemplate(lang, question, index)
	} else {
		instructions, err = j.contract.Template(qtypeNoul, lang, question, index)
	}
	if err != nil {
		return
	}
	seq, err := BuildSequence(j.enc, j.contract, qtypeNoul, instructions, noulOptions, v.Segment)
	if err != nil {
		return
	}
	start := time.Now()
	logits, err := j.model.Logits(ctx, seq)
	v.Latency = time.Since(start)
	if err != nil {
		return
	}
	probability, ok := trueProbability(logits, j.contract.TemperatureFor(qtypeNoul, len(noulOptions)))
	if !ok {
		return
	}
	v.Probability = probability
	v.Useful = probability >= j.cfg.MinProbability
	v.Judged = true
	j.memo.put(key, *v)
}

// trueProbability turns the model's logits into the calibrated P(sufficient):
// the logits are divided by the contract's temperature for this option count
// first, which is the entire point of the calibration, and then softmaxed. The
// "true" option is index 1 of noulOptions.
//
// ok is false when the model did not emit one score per option — e.g. every
// marker fell past max_len and was dropped. That is a degraded run, not a
// negative decision.
func trueProbability(logits []float32, temperature float64) (float64, bool) {
	if len(logits) != len(noulOptions) {
		return 0, false
	}
	scaled := make([]float64, len(logits))
	for i, logit := range logits {
		scaled[i] = float64(logit)
		if temperature > 0 {
			scaled[i] /= temperature
		}
	}
	return softmax(scaled)[1], true
}

// softmax is the numerically stable softmax over a non-empty slice.
func softmax(values []float64) []float64 {
	max := values[0]
	for _, v := range values[1:] {
		if v > max {
			max = v
		}
	}
	out := make([]float64, len(values))
	total := 0.0
	for i, v := range values {
		out[i] = math.Exp(v - max)
		total += out[i]
	}
	if total > 0 {
		for i := range out {
			out[i] /= total
		}
	}
	return out
}

// memoCap bounds the decision memo's entries. Chunk text is held until eviction,
// so the cap is what keeps it from growing with the corpus.
const memoCap = 4096

// memo caches a decision per (question, template index, segment). The key
// carries all three inputs the decision depends on, so a hit can only return
// what a fresh run would have returned; it is what makes "one judgement per
// chunk per round" hold when the same chunk is retrieved twice, without making
// the result depend on how many times it was seen.
type memo struct {
	mu      sync.Mutex
	entries map[string]memoEntry
}

type memoEntry struct {
	probability float64
	useful      bool
	judged      bool
}

// memoKey identifies one decision. The question STYLE is part of the key: the
// same (question, segment) asked as "does it answer this" and as "is it
// relevant" are two different decisions and must not share a cached verdict.
func memoKey(style, question string, index int, segment string) string {
	return fmt.Sprintf("%s\x1f%d\x1f%s\x1f%s", style, index, question, segment)
}

func (m *memo) get(key string) (memoEntry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.entries[key]
	return entry, ok
}

func (m *memo) put(key string, v Verdict) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entries == nil {
		m.entries = map[string]memoEntry{}
	}
	if len(m.entries) >= memoCap {
		// Dropping the map wholesale is the eviction policy: it needs no
		// per-entry bookkeeping, and a miss only costs one forward pass.
		m.entries = map[string]memoEntry{}
	}
	m.entries[key] = memoEntry{probability: v.Probability, useful: v.Useful, judged: v.Judged}
}
