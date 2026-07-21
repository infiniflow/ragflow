package document

import (
	"encoding/hex"

	"github.com/zeebo/xxh3"
)

// contentHashHex mirrors Python xxhash.xxh128(blob).hexdigest().
func contentHashHex(blob []byte) string {
	sum := xxh3.Hash128(blob).Bytes()
	return hex.EncodeToString(sum[:])
}
