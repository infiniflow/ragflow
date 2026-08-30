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
