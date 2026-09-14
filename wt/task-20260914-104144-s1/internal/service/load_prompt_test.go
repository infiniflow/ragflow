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

// TestJSONFenceREParityWithPython pins down jsonFenceRE against the Python
// original
//
//	re.sub(r"(^.*</think>|```json\n|```\n*$)", "", ans, flags=re.DOTALL)
//
// The Go port is split across common.StripThinkTrailing (run first in callers)
// and jsonFenceRE; this test exercises jsonFenceRE in isolation. The trailing
// alternative uses \n* (not \s*) so a closing fence followed by other
// whitespace — e.g. "```   \n" — is preserved exactly as Python does.
func TestJSONFenceREParityWithPython(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "opening_json_fence_stripped",
			in:   "```json\n{\"a\":1}\n",
			want: "{\"a\":1}\n",
		},
		{
			name: "opening_json_fence_no_content_newline",
			in:   "```json\n{\"a\":1}",
			want: "{\"a\":1}", // \n is the separator between ```json and the body
		},
		{
			name: "trailing_fence_with_newline_stripped",
			in:   "{\"a\":1}\n```\n",
			want: "{\"a\":1}\n",
		},
		{
			name: "trailing_fence_no_newline_stripped",
			in:   "{\"a\":1}\n```",
			want: "{\"a\":1}\n", // \n* matches the existing \n before ```
		},
		{
			name: "trailing_fence_no_preceding_newline_stripped",
			in:   "{\"a\":1}```",
			want: "{\"a\":1}",
		},
		{
			name: "trailing_fence_with_spaces_preserved",
			// Python leaves this alone: \n* does not match spaces.
			// A `\s*` form would strip it.
			in:   "```   \n",
			want: "```   \n",
		},
		{
			name: "bare_fence_stripped",
			in:   "```",
			want: "",
		},
		{
			name: "two_opening_fences_both_stripped",
			// The opening alternative is not anchored, so a second
			// ```json\n mid-stream is also stripped.
			in:   "```json\n{\"a\":1}\n```\n```json\n{\"a\":2}",
			want: "{\"a\":1}\n```\n{\"a\":2}",
		},
		{
			name: "mid_text_closing_fence_not_stripped",
			// The closing alternative has a $ anchor, so a bare
			// ```\n in the middle of the string is NOT stripped.
			in:   "{\"a\":1}\n```\n{\"a\":2}",
			want: "{\"a\":1}\n```\n{\"a\":2}",
		},
		{
			name: "plain_text_unchanged",
			in:   "plain text response",
			want: "plain text response",
		},
		{
			name: "open_close_pair",
			in:   "```json\n{\"a\":1}\n```",
			want: "{\"a\":1}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jsonFenceRE.ReplaceAllString(tt.in, "")
			if got != tt.want {
				t.Errorf("input:    %q\n got:     %q\n want:    %q", tt.in, got, tt.want)
			}
		})
	}
}
