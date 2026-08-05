package service

import (
	"context"
	"encoding/hex"
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"github.com/zeebo/xxh3"
)

// Hash128 mirrors Python api.utils.common.hash128.
func Hash128(data string) string {
	sum := xxh3.Hash128([]byte(data)).Bytes()
	return hex.EncodeToString(sum[:])
}

// ResolvedDocumentID contains legacy and new connector document IDs.
type ResolvedDocumentID struct {
	DocID             string
	LegacyID          string
	NewID             string
	StoredFingerprint string
}

// DocumentStore reads existing connector documents.
type DocumentStore interface {
	ListIDs(ctx context.Context, kbID, sourceType string) (map[string]struct{}, error)
	ListFingerprints(ctx context.Context, kbID, sourceType string) (map[string]string, error)
}

// DocumentIDResolver applies Python-compatible legacy/new ID rules.
type DocumentIDResolver struct {
	store DocumentStore
}

// NewDocumentIDResolver creates a document ID resolver.
func NewDocumentIDResolver(store DocumentStore) *DocumentIDResolver {
	return &DocumentIDResolver{store: store}
}

// Resolve chooses the legacy ID when already present in the KB.
func (r *DocumentIDResolver) Resolve(ctx context.Context, kbID, connectorID, sourceType, sourceID string) (ResolvedDocumentID, error) {
	legacyID := Hash128(connectorID + ":" + sourceID)
	newID := Hash128(kbID + ":" + connectorID + ":" + sourceID)

	existing, err := r.store.ListIDs(ctx, kbID, sourceType)
	if err != nil {
		return ResolvedDocumentID{}, err
	}

	docID := newID
	if _, ok := existing[legacyID]; ok {
		docID = legacyID
	}

	fingerprints, err := r.store.ListFingerprints(ctx, kbID, sourceType)
	if err != nil {
		return ResolvedDocumentID{}, err
	}

	stored := fingerprints[legacyID]
	if stored == "" {
		stored = fingerprints[newID]
	}

	return ResolvedDocumentID{DocID: docID, LegacyID: legacyID, NewID: newID, StoredFingerprint: stored}, nil
}

// GormDocumentStore reads documents from the current GORM database.
type GormDocumentStore struct{}

// NewGormDocumentStore creates a GORM-backed document store.
func NewGormDocumentStore() *GormDocumentStore {
	return &GormDocumentStore{}
}

// ListIDs returns existing connector document IDs in a KB.
func (s *GormDocumentStore) ListIDs(ctx context.Context, kbID, sourceType string) (map[string]struct{}, error) {
	var docs []entity.Document
	if err := dao.GetDB().WithContext(ctx).Select("id").Where("kb_id = ? AND source_type = ?", kbID, sourceType).Find(&docs).Error; err != nil {
		return nil, err
	}

	result := make(map[string]struct{}, len(docs))
	for _, doc := range docs {
		result[doc.ID] = struct{}{}
	}

	return result, nil
}

// ListFingerprints returns existing connector document fingerprints in a KB.
func (s *GormDocumentStore) ListFingerprints(ctx context.Context, kbID, sourceType string) (map[string]string, error) {
	var docs []entity.Document
	if err := dao.GetDB().WithContext(ctx).Select("id", "content_hash").Where("kb_id = ? AND source_type = ?", kbID, sourceType).Find(&docs).Error; err != nil {
		return nil, err
	}

	result := make(map[string]string, len(docs))
	for _, doc := range docs {
		if doc.ContentHash == nil {
			continue
		}
		result[doc.ID] = *doc.ContentHash
	}
	
	return result, nil
}
