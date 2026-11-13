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

// A complete answer is delivered as-is except for citation markers: the
// streaming tail pattern must not trim its own last characters.
func TestStripChannelCitationsKeepsCompleteAnswerText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"hash suffix", "The language is C#", "The language is C#"},
		{"ref suffix", "Pass the parameter by ref", "Pass the parameter by ref"},
		{"open bracket suffix", "The character class starts with [", "The character class starts with ["},
		{"index syntax", "Use arr[0] to read the first item.", "Use arr[0] to read the first item."},
		{"call result index", "See f()[0] for details.", "See f()[0] for details."},
		{"chained index", "Then a[0][1] is nested.", "Then a[0][1] is nested."},
		{"array literal", "Literal [[1],[2]] stays.", "Literal [[1],[2]] stays."},
		{"numeric citation", "The answer is 42 [1].", "The answer is 42 ."},
		{"leading numeric citation", "[1] is the first reference.", " is the first reference."},
		{"citation run", "Two cites [1][2] end.", "Two cites  end."},
		{"canonical marker", "Canonical [ID:3] marker.", "Canonical  marker."},
		{"parenthesized marker", "Paren (ID: 4) marker.", "Paren  marker."},
		{"full-width marker", "Full-width 【ID: 5】 marker.", "Full-width  marker."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripChannelCitations(tc.in, false); got != tc.want {
				t.Fatalf("stripChannelCitations(%q, false) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// A streamed snapshot can end in the middle of a marker; hiding that half is
// the only reason the tail pattern exists.
func TestStripChannelCitationsHidesSplitMarkersWhileStreaming(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"split ID marker", "Answer text [ID:8", "Answer text "},
		{"split numeric citation", "Answer text [1", "Answer text "},
		{"split ref marker", "Answer text ref", "Answer text "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripChannelCitations(tc.in, true); got != tc.want {
				t.Fatalf("stripChannelCitations(%q, true) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
