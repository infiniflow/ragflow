//go:build integration

package opensearch

import (
	"context"
	"testing"

	"ragflow/internal/engine/types"
)

// TestOpenSearchMetadataLifecycle exercises the doc metadata index path
// against a real cluster: CreateMetadataStore rewrites the ES-only
// "dynamic":"runtime" mapping into plain dynamic (OpenSearch rejects runtime),
// then insert / search / update / delete are driven through the engine API.
func TestOpenSearchMetadataLifecycle(t *testing.T) {
	eng, err := NewEngine(t.Context(), getOSTestConfig())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	tenantID := "t-os-it"
	if err := eng.DropMetadataStore(t.Context(), tenantID); err != nil {
		t.Fatalf("DropMetadataStore (cleanup): %v", err)
	}
	// t.Context() is cancelled by the time Cleanup runs; use a fresh context so
	// the teardown delete actually reaches the cluster.
	t.Cleanup(func() {
		_ = eng.DropMetadataStore(context.Background(), tenantID)
	})

	if err := eng.CreateMetadataStore(t.Context(), tenantID); err != nil {
		t.Fatalf("CreateMetadataStore: %v", err)
	}
	exists, err := eng.MetadataStoreExists(t.Context(), tenantID)
	if err != nil {
		t.Fatalf("MetadataStoreExists: %v", err)
	}
	if !exists {
		t.Fatal("MetadataStoreExists = false after create")
	}

	meta := []map[string]interface{}{
		{
			"id":    "doc1",
			"kb_id": osTestKBID,
			"meta_fields": map[string]interface{}{
				"name": "alice",
				"age":  float64(30),
			},
		},
		{
			"id":    "doc2",
			"kb_id": osTestKBID,
			"meta_fields": map[string]interface{}{
				"name": "bob",
				"age":  float64(25),
			},
		},
	}
	if _, err := eng.InsertMetadata(t.Context(), meta, tenantID); err != nil {
		t.Fatalf("InsertMetadata: %v", err)
	}

	// SearchMetadata by kb_id
	res, err := eng.SearchMetadata(t.Context(), &types.SearchMetadataRequest{
		TenantID: tenantID,
		Filter:   map[string]interface{}{"kb_id": osTestKBID},
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("SearchMetadata: %v", err)
	}
	if res.Total != 2 {
		t.Fatalf("SearchMetadata total = %d, want 2", res.Total)
	}

	// UpdateMetadata fully replaces meta_fields
	if err := eng.UpdateMetadata(t.Context(), "doc1", osTestKBID, map[string]interface{}{
		"name":  "alice wang",
		"email": "alice@example.com",
	}, tenantID); err != nil {
		t.Fatalf("UpdateMetadata: %v", err)
	}
	res, err = eng.SearchMetadata(t.Context(), &types.SearchMetadataRequest{
		TenantID:     tenantID,
		Filter:       map[string]interface{}{"id": "doc1"},
		SelectFields: []string{"meta_fields"},
		Limit:        10,
	})
	if err != nil {
		t.Fatalf("SearchMetadata after update: %v", err)
	}
	if res.Total != 1 || len(res.MetadataRecords) != 1 {
		t.Fatalf("SearchMetadata after update: total=%d records=%d, want 1/1", res.Total, len(res.MetadataRecords))
	}
	mf, ok := res.MetadataRecords[0]["meta_fields"].(map[string]interface{})
	if !ok {
		t.Fatalf("meta_fields = %T", res.MetadataRecords[0]["meta_fields"])
	}
	// UpdateMetadata must fully replace: the stale "age" key must be gone.
	if _, stale := mf["age"]; stale {
		t.Fatalf("stale meta_fields key 'age' survived UpdateMetadata: %#v", mf)
	}
	if mf["email"] != "alice@example.com" {
		t.Fatalf("meta_fields.email = %#v", mf["email"])
	}

	// DeleteMetadataKeys removes a key but keeps the document
	if err := eng.DeleteMetadataKeys(t.Context(), "doc1", osTestKBID, []string{"email"}, tenantID); err != nil {
		t.Fatalf("DeleteMetadataKeys: %v", err)
	}
	res, err = eng.SearchMetadata(t.Context(), &types.SearchMetadataRequest{
		TenantID: tenantID,
		Filter:   map[string]interface{}{"id": "doc1"},
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("SearchMetadata after key delete: %v", err)
	}
	if res.Total != 1 {
		t.Fatalf("document removed when only some keys deleted: total=%d", res.Total)
	}

	// DeleteMetadata by condition removes matching docs
	deleted, err := eng.DeleteMetadata(t.Context(), map[string]interface{}{"kb_id": osTestKBID}, tenantID)
	if err != nil {
		t.Fatalf("DeleteMetadata: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("DeleteMetadata deleted %d, want 2", deleted)
	}
	res, err = eng.SearchMetadata(t.Context(), &types.SearchMetadataRequest{
		TenantID: tenantID,
		Filter:   map[string]interface{}{"kb_id": osTestKBID},
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("SearchMetadata after delete: %v", err)
	}
	if res.Total != 0 {
		t.Fatalf("SearchMetadata after delete total = %d, want 0", res.Total)
	}
}
