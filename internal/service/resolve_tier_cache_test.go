package service

import (
	"context"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

// tenantToTierResolver maps a specific tenant to a tier so a test can tell
// whether resolveTier handed the resolver the cached tenant or the DB tenant.
type tenantToTierResolver struct {
	byTenant map[string]common.Tier
	got      string
	calls    int
}

func (r *tenantToTierResolver) resolve(_ context.Context, tenantID string) (common.Tier, error) {
	r.calls++
	r.got = tenantID
	return r.byTenant[tenantID], nil
}

// TestResolveTierReusesKBCache asserts that resolveTier prefers a
// pre-populated kbCache entry over a fresh kbDAO.GetByID. The cache entry uses a
// tenant the DB does not contain, so if the cache were ignored the call would
// hit the DAO, fail to find the KB, and surface an error — a wrong
// implementation that always re-queries would fail here.
func TestResolveTierReusesKBCache(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	svc := NewIngestionTaskService()
	resolver := &tenantToTierResolver{byTenant: map[string]common.Tier{
		"tenant-cache": common.TierDefault,
		"tenant-db":    common.Tier("alt"),
	}}
	svc.tierResolver = resolver.resolve

	// Cache hit for a KB that does NOT exist in the DB.
	cache := map[string]*entity.Knowledgebase{
		"missing-kb": {ID: "missing-kb", TenantID: "tenant-cache"},
	}
	tier, err := svc.resolveTier(t.Context(), "missing-kb", cache)
	if err != nil {
		t.Fatalf("resolveTier with cached KB returned error: %v", err)
	}
	if tier != common.TierDefault {
		t.Fatalf("resolveTier used DB tenant, want cached tenant tier %q, got %q", common.TierDefault, tier)
	}
	if resolver.got != "tenant-cache" {
		t.Fatalf("resolver received tenant %q, want %q (cache not used)", resolver.got, "tenant-cache")
	}
}

// TestResolveTierFallsBackToDAOOnCacheMiss asserts that when the cache has no
// entry resolveTier queries the DAO, resolves via the DB tenant, and fills the
// (non-nil) cache so a later call for the same dataset reuses the entry.
func TestResolveTierFallsBackToDAOOnCacheMiss(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-real", "tenant-db", 1, 0, 0)

	svc := NewIngestionTaskService()
	resolver := &tenantToTierResolver{byTenant: map[string]common.Tier{
		"tenant-db": common.Tier("alt"),
	}}
	svc.tierResolver = resolver.resolve

	cache := map[string]*entity.Knowledgebase{}
	tier, err := svc.resolveTier(t.Context(), "kb-real", cache)
	if err != nil {
		t.Fatalf("resolveTier on cache miss returned error: %v", err)
	}
	if tier != common.Tier("alt") {
		t.Fatalf("resolveTier miss path: want %q, got %q", common.Tier("alt"), tier)
	}
	if resolver.got != "tenant-db" {
		t.Fatalf("resolver received tenant %q, want %q", resolver.got, "tenant-db")
	}
	if cache["kb-real"] == nil {
		t.Fatalf("resolveTier did not fill the cache on miss")
	}

	// Second call for the same dataset must hit the cache and return the same
	// tier without re-querying the DAO. The DAO dedup itself is proven by
	// TestResolveTierReusesKBCache (a cached KB absent from the DB resolves with
	// no error); here we only assert the cache-hit path returns a valid tier.
	if tier2, err := svc.resolveTier(t.Context(), "kb-real", cache); err != nil {
		t.Fatalf("resolveTier second call returned error: %v", err)
	} else if tier2 != common.Tier("alt") {
		t.Fatalf("resolveTier cache-hit tier = %q, want %q", tier2, common.Tier("alt"))
	}
}
