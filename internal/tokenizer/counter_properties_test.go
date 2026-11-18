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

// Counter property tests: the invariants every Counter must hold, checked on
// every counter whose asset is present in this environment.
//
// These are deliberately separate from TestCountersMatchOracle. The oracle test
// answers "is this the model's tokenizer?" and needs the 17 MB oracle fixtures;
// this file answers "is this a *well-behaved* counter?" and needs only the asset
// the counter itself loads. Both are required: a counter can match the oracle on
// 35 samples and still violate Count(TrimToLimit(t, n)) <= n for an input nobody
// sampled, and the ingest path relies on that inequality to stay under the
// provider's window.
//
// If a counter is skipped here, its asset is missing - not its correctness.

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// propertyCorpus is one text per content class. The classes are the ones that
// have produced real bugs (see embedding_token_limits.md "one bug, one sample
// class"): prose, table rows, digits, CJK, emoji, base64-like, a long run of one
// character, and whitespace-heavy text.
func propertyCorpus() map[string]string {
	return map[string]string{
		"prose":        strings.Repeat("The quick brown fox jumps over the lazy dog. ", 60),
		"table_rows":   strings.Repeat("| 1976 | | 383/1 | 383/2 | 383/3 | 383/4 |\n", 40),
		"digits":       strings.Repeat("0123456789 ", 300),
		"cjk":          strings.Repeat("中文分词测试，用于对比 tokenizer。", 40),
		"emoji":        strings.Repeat("🚀🔥 embedding ✅ 测试 ", 40),
		"base64_like":  strings.Repeat("QWxhZGRpbjpvcGVuIHNlc2FtZQ", 100),
		"one_char_run": strings.Repeat("a", 2000),
		"whitespace":   "   padded   text \t with \n\n edge   whitespace   ",
		"accents":      "e\u0301 a\u0300 o\u0308 café naïve résumé ",
		"long_word":    strings.Repeat("supercalifragilisticexpialidocious", 20),
	}
}

// propertyLimits are the budgets the ingest path realistically uses: the floor of
// the shrink ladder, small windows (the catalog has 512-token models), and a full
// 8192-token window minus the margin.
func propertyLimits() []int { return []int{1, 8, 32, 64, 512, 2048, 8028} }

// TestCountersSatisfyTrimProperties holds every available counter to the same
// contract. Each property maps to something the ingest path depends on:
//
//	prefix          - L3 re-trims an already trimmed text, so trimming must be a
//	                  prefix operation (never reorder, never re-add text)
//	rune boundary   - a trimmed chunk must remain valid UTF-8
//	count <= limit  - the whole point: fit the model's window
//	idempotence     - the ladder and the migration path trim repeatedly
//	monotone        - a larger limit must never yield less text (the trim
//	                  implementations binary-search prefixes, which assumes this)
func TestCountersSatisfyTrimProperties(t *testing.T) {
	ids := []string{CounterCL100K, CounterXLMRSentence, CounterBERTWordPiece, CounterQwenBPE, CounterLlamaBPE}
	corpus, limits := propertyCorpus(), propertyLimits()
	ran := make([]string, 0, len(ids))
	for _, id := range ids {
		counter, ok := CounterByID(id)
		if !ok {
			t.Logf("%s: skipped (asset not present in this environment)", id)
			continue
		}
		ran = append(ran, id)
		t.Run(id, func(t *testing.T) {
			// An empty text is zero tokens for every family. This is not
			// academic: the SPM counter returned 1 here before the dummy-prefix
			// rule was fixed.
			if got := counter.Count(""); got != 0 {
				t.Errorf("Count(\"\") = %d, want 0", got)
			}
			if got := counter.TrimToLimit("anything at all", 0); got != "" {
				t.Errorf("TrimToLimit(text, 0) = %q, want empty", got)
			}
			for name, text := range corpus {
				prevCount, prevLen := -1, -1
				for _, limit := range limits {
					trimmed := counter.TrimToLimit(text, limit)
					if !strings.HasPrefix(text, trimmed) {
						t.Fatalf("%s limit=%d: result is not a prefix of the input", name, limit)
					}
					if !utf8.ValidString(trimmed) || strings.ContainsRune(trimmed, utf8.RuneError) {
						t.Fatalf("%s limit=%d: result is not valid UTF-8", name, limit)
					}
					count := counter.Count(trimmed)
					if count > limit {
						t.Fatalf("%s limit=%d: trimmed text still counts %d tokens", name, limit, count)
					}
					if again := counter.TrimToLimit(trimmed, limit); again != trimmed {
						t.Fatalf("%s limit=%d: trimming is not idempotent (%d vs %d bytes)", name, limit, len(trimmed), len(again))
					}
					if count < prevCount || len(trimmed) < prevLen {
						t.Fatalf("%s limit=%d: a larger limit produced less text (%d tokens/len %d after %d/%d)",
							name, limit, count, len(trimmed), prevCount, prevLen)
					}
					prevCount, prevLen = count, len(trimmed)
				}
			}
		})
	}
	t.Logf("counters exercised: %v", ran)
}

// TestCountTrimFitsEveryLimit is the ingest-path-shaped version of the property
// above: take a chunk that is far too long, ask each counter to fit it into the
// same window the embedder would use, and assert the result fits.
func TestCountTrimFitsEveryLimit(t *testing.T) {
	text := strings.Repeat("| 1976 | | 383/1 | 383/2 | 383/3 | 383/4 | 383/5 |\n", 500)
	window := EmbeddingTokenLimit(8192)
	for _, id := range []string{CounterCL100K, CounterXLMRSentence, CounterBERTWordPiece, CounterQwenBPE, CounterLlamaBPE} {
		counter, ok := CounterByID(id)
		if !ok {
			continue
		}
		trimmed := counter.TrimToLimit(text, window)
		if got := counter.Count(trimmed); got > window {
			t.Errorf("%s: %d tokens after trimming to %d", id, got, window)
		}
		if len(trimmed) == 0 {
			t.Errorf("%s: trimmed the whole chunk away", id)
		}
	}
}
