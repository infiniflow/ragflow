package pipeline

import (
	"context"
	"errors"

	"ragflow/internal/ingestion/chunkcache"
)

// PurgeTaskState removes the per-chunk cache state for a task using the
// process-wide Kvrocks client. The checkpoint (eino resume) store was removed
// from the ingestion pipeline, so only the per-chunk LLM/embedding results are
// purged here; a rerun therefore starts with a cold chunk cache.
func PurgeTaskState(ctx context.Context, taskID string) error {
	if taskID == "" {
		return errors.New("task state cleanup requires a task id")
	}
	return chunkcache.PurgeTask(ctx, chunkcache.Client(), taskID)
}
