package pregel

import (
	"context"
	"fmt"
	"sync"

	"ragflow/internal/harness/graph/channels"
)

// ChannelRead represents a read operation from channels.
// It encapsulates the logic for reading state from multiple channels.
type ChannelRead struct {
	registry    *channels.Registry
	selector    ChannelSelector
	transformer ChannelTransformer
	mu          sync.RWMutex
}

// ChannelSelector selects which channels to read.
type ChannelSelector interface {
	Select(registry *channels.Registry) ([]string, error)
}

// ChannelTransformer transforms the raw channel values.
type ChannelTransformer interface {
	Transform(values map[string]any) (map[string]any, error)
}

// NewChannelRead creates a new channel read operation.
func NewChannelRead(registry *channels.Registry, opts ...ChannelReadOption) *ChannelRead {
	cr := &ChannelRead{
		registry:    registry,
		selector:    &AllChannelsSelector{},
		transformer: &IdentityTransformer{},
	}

	for _, opt := range opts {
		opt(cr)
	}

	return cr
}

// ChannelReadOption configures a ChannelRead.
type ChannelReadOption func(*ChannelRead)

// WithSelector sets the channel selector.
func WithSelector(selector ChannelSelector) ChannelReadOption {
	return func(cr *ChannelRead) {
		cr.selector = selector
	}
}

// WithTransformer sets the channel transformer.
func WithTransformer(transformer ChannelTransformer) ChannelReadOption {
	return func(cr *ChannelRead) {
		cr.transformer = transformer
	}
}

// Read performs the read operation.
func (cr *ChannelRead) Read(ctx context.Context) (map[string]any, error) {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	// Select channels to read
	channelNames, err := cr.selector.Select(cr.registry)
	if err != nil {
		return nil, fmt.Errorf("channel selection failed: %w", err)
	}

	// Read values from channels
	values := make(map[string]any)
	for _, name := range channelNames {
		if ch, ok := cr.registry.Get(name); ok {
			val, err := ch.Get()
			if err == nil {
				values[name] = val
			}
		}
	}

	// Transform values
	if cr.transformer != nil {
		transformed, err := cr.transformer.Transform(values)
		if err != nil {
			return nil, fmt.Errorf("channel transformation failed: %w", err)
		}
		values = transformed
	}

	return values, nil
}

// ReadChannel reads a single channel by name.
func (cr *ChannelRead) ReadChannel(ctx context.Context, name string) (any, error) {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	if ch, ok := cr.registry.Get(name); ok {
		return ch.Get()
	}

	return nil, fmt.Errorf("channel not found: %s", name)
}

// HasChannel checks if a channel exists.
func (cr *ChannelRead) HasChannel(name string) bool {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	_, ok := cr.registry.Get(name)
	return ok
}

// ListChannels returns all available channel names.
func (cr *ChannelRead) ListChannels() []string {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	return cr.registry.List()
}

// ==================== Channel Selectors ====================

// AllChannelsSelector selects all available channels.
type AllChannelsSelector struct{}

func (s *AllChannelsSelector) Select(registry *channels.Registry) ([]string, error) {
	return registry.List(), nil
}

// SpecificChannelsSelector selects specific channels.
type SpecificChannelsSelector struct {
	channels []string
}

// NewSpecificChannelsSelector creates a selector for specific channels.
func NewSpecificChannelsSelector(channels ...string) *SpecificChannelsSelector {
	return &SpecificChannelsSelector{channels: channels}
}

func (s *SpecificChannelsSelector) Select(registry *channels.Registry) ([]string, error) {
	result := make([]string, 0, len(s.channels))
	for _, name := range s.channels {
		if _, ok := registry.Get(name); ok {
			result = append(result, name)
		}
	}
	return result, nil
}

// PrefixChannelsSelector selects channels with a specific prefix.
type PrefixChannelsSelector struct {
	prefix string
}

// NewPrefixChannelsSelector creates a selector for channels with a prefix.
func NewPrefixChannelsSelector(prefix string) *PrefixChannelsSelector {
	return &PrefixChannelsSelector{prefix: prefix}
}

func (s *PrefixChannelsSelector) Select(registry *channels.Registry) ([]string, error) {
	all := registry.List()
	result := make([]string, 0)
	for _, name := range all {
		if len(name) >= len(s.prefix) && name[:len(s.prefix)] == s.prefix {
			result = append(result, name)
		}
	}
	return result, nil
}

// AvailableChannelsSelector selects only available (non-empty) channels.
type AvailableChannelsSelector struct{}

func (s *AvailableChannelsSelector) Select(registry *channels.Registry) ([]string, error) {
	all := registry.List()
	result := make([]string, 0)
	for _, name := range all {
		if ch, ok := registry.Get(name); ok && ch.IsAvailable() {
			result = append(result, name)
		}
	}
	return result, nil
}

// ==================== Channel Transformers ====================

// IdentityTransformer returns values as-is.
type IdentityTransformer struct{}

func (t *IdentityTransformer) Transform(values map[string]any) (map[string]any, error) {
	return values, nil
}

// MappingTransformer renames channels.
type MappingTransformer struct {
	mappings map[string]string
}

// NewMappingTransformer creates a transformer with channel name mappings.
func NewMappingTransformer(mappings map[string]string) *MappingTransformer {
	return &MappingTransformer{mappings: mappings}
}

func (t *MappingTransformer) Transform(values map[string]any) (map[string]any, error) {
	result := make(map[string]any)
	for oldName, value := range values {
		newName := oldName
		if mapped, ok := t.mappings[oldName]; ok {
			newName = mapped
		}
		result[newName] = value
	}
	return result, nil
}

// FilterTransformer filters channels.
type FilterTransformer struct {
	filter map[string]bool
}

// NewFilterTransformer creates a transformer that filters channels.
func NewFilterTransformer(keep ...string) *FilterTransformer {
	filter := make(map[string]bool)
	for _, name := range keep {
		filter[name] = true
	}
	return &FilterTransformer{filter: filter}
}

func (t *FilterTransformer) Transform(values map[string]any) (map[string]any, error) {
	result := make(map[string]any)
	for name, value := range values {
		if t.filter[name] {
			result[name] = value
		}
	}
	return result, nil
}

// DefaultTransformer provides default values for missing channels.
type DefaultTransformer struct {
	defaults map[string]any
}

// NewDefaultTransformer creates a transformer with default values.
func NewDefaultTransformer(defaults map[string]any) *DefaultTransformer {
	return &DefaultTransformer{defaults: defaults}
}

func (t *DefaultTransformer) Transform(values map[string]any) (map[string]any, error) {
	result := make(map[string]any)
	for name, defValue := range t.defaults {
		if value, ok := values[name]; ok {
			result[name] = value
		} else {
			result[name] = defValue
		}
	}
	// Add any extra values that don't have defaults
	for name, value := range values {
		if _, ok := result[name]; !ok {
			result[name] = value
		}
	}
	return result, nil
}

// MergingTransformer merges channels into a single value.
type MergingTransformer struct {
	target string
	merger func([]any) (any, error)
}

// NewMergingTransformer creates a transformer that merges channels.
func NewMergingTransformer(target string, merger func([]any) (any, error)) *MergingTransformer {
	return &MergingTransformer{
		target: target,
		merger: merger,
	}
}

func (t *MergingTransformer) Transform(values map[string]any) (map[string]any, error) {
	// Collect all values
	items := make([]any, 0, len(values))
	for _, value := range values {
		items = append(items, value)
	}

	// Merge
	merged, err := t.merger(items)
	if err != nil {
		return nil, err
	}

	// Return merged value under target key
	return map[string]any{t.target: merged}, nil
}

// ==================== Triggers ====================

// Trigger determines when a channel read should execute.
type Trigger interface {
	ShouldTrigger(registry *channels.Registry) bool
}

// AlwaysTrigger always triggers.
type AlwaysTrigger struct{}

func (t *AlwaysTrigger) ShouldTrigger(registry *channels.Registry) bool {
	return true
}

// AnyAvailableTrigger triggers when any selected channel is available.
type AnyAvailableTrigger struct {
	channels []string
}

// NewAnyAvailableTrigger creates a trigger that fires when any channel is available.
func NewAnyAvailableTrigger(channels ...string) *AnyAvailableTrigger {
	return &AnyAvailableTrigger{channels: channels}
}

func (t *AnyAvailableTrigger) ShouldTrigger(registry *channels.Registry) bool {
	for _, name := range t.channels {
		if ch, ok := registry.Get(name); ok && ch.IsAvailable() {
			return true
		}
	}
	return false
}

// AllAvailableTrigger triggers when all selected channels are available.
type AllAvailableTrigger struct {
	channels []string
}

// NewAllAvailableTrigger creates a trigger that fires when all channels are available.
func NewAllAvailableTrigger(channels ...string) *AllAvailableTrigger {
	return &AllAvailableTrigger{channels: channels}
}

func (t *AllAvailableTrigger) ShouldTrigger(registry *channels.Registry) bool {
	for _, name := range t.channels {
		if ch, ok := registry.Get(name); !ok || !ch.IsAvailable() {
			return false
		}
	}
	return true
}

// ChannelChangedTrigger triggers when a specific channel's value changes.
type ChannelChangedTrigger struct {
	channel     string
	lastVersion int64
}

// NewChannelChangedTrigger creates a trigger that fires when a channel changes.
func NewChannelChangedTrigger(channel string) *ChannelChangedTrigger {
	return &ChannelChangedTrigger{
		channel:     channel,
		lastVersion: -1,
	}
}

func (t *ChannelChangedTrigger) ShouldTrigger(registry *channels.Registry) bool {
	if ch, ok := registry.Get(t.channel); ok {
		// Track the channel version to detect actual changes (not just availability).
		// GetVersion returns -1 if the channel does not support versioning, in which
		// case we fall back to IsAvailable() for backward compatibility.
		version := int64(ch.GetVersion())
		if version >= 0 && version != t.lastVersion {
			t.lastVersion = version
			return true
		}
		// Fallback for channels without version tracking.
		return version < 0 && ch.IsAvailable()
	}
	return false
}

// ==================== Utility Functions ====================

// ReadContext represents the context of a channel read operation.
type ReadContext struct {
	Node     string
	Step     int
	Triggers []Trigger
	Readers  map[string]*ChannelRead
}

// NewReadContext creates a new read context.
func NewReadContext(node string, step int) *ReadContext {
	return &ReadContext{
		Node:     node,
		Step:     step,
		Triggers: make([]Trigger, 0),
		Readers:  make(map[string]*ChannelRead),
	}
}

// AddReader adds a channel reader.
func (rc *ReadContext) AddReader(name string, reader *ChannelRead) {
	rc.Readers[name] = reader
}

// GetReader gets a channel reader by name.
func (rc *ReadContext) GetReader(name string) *ChannelRead {
	return rc.Readers[name]
}

// ShouldExecute checks if any trigger fires.
func (rc *ReadContext) ShouldExecute(registry *channels.Registry) bool {
	if len(rc.Triggers) == 0 {
		return true
	}

	for _, trigger := range rc.Triggers {
		if trigger.ShouldTrigger(registry) {
			return true
		}
	}

	return false
}

// ReadAll executes all readers and combines their results.
func (rc *ReadContext) ReadAll(ctx context.Context) (map[string]any, error) {
	combined := make(map[string]any)
	for name, reader := range rc.Readers {
		values, err := reader.Read(ctx)
		if err != nil {
			return nil, fmt.Errorf("reader %s failed: %w", name, err)
		}
		for k, v := range values {
			combined[name+"."+k] = v
		}
	}
	return combined, nil
}
