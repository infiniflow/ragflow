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

// Package runtime implements per-tenant runtime selection for the
// agent canvas port.
//
// Two pieces live in this package:
//
//   - Selector (this file): reads/writes the per-tenant runtime
//     override in Redis. The default is RuntimeGo; per-tenant
//     overrides still let operators force a tenant back to Python
//     during the agent_api.py deprecation window.
//   - Metrics (metrics.go): Prometheus counter + histogram for
//     per-run observation, keyed by runtime mode.
package runtime

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RuntimeMode identifies which agent-canvas runtime implementation
// serves a given tenant. Supports "go" and "python"; "auto" is
// reserved for future adaptive policies.
type RuntimeMode string

const (
	// RuntimeGo routes the tenant to the Go-side eino
	// implementation. This is the process-wide default.
	RuntimeGo RuntimeMode = "go"
	// RuntimePython routes the tenant to the legacy Python
	// agent_api.py implementation. Retained for the 1-release
	// deprecation window; per-tenant overrides via Selector.Set
	// can still force a tenant to this mode.
	RuntimePython RuntimeMode = "python"
	// RuntimeAuto defers to the per-tenant override, then to the
	// process-wide Default(). It exists as a sentinel for clients that
	// want explicit "I don't care, pick for me" semantics.
	RuntimeAuto RuntimeMode = "auto"
)

// defaultEnvKey is the environment variable consulted by Default() when no
// override is registered for a tenant.
const defaultEnvKey = "RAGFLOW_CANVAS_DEFAULT_RUNTIME"

// overrideKeyPrefix is the Redis key namespace for per-tenant runtime
// overrides. Final keys look like "tenant_canvas_runtime:<tenantID>".
const overrideKeyPrefix = "tenant_canvas_runtime:"

var (
	defaultOnce sync.Once
	defaultMode RuntimeMode
)

// Default returns the process-wide default runtime mode.
//
// The default is Go. The per-tenant override (via Selector.Set)
// can still force a tenant back to Python for the 1-release
// deprecation window of agent_api.py.
//
// The value is read once from the RAGFLOW_CANVAS_DEFAULT_RUNTIME env var;
// subsequent calls return the cached result. Unknown env values fall back
// to RuntimeGo (the new default) so a misconfig still lands on the Go path.
func Default() RuntimeMode {
	defaultOnce.Do(func() {
		raw := os.Getenv(defaultEnvKey)
		switch RuntimeMode(raw) {
		case RuntimeGo, RuntimePython, RuntimeAuto:
			defaultMode = RuntimeMode(raw)
		default:
			defaultMode = RuntimeGo
		}
	})
	return defaultMode
}

// ResetDefaultCache clears the cached default-mode value. Test-only helper.
func ResetDefaultCache() {
	defaultOnce = sync.Once{}
	defaultMode = ""
}

// Selector resolves the runtime mode for a tenant at request time. It is
// safe for concurrent use.
type Selector struct {
	redis  *redis.Client
	logger *zap.Logger
}

// NewSelector constructs a Selector backed by the supplied Redis client. A
// nil logger is replaced with zap.NewNop() so callers in tests can omit it.
func NewSelector(rdb *redis.Client, logger *zap.Logger) *Selector {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Selector{redis: rdb, logger: logger}
}

// overrideKey returns the Redis key for a tenant's runtime override.
func overrideKey(tenantID string) string {
	return overrideKeyPrefix + tenantID
}

// Select returns the runtime mode registered for tenantID. The lookup
// order is:
//
//  1. The Redis key "tenant_canvas_runtime:<tenantID>" if present.
//  2. The process-wide Default() (env RAGFLOW_CANVAS_DEFAULT_RUNTIME,
//     falling back to RuntimeGo).
//
// A nil Redis client short-circuits to the default and never errors.
func (s *Selector) Select(ctx context.Context, tenantID string) (RuntimeMode, error) {
	if s == nil || s.redis == nil {
		return Default(), nil
	}
	raw, err := s.redis.Get(ctx, overrideKey(tenantID)).Result()
	if err == redis.Nil {
		return Default(), nil
	}
	if err != nil {
		s.logger.Warn("runtime selector: redis get failed, falling back to default",
			zap.String("tenant_id", tenantID), zap.Error(err))
		return Default(), err
	}
	mode := RuntimeMode(raw)
	switch mode {
	case RuntimeGo, RuntimePython, RuntimeAuto:
		return mode, nil
	default:
		s.logger.Warn("runtime selector: unrecognized value, falling back to default",
			zap.String("tenant_id", tenantID), zap.String("value", raw))
		return Default(), fmt.Errorf("unrecognized runtime mode %q for tenant %q", raw, tenantID)
	}
}

// Set overrides the runtime mode for a tenant. The override has no TTL
// (it is permanent until explicitly changed) so the operator does not have
// to remember to re-set it after a Redis flush of short-lived keys. Used
// by the admin runtime endpoint and tests.
func (s *Selector) Set(ctx context.Context, tenantID string, mode RuntimeMode) error {
	if s == nil || s.redis == nil {
		return fmt.Errorf("runtime selector: no redis client configured")
	}
	switch mode {
	case RuntimeGo, RuntimePython, RuntimeAuto:
	default:
		return fmt.Errorf("runtime selector: refusing to set invalid mode %q", mode)
	}
	return s.redis.Set(ctx, overrideKey(tenantID), string(mode), 0).Err()
}
