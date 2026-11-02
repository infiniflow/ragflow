//go:build integration

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

package mediacache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/engine/kvrocks"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/globals"
	"ragflow/internal/parser/media"
	"ragflow/internal/server"
)

var (
	realKvrocksOnce sync.Once
	realKvrocksErr  error
)

// requireRealKvrocks connects the process-wide Kvrocks client to the service
// named by RAGFLOW_TEST_KVROCKS_ADDR (host:port). The password defaults to
// "infini_rag_flow" (the checked-in conf value) and can be overridden with
// REDIS_PASSWORD. Without an address the test is skipped instead of falling
// back to a fake.
func requireRealKvrocks(t *testing.T) *kvrocks.Client {
	t.Helper()
	addr := strings.TrimSpace(os.Getenv("RAGFLOW_TEST_KVROCKS_ADDR"))
	if addr == "" {
		t.Skip("RAGFLOW_TEST_KVROCKS_ADDR is not set; set it to host:port (for example localhost:6379) to run the real-Kvrocks media cache tests")
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil || host == "" {
		t.Fatalf("RAGFLOW_TEST_KVROCKS_ADDR = %q, want host:port", addr)
	}
	if _, err := strconv.Atoi(port); err != nil {
		t.Fatalf("RAGFLOW_TEST_KVROCKS_ADDR = %q has a non-numeric port", addr)
	}
	realKvrocksOnce.Do(func() {
		realKvrocksErr = initRealKvrocks(addr)
	})
	if realKvrocksErr != nil {
		t.Fatalf("initialize real Kvrocks at %s: %v", addr, realKvrocksErr)
	}
	if !kvrocks.IsEnabled() {
		t.Fatal("Kvrocks client is not initialized")
	}
	return kvrocks.Get()
}

func initRealKvrocks(addr string) error {
	password := os.Getenv("REDIS_PASSWORD")
	if password == "" {
		password = "infini_rag_flow"
	}
	cfg := map[string]any{
		"general": map[string]any{"cache_engine": "kvrocks"},
		"kvrocks": map[string]any{
			"host":     addr,
			"password": password,
			"db":       1,
		},
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal service config: %w", err)
	}
	dir, err := os.MkdirTemp("", "ragflow-mediacache-it-")
	if err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "service_conf.yaml")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write service config: %w", err)
	}
	if err := server.Init(path); err != nil {
		return fmt.Errorf("server.Init: %w", err)
	}
	if err := kvrocks.Init(context.Background()); err != nil {
		return fmt.Errorf("kvrocks.Init: %w", err)
	}
	return nil
}

// mediaTaskContext mirrors the ingestion run context: Store only records the
// media manifest when the context carries the run's task id.
func mediaTaskContext(t *testing.T, taskID string) context.Context {
	t.Helper()
	ctx := runtime.WithState(t.Context(), runtime.NewCanvasState("", ""))
	globals.SetTaskID(ctx, taskID)
	return ctx
}

// newMediaIntegrationScope returns a unique run scope and registers a
// best-effort PurgeTask cleanup so a failed assertion cannot leak real keys.
func newMediaIntegrationScope(t *testing.T, backend *kvrocks.Client, name string) (string, context.Context) {
	t.Helper()
	scope := fmt.Sprintf("it-mediacache-%d-%s", time.Now().UnixNano(), name)
	ctx := mediaTaskContext(t, scope)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := PurgeTask(cleanupCtx, backend, scope); err != nil {
			t.Errorf("PurgeTask(%s): %v", scope, err)
		}
	})
	return scope, ctx
}

func mediaDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func assertMediaKeys(t *testing.T, backend *kvrocks.Client, ctx context.Context, keys ...string) {
	t.Helper()
	for _, key := range keys {
		exists, err := backend.GetClient().Exists(ctx, key).Result()
		if err != nil {
			t.Fatalf("EXISTS %s: %v", key, err)
		}
		if exists != 0 {
			t.Fatalf("key %s still exists after purge", key)
		}
	}
}

func assertOneDayTTL(t *testing.T, backend *kvrocks.Client, ctx context.Context, keys ...string) {
	t.Helper()
	for _, key := range keys {
		ttl, err := backend.GetClient().TTL(ctx, key).Result()
		if err != nil {
			t.Fatalf("TTL %s: %v", key, err)
		}
		if ttl <= 23*time.Hour || ttl > TTL {
			t.Fatalf("%s TTL = %v, want the one-day media lifetime", key, ttl)
		}
	}
}

// TestStoreRealKvrocksLifecycle exercises the actual Lua script, key layout,
// TTL and manifest against a live Kvrocks server. Subtests are sequential
// because they mutate process-wide media env limits and share kvrocks.Init.
func TestStoreRealKvrocksLifecycle(t *testing.T) {
	backend := requireRealKvrocks(t)

	t.Run("PutGetRoundTrip", func(t *testing.T) {
		scope, ctx := newMediaIntegrationScope(t, backend, "roundtrip")
		store := New(backend, scope)
		data := []byte("real-kvrocks-media-payload")

		ref, err := store.Put(ctx, "html", "  IMAGE/PNG  ", data)
		if err != nil {
			t.Fatalf("Put: %v", err)
		}
		if ref.Format != "html" || ref.Kind != entity.MediaKindKV || ref.Scope != scope ||
			ref.SHA256 != mediaDigest(data) || ref.MIME != "image/png" || ref.Size != int64(len(data)) {
			t.Fatalf("ref = %+v, want format=html kind=kv scope=%s sha256=%s mime=image/png size=%d",
				ref, scope, mediaDigest(data), len(data))
		}

		for attempt := 1; attempt <= 2; attempt++ {
			got, err := store.Get(ctx, ref)
			if err != nil {
				t.Fatalf("Get #%d: %v", attempt, err)
			}
			if !bytes.Equal(got, data) {
				t.Fatalf("Get #%d = %q, want %q", attempt, got, data)
			}
		}

		mediaKey := store.key(ref.SHA256)
		raw, err := backend.GetClient().Get(ctx, mediaKey).Result()
		if err != nil {
			t.Fatalf("read raw media key %s: %v", mediaKey, err)
		}
		if raw != string(data) {
			t.Fatalf("raw media value = %q, want %q", raw, data)
		}

		manifest := manifestKey(scope)
		digestKey, stateKey := quotaKeys(scope)
		members, err := backend.GetClient().SMembers(ctx, manifest).Result()
		if err != nil {
			t.Fatalf("SMEMBERS %s: %v", manifest, err)
		}
		if len(members) != 1 || members[0] != mediaKey {
			t.Fatalf("manifest = %v, want [%s]", members, mediaKey)
		}
		member, err := backend.GetClient().SIsMember(ctx, digestKey, ref.SHA256).Result()
		if err != nil || !member {
			t.Fatalf("quota digest membership = %v, %v; want true", member, err)
		}
		images, err := backend.GetClient().SCard(ctx, digestKey).Result()
		if err != nil || images != 1 {
			t.Fatalf("quota image count = %d, %v; want 1", images, err)
		}
		used, err := backend.GetClient().HGet(ctx, stateKey, "bytes").Result()
		if err != nil || used != strconv.Itoa(len(data)) {
			t.Fatalf("quota bytes = %q, %v; want %d", used, err, len(data))
		}
		assertOneDayTTL(t, backend, ctx, mediaKey, manifest, digestKey, stateKey)
	})

	t.Run("DuplicatePutChargesOnce", func(t *testing.T) {
		scope, ctx := newMediaIntegrationScope(t, backend, "duplicate")
		store := New(backend, scope)
		data := []byte("duplicate-media-payload")

		ref, err := store.Put(ctx, "html", "image/png", data)
		if err != nil {
			t.Fatalf("first Put: %v", err)
		}
		duplicate, err := store.Put(ctx, "html", "image/png", data)
		if err != nil {
			t.Fatalf("duplicate Put: %v", err)
		}
		if duplicate != ref {
			t.Fatalf("duplicate ref = %+v, want the first ref %+v", duplicate, ref)
		}

		digestKey, stateKey := quotaKeys(scope)
		images, err := backend.GetClient().SCard(ctx, digestKey).Result()
		if err != nil || images != 1 {
			t.Fatalf("quota image count = %d, %v; want 1 after duplicate Put", images, err)
		}
		used, err := backend.GetClient().HGet(ctx, stateKey, "bytes").Result()
		if err != nil || used != strconv.Itoa(len(data)) {
			t.Fatalf("quota bytes = %q, %v; want %d after duplicate Put", used, err, len(data))
		}
		manifest, err := backend.GetClient().SCard(ctx, manifestKey(scope)).Result()
		if err != nil || manifest != 1 {
			t.Fatalf("manifest entries = %d, %v; want 1 after duplicate Put", manifest, err)
		}
		if got, err := store.Get(ctx, duplicate); err != nil || !bytes.Equal(got, data) {
			t.Fatalf("duplicate Get = %q, %v; want %q, nil", got, err, data)
		}
	})

	t.Run("QuotaExactBoundary", func(t *testing.T) {
		t.Run("bytes", func(t *testing.T) {
			t.Setenv(common.EnvMediaMaxBytesPerRun, "5")
			t.Setenv(common.EnvMediaMaxImagesPerRun, "10")
			scope, ctx := newMediaIntegrationScope(t, backend, "quota-bytes")
			store := New(backend, scope)

			if _, err := store.Put(ctx, "html", "image/png", []byte("abc")); err != nil {
				t.Fatalf("first image: %v", err)
			}
			if _, err := store.Put(ctx, "html", "image/png", []byte("de")); err != nil {
				t.Fatalf("exact-boundary image: %v", err)
			}
			if _, err := store.Put(ctx, "html", "image/png", []byte("f")); !errors.Is(err, media.ErrMediaLimitExceeded) {
				t.Fatalf("over-boundary error = %v, want ErrMediaLimitExceeded", err)
			}
			// Dedupe must stay admitted at the exact cap without charging again.
			if _, err := store.Put(ctx, "html", "image/png", []byte("abc")); err != nil {
				t.Fatalf("duplicate at exact byte cap: %v", err)
			}

			digestKey, stateKey := quotaKeys(scope)
			images, err := backend.GetClient().SCard(ctx, digestKey).Result()
			if err != nil || images != 2 {
				t.Fatalf("quota image count = %d, %v; want 2", images, err)
			}
			used, err := backend.GetClient().HGet(ctx, stateKey, "bytes").Result()
			if err != nil || used != "5" {
				t.Fatalf("quota bytes = %q, %v; want 5", used, err)
			}
			rejectedKey := store.key(mediaDigest([]byte("f")))
			if exists, err := backend.GetClient().Exists(ctx, rejectedKey).Result(); err != nil || exists != 0 {
				t.Fatalf("rejected image key exists=%d, err=%v; want 0", exists, err)
			}
			if members, err := backend.GetClient().SCard(ctx, manifestKey(scope)).Result(); err != nil || members != 2 {
				t.Fatalf("manifest entries = %d, %v; want 2", members, err)
			}
		})

		t.Run("images", func(t *testing.T) {
			t.Setenv(common.EnvMediaMaxImagesPerRun, "2")
			t.Setenv(common.EnvMediaMaxBytesPerRun, "100")
			scope, ctx := newMediaIntegrationScope(t, backend, "quota-images")
			store := New(backend, scope)

			for _, image := range []string{"a", "b"} {
				if _, err := store.Put(ctx, "html", "image/png", []byte(image)); err != nil {
					t.Fatalf("image %q: %v", image, err)
				}
			}
			if _, err := store.Put(ctx, "html", "image/png", []byte("c")); !errors.Is(err, media.ErrMediaLimitExceeded) {
				t.Fatalf("third image error = %v, want ErrMediaLimitExceeded", err)
			}

			digestKey, stateKey := quotaKeys(scope)
			images, err := backend.GetClient().SCard(ctx, digestKey).Result()
			if err != nil || images != 2 {
				t.Fatalf("quota image count = %d, %v; want 2", images, err)
			}
			used, err := backend.GetClient().HGet(ctx, stateKey, "bytes").Result()
			if err != nil || used != "2" {
				t.Fatalf("quota bytes = %q, %v; want 2", used, err)
			}
			rejectedKey := store.key(mediaDigest([]byte("c")))
			if exists, err := backend.GetClient().Exists(ctx, rejectedKey).Result(); err != nil || exists != 0 {
				t.Fatalf("rejected image key exists=%d, err=%v; want 0", exists, err)
			}
		})
	})

	t.Run("CorruptPayloadIsIntegrityError", func(t *testing.T) {
		scope, ctx := newMediaIntegrationScope(t, backend, "corrupt")
		store := New(backend, scope)
		data := []byte("integrity-media-payload")

		ref, err := store.Put(ctx, "html", "image/png", data)
		if err != nil {
			t.Fatalf("Put: %v", err)
		}
		if err := backend.GetClient().Set(ctx, store.key(ref.SHA256), "corrupted", TTL).Err(); err != nil {
			t.Fatalf("corrupt payload: %v", err)
		}

		_, err = store.Get(ctx, ref)
		if err == nil {
			t.Fatal("Get accepted a corrupt payload")
		}
		if errors.Is(err, media.ErrMediaNotFound) {
			t.Fatalf("integrity failure collapsed into a cache miss: %v", err)
		}
		if !strings.Contains(err.Error(), "digest mismatch") {
			t.Fatalf("Get corrupt error = %v, want a digest mismatch", err)
		}
	})

	t.Run("CrossScopeGetRejected", func(t *testing.T) {
		scopeA, ctxA := newMediaIntegrationScope(t, backend, "scope-a")
		scopeB, ctxB := newMediaIntegrationScope(t, backend, "scope-b")
		ref, err := New(backend, scopeA).Put(ctxA, "html", "image/png", []byte("scope-bound-payload"))
		if err != nil {
			t.Fatalf("Put in scope A: %v", err)
		}

		if _, err := New(backend, scopeB).Get(ctxB, ref); err == nil || !strings.Contains(err.Error(), "scope") {
			t.Fatalf("cross-scope Get error = %v, want a scope rejection", err)
		}
		forged := ref
		forged.Scope = scopeB
		if _, err := New(backend, scopeB).Get(ctxB, forged); !errors.Is(err, media.ErrMediaNotFound) {
			t.Fatalf("re-scoped reference error = %v, want ErrMediaNotFound (other scope's key space)", err)
		}
	})

	t.Run("PurgeRemovesEveryKey", func(t *testing.T) {
		scope, ctx := newMediaIntegrationScope(t, backend, "purge")
		store := New(backend, scope)
		refs := make([]entity.MediaRef, 0, 2)
		for _, image := range []string{"purge-first", "purge-second"} {
			ref, err := store.Put(ctx, "html", "image/png", []byte(image))
			if err != nil {
				t.Fatalf("Put %q: %v", image, err)
			}
			refs = append(refs, ref)
		}

		digestKey, stateKey := quotaKeys(scope)
		manifest := manifestKey(scope)
		keys := []string{manifest, digestKey, stateKey}
		for _, ref := range refs {
			keys = append(keys, store.key(ref.SHA256))
		}
		if err := PurgeTask(ctx, backend, scope); err != nil {
			t.Fatalf("PurgeTask: %v", err)
		}
		assertMediaKeys(t, backend, ctx, keys...)
		for _, ref := range refs {
			if _, err := store.Get(ctx, ref); !errors.Is(err, media.ErrMediaNotFound) {
				t.Fatalf("Get after purge = %v, want ErrMediaNotFound", err)
			}
		}
	})
}
