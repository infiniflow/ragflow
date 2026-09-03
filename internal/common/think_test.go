package common

import "testing"

// TestStripThinkTrailingParityWithPython pins down the exact behaviour of
// StripThinkTrailing so it stays in sync with the Python original
//
//	re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
//
// The Python regex is greedy: with re.DOTALL the leading `.*` matches as much
// as possible, so for an input with more than one </think> the substitution
// strips everything up to and including the LAST marker. A non-greedy `*?`
// would diverge here and leave the tail-visible-portion of the response behind.
func TestStripThinkTrailingParityWithPython(t *testing.T) {
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
			// No </think> at all — the helper requires the closing tag,
			// so nothing is stripped and the original passes through.
			in:   "<think>no closing tag here",
			want: "<think>no closing tag here",
		},
		{
			name: "lookalike_closing_tag_strips_everything",
			// Quirky but intentional parity case: the helper only requires
			// the substring </think> to be present, regardless of whether
			// an opening <think> exists. The Python original has the same
			// behaviour, so the port must match — this is a pre-existing
			// limitation, not a new bug.
			in:   "use </tag> to mean end, not the same as </think>",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripThinkTrailing(tt.in); got != tt.want {
				t.Errorf("input: %q\n got: %q\n want: %q", tt.in, got, tt.want)
			}
		})
	}
}
