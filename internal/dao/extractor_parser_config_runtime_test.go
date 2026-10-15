package dao_test

import (
	"encoding/json"
	"os"
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
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.SystemSettings{}); err != nil {
		t.Fatal(err)
	}
	input := `{"auto_keywords":8,"auto_questions":2,"metadata":{"enabled":true,"metadata":[{"key":"author","type":"string"}],"built_in_metadata":[{"key":"file_name","type":"string"}]},"Extractor:AutoExtractDefault":{"tags":{"top_n":5,"tag_file_id":"tag-file"},"keywords_sys_prompt":"custom keywords","enable_summary":true,"summary":{"system_prompt":"custom summary"}}}`
	if err := db.Exec(`INSERT INTO knowledgebase (id, tenant_id, name, embd_id, created_by, parser_id, parser_config) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"kb-1", "tenant-1", "dataset", "embedding", "user-1", "general", input).Error; err != nil {
		t.Fatal(err)
	}
	if err := dao.MigrateExtractorParserConfigsForTest(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := db.Table("knowledgebase").Where("id = ?", "kb-1").Pluck("parser_config", &raw).Error; err != nil {
		t.Fatal(err)
	}
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		t.Fatal(err)
	}
	template, err := os.ReadFile("../ingestion/pipeline/template/ingestion_pipeline_general.json")
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		DSL json.RawMessage `json:"dsl"`
	}
	if err := json.Unmarshal(template, &envelope); err != nil {
		t.Fatal(err)
	}
	cleaned := service.ApplyComponentScopedParserConfig(pipeline.BuildParserConfig(envelope.DSL, config), "tenant-llm")
	params := cleaned["Extractor:AutoExtractDefault"].(map[string]interface{})
	runtimeComponent, err := component.NewExtractorComponent(params)
	if err != nil {
		t.Fatal(err)
	}
	got := runtimeComponent.(*component.ExtractorComponent).Param
	if !got.Keywords.Enabled || got.Keywords.TopN != 8 || got.Keywords.SystemPrompt != "custom keywords" {
		t.Fatalf("keywords = %#v", got.Keywords)
	}
	if !got.Questions.Enabled || got.Questions.TopN != 2 {
		t.Fatalf("questions = %#v", got.Questions)
	}
	if !got.Tags.Enabled || got.Tags.TopN != 5 || got.Tags.TagFileID != "tag-file" {
		t.Fatalf("tags = %#v", got.Tags)
	}
	if !got.Summary.Enabled || got.Summary.SystemPrompt != "custom summary" {
		t.Fatalf("summary = %#v", got.Summary)
	}
	if !got.Metadata.Enabled || len(got.Metadata.Metadata) != 1 || got.Metadata.Metadata[0].Key != "author" || len(got.Metadata.BuiltInMetadata) != 1 || got.Metadata.BuiltInMetadata[0].Key != "file_name" {
		t.Fatalf("metadata = %#v", got.Metadata)
	}
	if got.LLMID != "tenant-llm" {
		t.Fatalf("llm_id = %q", got.LLMID)
	}
}
