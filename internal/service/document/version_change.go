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

package document

import (
	"context"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// documentVersionChange describes one document in a dataset version's change
// set. The name is captured here, at write time, because a deleted document has
// no row left for the versions page to read a name from later.
func documentVersionChange(action string, doc *entity.Document) entity.DatasetVersionChange {
	change := entity.DatasetVersionChange{
		Entity: entity.DatasetVersionChangeEntityDocument,
		Action: action,
	}
	if doc == nil {
		return change
	}
	name := ""
	if doc.Name != nil {
		name = *doc.Name
	}
	change.DocID = doc.ID
	change.Name = name
	return change
}

// documentVersionChanges turns a set of document ids and their names into add or
// delete entries, for the paths that only keep the ids.
func documentVersionChanges(action string, names map[string]string) []entity.DatasetVersionChange {
	if len(names) == 0 {
		return nil
	}
	changes := make([]entity.DatasetVersionChange, 0, len(names))
	for docID, name := range names {
		changes = append(changes, entity.DatasetVersionChange{
			Entity: entity.DatasetVersionChangeEntityDocument,
			Action: action,
			DocID:  docID,
			Name:   name,
		})
	}
	return changes
}

// documentNamesByID reads the display names of documents that are about to be
// deleted. A failed lookup returns nil: the change set then names the ids
// without names, which is worse to read but never wrong.
func documentNamesByID(ctx context.Context, ids []string) map[string]string {
	if len(ids) == 0 || dao.DB == nil {
		return nil
	}
	var docs []entity.Document
	if err := dao.DB.WithContext(ctx).
		Select("id", "name").
		Where("id IN ?", ids).
		Find(&docs).Error; err != nil {
		return nil
	}
	names := make(map[string]string, len(docs))
	for _, doc := range docs {
		if doc.Name != nil {
			names[doc.ID] = *doc.Name
		} else {
			names[doc.ID] = ""
		}
	}
	return names
}
