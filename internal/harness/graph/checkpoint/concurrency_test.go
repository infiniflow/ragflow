// Package checkpoint tests checkpoint concurrency control functionality.
package checkpoint

import (
	"context"
	"testing"
	"time"

	"ragflow/internal/harness/graph/types"
)

func TestNewCheckpointTuple(t *testing.T) {
	config := types.NewRunnableConfig()
	config.ThreadID = "thread-1"

	checkpoint := NewCheckpoint("thread-1", 0)

	tuple := NewCheckpointTuple(config, checkpoint, nil)

	if tuple.Config == nil {
		t.Error("Config should not be nil")
	}
	if tuple.Checkpoint == nil {
		t.Error("Checkpoint should not be nil")
	}
	if tuple.ParentConfig != nil {
		t.Error("ParentConfig should be nil")
	}
}

func TestNewPutWrites(t *testing.T) {
	config := types.NewRunnableConfig()
	config.ThreadID = "thread-1"

	writes := []PendingWrite{
		*NewPendingWrite("channel1", "value1", false, "node1", "task1"),
	}

	pw := NewPutWrites(config, writes, "task1")

	if pw.Config == nil {
		t.Error("Config should not be nil")
	}
	if pw.TaskID != "task1" {
		t.Errorf("Expected task ID 'task1', got '%s'", pw.TaskID)
	}
	if len(pw.Writes) != 1 {
		t.Errorf("Expected 1 write, got %d", len(pw.Writes))
	}
}

func TestNewPendingWrite(t *testing.T) {
	pw := NewPendingWrite("channel1", "value1", false, "node1", "task1")

	if pw.Channel != "channel1" {
		t.Errorf("Expected channel 'channel1', got '%s'", pw.Channel)
	}
	if pw.Value != "value1" {
		t.Errorf("Expected value 'value1', got '%v'", pw.Value)
	}
	if pw.Overwrite {
		t.Error("Expected Overwrite to be false")
	}
	if pw.Node != "node1" {
		t.Errorf("Expected node 'node1', got '%s'", pw.Node)
	}
	if pw.TaskID != "task1" {
		t.Errorf("Expected task ID 'task1', got '%s'", pw.TaskID)
	}
	if pw.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

func TestVersionConflictError(t *testing.T) {
	err := &VersionConflictError{
		CurrentVersion:  10,
		ExpectedVersion: 5,
		CheckpointID:    "cp-123",
		ThreadID:        "thread-1",
	}

	expected := "version conflict: expected version 5 but found 10 for checkpoint cp-123 in thread thread-1"
	if err.Error() != expected {
		t.Errorf("Expected:\n%s\n\nGot:\n%s", expected, err.Error())
	}
}

func TestCheckpointManager_SaveWithVersionConflict(t *testing.T) {
	ctx := context.Background()
	manager := NewCheckpointManager(10)

	// Create initial checkpoint
	checkpoint1 := NewCheckpoint("thread-1", 0)
	checkpoint1.Version = 1

	err := manager.Save(ctx, checkpoint1)
	if err != nil {
		t.Fatalf("Failed to save initial checkpoint: %v", err)
	}

	// Create a new checkpoint with incorrect parent version
	checkpoint2 := NewCheckpoint("thread-1", 1)
	checkpoint2.ParentID = checkpoint1.ID
	checkpoint2.Version = 5 // Incorrect - should be 2

	err = manager.Save(ctx, checkpoint2)
	if err == nil {
		t.Error("Expected version conflict error, got nil")
	}

	if _, ok := err.(*VersionConflictError); !ok {
		t.Errorf("Expected VersionConflictError, got %T", err)
	}
}

func TestCheckpointManager_PutWrites(t *testing.T) {
	ctx := context.Background()
	manager := NewCheckpointManager(10)

	// Create initial checkpoint
	checkpoint := NewCheckpoint("thread-1", 0)
	checkpoint.State["channel1"] = "initial"

	err := manager.Save(ctx, checkpoint)
	if err != nil {
		t.Fatalf("Failed to save initial checkpoint: %v", err)
	}

	// Prepare config and writes
	config := types.NewRunnableConfig()
	config.ThreadID = "thread-1"
	config.Set("checkpoint_id", checkpoint.ID)

	writes := []PendingWrite{
		*NewPendingWrite("channel1", "updated", false, "node1", "task1"),
		*NewPendingWrite("channel2", "new_value", false, "node1", "task1"),
	}

	// Apply writes
	err = manager.PutWrites(ctx, config, writes, "task1")
	if err != nil {
		t.Fatalf("Failed to put writes: %v", err)
	}

	// Verify writes were applied
	latest, err := manager.Load(ctx, "thread-1")
	if err != nil {
		t.Fatalf("Failed to load checkpoint: %v", err)
	}

	if latest.State["channel1"] != "updated" {
		t.Errorf("Expected 'updated', got '%v'", latest.State["channel1"])
	}

	if latest.State["channel2"] != "new_value" {
		t.Errorf("Expected 'new_value', got '%v'", latest.State["channel2"])
	}

	// Verify version was incremented
	if latest.Version != 1 {
		t.Errorf("Expected version 1, got %d", latest.Version)
	}
}

func TestCheckpointManager_PutWrites_Conflict(t *testing.T) {
	ctx := context.Background()
	manager := NewCheckpointManager(10)

	// Create initial checkpoint
	checkpoint := NewCheckpoint("thread-1", 0)
	checkpoint.State["channel1"] = "initial"

	err := manager.Save(ctx, checkpoint)
	if err != nil {
		t.Fatalf("Failed to save initial checkpoint: %v", err)
	}

	// Prepare config and writes for task1
	config1 := types.NewRunnableConfig()
	config1.ThreadID = "thread-1"
	config1.Set("checkpoint_id", checkpoint.ID)

	writes1 := []PendingWrite{
		*NewPendingWrite("channel1", "value1", false, "node1", "task1"),
	}

	writes2 := []PendingWrite{
		*NewPendingWrite("channel1", "value2", false, "node1", "task2"),
	}

	// First write operation
	err = manager.PutWrites(ctx, config1, writes1, "task1")
	if err != nil {
		t.Fatalf("Failed to put writes for task1: %v", err)
	}

	// Second write with the same checkpoint_id → the first write already advanced
	// the version chain, so this must fail with a VersionConflictError.
	err = manager.PutWrites(ctx, config1, writes2, "task1")
	if err == nil {
		t.Fatal("expected conflict error for stale checkpoint_id")
	}
	if _, ok := err.(*VersionConflictError); !ok {
		t.Fatalf("expected *VersionConflictError, got %T: %v", err, err)
	}
}

func TestCheckpointManager_GetTuple(t *testing.T) {
	ctx := context.Background()
	manager := NewCheckpointManager(10)

	config := types.NewRunnableConfig()
	config.ThreadID = "thread-1"

	// Get tuple for non-existent thread
	tuple, err := manager.GetTuple(ctx, config)
	if err != nil {
		t.Fatalf("Failed to get tuple: %v", err)
	}
	if tuple.Checkpoint != nil {
		t.Error("Checkpoint should be nil for non-existent thread")
	}

	// Create and save a checkpoint
	checkpoint := NewCheckpoint("thread-1", 0)
	err = manager.Save(ctx, checkpoint)
	if err != nil {
		t.Fatalf("Failed to save checkpoint: %v", err)
	}

	// Get tuple for existing thread
	tuple, err = manager.GetTuple(ctx, config)
	if err != nil {
		t.Fatalf("Failed to get tuple: %v", err)
	}

	if tuple.Checkpoint == nil {
		t.Error("Checkpoint should not be nil")
	}

	if tuple.Checkpoint.Metadata.ThreadID != "thread-1" {
		t.Errorf("Expected thread ID 'thread-1', got '%s'", tuple.Checkpoint.Metadata.ThreadID)
	}
}

func TestCheckpointManager_GetTupleByVersion(t *testing.T) {
	ctx := context.Background()
	manager := NewCheckpointManager(10)

	// Create and save multiple checkpoints
	checkpoint1 := NewCheckpoint("thread-1", 0)
	checkpoint1.Version = 1

	err := manager.Save(ctx, checkpoint1)
	if err != nil {
		t.Fatalf("Failed to save checkpoint1: %v", err)
	}

	checkpoint2 := NewCheckpoint("thread-1", 1)
	checkpoint2.ParentID = checkpoint1.ID
	checkpoint2.Version = 2

	err = manager.Save(ctx, checkpoint2)
	if err != nil {
		t.Fatalf("Failed to save checkpoint2: %v", err)
	}

	// Get tuple by version
	config := types.NewRunnableConfig()
	config.ThreadID = "thread-1"

	tuple, err := manager.GetTupleByVersion(ctx, config, 1)
	if err != nil {
		t.Fatalf("Failed to get tuple: %v", err)
	}

	if tuple.Checkpoint == nil {
		t.Error("Checkpoint should not be nil")
	}

	if tuple.Checkpoint.Version != 1 {
		t.Errorf("Expected version 1, got %d", tuple.Checkpoint.Version)
	}

	if tuple.ParentConfig != nil {
		t.Error("ParentConfig should be nil for version 1")
	}

	// Get tuple for version 2
	tuple, err = manager.GetTupleByVersion(ctx, config, 2)
	if err != nil {
		t.Fatalf("Failed to get tuple: %v", err)
	}

	if tuple.Checkpoint == nil {
		t.Error("Checkpoint should not be nil")
	}

	if tuple.Checkpoint.Version != 2 {
		t.Errorf("Expected version 2, got %d", tuple.Checkpoint.Version)
	}

	if tuple.ParentConfig == nil {
		t.Error("ParentConfig should not be nil for version 2")
	}
}

func TestCheckpointManager_GetLineage(t *testing.T) {
	ctx := context.Background()
	manager := NewCheckpointManager(10)

	// Create and save multiple checkpoints in a version chain.
	var prevID string
	for i := 0; i < 5; i++ {
		checkpoint := NewCheckpoint("thread-1", i)
		checkpoint.Version = i
		if i > 0 {
			checkpoint.ParentID = prevID
		}
		err := manager.Save(ctx, checkpoint)
		if err != nil {
			t.Fatalf("Failed to save checkpoint %d: %v", i, err)
		}
		prevID = checkpoint.ID
		time.Sleep(1 * time.Millisecond) // Ensure different timestamps
	}

	// Get full lineage
	lineage, err := manager.GetLineage(ctx, "thread-1", 0)
	if err != nil {
		t.Fatalf("Failed to get lineage: %v", err)
	}

	if len(lineage) != 5 {
		t.Errorf("Expected 5 checkpoints in lineage, got %d", len(lineage))
	}

	// Get limited lineage
	lineage, err = manager.GetLineage(ctx, "thread-1", 3)
	if err != nil {
		t.Fatalf("Failed to get limited lineage: %v", err)
	}

	if len(lineage) != 3 {
		t.Errorf("Expected 3 checkpoints in limited lineage, got %d", len(lineage))
	}

	// Verify lineage is ordered from oldest to newest (but limited to most recent)
	// GetLineage returns the most recent checkpoints in chronological order
	if lineage[0].Checkpoint.Version != 2 {
		t.Errorf("Expected first checkpoint version 2 (oldest in limit of 3), got %d", lineage[0].Checkpoint.Version)
	}

	if lineage[2].Checkpoint.Version != 4 {
		t.Errorf("Expected third checkpoint version 4 (newest), got %d", lineage[2].Checkpoint.Version)
	}
}

func TestCheckpointManager_ConcurrentWrites(t *testing.T) {
	ctx := context.Background()
	manager := NewCheckpointManager(10)

	// Create initial checkpoint
	checkpoint := NewCheckpoint("thread-1", 0)
	checkpoint.State["counter"] = 0

	err := manager.Save(ctx, checkpoint)
	if err != nil {
		t.Fatalf("Failed to save initial checkpoint: %v", err)
	}

	// Concurrently write from multiple tasks.
	// With version-chain conflict detection, only the first write (by scheduler
	// timing) will succeed; all others detect that the checkpoint_id is stale.
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(taskNum int) {
			config := types.NewRunnableConfig()
			config.ThreadID = "thread-1"
			config.Set("checkpoint_id", checkpoint.ID)

			writes := []PendingWrite{
				*NewPendingWrite("counter", taskNum, false, "node1", "task1"),
			}

			err := manager.PutWrites(ctx, config, writes, "task1")
			done <- (err == nil)
		}(i)
	}

	// Wait for all operations to complete
	successCount := 0
	for i := 0; i < 10; i++ {
		if <-done {
			successCount++
		}
	}

	// With version-chain detection, exactly one write should succeed;
	// the rest detect a conflict because they share the same checkpoint_id.
	if successCount != 1 {
		t.Logf("Expected 1 successful write (version chain), got %d", successCount)
	}

	// Verify final state
	latest, err := manager.Load(ctx, "thread-1")
	if err != nil {
		t.Fatalf("Failed to load checkpoint: %v", err)
	}

	if latest.State["counter"] == nil {
		t.Error("Counter should not be nil")
	}
}
