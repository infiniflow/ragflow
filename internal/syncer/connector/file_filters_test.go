//
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
//

package connector

import "testing"

func TestFileFiltersMatching(t *testing.T) {
	filters, err := newFileFilters(map[string]any{
		"excluded_extensions":   []any{" .MP4 ", "zip", "tar.gz"},
		"excluded_folder_names": []string{"Archive"},
		"excluded_folder_paths": []any{"Projects/Old", "Документы/Видео"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{"MOVIE.Mp4": true, "data.zip": true, "backup.TAR.GZ": true, "data.gz": false, "zip.txt": false, "report.pdf": false, "no-extension": false} {
		if got := filters.excludesFile(name); got != want {
			t.Errorf("file %q: %v, want %v", name, got, want)
		}
	}
	for name, want := range map[string]bool{
		"Archive": true, "Nested/Archive": true, "archive": false,
		"Projects/Old": true, "Projects/Older": false, "Other/Projects/Old": false,
		"Документы/Видео": true, "Keep": false,
	} {
		if got := filters.excludesFolder(name); got != want {
			t.Errorf("folder %q: %v, want %v", name, got, want)
		}
	}
}

func TestFileFiltersRejectInvalidRules(t *testing.T) {
	for _, config := range []map[string]any{
		{"excluded_extensions": "mp4,zip"}, {"excluded_extensions": []any{12}},
		{"excluded_extensions": []string{"*.mp4"}}, {"excluded_extensions": []string{"."}},
		{"excluded_folder_names": []string{"a/b"}}, {"excluded_folder_names": []string{""}},
		{"excluded_folder_paths": []string{"../secret"}}, {"excluded_folder_paths": []string{"a/../b"}},
		{"excluded_folder_paths": []string{"/root"}}, {"excluded_folder_paths": []string{"root/"}},
		{"excluded_folder_paths": []string{"a\\b"}}, {"excluded_folder_paths": []string{"a/**"}},
	} {
		if _, err := newFileFilters(config); err == nil {
			t.Errorf("accepted invalid config: %v", config)
		}
	}
}

func TestFileFiltersEmptyConfig(t *testing.T) {
	filters, err := newFileFilters(nil)
	if err != nil || filters.excludesFile("movie.mp4") || filters.excludesFolder("Archive") {
		t.Fatalf("empty rules: %+v, %v", filters, err)
	}
}
