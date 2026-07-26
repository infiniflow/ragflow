//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package inference

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"time"

	"ragflow/internal/deepdoc/parser/pdf/util"
	doctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/engine/redis"
)

// DefaultCacheTTL is the cache entry lifetime applied when the
// caller does not supply one. 1 hour matches the python
// deepdoc cache policy.
const DefaultCacheTTL = time.Hour

// cacheKeyPrefix namespaces DocAnalyzer cache entries so they
// do not collide with other Redis users in the same database.
const cacheKeyPrefix = "ddoc:cache:"

// cacheStore abstracts the Redis-side persistence so the
// wrapper can be exercised against an in-memory fake in unit
// tests. The interface intentionally mirrors only the
// operations the cache needs; Redis features beyond this stay
// out of the wrapping path.
type cacheStore interface {
	Enabled() bool
	GetObj(key string, dest any) bool
	SetObj(key string, value any, ttl time.Duration) bool
}

// redisCacheStore is the production store, backed by the
// package-level Redis singleton. When Redis is unconfigured
// the package-level methods become no-ops (return false / treat
// as miss), which is exactly the behaviour we want.
type redisCacheStore struct{}

// Enabled forwards to redis.IsEnabled().
func (redisCacheStore) Enabled() bool { return redis.IsEnabled() }

// GetObj forwards to the package-level Redis singleton.
// Errors are logged inside the engine; we treat any failure
// (including connectivity) as a cache miss for safety.
func (redisCacheStore) GetObj(key string, dest any) bool {
	return redis.Get().GetObj(key, dest)
}

// SetObj forwards to the package-level Redis singleton.
func (redisCacheStore) SetObj(key string, value any, ttl time.Duration) bool {
	return redis.Get().SetObj(key, value, ttl)
}

// DocAnalyzerCache wraps an inner doctype.DocAnalyzer and
// transparently caches its 4 image-keyed methods in Redis.
//
// Behaviour:
//
//   - If the store is disabled, the wrapper passes every call
//     straight through to the inner analyser. Tests and
//     developer machines without Redis configured see no
//     behavioural change.
//   - On cache hit the inner call is skipped entirely.
//   - On cache miss the inner call runs; its successful result
//     is JSON-serialised and stored under the deterministic
//     sha256-based key with the configured TTL.
//   - Health() is a pure passthrough; caching it would not be
//     semantically meaningful.
//
// The wrapper is safe for concurrent use: the cache lookup
// path uses only the store's own synchronisation, so callers
// can route per-image OCR through OCRRecognize from multiple
// page workers without additional locking.
type DocAnalyzerCache struct {
	inner doctype.DocAnalyzer
	store cacheStore
	ttl   time.Duration
}

// NewDocAnalyzerCache wraps inner with a Redis-backed cache
// using the supplied TTL (use DefaultCacheTTL for 1h). When
// Redis is not configured, the wrapper transparently degrades
// to a pass-through.
func NewDocAnalyzerCache(inner doctype.DocAnalyzer, ttl time.Duration) *DocAnalyzerCache {
	if ttl <= 0 {
		ttl = DefaultCacheTTL
	}
	return &DocAnalyzerCache{
		inner: inner,
		store: redisCacheStore{},
		ttl:   ttl,
	}
}

// newDocAnalyzerCacheWithStore is the test seam. Production
// code uses NewDocAnalyzerCache; tests inject a fake store to
// drive the cache behaviour deterministically without
// depending on a running Redis.
func newDocAnalyzerCacheWithStore(inner doctype.DocAnalyzer, s cacheStore, ttl time.Duration) *DocAnalyzerCache {
	if ttl <= 0 {
		ttl = DefaultCacheTTL
	}
	return &DocAnalyzerCache{inner: inner, store: s, ttl: ttl}
}

// cacheKey returns the cache key used for the given method +
// image. It encodes the image to JPEG (matching what the
// inference HTTP client actually sends) and hashes the
// resulting bytes. JPEG encoding is deterministic for a given
// image and quality, so two DocAnalyzer calls with the same
// image content produce the same key.
func cacheKey(method string, img image.Image) (string, error) {
	if img == nil {
		return "", fmt.Errorf("%s: nil image", method)
	}
	data, err := util.EncodeJPEG(img)
	if err != nil {
		return "", fmt.Errorf("%s: encode: %w", method, err)
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%s%s:%s", cacheKeyPrefix, method, hex.EncodeToString(sum[:])), nil
}

// cacheKeyOrEmpty returns the cache key for img, or "" if the
// image should bypass caching (nil image, encode failure). The
// empty string is the "no key" sentinel — callers must skip
// the cache and call inner directly in that case.
func cacheKeyOrEmpty(method string, img image.Image) string {
	k, err := cacheKey(method, img)
	if err != nil {
		return ""
	}
	return k
}

// DLA see doctype.DocAnalyzer.
func (c *DocAnalyzerCache) DLA(ctx context.Context, img image.Image) ([]doctype.DLARegion, error) {
	if key := cacheKeyOrEmpty("dla", img); key != "" && c.store.Enabled() {
		var cached []doctype.DLARegion
		if c.store.GetObj(key, &cached) {
			return cached, nil
		}
	}
	out, err := c.inner.DLA(ctx, img)
	if err != nil || img == nil {
		return out, err
	}
	if key := cacheKeyOrEmpty("dla", img); key != "" && c.store.Enabled() {
		c.store.SetObj(key, out, c.ttl)
	}
	return out, nil
}

// TSR see doctype.DocAnalyzer.
func (c *DocAnalyzerCache) TSR(ctx context.Context, img image.Image) ([]doctype.TSRCell, error) {
	if key := cacheKeyOrEmpty("tsr", img); key != "" && c.store.Enabled() {
		var cached []doctype.TSRCell
		if c.store.GetObj(key, &cached) {
			return cached, nil
		}
	}
	out, err := c.inner.TSR(ctx, img)
	if err != nil || img == nil {
		return out, err
	}
	if key := cacheKeyOrEmpty("tsr", img); key != "" && c.store.Enabled() {
		c.store.SetObj(key, out, c.ttl)
	}
	return out, nil
}

// OCRDetect see doctype.DocAnalyzer.
func (c *DocAnalyzerCache) OCRDetect(ctx context.Context, img image.Image) ([]doctype.OCRBox, error) {
	if key := cacheKeyOrEmpty("ocr_detect", img); key != "" && c.store.Enabled() {
		var cached []doctype.OCRBox
		if c.store.GetObj(key, &cached) {
			return cached, nil
		}
	}
	out, err := c.inner.OCRDetect(ctx, img)
	if err != nil || img == nil {
		return out, err
	}
	if key := cacheKeyOrEmpty("ocr_detect", img); key != "" && c.store.Enabled() {
		c.store.SetObj(key, out, c.ttl)
	}
	return out, nil
}

// OCRRecognize see doctype.DocAnalyzer.
func (c *DocAnalyzerCache) OCRRecognize(ctx context.Context, img image.Image) ([]doctype.OCRText, error) {
	if key := cacheKeyOrEmpty("ocr_recognize", img); key != "" && c.store.Enabled() {
		var cached []doctype.OCRText
		if c.store.GetObj(key, &cached) {
			return cached, nil
		}
	}
	out, err := c.inner.OCRRecognize(ctx, img)
	if err != nil || img == nil {
		return out, err
	}
	if key := cacheKeyOrEmpty("ocr_recognize", img); key != "" && c.store.Enabled() {
		c.store.SetObj(key, out, c.ttl)
	}
	return out, nil
}

// Health see doctype.DocAnalyzer. Health probes are
// deliberately uncached — caching the outcome would defeat the
// purpose of a health check.
func (c *DocAnalyzerCache) Health() bool {
	if c == nil || c.inner == nil {
		return false
	}
	return c.inner.Health()
}

// Compile-time guarantee that DocAnalyzerCache satisfies the
// doctype.DocAnalyzer interface.
var _ doctype.DocAnalyzer = (*DocAnalyzerCache)(nil)
