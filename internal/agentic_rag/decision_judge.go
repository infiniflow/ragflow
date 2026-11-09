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

package agentic_rag

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"ragflow/internal/agentic_rag/decisionhead"
	"ragflow/internal/common"
	"ragflow/internal/entity/models"
)

// The decision-model backend answers the filter's per-passage question over
// HTTP instead of with the local decision head. The two are the same model family
// — same question types, same noul/choice/score vocabulary, same instructions
// and criteria fields — and the head is a distillation of it, so the swap is a
// change of judge, not of question.
//
// What it buys, measured on the same 68 labelled passages
// (plans/20261016-decision-model-judge-task.md §9):
//
//	judge                     precision  recall
//	local head (threshold 0.9)     0.62     0.36
//	MiniMax-M2                    1.00     0.17   (says NO to nearly everything)
//	MiniMax-M3                    0.75     0.43
//	decision model, relevance     0.80     0.71   <- this backend
//
// and, decisively, the head does not react to a passage that states what is
// missing (0.978 with the sentence, 0.979 without) while this one does
// (0.71 -> 0.01), which is the difference between "judged the form" and
// "judged the content".

// decisionRelevanceTemplates are the four phrasings asked per passage, mean
// averaged. They are the decision contract's own relevance wording
// (internal/agentic_rag/decisionhead/contract.go relevanceTemplates["en"]) and are verbatim on
// purpose: the calibration that produced the 0.80/0.71 operating point used
// these exact sentences, and the head can be asked the same ones.
//
// The reading is RELEVANCE, not sufficiency. Sufficiency asks whether the
// passage alone settles the question, which no answer-blind judge can decide —
// a multi-hop passage can be a necessary link without answering anything —
// and it is where the labelled evidence was being missed. Relevance is the
// question a retrieval filter actually needs answered.
var decisionRelevanceTemplates = []string{
	"Is the provided evidence relevant to the question: %s?",
	"Does the evidence have anything to do with the question: %s?",
	"Is the evidence on-topic for the question: %s?",
	"Is the evidence related to the question: %s?",
}

// decisionStepTemplates ask the same question against the CALL's own
// information need, with the overall question kept in view.
//
// Relevance to the final question is the wrong test for one step of a
// multi-hop run: a passage can be a necessary link — on Sarah Polk's mother,
// useless for "what is my future wife's name" — and a judge asked only the
// final question drops it. That is the measured failure of the previous arm: it
// lost 30% of the labelled evidence with no change in accuracy (§13), i.e. it
// was discarding links it could not recognise.
//
// Both are in the text on purpose. The intent alone would let a narrow or
// mistaken intent discard evidence the run needed; the overall question alone
// is the reading that failed. The intent says what this step wants, the
// question says what the run is for, and the templates ask whether the evidence
// serves the step.
//
// The first placeholder is the intent, the second the overall question.
var decisionStepTemplates = []string{
	"The current step needs evidence for: %s\nOverall question: %s\n\nIs the provided evidence relevant to this step?",
	"What this step is looking for: %s\nIt serves the question: %s\n\nDoes the evidence relate to what this step is looking for?",
	"Step goal: %s\nFinal question: %s\n\nIs the evidence on-topic for the step goal?",
	"Information need of this call: %s\nOverall question being answered: %s\n\nIs the evidence relevant to that information need?",
}

const (
	defaultDecisionURL   = "https://maas.qianwenaiapi.com/compatible-mode/v1/systemone"
	defaultDecisionModel = "decision-model-preview"
	// defaultDecisionPhrasings is one, not all four.
	//
	// Each extra phrasing costs one more question in the batch request, so four
	// phrasings cap a batch at 16/4 = 4 passages while one caps it at 16. The
	// per-phrasing spread measured on labelled pairs is ~0.27, which is why the
	// threshold was re-calibrated from 0.15 (four-phrasing mean) to 0.30 (single
	// phrasing) rather than reused.
	defaultDecisionPhrasings = 1

	defaultDecisionMinProb   = 0.30
	defaultDecisionMaxChars  = 6000
	defaultDecisionTimeoutMS = 8000
)

// decisionBackend is the client. One per process: the filter asks about one
// passage at a time and the endpoint is stateless.
type decisionBackend struct {
	url      string
	key      string
	model    string
	minProb  float64
	maxChars int
	driver   *models.SystemOneModel
	// breaker switches the gate off when the endpoint stops answering: past a
	// few consecutive failures the verdicts are all "keep" anyway, so the only
	// thing still being paid for is the timeout, per passage.
	breaker *decisionBreaker
}

var (
	decisionOnce    sync.Once
	decisionCurrent *decisionBackend
)

// decisionBackendFromEnv returns the configured backend, or nil when the
// backend is not selected. Resolution is lazy and memoised because the env is
// loaded after package init, and because the answer must not change mid-run.
func decisionBackendFromEnv() *decisionBackend {
	decisionOnce.Do(func() {
		if !strings.EqualFold(strings.TrimSpace(common.GetEnv(common.EnvDecisionBackend)), "decision-api") {
			return
		}
		key := strings.TrimSpace(common.GetEnv(common.EnvDecisionKey))
		if key == "" {
			// Without a key every call would 401, so the backend declines to
			// exist and the filter degrades to pass-through rather than
			// spending a request per passage to learn that.
			common.WarnCtx(context.Background(), "agentic_rag: decision backend selected without a key, filter disabled")
			return
		}
		url := envStringDefault(common.EnvDecisionURL, defaultDecisionURL)
		model := envStringDefault(common.EnvDecisionModel, defaultDecisionModel)
		b := newDecisionBackend(url, key, model)
		common.InfoCtx(context.Background(), "agentic_rag: decision backend ready",
			zap.String("endpoint", url),
			zap.String("model", b.model),
			zap.Float64("min_prob", b.minProb),
			zap.Int("max_chars", b.maxChars),
			zap.Int("templates", len(decisionRelevanceTemplates)),
			zap.Int("batch", b.batchSize("")),
			zap.Int("phrasings", len(phrasingsFor(""))),
			zap.Int("breaker_failures", b.breaker.threshold),
			zap.Duration("breaker_cooldown", b.breaker.cooldown))
		decisionCurrent = b
	})
	return decisionCurrent
}

// newDecisionBackend builds a backend for one explicit endpoint.
//
// The tuning knobs — threshold, char cap, timeout, breaker — stay env-derived on
// purpose: they describe HOW to talk to a judge, not WHICH judge to talk to, so
// a model configured in the UI behaves exactly like the env-configured one.
// Each backend also gets its own breaker, which is what keeps one broken
// instance from silencing the gate for every other tenant.
func newDecisionBackend(url, key, model string) *decisionBackend {
	return &decisionBackend{
		url:      url,
		key:      key,
		model:    model,
		minProb:  envFloatDefault(common.EnvDecisionMinProb, defaultDecisionMinProb),
		maxChars: envIntDefault(common.EnvDecisionMaxChars, defaultDecisionMaxChars),
		// This configuration supplies the complete endpoint, including any
		// custom path, rather than the provider's base URL.
		driver:  models.NewSystemOneModel(map[string]string{"default": url}, models.URLSuffix{Decision: "/"}),
		breaker: newDecisionBreaker(),
	}
}

// available reports whether the filter can judge at all.
func (b *decisionBackend) available() bool { return b != nil }

// split returns the whole passage as one segment. The relevance calibration
// judged whole passages, so sub-segmenting would ask a different question than
// the one the threshold was fitted to.
func (b *decisionBackend) split(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	// The cap applies here too: this segment is what gets sent, and a passage
	// taller than the cap must not reach the endpoint whole just because it
	// arrived through the sub-segment path.
	if len([]rune(text)) > b.maxChars {
		return []string{string([]rune(text)[:b.maxChars])}
	}
	return []string{text}
}

// splitWhole mirrors decisionhead.SplitWhole's contract — one segment, the text the
// model was asked about — but truncates by CHARACTERS, because there is no
// local tokenizer on this path. The cap is the one the calibration used
// (6000 characters), so a passage is judged on the same span here as it was
// there.
func (b *decisionBackend) splitWhole(text string, _ int) ([]string, string) {
	if strings.TrimSpace(text) == "" {
		return nil, ""
	}
	if len(text) <= b.maxChars {
		return []string{text}, ""
	}
	// Cut on a rune boundary so a multi-byte character is never halved.
	r := []rune(text)
	if len(r) > b.maxChars {
		return []string{string(r[:b.maxChars])}, text
	}
	return []string{text}, ""
}

// judge asks the endpoint about each segment and returns one verdict per
// segment. Every failure — a refusal, a timeout, a malformed body — leaves that
// segment Judged=false and Useful=true, the same contract the local head keeps:
// a segment the model never ruled on must be kept, or a filter outage silently
// deletes evidence.
func (b *decisionBackend) judge(ctx context.Context, question string, segments []string) []decisionhead.Verdict {
	out := make([]decisionhead.Verdict, len(segments))
	for i, segment := range segments {
		out[i] = decisionhead.Verdict{Segment: segment, Useful: true}
	}
	if b == nil || strings.TrimSpace(question) == "" {
		return out
	}
	intent := intentFrom(ctx)
	// Passages are judged in batches: the endpoint takes several questions in
	// ONE request, so four passages cost the same round trip as one. That is the
	// batching the endpoint offers; it is not client-side concurrency, and it
	// leaves the verdicts exactly as the per-passage form produced them.
	size := b.batchSize(intent)
	for start := 0; start < len(out); start += size {
		if ctx.Err() != nil {
			break
		}
		end := start + size
		if end > len(out) {
			end = len(out)
		}
		if !b.breaker.allow() {
			// The breaker is open: the gate is off, and these passages pass
			// through exactly as a failed judgement would have left them — minus
			// the timeout that made the failure expensive. The open/close
			// transitions are logged once each instead of per passage.
			continue
		}
		texts := make([]string, 0, end-start)
		for i := start; i < end; i++ {
			text := out[i].Segment
			if len([]rune(text)) > b.maxChars {
				text = string([]rune(text)[:b.maxChars])
			}
			texts = append(texts, text)
		}
		sent := time.Now()
		res, err := b.askBatch(ctx, question, intent, texts)
		elapsed := time.Since(sent)
		if err != nil {
			b.breaker.failure(ctx, err)
			common.InfoCtx(ctx, "agentic_rag: decision judge failed, passages kept",
				zap.Int("passages", len(texts)),
				zap.String("error", err.Error()))
			continue
		}
		b.breaker.success(ctx)
		// One request answered all of them, so the request's latency and input
		// tokens are shared: dividing keeps "sum of latency" equal to the time
		// actually spent, which is what makes wall accounting readable.
		share := elapsed / time.Duration(len(texts))
		tokens := res.tokens / len(texts)
		for k := range texts {
			i := start + k
			out[i].Latency = share
			out[i].Tokens = tokens
			if out[i].Tokens == 0 {
				out[i].Tokens = len([]rune(texts[k])) / 4
			}
			if !res.judged[k] {
				// Nothing came back for this one passage: the documented
				// degraded path is to keep it.
				continue
			}
			out[i].Judged = true
			out[i].Probability = res.probs[k]
			out[i].Useful = res.probs[k] >= b.minProb
		}
	}
	return out
}

// batchSize is how many passages go into one request.
//
// The endpoint caps a request at decisionMaxQuestions questions, and each
// passage spends one question per phrasing, so the passage count is derived
// from that cap rather than configured independently: asking for more passages
// than fit would fail the whole batch, not just the overflow.
func (b *decisionBackend) batchSize(intent string) int {
	per := len(phrasingsFor(intent))
	if per < 1 {
		per = 1
	}
	limit := decisionMaxQuestions / per
	if limit < 1 {
		limit = 1
	}
	n := envIntDefault(common.EnvDecisionBatch, limit)
	if n > limit {
		n = limit
	}
	if n < 1 {
		n = 1
	}
	return n
}

// ask sends one passage and returns the mean of the templates' probabilities.
//
// All four phrasings go in ONE request: the endpoint takes one state and a map
// of questions about it, so asking them together costs one round trip instead
// of four. The mean, not the max, is what the threshold was fitted to — a
// passage the phrasings disagree about is one the run should keep, and averaging
// is what makes that the outcome.
// decisionMaxQuestions is the endpoint's hard cap on questions per request;
// going over fails the WHOLE request, so the passage count per request is
// derived from it rather than configured independently.
//
// Verified against the live endpoint, not read from a spec: 16 questions → 200,
// 17 → 400 `ValidationError: questions: 17 exceeds the limit of 16`. The cap is
// on questions, not passages, which is why asking several phrasings per passage
// divides the batch (4 phrasings → 4 passages).
const decisionMaxQuestions = models.MaxDecisionQuestions

// batchResult is one request's outcome, indexed like the passages it carried.
// A passage may come back without a noul value while others in the same request
// answered; that one is kept, like every other degraded path.
type batchResult struct {
	probs  []float64
	judged []bool
	tokens int
}

func decisionTemplatesFor(intent string) []string {
	if strings.TrimSpace(intent) != "" {
		return decisionStepTemplates
	}
	return decisionRelevanceTemplates
}

// phrasingsFor returns the templates to ask per passage. Asking the same
// relevance question in several wordings and averaging is a variance reduction:
// measured on real passages the four answers differ by a median range of 0.27,
// so a single wording is one noisy sample of the same quantity. Fewer phrasings
// buy a bigger batch (the cap is questions per request) at the cost of putting
// that noise straight into the decision — and invalidating the calibrated 0.15.
func phrasingsFor(intent string) []string {
	all := decisionTemplatesFor(intent)
	n := envIntDefault(common.EnvDecisionPhrasings, defaultDecisionPhrasings)
	if n > len(all) {
		n = len(all)
	}
	if n < 1 {
		n = 1
	}
	return all[:n]
}

// askBatch judges several passages in ONE request.
//
// The endpoint takes many questions about one state, so a batch carries the
// evidence inside each question and leaves the state empty: one state can hold
// one passage, and the point of batching is to stop paying a round trip per
// passage. The state is empty rather than repurposed as the question — putting
// the question there was measured to cost separation.
func (b *decisionBackend) askBatch(ctx context.Context, question, intent string, passages []string) (batchResult, error) {
	res := batchResult{probs: make([]float64, len(passages)), judged: make([]bool, len(passages))}
	if len(passages) == 0 {
		return res, nil
	}
	templates := phrasingsFor(intent)
	step := strings.TrimSpace(intent) != ""
	questions := make(map[string]models.Question, len(passages)*len(templates))
	for i, passage := range passages {
		for j, tpl := range templates {
			instr := fmt.Sprintf(tpl, question)
			if step {
				instr = fmt.Sprintf(tpl, intent, question)
			}
			questions[fmt.Sprintf("p%d_t%d", i, j)] = models.Noul{
				Instructions: instr + "\n\nEvidence:\n" + passage,
			}
		}
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(envIntDefault(common.EnvDecisionTimeoutMS, defaultDecisionTimeoutMS))*time.Millisecond)
	defer cancel()
	parsed, err := b.driver.Decide(ctx, &b.model, &models.DecideRequest{
		State: map[string]string{"passage": ""}, Questions: questions,
	}, &models.APIConfig{ApiKey: &b.key}, nil)
	if err != nil {
		return res, err
	}
	sums := make([]float64, len(passages))
	counts := make([]int, len(passages))
	for key, answer := range parsed.Answers {
		if answer.Noul == nil {
			continue
		}
		var i, j int
		if _, err := fmt.Sscanf(key, "p%d_t%d", &i, &j); err != nil {
			continue
		}
		if i < 0 || i >= len(passages) {
			continue
		}
		sums[i] += *answer.Noul
		counts[i]++
	}
	answered := 0
	for i := range passages {
		if counts[i] == 0 {
			continue
		}
		res.probs[i] = sums[i] / float64(counts[i])
		res.judged[i] = true
		answered++
	}
	if answered == 0 {
		// Every passage came back without a noul value: a body we cannot read is
		// a failure, not a verdict of zero.
		return res, fmt.Errorf("no noul answers in response")
	}
	res.tokens = parsed.Usage.InputTokens
	return res, nil
}

func (b *decisionBackend) ask(ctx context.Context, question, intent, passage string) (float64, int, error) {
	res, err := b.askBatch(ctx, question, intent, []string{passage})
	if err != nil {
		return 0, 0, err
	}
	if !res.judged[0] {
		return 0, 0, fmt.Errorf("no noul answer for the passage")
	}
	return res.probs[0], res.tokens, nil
}

// firstRunes is the error-message slice: enough of a body to see what came
// back, never enough to flood a log line.
func firstRunes(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "..."
}

// envStringDefault reads a string env var.
func envStringDefault(name, def string) string {
	if v := strings.TrimSpace(common.GetEnv(name)); v != "" {
		return v
	}
	return def
}

// envFloatDefault reads a float env var, returning def when unset or
// unparseable. Unlike the head's envFloat this accepts 0 and 1: a threshold of 0
// keeps everything and is a legitimate way to measure the request path without
// letting it change the result.
func envFloatDefault(name string, def float64) float64 {
	raw := strings.TrimSpace(common.GetEnv(name))
	if raw == "" {
		return def
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return def
	}
	return v
}
