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
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

// writeCachedMappingFixture creates <base>/conf/<name> and points the project
// base at <base> so loadFieldMapping resolves to that exact path.
func writeCachedMappingFixture(t *testing.T, name, contents string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "conf"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "conf", name)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RAG_PROJECT_BASE", dir)
	return path
}

// TestLoadFieldMappingCachesAcrossCalls pins the hot-path fix: RunSQL calls
// loadFieldMapping on every invocation, so an unchanged file must be served
// from the cache rather than read and parsed again. Making the file unreadable
// after the first load turns any re-read into a hard failure.
func TestLoadFieldMappingCachesAcrossCalls(t *testing.T) {
	path := writeCachedMappingFixture(t, "cached-mapping.json", `{
		"actual": {"type": "varchar", "comment": "first_alias"}
	}`)

	gotFirst, _, err := loadFieldMapping("cached-mapping.json")
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	if want := map[string]string{"first_alias": "actual"}; !reflect.DeepEqual(gotFirst, want) {
		t.Fatalf("first load = %v, want %v", gotFirst, want)
	}

	// Stat still succeeds on a 000-mode file; ReadFile does not. A valid
	// cache hit therefore succeeds while any re-read fails.
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o644) })

	gotSecond, _, err := loadFieldMapping("cached-mapping.json")
	if err != nil {
		t.Fatalf("second load should be served from cache, got: %v", err)
	}
	if !reflect.DeepEqual(gotSecond, gotFirst) {
		t.Fatalf("second load = %v, want cached %v", gotSecond, gotFirst)
	}
}

// TestLoadFieldMappingReloadsOnFileChange pins the restored hot-reload
// semantics: the baseline re-read the file on every query, so replacing the
// mapping file at runtime used to take effect on the next call. A pure path
// cache silently froze the old mapping — this test fails on that regression.
func TestLoadFieldMappingReloadsOnFileChange(t *testing.T) {
	path := writeCachedMappingFixture(t, "hot-reload-mapping.json", `{
		"actual": {"type": "varchar", "comment": "before_alias"}
	}`)

	before, _, err := loadFieldMapping("hot-reload-mapping.json")
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	if want := map[string]string{"before_alias": "actual"}; !reflect.DeepEqual(before, want) {
		t.Fatalf("first load = %v, want %v", before, want)
	}

	// Change both content and size so the file fingerprint cannot match.
	if err := os.WriteFile(path, []byte(`{
		"actual": {"type": "varchar", "comment": "after_alias_with_a_longer_name"}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, _, err := loadFieldMapping("hot-reload-mapping.json")
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	want := map[string]string{"after_alias_with_a_longer_name": "actual"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("second load = %v, want %v (stale cache survived a file change)", got, want)
	}
}

// TestLoadFieldMappingReloadsOnSameSizeChange is the adversarial fingerprint
// case: content changed with identical length, so only the modification time
// can detect the replacement.
func TestLoadFieldMappingReloadsOnSameSizeChange(t *testing.T) {
	path := writeCachedMappingFixture(t, "same-size-mapping.json", `{
		"actual": {"type": "varchar", "comment": "aaaa_alias"}
	}`)

	first, _, err := loadFieldMapping("same-size-mapping.json")
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"aaaa_alias": "actual"}; !reflect.DeepEqual(first, want) {
		t.Fatalf("first load = %v, want %v", first, want)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Same byte length, different alias. Force a different mtime so a
	// fingerprint check (but not a size-only check) sees the change.
	newContents := `{
		"actual": {"type": "varchar", "comment": "bbbb_alias"}
	}`
	if len(newContents) != len(`{
		"actual": {"type": "varchar", "comment": "aaaa_alias"}
	}`) {
		t.Fatal("fixture lengths must match")
	}
	if err := os.WriteFile(path, []byte(newContents), 0o644); err != nil {
		t.Fatal(err)
	}
	later := info.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}

	got, _, err := loadFieldMapping("same-size-mapping.json")
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"bbbb_alias": "actual"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("same-size reload = %v, want %v", got, want)
	}
}

// TestLoadFieldMappingCacheIsolatedByFileName is the adversarial isolation
// case: two mapping names in the same project must not share a cache entry.
func TestLoadFieldMappingCacheIsolatedByFileName(t *testing.T) {
	writeCachedMappingFixture(t, "cache-one.json", `{
		"one": {"type": "varchar", "comment": "alias_one"}
	}`)
	// Same project dir, second fixture.
	dir := os.Getenv("RAG_PROJECT_BASE")
	if err := os.WriteFile(filepath.Join(dir, "conf", "cache-two.json"), []byte(`{
		"two": {"type": "varchar", "comment": "alias_two"}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	one, _, err := loadFieldMapping("cache-one.json")
	if err != nil {
		t.Fatal(err)
	}
	two, _, err := loadFieldMapping("cache-two.json")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(one, map[string]string{"alias_one": "one"}) {
		t.Fatalf("cache-one = %v, want alias_one→one", one)
	}
	if !reflect.DeepEqual(two, map[string]string{"alias_two": "two"}) {
		t.Fatalf("cache-two = %v, want alias_two→two", two)
	}
}

// TestLoadFieldMappingCacheConcurrent guards the sync.Map choice: concurrent
// RunSQL calls must all see the same mapping with no race and no torn parse.
func TestLoadFieldMappingCacheConcurrent(t *testing.T) {
	writeCachedMappingFixture(t, "cache-concurrent.json", `{
		"actual": {"type": "varchar", "comment": "alias_a, alias_b"}
	}`)
	want := map[string]string{"alias_a": "actual", "alias_b": "actual"}

	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, _, err := loadFieldMapping("cache-concurrent.json")
			if err != nil {
				errs <- err
				return
			}
			if !reflect.DeepEqual(got, want) {
				errs <- &mappingMismatchError{got: got, want: want}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// TestInvalidateFieldMappingReloads pins the escape hatch: after a mapping
// file is replaced at runtime, invalidation must make the next load observe
// the new contents.
func TestInvalidateFieldMappingReloads(t *testing.T) {
	writeCachedMappingFixture(t, "invalidate-mapping.json", `{
		"actual": {"type": "varchar", "comment": "before"}
	}`)
	before, _, err := loadFieldMapping("invalidate-mapping.json")
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(os.Getenv("RAG_PROJECT_BASE"), "conf", "invalidate-mapping.json")
	if err := os.WriteFile(path, []byte(`{
		"actual": {"type": "varchar", "comment": "after"}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	InvalidateFieldMapping()
	after, _, err := loadFieldMapping("invalidate-mapping.json")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, map[string]string{"before": "actual"}) {
		t.Fatalf("before = %v", before)
	}
	if !reflect.DeepEqual(after, map[string]string{"after": "actual"}) {
		t.Fatalf("after = %v, want the rewritten mapping", after)
	}
}

type mappingMismatchError struct{ got, want map[string]string }

func (e *mappingMismatchError) Error() string {
	return fmt.Sprintf("concurrent load returned %v, want %v", e.got, e.want)
}

// BenchmarkLoadFieldMappingCached guards the RunSQL hot path: the cached
// lookup must stay allocation-light compared with a fresh read/parse.
func BenchmarkLoadFieldMappingCached(b *testing.B) {
	dir := b.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "conf"), 0o755); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "conf", "bench-mapping.json"), []byte(`{
		"docnm": {"type": "varchar", "comment": "docnm_kwd, title_tks"},
		"content": {"type": "varchar", "comment": "content_ltks"}
	}`), 0o644); err != nil {
		b.Fatal(err)
	}
	old, had := os.LookupEnv("RAG_PROJECT_BASE")
	if err := os.Setenv("RAG_PROJECT_BASE", dir); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if had {
			os.Setenv("RAG_PROJECT_BASE", old)
		} else {
			os.Unsetenv("RAG_PROJECT_BASE")
		}
	})
	InvalidateFieldMapping()
	if _, _, err := loadFieldMapping("bench-mapping.json"); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for range b.N {
		if _, _, err := loadFieldMapping("bench-mapping.json"); err != nil {
			b.Fatal(err)
		}
	}
}
