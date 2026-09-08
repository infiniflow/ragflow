package common

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"time"

	appcommon "ragflow/internal/common"

	"go.uber.org/zap"
)

// jsonRetryMax is how many times a non-JSON (or otherwise transiently failed)
// LLM reply is retried before GenJSON gives up. The highest-frequency LLM
// integration must not drop a knowledge unit on a single formatting hiccup, so
// a one-off malformed reply triggers a fresh LLM call instead of an immediate
// failure.
const jsonRetryMax = 5

// jsonRetryDelay is the initial exponential-backoff delay between retries.
const jsonRetryDelay = 2 * time.Second

// fencedJSONRE matches a ```json ... ``` or ``` ... ``` fenced block. Models
// frequently wrap JSON in such fences even when JSONMode is requested, which
// previously slipped through as an unparseable "_raw" payload and was silently
// dropped by the extraction parser (a data-loss path). We strip the fence and
// retry before giving up.
var fencedJSONRE = regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)\\s*```")

// GenJSON dispatches a chat call in JSON mode and parses the response into a
// map. It first tries the raw content, then a fenced ```json ... ``` block the
// model may have wrapped around the JSON, and finally the outermost {...} span
// (handles "Here is the JSON: {...}" prose). A genuine parse failure is
// returned as an error (NOT a silent {"_raw": ...}) so the caller fails loudly
// or retries rather than silently dropping the extraction — the
// highest-frequency LLM integration must not lose knowledge units on a
// formatting hiccup.
// GenJSON asks the model for a JSON reply and parses it. retryMax is an
// optional override for jsonRetryMax: pass 0 to disable retries entirely
// (used where the caller already budgets external calls itself, e.g. the
// entity-merge disambiguator — retrying there would blow through the budget).
func GenJSON(ctx context.Context, chat ChatInvoker, req ChatRequest, retryMax ...int) (map[string]any, error) {
	maxRetries := jsonRetryMax
	if len(retryMax) > 0 {
		maxRetries = retryMax[0]
	}
	req.JSONMode = true
	// GenJSON owns retries for both transport failures and malformed JSON. Tell
	// the production ChatInvoker to make exactly one provider request per outer
	// GenJSON attempt, avoiding multiplicative nested retries.
	req.DisableRetry = true
	var lastErr error
	delay := jsonRetryDelay
	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err := chat.Chat(ctx, req)
		if err != nil {
			// Permanent chat errors (auth, unknown model, context-length,
			// cancelled ctx) cannot succeed on a retry; escape immediately.
			if !appcommon.IsTransientError(err) {
				return nil, err
			}
			// Transient chat failure (timeout / transport / provider); retry
			// with a fresh LLM call.
			lastErr = err
		} else {
			candidates := jsonCandidates(resp.Content)
			for _, candidate := range candidates {
				if m, ok := tryUnmarshalJSON(candidate); ok {
					return m, nil
				}
			}
			// A non-JSON reply must NOT be persisted/reused; discard it and
			// immediately re-issue the call so a formatting hiccup does not
			// abort the whole compile. Log candidate length and the unmarshal
			// error only — the raw body may carry customer-derived content
			// (PII), so it is deliberately excluded from the log.
			for i, candidate := range candidates {
				_, perr := tryUnmarshalJSONErr(candidate)
				appcommon.Info("knowledge_compiler: GenJSON unparseable candidate",
					zap.Int("attempt", attempt), zap.Int("candidate", i),
					zap.Int("len", len(candidate)), zap.Error(perr))
			}
			lastErr = fmt.Errorf("knowledge_compiler: LLM response is not parseable JSON (%d bytes)", len(resp.Content))
		}
		if attempt == maxRetries {
			break
		}
		appcommon.Info("knowledge_compiler: GenJSON attempt failed, retrying",
			zap.Int("attempt", attempt), zap.Duration("delay", delay),
			zap.Error(lastErr))
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay + time.Duration(rand.Int63n(int64(delay/2)+1))):
			// Jittered backoff so concurrent GenJSON jobs (parallel wiki/plan
			// batches) do not back off in lockstep and pile up on the provider.
		}
		delay *= 2
		if delay > time.Minute {
			delay = time.Minute
		}
	}
	return nil, lastErr
}

// jsonCandidates yields progressively "cleaned" versions of an LLM reply that
// may contain JSON: the raw text, a fenced ```json ... ``` block, and the
// outermost {...} span.
func jsonCandidates(s string) []string {
	cands := []string{s}
	if loc := fencedJSONRE.FindStringSubmatch(s); len(loc) == 2 {
		cands = append(cands, loc[1])
	}
	if i := strings.Index(s, "{"); i >= 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			cands = append(cands, s[i:j+1])
		}
	}
	return cands
}

func tryUnmarshalJSON(s string) (map[string]any, bool) {
	m, err := tryUnmarshalJSONErr(s)
	return m, err == nil
}

func tryUnmarshalJSONErr(s string) (map[string]any, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty candidate")
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, err
	}
	return m, nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
