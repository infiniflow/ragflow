package channels

import (
	"fmt"

	"ragflow/internal/harness/graph/errors"
)

// LastValue stores the last value received, can receive at most one value per step.
type LastValue struct {
	BaseChannel
	value interface{}
}

// NewLastValue creates a new LastValue channel.
func NewLastValue(typ interface{}) *LastValue {
	return &LastValue{
		BaseChannel: BaseChannel{Typ: typ},
		value:       Missing,
	}
}

// Get returns the current value of the channel.
func (c *LastValue) Get() (interface{}, error) {
	if IsMissing(c.value) {
		return nil, &errors.EmptyChannelError{}
	}
	return c.value, nil
}

// IsAvailable returns true if the channel has a value.
func (c *LastValue) IsAvailable() bool {
	return !IsMissing(c.value)
}

// Update updates the channel with a single value.
func (c *LastValue) Update(values []interface{}) (bool, error) {
	if len(values) == 0 {
		return false, nil
	}
	if len(values) != 1 {
		return false, &errors.InvalidUpdateError{
			Message: fmt.Sprintf("At key '%s': Can receive only one value per step. Use a reducer to handle multiple values.", c.Key),
		}
	}
	c.value = values[len(values)-1]
	return true, nil
}

// Copy returns a copy of the channel.
func (c *LastValue) Copy() Channel {
	newCh := NewLastValue(c.Typ)
	newCh.Key = c.Key
	newCh.value = c.value
	return newCh
}

// Checkpoint returns the current value.
func (c *LastValue) Checkpoint() interface{} {
	return c.value
}

// FromCheckpoint restores the channel from a checkpoint.
func (c *LastValue) FromCheckpoint(checkpoint interface{}) Channel {
	newCh := NewLastValue(c.Typ)
	newCh.Key = c.Key
	if !IsMissing(checkpoint) {
		newCh.value = checkpoint
	}
	return newCh
}
