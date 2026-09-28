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
	"fmt"

	officeOxide "github.com/yfedoseev/office_oxide/go"
)

// OpenAndExtract opens an Office document with the native office_oxide
// backend and returns its three text views: the structured IR JSON, the
// Markdown rendering, and the raw plain text. The only place that depends on
// office_oxide lives here, so the parser package can stay CGO-free and own
// only the format-agnostic post-processing.
//
// ToIRJSON / ToMarkdown / PlainText failures are non-fatal: the corresponding
// view is returned empty and the parser package's salvage logic (e.g. picking
// the longest non-empty view, or falling back to plain text) handles it. Only
// a failed OpenFromBytes is reported as an error.
func OpenAndExtract(data []byte, format string) (irJSON, markdown, plainText string, err error) {
	doc, err := officeOxide.OpenFromBytes(data, format)
	if err != nil {
		return "", "", "", fmt.Errorf("office_oxide open %s: %w", format, err)
	}
	defer doc.Close()
	irJSON, _ = doc.ToIRJSON()
	markdown, _ = doc.ToMarkdown()
	plainText, _ = doc.PlainText()
	return irJSON, markdown, plainText, nil
}
