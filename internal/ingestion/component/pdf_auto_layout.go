//go:build cgo

//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//

package component

import (
	"go.uber.org/zap"

	"ragflow/internal/common"
	pdfoxide "ragflow/internal/deepdoc/parser/pdf/pdfoxide"
	"ragflow/internal/ingestion/component/schema"
)

// resolveAutoPDFLayoutRecognizer probes the PDF text layer and picks the text or
// scanned parser configured for layout_recognize Auto (issue #19886).
func resolveAutoPDFLayoutRecognizer(setup schema.ParserSetup, filename string, binary []byte) (string, error) {
	textParser := getStringOr(setup, parserConfigAutoTextKey, defaultAutoTextLayout)
	scannedParser := getStringOr(setup, parserConfigAutoScannedKey, defaultAutoScannedLayout)
	minChars := autoMinCharsFromSetup(setup)
	avg, err := averageCharsOverWindows(binary, autoProbePageWindows(setup))
	if err != nil {
		return "", err
	}
	hasText := avg >= float64(max(1, minChars))
	chosen := chooseAutoPDFLayout(avg, minChars, textParser, scannedParser)
	common.Info("Auto PDF parser selection",
		zap.String("filename", filename),
		zap.Float64("avg_chars_per_page", avg),
		zap.Int("threshold", minChars),
		zap.Bool("has_text_layer", hasText),
		zap.String("chosen", chosen))
	return chosen, nil
}

func averageCharsOverWindows(binary []byte, windows [][2]int) (float64, error) {
	if len(windows) == 1 {
		return pdfoxide.AverageNonWhitespaceCharsPerPage(binary, windows[0][0], windows[0][1])
	}
	var weighted, pages float64
	for _, w := range windows {
		avg, err := pdfoxide.AverageNonWhitespaceCharsPerPage(binary, w[0], w[1])
		if err != nil {
			return 0, err
		}
		if w[1] < 0 {
			return avg, nil
		}
		n := float64(w[1] - w[0])
		if n <= 0 {
			continue
		}
		weighted += avg * n
		pages += n
	}
	if pages == 0 {
		return 0, nil
	}
	return weighted / pages, nil
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
	return applyChosenAutoPDFLayout(setup, chosen), nil
}
