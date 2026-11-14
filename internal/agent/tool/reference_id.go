package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// referenceIDs derives the citation identifiers for one search result.
// documentID keys the chunk and its docAgg; displayID is the citation marker
// rendered into the prompt. The URL participates in the seed so identical
// content served from different URLs stays distinguishable, and the 64-bit
// halves make collisions negligible at search-result volumes.
func referenceIDs(resultURL, content string) (documentID, displayID string) {
	seed := strings.TrimSpace(resultURL) + "\n" + content
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:8]), hex.EncodeToString(sum[8:16])
}
