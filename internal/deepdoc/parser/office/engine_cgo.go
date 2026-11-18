//go:build cgo

// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package office

import (
	"errors"
	"fmt"

	officeOxide "github.com/yfedoseev/office_oxide/go"
)

// OpenAndExtract opens an Office document with the native office_oxide
// backend and returns its three text views: the structured IR JSON, the
// Markdown rendering, and the raw plain text. The only place that depends on
// office_oxide lives here, so the parser package can stay CGO-free and own
// only the format-agnostic post-processing.
//
// A single failing view is non-fatal: the parser package salvages the
// surviving views (e.g. the DOCX JSON path errors on empty IR, the Markdown
// path prefers the Markdown view, the DOC path picks the longest view). mdErr
// is returned separately so the Markdown path can fail loud when only the
// Markdown view fails. If EVERY view fails there is nothing to recover, so err
// is set to the joined view errors instead of three empty strings that
// downstream code would silently treat as "empty document" (PR review
// Finding 1).
func OpenAndExtract(data []byte, format string) (irJSON, markdown, plainText string, mdErr error, err error) {
	doc, openErr := officeOxide.OpenFromBytes(data, format)
	if openErr != nil {
		return "", "", "", nil, fmt.Errorf("office_oxide open %s: %w", format, openErr)
	}
	defer doc.Close()

	irJSON, irErr := doc.ToIRJSON()
	markdown, mdErr = doc.ToMarkdown()
	plainText, ptErr := doc.PlainText()

	// Every view failed: nothing to salvage, so report the combined failures
	// rather than returning three empty strings that downstream code would
	// silently treat as an empty document (PR review Finding 1).
	if irJSON == "" && markdown == "" && plainText == "" {
		return "", "", "", mdErr, fmt.Errorf(
			"office_oxide %s: all text views failed: %w",
			format, errors.Join(irErr, mdErr, ptErr),
		)
	}
	return irJSON, markdown, plainText, mdErr, nil
}
