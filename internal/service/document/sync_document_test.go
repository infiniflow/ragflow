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

package document

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestSyncDocumentFilenameKeepsUTF8WhenTruncating verifies a filename longer
// than 255 bytes is shortened on a character boundary, so a multi-byte name
// never ends in a partial character.
func TestSyncDocumentFilenameKeepsUTF8WhenTruncating(t *testing.T) {
	got := syncDocumentFilename(strings.Repeat("数", 100), ".txt", "source-id")
	if !utf8.ValidString(got) {
		t.Fatalf("filename %q is not valid UTF-8", got)
	}
	if want := strings.Repeat("数", 83) + ".txt"; got != want {
		t.Fatalf("filename = %q, want %q", got, want)
	}
}
