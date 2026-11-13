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

package component

import (
	"testing"

	"ragflow/internal/utility"
)

func TestParserFileFamilyRegisteredOfficeExtensions(t *testing.T) {
	for _, tc := range []struct {
		ext    string
		ft     utility.FileType
		family string
	}{
		{"officeword", utility.FileTypeDOC, "doc"},
		{"officewordxml", utility.FileTypeDOCX, "docx"},
		{"officebook", utility.FileTypeXLS, "spreadsheet"},
		{"officebookxml", utility.FileTypeXLSX, "spreadsheet"},
		{"officedeck", utility.FileTypePPT, "slides"},
		{"officedeckxml", utility.FileTypePPTX, "slides"},
	} {
		t.Run(tc.ext, func(t *testing.T) {
			utility.RegisterExtensionFileType(tc.ext, tc.ft)
			t.Cleanup(func() { utility.RegisterExtensionFileType(tc.ext, utility.FileTypeOTHER) })
			if got := ParserFileFamily(tc.ext); got != tc.family {
				t.Errorf("ParserFileFamily(%q) = %q, want %q", tc.ext, got, tc.family)
			}
			if got := fileTypeFromInputs(map[string]any{"name": "sample." + tc.ext}); got != tc.ft {
				t.Errorf("file type = %q, want %q", got, tc.ft)
			}
		})
	}
}
