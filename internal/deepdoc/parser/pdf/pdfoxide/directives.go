package pdfoxide

import (
	"bytes"
	"strconv"
)

// pageDirectives carries the two page attributes the oxide extraction path
// needs but pdf_oxide does not expose in post-rotation space.
type pageDirectives struct {
	rotate  int
	cropBox [4]float64
	hasCrop bool
}

// scanPageDirectives collects /Rotate and /CropBox for every page in a single
// pass over the raw PDF bytes. Both values are keyed the way the per-page
// scanners keyed them — the nth occurrence carrying a parseable value is page
// n — so callers observe identical results. A non-numeric /Rotate or a
// malformed /CropBox is skipped without consuming a page index.
func scanPageDirectives(data []byte) map[int]pageDirectives {
	out := make(map[int]pageDirectives)
	rotPage, cropPage := 0, 0
	for i := 0; i < len(data); i++ {
		// The explicit bound keeps the previous scanners' tail rule: a key
		// starting in the last len(key)-1 bytes is not matched. bytes.HasPrefix
		// alone would accept a key that ends exactly at EOF.
		if i+len("/Rotate") < len(data) && bytes.HasPrefix(data[i:], []byte("/Rotate")) {
			val, next, ok := parseRotationValueAt(data, i+len("/Rotate"))
			if ok {
				d := out[rotPage]
				d.rotate = val
				out[rotPage] = d
				rotPage++
			}
			i = next - 1
			continue
		}
		if i+len("/CropBox") < len(data) && bytes.HasPrefix(data[i:], []byte("/CropBox")) {
			box, next, ok := parseCropBoxAt(data, i+len("/CropBox"))
			if ok {
				d := out[cropPage]
				d.cropBox = box
				d.hasCrop = true
				out[cropPage] = d
				cropPage++
			}
			i = next - 1
			continue
		}
	}
	return out
}

// parseRotationValueAt parses the integer that follows a /Rotate key at pos.
// It returns the value, the position just past it, and whether a value was
// found. Whitespace between the key and the value is tolerated.
func parseRotationValueAt(data []byte, pos int) (int, int, bool) {
	for pos < len(data) && isSpace(data[pos]) {
		pos++
	}
	val, digits := 0, 0
	for pos < len(data) && data[pos] >= '0' && data[pos] <= '9' {
		val = val*10 + int(data[pos]-'0')
		pos++
		digits++
	}
	if digits == 0 {
		return 0, pos, false
	}
	return val, pos, true
}

// parseCropBoxAt parses "[x0 y0 x1 y1]" after a /CropBox key at pos. It returns
// the box, the position just past it, and whether a complete box was found.
func parseCropBoxAt(data []byte, pos int) ([4]float64, int, bool) {
	var box [4]float64
	for pos < len(data) && isSpace(data[pos]) {
		pos++
	}
	if pos >= len(data) || data[pos] != '[' {
		return box, pos, false
	}
	pos++
	for i := 0; i < 4; i++ {
		// parseFloat skips leading whitespace itself and reports how far it
		// consumed, so no separate skip is needed here.
		v, n := parseFloat(data[pos:])
		if n == 0 {
			return box, pos, false
		}
		box[i] = v
		pos += n
	}
	return box, pos, true
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// parseFloat parses a decimal number from the beginning of s.
// Returns the value and the number of bytes consumed (0 on failure).
func parseFloat(s []byte) (float64, int) {
	i := 0
	for i < len(s) && isSpace(s[i]) {
		i++
	}
	j := i
	// Scan: optional sign, digits, optional decimal point + digits
	if j < len(s) && (s[j] == '+' || s[j] == '-') {
		j++
	}
	hasDigit := false
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
		hasDigit = true
	}
	if j < len(s) && s[j] == '.' {
		j++
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
			hasDigit = true
		}
	}
	if !hasDigit || j == i {
		return 0, 0
	}
	v, err := strconv.ParseFloat(string(s[i:j]), 64)
	if err != nil {
		return 0, 0
	}
	return v, j
}
