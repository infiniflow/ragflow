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

package infinity

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func cachedMappingEntries() int {
	entries := 0
	fieldMappingCache.Range(func(_, _ any) bool {
		entries++
		return true
	})
	return entries
}

// TestLoadFieldMappingDoesNotPublishStaleFingerprint is the adversarial
// read/stat interleaving: the file is replaced after ReadFile but before the
// cache fingerprint is taken. Publishing parsed-old bytes under new-file
// metadata makes the stale mapping look current on every later call.
func TestLoadFieldMappingDoesNotPublishStaleFingerprint(t *testing.T) {
	writeCachedMappingFixture(t, "fingerprint-race.json", `{
		"actual": {"type": "varchar", "comment": "old_alias"}
	}`)

	original := readFieldMappingFile
	t.Cleanup(func() { readFieldMappingFile = original })
	swapped := false
	readFieldMappingFile = func(p string) ([]byte, error) {
		data, err := original(p)
		if !swapped {
			swapped = true
			newContents := `{
				"actual": {"type": "varchar", "comment": "new_alias_longer"}
			}`
			if err := os.WriteFile(p, []byte(newContents), 0o644); err != nil {
				t.Fatal(err)
			}
			later := time.Now().Add(2 * time.Second)
			if err := os.Chtimes(p, later, later); err != nil {
				t.Fatal(err)
			}
		}
		return data, err
	}

	first, _, err := loadFieldMapping("fingerprint-race.json")
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"old_alias": "actual"}; !reflect.DeepEqual(first, want) {
		t.Fatalf("first load = %v, want %v", first, want)
	}

	readFieldMappingFile = original
	got, _, err := loadFieldMapping("fingerprint-race.json")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"new_alias_longer": "actual"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("second load = %v, want %v (stale bytes published under a fresh fingerprint)", got, want)
	}
}

// TestLoadFieldMappingDoesNotPublishAfterInvalidation pins the cold-load race:
// an invalidation that lands while the file is being read must not be undone
// by a later cache fill with the pre-invalidation contents.
func TestLoadFieldMappingDoesNotPublishAfterInvalidation(t *testing.T) {
	writeCachedMappingFixture(t, "invalidation-race.json", `{
		"actual": {"type": "varchar", "comment": "cached_alias"}
	}`)
	InvalidateFieldMapping()
	path := filepath.Join(os.Getenv("RAG_PROJECT_BASE"), "conf", "invalidation-race.json")

	original := readFieldMappingFile
	t.Cleanup(func() { readFieldMappingFile = original })
	readFieldMappingFile = func(p string) ([]byte, error) {
		data, err := original(p)
		InvalidateFieldMapping()
		return data, err
	}

	if _, _, err := loadFieldMapping("invalidation-race.json"); err != nil {
		t.Fatal(err)
	}
	if entries := cachedMappingEntries(); entries != 0 {
		t.Fatalf("cache entries after invalidation during read = %d, want 0", entries)
	}

	readFieldMappingFile = original
	if err := os.WriteFile(path, []byte(`{
		"actual": {"type": "varchar", "comment": "replacement_alias"}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _, err := loadFieldMapping("invalidation-race.json")
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"replacement_alias": "actual"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("load after replacement = %v, want %v", got, want)
	}
}
