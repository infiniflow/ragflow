//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package knowledge_compile

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	wikiDirtyDebounce = 20 * time.Second
	wikiDirtyLease    = 10 * time.Minute
	wikiDirtyPoll     = time.Second
)

// WikiDirtyRequest describes one debounced document-level Wiki refresh.
type WikiDirtyRequest struct {
	TenantID         string
	DatasetID        string
	DocumentID       string
	Revision         uint64
	AffectedChunkIDs []string
}

// WikiDirtyCompiler regenerates the affected document-level Wiki products.
type WikiDirtyCompiler func(context.Context, WikiDirtyRequest) error

var wikiDirtyCompilerState struct {
	sync.RWMutex
	compiler WikiDirtyCompiler
}

// SetWikiDirtyCompiler installs the document-level compiler owned by the task
// composition root.
func SetWikiDirtyCompiler(compiler WikiDirtyCompiler) {
	wikiDirtyCompilerState.Lock()
	wikiDirtyCompilerState.compiler = compiler
	wikiDirtyCompilerState.Unlock()
}

// MarkWikiDocumentDirty durably applies a trailing-edge debounce. Repeated
// mutations merge chunk ids and postpone execution until twenty seconds after
// the most recent successful mutation.
func MarkWikiDocumentDirty(ctx context.Context, tenantID, datasetID, documentID string, chunkIDs []string) error {
	db := kcDB
	if db == nil {
		db = dao.DB
	}
	if db == nil {
		return fmt.Errorf("mark Wiki document dirty: database is unavailable")
	}
	if tenantID == "" || datasetID == "" || documentID == "" {
		return fmt.Errorf("mark Wiki document dirty: tenant, dataset, and document ids are required")
	}
	runAfter := time.Now().Add(wikiDirtyDebounce)
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := entity.WikiDocumentDirty{DocumentID: documentID}
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("document_id = ?", documentID).First(&row).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		if err == gorm.ErrRecordNotFound {
			row = entity.WikiDocumentDirty{
				DocumentID:       documentID,
				DatasetID:        datasetID,
				TenantID:         tenantID,
				Revision:         1,
				AffectedChunkIDs: marshalDirtyChunkIDs(chunkIDs),
				RunAfter:         runAfter,
				State:            entity.WikiDirtyStatePending,
			}
			return tx.Create(&row).Error
		}
		row.TenantID = tenantID
		row.DatasetID = datasetID
		row.Revision++
		row.AffectedChunkIDs = marshalDirtyChunkIDs(append(parseDirtyChunkIDs(row.AffectedChunkIDs), chunkIDs...))
		row.RunAfter = runAfter
		row.State = entity.WikiDirtyStatePending
		row.ErrorMsg = ""
		row.ClaimOwner = ""
		row.ClaimExpiresAt = nil
		return tx.Save(&row).Error
	})
}

func runWikiDirtyWorker(ctx context.Context, db *gorm.DB, owner string) {
	ticker := time.NewTicker(wikiDirtyPoll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for {
				row, ok, err := claimWikiDirty(ctx, db, owner)
				if err != nil {
					common.Warn("wiki dirty worker: claim failed", zap.Error(err))
					break
				}
				if !ok {
					break
				}
				processWikiDirty(ctx, db, owner, row)
			}
		}
	}
}

func claimWikiDirty(ctx context.Context, db *gorm.DB, owner string) (entity.WikiDocumentDirty, bool, error) {
	var claimed entity.WikiDocumentDirty
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		query := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("run_after <= ? AND (state = ? OR (state = ? AND claim_expires_at <= ?))",
				now, entity.WikiDirtyStatePending, entity.WikiDirtyStateRunning, now).
			Order("run_after ASC").Limit(1)
		if err := query.First(&claimed).Error; err != nil {
			return err
		}
		expires := now.Add(wikiDirtyLease)
		return tx.Model(&entity.WikiDocumentDirty{}).Where("document_id = ? AND revision = ?", claimed.DocumentID, claimed.Revision).
			Updates(map[string]interface{}{
				"state":            entity.WikiDirtyStateRunning,
				"claim_owner":      owner,
				"claim_expires_at": expires,
			}).Error
	})
	if err == gorm.ErrRecordNotFound {
		return entity.WikiDocumentDirty{}, false, nil
	}
	return claimed, err == nil, err
}

func processWikiDirty(ctx context.Context, db *gorm.DB, owner string, row entity.WikiDocumentDirty) {
	wikiDirtyCompilerState.RLock()
	compiler := wikiDirtyCompilerState.compiler
	wikiDirtyCompilerState.RUnlock()
	request := WikiDirtyRequest{
		TenantID:         row.TenantID,
		DatasetID:        row.DatasetID,
		DocumentID:       row.DocumentID,
		Revision:         row.Revision,
		AffectedChunkIDs: parseDirtyChunkIDs(row.AffectedChunkIDs),
	}
	var err error
	if compiler == nil {
		err = fmt.Errorf("Wiki dirty compiler is not configured")
	} else {
		err = compiler(ctx, request)
	}
	if err == nil {
		db.WithContext(ctx).Where("document_id = ? AND revision = ? AND claim_owner = ?", row.DocumentID, row.Revision, owner).
			Delete(&entity.WikiDocumentDirty{})
		return
	}
	common.Warn("wiki dirty worker: compile failed",
		zap.String("document_id", row.DocumentID), zap.Uint64("revision", row.Revision), zap.Error(err))
	next := time.Now().Add(wikiDirtyDebounce)
	db.WithContext(ctx).Model(&entity.WikiDocumentDirty{}).
		Where("document_id = ? AND revision = ? AND claim_owner = ?", row.DocumentID, row.Revision, owner).
		Updates(map[string]interface{}{
			"state":            entity.WikiDirtyStatePending,
			"run_after":        next,
			"claim_owner":      "",
			"claim_expires_at": nil,
			"error_msg":        err.Error(),
		})
}

func parseDirtyChunkIDs(raw string) []string {
	var ids []string
	_ = json.Unmarshal([]byte(raw), &ids)
	return uniqueSortedStrings(ids)
}

func marshalDirtyChunkIDs(ids []string) string {
	payload, _ := json.Marshal(uniqueSortedStrings(ids))
	return string(payload)
}

func uniqueSortedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
