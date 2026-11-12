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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const (
	officeOxideModule = "github.com/yfedoseev/office_oxide/go"
	pdfOxideModule    = "github.com/yfedoseev/pdf_oxide/go"

	// ortReleaseTag is the infiniflow/ragflow-build release that publishes the
	// minimal ONNX Runtime static builds. The patch2 rebuild re-adds Clip@13,
	// which the DeepDoc rec/tsr weights need; its asset and directory names keep
	// the bare DeepDocORTVersion, so only the tag differs from the version.
	ortReleaseTag = "onnxruntime-v1.29.0-patch2"
)

type platform struct {
	goos, goarch string
}

// resolvePlatform normalizes the target platform the same way build.sh's
// detect_target_platform does. Empty values default to the host.
func resolvePlatform(rawOS, rawArch string) (platform, error) {
	if rawOS == "" {
		rawOS = runtime.GOOS
	}
	if rawArch == "" {
		rawArch = runtime.GOARCH
	}
	var p platform
	switch strings.ToLower(rawOS) {
	case "linux":
		p.goos = "linux"
	case "darwin":
		p.goos = "darwin"
	default:
		return p, fmt.Errorf("unsupported target OS %q; expected linux or darwin", rawOS)
	}
	switch strings.ToLower(rawArch) {
	case "amd64", "x86_64":
		p.goarch = "amd64"
	case "arm64", "aarch64":
		p.goarch = "arm64"
	default:
		return p, fmt.Errorf("unsupported target arch %q; expected amd64 or arm64", rawArch)
	}
	return p, nil
}

// Release asset names per platform. The tarballs extract into the same internal
// layout on every platform, so build.sh resolves identical paths everywhere.
var (
	officeOxideAssets = map[platform]string{
		{"linux", "amd64"}:  "native-linux-x86_64.tar.gz",
		{"linux", "arm64"}:  "native-linux-aarch64.tar.gz",
		{"darwin", "amd64"}: "native-macos-x86_64.tar.gz",
		{"darwin", "arm64"}: "native-macos-aarch64.tar.gz",
	}
	pdfiumStaticAssets = map[platform]string{
		{"linux", "amd64"}:  "pdfium-linux-x64-static.tgz",
		{"linux", "arm64"}:  "pdfium-linux-arm64-static.tgz",
		{"darwin", "amd64"}: "pdfium-mac-x64-static.tgz",
		{"darwin", "arm64"}: "pdfium-mac-arm64-static.tgz",
	}
	pdfOxideAssets = map[platform]string{
		{"linux", "amd64"}:  "pdf_oxide-go-ffi-linux-amd64.tar.gz",
		{"linux", "arm64"}:  "pdf_oxide-go-ffi-linux-arm64.tar.gz",
		{"darwin", "amd64"}: "pdf_oxide-go-ffi-darwin-amd64.tar.gz",
		{"darwin", "arm64"}: "pdf_oxide-go-ffi-darwin-arm64.tar.gz",
	}
)

// findRepoRoot walks up from dir to the directory holding go.mod.
func findRepoRoot(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found: run ragflow-deps from inside the repository")
		}
		dir = parent
	}
}

// nativeLibsDir mirrors ragflow_native_libs_dir in native_lib_version.sh: an
// empty override falls back to the home default.
func nativeLibsDir(override, home string) string {
	if override != "" {
		return override
	}
	return filepath.Join(home, "ragflow-native-libs")
}

// goModuleVersion returns the bare version (no "v") go.mod pins for module.
func goModuleVersion(repoRoot, module string) (string, error) {
	path := filepath.Join(repoRoot, "go.mod")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`(?m)^\s*(?:require\s+)?` + regexp.QuoteMeta(module) + `\s+v(\S+)`)
	m := re.FindSubmatch(data)
	if m == nil {
		return "", fmt.Errorf("%s version not found in %s", module, path)
	}
	return string(m[1]), nil
}

// deepdocORTVersion reads DeepDocORTVersion from internal/common/environments.go.
func deepdocORTVersion(repoRoot string) (string, error) {
	path := filepath.Join(repoRoot, "internal", "common", "environments.go")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	m := regexp.MustCompile(`DeepDocORTVersion\s*=\s*"([^"]+)"`).FindSubmatch(data)
	if m == nil {
		return "", fmt.Errorf("DeepDocORTVersion const not found in %s", path)
	}
	return string(m[1]), nil
}

// pdfiumStaticVersion asks tools/scripts/native_lib_version.sh, the shared
// source build.sh also sources, for the pinned pdfium build number.
func pdfiumStaticVersion(repoRoot string) (string, error) {
	script := filepath.Join(repoRoot, "tools", "scripts", "native_lib_version.sh")
	if _, err := os.Stat(script); err != nil {
		return "", err
	}
	out, err := exec.Command("bash", "-c", `source "$1" >/dev/null 2>&1; pdfium_static_version`, "_", script).Output()
	if err != nil {
		return "", fmt.Errorf("pdfium_static_version failed: %w", err)
	}
	version := strings.TrimSpace(string(out))
	if version == "" {
		return "", errors.New("pdfium_static_version produced no output")
	}
	return version, nil
}

// ortTarget is the infiniflow/ragflow-build release target. No osx-x86_64
// build is published; Intel Macs use the universal archive.
func ortTarget(p platform) string {
	if p.goos == "linux" {
		if p.goarch == "amd64" {
			return "linux-x86_64"
		}
		return "linux-aarch64"
	}
	if p.goarch == "amd64" {
		return "osx-universal"
	}
	return "osx-arm64"
}

func ortAssetName(version string, p platform) string {
	return fmt.Sprintf("onnxruntime-v%s-%s.zip", version, ortTarget(p))
}

// ortNormalizedDir is the directory name build.sh's ort_static_lib_prefix glob
// expects under onnxruntime/static_lib; extraction renames the zip's top-level
// directory to it.
func ortNormalizedDir(version string, p platform) string {
	if p.goos == "linux" {
		arch := "x64"
		if p.goarch == "arm64" {
			arch = "aarch64"
		}
		return fmt.Sprintf("onnxruntime-linux-%s-static_lib-%s-glibc2_28", arch, version)
	}
	arch := "arm64"
	if p.goarch == "amd64" {
		arch = "universal"
	}
	return fmt.Sprintf("onnxruntime-osx-%s-static_lib-%s", arch, version)
}

func (d *downloader) ortURL(version string) string {
	return fmt.Sprintf("%shttps://github.com/infiniflow/ragflow-build/releases/download/%s/%s", d.ghProxy, ortReleaseTag, ortAssetName(version, d.platform))
}

func (d *downloader) pdfiumURL(version string) string {
	return fmt.Sprintf("%shttps://github.com/kognitos/pdfium-static/releases/download/chromium%%2F%s/%s", d.ghProxy, version, pdfiumStaticAssets[d.platform])
}

// pdfiumLocalAssetName versions the local tarball name. The upstream asset
// name carries no version and the .a has no stamp, so the name is the only way
// to keep a stale tarball from being reused after a pdfium bump.
func pdfiumLocalAssetName(version string, p platform) string {
	base := pdfiumStaticAssets[p]
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext) + "-" + version + ext
}

func (d *downloader) pdfOxideURL(version string) string {
	return fmt.Sprintf("%shttps://github.com/yfedoseev/pdf_oxide/releases/download/v%s/%s", d.ghProxy, version, pdfOxideAssets[d.platform])
}

func (d *downloader) officeOxideURL(version string) string {
	return fmt.Sprintf("%shttps://github.com/yfedoseev/office_oxide/releases/download/v%s/%s", d.ghProxy, version, officeOxideAssets[d.platform])
}
