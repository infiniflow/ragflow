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

//go:build manual

package tokenizer

import "testing"

// TestNumTokensFromString_MatchesPythonAnchors pins exact counts taken from the
// Python reference suite (test/unit_test/common/test_token_utils.py:28-49) and
// from common.token_utils.num_tokens_from_string for the CJK cases. The corpus
// was later expanded to ~24 entries spanning ASCII, punctuation, digits,
// whitespace, newlines, CJK, emoji, mixed-language, and code-like strings, all
// recomputed against tiktoken's cl100k encoder.
//
// Exact values matter more than they look. NumTokensFromString swallows loader
// errors and returns 0, so an assertion of the form "> 0" passes for an empty
// string and fails to notice a dead encoder — which is precisely how the
// offline breakage stayed invisible. Pinning the numbers also catches loading a
// structurally valid but wrong table.
//
// This test needs the real cl100k_base table on disk (TIKTOKEN_CACHE_DIR,
// the Dockerfile's /ragflow/<sha1> file, or ragflow_deps/cl100k_base.tiktoken),
// so it is tagged `manual` and runs only under `build.sh --test-manual`,
// which the docker builder provisions with /usr/share/infinity/resource.
func TestNumTokensFromString_MatchesPythonAnchors(t *testing.T) {
	anchors := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"hello", 1},
		{"hello world", 2},
		{"hello, world!", 4},
		{"世界", 3},
		{"Hello 世界 🌍", 8},
		{"RAGFlow", 3},
		{"1234567890", 4},
		{"a  b", 3},
		{"hello\nworld", 3},
		{"user@example.com", 3},
		{"https://example.com/path?x=1", 9},
		{"func main() {}", 4},
		{"aaaaaaaaaa", 2},
		{"中文字符测试", 4},
		{"🚀🔥", 6},
		{"state-of-the-art", 4},
		{`"quoted"`, 3},
		{"The quick brown fox jumps over the lazy dog.", 10},
		{"Café naïve résumé", 8},
		{"x² + y² = z²", 8},
		{"混合 English 和 中文 的 sentence。", 10},
		{"tokenization is the process of splitting text into tokens", 10},
		{"人工智能正在改变世界，这是毫无疑问的事实。", 25},
		{"SELECT * FROM users WHERE id = 1;", 10},
		{"こんにちは世界", 4},
	}
	for _, tc := range anchors {
		if got := NumTokensFromString(tc.in); got != tc.want {
			t.Errorf("NumTokensFromString(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
