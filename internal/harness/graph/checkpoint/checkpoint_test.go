package checkpoint

import (
	"context"
	"testing"
)

func TestMemorySaver(t *testing.T) {
	ctx := context.Background()
	saver := NewMemorySaver()

	threadID := "test-thread-1"

	// Save a checkpoint
	checkpoint := map[string]interface{}{
		"messages": []string{"hello", "world"},
		"counter":  42,
	}

	config := map[string]interface{}{
		"thread_id": threadID,
	}

	err := saver.Put(ctx, config, checkpoint)
	if err != nil {
		t.Fatalf("Failed to save checkpoint: %v", err)
	}

	// Retrieve the checkpoint
	retrieved, err := saver.Get(ctx, config)
	if err != nil {
		t.Fatalf("Failed to get checkpoint: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected non-nil checkpoint")
	}

	// Verify values
	msgs, ok := retrieved["messages"].([]interface{})
	if !ok || len(msgs) != 2 {
		t.Errorf("Expected 2 messages, got %v", msgs)
	}

	counter, ok := retrieved["counter"].(float64) // JSON unmarshals numbers as float64
	if !ok || counter != 42 {
		t.Errorf("Expected counter=42, got %v", retrieved["counter"])
	}
}

func TestMemorySaverMultipleVersions(t *testing.T) {
	ctx := context.Background()
	saver := NewMemorySaver()

	threadID := "test-thread-2"

	// Save multiple checkpoints
	for i := 0; i < 3; i++ {
		checkpoint := map[string]interface{}{
			"step": i,
		}
		config := map[string]interface{}{
			"thread_id": threadID,
		}
		err := saver.Put(ctx, config, checkpoint)
		if err != nil {
			t.Fatalf("Failed to save checkpoint %d: %v", i, err)
		}
	}

	// List checkpoints
	config := map[string]interface{}{
		"thread_id": threadID,
	}
	checkpoints, err := saver.List(ctx, config, 10)
	if err != nil {
		t.Fatalf("Failed to list checkpoints: %v", err)
	}

	if len(checkpoints) != 3 {
		t.Errorf("Expected 3 checkpoints, got %d", len(checkpoints))
	}

	// Get should return latest
	latest, err := saver.Get(ctx, config)
	if err != nil {
		t.Fatalf("Failed to get latest checkpoint: %v", err)
	}

	step := latest["step"].(float64)
	if step != 2 {
		t.Errorf("Expected latest step=2, got %v", step)
	}
}

func TestMemorySaverMultipleThreads(t *testing.T) {
	ctx := context.Background()
	saver := NewMemorySaver()

	// Save checkpoints for different threads
	threads := []string{"thread-a", "thread-b", "thread-c"}
	for i, threadID := range threads {
		checkpoint := map[string]interface{}{
			"thread_index": i,
		}
		config := map[string]interface{}{
			"thread_id": threadID,
		}
		err := saver.Put(ctx, config, checkpoint)
		if err != nil {
			t.Fatalf("Failed to save checkpoint for %s: %v", threadID, err)
		}
	}

	// Retrieve each thread's checkpoint
	for i, threadID := range threads {
		config := map[string]interface{}{
			"thread_id": threadID,
		}
		checkpoint, err := saver.Get(ctx, config)
		if err != nil {
			t.Fatalf("Failed to get checkpoint for %s: %v", threadID, err)
		}

		index := checkpoint["thread_index"].(float64)
		if int(index) != i {
			t.Errorf("For thread %s, expected index %d, got %v", threadID, i, index)
		}
	}
}

func TestDeepCopy(t *testing.T) {
	original := map[string]interface{}{
		"messages": []string{"hello", "world"},
		"nested": map[string]interface{}{
			"key": "value",
		},
	}

	copied := deepCopy(original)

	// Modify original
	original["messages"] = []string{"modified"}
	original["nested"].(map[string]interface{})["key"] = "modified"

	// Copy should be unchanged
	copiedMap := copied.(map[string]interface{})
	msgs := copiedMap["messages"].([]interface{})
	if len(msgs) != 2 || msgs[0] != "hello" {
		t.Error("Deep copy did not preserve original messages")
	}

	nested := copiedMap["nested"].(map[string]interface{})
	if nested["key"] != "value" {
		t.Error("Deep copy did not preserve nested value")
	}
}

func TestDeepCopySlice(t *testing.T) {
	original := []interface{}{"a", "b", "c"}
	copied := deepCopySlice(original)

	// Modify original
	original[0] = "modified"

	// Copy should be unchanged
	if copied[0] != "a" {
		t.Error("Deep copy slice did not preserve original")
	}
}

func TestMemorySaverWithMetadata(t *testing.T) {
	ctx := context.Background()
	saver := NewMemorySaver()

	threadID := "test-thread-meta"
	checkpoint := map[string]interface{}{
		"data": "value",
	}
	config := map[string]interface{}{
		"thread_id":  threadID,
		"metadata_1": "value_1",
		"metadata_2": 42,
	}

	err := saver.Put(ctx, config, checkpoint)
	if err != nil {
		t.Fatalf("Failed to save checkpoint: %v", err)
	}

	// List and verify metadata
	listConfig := map[string]interface{}{
		"thread_id": threadID,
	}
	checkpoints, err := saver.List(ctx, listConfig, 1)
	if err != nil {
		t.Fatalf("Failed to list checkpoints: %v", err)
	}

	if len(checkpoints) != 1 {
		t.Fatalf("Expected 1 checkpoint, got %d", len(checkpoints))
	}

	metadata := checkpoints[0]["metadata"].(map[string]interface{})
	if metadata["metadata_1"] != "value_1" {
		t.Error("Metadata not preserved correctly")
	}
}

func TestMemorySaverCheckpointID(t *testing.T) {
	ctx := context.Background()
	saver := NewMemorySaver()

	threadID := "test-thread-id"
	checkpointID := "custom-checkpoint-id"

	checkpoint := map[string]interface{}{
		"step": 1,
	}
	config := map[string]interface{}{
		"thread_id":    threadID,
		"checkpoint_id": checkpointID,
	}

	err := saver.Put(ctx, config, checkpoint)
	if err != nil {
		t.Fatalf("Failed to save checkpoint: %v", err)
	}

	// Retrieve by specific checkpoint ID
	getConfig := map[string]interface{}{
		"thread_id":     threadID,
		"checkpoint_id": checkpointID,
	}
	retrieved, err := saver.Get(ctx, getConfig)
	if err != nil {
		t.Fatalf("Failed to get checkpoint by ID: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected non-nil checkpoint")
	}

	step := retrieved["step"].(float64)
	if step != 1 {
		t.Errorf("Expected step=1, got %v", step)
	}
}
