package pipeline

import (
	"context"
	"errors"

	"ragflow/internal/ingestion/chunkcache"
	"ragflow/internal/ingestion/mediacache"
)

// PurgeTaskState removes the run-scoped cache state for a task using the
// process-wide Kvrocks client. Model results and media use separate manifests.
func PurgeTaskState(ctx context.Context, taskID string) error {
	if taskID == "" {
		return errors.New("task state cleanup requires a task id")
	}
	return errors.Join(chunkcache.PurgeTask(ctx, chunkcache.Client(), taskID), mediacache.PurgeTask(ctx, chunkcache.Client(), taskID))
}
