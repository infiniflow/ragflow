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

package harness

import (
	"reflect"
	"testing"
)

// TestSplitSentences_TerminatorsKept asserts terminators stay attached and empty
// segments are dropped.
func TestSplitSentences_TerminatorsKept(t *testing.T) {
	got := splitSentences("Paris has people. It is in France! Really?")
	want := []string{"Paris has people.", "It is in France!", "Really?"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("splitSentences = %#v, want %#v", got, want)
	}
}

// TestSplitSentences_DecimalGuard asserts "3.14" / "v1.2" do not split.
func TestSplitSentences_DecimalGuard(t *testing.T) {
	got := splitSentences("Value is 3.14 today. Version v1.2 released.")
	if len(got) != 2 {
		t.Fatalf("splitSentences = %#v, want 2 sentences (decimals not split)", got)
	}
	if got[0] != "Value is 3.14 today." {
		t.Errorf("first = %q, want decimal intact", got[0])
	}
}

// TestSplitSentences_CJK asserts Chinese terminators split and are kept.
func TestSplitSentences_CJK(t *testing.T) {
	got := splitSentences("巴黎有两百万人。它在法国！")
	want := []string{"巴黎有两百万人。", "它在法国！"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("splitSentences(CJK) = %#v, want %#v", got, want)
	}
}

// TestSplitSentences_TableAtomic asserts a markdown table is one atomic sentence.
func TestSplitSentences_TableAtomic(t *testing.T) {
	text := "Intro here.\n| a | b |\n| - | - |\n| 1 | 2 |\nAfter table."
	got := splitSentences(text)
	if len(got) != 3 {
		t.Fatalf("splitSentences(table) = %#v, want 3 (table atomic)", got)
	}
	// The table block must be a single sentence.
	if got[1] != "| a | b |\n| - | - |\n| 1 | 2 |" {
		t.Errorf("table sentence = %q, want atomic table", got[1])
	}
}

// TestSplitSentences_Empty asserts empty input yields nil.
func TestSplitSentences_Empty(t *testing.T) {
	if got := splitSentences(""); got != nil {
		t.Errorf("splitSentences(\"\") = %#v, want nil", got)
	}
}
