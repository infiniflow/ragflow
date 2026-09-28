//go:build !cgo

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
	"errors"
	"testing"
)

// TestXLSParser_BIFFRequiresCGO pins the !cgo fallback for genuine
// BIFF8/OLE .xls input. Without cgo, office_oxide is unavailable, so
// the parser must surface an ErrOfficeCGORequired-wrapped error with a
// "convert to XLSX or enable CGO" hint instead of routing through
// Excelize (which only handles OOXML — an OLE payload would surface as
// a zip-format error and confuse callers).
func TestXLSParser_BIFFRequiresCGO(t *testing.T) {
	p, err := NewXLSParser("")
	if err != nil {
		t.Fatalf("NewXLSParser: %v", err)
	}

	// Minimal OLE Compound Document header.
	data := []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
	res := p.ParseWithResult(t.Context(), "legacy.xls", data)
	if res.Err == nil {
		t.Fatal("expected error for BIFF input without cgo, got nil")
	}
	if !errors.Is(res.Err, ErrOfficeCGORequired) {
		t.Fatalf("Err = %v, want it to wrap ErrOfficeCGORequired", res.Err)
	}
	if got := res.File["format"]; got != "xls" {
		t.Fatalf("File[format] = %v, want %q", got, "xls")
	}
}
