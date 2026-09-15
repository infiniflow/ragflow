package nats

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// setupNATSEngine is a thin alias for the white-box helper defined in
// syncer_test.go (it boots an embedded NATS server with JetStream and returns an
// initialized NatsEngine). We cannot use testutil.SetupNatsEngine here because
// that package imports this one, which would create an import cycle.
func setupNATSEngine(t *testing.T) *NatsEngine {
	t.Helper()
	return setupSyncerNATSEngine(t)
}

// newTestKVStore boots an embedded engine and creates a uniquely-named bucket
// for the calling test.
func newTestKVStore(t *testing.T, cfg BucketConfig) (*NatsKVStore, *NatsEngine) {
	t.Helper()
	engine := setupNATSEngine(t)
	if cfg.Name == "" {
		cfg.Name = "kvstore_test_" + sanitizeTestName(t.Name())
	}
	store, err := engine.EnsureKVBucket(t.Context(), cfg)
	if err != nil {
		t.Fatalf("EnsureKVBucket: %v", err)
	}
	return store, engine
}

func TestNatsKVStorePutGetHappyPath(t *testing.T) {
	store, _ := newTestKVStore(t, BucketConfig{})

	const key = "ocr.page1.hash-a1b2c3"
	want := []byte("ocr-result-bytes")

	if err := store.Put(t.Context(), key, want); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, found, err := store.Get(t.Context(), key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatal("Get returned found=false for an existing key")
	}
	if string(got) != string(want) {
		t.Fatalf("Get value = %q, want %q", got, want)
	}
}

func TestNatsKVStoreGetMissingKey(t *testing.T) {
	store, _ := newTestKVStore(t, BucketConfig{})

	got, found, err := store.Get(t.Context(), "never.written.key")
	if err != nil {
		t.Fatalf("Get on missing key returned error: %v", err)
	}
	if found {
		t.Fatal("Get on missing key returned found=true")
	}
	if got != nil {
		t.Fatalf("Get on missing key returned non-nil value: %v", got)
	}
}

func TestNatsKVStoreDeleteMissingKeyIsIdempotent(t *testing.T) {
	store, _ := newTestKVStore(t, BucketConfig{})

	if err := store.Delete(t.Context(), "never.written.key"); err != nil {
		t.Fatalf("Delete on missing key returned error (should be nil): %v", err)
	}
	// Deleting the same missing key a second time must also be nil.
	if err := store.Delete(t.Context(), "never.written.key"); err != nil {
		t.Fatalf("second Delete on missing key returned error: %v", err)
	}
}

func TestNatsKVStoreDeleteExistingKey(t *testing.T) {
	store, _ := newTestKVStore(t, BucketConfig{})
	const key = "emb.model-x.4096.text-hash"

	if err := store.Put(t.Context(), key, []byte("v1")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := store.Delete(t.Context(), key); err != nil {
		t.Fatalf("Delete existing key: %v", err)
	}
	_, found, err := store.Get(t.Context(), key)
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if found {
		t.Fatal("key still found after Delete")
	}
}

func TestNatsKVStoreKeysListsWrittenKeys(t *testing.T) {
	store, _ := newTestKVStore(t, BucketConfig{})

	want := []string{"a.1", "b.2", "c.3"}
	for _, k := range want {
		if err := store.Put(t.Context(), k, []byte("x")); err != nil {
			t.Fatalf("Put %q: %v", k, err)
		}
	}

	got, err := store.Keys(t.Context())
	if err != nil {
		t.Fatalf("Keys: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("Keys returned %d keys, want %d (%v)", len(got), len(want), got)
	}
	slices.Sort(got)
	slices.Sort(want)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Keys mismatch: got %v want %v", got, want)
		}
	}
}

func TestNatsKVStoreKeysEmptyBucket(t *testing.T) {
	store, _ := newTestKVStore(t, BucketConfig{})

	got, err := store.Keys(t.Context())
	if err != nil {
		t.Fatalf("Keys on empty bucket: %v", err)
	}
	if got == nil {
		t.Fatal("Keys on empty bucket returned nil slice; want non-nil empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("Keys on empty bucket returned %v, want empty", got)
	}
}

func TestNatsKVStoreOverwriteKeepsLatest(t *testing.T) {
	store, _ := newTestKVStore(t, BucketConfig{History: 1})
	const key = "checkpoint.user-1"

	if err := store.Put(t.Context(), key, []byte("v1")); err != nil {
		t.Fatalf("Put v1: %v", err)
	}
	if err := store.Put(t.Context(), key, []byte("v2")); err != nil {
		t.Fatalf("Put v2: %v", err)
	}

	got, found, err := store.Get(t.Context(), key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatal("overwritten key not found")
	}
	if string(got) != "v2" {
		t.Fatalf("Get = %q, want latest %q", got, "v2")
	}

	// With History=1 the bucket keeps exactly one revision, so Keys must show
	// the key exactly once, not once per revision.
	keys, err := store.Keys(t.Context())
	if err != nil {
		t.Fatalf("Keys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("Keys after overwrite = %v, want exactly 1 key", keys)
	}
}

func TestNatsKVStoreEmptyValue(t *testing.T) {
	store, _ := newTestKVStore(t, BucketConfig{})
	const key = "ocr.empty"

	if err := store.Put(t.Context(), key, []byte{}); err != nil {
		t.Fatalf("Put empty value: %v", err)
	}
	got, found, err := store.Get(t.Context(), key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatal("key with empty value not found")
	}
	if len(got) != 0 {
		t.Fatalf("Get empty value = %v (len %d), want len 0", got, len(got))
	}
}

func TestNatsKVStoreSpecialKeyNames(t *testing.T) {
	store, _ := newTestKVStore(t, BucketConfig{})

	keys := []string{
		"ocr.parser-config.image-hash",    // dots and dashes
		"emb_model-x_dim-4096_text-hash",  // underscores
		"12345",                           // purely numeric
		"aVeryLongKey" + repeat("x", 200), // 200+ char key
	}
	for _, k := range keys {
		if err := store.Put(t.Context(), k, []byte("v")); err != nil {
			t.Fatalf("Put %q: %v", k, err)
		}
	}

	got, err := store.Keys(t.Context())
	if err != nil {
		t.Fatalf("Keys: %v", err)
	}
	if len(got) != len(keys) {
		t.Fatalf("Keys = %v, want %d keys", got, len(keys))
	}
}

func TestNatsKVStoreDeleteRemovesFromKeys(t *testing.T) {
	store, _ := newTestKVStore(t, BucketConfig{})
	const keep = "keep.me"
	const drop = "drop.me"

	if err := store.Put(t.Context(), keep, []byte("k")); err != nil {
		t.Fatalf("Put keep: %v", err)
	}
	if err := store.Put(t.Context(), drop, []byte("d")); err != nil {
		t.Fatalf("Put drop: %v", err)
	}
	if err := store.Delete(t.Context(), drop); err != nil {
		t.Fatalf("Delete drop: %v", err)
	}

	got, err := store.Keys(t.Context())
	if err != nil {
		t.Fatalf("Keys: %v", err)
	}
	if slices.Contains(got, drop) {
		t.Fatalf("deleted key %q still present in Keys: %v", drop, got)
	}
	if !slices.Contains(got, keep) {
		t.Fatalf("kept key %q missing from Keys: %v", keep, got)
	}
}

func TestNatsKVStoreDeleteInvalidKeyReturnsError(t *testing.T) {
	store, _ := newTestKVStore(t, BucketConfig{})

	// A key that is not a valid NATS subject (contains a space) must surface a
	// concrete error from the underlying store, NOT be silently treated as a
	// successful delete of a missing key.
	err := store.Delete(t.Context(), "bad key with space")
	if err == nil {
		t.Fatal("Delete with invalid key returned nil error; want error")
	}
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("Delete with invalid key surfaced as ErrKeyNotFound: %v", err)
	}
}

func TestNatsKVStoreContextCancel(t *testing.T) {
	store, _ := newTestKVStore(t, BucketConfig{})
	const key = "ctx.key"

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // already canceled

	// A canceled context must surface as an error, NOT as a silent not-found.
	if _, _, err := store.Get(ctx, key); err == nil {
		t.Fatal("Get with canceled ctx returned nil error; want context error")
	}
	if err := store.Put(ctx, key, []byte("v")); err == nil {
		t.Fatal("Put with canceled ctx returned nil error; want context error")
	}
	if err := store.Delete(ctx, key); err == nil {
		t.Fatal("Delete with canceled ctx returned nil error; want context error")
	}
	if _, err := store.Keys(ctx); err == nil {
		t.Fatal("Keys with canceled ctx returned nil error; want context error")
	}
}

func TestNatsKVStoreNewIsIdempotent(t *testing.T) {
	engine := setupNATSEngine(t)
	cfg := BucketConfig{Name: "idempotent_bucket_" + sanitizeTestName(t.Name())}

	s1, err := engine.EnsureKVBucket(t.Context(), cfg)
	if err != nil {
		t.Fatalf("first EnsureKVBucket: %v", err)
	}
	s2, err := engine.EnsureKVBucket(t.Context(), cfg)
	if err != nil {
		t.Fatalf("second EnsureKVBucket (idempotent): %v", err)
	}

	// Both handles point at the same bucket; writing via s2 is visible via s1.
	if err := s2.Put(t.Context(), "k", []byte("v")); err != nil {
		t.Fatalf("Put via s2: %v", err)
	}
	if _, found, err := s1.Get(t.Context(), "k"); err != nil || !found {
		t.Fatalf("Get via s1: found=%v err=%v", found, err)
	}
}

func TestNatsKVStoreDefaultsApplied(t *testing.T) {
	store, _ := newTestKVStore(t, BucketConfig{Name: "defaults_bucket_" + sanitizeTestName(t.Name())})

	status, err := store.kv.Status(t.Context())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if got := status.History(); got != 1 {
		t.Fatalf("default History = %d, want 1", got)
	}
	if got := int(status.Config().MaxValueSize); got != 1024*1024 {
		t.Fatalf("default MaxValueSize = %d, want %d", got, 1024*1024)
	}
	if got := status.Config().Storage; got != jetstream.FileStorage {
		t.Fatalf("default Storage = %v, want FileStorage", got)
	}
}

func TestNatsKVStoreConfigApplied(t *testing.T) {
	cfg := BucketConfig{
		Name:         "config_bucket_" + sanitizeTestName(t.Name()),
		Description:  "unit-test bucket",
		History:      1,
		TTL:          7 * 24 * time.Hour,
		MaxValueSize: 8 * 1024 * 1024,
		Storage:      jetstream.FileStorage,
		Replicas:     1,
		Compression:  true,
	}
	store, _ := newTestKVStore(t, cfg)

	status, err := store.kv.Status(t.Context())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if got := status.History(); got != 1 {
		t.Fatalf("History = %d, want 1", got)
	}
	if got := status.TTL(); got != cfg.TTL {
		t.Fatalf("TTL = %v, want %v", got, cfg.TTL)
	}
	if got := int(status.Config().MaxValueSize); got != cfg.MaxValueSize {
		t.Fatalf("MaxValueSize = %d, want %d", got, cfg.MaxValueSize)
	}
	if got := status.Config().Storage; got != jetstream.FileStorage {
		t.Fatalf("Storage = %v, want FileStorage", got)
	}
	if got := status.Config().Replicas; got != 1 {
		t.Fatalf("Replicas = %d, want 1", got)
	}
	if !status.IsCompressed() {
		t.Fatal("Compression enabled but Status reports not compressed")
	}
	if got := status.Bucket(); got != cfg.Name {
		t.Fatalf("Bucket name = %q, want %q", got, cfg.Name)
	}
}

func TestNatsKVStoreMaxValueSizeEnforced(t *testing.T) {
	store, _ := newTestKVStore(t, BucketConfig{
		Name:         "oversize_bucket_" + sanitizeTestName(t.Name()),
		MaxValueSize: 10,
	})

	// Exactly at the limit is allowed.
	if err := store.Put(t.Context(), "atlimit", make([]byte, 10)); err != nil {
		t.Fatalf("Put at limit (10 bytes): %v", err)
	}
	// One byte over the limit must be rejected.
	err := store.Put(t.Context(), "over", make([]byte, 11))
	if err == nil {
		t.Fatal("Put over MaxValueSize returned nil error; want rejection")
	}
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("oversize Put surfaced as ErrKeyNotFound: %v", err)
	}
}

func TestNatsKVStoreTTLExpiry(t *testing.T) {
	store, _ := newTestKVStore(t, BucketConfig{
		Name: "ttl_bucket_" + sanitizeTestName(t.Name()),
		TTL:  2 * time.Second,
	})
	const key = "expiring.key"

	if err := store.Put(t.Context(), key, []byte("v")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		_, found, err := store.Get(t.Context(), key)
		if err != nil {
			t.Fatalf("Get during TTL poll: %v", err)
		}
		if !found {
			return // expired as expected
		}
		if time.Now().After(deadline) {
			t.Fatal("key not expired within 15s despite 2s TTL")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func TestNatsKVStoreEnsureKVBucketRequiresInit(t *testing.T) {
	// NewNatsEngine without Init leaves jetStream nil.
	engine := NewNatsEngine("127.0.0.1", 4222)
	if _, err := engine.EnsureKVBucket(t.Context(), BucketConfig{Name: "x"}); err == nil {
		t.Fatal("EnsureKVBucket on uninitialized engine returned nil error; want error")
	}
}

func TestNatsKVStoreNewRejectsEmptyName(t *testing.T) {
	// Empty bucket name must be rejected before any JetStream call. A nil js is
	// therefore safe to pass here.
	if _, err := NewNatsKVStore(t.Context(), nil, BucketConfig{Name: ""}); err == nil {
		t.Fatal("NewNatsKVStore with empty Name returned nil error; want error")
	}
}

func TestNatsKVStoreConcurrentPuts(t *testing.T) {
	store, _ := newTestKVStore(t, BucketConfig{})

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			key := "concurrent." + itoa(i)
			if err := store.Put(t.Context(), key, []byte(itoa(i))); err != nil {
				t.Errorf("Put %q: %v", key, err)
			}
		}(i)
	}
	wg.Wait()

	keys, err := store.Keys(t.Context())
	if err != nil {
		t.Fatalf("Keys: %v", err)
	}
	if len(keys) != n {
		t.Fatalf("after %d concurrent puts, Keys = %d keys: %v", n, len(keys), keys)
	}
}

// --- small local helpers (avoid pulling test-only deps) ---

func repeat(s string, n int) string {
	out := make([]byte, 0, n*len(s))
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

func sanitizeTestName(name string) string {
	out := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	if len(out) > 40 {
		out = out[:40]
	}
	return string(out)
}
