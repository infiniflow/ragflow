//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//

package component

import (
	"strconv"
	"strings"

	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/utility"
)

const (
	layoutRecognizeAuto             = "auto"
	defaultAutoTextLayout           = "DeepDOC"
	defaultAutoScannedLayout        = "DeepDOC"
	defaultAutoMinCharsPerPage      = 100
	parserConfigAutoTextKey         = "layout_recognize_auto_text"
	parserConfigAutoScannedKey      = "layout_recognize_auto_scanned"
	parserConfigAutoMinCharsKey     = "layout_recognize_auto_min_chars_per_page"
	parserConfigLayoutRecognizeKey  = "layout_recognize"
	parserConfigLayoutRecognizerKey = "layout_recognizer"
	parserConfigParseMethodKey      = "parse_method"
)

func isAutoLayoutRecognize(value string) bool {
	return strings.TrimSpace(strings.ToLower(value)) == layoutRecognizeAuto
}

func pdfLayoutRecognizerFromSetup(setup schema.ParserSetup) string {
	recognizer := getStringOr(setup, parserConfigLayoutRecognizerKey, "")
	recognize := getStringOr(setup, parserConfigLayoutRecognizeKey, "")
	parseMethod := getStringOr(setup, parserConfigParseMethodKey, "")
	// Auto is a meta-selector. Prefer it on any of the three keys so a leftover
	// concrete layout_recognizer cannot suppress parse_method/layout_recognize Auto.
	if isAutoLayoutRecognize(recognize) {
		return recognize
	}
	if isAutoLayoutRecognize(parseMethod) {
		return parseMethod
	}
	if isAutoLayoutRecognize(recognizer) {
		return recognizer
	}
	if recognizer != "" {
		return recognizer
	}
	if recognize != "" {
		return recognize
	}
	return parseMethod
}

func autoProbePageWindows(setup schema.ParserSetup) [][2]int {
	ranges, err := utility.NormalizePDFPages(setup["pages"])
	if err != nil || len(ranges) == 0 {
		return [][2]int{{0, -1}}
	}
	windows := make([][2]int, 0, len(ranges))
	for _, r := range ranges {
		// NormalizePDFPages is 1-indexed inclusive; AverageNonWhitespaceCharsPerPage
		// is 0-indexed [from, to). Probe each requested range, not the span between them.
		windows = append(windows, [2]int{r[0] - 1, r[1]})
	}
	return windows
}

func autoMinCharsFromSetup(setup schema.ParserSetup) int {
	minChars := defaultAutoMinCharsPerPage
	if raw := getStringOr(setup, parserConfigAutoMinCharsKey, ""); raw != "" {
		if n, err := parsePositiveInt(raw); err == nil {
			return n
		}
	}
	if v, ok := setup[parserConfigAutoMinCharsKey]; ok {
		switch n := v.(type) {
		case int:
			if n > 0 {
				return n
			}
		case float64:
			if n > 0 {
				return int(n)
			}
		}
	}
	return minChars
}

func chooseAutoPDFLayout(avg float64, minChars int, textParser, scannedParser string) string {
	if avg >= float64(max(1, minChars)) {
		return textParser
	}
	return scannedParser
}

func applyChosenAutoPDFLayout(setup schema.ParserSetup, chosen string) schema.ParserSetup {
	updated := make(schema.ParserSetup, len(setup)+2)
	for k, v := range setup {
		updated[k] = v
	}
	updated[parserConfigLayoutRecognizerKey] = chosen
	updated[parserConfigParseMethodKey] = chosen
	if isAutoLayoutRecognize(getStringOr(setup, parserConfigLayoutRecognizeKey, "")) {
		updated[parserConfigLayoutRecognizeKey] = chosen
	}
	return updated
}

func parsePositiveInt(raw string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, err
	}
	if n <= 0 {
		return 0, strconv.ErrRange
	}
	return n, nil
}
