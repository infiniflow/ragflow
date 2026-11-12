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
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"ragflow/internal/common"
)

func TestResolvePlatformNormalizesBuildShAliases(t *testing.T) {
	cases := []struct {
		os, arch string
		want     platform
	}{
		{"Linux", "x86_64", platform{"linux", "amd64"}},
		{"linux", "amd64", platform{"linux", "amd64"}},
		{"Darwin", "aarch64", platform{"darwin", "arm64"}},
		{"darwin", "arm64", platform{"darwin", "arm64"}},
	}
	for _, c := range cases {
		got, err := resolvePlatform(c.os, c.arch)
		if err != nil || got != c.want {
			t.Errorf("resolvePlatform(%q, %q) = %v, %v; want %v", c.os, c.arch, got, err, c.want)
		}
	}
}

func TestResolvePlatformRejectsUnsupported(t *testing.T) {
	if _, err := resolvePlatform("windows", "amd64"); err == nil {
		t.Error("windows accepted")
	}
	if _, err := resolvePlatform("linux", "riscv64"); err == nil {
		t.Error("riscv64 accepted")
	}
}

func TestEveryPlatformHasEveryAsset(t *testing.T) {
	for _, p := range []platform{{"linux", "amd64"}, {"linux", "arm64"}, {"darwin", "amd64"}, {"darwin", "arm64"}} {
		for name, assets := range map[string]map[platform]string{"office_oxide": officeOxideAssets, "pdfium": pdfiumStaticAssets, "pdf_oxide": pdfOxideAssets} {
			if assets[p] == "" {
				t.Errorf("%s has no asset for %v", name, p)
			}
		}
	}
}

func TestGoModuleVersionReadsItsOwnModuleLine(t *testing.T) {
	root := fakeRepo(t)
	pdf, err := goModuleVersion(root, pdfOxideModule)
	if err != nil || pdf != "0.4.1" {
		t.Errorf("pdf_oxide = %q, %v; want 0.4.1", pdf, err)
	}
	office, err := goModuleVersion(root, officeOxideModule)
	if err != nil || office != "0.1.12" {
		t.Errorf("office_oxide = %q, %v; want 0.1.12", office, err)
	}
	if _, err := goModuleVersion(root, "example.com/absent"); err == nil {
		t.Error("absent module resolved")
	}
}

func TestDeepDocORTVersionMatchesCommon(t *testing.T) {
	if got, err := deepdocORTVersion(fakeRepo(t)); err != nil || got != "9.9.9" {
		t.Errorf("fixture version = %q, %v; want 9.9.9", got, err)
	}
	root, err := findRepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	got, err := deepdocORTVersion(root)
	if err != nil || got != common.DeepDocORTVersion {
		t.Errorf("repo version = %q, %v; want common.DeepDocORTVersion %q", got, err, common.DeepDocORTVersion)
	}
	if _, err := deepdocORTVersion(t.TempDir()); err == nil {
		t.Error("missing environments.go resolved a version")
	}
}

func TestPdfiumStaticVersionReadsSharedScript(t *testing.T) {
	if got, err := pdfiumStaticVersion(fakeRepo(t)); err != nil || got != "1234" {
		t.Errorf("fixture version = %q, %v; want 1234", got, err)
	}
	root, err := findRepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := pdfiumStaticVersion(root); err != nil || got == "" {
		t.Errorf("repo version = %q, %v", got, err)
	}
}

func TestDeepDocModelFilesMatchCommon(t *testing.T) {
	if !slices.Equal(deepdocModelFiles, common.DeepDocModelFiles) {
		t.Errorf("deepdocModelFiles %v != common.DeepDocModelFiles %v", deepdocModelFiles, common.DeepDocModelFiles)
	}
}

func TestORTNamesMatchReleaseAndBuildSh(t *testing.T) {
	cases := []struct {
		p           platform
		asset, norm string
	}{
		{platform{"linux", "amd64"}, "onnxruntime-v1.29.0-linux-x86_64.zip", "onnxruntime-linux-x64-static_lib-1.29.0-glibc2_28"},
		{platform{"linux", "arm64"}, "onnxruntime-v1.29.0-linux-aarch64.zip", "onnxruntime-linux-aarch64-static_lib-1.29.0-glibc2_28"},
		{platform{"darwin", "arm64"}, "onnxruntime-v1.29.0-osx-arm64.zip", "onnxruntime-osx-arm64-static_lib-1.29.0"},
		{platform{"darwin", "amd64"}, "onnxruntime-v1.29.0-osx-universal.zip", "onnxruntime-osx-universal-static_lib-1.29.0"},
	}
	for _, c := range cases {
		if got := ortAssetName("1.29.0", c.p); got != c.asset {
			t.Errorf("ortAssetName(%v) = %q; want %q", c.p, got, c.asset)
		}
		if got := ortNormalizedDir("1.29.0", c.p); got != c.norm {
			t.Errorf("ortNormalizedDir(%v) = %q; want %q", c.p, got, c.norm)
		}
	}
}

func TestURLBuildersSelectPlatformAssets(t *testing.T) {
	d := &downloader{platform: platform{"darwin", "arm64"}}
	for got, want := range map[string]string{
		d.pdfiumURL("7809"):        "https://github.com/kognitos/pdfium-static/releases/download/chromium%2F7809/pdfium-mac-arm64-static.tgz",
		d.pdfOxideURL("0.4.1"):     "https://github.com/yfedoseev/pdf_oxide/releases/download/v0.4.1/pdf_oxide-go-ffi-darwin-arm64.tar.gz",
		d.officeOxideURL("0.1.12"): "https://github.com/yfedoseev/office_oxide/releases/download/v0.1.12/native-macos-aarch64.tar.gz",
		d.ortURL("1.29.0"):         "https://github.com/infiniflow/ragflow-build/releases/download/" + ortReleaseTag + "/onnxruntime-v1.29.0-osx-arm64.zip",
	} {
		if got != want {
			t.Errorf("got %q; want %q", got, want)
		}
	}
	d.ghProxy = ghProxyURL
	if url := d.pdfOxideURL("0.4.1"); !strings.HasPrefix(url, "https://gh-proxy.com/https://github.com/") {
		t.Errorf("GitHub proxy not applied: %q", url)
	}
}

func TestPdfiumLocalAssetNameIsVersionedButURLIsNot(t *testing.T) {
	p := platform{"linux", "amd64"}
	if got := pdfiumLocalAssetName("9999", p); got != "pdfium-linux-x64-static-9999.tgz" {
		t.Errorf("local name = %q", got)
	}
	if got := pdfiumLocalAssetName("9999", platform{"darwin", "arm64"}); got != "pdfium-mac-arm64-static-9999.tgz" {
		t.Errorf("local name = %q", got)
	}
	d := &downloader{platform: p}
	if url := d.pdfiumURL("9999"); !strings.HasSuffix(url, "/pdfium-linux-x64-static.tgz") {
		t.Errorf("upstream asset name must stay version-agnostic: %q", url)
	}
}

func TestResolveGHProxyRejectsArbitraryHosts(t *testing.T) {
	if _, err := resolveGHProxy("https://attacker.invalid/"); err == nil {
		t.Error("untrusted proxy accepted")
	}
	for _, raw := range []string{"https://gh-proxy.com/", "https://gh-proxy.com"} {
		if got, err := resolveGHProxy(raw); err != nil || got != ghProxyURL {
			t.Errorf("resolveGHProxy(%q) = %q, %v", raw, got, err)
		}
	}
	if got, err := resolveGHProxy("  "); err != nil || got != "" {
		t.Errorf("blank proxy = %q, %v", got, err)
	}
}

func TestNativeLibsDirOverride(t *testing.T) {
	if got := nativeLibsDir("", "/home/alice"); got != filepath.Join("/home/alice", "ragflow-native-libs") {
		t.Errorf("default = %q", got)
	}
	if got := nativeLibsDir("/mnt/cache", "/home/alice"); got != "/mnt/cache" {
		t.Errorf("override = %q", got)
	}
}
