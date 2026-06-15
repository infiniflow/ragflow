package store

import (
	"context"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// InMemoryStore is an in-memory implementation of BaseStore.
// It is not thread-safe by default, use NewInMemoryStore() for a thread-safe version.
type InMemoryStore struct {
	mu           sync.RWMutex
	data         map[string]map[string]map[string]interface{}
	ttl          map[string]time.Time
	indexes      map[string]map[string][]float64 // namespaceKey -> key -> embedding vector
	indexConfigs map[string]IndexConfig          // namespaceKey -> index config
	closed       bool
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
}

// NewInMemoryStore creates a new thread-safe in-memory store.
// It starts a background goroutine for TTL cleanup every minute.
func NewInMemoryStore() *InMemoryStore {
	store := &InMemoryStore{
		data:         make(map[string]map[string]map[string]interface{}),
		ttl:          make(map[string]time.Time),
		indexes:      make(map[string]map[string][]float64),
		indexConfigs: make(map[string]IndexConfig),
		stopCleanup:  make(chan struct{}),
	}
	// Start TTL cleanup goroutine
	store.cleanupTicker = time.NewTicker(1 * time.Minute)
	go store.cleanupExpired()
	return store
}

// Get retrieves a value from the store.
func (s *InMemoryStore) Get(ctx context.Context, namespace []string, key string) (map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, &StoreError{Message: "store is closed"}
	}

	nsKey := s.nsKey(namespace)
	if nsData, ok := s.data[nsKey]; ok {
		// Check TTL
		if fullKey := s.fullKey(nsKey, key); !s.checkTTL(fullKey) {
			return nil, nil
		}

		if value, ok := nsData[key]; ok {
			return s.copyValue(value), nil
		}
	}

	return nil, nil
}

// Put stores a value in the store.
func (s *InMemoryStore) Put(ctx context.Context, namespace []string, key string, value map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return &StoreError{Message: "store is closed"}
	}

	nsKey := s.nsKey(namespace)
	if _, ok := s.data[nsKey]; !ok {
		s.data[nsKey] = make(map[string]map[string]interface{})
	}

	s.data[nsKey][key] = s.copyValue(value)

	return nil
}

// Delete removes a value from the store.
func (s *InMemoryStore) Delete(ctx context.Context, namespace []string, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return &StoreError{Message: "store is closed"}
	}

	nsKey := s.nsKey(namespace)
	if nsData, ok := s.data[nsKey]; ok {
		delete(nsData, key)
		fullKey := s.fullKey(nsKey, key)
		delete(s.ttl, fullKey)
	}

	return nil
}

// Search searches for values in the store.
func (s *InMemoryStore) Search(ctx context.Context, namespace []string, query string, limit int) ([]map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, &StoreError{Message: "store is closed"}
	}

	nsKey := s.nsKey(namespace)
	nsData, ok := s.data[nsKey]
	if !ok {
		return nil, nil
	}

	results := make([]map[string]interface{}, 0)
	for key, value := range nsData {
		fullKey := s.fullKey(nsKey, key)
		if !s.checkTTL(fullKey) {
			continue
		}

		// Simple query matching (can be extended)
		if query == "" || s.matchQuery(value, query) {
			results = append(results, s.copyValue(value))
			if limit > 0 && len(results) >= limit {
				break
			}
		}
	}

	return results, nil
}

// List lists all keys in the namespace.
func (s *InMemoryStore) List(ctx context.Context, namespace []string, limit int) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, &StoreError{Message: "store is closed"}
	}

	nsKey := s.nsKey(namespace)
	nsData, ok := s.data[nsKey]
	if !ok {
		return nil, nil
	}

	keys := make([]string, 0, len(nsData))
	for key := range nsData {
		fullKey := s.fullKey(nsKey, key)
		if s.checkTTL(fullKey) {
			keys = append(keys, key)
			if limit > 0 && len(keys) >= limit {
				break
			}
		}
	}

	return keys, nil
}

// Batch executes multiple operations atomically.
func (s *InMemoryStore) Batch(ctx context.Context, ops []Op) ([]Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, &StoreError{Message: "store is closed"}
	}

	results := make([]Result, len(ops))
	for i, op := range ops {
		switch o := op.(type) {
		case GetOp:
			nsKey := s.nsKey(o.Namespace)
			if nsData, ok := s.data[nsKey]; ok {
				fullKey := s.fullKey(nsKey, o.Key)
				if !s.checkTTL(fullKey) {
					results[i] = Result{Value: nil, Error: nil}
					continue
				}
				if value, ok := nsData[o.Key]; ok {
					results[i] = Result{Value: s.copyValue(value), Error: nil}
				} else {
					results[i] = Result{Value: nil, Error: nil}
				}
			} else {
				results[i] = Result{Value: nil, Error: nil}
			}
		case PutOp:
			nsKey := s.nsKey(o.Namespace)
			if _, ok := s.data[nsKey]; !ok {
				s.data[nsKey] = make(map[string]map[string]interface{})
			}
			s.data[nsKey][o.Key] = s.copyValue(o.Value)
			if o.TTL != nil {
				fullKey := s.fullKey(nsKey, o.Key)
				s.ttl[fullKey] = time.Now().Add(*o.TTL)
			}
			results[i] = Result{Value: nil, Error: nil}
		case SearchOp:
			// Simplified search for batch operation
			nsKey := s.nsKey(o.NamespacePrefix)
			nsData, ok := s.data[nsKey]
			if !ok {
				results[i] = Result{Value: nil, Error: nil}
				continue
			}
			searchResults := make([]map[string]interface{}, 0)
			for key, value := range nsData {
				fullKey := s.fullKey(nsKey, key)
				if !s.checkTTL(fullKey) {
					continue
				}
				if o.Query != nil && *o.Query != "" && !s.matchQuery(value, *o.Query) {
					continue
				}
				if o.Filter != nil && !s.matchFilter(value, o.Filter) {
					continue
				}
				searchResults = append(searchResults, s.copyValue(value))
				if o.Limit > 0 && len(searchResults) >= o.Limit {
					break
				}
			}
			// Apply offset
			if o.Offset > 0 && o.Offset < len(searchResults) {
				searchResults = searchResults[o.Offset:]
			}
			results[i] = Result{Value: searchResults, Error: nil}
		case ListNamespacesOp:
			// Implement ListNamespacesOp
			namespaces, err := s.ListNamespaces(ctx, o.MatchConditions, o.MaxDepth, o.Limit, o.Offset)
			if err != nil {
				results[i] = Result{Value: nil, Error: err}
			} else {
				results[i] = Result{Value: namespaces, Error: nil}
			}
		default:
			results[i] = Result{Value: nil, Error: &StoreError{Message: "unknown operation type"}}
		}
	}
	return results, nil
}

// BatchPut performs multiple put operations atomically (deprecated).
func (s *InMemoryStore) BatchPut(ctx context.Context, ops []PutOperation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return &StoreError{Message: "store is closed"}
	}

	for _, op := range ops {
		nsKey := s.nsKey(op.Namespace)
		if _, ok := s.data[nsKey]; !ok {
			s.data[nsKey] = make(map[string]map[string]interface{})
		}
		s.data[nsKey][op.Key] = s.copyValue(op.Value)
	}

	return nil
}

// SetTTL sets a time-to-live for a key.
func (s *InMemoryStore) SetTTL(ctx context.Context, namespace []string, key string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return &StoreError{Message: "store is closed"}
	}

	if ttl <= 0 {
		return nil
	}

	fullKey := s.fullKey(s.nsKey(namespace), key)
	s.ttl[fullKey] = time.Now().Add(ttl)

	return nil
}

// GetItem retrieves a value with metadata (created_at, updated_at).
func (s *InMemoryStore) GetItem(ctx context.Context, namespace []string, key string, refreshTTL *bool) (*Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, &StoreError{Message: "store is closed"}
	}

	nsKey := s.nsKey(namespace)
	if nsData, ok := s.data[nsKey]; ok {
		fullKey := s.fullKey(nsKey, key)
		if !s.checkTTL(fullKey) {
			return nil, nil
		}
		if value, ok := nsData[key]; ok {
			// In memory store doesn't track created_at/updated_at, use current time
			now := time.Now()
			item := &Item{
				Value:     s.copyValue(value),
				Key:       key,
				Namespace: namespace,
				CreatedAt: now,
				UpdatedAt: now,
			}
			if expiry, ok := s.ttl[fullKey]; ok {
				item.ExpiresAt = &expiry
			}
			return item, nil
		}
	}
	return nil, nil
}

// PutItem stores a value with TTL and indexing options.
func (s *InMemoryStore) PutItem(ctx context.Context, namespace []string, key string, value map[string]interface{},
	index interface{}, ttl *time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return &StoreError{Message: "store is closed"}
	}

	nsKey := s.nsKey(namespace)
	if _, ok := s.data[nsKey]; !ok {
		s.data[nsKey] = make(map[string]map[string]interface{})
	}
	s.data[nsKey][key] = s.copyValue(value)
	
	if ttl != nil {
		fullKey := s.fullKey(nsKey, key)
		s.ttl[fullKey] = time.Now().Add(*ttl)
	}
	
	// Handle indexing
	if index != nil {
		switch idx := index.(type) {
		case IndexConfig:
			// Store index config
			s.indexConfigs[nsKey] = idx
			// Extract embedding from value if possible
			if idx.Embed != nil && len(idx.Fields) > 0 {
				// Simplified: assume first field contains embedding vector
				for _, field := range idx.Fields {
					if emb, ok := value[field].([]float64); ok {
						if _, ok := s.indexes[nsKey]; !ok {
							s.indexes[nsKey] = make(map[string][]float64)
						}
						s.indexes[nsKey][key] = emb
						break
					}
				}
			}
		case []string:
			// Treat as list of fields to index (simplified)
			if len(idx) > 0 {
				config := IndexConfig{
					Fields: idx,
				}
				s.indexConfigs[nsKey] = config
			}
		case bool:
			if idx {
				// Enable indexing with default fields
				config := IndexConfig{
					Fields: []string{"embedding"},
				}
				s.indexConfigs[nsKey] = config
			}
		}
	}
	return nil
}

// SearchItems searches for items with advanced filtering and natural language query.
// Supports semantic search via embedding vectors in filter["$embedding"].
func (s *InMemoryStore) SearchItems(ctx context.Context, namespace []string, query *string, filter map[string]interface{},
	limit, offset int, refreshTTL *bool) ([]*SearchItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, &StoreError{Message: "store is closed"}
	}

	// Check for semantic search via embedding
	if filter != nil {
		if emb, ok := filter["$embedding"].([]float64); ok {
			// Perform semantic search
			nsKey := s.nsKey(namespace)
			results := s.searchByEmbedding(nsKey, emb, limit)
			// Apply offset
			if offset > 0 && offset < len(results) {
				results = results[offset:]
			}
			return results, nil
		}
	}

	nsKey := s.nsKey(namespace)
	nsData, ok := s.data[nsKey]
	if !ok {
		return nil, nil
	}

	results := make([]*SearchItem, 0)
	now := time.Now()
	for key, value := range nsData {
		fullKey := s.fullKey(nsKey, key)
		if !s.checkTTL(fullKey) {
			continue
		}
		if query != nil && *query != "" && !s.matchQuery(value, *query) {
			continue
		}
		if filter != nil && !s.matchFilter(value, filter) {
			continue
		}
		item := &Item{
			Value:     s.copyValue(value),
			Key:       key,
			Namespace: namespace,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if expiry, ok := s.ttl[fullKey]; ok {
			item.ExpiresAt = &expiry
		}
		searchItem := &SearchItem{
			Item:  item,
			Score: nil, // No scoring in this simple implementation
		}
		results = append(results, searchItem)
		if limit > 0 && len(results) >= limit {
			break
		}
	}
	// Apply offset
	if offset > 0 && offset < len(results) {
		results = results[offset:]
	}
	return results, nil
}

// ListNamespaces lists all namespaces matching given conditions.
func (s *InMemoryStore) ListNamespaces(ctx context.Context, conditions []MatchCondition, maxDepth *int,
	limit, offset int) ([][]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, &StoreError{Message: "store is closed"}
	}

	namespaceSet := make(map[string][][]string)
	for nsKey := range s.data {
		parts := strings.Split(nsKey, "|")
		// Build namespace hierarchy
		for i := 1; i <= len(parts); i++ {
			prefix := strings.Join(parts[:i], "|")
			if _, ok := namespaceSet[prefix]; !ok {
				namespaceSet[prefix] = [][]string{parts[:i]}
			}
		}
	}
	
	// Apply conditions
	filtered := make([][]string, 0)
	for _, nsParts := range namespaceSet {
		for _, ns := range nsParts {
			if s.matchNamespaceConditions(ns, conditions) {
				filtered = append(filtered, ns)
			}
		}
	}
	
	// Apply maxDepth
	if maxDepth != nil {
		filtered2 := make([][]string, 0)
		for _, ns := range filtered {
			if len(ns) <= *maxDepth {
				filtered2 = append(filtered2, ns)
			}
		}
		filtered = filtered2
	}
	
	// Apply offset and limit
	if offset > 0 && offset < len(filtered) {
		filtered = filtered[offset:]
	}
	if limit > 0 && limit < len(filtered) {
		filtered = filtered[:limit]
	}
	
	return filtered, nil
}

// matchNamespaceConditions checks if a namespace matches the given conditions.
func (s *InMemoryStore) matchNamespaceConditions(namespace []string, conditions []MatchCondition) bool {
	if len(conditions) == 0 {
		return true
	}
	for _, cond := range conditions {
		switch cond.MatchType {
		case "prefix":
			if len(namespace) < len(cond.Path) {
				return false
			}
			for i, part := range cond.Path {
				if namespace[i] != part {
					return false
				}
			}
			return true
		case "suffix":
			if len(namespace) < len(cond.Path) {
				return false
			}
			for i, part := range cond.Path {
				if namespace[len(namespace)-len(cond.Path)+i] != part {
					return false
				}
			}
			return true
		}
	}
	return false
}

// Clear clears all data from the store.
func (s *InMemoryStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data = make(map[string]map[string]map[string]interface{})
	s.ttl = make(map[string]time.Time)

	return nil
}

// Close closes the store and stops the TTL cleanup goroutine.
func (s *InMemoryStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true
	if s.cleanupTicker != nil {
		s.cleanupTicker.Stop()
	}
	close(s.stopCleanup)
	return nil
}

// Helper methods

func (s *InMemoryStore) nsKey(namespace []string) string {
	return strings.Join(namespace, "|")
}

func (s *InMemoryStore) fullKey(nsKey, key string) string {
	return nsKey + ":" + key
}

func (s *InMemoryStore) checkTTL(fullKey string) bool {
	if expiry, ok := s.ttl[fullKey]; ok {
		return time.Now().Before(expiry)
	}
	return true
}

func (s *InMemoryStore) copyValue(value map[string]interface{}) map[string]interface{} {
	copied := make(map[string]interface{}, len(value))
	for k, v := range value {
		copied[k] = v
	}
	return copied
}

func (s *InMemoryStore) matchQuery(value map[string]interface{}, query string) bool {
	// Simple substring matching across all values
	// Can be extended to support more complex queries
	for _, v := range value {
		if str, ok := v.(string); ok {
			if strings.Contains(strings.ToLower(str), strings.ToLower(query)) {
				return true
			}
		}
	}
	return false
}

// matchFilter checks if a value matches the filter conditions.
// Supports comparison operators: $eq, $ne, $gt, $gte, $lt, $lte, $in, $nin, $regex
func (s *InMemoryStore) matchFilter(value map[string]interface{}, filter map[string]interface{}) bool {
	for field, condition := range filter {
		fieldValue, exists := value[field]
		if !exists {
			return false
		}
		
		// Handle nested operators
		if condMap, ok := condition.(map[string]interface{}); ok {
			for op, opValue := range condMap {
				if !s.compare(fieldValue, op, opValue) {
					return false
				}
			}
		} else {
			// Direct equality
			if !s.compare(fieldValue, "$eq", condition) {
				return false
			}
		}
	}
	return true
}

// compare performs a comparison based on the operator.
func (s *InMemoryStore) compare(fieldValue interface{}, operator string, opValue interface{}) bool {
	switch operator {
	case "$eq":
		return s.equal(fieldValue, opValue)
	case "$ne":
		return !s.equal(fieldValue, opValue)
	case "$gt":
		return s.greaterThan(fieldValue, opValue)
	case "$gte":
		return s.greaterThan(fieldValue, opValue) || s.equal(fieldValue, opValue)
	case "$lt":
		return s.lessThan(fieldValue, opValue)
	case "$lte":
		return s.lessThan(fieldValue, opValue) || s.equal(fieldValue, opValue)
	case "$in":
		return s.inArray(fieldValue, opValue)
	case "$nin":
		return !s.inArray(fieldValue, opValue)
	case "$regex":
		return s.regexMatch(fieldValue, opValue)
	default:
		return false
	}
}

// equal checks if two values are equal.
func (s *InMemoryStore) equal(a, b interface{}) bool {
	return a == b
}

// greaterThan checks if a > b (supports numeric types).
func (s *InMemoryStore) greaterThan(a, b interface{}) bool {
	switch av := a.(type) {
	case int:
		if bv, ok := b.(int); ok {
			return av > bv
		}
	case int64:
		if bv, ok := b.(int64); ok {
			return av > bv
		}
	case float64:
		if bv, ok := b.(float64); ok {
			return av > bv
		}
	case string:
		if bv, ok := b.(string); ok {
			return av > bv
		}
	}
	return false
}

// lessThan checks if a < b (supports numeric types).
func (s *InMemoryStore) lessThan(a, b interface{}) bool {
	switch av := a.(type) {
	case int:
		if bv, ok := b.(int); ok {
			return av < bv
		}
	case int64:
		if bv, ok := b.(int64); ok {
			return av < bv
		}
	case float64:
		if bv, ok := b.(float64); ok {
			return av < bv
		}
	case string:
		if bv, ok := b.(string); ok {
			return av < bv
		}
	}
	return false
}

// inArray checks if a value is in an array.
func (s *InMemoryStore) inArray(value interface{}, array interface{}) bool {
	arr, ok := array.([]interface{})
	if !ok {
		return false
	}
	for _, v := range arr {
		if s.equal(value, v) {
			return true
		}
	}
	return false
}

// regexMatch checks if a string matches a regex pattern.
func (s *InMemoryStore) regexMatch(value interface{}, pattern interface{}) bool {
	str, ok := value.(string)
	if !ok {
		return false
	}
	patternStr, ok := pattern.(string)
	if !ok {
		return false
	}
	// Compile regex pattern
	re, err := regexp.Compile(patternStr)
	if err != nil {
		// If pattern is invalid, treat as no match
		return false
	}
	return re.MatchString(str)
}

// StoreError represents a store error.
type StoreError struct {
	Message string
	Code    string
}

func (e *StoreError) Error() string {
	if e.Code != "" {
		return e.Code + ": " + e.Message
	}
	return e.Message
}

// cleanupExpired periodically removes expired TTL entries.
func (s *InMemoryStore) cleanupExpired() {
	for {
		select {
		case <-s.cleanupTicker.C:
			s.mu.Lock()
			now := time.Now()
			for fullKey, expiry := range s.ttl {
				if now.After(expiry) {
					// Parse fullKey to get namespace and key
					parts := strings.Split(fullKey, ":")
					if len(parts) == 2 {
						nsKey, key := parts[0], parts[1]
						if nsData, ok := s.data[nsKey]; ok {
							delete(nsData, key)
							if len(nsData) == 0 {
								delete(s.data, nsKey)
							}
						}
					}
					delete(s.ttl, fullKey)
					// Also clean up indexes
					for nsKey := range s.indexes {
						if idxMap, ok := s.indexes[nsKey]; ok {
							delete(idxMap, fullKey)
							if len(idxMap) == 0 {
								delete(s.indexes, nsKey)
							}
						}
					}
				}
			}
			s.mu.Unlock()
		case <-s.stopCleanup:
			return
		}
	}
}

// cosineSimilarity calculates cosine similarity between two vectors.
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}
	var dot, normA, normB float64
	for i := 0; i < len(a); i++ {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0.0
	}
	return dot / (sqrt(normA) * sqrt(normB))
}

// sqrt is a simple square root implementation using math.Sqrt.
func sqrt(x float64) float64 {
	return math.Sqrt(x)
}

// searchByEmbedding performs semantic search using embedding vectors.
func (s *InMemoryStore) searchByEmbedding(nsKey string, queryEmbedding []float64, limit int) []*SearchItem {
	if idxMap, ok := s.indexes[nsKey]; ok {
		type scoredItem struct {
			item  *SearchItem
			score float64
		}
		scored := make([]scoredItem, 0, len(idxMap))
		for key, embedding := range idxMap {
			score := cosineSimilarity(queryEmbedding, embedding)
			// Get the corresponding item
			if nsData, ok := s.data[nsKey]; ok {
				if value, ok := nsData[key]; ok {
					fullKey := s.fullKey(nsKey, key)
					if !s.checkTTL(fullKey) {
						continue
					}
					now := time.Now()
					item := &Item{
						Value:     s.copyValue(value),
						Key:       key,
						Namespace: strings.Split(nsKey, "|"),
						CreatedAt: now,
						UpdatedAt: now,
					}
					if expiry, ok := s.ttl[fullKey]; ok {
						item.ExpiresAt = &expiry
					}
					searchItem := &SearchItem{
						Item:  item,
						Score: &score,
					}
					scored = append(scored, scoredItem{searchItem, score})
				}
			}
		}
		// Sort by score descending
		sort.Slice(scored, func(i, j int) bool {
			return scored[i].score > scored[j].score
		})
		// Apply limit
		if limit > 0 && limit < len(scored) {
			scored = scored[:limit]
		}
		// Extract search items
		result := make([]*SearchItem, len(scored))
		for i, s := range scored {
			result[i] = s.item
		}
		return result
	}
	return nil
}
