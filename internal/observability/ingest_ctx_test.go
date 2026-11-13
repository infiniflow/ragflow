package observability

import (
	"context"
	"testing"
)

// TestIngestContextAccessors round-trips every identity key through the
// context and confirms the zero-value fallback when a key is absent.
func TestIngestContextAccessors(t *testing.T) {
	ctx := context.Background()
	ctx = WithIngestorName(ctx, "ing-7")
	ctx = WithNatsQueue(ctx, "tasks.RAGFLOW")
	ctx = WithWorkerName(ctx, "worker-3")
	ctx = WithUserName(ctx, "alice")
	ctx = WithTenantName(ctx, "Acme")

	if got := IngestorName(ctx); got != "ing-7" {
		t.Errorf("IngestorName = %q, want ing-7", got)
	}
	if got := NatsQueue(ctx); got != "tasks.RAGFLOW" {
		t.Errorf("NatsQueue = %q, want tasks.RAGFLOW", got)
	}
	if got := WorkerName(ctx); got != "worker-3" {
		t.Errorf("WorkerName = %q, want worker-3", got)
	}
	if got := UserName(ctx); got != "alice" {
		t.Errorf("UserName = %q, want alice", got)
	}
	if got := TenantName(ctx); got != "Acme" {
		t.Errorf("TenantName = %q, want Acme", got)
	}
}

// TestIngestContextAbsentReturnsEmpty confirms each accessor returns "" when
// the key is not present, so a path that forgets to inject identity emits an
// empty (omitted) field instead of panicking.
func TestIngestContextAbsentReturnsEmpty(t *testing.T) {
	ctx := context.Background()
	if got := IngestorName(ctx); got != "" {
		t.Errorf("IngestorName = %q, want empty", got)
	}
	if got := NatsQueue(ctx); got != "" {
		t.Errorf("NatsQueue = %q, want empty", got)
	}
	if got := WorkerName(ctx); got != "" {
		t.Errorf("WorkerName = %q, want empty", got)
	}
	if got := UserName(ctx); got != "" {
		t.Errorf("UserName = %q, want empty", got)
	}
	if got := TenantName(ctx); got != "" {
		t.Errorf("TenantName = %q, want empty", got)
	}
}

// TestIngestContextKeysDoNotLeak verifies keys are distinct: setting one must
// not surface in another accessor.
func TestIngestContextKeysDoNotLeak(t *testing.T) {
	ctx := WithIngestorName(context.Background(), "ing-7")
	if got := NatsQueue(ctx); got != "" {
		t.Errorf("NatsQueue leaked ingestor name: %q", got)
	}
	if got := WorkerName(ctx); got != "" {
		t.Errorf("WorkerName leaked ingestor name: %q", got)
	}
}

// TestWithIngestIdentityBundles confirms the convenience injector sets every
// field from the struct in one call.
func TestWithIngestIdentityBundles(t *testing.T) {
	ctx := WithIngestIdentity(context.Background(), IngestIdentity{
		IngestorName: "ing-7",
		NatsQueue:    "tasks.RAGFLOW",
		WorkerName:   "worker-3",
		UserName:     "alice",
		TenantName:   "Acme",
	})
	if IngestorName(ctx) != "ing-7" || NatsQueue(ctx) != "tasks.RAGFLOW" ||
		WorkerName(ctx) != "worker-3" || UserName(ctx) != "alice" || TenantName(ctx) != "Acme" {
		t.Errorf("WithIngestIdentity did not propagate all fields: %+v", ctx)
	}
}

// TestWithIngestIdentitySkipsEmpty confirms the bundler does not overwrite an
// already-present value with an empty one (so a higher-priority injector wins).
func TestWithIngestIdentitySkipsEmpty(t *testing.T) {
	ctx := WithIngestorName(context.Background(), "preferred")
	ctx = WithIngestIdentity(ctx, IngestIdentity{IngestorName: ""})
	if got := IngestorName(ctx); got != "preferred" {
		t.Errorf("IngestorName = %q, want preferred (empty must not clobber)", got)
	}
}
