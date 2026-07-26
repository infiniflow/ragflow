//go:build !cgo

package docx

import "errors"

// ExtractRawBlocks is not available without cgo because the underlying
// office_oxide library requires CGo.  Rebuild with CGO_ENABLED=1.
func ExtractRawBlocks(_ []byte) ([]RawBlock, error) {
	return nil, errors.New("office_oxide requires cgo; rebuild with CGO_ENABLED=1")
}
