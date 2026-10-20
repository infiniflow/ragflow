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

// TestFilenameTypeVideo covers the video/visual split: video has its own
// upload category, so it is accepted at upload without being routed as an
// image. Every extension in the shared video set must classify as video.
func TestFilenameTypeVideo(t *testing.T) {
	cases := []struct {
		name string
		want FileType
	}{
		{"clip.mp4", FileTypeVIDEO},
		{"clip.mov", FileTypeVIDEO},
		{"CLIP.MOV", FileTypeVIDEO},
		{"clip.mkv", FileTypeVIDEO},
		{"clip.webm", FileTypeVIDEO},
		{"clip.flv", FileTypeVIDEO},
		{"clip.wmv", FileTypeVIDEO},
		{"clip.mpeg", FileTypeVIDEO},
		{"clip.mpg", FileTypeVIDEO},
		{"clip.3gp", FileTypeVIDEO},
		{"clip.3gpp", FileTypeVIDEO},
		{"clip.rm", FileTypeVIDEO},
		{"clip.rmvb", FileTypeVIDEO},
		{"clip.asf", FileTypeVIDEO},
		{"clip.dat", FileTypeVIDEO},
		{"clip.asx", FileTypeVIDEO},
		{"clip.wvx", FileTypeVIDEO},
		{"clip.mpe", FileTypeVIDEO},
		{"clip.mpa", FileTypeVIDEO},
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
