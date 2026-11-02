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
	"strconv"
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

// A listing that hits either bound is refused rather than silently shortened,
// otherwise collection would report fewer artifacts than the directory holds.
func TestListingBoundsErrorRefusesShortenedListings(t *testing.T) {
	if err := listingBoundsError("/srv/run/artifacts", false, 3); err != nil {
		t.Fatalf("listing that fits was refused: %v", err)
	}
	// Exactly at the row cap still fits: only a row beyond it refuses.
	if err := listingBoundsError("/srv/run/artifacts", false, sshMaxListingEntries); err != nil {
		t.Fatalf("listing at the row cap was refused: %v", err)
	}
	if err := listingBoundsError("/srv/run/artifacts", false, sshMaxListingEntries+1); err == nil ||
		!strings.Contains(err.Error(), strconv.Itoa(sshMaxListingEntries)) {
		t.Fatalf("row cap error = %v, want the cap in the message", err)
	}
	if err := listingBoundsError("/srv/run/artifacts", true, 3); err == nil ||
		!strings.Contains(err.Error(), strconv.Itoa(sshMaxListingBytes)) {
		t.Fatalf("byte cap error = %v, want the cap in the message", err)
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
