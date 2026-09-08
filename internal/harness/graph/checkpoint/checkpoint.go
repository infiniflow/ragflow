// Package checkpoint provides production-grade checkpoint management.
package checkpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"ragflow/internal/harness/graph/constants"
	"ragflow/internal/harness/graph/types"
)

// BaseCheckpointer is the canonical checkpoint persistence interface.
// It provides the standard Get/Put/List contract used by graph and pregel packages.
// All concrete checkpoint implementations (MemorySaver, SqliteSaver, PostgresSaver)
// satisfy this interface.
type BaseCheckpointer interface {
	Get(ctx context.Context, config map[string]interface{}) (map[string]interface{}, error)
	Put(ctx context.Context, config map[string]interface{}, checkpoint map[string]interface{}) error
	List(ctx context.Context, config map[string]interface{}, limit int) ([]map[string]interface{}, error)
}

// CheckpointMetadata contains metadata about a checkpoint.
type CheckpointMetadata struct {
	// ID is the unique identifier for this checkpoint
	ID string
	// ParentID is the ID of the parent checkpoint
	ParentID string
	// ThreadID is the thread this checkpoint belongs to
	ThreadID string
	// Step is the step number when this checkpoint was created
	Step int
	// CreatedAt is the timestamp when this checkpoint was created
	CreatedAt time.Time
	// Source indicates where this checkpoint came from
	Source CheckpointSource
	// Custom metadata
	Metadata map[string]interface{}
}

// CheckpointSource indicates the source of a checkpoint.
type CheckpointSource string

const (
	SourceNode      CheckpointSource = "node"      // Created after node execution
	SourceEdge      CheckpointSource = "edge"      // Created after edge traversal
	SourceInterrupt CheckpointSource = "interrupt" // Created on interrupt
	SourceManual    CheckpointSource = "manual"    // Manually created
	SourceResume    CheckpointSource = "resume"    // Created on resume
)

// PendingWrite represents a write that hasn't been applied yet.
type PendingWrite struct {
	// Channel is the channel to write to
	Channel string
	// Value is the value to write
	Value interface{}
	// Overwrite indicates if this should bypass reducers
	Overwrite bool
	// Node that initiated this write
	Node string
	// Timestamp when this write was created
	Timestamp time.Time
	// TaskID is the ID of the task that created this write
	TaskID string
}

// NewPendingWrite creates a new pending write.
func NewPendingWrite(channel string, value interface{}, overwrite bool, node, taskID string) *PendingWrite {
	return &PendingWrite{
		Channel:   channel,
		Value:     value,
		Overwrite: overwrite,
		Node:      node,
		Timestamp: time.Now(),
		TaskID:    taskID,
	}
}

// Checkpoint represents a complete checkpoint with versioning.
type Checkpoint struct {
	// ID is the unique identifier
	ID string
	// Version is the checkpoint version (monotonically increasing)
	Version int
	// ParentID is the ID of the parent checkpoint (for lineage tracking)
	ParentID string
	// ChannelVersions tracks versions of each channel
	ChannelVersions map[string]int
	// VersionsSeen tracks which channel versions each node has seen
	VersionsSeen map[string]map[string]int
	// State is the current state
	State map[string]interface{}
	// PendingWrites are writes that haven't been applied yet
	PendingWrites []PendingWrite
	// Metadata about this checkpoint
	Metadata CheckpointMetadata
}

// NewCheckpoint creates a new checkpoint.
func NewCheckpoint(threadID string, step int) *Checkpoint {
	id := uuid.New().String()
	return &Checkpoint{
		ID:              id,
		Version:         0,
		ChannelVersions: make(map[string]int),
		VersionsSeen:    make(map[string]map[string]int),
		State:           make(map[string]interface{}),
		PendingWrites:   make([]PendingWrite, 0),
		Metadata: CheckpointMetadata{
			ID:        id,
			ThreadID:  threadID,
			Step:      step,
			CreatedAt: time.Now(),
			Source:    SourceNode,
			Metadata:  make(map[string]interface{}),
		},
	}
}

// Clone creates a deep copy of the checkpoint.
func (c *Checkpoint) Clone() *Checkpoint {
	newID := uuid.New().String()
	clone := &Checkpoint{
		ID:              newID,
		Version:         c.Version,
		ParentID:        c.ID,
		ChannelVersions: make(map[string]int, len(c.ChannelVersions)),
		VersionsSeen:    make(map[string]map[string]int, len(c.VersionsSeen)),
		State:           make(map[string]interface{}, len(c.State)),
		PendingWrites:   make([]PendingWrite, len(c.PendingWrites)),
		Metadata: CheckpointMetadata{
			ID:        newID, // same ID as the clone
			ParentID:  c.ID,
			ThreadID:  c.Metadata.ThreadID,
			Step:      c.Metadata.Step,
			CreatedAt: time.Now(),
			Source:    c.Metadata.Source,
			Metadata:  make(map[string]interface{}),
		},
	}

	// Copy channel versions
	for k, v := range c.ChannelVersions {
		clone.ChannelVersions[k] = v
	}

	// Copy versions seen
	for node, versions := range c.VersionsSeen {
		clone.VersionsSeen[node] = make(map[string]int)
		for k, v := range versions {
			clone.VersionsSeen[node][k] = v
		}
	}

	// Copy state
	for k, v := range c.State {
		clone.State[k] = deepCopy(v)
	}

	// Deep-copy pending writes — each Value must be independently copied so that
	// mutating the clone's Value never affects the original.
	clone.PendingWrites = make([]PendingWrite, len(c.PendingWrites))
	for i, pw := range c.PendingWrites {
		clone.PendingWrites[i] = pw
		clone.PendingWrites[i].Value = deepCopy(pw.Value)
	}

	// Copy metadata — deep copy to avoid shared maps/slices.
	for k, v := range c.Metadata.Metadata {
		clone.Metadata.Metadata[k] = deepCopy(v)
	}

	return clone
}

// IncrementChannel increments the version of a channel.
func (c *Checkpoint) IncrementChannel(channel string) {
	c.ChannelVersions[channel]++
}

// MarkSeen marks that a node has seen a channel's current version.
func (c *Checkpoint) MarkSeen(node, channel string) {
	if _, ok := c.VersionsSeen[node]; !ok {
		c.VersionsSeen[node] = make(map[string]int)
	}
	c.VersionsSeen[node][channel] = c.ChannelVersions[channel]
}

// HasSeen checks if a node has seen a channel's version.
func (c *Checkpoint) HasSeen(node, channel string) bool {
	if versions, ok := c.VersionsSeen[node]; ok {
		if version, ok := versions[channel]; ok {
			return version == c.ChannelVersions[channel]
		}
	}
	return false
}

// AddPendingWrite adds a pending write.
func (c *Checkpoint) AddPendingWrite(channel string, value interface{}, overwrite bool, node string) {
	c.PendingWrites = append(c.PendingWrites, PendingWrite{
		Channel:   channel,
		Value:     value,
		Overwrite: overwrite,
		Node:      node,
	})
}

// ClearPendingWrites clears all pending writes.
func (c *Checkpoint) ClearPendingWrites() {
	c.PendingWrites = make([]PendingWrite, 0)
}

// ToMap converts the checkpoint to a map for storage.
func (c *Checkpoint) ToMap() map[string]interface{} {
	result := make(map[string]interface{})

	result["id"] = c.ID
	result["version"] = c.Version
	result["parent_id"] = c.ParentID

	channelVersions := make(map[string]int)
	for k, v := range c.ChannelVersions {
		channelVersions[k] = v
	}
	result["channel_versions"] = channelVersions

	versionsSeen := make(map[string]map[string]int)
	for node, versions := range c.VersionsSeen {
		versionsSeen[node] = make(map[string]int)
		for k, v := range versions {
			versionsSeen[node][k] = v
		}
	}
	result["versions_seen"] = versionsSeen

	state := make(map[string]interface{})
	for k, v := range c.State {
		state[k] = deepCopy(v)
	}
	result["state"] = state

	pendingWrites := make([]map[string]interface{}, len(c.PendingWrites))
	for i, pw := range c.PendingWrites {
		pendingWrites[i] = map[string]interface{}{
			"channel":   pw.Channel,
			"value":     pw.Value,
			"overwrite": pw.Overwrite,
			"node":      pw.Node,
		}
	}
	result["pending_writes"] = pendingWrites

	metadata := map[string]interface{}{
		"id":         c.Metadata.ID,
		"parent_id":  c.Metadata.ParentID,
		"thread_id":  c.Metadata.ThreadID,
		"step":       c.Metadata.Step,
		"created_at": c.Metadata.CreatedAt.Format(time.RFC3339Nano),
		"source":     string(c.Metadata.Source),
	}
	for k, v := range c.Metadata.Metadata {
		metadata[k] = v
	}
	result["metadata"] = metadata

	return result
}

// FromMap creates a checkpoint from a map.
func FromMap(data map[string]interface{}) (*Checkpoint, error) {
	c := &Checkpoint{
		ID:              getString(data, "id"),
		ParentID:        getString(data, "parent_id"),
		Version:         getInt(data, "version", 0),
		ChannelVersions: make(map[string]int),
		VersionsSeen:    make(map[string]map[string]int),
		State:           make(map[string]interface{}),
		PendingWrites:   make([]PendingWrite, 0),
	}

	// Parse channel versions
	if cv, ok := data["channel_versions"].(map[string]interface{}); ok {
		for k, v := range cv {
			if num, ok := v.(float64); ok {
				c.ChannelVersions[k] = int(num)
			}
		}
	}

	// Parse versions seen
	if vs, ok := data["versions_seen"].(map[string]interface{}); ok {
		for node, versions := range vs {
			if vMap, ok := versions.(map[string]interface{}); ok {
				c.VersionsSeen[node] = make(map[string]int)
				for k, v := range vMap {
					if num, ok := v.(float64); ok {
						c.VersionsSeen[node][k] = int(num)
					}
				}
			}
		}
	}

	// Parse state
	if state, ok := data["state"].(map[string]interface{}); ok {
		for k, v := range state {
			c.State[k] = deepCopy(v)
		}
	}

	// Parse pending writes
	if pws, ok := data["pending_writes"].([]interface{}); ok {
		for _, pw := range pws {
			if pwMap, ok := pw.(map[string]interface{}); ok {
				c.PendingWrites = append(c.PendingWrites, PendingWrite{
					Channel:   getString(pwMap, "channel"),
					Overwrite: getBool(pwMap, "overwrite", false),
					Node:      getString(pwMap, "node"),
					Value:     pwMap["value"],
				})
			}
		}
	}

	// Parse metadata
	if md, ok := data["metadata"].(map[string]interface{}); ok {
		c.Metadata = CheckpointMetadata{
			ID:       getString(md, "id"),
			ParentID: getString(md, "parent_id"),
			ThreadID: getString(md, "thread_id"),
			Step:     getInt(md, "step", 0),
			Source:   CheckpointSource(getString(md, "source")),
			Metadata: make(map[string]interface{}),
		}

		if createdAtStr := getString(md, "created_at"); createdAtStr != "" {
			if t, err := time.Parse(time.RFC3339Nano, createdAtStr); err == nil {
				c.Metadata.CreatedAt = t
			}
		}

		// Copy custom metadata
		for k, v := range md {
			if k != "id" && k != "parent_id" && k != "thread_id" && k != "step" && k != "created_at" && k != "source" {
				c.Metadata.Metadata[k] = v
			}
		}
	}

	return c, nil
}

// Helper functions
func getString(data map[string]any, key string) string {
	if val, ok := data[key].(string); ok {
		return val
	}
	return ""
}

func getInt(data map[string]any, key string, defaultVal int) int {
	if val, ok := data[key].(float64); ok {
		return int(val)
	}
	return defaultVal
}

func getBool(data map[string]any, key string, defaultVal bool) bool {
	if val, ok := data[key].(bool); ok {
		return val
	}
	return defaultVal
}

// CheckpointTuple represents a checkpoint with its parent and version information.
type CheckpointTuple struct {
	// Config is the configuration used to fetch this checkpoint
	Config *types.RunnableConfig
	// Checkpoint is the checkpoint data
	Checkpoint *Checkpoint
	// ParentConfig is the configuration used to fetch the parent checkpoint
	ParentConfig *types.RunnableConfig
	// Metadata about this checkpoint
	Metadata map[string]interface{}
}

// NewCheckpointTuple creates a new checkpoint tuple.
func NewCheckpointTuple(config *types.RunnableConfig, checkpoint *Checkpoint, parentConfig *types.RunnableConfig) *CheckpointTuple {
	if config == nil {
		config = types.NewRunnableConfig()
	}

	metadata := make(map[string]interface{})
	if checkpoint != nil {
		metadata["id"] = checkpoint.ID
		metadata["version"] = checkpoint.Version
		metadata["thread_id"] = checkpoint.Metadata.ThreadID
		metadata["step"] = checkpoint.Metadata.Step
		metadata["source"] = string(checkpoint.Metadata.Source)
		metadata["created_at"] = checkpoint.Metadata.CreatedAt
	}

	return &CheckpointTuple{
		Config:       config,
		Checkpoint:   checkpoint,
		ParentConfig: parentConfig,
		Metadata:     metadata,
	}
}

// PutWrites represents a set of writes to apply to a checkpoint.
type PutWrites struct {
	// Config is the configuration for the checkpoint
	Config *types.RunnableConfig
	// Writes are the writes to apply
	Writes []PendingWrite
	// TaskID is the ID of the task that created these writes
	TaskID string
}

// NewPutWrites creates a new PutWrites.
func NewPutWrites(config *types.RunnableConfig, writes []PendingWrite, taskID string) *PutWrites {
	return &PutWrites{
		Config: config,
		Writes: writes,
		TaskID: taskID,
	}
}

// CheckpointListFilter represents filter criteria for listing checkpoints.
type CheckpointListFilter struct {
	ThreadID string
	Limit    int
}

// CheckpointListResponse represents a checkpoint in list results.
type CheckpointListResponse struct {
	ID        string
	ThreadID  string
	Version   int
	CreatedAt time.Time
	Metadata  map[string]interface{}
}

// LineageEntry represents an entry in a checkpoint lineage.
type LineageEntry struct {
	Checkpoint *Checkpoint
	Metadata   map[string]interface{}
}

// VersionConflictError is raised when there is a version conflict.
type VersionConflictError struct {
	CurrentVersion  int
	ExpectedVersion int
	CheckpointID    string
	ThreadID        string
}

func (e *VersionConflictError) Error() string {
	return fmt.Sprintf(
		"version conflict: expected version %d but found %d for checkpoint %s in thread %s",
		e.ExpectedVersion,
		e.CurrentVersion,
		e.CheckpointID,
		e.ThreadID,
	)
}

// CheckpointManager manages checkpoints with versioning and concurrency control.
type CheckpointManager struct {
	mu          sync.RWMutex
	checkpoints map[string][]*Checkpoint // threadID -> checkpoints
	maxVersions int                      // Maximum versions to keep per thread
}

// NewCheckpointManager creates a new checkpoint manager.
func NewCheckpointManager(maxVersions int) *CheckpointManager {
	if maxVersions <= 0 {
		maxVersions = constants.DefaultCheckpointMaxVersions
	}

	return &CheckpointManager{
		checkpoints: make(map[string][]*Checkpoint),
		maxVersions: maxVersions,
	}
}

// RunnableConfigToMap converts a types.RunnableConfig to the map[string]interface{}
// format used by BaseCheckpointer implementations. This provides a single adaptation
// point so callers do not need to manually construct config maps.
func RunnableConfigToMap(cfg *types.RunnableConfig) map[string]interface{} {
	if cfg == nil {
		return map[string]interface{}{}
	}
	result := map[string]interface{}{
		"thread_id":     cfg.ThreadID,
		"checkpoint_id": cfg.GetOrEmpty("checkpoint_id"),
		"checkpoint_ns": cfg.GetOrEmpty("checkpoint_ns"),
	}
	return result
}

// Save saves a checkpoint with version conflict detection.
func (cm *CheckpointManager) Save(ctx context.Context, checkpoint *Checkpoint) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	threadID := checkpoint.Metadata.ThreadID

	// Check for version conflict whenever a thread already has checkpoints.
	// Two independent checks:
	//   1. ParentID mismatch — someone else wrote to this thread since we loaded.
	//   2. Version out of sequence — the caller's declared version doesn't follow the latest.
	if checkpoints := cm.checkpoints[threadID]; len(checkpoints) > 0 {
		latest := checkpoints[len(checkpoints)-1]

		// Check 1: ParentID must match the latest checkpoint.
		if checkpoint.ParentID != "" && latest.ID != checkpoint.ParentID {
			return &VersionConflictError{
				CurrentVersion:  checkpoint.Version,
				ExpectedVersion: latest.Version + 1,
				CheckpointID:    checkpoint.ID,
				ThreadID:        threadID,
			}
		}

		// Check 2: Version must be sequential. Only enforce when the caller
		// explicitly set a version (> 0); Version == 0 means the checkpoint
		// was created by NewCheckpoint (which always sets Version=0) and
		// the caller didn't intend to participate in version conflict detection.
		if checkpoint.Version > 0 && latest.Version != checkpoint.Version-1 {
			return &VersionConflictError{
				CurrentVersion:  checkpoint.Version,
				ExpectedVersion: latest.Version + 1,
				CheckpointID:    checkpoint.ID,
				ThreadID:        threadID,
			}
		}
	}

	// Append to thread's checkpoint history
	cm.checkpoints[threadID] = append(cm.checkpoints[threadID], checkpoint)

	// Trim if we have too many versions
	if len(cm.checkpoints[threadID]) > cm.maxVersions {
		cm.checkpoints[threadID] = cm.checkpoints[threadID][len(cm.checkpoints[threadID])-cm.maxVersions:]
	}

	return nil
}

// PutWrites applies writes to a checkpoint with version-chain conflict detection.
// Uses a monotonic checkpoint version chain instead of a transient activeWrites map
// to avoid the TOCTOU race (activeWrites is cleared on success, allowing a stale
// concurrent writer to slip past undetected).
func (cm *CheckpointManager) PutWrites(ctx context.Context, config *types.RunnableConfig, writes []PendingWrite, taskID string) error {
	threadID := config.ThreadID
	if threadID == "" {
		return fmt.Errorf("thread ID is required for put_writes")
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Load the current checkpoint for this thread
	checkpoints := cm.checkpoints[threadID]
	if len(checkpoints) == 0 {
		return fmt.Errorf("no checkpoint found for thread %s", threadID)
	}

	current := checkpoints[len(checkpoints)-1]

	// Version-chain conflict detection:
	// The caller MUST provide the checkpoint_id they loaded (via config).
	// If missing, the write is not properly scoped and cannot be validated.
	// If it doesn't match the actual latest checkpoint, another writer
	// has already committed and this is a stale write — reject both cases.
	callerID := config.GetOrEmpty("checkpoint_id")
	if callerID == "" {
		return fmt.Errorf("checkpoint_id is required for put_writes on thread %s", threadID)
	}
	if callerID != current.ID {
		return &VersionConflictError{
			CurrentVersion:  current.Version,
			ExpectedVersion: current.Version + 1,
			CheckpointID:    current.ID,
			ThreadID:        threadID,
		}
	}

	// Create a new checkpoint with the writes applied
	newCheckpoint := current.Clone()
	newCheckpoint.Version = current.Version + 1
	newCheckpoint.ParentID = current.ID

	// Apply writes
	for _, write := range writes {
		newCheckpoint.State[write.Channel] = write.Value
		newCheckpoint.IncrementChannel(write.Channel)
	}

	// Save the new checkpoint
	cm.checkpoints[threadID] = append(cm.checkpoints[threadID], newCheckpoint)

	return nil
}

// Load loads the latest checkpoint for a thread.
func (cm *CheckpointManager) Load(ctx context.Context, threadID string) (*Checkpoint, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	checkpoints := cm.checkpoints[threadID]
	if len(checkpoints) == 0 {
		return nil, nil
	}

	return checkpoints[len(checkpoints)-1].Clone(), nil
}

// LoadByCheckpointID loads a specific checkpoint by ID.
func (cm *CheckpointManager) LoadByCheckpointID(ctx context.Context, checkpointID string) (*Checkpoint, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	for _, checkpoints := range cm.checkpoints {
		for _, cp := range checkpoints {
			if cp.ID == checkpointID {
				return cp.Clone(), nil
			}
		}
	}

	return nil, fmt.Errorf("checkpoint not found: %s", checkpointID)
}

// List lists checkpoints for a thread.
func (cm *CheckpointManager) List(ctx context.Context, threadID string, limit int) ([]*Checkpoint, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	checkpoints := cm.checkpoints[threadID]
	if len(checkpoints) == 0 {
		return nil, nil
	}

	if limit <= 0 || limit > len(checkpoints) {
		limit = len(checkpoints)
	}

	// Return most recent checkpoints first
	result := make([]*Checkpoint, limit)
	start := len(checkpoints) - limit
	for i := 0; i < limit; i++ {
		result[i] = checkpoints[start+i].Clone()
	}

	return result, nil
}

// GetTuple loads a checkpoint and its parent as a tuple.
func (cm *CheckpointManager) GetTuple(ctx context.Context, config *types.RunnableConfig) (*CheckpointTuple, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	threadID := config.ThreadID
	if threadID == "" {
		return nil, fmt.Errorf("thread ID is required")
	}

	checkpoints := cm.checkpoints[threadID]
	if len(checkpoints) == 0 {
		return NewCheckpointTuple(config, nil, nil), nil
	}

	// Get the latest checkpoint
	latest := checkpoints[len(checkpoints)-1].Clone()

	// Get parent checkpoint
	var parent *Checkpoint
	if len(checkpoints) > 1 {
		parent = checkpoints[len(checkpoints)-2].Clone()
	}

	// Create parent config
	var parentConfig *types.RunnableConfig
	if parent != nil {
		parentConfig = types.NewRunnableConfig()
		parentConfig.ThreadID = threadID
		parentConfig.Set("checkpoint_id", parent.ID)
	}

	return NewCheckpointTuple(config, latest, parentConfig), nil
}

// GetTupleByVersion gets a checkpoint tuple by version.
func (cm *CheckpointManager) GetTupleByVersion(ctx context.Context, config *types.RunnableConfig, version int) (*CheckpointTuple, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	threadID := config.ThreadID
	if threadID == "" {
		return nil, fmt.Errorf("thread ID is required")
	}

	checkpoints := cm.checkpoints[threadID]
	var target *Checkpoint
	var parent *Checkpoint
	var parentConfig *types.RunnableConfig

	for i, cp := range checkpoints {
		if cp.Version == version {
			target = cp.Clone()
			// Get parent checkpoint
			if i > 0 {
				parent = checkpoints[i-1].Clone()
				parentConfig = types.NewRunnableConfig()
				parentConfig.ThreadID = threadID
				parentConfig.Set("checkpoint_id", parent.ID)
			}
			break
		}
	}

	if target == nil {
		return nil, fmt.Errorf("version not found: %d", version)
	}

	return NewCheckpointTuple(config, target, parentConfig), nil
}

// GetLineage gets the lineage (history) of checkpoints for a thread.
func (cm *CheckpointManager) GetLineage(ctx context.Context, threadID string, limit int) ([]*CheckpointTuple, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	checkpoints := cm.checkpoints[threadID]
	if len(checkpoints) == 0 {
		return nil, nil
	}

	if limit <= 0 || limit > len(checkpoints) {
		limit = len(checkpoints)
	}

	result := make([]*CheckpointTuple, 0, limit)
	for i := len(checkpoints) - limit; i < len(checkpoints); i++ {
		cp := checkpoints[i].Clone()

		config := types.NewRunnableConfig()
		config.ThreadID = threadID
		config.Set("checkpoint_id", cp.ID)

		var parentConfig *types.RunnableConfig
		if i > 0 {
			parentConfig = types.NewRunnableConfig()
			parentConfig.ThreadID = threadID
			parentConfig.Set("checkpoint_id", checkpoints[i-1].ID)
		}

		result = append(result, NewCheckpointTuple(config, cp, parentConfig))
	}

	return result, nil
}

// GetVersion gets a specific version of a checkpoint.
func (cm *CheckpointManager) GetVersion(ctx context.Context, threadID string, version int) (*Checkpoint, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	checkpoints := cm.checkpoints[threadID]
	for _, cp := range checkpoints {
		if cp.Version == version {
			return cp.Clone(), nil
		}
	}

	return nil, fmt.Errorf("version not found: %d", version)
}

// Delete deletes a checkpoint.
func (cm *CheckpointManager) Delete(ctx context.Context, checkpointID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for threadID, checkpoints := range cm.checkpoints {
		for i, cp := range checkpoints {
			if cp.ID == checkpointID {
				// Remove from slice
				cm.checkpoints[threadID] = append(checkpoints[:i], checkpoints[i+1:]...)
				return nil
			}
		}
	}

	return fmt.Errorf("checkpoint not found: %s", checkpointID)
}

// ClearThread clears all checkpoints for a thread.
func (cm *CheckpointManager) ClearThread(ctx context.Context, threadID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	delete(cm.checkpoints, threadID)
	return nil
}

// ClearAll clears all checkpoints.
func (cm *CheckpointManager) ClearAll(ctx context.Context) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.checkpoints = make(map[string][]*Checkpoint)
	return nil
}

// deepCopy creates a deep copy of a value using JSON serialization.
// Maps and slices are handled via dedicated deep-copy helpers.
// For other types, JSON marshal/unmarshal provides a reliable deep copy
// (converting numbers to float64 consistently).
// On serialization failure (channels, functions, circular references),
// returns nil rather than a shared reference that would silently
// propagate mutations between checkpoint and its clone.
func deepCopy(val any) any {
	if val == nil {
		return nil
	}

	// Handle common collection types with dedicated helpers
	if m, ok := val.(map[string]any); ok {
		return deepCopyMap(m)
	}
	if slice, ok := val.([]any); ok {
		return deepCopySlice(slice)
	}

	// For all other types, JSON round-trip provides reliable deep copy.
	data, err := json.Marshal(val)
	if err != nil {
		return nil
	}
	var result any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}

// deepCopyMap creates a deep copy of a map.
func deepCopyMap(src map[string]any) map[string]any {
	result := make(map[string]any, len(src))
	for key, val := range src {
		result[key] = deepCopy(val)
	}
	return result
}

// deepCopySlice creates a deep copy of a slice.
func deepCopySlice(slice []any) []any {
	result := make([]any, len(slice))
	for i, val := range slice {
		result[i] = deepCopy(val)
	}
	return result
}
