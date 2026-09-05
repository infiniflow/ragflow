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

//go:build !integration && !e2e && !manual

package handler

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateDevImportPath(t *testing.T) {
	dir := t.TempDir()
	old := devImportAllowedDir
	devImportAllowedDir = dir
	t.Cleanup(func() { devImportAllowedDir = old })

	inner := filepath.Join(dir, "sub")
	os.MkdirAll(inner, 0o755)
	f := filepath.Join(inner, "data.json")
	os.WriteFile(f, []byte("{}"), 0o644)

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid file", f, false},
		{"valid dir", inner, false},
		{"relative path", "sub/data.json", true},
		{"traversal", filepath.Join(dir, "sub", "..", "..", "etc", "passwd"), true},
		{"outside dir", "/etc/passwd", true},
		{"nonexistent", filepath.Join(dir, "no-such-file"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateDevImportPath(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got path %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != f && got != inner {
				t.Fatalf("unexpected resolved path: %q", got)
			}
		})
	}
}

func TestValidateDevImportPath_SymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	old := devImportAllowedDir
	devImportAllowedDir = dir
	t.Cleanup(func() { devImportAllowedDir = old })

	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644)

	link := filepath.Join(dir, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("symlinks not supported")
	}

	_, err := validateDevImportPath(filepath.Join(link, "secret.txt"))
	if err == nil {
		t.Fatal("symlink escape should be rejected")
	}
}
