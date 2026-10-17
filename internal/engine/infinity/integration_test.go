//go:build integration

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

package infinity

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/server/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func newTestEngine(t *testing.T) (context.Context, *Engine) {
	t.Helper()
	uri := os.Getenv("RAGFLOW_INFINITY_URI")
	if uri == "" {
		t.Skip("RAGFLOW_INFINITY_URI is not set")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	t.Cleanup(cancel)
	engine, err := NewEngine(ctx, config.InfinityConfig{URI: uri, DBName: "default_db"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { engine.Close() })
	return ctx, engine
}

func observeLogs(t *testing.T) *observer.ObservedLogs {
	t.Helper()
	prevLogger := common.Logger
	t.Cleanup(func() { common.Logger = prevLogger })
	core, logs := observer.New(zapcore.InfoLevel)
	common.Logger = zap.New(core)
	return logs
}

// Admin user deletion drops one chunk table per owned dataset and the tenant's
// doc-meta table, and Infinity only creates either on first write.
func TestDropStoresToleratesNeverCreatedTables(t *testing.T) {
	ctx, engine := newTestEngine(t)
	logs := observeLogs(t)

	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := engine.DropChunkStore(ctx, "ragflow_tenant"+suffix, "kb"+suffix); err != nil {
		t.Fatalf("drop never-created chunk table: %v", err)
	}
	if err := engine.DropMetadataStore(ctx, "tenant"+suffix); err != nil {
		t.Fatalf("drop never-created metadata table: %v", err)
	}
	if dropped := logs.FilterMessage("Infinity dropped table").Len(); dropped != 0 {
		t.Fatalf("logged %d drops for tables that never existed", dropped)
	}
}

func TestDropMetadataStoreLogsAnActualDrop(t *testing.T) {
	ctx, engine := newTestEngine(t)
	tenantID := "tenant" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := engine.CreateMetadataStore(ctx, tenantID); err != nil {
		t.Fatal(err)
	}
	// t.Context is already cancelled when cleanups run
	t.Cleanup(func() { _ = engine.DropMetadataStore(context.Background(), tenantID) })
	logs := observeLogs(t)

	if err := engine.DropMetadataStore(ctx, tenantID); err != nil {
		t.Fatal(err)
	}
	if dropped := logs.FilterMessage("Infinity dropped table").Len(); dropped != 1 {
		t.Fatalf("logged %d drops, want 1", dropped)
	}
	exists, err := engine.MetadataStoreExists(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatalf("metadata table for %s still exists after drop", tenantID)
	}
}
