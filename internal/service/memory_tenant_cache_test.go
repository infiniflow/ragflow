package service

import (
	"context"
	"errors"
	"testing"

	"ragflow/internal/entity"
)

// TestResolveMemoryTenantDedupPerPass asserts that resolveMemoryTenant queries
// the memory config at most once per unique memoryID within a single reconcile
// pass. Two tasks that share memoryID M1 and a third referencing M2 must trigger
// exactly two config lookups (one per unique memory), so a wrong implementation
// that re-queries every task would fail here.
func TestResolveMemoryTenantDedupPerPass(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	svc := NewMemoryMessageService(nil)
	var calls int
	svc.memoryConfigFn = func(_ context.Context, memoryID string) (*CreateMemoryResponse, error) {
		calls++
		return &CreateMemoryResponse{Memory: entity.Memory{TenantID: "tenant-" + memoryID}}, nil
	}

	cache := map[string]string{}
	if _, err := svc.resolveMemoryTenant(t.Context(), "M1", cache); err != nil {
		t.Fatalf("resolveMemoryTenant M1: %v", err)
	}
	if _, err := svc.resolveMemoryTenant(t.Context(), "M1", cache); err != nil {
		t.Fatalf("resolveMemoryTenant M1 again: %v", err)
	}
	if _, err := svc.resolveMemoryTenant(t.Context(), "M2", cache); err != nil {
		t.Fatalf("resolveMemoryTenant M2: %v", err)
	}
	if calls != 2 {
		t.Fatalf("getMemoryConfig called %d times, want 2 (once per unique memoryID)", calls)
	}
	if cache["M1"] == "" || cache["M2"] == "" {
		t.Fatalf("per-pass cache not populated: %v", cache)
	}
}

// TestResolveMemoryTenantCachesMissingMemory asserts that a memory whose config
// lookup fails is still resolved to an empty tenant (so the caller falls back to
// the primary lane) and that the failed lookup is cached for the pass — a deleted
// memory's lingering task rows must not re-trigger the same lookup, nor repeat
// the warning, on every task of every reconcile pass.
func TestResolveMemoryTenantCachesMissingMemory(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	svc := NewMemoryMessageService(nil)
	var calls int
	svc.memoryConfigFn = func(_ context.Context, _ string) (*CreateMemoryResponse, error) {
		calls++
		return nil, errors.New("memory not found")
	}

	cache := map[string]string{}
	for i := 0; i < 3; i++ {
		tid, err := svc.resolveMemoryTenant(t.Context(), "missing", cache)
		if err != nil {
			t.Fatalf("resolveMemoryTenant missing (iter %d): %v", i, err)
		}
		if tid != "" {
			t.Fatalf("missing memory should yield empty tenant, got %q", tid)
		}
	}
	if calls != 1 {
		t.Fatalf("getMemoryConfig called %d times for a missing memory, want 1 (cached fallback)", calls)
	}
}
