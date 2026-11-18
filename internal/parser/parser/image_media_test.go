//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package parser

import (
	"sync"
	"testing"
)

// resetImageRasterMaxBytesForTest re-reads EnvImageRasterMaxBytes between
// subtests by resetting the once guard. It lives in the test file so it is not
// compiled into production; the parser package's unexported vars are reachable
// because this is a same-package (white-box) test.
func resetImageRasterMaxBytesForTest() {
	imageRasterMaxBytesOnce = sync.Once{}
	imageRasterMaxBytes = 0
}

// TestImageRasterMaxBytesResolution covers env parsing: unset/zero/invalid/negative
// all mean unlimited (0), while a positive integer is honoured.
func TestImageRasterMaxBytesResolution(t *testing.T) {
	cases := []struct {
		name string
		env  string
		want int64
	}{
		{"unset means unlimited", "", 0},
		{"explicit zero means unlimited", "0", 0},
		{"positive value honoured", "123456789", 123456789},
		{"invalid falls back to unlimited", "not-a-number", 0},
		{"negative falls back to unlimited", "-5", 0},
		{"whitespace-trimmed zero", "  0  ", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(EnvImageRasterMaxBytes, c.env)
			resetImageRasterMaxBytesForTest()
			t.Cleanup(resetImageRasterMaxBytesForTest)
			if got := ImageRasterMaxBytes(); got != c.want {
				t.Errorf("ImageRasterMaxBytes() with env %q = %d, want %d", c.env, got, c.want)
			}
		})
	}
}

// TestCheckImageRasterLimit covers the opt-in raster-byte ceiling against the
// real 2692x16384 image that previously tripped the hard dimension guard.
// raster bytes = 2692 * 16384 * 4 = 176,422,912.
func TestCheckImageRasterLimit(t *testing.T) {
	cases := []struct {
		name    string
		w, h    int
		env     string
		wantErr bool
	}{
		{"default unlimited passes huge image", 2692, 16384, "", false},
		{"explicit zero passes huge image", 2692, 16384, "0", false},
		{"negative treated unlimited passes", 2692, 16384, "-1", false},
		{"invalid treated unlimited passes", 2692, 16384, "garbage", false},
		{"ceiling below raster blocks", 2692, 16384, "100000000", true},
		{"ceiling above raster passes", 2692, 16384, "200000000", false},
		{"ceiling exactly equal passes (not strictly over)", 2692, 16384, "176422912", false},
		{"ceiling one byte below blocks", 2692, 16384, "176422911", true},
		{"small image under generous ceiling passes", 100, 100, "100000", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(EnvImageRasterMaxBytes, c.env)
			resetImageRasterMaxBytesForTest()
			t.Cleanup(resetImageRasterMaxBytesForTest)
			err := CheckImageRasterLimit(c.w, c.h)
			if (err != nil) != c.wantErr {
				t.Errorf("CheckImageRasterLimit(%d,%d) env=%q: err=%v, wantErr=%v", c.w, c.h, c.env, err, c.wantErr)
			}
		})
	}
}
