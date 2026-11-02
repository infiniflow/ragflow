//go:build !cgo

package parser

import (
	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

func pdfOutlinesFromData([]byte) []deepdoctype.Outline { return nil }
