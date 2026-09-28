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

package pdf

import "testing"

// TestPlainText_EmptyInput locks the empty-input contract: a nil/empty
// payload yields (nil, 0, nil) rather than an error, so the caller's
// empty-PDF short-circuit stays correct under the pure-Go engine.
func TestPlainText_EmptyInput(t *testing.T) {
	items, pageCount, err := PlainText(nil)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if items != nil || pageCount != 0 {
		t.Fatalf("got (items=%v, pageCount=%d), want (nil, 0)", items, pageCount)
	}
}

// TestPlainText_GarbageReturnsError locks that non-PDF input fails loudly
// instead of silently producing empty items — the pure-Go ledongthuc/pdf
// path must surface a parse error, never a plausible empty success.
func TestPlainText_GarbageReturnsError(t *testing.T) {
	_, _, err := PlainText([]byte("this is not a pdf"))
	if err == nil {
		t.Fatal("expected error for non-PDF input, got nil")
	}
}
