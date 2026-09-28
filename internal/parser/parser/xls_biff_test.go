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
	"testing"
)

// TestXLSParser_BIFFRoutesToOfficeOxide pins that an OLE/BIFF8 .xls
// payload goes through the office_oxide legacy-format path instead of
// the Excelize OOXML path (#19496). The test uses the same seam pattern
// as the DOCX/PPTX container-sniffing tests: the test seam captures the
// format string passed to officeOxide.OpenFromBytes, and a minimal OLE
// header (8 bytes) is enough to trigger the BIFF branch without needing
// a full valid BIFF fixture.
func TestXLSParser_BIFFRoutesToOfficeOxide(t *testing.T) {
	ctx := t.Context()
	p, err := NewXLSParser("")
	if err != nil {
		t.Fatalf("NewXLSParser: %v", err)
	}

	orig := xlsBIFFOpenFromBytes
	defer func() { xlsBIFFOpenFromBytes = orig }()

	var gotFormat string
	xlsBIFFOpenFromBytes = func(data []byte, format string) (any, error) {
		gotFormat = format
		return nil, fmt.Errorf("stub xls biff open: %s", format)
	}

	// Minimal OLE Compound Document header: D0 CF 11 E0 A1 B1 1A E1.
	data := []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
	res := p.ParseWithResult(ctx, "legacy.xls", data)
	if res.Err == nil {
		t.Fatal("expected error from the stub seam, got nil")
	}
	if gotFormat != "xls" {
		t.Fatalf("xlsBIFFOpenFromBytes format = %q, want %q (BIFF branch not triggered)", gotFormat, "xls")
	}
	if got := res.File["format"]; got != "xls" {
		t.Fatalf("File[format] = %v, want %q", got, "xls")
	}
}

// TestXLSParser_TruncatedOLENotBIFF guards against a truncated OLE
// prefix (only 4 bytes) being misclassified as a BIFF payload. The
// officeContainer sniff requires the full 8-byte signature, so a 4-byte
// prefix should fall through to Excelize and surface as a zip-format
// error from parseXLSXBytes.
func TestXLSParser_TruncatedOLENotBIFF(t *testing.T) {
	ctx := t.Context()
	p, err := NewXLSParser("")
	if err != nil {
		t.Fatalf("NewXLSParser: %v", err)
	}

	orig := xlsBIFFOpenFromBytes
	defer func() { xlsBIFFOpenFromBytes = orig }()

	xlsBIFFOpenFromBytes = func(_ []byte, _ string) (any, error) {
		t.Fatal("BIFF branch must not fire for truncated OLE prefix")
		return nil, nil
	}

	data := []byte{0xD0, 0xCF, 0x11, 0xE0}
	res := p.ParseWithResult(ctx, "truncated.xls", data)
	if res.Err == nil {
		t.Fatal("expected error for non-spreadsheet bytes, got nil")
	}
}
