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

package entity

import (
	"strings"
	"testing"
)

func TestMediaRefValidDigestShape(t *testing.T) {
	base := MediaRef{
		Format: "html",
		Kind:   MediaKindDataURI,
		SHA256: strings.Repeat("a", 64),
	}
	if !base.Valid() {
		t.Fatal("well-formed reference is invalid")
	}

	for _, digest := range []string{
		"",
		strings.Repeat("a", 63),
		strings.Repeat("a", 65),
		strings.Repeat("A", 64),
		strings.Repeat("z", 64),
	} {
		ref := base
		ref.SHA256 = digest
		if ref.Valid() {
			t.Errorf("digest %q accepted, want invalid", digest)
		}
	}

	var nilRef *MediaRef
	if nilRef.Valid() {
		t.Fatal("nil reference is valid")
	}
}
