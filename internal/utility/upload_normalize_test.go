//
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
//

package utility

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestNormalizeUploadInfoContentPreservesPDF(t *testing.T) {
	data := pdfWithHTMLText()
	for _, tt := range []struct {
		name        string
		filename    string
		contentType string
		wantName    string
	}{
		{"pdf MIME", "report.pdf", "application/pdf", "report.pdf"},
		{"MIME parameters", "report.pdf", "Application/PDF; version=1.7", "report.pdf"},
		{"missing extension", "report", "application/pdf", "report.pdf"},
		{"uppercase extension", "report.PDF", "application/pdf", "report.PDF"},
		{"HTML MIME with PDF magic", "download", "text/html", "download.pdf"},
		{"binary MIME with PDF magic", "download", "application/octet-stream", "download.pdf"},
		{"sniffed PDF", "download", "", "download.pdf"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			original := bytes.Clone(data)
			name, contentType, got := NormalizeUploadInfoContent(tt.filename, tt.contentType, original)
			if name != tt.wantName {
				t.Errorf("filename = %q, want %q", name, tt.wantName)
			}
			if contentType != "application/pdf" {
				t.Errorf("content type = %q, want application/pdf", contentType)
			}
			if !bytes.Equal(got, data) {
				t.Errorf("PDF bytes changed: got %d bytes, want %d", len(got), len(data))
			}
		})
	}
}

func TestNormalizeUploadInfoContentStillConvertsHTML(t *testing.T) {
	data := []byte("<html><body><p>hello</p><div>world</div></body></html>")
	for _, contentType := range []string{"text/html", "application/xhtml+xml", "application/octet-stream", ""} {
		t.Run(contentType, func(t *testing.T) {
			name, gotType, got := NormalizeUploadInfoContent("page.html", contentType, data)
			if name != "page.html" {
				t.Errorf("filename = %q, want page.html", name)
			}
			wantType := contentType
			if wantType == "" {
				wantType = "text/html; charset=utf-8"
			}
			if gotType != wantType {
				t.Errorf("content type = %q, want %q", gotType, wantType)
			}
			if bytes.Contains(got, []byte("<")) || !bytes.Contains(got, []byte("hello")) || !bytes.Contains(got, []byte("world")) {
				t.Errorf("HTML was not converted to readable text: %q", got)
			}
		})
	}
}

// pdfWithHTMLText builds a one-page PDF with valid stream lengths and xref
// offsets. HTML markers are literal PDF text, not an HTML response body.
func pdfWithHTMLText() []byte {
	stream := "BT /F1 12 Tf 20 40 Td (<html><body><div>example</div></body></html>) Tj ET\n"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var pdf strings.Builder
	pdf.WriteString("%PDF-1.7\n")
	offsets := make([]int, len(objects))
	for i, object := range objects {
		offsets[i] = pdf.Len()
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return []byte(pdf.String())
}
