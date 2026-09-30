//go:build !cgo

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

package office

import (
	"errors"
	"testing"
)

// TestOpenAndExtract_NoCGO locks the degrade contract: without the native
// office_oxide backend, OpenAndExtract must report ErrOfficeCGORequired
// (not attempt to link the C library or panic). This is the behavior the
// parser package's facade relies on to wrap it into its own sentinel.
func TestOpenAndExtract_NoCGO(t *testing.T) {
	_, _, _, _, err := OpenAndExtract([]byte("PK\x03\x04"), "docx")
	if err == nil {
		t.Fatal("expected ErrOfficeCGORequired, got nil")
	}
	if !errors.Is(err, ErrOfficeCGORequired) {
		t.Fatalf("err = %v, want ErrOfficeCGORequired", err)
	}
}
