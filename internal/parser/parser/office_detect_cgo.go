//go:build cgo

//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package parser

// officeContainer reports the on-disk container format of an Office
// document by sniffing its leading magic bytes. It returns "ooxml" for
// the ZIP-based OOXML family (docx/xlsx/pptx), "ole" for the legacy
// binary OLE family (doc/xls/ppt), or "" when the container cannot be
// determined from the first bytes. The return values are internal
// markers — callers map them to the concrete office_oxide format strings
// ("docx"→"doc", "pptx"→"ppt"); this avoids coupling the detector to a
// single family and keeps it reusable for xlsx/xls if needed later.
//
// The office_oxide backend's OpenFromBytes requires the caller to name
// the container format and does no magic-byte detection of its own, so
// a legacy file renamed to an OOXML extension (e.g. a .doc uploaded as
// .docx) is otherwise parsed as ZIP and fails with OFFICE_ERR_PARSE.
// Sniffing here lets the cgo parsers pass the real container format.
func officeContainer(data []byte) string {
	if len(data) >= 4 && data[0] == 0x50 && data[1] == 0x4B && data[2] == 0x03 && data[3] == 0x04 {
		return "ooxml"
	}
	// OLE Compound Document signature: D0 CF 11 E0 A1 B1 1A E1. Require
	// the full eight bytes to avoid misclassifying truncated input
	// (a four-byte prefix could also be garbage).
	if len(data) >= 8 && data[0] == 0xD0 && data[1] == 0xCF && data[2] == 0x11 && data[3] == 0xE0 &&
		data[4] == 0xA1 && data[5] == 0xB1 && data[6] == 0x1A && data[7] == 0xE1 {
		return "ole"
	}
	return ""
}
