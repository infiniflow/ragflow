package advanced_rag

import "time"

// trunc truncates s to at most n bytes, returning s unchanged when it already
// fits. Mirrors the harness helper of the same name; kept local so the
// consolidated RAGTools methods do not depend on unexported harness symbols.
func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// deadlineToDuration converts a seconds budget to a context deadline. A
// non-positive budget falls back to the default answer timeout so a caller that
// omits it does not produce an already-expired context.
func deadlineToDuration(seconds float64) time.Duration {
	if seconds <= 0 {
		seconds = answerTimeoutS
	}
	return time.Duration(seconds * float64(time.Second))
}
