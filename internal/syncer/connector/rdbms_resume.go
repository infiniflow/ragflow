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

package connector

import (
	"encoding/json"
	"fmt"
)

// rdbmsQuery is one base SQL query plus the identity used by resume cursors:
// the table name in per-table mode, or "" for a single custom query.
type rdbmsQuery struct {
	name string
	sql  string
}

// rdbmsSyncQuery is one prepared sync query with its resume metadata.
type rdbmsSyncQuery struct {
	name     string
	sql      string
	ordered  bool
	fallback string
}

// rdbmsResumeCursor identifies where a MySQL/PostgreSQL sync left off: the
// query that produced the last committed batch, the ordering column the
// stream was sorted by, and the SourceID of the last emitted row (the resume
// anchor). It is stored JSON-encoded in SyncCheckpoint.Cursor so arbitrary
// table names and source ids survive the round trip.
type rdbmsResumeCursor struct {
	Query    string `json:"q"`
	Order    string `json:"o"`
	SourceID string `json:"s"`
}

// encodeRDBMSCursor serializes a resume cursor for SyncCheckpoint.Cursor.
func encodeRDBMSCursor(query, order, sourceID string) string {
	raw, _ := json.Marshal(rdbmsResumeCursor{Query: query, Order: order, SourceID: sourceID})
	return string(raw)
}

// parseRDBMSCursor decodes a resume cursor. A missing or malformed cursor is
// treated as invalid progress so the runner restarts the task window instead
// of guessing an offset.
func parseRDBMSCursor(cursor string) (rdbmsResumeCursor, error) {
	var c rdbmsResumeCursor
	if cursor == "" {
		return c, fmt.Errorf("rdbms sync checkpoint has no cursor: %w", ErrSyncResumeInvalid)
	}
	if err := json.Unmarshal([]byte(cursor), &c); err != nil {
		return c, fmt.Errorf("rdbms sync checkpoint cursor is malformed: %w", ErrSyncResumeInvalid)
	}
	if c.SourceID == "" {
		return c, fmt.Errorf("rdbms sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	return c, nil
}
