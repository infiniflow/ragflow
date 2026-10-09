//go:build !cgo

package component

import "fmt"

func defaultRenderPDFVisionPages(_ []byte, _ [][]int) ([]pdfVisionPage, error) {
	return nil, fmt.Errorf("tenant-aware PDF IMAGE2TEXT backend requires cgo rendering support")
}
