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
	Get(ctx context.Context, key string) (interface{}, bool)
	// Set stores a value in the cache.
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration)
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
}

type cacheEntry struct {
	value       interface{}
	expiration  time.Time
	lastAccess  time.Time
	hits        int64
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
func (c *MemoryCache) Get(ctx context.Context, key string) (interface{}, bool) {
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
	entry.lastAccess = time.Now()
	return entry.value, true
}

// Set stores a value in the cache.
func (c *MemoryCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) {
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

	c.data[key] = &cacheEntry{
		value:       value,
		expiration:  expiration,
		lastAccess:  time.Now(),
		hits:        0,
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
		// Find least recently used entry
		var oldest time.Time
		for k, v := range c.data {
			if oldest.IsZero() || v.lastAccess.Before(oldest) {
				oldest = v.lastAccess
				keyToDelete = k
			}
		}
	case EvictLFU:
		// Find least frequently used
		var minHits int64 = -1
		for k, v := range c.data {
			if minHits == -1 || v.hits < minHits {
				minHits = v.hits
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
func GenerateCacheKey(nodeName string, input interface{}) string {
	data, _ := json.Marshal(input)
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
func (e *CachedExecutor) Execute(ctx context.Context, nodeName string, input interface{}, fn func(context.Context, interface{}) (interface{}, error)) (interface{}, error) {
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
func (n *NoopCache) Get(ctx context.Context, key string) (interface{}, bool) {
	return nil, false
}

// Set is a no-op.
func (n *NoopCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) {}

// Delete is a no-op.
func (n *NoopCache) Delete(ctx context.Context, key string) {}

// Clear is a no-op.
func (n *NoopCache) Clear() {}
