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
	"encoding/json"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// legacyParserConfig is a dataset parser config as written by a release that
// still carried the raptor and graphrag sections.
const legacyParserConfig = `{"llm_id":"Qwen/Qwen3-8B@qq@SILICONFLOW","raptor":{"use_raptor":true,"max_token":256,"threshold":0.1,"max_cluster":64,"random_seed":0},"graphrag":{"method":"light","use_graphrag":true,"entity_types":["organization","person"],"retry_attempts":2},"delimiter":"\n","topn_tags":3,"html4excel":false,"chunk_token_num":512,"layout_recognize":"DeepDOC","auto_keywords":0,"auto_questions":0}`

func setupKnowledgebaseParserConfigDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.SystemSettings{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func insertKnowledgebase(t *testing.T, db *gorm.DB, id, parserConfig string) {
	t.Helper()
	if err := db.Exec("INSERT INTO knowledgebase (id, tenant_id, name, embd_id, created_by, parser_config) VALUES (?, ?, ?, ?, ?, ?)",
		id, "tenant-"+id, "kb-"+id, "embd-"+id, "user-"+id, parserConfig).Error; err != nil {
		t.Fatalf("failed to insert knowledgebase %s: %v", id, err)
	}
}

func knowledgebaseParserConfigRaw(t *testing.T, db *gorm.DB, id string) string {
	t.Helper()
	var raw string
	if err := db.Table("knowledgebase").Where("id = ?", id).Pluck("parser_config", &raw).Error; err != nil {
		t.Fatalf("failed to read parser config %s: %v", id, err)
	}
	return raw
}

func knowledgebaseParserConfig(t *testing.T, db *gorm.DB, id string) map[string]interface{} {
	t.Helper()
	raw := knowledgebaseParserConfigRaw(t, db, id)
	config := make(map[string]interface{})
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		t.Fatalf("failed to parse parser config %s: %v", id, err)
	}
	return config
}

func TestMigrateKnowledgebaseParserConfigDropsRaptorAndGraphrag(t *testing.T) {
	db := setupKnowledgebaseParserConfigDB(t)
	insertKnowledgebase(t, db, "kb1", legacyParserConfig)

	if err := migrateIngestionParserConfigs(t.Context(), db); err != nil {
		t.Fatalf("migrateIngestionParserConfigs: %v", err)
	}

	config := knowledgebaseParserConfig(t, db, "kb1")
	for _, key := range removedParserConfigKeys {
		if _, ok := config[key]; ok {
			t.Fatalf("%s survived the migration: %#v", key, config)
		}
	}
	if got := config["llm_id"]; got != "Qwen/Qwen3-8B@qq@SILICONFLOW" {
		t.Fatalf("llm_id = %v, want the original value", got)
	}
	chunker := config["GeneralChunker:SixApplesFall"].(map[string]any)
	if got := chunker["chunk_token_size"]; got != float64(512) {
		t.Fatalf("chunk_token_size = %v, want 512", got)
	}
	parser := config["Parser:HipSignsRhyme"].(map[string]any)
	pdf := parser["pdf"].(map[string]any)
	if got := pdf["parse_method"]; got != "deepdoc" {
		t.Fatalf("parse_method = %v, want deepdoc", got)
	}
	if got := databaseVersion(t, db); got != ingestionParserConfigTargetVersion {
		t.Fatalf("database version = %q, want %q", got, ingestionParserConfigTargetVersion)
	}
}

func TestMigrateKnowledgebaseParserConfigKeepsCleanConfig(t *testing.T) {
	clean := `{"llm_id":"model","topn_tags":3}`
	db := setupKnowledgebaseParserConfigDB(t)
	insertKnowledgebase(t, db, "kb1", clean)

	if err := migrateIngestionParserConfigs(t.Context(), db); err != nil {
		t.Fatalf("migrateIngestionParserConfigs: %v", err)
	}

	config := knowledgebaseParserConfig(t, db, "kb1")
	if len(config) != 2 || config["llm_id"] != "model" {
		t.Fatalf("clean config changed: %#v", config)
	}
	if got := databaseVersion(t, db); got != ingestionParserConfigTargetVersion {
		t.Fatalf("database version = %q, want %q", got, ingestionParserConfigTargetVersion)
	}
}

func TestMigrateKnowledgebaseParserConfigSkipsMigratedDatabase(t *testing.T) {
	db := setupKnowledgebaseParserConfigDB(t)
	insertKnowledgebase(t, db, "kb1", legacyParserConfig)
	writeDatabaseVersion(t, db, ingestionParserConfigTargetVersion)

	if err := migrateIngestionParserConfigs(t.Context(), db); err != nil {
		t.Fatalf("migrateIngestionParserConfigs: %v", err)
	}

	config := knowledgebaseParserConfig(t, db, "kb1")
	if _, ok := config["raptor"]; !ok {
		t.Fatalf("raptor removed on an already migrated database: %#v", config)
	}
}

func TestMigrateKnowledgebaseParserConfigRejectsMalformedPayload(t *testing.T) {
	db := setupKnowledgebaseParserConfigDB(t)
	insertKnowledgebase(t, db, "kb0", legacyParserConfig)
	insertKnowledgebase(t, db, "kb1", "not-json")
	insertKnowledgebase(t, db, "kb2", legacyParserConfig)
	insertKnowledgebase(t, db, "kb3", "")

	if err := migrateIngestionParserConfigs(t.Context(), db); err == nil {
		t.Fatal("malformed payload did not stop migration")
	}
	for id, want := range map[string]string{"kb0": legacyParserConfig, "kb1": "not-json", "kb2": legacyParserConfig, "kb3": ""} {
		if got := knowledgebaseParserConfigRaw(t, db, id); got != want {
			t.Fatalf("%s: failed batch changed config to %q", id, got)
		}
	}
	if got := databaseVersion(t, db); got != "" {
		t.Fatalf("failed migration advanced version to %q", got)
	}
}

func TestMigrateKnowledgebaseParserConfigWithoutTable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.SystemSettings{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	if err := migrateIngestionParserConfigs(t.Context(), db); err != nil {
		t.Fatalf("migrateIngestionParserConfigs: %v", err)
	}
	if got := databaseVersion(t, db); got != "" {
		t.Fatalf("database version = %q, want it left unset", got)
	}
}
