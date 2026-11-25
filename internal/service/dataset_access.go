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
	"strings"

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

// filterAndLoadKBs authorizes and loads datasets with one batch request each,
// returning the KB slice in request order together with the set of refs the
// batch authorized. Callers that must interleave permission, existence and
// per-dataset validation errors can walk the ordered slice and use the set to
// report the first error in request order; a denied ref still gets a real
// single-resource check so the exact legacy error is preserved, while the
// allowed path pays no per-item round-trip.
func filterAndLoadKBs(ctx context.Context, db *gorm.DB, checker permission.ResourceChecker, kbDAO *dao.KnowledgebaseDAO, ids []string, op permission.Operation) ([]*entity.Knowledgebase, map[permission.ResourceRef]bool, error) {
	if len(ids) == 0 {
		return nil, nil, nil
	}
	refs := make([]permission.ResourceRef, 0, len(ids))
	for _, id := range ids {
		refs = append(refs, permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: id})
	}
	// A batch-load failure (bad ref, unreachable source, tenant without an
	// owner) must not short-circuit validation of earlier datasets. Leave the
	// allowed set empty: the caller's ordered pass then asks CheckResource for
	// each ref and reports the first failure in request order. Allowed refs
	// cost one single-resource check only on this error path.
	accessible, err := checker.FilterResources(ctx, refs, op)
	allowed := make(map[permission.ResourceRef]bool, len(accessible))
	if err == nil {
		for _, ref := range accessible {
			allowed[ref] = true
		}
	}
	kbs, err := LoadKBsInOrder(ctx, db, kbDAO, ids)
	if err != nil {
		return nil, nil, err
	}
	return kbs, allowed, nil
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
	byFoldedID := make(map[string]*entity.Knowledgebase, len(kbs))
	for _, kb := range kbs {
		if kb == nil {
			continue
		}
		byID[kb.ID] = kb
		// A database with a case-insensitive collation can return the stored
		// row even when the request used a different case. Keep a folded index
		// so the requested id still resolves to that row.
		folded := strings.ToLower(kb.ID)
		if _, exists := byFoldedID[folded]; !exists {
			byFoldedID[folded] = kb
		}
	}
	out := make([]*entity.Knowledgebase, len(ids))
	for i, id := range ids {
		kb := byID[id]
		if kb == nil {
			kb = byFoldedID[strings.ToLower(id)]
		}
		out[i] = kb
	}
	return out, nil
}
