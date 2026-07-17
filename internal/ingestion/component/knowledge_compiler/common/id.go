package common

import (
	"fmt"
	"strings"

	"github.com/cespare/xxhash/v2"
)

// StableRowID returns a deterministic, collision-resistant hex id from the
// given parts (e.g. tenant_id, doc_id, variant, content hash). Field
// separators are NUL bytes so distinct part orderings never collide.
func StableRowID(parts ...string) string {
	h := xxhash.New()
	for i, p := range parts {
		if i > 0 {
			h.Write([]byte{0})
		}
		h.WriteString(p)
	}
	return fmt.Sprintf("%016x", h.Sum64())
}

// ContentHash returns a stable hash of arbitrary text, used as a dedup key
// component for row ids and for in-run equality checks.
func ContentHash(text string) string {
	h := xxhash.New()
	h.WriteString(strings.TrimSpace(text))
	return fmt.Sprintf("%016x", h.Sum64())
}
