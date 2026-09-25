package service

import (
	"context"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

func rebaseTestDSL(chunkTokenSize int, llmID string) entity.JSONMap {
	return entity.JSONMap{
		"components": map[string]any{
			"GeneralChunker:SixApplesFall": map[string]any{
				"obj": map[string]any{
					"component_name": "GeneralChunker",
					"params": map[string]any{
						"chunk_token_size": float64(chunkTokenSize),
					},
				},
			},
			"Compiler:Compose": map[string]any{
				"obj": map[string]any{
					"component_name": "Compiler",
					"params": map[string]any{
						"llm_id": llmID,
					},
				},
			},
		},
	}
}

func TestRebaseBoundPipelineParserConfigsUpdatesBoundRows(t *testing.T) {
	db := setupServiceTestDB(t)
	originalDB := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = originalDB })

	pipelineID := "pipe-rebase-1"
	unboundPipeline := "pipe-other"
	snapshot := entity.JSONMap{
		"GeneralChunker:SixApplesFall": map[string]any{
			"chunk_token_size":   512.0,
			"overlapped_percent": 0.2,
		},
		"Compiler:Compose": map[string]any{
			"llm_id": "old-model@inst@provider",
		},
	}
	if err := db.Create(&entity.Knowledgebase{
		ID:           "kb-bound",
		TenantID:     "t1",
		Name:         "bound",
		Permission:   "me",
		CreatedBy:    "t1",
		PipelineID:   &pipelineID,
		ParserConfig: snapshot,
	}).Error; err != nil {
		t.Fatalf("create bound kb: %v", err)
	}
	if err := db.Create(&entity.Knowledgebase{
		ID:           "kb-unbound",
		TenantID:     "t1",
		Name:         "unbound",
		Permission:   "me",
		CreatedBy:    "t1",
		PipelineID:   &unboundPipeline,
		ParserConfig: snapshot,
	}).Error; err != nil {
		t.Fatalf("create unbound kb: %v", err)
	}
	docName := "doc.txt"
	if err := db.Create(&entity.Document{
		ID:           "doc-bound",
		KbID:         "kb-bound",
		Name:         &docName,
		PipelineID:   &pipelineID,
		ParserConfig: snapshot,
	}).Error; err != nil {
		t.Fatalf("create bound doc: %v", err)
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		return rebaseBoundPipelineParserConfigs(
			t.Context(), tx, pipelineID,
			rebaseTestDSL(512, "old-model@inst@provider"),
			rebaseTestDSL(64, "new-model@inst@provider"),
		)
	})
	if err != nil {
		t.Fatalf("rebase: %v", err)
	}

	var boundKB entity.Knowledgebase
	if err := db.Where("id = ?", "kb-bound").First(&boundKB).Error; err != nil {
		t.Fatalf("load bound kb: %v", err)
	}
	chunker := boundKB.ParserConfig["GeneralChunker:SixApplesFall"].(map[string]any)
	if _, exists := chunker["chunk_token_size"]; exists {
		t.Errorf("bound kb: seeded chunk_token_size must follow the edited DSL, got %v", boundKB.ParserConfig)
	}
	if got := chunker["overlapped_percent"]; got != 0.2 {
		t.Errorf("bound kb: user override must survive, got %v", boundKB.ParserConfig)
	}
	if _, exists := boundKB.ParserConfig["Compiler:Compose"]; exists {
		t.Errorf("bound kb: compiler entry seeded from old default llm must be dropped so the new llm applies, got %v", boundKB.ParserConfig)
	}

	var boundDoc entity.Document
	if err := db.Where("id = ?", "doc-bound").First(&boundDoc).Error; err != nil {
		t.Fatalf("load bound doc: %v", err)
	}
	docChunker := boundDoc.ParserConfig["GeneralChunker:SixApplesFall"].(map[string]any)
	if _, exists := docChunker["chunk_token_size"]; exists {
		t.Errorf("bound doc: seeded chunk_token_size must follow the edited DSL, got %v", boundDoc.ParserConfig)
	}

	var unboundKB entity.Knowledgebase
	if err := db.Where("id = ?", "kb-unbound").First(&unboundKB).Error; err != nil {
		t.Fatalf("load unbound kb: %v", err)
	}
	if _, exists := unboundKB.ParserConfig["Compiler:Compose"]; !exists {
		t.Errorf("unbound kb must be untouched, got %v", unboundKB.ParserConfig)
	}
}

func TestRebaseBoundPipelineParserConfigsSkipsUnchangedDSL(t *testing.T) {
	db := setupServiceTestDB(t)
	originalDB := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = originalDB })

	pipelineID := "pipe-rebase-2"
	if err := db.Create(&entity.Knowledgebase{
		ID:         "kb-bound",
		TenantID:   "t1",
		Name:       "bound",
		Permission: "me",
		CreatedBy:  "t1",
		PipelineID: &pipelineID,
		ParserConfig: entity.JSONMap{
			"GeneralChunker:SixApplesFall": map[string]any{"chunk_token_size": 512.0},
		},
	}).Error; err != nil {
		t.Fatalf("create bound kb: %v", err)
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		return rebaseBoundPipelineParserConfigs(
			context.Background(), tx, pipelineID,
			rebaseTestDSL(512, ""), rebaseTestDSL(512, ""),
		)
	})
	if err != nil {
		t.Fatalf("rebase: %v", err)
	}

	var boundKB entity.Knowledgebase
	if err := db.Where("id = ?", "kb-bound").First(&boundKB).Error; err != nil {
		t.Fatalf("load bound kb: %v", err)
	}
	if _, exists := boundKB.ParserConfig["GeneralChunker:SixApplesFall"]; !exists {
		t.Errorf("unchanged DSL must not rewrite stored config, got %v", boundKB.ParserConfig)
	}
}
