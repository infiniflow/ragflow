//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//

package runtime

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// newTestRedis spins up an in-process miniredis and returns a connected
// client plus a teardown. The miniredis instance is reachable only from
// the test goroutine that called this helper.
func newTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
	})
	return rdb, mr
}

// TestDefault_ReturnsGo is the default-runtime acceptance
// assertion: with no env var override, Default() must return
// RuntimeGo. The env var is explicitly cleared so the test is
// hermetic.
func TestDefault_ReturnsGo(t *testing.T) {
	t.Setenv(defaultEnvKey, "")
	ResetDefaultCache()
	if got := Default(); got != RuntimeGo {
		t.Fatalf("Default() = %q, want %q (Go default)", got, RuntimeGo)
	}
}

// TestDefault_RespectsEnv exercises the env-var override path through
// Default(). The Go default holds for unset/unknown values; explicit
// "python" / "auto" still win.
func TestDefault_RespectsEnv(t *testing.T) {
	t.Setenv(defaultEnvKey, string(RuntimePython))
	ResetDefaultCache()
	if got := Default(); got != RuntimePython {
		t.Fatalf("Default() with python env = %q, want %q", got, RuntimePython)
	}

	t.Setenv(defaultEnvKey, string(RuntimeAuto))
	ResetDefaultCache()
	if got := Default(); got != RuntimeAuto {
		t.Fatalf("Default() with auto env = %q, want %q", got, RuntimeAuto)
	}

	t.Setenv(defaultEnvKey, "bogus")
	ResetDefaultCache()
	if got := Default(); got != RuntimeGo {
		t.Fatalf("Default() with invalid env = %q, want %q (Go fallback)", got, RuntimeGo)
	}
}

// TestSelector_PerTenantOverride verifies the per-tenant override
// mechanism still works with the Go default: even when Default()
// would return Go, a tenant explicitly Set to Python must be
// routed there.
func TestSelector_PerTenantOverride(t *testing.T) {
	t.Setenv(defaultEnvKey, "")
	ResetDefaultCache()

	rdb, _ := newTestRedis(t)
	s := NewSelector(rdb, nil)
	ctx := context.Background()

	// Sanity: the global default is Go now.
	if got := Default(); got != RuntimeGo {
		t.Fatalf("setup: Default() = %q, want %q", got, RuntimeGo)
	}

	// Tenant with no override -> Go.
	got, err := s.Select(ctx, "tenant_unset")
	if err != nil {
		t.Fatalf("Select() unset tenant: %v", err)
	}
	if got != RuntimeGo {
		t.Fatalf("Select() unset tenant = %q, want %q", got, RuntimeGo)
	}

	// Force tenant_force_python to Python even though default is Go.
	if err := s.Set(ctx, "tenant_force_python", RuntimePython); err != nil {
		t.Fatalf("Set() unexpected error: %v", err)
	}
	got, err = s.Select(ctx, "tenant_force_python")
	if err != nil {
		t.Fatalf("Select() force-python tenant: %v", err)
	}
	if got != RuntimePython {
		t.Fatalf("Select() force-python tenant = %q, want %q (per-tenant override)", got, RuntimePython)
	}
}

// TestSelector_Select_FallbackToDefault covers the "no Redis key" path:
// Select must return the process-wide default with no error.
func TestSelector_Select_FallbackToDefault(t *testing.T) {
	t.Setenv(defaultEnvKey, string(RuntimeGo))
	ResetDefaultCache()

	rdb, _ := newTestRedis(t)
	s := NewSelector(rdb, nil)

	got, err := s.Select(context.Background(), "tenant_42")
	if err != nil {
		t.Fatalf("Select() unexpected error: %v", err)
	}
	if got != RuntimeGo {
		t.Fatalf("Select() = %q, want %q", got, RuntimeGo)
	}
}

// TestSelector_SetAndSelectOverride proves that Set makes the override
// visible to subsequent Select calls, even if the env default would have
// been different.
func TestSelector_SetAndSelectOverride(t *testing.T) {
	t.Setenv(defaultEnvKey, string(RuntimeGo))
	ResetDefaultCache()

	rdb, _ := newTestRedis(t)
	s := NewSelector(rdb, nil)
	ctx := context.Background()

	if err := s.Set(ctx, "tenant_a", RuntimePython); err != nil {
		t.Fatalf("Set() unexpected error: %v", err)
	}
	got, err := s.Select(ctx, "tenant_a")
	if err != nil {
		t.Fatalf("Select() unexpected error: %v", err)
	}
	if got != RuntimePython {
		t.Fatalf("Select() after Set = %q, want %q", got, RuntimePython)
	}

	// Different tenant still falls back to the Go default.
	got, err = s.Select(ctx, "tenant_b")
	if err != nil {
		t.Fatalf("Select() for other tenant: %v", err)
	}
	if got != RuntimeGo {
		t.Fatalf("Select() for other tenant = %q, want %q", got, RuntimeGo)
	}
}

// TestSelector_SetRejectsInvalidMode makes sure a bad mode cannot poison
// the override key.
func TestSelector_SetRejectsInvalidMode(t *testing.T) {
	rdb, _ := newTestRedis(t)
	s := NewSelector(rdb, nil)
	if err := s.Set(context.Background(), "tenant_x", RuntimeMode("rust")); err == nil {
		t.Fatal("Set() with invalid mode should error, got nil")
	}
}

// TestSelector_NilRedis_NoError covers the "Redis not initialised" path:
// Select returns the default and never errors; Set errors so the caller
// knows the override wasn't applied.
func TestSelector_NilRedis_NoError(t *testing.T) {
	t.Setenv(defaultEnvKey, string(RuntimeGo))
	ResetDefaultCache()

	s := NewSelector(nil, nil)
	got, err := s.Select(context.Background(), "tenant_1")
	if err != nil {
		t.Fatalf("Select() with nil redis errored: %v", err)
	}
	if got != RuntimeGo {
		t.Fatalf("Select() with nil redis = %q, want default %q", got, RuntimeGo)
	}

	if err := s.Set(context.Background(), "tenant_1", RuntimePython); err == nil {
		t.Fatal("Set() with nil redis should error")
	}
}
