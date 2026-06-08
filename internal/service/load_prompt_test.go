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

// TestThinkBlockREParityWithPython pins down the exact behaviour of
// thinkBlockRE so it stays in sync with the Python original
//
//	re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
//
// The Python regex is greedy: with re.DOTALL the leading `.*` matches
// as much as possible, so for an input with more than one </think>
// the substitution strips everything up to and including the LAST
// marker. A non-greedy `*?` would diverge here and leave the
// tail-visible-portion of the response behind.
// TestJSONFenceREParityWithPython pins down jsonFenceRE against the Python
// original
//
//	re.sub(r"(^.*</think>|```json\n|```\n*$)", "", ans, flags=re.DOTALL)
//
// The Go port is split across thinkBlockRE (run first in callers) and
// jsonFenceRE; this test exercises jsonFenceRE in isolation. The trailing
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

func TestThinkBlockREParityWithPython(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "no_think_tag_unchanged",
			in:   "plain answer, no tags",
			want: "plain answer, no tags",
		},
		{
			name: "single_think_block_stripped",
			in:   "<think>hidden reasoning</think>visible answer",
			want: "visible answer",
		},
		{
			name: "multiline_single_think_block",
			in:   "<think>\nline 1\nline 2\n</think>\nvisible",
			want: "\nvisible",
		},
		{
			name: "two_think_blocks_greedy_strips_to_last",
			// Python: greedy `^.*</think>` strips
			// "<think>A</think>part1<think>B</think>", leaving "part2".
			// A non-greedy form would have left "part1<think>B</think>part2".
			in:   "<think>A</think>part1<think>B</think>part2",
			want: "part2",
		},
		{
			name: "two_think_blocks_with_answer_greedy",
			// Mirrors a real-world malformed stream where the model
			// re-emits a stray </think> after the answer.
			in:   "<think>reasoning</think>Answer<think>noise</think>real tail",
			want: "real tail",
		},
		{
			name: "unclosed_think_tag_does_not_match",
			// No </think> at all — the regex requires the closing tag,
			// so nothing is stripped and the original passes through.
			in:   "<think>no closing tag here",
			want: "<think>no closing tag here",
		},
		{
			name: "lookalike_closing_tag_strips_everything",
			// Quirky but intentional parity case: the regex only requires
			// the substring </think> to be present, regardless of whether
			// an opening <think> exists. The Python original has the same
			// behaviour, so the Go port must match — this is a
			// pre-existing limitation, not a new bug.
			in:   "use </tag> to mean end, not the same as </think>",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := thinkBlockRE.ReplaceAllString(tt.in, "")
			if got != tt.want {
				t.Errorf("input:    %q\n got:     %q\n want:    %q", tt.in, got, tt.want)
			}
		})
	}
}
