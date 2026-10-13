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

package common

import (
	"path/filepath"
	"testing"
)

func TestModelAssetCandidatesOnlyAppliesWhenTheEnvIsSet(t *testing.T) {
	t.Setenv(EnvModelAssetsDir, "")
	if got := ModelAssetCandidates("ragflow_deps/huggingface.co/BAAI/bge-m3/sentencepiece.bpe.model"); got != nil {
		t.Fatalf("expected no candidates without %s, got %v", EnvModelAssetsDir, got)
	}
	t.Setenv(EnvModelAssetsDir, "   ")
	if got := ModelAssetCandidates("cl100k_base.tiktoken"); got != nil {
		t.Fatalf("expected whitespace-only %s to be ignored, got %v", EnvModelAssetsDir, got)
	}
}

// The three layouts an operator may have in mind when pointing MODEL_ASSETS_DIR at a
// tree: the ragflow_deps-like root, a huggingface.co-like root, or a flat directory.
func TestModelAssetCandidatesCoverEverySensibleRoot(t *testing.T) {
	root := filepath.Join(string(filepath.Separator)+"assets", "models")
	t.Setenv(EnvModelAssetsDir, root)

	got := ModelAssetCandidates("ragflow_deps/huggingface.co/BAAI/bge-m3/sentencepiece.bpe.model")
	want := []string{
		filepath.Join(root, "huggingface.co", "BAAI", "bge-m3", "sentencepiece.bpe.model"),
		filepath.Join(root, "BAAI", "bge-m3", "sentencepiece.bpe.model"),
		// The flat layout: the operator kept the file without its repo directory.
		filepath.Join(root, "sentencepiece.bpe.model"),
	}
	if len(got) != len(want) {
		t.Fatalf("candidates = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("candidate %d = %q, want %q", i, got[i], want[i])
		}
	}

	// The same asset named without the ragflow_deps/ prefix must resolve identically.
	bare := ModelAssetCandidates("huggingface.co/BAAI/bge-m3/sentencepiece.bpe.model")
	if len(bare) != len(want) || bare[0] != want[0] {
		t.Errorf("prefix form differs: %v vs %v", bare, want)
	}

	// A file that is not an HF repo (the cl100k table) sits at the root itself,
	// and its single candidate must not be repeated.
	flat := ModelAssetCandidates("ragflow_deps/cl100k_base.tiktoken")
	if len(flat) != 1 || flat[0] != filepath.Join(root, "cl100k_base.tiktoken") {
		t.Errorf("flat asset candidates = %v, want exactly one root-level candidate", flat)
	}
}
