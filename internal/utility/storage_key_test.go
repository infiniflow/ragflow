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

package utility

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestStorageObjectName(t *testing.T) {
	longCJK := strings.Repeat("界", 100) + ".pdf"
	longASCII := strings.Repeat("a", 300) + ".txt"

	cases := []struct {
		name       string
		filename   string
		attempt    int
		maxBytes   int
		want       string
		checkWant  bool
		wantSuffix string
		wantPart   string
	}{
		{name: "short ascii unchanged", filename: "deck.pptx", maxBytes: 255, want: "deck.pptx", checkWant: true},
		{name: "long ascii keeps extension", filename: longASCII, maxBytes: 255, wantSuffix: ".txt"},
		{name: "long cjk keeps extension", filename: longCJK, maxBytes: 255, wantSuffix: ".pdf"},
		{name: "collision suffix stays in budget", filename: longCJK, attempt: 1, maxBytes: 255, wantSuffix: ".pdf", wantPart: "(1).pdf"},
		{name: "no extension", filename: strings.Repeat("界", 100), maxBytes: 255},
		{name: "extension larger than budget", filename: "file.pdf", maxBytes: 4, want: "file", checkWant: true},
		{name: "zero budget", filename: "file.pdf", maxBytes: 0, want: "", checkWant: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := StorageObjectName(tc.filename, tc.attempt, tc.maxBytes)
			if tc.checkWant && got != tc.want {
				t.Fatalf("StorageObjectName(%q, %d, %d) = %q, want %q", tc.filename, tc.attempt, tc.maxBytes, got, tc.want)
			}
			if tc.maxBytes > 0 && len(got) > tc.maxBytes {
				t.Fatalf("StorageObjectName(%q, %d, %d) = %q (%d bytes), want <= %d bytes", tc.filename, tc.attempt, tc.maxBytes, got, len(got), tc.maxBytes)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("StorageObjectName(%q, %d, %d) = %q, want valid UTF-8", tc.filename, tc.attempt, tc.maxBytes, got)
			}
			if tc.wantSuffix != "" && !strings.HasSuffix(got, tc.wantSuffix) {
				t.Fatalf("StorageObjectName(%q, %d, %d) = %q, want suffix %q", tc.filename, tc.attempt, tc.maxBytes, got, tc.wantSuffix)
			}
			if tc.wantPart != "" && !strings.Contains(got, tc.wantPart) {
				t.Fatalf("StorageObjectName(%q, %d, %d) = %q, want %q", tc.filename, tc.attempt, tc.maxBytes, got, tc.wantPart)
			}
		})
	}
}
