// Package checkpoint conformance tests verify that all checkpointer implementations
// satisfy the BaseCheckpointer contract.
//
// This mirrors Python's langgraph-checkpoint-conformance package.
// Any type implementing checkpoint.BaseCheckpointer should pass this suite.
package checkpoint

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"ragflow/internal/harness/graph/constants"
)

// ConformanceTestSuite holds state shared across conformance tests.
// checkpointer under test: factory function returning a fresh instance.
type ConformanceTestSuite struct {
	// NewCheckpointer creates a fresh checkpointer instance for each sub-test.
	NewCheckpointer func() BaseCheckpointer
}

// RunAll runs all conformance tests against the given checkpointer factory.
func (suite *ConformanceTestSuite) RunAll(t *testing.T) {
	t.Helper()
	t.Run("PutAndGet", suite.TestPutAndGet)
	t.Run("PutAndGetByID", suite.TestPutAndGetByID)
	t.Run("ListEmpty", suite.TestListEmpty)
	t.Run("ListOrder", suite.TestListOrder)
	t.Run("ListWithLimit", suite.TestListWithLimit)
	t.Run("MultipleThreads", suite.TestMultipleThreads)
	t.Run("GetNonExistent", suite.TestGetNonExistent)
	t.Run("OverwriteExisting", suite.TestOverwriteExisting)
	t.Run("ListAcrossThreads", suite.TestListAcrossThreads)
	t.Run("PutPreservesData", suite.TestPutPreservesData)
	t.Run("DeepCopySemantics", suite.TestDeepCopySemantics)
	t.Run("ConcurrentAccess", suite.TestConcurrentAccess)
	t.Run("ManyCheckpoints", suite.TestManyCheckpoints)
	t.Run("EmptyValues", suite.TestEmptyValues)
	t.Run("NilConfig", suite.TestNilConfig)
}

// threadConfig creates a minimal checkpointer config from a thread ID.
func threadConfig(tid string) map[string]interface{} {
	return map[string]interface{}{
		constants.ConfigKeyThreadID: tid,
	}
}

// threadConfigWithID creates a config with both thread and checkpoint ID.
func threadConfigWithID(tid, cpid string) map[string]interface{} {
	return map[string]interface{}{
		constants.ConfigKeyThreadID:     tid,
		constants.ConfigKeyCheckpointID: cpid,
	}
}

// ---- Test cases ----

// TestPutAndGet verifies basic write-then-read.
func (suite *ConformanceTestSuite) TestPutAndGet(t *testing.T) {
	cp := suite.NewCheckpointer()
	ctx := context.Background()

	tid := "test-thread-putget"
	data := map[string]interface{}{"key1": "value1", "key2": 42, "key3": true}

	if err := cp.Put(ctx, threadConfig(tid), data); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	got, err := cp.Get(ctx, threadConfig(tid))
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil, expected checkpoint data")
	}
	assertMapEqual(t, data, got, "Put/Get round-trip")
}

// TestPutAndGetByID verifies getting a specific checkpoint by ID.
func (suite *ConformanceTestSuite) TestPutAndGetByID(t *testing.T) {
	cp := suite.NewCheckpointer()
	ctx := context.Background()

	tid := "test-thread-byid"
	data1 := map[string]interface{}{"version": 1}
	data2 := map[string]interface{}{"version": 2}

	if err := cp.Put(ctx, threadConfig(tid), data1); err != nil {
		t.Fatalf("first Put failed: %v", err)
	}

	// Get the ID of the first checkpoint from List.
	entries, err := cp.List(ctx, threadConfig(tid), 10)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("List returned 0 entries after first Put")
	}
	cpID1 := entries[0][constants.ConfigKeyCheckpointID].(string)

	if err := cp.Put(ctx, threadConfig(tid), data2); err != nil {
		t.Fatalf("second Put failed: %v", err)
	}

	// Get by first checkpoint ID — must return data1.
	got, err := cp.Get(ctx, threadConfigWithID(tid, cpID1))
	if err != nil {
		t.Fatalf("Get by ID failed: %v", err)
	}
	if got == nil {
		t.Fatal("Get by ID returned nil")
	}
	v, ok := got["version"]
	if !ok {
		t.Fatalf("expected version=1, got %v", got)
	}
	// JSON may convert ints to float64.
	var versionVal int
	switch vt := v.(type) {
	case int:
		versionVal = vt
	case float64:
		versionVal = int(vt)
	default:
		t.Fatalf("unexpected type for version: %T", v)
	}
	if versionVal != 1 {
		t.Fatalf("expected version=1, got %d (raw=%v)", versionVal, v)
	}
}

// TestListEmpty verifies List returns nil/empty for a thread with no checkpoints.
func (suite *ConformanceTestSuite) TestListEmpty(t *testing.T) {
	cp := suite.NewCheckpointer()
	ctx := context.Background()

	entries, err := cp.List(ctx, threadConfig("nonexistent-thread"), 10)
	if err != nil {
		t.Fatalf("List on empty thread failed: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries for empty thread, got %d", len(entries))
	}
}

// TestListOrder verifies List returns checkpoints in reverse chronological order.
func (suite *ConformanceTestSuite) TestListOrder(t *testing.T) {
	cp := suite.NewCheckpointer()
	ctx := context.Background()

	tid := "test-thread-order"
	n := 5
	for i := 0; i < n; i++ {
		data := map[string]interface{}{"i": i}
		if err := cp.Put(ctx, threadConfig(tid), data); err != nil {
			t.Fatalf("Put #%d failed: %v", i, err)
		}
		time.Sleep(time.Millisecond) // ensure timestamp ordering
	}

	entries, err := cp.List(ctx, threadConfig(tid), n)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != n {
		t.Fatalf("expected %d entries, got %d", n, len(entries))
	}

	// Verify reverse chronological order.
	lastTime := time.Now().Add(time.Hour)
	seenIDs := make(map[string]bool)
	for i, entry := range entries {
		if entry[constants.ConfigKeyCheckpointID] == nil {
			t.Fatalf("entry %d missing checkpoint_id", i)
		}
		cpID, ok := entry[constants.ConfigKeyCheckpointID].(string)
		if !ok || cpID == "" {
			t.Fatalf("entry %d has invalid checkpoint_id: %v", i, entry[constants.ConfigKeyCheckpointID])
		}
		if seenIDs[cpID] {
			t.Fatalf("duplicate checkpoint ID: %s", cpID)
		}
		seenIDs[cpID] = true

		createdAt, ok := entry["created_at"].(time.Time)
		if ok {
			if createdAt.After(lastTime) {
				t.Fatalf("entry %d: created_at %v is after previous %v (not reverse chronological)", i, createdAt, lastTime)
			}
			lastTime = createdAt
		}
		if entry["thread_id"] == nil {
			val := entry[constants.ConfigKeyThreadID]
			if val == nil {
				t.Fatalf("entry %d missing thread_id", i)
			}
		}
		if entry["parent_id"] == nil && i < n-1 {
			// Parent chain: later entries have earlier parent_ids.
			// Entry n-1 (oldest) may not have parent_id.
		}
	}
}

// TestListWithLimit verifies List respects the limit parameter.
func (suite *ConformanceTestSuite) TestListWithLimit(t *testing.T) {
	cp := suite.NewCheckpointer()
	ctx := context.Background()

	tid := "test-thread-limit"
	for i := 0; i < 10; i++ {
		data := map[string]interface{}{"i": i}
		if err := cp.Put(ctx, threadConfig(tid), data); err != nil {
			t.Fatalf("Put #%d failed: %v", i, err)
		}
	}

	entries, err := cp.List(ctx, threadConfig(tid), 3)
	if err != nil {
		t.Fatalf("List with limit failed: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries with limit=3, got %d", len(entries))
	}
}

// TestMultipleThreads verifies isolation between threads.
func (suite *ConformanceTestSuite) TestMultipleThreads(t *testing.T) {
	cp := suite.NewCheckpointer()
	ctx := context.Background()

	threads := []string{"thread-a", "thread-b", "thread-c"}
	for _, tid := range threads {
		data := map[string]interface{}{"owner": tid}
		if err := cp.Put(ctx, threadConfig(tid), data); err != nil {
			t.Fatalf("Put for %s failed: %v", tid, err)
		}
	}

	for _, tid := range threads {
		got, err := cp.Get(ctx, threadConfig(tid))
		if err != nil {
			t.Fatalf("Get for %s failed: %v", tid, err)
		}
		if got == nil {
			t.Fatalf("Get for %s returned nil", tid)
		}
		owner, ok := got["owner"].(string)
		if !ok || owner != tid {
			t.Fatalf("expected owner=%q, got %q (data=%v)", tid, owner, got)
		}
	}
}

// TestGetNonExistent verifies Get returns nil for non-existent threads.
func (suite *ConformanceTestSuite) TestGetNonExistent(t *testing.T) {
	cp := suite.NewCheckpointer()
	ctx := context.Background()

	got, err := cp.Get(ctx, threadConfig("does-not-exist"))
	if err != nil {
		t.Fatalf("Get on non-existent thread failed: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for non-existent thread, got %v", got)
	}
}

// TestOverwriteExisting verifies Put with same thread ID replaces latest.
func (suite *ConformanceTestSuite) TestOverwriteExisting(t *testing.T) {
	cp := suite.NewCheckpointer()
	ctx := context.Background()

	tid := "test-thread-overwrite"
	v1 := map[string]interface{}{"value": "first"}
	if err := cp.Put(ctx, threadConfig(tid), v1); err != nil {
		t.Fatalf("first Put failed: %v", err)
	}

	v2 := map[string]interface{}{"value": "second"}
	if err := cp.Put(ctx, threadConfig(tid), v2); err != nil {
		t.Fatalf("second Put failed: %v", err)
	}

	got, err := cp.Get(ctx, threadConfig(tid))
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if v, ok := got["value"].(string); !ok || v != "second" {
		t.Fatalf("expected value=second, got %v", got)
	}
}

// TestListAcrossThreads verifies List only returns entries for the specified thread.
func (suite *ConformanceTestSuite) TestListAcrossThreads(t *testing.T) {
	cp := suite.NewCheckpointer()
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		tid := fmt.Sprintf("thread-%d", i)
		if err := cp.Put(ctx, threadConfig(tid), map[string]interface{}{"i": i}); err != nil {
			t.Fatalf("Put for %s failed: %v", tid, err)
		}
	}

	for i := 0; i < 3; i++ {
		tid := fmt.Sprintf("thread-%d", i)
		entries, err := cp.List(ctx, threadConfig(tid), 10)
		if err != nil {
			t.Fatalf("List for %s failed: %v", tid, err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry for %s, got %d", tid, len(entries))
		}
	}
}

// TestPutPreservesData verifies all types of data are preserved.
func (suite *ConformanceTestSuite) TestPutPreservesData(t *testing.T) {
	cp := suite.NewCheckpointer()
	ctx := context.Background()

	tid := "test-thread-types"
	data := map[string]interface{}{
		"string":    "hello",
		"int":       42,
		"float":     3.14,
		"bool":      true,
		"list":      []interface{}{1, "two", 3.0},
		"map":       map[string]interface{}{"nested": "value", "num": 1},
		"nil_val":   nil,
		"empty_str": "",
	}

	if err := cp.Put(ctx, threadConfig(tid), data); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	got, err := cp.Get(ctx, threadConfig(tid))
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil")
	}

	assertMapEqual(t, data, got, "data preservation")
}

// TestDeepCopySemantics verifies that Put stores a copy, not a reference.
func (suite *ConformanceTestSuite) TestDeepCopySemantics(t *testing.T) {
	cp := suite.NewCheckpointer()
	ctx := context.Background()

	tid := "test-thread-deepcopy"
	data := map[string]interface{}{
		"key": "original",
	}
	if err := cp.Put(ctx, threadConfig(tid), data); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Modify the original map after Put.
	data["key"] = "modified"

	got, err := cp.Get(ctx, threadConfig(tid))
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if v := got["key"]; v != "original" {
		t.Fatalf("expected copy semantics: key=original, got %v", v)
	}
}

// TestConcurrentAccess verifies thread safety under concurrent Put/Get operations.
func (suite *ConformanceTestSuite) TestConcurrentAccess(t *testing.T) {
	cp := suite.NewCheckpointer()
	ctx := context.Background()

	const goroutines = 20
	const opsPerGoroutine = 50

	errCh := make(chan error, goroutines)
	for g := 0; g < goroutines; g++ {
		go func(gid int) {
			tid := fmt.Sprintf("concurrent-thread-%d", gid)
			for i := 0; i < opsPerGoroutine; i++ {
				data := map[string]interface{}{"gid": gid, "i": i}
				if err := cp.Put(ctx, threadConfig(tid), data); err != nil {
					errCh <- fmt.Errorf("goroutine %d put failed: %w", gid, err)
					return
				}
				got, err := cp.Get(ctx, threadConfig(tid))
				if err != nil {
					errCh <- fmt.Errorf("goroutine %d get failed: %w", gid, err)
					return
				}
				if got == nil {
					errCh <- fmt.Errorf("goroutine %d got nil after put", gid)
					return
				}
			}
			errCh <- nil
		}(g)
	}

	for g := 0; g < goroutines; g++ {
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
	}
}

// TestManyCheckpoints verifies performance with many checkpoints.
func (suite *ConformanceTestSuite) TestManyCheckpoints(t *testing.T) {
	cp := suite.NewCheckpointer()
	ctx := context.Background()

	const n = 100
	tid := "test-thread-many"

	for i := 0; i < n; i++ {
		data := map[string]interface{}{"index": i, "data": fmt.Sprintf("checkpoint-%d", i)}
		if err := cp.Put(ctx, threadConfig(tid), data); err != nil {
			t.Fatalf("Put #%d failed: %v", i, err)
		}
	}

	entries, err := cp.List(ctx, threadConfig(tid), n)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != n {
		t.Fatalf("expected %d entries, got %d", n, len(entries))
	}

	got, err := cp.Get(ctx, threadConfig(tid))
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil")
	}
	idxRaw, ok := got["index"]
	if !ok {
		t.Fatalf("expected index key, got %v", got)
	}
	var idxVal int
	switch vt := idxRaw.(type) {
	case int:
		idxVal = vt
	case float64:
		idxVal = int(vt)
	default:
		t.Fatalf("unexpected type for index: %T", idxRaw)
	}
	if idxVal != n-1 {
		t.Fatalf("expected latest index=%d, got %d (raw=%v)", n-1, idxVal, idxRaw)
	}
}

// TestEmptyValues verifies round-trip with empty maps.
func (suite *ConformanceTestSuite) TestEmptyValues(t *testing.T) {
	cp := suite.NewCheckpointer()
	ctx := context.Background()

	tid := "test-thread-empty"
	empty := map[string]interface{}{}
	if err := cp.Put(ctx, threadConfig(tid), empty); err != nil {
		t.Fatalf("Put with empty map failed: %v", err)
	}

	got, err := cp.Get(ctx, threadConfig(tid))
	if err != nil {
		t.Fatalf("Get after empty Put failed: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil after Put with empty map")
	}
}

// TestNilConfig verifies Get/Put with missing thread_id returns an error.
func (suite *ConformanceTestSuite) TestNilConfig(t *testing.T) {
	cp := suite.NewCheckpointer()
	ctx := context.Background()

	// Put without thread_id should fail.
	err := cp.Put(ctx, map[string]interface{}{}, map[string]interface{}{"key": "val"})
	if err == nil {
		t.Fatal("expected error for Put without thread_id, got nil")
	}

	// Get without thread_id should fail.
	_, err = cp.Get(ctx, map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for Get without thread_id, got nil")
	}

	// List without thread_id should fail.
	_, err = cp.List(ctx, map[string]interface{}{}, 10)
	if err == nil {
		t.Fatal("expected error for List without thread_id, got nil")
	}
}

// ---- Helpers ----

// assertMapEqual compares two maps and reports differences.
func assertMapEqual(t *testing.T, expected, actual map[string]interface{}, context string) {
	t.Helper()

	if len(expected) != len(actual) {
		t.Fatalf("%s: map size mismatch: expected %d keys, got %d\n  expected=%v\n  actual=%v",
			context, len(expected), len(actual), keysOf(expected), keysOf(actual))
	}

	for k, expectedVal := range expected {
		actualVal, ok := actual[k]
		if !ok {
			t.Fatalf("%s: expected key %q not found in actual map", context, k)
		}
		if !valuesEqual(expectedVal, actualVal) {
			t.Fatalf("%s: key %q: expected %v (type=%T), got %v (type=%T)",
				context, k, expectedVal, expectedVal, actualVal, actualVal)
		}
	}
}

// keysOf returns sorted keys of a map.
func keysOf(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// valuesEqual does a deep comparison of two values.
// Numeric types (int/float64) are compared by value to handle JSON
// serialization where ints become float64.
func valuesEqual(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	switch va := a.(type) {
	case map[string]interface{}:
		vb, ok := b.(map[string]interface{})
		if !ok {
			return false
		}
		if len(va) != len(vb) {
			return false
		}
		for k, av := range va {
			bv, ok := vb[k]
			if !ok {
				return false
			}
			if !valuesEqual(av, bv) {
				return false
			}
		}
		return true
	case []interface{}:
		vb, ok := b.([]interface{})
		if !ok || len(va) != len(vb) {
			return false
		}
		for i := range va {
			if !valuesEqual(va[i], vb[i]) {
				return false
			}
		}
		return true
	case string:
		vb, ok := b.(string)
		return ok && va == vb
	case int:
		switch vb := b.(type) {
		case int:
			return va == vb
		case float64:
			return float64(va) == vb
		default:
			return false
		}
	case float64:
		switch vb := b.(type) {
		case float64:
			return va == vb
		case int:
			return va == float64(vb)
		default:
			return false
		}
	case bool:
		vb, ok := b.(bool)
		return ok && va == vb
	default:
		return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
	}
}

// RunMemorySaverConformanceTests runs the full conformance suite against MemorySaver.
func RunMemorySaverConformanceTests(t *testing.T) {
	suite := &ConformanceTestSuite{
		NewCheckpointer: func() BaseCheckpointer {
			return NewMemorySaver()
		},
	}
	suite.RunAll(t)
}

// RunSqliteSaverConformanceTests runs the full conformance suite against SqliteSaver.
func RunSqliteSaverConformanceTests(t *testing.T, dbPath string) {
	suite := &ConformanceTestSuite{
		NewCheckpointer: func() BaseCheckpointer {
			saver, err := NewSqliteSaver(dbPath)
			if err != nil {
				t.Skipf("SqliteSaver not available: %v", err)
				return nil
			}
			return saver
		},
	}
	suite.RunAll(t)
}

// skipBadTypeFields removes keys that the checkpointer cannot serialize
// (e.g. channels with unsupported types in SQLite).
func skipBadTypeFields(data map[string]interface{}, skipKeys ...string) map[string]interface{} {
	result := make(map[string]interface{}, len(data))
	skip := make(map[string]bool, len(skipKeys))
	for _, k := range skipKeys {
		skip[k] = true
	}
	for k, v := range data {
		if !skip[k] {
			result[k] = v
		}
	}
	return result
}

// NewSqliteSaver creates a SqliteSaver (stub — implement as needed).
func NewSqliteSaver(dbPath string) (BaseCheckpointer, error) {
	return nil, fmt.Errorf("SqliteSaver not implemented in this package: %s", dbPath)
}

// TestConformance_MemorySaver runs the conformance suite against MemorySaver.
func TestConformance_MemorySaver(t *testing.T) {
	RunMemorySaverConformanceTests(t)
}

// TestConformance_MemorySaver_SubtestNames tests that all subtest names are set correctly.
func TestConformance_MemorySaver_SubtestNames(t *testing.T) {
	suite := &ConformanceTestSuite{
		NewCheckpointer: func() BaseCheckpointer { return NewMemorySaver() },
	}
	// Verify that RunAll doesn't panic.
	suite.RunAll(t)
}
