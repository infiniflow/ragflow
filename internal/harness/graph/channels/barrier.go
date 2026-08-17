package channels

import (
	"fmt"
	"sync"

	"ragflow/internal/harness/graph/errors"
)

// NamedBarrierValue waits until all specified named values are received before making the value available.
// This implementation matches Python LangGraph's NamedBarrierValue semantics.
type NamedBarrierValue struct {
	BaseChannel
	names map[string]bool // Set of expected values
	seen  map[string]bool // Set of received values
	mu    sync.RWMutex
}

// NewNamedBarrierValue creates a new NamedBarrierValue channel.
// waitFor is a slice of expected value names.
func NewNamedBarrierValue(typ interface{}, waitFor []string) *NamedBarrierValue {
	names := make(map[string]bool)
	for _, name := range waitFor {
		names[name] = true
	}
	return &NamedBarrierValue{
		BaseChannel: BaseChannel{Typ: typ},
		names:       names,
		seen:        make(map[string]bool),
	}
}

// Get returns the value (always nil for NamedBarrierValue) if all values have been received.
func (c *NamedBarrierValue) Get() (interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.namesMatchSeen() {
		return nil, &errors.EmptyChannelError{}
	}
	return nil, nil
}

// IsAvailable returns true if all expected values have been received.
func (c *NamedBarrierValue) IsAvailable() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.namesMatchSeen()
}

// Update updates the channel with new values.
// Each value should be a string name from the expected names set.
func (c *NamedBarrierValue) Update(values []interface{}) (bool, error) {
	if len(values) == 0 {
		return false, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	updated := false
	for _, val := range values {
		name, ok := val.(string)
		if !ok {
			return false, &errors.InvalidUpdateError{
				Message: fmt.Sprintf("value must be a string, got %T", val),
			}
		}

		if _, exists := c.names[name]; exists {
			if !c.seen[name] {
				c.seen[name] = true
				updated = true
			}
		} else {
			return false, &errors.InvalidUpdateError{
				Message: fmt.Sprintf("value '%s' not in expected names %v", name, c.names),
			}
		}
	}

	return updated, nil
}

// Copy returns a copy of the channel.
func (c *NamedBarrierValue) Copy() Channel {
	c.mu.RLock()
	defer c.mu.RUnlock()

	newCh := NewNamedBarrierValue(c.Typ, nil)
	newCh.Key = c.Key

	// Copy names map
	newCh.names = make(map[string]bool, len(c.names))
	for k, v := range c.names {
		newCh.names[k] = v
	}

	// Copy seen map
	newCh.seen = make(map[string]bool, len(c.seen))
	for k, v := range c.seen {
		newCh.seen[k] = v
	}

	return newCh
}

// Checkpoint returns the seen values as a map.
func (c *NamedBarrierValue) Checkpoint() interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.seen) == 0 {
		return Missing
	}

	// Return a copy of seen
	result := make(map[string]bool, len(c.seen))
	for k, v := range c.seen {
		result[k] = v
	}
	return result
}

// FromCheckpoint restores the channel from a checkpoint.
func (c *NamedBarrierValue) FromCheckpoint(checkpoint interface{}) Channel {
	c.mu.Lock()
	defer c.mu.Unlock()

	newCh := NewNamedBarrierValue(c.Typ, nil)
	newCh.Key = c.Key
	// Restore names from original (FromCheckpoint is called on the same channel).
	newCh.names = make(map[string]bool, len(c.names))
	for k, v := range c.names {
		newCh.names[k] = v
	}

	if checkpoint != nil && !IsMissing(checkpoint) {
		// Restore seen from checkpoint.
		// After JSON round-trip, map[string]bool becomes map[string]interface{}.
		newCh.seen = make(map[string]bool)
		switch v := checkpoint.(type) {
		case map[string]bool:
			for k, bv := range v {
				newCh.seen[k] = bv
			}
		case map[string]interface{}:
			for k, bv := range v {
				if b, ok := bv.(bool); ok {
					newCh.seen[k] = b
				}
			}
		}
	}

	return newCh
}

// Finish checks if all expected values have been received and clears them.
// Returns true if there were values to clear.
func (c *NamedBarrierValue) Finish() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.seen) > 0 {
		c.seen = make(map[string]bool)
		return true
	}
	return false
}

// Consume resets the channel after all values have been received.
// Returns true if the channel was consumed.
func (c *NamedBarrierValue) Consume() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.namesMatchSeen() && len(c.seen) > 0 {
		c.seen = make(map[string]bool)
		return true
	}
	return false
}

// namesMatchSeen checks if all expected names have been seen.
func (c *NamedBarrierValue) namesMatchSeen() bool {
	if len(c.names) != len(c.seen) {
		return false
	}
	for name := range c.names {
		if !c.seen[name] {
			return false
		}
	}
	return true
}

// NamedBarrierValueAfterFinish waits until all specified named values are received,
// but only makes the value available after finish() is called.
type NamedBarrierValueAfterFinish struct {
	BaseChannel
	names    map[string]bool // Set of expected values
	seen     map[string]bool // Set of received values
	finished bool
	mu       sync.RWMutex
}

// NewNamedBarrierValueAfterFinish creates a new NamedBarrierValueAfterFinish channel.
func NewNamedBarrierValueAfterFinish(typ interface{}, waitFor []string) *NamedBarrierValueAfterFinish {
	names := make(map[string]bool)
	for _, name := range waitFor {
		names[name] = true
	}
	return &NamedBarrierValueAfterFinish{
		BaseChannel: BaseChannel{Typ: typ},
		names:       names,
		seen:        make(map[string]bool),
		finished:    false,
	}
}

// Get returns the value (always nil) if finished and all values have been received.
func (c *NamedBarrierValueAfterFinish) Get() (interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.finished || !c.namesMatchSeen() {
		return nil, &errors.EmptyChannelError{}
	}
	return nil, nil
}

// IsAvailable returns true if finished and all expected values have been received.
func (c *NamedBarrierValueAfterFinish) IsAvailable() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.finished && c.namesMatchSeen()
}

// Update updates the channel with new values.
func (c *NamedBarrierValueAfterFinish) Update(values []interface{}) (bool, error) {
	if len(values) == 0 {
		return false, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	updated := false
	for _, val := range values {
		name, ok := val.(string)
		if !ok {
			return false, &errors.InvalidUpdateError{
				Message: fmt.Sprintf("value must be a string, got %T", val),
			}
		}

		if _, exists := c.names[name]; exists {
			if !c.seen[name] {
				c.seen[name] = true
				updated = true
			}
		} else {
			return false, &errors.InvalidUpdateError{
				Message: fmt.Sprintf("value '%s' not in expected names %v", name, c.names),
			}
		}
	}

	return updated, nil
}

// Copy returns a copy of the channel.
func (c *NamedBarrierValueAfterFinish) Copy() Channel {
	c.mu.RLock()
	defer c.mu.RUnlock()

	newCh := NewNamedBarrierValueAfterFinish(c.Typ, nil)
	newCh.Key = c.Key

	// Copy names map
	newCh.names = make(map[string]bool, len(c.names))
	for k, v := range c.names {
		newCh.names[k] = v
	}

	// Copy seen map
	newCh.seen = make(map[string]bool, len(c.seen))
	for k, v := range c.seen {
		newCh.seen[k] = v
	}

	newCh.finished = c.finished

	return newCh
}

// Checkpoint returns a tuple of (seen, finished).
func (c *NamedBarrierValueAfterFinish) Checkpoint() interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.seen) == 0 && !c.finished {
		return Missing
	}

	// Return as a map with both values
	result := map[string]interface{}{
		"seen":     c.seen,
		"finished": c.finished,
	}
	return result
}

// FromCheckpoint restores the channel from a checkpoint.
func (c *NamedBarrierValueAfterFinish) FromCheckpoint(checkpoint interface{}) Channel {
	c.mu.Lock()
	defer c.mu.Unlock()

	newCh := NewNamedBarrierValueAfterFinish(c.Typ, nil)
	newCh.Key = c.Key
	// Restore names from original.
	newCh.names = make(map[string]bool, len(c.names))
	for k, v := range c.names {
		newCh.names[k] = v
	}

	if checkpoint != nil && !IsMissing(checkpoint) {
		if cp, ok := checkpoint.(map[string]interface{}); ok {
			// After JSON round-trip, map[string]bool becomes map[string]interface{}.
			newCh.seen = make(map[string]bool)
			switch seen := cp["seen"].(type) {
			case map[string]bool:
				for k, bv := range seen {
					newCh.seen[k] = bv
				}
			case map[string]interface{}:
				for k, bv := range seen {
					if b, ok := bv.(bool); ok {
						newCh.seen[k] = b
					}
				}
			}
			if finished, ok := cp["finished"].(bool); ok {
				newCh.finished = finished
			}
		}
	}

	return newCh
}

// Finish marks the channel as finished if all values have been received.
// Returns true if the channel was marked as finished.
func (c *NamedBarrierValueAfterFinish) Finish() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.finished && c.namesMatchSeen() {
		c.finished = true
		return true
	}
	return false
}

// Consume resets the channel after finish and all values have been received.
// Returns true if the channel was consumed.
func (c *NamedBarrierValueAfterFinish) Consume() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.finished && c.namesMatchSeen() && len(c.seen) > 0 {
		c.finished = false
		c.seen = make(map[string]bool)
		return true
	}
	return false
}

// namesMatchSeen checks if all expected names have been seen.
func (c *NamedBarrierValueAfterFinish) namesMatchSeen() bool {
	if len(c.names) != len(c.seen) {
		return false
	}
	for name := range c.names {
		if !c.seen[name] {
			return false
		}
	}
	return true
}

// LastValueAfterFinish stores the last value received, but only makes it available after finish().
type LastValueAfterFinish struct {
	BaseChannel
	value    interface{}
	finished bool
	mu       sync.RWMutex
}

// NewLastValueAfterFinish creates a new LastValueAfterFinish channel.
func NewLastValueAfterFinish(typ interface{}) *LastValueAfterFinish {
	return &LastValueAfterFinish{
		BaseChannel: BaseChannel{Typ: typ},
		value:       Missing,
		finished:    false,
	}
}

// Get returns the last value received after finish().
func (c *LastValueAfterFinish) Get() (interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.finished {
		return nil, &errors.EmptyChannelError{}
	}

	if IsMissing(c.value) {
		return nil, &errors.EmptyChannelError{}
	}

	return c.value, nil
}

// IsAvailable returns true if finished and has a value.
func (c *LastValueAfterFinish) IsAvailable() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.finished && !IsMissing(c.value)
}

// Update updates the channel with new values.
func (c *LastValueAfterFinish) Update(values []interface{}) (bool, error) {
	if len(values) == 0 {
		return false, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Only accept one value per step
	if len(values) > 1 {
		return false, &errors.InvalidUpdateError{
			Message: "Can receive only one value per step. Use a reducer to handle multiple values.",
		}
	}

	c.value = values[0]
	return true, nil
}

// Copy returns a copy of the channel.
func (c *LastValueAfterFinish) Copy() Channel {
	c.mu.RLock()
	defer c.mu.RUnlock()

	newCh := NewLastValueAfterFinish(c.Typ)
	newCh.Key = c.Key
	newCh.value = c.value
	newCh.finished = c.finished
	return newCh
}

// Checkpoint returns the current value.
func (c *LastValueAfterFinish) Checkpoint() interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.value
}

// FromCheckpoint restores the channel from a checkpoint.
func (c *LastValueAfterFinish) FromCheckpoint(checkpoint interface{}) Channel {
	c.mu.Lock()
	defer c.mu.Unlock()

	newCh := NewLastValueAfterFinish(c.Typ)
	newCh.Key = c.Key

	if !IsMissing(checkpoint) {
		newCh.value = checkpoint
	}

	return newCh
}

// Finish marks the channel as finished.
func (c *LastValueAfterFinish) Finish() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.finished {
		c.finished = true
		return true
	}
	return false
}

// Consume always returns false for this channel.
func (c *LastValueAfterFinish) Consume() bool {
	return false
}
