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

package common

import (
	"regexp"
	"strings"
)

// ContainsFold reports whether substr is contained in s after folding both to
// lower case. It is the single shared form of the previous inline
// strings.Contains(strings.ToLower(s), strings.ToLower(substr)) copies; callers
// that only lower-case one side are deliberately left alone because their
// needle is already normalized. An empty substr reports true, matching
// strings.Contains.
func ContainsFold(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// Redundant-space cleanup regexes. Mirrors
// common.string_utils.remove_redundant_spaces (string_utils.py:20-46).
// Pass 1: drop spaces after a "left boundary" character (parens, <, >).
// Pass 2: drop spaces before a "right boundary" character (parens, !).
//
// The word-character class is Unicode-aware: letters, combining marks,
// numbers and `_` (`\p{L}`, `\p{M}`, `\p{N}`, `_`). An ASCII-only `a-z0-9`
// puts every non-Latin letter into the negated "punctuation" set, which
// deletes the space between two words: "привет мир" -> "приветмир". Marks
// belong in the class too (Python's `\w` leaves them out): many Indic words
// end in a vowel sign such as U+0947 (Mn) or U+0940 (Mc), and without `\p{M}`
// "के बाद" becomes "केबाद". Letter case needs no flag because `\p{L}` covers
// both cases — that is what Python's `re.IGNORECASE` does for `a-z`.
//
// The non-space class is a literal space, like Python's `[^ ]`: a tab or
// newline is a boundary character there, not whitespace to skip.
var (
	redundantSpacePass1Re = regexp.MustCompile(`([^\p{L}\p{M}\p{N}_.,)>]) +([^ ])`)
	redundantSpacePass2Re = regexp.MustCompile(`([^ ]) +([^\p{L}\p{M}\p{N}_.,(<])`)
)

// RemoveRedundantSpaces removes redundant spaces around punctuation marks
// while preserving the spaces between words.
func RemoveRedundantSpaces(s string) string {
	s = redundantSpacePass1Re.ReplaceAllString(s, "$1$2")
	s = redundantSpacePass2Re.ReplaceAllString(s, "$1$2")
	return s
}

// Markdown-fence recognizers. Mirrors
// common.string_utils.clean_markdown_block (string_utils.py:49-...).
// Matches Python without re.MULTILINE so ^/$ anchor only the whole text.
var (
	markdownFenceOpenRe  = regexp.MustCompile(`^\s*` + "```" + `markdown\s*\n?`)
	markdownFenceCloseRe = regexp.MustCompile(`\n?\s*` + "```" + `\s*$`)
)

// CleanMarkdownBlock removes a closing fence only after recognizing an opening
// markdown wrapper; anything else is returned trimmed and otherwise untouched.
func CleanMarkdownBlock(s string) string {
	if markdownFenceOpenRe.MatchString(s) {
		s = markdownFenceOpenRe.ReplaceAllString(s, "")
		s = markdownFenceCloseRe.ReplaceAllString(s, "")
	}
	return strings.TrimSpace(s)
}
