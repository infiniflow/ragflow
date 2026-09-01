//go:build !cgo

package component

import "fmt"

func defaultRenderPDFVisionPages(_ []byte) ([]pdfVisionPage, error) {
	return nil, fmt.Errorf("tenant-aware PDF IMAGE2TEXT backend requires cgo rendering support")
}
