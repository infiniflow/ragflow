//go:build !ee

package table

import (
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// ConstructTableVH is the OSS/community stub. The real V/H assembler renderer
// lives behind the //go:build ee tag in the enterprise tree. No OSS-built
// TableItem ever sets RenderVH, so this is never reached; it exists only so
// the shared ConstructTable compiles on both the ee and !ee builds.
func ConstructTableVH(cells []pdf.TSRCell, boxes []pdf.TextBox, caption string) string {
	return ""
}
