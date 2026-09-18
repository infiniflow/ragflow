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

//go:build integration

// Package kvrocks integration tests exercise the real connection path used by
// the server (cmd/ragflow_server.go calls kvrocks.Init). By default they run
// against an in-process miniredis (Redis-protocol) instance so they need no
// external service. Set KVROCKS_TEST_ADDR (host:port) to point them at a real
// Kvrocks deployment and prove the Go stack actually talks to Kvrocks.
package kvrocks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"ragflow/internal/server"
)

// loadConfigForTest writes a minimal service_conf.yaml whose `kvrocks` section
// points at kvrocksAddr (host:port) and loads it via server.Init. server.Init
// only parses config (no connections), so a minimal file is enough to drive
// kvrocks.Init's connection path.
func loadConfigForTest(t *testing.T, kvrocksAddr string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "service_conf.yaml")
	content := fmt.Sprintf(`general:
  cache_engine: kvrocks
kvrocks:
  db: 1
  password: 'infini_rag_flow'
  host: '%s'
`, kvrocksAddr)
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := server.Init(cfgPath); err != nil {
		t.Fatalf("server.Init: %v", err)
	}
}

// resetKvrocksState clears the package-level init once and client so each test
// can re-run kvrocks.Init against a freshly parsed config. kvrocks.Init is
// guarded by a sync.Once, while server.Init re-parses the global config on every
// call, so resetting here lets both tests share one process safely.
func resetKvrocksState() {
	once = sync.Once{}
	globalClient = nil
}

// TestKvrocksInitConnects proves the Go stack initializes a client against a
// reachable Kvrocks and can round-trip a key. This is the exact Init call the
// server makes at startup.
func TestKvrocksInitConnects(t *testing.T) {
	resetKvrocksState()
	addr := os.Getenv("KVROCKS_TEST_ADDR")
	var mr *miniredis.Miniredis
	if addr == "" {
		var err error
		mr, err = miniredis.Run()
		if err != nil {
			t.Fatalf("miniredis: %v", err)
		}
		t.Cleanup(mr.Close)
		addr = mr.Addr()
	}

	loadConfigForTest(t, addr)

	if err := Init(context.Background()); err != nil {
		t.Fatalf("kvrocks.Init to %s should succeed, got: %v", addr, err)
	}
	defer Close()

	if Get() == nil {
		t.Fatal("kvrocks client should be initialized")
	}
	if !IsEnabled() {
		t.Fatal("kvrocks should report enabled")
	}

	// Round-trip a key to prove the connection is real, not just a Ping.
	ctx := context.Background()
	const probeKey = "kvrocks_integration_probe"
	if !Get().Set(ctx, probeKey, "ok", 0) {
		t.Fatal("Set failed against backend")
	}
	val, err := Get().Get(ctx, probeKey)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "ok" {
		t.Fatalf("round-trip mismatch: got %q", val)
	}
	Get().Delete(ctx, probeKey)
}

// TestKvrocksInitFailFast proves that when Kvrocks is unreachable the process
// fails fast with a clear error instead of silently falling back to another
// backend (the Valkey OOM regression this migration fixes).
func TestKvrocksInitFailFast(t *testing.T) {
	resetKvrocksState()
	// 127.0.0.1:1 has nothing listening, so Ping fails immediately.
	const deadAddr = "127.0.0.1:1"
	loadConfigForTest(t, deadAddr)

	err := Init(context.Background())
	if err == nil {
		t.Fatal("kvrocks.Init should fail when Kvrocks is unreachable")
	}
	if !strings.Contains(err.Error(), "failed to connect to Kvrocks") {
		t.Fatalf("expected fail-fast error, got: %v", err)
	}
}
