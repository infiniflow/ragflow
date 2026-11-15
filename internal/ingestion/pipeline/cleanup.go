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
	return purgeTaskStateWithBackend(ctx, chunkcache.Client(), taskID)
}

// purgeTaskStateWithBackend purges the chunk and media manifests on one
// backend. The task-id guard stays in both entry points so an empty id can
// never purge a shared or empty-keyed manifest.
func purgeTaskStateWithBackend(ctx context.Context, backend chunkcache.Store, taskID string) error {
	if taskID == "" {
		return errors.New("task state cleanup requires a task id")
	}
	return errors.Join(
		chunkcache.PurgeTask(ctx, backend, taskID),
		mediacache.PurgeTask(ctx, backend, taskID),
	)
}
