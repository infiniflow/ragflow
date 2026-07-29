package common

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// StableRowID returns a deterministic, collision-resistant hex id from the
// given parts (e.g. tenant_id, doc_id, variant, content hash). Field
// separators are NUL bytes so distinct part orderings never collide. The
// digest is SHA-256 (not a 64-bit hash) so distinct products cannot
// accidentally share an id and be overwritten by Upsert/merge paths (M10).
func StableRowID(parts ...string) string {
	h := sha256.New()
	for i, p := range parts {
		if i > 0 {
			h.Write([]byte{0})
		}
		_, _ = h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ContentHash returns a stable hash of arbitrary text, used as a dedup key
// component for row ids and for in-run equality checks. SHA-256 for the same
// collision-resistance reason as StableRowID (M10).
func ContentHash(text string) string {
	h := sha256.New()
	_, _ = h.Write([]byte(strings.TrimSpace(text)))
	return hex.EncodeToString(h.Sum(nil))
}
