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

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// IsDiacriticFoldingLanguage reports whether tokens for the given dataset
// language are indexed with diacritics folded to ASCII. The analyzer's split
// regex only captures ASCII letter runs, so accented words are otherwise
// fragmented before indexing ('škola' -> 'š kola'). Neither language has a
// Snowball stemmer, so the analyzer disables stemming for them.
// Mirrors DIACRITIC_FOLDING_LANGUAGES in rag/nlp/rag_tokenizer.py.
func IsDiacriticFoldingLanguage(lang string) bool {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "slovak", "czech":
		return true
	}
	return false
}

// FoldDiacritics strips combining marks from latin text: 'škola' -> 'skola'.
// Mirrors fold_diacritics in rag/nlp/rag_tokenizer.py (NFD decomposition,
// drop combining marks). Pure-ASCII input is returned unchanged.
func FoldDiacritics(text string) string {
	if isASCII(text) {
		return text
	}
	decomposed := norm.NFD.String(text)
	var b strings.Builder
	b.Grow(len(decomposed))
	for _, r := range decomposed {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// foldForLanguage folds diacritics for folding languages only, before the text
// reaches the C++ analyzer.
func foldForLanguage(lang, text string) string {
	if IsDiacriticFoldingLanguage(lang) {
		return FoldDiacritics(text)
	}
	return text
}

// foldWithOffsets folds diacritics and returns, alongside the folded text, a
// table mapping every byte offset in it back to a byte offset in text. Folding
// shortens the input ('š' is two bytes, 's' is one), so positions the analyzer
// reports against the folded text do not index the caller's string; the table
// is what puts them back.
//
// The table has len(folded)+1 entries so an end offset is mappable too.
func foldWithOffsets(text string) (string, []int) {
	var b strings.Builder
	b.Grow(len(text))
	offsets := make([]int, 0, len(text)+1)

	for i, r := range text {
		// NFD decomposes per rune and never composes across runes, so folding
		// one rune at a time gives the same result as folding the whole string.
		for _, folded := range norm.NFD.String(string(r)) {
			if unicode.Is(unicode.Mn, folded) {
				continue
			}
			n := b.Len()
			b.WriteRune(folded)
			for range b.Len() - n {
				offsets = append(offsets, i)
			}
		}
	}
	offsets = append(offsets, len(text))
	return b.String(), offsets
}

// remapOffset translates a byte offset in the folded text back to the original.
func remapOffset(offsets []int, folded uint32) uint32 {
	i := int(folded)
	if i < 0 || i >= len(offsets) {
		// Out of range means the analyzer reported a position that does not
		// belong to the text it was given; leave it rather than guess.
		return folded
	}
	return uint32(offsets[i])
}
