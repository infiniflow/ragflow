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
