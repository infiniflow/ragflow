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

package source

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestLocalSourceFetchSkipsIgnoredDirectoriesAndUsesSlashPaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "node_modules")
	files := map[string]string{
		"SKILL.md":                     "---\nname: demo\n---\nbody\n",
		"scripts/run.sh":               "echo ok\n",
		"node_modules/pkg/index.js":    "module.exports = 1\n",
		".vscode/settings.json":        "{}\n",
		".idea/workspace.xml":          "<project/>\n",
		".git/info/exclude.txt":        "x\n",
		"nested/node_modules/dep/a.js": "1\n",
		"nested/my_node_modules/a.js":  "1\n",
		"._docs/readme.md":             "keep\n",
		"._metadata.md":                "ignore\n",
		"nested/settings.env.json":     "{}\n",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	bundle, err := NewLocalSource().Fetch(root)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	got := make([]string, 0, len(bundle.Files))
	for name := range bundle.Files {
		got = append(got, name)
	}
	slices.Sort(got)
	want := []string{"._docs/readme.md", "SKILL.md", "nested/my_node_modules/a.js", "scripts/run.sh"}
	if !slices.Equal(got, want) {
		t.Fatalf("bundle files = %q, want %q", got, want)
	}
}
