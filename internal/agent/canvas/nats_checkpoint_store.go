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

// nats_checkpoint_store.go implements the eino CheckPointStore interface
// backed by a NATS JetStream KV bucket. It replaces the Redis-backed
// RedisCheckPointStore for checkpoint payloads (PR1). See plan §checkpoint.
//
// Unlike Redis, NATS KV keys are valid NATS subjects, so ":" is illegal and
// the Redis "agent:cp:{id}" prefix cannot be reused. We use a dedicated
// bucket (agentCheckpointsBucket) keyed by the raw id.
package canvas

import (
	"context"
	"errors"
	"time"

	"ragflow/internal/engine"
	"ragflow/internal/engine/nats"

	"github.com/nats-io/nats.go/jetstream"
)

// agentCheckpointsBucket is the NATS KV bucket holding eino checkpoint
// payloads. The key is the raw canvas run id (no ":"-prefixed Redis
// namespace, since that is not a legal NATS subject). NATS KV keys must be
// valid subjects, so the id must not contain '/', '*', or '>' — the
// ingestion task id (32-char lowercase hex) satisfies this today; if the id
// format ever changes, Put/Get will surface a clear error rather than
// silently corrupting the key.
const agentCheckpointsBucket = "agent_checkpoints"

// defaultCheckpointMaxValueSize caps a single checkpoint payload at 16 MiB.
// eino checkpoints are structured graphs of component state; real payloads
// are far smaller, but a hard cap prevents a runaway component from
// exhausting the bucket.
//
// Deployment note: this bucket-level cap is enforced by JetStream, but the
// NATS *server* also has a separate MaxPayload limit (default 1 MiB). The
// server limit is lower-level and rejects the message before the bucket cap is
// even considered, so a deployment must raise the server MaxPayload to at
// least this value (the embedded test server uses 64 MiB) or large
// checkpoints fail at the transport layer.
const defaultCheckpointMaxValueSize = 16 * 1024 * 1024

// NatsCheckPointStore is a NATS JetStream KV-backed eino CheckPointStore /
// CheckPointDeleter. Values are stored as raw bytes — the eino Serializer has
// already marshaled the structured payload, so we do not re-encode.
type NatsCheckPointStore struct {
	kv *nats.NatsKVStore
}

// NewNatsCheckPointStore creates (or updates, idempotently) the
// agent_checkpoints bucket on the given engine and returns a ready store. The
// TTL is applied at bucket level (NATS KV has no per-key expiry), so it is set
// here rather than on every Set.
func NewNatsCheckPointStore(ctx context.Context, engine *nats.NatsEngine, ttl time.Duration) (*NatsCheckPointStore, error) {
	kv, err := engine.EnsureKVBucket(ctx, nats.BucketConfig{
		Name:         agentCheckpointsBucket,
		Description:  "eino canvas checkpoint payloads",
		History:      1,
		TTL:          ttl,
		MaxValueSize: defaultCheckpointMaxValueSize,
		Storage:      jetstream.FileStorage,
	})
	if err != nil {
		return nil, err
	}
	return &NatsCheckPointStore{kv: kv}, nil
}

// Get implements eino's CheckPointStore.Get. Returns (nil, false, nil) when
// the key does not exist so callers can distinguish "missing" from
// "present-but-error".
func (s *NatsCheckPointStore) Get(ctx context.Context, id string) ([]byte, bool, error) {
	if s == nil || s.kv == nil {
		return nil, false, errors.New("checkpoint store: nats kv not initialized")
	}
	data, found, err := s.kv.Get(ctx, id)
	if err != nil {
		return nil, false, err
	}
	return data, found, nil
}

// Set implements eino's CheckPointStore.Set.
func (s *NatsCheckPointStore) Set(ctx context.Context, id string, payload []byte) error {
	if s == nil || s.kv == nil {
		return errors.New("checkpoint store: nats kv not initialized")
	}
	return s.kv.Put(ctx, id, payload)
}

// Delete implements eino's optional CheckPointDeleter. It is safe to call on a
// non-existent key (idempotent).
func (s *NatsCheckPointStore) Delete(ctx context.Context, id string) error {
	if s == nil || s.kv == nil {
		return errors.New("checkpoint store: nats kv not initialized")
	}
	return s.kv.Delete(ctx, id)
}

// NatsCheckpointExists reports whether a pipeline checkpoint is present for
// id in the NATS KV bucket. It is used by ingestion progress handling to
// distinguish a fresh run from a resume. When the NATS engine is not
// initialized (no NATS deployment) it reports (false, nil): there can be no
// checkpoint to resume, so the run proceeds as a fresh run rather than
// failing outright.
func NatsCheckpointExists(ctx context.Context, id string) (bool, error) {
	ne := engine.GetNatsEngine()
	if ne == nil {
		return false, nil
	}
	return ne.KeyValueExists(ctx, agentCheckpointsBucket, id)
}
