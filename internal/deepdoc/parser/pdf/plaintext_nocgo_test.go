//go:build !cgo

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

package pdf

import (
	"bytes"
	"fmt"
	"testing"
)

// TestPlainText_EmptyInput locks the empty-input contract: a nil/empty
// payload yields (nil, 0, nil) rather than an error, so the caller's
// empty-PDF short-circuit stays correct under the pure-Go engine.
func TestPlainText_EmptyInput(t *testing.T) {
	items, pageCount, err := PlainText(nil)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if items != nil || pageCount != 0 {
		t.Fatalf("got (items=%v, pageCount=%d), want (nil, 0)", items, pageCount)
	}
}

// TestPlainText_GarbageReturnsError locks that non-PDF input fails loudly
// instead of silently producing empty items — the pure-Go ledongthuc/pdf
// path must surface a parse error, never a plausible empty success.
func TestPlainText_GarbageReturnsError(t *testing.T) {
	_, _, err := PlainText([]byte("this is not a pdf"))
	if err == nil {
		t.Fatal("expected error for non-PDF input, got nil")
	}
}

// TestPlainText_NegativePageCountDoesNotPanic locks the Blocker from the PR
// review: ledongthuc/pdf's NumPage() reads the catalog /Count verbatim, so a
// malformed (negative) /Count yields a negative page count. The old code fed
// that straight into make([]map[string]any, 0, pageCount), which panics with
// "makeslice: cap out of range". PlainText must validate the page count and
// fail loud instead of crashing the ingestor on a single corrupt PDF.
func TestPlainText_NegativePageCountDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PlainText panicked on a negative /Count: %v", r)
		}
	}()
	data := buildPDFWithCount(t, -1)
	_, _, err := PlainText(data)
	if err == nil {
		t.Fatal("expected error for a PDF with a negative /Count, got nil")
	}
}

// TestPlainText_HugePageCountDoesNotHang locks the sibling failure mode of the
// same Blocker: an absurd /Count must not allocate a gigantic slice or loop
// for billions of iterations. We cap the page count and reject grossly
// implausible values up front.
func TestPlainText_HugePageCountDoesNotHang(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PlainText panicked on a huge /Count: %v", r)
		}
	}()
	data := buildPDFWithCount(t, 1<<30)
	_, _, err := PlainText(data)
	if err == nil {
		t.Fatal("expected error for a PDF with a huge /Count, got nil")
	}
}

// buildPDFWithCount emits a minimal but structurally valid PDF whose /Pages
// /Count is set to the supplied value. Offsets are computed so the xref table
// is self-consistent and ledongthuc/pdf can open it, letting NumPage() observe
// the (possibly malformed) /Count.
func buildPDFWithCount(t *testing.T, count int) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")

	off1 := buf.Len()
	fmt.Fprintf(&buf, "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	off2 := buf.Len()
	fmt.Fprintf(&buf, "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count %d >>\nendobj\n", count)
	off3 := buf.Len()
	fmt.Fprintf(&buf, "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> >>\nendobj\n")

	xrefStart := buf.Len()
	buf.WriteString("xref\n0 4\n")
	buf.WriteString("0000000000 65535 f \n")
	fmt.Fprintf(&buf, "%010d 00000 n \n", off1)
	fmt.Fprintf(&buf, "%010d 00000 n \n", off2)
	fmt.Fprintf(&buf, "%010d 00000 n \n", off3)
	fmt.Fprintf(&buf, "trailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefStart)
	return buf.Bytes()
}
