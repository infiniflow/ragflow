package pipeline

import (
	"context"
	"errors"
	"fmt"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/ingestion/chunkcache"
)

// cleanupCheckpointState removes the checkpoint and the fingerprints that
// describe the graph which produced it. It returns storage failures to
// callers that need to gate a new run on cleanup completion; the pipeline's
// cancellation path logs those failures and remains best-effort.
func cleanupCheckpointState(ctx context.Context, store canvas.CheckPointStore, tracker *canvas.RunTracker, checkpointID string) error {
	if checkpointID == "" {
		return errors.New("checkpoint cleanup requires a checkpoint id")
	}
	var cleanupErr error
	if store != nil {
		for _, key := range []string{checkpointID, checkpointID + dslKeySuffix, checkpointID + ovfKeySuffix} {
			if err := store.Delete(ctx, key); err != nil {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("delete %s: %w", key, err))
			}
		}
	}
	if tracker != nil {
		if err := tracker.ClearInterruptID(ctx, checkpointID); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("clear interrupt %s: %w", checkpointID, err))
		}
	}
	return cleanupErr
}

// cleanupTaskState clears all resumable state associated with a task before
// its terminal task row is removed for a rerun. Cache entries are purged only
// after checkpoint cleanup succeeds so a failed cleanup remains retryable.
func cleanupTaskState(ctx context.Context, store canvas.CheckPointStore, tracker *canvas.RunTracker, taskID string) error {
	if err := cleanupCheckpointState(ctx, store, tracker, taskID); err != nil {
		return err
	}
	if err := chunkcache.PurgeTask(ctx, chunkcache.Client(), taskID); err != nil {
		return fmt.Errorf("purge chunk cache: %w", err)
	}
	return nil
}

// PurgeTaskState removes checkpoint, tracker, and per-chunk cache state for a
// task using the process-wide Redis clients. Document rerun cleanup calls this
// while it owns the durable cleanup claim; the caller supplies a bounded,
// detached context and fences the claim at the batch boundary.
func PurgeTaskState(ctx context.Context, taskID string) error {
	if taskID == "" {
		return errors.New("task state cleanup requires a task id")
	}
	p := &Pipeline{taskID: taskID}
	return cleanupTaskState(ctx, p.resolveStore(), p.resolveTracker(), taskID)
}
