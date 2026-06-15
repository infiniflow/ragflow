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
	step             int64               // current step number
	stop             bool                // stop flag
	callCounter      int64               // number of calls
	interruptCounter int64               // number of interrupts
	resume           bool                // resume flag
	subgraphCounter  int64               // subgraph invocation count
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
		DataSize:       len(p.data),
		CountersCount: len(p.counters),
		MetadataSize:   len(p.metadata),
		CreatedAt:      p.createdAt,
		LastAccess:     p.lastAccess,
	}
}

// ScratchpadStats represents statistics about a scratchpad.
type ScratchpadStats struct {
	DataSize       int
	CountersCount  int
	MetadataSize   int
	CreatedAt      time.Time
	LastAccess     time.Time
}
