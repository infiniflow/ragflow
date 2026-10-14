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

package service

import "testing"

// TestAppendKeywordsSeparatesQuestionFromKeywords pins the delimiter used when
// keyword extraction output is appended to the retrieval question. The
// delimiter matters because without it the question's last token merges with
// the first keyword during tokenization.
func TestAppendKeywordsSeparatesQuestionFromKeywords(t *testing.T) {
	if got := AppendKeywords("什么是RAG", "RAG,检索,增强"); got != "什么是RAG,RAG,检索,增强" {
		t.Fatalf("AppendKeywords = %q", got)
	}
	if got := AppendKeywords("how do I reset my password", "password,reset"); got != "how do I reset my password,password,reset" {
		t.Fatalf("AppendKeywords = %q", got)
	}
}

// TestAppendKeywordsEmptyKeepsQuestion guards against a dangling delimiter when
// keyword extraction yields nothing (LLM error or empty reply).
func TestAppendKeywordsEmptyKeepsQuestion(t *testing.T) {
	if got := AppendKeywords("what is rag", ""); got != "what is rag" {
		t.Fatalf("AppendKeywords with empty keywords = %q, want the question unchanged", got)
	}
	if got := AppendKeywords("", "rag"); got != ",rag" {
		t.Fatalf("AppendKeywords with empty question = %q", got)
	}
}

// TestKeywordDelimiterIsComma keeps every Go callsite on the same delimiter;
// the value is asserted here so a drift shows up as a single failing test
// rather than three divergent concatenations.
func TestKeywordDelimiterIsComma(t *testing.T) {
	if KeywordDelimiter != "," {
		t.Fatalf("KeywordDelimiter = %q, want %q", KeywordDelimiter, ",")
	}
}
