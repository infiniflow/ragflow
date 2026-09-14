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
	"testing"
)

func TestGetFileType_ImageExtensions(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		want     FileType
	}{
		// Image formats that should resolve to VISUAL.
		{"png", "image.png", FileTypeVISUAL},
		{"jpg", "photo.jpg", FileTypeVISUAL},
		{"jpeg", "photo.jpeg", FileTypeVISUAL},
		{"gif", "animation.gif", FileTypeVISUAL},
		{"bmp", "bitmap.bmp", FileTypeVISUAL},
		{"tiff", "image.tiff", FileTypeVISUAL},
		{"tif", "image.tif", FileTypeVISUAL},
		{"webp", "image.webp", FileTypeVISUAL},
		{"svg", "vector.svg", FileTypeVISUAL},
		{"ico", "icon.ico", FileTypeVISUAL},
		{"avif", "image.avif", FileTypeVISUAL},
		{"heic", "photo.heic", FileTypeVISUAL},
		{"apng", "animation.apng", FileTypeVISUAL},

		// Capitalised extension should still match.
		{"png uppercase", "image.PNG", FileTypeVISUAL},
		{"jpg mixed case", "photo.JpG", FileTypeVISUAL},

		// Path with directory should still resolve.
		{"png with path", "/path/to/image.png", FileTypeVISUAL},
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

func TestGetFileType_ExistingFormats_NoRegression(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		want     FileType
	}{
		{"pdf", "doc.pdf", FileTypePDF},
		{"doc", "old.doc", FileTypeDOC},
		{"docx", "report.docx", FileTypeDOCX},
		{"xls", "spreadsheet.xls", FileTypeXLS},
		{"xlsx", "spreadsheet.xlsx", FileTypeXLSX},
		{"csv", "data.csv", FileTypeCSV},
		{"ppt", "slides.ppt", FileTypePPT},
		{"pptx", "slides.pptx", FileTypePPTX},
		{"html", "page.html", FileTypeHTML},
		{"htm", "page.htm", FileTypeHTML},
		{"md", "readme.md", FileTypeMarkdown},
		{"markdown", "readme.markdown", FileTypeMarkdown},
		{"txt", "notes.txt", FileTypeTXT},
		{"py", "script.py", FileTypeTXT},
		{"js", "script.js", FileTypeTXT},
		{"go", "main.go", FileTypeTXT},
		{"java", "Main.java", FileTypeTXT},
		{"epub", "book.epub", FileTypeEPUB},
		{"json", "data.json", FileTypeJSON},
		{"jsonl", "data.jsonl", FileTypeJSON},
		{"eml", "email.eml", FileTypeEMAIL},
		{"msg", "email.msg", FileTypeEMAIL},
		{"mp3", "audio.mp3", FileTypeAURAL},
		{"wav", "audio.wav", FileTypeAURAL},
		{"flac", "audio.flac", FileTypeAURAL},
		{"mp4", "video.mp4", FileTypeVIDEO},
		{"avi", "video.avi", FileTypeVIDEO},
		{"mkv", "video.mkv", FileTypeVIDEO},
		{"mov", "video.mov", FileTypeVIDEO},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := GetFileType(tc.filename)
			if got != tc.want {
				t.Errorf("GetFileType(%q) = %q, want %q (regression)", tc.filename, got, tc.want)
			}
		})
	}
}

func TestGetFileType_UnknownExtension_ReturnsOTHER(t *testing.T) {
	cases := []struct {
		name     string
		filename string
	}{
		{"no extension", "Makefile"},
		{"unknown extension", "data.xyz"},
		{"dotfile", ".hidden"},
		{"empty string", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := GetFileType(tc.filename)
			if got != FileTypeOTHER {
				t.Errorf("GetFileType(%q) = %q, want %q (FileTypeOTHER)", tc.filename, got, FileTypeOTHER)
			}
		})
	}
}
