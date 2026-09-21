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

import (
	"errors"
	"strings"
	"testing"
)

// fakeCounter counts one unit per N runes, so tests can exercise the limit maths
// without depending on a shipped asset.
type fakeCounter struct {
	id        string
	perRune   int
	available bool
}

func (f fakeCounter) ID() string { return f.id }

func (f fakeCounter) Count(s string) int {
	if f.perRune <= 0 {
		return 0
	}
	return len([]rune(s)) * f.perRune
}

func (f fakeCounter) TrimToLimit(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	maxRunes := limit / f.perRune
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes])
}

func (f fakeCounter) Available() bool { return f.available }

func TestEmbeddingTokenLimitMargin(t *testing.T) {
	cases := []struct {
		maxTokens int
		want      int
	}{
		// 2% of 8192 = 163.84 -> 164, well above the 32-token floor.
		{8192, 8192 - 164},
		// 2% of 512 = 10.24 -> the 32-token floor wins.
		{512, 512 - 32},
		// Degenerate windows keep a usable budget instead of going negative.
		{32, 16},
		{2, 1},
		{0, 0},
	}
	for _, c := range cases {
		if got := EmbeddingTokenLimit(c.maxTokens); got != c.want {
			t.Errorf("EmbeddingTokenLimit(%d) = %d, want %d", c.maxTokens, got, c.want)
		}
	}
}

func TestResolveEmbeddingMaxTokens(t *testing.T) {
	cases := []struct {
		declared, contextLength, want int
	}{
		{8192, 32768, 8192}, // the explicit model value wins
		{0, 512, 512},       // catalog context_length, not a hard-coded 8192
		{0, 0, EmbeddingTokenLimitDefault},
	}
	for _, c := range cases {
		if got := ResolveEmbeddingMaxTokens(c.declared, c.contextLength); got != c.want {
			t.Errorf("ResolveEmbeddingMaxTokens(%d,%d) = %d, want %d", c.declared, c.contextLength, got, c.want)
		}
	}
}

func TestCalibrationRatchet(t *testing.T) {
	cal := NewCalibration(DefaultUncountedRatioUpper)
	key := "siliconflow|bge-m3"
	if got := cal.RatioUpper(key); got != DefaultUncountedRatioUpper {
		t.Fatalf("unobserved ratio = %v, want %v", got, DefaultUncountedRatioUpper)
	}
	// A successful call whose real count is lower than ours must not lower the
	// bound: it stays at the configured default. The calibration only ratchets
	// up, so one under-counting observation cannot replace the margin with an
	// estimate of its own.
	cal.ObserveUsage(key, 8182, 8027)
	if got := cal.RatioUpper(key); got != DefaultUncountedRatioUpper {
		t.Fatalf("ratio after an under-counting observation = %v, want the default %v", got, DefaultUncountedRatioUpper)
	}
	// A real count above ours ratchets the bound up.
	cal.ObserveUsage(key, 1000, 1030)
	if got := cal.RatioUpper(key); got < 1.03 {
		t.Fatalf("ratio after ObserveUsage = %v, want >= 1.03", got)
	}
	ratio, samples, rejects, ok := cal.Stats(key)
	if !ok || samples != 2 || rejects != 0 || ratio < 1.03 {
		t.Fatalf("Stats = (%v,%d,%d,%t), want ratio >= 1.03, samples 2, rejects 0, ok", ratio, samples, rejects, ok)
	}
	cal.Reset(key)
	if got := cal.RatioUpper(key); got != DefaultUncountedRatioUpper {
		t.Fatalf("ratio after Reset = %v, want the default", got)
	}
}

// TestCalibrationOverLimitInfersRatio is the 78785.md case: our counter scored a
// chunk at 8,143 tokens against an 8192-token window and the provider rejected it
// for being over the limit. The rejection itself proves the true count is above
// 8192, so the ratio bound must rise above 1 even though no usage was returned.
func TestCalibrationOverLimitInfersRatio(t *testing.T) {
	cal := NewCalibration(1.0)
	key := "siliconflow|bge-m3"
	cal.ObserveOverLimit(key, 8143, 8192)
	ratio := cal.RatioUpper(key)
	want := 8192.0 / 8143.0 * 1.01
	if ratio < want-1e-9 {
		t.Fatalf("ratio after an over-limit rejection = %v, want >= %v", ratio, want)
	}
	if _, _, rejects, _ := cal.Stats(key); rejects != 1 {
		t.Fatalf("limitRejects = %d, want 1", rejects)
	}
}

func TestLimiterLimitUsesRatioAndMargin(t *testing.T) {
	exact := NewExactLimiter(fakeCounter{id: "exact", perRune: 1, available: true})
	if got, want := exact.Limit(8192), EmbeddingTokenLimit(8192); got != want {
		t.Fatalf("exact Limit(8192) = %d, want %d", got, want)
	}

	cal := NewCalibration(1.0)
	key := "p|m"
	calibrated := NewCalibratedLimiter(fakeCounter{id: "approx", perRune: 1, available: true}, key, cal)
	if got := calibrated.Limit(8192); got != EmbeddingTokenLimit(8192) {
		t.Fatalf("ratio-1 calibrated Limit(8192) = %d, want %d", got, EmbeddingTokenLimit(8192))
	}

	// The limiter reads the calibration live: an over-limit rejection recorded
	// while it is in flight must tighten the very next Limit() call.
	cal.ObserveOverLimit(key, 8143, 8192)
	tightened := calibrated.Limit(8192)
	if tightened >= EmbeddingTokenLimit(8192) {
		t.Fatalf("Limit after an over-limit observation = %d, want less than %d", tightened, EmbeddingTokenLimit(8192))
	}
	budget := int(8192 / calibrated.Ratio())
	if tightened != EmbeddingTokenLimit(budget) {
		t.Fatalf("Limit = %d, want EmbeddingTokenLimit(%d) = %d", tightened, budget, EmbeddingTokenLimit(budget))
	}
}

func TestLimiterTrimRespectsLimit(t *testing.T) {
	counter := fakeCounter{id: "fake", perRune: 2, available: true}
	limiter := NewExactLimiter(counter)
	text := strings.Repeat("a", 10000)
	trimmed, tokens := limiter.Trim(text, 8192)
	limit := limiter.Limit(8192)
	if tokens > limit {
		t.Fatalf("Trim returned %d tokens, limit is %d", tokens, limit)
	}
	if got := counter.Count(trimmed); got > limit {
		t.Fatalf("counter reports %d tokens for the trimmed text, limit is %d", got, limit)
	}
	if len(trimmed) == len(text) {
		t.Fatalf("expected the text to be cut, got the whole %d-byte input", len(text))
	}
}

// TestLimiterWithoutUsableCounter covers the case where the cl100k table is
// missing: the byte-level bound must still cut the text and must never report a
// token count that was invented from a dead encoder.
func TestLimiterWithoutUsableCounter(t *testing.T) {
	limiter := NewExactLimiter(fakeCounter{id: "dead", perRune: 1, available: false})
	text := strings.Repeat("x", 1_000_000)
	trimmed, tokens := limiter.Trim(text, 8192)
	limit := limiter.Limit(8192)
	// The fallback bounds BYTES by the token limit: one byte per token is the
	// only budget that cannot exceed it.
	if len(trimmed) > limit {
		t.Fatalf("byte-level fallback kept %d bytes, want <= %d", len(trimmed), limit)
	}
	if tokens != limit {
		t.Fatalf("fallback token count = %d, want the limit %d", tokens, limit)
	}
	if strings.ContainsRune(trimmed, '\uFFFD') {
		t.Fatal("byte-level fallback split a multi-byte rune")
	}
}

func TestTrimByBytesKeepsRunesIntact(t *testing.T) {
	text := strings.Repeat("中", 100) // 300 bytes
	got := trimByBytes(text, 10)     // 10-byte bound
	if len(got) > 10 {
		t.Fatalf("trimByBytes kept %d bytes, want <= 10", len(got))
	}
	if !strings.HasPrefix(text, got) {
		t.Fatal("trimByBytes did not return a prefix")
	}
	for _, r := range got {
		if r == '\uFFFD' {
			t.Fatal("trimByBytes cut a rune in half")
		}
	}
}

func TestResolveCounterFallsBackForUnknownID(t *testing.T) {
	// Loading cl100k here is fine: failfast_test.go re-execs itself in a fresh
	// process precisely so that tiktoken-go's process-global encoding cache
	// cannot make that test order-dependent.
	defer resetCL100KEncoderForTest()
	for _, id := range []string{"", "not-a-tokenizer", CounterXLMRSentence} {
		c := ResolveCounter(id)
		if c == nil {
			t.Fatalf("ResolveCounter(%q) returned nil", id)
		}
		// Whether cl100k's table is present in this environment or not, the
		// resolved counter must be safe to call and must trim without panicking.
		text := strings.Repeat("word ", 5000)
		trimmed := c.TrimToLimit(text, 100)
		if !strings.HasPrefix(text, trimmed) {
			t.Fatalf("ResolveCounter(%q) did not return a prefix", id)
		}
		if strings.ContainsRune(trimmed, '\uFFFD') {
			t.Fatalf("ResolveCounter(%q) split a rune while trimming", id)
		}
		if c.Available() {
			if got := c.Count(trimmed); got > 100 {
				t.Fatalf("ResolveCounter(%q) kept %d tokens for a 100-token limit", id, got)
			}
			continue
		}
		if len(trimmed) > 100*4 {
			t.Fatalf("ResolveCounter(%q) kept %d bytes for a 100-token limit with no counter available", id, len(trimmed))
		}
	}
}

// TestCL100KTrimNeverExceedsLimit is the property every counter must hold: the
// trimmed text fits the limit according to the counter that produced it.
func TestCL100KTrimNeverExceedsLimit(t *testing.T) {
	defer resetCL100KEncoderForTest()
	counter := CountCL100K()
	if !counter.Available() {
		t.Skip("cl100k table not present in this environment")
	}
	samples := []string{
		strings.Repeat("hello world ", 500),
		strings.Repeat("| 1976 | | 383/1 | 383/2 | 383/3 |\n", 200),
		strings.Repeat("中", 2000),
		strings.Repeat("QWxhZGRpbjpvcGVuIHNlc2FtZQ", 200),
	}
	for _, limit := range []int{1, 50, 512, 8028} {
		for i, s := range samples {
			trimmed := counter.TrimToLimit(s, limit)
			if got := counter.Count(trimmed); got > limit {
				t.Errorf("sample %d limit %d: trimmed text counts %d tokens", i, limit, got)
			}
		}
	}
}

// TestIsOverLimitErrorMatchesDelimitedNumbers pins the difference between a substring
// test and a delimited one. The caller acts on this answer by re-embedding a
// truncated input, so a false positive silently replaces a real error with a
// window-limit one; 120015 merely contains 20015, and 1400 merely contains 400.
func TestIsOverLimitErrorMatchesDelimitedNumbers(t *testing.T) {
	cases := []struct {
		name string
		err  string
		want bool
	}{
		{
			"siliconflow over-window",
			`SILICONFLOW API error: 400 Bad Request, body: {"code":20015,"message":"The parameter is invalid. Please check again.","data":null}`,
			true,
		},
		{
			"siliconflow code as a string",
			`SILICONFLOW API error: 400 Bad Request, body: {"code":"20015"}`,
			true,
		},
		{
			"openai wording",
			`OpenAI embeddings API error: 400 Bad Request, body: {"error":{"message":"This model's maximum context length is 8192 tokens"}}`,
			true,
		},
		{"413", `API error: 413 Request Entity Too Large, body: input is too long`, true},
		{"422", `API error: 422 Unprocessable Entity, body: too many tokens`, true},
		// Delimiter neighbours: a longer provider code that merely contains 20015, and
		// a status that merely contains 400.
		{"code 120015", `SILICONFLOW API error: 400 Bad Request, body: {"code":120015}`, false},
		{"code 200150", `SILICONFLOW API error: 400 Bad Request, body: {"code":200150}`, false},
		{"status 1400", `API error: 1400 Bad Request, body: too long`, false},
		// Rate limits and provider failures must not be mistaken for size.
		{"rate limit", `SILICONFLOW API error: 429 Too Many Requests, body: {"message":"Request was rejected due to rate limiting. Details: TPM limit reached."}`, false},
		{"server error", `SILICONFLOW API error: 500 Internal Server Error, body: too long`, false},
		{"unauthorized", `OpenAI embeddings API error: 401 Unauthorized, body: invalid api key`, false},
		{"network", `failed to send request: dial tcp: connection refused`, false},
	}
	for _, c := range cases {
		if got := IsOverLimitError(errors.New(c.err)); got != c.want {
			t.Errorf("%s: IsOverLimitError = %t, want %t (%s)", c.name, got, c.want, c.err)
		}
	}
	if IsOverLimitError(nil) {
		t.Error("IsOverLimitError(nil) = true, want false")
	}
}
