package document

import (
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

func TestUpdateDatasetDocumentChunkMethodAcceptsCurrentTemplates(t *testing.T) {
	for _, method := range []string{"general", "audio"} {
		t.Run(method, func(t *testing.T) {
			db := setupServiceTestDB(t)
			pushServiceDB(t, db)
			insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
			insertNamedTestDoc(t, "doc-1", "kb-1", "report.docx", 0, 0)
			resp, code, err := testDocumentService(t).UpdateDatasetDocument(documentTestContext(t.Context()), "user-1", "kb-1", "doc-1", &UpdateDatasetDocumentRequest{
				ChunkMethod: &method,
			}, map[string]bool{"chunk_method": true})
			if err != nil || code != common.CodeSuccess {
				t.Fatalf("select %s: code=%d err=%v", method, code, err)
			}
			doc, err := dao.NewDocumentDAO().GetByID(t.Context(), db, "doc-1")
			if err != nil {
				t.Fatal(err)
			}
			if resp.ParserID != method || doc.ParserID != method {
				t.Fatalf("selection not saved: response=%q stored=%q want=%q", resp.ParserID, doc.ParserID, method)
			}
		})
	}
}

func TestUpdateDatasetDocumentUsesEffectiveParserForConfig(t *testing.T) {
	for _, changeMethod := range []bool{false, true} {
		name := "document selection"
		if changeMethod {
			name = "chunk method switch"
		}
		t.Run(name, func(t *testing.T) {
			db := setupServiceTestDB(t)
			pushServiceDB(t, db)
			insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
			insertNamedTestDoc(t, "doc-1", "kb-1", "report.docx", 0, 0)
			req := &UpdateDatasetDocumentRequest{ParserConfig: map[string]any{
				"ManualChunker:NineInsectsFind": map[string]any{"hierarchy": 2},
				"GeneralChunker:SixApplesFall":  map[string]any{"chunk_token_size": 123},
			}}
			present := map[string]bool{"parser_config": true}
			if changeMethod {
				req.ChunkMethod = new("manual")
				req.ParserID, req.PipelineID = new(""), new("")
				present["chunk_method"] = true
				present["parser_id"], present["pipeline_id"] = true, true
			} else if err := db.Model(&entity.Document{}).Where("id = ?", "doc-1").Update("parser_id", "manual").Error; err != nil {
				t.Fatal(err)
			}
			_, code, err := testDocumentService(t).UpdateDatasetDocument(documentTestContext(t.Context()), "user-1", "kb-1", "doc-1", req, present)
			if err != nil || code != common.CodeSuccess {
				t.Fatalf("update: code=%d err=%v", code, err)
			}
			doc, err := dao.NewDocumentDAO().GetByID(t.Context(), db, "doc-1")
			if err != nil {
				t.Fatal(err)
			}
			chunker, ok := doc.ParserConfig["ManualChunker:NineInsectsFind"].(map[string]any)
			if doc.ParserID != "manual" || !ok || chunker["hierarchy"] != float64(2) {
				t.Fatalf("target template config lost: parser=%q config=%#v", doc.ParserID, doc.ParserConfig)
			}
			if _, ok := doc.ParserConfig["GeneralChunker:SixApplesFall"]; ok {
				t.Fatalf("config retained a node from the dataset template: %#v", doc.ParserConfig)
			}
		})
	}
}

func TestUpdateDatasetDocumentKeepsDocumentCanvasForConfigPatch(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertNamedTestDoc(t, "doc-1", "kb-1", "report.docx", 0, 0)
	const pipelineID = "1234567890abcdef1234567890abcdef"
	if err := db.Create(&entity.UserCanvas{
		ID: pipelineID, UserID: "user-1", TenantID: new("tenant-1"), Permission: string(entity.TenantPermissionMe),
		DSL: entity.JSONMap{
			"components": map[string]any{
				"Parser:custom": map[string]any{"obj": map[string]any{
					"component_name": "Parser", "params": map[string]any{},
				}},
			},
		},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&entity.Document{}).Where("id = ?", "doc-1").Update("pipeline_id", pipelineID).Error; err != nil {
		t.Fatal(err)
	}
	_, code, err := testDocumentService(t).UpdateDatasetDocument(documentTestContext(t.Context()), "user-1", "kb-1", "doc-1", &UpdateDatasetDocumentRequest{
		ParserConfig: map[string]any{"Parser:custom": map[string]any{"vlm": map[string]any{"llm_id": "vision-model"}}},
	}, map[string]bool{"parser_config": true})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("update: code=%d err=%v", code, err)
	}
	doc, err := dao.NewDocumentDAO().GetByID(t.Context(), db, "doc-1")
	if err != nil {
		t.Fatal(err)
	}
	parser, ok := doc.ParserConfig["Parser:custom"].(map[string]any)
	if !ok || !reflect.DeepEqual(parser["vlm"], map[string]any{"llm_id": "vision-model"}) || doc.PipelineID == nil || *doc.PipelineID != pipelineID {
		t.Fatalf("document canvas config lost: %#v", doc)
	}
}

func TestUpdateDatasetDocumentRejectsParserErrorsBeforeWrites(t *testing.T) {
	for _, failure := range []string{"unknown builtin", "unknown builtin without config", "empty canvas", "malformed canvas", "invalid selection", "knowledge_graph", "tag"} {
		t.Run(failure, func(t *testing.T) {
			db := setupServiceTestDB(t)
			pushServiceDB(t, db)
			insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
			insertNamedTestDoc(t, "doc-1", "kb-1", "old.docx", 0, 0)
			insertTestFile(t, "file-1", "folder-1", "old.docx", new("old.docx"))
			insertTestFile2Document(t, "f2d-1", "file-1", "doc-1")
			req := &UpdateDatasetDocumentRequest{
				Name: new("new.docx"), ParseType: new(1), ParserID: new("missing-parser"),
				MetaFields:   map[string]any{"author": "must-not-save"},
				ParserConfig: map[string]any{"Parser:HipSignsRhyme": map[string]any{"vlm": map[string]any{"llm_id": "must-not-save"}}},
			}
			present := map[string]bool{"name": true, "parse_type": true, "parser_id": true, "parser_config": true, "meta_fields": true}
			wantError := "unknown builtin parser_id"
			switch failure {
			case "knowledge_graph", "tag":
				req.ParseType, req.ParserID, req.ChunkMethod = nil, nil, &failure
				delete(present, "parse_type")
				delete(present, "parser_id")
				present["chunk_method"] = true
			case "unknown builtin without config":
				req.ParserConfig = nil
				delete(present, "parser_config")
			case "empty canvas", "malformed canvas":
				const pipelineID = "1234567890abcdef1234567890abcdef"
				dsl := entity.JSONMap{}
				wantError = "has no DSL"
				if failure == "malformed canvas" {
					dsl["graph"] = map[string]any{}
					wantError = "DSL has no components"
				}
				if err := db.Create(&entity.UserCanvas{
					ID: pipelineID, UserID: "user-1", TenantID: new("tenant-1"), Permission: string(entity.TenantPermissionMe), DSL: dsl,
				}).Error; err != nil {
					t.Fatal(err)
				}
				req.ParseType, req.ParserID, req.PipelineID = new(2), nil, new(pipelineID)
				delete(present, "parser_id")
				present["pipeline_id"] = true
			case "invalid selection":
				req.ParseType = new(2)
				wantError = "parser_id must not be set"
			}
			engine := newMetadataDocEngine(map[string]map[string]any{"doc-1": {"author": "original"}}, map[string]string{"doc-1": "kb-1"})
			svc := testDocumentService(t)
			svc.docEngine = engine
			svc.metadataSvc = service.NewMetadataServiceForTest(dao.NewKnowledgebaseDAO(), engine)
			_, code, err := svc.UpdateDatasetDocument(documentTestContext(t.Context()), "user-1", "kb-1", "doc-1", req, present)
			if code != common.CodeDataError || err == nil || !strings.Contains(err.Error(), wantError) {
				t.Fatalf("expected %q, got code=%d err=%v", wantError, code, err)
			}
			doc, err := dao.NewDocumentDAO().GetByID(t.Context(), db, "doc-1")
			if err != nil {
				t.Fatal(err)
			}
			if doc.Name == nil || *doc.Name != "old.docx" || doc.ParserID != "naive" || (doc.PipelineID != nil && *doc.PipelineID != "") || len(doc.ParserConfig) != 0 {
				t.Fatalf("rejected request changed document: %#v", doc)
			}
			file, err := dao.NewFileDAO().GetByID(t.Context(), db, "file-1")
			if err != nil || file.Name != "old.docx" {
				t.Fatalf("rejected request changed file: file=%#v err=%v", file, err)
			}
			if !reflect.DeepEqual(engine.records["doc-1"], map[string]any{"author": "original"}) {
				t.Fatalf("rejected request changed metadata: %#v", engine.records)
			}
		})
	}
}

func TestBatchUpdateDocumentStatusReturnsPermissionErrorWhenAllDenied(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Update("permission", string(entity.TenantPermissionMe)).Error; err != nil {
		t.Fatalf("make dataset private: %v", err)
	}
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	valid := string(entity.StatusValid)
	if err := db.Create(&entity.UserTenant{
		ID: "member-tenant-1", UserID: "member", TenantID: "tenant-1", Role: "normal", Status: &valid,
	}).Error; err != nil {
		t.Fatalf("create tenant member: %v", err)
	}

	svc := testDocumentService(t)
	ctx := documentTestActorContext(t.Context(), "member", "tenant-1")
	_, code, err := svc.BatchUpdateDocumentStatus(ctx, "member", "kb-1", "0", []string{"doc-1"})
	if err == nil {
		t.Fatal("expected permission error for an inaccessible document")
	}
	if code != common.CodeForbidden {
		t.Fatalf("code = %d, want %d; error: %v", code, common.CodeForbidden, err)
	}
}

func TestBatchUpdateDocumentStatusRetainsPartialFailure(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Update("permission", string(entity.TenantPermissionMe)).Error; err != nil {
		t.Fatalf("make dataset private: %v", err)
	}
	insertTestDoc(t, "owner-doc", "kb-1", 0, 0)
	insertTestDoc(t, "member-doc", "kb-1", 0, 0)
	if err := db.Model(&entity.Document{}).Where("id = ?", "member-doc").Update("created_by", "member").Error; err != nil {
		t.Fatalf("make one document member-owned: %v", err)
	}
	valid := string(entity.StatusValid)
	if err := db.Create(&entity.UserTenant{
		ID: "member-tenant-1", UserID: "member", TenantID: "tenant-1", Role: "normal", Status: &valid,
	}).Error; err != nil {
		t.Fatalf("create tenant member: %v", err)
	}

	svc := testDocumentService(t)
	ctx := documentTestActorContext(t.Context(), "member", "tenant-1")
	_, code, err := svc.BatchUpdateDocumentStatus(ctx, "member", "kb-1", "1", []string{"member-doc", "owner-doc"})
	if err == nil {
		t.Fatal("expected a partial failure")
	}
	if code != common.CodeServerError || err.Error() != "partial failure" {
		t.Fatalf("result code/error = %d/%v, want partial failure", code, err)
	}
}
