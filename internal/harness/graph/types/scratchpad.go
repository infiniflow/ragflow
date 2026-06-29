package types

import (
	"sync"
	"time"
)

// PregelScratchpad provides temporary data storage for Pregel execution.
// It stores data that is needed across nodes but not persisted to checkpoints.
type PregelScratchpad struct {
	mu               sync.RWMutex
	data             map[string]interface{}
	counters         map[string]int64
	metadata         map[string]interface{}
	createdAt        time.Time
	lastAccess       time.Time
	step             int64 // current step number
	stop             bool  // stop flag
	callCounter      int64 // number of calls
	interruptCounter int64 // number of interrupts
	resume           bool  // resume flag
	subgraphCounter  int64 // subgraph invocation count
}

// NewPregelScratchpad creates a new scratchpad.
func NewPregelScratchpad() *PregelScratchpad {
	now := time.Now()
	return &PregelScratchpad{
		data:             make(map[string]interface{}),
		counters:         make(map[string]int64),
		metadata:         make(map[string]interface{}),
		createdAt:        now,
		lastAccess:       now,
		step:             0,
		stop:             false,
		callCounter:      0,
		interruptCounter: 0,
		resume:           false,
		subgraphCounter:  0,
	}
}

// Get retrieves a value from the scratchpad.
func (p *PregelScratchpad) Get(key string) (interface{}, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.lastAccess = time.Now()
	value, ok := p.data[key]
	return value, ok
}

// Set stores a value in the scratchpad.
func (p *PregelScratchpad) Set(key string, value interface{}) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.data[key] = value
	p.lastAccess = time.Now()
}

// Delete removes a value from the scratchpad.
func (p *PregelScratchpad) Delete(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.data, key)
	p.lastAccess = time.Now()
}

// Has checks if a key exists in the scratchpad.
func (p *PregelScratchpad) Has(key string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	_, ok := p.data[key]
	return ok
}

// GetAll returns all data from the scratchpad.
func (p *PregelScratchpad) GetAll() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()

	copied := make(map[string]interface{}, len(p.data))
	for k, v := range p.data {
		copied[k] = v
	}
	return copied
}

// Clear clears all data from the scratchpad.
func (p *PregelScratchpad) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.data = make(map[string]interface{})
	p.counters = make(map[string]int64)
	p.lastAccess = time.Now()
}

// Counter operations

// IncrementCounter increments a counter and returns the new value.
func (p *PregelScratchpad) IncrementCounter(key string) int64 {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.counters[key]++
	p.lastAccess = time.Now()
	return p.counters[key]
}

// DecrementCounter decrements a counter and returns the new value.
func (p *PregelScratchpad) DecrementCounter(key string) int64 {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.counters[key]--
	p.lastAccess = time.Now()
	return p.counters[key]
}

// GetCounter returns the current value of a counter.
func (p *PregelScratchpad) GetCounter(key string) int64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.counters[key]
}

// SetCounter sets a counter to a specific value.
func (p *PregelScratchpad) SetCounter(key string, value int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.counters[key] = value
	p.lastAccess = time.Now()
}

// ResetCounter resets a counter to zero.
func (p *PregelScratchpad) ResetCounter(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.counters, key)
	p.lastAccess = time.Now()
}

// ListCounters returns all counters.
func (p *PregelScratchpad) ListCounters() map[string]int64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	copied := make(map[string]int64, len(p.counters))
	for k, v := range p.counters {
		copied[k] = v
	}
	return copied
}

// Metadata operations

// SetMetadata sets a metadata value.
func (p *PregelScratchpad) SetMetadata(key string, value interface{}) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.metadata[key] = value
	p.lastAccess = time.Now()
}

// GetMetadata retrieves a metadata value.
func (p *PregelScratchpad) GetMetadata(key string) (interface{}, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	value, ok := p.metadata[key]
	return value, ok
}

// GetAllMetadata returns all metadata.
func (p *PregelScratchpad) GetAllMetadata() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()

	copied := make(map[string]interface{}, len(p.metadata))
	for k, v := range p.metadata {
		copied[k] = v
	}
	return copied
}

// Stack operations (for tracking execution context)

// Stack represents a simple stack for values.
type Stack struct {
	items []interface{}
}

// NewStack creates a new stack.
func NewStack() *Stack {
	return &Stack{
		items: make([]interface{}, 0),
	}
}

// Push pushes a value onto the stack.
func (s *Stack) Push(value interface{}) {
	s.items = append(s.items, value)
}

// Pop pops a value from the stack.
func (s *Stack) Pop() (interface{}, bool) {
	if len(s.items) == 0 {
		return nil, false
	}
	index := len(s.items) - 1
	value := s.items[index]
	s.items = s.items[:index]
	return value, true
}

// Peek returns the top value without removing it.
func (s *Stack) Peek() (interface{}, bool) {
	if len(s.items) == 0 {
		return nil, false
	}
	return s.items[len(s.items)-1], true
}

// Size returns the size of the stack.
func (s *Stack) Size() int {
	return len(s.items)
}

// IsEmpty returns true if the stack is empty.
func (s *Stack) IsEmpty() bool {
	return len(s.items) == 0
}

// Clear clears the stack.
func (s *Stack) Clear() {
	s.items = make([]interface{}, 0)
}

// ToSlice returns the stack as a slice.
func (s *Stack) ToSlice() []interface{} {
	copied := make([]interface{}, len(s.items))
	copy(copied, s.items)
	return copied
}

// ScratchpadStack is a stack stored in the scratchpad.
type ScratchpadStack struct {
	scratchpad *PregelScratchpad
	key        string
}

// NewScratchpadStack creates a new stack backed by the scratchpad.
func NewScratchpadStack(scratchpad *PregelScratchpad, key string) *ScratchpadStack {
	return &ScratchpadStack{
		scratchpad: scratchpad,
		key:        key,
	}
}

// Push pushes a value onto the stack.
func (s *ScratchpadStack) Push(value interface{}) {
	var stack *Stack
	if val, ok := s.scratchpad.Get(s.key); ok {
		if st, ok := val.(*Stack); ok {
			stack = st
		}
	}
	if stack == nil {
		stack = NewStack()
	}
	stack.Push(value)
	s.scratchpad.Set(s.key, stack)
}

// Pop pops a value from the stack.
func (s *ScratchpadStack) Pop() (interface{}, bool) {
	val, ok := s.scratchpad.Get(s.key)
	if !ok {
		return nil, false
	}
	stack, ok := val.(*Stack)
	if !ok {
		return nil, false
	}
	value, ok := stack.Pop()
	if stack.IsEmpty() {
		s.scratchpad.Delete(s.key)
	} else {
		s.scratchpad.Set(s.key, stack)
	}
	return value, ok
}

// Peek returns the top value without removing it.
func (s *ScratchpadStack) Peek() (interface{}, bool) {
	val, ok := s.scratchpad.Get(s.key)
	if !ok {
		return nil, false
	}
	stack, ok := val.(*Stack)
	if !ok {
		return nil, false
	}
	return stack.Peek()
}

// Size returns the size of the stack.
func (s *ScratchpadStack) Size() int {
	val, ok := s.scratchpad.Get(s.key)
	if !ok {
		return 0
	}
	stack, ok := val.(*Stack)
	if !ok {
		return 0
	}
	return stack.Size()
}

// IsEmpty returns true if the stack is empty.
func (s *ScratchpadStack) IsEmpty() bool {
	return s.Size() == 0
}

// Clear clears the stack.
func (s *ScratchpadStack) Clear() {
	s.scratchpad.Delete(s.key)
}

// Step returns the current step number.
func (p *PregelScratchpad) Step() int64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.step
}

// SetStep sets the current step number.
func (p *PregelScratchpad) SetStep(step int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.step = step
	p.lastAccess = time.Now()
}

// Stop returns the stop flag.
func (p *PregelScratchpad) Stop() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.stop
}

// SetStop sets the stop flag.
func (p *PregelScratchpad) SetStop(stop bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stop = stop
	p.lastAccess = time.Now()
}

// CallCounter returns the number of calls.
func (p *PregelScratchpad) CallCounter() int64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.callCounter
}

// IncrementCallCounter increments the call counter.
func (p *PregelScratchpad) IncrementCallCounter() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.callCounter++
	p.lastAccess = time.Now()
	return p.callCounter
}

// InterruptCounter returns the number of interrupts.
func (p *PregelScratchpad) InterruptCounter() int64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.interruptCounter
}

// IncrementInterruptCounter increments the interrupt counter.
func (p *PregelScratchpad) IncrementInterruptCounter() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.interruptCounter++
	p.lastAccess = time.Now()
	return p.interruptCounter
}

// Resume returns the resume flag.
func (p *PregelScratchpad) Resume() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.resume
}

// SetResume sets the resume flag.
func (p *PregelScratchpad) SetResume(resume bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.resume = resume
	p.lastAccess = time.Now()
}

// SubgraphCounter returns the subgraph invocation count.
func (p *PregelScratchpad) SubgraphCounter() int64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.subgraphCounter
}

// IncrementSubgraphCounter increments the subgraph counter.
func (p *PregelScratchpad) IncrementSubgraphCounter() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.subgraphCounter++
	p.lastAccess = time.Now()
	return p.subgraphCounter
}

// Stats returns statistics about the scratchpad.
func (p *PregelScratchpad) Stats() ScratchpadStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return ScratchpadStats{
		DataSize:      len(p.data),
		CountersCount: len(p.counters),
		MetadataSize:  len(p.metadata),
		CreatedAt:     p.createdAt,
		LastAccess:    p.lastAccess,
	}
}

// ScratchpadStats represents statistics about a scratchpad.
type ScratchpadStats struct {
	DataSize      int
	CountersCount int
	MetadataSize  int
	NodeContexts  int
	CreatedAt     time.Time
	LastAccess    time.Time
}

// ===== Node-local context =====

// NodeContext provides per-node temporary storage.
// Data is automatically cleared when the node completes.
type NodeContext struct {
	mu   sync.RWMutex
	data map[string]interface{}
}

// NewNodeContext creates a new node-local context.
func NewNodeContext() *NodeContext {
	return &NodeContext{data: make(map[string]interface{})}
}

// Get retrieves a value from the node context.
func (nc *NodeContext) Get(key string) (interface{}, bool) {
	nc.mu.RLock()
	defer nc.mu.RUnlock()
	v, ok := nc.data[key]
	return v, ok
}

// Set stores a value in the node context.
func (nc *NodeContext) Set(key string, value interface{}) {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	nc.data[key] = value
}

// Delete removes a value.
func (nc *NodeContext) Delete(key string) {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	delete(nc.data, key)
}

// Clear removes all values from this node context.
func (nc *NodeContext) Clear() {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	nc.data = make(map[string]interface{})
}

// GetAll returns a copy of all data.
func (nc *NodeContext) GetAll() map[string]interface{} {
	nc.mu.RLock()
	defer nc.mu.RUnlock()
	result := make(map[string]interface{}, len(nc.data))
	for k, v := range nc.data {
		result[k] = v
	}
	return result
}

// NodeContext gets or creates a node-local context by node name.
// When the node completes, call ClearNodeContext to free the storage.
func (p *PregelScratchpad) NodeContext(nodeName string) *NodeContext {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := "_node_ctx:" + nodeName
	raw, ok := p.data[key]
	if !ok {
		nc := NewNodeContext()
		p.data[key] = nc
		return nc
	}
	nc, ok := raw.(*NodeContext)
	if !ok {
		nc = NewNodeContext()
		p.data[key] = nc
	}
	return nc
}

// ClearNodeContext clears the node-local context for the given node.
func (p *PregelScratchpad) ClearNodeContext(nodeName string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.data, "_node_ctx:"+nodeName)
}

// ClearAllNodeContexts clears all node-local contexts.
func (p *PregelScratchpad) ClearAllNodeContexts() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k := range p.data {
		if len(k) > 10 && k[:10] == "_node_ctx:" {
			delete(p.data, k)
		}
	}
}

// ===== Snapshot / Restore =====

// ScratchpadSnapshot captures the full state of the scratchpad for later restore.
// This is useful for checkpointing scratchpad state across graph resumptions.
type ScratchpadSnapshot struct {
	Data       map[string]interface{} `json:"data"`
	Counters   map[string]int64       `json:"counters,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Step       int64                  `json:"step"`
	CallCount  int64                  `json:"call_count"`
	Interrupts int64                  `json:"interrupts"`
	SubGraphs  int64                  `json:"subgraphs"`
}

// Snapshot captures the current scratchpad state.
// Node-local contexts are NOT included (they are ephemeral).
func (p *PregelScratchpad) Snapshot() *ScratchpadSnapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Deep copy data, excluding internal node-context keys.
	dataCopy := make(map[string]interface{}, len(p.data))
	for k, v := range p.data {
		if len(k) > 10 && k[:10] == "_node_ctx:" {
			continue // skip node-local contexts
		}
		dataCopy[k] = deepCopyValue(v)
	}
	countersCopy := make(map[string]int64, len(p.counters))
	for k, v := range p.counters {
		countersCopy[k] = v
	}
	metaCopy := make(map[string]interface{}, len(p.metadata))
	for k, v := range p.metadata {
		metaCopy[k] = deepCopyValue(v)
	}

	return &ScratchpadSnapshot{
		Data:       dataCopy,
		Counters:   countersCopy,
		Metadata:   metaCopy,
		Step:       p.step,
		CallCount:  p.callCounter,
		Interrupts: p.interruptCounter,
		SubGraphs:  p.subgraphCounter,
	}
}

// Restore restores the scratchpad state from a snapshot.
// Current node-local contexts are preserved (not overwritten).
func (p *PregelScratchpad) Restore(snap *ScratchpadSnapshot) {
	if snap == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	// Preserve node-local contexts before overwriting data.
	nodeCtxs := make(map[string]interface{})
	for k, v := range p.data {
		if len(k) > 10 && k[:10] == "_node_ctx:" {
			nodeCtxs[k] = v
		}
	}

	p.data = make(map[string]interface{}, len(snap.Data))
	for k, v := range snap.Data {
		p.data[k] = deepCopyValue(v)
	}
	// Restore node contexts.
	for k, v := range nodeCtxs {
		p.data[k] = v
	}

	p.counters = make(map[string]int64, len(snap.Counters))
	for k, v := range snap.Counters {
		p.counters[k] = v
	}
	p.metadata = make(map[string]interface{}, len(snap.Metadata))
	for k, v := range snap.Metadata {
		p.metadata[k] = v
	}
	p.step = snap.Step
	p.callCounter = snap.CallCount
	p.interruptCounter = snap.Interrupts
	p.subgraphCounter = snap.SubGraphs
}

// ===== Merge (for parallel branches) =====

// MergeFrom merges data from another scratchpad into this one.
// When keys collide, the values from the 'other' scratchpad take precedence.
// Node-local contexts are NOT merged (they are per-node).
// Counters are added together.
func (p *PregelScratchpad) MergeFrom(other *PregelScratchpad) {
	if other == nil {
		return
	}

	// Snapshot other under its read lock first, then release.
	other.mu.RLock()
	otherData := make(map[string]interface{}, len(other.data))
	for k, v := range other.data {
		if len(k) > 10 && k[:10] == "_node_ctx:" {
			continue
		}
		otherData[k] = deepCopyValue(v)
	}
	otherCounters := make(map[string]int64, len(other.counters))
	for k, v := range other.counters {
		otherCounters[k] = v
	}
	otherMeta := make(map[string]interface{}, len(other.metadata))
	for k, v := range other.metadata {
		otherMeta[k] = deepCopyValue(v)
	}
	otherStep := other.step
	otherCallCounter := other.callCounter
	otherInterruptCounter := other.interruptCounter
	otherSubgraphCounter := other.subgraphCounter
	other.mu.RUnlock()

	// Now acquire p's lock and apply.
	p.mu.Lock()
	defer p.mu.Unlock()

	// Merge data (other wins conflicts).
	for k, v := range otherData {
		p.data[k] = v
	}

	// Merge counters (sum).
	for k, v := range otherCounters {
		p.counters[k] += v
	}

	// Merge metadata (other wins conflicts).
	for k, v := range otherMeta {
		p.metadata[k] = v
	}

	// Merge step-related fields (take max).
	if otherStep > p.step {
		p.step = otherStep
	}
	p.callCounter += otherCallCounter
	p.interruptCounter += otherInterruptCounter
	p.subgraphCounter += otherSubgraphCounter
	p.lastAccess = time.Now()
}

// ===== Timeout / Expiry =====

// TimeoutConfig configures automatic scratchpad expiry.
type TimeoutConfig struct {
	// TTL is the maximum time a scratchpad lives before auto-clear.
	TTL time.Duration
	// ResetOnAccess resets the TTL timer on every read/write.
	ResetOnAccess bool
	// AutoClearData clears only data (not counters/metadata) on timeout.
	AutoClearData bool
}

// defaultTimeoutConfig returns the default timeout configuration.
func defaultTimeoutConfig() *TimeoutConfig {
	return &TimeoutConfig{
		TTL:           5 * time.Minute,
		ResetOnAccess: true,
		AutoClearData: true,
	}
}

// SetTimeout configures the scratchpad to auto-clear after the given duration.
func (p *PregelScratchpad) SetTimeout(d time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.metadata["_timeout_ttl"] = d
	p.metadata["_timeout_start"] = time.Now()
}

// IsExpired returns true if the scratchpad has timed out.
func (p *PregelScratchpad) IsExpired() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	rawTTL, ok := p.metadata["_timeout_ttl"]
	if !ok {
		return false
	}
	ttl, ok := rawTTL.(time.Duration)
	if !ok || ttl <= 0 {
		return false
	}
	rawStart, ok := p.metadata["_timeout_start"]
	if !ok {
		return false
	}
	start, ok := rawStart.(time.Time)
	if !ok {
		return false
	}
	return time.Since(start) > ttl
}

// ClearExpired checks if the scratchpad has expired and clears it if so.
// Returns true if the scratchpad was cleared.
func (p *PregelScratchpad) ClearExpired() bool {
	if !p.IsExpired() {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check once more under write lock.
	rawTTL, ok := p.metadata["_timeout_ttl"]
	if !ok {
		return false
	}
	ttl, ok := rawTTL.(time.Duration)
	if !ok || ttl <= 0 {
		return false
	}
	rawStart, ok := p.metadata["_timeout_start"]
	if !ok {
		return false
	}
	start, ok := rawStart.(time.Time)
	if !ok {
		return false
	}
	if time.Since(start) <= ttl {
		return false
	}

	// Expired: clear ephemeral data and timeout metadata so newly
	// written data is not immediately treated as expired.
	p.data = make(map[string]interface{})
	p.counters = make(map[string]int64)
	delete(p.metadata, "_timeout_ttl")
	delete(p.metadata, "_timeout_start")
	p.lastAccess = time.Now()
	return true
}

// deepCopyValue recursively clones a value, handling map[string]interface{}
// and []interface{} containers to prevent aliasing.
func deepCopyValue(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case map[string]interface{}:
		dst := make(map[string]interface{}, len(val))
		for k, v2 := range val {
			dst[k] = deepCopyValue(v2)
		}
		return dst
	case []interface{}:
		dst := make([]interface{}, len(val))
		for i, v2 := range val {
			dst[i] = deepCopyValue(v2)
		}
		return dst
	default:
		return v
	}
}

// deepCopyMap recursively copies a string-keyed map.
func deepCopyMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = deepCopyValue(v)
	}
	return dst
}

func init() {
	// Ensure scratchpad reset on init.
	_ = defaultTimeoutConfig
}
