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

package media

import "testing"

func TestNormalizeBase64ImagePayloadByteLimit(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		limit         int
		want          string
		wantSize      int64
	}{
		{"negative_limit", "YQ", -1, "", 0},
		{"zero_limit", "YQ", 0, "", 0},
		{"exact_limit_padded", "YWI=", 2, "YWI=", 2},
		{"exact_limit_unpadded", "YWI", 2, "YWI=", 2},
		{"exact_limit_wrapped", "YW\r\nI", 2, "YWI=", 2},
		{"over_limit_padded", "YWI=", 1, "", 0},
		{"over_limit_unpadded", "YWI", 1, "", 0},
		{"over_encoded_limit", "YWJjZA==", 2, "", 0},
		{"data_after_padding", "G0==JbjQ", 16, "", 0},
		{"data_after_padding_long", "aMRFPqfZALtY6rw50P==Pn5d", 16, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, size, _, ok := NormalizeBase64ImagePayload(tc.payload, tc.limit)
			if got != tc.want || ok != (tc.want != "") || size != tc.wantSize {
				t.Errorf("NormalizeBase64ImagePayload(%q, %d) = (%q, %d, %v), want (%q, %d, %v)",
					tc.payload, tc.limit, got, size, ok, tc.want, tc.wantSize, tc.want != "")
			}
		})
	}
}
