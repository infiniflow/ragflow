/*
Copyright 2026 The InfiniFlow Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utility

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindProjectRootWalksUpToConfMarkers(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "conf"), 0o755); err != nil {
		t.Fatalf("mkdir conf: %v", err)
	}
	for _, name := range []string{"mapping.json", "service_conf.yaml"} {
		if err := os.WriteFile(filepath.Join(root, "conf", name), []byte("{}"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	nested := filepath.Join(root, "cmd", "server")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	got, ok := findProjectRoot(nested)
	if !ok {
		t.Fatal("expected project root to be found")
	}
	if got != root {
		t.Fatalf("root = %q, want %q", got, root)
	}
}

func TestFindProjectRootRequiresMarkers(t *testing.T) {
	if got, ok := findProjectRoot(t.TempDir()); ok {
		t.Fatalf("unexpected project root %q", got)
	}
}
