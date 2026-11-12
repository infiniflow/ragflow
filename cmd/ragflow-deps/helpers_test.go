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
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// tarEntry is one member of a fixture tarball. A zero typeflag means a
// regular file.
type tarEntry struct {
	name     string
	body     []byte
	typeflag byte
	linkname string
}

func writeTarGz(t *testing.T, path string, entries []tarEntry) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Mode: 0o644, Typeflag: e.typeflag, Linkname: e.linkname}
		if hdr.Typeflag == 0 {
			hdr.Typeflag = tar.TypeReg
		}
		if hdr.Typeflag == tar.TypeReg {
			hdr.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if hdr.Typeflag == tar.TypeReg {
			if _, err := tw.Write(e.body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, path, buf.Bytes())
}

// writeORTZip builds a zip shaped like the infiniflow/ragflow-build release:
// one top-level directory holding lib/libonnxruntime.a.
func writeORTZip(t *testing.T, path, topDir string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(topDir + "/lib/libonnxruntime.a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("!<arch>\nort-payload")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, path, buf.Bytes())
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// fakeRepo lays out the files the version resolvers read.
func fakeRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), []byte("module fixture\n\nrequire (\n"+
		"\tgithub.com/yfedoseev/office_oxide/go v0.1.12\n"+
		"\tgithub.com/yfedoseev/pdf_oxide/go v0.4.1\n)\n"))
	mustWrite(t, filepath.Join(root, "internal", "common", "environments.go"),
		[]byte("package common\n\nconst DeepDocORTVersion = \"9.9.9\"\n"))
	mustWrite(t, filepath.Join(root, "tools", "scripts", "native_lib_version.sh"),
		[]byte("pdfium_static_version() { echo 1234; }\n"))
	return root
}

// fakeRemote serves fixed bodies by URL path. Every request the downloader
// makes, whatever its host, is routed to it, so the real release URLs are
// exercised without network access.
type fakeRemote struct {
	mu     sync.Mutex
	files  map[string][]byte
	status map[string]int
	hits   []string
}

func newFakeRemote() *fakeRemote {
	return &fakeRemote{files: map[string][]byte{}, status: map[string]int{}}
}

func (f *fakeRemote) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.hits = append(f.hits, r.URL.Path)
	body, ok := f.files[r.URL.Path]
	code := f.status[r.URL.Path]
	f.mu.Unlock()
	if code != 0 {
		w.WriteHeader(code)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	_, _ = w.Write(body)
}

func (f *fakeRemote) hitCount(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, h := range f.hits {
		if h == path {
			n++
		}
	}
	return n
}

func (f *fakeRemote) resetHits() {
	f.mu.Lock()
	f.hits = nil
	f.mu.Unlock()
}

type rerouteTransport struct {
	target *url.URL
}

func (rt rerouteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.URL.Scheme = rt.target.Scheme
	r.URL.Host = rt.target.Host
	r.Host = rt.target.Host
	return http.DefaultTransport.RoundTrip(r)
}

// testDownloader returns a downloader rooted in a fake repository whose HTTP
// traffic goes to remote.
func testDownloader(t *testing.T, remote http.Handler) *downloader {
	t.Helper()
	srv := httptest.NewServer(remote)
	t.Cleanup(srv.Close)
	target, _ := url.Parse(srv.URL)
	root := fakeRepo(t)
	return &downloader{
		out:        io.Discard,
		client:     &http.Client{Transport: rerouteTransport{target: target}},
		repoRoot:   root,
		depsDir:    filepath.Join(root, "deps"),
		nativeDir:  filepath.Join(t.TempDir(), "native-libs"),
		hfEndpoint: defaultHFEndpoint,
		platform:   platform{"linux", "amd64"},
		retryDelay: func(int) {},
	}
}
