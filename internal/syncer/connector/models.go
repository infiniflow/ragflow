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

import "time"

// SourceDocument is the normalized document emitted by datasource connectors.
type SourceDocument struct {
	SourceID           string
	SemanticIdentifier string
	Extension          string
	Blob               []byte
	FetchRef           *FetchReference
	UpdatedAt          time.Time
	SizeBytes          int64
	Metadata           map[string]any
	Fingerprint        string
}

// FetchReference describes a lazy document fetch.
type FetchReference struct {
	Key      string
	SizeHint int64
}

// SyncRequest describes one fixed sync window.
type SyncRequest struct {
	TaskID        string
	ConnectorID   string
	KBID          string
	FromBeginning bool
	WindowStart   *time.Time
	WindowEnd     time.Time
}

// SyncBatch contains one serially processed batch.
type SyncBatch struct {
	Documents []SourceDocument
}

// PruneRequest describes one complete prune snapshot request.
type PruneRequest struct {
	TaskID      string
	ConnectorID string
	KBID        string
}

// SlimDocument is the minimal prune snapshot row.
type SlimDocument struct {
	SourceID string
}

// PruneBatch contains one slim snapshot batch.
type PruneBatch struct {
	Documents []SlimDocument
}
