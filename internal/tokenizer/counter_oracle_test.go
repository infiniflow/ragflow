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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// This file is the offline half of the verification plan in
// internal/tokenizer/embedding_token_limits.md: every hand-written counter must
// agree, sample by sample, with the tokenizer the model is actually served with.
//
// Fixtures come from scripts/gen_tokenizer_oracle.py, which tokenizes a fixed
// corpus with the HuggingFace `tokenizers` library reading the model's own
// tokenizer.json:
//
//	mkdir -p /tmp/tokenizer_oracle
//	for m in bge-m3 bge-large-en qwen3-embedding e5-mistral; do
//	  uv run scripts/gen_tokenizer_oracle.py --model $m --corpus <path> --out /tmp/tokenizer_oracle/$m.json
//	done
//	TOKENIZER_ORACLE_DIR=/tmp/tokenizer_oracle ./build.sh --test -run TestCountersMatchOracle ./internal/tokenizer/
//
// The test reads every fixture in the directory, maps fixture -> counter id via
// the fixture's own "counter" field, and fails on any count mismatch: a
// hand-written tokenizer that is "close" is not good enough, because an
// under-count lets an oversized input through and the provider answers 400.

type oracleFixture struct {
	Model      string `json:"model"`
	Counter    string `json:"counter"`
	Oracle     string `json:"oracle"`
	Reference  string `json:"reference"`
	Crosscheck string `json:"crosscheck"`
	Samples    []struct {
		Label       string   `json:"label"`
		Text        string   `json:"text"`
		Count       int      `json:"count"`
		Chars       int      `json:"chars"`
		IDs         []int32  `json:"ids"`
		Tokens      []string `json:"tokens"`
		KnownApprox bool     `json:"known_approx"`
	} `json:"samples"`
}

// idCounter is implemented by the counters that can report the exact token ids. The
// oracle test uses it to compare segmentations; the Count interface alone only
// supports comparing their lengths, which two different segmentations can share.
type idCounter interface {
	IDs(text string) []int32
}

// nameCounter is implemented by the counters that can report the token texts. The
// test prefers them over ids because a token id is only meaningful inside one
// numbering: the HuggingFace conversion of bge-m3 reorders the SentencePiece
// specials and inserts <pad>/<mask>, so every regular piece is +1 relative to the
// .model file while the tokens themselves are identical.
type nameCounter interface {
	Names(text string) []string
}

func TestCountersMatchOracle(t *testing.T) {
	dir := os.Getenv("TOKENIZER_ORACLE_DIR")
	if dir == "" {
		t.Skip("set TOKENIZER_ORACLE_DIR to a directory of fixtures from scripts/gen_tokenizer_oracle.py")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the oracle directory %s: %v", dir, err)
	}
	seen := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading the oracle fixture %s: %v", path, err)
		}
		var fx oracleFixture
		if err := json.Unmarshal(raw, &fx); err != nil {
			t.Fatalf("parsing the oracle fixture %s: %v", path, err)
		}
		seen++
		t.Run(fx.Counter, func(t *testing.T) { checkCounterAgainstOracle(t, fx) })
	}
	if seen == 0 {
		t.Fatalf("no fixtures found in %s", dir)
	}
}

func checkCounterAgainstOracle(t *testing.T, fx oracleFixture) {
	counter, ok := CounterByID(fx.Counter)
	if !ok {
		// A fixture names the counter it was generated for, so a miss cannot be a
		// reason to pass. Distinguish the two causes, because the fix differs:
		// either this build does not have that counter at all, or it has it and the
		// asset it needs is not on disk.
		for _, status := range CounterStatuses() {
			if status.ID == fx.Counter {
				t.Fatalf("fixture %q needs counter %q, which is declared but unavailable: its asset is missing from ragflow_deps/huggingface.co (run `uv run ragflow_deps/download_go_deps.py`)", fx.Model, fx.Counter)
			}
		}
		t.Fatalf("fixture %q names counter %q, which this build does not implement", fx.Model, fx.Counter)
	}
	ids, haveIDs := counter.(idCounter)
	names, haveNames := counter.(nameCounter)
	var (
		corpus       string
		corpusOracle int
		mismatches   int
		idCompared   int
		nameCompared int
		// Per sample, not per check: one sample is examined by both the count
		// comparison and the token-text comparison, and the agreement line at the
		// end must not subtract it twice.
		approximated = map[string]bool{}
	)
	for _, sample := range fx.Samples {
		got := counter.Count(sample.Text)
		if got != sample.Count {
			// The one documented approximation (two or more combining marks on one
			// base character) may move the count by a token or two; the fixture
			// marks those samples explicitly. Everything else must be exact.
			if sample.KnownApprox && abs(got-sample.Count) <= knownApproxTokenTolerance {
				approximated[sample.Label] = true
			} else {
				mismatches++
				t.Errorf("%s: our count=%d, %s oracle=%d (chars=%d)", sample.Label, got, fx.Model, sample.Count, sample.Chars)
			}
		}
		if sample.Label == corpusSampleLabel {
			corpus, corpusOracle = sample.Text, sample.Count
		}
		if len(sample.IDs) != sample.Count {
			mismatches++
			t.Errorf("%s: fixture is self-inconsistent: %d ids but count=%d", sample.Label, len(sample.IDs), sample.Count)
			continue
		}
		// Our own count and our own segmentation must agree; if they ever do not,
		// one of the two paths inside the counter has drifted from the other.
		checkOwnAgreement := func(t *testing.T, label string, got int, n int) bool {
			if n != got {
				t.Helper()
				mismatches++
				t.Errorf("%s: segmentation has %d tokens but count=%d inside our own counter", label, n, got)
				return false
			}
			return true
		}
		if haveNames && len(sample.Tokens) > 0 {
			ours := names.Names(sample.Text)
			nameCompared++
			if !checkOwnAgreement(t, sample.Label, got, len(ours)) {
				continue
			}
			// A base character with two or more combining marks is the one shape
			// where our NFKC-based normalization provably differs from the served
			// Precompiled charmap (see the fixture's known_approx flag, set by
			// scripts/gen_tokenizer_oracle.py). The divergence is bounded here
			// rather than excused: anything larger, or any divergence at all on a
			// sample without that shape, still fails.
			if sample.KnownApprox {
				if delta := len(ours) - len(sample.Tokens); delta > knownApproxTokenTolerance || delta < -knownApproxTokenTolerance {
					mismatches++
					t.Errorf("%s: known-approximation sample differs by %d tokens (tolerance %d): ours=%d oracle=%d",
						sample.Label, delta, knownApproxTokenTolerance, len(ours), len(sample.Tokens))
				} else {
					approximated[sample.Label] = true
				}
				continue
			}
			if at, ok := firstDivergence(ours, sample.Tokens); ok {
				mismatches++
				t.Errorf("%s: tokens diverge at %d of %d\n  ours   %s\n  oracle %s\n  text   %s",
					sample.Label, at, len(sample.Tokens), describeWindow(ours, at), describeWindow(sample.Tokens, at), headRunes(sample.Text, 120))
			}
			continue
		}
		if !haveIDs || len(sample.IDs) == 0 {
			continue
		}
		ours := ids.IDs(sample.Text)
		idCompared++
		if !checkOwnAgreement(t, sample.Label, got, len(ours)) {
			continue
		}
		if at, ok := firstDivergence(ours, sample.IDs); ok {
			mismatches++
			t.Errorf("%s: token ids diverge at %d of %d\n  ours   %s\n  oracle %s\n  text   %s",
				sample.Label, at, len(sample.IDs), describeWindow(ours, at), describeWindow(sample.IDs, at), headRunes(sample.Text, 120))
		}
	}
	mode := "counts"
	switch {
	case nameCompared > 0:
		mode = "token texts"
	case idCompared > 0:
		mode = "id sequences"
	}
	t.Logf("agreement: %d/%d samples compared by %s (oracle: %s)", len(fx.Samples)-mismatches-len(approximated), len(fx.Samples), mode, fx.Oracle)
	if len(approximated) > 0 {
		t.Logf("within the documented approximation on %d/%d samples (combining marks in a shape the served charmap normalizes differently - see the known_approx flag in scripts/gen_tokenizer_oracle.py - tolerance %d tokens)",
			len(approximated), len(fx.Samples), knownApproxTokenTolerance)
	}
	if fx.Crosscheck != "" {
		t.Logf("second reference: %s", fx.Crosscheck)
	}

	// The corpus, pinned as a test. The document passed in with --corpus is the one
	// the incident failed on, and its whole content is sampled - so what can be
	// pinned here is the window: the model's tokenizer sees MORE than the
	// 8192-token limit, which is why this document has to take the exact-count path
	// rather than a coarser one. The under-count that actually triggered 400/20015
	// lived in a 25k-character span of this document (cl100k below the limit while
	// the model was above it); that shape belongs to the span, not to the document,
	// and a whole-document sample cannot reproduce it - pass a corpus file cut to
	// that span to pin it. The counters' agreement with their oracles on the whole
	// document is what the loop above checks, token by token.
	if fx.Counter != CounterXLMRSentence {
		return
	}
	if corpus == "" {
		t.Errorf("fixture has no %q sample: regenerate it with --corpus <document> (scripts/gen_tokenizer_oracle.py)", corpusSampleLabel)
		return
	}
	if corpusOracle <= 8192 {
		t.Errorf("the corpus document counts %d tokens in %s, expected more than the 8192-token window", corpusOracle, fx.Model)
	}
	if cl100k := CountCL100K(); cl100k.Available() {
		t.Logf("corpus document: cl100k=%d vs %s=%d", cl100k.Count(corpus), fx.Model, corpusOracle)
	}
}

// knownApproxTokenTolerance bounds the divergence allowed on the samples the fixture
// marks as the documented normalization approximation (a base character carrying two
// or more combining marks, where NFKC composes and the served charmap does not).
const knownApproxTokenTolerance = 2

// corpusSampleLabel is the fixture sample holding the whole document passed to
// scripts/gen_tokenizer_oracle.py --corpus. The incident check at the end of
// checkCounterAgainstOracle pins that sample, so a fixture generated without a
// corpus (the flag is required) cannot quietly drop the guarantee.
const corpusSampleLabel = "incident_full"

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// firstDivergence reports where two token sequences stop agreeing (including a pure
// length difference, which is a divergence at the shorter end).
func firstDivergence[T comparable](ours, oracle []T) (int, bool) {
	for i := range ours {
		if i >= len(oracle) || ours[i] != oracle[i] {
			return i, true
		}
	}
	if len(ours) != len(oracle) {
		return len(ours), true
	}
	return 0, false
}

// describeWindow renders a small window of tokens around a divergence; the marker
// points at the first token that differs.
func describeWindow[T any](values []T, at int) string {
	lo, hi := at-3, at+4
	if lo < 0 {
		lo = 0
	}
	if hi > len(values) {
		hi = len(values)
	}
	var b strings.Builder
	b.WriteString("[")
	for i := lo; i < hi; i++ {
		if i > lo {
			b.WriteString(" ")
		}
		if i == at {
			b.WriteString("»")
		}
		fmt.Fprintf(&b, "%v", values[i])
	}
	b.WriteString("]")
	return b.String()
}

// headRunes quotes the first n runes of text, for context in a failure message.
func headRunes(text string, n int) string {
	runes := []rune(text)
	if len(runes) <= n {
		return strconv.Quote(string(runes))
	}
	return strconv.Quote(string(runes[:n])) + "…"
}
