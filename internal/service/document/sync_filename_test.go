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

package document

import (
	"strings"
	"testing"
	"unicode/utf8"

	"ragflow/internal/entity"
)

func TestSyncDocumentFilenamePreservesUTF8(t *testing.T) {
	for _, tc := range []struct {
		name, extension, fallback, want string
	}{
		{"a" + strings.Repeat("я", 130) + ".xlsx", ".xlsx", "", "a" + strings.Repeat("я", 124) + ".xlsx"},
		{"a" + strings.Repeat("я", 130), "docx", "", "a" + strings.Repeat("я", 124) + ".docx"},
		{strings.Repeat("文", 100), ".txt", "", strings.Repeat("文", 83) + ".txt"},
		{strings.Repeat("📄", 100), ".pdf", "", strings.Repeat("📄", 62) + ".pdf"},
		{"", ".xlsx", "a" + strings.Repeat("я", 130), "a" + strings.Repeat("я", 124) + ".xlsx"},
		{"Отчёт.XLSX", "xlsx", "", "Отчёт.XLSX"},
		{strings.Repeat("a", 260), ".txt", "", strings.Repeat("a", 251) + ".txt"},
		{"Отчёт", "." + strings.Repeat("я", 130), "", "Отчёт." + strings.Repeat("я", 122)},
	} {
		got := syncDocumentFilename(tc.name, tc.extension, tc.fallback)
		if got != tc.want || !utf8.ValidString(got) || len(got) > 255 {
			t.Errorf("filename(%q, %q) = %q (%d bytes), want %q", tc.name, tc.extension, got, len(got), tc.want)
		}
	}
}

func TestSyncDocumentStoresUTF8Filename(t *testing.T) {
	svc, db, _, input := setupSyncDocumentTest(t, false)
	input.SourceDocument.SemanticIdentifier = "a" + strings.Repeat("я", 130) + ".xlsx"
	input.SourceDocument.Extension = ".xlsx"
	if _, err := svc.Upsert(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	var doc entity.Document
	if err := db.Where("id = ?", input.DocumentID).Take(&doc).Error; err != nil {
		t.Fatal(err)
	}
	want := "a" + strings.Repeat("я", 124) + ".xlsx"
	if doc.Name == nil || *doc.Name != want || !utf8.ValidString(*doc.Name) {
		t.Fatalf("stored document name = %v, want valid UTF-8 %q", doc.Name, want)
	}
}
