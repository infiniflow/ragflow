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

package sandbox

import "testing"

// find's %m is the permission bits only ("644"), not the st_mode file type, so
// the entry kind has to come from %y. Masking %m for S_IFDIR/S_IFREG never
// matches and makes every artifact look unsupported.
func TestParseRemoteEntriesUsesFindFileType(t *testing.T) {
	stdout := "/srv/run/artifacts/report.csv\t42\t644\tf\n" +
		"/srv/run/artifacts/plots\t4096\t755\td\n" +
		"/srv/run/artifacts/latest.csv\t7\t777\tl\n"

	entries := parseRemoteEntries(stdout, "/srv/run/artifacts")
	if len(entries) != 3 {
		t.Fatalf("parseRemoteEntries returned %d entries, want 3", len(entries))
	}

	want := []struct {
		name   string
		size   int64
		isDir  bool
		isFile bool
	}{
		{"report.csv", 42, false, true},
		{"plots", 4096, true, false},
		{"latest.csv", 7, false, false}, // symlink: neither a regular file nor a dir
	}
	for i, w := range want {
		got := entries[i]
		if got.Name != w.name || got.Size != w.size || got.isDir() != w.isDir || got.isFile() != w.isFile {
			t.Errorf("entry %d = {name:%q size:%d dir:%v file:%v}, want {name:%q size:%d dir:%v file:%v}",
				i, got.Name, got.Size, got.isDir(), got.isFile(), w.name, w.size, w.isDir, w.isFile)
		}
	}
}

func TestParseRemoteEntriesSkipsMalformedLines(t *testing.T) {
	stdout := "/srv/run/artifacts/a.txt\t1\t644\tf\n" +
		"not-a-find-line\n" +
		"\n" +
		"/srv/run/artifacts/b.txt\t2\t644\tf\n"

	entries := parseRemoteEntries(stdout, "/srv/run/artifacts")
	if len(entries) != 2 {
		t.Fatalf("parseRemoteEntries returned %d entries, want 2: %+v", len(entries), entries)
	}
	if entries[0].Name != "a.txt" || entries[1].Name != "b.txt" {
		t.Fatalf("unexpected entry names: %+v", entries)
	}
}
