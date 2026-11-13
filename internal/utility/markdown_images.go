package utility

import "regexp"

var markdownImageMarkup = regexp.MustCompile(`(?is)!\[[^\]]*\]\(([^()]|\([^()]*\))*\)|<div[^>]*>\s*<img[^>]*>\s*</div>|<img[^>]*>`)

// StripMarkdownImages removes inline Markdown images and HTML image markup.
// Inline destinations may contain one level of balanced parentheses.
func StripMarkdownImages(text string) string {
	return markdownImageMarkup.ReplaceAllString(text, "")
}
