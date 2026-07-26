package pdfoxide

import "strconv"

// parseCropBoxFromRaw scans raw PDF bytes for /CropBox entries and
// returns the array [x0, y0, x1, y1] for the given page index (0-based).
// The second return value is false if no /CropBox was found.
//
// Algorithm: sequential scan of "/CropBox [...]" patterns — same approach
// as parsePageRotationFromRaw.  Works for all common PDF generators.
func parseCropBoxFromRaw(data []byte, pageIdx int) ([4]float64, bool) {
	type cb [4]float64
	var boxes []cb
	rest := data
	for {
		idx := indexAfter(rest, "/CropBox")
		if idx < 0 {
			break
		}
		rest = rest[idx:]
		// Skip whitespace, expect '['
		for len(rest) > 0 && isSpace(rest[0]) {
			rest = rest[1:]
		}
		if len(rest) == 0 || rest[0] != '[' {
			continue
		}
		rest = rest[1:]
		// Parse 4 float values inside [...]
		var vals [4]float64
		ok := true
		for i := 0; i < 4; i++ {
			for len(rest) > 0 && isSpace(rest[0]) {
				rest = rest[1:]
			}
			v, n := parseFloat(rest)
			if n == 0 {
				ok = false
				break
			}
			vals[i] = v
			rest = rest[n:]
		}
		if !ok {
			continue
		}
		boxes = append(boxes, cb(vals))
	}
	if pageIdx < len(boxes) {
		return boxes[pageIdx], true
	}
	return [4]float64{}, false
}

// indexAfter finds the byte position right after the first occurrence of s in
// data. Returns -1 if not found.
func indexAfter(data []byte, s string) int {
	for i := 0; i < len(data)-len(s); i++ {
		match := true
		for j := 0; j < len(s); j++ {
			if data[i+j] != s[j] {
				match = false
				break
			}
		}
		if match {
			return i + len(s)
		}
	}
	return -1
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
