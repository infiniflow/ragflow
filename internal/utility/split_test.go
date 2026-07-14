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

package utility

import "testing"

// SplitKeywords - Python task_executor.run_dataflow:879
// re.split(r"[,，;；、\r\n]+", keywords) with empty filtering.

func TestSplitKeywords_Comma(t *testing.T) {
	result := SplitKeywords("kw1,kw2,kw3")
	if len(result) != 3 {
		t.Fatalf("len = %d, want 3", len(result))
	}
}

func TestSplitKeywords_ChineseComma(t *testing.T) {
	result := SplitKeywords("kw1，kw2，kw3")
	if len(result) != 3 {
		t.Fatalf("len = %d, want 3", len(result))
	}
}

func TestSplitKeywords_MixedDelimiters(t *testing.T) {
	result := SplitKeywords("kw1,kw2；kw3")
	if len(result) != 3 {
		t.Fatalf("len = %d, want 3, got %v", len(result), result)
	}
}

func TestSplitKeywords_FiltersEmptyStrings(t *testing.T) {
	result := SplitKeywords("kw1,,kw2")
	if len(result) != 2 {
		t.Errorf("empty strings should be filtered: got %v", result)
	}
}

func TestSplitKeywords_Empty(t *testing.T) {
	result := SplitKeywords("")
	if len(result) != 0 {
		t.Errorf("got %v, want empty", result)
	}
}

// SplitQuestions - splits by newline only, dropping empty lines.

func TestSplitQuestions_MultipleLines(t *testing.T) {
	result := SplitQuestions("Q1\nQ2\nQ3")
	if len(result) != 3 || result[0] != "Q1" || result[2] != "Q3" {
		t.Fatalf("got %v, want [Q1 Q2 Q3]", result)
	}
}

func TestSplitQuestions_Empty(t *testing.T) {
	if result := SplitQuestions(""); result != nil {
		t.Errorf("got %v, want nil", result)
	}
}

func TestSplitQuestions_TrailingNewlineDropped(t *testing.T) {
	// "Q1\n" would yield ["Q1", ""] with a plain split; the trailing empty
	// must be filtered so the index is not seeded with a spurious [""].
	result := SplitQuestions("Q1\n")
	if len(result) != 1 || result[0] != "Q1" {
		t.Fatalf("got %v, want [Q1]", result)
	}
}

func TestSplitQuestions_BlankLinesDropped(t *testing.T) {
	result := SplitQuestions("Q1\n\nQ2")
	if len(result) != 2 || result[0] != "Q1" || result[1] != "Q2" {
		t.Fatalf("got %v, want [Q1 Q2]", result)
	}
}

func TestSplitQuestions_PreservesCommas(t *testing.T) {
	// Unlike SplitKeywords, questions must NOT split on commas.
	result := SplitQuestions("请问A,B和C有何区别？")
	if len(result) != 1 || result[0] != "请问A,B和C有何区别？" {
		t.Fatalf("got %v, want single element preserving comma", result)
	}
}
