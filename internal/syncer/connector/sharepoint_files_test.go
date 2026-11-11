//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package connector

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSharePointRootAndFolderExclusions(t *testing.T) {
	fixtures := &sharePointTestFixtures{
		drivesBody:    `{"value":[{"id":"drive","name":"Docs"},{"id":"other"}]}`,
		itemsByKey:    map[string]string{"drive:root": sharePointFolderJSON("root", "Selected")},
		childrenCalls: &atomic.Int64{},
		contentCalls:  &atomic.Int64{},
		childrenByKey: map[string]string{
			"drive:root": `{"value":[` + sharePointFolderJSON("ignore", "Archive") + `,` + sharePointFolderJSON("keep", "Projects") + `]}`,
			"drive:keep": `{"value":[` + sharePointFolderJSON("old", "Old") + `,` + sharePointFileJSON("doc", "report.pdf", "", "2020-01-01T00:00:00Z", 2) + `]}`,
		},
	}
	server := newTestSharePointServer(t, fixtures)
	c := mustSharePointConnector(t, server.URL, map[string]any{
		"drive_id": "drive", "root_folder_id": "root",
		"excluded_folder_names": []string{"Archive"},
		"excluded_folder_paths": []string{"Projects/Old"},
	})
	session, err := c.OpenSync(t.Context(), SyncRequest{})
	if err != nil {
		t.Fatal(err)
	}
	docs := drainSharePointSync(t, session)
	if len(docs) != 1 || docs[0].SourceID != "drive:doc" || docs[0].Metadata["path"] != "Projects/report.pdf" {
		t.Fatalf("unexpected documents: %+v", docs)
	}
	if fixtures.childrenCalls.Load() != 2 || fixtures.contentCalls.Load() != 0 {
		t.Fatal("must not list excluded subtrees or other libraries, or download while listing")
	}
}

func TestSharePointPerFileSkipReasons(t *testing.T) {
	fixtures := &sharePointTestFixtures{
		drivesBody:   `{"value":[{"id":"drive"}]}`,
		contentCalls: &atomic.Int64{},
		childrenByKey: map[string]string{"drive:/": `{"value":[
			{"id":"mime","name":"video.txt","file":{"mimeType":"video/mp4"}},
			{"id":"ext","name":"video.MP4","file":{"mimeType":"application/octet-stream"}},
			{"id":"rule","name":"data.CSV","file":{}},
			{"id":"unsupported","name":"app.exe","file":{}},
			{"id":"good","name":"report.pdf","file":{}}
		]}`},
	}
	server := newTestSharePointServer(t, fixtures)
	c := mustSharePointConnector(t, server.URL, map[string]any{"excluded_extensions": []string{"csv"}, "exclude_videos": true})
	session, err := c.OpenSync(t.Context(), SyncRequest{})
	if err != nil {
		t.Fatal(err)
	}
	docs := drainSharePointSync(t, session)
	want := map[string]string{"drive:mime": "excluded video", "drive:ext": "excluded video", "drive:rule": "excluded extension", "drive:unsupported": "unsupported file type", "drive:good": ""}
	if len(docs) != len(want) {
		t.Fatalf("documents = %d", len(docs))
	}
	for _, doc := range docs {
		if reason, ok := want[doc.SourceID]; !ok || reason != doc.SkipReason {
			t.Errorf("unexpected skip: %+v", doc)
		}
	}
	if fixtures.contentCalls.Load() != 0 {
		t.Fatal("listing downloaded content")
	}
}

func TestSharePointPruneRetainsExcludedAndOutOfScopeFiles(t *testing.T) {
	fixtures := &sharePointTestFixtures{
		drivesBody: `{"value":[{"id":"drive"},{"id":"other"}]}`,
		itemsByKey: map[string]string{"drive:root": sharePointFolderJSON("root", "Selected")},
		childrenByKey: map[string]string{
			"drive:/":    `{"value":[` + sharePointFolderJSON("root", "Selected") + `,` + sharePointFileJSON("outside", "video.mp4", "", "", 1) + `]}`,
			"drive:root": `{"value":[` + sharePointFileJSON("inside", "report.pdf", "", "", 1) + `]}`,
			"other:/":    `{"value":[` + sharePointFileJSON("other", "other.txt", "", "", 1) + `]}`,
		},
	}
	server := newTestSharePointServer(t, fixtures)
	c := mustSharePointConnector(t, server.URL, map[string]any{
		"drive_id": "drive", "root_folder_id": "root", "exclude_videos": true,
		"excluded_extensions": []string{"pdf"}, "excluded_folder_names": []string{"Selected"},
	})
	prune, err := c.OpenPrune(t.Context(), PruneRequest{})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for {
		batch, err := prune.NextBatch(t.Context())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, doc := range batch.Documents {
			ids[doc.SourceID] = true
		}
	}
	if len(ids) != 3 || !ids["drive:inside"] || !ids["drive:outside"] || !ids["other:other"] {
		t.Fatalf("unsafe prune snapshot: %v", ids)
	}
}

func TestSharePointFingerprint(t *testing.T) {
	file := sharePointItem{CTag: "content-1", ETag: "metadata-1", Size: 5, Name: "file.txt", relativePath: "Docs/file.txt"}
	first := sharePointFingerprint(file)
	if len(first) != 32 {
		t.Fatal("fingerprint must fit the document content_hash column")
	}
	file.ETag = "metadata-2"
	if first == "" || sharePointFingerprint(file) != first {
		t.Fatal("cTag must take precedence over eTag")
	}
	file.CTag = "content-2"
	if sharePointFingerprint(file) == first {
		t.Fatal("changed content tag must change fingerprint")
	}
	file.CTag = ""
	if sharePointFingerprint(file) == "" {
		t.Fatal("missing cTag must fall back to eTag")
	}
	file.ETag = ""
	if sharePointFingerprint(file) != "" {
		t.Fatal("without a tag, never assume unchanged")
	}
}

func TestSharePointFetchChecksRevision(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "stable", true: "changed"}[changed], func(t *testing.T) {
			tag := "v1"
			if changed {
				tag = "v2"
			}
			fixtures := &sharePointTestFixtures{
				contentByKey: map[string]string{"drive:file": "abc"},
				itemsByKey:   map[string]string{"drive:file": `{"id":"file","size":3,"cTag":"` + tag + `","file":{}}`},
			}
			server := newTestSharePointServer(t, fixtures)
			session := &sharePointSyncSession{connector: mustSharePointConnector(t, server.URL, nil)}
			ref := sharePointFetchRef("drive", sharePointItem{ID: "file", CTag: "v1", Size: 3})
			blob, err := session.Fetch(context.Background(), *ref)
			if changed {
				if err == nil || len(blob) != 0 || !strings.Contains(err.Error(), "changed during download") {
					t.Fatalf("Fetch = %q, %v", blob, err)
				}
			} else if err != nil || string(blob) != "abc" {
				t.Fatalf("Fetch = %q, %v", blob, err)
			}
		})
	}
}

func TestSharePointRejectsInvalidScope(t *testing.T) {
	if _, err := NewSharePointConnector(map[string]any{"root_folder_id": "root"}); err == nil {
		t.Fatal("root without drive accepted")
	}
	if _, err := NewSharePointConnector(map[string]any{"exclude_videos": "false"}); err == nil {
		t.Fatal("invalid boolean accepted")
	}
	fixtures := &sharePointTestFixtures{
		drivesBody: `{"value":[{"id":"drive"}]}`,
		itemsByKey: map[string]string{"drive:file": `{"id":"file","file":{}}`},
	}
	server := newTestSharePointServer(t, fixtures)
	for _, config := range []map[string]any{{"drive_id": "missing"}, {"drive_id": "drive", "root_folder_id": "file"}} {
		c := mustSharePointConnector(t, server.URL, config)
		if err := c.Validate(t.Context()); err == nil {
			t.Errorf("accepted invalid scope: %v", config)
		}
	}
}

func TestSharePointRejectsForeignPaginationOrigin(t *testing.T) {
	var calls atomic.Int64
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer foreign.Close()
	fixtures := &sharePointTestFixtures{
		drivesBody: `{"value":[],"@odata.nextLink":"` + foreign.URL + `/steal"}`,
	}
	server := newTestSharePointServer(t, fixtures)
	c := mustSharePointConnector(t, server.URL, nil)
	if _, err := c.OpenSync(t.Context(), SyncRequest{}); err == nil {
		t.Fatal("foreign pagination URL accepted")
	}
	if calls.Load() != 0 {
		t.Fatal("request sent to foreign origin")
	}
}

func TestSharePointFetchDoesNotSwallowAccessFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "oauth2") {
			w.Write([]byte(`{"access_token":"test","expires_in":3600}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	session := &sharePointSyncSession{connector: mustSharePointConnector(t, server.URL, nil)}
	_, err := session.Fetch(t.Context(), *sharePointFetchRef("drive", sharePointItem{ID: "file"}))
	var apiErr *sharePointAPIError
	if !errors.As(err, &apiErr) || apiErr.status != http.StatusForbidden {
		t.Fatalf("Fetch error = %v", err)
	}
}
