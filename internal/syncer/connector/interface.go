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
	"context"
	"time"
)

const connectorSettingValidationTimeout = 7 * time.Second

// SyncSession streams source documents for one fixed sync window.
type SyncSession interface {
	// NextBatch returns the next source document batch.
	NextBatch(ctx context.Context) (SyncBatch, error)
	// Close releases connector resources.
	Close() error
}

// Fetcher lazily downloads a source document body.
type Fetcher interface {
	// Fetch downloads a delayed source document body.
	Fetch(ctx context.Context, ref FetchReference) ([]byte, error)
}

// SettingValidator validates connector settings from an unsaved config.
type SettingValidator interface {
	ValidateConnectorSetting(ctx context.Context, request map[string]any) error
}

// PruneSession streams a complete slim source snapshot.
type PruneSession interface {
	// NextBatch returns the next slim snapshot batch.
	NextBatch(ctx context.Context) (PruneBatch, error)
	// Close releases connector resources.
	Close() error
}

// Connector validates configuration and opens sync/prune sessions.
type Connector interface {
	// Validate validates connector configuration and credentials.
	Validate(ctx context.Context) error
	// OpenSync opens one synchronization session.
	OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error)
	// OpenPrune opens one complete prune snapshot session.
	OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error)
}
