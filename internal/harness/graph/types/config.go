package types

import "ragflow/internal/harness/graph/constants"

// RunnableConfig represents configuration for a runnable execution.
type RunnableConfig struct {
	// Configurable values that can be used by nodes and checkpointers
	Configurable map[string]interface{}
	
	// RecursionLimit is the maximum number of steps before raising GraphRecursionError
	RecursionLimit int
	
	// Tags for the execution
	Tags []string
	
	// Metadata for the execution
	Metadata map[string]interface{}
	
	// RunID is a unique identifier for this run
	RunID string
	
	// ThreadID is the thread identifier for checkpointing
	ThreadID string
	
	// Durability controls when checkpoint writes are persisted
	Durability Durability
}

// NewRunnableConfig creates a new RunnableConfig with defaults.
func NewRunnableConfig() *RunnableConfig {
	return &RunnableConfig{
		Configurable:   make(map[string]interface{}),
		RecursionLimit: constants.DefaultRecursionLimit,
		Tags:           make([]string, 0),
		Metadata:       make(map[string]interface{}),
		Durability:     DurabilitySync,
	}
}

// Get gets a value from the configurable map.
func (c *RunnableConfig) Get(key string) (interface{}, bool) {
	if c.Configurable == nil {
		return nil, false
	}
	val, ok := c.Configurable[key]
	return val, ok
}

// Set sets a value in the configurable map.
func (c *RunnableConfig) Set(key string, value interface{}) {
	if c.Configurable == nil {
		c.Configurable = make(map[string]interface{})
	}
	c.Configurable[key] = value
}

// Merge merges another config into this one.
func (c *RunnableConfig) Merge(other *RunnableConfig) *RunnableConfig {
	if other == nil {
		return c
	}
	
	// Merge configurable
	for k, v := range other.Configurable {
		c.Set(k, v)
	}
	
	// Use other's recursion limit if set
	if other.RecursionLimit > 0 {
		c.RecursionLimit = other.RecursionLimit
	}
	
	// Merge tags
	c.Tags = append(c.Tags, other.Tags...)
	
	// Merge metadata
	for k, v := range other.Metadata {
		c.Metadata[k] = v
	}
	
	// Use other's RunID if set
	if other.RunID != "" {
		c.RunID = other.RunID
	}
	
	// Use other's ThreadID if set
	if other.ThreadID != "" {
		c.ThreadID = other.ThreadID
	}
	
	// Use other's Durability if set (not default)
	if other.Durability != "" && other.Durability != DurabilitySync {
		c.Durability = other.Durability
	}
	
	return c
}

// WithConfigurable returns a new config with the given configurable values.
func (c *RunnableConfig) WithConfigurable(configurable map[string]interface{}) *RunnableConfig {
	c.Configurable = configurable
	return c
}

// WithRecursionLimit returns a new config with the given recursion limit.
func (c *RunnableConfig) WithRecursionLimit(limit int) *RunnableConfig {
	c.RecursionLimit = limit
	return c
}

// WithTags returns a new config with the given tags.
func (c *RunnableConfig) WithTags(tags ...string) *RunnableConfig {
	c.Tags = tags
	return c
}

// WithMetadata returns a new config with the given metadata.
func (c *RunnableConfig) WithMetadata(metadata map[string]interface{}) *RunnableConfig {
	c.Metadata = metadata
	return c
}

// WithRunID returns a new config with the given run ID.
func (c *RunnableConfig) WithRunID(runID string) *RunnableConfig {
	c.RunID = runID
	return c
}

// WithThreadID returns a new config with the given thread ID.
func (c *RunnableConfig) WithThreadID(threadID string) *RunnableConfig {
	c.ThreadID = threadID
	return c
}

// WithDurability returns a new config with the given durability mode.
func (c *RunnableConfig) WithDurability(durability Durability) *RunnableConfig {
	c.Durability = durability
	return c
}

// GetOrEmpty returns the string value for a configurable key, or "" if not present.
// This is a convenience for checkpoint config map construction.
func (c *RunnableConfig) GetOrEmpty(key string) string {
	if c.Configurable == nil {
		return ""
	}
	v, ok := c.Configurable[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// ConfigPatcher is a function that patches a config.
type ConfigPatcher func(*RunnableConfig) *RunnableConfig

// PatchConfig applies a series of patchers to a config.
func PatchConfig(config *RunnableConfig, patchers ...ConfigPatcher) *RunnableConfig {
	if config == nil {
		config = NewRunnableConfig()
	}
	
	for _, patcher := range patchers {
		if patcher != nil {
			config = patcher(config)
		}
	}
	
	return config
}

// EnsureConfig ensures a config is not nil.
func EnsureConfig(config *RunnableConfig) *RunnableConfig {
	if config == nil {
		return NewRunnableConfig()
	}
	return config
}

// MergeConfigs merges multiple configs into one.
func MergeConfigs(configs ...*RunnableConfig) *RunnableConfig {
	result := NewRunnableConfig()
	
	for _, config := range configs {
		result.Merge(config)
	}
	
	return result
}
