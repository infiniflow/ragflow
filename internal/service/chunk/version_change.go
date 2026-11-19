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

package chunk

import "ragflow/internal/entity"

// chunkVersionChange describes one chunk in a dataset version's change set.
//
// It names the chunk and its document and stops there: the text the reader sees
// comes from the chunk history, replayed at the version's anchor. Copying the
// text into the entry would store the same bytes twice, and the two copies would
// disagree as soon as one of them was purged.
func chunkVersionChange(action, docID, chunkID string) entity.DatasetVersionChange {
	return entity.DatasetVersionChange{
		Entity:  entity.DatasetVersionChangeEntityChunk,
		Action:  action,
		DocID:   docID,
		ChunkID: chunkID,
	}
}

// chunkVersionChanges turns a set of chunk ids into entries for one action.
func chunkVersionChanges(action, docID string, chunkIDs []string) []entity.DatasetVersionChange {
	if len(chunkIDs) == 0 {
		return nil
	}
	changes := make([]entity.DatasetVersionChange, 0, len(chunkIDs))
	for _, chunkID := range chunkIDs {
		changes = append(changes, chunkVersionChange(action, docID, chunkID))
	}
	return changes
}

// documentVersionChange describes a document-level change recorded from a chunk
// path: a parse rewrites a whole document's chunk set, and clearing a document's
// chunks is one change, not one per chunk.
//
// Unlike a chunk entry, it carries the document name: a deleted document has no
// row left to read it from, so the name has to travel with the entry.
func documentVersionChange(action, docID, name string) entity.DatasetVersionChange {
	return entity.DatasetVersionChange{
		Entity: entity.DatasetVersionChangeEntityDocument,
		Action: action,
		DocID:  docID,
		Name:   name,
	}
}

// documentName reads a document's display name for a change entry.
func documentName(doc *entity.Document) string {
	if doc == nil || doc.Name == nil {
		return ""
	}
	return *doc.Name
}
