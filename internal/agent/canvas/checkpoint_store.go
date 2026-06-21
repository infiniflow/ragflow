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

// checkpoint_store.go implements the eino CheckPointStore / CheckPointDeleter
// interfaces backed by Redis. See plan §2.6 (Redis-backed CheckPointStore).
//
// The store holds raw eino-serialized checkpoint bytes keyed by
// "agent:cp:{id}". Business metadata (canvas_id, run_id, status, ...) lives
// in a separate Hash key managed by run_tracker.go.
package canvas

import (
	"context"
	"errors"
	redis2 "ragflow/internal/engine/redis"
	"time"

	"github.com/redis/go-redis/v9"
)

// checkpointKeyPrefix is the Redis key namespace for checkpoint payloads.
// The full key is "agent:cp:{id}".
const checkpointKeyPrefix = "agent:cp:"

// RedisCheckPointStore is a Redis-backed eino CheckPointStore /
// CheckPointDeleter. Values are stored as raw bytes — the eino Serializer
// has already marshaled the structured payload, so we do not re-encode.
type RedisCheckPointStore struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedisCheckPointStore returns a store wired to the global Redis client
// from internal/cache. Returns a non-nil store even when the cache is
// uninitialized (client is nil); Get/Set/Delete will return an error in that
// case rather than nil-deref, but the type stays usable for tests that
// inject their own client via struct-literal construction.
func NewRedisCheckPointStore(ttl time.Duration) *RedisCheckPointStore {
	var client *redis.Client
	if rc := redis2.Get(); rc != nil {
		client = rc.GetClient()
	}
	return &RedisCheckPointStore{client: client, ttl: ttl}
}

// NewRedisCheckPointStoreWithClient returns a store wired to a
// caller-supplied redis.Client. Same rationale as
// NewRunTrackerWithClient: enables test code (or any code that
// needs a dedicated Redis pool) to inject a client without going
// through the global cache singleton.
func NewRedisCheckPointStoreWithClient(client *redis.Client, ttl time.Duration) *RedisCheckPointStore {
	return &RedisCheckPointStore{client: client, ttl: ttl}
}

// Get implements eino's CheckPointStore.Get. Returns (nil, false, nil) when
// the key does not exist (redis.Nil) so callers can distinguish "missing"
// from "present-but-error".
func (s *RedisCheckPointStore) Get(ctx context.Context, id string) ([]byte, bool, error) {
	if s == nil || s.client == nil {
		return nil, false, errors.New("checkpoint store: redis client not initialized")
	}
	data, err := s.client.Get(ctx, checkpointKeyPrefix+id).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

// Set implements eino's CheckPointStore.Set. The TTL is applied on every
// call so a frequently-updated checkpoint does not expire mid-run.
func (s *RedisCheckPointStore) Set(ctx context.Context, id string, payload []byte) error {
	if s == nil || s.client == nil {
		return errors.New("checkpoint store: redis client not initialized")
	}
	return s.client.Set(ctx, checkpointKeyPrefix+id, payload, s.ttl).Err()
}

// Delete implements eino's optional CheckPointDeleter. It is safe to call
// on a non-existent key (Del returns 0, no error).
func (s *RedisCheckPointStore) Delete(ctx context.Context, id string) error {
	if s == nil || s.client == nil {
		return errors.New("checkpoint store: redis client not initialized")
	}
	return s.client.Del(ctx, checkpointKeyPrefix+id).Err()
}
