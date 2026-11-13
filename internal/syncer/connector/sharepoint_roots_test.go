// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package connector

import (
	"encoding/json"
	"reflect"
	"sort"
	"sync/atomic"
	"testing"
)

func folderWithParent(id, name, parent string) string {
	body, _ := json.Marshal(map[string]any{
		"id": id, "name": name, "folder": map[string]any{},
		"parentReference": map[string]string{"id": parent},
	})
	return string(body)
}

func multiRootFixtures() *sharePointTestFixtures {
	return &sharePointTestFixtures{
		drivesBody:    `{"value":[{"id":"drive"},{"id":"outside"}]}`,
		childrenCalls: &atomic.Int64{}, contentCalls: &atomic.Int64{},
		itemsByKey: map[string]string{
			"drive:top":      folderWithParent("top", "", ""),
			"drive:z-parent": folderWithParent("z-parent", "Parent", "top"),
			"drive:middle":   folderWithParent("middle", "Middle", "z-parent"),
			"drive:a-child":  folderWithParent("a-child", "Child", "middle"),
			"drive:second":   folderWithParent("second", "Second", "top"),
			"drive:third":    folderWithParent("third", "Third", "top"),
		},
		childrenByKey: map[string]string{
			"drive:z-parent": `{"value":[` + sharePointFolderJSON("middle", "Middle") + `,` + sharePointFileJSON("parent-file", "same.txt", "", "", 1) + `]}`,
			"drive:middle":   `{"value":[` + sharePointFolderJSON("a-child", "Child") + `]}`,
			"drive:a-child":  `{"value":[` + sharePointFileJSON("child-file", "same.txt", "", "", 1) + `]}`,
			"drive:second":   `{"value":[` + sharePointFileJSON("second-file", "same.txt", "", "", 1) + `]}`,
			"drive:third":    `{"value":[` + sharePointFileJSON("third-file", "same.txt", "", "", 1) + `]}`,
		},
	}
}

func TestSharePointMultipleRoots(t *testing.T) {
	for _, tc := range []struct {
		name      string
		roots     []string
		filters   map[string]any
		wantPaths map[string]string
		listings  int64
	}{
		{"three independent", []string{"a-child", "second", "third"}, nil,
			map[string]string{"child-file": "same.txt", "second-file": "same.txt", "third-file": "same.txt"}, 3},
		{"nested and repeated", []string{"a-child", "third", "z-parent", "second", "z-parent"}, nil,
			map[string]string{"child-file": "Middle/Child/same.txt", "parent-file": "same.txt", "second-file": "same.txt", "third-file": "same.txt"}, 5},
		{"reversed roots", []string{"z-parent", "second", "third", "a-child"}, nil,
			map[string]string{"child-file": "Middle/Child/same.txt", "parent-file": "same.txt", "second-file": "same.txt", "third-file": "same.txt"}, 5},
		{"name exclusion wins", []string{"a-child", "z-parent", "second", "third"}, map[string]any{"excluded_folder_names": []string{"Child"}},
			map[string]string{"parent-file": "same.txt", "second-file": "same.txt", "third-file": "same.txt"}, 4},
		{"path exclusion wins", []string{"a-child", "z-parent", "second", "third"}, map[string]any{"excluded_folder_paths": []string{"Middle/Child"}},
			map[string]string{"parent-file": "same.txt", "second-file": "same.txt", "third-file": "same.txt"}, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixtures := multiRootFixtures()
			server := newTestSharePointServer(t, fixtures)
			config := map[string]any{"drive_id": "drive", "root_folder_ids": tc.roots, "batch_size": 1}
			for key, value := range tc.filters {
				config[key] = value
			}
			c := mustSharePointConnector(t, server.URL, config)
			session, err := c.OpenSync(t.Context(), SyncRequest{})
			if err != nil {
				t.Fatal(err)
			}
			docs := drainSharePointSync(t, session)
			if len(docs) != len(tc.wantPaths) {
				t.Fatalf("duplicate or missing documents: %+v", docs)
			}
			for _, doc := range docs {
				id := doc.Metadata["drive_item_id"].(string)
				path, ok := tc.wantPaths[id]
				if !ok || doc.SourceID != "drive:"+id || doc.Metadata["path"] != path {
					t.Fatalf("unexpected document: %+v", doc)
				}
				delete(tc.wantPaths, id)
			}
			if fixtures.childrenCalls.Load() != tc.listings || fixtures.contentCalls.Load() != 0 {
				t.Fatalf("unexpected listing/download counts: %d / %d", fixtures.childrenCalls.Load(), fixtures.contentCalls.Load())
			}
		})
	}
}

func TestSharePointSelectedRootExclusions(t *testing.T) {
	for _, tc := range []struct {
		name     string
		roots    []string
		filters  map[string]any
		wantIDs  []string
		listings int64
	}{
		{"excluded root", []string{"z-parent"}, map[string]any{"excluded_folder_names": []string{"Archive"}}, nil, 0},
		{"other root remains", []string{"z-parent", "second"}, map[string]any{"excluded_folder_names": []string{"Archive"}}, []string{"drive:second-file"}, 1},
		{"nested root cannot bypass", []string{"a-child", "z-parent"}, map[string]any{"excluded_folder_names": []string{"Archive"}}, nil, 0},
		{"name is case sensitive", []string{"z-parent"}, map[string]any{"excluded_folder_names": []string{"archive"}}, []string{"drive:child-file", "drive:parent-file"}, 3},
		{"paths are relative to root", []string{"z-parent"}, map[string]any{"excluded_folder_paths": []string{"Archive"}}, []string{"drive:child-file", "drive:parent-file"}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixtures := multiRootFixtures()
			fixtures.itemsByKey["drive:z-parent"] = folderWithParent("z-parent", "Archive", "top")
			server := newTestSharePointServer(t, fixtures)
			config := map[string]any{"drive_id": "drive", "root_folder_ids": tc.roots}
			for key, value := range tc.filters {
				config[key] = value
			}
			c := mustSharePointConnector(t, server.URL, config)
			session, err := c.OpenSync(t.Context(), SyncRequest{})
			if err != nil {
				t.Fatal(err)
			}
			var ids []string
			for _, doc := range drainSharePointSync(t, session) {
				ids = append(ids, doc.SourceID)
			}
			sort.Strings(ids)
			if !reflect.DeepEqual(ids, tc.wantIDs) {
				t.Fatalf("documents = %v, want %v", ids, tc.wantIDs)
			}
			if fixtures.childrenCalls.Load() != tc.listings || fixtures.contentCalls.Load() != 0 {
				t.Fatalf("unexpected listing/download counts: %d / %d", fixtures.childrenCalls.Load(), fixtures.contentCalls.Load())
			}
		})
	}
}

func TestSharePointRootListConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name    string
		config  map[string]any
		want    []string
		invalid bool
	}{
		{"saved single", map[string]any{"root_folder_id": " one "}, []string{"one"}, false},
		{"array", map[string]any{"root_folder_ids": []any{"two", " one ", "two"}}, []string{"one", "two"}, false},
		{"clear overrides single", map[string]any{"root_folder_ids": []string{}, "root_folder_id": "one"}, []string{}, false},
		{"string", map[string]any{"root_folder_ids": "one;two"}, nil, true},
		{"null", map[string]any{"root_folder_ids": nil}, nil, true},
		{"comma", map[string]any{"root_folder_ids": []string{"one,two"}}, nil, true},
		{"empty entry", map[string]any{"root_folder_ids": []string{" "}}, nil, true},
		{"wrong item", map[string]any{"root_folder_ids": []any{12}}, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ids, err := sharePointRootIDs(tc.config)
			if (err != nil) != tc.invalid {
				t.Fatalf("error = %v", err)
			}
			if !tc.invalid && !reflect.DeepEqual(ids, tc.want) {
				t.Fatalf("roots = %v, want %v", ids, tc.want)
			}
		})
	}
	if _, err := NewSharePointConnector(map[string]any{"root_folder_ids": []string{"one", "two"}}); err == nil {
		t.Fatal("accepted roots without a library")
	}
}

func TestSharePointMultipleRootsFailClosed(t *testing.T) {
	for _, kind := range []string{"missing root", "file root", "inaccessible ancestor", "cycle"} {
		t.Run(kind, func(t *testing.T) {
			fixtures := multiRootFixtures()
			switch kind {
			case "missing root":
				delete(fixtures.itemsByKey, "drive:third")
			case "file root":
				fixtures.itemsByKey["drive:third"] = sharePointFileJSON("third", "file.txt", "", "", 1)
			case "inaccessible ancestor":
				delete(fixtures.itemsByKey, "drive:top")
			case "cycle":
				fixtures.itemsByKey["drive:top"] = folderWithParent("top", "", "third")
			}
			server := newTestSharePointServer(t, fixtures)
			c := mustSharePointConnector(t, server.URL, map[string]any{"drive_id": "drive", "root_folder_ids": []string{"second", "third"}})
			session, err := c.OpenSync(t.Context(), SyncRequest{})
			if err == nil {
				_, err = session.NextBatch(t.Context())
			}
			if err == nil {
				t.Fatal("invalid scope accepted")
			}
			if fixtures.childrenCalls.Load() != 0 {
				t.Fatal("listed files after scope failure")
			}
		})
	}
}

func TestSharePointIndependentRootFiltersAndDuplicateItems(t *testing.T) {
	fixtures := multiRootFixtures()
	for _, root := range []string{"second", "third"} {
		fixtures.childrenByKey["drive:"+root] = `{"value":[` +
			sharePointFolderJSON(root+"-archive", "Archive") + `,` +
			sharePointFolderJSON(root+"-video", "Video") + `,` +
			sharePointFileJSON("same-item", "same.txt", "", "", 1) + `]}`
	}
	server := newTestSharePointServer(t, fixtures)
	c := mustSharePointConnector(t, server.URL, map[string]any{
		"drive_id": "drive", "root_folder_ids": []string{"second", "third"},
		"excluded_folder_paths": []string{"Archive", "Video"},
	})
	session, err := c.OpenSync(t.Context(), SyncRequest{})
	if err != nil {
		t.Fatal(err)
	}
	docs := drainSharePointSync(t, session)
	if len(docs) != 1 || docs[0].SourceID != "drive:same-item" {
		t.Fatalf("duplicate item: %+v", docs)
	}
	if fixtures.childrenCalls.Load() != 2 {
		t.Fatal("excluded children were listed")
	}
}
