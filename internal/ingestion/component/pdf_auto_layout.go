//go:build cgo

//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//

package component

import (
	"strconv"
	"strings"

	"go.uber.org/zap"

	"ragflow/internal/common"
	pdfoxide "ragflow/internal/deepdoc/parser/pdf/pdfoxide"
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
	if v := getStringOr(setup, parserConfigLayoutRecognizerKey, ""); v != "" {
		return v
	}
	if v := getStringOr(setup, parserConfigLayoutRecognizeKey, ""); v != "" {
		return v
	}
	return getStringOr(setup, parserConfigParseMethodKey, "")
}

func autoProbePageWindow(setup schema.ParserSetup) (fromPage, toPage int) {
	ranges, err := utility.NormalizePDFPages(setup["pages"])
	if err != nil || len(ranges) == 0 {
		return 0, -1
	}
	// NormalizePDFPages is 1-indexed inclusive; AverageNonWhitespaceCharsPerPage
	// is 0-indexed [from, to). Use the span of the requested ranges.
	return ranges[0][0] - 1, ranges[len(ranges)-1][1]
}

// resolveAutoPDFLayoutRecognizer probes the PDF text layer and picks the text or
// scanned parser configured for layout_recognize Auto (issue #19886).
func resolveAutoPDFLayoutRecognizer(setup schema.ParserSetup, filename string, binary []byte) (string, error) {
	textParser := getStringOr(setup, parserConfigAutoTextKey, defaultAutoTextLayout)
	scannedParser := getStringOr(setup, parserConfigAutoScannedKey, defaultAutoScannedLayout)
	minChars := defaultAutoMinCharsPerPage
	if raw := getStringOr(setup, parserConfigAutoMinCharsKey, ""); raw != "" {
		if n, err := parsePositiveInt(raw); err == nil {
			minChars = n
		}
	} else if v, ok := setup[parserConfigAutoMinCharsKey]; ok {
		switch n := v.(type) {
		case int:
			if n > 0 {
				minChars = n
			}
		case float64:
			if n > 0 {
				minChars = int(n)
			}
		}
	}
	fromPage, toPage := autoProbePageWindow(setup)
	avg, err := pdfoxide.AverageNonWhitespaceCharsPerPage(binary, fromPage, toPage)
	if err != nil {
		return "", err
	}
	hasText := avg >= float64(max(1, minChars))
	chosen := textParser
	if !hasText {
		chosen = scannedParser
	}
	common.Info("Auto PDF parser selection",
		zap.String("filename", filename),
		zap.Float64("avg_chars_per_page", avg),
		zap.Int("threshold", minChars),
		zap.Bool("has_text_layer", hasText),
		zap.String("chosen", chosen))
	return chosen, nil
}

func maybeResolveAutoPDFLayout(setup schema.ParserSetup, filename string, binary []byte) (schema.ParserSetup, error) {
	layout := pdfLayoutRecognizerFromSetup(setup)
	if !isAutoLayoutRecognize(layout) {
		return setup, nil
	}
	chosen, err := resolveAutoPDFLayoutRecognizer(setup, filename, binary)
	if err != nil {
		return setup, err
	}
	updated := make(schema.ParserSetup, len(setup)+1)
	for k, v := range setup {
		updated[k] = v
	}
	updated[parserConfigLayoutRecognizerKey] = chosen
	if isAutoLayoutRecognize(getStringOr(setup, parserConfigParseMethodKey, "")) {
		updated[parserConfigParseMethodKey] = chosen
	}
	return updated, nil
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
