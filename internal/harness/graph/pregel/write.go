package pregel

import (
	"context"
	"fmt"
	"sync"

	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/types"
)

// ChannelWrite represents a write operation to channels.
// It encapsulates the logic for writing state updates to multiple channels.
type ChannelWrite struct {
	registry  *channels.Registry
	entries   []*ChannelWriteEntry
	transformer WriteTransformer
	validator WriteValidator
	mu        sync.RWMutex
}

// ChannelWriteEntry represents a single write operation.
type ChannelWriteEntry struct {
	Channel   string
	Value     interface{}
	Overwrite bool
	Node      string
	Metadata  map[string]interface{}
}

// WriteTransformer transforms write values before applying them.
type WriteTransformer interface {
	Transform(entry *ChannelWriteEntry) (*ChannelWriteEntry, error)
}

// WriteValidator validates write operations.
type WriteValidator interface {
	Validate(entry *ChannelWriteEntry) error
}

// NewChannelWrite creates a new channel write operation.
func NewChannelWrite(registry *channels.Registry, opts ...ChannelWriteOption) *ChannelWrite {
	cw := &ChannelWrite{
		registry:  registry,
		entries:   make([]*ChannelWriteEntry, 0),
		transformer: &IdentityWriteTransformer{},
		validator: &NoOpValidator{},
	}

	for _, opt := range opts {
		opt(cw)
	}

	return cw
}

// ChannelWriteOption configures a ChannelWrite.
type ChannelWriteOption func(*ChannelWrite)

// WithWriteTransformer sets the write transformer.
func WithWriteTransformer(transformer WriteTransformer) ChannelWriteOption {
	return func(cw *ChannelWrite) {
		cw.transformer = transformer
	}
}

// WithValidator sets the write validator.
func WithValidator(validator WriteValidator) ChannelWriteOption {
	return func(cw *ChannelWrite) {
		cw.validator = validator
	}
}

// AddEntry adds a write entry.
func (cw *ChannelWrite) AddEntry(entry *ChannelWriteEntry) {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	cw.entries = append(cw.entries, entry)
}

// AddEntries adds multiple write entries.
func (cw *ChannelWrite) AddEntries(entries ...*ChannelWriteEntry) {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	cw.entries = append(cw.entries, entries...)
}

// WriteTo adds a simple write to a channel.
func (cw *ChannelWrite) WriteTo(channel string, value interface{}) {
	cw.AddEntry(&ChannelWriteEntry{
		Channel:   channel,
		Value:     value,
		Overwrite: false,
	})
}

// Overwrite overwrites a channel with a value.
func (cw *ChannelWrite) Overwrite(channel string, value interface{}) {
	cw.AddEntry(&ChannelWriteEntry{
		Channel:   channel,
		Value:     value,
		Overwrite: true,
	})
}

// WriteNode writes from a specific node.
func (cw *ChannelWrite) WriteNode(node string, channel string, value interface{}) {
	cw.AddEntry(&ChannelWriteEntry{
		Channel:   channel,
		Value:     value,
		Overwrite: false,
		Node:      node,
	})
}

// Write executes all write operations.
func (cw *ChannelWrite) Write(ctx context.Context) (map[string]bool, error) {
	cw.mu.Lock()
	defer cw.mu.Unlock()

	updated := make(map[string]bool)

	for _, entry := range cw.entries {
		// Validate
		if cw.validator != nil {
			if err := cw.validator.Validate(entry); err != nil {
				return nil, fmt.Errorf("validation failed for channel %s: %w", entry.Channel, err)
			}
		}

		// Transform
		transformed := entry
		if cw.transformer != nil {
			var err error
			transformed, err = cw.transformer.Transform(entry)
			if err != nil {
				return nil, fmt.Errorf("transformation failed for channel %s: %w", entry.Channel, err)
			}
		}

		// Apply write
		if ch, ok := cw.registry.Get(transformed.Channel); ok {
			// Check for Overwrite wrapper
			value := transformed.Value
			if transformed.Overwrite {
				value = &types.Overwrite{Value: value}
			}

			wasUpdated, err := ch.Update([]interface{}{value})
			if err != nil {
				return nil, fmt.Errorf("failed to update channel %s: %w", transformed.Channel, err)
			}
			if wasUpdated {
				updated[transformed.Channel] = true
			}
		} else {
			return nil, fmt.Errorf("channel not found: %s", transformed.Channel)
		}
	}

	// Clear entries after write
	cw.entries = make([]*ChannelWriteEntry, 0)

	return updated, nil
}

// Clear clears all pending write entries.
func (cw *ChannelWrite) Clear() {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	cw.entries = make([]*ChannelWriteEntry, 0)
}

// EntryCount returns the number of pending entries.
func (cw *ChannelWrite) EntryCount() int {
	cw.mu.RLock()
	defer cw.mu.RUnlock()
	return len(cw.entries)
}

// GetEntries returns a copy of all entries.
func (cw *ChannelWrite) GetEntries() []*ChannelWriteEntry {
	cw.mu.RLock()
	defer cw.mu.RUnlock()

	entries := make([]*ChannelWriteEntry, len(cw.entries))
	copy(entries, cw.entries)
	return entries
}

// ==================== Write Transformers ====================

// IdentityWriteTransformer doesn't transform.
type IdentityWriteTransformer struct{}

func (t *IdentityWriteTransformer) Transform(entry *ChannelWriteEntry) (*ChannelWriteEntry, error) {
	return entry, nil
}

// MappingWriteTransformer maps channel names.
type MappingWriteTransformer struct {
	mappings map[string]string
}

// NewMappingWriteTransformer creates a transformer that maps channel names.
func NewMappingWriteTransformer(mappings map[string]string) *MappingWriteTransformer {
	return &MappingWriteTransformer{mappings: mappings}
}

func (t *MappingWriteTransformer) Transform(entry *ChannelWriteEntry) (*ChannelWriteEntry, error) {
	if newName, ok := t.mappings[entry.Channel]; ok {
		transformed := *entry
		transformed.Channel = newName
		return &transformed, nil
	}
	return entry, nil
}

// PrefixWriteTransformer adds a prefix to channel names.
type PrefixWriteTransformer struct {
	prefix string
}

// NewPrefixWriteTransformer creates a transformer that adds a prefix.
func NewPrefixWriteTransformer(prefix string) *PrefixWriteTransformer {
	return &PrefixWriteTransformer{prefix: prefix}
}

func (t *PrefixWriteTransformer) Transform(entry *ChannelWriteEntry) (*ChannelWriteEntry, error) {
	transformed := *entry
	transformed.Channel = t.prefix + entry.Channel
	return &transformed, nil
}

// MetadataWriteTransformer adds metadata to entries.
type MetadataWriteTransformer struct {
	metadata map[string]interface{}
}

// NewMetadataWriteTransformer creates a transformer that adds metadata.
func NewMetadataWriteTransformer(metadata map[string]interface{}) *MetadataWriteTransformer {
	return &MetadataWriteTransformer{metadata: metadata}
}

func (t *MetadataWriteTransformer) Transform(entry *ChannelWriteEntry) (*ChannelWriteEntry, error) {
	transformed := *entry
	if transformed.Metadata == nil {
		transformed.Metadata = make(map[string]interface{})
	}
	for k, v := range t.metadata {
		transformed.Metadata[k] = v
	}
	return &transformed, nil
}

// NodeWriteTransformer adds node information to entries.
type NodeWriteTransformer struct {
	node string
}

// NewNodeWriteTransformer creates a transformer that adds node info.
func NewNodeWriteTransformer(node string) *NodeWriteTransformer {
	return &NodeWriteTransformer{node: node}
}

func (t *NodeWriteTransformer) Transform(entry *ChannelWriteEntry) (*ChannelWriteEntry, error) {
	if entry.Node == "" {
		transformed := *entry
		transformed.Node = t.node
		return &transformed, nil
	}
	return entry, nil
}

// FilterWriteTransformer filters entries based on a predicate.
type FilterWriteTransformer struct {
	predicate func(*ChannelWriteEntry) bool
}

// NewFilterWriteTransformer creates a transformer that filters entries.
func NewFilterWriteTransformer(predicate func(*ChannelWriteEntry) bool) *FilterWriteTransformer {
	return &FilterWriteTransformer{predicate: predicate}
}

func (t *FilterWriteTransformer) Transform(entry *ChannelWriteEntry) (*ChannelWriteEntry, error) {
	if t.predicate != nil && !t.predicate(entry) {
		return nil, &WriteSkipError{Channel: entry.Channel}
	}
	return entry, nil
}

// ==================== Write Validators ====================

// NoOpValidator doesn't validate.
type NoOpValidator struct{}

func (v *NoOpValidator) Validate(entry *ChannelWriteEntry) error {
	return nil
}

// TypeWriteValidator validates value types.
type TypeWriteValidator struct {
	types map[string]interface{}
}

// NewTypeWriteValidator creates a validator for value types.
func NewTypeWriteValidator(types map[string]interface{}) *TypeWriteValidator {
	return &TypeWriteValidator{types: types}
}

func (v *TypeWriteValidator) Validate(entry *ChannelWriteEntry) error {
	if expectedType, ok := v.types[entry.Channel]; ok {
		if entry.Value != nil && fmt.Sprintf("%T", entry.Value) != fmt.Sprintf("%T", expectedType) {
			return &WriteValidationError{
				Channel: entry.Channel,
				Message: fmt.Sprintf("expected type %T, got %T", expectedType, entry.Value),
			}
		}
	}
	return nil
}

// NonNullWriteValidator ensures values are not nil.
type NonNullWriteValidator struct {
	whitelist []string
}

// NewNonNullWriteValidator creates a validator that rejects nil values.
func NewNonNullWriteValidator(whitelist ...string) *NonNullWriteValidator {
	return &NonNullWriteValidator{whitelist: whitelist}
}

func (v *NonNullWriteValidator) Validate(entry *ChannelWriteEntry) error {
	for _, channel := range v.whitelist {
		if entry.Channel == channel {
			return nil
		}
	}

	if entry.Value == nil {
		return &WriteValidationError{
			Channel: entry.Channel,
			Message: "value cannot be nil",
		}
	}
	return nil
}

// LengthWriteValidator validates slice/string lengths.
type LengthWriteValidator struct {
	minLengths map[string]int
	maxLengths map[string]int
}

// NewLengthWriteValidator creates a validator for lengths.
func NewLengthWriteValidator(minLengths, maxLengths map[string]int) *LengthWriteValidator {
	return &LengthWriteValidator{
		minLengths: minLengths,
		maxLengths: maxLengths,
	}
}

func (v *LengthWriteValidator) Validate(entry *ChannelWriteEntry) error {
	var length int

	switch val := entry.Value.(type) {
	case []interface{}:
		length = len(val)
	case string:
		length = len(val)
	case map[string]interface{}:
		length = len(val)
	default:
		return nil
	}

	if min, ok := v.minLengths[entry.Channel]; ok && length < min {
		return &WriteValidationError{
			Channel: entry.Channel,
			Message: fmt.Sprintf("length %d is less than minimum %d", length, min),
		}
	}

	if max, ok := v.maxLengths[entry.Channel]; ok && length > max {
		return &WriteValidationError{
			Channel: entry.Channel,
			Message: fmt.Sprintf("length %d exceeds maximum %d", length, max),
		}
	}

	return nil
}

// ==================== Write Batches ====================

// WriteBatch represents a batch of write operations.
type WriteBatch struct {
	entries []*ChannelWriteEntry
}

// NewWriteBatch creates a new write batch.
func NewWriteBatch() *WriteBatch {
	return &WriteBatch{
		entries: make([]*ChannelWriteEntry, 0),
	}
}

// Add adds an entry to the batch.
func (b *WriteBatch) Add(entry *ChannelWriteEntry) {
	b.entries = append(b.entries, entry)
}

// WriteTo adds a simple write to the batch.
func (b *WriteBatch) WriteTo(channel string, value interface{}) {
	b.Add(&ChannelWriteEntry{
		Channel:   channel,
		Value:     value,
		Overwrite: false,
	})
}

// Overwrite adds an overwrite to the batch.
func (b *WriteBatch) Overwrite(channel string, value interface{}) {
	b.Add(&ChannelWriteEntry{
		Channel:   channel,
		Value:     value,
		Overwrite: true,
	})
}

// Entries returns all entries in the batch.
func (b *WriteBatch) Entries() []*ChannelWriteEntry {
	return b.entries
}

// Size returns the number of entries in the batch.
func (b *WriteBatch) Size() int {
	return len(b.entries)
}

// Clear clears all entries.
func (b *WriteBatch) Clear() {
	b.entries = make([]*ChannelWriteEntry, 0)
}

// ==================== Write Context ====================

// WriteContext represents the context of a write operation.
type WriteContext struct {
	Node    string
	Step    int
	Writer  *ChannelWrite
	Batches map[string]*WriteBatch
}

// NewWriteContext creates a new write context.
func NewWriteContext(node string, step int, writer *ChannelWrite) *WriteContext {
	return &WriteContext{
		Node:    node,
		Step:    step,
		Writer:  writer,
		Batches: make(map[string]*WriteBatch),
	}
}

// CreateBatch creates a new named batch.
func (wc *WriteContext) CreateBatch(name string) *WriteBatch {
	batch := NewWriteBatch()
	wc.Batches[name] = batch
	return batch
}

// GetBatch gets an existing batch.
func (wc *WriteContext) GetBatch(name string) *WriteBatch {
	return wc.Batches[name]
}

// Flush writes all batches to the main writer.
func (wc *WriteContext) Flush(ctx context.Context) (map[string]bool, error) {
	for _, batch := range wc.Batches {
		wc.Writer.AddEntries(batch.Entries()...)
	}
	return wc.Writer.Write(ctx)
}

// ==================== Errors ====================

// WriteValidationError represents a validation error.
type WriteValidationError struct {
	Channel string
	Message string
}

func (e *WriteValidationError) Error() string {
	return fmt.Sprintf("write validation error for channel %s: %s", e.Channel, e.Message)
}

// WriteSkipError indicates an entry should be skipped.
type WriteSkipError struct {
	Channel string
}

func (e *WriteSkipError) Error() string {
	return fmt.Sprintf("write skipped for channel %s", e.Channel)
}

// IsWriteSkipError checks if an error is a skip error.
func IsWriteSkipError(err error) bool {
	_, ok := err.(*WriteSkipError)
	return ok
}
