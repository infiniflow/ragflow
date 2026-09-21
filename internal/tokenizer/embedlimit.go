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

package tokenizer

// Embedding input limits: counters, the margin, and calibration.
//
// The invariant this file exists to enforce:
//
//	Text handed to an embedding API must be counted with the embedding model's
//	OWN tokenizer, or with a CALIBRATED UPPER BOUND of it. Counting with
//	cl100k_base alone is never sufficient.
//
// Why it matters, concretely: cl100k_base (tiktoken) and a model's own tokenizer
// disagree by roughly +/-2% and the sign depends on the content. On repetitive
// numeric tables cl100k compresses far harder than XLM-R SentencePiece, so a
// 25,000-character chunk that cl100k scores at 8,143 tokens (just under the
// 8,182-token guard) can be 8,250 tokens in the model's own tokenizer. The
// provider then rejects the whole request ("400 ... code 20015 The parameter is
// invalid" on SiliconFlow) and, before this change, the whole document failed
// with it. See internal/tokenizer/embedding_token_limits.md.

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Counter is one model tokenizer. Implementations must be exact for their own
// tokenizer: TrimToLimit returns a prefix whose Count is <= limit.
type Counter interface {
	// ID is the stable identifier shared with the model catalog
	// (conf/all_models.json "tokenizer").
	ID() string
	// Count is the number of tokens text occupies for this tokenizer.
	Count(text string) int
	// TrimToLimit returns the longest prefix of text with Count(prefix) <= limit.
	// A limit <= 0 yields "".
	TrimToLimit(text string, limit int) string
	// Available reports whether the tokenizer's data loaded. An unavailable
	// counter must not be trusted: an encoder that failed to load silently
	// reports 0 tokens for everything (see bpe_loader.go).
	Available() bool
}

// Counter ids. These are the values accepted in the model catalog's "tokenizer"
// field; anything else (or a missing asset) resolves to the calibrated fallback.
//
// Only ids that actually have a loader are declared: a phantom id would silently
// become a calibrated cl100k count, which is safe but surprising. Adding a family
// means adding its constant together with its loader (see embedding_token_limits.md).
const (
	CounterCL100K        = "cl100k_base"
	CounterXLMRSentence  = "xlmr-spm"
	CounterBERTWordPiece = "bert-wordpiece"
	CounterQwenBPE       = "qwen-bpe"
	CounterLlamaBPE      = "llama-bpe"
)

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

var counterRegistry = struct {
	sync.RWMutex
	byID map[string]Counter
	ctor map[string]func() (Counter, error)
}{byID: make(map[string]Counter), ctor: make(map[string]func() (Counter, error))}

// RegisterCounter installs a ready counter under id, replacing any previous one.
func RegisterCounter(id string, c Counter) {
	if id == "" || c == nil {
		return
	}
	counterRegistry.Lock()
	defer counterRegistry.Unlock()
	counterRegistry.byID[id] = c
}

// RegisterCounterLoader installs a lazy loader for id. The loader runs at most
// once per process; a loader that fails (asset missing) is remembered as
// failed so a broken asset does not re-read the disk for every document.
func RegisterCounterLoader(id string, load func() (Counter, error)) {
	if id == "" || load == nil {
		return
	}
	counterRegistry.Lock()
	defer counterRegistry.Unlock()
	if _, ok := counterRegistry.byID[id]; ok {
		return
	}
	counterRegistry.ctor[id] = load
}

// CounterByID returns the counter for id when it is registered and usable.
func CounterByID(id string) (Counter, bool) {
	if id == "" {
		return nil, false
	}
	counterRegistry.RLock()
	c, ok := counterRegistry.byID[id]
	ctor := counterRegistry.ctor[id]
	counterRegistry.RUnlock()
	if ok && c != nil {
		return c, c.Available()
	}
	if ctor != nil {
		loaded, err := ctor()
		counterRegistry.Lock()
		delete(counterRegistry.ctor, id)
		if err == nil && loaded != nil {
			counterRegistry.byID[id] = loaded
		}
		counterRegistry.Unlock()
		if err == nil && loaded != nil {
			return loaded, loaded.Available()
		}
	}
	return nil, false
}

// CounterStatus is one line of the availability report a process can log at startup.
type CounterStatus struct {
	ID        string
	Available bool
	Source    string // the file that was loaded, when the counter knows it
}

// sourcePather is implemented by the counters that know which file they loaded.
type sourcePather interface{ SourcePath() string }

// CounterStatuses reports every counter the model catalog can reference, so a process can
// say once, at startup, whether the exact counters are usable in this deployment.
//
// This is the earliest sign of a missing asset: the ingest path fails loudly on the first
// document whose model declares an unavailable counter (it refuses to substitute the
// calibrated cl100k estimate, which under-counts some tokenizers - see
// embedding_token_limits.md), so this report names the asset to restore before that happens.
func CounterStatuses() []CounterStatus {
	ids := []string{CounterCL100K, CounterXLMRSentence, CounterBERTWordPiece, CounterQwenBPE, CounterLlamaBPE}
	out := make([]CounterStatus, 0, len(ids))
	for _, id := range ids {
		counter, ok := CounterByID(id)
		if !ok || counter == nil {
			out = append(out, CounterStatus{ID: id})
			continue
		}
		status := CounterStatus{ID: id, Available: true}
		if pathAware, ok := counter.(sourcePather); ok {
			status.Source = pathAware.SourcePath()
		}
		out = append(out, status)
	}
	return out
}

// ResolveCounter maps a catalog tokenizer id onto a counter. Unknown ids, empty
// ids and unloadable assets all resolve to cl100k_base, which the caller compensates
// for with a calibrated ratio (see Limiter). Callers that must NOT degrade - an embedder
// whose model declares a tokenizer - check CounterExact first and refuse instead
// (internal/ingestion/task/embedder.go).
func ResolveCounter(id string) Counter {
	if c, ok := CounterByID(id); ok {
		return c
	}
	if c, ok := CounterByID(CounterCL100K); ok {
		return c
	}
	return unavailableCounter{id: id}
}

// CounterExact reports whether id names a counter that is loaded and therefore
// safe to use without a calibrated ratio.
func CounterExact(id string) bool {
	_, ok := CounterByID(id)
	return ok
}

// ---------------------------------------------------------------------------
// The declared limit and the margin
// ---------------------------------------------------------------------------

const (
	// EmbeddingMarginRatio is the safety margin kept below the model's declared
	// input limit. It is a *ratio*, not a constant: the previous 10-token
	// margin was 0.12% of an 8192-token limit, an order of magnitude smaller
	// than the disagreement between two tokenizers.
	EmbeddingMarginRatio = 0.02
	// EmbeddingMarginFloor keeps the ratio from collapsing for small models.
	EmbeddingMarginFloor = 32
	// EmbeddingTokenLimitDefault is used when a model declares no limit at all.
	// The provider catalog's context_length is the preferred source; this is
	// the last resort and is deliberately the smallest common embedding window
	// rather than 8192, because overshooting a model's window is a hard 400
	// while undershooting only truncates.
	EmbeddingTokenLimitDefault = 2048
	// DefaultUncountedRatioUpper is the starting upper bound for real/own when
	// the model's tokenizer is not available. The L3 shrink-and-retry path
	// catches whatever this misses; being slightly too conservative here only
	// truncates a few percent more text.
	DefaultUncountedRatioUpper = 1.05
)

// EmbeddingTokenLimit returns how many tokens of text may be sent to a model
// whose declared input limit is maxTokens.
func EmbeddingTokenLimit(maxTokens int) int {
	if maxTokens <= 0 {
		return 0
	}
	margin := int(math.Ceil(float64(maxTokens) * EmbeddingMarginRatio))
	if margin < EmbeddingMarginFloor {
		margin = EmbeddingMarginFloor
	}
	if margin >= maxTokens {
		// Tiny windows: keep at least one token instead of going negative.
		if maxTokens <= 1 {
			return maxTokens
		}
		return maxTokens / 2
	}
	return maxTokens - margin
}

// ResolveEmbeddingMaxTokens picks the declared input limit for a model, in the
// order the rest of the system already uses elsewhere: an explicit model value
// first, then the provider catalog's context_length, then the default. It exists
// because defaulting to a hard-coded 8192 overshoots the window of every model
// with a smaller one (the catalog contains 512-token embedding models), and an
// overshoot is a rejected request rather than a truncated one.
func ResolveEmbeddingMaxTokens(declared, contextLength int) int {
	for _, candidate := range []int{declared, contextLength} {
		if candidate > 0 {
			return candidate
		}
	}
	return EmbeddingTokenLimitDefault
}

// ---------------------------------------------------------------------------
// Calibration (L2)
// ---------------------------------------------------------------------------

// calibrationEntry is what we have learned about one (provider instance, model)
// pair's tokenizer ratio: real_tokens / own_tokens. Only ever ratchets up.
type calibrationEntry struct {
	ratio        float64
	samples      int
	limitRejects int
}

// calibrationKey identifies a tokenizer's owner: it must include the endpoint and
// the model, because "bge-m3" tokenizes differently from "bge-m3 @ another
// provider" only in what the provider accepts, not in the tokenizer — but the
// *limit* differs per deployment, so the key stays per provider instance.
type calibrationKey string

// Calibration learns the true/own token ratio per model from provider usage.
type Calibration struct {
	mu      sync.RWMutex
	entries map[calibrationKey]*calibrationEntry
	// defaultRatio is the starting upper bound before any observation.
	defaultRatio float64
}

var defaultCalibration = NewCalibration(DefaultUncountedRatioUpper)

// NewCalibration creates an empty calibration. A non-positive default falls back
// to DefaultUncountedRatioUpper.
func NewCalibration(defaultRatio float64) *Calibration {
	if defaultRatio <= 0 {
		defaultRatio = DefaultUncountedRatioUpper
	}
	return &Calibration{entries: make(map[calibrationKey]*calibrationEntry), defaultRatio: defaultRatio}
}

// DefaultCalibration is the process-wide calibration used by the ingest path.
func DefaultCalibration() *Calibration { return defaultCalibration }

// ObserveUsage records a successful call: the provider reported realCount tokens
// for text our counter scored at ownCount.
func (c *Calibration) ObserveUsage(key string, ownCount, realCount int) {
	if c == nil || ownCount <= 0 || realCount <= 0 {
		return
	}
	ratio := float64(realCount) / float64(ownCount)
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.entryLocked(key)
	e.samples++
	if ratio > e.ratio {
		e.ratio = ratio
	}
}

// ObserveOverLimit records that the provider rejected a request for exceeding
// the model's input window while our own counter scored **the offending input**
// at ownCount tokens against a declared window of maxTokens. That observation is
// enough to raise the ratio bound: the true count is above maxTokens, so the true
// ratio exceeds maxTokens/ownCount. This is how a rejection teaches the next
// attempt to be more conservative instead of repeating the same failure.
//
// ownCount must be a SINGLE input's count, not a batch total: the window bounds
// each input individually, and a batch total is normally above the window, which
// would imply a ratio below 1 and teach nothing.
func (c *Calibration) ObserveOverLimit(key string, ownCount, maxTokens int) {
	if c == nil || ownCount <= 0 || maxTokens <= 0 {
		return
	}
	// A little headroom on top of the implied bound: the observed count is a
	// lower bound on the true count, and we want the next attempt to pass.
	ratio := float64(maxTokens) / float64(ownCount) * 1.01
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.entryLocked(key)
	e.limitRejects++
	if ratio > e.ratio {
		e.ratio = ratio
	}
}

// RatioUpper returns the upper bound of real/own for key, never below 1.
func (c *Calibration) RatioUpper(key string) float64 {
	if c == nil {
		return DefaultUncountedRatioUpper
	}
	c.mu.RLock()
	e := c.entries[calibrationKey(key)]
	c.mu.RUnlock()
	if e == nil || e.ratio <= 0 {
		return c.defaultRatio
	}
	if e.ratio < 1 {
		return 1
	}
	return e.ratio
}

// Stats reports what has been learned, for logging and tests.
func (c *Calibration) Stats(key string) (ratio float64, samples, limitRejects int, ok bool) {
	if c == nil {
		return 0, 0, 0, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	e := c.entries[calibrationKey(key)]
	if e == nil {
		return c.defaultRatio, 0, 0, false
	}
	// The ratio is computed inline rather than through RatioUpper: that helper
	// takes the read lock again, and a writer waiting between the two RLock calls
	// blocks the second one (Go's RWMutex starves new readers once a writer is
	// waiting) while it waits for the first lock to be released - a deadlock.
	ratio = c.defaultRatio
	if e.ratio > 0 {
		ratio = e.ratio
	}
	if ratio < 1 {
		ratio = 1
	}
	return ratio, e.samples, e.limitRejects, true
}

// Reset drops what has been learned for key (used when a model or its tokenizer
// configuration changes).
func (c *Calibration) Reset(key string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, calibrationKey(key))
}

// Keys lists the observed keys, sorted, for diagnostics.
func (c *Calibration) Keys() []string {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, 0, len(c.entries))
	for k := range c.entries {
		out = append(out, string(k))
	}
	sort.Strings(out)
	return out
}

func (c *Calibration) entryLocked(key string) *calibrationEntry {
	e := c.entries[calibrationKey(key)]
	if e == nil {
		// Seed from the configured upper bound, never below 1. The calibration
		// only ratchets UP, so seeding a fresh entry at 1 would drop the safety
		// margin the moment any observation arrives, and a later input with a
		// different token distribution could then be under-counted and rejected.
		e = &calibrationEntry{ratio: math.Max(c.defaultRatio, 1)}
		c.entries[calibrationKey(key)] = e
	}
	return e
}

// ---------------------------------------------------------------------------
// Limiter: counter + calibration + margin, in one place
// ---------------------------------------------------------------------------

// Limiter turns "the model accepts maxTokens" into "send at most this much text".
type Limiter struct {
	counter Counter
	// ratio is the upper bound of real/own. It is 1 for an exact counter and is
	// only consulted when cal is nil.
	ratio float64
	// cal, when set, is read on every call rather than captured, so an
	// over-limit rejection observed by a retry immediately tightens the budget
	// of the next attempt instead of the next document.
	cal *Calibration
	key string
}

// NewExactLimiter is for counters that ARE the model's tokenizer.
func NewExactLimiter(counter Counter) Limiter {
	return Limiter{counter: counter, ratio: 1}
}

// NewCalibratedLimiter is for everything else, including counters that are only
// an approximation: the ratio comes from Calibration.
func NewCalibratedLimiter(counter Counter, key string, cal *Calibration) Limiter {
	if cal == nil {
		cal = defaultCalibration
	}
	return Limiter{counter: counter, cal: cal, key: key}
}

// LimiterFor resolves the counter for a catalog tokenizer id and binds the
// right ratio: exact when the model's own tokenizer is loaded, calibrated
// otherwise.
func LimiterFor(tokenizerID, calibrationKey string, cal *Calibration) Limiter {
	if CounterExact(tokenizerID) {
		return NewExactLimiter(ResolveCounter(tokenizerID))
	}
	return NewCalibratedLimiter(ResolveCounter(tokenizerID), calibrationKey, cal)
}

// Counter returns the underlying counter.
func (l Limiter) Counter() Counter { return l.counter }

// Ratio returns the upper bound of real/own in use.
func (l Limiter) Ratio() float64 {
	if l.cal != nil {
		r := l.cal.RatioUpper(l.key)
		if r < 1 {
			return 1
		}
		return r
	}
	if l.ratio < 1 {
		return 1
	}
	return l.ratio
}

// Calibration exposes the calibration a calibrated limiter reads from, so the
// retry path can record an over-limit rejection against the same key.
func (l Limiter) Calibration() (*Calibration, string) { return l.cal, l.key }

// Limit is the number of tokens of *model* tokens we may send.
func (l Limiter) Limit(maxTokens int) int {
	declared := ResolveEmbeddingMaxTokens(maxTokens, 0)
	if declared <= 0 {
		return 0
	}
	budget := int(math.Floor(float64(declared) / l.Ratio()))
	if budget < 1 {
		budget = 1
	}
	return EmbeddingTokenLimit(budget)
}

// Trim cuts text down to the limiter's budget for maxTokens and reports both the
// trimmed text and its count in the limiter's own counter.
func (l Limiter) Trim(text string, maxTokens int) (string, int) {
	limit := l.Limit(maxTokens)
	if l.counter == nil || !l.counter.Available() {
		// No usable counter: fall back to a byte-level bound that cannot be too
		// generous for any BPE/SPM tokenizer we know of (>= 1 token per 4
		// bytes is the practical floor for text). This path only runs when the
		// cl100k table itself is missing, which the loader already screams
		// about.
		return trimByBytes(text, limit), limit
	}
	trimmed := l.counter.TrimToLimit(text, limit)
	return trimmed, l.counter.Count(trimmed)
}

// trimByBytes is the last-resort bound used when no counter is available.
func trimByBytes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	// One byte per token is the only bound that holds for every tokenizer (a
	// token consumes at least one byte), so `limit` bytes cannot exceed `limit`
	// tokens. A larger byte budget assumes more bytes per token than a worst-case
	// input provides, which would let the provider reject the input for exceeding
	// its window - exactly the failure this fallback exists to avoid.
	maxBytes := limit
	if len(text) <= maxBytes {
		return text
	}
	cut := maxBytes
	for cut > 0 && !utf8Start(text[cut]) {
		cut--
	}
	return text[:cut]
}

// utf8Start reports whether b is not a UTF-8 continuation byte.
func utf8Start(b byte) bool { return b&0xC0 != 0x80 }

// ---------------------------------------------------------------------------
// Over-limit handling: the marker set, the shrink ladder, and the refusal every
// embedder shares. The embedding side of this - the loop that trims, calls the
// driver, and walks the ladder - is EmbeddingModel.EmbedWithinLimit in
// internal/entity/models, next to the RerankModel cut it mirrors.
// ---------------------------------------------------------------------------

// OverLimitFloorTokens is the smallest budget worth trying before calling an input
// genuinely broken rather than merely long.
const OverLimitFloorTokens = 64

// OverLimitMarkers are the provider wordings that mean "this input is longer than
// the model accepts". They are phrases, not codes, so a plain substring test is the
// right one for them.
var OverLimitMarkers = []string{
	"too long",
	"too many tokens",
	"maximum context",
	"context length",
	"context_length",
	"input length",
	"token limit",
	"reduce the length",
	"maximum allowed",
}

// OverLimitCodes are the provider error codes that mean the same thing. 20015 is
// SiliconFlow's: it is returned with a generic "The parameter is invalid" message, so
// the code is the only usable signal.
var OverLimitCodes = []string{"20015"}

// overLimitStatus matches an HTTP 4xx status as a delimited number: "400 Bad
// Request" matches, "1400" and "4000" do not.
var overLimitStatus = regexp.MustCompile(`\b(?:400|413|422)\b`)

// IsOverLimitError reports whether err is an over-limit rejection rather than a
// rate limit or a genuine failure. Only 4xx rejections qualify: a 5xx is the
// provider's problem and a shorter input would not fix it.
//
// Status codes and provider codes are matched as DELIMITED numbers. A substring test
// would read provider code 120015 as SiliconFlow's 20015 (and 1400 as 400), and the
// caller would answer an unrelated failure by re-embedding a silently truncated
// input — or replace the real error with a window-limit one.
func IsOverLimitError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if !overLimitStatus.MatchString(msg) {
		return false
	}
	for _, marker := range OverLimitMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	for _, code := range OverLimitCodes {
		if hasDelimitedNumber(msg, code) {
			return true
		}
	}
	return false
}

// hasDelimitedNumber reports whether s contains num as a whole number: neither
// neighbour may be a digit, so `"code":20015,` matches while 120015 and 200150 do
// not.
func hasDelimitedNumber(s, num string) bool {
	for from := 0; from+len(num) <= len(s); {
		at := strings.Index(s[from:], num)
		if at < 0 {
			return false
		}
		at += from
		end := at + len(num)
		if (at == 0 || !isASCIIDigit(s[at-1])) && (end == len(s) || !isASCIIDigit(s[end])) {
			return true
		}
		from = at + 1
	}
	return false
}

func isASCIIDigit(b byte) bool { return b >= '0' && b <= '9' }

// OverLimitLadder returns the budgets to try, in order, after a provider rejects
// an input as over its window: the caller's budget first, then progressively
// smaller fractions of it, never below OverLimitFloorTokens. Strictly decreasing,
// so the loop always makes progress.
func OverLimitLadder(budget int) []int {
	if budget <= 0 {
		return []int{0}
	}
	limits := []int{budget}
	for _, factor := range []float64{0.75, 0.5, 0.25, 0.125} {
		next := int(float64(budget) * factor)
		if next < OverLimitFloorTokens {
			next = OverLimitFloorTokens
		}
		if next < limits[len(limits)-1] {
			limits = append(limits, next)
		}
	}
	return limits
}

// OverLimitLadderToFloor is OverLimitLadder plus the last-resort floor, for the
// per-input isolation path: an input that no proportional step fits may still fit
// at the floor, and trying it there is what turns "this document cannot be
// embedded" into an embedded (truncated) chunk.
//
// A batch loop deliberately stops above the floor: reaching it there would trim
// EVERY input of the batch to 64 tokens just because one input is pathological,
// while isolating lets each healthy input keep its own budget.
func OverLimitLadderToFloor(budget int) []int {
	limits := OverLimitLadder(budget)
	if limits[len(limits)-1] > OverLimitFloorTokens {
		limits = append(limits, OverLimitFloorTokens)
	}
	return limits
}

// RefuseUnavailableCounter is the refusal every embedder shares: a model that
// DECLARES a tokenizer whose asset is not on disk must not be counted with the
// calibrated cl100k estimate, because that count belongs to a different tokenizer
// and an under-count is what makes a provider answer 400. Silently substituting it
// would trade away the exactness these counters exist for.
func RefuseUnavailableCounter(tokenizerID, calibrationKey string) error {
	if tokenizerID == "" || CounterExact(tokenizerID) {
		return nil
	}
	return fmt.Errorf(
		"embedding tokenizer %q is declared for %s but its asset is unavailable (check that ragflow_deps/huggingface.co is present; run `uv run ragflow_deps/download_go_deps.py`): refusing to count with the calibrated estimate",
		tokenizerID, calibrationKey)
}

// OwnTokenMax is the largest single input's cost in our own counter. The
// provider's window bounds each input individually, so this - not the batch total
// - is what an over-limit rejection tells us about.
func OwnTokenMax(texts []string, counter Counter) int {
	max := 0
	for _, t := range texts {
		if n := counter.Count(t); n > max {
			max = n
		}
	}
	return max
}

// ---------------------------------------------------------------------------
// Counters we can build without extra assets
// ---------------------------------------------------------------------------

type cl100kCounter struct{ id string }

// CountCL100K is the cl100k_base counter, backed by the BPE table shipped in
// ragflow_deps (see bpe_loader.go).
func CountCL100K() Counter { return cl100kCounter{id: CounterCL100K} }

func (c cl100kCounter) ID() string { return c.id }

func (c cl100kCounter) Count(text string) int { return NumTokensFromString(text) }

// IDs reports the token ids cl100k_base assigns to text, for the oracle test's
// id-level comparison.
func (c cl100kCounter) IDs(text string) []int32 {
	enc, err := getCL100KEncoder()
	if err != nil || enc == nil {
		return nil
	}
	tokens := enc.Encode(text, nil, nil)
	ids := make([]int32, len(tokens))
	for i, tok := range tokens {
		ids[i] = int32(tok)
	}
	return ids
}

func (c cl100kCounter) TrimToLimit(text string, limit int) string {
	return TrimContentToTokenLimit(text, limit)
}

func (c cl100kCounter) Available() bool {
	enc, err := getCL100KEncoder()
	return err == nil && enc != nil
}

// SourcePath is the table file the loader accepted, when it got that far.
func (c cl100kCounter) SourcePath() string { return cl100kTableSource() }

// unavailableCounter is returned when nothing else could be built. It reports
// zero tokens, so callers must check Available() before trusting a count; the
// Limiter does.
type unavailableCounter struct{ id string }

func (c unavailableCounter) ID() string {
	if c.id == "" {
		return "unavailable"
	}
	return c.id
}

func (unavailableCounter) Count(string) int { return 0 }

func (unavailableCounter) TrimToLimit(text string, limit int) string {
	return trimByBytes(text, limit)
}

func (unavailableCounter) Available() bool { return false }

func init() {
	RegisterCounter(CounterCL100K, CountCL100K())
}

// DescribeCounter renders a counter for logs.
func DescribeCounter(c Counter) string {
	if c == nil {
		return "counter=<nil>"
	}
	return fmt.Sprintf("counter=%s(available=%t)", strings.TrimSpace(c.ID()), c.Available())
}
