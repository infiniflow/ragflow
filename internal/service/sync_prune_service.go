//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package service

import (
	"context"
	"errors"
)

// ErrSyncDocumentDeleterNotConfigured reports a missing production delete service.
var ErrSyncDocumentDeleterNotConfigured = errors.New("sync document deleter is not configured")

// SyncDocumentDeleter removes documents through the owning business service.
type SyncDocumentDeleter interface {
	// DeleteDocument removes a document and dependent business state.
	DeleteDocument(ctx context.Context, docID string) error
}

// SyncPruneService owns stale document deletion for complete prune snapshots.
type SyncPruneService struct {
	deleter SyncDocumentDeleter
	store   DocumentStore
}

// NewSyncPruneService creates a prune service.
func NewSyncPruneService(deleter SyncDocumentDeleter, store DocumentStore) *SyncPruneService {
	if store == nil {
		store = NewGormDocumentStore()
	}
	return &SyncPruneService{deleter: deleter, store: store}
}

// DeleteStale removes documents missing from the complete source snapshot.
func (s *SyncPruneService) DeleteStale(ctx context.Context, taskContext SyncTaskContext, retain map[string]struct{}) (int64, error) {
	sourceType := SourceType(taskContext.Connector.Source, taskContext.Connector.ID)
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
	return removed, nil
}

// RetainDocumentIDs calculates PRUNE retain IDs using both legacy and new schemes.
func RetainDocumentIDs(kbID, connectorID string, sourceIDs []string) map[string]struct{} {
	retain := make(map[string]struct{}, len(sourceIDs)*2)
	for _, sourceID := range sourceIDs {
		AddRetainDocumentID(retain, kbID, connectorID, sourceID)
	}
	return retain
}

// AddRetainDocumentID adds both legacy and current document IDs for one source ID.
func AddRetainDocumentID(retain map[string]struct{}, kbID, connectorID, sourceID string) {
	retain[Hash128(connectorID+":"+sourceID)] = struct{}{}
	retain[Hash128(kbID+":"+connectorID+":"+sourceID)] = struct{}{}
}
