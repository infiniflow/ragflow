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

package models

import "testing"

func TestExtractDefaultStreamReasoning(t *testing.T) {
	cases := []struct {
		name  string
		delta map[string]any
		want  string
	}{
		{"reasoning_content", map[string]any{"reasoning_content": "think"}, "think"},
		{"reasoning (vLLM >= 0.16)", map[string]any{"reasoning": "think"}, "think"},
		{"reasoning_content wins when both set", map[string]any{"reasoning_content": "a", "reasoning": "b"}, "a"},
		{"empty reasoning_content falls back", map[string]any{"reasoning_content": "", "reasoning": "b"}, "b"},
		{"none", map[string]any{"content": "hi"}, ""},
		{"null reasoning", map[string]any{"reasoning": nil}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractDefaultStreamReasoning(tc.delta); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
