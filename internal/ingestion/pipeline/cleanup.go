package pipeline

import (
	"context"
	"errors"

	"ragflow/internal/ingestion/chunkcache"
)

// PurgeTaskState removes the run-scoped cache state for a task using the
// process-wide Kvrocks client. The shared task manifest covers chunk
// LLM/embedding results and parser media, so a rerun starts with both caches
// cold.
func PurgeTaskState(ctx context.Context, taskID string) error {
	if taskID == "" {
		return errors.New("task state cleanup requires a task id")
	}
	return chunkcache.PurgeTask(ctx, chunkcache.Client(), taskID)
}
