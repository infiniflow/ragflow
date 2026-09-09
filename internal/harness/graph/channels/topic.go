package channels

import (
	"ragflow/internal/harness/graph/errors"
)

// Topic is a configurable PubSub Topic.
type Topic struct {
	BaseChannel
	values     []interface{}
	accumulate bool
}

// NewTopic creates a new Topic channel.
// If accumulate is false, the channel will be emptied after each step.
func NewTopic(typ interface{}, accumulate bool) *Topic {
	return &Topic{
		BaseChannel: BaseChannel{Typ: typ},
		values:      make([]interface{}, 0),
		accumulate:  accumulate,
	}
}

// Get returns the current values of the channel.
func (c *Topic) Get() (interface{}, error) {
	if len(c.values) == 0 {
		return nil, &errors.EmptyChannelError{}
	}
	result := make([]interface{}, len(c.values))
	copy(result, c.values)
	return result, nil
}

// IsAvailable returns true if the channel has values.
func (c *Topic) IsAvailable() bool {
	return len(c.values) > 0
}

// flatten flattens a sequence of values that may contain lists.
func flatten(values []interface{}) []interface{} {
	result := make([]interface{}, 0)
	for _, v := range values {
		if list, ok := v.([]interface{}); ok {
			result = append(result, list...)
		} else {
			result = append(result, v)
		}
	}
	return result
}

// Update updates the channel with values.
func (c *Topic) Update(values []interface{}) (bool, error) {
	updated := false
	if !c.accumulate {
		if len(c.values) > 0 {
			updated = true
		}
		c.values = make([]interface{}, 0)
	}
	flatValues := flatten(values)
	if len(flatValues) > 0 {
		updated = true
		c.values = append(c.values, flatValues...)
	}
	return updated, nil
}

// Copy returns a copy of the channel.
func (c *Topic) Copy() Channel {
	newCh := NewTopic(c.Typ, c.accumulate)
	newCh.Key = c.Key
	newCh.values = make([]interface{}, len(c.values))
	copy(newCh.values, c.values)
	return newCh
}

// Checkpoint returns the current values, or Missing if empty.
func (c *Topic) Checkpoint() interface{} {
	if len(c.values) == 0 {
		return Missing
	}
	result := make([]interface{}, len(c.values))
	copy(result, c.values)
	return result
}

// FromCheckpoint restores the channel from a checkpoint.
func (c *Topic) FromCheckpoint(checkpoint interface{}) Channel {
	newCh := NewTopic(c.Typ, c.accumulate)
	newCh.Key = c.Key
	if !IsMissing(checkpoint) {
		if v, ok := checkpoint.([]interface{}); ok {
			newCh.values = make([]interface{}, len(v))
			copy(newCh.values, v)
		}
	}
	return newCh
}
