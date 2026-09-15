//go:build cgo

//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package parser

import (
	"fmt"

	officeOxide "github.com/yfedoseev/office_oxide/go"
)

// xlsBIFFOpenFromBytes is a test seam mirroring officeOxide.OpenFromBytes
// for the BIFF8/OLE .xls path. Overridden in tests to capture the
// effective container format without spinning up the office_oxide
// runtime on a real BIFF fixture.
var xlsBIFFOpenFromBytes = officeOxide.OpenFromBytes

// xlsOpenFromBIFF parses a genuine BIFF8/OLE .xls file via office_oxide.
// Office_oxide converts the legacy .xls container to OOXML internally, so
// the surface is PlainText wrapped as a single JSON item — the same
// salvage shape used by PPTXParser when its structured IR serialisation
// fails. Structured per-sheet items with positions are deferred to a
// follow-up that reads the converted OOXML IR.
func xlsOpenFromBIFF(data []byte, filename string) ParseResult {
	doc, err := xlsBIFFOpenFromBytes(data, "xls")
	if err != nil {
		return ParseResult{Err: fmt.Errorf("xls (biff) open: %w", err)}
	}
	defer doc.Close()

	text, err := doc.PlainText()
	if err != nil {
		return ParseResult{Err: fmt.Errorf("xls (biff) plain text: %w", err)}
	}
	return ParseResult{
		OutputFormat: "json",
		File:         map[string]any{"name": filename, "format": "xls"},
		JSON:         itemsFromPlainText(text),
	}
}
