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

package utility

import (
	"os"
	"path/filepath"
	"testing"

	"ragflow/internal/common"
)

// TestConfDirFindsTheSourceTreeFromAnyPackageDepth pins the working-directory
// walk: `go test` runs from the package directory, so a package nested deeper
// than the walk looked used to fall through to the relative "conf" fallback and
// lose all_models.json. The walk is anchored to go.mod, not to a depth cap.
func TestConfDirFindsTheSourceTreeFromAnyPackageDepth(t *testing.T) {
	t.Setenv(common.EnvRAGFlowConfDir, "")
	t.Setenv(common.EnvRAGProjectBase, "")
	t.Setenv(common.EnvRAGDeployBase, "")

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "conf", "models"), 0o755); err != nil {
		t.Fatalf("create conf/models: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	deep := filepath.Join(root, "a", "b", "c", "d", "e")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("create package dir: %v", err)
	}
	t.Chdir(deep)

	if got, want := ConfDir(), filepath.Join(root, "conf"); got != want {
		t.Fatalf("ConfDir() = %q, want %q", got, want)
	}
}
