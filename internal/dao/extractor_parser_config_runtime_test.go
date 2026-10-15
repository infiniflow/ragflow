package dao_test

import (
	"encoding/json"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component"
	"ragflow/internal/ingestion/pipeline"
	"ragflow/internal/service"
)

func TestMigratedExtractorConfigReachesRuntime(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.SystemSettings{}); err != nil {
		t.Fatal(err)
	}
	input := `{"auto_keywords":8,"auto_questions":2,"enable_metadata":true,"metadata":[{"key":"author","type":"string"}],"built_in_metadata":[{"key":"file_name","type":"string"}],"Extractor:AutoExtractDefault":{"tags":{"top_n":5,"tag_file_id":"tag-file"},"keywords":{"system_prompt":"custom keywords"},"summary":{"enabled":true,"system_prompt":"custom summary"}}}`
	if err := db.Exec(`INSERT INTO knowledgebase (id,tenant_id,name,embd_id,created_by,parser_id,parser_config) VALUES (?,?,?,?,?,?,?)`, "kb-1", "tenant-1", "dataset", "embedding", "user-1", "general", input).Error; err != nil {
		t.Fatal(err)
	}
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
	got := extractorFromMigratedConfig(t, config).Param
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
			tt.check(t, extractorFromMigratedConfig(t, config))
		})
	}
}

func extractorFromMigratedConfig(t *testing.T, config map[string]any) *component.ExtractorComponent {
	t.Helper()
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
	return c.(*component.ExtractorComponent)
}
