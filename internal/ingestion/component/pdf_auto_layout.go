//go:build cgo

//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//

package component

import (
	"fmt"
	"strings"

	"go.uber.org/zap"

	"ragflow/internal/common"
	pdfoxide "ragflow/internal/deepdoc/parser/pdf/pdfoxide"
	"ragflow/internal/ingestion/component/schema"
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
)

func isAutoLayoutRecognize(value string) bool {
	return strings.TrimSpace(strings.ToLower(value)) == layoutRecognizeAuto
}

func pdfLayoutRecognizerFromSetup(setup schema.ParserSetup) string {
	if v := getStringOr(setup, parserConfigLayoutRecognizerKey, ""); v != "" {
		return v
	}
	return getStringOr(setup, parserConfigLayoutRecognizeKey, "")
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
	avg, err := pdfoxide.AverageNonWhitespaceCharsPerPage(binary, 0, -1)
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
	return updated, nil
}

func parsePositiveInt(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("empty")
	}
	n := 0
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("not an integer")
		}
		n = n*10 + int(ch-'0')
	}
	if n <= 0 {
		return 0, fmt.Errorf("not positive")
	}
	return n, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
