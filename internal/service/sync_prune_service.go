package service

import (
	"context"
	"errors"
	"ragflow/internal/dao"
	syncerconnector "ragflow/internal/syncer/connector"
)

// ErrSyncDocumentDeleterNotConfigured reports a missing production delete service.
var ErrSyncDocumentDeleterNotConfigured = errors.New("sync document deleter is not configured")

// SyncDocumentDeleter removes documents through the owning business service.
type SyncDocumentDeleter interface {
	// DeleteDocument removes a document and dependent business state.
	DeleteDocument(ctx context.Context, docID string) error
}

// SyncPruneService owns prune snapshots and stale document deletion.
type SyncPruneService struct {
	snapshotDAO *dao.SyncPruneSnapshotDAO
	deleter     SyncDocumentDeleter
	store       DocumentStore
}

// NewSyncPruneService creates a prune service.
func NewSyncPruneService(snapshotDAO *dao.SyncPruneSnapshotDAO, deleter SyncDocumentDeleter, store DocumentStore) *SyncPruneService {
	if store == nil {
		store = NewGormDocumentStore()
	}
	return &SyncPruneService{snapshotDAO: snapshotDAO, deleter: deleter, store: store}
}

// ClearSnapshot removes temporary snapshot rows.
func (s *SyncPruneService) ClearSnapshot(ctx context.Context, taskID string) error {
	return s.snapshotDAO.Clear(ctx, taskID)
}

// AddSnapshotBatch writes one slim snapshot batch.
func (s *SyncPruneService) AddSnapshotBatch(ctx context.Context, taskID string, documents []syncerconnector.SlimDocument) error {
	return s.snapshotDAO.InsertBatch(ctx, taskID, documents)
}

// DeleteStale removes documents missing from the complete source snapshot.
func (s *SyncPruneService) DeleteStale(ctx context.Context, taskContext SyncTaskContext) (int64, error) {
	sourceIDs, err := s.snapshotDAO.ListSourceIDs(ctx, taskContext.Task.ID)
	if err != nil {
		return 0, err
	}
	sourceType := SourceType(taskContext.Connector.Source, taskContext.Connector.ID)
	retain := RetainDocumentIDs(taskContext.Knowledgebase.ID, taskContext.Connector.ID, sourceIDs)
	existing, err := s.store.ListIDs(ctx, taskContext.Knowledgebase.ID, sourceType)
	if err != nil {
		return 0, err
	}
	var removed int64
	for docID := range existing {
		if _, ok := retain[docID]; ok {
			continue
		}
		if s.deleter == nil {
			return removed, ErrSyncDocumentDeleterNotConfigured
		}
		if err = s.deleter.DeleteDocument(ctx, docID); err != nil {
			return removed, err
		}
		removed++
	}
	if err = s.snapshotDAO.Clear(ctx, taskContext.Task.ID); err != nil {
		return removed, err
	}
	return removed, nil
}

// RetainDocumentIDs calculates PRUNE retain IDs using both legacy and new schemes.
func RetainDocumentIDs(kbID, connectorID string, sourceIDs []string) map[string]struct{} {
	retain := make(map[string]struct{}, len(sourceIDs)*2)
	for _, sourceID := range sourceIDs {
		retain[Hash128(connectorID+":"+sourceID)] = struct{}{}
		retain[Hash128(kbID+":"+connectorID+":"+sourceID)] = struct{}{}
	}
	return retain
}
