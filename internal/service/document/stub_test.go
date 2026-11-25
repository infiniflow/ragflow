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

//go:build !ee

package document

import (
	"testing"

	"gorm.io/gorm"
)

// expectsBlobRetained reports that the community edition removes the blob with
// the row: it keeps no content history, so nothing would read the object again.
func expectsBlobRetained() bool { return false }

// countRemovedDocuments is zero in the community edition: no removal row is written.
func countRemovedDocuments(_ *testing.T, _ *gorm.DB, _ string) int64 { return 0 }

// migrateDatasetVersionTableEE is a no-op: the community edition keeps no
// dataset history, so its test database needs no version table.
func migrateDatasetVersionTableEE(_ *testing.T, _ *gorm.DB) {}

// installMetadataHistoryStoreEE is a no-op: the community edition keeps no
// metadata states, so there is no store to stand in for the document engine.
func installMetadataHistoryStoreEE(_ *testing.T) {}
