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

	ingestiontable "ragflow/internal/ingestion/table"
)

// RevokeTableProfile drops a document's derived table columns and the metadata
// values that published them. Callers outside this package use it when rows are
// removed by a path that cannot re-derive what the remaining rows still hold:
// keeping the record would advertise columns the index no longer answers for.
func (s *DocumentService) RevokeTableProfile(ctx context.Context, docID string) error {
	return s.revokeTableProfile(ctx, docID)
}

// revokeTableProfile drops the derived column record a spreadsheet run published,
// together with the document-metadata values that run wrote.
//
// It belongs to every path that discards a document's indexed output: the rows
// those columns describe are going away, and a query that still named them would
// resolve against nothing. Deleting the record here rather than waiting for the
// next successful run also means a re-parse that fails leaves the document
// unqueryable instead of queryable against stale data.
//
// Metadata keys the table system never owned — written by a user or the LLM, or
// taken over since — are left alone; only the published ownership list is
// removed.
func (s *DocumentService) revokeTableProfile(ctx context.Context, docID string) error {
	if s.docEngine == nil || s.metadataSvc == nil {
		return nil
	}
	existing, err := s.GetDocumentMetadataRaw(ctx, docID)
	if err != nil {
		return err
	}
	profile, ok, err := ingestiontable.DecodeProfile(existing[ingestiontable.ProfileMetadataField])
	if err != nil {
		// An unreadable record still has to go: it makes the document look
		// queryable when nothing can resolve the columns it claims.
		return s.DeleteDocumentMetadata(ctx, docID, []string{ingestiontable.ProfileMetadataField})
	}
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(profile.OwnedMetadata)+1)
	keys = append(keys, ingestiontable.ProfileMetadataField)
	keys = append(keys, profile.OwnedMetadata...)
	return s.DeleteDocumentMetadata(ctx, docID, keys)
}
