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
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// MigrateExtractorParserConfigsForTest lets external tests verify the migration
// through the pipeline and component packages without adding a production API.
var MigrateExtractorParserConfigsForTest = migrateExtractorParserConfigs

// python027ParserConfig is a knowledgebase/document parser config as written
// by the Python 0.27.2 backend: extraction settings are top-level flat keys
// (auto_keywords, auto_questions, topn_tags, tag_kb_ids, enable_metadata plus
// the metadata/built_in_metadata field lists in {name,...} shape).
const python027ParserConfig = `{"layout_recognize":"DeepDOC","chunk_token_num":512,"delimiter":"\n",` +
	`"auto_keywords":8,"auto_questions":3,"topn_tags":5,"tag_kb_ids":["kb-tag-1"],` +
	`"enable_metadata":true,` +
	`"metadata":[{"name":"author","type":"string","description":"who wrote it","examples":[],"restrict_values":false}],` +
	`"built_in_metadata":[{"name":"update_time","type":"time"}]}`

func setupExtractorMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.Document{}, &entity.SystemSettings{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func insertExtractorKB(t *testing.T, db *gorm.DB, id, parserID, parserConfig string) {
	t.Helper()
	if err := db.Exec("INSERT INTO knowledgebase (id, tenant_id, name, embd_id, created_by, parser_id, parser_config) VALUES (?, ?, ?, ?, ?, ?, ?)",
		id, "tenant-"+id, "kb-"+id, "embd-"+id, "user-"+id, parserID, parserConfig).Error; err != nil {
		t.Fatalf("failed to insert knowledgebase %s: %v", id, err)
	}
}

func insertExtractorDocument(t *testing.T, db *gorm.DB, id, parserID, parserConfig string) {
	t.Helper()
	if err := db.Exec("INSERT INTO document (id, kb_id, created_by, parser_id, parser_config, source_type, type, suffix) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		id, "kb-doc", "user-"+id, parserID, parserConfig, "local", "doc", "pdf").Error; err != nil {
		t.Fatalf("failed to insert document %s: %v", id, err)
	}
}

func extractorRowParserConfig(t *testing.T, db *gorm.DB, table, id string) map[string]interface{} {
	t.Helper()
	var raw string
	if err := db.Table(table).Where("id = ?", id).Pluck("parser_config", &raw).Error; err != nil {
		t.Fatalf("failed to read parser config %s.%s: %v", table, id, err)
	}
	config := make(map[string]interface{})
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		t.Fatalf("failed to parse parser config %s.%s: %v", table, id, err)
	}
	return config
}

func extractorNode(t *testing.T, config map[string]interface{}) map[string]interface{} {
	t.Helper()
	node, ok := config[builtinExtractorNodeID].(map[string]interface{})
	if !ok {
		t.Fatalf("expected %s node, got keys %v", builtinExtractorNodeID, keysOf(config))
	}
	return node
}

func keysOf(config map[string]interface{}) []string {
	keys := make([]string, 0, len(config))
	for key := range config {
		keys = append(keys, key)
	}
	return keys
}

func assertNoFlatExtractorKeys(t *testing.T, config map[string]interface{}) {
	t.Helper()
	for _, key := range append(flatExtractorConfigKeys, "tags") {
		if _, ok := config[key]; ok {
			t.Fatalf("flat key %q survived the migration: %#v", key, config)
		}
	}
}

func TestMigrateExtractorParserConfigsPythonFlatKBRow(t *testing.T) {
	db := setupExtractorMigrationDB(t)
	insertExtractorKB(t, db, "kb1", "naive", python027ParserConfig)

	if err := migrateExtractorParserConfigs(t.Context(), db); err != nil {
		t.Fatalf("migrateExtractorParserConfigs: %v", err)
	}

	config := extractorRowParserConfig(t, db, "knowledgebase", "kb1")
	assertNoFlatExtractorKeys(t, config)
	// Non-extraction flat keys are out of scope and must be preserved.
	if got := config["chunk_token_num"]; got != float64(512) {
		t.Fatalf("chunk_token_num = %v, want 512", got)
	}
	if got := config["layout_recognize"]; got != "DeepDOC" {
		t.Fatalf("layout_recognize = %v, want DeepDOC", got)
	}

	node := extractorNode(t, config)
	if got := node["keywords"].(map[string]interface{})["top_n"]; got != float64(8) {
		t.Fatalf("keywords.top_n = %v, want 8", got)
	}
	if got := node["questions"].(map[string]interface{})["top_n"]; got != float64(3) {
		t.Fatalf("questions.top_n = %v, want 3", got)
	}
	if got := node["tags"].(map[string]interface{})["top_n"]; got != float64(5) {
		t.Fatalf("tags.top_n = %v, want 5", got)
	}
	meta := node["metadata"].(map[string]interface{})
	if enabled, _ := meta["enabled"].(bool); !enabled {
		t.Fatalf("metadata.enabled = %v, want true", meta["enabled"])
	}
	fields := meta["metadata"].([]interface{})
	if len(fields) != 1 {
		t.Fatalf("metadata.metadata = %#v, want 1 field", meta["metadata"])
	}
	field := fields[0].(map[string]interface{})
	if field["key"] != "author" || field["type"] != "string" || field["description"] != "who wrote it" {
		t.Fatalf("metadata field = %#v, want key/type/description preserved", field)
	}
	if _, ok := field["examples"]; ok {
		t.Fatalf("examples survived the migration: %#v", field)
	}
	if _, ok := field["restrict_values"]; ok {
		t.Fatalf("restrict_values survived the migration: %#v", field)
	}
	builtIn := meta["built_in_metadata"].([]interface{})
	if len(builtIn) != 1 || builtIn[0].(map[string]interface{})["key"] != "update_time" {
		t.Fatalf("built_in_metadata = %#v, want the update_time field", meta["built_in_metadata"])
	}
	if got := databaseVersion(t, db); got != extractorParserConfigTargetVersion {
		t.Fatalf("database version = %q, want %q", got, extractorParserConfigTargetVersion)
	}
}

func TestMigrateExtractorParserConfigsDocumentRow(t *testing.T) {
	db := setupExtractorMigrationDB(t)
	insertExtractorDocument(t, db, "doc1", "general", python027ParserConfig)

	if err := migrateExtractorParserConfigs(t.Context(), db); err != nil {
		t.Fatalf("migrateExtractorParserConfigs: %v", err)
	}

	config := extractorRowParserConfig(t, db, "document", "doc1")
	assertNoFlatExtractorKeys(t, config)
	node := extractorNode(t, config)
	if got := node["keywords"].(map[string]interface{})["top_n"]; got != float64(8) {
		t.Fatalf("keywords.top_n = %v, want 8", got)
	}
	if got := databaseVersion(t, db); got != extractorParserConfigTargetVersion {
		t.Fatalf("database version = %q, want %q", got, extractorParserConfigTargetVersion)
	}
}

func TestMigrateExtractorParserConfigsRespectsExistingGroups(t *testing.T) {
	db := setupExtractorMigrationDB(t)
	row := `{"auto_keywords":8,"topn_tags":5,"enable_metadata":true,` +
		`"metadata":[{"name":"fresh","type":"string"}],` +
		`"Extractor:AutoExtractDefault":{"keywords":{"top_n":2},"tags":{"top_n":7,"tag_file_id":"file-1"}}}`
	insertExtractorKB(t, db, "kb1", "general", row)

	if err := migrateExtractorParserConfigs(t.Context(), db); err != nil {
		t.Fatalf("migrateExtractorParserConfigs: %v", err)
	}

	node := extractorNode(t, extractorRowParserConfig(t, db, "knowledgebase", "kb1"))
	if got := node["keywords"].(map[string]interface{})["top_n"]; got != float64(2) {
		t.Fatalf("keywords.top_n = %v, want the existing 2, not the flat 8", got)
	}
	tags := node["tags"].(map[string]interface{})
	if got := tags["top_n"]; got != float64(7) {
		t.Fatalf("tags.top_n = %v, want the existing 7, not the flat 5", got)
	}
	if tags["tag_file_id"] != "file-1" {
		t.Fatalf("tags.tag_file_id = %v, want file-1", tags["tag_file_id"])
	}
	// No metadata group preset: the flat metadata trio still migrates.
	if _, ok := node["metadata"].(map[string]interface{}); !ok {
		t.Fatalf("metadata group missing: %#v", node)
	}
}

func TestMigrateExtractorParserConfigsLegacyNodeFields(t *testing.T) {
	db := setupExtractorMigrationDB(t)
	row := `{"Extractor:AutoExtractDefault":{` +
		`"auto_keywords":4,"keywords_sys_prompt":"kw prompt",` +
		`"auto_questions":2,"questions_sys_prompt":"q prompt",` +
		`"auto_tags":6,"tag_file_id":"file-9",` +
		`"enable_summary":true,"sys_prompt":"sum prompt",` +
		`"metadata_config":{"enabled":true,"metadata":[{"key":"k","type":"string"}],"built_in_metadata":[]},` +
		`"field_name":"summary"}}`
	insertExtractorKB(t, db, "kb1", "general", row)

	if err := migrateExtractorParserConfigs(t.Context(), db); err != nil {
		t.Fatalf("migrateExtractorParserConfigs: %v", err)
	}

	node := extractorNode(t, extractorRowParserConfig(t, db, "knowledgebase", "kb1"))
	for _, key := range []string{"auto_keywords", "keywords_sys_prompt", "auto_questions",
		"questions_sys_prompt", "auto_tags", "tag_file_id", "enable_summary",
		"sys_prompt", "metadata_config", "field_name"} {
		if _, ok := node[key]; ok {
			t.Fatalf("legacy node field %q survived: %#v", key, node)
		}
	}
	kw := node["keywords"].(map[string]interface{})
	if kw["top_n"] != float64(4) || kw["system_prompt"] != "kw prompt" {
		t.Fatalf("keywords = %#v", kw)
	}
	q := node["questions"].(map[string]interface{})
	if q["top_n"] != float64(2) || q["system_prompt"] != "q prompt" {
		t.Fatalf("questions = %#v", q)
	}
	tags := node["tags"].(map[string]interface{})
	if tags["top_n"] != float64(6) || tags["tag_file_id"] != "file-9" {
		t.Fatalf("tags = %#v", tags)
	}
	sum := node["summary"].(map[string]interface{})
	if enabled, _ := sum["enabled"].(bool); !enabled || sum["system_prompt"] != "sum prompt" {
		t.Fatalf("summary = %#v", sum)
	}
	meta := node["metadata"].(map[string]interface{})
	if enabled, _ := meta["enabled"].(bool); !enabled {
		t.Fatalf("metadata.enabled = %v, want true", meta["enabled"])
	}
}

func TestMigrateExtractorParserConfigsCanvasRowKeepsNoNode(t *testing.T) {
	db := setupExtractorMigrationDB(t)
	if err := db.Exec("INSERT INTO knowledgebase (id, tenant_id, name, embd_id, created_by, parser_id, pipeline_id, parser_config) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		"kb1", "tenant-kb1", "kb-kb1", "embd-kb1", "user-kb1", "", "abc123abc123abc123abc123abc12345", python027ParserConfig).Error; err != nil {
		t.Fatalf("failed to insert canvas knowledgebase: %v", err)
	}

	if err := migrateExtractorParserConfigs(t.Context(), db); err != nil {
		t.Fatalf("migrateExtractorParserConfigs: %v", err)
	}

	config := extractorRowParserConfig(t, db, "knowledgebase", "kb1")
	if config["auto_keywords"] != float64(8) || config["enable_metadata"] != true {
		t.Fatalf("settings without an Extractor target were discarded: %#v", config)
	}
	if tags, ok := config["tags"].(map[string]interface{}); !ok || tags["top_n"] != float64(5) {
		t.Fatalf("retrieval tag count was discarded: %#v", config)
	}
	for key := range config {
		if strings.HasPrefix(strings.ToLower(key), "extractor:") {
			t.Fatalf("fabricated an extractor node on a canvas row: %#v", config)
		}
	}
}

func TestMigrateExtractorParserConfigsQaTemplateKeepsNoNode(t *testing.T) {
	db := setupExtractorMigrationDB(t)
	insertExtractorKB(t, db, "kb1", "qa", python027ParserConfig)

	if err := migrateExtractorParserConfigs(t.Context(), db); err != nil {
		t.Fatalf("migrateExtractorParserConfigs: %v", err)
	}

	config := extractorRowParserConfig(t, db, "knowledgebase", "kb1")
	if config["auto_keywords"] != float64(8) || config["enable_metadata"] != true {
		t.Fatalf("settings without an Extractor target were discarded: %#v", config)
	}
	if tags, ok := config["tags"].(map[string]interface{}); !ok || tags["top_n"] != float64(5) {
		t.Fatalf("retrieval tag count was discarded: %#v", config)
	}
	for key := range config {
		if strings.HasPrefix(strings.ToLower(key), "extractor:") {
			t.Fatalf("fabricated an extractor node for the qa template: %#v", config)
		}
	}
}

func TestMigrateExtractorParserConfigsKeepsCleanRowUntouched(t *testing.T) {
	db := setupExtractorMigrationDB(t)
	clean := `{"GeneralChunker:SixApplesFall":{"chunk_token_size":512},` +
		`"Extractor:AutoExtractDefault":{"keywords":{"top_n":0},"metadata":{"enabled":false,"metadata":[],"built_in_metadata":[]}}}`
	insertExtractorKB(t, db, "kb1", "general", clean)

	if err := migrateExtractorParserConfigs(t.Context(), db); err != nil {
		t.Fatalf("migrateExtractorParserConfigs: %v", err)
	}

	var raw string
	if err := db.Table("knowledgebase").Where("id = ?", "kb1").Pluck("parser_config", &raw).Error; err != nil {
		t.Fatalf("failed to read parser config: %v", err)
	}
	if raw != clean {
		t.Fatalf("clean row rewritten:\n got: %s\nwant: %s", raw, clean)
	}
	if got := databaseVersion(t, db); got != extractorParserConfigTargetVersion {
		t.Fatalf("database version = %q, want %q", got, extractorParserConfigTargetVersion)
	}
}

func TestMigrateExtractorParserConfigsSkipsMigratedDatabase(t *testing.T) {
	db := setupExtractorMigrationDB(t)
	insertExtractorKB(t, db, "kb1", "general", python027ParserConfig)
	writeDatabaseVersion(t, db, extractorParserConfigTargetVersion)

	if err := migrateExtractorParserConfigs(t.Context(), db); err != nil {
		t.Fatalf("migrateExtractorParserConfigs: %v", err)
	}

	var raw string
	if err := db.Table("knowledgebase").Where("id = ?", "kb1").Pluck("parser_config", &raw).Error; err != nil {
		t.Fatalf("failed to read parser config: %v", err)
	}
	if raw != python027ParserConfig {
		t.Fatalf("migrated row rewritten on an already migrated database:\n got: %s\nwant: %s", raw, python027ParserConfig)
	}
}

func TestMigrateExtractorParserConfigsToleratesMalformedPayload(t *testing.T) {
	db := setupExtractorMigrationDB(t)
	insertExtractorKB(t, db, "kb1", "general", "not-json")
	insertExtractorKB(t, db, "kb2", "general", "")
	insertExtractorKB(t, db, "kb3", "general", python027ParserConfig)

	if err := migrateExtractorParserConfigs(t.Context(), db); err != nil {
		t.Fatalf("migrateExtractorParserConfigs: %v", err)
	}

	var raw string
	if err := db.Table("knowledgebase").Where("id = ?", "kb1").Pluck("parser_config", &raw).Error; err != nil {
		t.Fatalf("failed to read parser config kb1: %v", err)
	}
	if raw != "not-json" {
		t.Fatalf("malformed config rewritten as %q", raw)
	}
	if err := db.Table("knowledgebase").Where("id = ?", "kb2").Pluck("parser_config", &raw).Error; err != nil {
		t.Fatalf("failed to read parser config kb2: %v", err)
	}
	if raw != "" {
		t.Fatalf("empty config rewritten as %q", raw)
	}
	config := extractorRowParserConfig(t, db, "knowledgebase", "kb3")
	assertNoFlatExtractorKeys(t, config)
	if got := databaseVersion(t, db); got != extractorParserConfigTargetVersion {
		t.Fatalf("database version = %q, want %q", got, extractorParserConfigTargetVersion)
	}
}

func TestMigrateExtractorParserConfigsWithoutTables(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.SystemSettings{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	if err := migrateExtractorParserConfigs(t.Context(), db); err != nil {
		t.Fatalf("migrateExtractorParserConfigs: %v", err)
	}
	if got := databaseVersion(t, db); got != "" {
		t.Fatalf("database version = %q, want it left unset", got)
	}
}

func TestMigrateExtractorConfigIsIdempotent(t *testing.T) {
	config := map[string]interface{}{}
	if err := json.Unmarshal([]byte(python027ParserConfig), &config); err != nil {
		t.Fatalf("failed to parse fixture: %v", err)
	}
	if !migrateExtractorConfig(config, "general", nil) {
		t.Fatalf("first run reported no change on a legacy config")
	}
	snapshot, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("failed to snapshot: %v", err)
	}
	if migrateExtractorConfig(config, "general", nil) {
		t.Fatalf("second run reported a change; migration is not idempotent: %#v", config)
	}
	resnapshot, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("failed to re-snapshot: %v", err)
	}
	if string(snapshot) != string(resnapshot) {
		t.Fatalf("config changed between runs:\nfirst:  %s\nsecond: %s", snapshot, resnapshot)
	}
}

func TestMigrateExtractorConfigPreservesSettings(t *testing.T) {
	tests := []struct {
		name, parserID, input, want string
	}{
		{
			name: "modular metadata transport", parserID: "general",
			input: `{"metadata":{"enabled":true,"metadata":[{"key":"author","type":"string"}],"built_in_metadata":[{"key":"file_name"}]}}`,
			want:  `{"Extractor:AutoExtractDefault":{"metadata":{"enabled":true,"metadata":[{"key":"author","type":"string"}],"built_in_metadata":[{"key":"file_name"}]}}}`,
		},
		{
			name: "modular metadata with flat builtin fields", parserID: "general",
			input: `{"metadata":{"enabled":true,"metadata":[{"key":"author"}]},"built_in_metadata":[{"name":"file_name"}]}`,
			want:  `{"Extractor:AutoExtractDefault":{"metadata":{"enabled":true,"metadata":[{"key":"author"}],"built_in_metadata":[{"key":"file_name"}]}}}`,
		},
		{
			name: "empty node metadata accepts flat fields", parserID: "general",
			input: `{"enable_metadata":true,"metadata":[{"name":"author"}],"Extractor:AutoExtractDefault":{"metadata":{}}}`,
			want:  `{"Extractor:AutoExtractDefault":{"metadata":{"enabled":true,"metadata":[{"key":"author"}],"built_in_metadata":[]}}}`,
		},
		{
			name: "metadata config respects flat disable", parserID: "general",
			input: `{"Extractor:AutoExtractDefault":{"enable_metadata":false,"metadata_config":{"metadata":[{"key":"author"}]}}}`,
			want:  `{"Extractor:AutoExtractDefault":{"metadata":{"enabled":false,"metadata":[{"key":"author"}],"built_in_metadata":[]}}}`,
		},
		{
			name: "metadata fills missing group fields", parserID: "general",
			input: `{"metadata":{"enabled":true,"metadata":[{"key":"author"}],"built_in_metadata":[]},"Extractor:AutoExtractDefault":{"metadata":{"enabled":false}}}`,
			want:  `{"Extractor:AutoExtractDefault":{"metadata":{"enabled":false,"metadata":[{"key":"author"}],"built_in_metadata":[]}}}`,
		},
		{
			name: "tag source creates group", parserID: "general",
			input: `{"tags":{"top_n":5,"tag_file_id":"file-1","enabled":false}}`,
			want:  `{"Extractor:AutoExtractDefault":{"tags":{"top_n":5,"tag_file_id":"file-1","enabled":false}}}`,
		},
		{
			name: "tag group beats flat count", parserID: "general",
			input: `{"topn_tags":9,"tags":{"top_n":5,"tag_file_id":"file-1"}}`,
			want:  `{"Extractor:AutoExtractDefault":{"tags":{"top_n":5,"tag_file_id":"file-1"}}}`,
		},
		{
			name: "qa retrieval tags", parserID: "qa",
			input: `{"topn_tags":5,"tags":{"tag_file_id":"file-1"}}`,
			want:  `{"tags":{"top_n":5,"tag_file_id":"file-1"}}`,
		},
		{
			name: "unconverted extraction settings", parserID: "table",
			input: `{"auto_keywords":8,"enable_metadata":true,"metadata":[{"name":"author"}]}`,
			want:  `{"auto_keywords":8,"enable_metadata":true,"metadata":[{"name":"author"}]}`,
		},
		{
			name: "tag dataset references", parserID: "general",
			input: `{"topn_tags":5,"tag_kb_ids":["tag-kb-1"]}`,
			want:  `{"Extractor:AutoExtractDefault":{"tags":{"top_n":5}},"tag_kb_ids":["tag-kb-1"]}`,
		},
		{
			name: "prompt before flat count", parserID: "general",
			input: `{"auto_keywords":8,"auto_questions":2,"Extractor:AutoExtractDefault":{"keywords_sys_prompt":"keywords prompt","questions_sys_prompt":"questions prompt"}}`,
			want:  `{"Extractor:AutoExtractDefault":{"keywords":{"top_n":8,"system_prompt":"keywords prompt"},"questions":{"top_n":2,"system_prompt":"questions prompt"}}}`,
		},
		{
			name: "node schema metadata", parserID: "general",
			input: `{"Extractor:AutoExtractDefault":{"enable_metadata":true,"metadata":{"type":"object","properties":{"author":{"type":"string"}}},"built_in_metadata":[{"name":"file_name"}]}}`,
			want:  `{"Extractor:AutoExtractDefault":{"metadata":{"enabled":true,"metadata":[{"key":"author","type":"string"}],"built_in_metadata":[{"key":"file_name"}]}}}`,
		},
		{
			name: "node partial metadata", parserID: "general",
			input: `{"Extractor:AutoExtractDefault":{"enable_metadata":true,"metadata":{"metadata":[{"key":"author"}]},"built_in_metadata":[{"name":"file_name"}]}}`,
			want:  `{"Extractor:AutoExtractDefault":{"metadata":{"enabled":true,"metadata":[{"key":"author"}],"built_in_metadata":[{"key":"file_name"}]}}}`,
		},
		{
			name: "summary fills enabled", parserID: "general",
			input: `{"Extractor:AutoExtractDefault":{"enable_summary":true,"summary":{"system_prompt":"saved prompt"},"sys_prompt":"old prompt"}}`,
			want:  `{"Extractor:AutoExtractDefault":{"summary":{"enabled":true,"system_prompt":"saved prompt"}}}`,
		},
		{
			name: "empty row stays empty", parserID: "general", input: `{}`, want: `{}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config, want map[string]interface{}
			if err := json.Unmarshal([]byte(tt.input), &config); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tt.want), &want); err != nil {
				t.Fatal(err)
			}
			migrateExtractorConfig(config, tt.parserID, nil)
			raw, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]interface{}
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("migrated config = %s, want %s", raw, tt.want)
			}
			if migrateExtractorConfig(config, tt.parserID, nil) {
				t.Fatal("second migration changed config")
			}
		})
	}
}

func TestMigrateExtractorParserConfigsAcrossBatches(t *testing.T) {
	db := setupExtractorMigrationDB(t)
	for i := 0; i < 257; i++ {
		id := fmt.Sprintf("%032d", i)
		insertExtractorKB(t, db, id, "general", `{"auto_keywords":8}`)
		insertExtractorDocument(t, db, id, "general", `{"auto_questions":2}`)
	}
	if err := migrateExtractorParserConfigs(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 257; i++ {
		id := fmt.Sprintf("%032d", i)
		kbNode := extractorNode(t, extractorRowParserConfig(t, db, "knowledgebase", id))
		docNode := extractorNode(t, extractorRowParserConfig(t, db, "document", id))
		if kbNode["keywords"].(map[string]interface{})["top_n"] != float64(8) || docNode["questions"].(map[string]interface{})["top_n"] != float64(2) {
			t.Fatalf("row %s was not migrated", id)
		}
	}
}

// TestBuiltinExtractorNodeMatchesTemplates pins the migration's Extractor
// node assumptions to the builtin pipeline templates: every builtin parser
// with an Extractor node must resolve to that exact node id, and every
// builtin parser without one must resolve to no node.
func TestBuiltinExtractorNodeMatchesTemplates(t *testing.T) {
	templateDir := filepath.Join("..", "ingestion", "pipeline", "template")
	entries, err := os.ReadDir(templateDir)
	if err != nil {
		t.Fatalf("failed to list templates: %v", err)
	}
	type templateInfo struct {
		ParserIDs []string `json:"parser_ids"`
		DSL       struct {
			Components map[string]struct {
				Obj struct {
					ComponentName string `json:"component_name"`
				} `json:"obj"`
			} `json:"components"`
		} `json:"dsl"`
	}
	checked := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(templateDir, entry.Name()))
		if err != nil {
			t.Fatalf("failed to read %s: %v", entry.Name(), err)
		}
		var info templateInfo
		if err := json.Unmarshal(raw, &info); err != nil {
			t.Fatalf("failed to parse %s: %v", entry.Name(), err)
		}
		var extractorNodeID string
		for cpnID, comp := range info.DSL.Components {
			if comp.Obj.ComponentName == "Extractor" {
				if extractorNodeID != "" {
					t.Fatalf("%s carries two extractor nodes (%q and %q); the migration targets one",
						entry.Name(), extractorNodeID, cpnID)
				}
				extractorNodeID = cpnID
			}
		}
		for _, parserID := range info.ParserIDs {
			checked++
			if extractorNodeID == "" {
				if got := builtinExtractorNodeIDFor(parserID, nil); got != "" {
					t.Fatalf("template %s (parser %q) has no extractor node, but the migration would create %q",
						entry.Name(), parserID, got)
				}
				continue
			}
			if extractorNodeID != builtinExtractorNodeID {
				t.Fatalf("template %s extractor node is %q, want %q",
					entry.Name(), extractorNodeID, builtinExtractorNodeID)
			}
			if got := builtinExtractorNodeIDFor(parserID, nil); got != builtinExtractorNodeID {
				t.Fatalf("parser %q has extractor node %q but the migration resolves %q",
					parserID, extractorNodeID, got)
			}
		}
	}
	// "general" is the canonical builtin id rows carry (the Go default); it is
	// not listed in any template's parser_ids, so it needs its own assertion.
	if got := builtinExtractorNodeIDFor("general", nil); got != builtinExtractorNodeID {
		t.Fatalf("builtinExtractorNodeIDFor(general) = %q, want %q", got, builtinExtractorNodeID)
	}
	if checked == 0 {
		t.Fatalf("no templates checked; template dir layout changed?")
	}
}
