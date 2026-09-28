//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except under the License.
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package chunker

import "testing"

// TestSentenceBoundary_SharedRegexMatchesPython pins sentenceBoundaryRe to the
// exact Python SENTENCE_BOUNDARY_RE (rag/flow/chunker/_sentence_boundary.py),
// including the English ". " boundary used by the title chunker and the token
// chunker's JSON path.
func TestSentenceBoundary_SharedRegexMatchesPython(t *testing.T) {
	const want = `([。!?？；！\n]|\. )`
	if got := sentenceBoundaryRe.String(); got != want {
		t.Errorf("sentenceBoundaryRe = %q, want %q (Python SENTENCE_BOUNDARY_RE)", got, want)
	}
	// English ". " boundary must be recognized.
	if !sentenceBoundaryRe.MatchString("Hello. World") {
		t.Error("sentenceBoundaryRe does not match the English '. ' boundary")
	}
	// CJK boundaries still recognized.
	for _, p := range []string{"一。", "二！", "三？", "四；", "五\n"} {
		if !sentenceBoundaryRe.MatchString(p) {
			t.Errorf("sentenceBoundaryRe does not match boundary %q", p)
		}
	}
}

// TestSentenceBoundary_TokenTextPathDelimiterDistinct pins sentenceDelimiter
// (the token-chunker TEXT path / naive_merge port) to Python naive_merge's
// production default DEFAULT_DELIMITER ("\n!?;。；！？", rag/nlp/delim.py) —
// deliberately WITHOUT the English ". " boundary. The two constants mirror two
// distinct Python delimiters; asserting the exact difference documents that
// this is intentional, not a drift.
func TestSentenceBoundary_TokenTextPathDelimiterDistinct(t *testing.T) {
	const want = `(\n|[!?;。；！？])`
	if got := sentenceDelimiter.String(); got != want {
		t.Errorf("sentenceDelimiter = %q, want %q (Python naive_merge delimiter)", got, want)
	}
	// The text-path delimiter must NOT split on the English ". " boundary
	// (matching Python naive_merge), while the shared regex does.
	if sentenceDelimiter.MatchString("Hello. World") {
		t.Error("sentenceDelimiter split on '. ' but naive_merge's delimiter has no '. ' boundary")
	}
	// The text-path delimiter must split on the ASCII semicolon, matching the
	// unified Python default (issue #18562: the "; " omission was drift).
	if !sentenceDelimiter.MatchString("one; two") {
		t.Error("sentenceDelimiter should split on ';': naive_merge's default includes ';'")
	}
	if !sentenceBoundaryRe.MatchString("Hello. World") {
		t.Error("sentenceBoundaryRe should split on '. '")
	}
}
