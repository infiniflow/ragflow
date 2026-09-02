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

package canvas

import (
	"context"
	"testing"
	"time"

	"ragflow/internal/engine"
	"ragflow/internal/ingestion/testutil"
)

// compile-time assertion: NatsCheckPointStore satisfies the eino
// CheckPointStore interface (Get/Set/Delete) used by canvas.Compile.
var _ CheckPointStore = (*NatsCheckPointStore)(nil)

// newTestNatsStore spins up an embedded NATS server, creates the
// agent_checkpoints bucket, and returns a ready store. It also installs the
// engine into the global accessor so NatsCheckpointExists (which reads the
// global) observes it.
func newTestNatsStore(t *testing.T) *NatsCheckPointStore {
	t.Helper()
	ne := testutil.SetupNatsEngine(t)
	engine.SetNatsEngine(ne)
	store, err := NewNatsCheckPointStore(context.Background(), ne, time.Hour)
	if err != nil {
		t.Fatalf("NewNatsCheckPointStore: %v", err)
	}
	return store
}

func TestNatsCheckPointStoreSetGetRoundTrip(t *testing.T) {
	store := newTestNatsStore(t)
	ctx := context.Background()
	id := "run-1"
	want := []byte("checkpoint-payload-bytes")

	if err := store.Set(ctx, id, want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, found, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatal("Get: expected found=true for a key that was just set")
	}
	if string(got) != string(want) {
		t.Fatalf("Get: payload mismatch: got %q want %q", got, want)
	}
}

func TestNatsCheckPointStoreGetMissingFoundFalse(t *testing.T) {
	store := newTestNatsStore(t)
	ctx := context.Background()

	got, found, err := store.Get(ctx, "does-not-exist")
	if err != nil {
		t.Fatalf("Get on missing key: %v", err)
	}
	if found {
		t.Fatal("Get on missing key: expected found=false")
	}
	if got != nil {
		t.Fatalf("Get on missing key: expected nil payload, got %q", got)
	}
}

func TestNatsCheckPointStoreDeleteIdempotentExisting(t *testing.T) {
	store := newTestNatsStore(t)
	ctx := context.Background()
	id := "run-del"

	if err := store.Set(ctx, id, []byte("payload")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := store.Delete(ctx, id); err != nil {
		t.Fatalf("Delete existing: %v", err)
	}
	_, found, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if found {
		t.Fatal("Get after delete: expected found=false")
	}
}

func TestNatsCheckPointStoreDeleteMissingNoError(t *testing.T) {
	store := newTestNatsStore(t)
	ctx := context.Background()
	// Deleting a key that was never written must not error (idempotent).
	if err := store.Delete(ctx, "never-written"); err != nil {
		t.Fatalf("Delete missing: expected nil error, got %v", err)
	}
}

func TestNatsCheckPointStoreOverwriteKeepsLatest(t *testing.T) {
	store := newTestNatsStore(t)
	ctx := context.Background()
	id := "run-ow"

	if err := store.Set(ctx, id, []byte("v1")); err != nil {
		t.Fatalf("Set v1: %v", err)
	}
	if err := store.Set(ctx, id, []byte("v2-latest")); err != nil {
		t.Fatalf("Set v2: %v", err)
	}
	got, found, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatal("Get: expected found=true")
	}
	if string(got) != "v2-latest" {
		t.Fatalf("Get: expected latest value v2-latest, got %q", got)
	}
}

func TestNatsCheckPointStoreEmptyValue(t *testing.T) {
	store := newTestNatsStore(t)
	ctx := context.Background()
	id := "run-empty"

	if err := store.Set(ctx, id, []byte{}); err != nil {
		t.Fatalf("Set empty: %v", err)
	}
	got, found, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get empty: %v", err)
	}
	if !found {
		t.Fatal("Get empty: expected found=true")
	}
	if len(got) != 0 {
		t.Fatalf("Get empty: expected empty payload, got %d bytes", len(got))
	}
}

func TestNatsCheckPointStoreLargeValue(t *testing.T) {
	store := newTestNatsStore(t)
	ctx := context.Background()
	id := "run-large"
	// 4 MiB — well under the 16 MiB bucket cap, exercises a non-trivial
	// payload boundary (REDIS had no such cap; NATS enforces MaxValueSize).
	payload := make([]byte, 4*1024*1024)
	for i := range payload {
		payload[i] = byte(i % 251)
	}

	if err := store.Set(ctx, id, payload); err != nil {
		t.Fatalf("Set large: %v", err)
	}
	got, found, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get large: %v", err)
	}
	if !found {
		t.Fatal("Get large: expected found=true")
	}
	if len(got) != len(payload) {
		t.Fatalf("Get large: length mismatch %d != %d", len(got), len(payload))
	}
}

func TestNatsCheckPointStoreContextCancelSurfacesError(t *testing.T) {
	store := newTestNatsStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	// Operations against a cancelled ctx must surface the cancellation
	// error rather than silently succeeding or panicking.
	if err := store.Set(ctx, "run-ctx", []byte("x")); err == nil {
		t.Fatal("Set with cancelled ctx: expected an error, got nil")
	}
	if _, _, err := store.Get(ctx, "run-ctx"); err == nil {
		t.Fatal("Get with cancelled ctx: expected an error, got nil")
	}
}

func TestNatsCheckpointExistsTrue(t *testing.T) {
	store := newTestNatsStore(t)
	ctx := context.Background()
	id := "run-exists"

	if err := store.Set(ctx, id, []byte("payload")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	found, err := NatsCheckpointExists(ctx, id)
	if err != nil {
		t.Fatalf("NatsCheckpointExists: %v", err)
	}
	if !found {
		t.Fatal("NatsCheckpointExists: expected true for a written checkpoint")
	}
}

func TestNatsCheckpointExistsFalse(t *testing.T) {
	store := newTestNatsStore(t)
	_ = store
	ctx := context.Background()

	found, err := NatsCheckpointExists(ctx, "never-written")
	if err != nil {
		t.Fatalf("NatsCheckpointExists: %v", err)
	}
	if found {
		t.Fatal("NatsCheckpointExists: expected false for a missing checkpoint")
	}
}

func TestNatsCheckpointExistsNoEngine(t *testing.T) {
	// No NATS engine installed globally → there is no checkpoint to resume,
	// so the probe must report (false, nil), never an error.
	engine.SetNatsEngine(nil)
	ctx := context.Background()

	found, err := NatsCheckpointExists(ctx, "run-x")
	if err != nil {
		t.Fatalf("NatsCheckpointExists with no engine: expected nil error, got %v", err)
	}
	if found {
		t.Fatal("NatsCheckpointExists with no engine: expected false")
	}
}

func TestNatsCheckPointStoreBucketNameIsSubjectSafe(t *testing.T) {
	// NATS KV keys are valid NATS subjects, so ":" is illegal. The store keys
	// by the raw id (no "agent:cp:" prefix); assert the bucket name contains
	// no ":" so a regression to the Redis prefix cannot slip in.
	if len(agentCheckpointsBucket) == 0 {
		t.Fatal("agentCheckpointsBucket must not be empty")
	}
	for _, c := range agentCheckpointsBucket {
		if c == ':' {
			t.Fatalf("agentCheckpointsBucket %q contains illegal ':' for a NATS subject", agentCheckpointsBucket)
		}
	}
}
