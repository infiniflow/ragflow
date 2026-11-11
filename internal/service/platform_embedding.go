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

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	enginetypes "ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

// BindDatasetEmbedding pins the selected default to an unbound empty dataset.
func BindDatasetEmbedding(ctx context.Context, db *gorm.DB, store engine.DocEngine, kbID, tenantID string) (string, error) {
	if db == nil {
		return "", errors.New("dataset database is not initialized")
	}
	if active := permission.TenantIDFromContext(ctx); active != "" && active != tenantID {
		return "", permission.ErrIdentityMismatch
	}
	kb, err := dao.NewKnowledgebaseDAO().GetByIDAndTenantID(ctx, db, kbID, tenantID)
	if err != nil {
		return "", err
	}
	if ref := kb.EmbeddingModelReference(); ref != "" {
		return ref, nil
	}
	modelCtx := permission.WithTenantID(ctx, tenantID)
	info, err := NewModelFactory().ResolveDefaultInfo(modelCtx, entity.ModelTypeEmbedding)
	if errors.Is(err, ErrNoDefaultModel) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	selected := ModelReference(info)
	if strings.TrimSpace(selected) == "" {
		return "", nil
	}

	var ref string
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked entity.Knowledgebase
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Take(&locked, "id = ? AND tenant_id = ? AND status = ?", kbID, tenantID, string(entity.StatusValid)).Error; err != nil {
			return err
		}
		if ref = locked.EmbeddingModelReference(); ref != "" {
			return nil
		}
		if err := CheckDatasetEmbeddingIndexEmpty(ctx, store, &locked); err != nil {
			return err
		}
		var modelID any
		if info.ID != "" {
			modelID = info.ID
		}
		if err := tx.Model(&entity.Knowledgebase{}).Where("id = ?", kbID).Updates(map[string]any{"embd_id": selected, "tenant_embd_id": modelID}).Error; err != nil {
			return err
		}
		ref = selected
		return nil
	})
	return ref, err
}

// CheckDatasetEmbeddingIndexEmpty fails closed when existing vectors cannot be
// ruled out. The caller must hold the dataset row lock while deciding whether
// to change its binding.
func CheckDatasetEmbeddingIndexEmpty(ctx context.Context, store engine.DocEngine, kb *entity.Knowledgebase) error {
	if kb == nil || kb.ChunkNum != 0 || store == nil {
		return errors.New("dataset embedding model is unbound or changing; select the original model or rebuild the index")
	}
	index := fmt.Sprintf("ragflow_%s", kb.TenantID)
	exists, err := store.ChunkStoreExists(ctx, index, kb.ID)
	if err != nil {
		return fmt.Errorf("check dataset index before binding embedding model: %w", err)
	}
	if exists {
		// Some engines acknowledge writes before they become searchable.
		if refresher, ok := store.(interface {
			RefreshChunkStore(context.Context, string, string) error
		}); ok {
			if err := refresher.RefreshChunkStore(ctx, index, kb.ID); err != nil {
				return fmt.Errorf("refresh dataset index before checking chunks: %w", err)
			}
		}
		result, err := store.Search(ctx, &enginetypes.SearchRequest{IndexNames: []string{index}, KbIDs: []string{kb.ID}, Limit: 1, IncludeUnavailable: true, RequireComplete: true})
		if err != nil {
			return fmt.Errorf("check dataset chunks before binding embedding model: %w", err)
		}
		if result == nil || result.Total != 0 || len(result.Chunks) != 0 {
			return errors.New("dataset embedding model is unbound or changing; select the original model or rebuild the index")
		}
	}
	return nil
}

// WithStableDatasetEmbedding serializes a vector write with changes to the
// dataset binding and rejects vectors prepared using an older model.
func WithStableDatasetEmbedding(ctx context.Context, db *gorm.DB, kbID, tenantID, ref string, write func() error) error {
	return dao.WithStableDatasetEmbedding(ctx, db, kbID, tenantID, ref, write)
}
