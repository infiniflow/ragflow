//go:build cgo

package parser

import "html"

// OfficeOxide is the lib_type identifier for office_oxide backend.
const OfficeOxide = "office_oxide"

func htmlEscape(s string) string {
	return html.EscapeString(s)
}
