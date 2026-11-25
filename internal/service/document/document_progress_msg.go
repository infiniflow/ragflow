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
)

// UpdateProgressMsg persists a human-readable terminal message on the document
// row. The running-state snapshot is short-lived, so the terminal explanation
// needs a durable home that the document list and detail readers already have.
// An empty message carries no information and leaves the stored text untouched.
func (s *DocumentService) UpdateProgressMsg(ctx context.Context, docID, message string) error {
	if message == "" {
		return nil
	}
	return s.documentDAO.UpdateByID(ctx, dao.DB, docID, map[string]interface{}{
		"progress_msg": message,
	})
}
