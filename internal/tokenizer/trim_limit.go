package tokenizer

// trimToRuneLimitByCount returns the longest rune prefix of text whose token
// count (as reported by countTokens) is at most limit. It is the shared form
// of the three identical per-model trimToLimit implementations; each model
// injects its own counter so tokenization semantics stay model-specific.
func trimToRuneLimitByCount(text string, limit int, countTokens func(string) int) string {
	if limit <= 0 {
		return ""
	}
	if countTokens(text) <= limit {
		return text
	}
	runes := []rune(text)
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if countTokens(string(runes[:mid])) <= limit {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return string(runes[:lo])
}
