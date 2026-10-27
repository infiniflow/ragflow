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

import "testing"

// wpsRoutes maps a WPS private-format extension (without dot) to the MS-Office
// FileType it must resolve to. WPS reuses the legacy MS-OLE2/CFB binary
// structure, so .wps/.wpt are Word-compatible (DOC), .et/.ett are
// Excel-compatible (XLS), and .dps/.dpt are PowerPoint-compatible (PPT).
var wpsRoutes = []struct {
	ext  string
	want FileType
}{
	{"wps", FileTypeDOC},
	{"wpt", FileTypeDOC},
	{"et", FileTypeXLS},
	{"ett", FileTypeXLS},
	{"dps", FileTypePPT},
	{"dpt", FileTypePPT},
}

// TestGetFileType_WPS_ExactRouting verifies the precise parser-family routing
// for every WPS private extension. This is the gate that actually feeds
// parser.GetParser, so a wrong mapping silently drops the file to text mode.
func TestGetFileType_WPS_ExactRouting(t *testing.T) {
	for _, r := range wpsRoutes {
		t.Run(r.ext, func(t *testing.T) {
			got := GetFileType("sample." + r.ext)
			if got != r.want {
				t.Errorf("GetFileType(%q) = %q, want %q", "sample."+r.ext, got, r.want)
			}
		})
	}
}

// TestGetFileType_WPS_CaseInsensitiveAndPath ensures case folding and a
// leading directory do not defeat the match (mirrors how real uploads arrive).
func TestGetFileType_WPS_CaseInsensitiveAndPath(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		want     FileType
	}{
		{"uppercase", "SLIDE.DPT", FileTypePPT},
		{"mixed case", "Book.Et", FileTypeXLS},
		{"leading dir", "/data/reports/quarter.wps", FileTypeDOC},
		{"nested dir", "a/b/c/template.ett", FileTypeXLS},
		{"long path mixed case", "/X/Y/Z/DECK.Dps", FileTypePPT},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := GetFileType(tc.filename)
			if got != tc.want {
				t.Errorf("GetFileType(%q) = %q, want %q", tc.filename, got, tc.want)
			}
		})
	}
}

// TestGetFileType_WPS_NoOverMatch guards against accidentally widening the
// match: a non-WPS suffix that merely contains a WPS substring (or a doubled
// extension) must stay OTHER.
func TestGetFileType_WPS_NoOverMatch(t *testing.T) {
	cases := []string{
		"notes.wpsx", // not .wps
		"data.etx",   // not .et
		"saved.dpts", // not .dpt
		"contract.wps.bak",
		"slide.dpt.tmp",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			if got := GetFileType(name); got != FileTypeOTHER {
				t.Errorf("GetFileType(%q) = %q, want %q (must not over-match)", name, got, FileTypeOTHER)
			}
		})
	}
}

// TestFilenameType_WPS_Accepted verifies the upload gate (FilenameType) no
// longer rejects WPS private formats with FileTypeOTHER. FilenameType buckets
// all office documents as FileTypeDOC, consistent with how .ppt/.doc/.xls are
// already handled; the precise family is re-derived downstream by GetFileType.
func TestFilenameType_WPS_Accepted(t *testing.T) {
	for _, r := range wpsRoutes {
		t.Run(r.ext, func(t *testing.T) {
			got := FilenameType("sample." + r.ext)
			if got == FileTypeOTHER {
				t.Errorf("FilenameType(%q) = %q, want non-OTHER (upload gate must accept)", "sample."+r.ext, got)
			}
		})
	}
}

// TestFilenameType_WPS_DoesNotLoosenGate ensures the new extensions did not
// weaken the gate: a genuinely unknown extension is still rejected.
func TestFilenameType_WPS_DoesNotLoosenGate(t *testing.T) {
	for _, name := range []string{"mystery.xyz", "noext", "weird.wpsx"} {
		t.Run(name, func(t *testing.T) {
			if got := FilenameType(name); got != FileTypeOTHER {
				t.Errorf("FilenameType(%q) = %q, want %q (gate must stay strict)", name, got, FileTypeOTHER)
			}
		})
	}
}
