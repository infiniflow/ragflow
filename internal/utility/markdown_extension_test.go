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

import "testing"

// TestMarkdownUploadAliases verifies that supported Markdown aliases retain
// Markdown parser selection and pass the document upload gate.
func TestMarkdownUploadAliases(t *testing.T) {
	for _, name := range []string{"notes.markdown", "NOTES.MARKDOWN", "folder/notes.markdown", "notes.md", "notes.mdx"} {
		t.Run(name, func(t *testing.T) {
			if got := GetFileType(name); got != FileTypeMarkdown {
				t.Fatalf("parser classifies %q as %q, want Markdown", name, got)
			}
			if got := FilenameType(name); got != FileTypeDOC {
				t.Fatalf("upload gate classifies %q as %q, want document", name, got)
			}
		})
	}
}

// TestUnsupportedFilenameRemainsRejected verifies that executable filenames and
// unsupported Markdown-like suffixes are rejected by the upload classifier.
func TestUnsupportedFilenameRemainsRejected(t *testing.T) {
	for _, name := range []string{"payload.exe", "notes.markdown.exe", "notes.markdownx"} {
		if got := FilenameType(name); got != FileTypeOTHER {
			t.Errorf("FilenameType(%q) = %q, want other", name, got)
		}
	}
}
