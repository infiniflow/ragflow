package document

import (
	"context"
	"errors"
	"time"

	"ragflow/internal/engine/kvrocks"
	"ragflow/internal/utility"
)

type metadataLockStore interface {
	SetNX(context.Context, string, string, time.Duration) bool
	DeleteIfEqual(context.Context, string, string) bool
}

// WithDocumentMetadataLock serializes metadata read-modify-write operations
// across API and ingestor processes using their existing Kvrocks connection.
// The operation deadline is shorter than the lease, including acquisition.
func (s *DocumentService) WithDocumentMetadataLock(ctx context.Context, docID string, update func(context.Context) error) error {
	store := s.metadataLocks
	if store == nil {
		client := kvrocks.Get()
		if client == nil {
			return errors.New("metadata lock store is not initialized")
		}
		store = client
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	key, owner := "document-metadata:"+docID, utility.GenerateUUID()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if store.SetNX(ctx, key, owner, 30*time.Second) {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		store.DeleteIfEqual(releaseCtx, key, owner)
	}()
	if err := update(ctx); err != nil {
		return err
	}
	return ctx.Err()
}
