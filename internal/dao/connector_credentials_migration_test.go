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

package dao

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

const testConnectorKey = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="

func setupConnectorCredentialsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupSyncTaskTestDB(t)
	if err := db.AutoMigrate(&entity.Connector{}); err != nil {
		t.Fatalf("failed to migrate Connector: %v", err)
	}
	return db
}

func insertRawConnector(t *testing.T, db *gorm.DB, id, config string) {
	t.Helper()
	if err := db.Exec(
		"INSERT INTO connector (id, tenant_id, name, source, input_type, config, status) VALUES (?, ?, ?, ?, ?, ?, ?)",
		id, "tenant-1", id, "github", "poll", config, "schedule",
	).Error; err != nil {
		t.Fatalf("insert connector %q: %v", id, err)
	}
}

func rawConnectorConfig(t *testing.T, db *gorm.DB, id string) string {
	t.Helper()
	var config string
	if err := db.Raw("SELECT config FROM connector WHERE id = ?", id).Scan(&config).Error; err != nil {
		t.Fatalf("read connector %q: %v", id, err)
	}
	return config
}

func TestMigrateConnectorCredentialsEncryptsPlaintextRows(t *testing.T) {
	t.Setenv(common.EnvRAGFlowConnectorKey, testConnectorKey)
	db := setupConnectorCredentialsTestDB(t)
	ctx := context.Background()
	// The spacing differs from json.Marshal output, so any needless rewrite shows.
	untouched := map[string]string{
		"no-credentials": `{"wiki": "x"}`,
		"invalid-json":   `not json`,
		"encrypted":      `{"credentials": "enc:v1:abc"}`,
	}
	insertRawConnector(t, db, "plain", `{"credentials":{"api_token":"tok-123"},"wiki":"x"}`)
	for id, config := range untouched {
		insertRawConnector(t, db, id, config)
	}

	if err := migrateConnectorCredentials(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	migrated := rawConnectorConfig(t, db, "plain")
	var stored map[string]interface{}
	if err := json.Unmarshal([]byte(migrated), &stored); err != nil {
		t.Fatalf("migrated config %q is not JSON: %v", migrated, err)
	}
	if credentials, _ := stored["credentials"].(string); !strings.HasPrefix(credentials, "enc:v1:") {
		t.Fatalf("migrated config = %s, want enc:v1: credentials", migrated)
	}
	decrypted, err := common.DecryptConnectorCredentials(stored)
	if err != nil {
		t.Fatalf("decrypt migrated config: %v", err)
	}
	want := map[string]interface{}{"credentials": map[string]interface{}{"api_token": "tok-123"}, "wiki": "x"}
	if !reflect.DeepEqual(decrypted, want) {
		t.Fatalf("decrypted = %#v, want %#v", decrypted, want)
	}
	for id, config := range untouched {
		if got := rawConnectorConfig(t, db, id); got != config {
			t.Fatalf("connector %q config = %q, want it untouched (%q)", id, got, config)
		}
	}

	if err := migrateConnectorCredentials(ctx, db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if got := rawConnectorConfig(t, db, "plain"); got != migrated {
		t.Fatalf("second run rewrote the row: %q, want %q", got, migrated)
	}
}

func TestMigrateConnectorCredentialsCoversEveryBatch(t *testing.T) {
	t.Setenv(common.EnvRAGFlowConnectorKey, testConnectorKey)
	db := setupConnectorCredentialsTestDB(t)
	total := connectorCredentialsMigrationBatchSize + 1
	for i := 0; i < total; i++ {
		insertRawConnector(t, db, fmt.Sprintf("c%04d", i), `{"credentials":{"api_token":"tok-123"}}`)
	}

	if err := migrateConnectorCredentials(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var encrypted int64
	if err := db.Table("connector").Where("config LIKE ?", `%"enc:v1:%`).Count(&encrypted).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if encrypted != int64(total) {
		t.Fatalf("encrypted rows = %d, want %d", encrypted, total)
	}
}

func TestMigrateConnectorCredentialsWithoutKey(t *testing.T) {
	t.Setenv(common.EnvRAGFlowConnectorKey, "")
	db := setupConnectorCredentialsTestDB(t)
	plain := `{"credentials": {"api_token": "tok-123"}}`
	insertRawConnector(t, db, "plain", plain)

	if err := migrateConnectorCredentials(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if got := rawConnectorConfig(t, db, "plain"); got != plain {
		t.Fatalf("config = %q, want it untouched", got)
	}
}

func TestUpdateConnectorConfigIfUnchangedSkipsStaleRow(t *testing.T) {
	db := setupConnectorCredentialsTestDB(t)
	ctx := context.Background()
	insertRawConnector(t, db, "c1", `{"credentials":"new"}`)

	updated, err := updateConnectorConfigIfUnchanged(ctx, db, "c1", `{"credentials":"old"}`, `{"credentials":"enc:v1:x"}`)
	if err != nil || updated {
		t.Fatalf("stale update: got (%v, %v), want (false, nil)", updated, err)
	}
	if got := rawConnectorConfig(t, db, "c1"); got != `{"credentials":"new"}` {
		t.Fatalf("stale update overwrote the row: %q", got)
	}

	updated, err = updateConnectorConfigIfUnchanged(ctx, db, "c1", `{"credentials":"new"}`, `{"credentials":"enc:v1:x"}`)
	if err != nil || !updated {
		t.Fatalf("current update: got (%v, %v), want (true, nil)", updated, err)
	}
	if got := rawConnectorConfig(t, db, "c1"); got != `{"credentials":"enc:v1:x"}` {
		t.Fatalf("config = %q, want the new value", got)
	}
}

func TestConnectorWritesEncryptCredentials(t *testing.T) {
	t.Setenv(common.EnvRAGFlowConnectorKey, testConnectorKey)
	db := setupConnectorCredentialsTestDB(t)
	ctx := context.Background()
	connectorDAO := NewConnectorDAO()
	assertEncrypted := func(step string) {
		t.Helper()
		raw := rawConnectorConfig(t, db, "c1")
		if !strings.Contains(raw, `"credentials":"enc:v1:`) || strings.Contains(raw, "tok-") {
			t.Fatalf("%s stored %s, want encrypted credentials", step, raw)
		}
	}

	if err := connectorDAO.Create(ctx, db, &entity.Connector{
		ID: "c1", TenantID: "tenant-1", Name: "c1", Source: "github", InputType: "poll", Status: "schedule",
		Config: entity.ConnectorConfig{"credentials": map[string]interface{}{"api_token": "tok-123"}},
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	assertEncrypted("create")

	updates := map[string]interface{}{"config": entity.ConnectorConfig{"credentials": map[string]interface{}{"api_token": "tok-456"}}}
	if err := connectorDAO.UpdateByID(ctx, db, "c1", updates); err != nil {
		t.Fatalf("update: %v", err)
	}
	assertEncrypted("map update")
}
