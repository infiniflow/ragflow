package knowledge_compile

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

// TestRemoveEntriesMatchesByDocAndEventType locks the A0-1 contract: BacklogEntry
// now carries a Variants slice (not comparable), so removeEntries must key on
// DocID+EventType only and must not be affected by Variants. Two entries with the
// same doc_id+event_type but different variants must collapse to a single removal,
// and an entry whose Variants differ from the inflight copy must still be removed
// exactly once.
func TestRemoveEntriesMatchesByDocAndEventType(t *testing.T) {
	inflight := []BacklogEntry{
		{DocID: "d1", EventType: "doc_completed", Variants: []string{"tree", "wiki"}},
		{DocID: "d2", EventType: "doc_completed", Variants: []string{"structure"}},
		{DocID: "d3", EventType: "doc_deleted"},
	}

	// Batch claims d1 (with a variants subset different from inflight) and d3.
	batch := []BacklogEntry{
		{DocID: "d1", EventType: "doc_completed", Variants: []string{"wiki"}},
		{DocID: "d3", EventType: "doc_deleted"},
	}

	out := removeEntries(inflight, batch)

	if len(out) != 1 || out[0].DocID != "d2" {
		t.Fatalf("removeEntries should keep only d2, got %+v", out)
	}
}

func TestFakeSchedulerReclaimsInterruptedClaim(t *testing.T) {
	f := NewFakeScheduler()
	if err := f.Publish(t.Context(), "t1", "kb1", "d1", string(EventTypeCompleted), []string{"wiki"}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	first, ok, err := f.Claim(t.Context(), "kb1")
	if err != nil || !ok {
		t.Fatalf("initial claim: ok=%v err=%v", ok, err)
	}
	f.mu.Lock()
	expired := time.Now().Add(-time.Second)
	f.rows["kb1"].expires = &expired
	f.mu.Unlock()
	second, ok, err := f.TryClaim(t.Context())
	if err != nil || !ok {
		t.Fatalf("reclaimed claim: ok=%v err=%v", ok, err)
	}
	if second.Token == first.Token || len(second.Entries) != 1 || second.Entries[0].DocID != "d1" {
		t.Fatalf("reclaimed claim did not replace the old lease: first=%+v second=%+v", first, second)
	}
}

func TestFakeSchedulerProgressIsClaimScoped(t *testing.T) {
	f := NewFakeScheduler()
	if err := f.Publish(t.Context(), "t1", "kb1", "d1", string(EventTypeCompleted), nil); err != nil {
		t.Fatalf("publish: %v", err)
	}
	claim, ok, err := f.Claim(t.Context(), "kb1")
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	if err := f.UpdateProgress(t.Context(), "kb1", claim.Token, 0.5, "routing_pages", "Routing pages"); err != nil {
		t.Fatalf("update progress: %v", err)
	}
	if err := f.UpdateProgress(t.Context(), "kb1", "stale-token", 0.9, "wrong", "Should be ignored"); err != nil {
		t.Fatalf("stale update: %v", err)
	}
	f.mu.Lock()
	row := f.rows["kb1"]
	f.mu.Unlock()
	if row.progress != 0.5 || row.currentPhase != "routing_pages" || row.progressMsg != "Routing pages" {
		t.Fatalf("unexpected progress state: %+v", row)
	}
}

func TestWithWriteLockRejectsSupersededClaim(t *testing.T) {
	f := NewFakeScheduler()
	if err := f.Publish(t.Context(), "t1", "kb1", "d1", string(EventTypeCompleted), []string{"wiki"}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	claim, ok, err := f.Claim(t.Context(), "kb1")
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	c := &Consumer{scheduler: f}
	if err := f.CancelInflight(t.Context(), "kb1", claim.Token); err != nil {
		t.Fatalf("cancel inflight: %v", err)
	}
	writes := 0
	err = c.withWriteLock(t.Context(), "kb1", claim.Token, func() error {
		writes++
		return nil
	})
	if !errors.Is(err, errClaimSuperseded) {
		t.Fatalf("withWriteLock error = %v, want errClaimSuperseded", err)
	}
	if writes != 0 {
		t.Fatalf("superseded claim executed a write callback")
	}
}

// TestRemoveEntriesEmptyBatch verifies removeEntries with an empty batch is a no-op.
func TestRemoveEntriesEmptyBatch(t *testing.T) {
	inflight := []BacklogEntry{{DocID: "d1", EventType: "doc_completed", Variants: []string{"tree"}}}
	out := removeEntries(inflight, nil)
	if !reflect.DeepEqual(out, inflight) {
		t.Fatalf("empty batch should preserve inflight, got %+v", out)
	}
}

// TestBacklogEntryJSONBackwardCompat locks the A0-1 JSON contract: an old backlog
// row serialized without the "variants" field must unmarshal with Variants == nil
// (empty, not an error), which the consumer treats as "legacy/unknown" and falls
// back to the unified path.
func TestBacklogEntryJSONBackwardCompat(t *testing.T) {
	old := []byte(`{"doc_id":"d1","event_type":"doc_completed"}`)
	var e BacklogEntry
	if err := json.Unmarshal(old, &e); err != nil {
		t.Fatalf("unmarshal legacy backlog: %v", err)
	}
	if e.DocID != "d1" || e.EventType != "doc_completed" {
		t.Fatalf("legacy fields not restored: %+v", e)
	}
	if e.Variants != nil {
		t.Fatalf("legacy backlog should unmarshal Variants as nil, got %v", e.Variants)
	}

	// A completed event with variants round-trips intact.
	withVariants := []byte(`{"doc_id":"d1","event_type":"doc_completed","variants":["tree","wiki"]}`)
	if err := json.Unmarshal(withVariants, &e); err != nil {
		t.Fatalf("unmarshal variants backlog: %v", err)
	}
	if !reflect.DeepEqual(e.Variants, []string{"tree", "wiki"}) {
		t.Fatalf("variants not restored: %+v", e.Variants)
	}
	// omitempty: a nil Variants serializes without the key (matches legacy format).
	enc, err := json.Marshal(BacklogEntry{DocID: "d2", EventType: "doc_deleted"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(enc, &decoded); err != nil {
		t.Fatalf("re-marshal decode: %v", err)
	}
	if _, hasVariants := decoded["variants"]; hasVariants {
		t.Fatalf("nil Variants should be omitted, got %s", enc)
	}
}

// TestFakeSchedulerPublishCarriesVariants locks the A0-2 contract: FakeScheduler
// records the variants on the backlog entry, so the consumer sees them.
func TestFakeSchedulerPublishCarriesVariants(t *testing.T) {
	f := NewFakeScheduler()
	if err := f.Publish(t.Context(), "t1", "kb1", "d1", string(EventTypeCompleted), []string{"tree", "structure"}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := f.Publish(t.Context(), "t1", "kb1", "d2", string(EventTypeDeleted), nil); err != nil {
		t.Fatalf("publish deleted: %v", err)
	}
	res, ok, err := f.Claim(t.Context(), "kb1")
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	if len(res.Entries) != 2 {
		t.Fatalf("want 2 backlog entries, got %d", len(res.Entries))
	}
	if !reflect.DeepEqual(res.Entries[0].Variants, []string{"tree", "structure"}) {
		t.Fatalf("completed variants not carried: %+v", res.Entries[0])
	}
	if res.Entries[1].Variants != nil {
		t.Fatalf("deleted event should carry nil variants, got %+v", res.Entries[1])
	}
}
