package esoscommon

import "strings"

// SanitizeString replaces apostrophes, newlines and carriage returns with a
// single space and trims surrounding whitespace. It was previously duplicated
// byte-for-byte in the Elasticsearch and OpenSearch engines; both now call this
// shared copy so the two backends cannot drift.
func SanitizeString(s string) string {
	s = strings.ReplaceAll(s, "'", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.TrimSpace(s)
}
