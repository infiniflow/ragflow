package channels

import (
	"ragflow/internal/harness/graph/errors"
)

// AnyValue stores any value received, overwriting the previous value.
type AnyValue struct {
	BaseChannel
	value interface{}
}

// NewAnyValue creates a new AnyValue channel.
func NewAnyValue(typ interface{}) *AnyValue {
	return &AnyValue{
		BaseChannel: BaseChannel{Typ: typ},
		value:       Missing,
	}
}

// Get returns the current value of the channel.
func (c *AnyValue) Get() (interface{}, error) {
	if IsMissing(c.value) {
		return nil, &errors.EmptyChannelError{}
	}
	return c.value, nil
}

// IsAvailable returns true if the channel has a value.
func (c *AnyValue) IsAvailable() bool {
	return !IsMissing(c.value)
}

// Update updates the channel with values.
// Accepts any number of values, keeps the last one.
func (c *AnyValue) Update(values []interface{}) (bool, error) {
	if len(values) == 0 {
		return false, nil
	}
	c.value = values[len(values)-1]
	return true, nil
}

// Copy returns a copy of the channel.
func (c *AnyValue) Copy() Channel {
	newCh := NewAnyValue(c.Typ)
	newCh.Key = c.Key
	newCh.value = c.value
	return newCh
}

// Checkpoint returns the current value.
func (c *AnyValue) Checkpoint() interface{} {
	return c.value
}

// FromCheckpoint restores the channel from a checkpoint.
func (c *AnyValue) FromCheckpoint(checkpoint interface{}) Channel {
	newCh := NewAnyValue(c.Typ)
	newCh.Key = c.Key
	if !IsMissing(checkpoint) {
		newCh.value = checkpoint
	}
	return newCh
}
