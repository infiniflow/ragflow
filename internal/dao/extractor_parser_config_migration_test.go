package dao_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component"
	"ragflow/internal/ingestion/pipeline"
	"ragflow/internal/service"
)

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
		{name: "custom node", pipeline: ptrString("canvas-1"), input: `{"auto_questions":2,"Extractor:Custom":{"summary":{"enabled":true}}}`, want: `{"Extractor:Custom":{"questions":{"top_n":2},"summary":{"enabled":true}}}`},
		{name: "QA has no target", parser: "qa", input: python027ParserConfig, want: python027ParserConfig},
		{name: "table has no target", parser: "table", input: python027ParserConfig, want: python027ParserConfig},
		{name: "QA ignores stale extractor", parser: "qa", input: `{"auto_keywords":8,"Extractor:Old":{}}`, want: `{"auto_keywords":8,"Extractor:Old":{}}`},
		{name: "table ignores stale extractor", parser: "table", input: `{"auto_keywords":8,"Extractor:Old":{}}`, want: `{"auto_keywords":8,"Extractor:Old":{}}`},
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
			if _, err := dao.MigrateExtractorConfigForTest(config, parser, tc.pipeline); err != nil {
				t.Fatal(err)
			}
			if got := decodeExtractorConfig(t, encodeExtractorConfig(t, config)); !reflect.DeepEqual(got, want) {
				t.Fatalf("got %s; want %s", encodeExtractorConfig(t, config), tc.want)
			}
			snapshot := encodeExtractorConfig(t, config)
			if changed, err := dao.MigrateExtractorConfigForTest(config, parser, tc.pipeline); err != nil || changed {
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
	if err := dao.MigrateExtractorParserConfigsForTest(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"knowledgebase", "document"} {
		var rows []struct {
			ID           string
			ParserConfig *string
		}
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
	if got := dao.ExtractorDatabaseVersionForTest(t, db); got != "v1.0.0-rc2.dev2" {
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
			if err := dao.MigrateExtractorParserConfigsForTest(t.Context(), db); err == nil {
				t.Fatal("invalid input was marked as migrated")
			}
			var rows []struct {
				ID           string
				ParserConfig *string
			}
			if err := db.Table("knowledgebase").Order("id").Find(&rows).Error; err != nil {
				t.Fatal(err)
			}
			if *rows[0].ParserConfig != python027ParserConfig || *rows[1].ParserConfig != raw {
				t.Fatal("failed batch modified source configs")
			}
			if dao.ExtractorDatabaseVersionForTest(t, db) != "" {
				t.Fatal("failed migration advanced version")
			}
		})
	}
}

func TestMigrateExtractorParserConfigsVersionAndEmptyRows(t *testing.T) {
	for _, version := range []string{"", "v1.0.0-rc2.dev2", "v1.0.0"} {
		t.Run(version, func(t *testing.T) {
			db := openExtractorMigrationDB(t)
			if version != "" {
				dao.WriteExtractorDatabaseVersionForTest(t, db, version)
			}
			for i, raw := range []string{``, `null`, `{}`, `{"Extractor:AutoExtractDefault":{"metadata":{"enabled":false}}}`} {
				insertExtractorConfig(t, db, "knowledgebase", fmt.Sprint(i), raw)
			}
			if version != "" {
				insertExtractorConfig(t, db, "knowledgebase", "old", python027ParserConfig)
			}
			if err := dao.MigrateExtractorParserConfigsForTest(t.Context(), db); err != nil {
				t.Fatal(err)
			}
			var rows []struct {
				ID           string
				ParserConfig *string
			}
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
				wantVersion = "v1.0.0-rc2.dev2"
			} else if *rows[4].ParserConfig != python027ParserConfig {
				t.Fatal("version gate did not skip")
			}
			if dao.ExtractorDatabaseVersionForTest(t, db) != wantVersion {
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

func TestMigratedExtractorConfigReachesRuntime(t *testing.T) {
	db := openExtractorMigrationDB(t)
	input := `{"auto_keywords":8,"auto_questions":2,"enable_metadata":true,"metadata":[{"key":"author","type":"string"}],"built_in_metadata":[{"key":"file_name","type":"string"}],"Extractor:AutoExtractDefault":{"tags":{"top_n":5,"tag_file_id":"tag-file"},"keywords":{"system_prompt":"custom keywords"},"summary":{"enabled":true,"system_prompt":"custom summary"}}}`
	insertExtractorConfig(t, db, "knowledgebase", "kb-1", input)
	if err := dao.MigrateExtractorParserConfigsForTest(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := db.Table("knowledgebase").Where("id = ?", "kb-1").Pluck("parser_config", &raw).Error; err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		t.Fatal(err)
	}
	registry, err := pipeline.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	template, ok := registry.Get("general")
	if !ok {
		t.Fatal("general template missing")
	}
	dsl, err := json.Marshal(template.DSL)
	if err != nil {
		t.Fatal(err)
	}
	cleaned := service.ApplyComponentScopedParserConfig(pipeline.BuildParserConfig(dsl, config), "tenant-llm")
	c, err := component.NewExtractorComponent(cleaned["Extractor:AutoExtractDefault"].(map[string]any))
	if err != nil {
		t.Fatal(err)
	}
	got := c.(*component.ExtractorComponent).Param
	if !got.Keywords.Enabled || got.Keywords.TopN != 8 || got.Keywords.SystemPrompt != "custom keywords" || !got.Questions.Enabled || got.Questions.TopN != 2 {
		t.Fatalf("keyword/question settings lost: %#v", got)
	}
	if !got.Metadata.Enabled || len(got.Metadata.Metadata) != 1 || got.Metadata.Metadata[0].Key != "author" || len(got.Metadata.BuiltInMetadata) != 1 || got.Metadata.BuiltInMetadata[0].Key != "file_name" {
		t.Fatalf("metadata=%#v", got.Metadata)
	}
	if !got.Tags.Enabled || got.Tags.TagFileID != "tag-file" || got.Tags.TopN != 5 || !got.Summary.Enabled || got.Summary.SystemPrompt != "custom summary" || got.LLMID != "tenant-llm" {
		t.Fatalf("existing scoped settings lost: %#v", got)
	}
}

func TestExtractorMigrationMatchesBuiltinTemplates(t *testing.T) {
	registry, err := pipeline.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range append(registry.Refs(), "naive") {
		t.Run(ref, func(t *testing.T) {
			template, ok := registry.Get(ref)
			if !ok {
				t.Fatal("template missing")
			}
			components := template.DSL["components"].(map[string]any)
			_, hasExtractor := components["Extractor:AutoExtractDefault"]
			config := map[string]any{"auto_keywords": float64(8)}
			changed, err := dao.MigrateExtractorConfigForTest(config, ref, nil)
			if err != nil {
				t.Fatal(err)
			}
			if changed != hasExtractor {
				t.Fatalf("changed=%v; template has Extractor=%v", changed, hasExtractor)
			}
			if hasExtractor {
				if node, ok := config["Extractor:AutoExtractDefault"].(map[string]any); !ok || node["keywords"].(map[string]any)["top_n"] != 8 {
					t.Fatalf("config=%v", config)
				}
			} else if config["auto_keywords"] != float64(8) {
				t.Fatal("settings without target were discarded")
			}
		})
	}
}

func TestExtractorMigrationPreservesRuntimeSettings(t *testing.T) {
	registry, err := pipeline.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	template, ok := registry.Get("general")
	if !ok {
		t.Fatal("general template missing")
	}
	dsl, err := json.Marshal(template.DSL)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, input string
		check       func(*testing.T, *component.ExtractorComponent)
	}{
		{
			name:  "empty Python metadata keeps builtins",
			input: `{"enable_metadata":true,"metadata":{},"built_in_metadata":[{"key":"file_name"}]}`,
			check: func(t *testing.T, extractor *component.ExtractorComponent) {
				got := extractor.Param.Metadata
				if !got.Enabled || len(got.BuiltInMetadata) != 1 || got.BuiltInMetadata[0].Key != "file_name" {
					t.Fatalf("metadata=%#v", got)
				}
			},
		},
		{
			name:  "builtin uses active extractor despite stale node",
			input: `{"auto_keywords":8,"Extractor:Old":{"summary":{"enabled":true}}}`,
			check: func(t *testing.T, extractor *component.ExtractorComponent) {
				got := extractor.Param.Keywords
				if !got.Enabled || got.TopN != 8 {
					t.Fatalf("keywords=%#v", got)
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var config map[string]any
			if err := json.Unmarshal([]byte(tt.input), &config); err != nil {
				t.Fatal(err)
			}
			if changed, err := dao.MigrateExtractorConfigForTest(config, "general", nil); err != nil || !changed {
				t.Fatalf("changed=%v, err=%v", changed, err)
			}
			cleaned := pipeline.BuildParserConfig(dsl, config)
			extractor, err := component.NewExtractorComponent(cleaned["Extractor:AutoExtractDefault"].(map[string]any))
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, extractor.(*component.ExtractorComponent))
		})
	}
}
