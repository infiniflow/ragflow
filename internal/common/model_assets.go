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
	"strings"
)

// EnvModelAssetsDir points at a directory that plays the role of `ragflow_deps/`:
// downloaded model assets stored in the HuggingFace repo layout, i.e.
// `<dir>/huggingface.co/<repo>/<file>` (plus loose files that are not HF repos, such as
// the cl100k BPE table). It is how a deployment keeps model assets outside the checkout -
// a mounted volume, a read-only image layer, or a shared cache - instead of relying on
// the working directory the process happens to have.
//
// The name is deliberately not embedding-specific: the layout is shared by every
// downloaded model asset (embedding tokenizers, DeepDoc weights, and whatever comes
// next), so any loader that reads `huggingface.co/<repo>/<file>` should consult
// ModelAssetCandidates rather than inventing its own variable.
const EnvModelAssetsDir = "MODEL_ASSETS_DIR"

// ModelAssetCandidates returns the paths to try, in priority order, for one asset when
// MODEL_ASSETS_DIR is set. `relative` is the shipped name - either
// "ragflow_deps/huggingface.co/BAAI/bge-m3/sentencepiece.bpe.model" or a bare
// "cl100k_base.tiktoken" - and the candidates let an operator point at any of the three
// sensible roots:
//
//	<dir>/huggingface.co/<repo>/<file>   the ragflow_deps-like root
//	<dir>/<repo>/<file>                  a huggingface.co-like root
//	<dir>/<file>                         a flat directory holding the files
//
// Returns nil when the variable is unset, leaving the caller's own search untouched.
func ModelAssetCandidates(relative string) []string {
	root := strings.TrimSpace(GetEnv(EnvModelAssetsDir))
	if root == "" {
		return nil
	}
	rel := strings.TrimPrefix(filepath.ToSlash(relative), "ragflow_deps/")
	out := make([]string, 0, 3)
	seen := make(map[string]struct{}, 3)
	add := func(p string) {
		if _, dup := seen[p]; dup {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	add(filepath.Join(root, filepath.FromSlash(rel)))
	if rest, ok := strings.CutPrefix(rel, "huggingface.co/"); ok {
		add(filepath.Join(root, filepath.FromSlash(rest)))
		// The flat layout documented above, for an operator who kept the files
		// without their repo directories. It is last because a repo-shipped name
		// like tokenizer.json collides across repos, so the repo layouts win.
		add(filepath.Join(root, filepath.Base(rel)))
	} else {
		// A loose file (the cl100k table) has no repo directory: its root-level
		// candidate is the one already added above, so this cannot add a second.
		add(filepath.Join(root, filepath.Base(rel)))
	}
	return out
}
