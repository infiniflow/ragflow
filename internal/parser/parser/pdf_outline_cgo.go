//go:build cgo

package parser

import (
	"ragflow/internal/deepdoc/parser/pdf/pdfium"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

func pdfOutlinesFromData(data []byte) []deepdoctype.Outline {
	return pdfium.ExtractOutlines(data)
}
