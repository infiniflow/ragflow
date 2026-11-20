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
	"context"
	"strconv"
	"strings"
	"testing"
)

// artifactFixture wires a fake lister/reader pair into collectArtifactsInto so
// the traversal can be exercised without an SSH server.
type artifactFixture struct {
	dirs    map[string][]remoteEntry
	files   map[string]string
	readErr map[string]error
	listed  []string
}

func (f *artifactFixture) list(_ context.Context, dir string) ([]remoteEntry, error) {
	f.listed = append(f.listed, dir)
	return f.dirs[dir], nil
}

func (f *artifactFixture) read(_ context.Context, path string) (string, error) {
	if err, ok := f.readErr[path]; ok {
		return "", err
	}
	return f.files[path], nil
}

func fileEntry(name string, size int64) remoteEntry {
	return remoteEntry{Name: name, Size: size, Type: "f"}
}

func dirEntry(name string) remoteEntry {
	return remoteEntry{Name: name, Type: "d"}
}

// Artifact names must be relative to the artifacts root, like the local
// provider, otherwise files with the same base name in different
// subdirectories collide and lose their directory.
func TestCollectArtifactsKeepsRelativePaths(t *testing.T) {
	f := &artifactFixture{
		dirs: map[string][]remoteEntry{
			"/w/artifacts":           {fileEntry("report.csv", 42), dirEntry("plots")},
			"/w/artifacts/plots":     {fileEntry("chart.png", 7), dirEntry("sub")},
			"/w/artifacts/plots/sub": {fileEntry("chart.png", 7)},
		},
		files: map[string]string{
			"/w/artifacts/report.csv":          "a,b",
			"/w/artifacts/plots/chart.png":     "png",
			"/w/artifacts/plots/sub/chart.png": "png",
		},
	}
	p := &SSHProvider{maxArtifacts: 10, maxArtifactBytes: 1 << 20}

	var out []map[string]any
	listings := 0
	if err := p.collectArtifactsInto(context.Background(), "/w/artifacts", "", 0, &listings, &out, f.list, f.read); err != nil {
		t.Fatalf("collectArtifactsInto: %v", err)
	}

	var names []string
	for _, a := range out {
		names = append(names, a["name"].(string))
	}
	want := []string{"report.csv", "plots/chart.png", "plots/sub/chart.png"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("artifact names = %v, want %v", names, want)
	}
}

// maxArtifacts must bound the whole tree. A per-subtree check lets every
// directory add another maxArtifacts entries.
func TestCollectArtifactsAppliesCapAcrossSubtrees(t *testing.T) {
	f := &artifactFixture{
		dirs: map[string][]remoteEntry{
			"/w/artifacts":   {dirEntry("a"), dirEntry("b")},
			"/w/artifacts/a": {fileEntry("one.png", 1), fileEntry("two.png", 1)},
			"/w/artifacts/b": {fileEntry("three.png", 1)},
		},
		files: map[string]string{
			"/w/artifacts/a/one.png":   "x",
			"/w/artifacts/a/two.png":   "x",
			"/w/artifacts/b/three.png": "x",
		},
	}
	p := &SSHProvider{maxArtifacts: 2, maxArtifactBytes: 1 << 20}

	var out []map[string]any
	listings := 0
	err := p.collectArtifactsInto(context.Background(), "/w/artifacts", "", 0, &listings, &out, f.list, f.read)
	if err == nil || !strings.Contains(err.Error(), "more than 2 artifacts") {
		t.Fatalf("expected the global artifact cap to trip, got err=%v, artifacts=%d", err, len(out))
	}
	if len(out) != 2 {
		t.Fatalf("collected %d artifacts before tripping the cap, want 2", len(out))
	}
}

func TestCollectArtifactsRejectsNonRegularEntries(t *testing.T) {
	f := &artifactFixture{
		dirs: map[string][]remoteEntry{
			"/w/artifacts": {{Name: "link.png", Type: "l"}},
		},
	}
	p := &SSHProvider{maxArtifacts: 10, maxArtifactBytes: 1 << 20}

	var out []map[string]any
	listings := 0
	err := p.collectArtifactsInto(context.Background(), "/w/artifacts", "", 0, &listings, &out, f.list, f.read)
	if err == nil || !strings.Contains(err.Error(), "unsupported artifact entry") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
}

// Directories produce no artifacts, so maxArtifacts does not bound the walk.
// A wide tree of empty directories must stop at the listing budget instead of
// issuing one remote find per directory.
func TestCollectArtifactsBoundsWideDirectoryWalk(t *testing.T) {
	root := make([]remoteEntry, 0, sshMaxArtifactListings+10)
	for i := 0; i < sshMaxArtifactListings+10; i++ {
		root = append(root, dirEntry("d"+strconv.Itoa(i)))
	}
	f := &artifactFixture{dirs: map[string][]remoteEntry{"/w/artifacts": root}}
	p := &SSHProvider{maxArtifacts: 20, maxArtifactBytes: 1 << 20}

	var out []map[string]any
	listings := 0
	err := p.collectArtifactsInto(context.Background(), "/w/artifacts", "", 0, &listings, &out, f.list, f.read)
	if err == nil || !strings.Contains(err.Error(), "listings") {
		t.Fatalf("expected the listing budget to trip, got %v", err)
	}
	if len(f.listed) > sshMaxArtifactListings+1 {
		t.Fatalf("walked %d directories, want at most %d", len(f.listed), sshMaxArtifactListings+1)
	}
}

func TestCollectArtifactsBoundsNestingDepth(t *testing.T) {
	dirs := map[string][]remoteEntry{}
	dir := "/w/artifacts"
	for i := 0; i <= sshMaxArtifactDepth+2; i++ {
		next := dir + "/n"
		dirs[dir] = []remoteEntry{dirEntry("n")}
		dir = next
	}
	f := &artifactFixture{dirs: dirs}
	p := &SSHProvider{maxArtifacts: 20, maxArtifactBytes: 1 << 20}

	var out []map[string]any
	listings := 0
	err := p.collectArtifactsInto(context.Background(), "/w/artifacts", "", 0, &listings, &out, f.list, f.read)
	if err == nil || !strings.Contains(err.Error(), "nesting exceeds") {
		t.Fatalf("expected the depth limit to trip, got %v", err)
	}
}
