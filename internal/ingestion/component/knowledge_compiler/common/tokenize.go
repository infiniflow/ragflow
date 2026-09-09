package common

// EstimateTokens is a lightweight, dependency-free token estimate used when no
// real Tokenizer is injected (≈4 chars/token, matching common LLM tokenizers'
// order of magnitude). Production wiring should inject a real Tokenizer.
func EstimateTokens(s string) int {
	n := (len(s) + 3) / 4
	if n == 0 {
		return 0
	}
	return n
}

// numTokens returns the token count of s using tok when non-nil, else EstimateTokens.
func numTokens(tok Tokenizer, s string) int {
	if tok != nil {
		return tok.NumTokens(s)
	}
	return EstimateTokens(s)
}
