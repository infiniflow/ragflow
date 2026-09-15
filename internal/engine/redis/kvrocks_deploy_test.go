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

package redis

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"
)

// TestKvrocksEntrypointRequiresPassword drives the real entrypoint script with a
// stub `kvrocks` on PATH so we can assert its pre-exec behavior without a real
// Kvrocks binary. It guards Improvement 1: the entrypoint must refuse to start
// without REDIS_PASSWORD (the previous Valkey deployment always required it) and
// must render requirepass from it.
func TestKvrocksEntrypointRequiresPassword(t *testing.T) {
	root := repoRoot(t)
	ep := filepath.Join(root, "docker/kvrocks-entrypoint.sh")
	if _, err := os.Stat(ep); err != nil {
		t.Fatalf("entrypoint not found: %v", err)
	}

	// runEntrypoint executes the entrypoint with a stub `kvrocks` on PATH and a
	// temp KVROCKS_DIR, injecting (or omitting) REDIS_PASSWORD.
	runEntrypoint := func(t *testing.T, password string) error {
		t.Helper()
		dir := t.TempDir()
		stub := filepath.Join(dir, "kvrocks")
		if err := os.WriteFile(stub, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("sh", ep)
		env := []string{
			"PATH=" + dir + ":" + os.Getenv("PATH"),
			"KVROCKS_DIR=" + dir,
			"HOME=" + dir,
		}
		if password != "" {
			env = append(env, "REDIS_PASSWORD="+password)
		}
		cmd.Env = env
		return cmd.Run()
	}

	t.Run("MissingPasswordFails", func(t *testing.T) {
		if err := runEntrypoint(t, ""); err == nil {
			t.Fatal("entrypoint should fail (non-zero) when REDIS_PASSWORD is unset")
		}
	})

	t.Run("RendersRequirepass", func(t *testing.T) {
		dir := t.TempDir()
		stub := filepath.Join(dir, "kvrocks")
		if err := os.WriteFile(stub, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("sh", ep)
		cmd.Env = []string{
			"PATH=" + dir + ":" + os.Getenv("PATH"),
			"KVROCKS_DIR=" + dir,
			"HOME=" + dir,
			"REDIS_PASSWORD=ci-test-pass",
		}
		if err := cmd.Run(); err != nil {
			t.Fatalf("entrypoint failed with password set: %v", err)
		}
		conf, err := os.ReadFile(filepath.Join(dir, "kvrocks.conf"))
		if err != nil {
			t.Fatalf("read conf: %v", err)
		}
		if !strings.Contains(string(conf), "requirepass ci-test-pass") {
			t.Fatalf("conf missing requirepass: %q", string(conf))
		}
	})
}

// TestKvrocksRequirepassEnforced asserts that the live Kvrocks enforces
// requirepass: a client without a password cannot PING. This is the runtime
// contract that Blocker A (workflow must forward -e REDIS_PASSWORD) and
// Improvement 1 (entrypoint requires the password) both produce.
func TestKvrocksRequirepassEnforced(t *testing.T) {
	_ = newKVClient(t) // ensures a live, password-protected Kvrocks
	ctx := t.Context()
	addr := os.Getenv("KV_ADDR")
	noAuth := redis.NewClient(&redis.Options{Addr: addr})
	defer func() { _ = noAuth.Close() }()
	if err := noAuth.Ping(ctx).Err(); err == nil {
		t.Fatal("PING without password succeeded; requirepass not enforced")
	}
}
