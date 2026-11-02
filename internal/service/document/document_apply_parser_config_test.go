package document

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func TestApplyDatasetParserConfigReplacesSettingsAndPreservesContent(t *testing.T) {
	db := setupConcurrentServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 205, 42, 7)
	insertTestKB(t, "kb-other", "tenant-1", 1, 0, 0)
	config := generalParserConfig()
	config["GeneralChunker:SixApplesFall"] = map[string]any{"chunk_token_size": 256}
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Updates(map[string]any{
		"parser_id": "general", "parser_config": config,
	}).Error; err != nil {
		t.Fatal(err)
	}
	for i := range 205 {
		insertNamedTestDoc(t, fmt.Sprintf("doc-%03d", i), "kb-1", "report.txt", 42, 7)
	}
	insertNamedTestDoc(t, "other", "kb-other", "report.txt", 0, 0)
	if err := db.Model(&entity.Document{}).Where("kb_id = ?", "kb-1").Updates(map[string]any{
		"parser_config":       entity.JSONMap{"FileOnly:Old": map[string]any{"value": "remove"}},
		"pipeline_version_id": "old-version", "thumbnail": "keep-thumbnail",
		"progress_msg": "keep-progress", "content_hash": "keep-hash",
	}).Error; err != nil {
		t.Fatal(err)
	}
	before, err := dao.NewDocumentDAO().GetByID(t.Context(), db, "doc-000")
	if err != nil {
		t.Fatal(err)
	}
	svc := testDocumentService(t)
	result, code, err := svc.ApplyDatasetParserConfig(documentTestContext(t.Context()), "user-1", "kb-1")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("apply: code=%d err=%v", code, err)
	}
	if result.TotalDocuments != 205 || result.UpdatedDocuments != 205 || result.UnchangedDocuments != 0 {
		t.Fatalf("unexpected counts: %+v", result)
	}
	var docs []entity.Document
	if err := db.Where("kb_id = ?", "kb-1").Find(&docs).Error; err != nil {
		t.Fatal(err)
	}
	kb, err := dao.NewKnowledgebaseDAO().GetByID(t.Context(), db, "kb-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, doc := range docs {
		if doc.ParserID != "general" || doc.PipelineID != nil || doc.PipelineVersionID != nil || !reflect.DeepEqual(doc.ParserConfig, kb.ParserConfig) {
			t.Fatalf("document settings differ from dataset: %+v", doc)
		}
		if doc.ChunkNum != before.ChunkNum || doc.TokenNum != before.TokenNum || doc.Progress != before.Progress ||
			*doc.Thumbnail != *before.Thumbnail || *doc.ProgressMsg != *before.ProgressMsg || *doc.ContentHash != *before.ContentHash {
			t.Fatalf("parsed content changed: %+v", doc)
		}
	}
	other, err := dao.NewDocumentDAO().GetByID(t.Context(), db, "other")
	if err != nil || other.ParserID != "naive" || len(other.ParserConfig) != 0 {
		t.Fatalf("other dataset changed: doc=%+v err=%v", other, err)
	}
	result, code, err = svc.ApplyDatasetParserConfig(documentTestContext(t.Context()), "user-1", "kb-1")
	if err != nil || code != common.CodeSuccess || result.UpdatedDocuments != 0 || result.UnchangedDocuments != 205 {
		t.Fatalf("repeat: result=%+v code=%d err=%v", result, code, err)
	}
	var taskCount int64
	if err := db.Model(&entity.IngestionTask{}).Count(&taskCount).Error; err != nil || taskCount != 0 {
		t.Fatalf("apply scheduled ingestion: count=%d err=%v", taskCount, err)
	}
}

func TestApplyDatasetParserConfigRollsBackAllBatches(t *testing.T) {
	for _, failure := range []string{common.CREATED, common.SCHEDULED, common.RUNNING, common.STOPPING, "unknown", "write failure"} {
		t.Run(failure, func(t *testing.T) {
			db := setupConcurrentServiceTestDB(t)
			pushServiceDB(t, db)
			insertTestKB(t, "kb-1", "tenant-1", 201, 0, 0)
			if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Updates(map[string]any{
				"parser_id": "general", "parser_config": generalParserConfig(),
			}).Error; err != nil {
				t.Fatal(err)
			}
			for i := range 201 {
				insertNamedTestDoc(t, fmt.Sprintf("doc-%03d", i), "kb-1", "report.txt", 0, 0)
			}
			wantCode := common.CodeConflict
			if failure == "write failure" {
				wantCode = common.CodeServerError
				if err := db.Exec(`CREATE TRIGGER reject_parser_update BEFORE UPDATE OF parser_config ON document
                    WHEN OLD.id = 'doc-200' BEGIN SELECT RAISE(ABORT, 'write failure'); END`).Error; err != nil {
					t.Fatal(err)
				}
			} else {
				insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-200", "kb-1", failure)
			}
			result, code, err := testDocumentService(t).ApplyDatasetParserConfig(documentTestContext(t.Context()), "user-1", "kb-1")
			if err == nil || code != wantCode || result != nil {
				t.Fatalf("result=%+v code=%d err=%v", result, code, err)
			}
			var changed int64
			if err := db.Model(&entity.Document{}).Where("parser_id <> ?", "naive").Count(&changed).Error; err != nil || changed != 0 {
				t.Fatalf("partial updates survived rollback: count=%d err=%v", changed, err)
			}
		})
	}
}

func TestApplyDatasetParserConfigResolvesMixedFileTypes(t *testing.T) {
	db := setupConcurrentServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 3, 0, 0)
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Updates(map[string]any{
		"parser_id": "manual", "parser_config": entity.JSONMap{
			"Parser:HipSignsRhyme": map[string]any{"pdf": map[string]any{}, "text&code": map[string]any{}},
		},
	}).Error; err != nil {
		t.Fatal(err)
	}
	files := []struct{ id, name, filetype, parserID string }{
		{"text", "notes.txt", "doc", "manual"},
		{"image", "photo.png", "visual", "picture"},
		{"slides", "deck.pptx", "doc", "presentation"},
	}
	for _, f := range files {
		insertNamedTestDoc(t, f.id, "kb-1", f.name, 0, 0)
		if err := db.Model(&entity.Document{}).Where("id = ?", f.id).Update("type", f.filetype).Error; err != nil {
			t.Fatal(err)
		}
	}
	_, code, err := testDocumentService(t).ApplyDatasetParserConfig(documentTestContext(t.Context()), "user-1", "kb-1")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("apply: code=%d err=%v", code, err)
	}
	for _, f := range files {
		doc, err := dao.NewDocumentDAO().GetByID(t.Context(), db, f.id)
		if err != nil || doc.ParserID != f.parserID {
			t.Fatalf("file=%s parser=%s err=%v", f.name, doc.ParserID, err)
		}
	}
}

func TestApplyDatasetParserConfigAuthorizationAndEmptyDataset(t *testing.T) {
	db := setupConcurrentServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 0, 0, 0)
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Updates(map[string]any{
		"parser_id": "general", "parser_config": generalParserConfig(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := testDocumentService(t)
	for _, id := range []string{"kb-1", "missing"} {
		result, code, err := svc.ApplyDatasetParserConfig(documentTestActorContext(t.Context(), "outsider", "tenant-other"), "outsider", id)
		if err == nil || code == common.CodeSuccess || result != nil {
			t.Fatalf("unauthorized apply: result=%+v code=%d err=%v", result, code, err)
		}
	}
	result, code, err := svc.ApplyDatasetParserConfig(documentTestContext(t.Context()), "user-1", "kb-1")
	if err != nil || code != common.CodeSuccess || result.TotalDocuments != 0 {
		t.Fatalf("empty dataset: result=%+v code=%d err=%v", result, code, err)
	}
	ctx, cancel := context.WithCancel(documentTestContext(t.Context()))
	cancel()
	if _, _, err := svc.ApplyDatasetParserConfig(ctx, "user-1", "kb-1"); err == nil {
		t.Fatal("canceled request succeeded")
	}
	if _, _, err := svc.ApplyDatasetParserConfig(t.Context(), "user-1", " "); err == nil || !strings.Contains(err.Error(), "dataset_id") {
		t.Fatalf("empty dataset ID: %v", err)
	}
}

func TestApplyDatasetParserConfigCopiesPipelineVersion(t *testing.T) {
	for _, missingVersion := range []bool{false, true} {
		t.Run(fmt.Sprintf("missing=%t", missingVersion), func(t *testing.T) {
			db := setupConcurrentServiceTestDB(t)
			pushServiceDB(t, db)
			if err := db.AutoMigrate(&entity.UserCanvasVersion{}); err != nil {
				t.Fatal(err)
			}
			insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
			insertNamedTestDoc(t, "doc-1", "kb-1", "report.txt", 0, 0)
			const canvasID = "1234567890abcdef1234567890abcdef"
			insertTestCanvas(t, canvasID, "user-1")
			if err := db.Model(&entity.UserCanvas{}).Where("id = ?", canvasID).Update("canvas_category", "dataflow_canvas").Error; err != nil {
				t.Fatal(err)
			}
			config := entity.JSONMap{"Parser:Shared": map[string]any{"text&code": map[string]any{}, "value": "saved"}}
			if !missingVersion {
				if err := db.Create(&entity.UserCanvasVersion{
					ID: "v-new", UserCanvasID: canvasID,
					DSL: entity.JSONMap{"components": map[string]any{
						"Parser:Shared": map[string]any{"obj": map[string]any{
							"component_name": "Parser", "params": config["Parser:Shared"],
						}},
					}},
				}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Updates(map[string]any{
				"parser_id": "", "pipeline_id": canvasID, "pipeline_version_id": "v-new", "parser_config": config,
			}).Error; err != nil {
				t.Fatal(err)
			}
			_, code, err := testDocumentService(t).ApplyDatasetParserConfig(documentTestContext(t.Context()), "user-1", "kb-1")
			doc, readErr := dao.NewDocumentDAO().GetByID(t.Context(), db, "doc-1")
			if readErr != nil {
				t.Fatal(readErr)
			}
			if missingVersion {
				if err == nil || code == common.CodeSuccess || doc.ParserID != "naive" || doc.PipelineID != nil {
					t.Fatalf("invalid version changed document: doc=%+v code=%d err=%v", doc, code, err)
				}
				return
			}
			if err != nil || code != common.CodeSuccess || doc.ParserID != "" || doc.PipelineID == nil || *doc.PipelineID != canvasID ||
				doc.PipelineVersionID == nil || *doc.PipelineVersionID != "v-new" || !reflect.DeepEqual(doc.ParserConfig, config) {
				t.Fatalf("pipeline settings not copied: doc=%+v code=%d err=%v", doc, code, err)
			}
		})
	}
}
