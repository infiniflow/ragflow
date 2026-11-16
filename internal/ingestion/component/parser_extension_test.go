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

func TestParserFileFamilyRegisteredExtensions(t *testing.T) {
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
		{"drawingfile", utility.FileType("drawing"), "drawing"},
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

func TestParserFamilyUsesResolvedFileType(t *testing.T) {
	type registration struct {
		extension string
		fileType  utility.FileType
	}
	for _, tc := range []struct {
		name          string
		registrations []registration
		extension     string
		fileType      utility.FileType
		family        string
	}{
		{"chain", []registration{{"familyalias", "familymiddle"}, {"familymiddle", utility.FileTypeDOCX}}, "familyalias", "familymiddle", "familymiddle"},
		{"self reference", []registration{{"familyself", "familyself"}}, "familyself", "familyself", "familyself"},
		{"cycle", []registration{{"familyleft", "familyright"}, {"familyright", "familyleft"}}, "familyleft", "familyright", "familyright"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, entry := range tc.registrations {
				utility.RegisterExtensionFileType(entry.extension, entry.fileType)
				t.Cleanup(func() { utility.RegisterExtensionFileType(entry.extension, utility.FileTypeOTHER) })
			}
			if got := fileTypeFromInputs(map[string]any{"name": "sample." + tc.extension}); got != tc.fileType {
				t.Fatalf("selected parser type = %q, want %q", got, tc.fileType)
			}
			if got := ParserFileFamily(tc.extension); got != tc.family {
				t.Errorf("upload family = %q, want %q", got, tc.family)
			}
			if got := resolveParserFamily(tc.fileType); got != tc.family {
				t.Errorf("runtime family = %q, want %q", got, tc.family)
			}
		})
	}
}

func TestParserFileFamilyHints(t *testing.T) {
	for _, tc := range []struct{ hint, family string }{
		{"slides", "slides"}, {"pages", "slides"}, {"spreadsheet", "spreadsheet"},
		{"text&code", "text&code"}, {"image", "image"}, {"visual", "image"},
		{"audio", "audio"}, {"aural", "audio"}, {"video", "video"},
		{"unrecognized-family", ""}, {"", ""},
	} {
		t.Run(tc.hint, func(t *testing.T) {
			if got := ParserFileFamily(tc.hint); got != tc.family {
				t.Errorf("ParserFileFamily(%q) = %q, want %q", tc.hint, got, tc.family)
			}
		})
	}
}
