package dao

import (
	"context"
	"reflect"
	"testing"

	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestMigrateGeneralChunkerConfigCopiesSupportedParams(t *testing.T) {
	config := map[string]interface{}{
		"TokenChunker:SixApplesFall": map[string]interface{}{
			"chunk_token_size":    768,
			"delimiters":          []interface{}{"\n", ";"},
			"children_delimiters": []interface{}{"|"},
			"image_context_size":  4,
			"overlapped_percent":  0.2,
			"table_context_size":  3,
			"delimiter_mode":      "delimiter",
			"outputs":             map[string]interface{}{"chunks": map[string]interface{}{}},
		},
	}

	changed := migrateGeneralChunkerConfig(config)
	if !changed {
		t.Fatal("migrateGeneralChunkerConfig reported no change")
	}
	if _, ok := config[legacyGeneralChunkerID]; ok {
		t.Fatalf("legacy component id remains: %#v", config)
	}

	want := map[string]interface{}{
		"chunk_token_size":    768,
		"delimiters":          []interface{}{"\n", ";"},
		"children_delimiters": []interface{}{"|"},
		"image_context_size":  4,
		"overlapped_percent":  0.2,
		"table_context_size":  3,
	}
	got, ok := config[currentGeneralChunkerID].(map[string]interface{})
	if !ok {
		t.Fatalf("new component params type = %T", config[currentGeneralChunkerID])
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("new component params = %#v, want %#v", got, want)
	}
}

func TestMigrateGeneralChunkerConfigKeepsExplicitNewParams(t *testing.T) {
	newParams := map[string]interface{}{"chunk_token_size": 256}
	config := map[string]interface{}{
		legacyGeneralChunkerID:  map[string]interface{}{"chunk_token_size": 768},
		currentGeneralChunkerID: newParams,
	}

	if !migrateGeneralChunkerConfig(config) {
		t.Fatal("migrateGeneralChunkerConfig reported no change")
	}
	if got := config[currentGeneralChunkerID]; !reflect.DeepEqual(got, newParams) {
		t.Fatalf("explicit new params changed: %#v", got)
	}
}

func TestMigrateGeneralChunkerConfigIgnoresOtherChunkers(t *testing.T) {
	config := map[string]interface{}{
		"TokenChunker:WarmBreadSmells": map[string]interface{}{"chunk_token_size": 128},
	}

	if migrateGeneralChunkerConfig(config) {
		t.Fatal("migration changed a non-general component")
	}
	if _, ok := config["TokenChunker:WarmBreadSmells"]; !ok {
		t.Fatal("non-general component was removed")
	}
}

func TestMigrateGeneralChunkerParserConfigsUpdatesBuiltinRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.Document{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	legacyParams := map[string]interface{}{
		"chunk_token_size": 256,
		"delimiter_mode":   "delimiter",
		"outputs":          map[string]interface{}{},
	}
	kb := &entity.Knowledgebase{
		ID:           "kb-migrate",
		TenantID:     "tenant-migrate",
		Name:         "migration",
		EmbdID:       "embedding",
		CreatedBy:    "user-migrate",
		ParserID:     "general",
		ParserConfig: entity.JSONMap{legacyGeneralChunkerID: legacyParams},
	}
	if err := db.Create(kb).Error; err != nil {
		t.Fatalf("create knowledgebase: %v", err)
	}
	doc := &entity.Document{
		ID:           "doc-migrate",
		KbID:         kb.ID,
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{legacyGeneralChunkerID: legacyParams},
		SourceType:   "local",
		Type:         "file",
		CreatedBy:    "user-migrate",
		Suffix:       "txt",
	}
	if err := db.Create(doc).Error; err != nil {
		t.Fatalf("create document: %v", err)
	}
	emptyPipelineID := ""
	emptyPipelineKB := &entity.Knowledgebase{
		ID:           "kb-empty-pipeline",
		TenantID:     "tenant-migrate",
		Name:         "empty-pipeline",
		EmbdID:       "embedding",
		CreatedBy:    "user-migrate",
		ParserID:     "general",
		PipelineID:   &emptyPipelineID,
		ParserConfig: entity.JSONMap{legacyGeneralChunkerID: legacyParams},
	}
	if err := db.Create(emptyPipelineKB).Error; err != nil {
		t.Fatalf("create empty-pipeline knowledgebase: %v", err)
	}
	emptyPipelineDoc := &entity.Document{
		ID:           "doc-empty-pipeline",
		KbID:         kb.ID,
		ParserID:     "naive",
		PipelineID:   &emptyPipelineID,
		ParserConfig: entity.JSONMap{legacyGeneralChunkerID: legacyParams},
		SourceType:   "local",
		Type:         "file",
		CreatedBy:    "user-migrate",
		Suffix:       "txt",
	}
	if err := db.Create(emptyPipelineDoc).Error; err != nil {
		t.Fatalf("create empty-pipeline document: %v", err)
	}
	customPipelineID := "canvas-migrate"
	customKB := &entity.Knowledgebase{
		ID:           "kb-custom",
		TenantID:     "tenant-migrate",
		Name:         "custom",
		EmbdID:       "embedding",
		CreatedBy:    "user-migrate",
		ParserID:     "naive",
		PipelineID:   &customPipelineID,
		ParserConfig: entity.JSONMap{legacyGeneralChunkerID: legacyParams},
	}
	if err := db.Create(customKB).Error; err != nil {
		t.Fatalf("create custom knowledgebase: %v", err)
	}

	if err := migrateGeneralChunkerParserConfigs(context.Background(), db); err != nil {
		t.Fatalf("migrate parser configs: %v", err)
	}

	var gotKB entity.Knowledgebase
	if err := db.First(&gotKB, "id = ?", kb.ID).Error; err != nil {
		t.Fatalf("reload knowledgebase: %v", err)
	}
	var gotDoc entity.Document
	if err := db.First(&gotDoc, "id = ?", doc.ID).Error; err != nil {
		t.Fatalf("reload document: %v", err)
	}
	var gotEmptyPipelineKB entity.Knowledgebase
	if err := db.First(&gotEmptyPipelineKB, "id = ?", emptyPipelineKB.ID).Error; err != nil {
		t.Fatalf("reload empty-pipeline knowledgebase: %v", err)
	}
	var gotEmptyPipelineDoc entity.Document
	if err := db.First(&gotEmptyPipelineDoc, "id = ?", emptyPipelineDoc.ID).Error; err != nil {
		t.Fatalf("reload empty-pipeline document: %v", err)
	}
	for name, config := range map[string]entity.JSONMap{
		"knowledgebase":                gotKB.ParserConfig,
		"document":                     gotDoc.ParserConfig,
		"empty-pipeline knowledgebase": gotEmptyPipelineKB.ParserConfig,
		"empty-pipeline document":      gotEmptyPipelineDoc.ParserConfig,
	} {
		if _, ok := config[legacyGeneralChunkerID]; ok {
			t.Errorf("%s retained legacy component id: %#v", name, config)
		}
		params, ok := config[currentGeneralChunkerID].(map[string]interface{})
		if !ok {
			t.Errorf("%s new component params type = %T", name, config[currentGeneralChunkerID])
			continue
		}
		if params["chunk_token_size"] != float64(256) && params["chunk_token_size"] != 256 {
			t.Errorf("%s migrated params = %#v", name, params)
		}
		if _, ok := params["delimiter_mode"]; ok {
			t.Errorf("%s retained delimiter_mode: %#v", name, params)
		}
	}
	var gotCustomKB entity.Knowledgebase
	if err := db.First(&gotCustomKB, "id = ?", customKB.ID).Error; err != nil {
		t.Fatalf("reload custom knowledgebase: %v", err)
	}
	if _, ok := gotCustomKB.ParserConfig[legacyGeneralChunkerID]; !ok {
		t.Fatalf("custom pipeline parser config was unexpectedly migrated: %#v", gotCustomKB.ParserConfig)
	}
}
