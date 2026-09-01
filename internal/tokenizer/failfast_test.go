//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain the License at
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
	"strings"
	"testing"
)

// TestInitCL100KEncoder_FailFast pins the contract that a missing cl100k_base
// BPE table is a hard error, never a silent success.
//
// Background: NumTokensFromString swallows the loader error and returns 0, so
// a Go image that forgot to bake cl100k_base.tiktoken (see the colleague
// comment on the missing table) silently zeroed every token count while
// content_ltks — which uses the separate C++ RAGAnalyzer — kept working. That
// divergence stayed invisible. The fix is to fail fast at server startup via
// InitCL100KEncoder; this test guards that the function reports the loader
// error instead of returning nil while the encoder is actually dead.
//
// The test is environment-agnostic: if the table happens to be on disk
// (e.g. a checkout that ran download_deps.py, or the Dockerfile-mounted copy),
// InitCL100KEncoder must succeed and the encoder must really count tokens. If
// the table is absent, it must return a non-nil error. What it forbids is
// InitCL100KEncoder returning nil while the encoder cannot load.
func TestInitCL100KEncoder_FailFast(t *testing.T) {
	// Narrow the search to an empty dir so the outcome is driven by whether a
	// table exists in the search roots (working dir / its ancestors under
	// ragflow_deps/, or the Dockerfile-mounted copy), not by a stale cache dir.
	dir := t.TempDir()
	t.Setenv("TIKTOKEN_CACHE_DIR", dir)
	t.Setenv("DATA_GYM_CACHE_DIR", dir)

	err := InitCL100KEncoder()
	if err != nil {
		if !strings.Contains(err.Error(), "cl100k") {
			t.Fatalf("InitCL100KEncoder returned an unexpected error: %v", err)
		}
		t.Logf("BPE table absent in this environment; fail-fast returned error as required: %v", err)
		return
	}

	// Table present: the encoder must actually tokenize, not just "load".
	if got := NumTokensFromString("hello world"); got <= 0 {
		t.Fatalf("InitCL100KEncoder succeeded but NumTokensFromString(%q) = %d, want > 0", "hello world", got)
	}
}
