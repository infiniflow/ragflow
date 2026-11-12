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

package main

import (
	"archive/tar"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var pdfiumLinux = nativeLib{
	name:     "pdfium-static",
	version:  "7809",
	required: pdfiumRequiredFiles(platform{"linux", "amd64"}),
}

func pdfiumEntries() []tarEntry {
	var entries []tarEntry
	for _, name := range pdfiumLinux.required {
		entries = append(entries, tarEntry{name: name, body: []byte("pdfium-payload")})
	}
	return entries
}

func TestPdfiumRequiresBundledLibcxxOnLinuxOnly(t *testing.T) {
	linux := pdfiumRequiredFiles(platform{"linux", "arm64"})
	if !slices.Contains(linux, "lib/libc++.a") || !slices.Contains(linux, "lib/libc++abi.a") {
		t.Errorf("linux pdfium must require the bundled libc++: %v", linux)
	}
	darwin := pdfiumRequiredFiles(platform{"darwin", "arm64"})
	if !slices.Equal(darwin, []string{"lib/libpdfium.a", "include/fpdfview.h"}) {
		t.Errorf("macOS archives ship no libc++; got %v", darwin)
	}
}

func TestExtractLandsInVersionedDirAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "pdfium.tgz")
	writeTarGz(t, archive, pdfiumEntries())
	target := pdfiumLinux.dir(filepath.Join(dir, "native-libs"))
	if want := filepath.Join(dir, "native-libs", "pdfium-static", "v7809"); target != want {
		t.Fatalf("target = %q; want %q", target, want)
	}
	if err := pdfiumLinux.extractTarGz(archive, target); err != nil {
		t.Fatal(err)
	}
	if !pdfiumLinux.provisioned(target) {
		t.Fatal("extracted tree is not provisioned")
	}
	if err := os.Remove(archive); err != nil {
		t.Fatal(err)
	}
	// Already provisioned: the archive is not even opened.
	if err := pdfiumLinux.extractTarGz(archive, target); err != nil {
		t.Errorf("rerun on a provisioned dir: %v", err)
	}
}

func TestIncompleteArchivePreservesExistingDirectory(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "native")
	mustWrite(t, filepath.Join(target, "existing"), []byte("keep"))
	archive := filepath.Join(dir, "incomplete.tar.gz")
	writeTarGz(t, archive, nil)
	lib := nativeLib{name: "x", version: "1", required: []string{"lib/library.a"}}
	err := lib.extractTarGz(archive, target)
	if err == nil || !strings.Contains(err.Error(), "missing required native files") {
		t.Fatalf("err = %v", err)
	}
	if got := string(mustRead(t, filepath.Join(target, "existing"))); got != "keep" {
		t.Errorf("existing file = %q", got)
	}
	if names := listDir(t, dir); !slices.Equal(names, []string{"incomplete.tar.gz", "native"}) {
		t.Errorf("staging left behind: %v", names)
	}
}

func TestExtractRejectsUnsafeMembers(t *testing.T) {
	cases := map[string]tarEntry{
		"traversal": {name: "../outside", body: []byte("unsafe")},
		"absolute":  {name: "/tmp/outside", body: []byte("unsafe")},
		"symlink":   {name: "link", typeflag: tar.TypeSymlink, linkname: "/etc/passwd"},
		"hardlink":  {name: "link", typeflag: tar.TypeLink, linkname: "/etc/passwd"},
		"fifo":      {name: "fifo", typeflag: tar.TypeFifo},
		"device":    {name: "dev", typeflag: tar.TypeChar},
	}
	for kind, bad := range cases {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "native")
			mustWrite(t, filepath.Join(target, "existing"), []byte("keep"))
			mustWrite(t, filepath.Join(dir, "outside"), []byte("untouched"))
			archive := filepath.Join(dir, "unsafe.tar.gz")
			writeTarGz(t, archive, []tarEntry{{name: "./lib/library.a", body: []byte("payload")}, bad})
			lib := nativeLib{name: "x", version: "1", required: []string{"lib/library.a"}}

			err := lib.extractTarGz(archive, target)
			if err == nil || !strings.Contains(err.Error(), "unsafe native archive member") {
				t.Fatalf("err = %v", err)
			}
			if got := string(mustRead(t, filepath.Join(dir, "outside"))); got != "untouched" {
				t.Errorf("outside file = %q", got)
			}
			if got := string(mustRead(t, filepath.Join(target, "existing"))); got != "keep" {
				t.Errorf("existing file = %q", got)
			}
			if nonEmptyFile(filepath.Join(target, "lib", "library.a")) {
				t.Error("partial extraction reached the target")
			}
			if names := listDir(t, dir); !slices.Equal(names, []string{"native", "outside", "unsafe.tar.gz"}) {
				t.Errorf("unexpected files: %v", names)
			}
		})
	}
}

func TestVersionStamps(t *testing.T) {
	cases := []struct {
		name   string
		lib    nativeLib
		stamp  string
		wantOK bool
	}{
		{"pdf_oxide text stamp", nativeLib{version: "0.3.78", stamp: stampText}, "pdf_oxide 0.3.78", true},
		{"pdf_oxide stale", nativeLib{version: "0.3.78", stamp: stampText}, "pdf_oxide 0.3.73", false},
		{"office_oxide null stamp", nativeLib{version: "0.1.16", stamp: stampNull}, "\x000.1.16\x00", true},
		{"office_oxide stale", nativeLib{version: "0.1.16", stamp: stampNull}, "\x000.1.15\x00", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			c.lib.name = "lib"
			c.lib.required = []string{"lib/libx.a", "include/x.h"}
			entries := []tarEntry{
				{name: "lib/libx.a", body: []byte("!<arch>\n" + c.stamp)},
				{name: "include/x.h", body: []byte("// header")},
			}
			archive := filepath.Join(dir, "lib.tar.gz")
			writeTarGz(t, archive, entries)
			if got := c.lib.validArchive(archive); got != c.wantOK {
				t.Errorf("validArchive = %v; want %v", got, c.wantOK)
			}
			tree := filepath.Join(dir, "tree")
			for _, e := range entries {
				mustWrite(t, filepath.Join(tree, e.name), e.body)
			}
			if got := c.lib.provisioned(tree); got != c.wantOK {
				t.Errorf("provisioned = %v; want %v", got, c.wantOK)
			}
		})
	}
}

func TestProvisionedRejectsMissingOrEmptyFiles(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "lib", "libpdfium.a"), []byte("x"))
	lib := nativeLib{required: []string{"lib/libpdfium.a", "include/fpdfview.h"}}
	if lib.provisioned(dir) {
		t.Error("missing header accepted")
	}
	mustWrite(t, filepath.Join(dir, "include", "fpdfview.h"), nil)
	if lib.provisioned(dir) {
		t.Error("empty header accepted")
	}
	mustWrite(t, filepath.Join(dir, "include", "fpdfview.h"), []byte("h"))
	if !lib.provisioned(dir) {
		t.Error("complete tree rejected")
	}
}

func seedORT(t *testing.T, staticLib, dirName string) {
	t.Helper()
	mustWrite(t, filepath.Join(staticLib, dirName, "lib", "libonnxruntime.a"), []byte("!<arch>\nort"))
}

func TestExtractORTRenamesReleaseDirAndPrunesStaleVersions(t *testing.T) {
	for _, p := range []platform{{"linux", "amd64"}, {"darwin", "arm64"}} {
		t.Run(p.goos, func(t *testing.T) {
			dir := t.TempDir()
			staticLib := filepath.Join(dir, "onnxruntime", "static_lib")
			stale := ortNormalizedDir("1.28.0", p)
			seedORT(t, staticLib, stale)
			archive := filepath.Join(dir, ortAssetName("1.29.0", p))
			writeORTZip(t, archive, "onnxruntime-v1.29.0-"+ortTarget(p))

			expected := ortNormalizedDir("1.29.0", p)
			if err := extractORT(staticLib, archive, expected, "1.29.0"); err != nil {
				t.Fatal(err)
			}
			if !nonEmptyFile(filepath.Join(staticLib, expected, "lib", "libonnxruntime.a")) {
				t.Fatal("bumped ORT version landed no .a")
			}
			for _, gone := range []string{stale, "onnxruntime-v1.29.0-" + ortTarget(p)} {
				if _, err := os.Stat(filepath.Join(staticLib, gone)); !os.IsNotExist(err) {
					t.Errorf("%s still present", gone)
				}
			}
		})
	}
}

func TestExtractORTShortCircuitStillPrunesForeignPlatform(t *testing.T) {
	dir := t.TempDir()
	staticLib := filepath.Join(dir, "static_lib")
	expected := ortNormalizedDir("1.29.0", platform{"linux", "amd64"})
	foreign := ortNormalizedDir("1.29.0", platform{"darwin", "arm64"})
	seedORT(t, staticLib, expected)
	seedORT(t, staticLib, foreign)
	archive := filepath.Join(dir, "ort.zip")
	writeORTZip(t, archive, "onnxruntime-v1.29.0-linux-x86_64")

	if err := extractORT(staticLib, archive, expected, "1.29.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(staticLib, foreign)); !os.IsNotExist(err) {
		t.Error("foreign-platform ORT dir survived")
	}
	before := listDir(t, staticLib)
	if err := extractORT(staticLib, archive, expected, "1.29.0"); err != nil {
		t.Fatal(err)
	}
	if after := listDir(t, staticLib); !slices.Equal(before, after) {
		t.Errorf("rerun changed static_lib: %v -> %v", before, after)
	}
}

func TestExtractORTFailsWithoutArchive(t *testing.T) {
	dir := t.TempDir()
	if err := extractORT(filepath.Join(dir, "static_lib"), filepath.Join(dir, "missing.zip"), "x", "1.29.0"); err == nil {
		t.Error("missing archive accepted")
	}
}

func TestStampORTVersionCreatesRelativeSymlink(t *testing.T) {
	staticLib := filepath.Join(t.TempDir(), "onnxruntime", "static_lib")
	norm := "onnxruntime-linux-x64-static_lib-1.29.0-glibc2_28"
	seedORT(t, staticLib, norm)
	if err := stampORTVersion(staticLib, norm, "1.29.0"); err != nil {
		t.Fatal(err)
	}
	stamps, _ := filepath.Glob(filepath.Join(staticLib, "v1.29.0-*"))
	if len(stamps) != 1 {
		t.Fatalf("stamp dirs = %v", stamps)
	}
	link := filepath.Join(stamps[0], "libonnxruntime.a")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.IsAbs(target) {
		t.Errorf("stamp symlink is absolute: %s", target)
	}
	if got := string(mustRead(t, link)); got != "!<arch>\nort" {
		t.Errorf("symlink resolves to %q", got)
	}

	before := listDir(t, staticLib)
	if err := stampORTVersion(staticLib, norm, "1.29.0"); err != nil {
		t.Fatal(err)
	}
	if after := listDir(t, staticLib); !slices.Equal(before, after) {
		t.Errorf("second stamp changed static_lib: %v -> %v", before, after)
	}
}

// remoteNativeRelease serves linux/amd64 fixtures for every native library
// at the paths of their real release URLs.
func remoteNativeRelease(t *testing.T) *fakeRemote {
	t.Helper()
	remote := newFakeRemote()
	fixtures := t.TempDir()
	build := func(name string, entries []tarEntry) []byte {
		path := filepath.Join(fixtures, name)
		writeTarGz(t, path, entries)
		return mustRead(t, path)
	}
	remote.files["/kognitos/pdfium-static/releases/download/chromium/1234/pdfium-linux-x64-static.tgz"] = build("pdfium", pdfiumEntries())
	remote.files["/yfedoseev/pdf_oxide/releases/download/v0.4.1/pdf_oxide-go-ffi-linux-amd64.tar.gz"] = build("pdf_oxide", []tarEntry{
		{name: "lib/linux_amd64/libpdf_oxide.a", body: []byte("!<arch>\npdf_oxide 0.4.1")},
		{name: "include/pdf_oxide.h", body: []byte("// header")},
	})
	remote.files["/yfedoseev/office_oxide/releases/download/v0.1.12/native-linux-x86_64.tar.gz"] = build("office_oxide", []tarEntry{
		{name: "lib/liboffice_oxide.a", body: []byte("!<arch>\n\x000.1.12\x00")},
		{name: "include/office_oxide_c/office_oxide.h", body: []byte("// header")},
	})
	zipPath := filepath.Join(fixtures, "ort.zip")
	writeORTZip(t, zipPath, "onnxruntime-v9.9.9-linux-x86_64")
	ortPath := "/infiniflow/ragflow-build/releases/download/" + ortReleaseTag + "/onnxruntime-v9.9.9-linux-x86_64.zip"
	remote.files[ortPath] = mustRead(t, zipPath)
	remote.files[ortPath+".sha256"] = []byte(sha256Hex(remote.files[ortPath]) + "  onnxruntime-v9.9.9-linux-x86_64.zip\n")
	return remote
}

func TestProvisionNativeEndToEnd(t *testing.T) {
	remote := remoteNativeRelease(t)
	d := testDownloader(t, remote)

	if err := d.provisionNative(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"pdfium-static/v1234/lib/libpdfium.a",
		"pdf_oxide/v0.4.1/lib/linux_amd64/libpdf_oxide.a",
		"office_oxide/v0.1.12/lib/liboffice_oxide.a",
		"onnxruntime/static_lib/onnxruntime-linux-x64-static_lib-9.9.9-glibc2_28/lib/libonnxruntime.a",
	} {
		if !nonEmptyFile(filepath.Join(d.nativeDir, filepath.FromSlash(want))) {
			t.Errorf("%s not provisioned", want)
		}
	}
	if stamps, _ := filepath.Glob(filepath.Join(d.nativeDir, "onnxruntime", "static_lib", "v9.9.9-*", "libonnxruntime.a")); len(stamps) != 1 {
		t.Errorf("ORT stamp links = %v", stamps)
	}

	// A provisioned cache needs only the ORT sidecar on the next run.
	remote.resetHits()
	if err := os.RemoveAll(d.depsDir); err != nil {
		t.Fatal(err)
	}
	if err := d.provisionNative(); err != nil {
		t.Fatal(err)
	}
	for _, hit := range remote.hits {
		if strings.HasSuffix(hit, ".tgz") || strings.HasSuffix(hit, ".tar.gz") {
			t.Errorf("provisioned library downloaded again: %s", hit)
		}
	}
}

func TestFetchORTArchiveRefreshesReissuedZip(t *testing.T) {
	remote := remoteNativeRelease(t)
	d := testDownloader(t, remote)
	zipPath := filepath.Join(d.depsDir, ortAssetName("9.9.9", d.platform))
	ortURLPath := "/infiniflow/ragflow-build/releases/download/" + ortReleaseTag + "/onnxruntime-v9.9.9-linux-x86_64.zip"

	// A stale same-named local zip and its extracted dir.
	writeORTZip(t, zipPath, "onnxruntime-v9.9.9-linux-x86_64-old")
	staleDir := filepath.Join(d.nativeDir, "onnxruntime", "static_lib", ortNormalizedDir("9.9.9", d.platform))
	seedORT(t, filepath.Dir(staleDir), filepath.Base(staleDir))

	if err := d.fetchORTArchive(zipPath, "9.9.9"); err != nil {
		t.Fatal(err)
	}
	if remote.hitCount(ortURLPath) != 1 {
		t.Error("checksum mismatch did not trigger a re-download")
	}
	if got := sha256Hex(mustRead(t, zipPath)); got != sha256Hex(remote.files[ortURLPath]) {
		t.Error("local zip not refreshed")
	}
	if _, err := os.Stat(staleDir); !os.IsNotExist(err) {
		t.Error("directory extracted from the stale zip survived")
	}

	remote.resetHits()
	if err := d.fetchORTArchive(zipPath, "9.9.9"); err != nil {
		t.Fatal(err)
	}
	if remote.hitCount(ortURLPath) != 0 {
		t.Error("matching zip downloaded again")
	}

	remote.files[ortURLPath+".sha256"] = []byte(strings.Repeat("0", 64))
	if err := os.Remove(zipPath); err != nil {
		t.Fatal(err)
	}
	if err := d.fetchORTArchive(zipPath, "9.9.9"); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("err = %v; want checksum mismatch", err)
	}
	if nonEmptyFile(zipPath) {
		t.Error("mismatching download kept")
	}
}
