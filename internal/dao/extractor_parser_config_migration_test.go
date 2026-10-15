package dao

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"ragflow/internal/entity"
)

// Test bridges keep migration internals out of the production API.
var MigrateExtractorParserConfigsForTest = migrateExtractorParserConfigs
var MigrateExtractorConfigForTest = migrateExtractorConfig

const python027ParserConfig = `{"auto_keywords":8,"auto_questions":3,"enable_metadata":true,"metadata":[{"key":"author","description":"who wrote it"}],"built_in_metadata":[{"key":"update_time"}],"topn_tags":5,"tag_kb_ids":["tag-kb"]}`

func TestMigrateExtractorConfig(t *testing.T) {
	cases := []struct {
		name, parser, input, want string
		pipeline                  *string
	}{
		{name: "Python fields", parser: "naive", input: python027ParserConfig, want: `{"Extractor:AutoExtractDefault":{"keywords":{"top_n":8},"questions":{"top_n":3},"metadata":{"enabled":true,"metadata":[{"key":"author","description":"who wrote it"}],"built_in_metadata":[{"key":"update_time"}]}},"topn_tags":5,"tag_kb_ids":["tag-kb"]}`},
		{name: "missing metadata toggle stays disabled", input: `{"metadata":[{"key":"author"}]}`, want: `{"Extractor:AutoExtractDefault":{"metadata":{"enabled":false,"metadata":[{"key":"author"}],"built_in_metadata":[]}}}`},
		{name: "explicit zero and false", input: `{"auto_keywords":0,"enable_metadata":false,"metadata":[]}`, want: `{"Extractor:AutoExtractDefault":{"keywords":{"top_n":0},"metadata":{"enabled":false,"metadata":[],"built_in_metadata":[]}}}`},
		{name: "existing node values win over Python fields", input: `{"auto_keywords":8,"enable_metadata":true,"metadata":[{"key":"author"}],"Extractor:AutoExtractDefault":{"keywords":{"top_n":2,"enabled":false},"metadata":{"enabled":false,"metadata":[],"built_in_metadata":[]}}}`, want: `{"Extractor:AutoExtractDefault":{"keywords":{"top_n":2,"enabled":false},"metadata":{"enabled":false,"metadata":[],"built_in_metadata":[]}}}`},
		{name: "modular transport is authoritative", input: `{"metadata":{"enabled":true,"metadata":[{"key":"author"}],"built_in_metadata":[]},"Extractor:AutoExtractDefault":{"metadata":{"enabled":false}}}`, want: `{"Extractor:AutoExtractDefault":{"metadata":{"enabled":true,"metadata":[{"key":"author"}],"built_in_metadata":[]}}}`},
		{name: "empty modular transport disables metadata", input: `{"metadata":{},"Extractor:AutoExtractDefault":{"metadata":{"enabled":true}}}`, want: `{"Extractor:AutoExtractDefault":{"metadata":{"enabled":false,"metadata":[],"built_in_metadata":[]}}}`},
		{name: "flat schema", input: `{"enable_metadata":true,"metadata":{"type":"object","properties":{"author":{"type":"string","description":"who","enum":["A"]}},"additionalProperties":false}}`, want: `{"Extractor:AutoExtractDefault":{"metadata":{"enabled":true,"metadata":[{"key":"author","type":"string","description":"who","enum":["A"]}],"built_in_metadata":[]}}}`},
		{name: "description alias", input: `{"enable_metadata":true,"metadata":[{"key":"author","descriptions":"who"}]}`, want: `{"Extractor:AutoExtractDefault":{"metadata":{"enabled":true,"metadata":[{"key":"author","description":"who"}],"built_in_metadata":[]}}}`},
		{name: "tag settings untouched", input: `{"topn_tags":5,"tag_kb_ids":["tag-kb"],"tags":{"tag_file_id":"file"}}`, want: `{"topn_tags":5,"tag_kb_ids":["tag-kb"],"tags":{"tag_file_id":"file"}}`},
		{name: "early Go nodes untouched", input: `{"Extractor:AutoExtractDefault":{"auto_keywords":8,"keywords_sys_prompt":"saved","enable_summary":true,"sys_prompt":"saved summary","auto_tags":5,"tag_file_id":"file","metadata_config":{"enabled":true}}}`, want: `{"Extractor:AutoExtractDefault":{"auto_keywords":8,"keywords_sys_prompt":"saved","enable_summary":true,"sys_prompt":"saved summary","auto_tags":5,"tag_file_id":"file","metadata_config":{"enabled":true}}}`},
		{name: "custom node", input: `{"auto_questions":2,"Extractor:Custom":{"summary":{"enabled":true}}}`, want: `{"Extractor:Custom":{"questions":{"top_n":2},"summary":{"enabled":true}}}`},
		{name: "QA has no target", parser: "qa", input: python027ParserConfig, want: python027ParserConfig},
		{name: "table has no target", parser: "table", input: python027ParserConfig, want: python027ParserConfig},
		{name: "canvas has no target", pipeline: ptrString("canvas-1"), input: python027ParserConfig, want: python027ParserConfig},
		{name: "empty row", input: `{}`, want: `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config, want := decodeExtractorConfig(t, tc.input), decodeExtractorConfig(t, tc.want)
			parser := tc.parser
			if parser == "" {
				parser = "general"
			}
			if _, err := migrateExtractorConfig(config, parser, tc.pipeline); err != nil {
				t.Fatal(err)
			}
			if got := decodeExtractorConfig(t, encodeExtractorConfig(t, config)); !reflect.DeepEqual(got, want) {
				t.Fatalf("got %s; want %s", encodeExtractorConfig(t, config), tc.want)
			}
			snapshot := encodeExtractorConfig(t, config)
			if changed, err := migrateExtractorConfig(config, parser, tc.pipeline); err != nil || changed {
				t.Fatalf("second run changed=%v, err=%v", changed, err)
			}
			if encodeExtractorConfig(t, config) != snapshot {
				t.Fatal("second run changed config")
			}
		})
	}
}

func TestMigrateExtractorParserConfigsBatches(t *testing.T) {
	db := openExtractorMigrationDB(t)
	for i := 0; i < 257; i++ {
		for _, table := range []string{"knowledgebase", "document"} {
			insertExtractorConfig(t, db, table, fmt.Sprintf("%032d", i), python027ParserConfig)
		}
	}
	if err := migrateExtractorParserConfigs(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"knowledgebase", "document"} {
		var rows []extractorConfigMigrationRow
		if err := db.Table(table).Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
		if len(rows) != 257 {
			t.Fatalf("%s rows=%d", table, len(rows))
		}
		for _, row := range rows {
			config := decodeExtractorConfig(t, *row.ParserConfig)
			node, ok := config["Extractor:AutoExtractDefault"].(map[string]any)
			if !ok || node["keywords"].(map[string]any)["top_n"] != float64(8) || config["topn_tags"] != float64(5) {
				t.Fatalf("%s.%s not migrated correctly: %v", table, row.ID, config)
			}
		}
	}
	if got := databaseVersion(t, db); got != extractorParserConfigTargetVersion {
		t.Fatalf("version=%s", got)
	}
}

func TestMigrateExtractorParserConfigsRejectsInvalidRows(t *testing.T) {
	for _, raw := range []string{
		`not-json`,
		`{"auto_keywords":"8","auto_questions":2}`,
		`{"enable_metadata":true,"metadata":[{"name":"author"}]}`,
		`{"enable_metadata":true,"metadata":{"type":"object","required":["author"],"properties":{"author":{"type":"object","properties":{"name":{"type":"string"}}}}}}`,
	} {
		t.Run(raw, func(t *testing.T) {
			db := openExtractorMigrationDB(t)
			insertExtractorConfig(t, db, "knowledgebase", "a", python027ParserConfig)
			insertExtractorConfig(t, db, "knowledgebase", "b", raw)
			if err := migrateExtractorParserConfigs(t.Context(), db); err == nil {
				t.Fatal("invalid input was marked as migrated")
			}
			var rows []extractorConfigMigrationRow
			if err := db.Table("knowledgebase").Order("id").Find(&rows).Error; err != nil {
				t.Fatal(err)
			}
			if *rows[0].ParserConfig != python027ParserConfig || *rows[1].ParserConfig != raw {
				t.Fatal("failed batch modified source configs")
			}
			if databaseVersion(t, db) != "" {
				t.Fatal("failed migration advanced version")
			}
		})
	}
}

func TestMigrateExtractorParserConfigsVersionAndEmptyRows(t *testing.T) {
	for _, version := range []string{"", extractorParserConfigTargetVersion, "v1.0.0"} {
		t.Run(version, func(t *testing.T) {
			db := openExtractorMigrationDB(t)
			if version != "" {
				writeDatabaseVersion(t, db, version)
			}
			for i, raw := range []string{``, `null`, `{}`, `{"Extractor:AutoExtractDefault":{"metadata":{"enabled":false}}}`} {
				insertExtractorConfig(t, db, "knowledgebase", fmt.Sprint(i), raw)
			}
			if version != "" {
				insertExtractorConfig(t, db, "knowledgebase", "old", python027ParserConfig)
			}
			if err := migrateExtractorParserConfigs(t.Context(), db); err != nil {
				t.Fatal(err)
			}
			var rows []extractorConfigMigrationRow
			if err := db.Table("knowledgebase").Order("id").Find(&rows).Error; err != nil {
				t.Fatal(err)
			}
			for i, raw := range []string{``, `null`, `{}`, `{"Extractor:AutoExtractDefault":{"metadata":{"enabled":false}}}`} {
				if *rows[i].ParserConfig != raw {
					t.Fatal("untouched row rewritten")
				}
			}
			wantVersion := version
			if version == "" {
				wantVersion = extractorParserConfigTargetVersion
			} else if *rows[4].ParserConfig != python027ParserConfig {
				t.Fatal("version gate did not skip")
			}
			if databaseVersion(t, db) != wantVersion {
				t.Fatal("unexpected version")
			}
		})
	}
}

func openExtractorMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.Document{}, &entity.SystemSettings{}); err != nil {
		t.Fatal(err)
	}
	return db
}
func insertExtractorConfig(t *testing.T, db *gorm.DB, table, id, raw string) {
	t.Helper()
	values := map[string]any{"id": id, "parser_id": "general", "parser_config": raw, "created_by": "user"}
	if table == "knowledgebase" {
		values["tenant_id"] = "tenant"
		values["name"] = "dataset"
		values["embd_id"] = "embedding"
	} else {
		values["kb_id"] = "dataset"
		values["source_type"] = "local"
		values["type"] = "doc"
		values["suffix"] = "pdf"
	}
	if err := db.Table(table).Create(values).Error; err != nil {
		t.Fatal(err)
	}
}
func decodeExtractorConfig(t *testing.T, raw string) map[string]any {
	t.Helper()
	var config map[string]any
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		t.Fatal(err)
	}
	return config
}
func encodeExtractorConfig(t *testing.T, config map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
func ptrString(value string) *string { return &value }
