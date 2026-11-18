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

package mock

import (
	"context"
	"io"
	syncerconnector "ragflow/internal/syncer/connector"
)

// Connector is a programmable connector for syncer tests.
type Connector struct {
	ValidateErr                 error
	ValidateConnectorSettingErr error
	SyncBatches                 []syncerconnector.SyncBatch
	SyncErrAt                   int
	NextBatchResumeInvalidAt    int
	NextBatchResumeFirstSession bool
	PruneBatches                []syncerconnector.PruneBatch
	PruneErrAt                  int
	FetchBlobs                  map[string][]byte
	OnSyncBatch                 func(index int)
	OnPruneBatch                func(index int)
	SyncRequests                []syncerconnector.SyncRequest
	OpenSyncResumeInvalidAt     int
	OpenSyncResumeInvalidAlways bool
	openCount                   int
}

// Validate returns the configured validation error.
func (c *Connector) Validate(ctx context.Context) error {
	return c.ValidateErr
}

// ValidateConnectorSetting returns the configured settings validation error.
func (c *Connector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	return c.ValidateConnectorSettingErr
}

// OpenSync opens a mock sync session.
func (c *Connector) OpenSync(ctx context.Context, request syncerconnector.SyncRequest) (syncerconnector.SyncSession, error) {
	c.openCount++
	c.SyncRequests = append(c.SyncRequests, request)
	if c.OpenSyncResumeInvalidAlways || (c.OpenSyncResumeInvalidAt > 0 && c.openCount == c.OpenSyncResumeInvalidAt) {
		return nil, syncerconnector.ErrSyncResumeInvalid
	}
	return &SyncSession{connector: c, openCount: c.openCount}, nil
}

// OpenPrune opens a mock prune session.
func (c *Connector) OpenPrune(ctx context.Context, request syncerconnector.PruneRequest) (syncerconnector.PruneSession, error) {
	return &PruneSession{connector: c}, nil
}

// SyncSession streams configured sync batches.
type SyncSession struct {
	connector *Connector
	openCount int
	index     int
}

// NextBatch returns the next configured sync batch.
func (s *SyncSession) NextBatch(ctx context.Context) (syncerconnector.SyncBatch, error) {
	if s.connector.SyncErrAt > 0 && s.index+1 == s.connector.SyncErrAt {
		return syncerconnector.SyncBatch{}, io.ErrUnexpectedEOF
	}
	if s.connector.NextBatchResumeInvalidAt > 0 && s.index+1 == s.connector.NextBatchResumeInvalidAt {
		if !s.connector.NextBatchResumeFirstSession || s.openCount == 1 {
			return syncerconnector.SyncBatch{}, syncerconnector.ErrSyncResumeInvalid
		}
	}
	if s.index >= len(s.connector.SyncBatches) {
		return syncerconnector.SyncBatch{}, io.EOF
	}
	if s.connector.OnSyncBatch != nil {
		s.connector.OnSyncBatch(s.index)
	}
	batch := s.connector.SyncBatches[s.index]
	s.index++
	return batch, nil
}

// Fetch returns a configured lazy blob.
func (s *SyncSession) Fetch(ctx context.Context, ref syncerconnector.FetchReference) ([]byte, error) {
	return s.connector.FetchBlobs[ref.Key], nil
}

// Close closes the mock sync session.
func (s *SyncSession) Close() error {
	return nil
}

// PruneSession streams configured prune batches.
type PruneSession struct {
	connector *Connector
	index     int
}

// NextBatch returns the next configured prune batch.
func (s *PruneSession) NextBatch(ctx context.Context) (syncerconnector.PruneBatch, error) {
	if s.connector.PruneErrAt > 0 && s.index+1 == s.connector.PruneErrAt {
		return syncerconnector.PruneBatch{}, io.ErrUnexpectedEOF
	}
	if s.index >= len(s.connector.PruneBatches) {
		return syncerconnector.PruneBatch{}, io.EOF
	}
	if s.connector.OnPruneBatch != nil {
		s.connector.OnPruneBatch(s.index)
	}
	batch := s.connector.PruneBatches[s.index]
	s.index++
	return batch, nil
}

// Close closes the mock prune session.
func (s *PruneSession) Close() error {
	return nil
}
