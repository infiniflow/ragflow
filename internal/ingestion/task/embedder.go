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

package task

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity/models"
	componentpkg "ragflow/internal/ingestion/component"
	"ragflow/internal/service"
	"ragflow/internal/tokenizer"
)

type embedder struct {
	model *models.EmbeddingModel

	// limiter is resolved once per embedder (one embedder is built per tokenizer
	// invocation, i.e. per document) and binds the model's tokenizer, the
	// declared input window and the calibration for this provider instance.
	limiterOnce sync.Once
	limiterVal  tokenizer.Limiter
	// limiterErr records the model declaring a tokenizer whose asset is not on disk.
	// It is not a warning: the counter that would be used instead is a calibrated
	// estimate of a *different* tokenizer, so the embedder refuses to run (limiter()).
	limiterErr error
}

func (e *embedder) MaxTokens() int {
	if e == nil || e.model == nil {
		return 0
	}
	return e.model.MaxTokens
}

// ResolveMaxTokens is the input window to actually honour: the model's declared
// value, then the provider catalog's context_length, then a conservative
// default. MaxTokens() stays as the raw declaration for compatibility.
func (e *embedder) ResolveMaxTokens() int {
	if e == nil || e.model == nil {
		return tokenizer.EmbeddingTokenLimitDefault
	}
	return tokenizer.ResolveEmbeddingMaxTokens(e.model.ResolveMaxTokens(), 0)
}

func (e *embedder) BatchSize() int {
	if e == nil || e.model == nil {
		return models.DefaultEmbeddingBatchSize
	}
	return e.model.ResolveBatchSize()
}

// limiter returns the counter+margin+calibration for this embedding model.
//
// A model that declares a tokenizer family whose asset is not on disk is an ERROR, not
// a warning: the calibrated cl100k fallback counts a different tokenizer (cl100k
// under-counts XLM-R on some content, and an under-count is what makes a provider answer
// 400), so carrying on would trade away the exactness these counters exist for. Trim()
// cannot report it - the EmbedderTrimmer seam returns no error - so it surfaces on the
// first Encode(), which every ingest reaches.
func (e *embedder) limiter() tokenizer.Limiter {
	e.limiterOnce.Do(func() {
		id := ""
		if e.model != nil {
			id = e.model.ResolveTokenizerID()
		}
		if id != "" && !tokenizer.CounterExact(id) {
			e.limiterErr = fmt.Errorf(
				"embedding tokenizer %q is declared for %s but its asset is unavailable (check that ragflow_deps/huggingface.co is present; run `uv run ragflow_deps/download_go_deps.py`): refusing to count with the calibrated estimate",
				id, e.quotaKey())
		}
		e.limiterVal = tokenizer.LimiterFor(id, string(e.quotaKey()), tokenizer.DefaultCalibration())
	})
	return e.limiterVal
}

// Trim cuts text down to what this model accepts. This implements the
// component's optional EmbedderTrimmer seam, which is what makes the ingest path
// count with the model's own tokenizer (or a calibrated bound of it) instead of
// cl100k alone.
func (e *embedder) Trim(text string) (string, int) {
	limiter := e.limiter()
	return limiter.Trim(text, e.ResolveMaxTokens())
}

// Encode embeds texts, keeping three properties the previous implementation did
// not have:
//
//  1. It never lets one oversized input fail the whole document: an over-limit
//     rejection shrinks the budget and retries, then isolates the offending
//     input, instead of returning the error to the caller;
//  2. It reports the provider's own token usage, which doubles as the token
//     count for accounting and as the oracle that calibrates our counting;
//  3. It records an over-limit rejection against the calibration, so the next
//     attempt - and the next document - starts from a tighter bound.
func (e *embedder) Encode(ctx context.Context, texts []string) ([]componentpkg.EmbeddingResult, error) {
	if e.model.ModelDriver == nil {
		return nil, fmt.Errorf("embedder: embedding model driver is nil for model %v", e.model.ModelName)
	}
	if len(texts) == 0 {
		return []componentpkg.EmbeddingResult{}, nil
	}
	limiter := e.limiter()
	if e.limiterErr != nil {
		return nil, e.limiterErr
	}
	maxTokens := e.ResolveMaxTokens()
	cal, calKey := limiter.Calibration()

	var lastErr error
	for _, limit := range overLimitLadder(limiter.Limit(maxTokens)) {
		candidate := trimAll(texts, limiter, maxTokens, limit)
		embeds, usage, err := e.embedWithRetry(ctx, candidate)
		if err == nil {
			e.recordUsage(cal, calKey, limiter, candidate, usage)
			return distributeTokenCount(embeds, usage, candidate, limiter), nil
		}
		if !isOverLimitErr(err) {
			return nil, err
		}
		// An over-limit rejection is the only signal that reveals the true
		// ratio for a model we cannot count exactly; use it before retrying. The
		// window applies per input, so the observation must be the LARGEST
		// input's own count: a batch total is expected to exceed the window and
		// would imply a ratio below 1, i.e. teach us nothing.
		if cal != nil {
			cal.ObserveOverLimit(calKey, ownTokenMax(candidate, limiter), maxTokens)
		}
		common.Warn("embedding input over the model limit; shrinking and retrying",
			zap.String("model", derefString(e.model.ModelName)),
			zap.Int("limit", limit),
			zap.String("counter", limiter.Counter().ID()),
			zap.Float64("ratio", limiter.Ratio()))
		lastErr = err
	}

	// The shrink ladder was not enough, which means the batch probably contains
	// one pathological input next to healthy ones. Isolate instead of failing.
	vecs, err := e.encodeIsolating(ctx, texts, limiter, maxTokens)
	if err == nil {
		return vecs, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, err
}

// overLimitLadder is the sequence of budgets tried, as fractions of the first
// one, oldest first. Trimming is a prefix operation, so each step simply sends a
// shorter prefix than the previous one.
//
// 1 -> 0.75 -> 0.5 -> 0.25 -> 0.125: the first two steps are gentle, because an
// over-limit rejection usually means the input is a little past the window (a
// tokenizer that under-counts by a percent or two), and cutting a quarter of the
// content for that would lose text for no reason. The later steps are for the
// genuinely oversized input (a table the chunker never split), where only a large
// cut helps.
//
// The ladder deliberately stops above the floor: reaching it here would trim EVERY
// input of the batch to 64 tokens because one input is pathological, while the
// per-input isolation pass (overLimitLadderToFloor, which Encode falls through to)
// lets each healthy input keep its own budget. A provider that rejects even the last
// proportional step therefore still gets one attempt at the floor - per input, on the
// input that actually needs it.
func overLimitLadder(budget int) []int {
	if budget <= 0 {
		return []int{0}
	}
	limits := []int{budget}
	for _, factor := range []float64{0.75, 0.5, 0.25, 0.125} {
		next := int(float64(budget) * factor)
		if next < overLimitFloorTokens {
			next = overLimitFloorTokens
		}
		if next < limits[len(limits)-1] {
			limits = append(limits, next)
		}
	}
	return limits
}

// overLimitLadderToFloor is overLimitLadder plus the last-resort floor, for the
// per-input isolation path: an input that no proportional step fits may still fit
// at the floor, and trying it there is what turns "this document cannot be
// embedded" into an embedded (truncated) chunk.
//
// The batch loop deliberately stops above the floor. Reaching it there would trim
// EVERY input of the batch to 64 tokens just because one input is pathological,
// while isolating lets each healthy input keep its own budget.
func overLimitLadderToFloor(budget int) []int {
	limits := overLimitLadder(budget)
	if limits[len(limits)-1] > overLimitFloorTokens {
		limits = append(limits, overLimitFloorTokens)
	}
	return limits
}

// overLimitFloorTokens is the smallest budget worth trying before declaring the
// input genuinely broken rather than merely long.
const overLimitFloorTokens = 64

// trimAll re-trims every text to `limit` tokens. Texts handed in by the caller
// are already trimmed to the full budget, and trimming a prefix again just makes
// it shorter, so this needs no access to the untrimmed input.
func trimAll(texts []string, limiter tokenizer.Limiter, maxTokens, limit int) []string {
	out := make([]string, len(texts))
	for i, t := range texts {
		out[i] = limiter.Counter().TrimToLimit(t, limit)
	}
	return out
}

// ownTokenTotal estimates what the batch costs in our own counter, used for the
// usage-based calibration.
func ownTokenTotal(texts []string, limiter tokenizer.Limiter) int {
	total := 0
	for _, t := range texts {
		total += limiter.Counter().Count(t)
	}
	return total
}

// ownTokenMax is the largest single input's cost in our own counter. The
// provider's window bounds each input individually, so this - not the batch
// total - is what an over-limit rejection tells us about.
func ownTokenMax(texts []string, limiter tokenizer.Limiter) int {
	max := 0
	for _, t := range texts {
		if n := limiter.Counter().Count(t); n > max {
			max = n
		}
	}
	return max
}

// embedWithRetry performs one embedding call, retrying rate limits exactly as
// before (a TPM-limited provider refills on a one-minute window, and the shared
// cooldown damps all workers at once). It returns the provider's token usage for
// the request.
func (e *embedder) embedWithRetry(ctx context.Context, texts []string) ([]models.EmbeddingData, int, error) {
	config := &models.EmbeddingConfig{Dimension: 0}
	req := models.EmbedRequest{Texts: texts}
	// The cooldown is scoped to this provider instance: workers embedding
	// through the same credentials+endpoint back off together, everything else
	// keeps running.
	quota := e.quotaKey()

	var (
		embeds []models.EmbeddingData
		err    error
	)
	for attempt := 1; attempt <= maxEncodeAttempts; attempt++ {
		// Sit out any cooldown that another worker's 429 started. The provider
		// quota is shared, so a worker that keeps firing while the window is
		// exhausted only earns more 429s - and the whole point of the cooldown
		// is that one worker's rejection damps all of them.
		if cerr := waitOutCooldown(ctx, quota); cerr != nil {
			return nil, 0, cerr
		}
		usage := &common.ModelUsage{}
		embeds, err = e.model.ModelDriver.Embed(ctx, e.model.ModelName, req, e.model.APIConfig, config, usage)
		if err == nil {
			return embeds, usage.InputTokens, nil
		}
		if !isRetryableErr(err) {
			return nil, 0, err
		}
		if attempt == maxEncodeAttempts {
			return nil, 0, err
		}
		wait := encodeBackoff(attempt, err)
		startCooldown(quota, wait)
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil, 0, err
}

// distributeTokenCount attaches a token count to every input. Providers report
// usage per request, so the total is spread over the inputs in proportion to our
// own counts of exactly the texts we sent; the rounding remainder goes to the
// largest input so the sum matches the provider's number.
func distributeTokenCount(embeds []models.EmbeddingData, usageTotal int, texts []string, limiter tokenizer.Limiter) []componentpkg.EmbeddingResult {
	vecs := make([]componentpkg.EmbeddingResult, len(embeds))
	own := make([]int, len(texts))
	total := 0
	for i, t := range texts {
		own[i] = limiter.Counter().Count(t)
		total += own[i]
	}
	if usageTotal <= 0 || total <= 0 {
		// No usage reported by the provider: fall back to our own count rather
		// than to 0, which is what the previous implementation always reported.
		for i, v := range embeds {
			tc := 0
			if i < len(own) {
				tc = own[i]
			}
			vecs[i] = componentpkg.EmbeddingResult{Vector: v.Embedding, TokenCount: tc}
		}
		return vecs
	}
	largest, assigned := 0, 0
	for i := range own {
		if own[i] > own[largest] {
			largest = i
		}
	}
	for i, v := range embeds {
		tc := 0
		if i < len(own) {
			tc = usageTotal * own[i] / total
			assigned += tc
		}
		vecs[i] = componentpkg.EmbeddingResult{Vector: v.Embedding, TokenCount: tc}
	}
	if reminder := usageTotal - assigned; reminder > 0 && largest < len(vecs) {
		vecs[largest].TokenCount += reminder
	}
	return vecs
}

// recordUsage feeds the calibration: the provider's total for this batch against
// our own count of the same texts.
func (e *embedder) recordUsage(cal *tokenizer.Calibration, key string, limiter tokenizer.Limiter, texts []string, usageTotal int) {
	if cal == nil || usageTotal <= 0 {
		return
	}
	own := ownTokenTotal(texts, limiter)
	if own <= 0 {
		return
	}
	cal.ObserveUsage(key, own, usageTotal)
}

// encodeIsolating is the last resort: it embeds inputs one at a time so a single
// pathological input cannot drag the rest down, and falls back to the floor
// budget for inputs that still do not fit. It returns an error only when an
// input fails at the floor, i.e. for reasons unrelated to size.
func (e *embedder) encodeIsolating(ctx context.Context, texts []string, limiter tokenizer.Limiter, maxTokens int) ([]componentpkg.EmbeddingResult, error) {
	out := make([]componentpkg.EmbeddingResult, len(texts))
	cal, calKey := limiter.Calibration()
	var lastErr error
	for i, text := range texts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		embedded := false
		for _, limit := range overLimitLadderToFloor(limiter.Limit(maxTokens)) {
			candidate := []string{limiter.Counter().TrimToLimit(text, limit)}
			embeds, usage, err := e.embedWithRetry(ctx, candidate)
			if err == nil {
				e.recordUsage(cal, calKey, limiter, candidate, usage)
				out[i] = distributeTokenCount(embeds, usage, candidate, limiter)[0]
				embedded = true
				break
			}
			if !isOverLimitErr(err) {
				return nil, err
			}
			if cal != nil {
				cal.ObserveOverLimit(calKey, limiter.Counter().Count(candidate[0]), maxTokens)
			}
			lastErr = err
		}
		if !embedded {
			return nil, fmt.Errorf("embedder: input %d does not fit the model window even at %d tokens: %w", i, overLimitFloorTokens, lastErr)
		}
	}
	return out, nil
}

// overLimitMarkers are the provider wordings that mean "this input is longer
// than the model accepts". 20015 is SiliconFlow's code for exactly that (it
// answers a generic "The parameter is invalid" message); the rest are the usual
// phrasings elsewhere.
var overLimitMarkers = []string{
	"20015",
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

// isOverLimitErr reports whether err is an over-limit rejection rather than a
// rate limit or a genuine failure. Only 4xx rejections qualify: a 5xx is the
// provider's problem and shrinking the input would not help.
func isOverLimitErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "400") && !strings.Contains(msg, "413") && !strings.Contains(msg, "422") {
		return false
	}
	for _, marker := range overLimitMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// Retry policy for rate-limited embedding calls.
//
// A TPM-limited provider (SiliconFlow bge-m3 answers "429 ... TPM limit
// reached") refills its quota on a one-minute window, so a retry schedule that
// exhausts itself inside a single window cannot succeed: the 2s/4s/8s/16s of the
// original implementation always ran out while the quota was still spent, and
// every concurrent worker kept hammering the same exhausted quota, turning a
// transient throttle into a FAILED document whose chunks had already been
// deleted. The schedule below spans roughly two and a half windows, and the
// shared cooldown makes the workers back off together instead of independently.
//
// These are variables rather than constants so tests can shrink them.
var (
	maxEncodeAttempts   = 6
	encodeBackoffBase   = 5 * time.Second
	encodeBackoffMax    = 120 * time.Second
	encodeBackoffJitter = 0.2
)

// providerQuotaKey identifies the thing a TPM quota actually belongs to: one
// provider instance (endpoint + credentials + model). See quotaKey.
type providerQuotaKey string

// rateLimitCooldowns parks callers per provider instance - and only per provider
// instance.
//
// The unit is the provider instance, not the process: a TPM quota is owned by
// the credentials, so a worker that got throttled on provider A must not stall
// ingestion that talks to provider B, and must not stall a different dataset
// embedding through a different model. It cannot be the embedder value either:
// newEmbedderResolver builds a fresh embedder for every tokenizer invocation
// (one per document), so state hung off the value would never be shared by the
// workers that are actually competing for the same quota. A keyed registry
// gives exactly the required scope - all workers embedding through the same
// provider instance back off together, everyone else is untouched.
var rateLimitCooldowns = struct {
	sync.Mutex
	until map[providerQuotaKey]time.Time
}{until: make(map[providerQuotaKey]time.Time)}

// startCooldown parks callers sharing `key` for at least d, extending an active
// cooldown but never shortening it.
func startCooldown(key providerQuotaKey, d time.Duration) {
	if d <= 0 {
		return
	}
	deadline := time.Now().Add(d)
	rateLimitCooldowns.Lock()
	defer rateLimitCooldowns.Unlock()
	if existing, ok := rateLimitCooldowns.until[key]; !ok || deadline.After(existing) {
		rateLimitCooldowns.until[key] = deadline
	}
	// Keys are bounded by the number of configured provider instances, so this
	// is housekeeping rather than a leak fix.
	if len(rateLimitCooldowns.until) > 64 {
		for k, until := range rateLimitCooldowns.until {
			if !until.After(time.Now()) {
				delete(rateLimitCooldowns.until, k)
			}
		}
	}
}

// cooldownRemaining reports how long the cooldown for `key` still has to run.
func cooldownRemaining(key providerQuotaKey) time.Duration {
	rateLimitCooldowns.Lock()
	defer rateLimitCooldowns.Unlock()
	return time.Until(rateLimitCooldowns.until[key])
}

// waitOutCooldown blocks until the cooldown for `key` expires, honoring ctx.
func waitOutCooldown(ctx context.Context, key providerQuotaKey) error {
	remaining := cooldownRemaining(key)
	if remaining <= 0 {
		return nil
	}
	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// quotaKey derives the provider-instance key the cooldown is scoped to. The API
// key decides who owns the quota, so it is part of the key but hashed: the key
// must never carry a plaintext secret.
func (e *embedder) quotaKey() providerQuotaKey {
	if e == nil || e.model == nil {
		return providerQuotaKey("embedder:unknown")
	}
	var baseURL, region, apiKey string
	if cfg := e.model.APIConfig; cfg != nil {
		baseURL = derefString(cfg.BaseURL)
		region = derefString(cfg.Region)
		apiKey = derefString(cfg.ApiKey)
	}
	sum := sha256.Sum256([]byte(apiKey))
	return providerQuotaKey(fmt.Sprintf("%s|%s|%s|%x", baseURL, region, derefString(e.model.ModelName), sum[:8]))
}

// derefString safely dereferences an optional string.
func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// encodeBackoff returns how long to wait before retry `attempt` (1-based) of a
// call that failed with `err`. A provider-supplied Retry-After wins; otherwise
// the delay grows exponentially with jitter so concurrent workers spread out
// instead of retrying in lockstep.
func encodeBackoff(attempt int, err error) time.Duration {
	if hint, ok := retryAfterFromError(err); ok {
		if hint > encodeBackoffMax {
			return encodeBackoffMax
		}
		return hint
	}
	delay := float64(encodeBackoffBase) * math.Pow(2, float64(attempt-1))
	if delay > float64(encodeBackoffMax) {
		delay = float64(encodeBackoffMax)
	}
	if encodeBackoffJitter > 0 {
		spread := delay * encodeBackoffJitter
		delay += (rand.Float64()*2 - 1) * spread
	}
	if delay < float64(time.Second) {
		delay = float64(time.Second)
	}
	return time.Duration(delay)
}

// retryAfterPattern matches the seconds form of Retry-After surfaced by the
// model drivers ("retry-after: 30", "retry_after=30", "retry after 30 seconds").
var retryAfterPattern = regexp.MustCompile(`(?i)retry[-_ ]?after["'\s:=]+(\d+(\.\d+)?)`)

// retryAfterFromError extracts a Retry-After hint in seconds from an error
// message, if the driver carried one.
func retryAfterFromError(err error) (time.Duration, bool) {
	if err == nil {
		return 0, false
	}
	m := retryAfterPattern.FindStringSubmatch(err.Error())
	if m == nil {
		return 0, false
	}
	seconds, perr := strconv.ParseFloat(m[1], 64)
	if perr != nil || seconds <= 0 {
		return 0, false
	}
	return time.Duration(seconds * float64(time.Second)), true
}

// retryableStatus reports whether an HTTP status means "the provider could not
// serve this request now" rather than "this request is bad": 429, 529 (Anthropic's
// overloaded status) and the provider-side 5xx family.
func retryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == 529 || status >= 500
}

// statusBefore/statusAfter pick the HTTP status a driver wrote into its own
// error message ("... error: 429 ...", "... status 503 ...", "500 Internal
// Server Error"). The status WORDING is required so a bare three-digit number in
// a response body (a token count, a document id) cannot be read as a status.
var (
	statusBefore = regexp.MustCompile(`(?i)\b(?:http|status|code|error)\b[^0-9]{0,24}(\d{3})`)
	statusAfter  = regexp.MustCompile(`(?i)(\d{3})[^0-9]{0,24}\b(?:http|status|code|error)\b`)
)

// statusFromMessage extracts the status a driver embedded in its message.
func statusFromMessage(msg string) (int, bool) {
	for _, re := range []*regexp.Regexp{statusBefore, statusAfter} {
		m := re.FindStringSubmatch(msg)
		if m == nil {
			continue
		}
		code, err := strconv.Atoi(m[1])
		if err != nil || code < 100 || code >= 600 {
			continue
		}
		return code, true
	}
	return 0, false
}

// isRetryableErr reports whether a failed Embed call is worth retrying: the
// provider is rate limiting us, failed on its side (5xx/529), or the request
// never completed (transport failure).
//
// Structured evidence decides first — models.APIStatusError carries the status
// seen by the shared HTTP helpers, and net.Error covers timeouts/DNS/refused
// connections. Only the drivers that build their own HTTP errors have to be read
// from the message (every embedding driver does, unlike the chat path, whose
// helpers return APIStatusError); there an explicit status decides in BOTH
// directions, so a permanent 4xx can no longer buy six retries plus a shared
// cooldown just because its body also mentions rate limits. Provider wording is
// consulted only when no status is present at all.
func isRetryableErr(err error) bool {
	if err == nil {
		return false
	}
	var statusErr *models.APIStatusError
	if errors.As(err, &statusErr) {
		return retryableStatus(statusErr.Status)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return isRetryableNetErr(err, netErr)
	}
	msg := strings.ToLower(err.Error())
	// net/http's HTTP/2 transport uses an internal error type for a graceful
	// connection retirement, so errors.As cannot identify it here. Embedding is
	// safe to repeat, and the transport will put the retry on a fresh connection.
	if strings.Contains(msg, "http2: server sent goaway and closed the connection") &&
		strings.Contains(msg, "errcode=no_error") {
		return true
	}
	if code, ok := statusFromMessage(msg); ok {
		return retryableStatus(code)
	}
	return strings.Contains(msg, "too many requests") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "rate limiting") ||
		strings.Contains(msg, "tpm limit")
}

// isRetryableNetErr separates transient transport failures from permanent ones. A
// net.Error is NOT automatically transient: net.DNSError reports NXDOMAIN ("no such
// host") as one, and a hostname that does not resolve - or a connection the host
// refuses - is a configuration fact no retry changes. Retrying it would spend six
// attempts and a shared cooldown on an answer that cannot get better, and delay the
// error an operator needs to see.
//
// Retried: a timeout, a DNS failure the resolver flagged as temporary, and a
// connection dropped after the request was accepted (reset / broken pipe / aborted).
// Everything else is returned to the caller as-is.
func isRetryableNetErr(err error, netErr net.Error) bool {
	if netErr.Timeout() {
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return dnsErr.IsTemporary || dnsErr.IsTimeout
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return errors.Is(opErr.Err, syscall.ECONNRESET) ||
			errors.Is(opErr.Err, syscall.EPIPE) ||
			errors.Is(opErr.Err, syscall.ECONNABORTED)
	}
	return false
}

// newEmbedderResolver builds the production embedder resolver used by the
// Tokenizer component. It always resolves the embedder from the dataset's
// configured embd_id (looked up by kbID) and returns that embd_id alongside the
// embedder, so the Tokenizer can key its per-chunk cache on the dataset-bound
// model. If the dataset has no embd_id configured, it returns a nil embedder and
// an empty embd_id (no embedding). Kept as a constructor over injectable deps so
// the resolution logic stays unit-testable without a live model provider / DB.
func newEmbedderResolver(
	getKBEmbdID func(ctx context.Context, kbID string) (string, error),
	getEmbeddingModel func(ctx context.Context, tenantID, embdID string) (*models.EmbeddingModel, error),
) componentpkg.EmbedderResolver {
	// The resolver derives the embedding model exclusively from the
	// knowledgebase's configured embd_id — never from any DSL-supplied
	// identifier. It returns embdID alongside the embedder so the tokenizer can
	// key its per-chunk cache on the dataset-bound model: when a KB's embedding
	// model changes, embdID changes, the cache key changes, and no stale vector
	// is ever served.
	return func(ctx context.Context, tenantID, kbID string) (componentpkg.Embedder, string, error) {
		embdID, err := getKBEmbdID(ctx, kbID)
		if err != nil {
			return nil, "", fmt.Errorf("embedder: resolve kb embd_id for kb_id=%s: %w", kbID, err)
		}
		embdID = strings.TrimSpace(embdID)
		if embdID == "" {
			return nil, "", nil
		}
		model, err := getEmbeddingModel(ctx, tenantID, embdID)
		if err != nil {
			return nil, "", err
		}
		if model == nil {
			return nil, "", fmt.Errorf("embedder: resolved embedding model is nil for embd_id=%s", embdID)
		}
		return &embedder{model: model}, embdID, nil
	}
}

// init wires the production embedder resolver into the component package. The
// component package must not import internal/service (dependency direction),
// so the concrete resolver is injected here - the task package is the
// composition root for ingestion runs.
func init() {
	componentpkg.DefaultEmbedderResolver = newEmbedderResolver(
		func(ctx context.Context, kbID string) (string, error) {
			kb, err := dao.NewKnowledgebaseDAO().GetByID(ctx, dao.DB, kbID)
			if err != nil {
				return "", err
			}
			if kb == nil {
				return "", nil
			}
			return kb.EmbdID, nil
		},
		service.NewModelProviderService().GetEmbeddingModel,
	)
}
