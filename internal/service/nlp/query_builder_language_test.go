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

package nlp

import (
	"strings"
	"testing"
	"time"

	"ragflow/internal/tokenizer"
)

// TestQueryBuilder_Question_FoldingLanguage verifies that a Slovak query is
// tokenized by the analyzer with diacritics folded to ASCII, so every emitted
// keyword and the query expression match the folded index tokens.
func TestQueryBuilder_Question_FoldingLanguage(t *testing.T) {
	// Folding happens inside the analyzer, so the real dictionaries must be
	// loaded (manual tier, like the analyzer-level tests in internal/tokenizer).
	if err := tokenizer.Init(&tokenizer.PoolConfig{MinSize: 1, MaxSize: 1, IdleTimeout: 5 * time.Second, AcquireTimeout: 5 * time.Second}); err != nil {
		t.Fatalf("Failed to initialize pool: %v", err)
	}
	defer tokenizer.Close()

	qb := NewQueryBuilder()
	hasDiacritics := func(s string) bool {
		for _, r := range s {
			if r > 127 {
				return true
			}
		}
		return false
	}

	expr, keywords := qb.Question("daňové priznanie pre živnostníkov", "test", 0.5, "Slovak")
	if expr == nil {
		t.Fatal("Question(Slovak) returned nil expr")
	}
	if hasDiacritics(expr.MatchingText) {
		t.Errorf("Question(Slovak) query contains diacritics: %q", expr.MatchingText)
	}
	for _, k := range keywords {
		if hasDiacritics(k) {
			t.Errorf("Question(Slovak) keyword contains diacritics: %q", k)
		}
	}
	if !strings.Contains(expr.MatchingText, "danove") {
		t.Errorf("Question(Slovak) query missing folded token 'danove': %q", expr.MatchingText)
	}

	// original_query must stay untouched for highlighting.
	if orig, _ := expr.ExtraOptions["original_query"].(string); orig != "daňové priznanie pre živnostníkov" {
		t.Errorf("Question(Slovak) original_query = %q, want the unfolded input", orig)
	}

	// Non-folding languages keep accents untouched.
	exprEn, _ := qb.Question("daňové priznanie pre živnostníkov", "test", 0.5, "")
	if exprEn == nil {
		t.Fatal("Question(English) returned nil expr")
	}
	if !hasDiacritics(exprEn.MatchingText) {
		t.Errorf("Question(English) unexpectedly folded diacritics: %q", exprEn.MatchingText)
	}
}
