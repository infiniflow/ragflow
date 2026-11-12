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
	"net/http"
	"path/filepath"
	"slices"
	"testing"
)

const artifactURL = "https://example.invalid/artifact"

func truncatedHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Length", "100")
	_, _ = w.Write([]byte("payload"))
}

func TestFailedDownloadPreservesPreviousFile(t *testing.T) {
	for name, handler := range map[string]http.Handler{
		"not found":   http.NotFoundHandler(),
		"interrupted": http.HandlerFunc(truncatedHandler),
	} {
		t.Run(name, func(t *testing.T) {
			d := testDownloader(t, handler)
			dir := t.TempDir()
			dest := filepath.Join(dir, "artifact")
			mustWrite(t, dest, []byte("previous valid artifact"))
			if err := d.download(artifactURL, dest); err == nil {
				t.Fatal("download succeeded")
			}
			if got := string(mustRead(t, dest)); got != "previous valid artifact" {
				t.Errorf("previous file replaced with %q", got)
			}
			if names := listDir(t, dir); !slices.Equal(names, []string{"artifact"}) {
				t.Errorf("temporary files left behind: %v", names)
			}
		})
	}
}

func TestSuccessfulDownloadReplacesPreviousFile(t *testing.T) {
	remote := newFakeRemote()
	remote.files["/artifact"] = []byte("payload")
	d := testDownloader(t, remote)
	dir := t.TempDir()
	dest := filepath.Join(dir, "artifact")
	mustWrite(t, dest, []byte("old"))
	if err := d.download(artifactURL, dest); err != nil {
		t.Fatal(err)
	}
	if got := string(mustRead(t, dest)); got != "payload" {
		t.Errorf("dest = %q", got)
	}
	if names := listDir(t, dir); !slices.Equal(names, []string{"artifact"}) {
		t.Errorf("temporary files left behind: %v", names)
	}
}

func TestEmptyDownloadFails(t *testing.T) {
	remote := newFakeRemote()
	remote.files["/artifact"] = []byte{}
	d := testDownloader(t, remote)
	dest := filepath.Join(t.TempDir(), "artifact")
	if err := d.download(artifactURL, dest); err == nil {
		t.Fatal("empty body accepted")
	}
	if nonEmptyFile(dest) {
		t.Error("empty download created dest")
	}
}

func TestDownloadRetriesServerErrorsOnly(t *testing.T) {
	remote := newFakeRemote()
	remote.status["/artifact"] = http.StatusBadGateway
	d := testDownloader(t, remote)
	dest := filepath.Join(t.TempDir(), "artifact")
	if err := d.download(artifactURL, dest); err == nil {
		t.Fatal("502 accepted")
	}
	if n := remote.hitCount("/artifact"); n != downloadAttempts {
		t.Errorf("502 attempted %d times; want %d", n, downloadAttempts)
	}

	remote.resetHits()
	remote.status["/artifact"] = http.StatusNotFound
	if err := d.download(artifactURL, dest); err == nil {
		t.Fatal("404 accepted")
	}
	if n := remote.hitCount("/artifact"); n != 1 {
		t.Errorf("404 attempted %d times; want 1", n)
	}
}

func TestHuggingFaceTokenOnlyGoesToHuggingFace(t *testing.T) {
	var auth []string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = append(auth, r.Header.Get("Authorization"))
		_, _ = w.Write([]byte("payload"))
	})
	d := testDownloader(t, handler)
	d.hfToken = "secret"
	dir := t.TempDir()
	if err := d.download(d.hfURL("BAAI/bge-m3", "vocab.txt"), filepath.Join(dir, "a")); err != nil {
		t.Fatal(err)
	}
	if err := d.download(artifactURL, filepath.Join(dir, "b")); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(auth, []string{"Bearer secret", ""}) {
		t.Errorf("Authorization headers = %q", auth)
	}
}
