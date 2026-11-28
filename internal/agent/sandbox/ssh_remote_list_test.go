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
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

// find's %m is the permission bits only ("644"), not the st_mode file type, so
// the entry kind has to come from %y. Masking %m for S_IFDIR/S_IFREG never
// matches and makes every artifact look unsupported.
func TestParseRemoteEntriesUsesFindFileType(t *testing.T) {
	stdout := "/srv/run/artifacts/report.csv\x0042\x00f\x00" +
		"/srv/run/artifacts/plots\x004096\x00d\x00" +
		"/srv/run/artifacts/latest.csv\x007\x00l\x00"

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

func TestParseRemoteEntriesSkipsMalformedRecords(t *testing.T) {
	stdout := "/srv/run/artifacts/a.txt\x001\x00f\x00" +
		"/srv/run/artifacts/sized.txt\x00not-a-number\x00f\x00" +
		"/srv/run/artifacts/b.txt\x002\x00f\x00"

	entries := parseRemoteEntries(stdout, "/srv/run/artifacts")
	if len(entries) != 2 {
		t.Fatalf("parseRemoteEntries returned %d entries, want 2: %+v", len(entries), entries)
	}
	if entries[0].Name != "a.txt" || entries[1].Name != "b.txt" {
		t.Fatalf("unexpected entry names: %+v", entries)
	}
}

// A filename may contain a newline. NUL delimiting keeps it in one record,
// where the previous newline-delimited format split it into two fragments and
// turned the tail into a different artifact.
func TestParseRemoteEntriesKeepsNewlineInName(t *testing.T) {
	stdout := "/srv/run/artifacts/foo\nbar.csv\x005\x00f\x00"

	entries := parseRemoteEntries(stdout, "/srv/run/artifacts")
	if len(entries) != 1 {
		t.Fatalf("parseRemoteEntries returned %d entries, want 1: %+v", len(entries), entries)
	}
	if got := entries[0].Name; got != "foo\nbar.csv" {
		t.Fatalf("entry name = %q, want the newline preserved", got)
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

// A capped listing stops mid-record. A record that lost its type terminator is
// dropped instead of becoming a bogus entry.
func TestParseRemoteEntriesDropsCutRecords(t *testing.T) {
	stdout := "/srv/run/artifacts/a.txt\x001\x00f\x00" +
		"/srv/run/artifacts/b.txt\x002"
	entries := parseRemoteEntries(stdout, "/srv/run/artifacts")
	if len(entries) != 1 || entries[0].Name != "a.txt" {
		t.Fatalf("entries = %+v, want only a.txt", entries)
	}

	// The same record cut one field later is still incomplete.
	cutType := "/srv/run/artifacts/a.txt\x001\x00f\x00" +
		"/srv/run/artifacts/b.txt\x002\x00f"
	if got := parseRemoteEntries(cutType, "/srv/run/artifacts"); len(got) != 1 {
		t.Fatalf("entries = %+v, want only the terminated record", got)
	}
}

// The remote format is the contract between remoteListDir and
// parseRemoteEntries: %y carries the entry type, NUL delimits the fields, and
// the permission bits %m are not asked for because they carry no type.
func TestSSHRemoteListDirCommandShape(t *testing.T) {
	commands := make(chan string, 1)
	client := newSSHExecTestClient(t, func(command string, _ io.Reader) error {
		commands <- command
		return nil
	})
	p := &SSHProvider{timeout: 5}

	if _, err := p.remoteListDir(t.Context(), client, "/srv/artifacts"); err != nil {
		t.Fatalf("remoteListDir: %v", err)
	}
	command := <-commands
	for _, want := range []string{"-mindepth 1", "-maxdepth 1", "-printf", `%p\0%s\0%y`} {
		if !strings.Contains(command, want) {
			t.Errorf("remote command %q is missing %q", command, want)
		}
	}
	if strings.Contains(command, "%m") {
		t.Errorf("remote command %q still asks find for the permission bits", command)
	}
}

// execChannel recovers the session channel the test server hands to its execute
// callback; most tests only read stdin, these also write stdout and stderr back.
// The callback runs on the server goroutine, so a harness defect is reported as
// an error rather than through t.Fatal.
func execChannel(r io.Reader) (ssh.Channel, error) {
	ch, ok := r.(ssh.Channel)
	if !ok {
		return nil, fmt.Errorf("test harness passed %T, want ssh.Channel", r)
	}
	return ch, nil
}

// `find` exits non-zero when the artifacts dir does not exist. That is the
// normal "no artifacts produced" path, but only when it emitted nothing.
func TestSSHRemoteListDirMissingRootIsEmpty(t *testing.T) {
	client := newSSHExecTestClient(t, func(_ string, stdin io.Reader) error {
		ch, err := execChannel(stdin)
		if err != nil {
			return err
		}
		_, _ = ch.Stderr().Write([]byte("find: '/srv/run/artifacts': No such file or directory\n"))
		return errors.New("find: no such directory")
	})
	p := &SSHProvider{timeout: 5}

	entries, err := p.remoteListDir(t.Context(), client, "/srv/run/artifacts")
	if err != nil {
		t.Fatalf("remoteListDir on a missing root: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("remoteListDir returned %d entries, want none", len(entries))
	}
}

// Rows already on stdout mean the listing is incomplete: treating it as an
// empty directory would silently drop every artifact it did emit.
func TestSSHRemoteListDirRejectsPartialListing(t *testing.T) {
	client := newSSHExecTestClient(t, func(_ string, stdin io.Reader) error {
		ch, err := execChannel(stdin)
		if err != nil {
			return err
		}
		_, _ = ch.Write([]byte("/srv/run/artifacts/a.csv\x001\x00f\x00"))
		_, _ = ch.Stderr().Write([]byte("find: '/srv/run/artifacts/b.csv': No such file or directory\n"))
		return errors.New("find: entry vanished mid-walk")
	})
	p := &SSHProvider{timeout: 5}

	if _, err := p.remoteListDir(t.Context(), client, "/srv/run/artifacts"); err == nil ||
		!strings.Contains(err.Error(), "No such file or directory") {
		t.Fatalf("remoteListDir on a partial listing = %v, want the find error", err)
	}
}

func TestSSHRemoteReadFileReturnsContent(t *testing.T) {
	client := newSSHExecTestClient(t, func(command string, stdin io.Reader) error {
		if !strings.HasPrefix(command, "cat ") {
			return errors.New("unexpected command: " + command)
		}
		ch, err := execChannel(stdin)
		if err != nil {
			return err
		}
		_, err = ch.Write([]byte("a,b\n"))
		return err
	})
	p := &SSHProvider{timeout: 5, maxArtifactBytes: 1 << 20}

	body, err := p.remoteReadFile(t.Context(), client, "/srv/run/artifacts/report.csv")
	if err != nil {
		t.Fatalf("remoteReadFile: %v", err)
	}
	if body != "a,b\n" {
		t.Fatalf("remoteReadFile = %q, want %q", body, "a,b\n")
	}
}

// `find` reported the size before the read, so a background process can keep
// writing past maxArtifactBytes. The read itself has to refuse the content.
func TestSSHRemoteReadFileRejectsArtifactBeyondCap(t *testing.T) {
	client := newSSHExecTestClient(t, func(_ string, stdin io.Reader) error {
		ch, err := execChannel(stdin)
		if err != nil {
			return err
		}
		_, err = ch.Write([]byte(strings.Repeat("x", 4096)))
		return err
	})
	p := &SSHProvider{timeout: 5, maxArtifactBytes: 64}

	body, err := p.remoteReadFile(t.Context(), client, "/srv/run/artifacts/big.csv")
	if err == nil || !strings.Contains(err.Error(), "exceeds 64 bytes") {
		t.Fatalf("read of an oversized artifact = %d bytes, %v; want a size error", len(body), err)
	}
	if body != "" {
		t.Fatalf("remoteReadFile returned %d bytes with the error, want none", len(body))
	}
}
