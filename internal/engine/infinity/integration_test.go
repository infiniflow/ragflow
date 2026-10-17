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

	"ragflow/internal/server/config"
)

// Admin user deletion drops one chunk table per owned dataset and the tenant's
// doc-meta table, and Infinity only creates either on first write.
func TestDropStoresToleratesNeverCreatedTables(t *testing.T) {
	uri := os.Getenv("RAGFLOW_INFINITY_URI")
	if uri == "" {
		t.Skip("RAGFLOW_INFINITY_URI is not set")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	engine, err := NewEngine(ctx, config.InfinityConfig{URI: uri, DBName: "default_db"})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := engine.DropChunkStore(ctx, "ragflow_tenant"+suffix, "kb"+suffix); err != nil {
		t.Fatalf("drop never-created chunk table: %v", err)
	}
	if err := engine.DropMetadataStore(ctx, "tenant"+suffix); err != nil {
		t.Fatalf("drop never-created metadata table: %v", err)
	}
}
