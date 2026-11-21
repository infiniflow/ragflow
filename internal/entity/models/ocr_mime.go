// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package models

import (
	"bytes"
	"mime"
	"net/http"
	"strings"
)

// ocrPDFHeaderWindow is how far into the file a PDF header may start;
// PDF readers accept leading bytes before %PDF- within the first kilobyte.
const ocrPDFHeaderWindow = 1024

// ocrMIMEType returns the bare media type of content for the OCR data
// URL. Image signatures are trusted first; otherwise PDFs are recognised even
// when bytes precede the header, which content sniffing alone reports as
// text/plain.
func ocrMIMEType(content []byte) string {
	mediaType, _, err := mime.ParseMediaType(http.DetectContentType(content))
	if err == nil && strings.HasPrefix(mediaType, "image/") {
		return mediaType
	}
	if bytes.Contains(content[:min(len(content), ocrPDFHeaderWindow)], []byte("%PDF-")) {
		return "application/pdf"
	}
	if err != nil {
		return "application/octet-stream"
	}
	return mediaType
}
