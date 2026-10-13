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
	"errors"
	"fmt"
	"net"
	"strings"
	"syscall"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity/models"
)

const tpmLimitErr = `SILICONFLOW API error: 429 Too Many Requests, body: {"message":"Request was rejected due to rate limiting. Details: TPM limit reached.","data":null}`

// flakyDriver fails its first `failures` Embed calls with `err`.
type flakyDriver struct {
	stubDriver
	failures int
	err      error
	calls    int
}

func (d *flakyDriver) Embed(ctx context.Context, modelName *string, request models.EmbedRequest, apiConfig *models.APIConfig, embeddingConfig *models.EmbeddingConfig, usage *common.ModelUsage) ([]models.EmbeddingData, error) {
	d.calls++
	if d.calls <= d.failures {
		return nil, d.err
	}
	return d.stubDriver.Embed(ctx, modelName, request, apiConfig, embeddingConfig, usage)
}

func newFlakyEmbedder(driver *flakyDriver) *embedder {
	return newFlakyEmbedderWithConfig(driver, &models.APIConfig{})
}

func newFlakyEmbedderWithConfig(driver *flakyDriver, cfg *models.APIConfig) *embedder {
	return &embedder{model: models.NewEmbeddingModel(driver, strPtr("embed"), cfg, 128)}
}

// freezeBackoff shrinks the retry schedule to milliseconds and clears any
// cooldown left behind by an earlier test, restoring both on cleanup.
func freezeBackoff(t *testing.T) {
	t.Helper()
	prevBase, prevMax, prevJitter, prevAttempts := encodeBackoffBase, encodeBackoffMax, encodeBackoffJitter, maxEncodeAttempts
	encodeBackoffBase = time.Millisecond
	encodeBackoffMax = 5 * time.Millisecond
	encodeBackoffJitter = 0
	resetCooldown()
	t.Cleanup(func() {
		encodeBackoffBase, encodeBackoffMax, encodeBackoffJitter, maxEncodeAttempts = prevBase, prevMax, prevJitter, prevAttempts
		resetCooldown()
	})
}

func resetCooldown() {
	rateLimitCooldowns.Lock()
	rateLimitCooldowns.until = make(map[providerQuotaKey]time.Time)
	rateLimitCooldowns.Unlock()
}

func TestEmbedderEncode_RetriesRateLimitThenSucceeds(t *testing.T) {
	freezeBackoff(t)
	driver := &flakyDriver{failures: 2, err: errors.New(tpmLimitErr)}
	emb := newFlakyEmbedder(driver)

	vecs, err := emb.Encode(t.Context(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if driver.calls != 3 {
		t.Fatalf("driver calls = %d, want 3 (two 429s then success)", driver.calls)
	}
	if len(vecs) != 2 {
		t.Fatalf("vectors = %d, want 2", len(vecs))
	}
}

func TestEmbedderEncode_GivesUpAfterMaxAttempts(t *testing.T) {
	freezeBackoff(t)
	maxEncodeAttempts = 3
	driver := &flakyDriver{failures: 99, err: errors.New(tpmLimitErr)}
	emb := newFlakyEmbedder(driver)

	if _, err := emb.Encode(t.Context(), []string{"a"}); err == nil {
		t.Fatal("expected the rate-limit error to surface once attempts are exhausted")
	}
	if driver.calls != 3 {
		t.Fatalf("driver calls = %d, want 3", driver.calls)
	}
}

func TestEmbedderEncode_NonRateLimitErrorFailsFast(t *testing.T) {
	freezeBackoff(t)
	driver := &flakyDriver{failures: 99, err: errors.New("SILICONFLOW API error: 400 Bad Request, body: bad input")}
	emb := newFlakyEmbedder(driver)

	if _, err := emb.Encode(t.Context(), []string{"a"}); err == nil {
		t.Fatal("expected an error")
	}
	if driver.calls != 1 {
		t.Fatalf("driver calls = %d, want 1: only rate limits are retried", driver.calls)
	}
}

// A 429 parks every worker on the same provider instance, not just the caller
// that saw it: the quota they compete for is the same one.
func TestEmbedderEncode_WaitsOutCooldownStartedByAnotherWorker(t *testing.T) {
	freezeBackoff(t)
	driver := &flakyDriver{}
	emb := newFlakyEmbedder(driver)
	startCooldown(emb.quotaKey(), 120*time.Millisecond)
	defer resetCooldown()

	start := time.Now()
	if _, err := emb.Encode(t.Context(), []string{"a"}); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Fatalf("Encode returned after %v; expected it to wait out the active cooldown", elapsed)
	}
	if driver.calls != 1 {
		t.Fatalf("driver calls = %d, want 1", driver.calls)
	}
}

func TestEmbedderEncode_CooldownHonorsContextCancellation(t *testing.T) {
	freezeBackoff(t)
	driver := &flakyDriver{}
	emb := newFlakyEmbedder(driver)
	startCooldown(emb.quotaKey(), 5*time.Second)
	defer resetCooldown()

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if _, err := emb.Encode(ctx, []string{"a"}); err == nil {
		t.Fatal("expected the cancelled context to surface")
	}
	if driver.calls != 0 {
		t.Fatalf("driver calls = %d, want 0: the call must not be made during a cooldown", driver.calls)
	}
}

// The cooldown is scoped to one provider instance. A throttled credential (or
// endpoint) must not stall ingestion that embeds somewhere else - otherwise a
// single throttled provider would freeze the whole process.
func TestEmbedderEncode_CooldownDoesNotBlockOtherProviderInstances(t *testing.T) {
	freezeBackoff(t)
	throttled := newFlakyEmbedderWithConfig(
		&flakyDriver{},
		&models.APIConfig{ApiKey: strPtr("key-a"), BaseURL: strPtr("https://a.example")},
	)
	startCooldown(throttled.quotaKey(), 5*time.Second)
	defer resetCooldown()

	other := newFlakyEmbedderWithConfig(
		&flakyDriver{},
		&models.APIConfig{ApiKey: strPtr("key-b"), BaseURL: strPtr("https://b.example")},
	)
	start := time.Now()
	if _, err := other.Encode(t.Context(), []string{"a"}); err != nil {
		t.Fatalf("Encode on the other provider instance: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("a cooldown on provider A delayed provider B by %v", elapsed)
	}
}

// Conversely, two workers embedding through the same provider instance must
// land on one quota key: the resolver builds a fresh embedder per tokenizer
// invocation, so per-value state would never be shared between them.
func TestEmbedderQuotaKey_ScopesByProviderInstance(t *testing.T) {
	cfg := &models.APIConfig{ApiKey: strPtr("key-a"), BaseURL: strPtr("https://a.example")}
	first := newFlakyEmbedderWithConfig(&flakyDriver{}, cfg)
	second := newFlakyEmbedderWithConfig(&flakyDriver{}, cfg)
	if first.quotaKey() != second.quotaKey() {
		t.Fatal("two embedders on the same provider instance must share one quota key")
	}

	otherKey := newFlakyEmbedderWithConfig(
		&flakyDriver{},
		&models.APIConfig{ApiKey: strPtr("key-b"), BaseURL: strPtr("https://a.example")},
	)
	if first.quotaKey() == otherKey.quotaKey() {
		t.Fatal("a different credential owns a different quota and must not share the key")
	}

	otherEndpoint := newFlakyEmbedderWithConfig(
		&flakyDriver{},
		&models.APIConfig{ApiKey: strPtr("key-a"), BaseURL: strPtr("https://b.example")},
	)
	if first.quotaKey() == otherEndpoint.quotaKey() {
		t.Fatal("a different endpoint must not share the key")
	}

	// The key must not carry the secret itself.
	if key := string(first.quotaKey()); strings.Contains(key, "key-a") {
		t.Fatalf("quota key leaks the API key: %q", key)
	}
}

func TestEncodeBackoff_GrowsAndIsCapped(t *testing.T) {
	prevBase, prevMax, prevJitter := encodeBackoffBase, encodeBackoffMax, encodeBackoffJitter
	defer func() {
		encodeBackoffBase, encodeBackoffMax, encodeBackoffJitter = prevBase, prevMax, prevJitter
	}()
	encodeBackoffBase = 5 * time.Second
	encodeBackoffMax = 120 * time.Second
	encodeBackoffJitter = 0

	want := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second, 80 * time.Second, 120 * time.Second}
	for i, w := range want {
		if got := encodeBackoff(i+1, errors.New(tpmLimitErr)); got != w {
			t.Errorf("attempt %d backoff = %v, want %v", i+1, got, w)
		}
	}
	// The whole schedule has to outlast a one-minute TPM window, otherwise every
	// attempt lands inside the same exhausted quota.
	total := time.Duration(0)
	for i := range want {
		total += encodeBackoff(i+1, errors.New(tpmLimitErr))
	}
	if total < 2*time.Minute {
		t.Errorf("total backoff = %v, want >= 2m to span more than one TPM window", total)
	}
}

func TestEncodeBackoff_HonorsRetryAfter(t *testing.T) {
	prevMax := encodeBackoffMax
	defer func() { encodeBackoffMax = prevMax }()
	encodeBackoffMax = 120 * time.Second

	cases := []struct {
		msg  string
		want time.Duration
	}{
		{"SILICONFLOW API error: 429 Too Many Requests, retry-after: 30, body: {}", 30 * time.Second},
		{"429 retry_after=7", 7 * time.Second},
		{"429 too many requests, retry after 12 seconds", 12 * time.Second},
		{`429 {"retry-after": 45}`, 45 * time.Second},
	}
	for _, tc := range cases {
		if got := encodeBackoff(1, errors.New(tc.msg)); got != tc.want {
			t.Errorf("encodeBackoff(%q) = %v, want %v", tc.msg, got, tc.want)
		}
	}
	// A provider hint larger than the cap is clamped.
	if got := encodeBackoff(1, errors.New("429 retry-after: 3600")); got != encodeBackoffMax {
		t.Errorf("huge Retry-After = %v, want the %v cap", got, encodeBackoffMax)
	}
}

func TestIsRetryableErr(t *testing.T) {
	retryable := []string{
		tpmLimitErr,
		"SILICONFLOW API error: 429 Too Many Requests, body: {}",
		"provider says: too many requests",
		"rate limit exceeded",
		"TPM limit reached",
		// Provider-side failures: a 5xx and Anthropic's overloaded 529 mean "not
		// now", so they are retried like a 429.
		"API request failed with status 503: unavailable",
		"error: 529 overloaded",
		"500 Internal Server Error",
	}
	for _, msg := range retryable {
		if !isRetryableErr(errors.New(msg)) {
			t.Errorf("isRetryableErr(%q) = false, want true", msg)
		}
	}
	notRetryable := []string{
		"",
		"402 Payment Required",
		// An explicit status decides in both directions: a permanent 4xx forbids
		// the retry even when the body also mentions rate limits.
		"SILICONFLOW API error: 400 Bad Request, body: rate limit header malformed",
		// A bare three-digit number in a body is data, not a status.
		`{"usage":{"total_tokens":429}}`,
	}
	for _, msg := range notRetryable {
		if isRetryableErr(errors.New(msg)) {
			t.Errorf("isRetryableErr(%q) = true, want false", msg)
		}
	}
	// Structured status from the shared HTTP helpers, no message parsing at all.
	if !isRetryableErr(&models.APIStatusError{Status: 429, Body: "{}"}) {
		t.Error("a typed 429 must be retryable")
	}
	if isRetryableErr(&models.APIStatusError{Status: 400, Body: "bad input"}) {
		t.Error("a typed 400 must not be retryable")
	}
	if isRetryableErr(nil) {
		t.Error("isRetryableErr(nil) = true, want false")
	}
	// A net.Error is not automatically transient. An unresolvable host or a refused
	// connection is configuration the operator has to fix; retrying it spends six
	// attempts and the shared cooldown on an answer that cannot get better.
	netCases := []struct {
		name string
		err  error
		want bool
	}{
		{"dns NXDOMAIN", &net.DNSError{Err: "no such host", Name: "provider.invalid", IsNotFound: true}, false},
		{"dns temporary", &net.DNSError{Err: "server misbehaving", Name: "provider.example", IsTemporary: true}, true},
		{"timeout", &net.DNSError{Err: "i/o timeout", Name: "provider.example", IsTimeout: true}, true},
		{"connection refused", &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}, false},
		{"connection reset after accept", &net.OpError{Op: "read", Err: syscall.ECONNRESET}, true},
		{"wrapped dns NXDOMAIN", fmt.Errorf("embed: %w", &net.DNSError{Err: "no such host", IsNotFound: true}), false},
	}
	for _, tc := range netCases {
		if got := isRetryableErr(tc.err); got != tc.want {
			t.Errorf("isRetryableErr(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
}
