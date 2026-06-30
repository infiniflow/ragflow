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

package utility

import (
	"container/list"
	"sync"
)

// EmbeddingLRU is a thread-safe LRU cache for embeddings.
// The key is a combination of question and embedding ID.
type EmbeddingLRU struct {
	capacity int
	cache    map[string]*list.Element
	list     *list.List
	mu       sync.RWMutex
}

// entry holds the key and value in the LRU cache.
type entry struct {
	key   string
	value []float64
}

// NewEmbeddingLRU creates a new EmbeddingLRU with the given capacity.
func NewEmbeddingLRU(capacity int) *EmbeddingLRU {
	return &EmbeddingLRU{
		capacity: capacity,
		cache:    make(map[string]*list.Element),
		list:     list.New(),
	}
}

// buildKey creates a composite key from question and embedding ID.
func buildKey(question, embeddingID string) string {
	// Use a delimiter that is unlikely to appear in the strings.
	// If needed, a more robust key generation can be implemented.
	return question + "::" + embeddingID
}

// Get retrieves the embedding for the given question and embedding ID.
// Returns the embedding and true if found, otherwise nil and false.
func (lru *EmbeddingLRU) Get(question, embeddingID string) ([]float64, bool) {
	key := buildKey(question, embeddingID)
	lru.mu.RLock()
	defer lru.mu.RUnlock()

	if elem, ok := lru.cache[key]; ok {
		// Move to front (most recently used)
		lru.list.MoveToFront(elem)
		ent := elem.Value.(*entry)
		// Return a copy to prevent external modification of cached slice
		embedding := make([]float64, len(ent.value))
		copy(embedding, ent.value)
		return embedding, true
	}
	return nil, false
}

// Put stores an embedding for the given question and embedding ID.
// If the key already exists, its value is updated and moved to front.
// If the cache is at capacity, the least recently used item is evicted.
func (lru *EmbeddingLRU) Put(question, embeddingID string, embedding []float64) {
	key := buildKey(question, embeddingID)
	lru.mu.Lock()
	defer lru.mu.Unlock()

	// If key exists, update value and move to front
	if elem, ok := lru.cache[key]; ok {
		lru.list.MoveToFront(elem)
		ent := elem.Value.(*entry)
		// Replace the embedding slice
		ent.value = make([]float64, len(embedding))
		copy(ent.value, embedding)
		return
	}

	// Add new entry
	ent := &entry{key: key, value: make([]float64, len(embedding))}
	copy(ent.value, embedding)
	elem := lru.list.PushFront(ent)
	lru.cache[key] = elem

	// Evict if capacity exceeded
	if lru.list.Len() > lru.capacity {
		lru.evictOldest()
	}
}

// evictOldest removes the least recently used item from the cache.
// Must be called with lock held.
func (lru *EmbeddingLRU) evictOldest() {
	elem := lru.list.Back()
	if elem != nil {
		lru.list.Remove(elem)
		ent := elem.Value.(*entry)
		delete(lru.cache, ent.key)
	}
}

// Remove removes the embedding for the given question and embedding ID.
func (lru *EmbeddingLRU) Remove(question, embeddingID string) {
	key := buildKey(question, embeddingID)
	lru.mu.Lock()
	defer lru.mu.Unlock()

	if elem, ok := lru.cache[key]; ok {
		lru.list.Remove(elem)
		delete(lru.cache, key)
	}
}

// Clear removes all items from the cache.
func (lru *EmbeddingLRU) Clear() {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	lru.cache = make(map[string]*list.Element)
	lru.list.Init()
}

// Len returns the number of items in the cache.
func (lru *EmbeddingLRU) Len() int {
	lru.mu.RLock()
	defer lru.mu.RUnlock()
	return lru.list.Len()
}
