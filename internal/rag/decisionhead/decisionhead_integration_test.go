//go:build cgo && integration

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

// Integration tier: these run against the real fp32 checkpoint and the real
// mmBERT tokenizer, which live in the deploy bundle (RAGFLOW_DECISION_DIR), not in
// the repo.
//
// There is deliberately no logits parity test. The training side's
// deploy_golden_examples.json carries expected input_ids/marker_pos and no
// expected logits, so a maxdiff gate has nothing to compare against. What is
// asserted instead is what the deployment actually depends on:
//
//	1. the production assembly (BuildSequence) reproduces the contract's
//	   expected sequence token for token — a one-token drift moves every marker
//	   and changes the decision silently (DEPLOY_CONTRACT.md §七);
//	2. the model ranks a chunk that answers the question above one that does
//	   not — the ordering the filter actually depends on.
//
// The absolute probability is a calibration question, not a correctness one: a
// chunk that answers the question can legitimately sit below the threshold if
// the threshold is wrong, and tuning that threshold is what the prototype is
// for. So the probability is logged and only its ORDER is asserted.
//
// Every expectation here is hand-labelled text, not a model output copied back
// into the test.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"ragflow/internal/tokenizer"
)

// decisionheadDeployDir resolves the deploy bundle from RAGFLOW_DECISION_DIR.
//
// There is deliberately no default: a built-in path would only work on the
// machine that wrote it, and CI has no bundle at all. An unset variable means
// "this checkout has no decision-head model", which the callers turn into a
// skip rather than a failure.
func decisionheadDeployDir() string {
	return strings.TrimSpace(os.Getenv("RAGFLOW_DECISION_DIR"))
}

type goldenStats struct {
	HeadTokens   int `json:"head_tokens"`
	StateTokens  int `json:"state_tokens"`
	TotalTokens  int `json:"total_tokens"`
	NumOptions   int `json:"num_options"`
	OptionTokens int `json:"option_tokens"`
}

type goldenCase struct {
	Lang         string      `json:"lang"`
	Qtype        string      `json:"qtype"`
	Instructions string      `json:"instructions"`
	OptionTexts  []string    `json:"option_texts"`
	State        string      `json:"state"`
	InputIDs     []int32     `json:"input_ids"`
	MarkerPos    []int32     `json:"marker_pos"`
	Stats        goldenStats `json:"stats"`
}

type goldenFile struct {
	HeadFormat string           `json:"head_format"`
	MaskToken  string           `json:"mask_token"`
	Specials   map[string]int32 `json:"specials"`
	Params     struct {
		MaxLen         int `json:"max_len"`
		HeadMaxLen     int `json:"head_max_len"`
		OptionTokenCap int `json:"option_token_cap"`
	} `json:"params"`
	Cases []goldenCase `json:"cases"`
}

func loadGolden(t *testing.T, dir string) goldenFile {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "deploy_golden_examples.json"))
	if err != nil {
		t.Skipf("golden examples not found in the deploy bundle: %v", err)
	}
	var golden goldenFile
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatalf("parse golden: %v", err)
	}
	if len(golden.Cases) == 0 {
		t.Fatal("golden carries no cases")
	}
	return golden
}

// loadRealJudge loads the bundle's contract, tokenizer and ONNX session exactly
// as the server does.
func loadRealJudge(t *testing.T) *judge {
	t.Helper()
	cfg := ConfigFromEnv()
	if !cfg.Enabled() {
		cfg.Dir = decisionheadDeployDir()
	}
	if !cfg.Available() {
		t.Skipf("decision-head deploy bundle not usable at %s", cfg.Dir)
	}
	j, err := open(cfg)
	if err != nil {
		// A checkout without the runtime or the converted model cannot exercise
		// this path, and failing here would make every integration run red on
		// machines that never installed the bundle. Both shapes of "missing" are
		// skipped; anything else is still a failure, because it means the bundle
		// IS present and the head does not work.
		//
		// The ONNX Runtime linked into this binary is an ORT-format-only build: it
		// rejects every .onnx file, including the binding's own fixtures, so the
		// bundle has to ship laya.ort.
		msg := err.Error()
		if strings.Contains(msg, "ONNX format model is not supported") ||
			strings.Contains(msg, "cannot open shared object file") ||
			strings.Contains(msg, "no such file or directory") {
			t.Skipf("decision-head model not loadable here (%s): %v", cfg.Dir, err)
		}
		t.Fatalf("open(%s): %v", cfg.Dir, err)
	}
	t.Cleanup(j.Close)
	return j
}

// goldenContract overlays the golden's own assembly budget and special tokens on
// the bundle's contract. The golden was produced with a smaller max_len than the
// deployed one, so reading the budget from anywhere but the golden could make a
// longer case diverge for the wrong reason.
func goldenContract(t *testing.T, golden goldenFile) (*Contract, string) {
	t.Helper()
	dir := decisionheadDeployDir()
	contract, err := LoadContract(filepath.Join(dir, "onnx", "laya_config.json"))
	if err != nil {
		t.Fatalf("load contract: %v", err)
	}
	contract.MaxLen = golden.Params.MaxLen
	contract.HeadMaxLen = golden.Params.HeadMaxLen
	contract.OptionTokenCap = golden.Params.OptionTokenCap
	if len(golden.Specials) > 0 {
		contract.SpecialIDs = golden.Specials
	}
	if golden.MaskToken != "" {
		contract.MaskToken = golden.MaskToken
	}
	if golden.HeadFormat != "" {
		contract.HeadFormat = golden.HeadFormat
	}
	return contract, filepath.Join(dir, "onnx", "tokenizer", "tokenizer.json")
}

// TestDecisionheadAssemblyMatchesContract is the parity gate the deploy bundle can
// actually support: the production assembly must reproduce the contract's
// expected id sequence and marker positions token for token.
func TestDecisionheadAssemblyMatchesContract(t *testing.T) {
	golden := loadGolden(t, decisionheadDeployDir())
	contract, tokenizerPath := goldenContract(t, golden)
	enc, err := tokenizer.LoadMetaspaceBPE(tokenizerPath)
	if err != nil {
		t.Fatalf("load tokenizer: %v", err)
	}
	for _, c := range golden.Cases {
		t.Run(c.Lang+"/"+c.Qtype, func(t *testing.T) {
			seq, err := BuildSequence(enc, contract, c.Qtype, c.Instructions, c.OptionTexts, c.State)
			if err != nil {
				t.Fatalf("BuildSequence: %v", err)
			}
			if !reflect.DeepEqual(seq.IDs, c.InputIDs) {
				t.Fatalf("input_ids mismatch:\n got  %v\n want %v", seq.IDs, c.InputIDs)
			}
			if !reflect.DeepEqual(seq.Markers, c.MarkerPos) {
				t.Fatalf("marker_pos = %v, want %v", seq.Markers, c.MarkerPos)
			}
		})
	}
}

// TestDecisionheadRanksAnsweringAboveUnrelated is the decision gate: on a question whose
// answer is unambiguous, the chunk that carries the answer must score above the
// chunk that does not.
//
// The labels are hand-written text, so a wrong ordering is the model's or the
// assembly's, never the fixture's.
func TestDecisionheadRanksAnsweringAboveUnrelated(t *testing.T) {
	j := loadRealJudge(t)
	cases := []struct {
		name string
		// enforce is false where the checkpoint is known not to support the
		// language yet: the numbers are still measured and logged, but a
		// noisy ordering there says nothing about this code.
		enforce   bool
		question  string
		answering string
		unrelated string
	}{
		{
			name:      "en",
			enforce:   true,
			question:  "Who is Sam Bankman-Fried?",
			answering: "Sam Bankman-Fried is an American entrepreneur who founded the cryptocurrency exchange FTX in 2019.",
			unrelated: "This survey reviews prior work on retrieval-augmented generation and its evaluation benchmarks.",
		},
		{
			// The checkpoint does not respond to Chinese EVIDENCE: holding the
			// question, the filler and the length fixed and only adding the
			// answering sentence moves the English pair by 0.72 (0.063 -> 0.778)
			// and the Chinese pair by 0.014 (0.054 -> 0.068). Routing, assembly
			// and options were all verified separately, and swapping in an
			// English instruction or the training side's own rendering leaves
			// the Chinese probability at ~0.056, so this is the model, not this
			// code. Asserting it today would be asserting noise.
			name:      "zh",
			enforce:   false,
			question:  "国家法定节假日共多少天？",
			answering: "现在法定假期是元旦1天，春节3天，清明1天，劳动节1天，端午1天，中秋1天，国庆3天，共计11天。",
			unrelated: "本综述回顾了检索增强生成的既有工作，并对比了若干评测基准的设计。",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			yes := j.decide(ctx, tc.question, []string{tc.answering})[0]
			no := j.decide(ctx, tc.question, []string{tc.unrelated})[0]
			t.Logf("P(sufficient): answering=%.4f (%d tokens, %s) unrelated=%.4f (%d tokens, %s)",
				yes.Probability, yes.Tokens, yes.Latency, no.Probability, no.Tokens, no.Latency)
			t.Logf("threshold=%.2f -> answering kept=%v, unrelated kept=%v",
				j.cfg.MinProbability, yes.Useful, no.Useful)
			if !yes.Judged || !no.Judged {
				t.Fatalf("a segment was not judged: answering=%+v unrelated=%+v", yes, no)
			}
			if yes.Probability <= no.Probability && tc.enforce {
				t.Errorf("answering chunk (%.4f) did not outrank the unrelated one (%.4f)",
					yes.Probability, no.Probability)
			}
		})
	}
}

// TestDecisionheadSegmentTiming records the cost of the two segment sizes the filter
// budget is sized from. It logs rather than asserts: the numbers belong in a
// report, not in a threshold that rots.
func TestDecisionheadSegmentTiming(t *testing.T) {
	j := loadRealJudge(t)
	unit := "The quick brown fox jumps over the lazy dog. "
	perUnit := j.enc.Count(unit)
	for _, target := range []int{512, 2048} {
		reps := (target + perUnit - 1) / perUnit
		chunk := strings.Repeat(unit, reps)
		start := time.Now()
		verdicts := j.decide(context.Background(), "What does the quick brown fox do?", []string{chunk})
		elapsed := time.Since(start)
		v := verdicts[0]
		t.Logf("segment: %d tokens (requested %d), one forward pass %s, wall %s, judged=%v P(sufficient)=%.4f",
			v.Tokens, target, v.Latency, elapsed, v.Judged, v.Probability)
		if !v.Judged {
			t.Fatalf("segment of %d tokens was not judged", v.Tokens)
		}
	}
}
