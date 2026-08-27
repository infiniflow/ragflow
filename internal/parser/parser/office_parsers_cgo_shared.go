//go:build cgo

package parser

import "html"

func htmlEscape(s string) string {
	return html.EscapeString(s)
}
