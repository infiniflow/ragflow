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

package tokenizer

import "testing"

func TestFoldDiacritics(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"slovak word", "škola", "skola"},
		{"slovak phrase", "daňové priznanie", "danove priznanie"},
		{"czech phrase", "příliš žluťoučký kůň", "prilis zlutoucky kun"},
		{"ascii idempotence", "already ascii text 123", "already ascii text 123"},
		{"uppercase", "Škola", "Skola"},
		{"slovak-specific letters", "ľĺŕôä", "llroa"},
		{"czech ring and carons", "ůěřďť", "uerdt"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FoldDiacritics(tt.in); got != tt.want {
				t.Errorf("FoldDiacritics(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFoldDiacritics_Idempotent(t *testing.T) {
	for _, in := range []string{"škola", "daňové priznanie", "příliš žluťoučký kůň"} {
		once := FoldDiacritics(in)
		if twice := FoldDiacritics(once); twice != once {
			t.Errorf("FoldDiacritics not idempotent for %q: %q -> %q", in, once, twice)
		}
	}
}

func TestIsDiacriticFoldingLanguage(t *testing.T) {
	for _, lang := range []string{"Slovak", "slovak", "SLOVAK", "Czech", "czech", " Czech "} {
		if !IsDiacriticFoldingLanguage(lang) {
			t.Errorf("IsDiacriticFoldingLanguage(%q) = false, want true", lang)
		}
	}
	for _, lang := range []string{"", "English", "Dutch", "German", "Slovenian", "Chinese"} {
		if IsDiacriticFoldingLanguage(lang) {
			t.Errorf("IsDiacriticFoldingLanguage(%q) = true, want false", lang)
		}
	}
}

func TestFoldWithOffsets_MatchesFoldDiacritics(t *testing.T) {
	for _, in := range []string{"škola dom", "daňové priznanie", "příliš žluťoučký kůň", "plain ascii", "", "mixed ľĺŕ abc"} {
		folded, offsets := foldWithOffsets(in)
		if want := FoldDiacritics(in); folded != want {
			t.Errorf("foldWithOffsets(%q) = %q, want %q", in, folded, want)
		}
		if len(offsets) != len(folded)+1 {
			t.Errorf("foldWithOffsets(%q): %d offsets for %d bytes, want %d", in, len(offsets), len(folded), len(folded)+1)
		}
	}
}

func TestFoldWithOffsets_MapsBackToTheOriginalBytes(t *testing.T) {
	// "škola dom": 'š' is two bytes, so every offset after it shifts by one.
	const in = "škola dom"
	folded, offsets := foldWithOffsets(in)
	if folded != "skola dom" {
		t.Fatalf("folded = %q", folded)
	}

	// The second word starts at byte 6 in the folded text and byte 7 in the original.
	start := remapOffset(offsets, 6)
	end := remapOffset(offsets, 9)
	if got := in[start:end]; got != "dom" {
		t.Errorf("in[%d:%d] = %q, want %q", start, end, got, "dom")
	}

	// The first token spans the whole accented word in the original.
	if got := in[remapOffset(offsets, 0):remapOffset(offsets, 5)]; got != "škola" {
		t.Errorf("first token = %q, want %q", got, "škola")
	}
}

func TestRemapOffset_LeavesOutOfRangeOffsetsAlone(t *testing.T) {
	_, offsets := foldWithOffsets("abc")
	if got := remapOffset(offsets, 99); got != 99 {
		t.Errorf("remapOffset(out of range) = %d, want 99", got)
	}
}
