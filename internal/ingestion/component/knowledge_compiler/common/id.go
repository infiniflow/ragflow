package common

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strings"
)

// StableRowID returns a deterministic, collision-resistant hex id from the
// given parts (e.g. tenant_id, doc_id, variant, content hash).
//
// Encoding contract: each part is length-prefixed (a 4-byte big-endian
// uint32 byte-length followed by the raw part bytes) before hashing. Length
// prefixing — rather than a NUL/join delimiter — makes the encoding
// unambiguous, so distinct part sequences can never collide (e.g. ["a\x00b"]
// vs ["a","b"]).
//
// The digest is SHA-256 (not a 64-bit hash) so distinct products cannot
// accidentally share an id and be overwritten by Upsert/merge paths (M10).
//
// STABILITY: the encoding and hash algorithm form the row-id contract. This
// feature is pre-GA, so previously generated row ids are disposable and
// re-runs regenerate them deterministically; if the encoding or hash ever
// changes after GA, a backfill/migration of existing compiled rows is
// required before rollout.
func StableRowID(parts ...string) string {
	h := sha256.New()
	var lenBuf [4]byte
	for _, p := range parts {
		binary.BigEndian.PutUint32(lenBuf[:], uint32(len(p)))
		h.Write(lenBuf[:])
		h.Write([]byte(p))
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
