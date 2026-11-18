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

func TestFilenameTypeBMP(t *testing.T) {
	cases := []struct {
		name string
		want FileType
	}{
		{"photo.bmp", FileTypeVISUAL},
		{"PHOTO.BMP", FileTypeVISUAL},
		{"path/to/lt50M_bmp_image_table.bmp", FileTypeVISUAL},
		{"x.png", FileTypeVISUAL},
		{"test.wmf", FileTypeVISUAL},
		{"TEST.WMF", FileTypeVISUAL},
		{"bad.exe", FileTypeOTHER},
	}
	for _, tc := range cases {
		if got := FilenameType(tc.name); got != tc.want {
			t.Fatalf("FilenameType(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestFilenameTypeTIFF verifies that the upload gate admits both TIFF
// extensions as visual, matching the GetFileType classification the picture
// parser dispatches on.
func TestFilenameTypeTIFF(t *testing.T) {
	cases := []struct {
		name string
		want FileType
	}{
		{"scan.tiff", FileTypeVISUAL},
		{"SCAN.TIFF", FileTypeVISUAL},
		{"path/to/fax_page_1.tiff", FileTypeVISUAL},
		{"scan.tif", FileTypeVISUAL},
		{"SCAN.TIF", FileTypeVISUAL},
		{"scan.tiffx", FileTypeOTHER},
	}
	for _, tc := range cases {
		if got := FilenameType(tc.name); got != tc.want {
			t.Errorf("FilenameType(%q) = %q, want %q", tc.name, got, tc.want)
		}
		if got := GetFileType(tc.name); got != tc.want {
			t.Errorf("GetFileType(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestFilenameTypeVideo covers the upload bucket: video is admitted as visual
// (legacy categorization) and routed to the picture pipeline; parsing is
// blocked later. Every extension in the shared video set must classify as
// visual here, while GetFileType reports them as video for runtime dispatch.
func TestFilenameTypeVideo(t *testing.T) {
	cases := []struct {
		name string
		want FileType
	}{
		{"clip.mp4", FileTypeVISUAL},
		{"clip.mov", FileTypeVISUAL},
		{"CLIP.MOV", FileTypeVISUAL},
		{"clip.mkv", FileTypeVISUAL},
		{"clip.webm", FileTypeVISUAL},
		{"clip.flv", FileTypeVISUAL},
		{"clip.wmv", FileTypeVISUAL},
		{"clip.mpeg", FileTypeVISUAL},
		{"clip.mpg", FileTypeVISUAL},
		{"clip.3gp", FileTypeVISUAL},
		{"clip.3gpp", FileTypeVISUAL},
		{"clip.rm", FileTypeVISUAL},
		{"clip.rmvb", FileTypeVISUAL},
		{"clip.asf", FileTypeVISUAL},
		{"clip.dat", FileTypeVISUAL},
		{"clip.asx", FileTypeVISUAL},
		{"clip.wvx", FileTypeVISUAL},
		{"clip.mpe", FileTypeVISUAL},
		{"clip.mpa", FileTypeVISUAL},
		{"photo.jpg", FileTypeVISUAL},
	}
	for _, tc := range cases {
		if got := FilenameType(tc.name); got != tc.want {
			t.Fatalf("FilenameType(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestIsVideoExtension pins the shared predicate that upload admission, runtime
// dispatch and the parser-family mapping read.
func TestIsVideoExtension(t *testing.T) {
	for _, ext := range []string{"mp4", ".MOV", "rmvb", "3gpp"} {
		if !IsVideoExtension(ext) {
			t.Fatalf("IsVideoExtension(%q) = false, want true", ext)
		}
	}
	for _, ext := range []string{"", "jpg", "pdf", "mp4x"} {
		if IsVideoExtension(ext) {
			t.Fatalf("IsVideoExtension(%q) = true, want false", ext)
		}
	}
	if !IsVideoFilename("path/to/CLIP.MOV") {
		t.Fatal("IsVideoFilename(\"path/to/CLIP.MOV\") = false, want true")
	}
}
