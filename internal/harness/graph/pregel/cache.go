// Package pregel provides caching support for Pregel execution.
package pregel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"ragflow/internal/harness/graph/types"
)

// Cache is the interface for caching node outputs.
type Cache interface {
	// Get retrieves a value from the cache.
	Get(ctx context.Context, key string) (any, bool)
	// Set stores a value in the cache.
	Set(ctx context.Context, key string, value any, ttl time.Duration)
	// Delete removes a value from the cache.
	Delete(ctx context.Context, key string)
	// Clear clears all values from the cache.
	Clear()
}

// MemoryCache is an in-memory cache implementation.
type MemoryCache struct {
	mu       sync.RWMutex
	data     map[string]*cacheEntry
	maxSize  int
	eviction EvictionPolicy
	// clock is a logical clock bumped on every Get/Set under c.mu. Recency is
	// ordered by this sequence instead of wall-clock timestamps: two
	// operations within one clock tick (guaranteed on Windows, possible on
	// any host with a coarse monotonic clock) would otherwise record equal
	// lastAccess values and let evict() pick an arbitrary entry instead of
	// the least recently used one. Values start at 1 so 0 stays a sentinel.
	clock uint64
}

type cacheEntry struct {
	value      any
	expiration time.Time
	lastAccess uint64
	hits       int64
}

// EvictionPolicy determines how entries are evicted when cache is full.
type EvictionPolicy int

const (
	// EvictLRU evicts least recently used entries.
	EvictLRU EvictionPolicy = iota
	// EvictLFU evicts least frequently used entries.
	EvictLFU
	// EvictRandom evicts random entries.
	EvictRandom
)

// NewMemoryCache creates a new in-memory cache.
func NewMemoryCache(maxSize int, eviction EvictionPolicy) *MemoryCache {
	return &MemoryCache{
		data:     make(map[string]*cacheEntry),
		maxSize:  maxSize,
		eviction: eviction,
	}
}

// Get retrieves a value from the cache.
func (c *MemoryCache) Get(ctx context.Context, key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.data[key]
	if !ok {
		return nil, false
	}

	// Check expiration
	if !entry.expiration.IsZero() && time.Now().After(entry.expiration) {
		return nil, false
	}

	entry.hits++
	c.clock++
	entry.lastAccess = c.clock
	return entry.value, true
}

// Set stores a value in the cache.
func (c *MemoryCache) Set(ctx context.Context, key string, value any, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict entries if cache is full
	if len(c.data) >= c.maxSize {
		c.evict()
	}

	expiration := time.Time{}
	if ttl > 0 {
		expiration = time.Now().Add(ttl)
	}

	c.clock++
	c.data[key] = &cacheEntry{
		value:      value,
		expiration: expiration,
		lastAccess: c.clock,
		hits:       0,
	}
}

// Delete removes a value from the cache.
func (c *MemoryCache) Delete(ctx context.Context, key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
}

// Clear clears all values from the cache.
func (c *MemoryCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = make(map[string]*cacheEntry)
}

// evict removes an entry based on the eviction policy.
func (c *MemoryCache) evict() {
	if len(c.data) == 0 {
		return
	}

	var keyToDelete string

	switch c.eviction {
	case EvictLRU:
		// Find least recently used entry. lastAccess comes from the logical
		// clock, so the values are unique and strictly ordered; comparing
		// wall-clock timestamps would leave ties to map iteration order and
		// evict an arbitrary entry.
		var oldest uint64
		for k, v := range c.data {
			if oldest == 0 || v.lastAccess < oldest {
				oldest = v.lastAccess
				keyToDelete = k
			}
		}
	case EvictLFU:
		// Find least frequently used, breaking hit ties by least recent use.
		var minHits int64 = -1
		var oldest uint64
		for k, v := range c.data {
			if minHits == -1 || v.hits < minHits || (v.hits == minHits && v.lastAccess < oldest) {
				minHits = v.hits
				oldest = v.lastAccess
				keyToDelete = k
			}
		}
	case EvictRandom:
		// Delete first entry (Go map iteration is randomized)
		for k := range c.data {
			keyToDelete = k
			break
		}
	}

	if keyToDelete != "" {
		delete(c.data, keyToDelete)
	}
}

// GenerateCacheKey generates a cache key from the given input.
func GenerateCacheKey(nodeName string, input any) string {
	data, err := json.Marshal(input)
	if err != nil {
		// Fall back to the type name so different inputs still produce different keys.
		return fmt.Sprintf("%s:%T", nodeName, input)
	}
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%s:%s", nodeName, hex.EncodeToString(hash[:]))
}

// CachedExecutor wraps a function with caching.
type CachedExecutor struct {
	cache       Cache
	cachePolicy *types.CachePolicy
}

// NewCachedExecutor creates a new cached executor.
func NewCachedExecutor(cache Cache, policy *types.CachePolicy) *CachedExecutor {
	return &CachedExecutor{
		cache:       cache,
		cachePolicy: policy,
	}
}

// Execute executes a function with caching.
func (e *CachedExecutor) Execute(ctx context.Context, nodeName string, input any, fn func(context.Context, any) (any, error)) (any, error) {
	// Generate cache key
	var key string
	if e.cachePolicy != nil && e.cachePolicy.KeyFunc != nil {
		key = e.cachePolicy.KeyFunc(ctx, input)
	} else {
		key = GenerateCacheKey(nodeName, input)
	}

	// Check cache
	if cached, ok := e.cache.Get(ctx, key); ok {
		return cached, nil
	}

	// Execute function
	result, err := fn(ctx, input)
	if err != nil {
		return nil, err
	}

	// Cache result
	var ttl time.Duration
	if e.cachePolicy != nil && e.cachePolicy.TTL != nil {
		ttl = *e.cachePolicy.TTL
	}
	e.cache.Set(ctx, key, result, ttl)

	return result, nil
}

// NoopCache is a cache that doesn't store anything.
type NoopCache struct{}

// Get always returns false.
func (n *NoopCache) Get(ctx context.Context, key string) (any, bool) {
	return nil, false
}

// Set is a no-op.
func (n *NoopCache) Set(ctx context.Context, key string, value any, ttl time.Duration) {}

// Delete is a no-op.
func (n *NoopCache) Delete(ctx context.Context, key string) {}

// Clear is a no-op.
func (n *NoopCache) Clear() {}
