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

// Package llmcache provides a production-grade, multi-tenant, generic LLM-result
// cache engine shared across all packages (ingestion extractor, canvas flow,
// replay harness, ...).
//
// Design goals:
//   - Tenant physical isolation: every key is bound to tenant_id, so no cross-
//     tenant cache hits are possible, and a tenant can be purged on demand.
//   - Generics: one engine instance per value type T (e.g. []string,
//     map[string]any); the engine is storage/serialization agnostic.
//   - Task-aware quality gate: a caller-supplied Validator decides whether a
//     computed value is worth caching (allows legitimate empty results,
//     rejects ERROR / truncated payloads).
//   - Observable: an optional MetricsHook receives hit/miss/compute/skip/purge
//     events for Prometheus instrumentation without hard-coupling the package.
package llmcache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/engine/redis"
)

var xxhashPool = sync.Pool{
	New: func() any {
		return xxhash.New()
	},
}

// KeyPrefix is the Redis namespace for all entries managed by this engine.
const KeyPrefix = "kc:llm"

// Validator reports whether a computed value should be cached.
// Return true to keep it (including legitimate empty results), false to drop
// it (e.g. an ERROR marker or a truncated payload). A nil Validator accepts
// everything.
type Validator[T any] func(v T) bool

// Event is emitted for every cache decision so callers can instrument hit-rate,
// compute cost, purge volume, etc.
type Event struct {
	Kind     string // "hit" | "miss" | "compute" | "skip_invalid" | "purge"
	TenantID string
	TaskType string
	Key      string
}

// MetricsHook receives cache events. It must be cheap and non-blocking.
type MetricsHook func(Event)

// Engine is a generic, multi-tenant LLM-result cache.
type Engine[T any] struct {
	validate  Validator[T]
	hook      MetricsHook
	redisCli  *redis.Client
	rawClient goredis.UniversalClient
}

// Option configures an Engine.
type Option[T any] func(*Engine[T])

// WithValidator sets the task-aware quality gate.
func WithValidator[T any](v Validator[T]) Option[T] {
	return func(e *Engine[T]) { e.validate = v }
}

// WithMetrics installs an event hook for observability.
func WithMetrics[T any](h MetricsHook) Option[T] {
	return func(e *Engine[T]) { e.hook = h }
}

// WithRedisClient overrides the Redis client instance (primarily for testing and custom wiring).
func WithRedisClient[T any](client *redis.Client) Option[T] {
	return func(e *Engine[T]) { e.redisCli = client }
}

// WithUniversalClient overrides the Redis client with any go-redis UniversalClient (primarily for testing and custom wiring).
func WithUniversalClient[T any](client goredis.UniversalClient) Option[T] {
	return func(e *Engine[T]) { e.rawClient = client }
}

// New constructs an Engine with the given options.
func New[T any](opts ...Option[T]) *Engine[T] {
	e := &Engine[T]{}
	for _, o := range opts {
		o(e)
	}
	return e
}

// BuildKey constructs the canonical, tenant-scoped cache key.
//
//	kc:llm:{tenant_id}:{task_type}:{xxhash64(keyParts...)}
//
// A NUL byte separates key parts to prevent cross-field hash collisions
// (e.g. ("ab","c") vs ("a","bc")). The engine never assumes anything about the
// business meaning of keyParts — that is the caller's responsibility.
func BuildKey(tenantID, taskType string, keyParts ...string) string {
	h := xxhashPool.Get().(*xxhash.Digest)
	for _, p := range keyParts {
		h.WriteString(p)
		h.WriteString("\x00")
	}
	key := fmt.Sprintf("%s:%s:%s:%x", KeyPrefix, tenantID, taskType, h.Sum64())
	h.Reset()
	xxhashPool.Put(h)
	return key
}

func (e *Engine[T]) getRedis() *redis.Client {
	if e.redisCli != nil {
		return e.redisCli
	}
	return redis.Get()
}

func (e *Engine[T]) getUniversalClient() goredis.UniversalClient {
	if e.rawClient != nil {
		return e.rawClient
	}
	if e.redisCli != nil {
		return e.redisCli.GetClient()
	}
	if rc := redis.Get(); rc != nil {
		return rc.GetClient()
	}
	return nil
}

func (e *Engine[T]) emit(ev Event) {
	if e.hook != nil {
		e.hook(ev)
	}
}

func (e *Engine[T]) get(ctx context.Context, key string) (T, bool) {
	var zero, dest T

	if e.rawClient != nil {
		data, err := e.rawClient.Get(ctx, key).Result()
		if err != nil {
			return zero, false
		}
		if err := json.Unmarshal([]byte(data), &dest); err != nil {
			return zero, false
		}
		return dest, true
	}
	client := e.getRedis()
	if client == nil {
		return zero, false
	}
	if !client.GetObj(ctx, key, &dest) {
		return zero, false
	}
	return dest, true
}

func (e *Engine[T]) set(ctx context.Context, tenantID, key string, v T) {
	if tenantID == "" {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	setCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()

	if e.rawClient != nil {
		data, err := json.Marshal(v)
		if err != nil {
			return
		}
		e.rawClient.Set(setCtx, key, data, 0)
		return
	}
	client := e.getRedis()
	if client == nil {
		return
	}
	client.SetObj(setCtx, key, v, 0)
}

// GetOrCompute returns the cached value for (tenantID, taskType, keyParts), or
// computes it via computeFn on a cache miss / invalid entry.
//
// Fail-Open (bypass): execute computeFn directly without caching when tenantID is empty to ensure business continuity without polluting global namespace.
func (e *Engine[T]) GetOrCompute(
	ctx context.Context,
	tenantID, taskType string,
	keyParts []string,
	computeFn func(ctx context.Context) (T, error),
) (T, error) {
	var zero T
	if tenantID == "" {
		return computeFn(ctx)
	}

	key := BuildKey(tenantID, taskType, keyParts...)

	if v, ok := e.get(ctx, key); ok {
		if e.validate == nil || e.validate(v) {
			e.emit(Event{Kind: "hit", TenantID: tenantID, TaskType: taskType, Key: key})
			return v, nil
		}
	}
	e.emit(Event{Kind: "miss", TenantID: tenantID, TaskType: taskType, Key: key})

	v, cerr := computeFn(ctx)
	if cerr != nil {
		return zero, cerr
	}
	if e.validate == nil || e.validate(v) {
		e.set(ctx, tenantID, key, v)
		e.emit(Event{Kind: "compute", TenantID: tenantID, TaskType: taskType, Key: key})
	} else {
		e.emit(Event{Kind: "skip_invalid", TenantID: tenantID, TaskType: taskType, Key: key})
		common.Warn("llmcache: dropping invalid computed value",
			zap.String("task_type", taskType), zap.String("tenant_id", tenantID))
	}
	return v, nil
}

// FlushTenant purges every cache entry for a tenant (GDPR erasure / tenant teardown).
// It scans for keys matching "kc:llm:{tenant_id}:*" using Redis SCAN cursor COUNT 1000,
// deletes them in pipeline batches, sleeps 10ms between batches, and responds promptly to ctx.Done().
// Returns the number of entries deleted and any error encountered.
func (e *Engine[T]) FlushTenant(ctx context.Context, tenantID string) (int, error) {
	if tenantID == "" {
		return 0, nil
	}
	rdb := e.getUniversalClient()
	if rdb == nil {
		return 0, nil
	}

	var cursor uint64
	var deletedCount int
	matchPattern := fmt.Sprintf("%s:%s:*", KeyPrefix, tenantID)

	timer := time.NewTimer(10 * time.Millisecond)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			common.Warn("llmcache: FlushTenant cancelled by context",
				zap.String("tenant_id", tenantID), zap.Error(ctx.Err()))
			return deletedCount, ctx.Err()
		default:
		}

		keys, nextCursor, err := rdb.Scan(ctx, cursor, matchPattern, 1000).Result()
		if err != nil {
			common.Warn("llmcache: FlushTenant SCAN failed",
				zap.String("tenant_id", tenantID), zap.Error(err))
			return deletedCount, err
		}

		if len(keys) > 0 {
			pipe := rdb.Pipeline()
			for _, k := range keys {
				pipe.Del(ctx, k)
			}
			cmders, err := pipe.Exec(ctx)
			for _, cmder := range cmders {
				if intCmd, ok := cmder.(*goredis.IntCmd); ok {
					deletedCount += int(intCmd.Val())
				}
			}
			if err != nil && !errors.Is(err, goredis.Nil) {
				common.Warn("llmcache: FlushTenant pipeline Del failed",
					zap.String("tenant_id", tenantID), zap.Error(err))
				return deletedCount, err
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}

		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(10 * time.Millisecond)

		select {
		case <-ctx.Done():
			common.Warn("llmcache: FlushTenant cancelled by context",
				zap.String("tenant_id", tenantID), zap.Error(ctx.Err()))
			return deletedCount, ctx.Err()
		case <-timer.C:
		}
	}

	e.emit(Event{Kind: "purge", TenantID: tenantID, Key: matchPattern})
	return deletedCount, nil
}
