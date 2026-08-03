package document

import (
	"encoding/hex"

	"github.com/zeebo/xxh3"
)

// Hash128Hex mirrors Python xxhash.xxh128(data).hexdigest(), which backs both
// content hashes and api/utils/common.hash128 (deterministic document IDs).
func Hash128Hex(data []byte) string {
	sum := xxh3.Hash128(data).Bytes()
	return hex.EncodeToString(sum[:])
}

// contentHashHex mirrors Python xxhash.xxh128(blob).hexdigest().
func contentHashHex(blob []byte) string {
	return Hash128Hex(blob)
}
