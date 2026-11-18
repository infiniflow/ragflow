package common

import (
	"regexp"
	"strings"
)

// Redundant-space cleanup regexes. Mirrors
// common.string_utils.remove_redundant_spaces (string_utils.py:20-46).
// Pass 1: drop spaces after a "left boundary" character (parens, <, >).
// Pass 2: drop spaces before a "right boundary" character (parens, !).
//
// The word-character class is Unicode-aware (`\p{L}`, `\p{N}`, `_`), matching
// Python's `\w`. An ASCII-only `a-z0-9` puts every non-Latin letter into the
// negated "punctuation" set, which deletes the space between two words:
// "привет мир" -> "приветмир". Letter case needs no flag because `\p{L}`
// covers both cases — that is what Python's `re.IGNORECASE` does for `a-z`.
//
// The non-space class is a literal space, like Python's `[^ ]`: a tab or
// newline is a boundary character there, not whitespace to skip.
var (
	redundantSpacePass1Re = regexp.MustCompile(`([^\p{L}\p{N}_.,)>]) +([^ ])`)
	redundantSpacePass2Re = regexp.MustCompile(`([^ ]) +([^\p{L}\p{N}_.,(<])`)
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
