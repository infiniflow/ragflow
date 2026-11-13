//go:build integration

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

package tokenizer

// Golden parity against the real mmBERT tokenizer shipped with the decision-head deploy
// package. This is the authoritative gate for the Metaspace flavor: the ONNX
// decision model consumes these exact ids, and marker positions are absolute
// indices into them, so a one-token drift moves every marker silently
// (DEPLOY_CONTRACT.md §七). The expectations come from
// deploy_golden_examples.json, produced by the training side.
//
// The tokenizer.json is ~34 MB and ships inside the model directory, not in the
// repo, so this test is integration-tier and resolves the path from
// RAGFLOW_DECISION_DIR (default: the workspace deploy package).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type goldenCase struct {
	Lang        string   `json:"lang"`
	Qtype       string   `json:"qtype"`
	HeadText    string   `json:"head_text"`
	OptionTexts []string `json:"option_texts"`
	State       string   `json:"state"`
	InputIDs    []int32  `json:"input_ids"`
	MarkerPos   []int32  `json:"marker_pos"`
	Stats       struct {
		HeadTokens   int `json:"head_tokens"`
		OptionTokens int `json:"option_tokens"`
		StateTokens  int `json:"state_tokens"`
		TotalTokens  int `json:"total_tokens"`
		NumOptions   int `json:"num_options"`
	} `json:"stats"`
}

type goldenFile struct {
	Specials map[string]int32 `json:"specials"`
	Cases    []goldenCase     `json:"cases"`
}

// decisionheadModelDir is the deploy bundle the golden comparison re-tokenizes.
//
// No default: a built-in path only works on the machine that wrote it, and CI
// has no bundle. Unset means the test skips.
func decisionheadModelDir() string {
	return os.Getenv("RAGFLOW_DECISION_DIR")
}

func TestMetaspaceGoldenParity(t *testing.T) {
	dir := decisionheadModelDir()
	tokenizerPath := filepath.Join(dir, "onnx", "tokenizer", "tokenizer.json")
	if _, err := os.Stat(tokenizerPath); err != nil {
		t.Skipf("Laya tokenizer not found at %s (set RAGFLOW_DECISION_DIR): %v", tokenizerPath, err)
	}
	enc, err := LoadMetaspaceBPE(tokenizerPath)
	if err != nil {
		t.Fatalf("LoadMetaspaceBPE(%s): %v", tokenizerPath, err)
	}

	goldenPath := filepath.Join("testdata", "deploy_golden_examples.json")
	raw, err := os.ReadFile(goldenPath)
	if err != nil {
		// The fixtures are produced by the deploy bundle's own tokenizer, so they
		// are not checked in. Without them there is nothing to compare against -
		// that is a missing fixture, not a broken encoder.
		t.Skipf("golden examples not present (%s): %v", goldenPath, err)
	}
	var golden goldenFile
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatalf("parse golden: %v", err)
	}
	cls, sep, mask := golden.Specials["cls"], golden.Specials["sep"], golden.Specials["mask"]
	if cls == 0 || sep == 0 || mask == 0 {
		t.Fatalf("golden specials incomplete: %v", golden.Specials)
	}

	for i, c := range golden.Cases {
		t.Run(c.Lang+"/"+c.Qtype, func(t *testing.T) {
			head := enc.IDs(c.HeadText)
			if len(head) != c.Stats.HeadTokens {
				t.Fatalf("head tokens = %d, want %d", len(head), c.Stats.HeadTokens)
			}
			state := enc.IDs(c.State)
			if len(state) != c.Stats.StateTokens {
				t.Fatalf("state tokens = %d, want %d", len(state), c.Stats.StateTokens)
			}

			// Reassemble the contract sequence:
			// [CLS] head [SEP] [MASK] opt0 [MASK] opt1 ... [SEP] state [SEP]
			seq := make([]int32, 0, c.Stats.TotalTokens)
			markers := make([]int32, 0, len(c.OptionTexts))
			seq = append(seq, cls)
			seq = append(seq, head...)
			seq = append(seq, sep)
			for _, opt := range c.OptionTexts {
				markers = append(markers, int32(len(seq)))
				seq = append(seq, mask)
				seq = append(seq, enc.IDs(" "+opt)...)
			}
			seq = append(seq, sep)
			seq = append(seq, state...)
			seq = append(seq, sep)

			if !reflect.DeepEqual(seq, c.InputIDs) {
				t.Fatalf("case %d (%s/%s) input_ids mismatch:\n got  %v\n want %v",
					i, c.Lang, c.Qtype, seq, c.InputIDs)
			}
			if !reflect.DeepEqual(markers, c.MarkerPos) {
				t.Fatalf("case %d (%s/%s) marker_pos = %v, want %v",
					i, c.Lang, c.Qtype, markers, c.MarkerPos)
			}
		})
	}
}
