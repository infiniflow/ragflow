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
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// stampFormat is how a native library embeds its version in its .a file.
type stampFormat int

const (
	stampNone stampFormat = iota
	stampNull             // office_oxide: "\x00<version>\x00"
	stampText             // pdf_oxide: "pdf_oxide <version>"
)

// nativeLib describes one native static library and where it lands in the
// version-keyed cache <nativeDir>/<name>/v<version>/.
type nativeLib struct {
	name     string
	url      string
	archive  string // file name under deps/
	version  string
	required []string // paths that must exist, non-empty, in the extracted tree
	stamp    stampFormat
}

func (l nativeLib) dir(nativeDir string) string {
	return filepath.Join(nativeDir, l.name, "v"+l.version)
}

func (d *downloader) nativeLibs() ([]nativeLib, error) {
	pdfium, err := pdfiumStaticVersion(d.repoRoot)
	if err != nil {
		return nil, err
	}
	pdfOxide, err := goModuleVersion(d.repoRoot, pdfOxideModule)
	if err != nil {
		return nil, err
	}
	officeOxide, err := goModuleVersion(d.repoRoot, officeOxideModule)
	if err != nil {
		return nil, err
	}
	p := d.platform
	return []nativeLib{
		{
			name:     "pdfium-static",
			url:      d.pdfiumURL(pdfium),
			archive:  pdfiumLocalAssetName(pdfium, p),
			version:  pdfium,
			required: pdfiumRequiredFiles(p),
		},
		{
			name:     "pdf_oxide",
			url:      d.pdfOxideURL(pdfOxide),
			archive:  pdfOxideAssets[p],
			version:  pdfOxide,
			required: []string{fmt.Sprintf("lib/%s_%s/libpdf_oxide.a", p.goos, p.goarch), "include/pdf_oxide.h"},
			stamp:    stampText,
		},
		{
			name:     "office_oxide",
			url:      d.officeOxideURL(officeOxide),
			archive:  officeOxideAssets[p],
			version:  officeOxide,
			required: []string{"lib/liboffice_oxide.a", "include/office_oxide_c/office_oxide.h"},
			stamp:    stampNull,
		},
	}, nil
}

// pdfiumRequiredFiles lists the files build.sh links from pdfium-static. Only
// the Linux archives bundle libc++, which build.sh links there with lld; the
// macOS archives use the system libc++ and do not ship it.
func pdfiumRequiredFiles(p platform) []string {
	if p.goos == "linux" {
		return []string{"lib/libpdfium.a", "lib/libc++.a", "lib/libc++abi.a", "include/fpdfview.h"}
	}
	return []string{"lib/libpdfium.a", "include/fpdfview.h"}
}

func stampPresent(data []byte, version string, format stampFormat) bool {
	null := []byte("\x00" + version + "\x00")
	switch format {
	case stampNull:
		return bytes.Contains(data, null)
	case stampText:
		return bytes.Contains(data, []byte("pdf_oxide "+version)) || bytes.Contains(data, null)
	default:
		return true
	}
}

// provisioned reports whether dir holds every required file, non-empty, with
// the version stamp in each .a when the library carries one.
func (l nativeLib) provisioned(dir string) bool {
	for _, name := range l.required {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if !nonEmptyFile(p) {
			return false
		}
		if l.stamp != stampNone && strings.HasSuffix(name, ".a") {
			data, err := os.ReadFile(p)
			if err != nil || !stampPresent(data, l.version, l.stamp) {
				return false
			}
		}
	}
	return true
}

// validArchive reports whether path is a readable .tar.gz whose stamped .a,
// if the library has one, carries the pinned version. This rejects a stale
// same-named tarball left over from a previous version bump.
func (l nativeLib) validArchive(archivePath string) bool {
	if !nonEmptyFile(archivePath) {
		return false
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return false
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return false
	}
	tr := tar.NewReader(gz)
	stampChecked := l.stamp == stampNone
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return stampChecked
		}
		if err != nil {
			return false
		}
		if stampChecked || hdr.Typeflag != tar.TypeReg || !strings.HasSuffix(hdr.Name, ".a") {
			continue
		}
		if path.Clean(hdr.Name) != l.required[0] {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil || !stampPresent(data, l.version, l.stamp) {
			return false
		}
		stampChecked = true
	}
}

// extractTarGz unpacks archivePath into target through a staging directory
// that replaces target only once every required file is present. Absolute,
// parent-escaping, and non-regular members (links, devices, FIFOs) are
// rejected, so a hostile archive cannot write outside the staging tree.
func (l nativeLib) extractTarGz(archivePath, target string) error {
	if l.provisioned(target) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(filepath.Dir(target), ".staging-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)

	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("%s: %w", archivePath, err)
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("%s: %w", archivePath, err)
		}
		name, err := safeMemberPath(hdr.Name)
		if err != nil {
			return err
		}
		dest := filepath.Join(staging, name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeFile(dest, tr, fs.FileMode(hdr.Mode)); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsafe native archive member: %s", hdr.Name)
		}
	}
	if !l.provisioned(staging) {
		return fmt.Errorf("%s is missing required native files or version %s", archivePath, l.version)
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return os.Rename(staging, target)
}

func safeMemberPath(name string) (string, error) {
	if strings.HasPrefix(name, "/") || !filepath.IsLocal(filepath.FromSlash(name)) {
		return "", fmt.Errorf("unsafe native archive member: %s", name)
	}
	return filepath.FromSlash(name), nil
}

func writeFile(dest string, r io.Reader, mode fs.FileMode) error {
	if mode = mode.Perm(); mode == 0 {
		mode = 0o644
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// provisionNative downloads and extracts pdfium, pdf_oxide, office_oxide, and
// ONNX Runtime. A library whose versioned cache directory is already complete
// (for example a cache CI mounts from the build host) is not downloaded again.
func (d *downloader) provisionNative() error {
	if err := os.MkdirAll(d.depsDir, 0o755); err != nil {
		return err
	}
	ortVersion, err := deepdocORTVersion(d.repoRoot)
	if err != nil {
		return err
	}
	ortZip := filepath.Join(d.depsDir, ortAssetName(ortVersion, d.platform))
	if err := d.fetchORTArchive(ortZip, ortVersion); err != nil {
		return err
	}

	libs, err := d.nativeLibs()
	if err != nil {
		return err
	}
	for _, lib := range libs {
		target := lib.dir(d.nativeDir)
		if lib.provisioned(target) {
			fmt.Fprintf(d.out, "  ✓ %s v%s already provisioned at %s\n", lib.name, lib.version, target)
			continue
		}
		archive := filepath.Join(d.depsDir, lib.archive)
		if lib.validArchive(archive) {
			fmt.Fprintf(d.out, "  ✓ %s already present\n", lib.archive)
		} else {
			fmt.Fprintf(d.out, "Downloading %s from %s...\n", lib.archive, lib.url)
			if err := d.download(lib.url, archive); err != nil {
				return err
			}
			if !lib.validArchive(archive) {
				os.Remove(archive)
				return fmt.Errorf("downloaded %s is not a valid v%s archive", lib.archive, lib.version)
			}
		}
		if err := lib.extractTarGz(archive, target); err != nil {
			os.Remove(archive)
			return err
		}
		fmt.Fprintf(d.out, "  ✓ extracted %s to %s\n", lib.archive, target)
	}

	staticLib := filepath.Join(d.nativeDir, "onnxruntime", "static_lib")
	if err := extractORT(staticLib, ortZip, ortNormalizedDir(ortVersion, d.platform), ortVersion); err != nil {
		return err
	}
	if !hasStaticArchives(filepath.Join(staticLib, ortNormalizedDir(ortVersion, d.platform))) {
		return fmt.Errorf("ONNX Runtime .a files missing under %s after extraction; build.sh will refuse to link", staticLib)
	}
	fmt.Fprintf(d.out, "  ✓ ONNX Runtime %s static archives ready under %s\n", ortVersion, staticLib)
	return nil
}

// fetchORTArchive keeps the ONNX Runtime zip in step with its published
// .sha256 sidecar. The release is occasionally re-issued under the same tag
// and asset name, so a matching name alone does not prove the content is
// current. When the sidecar is unreachable, any valid local zip is kept.
func (d *downloader) fetchORTArchive(archivePath, version string) error {
	url := d.ortURL(version)
	expected := ""
	if text, err := d.fetchText(url + ".sha256"); err != nil {
		fmt.Fprintf(d.out, "  WARNING: could not fetch %s.sha256 (%v); skipping checksum\n", url, err)
	} else if fields := strings.Fields(text); len(fields) > 0 {
		expected = fields[0]
	}

	name := filepath.Base(archivePath)
	if nonEmptyFile(archivePath) {
		if expected == "" {
			if validZip(archivePath) {
				return nil
			}
		} else if actual, err := fileSHA256(archivePath); err == nil && actual == expected {
			fmt.Fprintf(d.out, "  ✓ %s checksum matches released %s\n", name, expected)
			return nil
		} else {
			fmt.Fprintf(d.out, "  %s checksum differs from released %s; re-downloading\n", name, expected)
		}
	}

	fmt.Fprintf(d.out, "Downloading %s from %s...\n", name, url)
	if err := d.download(url, archivePath); err != nil {
		return err
	}
	if expected != "" {
		actual, err := fileSHA256(archivePath)
		if err != nil {
			return err
		}
		if actual != expected {
			os.Remove(archivePath)
			return fmt.Errorf("%s checksum mismatch after download (got %s, expected %s)", name, actual, expected)
		}
		fmt.Fprintf(d.out, "  ✓ %s checksum verified\n", name)
	}
	if !validZip(archivePath) {
		os.Remove(archivePath)
		return fmt.Errorf("downloaded %s is not a valid zip archive", name)
	}
	// A fresh archive must be re-extracted, so drop the directory extracted
	// from the previous copy.
	return os.RemoveAll(filepath.Join(d.nativeDir, "onnxruntime", "static_lib", ortNormalizedDir(version, d.platform)))
}

func validZip(path string) bool {
	r, err := zip.OpenReader(path)
	if err != nil {
		return false
	}
	return r.Close() == nil
}

func hasStaticArchives(dir string) bool {
	found := false
	_ = filepath.WalkDir(dir, func(_ string, e fs.DirEntry, err error) error {
		if err == nil && !e.IsDir() && strings.HasSuffix(e.Name(), ".a") {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
}

// pruneStaleORT removes every ONNX Runtime directory except expectedDir. A
// leftover from another version or platform would otherwise be linked next to
// the current one by build.sh's archive search.
func pruneStaleORT(staticLib, expectedDir string) error {
	entries, err := os.ReadDir(staticLib)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name() == expectedDir || !strings.HasPrefix(e.Name(), "onnxruntime-") {
			continue
		}
		if err := os.RemoveAll(filepath.Join(staticLib, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// extractORT lands the ONNX Runtime static archives under
// staticLib/expectedDir. The release zip wraps everything in a top-level
// onnxruntime-v<version>-<target>/ directory, which is renamed to the name
// build.sh expects.
func extractORT(staticLib, archivePath, expectedDir, version string) error {
	if !nonEmptyFile(archivePath) {
		return fmt.Errorf("ONNX Runtime archive %s not found", archivePath)
	}
	if err := pruneStaleORT(staticLib, expectedDir); err != nil {
		return err
	}
	expected := filepath.Join(staticLib, expectedDir)
	if !hasStaticArchives(expected) {
		if err := unzipORT(archivePath, staticLib, expected); err != nil {
			return err
		}
	}
	return stampORTVersion(staticLib, expectedDir, version)
}

func unzipORT(archivePath, staticLib, expected string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := os.MkdirAll(staticLib, 0o755); err != nil {
		return err
	}
	topLevel := map[string]bool{}
	for _, f := range r.File {
		name, err := safeMemberPath(f.Name)
		if err != nil {
			return err
		}
		if top, _, _ := strings.Cut(filepath.ToSlash(name), "/"); top != "" {
			topLevel[top] = true
		}
		dest := filepath.Join(staticLib, name)
		switch {
		case f.FileInfo().IsDir():
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return err
			}
		case f.FileInfo().Mode().IsRegular():
			rc, err := f.Open()
			if err != nil {
				return err
			}
			err = writeFile(dest, rc, f.Mode())
			rc.Close()
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsafe ONNX Runtime archive member: %s", f.Name)
		}
	}
	if len(topLevel) == 1 {
		for top := range topLevel {
			extracted := filepath.Join(staticLib, top)
			if extracted == expected {
				return nil
			}
			if err := os.RemoveAll(expected); err != nil {
				return err
			}
			return os.Rename(extracted, expected)
		}
	}
	return nil
}

// stampORTVersion creates staticLib/v<version>-<sha16>/libonnxruntime.a, a
// relative symlink to the real archive. build.sh keys its ORT link flag on
// this directory; creating it while the cache is writable keeps build.sh's
// own stamp step a no-op on a read-only CI mount.
func stampORTVersion(staticLib, normalizedDir, version string) error {
	primary := filepath.Join(staticLib, normalizedDir, "lib", "libonnxruntime.a")
	if !nonEmptyFile(primary) {
		return nil
	}
	sum, err := fileSHA256(primary)
	if err != nil {
		return err
	}
	stampDir := filepath.Join(staticLib, fmt.Sprintf("v%s-%s", version, sum[:16]))
	link := filepath.Join(stampDir, "libonnxruntime.a")
	rel, err := filepath.Rel(stampDir, primary)
	if err != nil {
		return err
	}
	if current, err := os.Readlink(link); err == nil && current == rel {
		return nil
	}
	if err := os.MkdirAll(stampDir, 0o755); err != nil {
		return err
	}
	if err := os.Remove(link); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return os.Symlink(rel, link)
}
