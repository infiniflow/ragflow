package font

import "strings"

// substitutes maps the family of a font of Microsoft Office, in lower case,
// to the family of a font with the same metrics that the module
// github.com/xo/ivy/fonts holds (D24).
var substitutes = map[string]string{
	"arial":           "Liberation Sans",
	"helvetica":       "Liberation Sans",
	"times new roman": "Liberation Serif",
	"times":           "Liberation Serif",
	"courier new":     "Liberation Mono",
	"courier":         "Liberation Mono",
	"calibri":         "Carlito",
	"cambria":         "Caladea",
}

// Substitute returns the family of a font that has the same metrics as the
// font of the family, such as Carlito for Calibri, and true. It returns
// false when it knows no such font. The case of family does not matter.
//
// The families that it returns are the fonts of the module
// github.com/xo/ivy/fonts, which a program gives with ivy.Config.Fonts. A
// format uses a substitute only when the document does not embed the font,
// and only when Config.Fonts holds the substitute (D24).
func Substitute(family string) (string, bool) {
	s, ok := substitutes[strings.ToLower(strings.TrimSpace(family))]
	return s, ok
}
