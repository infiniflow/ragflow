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
	"errors"
	"time"
)

// ErrSyncResumeInvalid reports that a saved sync resume anchor no longer exists
// in the current source listing. The runner treats this as invalid progress and
// restarts the current task window instead of silently guessing a new offset.
var ErrSyncResumeInvalid = errors.New("sync resume checkpoint is no longer valid")

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

// SyncCheckpoint is a connector-owned resume point.
type SyncCheckpoint struct {
	Cursor    string     `json:"cursor,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	SourceID  string     `json:"source_id,omitempty"`
}

// SyncCheckpointState is the running checkpoint for one sync task.
type SyncCheckpointState struct {
	Version       int             `json:"version"`
	TaskID        string          `json:"task_id"`
	ConnectorID   string          `json:"connector_id"`
	KBID          string          `json:"kb_id"`
	WindowStart   *time.Time      `json:"window_start,omitempty"`
	WindowEnd     time.Time       `json:"window_end"`
	NextCommitSeq int64           `json:"next_commit_seq"`
	Checkpoint    *SyncCheckpoint `json:"checkpoint,omitempty"`
	RestartCount  int             `json:"restart_count,omitempty"`
	Added         int64           `json:"added,omitempty"`
	Updated       int64           `json:"updated,omitempty"`
	Skipped       int64           `json:"skipped,omitempty"`
	ErrorCount    int64           `json:"error_count,omitempty"`
	ErrorMsg      string          `json:"error_msg,omitempty"`
}

// SyncRequest describes one fixed sync window.
type SyncRequest struct {
	TaskID       string
	ConnectorID  string
	KBID         string
	SourceType   string
	Fingerprints map[string]string

	FromBeginning bool
	WindowStart   *time.Time
	WindowEnd     time.Time
	Resume        *SyncCheckpoint
}

// SyncBatch contains one serially processed batch.
type SyncBatch struct {
	Documents  []SourceDocument
	Checkpoint *SyncCheckpoint
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
