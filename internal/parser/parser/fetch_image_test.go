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

package parser

import "testing"

func TestRemoteImageMIME(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{name: "png", data: []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), want: "image/png"},
		{name: "jpeg", data: []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'}, want: "image/jpeg"},
		{name: "svg", data: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"/>`), want: "image/svg+xml"},
		{name: "xml svg with bom", data: []byte("\xef\xbb\xbf<?xml version=\"1.0\"?><svg xmlns=\"http://www.w3.org/2000/svg\"/>"), want: "image/svg+xml"},
		{name: "opaque fallback", data: []byte("plain text not an image"), want: "image/octet-stream"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := remoteImageMIME(tc.data); got != tc.want {
				t.Fatalf("remoteImageMIME = %q, want %q", got, tc.want)
			}
		})
	}
}
