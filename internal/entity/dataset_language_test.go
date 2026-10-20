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

package entity

import "testing"

func kb(language *string) *Knowledgebase {
	return &Knowledgebase{Language: language}
}

func ptr(s string) *string { return &s }

func TestKnowledgebasesLanguage(t *testing.T) {
	tests := []struct {
		name string
		kbs  []*Knowledgebase
		want string
	}{
		{"no datasets", nil, ""},
		{"single dataset", []*Knowledgebase{kb(ptr("Slovak"))}, "Slovak"},
		{"all agree", []*Knowledgebase{kb(ptr("Slovak")), kb(ptr("Slovak"))}, "Slovak"},
		{"agree ignoring case and space", []*Knowledgebase{kb(ptr("Slovak")), kb(ptr(" slovak "))}, "Slovak"},
		// A mixed set has no single answer: one query is tokenized once, but the
		// datasets were indexed under different rules. Fall back to English.
		{"mixed languages", []*Knowledgebase{kb(ptr("Slovak")), kb(ptr("English"))}, ""},
		{"mixed with unset", []*Knowledgebase{kb(ptr("Slovak")), kb(nil)}, ""},
		{"unset first, set second", []*Knowledgebase{kb(nil), kb(ptr("Slovak"))}, ""},
		{"all unset", []*Knowledgebase{kb(nil), kb(nil)}, ""},
		{"nil element", []*Knowledgebase{kb(ptr("Slovak")), nil}, ""},
		// Order must not decide the result.
		{"mixed, reversed", []*Knowledgebase{kb(ptr("English")), kb(ptr("Slovak"))}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := KnowledgebasesLanguage(tt.kbs); got != tt.want {
				t.Errorf("KnowledgebasesLanguage() = %q, want %q", got, tt.want)
			}
		})
	}
}
