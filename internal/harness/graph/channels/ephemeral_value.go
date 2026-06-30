package channels

import (
	"ragflow/internal/harness/graph/errors"
)

// EphemeralValue stores a value that is cleared after being read once.
type EphemeralValue struct {
	BaseChannel
	value interface{}
	guard bool // if true, raises EmptyChannelError if value is not set
}

// NewEphemeralValue creates a new EphemeralValue channel.
// If guard is true, raises EmptyChannelError if value is not set when Get is called.
func NewEphemeralValue(typ interface{}, guard bool) *EphemeralValue {
	return &EphemeralValue{
		BaseChannel: BaseChannel{Typ: typ},
		value:       Missing,
		guard:       guard,
	}
}

// Get returns the current value of the channel and clears it.
func (c *EphemeralValue) Get() (interface{}, error) {
	if IsMissing(c.value) {
		if c.guard {
			return nil, &errors.EmptyChannelError{}
		}
		return nil, nil
	}
	val := c.value
	c.value = Missing
	return val, nil
}

// IsAvailable returns true if the channel has a value.
func (c *EphemeralValue) IsAvailable() bool {
	return !IsMissing(c.value)
}

// Update updates the channel with values.
// Keeps only the last value.
func (c *EphemeralValue) Update(values []interface{}) (bool, error) {
	if len(values) == 0 {
		return false, nil
	}
	c.value = values[len(values)-1]
	return true, nil
}

// Copy returns a copy of the channel.
func (c *EphemeralValue) Copy() Channel {
	newCh := NewEphemeralValue(c.Typ, c.guard)
	newCh.Key = c.Key
	newCh.value = c.value
	return newCh
}

// Checkpoint returns the current value.
func (c *EphemeralValue) Checkpoint() interface{} {
	return c.value
}

// FromCheckpoint restores the channel from a checkpoint.
func (c *EphemeralValue) FromCheckpoint(checkpoint interface{}) Channel {
	newCh := NewEphemeralValue(c.Typ, c.guard)
	newCh.Key = c.Key
	if !IsMissing(checkpoint) {
		newCh.value = checkpoint
	}
	return newCh
}
