package channels

import (
	"ragflow/internal/harness/graph/errors"
)

// UntrackedValue stores a value but does not track it for checkpointing.
// The value is lost when the graph is resumed from a checkpoint.
type UntrackedValue struct {
	BaseChannel
	value interface{}
}

// NewUntrackedValue creates a new UntrackedValue channel.
func NewUntrackedValue(typ interface{}) *UntrackedValue {
	return &UntrackedValue{
		BaseChannel: BaseChannel{Typ: typ},
		value:       Missing,
	}
}

// Get returns the current value of the channel.
func (c *UntrackedValue) Get() (interface{}, error) {
	if IsMissing(c.value) {
		return nil, &errors.EmptyChannelError{}
	}
	return c.value, nil
}

// IsAvailable returns true if the channel has a value.
func (c *UntrackedValue) IsAvailable() bool {
	return !IsMissing(c.value)
}

// Update updates the channel with values.
func (c *UntrackedValue) Update(values []interface{}) (bool, error) {
	if len(values) == 0 {
		return false, nil
	}
	c.value = values[len(values)-1]
	return true, nil
}

// Copy returns a copy of the channel (value is not copied, starts empty).
func (c *UntrackedValue) Copy() Channel {
	newCh := NewUntrackedValue(c.Typ)
	newCh.Key = c.Key
	return newCh
}

// Checkpoint returns Missing (value is not checkpointed).
func (c *UntrackedValue) Checkpoint() interface{} {
	return Missing
}

// FromCheckpoint restores the channel from a checkpoint (always starts empty).
func (c *UntrackedValue) FromCheckpoint(checkpoint interface{}) Channel {
	return NewUntrackedValue(c.Typ)
}
