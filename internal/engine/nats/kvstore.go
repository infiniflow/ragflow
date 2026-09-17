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

package nats

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// Default limits applied to a cache namespace bucket when the caller does not
// specify them. A cache entry only needs the latest revision, so History=1 and
// a 1 MiB value cap are sane defaults; Storage defaults to FileStorage so the
// cache survives restarts and is not bounded by RAM.
const (
	defaultKVHistory      = 1
	defaultKVMaxValueSize = 1024 * 1024
)

// BucketConfig describes the policy for one cache namespace's KV bucket. It is a
// thin, ergonomic wrapper over jetstream.KeyValueConfig: zero values for
// History/MaxValueSize/Storage fall back to cache-appropriate defaults in
// NewNatsKVStore rather than to the JetStream library defaults.
type BucketConfig struct {
	Name         string
	Description  string
	History      int // cache: 1 (keep only the latest revision)
	TTL          time.Duration
	MaxValueSize int // OCR: 8 MiB; others: 1 MiB default
	Storage      jetstream.StorageType
	Replicas     int // cache: 1 (rebuildable)
	Compression  bool
}

// NatsKVStore is a thin wrapper over a JetStream KV bucket. It normalizes the
// not-found and delete semantics used across the cache namespaces (ocr,
// embedding, checkpoint) so each caller shares one bucket-management path.
type NatsKVStore struct {
	kv jetstream.KeyValue
}

// NewNatsKVStore creates or updates the backing bucket and returns a handle.
// It uses CreateOrUpdateKeyValue so re-configuring an existing bucket is
// idempotent and safe to call on every process start.
func NewNatsKVStore(ctx context.Context, js jetstream.JetStream, cfg BucketConfig) (*NatsKVStore, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("NewNatsKVStore: bucket Name must not be empty")
	}
	if cfg.History == 0 {
		cfg.History = defaultKVHistory
	}
	storage := cfg.Storage
	if storage == 0 {
		storage = jetstream.FileStorage
	}
	if cfg.MaxValueSize == 0 {
		cfg.MaxValueSize = defaultKVMaxValueSize
	}

	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:       cfg.Name,
		Description:  cfg.Description,
		History:      uint8(cfg.History),
		TTL:          cfg.TTL,
		MaxValueSize: int32(cfg.MaxValueSize),
		Storage:      storage,
		Replicas:     cfg.Replicas,
		Compression:  cfg.Compression,
	})
	if err != nil {
		return nil, fmt.Errorf("NewNatsKVStore: create bucket %q: %w", cfg.Name, err)
	}
	return &NatsKVStore{kv: kv}, nil
}

// Put writes value for key, overwriting any existing value.
func (s *NatsKVStore) Put(ctx context.Context, key string, val []byte) error {
	if _, err := s.kv.Put(ctx, key, val); err != nil {
		return fmt.Errorf("NatsKVStore.Put: %w", err)
	}
	return nil
}

// Get returns (value, found, error). A missing key is surfaced as
// found=false with a nil error, never as ErrKeyNotFound.
func (s *NatsKVStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	entry, err := s.kv.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("NatsKVStore.Get: %w", err)
	}
	return entry.Value(), true, nil
}

// Delete removes key. Deleting a missing key is treated as success (idempotent)
// and does not return ErrKeyNotFound.
func (s *NatsKVStore) Delete(ctx context.Context, key string) error {
	if err := s.kv.Delete(ctx, key); err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil
		}
		return fmt.Errorf("NatsKVStore.Delete: %w", err)
	}
	return nil
}

// Keys lists all keys currently in the bucket. It returns a non-nil, empty
// slice when the bucket has no keys, so callers can safely range over or take
// len() of the result.
func (s *NatsKVStore) Keys(ctx context.Context) ([]string, error) {
	lister, err := s.kv.ListKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("NatsKVStore.Keys: %w", err)
	}
	defer lister.Stop()

	keys := make([]string, 0)
	for k := range lister.Keys() {
		keys = append(keys, k)
	}
	return keys, nil
}

// EnsureKVBucket creates or updates a cache bucket on this engine and returns a
// ready-to-use NatsKVStore. It does not reuse the engine's dedicated
// knowledge-compile lease handle (NatsEngine.kv); the two are independent
// buckets.
func (n *NatsEngine) EnsureKVBucket(ctx context.Context, cfg BucketConfig) (*NatsKVStore, error) {
	if n.jetStream == nil {
		return nil, fmt.Errorf("EnsureKVBucket: jetStream not initialized (call Init first)")
	}
	return NewNatsKVStore(ctx, n.jetStream, cfg)
}
