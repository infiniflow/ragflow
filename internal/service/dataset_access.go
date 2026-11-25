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
//

package service

import (
	"context"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"

	"gorm.io/gorm"
)

// LoadAuthorizedKBs authorizes every dataset ID with one batch permission load
// and fetches all knowledge bases with one query, instead of one authorization
// and one lookup round-trip per dataset. The returned slice has the same order
// as ids: result[i] is the KB for ids[i], or nil when that dataset is missing.
// Permission failures keep the order of ids; the caller normalizes the error.
func LoadAuthorizedKBs(ctx context.Context, db *gorm.DB, checker permission.ResourceChecker, kbDAO *dao.KnowledgebaseDAO, ids []string, op permission.Operation) ([]*entity.Knowledgebase, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	refs := make([]permission.ResourceRef, 0, len(ids))
	for _, id := range ids {
		refs = append(refs, permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: id})
	}
	if err := permission.CheckResources(ctx, checker, refs, op); err != nil {
		return nil, err
	}
	return LoadKBsInOrder(ctx, db, kbDAO, ids)
}

// LoadKBsInOrder fetches the given datasets in one query and returns them in
// the same order as ids, with a nil entry for each missing dataset.
func LoadKBsInOrder(ctx context.Context, db *gorm.DB, kbDAO *dao.KnowledgebaseDAO, ids []string) ([]*entity.Knowledgebase, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	kbs, err := kbDAO.GetByIDs(ctx, db, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*entity.Knowledgebase, len(kbs))
	for _, kb := range kbs {
		if kb != nil {
			byID[kb.ID] = kb
		}
	}
	out := make([]*entity.Knowledgebase, len(ids))
	for i, id := range ids {
		out[i] = byID[id]
	}
	return out, nil
}
