package document

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

func TestUpdateDatasetDocumentParentChildPatchUsesRequestedFields(t *testing.T) {
	for _, tc := range []struct {
		name     string
		enabled  bool
		children []any
		patch    map[string]any
		want     []any
	}{
		{"enable", false, []any{}, map[string]any{"parent_child": map[string]any{"use_parent_child": true}}, []any{"|"}},
		{"disable", true, []any{"|"}, map[string]any{"parent_child": map[string]any{"use_parent_child": false}}, []any{}},
		{"delimiter", true, []any{"|"}, map[string]any{"parent_child": map[string]any{"children_delimiter": "!"}}, []any{"!"}},
		{"explicit children", true, []any{"|"}, map[string]any{"parent_child": map[string]any{"children_delimiter": "!"}, "children_delimiters": []any{"?"}}, []any{"?"}},
		{"unrelated patch", false, []any{"|"}, map[string]any{"chunk_token_size": 128}, []any{"|"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupServiceTestDB(t)
			pushServiceDB(t, db)
			insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
			insertNamedTestDoc(t, "doc-1", "kb-1", "report.txt", 0, 0)
			if err := db.Model(&entity.Document{}).Where("id = ?", "doc-1").Update("parser_config", entity.JSONMap{
				"GeneralChunker:SixApplesFall": map[string]any{
					"children_delimiters": tc.children,
					"parent_child":        map[string]any{"use_parent_child": tc.enabled, "children_delimiter": "|"},
				},
			}).Error; err != nil {
				t.Fatal(err)
			}
			resp, code, err := testDocumentService(t).UpdateDatasetDocument(documentTestContext(t.Context()), "user-1", "kb-1", "doc-1", &UpdateDatasetDocumentRequest{
				ParserConfig: map[string]any{"GeneralChunker:SixApplesFall": tc.patch},
			}, map[string]bool{"parser_config": true})
			if code != common.CodeSuccess || err != nil {
				t.Fatalf("update: code=%d err=%v", code, err)
			}
			doc, err := dao.NewDocumentDAO().GetByID(t.Context(), db, "doc-1")
			if err != nil {
				t.Fatal(err)
			}
			for _, config := range []map[string]any{resp.ParserConfig, doc.ParserConfig} {
				chunker, _ := config["GeneralChunker:SixApplesFall"].(map[string]any)
				if !reflect.DeepEqual(chunker["children_delimiters"], tc.want) {
					t.Fatalf("children_delimiters=%v, want=%v", chunker["children_delimiters"], tc.want)
				}
			}
		})
	}
}

func TestUpdateDatasetDocumentConfigPatchPreservesOmittedSettings(t *testing.T) {
	for _, selection := range []string{"omitted", "same builtin", "same canvas"} {
		t.Run(selection, func(t *testing.T) {
			db := setupServiceTestDB(t)
			pushServiceDB(t, db)
			insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
			insertNamedTestDoc(t, "doc-1", "kb-1", "report.docx", 0, 0)
			config := entity.JSONMap{
				"Parser:HipSignsRhyme": map[string]any{
					"enable_vision_enhancement": true,
					"vlm":                       map[string]any{"llm_id": "old-model", "system_prompt": "keep this prompt"},
				},
				"GeneralChunker:SixApplesFall": map[string]any{"chunk_token_size": 123},
				"Parser:Removed":               map[string]any{"pdf": map[string]any{}},
			}
			updates := map[string]any{"parser_id": "general", "parser_config": config}
			req := &UpdateDatasetDocumentRequest{ParserConfig: map[string]any{
				"Parser:HipSignsRhyme": map[string]any{"vlm": map[string]any{"llm_id": "new-model"}},
			}}
			present := map[string]bool{"parser_config": true}
			switch selection {
			case "same builtin":
				req.ParseType, req.ParserID = new(1), new("general")
				present["parse_type"], present["parser_id"] = true, true
			case "same canvas":
				const pipelineID = "1234567890abcdef1234567890abcdef"
				insertTestCanvas(t, pipelineID, "user-1")
				updates["pipeline_id"] = pipelineID
				req.ParseType, req.PipelineID = new(2), new(pipelineID)
				present["parse_type"], present["pipeline_id"] = true, true
			}
			if err := db.Model(&entity.Document{}).Where("id = ?", "doc-1").Updates(updates).Error; err != nil {
				t.Fatal(err)
			}
			resp, code, err := testDocumentService(t).UpdateDatasetDocument(documentTestContext(t.Context()), "user-1", "kb-1", "doc-1", req, present)
			if err != nil || code != common.CodeSuccess {
				t.Fatalf("update: code=%d err=%v", code, err)
			}
			doc, err := dao.NewDocumentDAO().GetByID(t.Context(), db, "doc-1")
			if err != nil {
				t.Fatal(err)
			}
			for _, saved := range []map[string]any{resp.ParserConfig, doc.ParserConfig} {
				parser, _ := saved["Parser:HipSignsRhyme"].(map[string]any)
				vlm, _ := parser["vlm"].(map[string]any)
				chunker, _ := saved["GeneralChunker:SixApplesFall"].(map[string]any)
				if parser["enable_vision_enhancement"] != true || vlm["llm_id"] != "new-model" || vlm["system_prompt"] != "keep this prompt" || chunker["chunk_token_size"] != float64(123) {
					t.Fatalf("partial update lost settings: %#v", saved)
				}
				if _, exists := saved["Parser:Removed"]; exists {
					t.Fatalf("stale component survived cleaning: %#v", saved)
				}
			}
		})
	}
}

func TestUpdateDatasetDocumentRejectsUnsupportedFileFamilyBeforeWrites(t *testing.T) {
	for _, filename := range []string{"image.png", "deck.pptx", "deck.pages", "DECK.DPS", "deck.dpt"} {
		for _, selection := range []string{"chunk_method", "parser_id"} {
			for _, withConfig := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/config=%t", filename, selection, withConfig), func(t *testing.T) {
					db := setupServiceTestDB(t)
					pushServiceDB(t, db)
					insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
					insertNamedTestDoc(t, "doc-1", "kb-1", filename, 0, 0)
					insertTestFile(t, "file-1", "folder-1", filename, &filename)
					insertTestFile2Document(t, "f2d-1", "file-1", "doc-1")
					if filename == "image.png" {
						if err := db.Model(&entity.Document{}).Where("id = ?", "doc-1").Updates(map[string]any{
							"type": "visual", "parser_id": "picture", "parser_config": pictureParserConfig(),
						}).Error; err != nil {
							t.Fatal(err)
						}
					}
					before, err := dao.NewDocumentDAO().GetByID(t.Context(), db, "doc-1")
					if err != nil {
						t.Fatal(err)
					}
					req := &UpdateDatasetDocumentRequest{Name: new("renamed." + strings.SplitN(filename, ".", 2)[1]), MetaFields: map[string]any{"author": "changed"}}
					present := map[string]bool{"name": true, "meta_fields": true, selection: true}
					if selection == "parser_id" {
						req.ParseType, req.ParserID = new(1), new("manual")
						present["parse_type"] = true
					} else {
						req.ChunkMethod = new("manual")
					}
					if withConfig {
						req.ParserConfig = map[string]any{"Parser:HipSignsRhyme": map[string]any{"enable_vision_enhancement": true}}
						present["parser_config"] = true
					}
					engine := newMetadataDocEngine(map[string]map[string]any{"doc-1": {"author": "original"}}, map[string]string{"doc-1": "kb-1"})
					svc := testDocumentService(t)
					svc.docEngine = engine
					svc.metadataSvc = service.NewMetadataServiceForTest(dao.NewKnowledgebaseDAO(), engine)
					_, code, err := svc.UpdateDatasetDocument(documentTestContext(t.Context()), "user-1", "kb-1", "doc-1", req, present)
					if code != common.CodeDataError || err == nil || !strings.Contains(err.Error(), "does not support") {
						t.Fatalf("expected unsupported file rejection: code=%d err=%v", code, err)
					}
					after, err := dao.NewDocumentDAO().GetByID(t.Context(), db, "doc-1")
					if err != nil || !reflect.DeepEqual(before, after) {
						t.Fatalf("rejected request changed document: before=%#v after=%#v err=%v", before, after, err)
					}
					file, err := dao.NewFileDAO().GetByID(t.Context(), db, "file-1")
					if err != nil || file.Name != filename {
						t.Fatalf("rejected request changed file: file=%#v err=%v", file, err)
					}
					if !reflect.DeepEqual(engine.records["doc-1"], map[string]any{"author": "original"}) {
						t.Fatalf("rejected request changed metadata: %#v", engine.records)
					}
				})
			}
		}
	}
}

func TestUpdateDatasetDocumentParserAliasPreservesConfig(t *testing.T) {
	for _, current := range []string{"naive", "general"} {
		for _, selection := range []string{"chunk_method", "parser_id"} {
			for _, withConfig := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/config=%t", current, selection, withConfig), func(t *testing.T) {
					db := setupServiceTestDB(t)
					pushServiceDB(t, db)
					insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
					insertNamedTestDoc(t, "doc-1", "kb-1", "report.docx", 0, 0)
					if err := db.Model(&entity.Document{}).Where("id = ?", "doc-1").Updates(map[string]any{
						"parser_id":     current,
						"parser_config": entity.JSONMap{"GeneralChunker:SixApplesFall": map[string]any{"chunk_token_size": 123}},
					}).Error; err != nil {
						t.Fatal(err)
					}
					target := "general"
					if current == "general" {
						target = "naive"
					}
					req := &UpdateDatasetDocumentRequest{}
					present := map[string]bool{selection: true}
					if selection == "parser_id" {
						req.ParseType, req.ParserID = new(1), &target
						present["parse_type"] = true
					} else {
						req.ChunkMethod = &target
					}
					if withConfig {
						req.ParserConfig = map[string]any{"Parser:HipSignsRhyme": map[string]any{"enable_vision_enhancement": true}}
						present["parser_config"] = true
					}
					resp, code, err := testDocumentService(t).UpdateDatasetDocument(documentTestContext(t.Context()), "user-1", "kb-1", "doc-1", req, present)
					if err != nil || code != common.CodeSuccess {
						t.Fatalf("update: code=%d err=%v", code, err)
					}
					doc, err := dao.NewDocumentDAO().GetByID(t.Context(), db, "doc-1")
					if err != nil {
						t.Fatal(err)
					}
					if resp.ParserID != target || doc.ParserID != target {
						t.Fatalf("parser ID not saved: response=%q stored=%q want=%q", resp.ParserID, doc.ParserID, target)
					}
					for _, config := range []map[string]any{resp.ParserConfig, doc.ParserConfig} {
						chunker, _ := config["GeneralChunker:SixApplesFall"].(map[string]any)
						if chunker["chunk_token_size"] != float64(123) {
							t.Fatalf("alias switch reset chunk size: %#v", config)
						}
						if withConfig {
							parser, _ := config["Parser:HipSignsRhyme"].(map[string]any)
							if parser["enable_vision_enhancement"] != true {
								t.Fatalf("alias switch lost config patch: %#v", config)
							}
						}
					}
				})
			}
		}
	}
}

func TestUpdateDatasetDocumentSelectionWithoutConfig(t *testing.T) {
	for _, selection := range []string{"chunk_method", "parser_id"} {
		for _, current := range []string{"general", "manual"} {
			t.Run(selection+"/"+current, func(t *testing.T) {
				db := setupServiceTestDB(t)
				pushServiceDB(t, db)
				insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
				insertNamedTestDoc(t, "doc-1", "kb-1", "report.docx", 0, 0)
				config := entity.JSONMap{"ManualChunker:NineInsectsFind": map[string]any{"hierarchy": float64(7)}}
				if current == "general" {
					config = entity.JSONMap{"GeneralChunker:SixApplesFall": map[string]any{"chunk_token_size": 123}}
				}
				if err := db.Model(&entity.Document{}).Where("id = ?", "doc-1").Updates(map[string]any{"parser_id": current, "parser_config": config}).Error; err != nil {
					t.Fatal(err)
				}
				req := &UpdateDatasetDocumentRequest{}
				present := map[string]bool{selection: true}
				if selection == "parser_id" {
					req.ParseType, req.ParserID = new(1), new("manual")
					present["parse_type"] = true
				} else {
					req.ChunkMethod = new("manual")
				}
				_, code, err := testDocumentService(t).UpdateDatasetDocument(documentTestContext(t.Context()), "user-1", "kb-1", "doc-1", req, present)
				if err != nil || code != common.CodeSuccess {
					t.Fatalf("update: code=%d err=%v", code, err)
				}
				doc, err := dao.NewDocumentDAO().GetByID(t.Context(), db, "doc-1")
				if err != nil {
					t.Fatal(err)
				}
				if doc.ParserID != "manual" {
					t.Fatalf("parser=%q", doc.ParserID)
				}
				if current == "manual" {
					if !reflect.DeepEqual(doc.ParserConfig, config) {
						t.Fatalf("unchanged selection reset config: %#v", doc.ParserConfig)
					}
					return
				}
				if _, ok := doc.ParserConfig["ManualChunker:NineInsectsFind"]; !ok {
					t.Fatalf("new template config missing: %#v", doc.ParserConfig)
				}
				if _, ok := doc.ParserConfig["GeneralChunker:SixApplesFall"]; ok {
					t.Fatalf("old template config retained: %#v", doc.ParserConfig)
				}
				parser, ok := doc.ParserConfig["Parser:HipSignsRhyme"].(map[string]any)
				if !ok || parser["docx"] == nil {
					t.Fatalf("target file family missing: %#v", doc.ParserConfig)
				}
			})
		}
	}
}

func TestUpdateDatasetDocumentChunkMethodAcceptsCurrentTemplates(t *testing.T) {
	for _, file := range []struct{ method, name string }{{"general", "report.docx"}, {"audio", "recording.mp3"}} {
		t.Run(file.method, func(t *testing.T) {
			db := setupServiceTestDB(t)
			pushServiceDB(t, db)
			insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
			insertNamedTestDoc(t, "doc-1", "kb-1", file.name, 0, 0)
			resp, code, err := testDocumentService(t).UpdateDatasetDocument(documentTestContext(t.Context()), "user-1", "kb-1", "doc-1", &UpdateDatasetDocumentRequest{
				ChunkMethod: &file.method,
			}, map[string]bool{"chunk_method": true})
			if err != nil || code != common.CodeSuccess {
				t.Fatalf("select %s: code=%d err=%v", file.method, code, err)
			}
			doc, err := dao.NewDocumentDAO().GetByID(t.Context(), db, "doc-1")
			if err != nil {
				t.Fatal(err)
			}
			if resp.ParserID != file.method || doc.ParserID != file.method {
				t.Fatalf("selection not saved: response=%q stored=%q want=%q", resp.ParserID, doc.ParserID, file.method)
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
					"component_name": "Parser", "params": map[string]any{"docx": map[string]any{}},
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
	for _, failure := range []string{"unknown builtin", "unknown builtin without config", "empty canvas", "malformed canvas", "invalid selection", "knowledge_graph", "tag", "invalid chunk with builtin", "invalid chunk with canvas"} {
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
			case "invalid chunk with builtin", "invalid chunk with canvas":
				req.ParserID, req.ChunkMethod = new("general"), new("does-not-exist")
				present["chunk_method"] = true
				wantError = "unknown builtin parser_id: does-not-exist"
				if failure == "invalid chunk with canvas" {
					const pipelineID = "1234567890abcdef1234567890abcdef"
					insertTestCanvas(t, pipelineID, "user-1")
					req.ParseType, req.ParserID, req.PipelineID = new(2), nil, new(pipelineID)
					delete(present, "parser_id")
					present["pipeline_id"] = true
				}
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
