// Package channels provides channel implementations for LangGraph Go.
package channels

import (
	"fmt"
	"sync"
	"sync/atomic"

	"ragflow/internal/harness/graph/errors"
)

// Missing is a sentinel value to indicate a missing value.
var Missing = &missingSentinel{}

type missingSentinel struct{}

func (m *missingSentinel) String() string {
	return "<MISSING>"
}

// IsMissing checks if a value is the Missing sentinel.
func IsMissing(v interface{}) bool {
	if v == nil {
		return false
	}
	_, ok := v.(*missingSentinel)
	return ok
}

// Channel is the base interface for all channels.
type Channel interface {
	// GetKey returns the channel key.
	GetKey() string
	// SetKey sets the channel key.
	SetKey(key string)
	// Get returns the current value of the channel.
	// Returns EmptyChannelError if the channel is empty.
	Get() (interface{}, error)
	// IsAvailable returns true if the channel is available (not empty).
	IsAvailable() bool
	// Update updates the channel's value with the given sequence of updates.
	// Returns true if the channel was updated.
	Update(values []interface{}) (bool, error)
	// Copy returns a copy of the channel.
	Copy() Channel
	// Checkpoint returns a serializable representation of the channel's current state.
	// Returns Missing if the channel is empty.
	Checkpoint() interface{}
	// FromCheckpoint returns a new channel initialized from a checkpoint.
	FromCheckpoint(checkpoint interface{}) Channel
	// Consume notifies the channel that a subscribed task ran.
	// Returns true if the channel was updated.
	Consume() bool
	// Finish notifies the channel that the Pregel run is finishing.
	// Returns true if the channel was updated.
	Finish() bool
	// GetVersion returns the current version number of this channel.
	// Returns -1 if the channel does not support version tracking.
	GetVersion() int
}

// BaseChannel provides a base implementation of Channel.
// Embed this struct in your channel implementations.
type BaseChannel struct {
	Key     string
	Typ     interface{}
	Version int64 // atomic: channel version for change detection (thread-safe)
}

// GetKey returns the channel key.
func (c *BaseChannel) GetKey() string {
	return c.Key
}

// SetKey sets the channel key.
func (c *BaseChannel) SetKey(key string) {
	c.Key = key
}

// Consume is a no-op by default.
func (c *BaseChannel) Consume() bool {
	return false
}

// Finish is a no-op by default.
func (c *BaseChannel) Finish() bool {
	return false
}

// GetVersion returns the current version of this channel using atomic read.
// Returns -1 for channels that do not use version tracking.
func (c *BaseChannel) GetVersion() int {
	return int(atomic.LoadInt64(&c.Version))
}

// SetVersion sets the channel version atomically (used by the engine after applying writes).
func (c *BaseChannel) SetVersion(v int) {
	atomic.StoreInt64(&c.Version, int64(v))
}

// Registry is a registry of channel types.
type Registry struct {
	mu       sync.RWMutex
	channels map[string]Channel
}

// NewRegistry creates a new channel registry.
func NewRegistry() *Registry {
	return &Registry{
		channels: make(map[string]Channel),
	}
}

// Register registers a channel.
func (r *Registry) Register(name string, channel Channel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.channels[name] = channel
}

// Get gets a channel by name.
func (r *Registry) Get(name string) (Channel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ch, ok := r.channels[name]
	return ch, ok
}

// Remove removes a channel.
func (r *Registry) Remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.channels, name)
}

// Len returns the number of channels.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.channels)
}

// Names returns all channel names.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.channels))
	for name := range r.channels {
		names = append(names, name)
	}
	return names
}

// List returns all channel names (alias for Names).
func (r *Registry) List() []string {
	return r.Names()
}

// CreateCheckpoint creates a checkpoint for all channels.
func (r *Registry) CreateCheckpoint() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	checkpoint := make(map[string]interface{})
	for name, channel := range r.channels {
		cp := channel.Checkpoint()
		if !IsMissing(cp) {
			checkpoint[name] = cp
		}
	}
	return checkpoint
}

// RestoreFromCheckpoint restores all channels from a checkpoint.
func (r *Registry) RestoreFromCheckpoint(checkpoint map[string]interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, cp := range checkpoint {
		channel, ok := r.channels[name]
		if !ok {
			return fmt.Errorf("channel %s not found in registry", name)
		}
		newChannel := channel.FromCheckpoint(cp)
		r.channels[name] = newChannel
	}
	return nil
}

// UpdateChannels updates all channels with the given writes.
func (r *Registry) UpdateChannels(writes map[string][]interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, values := range writes {
		channel, ok := r.channels[name]
		if !ok {
			return &errors.ChannelNotFoundError{ChannelName: name}
		}
		if _, err := channel.Update(values); err != nil {
			return fmt.Errorf("failed to update channel %s: %w", name, err)
		}
	}
	return nil
}

// GetValues returns the current values of all channels.
func (r *Registry) GetValues() (map[string]interface{}, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	values := make(map[string]interface{})
	for name, channel := range r.channels {
		val, err := channel.Get()
		if err != nil {
			if errors.IsEmptyChannelError(err) {
				continue
			}
			return nil, fmt.Errorf("failed to get value from channel %s: %w", name, err)
		}
		values[name] = val
	}
	return values, nil
}
