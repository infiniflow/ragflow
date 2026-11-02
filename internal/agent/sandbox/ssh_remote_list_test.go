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

import (
	"strings"
	"testing"
)

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

// A wide directory must not let one `find` grow the provider's retained stdout
// without bound: the cap keeps the prefix and flags the cut.
func TestCappedBufferBoundsRemoteStdout(t *testing.T) {
	capped := &cappedBuffer{limit: 64}
	chunk := []byte(strings.Repeat("x", 32))
	for i := 0; i < 8; i++ {
		if n, err := capped.Write(chunk); n != len(chunk) || err != nil {
			t.Fatalf("Write = %d, %v; want %d, nil", n, err, len(chunk))
		}
	}
	if got := len(capped.String()); got != 64 {
		t.Fatalf("retained %d bytes, want 64", got)
	}
	if !capped.truncated {
		t.Fatal("cappedBuffer did not report the cut")
	}

	unlimited := &cappedBuffer{}
	for i := 0; i < 8; i++ {
		if _, err := unlimited.Write(chunk); err != nil {
			t.Fatalf("unlimited Write: %v", err)
		}
	}
	if got := len(unlimited.String()); got != 256 {
		t.Fatalf("unlimited buffer retained %d bytes, want 256", got)
	}
	if unlimited.truncated {
		t.Fatal("unlimited buffer reported a cut")
	}
}

// Very short names can fit far more rows than the byte cap suggests, so one
// listing is also bounded by row count.
func TestCapRemoteEntriesBoundsOneListing(t *testing.T) {
	entries := make([]remoteEntry, sshMaxListingEntries+5)
	if got := len(capRemoteEntries(entries)); got != sshMaxListingEntries {
		t.Fatalf("capRemoteEntries kept %d rows, want %d", got, sshMaxListingEntries)
	}
	small := entries[:3]
	if got := capRemoteEntries(small); len(got) != 3 || &got[0] != &small[0] {
		t.Fatalf("capRemoteEntries changed a listing that fits: %d rows", len(got))
	}
}

// head -c can stop inside a row. Complete rows still parse; the partial tail is
// dropped instead of becoming a bogus entry.
func TestParseRemoteEntriesDropsByteTruncatedTail(t *testing.T) {
	stdout := "/srv/run/artifacts/a.txt\t1\t644\tf\n" +
		"/srv/run/artifacts/b.txt\t2\t644"
	entries := parseRemoteEntries(stdout, "/srv/run/artifacts")
	if len(entries) != 1 || entries[0].Name != "a.txt" {
		t.Fatalf("entries = %+v, want only a.txt", entries)
	}

	// A row that arrived complete is kept even without its trailing newline.
	complete := "/srv/run/artifacts/a.txt\t1\t644\tf\n" +
		"/srv/run/artifacts/b.txt\t2\t644\tf"
	if got := parseRemoteEntries(complete, "/srv/run/artifacts"); len(got) != 2 {
		t.Fatalf("complete rows = %+v, want 2 entries", got)
	}
}
